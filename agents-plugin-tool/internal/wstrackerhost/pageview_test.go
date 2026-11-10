package wstrackerhost

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wstracker"
)

// blockingRunner is the real git runner whose fetches, once armed, wait
// until released: a remote that hangs mid-fetch.
type blockingRunner struct {
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func newBlockingRunner() *blockingRunner {
	return &blockingRunner{entered: make(chan struct{}, 16), release: make(chan struct{})}
}

func (r *blockingRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	if r.armed.Load() && slices.Contains(args, "fetch") {
		r.entered <- struct{}{}
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return batchRunner{}.RunGit(ctx, root, args...)
}

const pageWait = 200 * time.Millisecond

// within runs ViewWithin and fails when it takes much longer than wait.
func within(t *testing.T, h *Host, store string) *StoreView {
	t.Helper()
	start := time.Now()
	v, err := h.ViewWithin(context.Background(), store, 0, pageWait)
	if err != nil {
		t.Fatalf("view within %s: %v", store, err)
	}
	if took := time.Since(start); took > pageWait+time.Second {
		t.Fatalf("ViewWithin took %v, past its %v bound", took, pageWait)
	}
	return v
}

// TestViewWithinServesLastStateWhileFetchBlocked: while the daemon's own
// fetch holds the store, a bounded read returns the last folded state
// within its bound, the change callback runs once the fetch is released,
// and the next read is fresh.
func TestViewWithinServesLastStateWhileFetchBlocked(t *testing.T) {
	w := newWorld(t)
	runner := newBlockingRunner()
	daemon, err := New(Options{Root: filepath.Join(w.dir, "ada"), Clock: w.clock, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	defer daemon.Close()
	w.register(daemon, "team", "ada", &wstracker.Profile{Given: "Ada"})
	other := sameRoot(t, w, "ada")
	var changes counter
	stop := daemon.StartWatch(time.Hour, changes.inc) // only the late-reload signal fires
	defer stop()

	first := within(t, daemon, "team") // no folded state yet: waits
	id := createItem(t, other, "team", "written before the fetch")

	runner.armed.Store(true)
	checked := make(chan error, 1)
	go func() {
		_, err := daemon.Check(context.Background(), "team")
		checked <- err
	}()
	<-runner.entered

	stale := within(t, daemon, "team")
	if hasItem(stale, id) {
		t.Fatal("the blocked read must serve the last folded state, not a fresh one")
	}
	if len(stale.Items) != len(first.Items) {
		t.Fatalf("stale view has %d items, the last folded state %d", len(stale.Items), len(first.Items))
	}
	if changes.get() != 0 {
		t.Fatal("the change callback ran before the reload landed")
	}

	runner.armed.Store(false)
	close(runner.release)
	if err := <-checked; err != nil {
		t.Fatalf("check: %v", err)
	}
	waitFor(t, 10*time.Second, "the change callback after the late reload", func() bool { return changes.get() > 0 })
	time.Sleep(50 * time.Millisecond)
	if n := changes.get(); n != 1 {
		t.Fatalf("the late reload must call the change callback once, got %d", n)
	}
	if fresh := within(t, daemon, "team"); !hasItem(fresh, id) {
		t.Fatal("the read after the change callback must be fresh")
	}
}

// TestViewWithinReturnsWhileAnotherProcessHoldsTheStoreLock: another
// process holding the store's ledger lock (a push or fetch of its own)
// does not hold a bounded read past its bound, while an unbounded View
// (the MCP path) waits and then returns fresh state.
func TestViewWithinReturnsWhileAnotherProcessHoldsTheStoreLock(t *testing.T) {
	w := newWorld(t)
	daemon := w.machine("ada")
	res := w.register(daemon, "team", "ada", &wstracker.Profile{Given: "Ada"})
	other := sameRoot(t, w, "ada")
	mcpHost := sameRoot(t, w, "ada")
	var changes counter
	stop := daemon.StartWatch(time.Hour, changes.inc)
	defer stop()

	_ = within(t, daemon, "team")
	_ = view(t, mcpHost, "team")
	id := createItem(t, other, "team", "before the lock")

	lock := flock.New(filepath.Join(res.Entry.Dir, "lock"))
	if ok, err := lock.TryLock(); err != nil || !ok {
		t.Fatalf("take the store lock: %v %v", ok, err)
	}
	unlocked := false
	defer func() {
		if !unlocked {
			lock.Unlock()
		}
	}()

	if stale := within(t, daemon, "team"); hasItem(stale, id) {
		t.Fatal("the bounded read must serve the last folded state while the lock is held")
	}

	type result struct {
		v   *StoreView
		err error
	}
	mcp := make(chan result, 1)
	go func() {
		v, err := mcpHost.View(context.Background(), "team", 0)
		mcp <- result{v, err}
	}()
	select {
	case r := <-mcp:
		t.Fatalf("the unbounded View returned while the lock was held: %v", r.err)
	case <-time.After(300 * time.Millisecond):
	}

	lock.Unlock()
	unlocked = true
	r := <-mcp
	if r.err != nil {
		t.Fatalf("unbounded view: %v", r.err)
	}
	if !hasItem(r.v, id) {
		t.Fatal("the unbounded View must return fresh state once the lock is released")
	}
	waitFor(t, 10*time.Second, "the change callback after the late reload", func() bool { return changes.get() > 0 })
	if fresh := within(t, daemon, "team"); !hasItem(fresh, id) {
		t.Fatal("the read after the change callback must be fresh")
	}
}

// TestSharedViewRedactsApart: Redact on one reader's copy of a shared view
// leaves the shared view alone, so two pages may redact concurrently.
func TestSharedViewRedactsApart(t *testing.T) {
	remote := "https://user:secret@example.com/r.git"
	v := &StoreView{View: &wstracker.View{}, Store: StoreInfo{Remote: &remote},
		Sync: SyncInfo{Error: "fetch https://user:secret@example.com/r.git", WikiWarnings: []string{"w: https://user:secret@example.com/r.git"}}}
	a := v.shared()
	a.Redact()
	if *v.Store.Remote != remote || v.Sync.Error != "fetch "+remote || v.Sync.WikiWarnings[0] != "w: "+remote {
		t.Fatalf("redacting a copy changed the shared view: %+v", v.Sync)
	}
	if *a.Store.Remote == remote || a.Sync.WikiWarnings[0] == "w: "+remote {
		t.Fatal("the copy was not redacted")
	}
	if a.View != v.View {
		t.Fatal("the folded view is shared, not copied")
	}
	if (*StoreView)(nil).shared() != nil {
		t.Fatal("nil stays nil")
	}
}

// TestViewWithinAfterClose: a closed host refuses a bounded read.
func TestViewWithinAfterClose(t *testing.T) {
	w := newWorld(t)
	h := w.machine("ada")
	w.register(h, "team", "ada", &wstracker.Profile{Given: "Ada"})
	_ = within(t, h, "team")
	h.Close()
	if _, err := h.ViewWithin(context.Background(), "team", 0, pageWait); err == nil {
		t.Fatal("a closed host must refuse a read")
	}
}

// TestViewWithinQueuesBehindAReloadThatStartedReading: a read arriving
// after the in-flight reload began re-reading the store directory waits for
// the next reload, not the one that may have missed a write; both run, and
// the late signal fires once they land.
func TestViewWithinQueuesBehindAReloadThatStartedReading(t *testing.T) {
	w := newWorld(t)
	daemon := w.machine("ada")
	res := w.register(daemon, "team", "ada", &wstracker.Profile{Given: "Ada"})
	var changes counter
	stop := daemon.StartWatch(time.Hour, changes.inc)
	defer stop()
	_ = within(t, daemon, "team")

	lock := flock.New(filepath.Join(res.Entry.Dir, "lock"))
	if ok, err := lock.TryLock(); err != nil || !ok {
		t.Fatalf("take the store lock: %v %v", ok, err)
	}
	_ = within(t, daemon, "team") // starts a reload that blocks inside Reload
	st, err := daemon.store("team")
	if err != nil {
		t.Fatal(err)
	}
	st.viewMu.Lock()
	first, started := st.cur, st.cur != nil && st.cur.read
	st.viewMu.Unlock()
	if !started {
		lock.Unlock()
		t.Fatal("the blocked reload must have started reading")
	}
	_ = within(t, daemon, "team")
	st.viewMu.Lock()
	queued := st.next != nil && st.cur == first
	st.viewMu.Unlock()
	if !queued {
		lock.Unlock()
		t.Fatal("a read after the reload started reading must queue the next reload")
	}
	// A sync-state change while the reads wait, so the reloads that land
	// fold a view other than the one the late reads were handed.
	if err := writeJSON(filepath.Join(res.Entry.Dir, syncStateFile), syncState{Error: "offline meanwhile"}); err != nil {
		lock.Unlock()
		t.Fatal(err)
	}
	lock.Unlock()
	waitFor(t, 10*time.Second, "both reloads to land", func() bool {
		st.viewMu.Lock()
		defer st.viewMu.Unlock()
		return st.cur == nil && st.next == nil
	})
	if changes.get() == 0 {
		t.Fatal("the late reloads must signal")
	}
}

// slowRunner is the real git runner made slow: once armed, every git run
// first waits delay, so a reload outlasts any page read's wait (a machine
// where spawning git is slow).
type slowRunner struct {
	armed atomic.Bool
	delay time.Duration
}

func (r *slowRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	if r.armed.Load() {
		select {
		case <-time.After(r.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return batchRunner{}.RunGit(ctx, root, args...)
}

// TestLateReloadSignalsOnlyAChangedView: on a host whose every reload
// outlasts the page read's wait, a page that re-reads on each change signal
// settles. A late reload that folds the same view does not call the change
// callback; one that folds a changed view does, once, including a
// journal-only write by another process (same refs, one new uncommitted
// event) and a host-sync.json change; and the re-read that signal causes
// does not keep the loop going.
func TestLateReloadSignalsOnlyAChangedView(t *testing.T) {
	w := newWorld(t)
	runner := &slowRunner{delay: 80 * time.Millisecond}
	daemon, err := New(Options{Root: filepath.Join(w.dir, "ada"), Clock: w.clock, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	defer daemon.Close()
	res := w.register(daemon, "team", "ada", &wstracker.Profile{Given: "Ada"})
	other := sameRoot(t, w, "ada")
	st, err := daemon.store("team")
	if err != nil {
		t.Fatal(err)
	}

	const wait = 10 * time.Millisecond
	var changes counter
	// inflight counts the page's reads still running (a WaitGroup would
	// race: a signal adds a read while settle waits).
	var inflight, reads atomic.Int64
	read := func() {
		reads.Add(1)
		inflight.Add(1)
		go func() {
			defer inflight.Add(-1)
			_, _ = daemon.ViewWithin(context.Background(), "team", 0, wait)
		}()
	}
	// The page: every change signal makes it read again.
	stop := daemon.StartWatch(time.Hour, func() { changes.inc(); read() })
	defer stop()
	settle := func(what string) {
		t.Helper()
		for i := 0; i < 2; i++ {
			waitFor(t, 20*time.Second, what, func() bool {
				st.viewMu.Lock()
				defer st.viewMu.Unlock()
				return inflight.Load() == 0 && st.cur == nil && st.next == nil
			})
			time.Sleep(50 * time.Millisecond) // a signal racing the check
		}
	}

	if _, err := daemon.ViewWithin(context.Background(), "team", 0, wait); err != nil {
		t.Fatal(err) // no folded state yet: waits
	}
	runner.armed.Store(true)

	// Unchanged: the late reload lands with the view the read was handed.
	base := daemon.ReloadStats()
	read()
	settle("the unchanged late reload")
	stats := daemon.ReloadStats()
	if stats.Late-base.Late == 0 {
		t.Fatal("the read must have stopped waiting (the reload is slower than its wait)")
	}
	if n := changes.reset(); n != 0 {
		t.Fatalf("a late reload of an unchanged view called the change callback %d times", n)
	}
	if stats.LateChanged != base.LateChanged {
		t.Fatal("an unchanged late reload must not count as a change")
	}

	cases := []struct {
		name  string
		write func()
	}{
		{"journal-only write by another process", func() {
			refs := storeRefs(t, res.Entry.Dir)
			createItem(t, other, "team", "uncommitted elsewhere")
			if storeRefs(t, res.Entry.Dir) != refs {
				t.Fatal("the write must stay in the journal (same refs)")
			}
		}},
		{"host-sync.json change", func() {
			if err := writeJSON(filepath.Join(res.Entry.Dir, syncStateFile), syncState{Error: "remote went away"}); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		tc.write()
		base := daemon.ReloadStats()
		before := reads.Load()
		read()
		settle("the late reload after a " + tc.name)
		if n := changes.reset(); n != 1 {
			t.Fatalf("%s: the late reload must call the change callback once, got %d", tc.name, n)
		}
		stats := daemon.ReloadStats()
		if got := stats.LateChanged - base.LateChanged; got != 1 {
			t.Fatalf("%s: %d late reloads counted as a change, want 1", tc.name, got)
		}
		// The first read and the one re-read the signal caused.
		if got := reads.Load() - before; got != 2 {
			t.Fatalf("%s: the page read %d times, want 2", tc.name, got)
		}
		if got := stats.Reloads - base.Reloads; got > 3 {
			t.Fatalf("%s: %d reloads ran; the loop did not settle", tc.name, got)
		}
	}
	fresh, err := daemon.ViewWithin(context.Background(), "team", 0, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Sync.Error != "remote went away" {
		t.Fatalf("the settled view must carry the sync change, got %q", fresh.Sync.Error)
	}
}

// storeRefs lists the refs of a store's repository.
func storeRefs(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", filepath.Join(dir, "repo.git"), "for-each-ref").CombinedOutput()
	if err != nil {
		t.Fatalf("for-each-ref: %v: %s", err, out)
	}
	return string(out)
}

// TestViewIdentityIgnoresNow: two views that differ only in View.Now have
// one identity; any other difference changes it.
func TestViewIdentityIgnoresNow(t *testing.T) {
	a := &StoreView{View: &wstracker.View{Now: "2026-01-01T00:00:00Z"}}
	b := &StoreView{View: &wstracker.View{Now: "2026-01-02T00:00:00Z"}}
	if string(viewIdentity(a)) != string(viewIdentity(b)) {
		t.Fatal("views differing only in Now must share an identity")
	}
	if a.View.Now != "2026-01-01T00:00:00Z" {
		t.Fatal("the identity must not change the view")
	}
	c := &StoreView{View: &wstracker.View{Now: a.View.Now}, Sync: SyncInfo{Error: "x"}}
	if string(viewIdentity(a)) == string(viewIdentity(c)) {
		t.Fatal("a sync change must change the identity")
	}
	r := &reload{}
	r.serve(a)
	if r.differs(b) {
		t.Fatal("a landed view equal but for Now does not differ")
	}
	if !r.differs(c) {
		t.Fatal("a landed view with another sync state differs")
	}
	r.serve(nil)
	if !r.differs(b) {
		t.Fatal("a late read handed nothing always counts as a difference")
	}
}

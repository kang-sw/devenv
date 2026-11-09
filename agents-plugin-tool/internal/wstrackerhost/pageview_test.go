package wstrackerhost

import (
	"context"
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
	if took := time.Since(start); took > pageWait+2*time.Second {
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

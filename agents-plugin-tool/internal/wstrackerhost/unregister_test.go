package wstrackerhost

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
)

// shortUnregisterWait makes a busy store refuse quickly for the test.
func shortUnregisterWait(t *testing.T) {
	t.Helper()
	was := unregisterWait
	unregisterWait = 60 * time.Millisecond
	t.Cleanup(func() { unregisterWait = was })
}

// remoteRefs lists every ref of the bare remote with its value.
func (w *world) remoteRefs() string {
	w.t.Helper()
	out, err := exec.Command("git", "-C", w.remote, "for-each-ref", "--format=%(refname) %(objectname)").CombinedOutput()
	if err != nil {
		w.t.Fatalf("for-each-ref: %v: %s", err, out)
	}
	return string(out)
}

// wantRefusal fails unless err is an UnregisterRefusal of kind.
func wantRefusal(t *testing.T, err error, kind string) *UnregisterRefusal {
	t.Helper()
	var r *UnregisterRefusal
	if !errors.As(err, &r) || r.Kind != kind {
		t.Fatalf("unregister = %v, want a %q refusal", err, kind)
	}
	return r
}

// wantGone fails unless store id has neither a registry entry nor a
// directory under h's root, and no renamed-away directory is left.
func wantGone(t *testing.T, h *Host, id string) {
	t.Helper()
	if _, err := h.Entry(id); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("entry %s after unregister = %v, want ErrNotRegistered", id, err)
	}
	entries, err := os.ReadDir(h.Root())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == id || strings.HasPrefix(e.Name(), removedPrefix) {
			t.Fatalf("unregister left %s behind", e.Name())
		}
	}
}

// TestUnregisterSyncedStore: a synced store with a remote is removed from
// the machine (registry entry and directory), the remote is untouched, and
// registering it again under the same member brings it back. The second
// run renames after releasing the lock files, the order Windows needs.
func TestUnregisterSyncedStore(t *testing.T) {
	for _, underLocks := range []bool{true, false} {
		was := renameUnderLocks
		renameUnderLocks = underLocks
		t.Cleanup(func() { renameUnderLocks = was })

		w := newWorld(t)
		a := w.machine("a")
		p := &wstracker.Profile{Given: "Ada"}
		w.register(a, "team", "ada", p)
		w.register(a, "other", "ada", p)
		item := createItem(t, a, "team", "kept on the remote")
		syncStore(t, a, "team")
		refs := w.remoteRefs()

		if err := a.Unregister(context.Background(), "team", ""); err != nil {
			t.Fatalf("unregister (rename under locks: %v): %v", underLocks, err)
		}
		wantGone(t, a, "team")
		if _, err := a.View(context.Background(), "team", 0); !errors.Is(err, ErrNotRegistered) {
			t.Fatalf("view after unregister = %v", err)
		}
		if stores, _ := a.Stores(); len(stores) != 1 || stores[0].ID != "other" {
			t.Fatalf("stores after unregister = %+v, want only the other store", stores)
		}
		if got := w.remoteRefs(); got != refs {
			t.Fatalf("unregister moved the remote:\n%s\nwas\n%s", got, refs)
		}

		res := w.register(a, "team", "ada", p)
		if !res.Creator || res.Entry.Nonce == "" {
			t.Fatalf("re-register = %+v, want the creator's registration with a nonce", res)
		}
		if !hasItem(view(t, a, "team"), item) {
			t.Fatal("the re-registered store must hold what the remote kept")
		}
	}
}

// TestUnregisterRefusesUnpushed: own events the remote does not hold refuse
// the unregister, whatever confirm says, and a sync clears the refusal.
func TestUnregisterRefusesUnpushed(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	item := createItem(t, a, "team", "not pushed")
	ctx := context.Background()

	r := wantRefusal(t, a.Unregister(ctx, "team", "team"), UnregisterUnpushed)
	if !strings.Contains(r.Reason, "1 own event") || !strings.Contains(r.Reason, "sync the store first") {
		t.Fatalf("refusal = %q", r.Reason)
	}
	if !hasItem(view(t, a, "team"), item) {
		t.Fatal("a refused unregister must leave the store as it was")
	}
	syncStore(t, a, "team")
	if err := a.Unregister(ctx, "team", ""); err != nil {
		t.Fatalf("unregister after the sync: %v", err)
	}
	wantGone(t, a, "team")
}

// TestUnregisterCountsAnotherProcessWrite: an event another process
// appended to the shared clone after this one last read it still refuses.
func TestUnregisterCountsAnotherProcessWrite(t *testing.T) {
	w := newWorld(t)
	a := w.machine("ada")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	syncStore(t, a, "team")
	other := sameRoot(t, w, "ada")
	createItem(t, other, "team", "from another process")
	wantRefusal(t, a.Unregister(context.Background(), "team", ""), UnregisterUnpushed)
}

// TestUnregisterRefusesUnpushedWiki: a wiki only this clone holds refuses
// the unregister by name, though no event is pending.
func TestUnregisterRefusesUnpushedWiki(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	ctx := context.Background()
	edit := wsledger.WikiEdit{Wiki: "docs", NewWiki: true, Op: wsledger.WikiOpWrite, Path: "home", Content: "x\n"}
	if _, err := a.WikiSave(ctx, "team", WikiSave{Edit: edit}); err != nil {
		t.Fatal(err)
	}
	syncStore(t, a, "team")
	unpublish(t, w, a, "docs")

	r := wantRefusal(t, a.Unregister(ctx, "team", ""), UnregisterUnpushed)
	if !strings.Contains(r.Reason, "unpushed wiki changes (docs)") {
		t.Fatalf("refusal = %q", r.Reason)
	}
}

// TestUnregisterStoreWithoutRemote: a store with no remote holds its only
// copy here, so it goes only when the confirmation names it; its unpushed
// events do not refuse it.
func TestUnregisterStoreWithoutRemote(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	ctx := context.Background()
	if _, err := a.Register(ctx, RegisterRequest{ID: "solo", Member: "ada", Profile: &wstracker.Profile{Given: "Ada"}}); err != nil {
		t.Fatal(err)
	}
	createItem(t, a, "solo", "only copy")
	for _, confirm := range []string{"", "team", "Solo"} {
		r := wantRefusal(t, a.Unregister(ctx, "solo", confirm), UnregisterNeedsConfirm)
		if !strings.Contains(r.Reason, "permanently") {
			t.Fatalf("refusal = %q, want it to say the data is lost permanently", r.Reason)
		}
	}
	if _, err := a.Entry("solo"); err != nil {
		t.Fatalf("a refused unregister removed the store: %v", err)
	}
	if err := a.Unregister(ctx, "solo", "solo"); err != nil {
		t.Fatalf("confirmed unregister: %v", err)
	}
	wantGone(t, a, "solo")
}

// TestUnregisterRefusesWhileBusy: a store lock held elsewhere (wsledger's
// lock across another process's sync, the write lock across its write, or
// this process's own call in flight) refuses instead of waiting.
func TestUnregisterRefusesWhileBusy(t *testing.T) {
	shortUnregisterWait(t)
	w := newWorld(t)
	a := w.machine("ada")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	syncStore(t, a, "team")
	dir := filepath.Join(a.Root(), "team")
	ctx := context.Background()

	for _, name := range []string{wsledger.LockFile, writeLockFile} {
		fl := flock.New(filepath.Join(dir, name))
		if locked, err := fl.TryLock(); err != nil || !locked {
			t.Fatalf("lock %s: %v %v", name, locked, err)
		}
		wantRefusal(t, a.Unregister(ctx, "team", ""), UnregisterBusy)
		// A process that has not opened the store refuses the same way.
		wantRefusal(t, sameRoot(t, w, "ada").Unregister(ctx, "team", ""), UnregisterBusy)
		fl.Unlock()
	}

	st := a.stores["team"]
	st.mu.Lock()
	wantRefusal(t, a.Unregister(ctx, "team", ""), UnregisterBusy)
	st.mu.Unlock()

	if _, err := a.Entry("team"); err != nil {
		t.Fatalf("a refused unregister removed the store: %v", err)
	}
	if err := a.Unregister(ctx, "team", ""); err != nil {
		t.Fatalf("unregister once the locks are free: %v", err)
	}
}

// TestUnregisterNamesAStuckStore: events no sync can push refuse with the
// state named, since a "sync first" would never clear it.
func TestUnregisterNamesAStuckStore(t *testing.T) {
	for _, tc := range []struct {
		p    wsledger.PendingReport
		want string
	}{
		{wsledger.PendingReport{Count: 2, ForeignAdvance: true}, "the remote holds writes under this copy"},
		{wsledger.PendingReport{Count: 1, KeyNotRegistered: true}, "key is not registered"},
		{wsledger.PendingReport{Count: 1, IDTaken: true, KeyNotRegistered: true}, "another join took this machine's member id"},
	} {
		r := wantRefusal(t, unpushedRefusal("team", wsledger.Unpushed{Pending: tc.p}), UnregisterStuck)
		if !strings.Contains(r.Reason, tc.want) || strings.Contains(r.Reason, "sync the store first") {
			t.Fatalf("refusal for %+v = %q", tc.p, r.Reason)
		}
	}
	// A stuck flag with nothing unpushed loses nothing.
	if err := unpushedRefusal("team", wsledger.Unpushed{Pending: wsledger.PendingReport{KeyNotRegistered: true}}); err != nil {
		t.Fatalf("nothing unpushed = %v", err)
	}

	// End to end: a twin clone (the same member, key, and clone id from
	// another directory) pushed first, so this clone's own ref on the remote
	// holds commits it did not write.
	w := newWorld(t)
	a := w.machine("a")
	p := &wstracker.Profile{Given: "Ada"}
	w.register(a, "team", "ada", p)
	syncStore(t, a, "team")
	cloneID, err := os.ReadFile(filepath.Join(a.Root(), "team", "clone-id"))
	if err != nil {
		t.Fatal(err)
	}
	twinRoot := filepath.Join(w.dir, "twin")
	if err := os.MkdirAll(twinRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cp", "-R", filepath.Join(a.Root(), keysDir), filepath.Join(twinRoot, keysDir)).CombinedOutput(); err != nil {
		t.Fatalf("copy the key: %v: %s", err, out)
	}
	twin := w.machine("twin")
	w.register(twin, "team", "ada", p)
	twin.Close()
	if err := os.WriteFile(filepath.Join(twinRoot, "team", "clone-id"), cloneID, 0o600); err != nil {
		t.Fatal(err)
	}
	twin = sameRoot(t, w, "twin")
	createItem(t, a, "team", "pushed first")
	syncStore(t, a, "team")
	createItem(t, twin, "team", "cannot be pushed")
	ctx := context.Background()
	rep, _ := twin.Sync(ctx, "team")
	if !rep.Pending.ForeignAdvance || rep.Pending.Count == 0 {
		t.Fatalf("fixture: the twin's sync = %+v, want a foreign advance with pending events", rep.Pending)
	}
	r := wantRefusal(t, twin.Unregister(ctx, "team", ""), UnregisterStuck)
	if !strings.Contains(r.Reason, "the remote holds writes under this copy") {
		t.Fatalf("refusal = %q", r.Reason)
	}
}

// TestUnregisterSeenByAnotherProcess: a second Host that holds the store
// open answers "no registered store" once another process unregistered it,
// drops its handle, and leaves no file behind, whatever it runs late: a
// read, a seen mark, a sync status write, the ledger's own idle push.
func TestUnregisterSeenByAnotherProcess(t *testing.T) {
	w := newWorld(t)
	a := w.machine("ada")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	syncStore(t, a, "team")
	other := sameRoot(t, w, "ada")
	_ = view(t, other, "team")
	if err := other.MarkSeen("team", Seen{Badge: []string{"e1"}}); err != nil {
		t.Fatal(err)
	}
	st := other.stores["team"]
	ledger := st.ledger
	ctx := context.Background()

	if err := a.Unregister(ctx, "team", ""); err != nil {
		t.Fatal(err)
	}
	// Calls that had passed the registration check when the store went.
	if err := other.attempt(ctx, st, time.Second, ledger.Fetch); err == nil {
		t.Fatal("a late fetch of a removed store must fail")
	}
	if _, err := ledger.Sync(ctx); err == nil {
		t.Fatal("a late idle push of a removed store must fail")
	}
	if _, err := wstracker.Open(ctx, other.ledgerOptions(st.entry, "")); err == nil {
		t.Fatal("opening a removed store must fail")
	}
	wantGone(t, a, "team")

	if _, err := other.View(ctx, "team", 0); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("view in the other process = %v, want ErrNotRegistered", err)
	}
	if err := other.MarkSeen("team", Seen{Badge: []string{"e2"}}); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("mark seen in the other process = %v", err)
	}
	payload := []byte(`{"kind":"task","title":"late"}`)
	if _, err := other.Dispatch(ctx, "team", []wsledger.NewEvent{{Type: wstracker.TypeItemCreate, Payload: payload}}); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("write in the other process = %v", err)
	}
	if len(other.stores) != 0 || !st.retired || st.ledger != nil {
		t.Fatalf("the other process kept its handle: stores=%d retired=%v", len(other.stores), st.retired)
	}
	other.PollOnce(ctx)
	other.CheckAll(ctx)
	wantGone(t, a, "team")
}

// TestUnregisterThenReRegisterElsewhere: a second Host that holds the store
// open while another process unregisters it and registers the id again
// goes on with the new registration, not its stale ledger; and a Host
// holding a stale handle can itself register the id again.
func TestUnregisterThenReRegisterElsewhere(t *testing.T) {
	w := newWorld(t)
	a := w.machine("ada")
	p := &wstracker.Profile{Given: "Ada"}
	w.register(a, "team", "ada", p)
	syncStore(t, a, "team")
	other := sameRoot(t, w, "ada")
	oldStream := view(t, other, "team").Viewer.Stream
	stale := other.stores["team"]
	ctx := context.Background()

	if err := a.Unregister(ctx, "team", ""); err != nil {
		t.Fatal(err)
	}
	w.register(a, "team", "ada", p)

	v := view(t, other, "team")
	if v.Viewer.Stream == oldStream {
		t.Fatalf("the other process still reads its stale clone %s", oldStream)
	}
	if !stale.retired || other.stores["team"] == stale {
		t.Fatal("the stale handle must be retired")
	}
	item := createItem(t, other, "team", "into the new registration")
	if !hasItem(view(t, a, "team"), item) {
		t.Fatal("the write must land in the new registration")
	}
	if entries, _ := filepath.Glob(filepath.Join(a.Root(), "team", "journal.*.jsonl")); len(entries) != 1 {
		t.Fatalf("journals in the new clone = %v, want only its own", entries)
	}

	// The same from a Host whose own handle is the stale one.
	syncStore(t, a, "team")
	if err := a.Unregister(ctx, "team", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Register(ctx, RegisterRequest{ID: "team", Remote: w.remote, Member: "ada", Profile: p}); err != nil {
		t.Fatalf("register over a stale handle: %v", err)
	}
	if !hasItem(view(t, other, "team"), item) {
		t.Fatal("the store registered again must hold what the remote kept")
	}
}

// TestSweepRemovedLeftovers: a renamed-away directory an interrupted
// unregister left is ignored by the registry and deleted at the next Host
// start and the next unregister.
func TestSweepRemovedLeftovers(t *testing.T) {
	w := newWorld(t)
	a := w.machine("ada")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	syncStore(t, a, "team")
	left := func(name string) string {
		dir := filepath.Join(a.Root(), removedPrefix+name)
		if err := os.MkdirAll(filepath.Join(dir, "repo.git"), 0o700); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	first := left("gone-1")
	if stores, _ := a.Stores(); len(stores) != 1 {
		t.Fatalf("stores = %+v", stores)
	}
	sameRoot(t, w, "ada")
	if _, err := os.Stat(first); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a Host start must sweep the leftover: %v", err)
	}
	second := left("gone-2")
	if err := a.Unregister(context.Background(), "team", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(second); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an unregister must sweep the leftover: %v", err)
	}
}

// TestRelabel: the label is a machine-local registry field: it round-trips,
// reaches an open store's view, clears back to the id, and writes nothing
// to the store; a bad one is refused.
func TestRelabel(t *testing.T) {
	w := newWorld(t)
	a := w.machine("ada")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	syncStore(t, a, "team")
	other := sameRoot(t, w, "ada")
	if got := view(t, other, "team").Store.Label; got != "team" {
		t.Fatalf("label before = %q", got)
	}
	events := len(view(t, a, "team").Events)

	e, err := a.Relabel("team", "  Team Board  ")
	if err != nil || e.Label != "Team Board" || e.DisplayLabel() != "Team Board" {
		t.Fatalf("relabel = %+v, %v", e, err)
	}
	if got := view(t, other, "team").Store.Label; got != "Team Board" {
		t.Fatalf("label in another process's open store = %q", got)
	}
	if v := view(t, a, "team"); len(v.Events) != events || len(v.Sync.Unpushed) != 0 {
		t.Fatalf("relabel wrote to the store: %d events, unpushed %+v", len(v.Events), v.Sync.Unpushed)
	}
	for _, bad := range []string{"two\nlines", "tab\there", "bell\x07", strings.Repeat("x", MaxLabelLen+1)} {
		if _, err := a.Relabel("team", bad); !errors.Is(err, ErrBadLabel) {
			t.Fatalf("relabel %q = %v, want ErrBadLabel", bad, err)
		}
	}
	if e, err := a.Relabel("team", strings.Repeat("가", MaxLabelLen)); err != nil || len([]rune(e.Label)) != MaxLabelLen {
		t.Fatalf("a label of the longest length = %v", err)
	}
	if e, err := a.Relabel("team", " "); err != nil || e.Label != "" || e.DisplayLabel() != "team" {
		t.Fatalf("clearing = %+v, %v", e, err)
	}
	if got := view(t, other, "team").Store.Label; got != "team" {
		t.Fatalf("label after clearing = %q", got)
	}
	if _, err := a.Relabel("nope", "x"); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("relabel of an unregistered store = %v", err)
	}
}

// TestSetHidden: hiding is a machine-local registry flag: it round-trips,
// and a hidden store still reads, writes, and polls.
func TestSetHidden(t *testing.T) {
	w := newWorld(t)
	a, b := w.machine("a"), w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})

	e, err := a.SetHidden("team", true)
	if err != nil || !e.Hidden {
		t.Fatalf("hide = %+v, %v", e, err)
	}
	if stores, _ := a.Stores(); len(stores) != 1 || !stores[0].Hidden {
		t.Fatalf("stores = %+v, want the hidden store still listed", stores)
	}
	item := createItem(t, a, "team", "written while hidden")
	if !hasItem(view(t, a, "team"), item) {
		t.Fatal("a hidden store must still read and write")
	}
	theirs := createItem(t, b, "team", "from the other machine")
	syncStore(t, b, "team")
	a.PollOnce(context.Background())
	if !hasItem(view(t, a, "team"), theirs) {
		t.Fatal("a hidden store must still be polled")
	}
	if e, err := a.SetHidden("team", false); err != nil || e.Hidden {
		t.Fatalf("show = %+v, %v", e, err)
	}
	if _, err := a.SetHidden("nope", true); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("hiding an unregistered store = %v", err)
	}
}

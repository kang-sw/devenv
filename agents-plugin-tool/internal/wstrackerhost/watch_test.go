package wstrackerhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wstracker"
)

// counter counts change callbacks.
type counter struct{ n atomic.Int64 }

func (c *counter) inc()         { c.n.Add(1) }
func (c *counter) get() int64   { return c.n.Load() }
func (c *counter) reset() int64 { return c.n.Swap(0) }

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// sameRoot is a second Host over h's root: another process on the machine.
func sameRoot(t *testing.T, w *world, name string) *Host {
	t.Helper()
	h, err := New(Options{Root: filepath.Join(w.dir, name), Clock: w.clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

// TestWatchSeesAnotherProcessWrite: a write through another Host over the
// same root (another process's ws-mcp) calls the daemon's change callback
// within the watch interval, and the next read shows it.
func TestWatchSeesAnotherProcessWrite(t *testing.T) {
	w := newWorld(t)
	daemon := w.machine("ada")
	w.register(daemon, "team", "ada", &wstracker.Profile{Given: "Ada"})
	other := sameRoot(t, w, "ada")
	_ = view(t, daemon, "team")

	var changes counter
	stop := daemon.StartWatch(20*time.Millisecond, changes.inc)
	defer stop()

	id := createItem(t, other, "team", "from another process")
	waitFor(t, 5*time.Second, "the change callback after another process's write", func() bool { return changes.get() > 0 })
	v, err := daemon.ViewWithin(context.Background(), "team", 0, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !hasItem(v, id) {
		t.Fatalf("the read after the change must show item %s", id)
	}
}

// TestWatchIdleStoreCausesNoRepeatedChanges: reads of an idle store, which
// write host-sync.json and ui-seen.json and reload the ledger, never call
// the change callback.
func TestWatchIdleStoreCausesNoRepeatedChanges(t *testing.T) {
	w := newWorld(t)
	daemon := w.machine("ada")
	w.register(daemon, "team", "ada", &wstracker.Profile{Given: "Ada"})
	createItem(t, daemon, "team", "settled")
	syncStore(t, daemon, "team")
	_ = view(t, daemon, "team")

	var changes counter
	stop := daemon.StartWatch(10*time.Millisecond, changes.inc)
	defer stop()
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		if _, err := daemon.ViewWithin(ctx, "team", 0, time.Second); err != nil {
			t.Fatal(err)
		}
		if _, err := daemon.View(ctx, "team", 0); err != nil {
			t.Fatal(err)
		}
		if _, err := daemon.Check(ctx, "team"); err != nil {
			t.Fatal(err)
		}
		if err := daemon.MarkSeen("team", Seen{Badge: []string{"x"}}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(15 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if n := changes.get(); n != 0 {
		t.Fatalf("an idle store with repeated reads called the change callback %d times", n)
	}
}

// TestWatchSeesANewStore: a store registered after the watch started is
// watched (the registry is re-read each tick), and its appearance is a
// change too.
func TestWatchSeesANewStore(t *testing.T) {
	w := newWorld(t)
	daemon := w.machine("ada")
	var changes counter
	stop := daemon.StartWatch(10*time.Millisecond, changes.inc)
	defer stop()

	other := sameRoot(t, w, "ada")
	if _, err := other.Register(context.Background(), RegisterRequest{ID: "solo", Member: "ada", Profile: &wstracker.Profile{Given: "Ada"}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, "the change callback after a registration", func() bool { return changes.get() > 0 })
	time.Sleep(30 * time.Millisecond)
	changes.reset()
	createItem(t, other, "solo", "in the new store")
	waitFor(t, 5*time.Second, "the change callback after a write to the new store", func() bool { return changes.get() > 0 })
}

// TestStoreStampIgnoresReadSideFiles: the stamp moves with a journal append
// and not with the files a read writes.
func TestStoreStampIgnoresReadSideFiles(t *testing.T) {
	w := newWorld(t)
	h := w.machine("ada")
	res := w.register(h, "team", "ada", &wstracker.Profile{Given: "Ada"})
	dir := res.Entry.Dir
	before := storeStamp(dir)
	if err := writeJSON(filepath.Join(dir, syncStateFile), syncState{Error: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := h.MarkSeen("team", Seen{Read: []string{"y"}}); err != nil {
		t.Fatal(err)
	}
	if storeStamp(dir) != before {
		t.Fatal("a read-side file moved the store stamp")
	}
	createItem(t, h, "team", "moves it")
	if storeStamp(dir) == before {
		t.Fatal("a journal append did not move the store stamp")
	}
}

// failPushRunner is the real git runner whose pushes fail: an offline
// push after the batch has closed.
type failPushRunner struct{}

func (failPushRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	if slices.Contains(args, "push") {
		return nil, errors.New("push refused")
	}
	return batchRunner{}.RunGit(ctx, root, args...)
}

// TestWatchSeesAnotherProcessPush: a push whose batch commit was already
// stamped moves only the push-known refs, and the view's unpushed list
// reads them, so the push is a change too.
func TestWatchSeesAnotherProcessPush(t *testing.T) {
	w := newWorld(t)
	daemon := w.machine("ada")
	w.register(daemon, "team", "ada", &wstracker.Profile{Given: "Ada"})
	offline, err := New(Options{Root: filepath.Join(w.dir, "ada"), Clock: w.clock, Runner: failPushRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { offline.Close() })
	createItem(t, offline, "team", "to push")
	// The sync closes the batch into a commit; its push fails.
	_, _ = offline.Sync(context.Background(), "team")
	if v := view(t, daemon, "team"); len(v.Sync.Unpushed) == 0 || v.Sync.Unpushed[0].Open {
		t.Fatalf("the write must read as a closed, unpushed commit: %+v", v.Sync.Unpushed)
	}

	var changes counter
	stop := daemon.StartWatch(20*time.Millisecond, changes.inc)
	defer stop()
	time.Sleep(60 * time.Millisecond)
	changes.reset()
	syncStore(t, sameRoot(t, w, "ada"), "team")
	waitFor(t, 5*time.Second, "the change callback after another process's push", func() bool { return changes.get() > 0 })
	if v := view(t, daemon, "team"); len(v.Sync.Unpushed) != 0 {
		t.Fatalf("after the push the daemon must read nothing unpushed: %+v", v.Sync.Unpushed)
	}
}

// TestStoreStampStatsLooseRefs: a replaced loose ref moves the stamp by its
// time, while new loose-ref bytes under an unchanged size and time do not
// (the content is never read); packed-refs stays content-based, so new bytes
// under an unchanged size and time move it.
func TestStoreStampStatsLooseRefs(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo.git")
	heads := filepath.Join(repo, "refs", "heads")
	known := filepath.Join(repo, "refs", "ledger", "known", "origin")
	for _, d := range []string{heads, known} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeAt := func(path, data string, at time.Time) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	branch := filepath.Join(heads, "tracker")
	writeAt(branch, strings.Repeat("a", 40)+"\n", base)
	writeAt(filepath.Join(known, "tracker"), strings.Repeat("b", 40)+"\n", base)
	packed := filepath.Join(repo, "packed-refs")
	writeAt(packed, "# pack-refs with: peeled\n", base)

	var read []string
	prev := stampReadFile
	stampReadFile = func(path string) ([]byte, error) {
		read = append(read, path)
		return prev(path)
	}
	t.Cleanup(func() { stampReadFile = prev })

	before := storeStamp(dir)
	if again := storeStamp(dir); again != before {
		t.Fatal("an unchanged store moved the stamp")
	}
	// A replaced loose ref keeps its size; its modification time moves.
	writeAt(branch, strings.Repeat("c", 40)+"\n", base.Add(time.Second))
	afterRef := storeStamp(dir)
	if afterRef == before {
		t.Fatal("a replaced loose ref did not move the stamp")
	}
	// New loose-ref bytes under the same size and time: a stat stamp, so
	// no move.
	writeAt(branch, strings.Repeat("d", 40)+"\n", base.Add(time.Second))
	if storeStamp(dir) != afterRef {
		t.Fatal("loose-ref content under an unchanged size and time moved the stamp: it was read")
	}
	// packed-refs is hashed by content: same size and time, new bytes.
	writeAt(packed, "# pack-refs with: peeLED\n", base)
	if storeStamp(dir) == afterRef {
		t.Fatal("a packed-refs content change did not move the stamp")
	}
	refs := filepath.Join(repo, "refs") + string(filepath.Separator)
	for _, path := range read {
		if strings.HasPrefix(path, refs) {
			t.Fatalf("storeStamp read loose ref %s", path)
		}
	}
	if !slices.Contains(read, packed) {
		t.Fatalf("storeStamp did not read packed-refs through the seam: %q", read)
	}
}

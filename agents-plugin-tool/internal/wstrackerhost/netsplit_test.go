package wstrackerhost

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
)

// holdRunner is the real git runner with one remote command it stops at,
// once armed, until released: a remote that hangs mid-transfer. The command
// match selects is held before git runs, or, with after set, once git has
// run, so what it fetched is already in the clone while its caller still
// waits. It holds one command per arming.
type holdRunner struct {
	match   func(args []string) bool
	after   bool
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func newHoldRunner(match func(args []string) bool) *holdRunner {
	return &holdRunner{match: match, entered: make(chan struct{}, 1), release: make(chan struct{})}
}

func (r *holdRunner) hold(ctx context.Context) {
	r.entered <- struct{}{}
	select {
	case <-r.release:
	case <-ctx.Done():
	}
}

func (r *holdRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	held := r.match(args) && r.armed.CompareAndSwap(true, false)
	if held && !r.after {
		r.hold(ctx)
	}
	out, err := batchRunner{}.RunGit(ctx, root, args...)
	if held && r.after {
		r.hold(ctx)
	}
	return out, err
}

func isFetchCommand(args []string) bool { return slices.Contains(args, "fetch") }
func isPushCommand(args []string) bool  { return slices.Contains(args, "push") }

// isWikiFetch selects the last remote command of a check: by then the
// events it fetched are in the clone, not yet applied.
func isWikiFetch(args []string) bool {
	return isFetchCommand(args) && strings.HasPrefix(args[len(args)-1], "+refs/heads/wiki/")
}

// hostWith is a Host over the root of machine name whose git runs through
// runner.
func hostWith(t *testing.T, w *world, name string, runner *holdRunner) *Host {
	t.Helper()
	h, err := New(Options{Root: filepath.Join(w.dir, name), Clock: w.clock, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

// pageBound is the dashboard's bound on a page read's wait for a lock.
const pageBound = 500 * time.Millisecond

// TestLocalReadsDoNotWaitForNetwork: while a check's fetch, or a sync's
// push, waits on a remote that hangs, the store's local reads answer at
// once and fresh, in the process running it and in another one: the page's
// view, a view, a wiki read, a wiki image, and an attachment this clone
// holds. Each read carries the page's 500 ms lock bound, so one that had
// to wait for the network call (held until every read has answered) would
// answer ErrStoreBusy. A write goes through as well.
func TestLocalReadsDoNotWaitForNetwork(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		match func(args []string) bool
		run   func(h *Host) error
	}{
		{"a fetch held open in Check", isFetchCommand, func(h *Host) error {
			_, err := h.Check(ctx, "team")
			return err
		}},
		{"a push held open in Sync", isPushCommand, func(h *Host) error {
			_, err := h.Sync(ctx, "team")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			runner := newHoldRunner(tc.match)
			daemon := hostWith(t, w, "ada", runner)
			w.register(daemon, "team", "ada", &wstracker.Profile{Given: "Ada"})
			other := sameRoot(t, w, "ada")

			img := pngBytes("wiki image")
			edit := wsledger.WikiEdit{Wiki: "docs", NewWiki: true, Op: wsledger.WikiOpWrite, Path: "home", Content: "![x](attachment:" + attachmentOID(img) + ")\n"}
			if _, err := daemon.WikiSave(ctx, "team", WikiSave{Edit: edit, Images: []WikiImageInput{{Data: img}}}); err != nil {
				t.Fatal(err)
			}
			att := stageOK(t, daemon, "team", "a.png", pngBytes("attachment"))
			if _, err := createWithAttachment(t, daemon, "team", att); err != nil {
				t.Fatal(err)
			}
			syncStore(t, daemon, "team")
			_ = within(t, daemon, "team") // the state a stale page read would serve
			id := createItem(t, other, "team", "written before the network call")

			runner.armed.Store(true)
			done := make(chan error, 1)
			go func() { done <- tc.run(daemon) }()
			<-runner.entered

			// A wait far past the page's: a read that had to wait for the
			// held call would come back stale, without the item.
			if v, err := daemon.ViewWithin(ctx, "team", 0, 20*time.Second); err != nil || !hasItem(v, id) {
				t.Fatalf("page view under a held network call: fresh=%v err=%v, want the fresh view", err == nil && hasItem(v, id), err)
			}
			bounded := WithLockWait(ctx, pageBound)
			for _, h := range []*Host{daemon, other} {
				if v, err := h.View(bounded, "team", 0); err != nil || !hasItem(v, id) {
					t.Fatalf("view under a held network call: %v", err)
				}
				if content, _, err := h.WikiRead(bounded, "team", "docs", "home", ""); err != nil || content != edit.Content {
					t.Fatalf("wiki read under a held network call = %q, %v", content, err)
				}
				if list, err := h.WikiList(bounded, "team"); err != nil || len(list) != 1 {
					t.Fatalf("wiki list under a held network call = %+v, %v", list, err)
				}
				if data, err := h.ReadWikiImage(bounded, "team", "docs", attachmentOID(img)); err != nil || !bytes.Equal(data, img) {
					t.Fatalf("wiki image under a held network call = %d bytes, %v", len(data), err)
				}
				if data, err := h.ReadAttachment(bounded, "team", att.OID); err != nil || !bytes.Equal(data, pngBytes("attachment")) {
					t.Fatalf("local attachment under a held network call = %d bytes, %v", len(data), err)
				}
			}
			during := createItem(t, daemon, "team", "written during the network call")
			select {
			case err := <-done:
				t.Fatalf("the held network call returned early: %v", err)
			default:
			}

			close(runner.release)
			if err := <-done; err != nil {
				t.Fatalf("the network call after its release: %v", err)
			}
			if v := view(t, daemon, "team"); !hasItem(v, id) || !hasItem(v, during) {
				t.Fatal("a write made around the network call is gone after it")
			}
		})
	}
}

// TestRemoteAttachmentReadAnswersBusyWhileNetworkHeld: an attachment this
// clone has yet to fetch needs the network, so while a check holds it, a
// read with the page's lock bound answers ErrStoreBusy, in the process
// running the check (the network mutex) and in another one (net.lock),
// instead of waiting the check out; an attachment the clone holds is still
// served. After the check, the same bounded read fetches the blob.
func TestRemoteAttachmentReadAnswersBusyWhileNetworkHeld(t *testing.T) {
	w := newWorld(t)
	if out, err := exec.Command("git", "-C", w.remote, "config", "uploadpack.allowFilter", "true").CombinedOutput(); err != nil {
		t.Fatalf("allow filter: %v: %s", err, out)
	}
	a := w.machine("a")
	runner := newHoldRunner(isFetchCommand)
	b := hostWith(t, w, "b", runner)
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	twin := sameRoot(t, w, "b")
	ctx := context.Background()

	near := stageOK(t, b, "team", "near.png", pngBytes("held here"))
	if _, err := createWithAttachment(t, b, "team", near); err != nil {
		t.Fatal(err)
	}
	farData := pngBytes("fetched on demand")
	far := stageOK(t, a, "team", "far.png", farData)
	if _, err := createWithAttachment(t, a, "team", far); err != nil {
		t.Fatal(err)
	}
	syncStore(t, a, "team")
	if _, err := b.Check(ctx, "team"); err != nil {
		t.Fatal(err)
	}

	runner.armed.Store(true)
	checked := make(chan error, 1)
	go func() {
		_, err := b.Check(ctx, "team")
		checked <- err
	}()
	<-runner.entered

	bounded := WithLockWait(ctx, 50*time.Millisecond)
	for name, h := range map[string]*Host{"the checking process": b, "another process": twin} {
		if _, err := h.ReadAttachment(bounded, "team", far.OID); !errors.Is(err, ErrStoreBusy) {
			t.Fatalf("bounded read of a not-yet-local attachment in %s = %v, want ErrStoreBusy", name, err)
		}
		if data, err := h.ReadAttachment(bounded, "team", near.OID); err != nil || !bytes.Equal(data, pngBytes("held here")) {
			t.Fatalf("bounded read of a local attachment in %s = %d bytes, %v", name, len(data), err)
		}
	}

	close(runner.release)
	if err := <-checked; err != nil {
		t.Fatalf("check: %v", err)
	}
	if data, err := twin.ReadAttachment(bounded, "team", far.OID); err != nil || !bytes.Equal(data, farData) {
		t.Fatalf("bounded read after the check = %d bytes, %v", len(data), err)
	}
}

// TestAppendDuringHeldFetchSurvivesAndIsPushed: with a check's fetch done
// and its apply step still ahead, a write by this process and one by
// another land at once; a reader sees them and nothing of the fetch. The
// apply step then merges what was fetched with both writes, and the next
// sync pushes them.
func TestAppendDuringHeldFetchSurvivesAndIsPushed(t *testing.T) {
	w := newWorld(t)
	runner := newHoldRunner(isWikiFetch)
	runner.after = true
	a := hostWith(t, w, "a", runner)
	b := w.machine("b")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
	other := sameRoot(t, w, "a")
	ctx := context.Background()
	if _, err := a.Check(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	fetchedItem := createItem(t, b, "team", "from the remote")
	syncStore(t, b, "team")

	runner.armed.Store(true)
	checked := make(chan error, 1)
	go func() {
		_, err := a.Check(ctx, "team")
		checked <- err
	}()
	<-runner.entered

	own := createItem(t, a, "team", "appended during the fetch")
	theirs := createItem(t, other, "team", "appended by another process during the fetch")
	if v := view(t, a, "team"); !hasItem(v, own) || !hasItem(v, theirs) || hasItem(v, fetchedItem) {
		t.Fatalf("view before the apply step: own=%v theirs=%v fetched=%v, want both writes and nothing of the fetch",
			hasItem(v, own), hasItem(v, theirs), hasItem(v, fetchedItem))
	}

	close(runner.release)
	if err := <-checked; err != nil {
		t.Fatalf("check: %v", err)
	}
	if v := view(t, a, "team"); !hasItem(v, own) || !hasItem(v, theirs) || !hasItem(v, fetchedItem) {
		t.Fatalf("view after the apply step: own=%v theirs=%v fetched=%v, want all three",
			hasItem(v, own), hasItem(v, theirs), hasItem(v, fetchedItem))
	}
	syncStore(t, a, "team")
	if _, err := b.Check(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if v := view(t, b, "team"); !hasItem(v, own) || !hasItem(v, theirs) {
		t.Fatal("the writes made during the fetch did not reach the remote")
	}
}

// TestUnregisterDuringHeldFetchDiscardsTheResult: a check in flight does
// not make an unregister wait or refuse. The store is removed under the
// check, by this process or another, and the check's apply step, finding
// the registration gone, applies nothing: the check answers
// ErrNotRegistered, the handle is dropped, and nothing of the store's
// directory comes back. Where the rename follows the locks' release and is
// refused while the fetch holds a file in the directory (Windows; the last
// case refuses it on every OS), the unregister still succeeds and the
// check moves the directory away when it ends.
func TestUnregisterDuringHeldFetchDiscardsTheResult(t *testing.T) {
	for _, who := range []string{"this process", "another process", "another process, rename refused"} {
		t.Run(who, func(t *testing.T) {
			w := newWorld(t)
			runner := newHoldRunner(isWikiFetch)
			runner.after = true
			if strings.HasSuffix(who, "rename refused") {
				wasUnder, wasRename := renameUnderLocks, renameDir
				t.Cleanup(func() { renameUnderLocks, renameDir = wasUnder, wasRename })
				renameUnderLocks = false
				renameDir = func(from, to string) error {
					select {
					case <-runner.release:
						return wasRename(from, to)
					default:
						return &os.LinkError{Op: "rename", Old: from, New: to, Err: errors.New("access is denied")}
					}
				}
			}
			a := hostWith(t, w, "a", runner)
			b := w.machine("b")
			res := w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
			w.register(b, "team", "bo", &wstracker.Profile{Given: "Bo"})
			ctx := context.Background()
			if _, err := a.Check(ctx, "team"); err != nil {
				t.Fatal(err)
			}
			createItem(t, b, "team", "fetched, never applied")
			syncStore(t, b, "team")
			remover := a
			if who != "this process" {
				remover = sameRoot(t, w, "a")
			}

			runner.armed.Store(true)
			checked := make(chan error, 1)
			go func() {
				_, err := a.Check(ctx, "team")
				checked <- err
			}()
			<-runner.entered

			if err := remover.Unregister(ctx, "team", ""); err != nil {
				t.Fatalf("unregister under a held fetch: %v", err)
			}
			close(runner.release)
			if err := <-checked; !errors.Is(err, ErrNotRegistered) {
				t.Fatalf("check of a store unregistered during its fetch = %v, want ErrNotRegistered", err)
			}
			wantGone(t, a, "team")
			if _, err := os.Stat(res.Entry.Dir); !os.IsNotExist(err) {
				t.Fatalf("the store directory after the discarded fetch: %v, want gone", err)
			}
			a.mu.Lock()
			kept := len(a.stores)
			a.mu.Unlock()
			if kept != 0 {
				t.Fatalf("the checking host kept %d handles on the removed store", kept)
			}
			// The id is free again, and the next registration starts clean.
			w.register(a, "team", "ada", nil)
			if _, err := a.Check(ctx, "team"); err != nil {
				t.Fatalf("check of the new registration: %v", err)
			}
		})
	}
}

// TestNetworkCallsOfOneStoreRunOneAtATime: two calls of one Host that both
// need the store's remote take turns. While a check holds a fetch open, a
// sync waits without running a remote command, and one whose context
// bounds its lock wait answers ErrStoreBusy.
func TestNetworkCallsOfOneStoreRunOneAtATime(t *testing.T) {
	w := newWorld(t)
	var remote atomic.Int64
	runner := newHoldRunner(isFetchCommand)
	counted := func(args []string) bool {
		if isRemoteCommand(args) {
			remote.Add(1)
		}
		return isFetchCommand(args)
	}
	runner.match = counted
	a := hostWith(t, w, "a", runner)
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})
	ctx := context.Background()
	id := createItem(t, a, "team", "to push")

	runner.armed.Store(true)
	checked := make(chan error, 1)
	go func() {
		_, err := a.Check(ctx, "team")
		checked <- err
	}()
	<-runner.entered
	held := remote.Load()

	if _, err := a.Sync(WithLockWait(ctx, 30*time.Millisecond), "team"); !errors.Is(err, ErrStoreBusy) {
		t.Fatalf("bounded sync under a held check = %v, want ErrStoreBusy", err)
	}
	synced := make(chan error, 1)
	go func() {
		_, err := a.Sync(ctx, "team")
		synced <- err
	}()
	select {
	case err := <-synced:
		t.Fatalf("a sync returned while a check held the network: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if n := remote.Load(); n != held {
		t.Fatalf("%d remote commands started while the check held the network, want none", n-held)
	}

	close(runner.release)
	if err := <-checked; err != nil {
		t.Fatalf("check: %v", err)
	}
	if err := <-synced; err != nil {
		t.Fatalf("sync: %v", err)
	}
	if v := view(t, a, "team"); !hasItem(v, id) || len(v.Sync.Unpushed) != 0 {
		t.Fatalf("after the sync: item=%v unpushed=%d, want it pushed", hasItem(v, id), len(v.Sync.Unpushed))
	}
}

package wstrackerhost

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// removedPrefix starts the name a store directory is renamed to before it
// is deleted: <root>/.removed-<id>-<unix nanoseconds>. A store id cannot
// start with '.', so the name never collides with a store.
const removedPrefix = ".removed-"

// unregisterWait bounds how long Unregister waits for each of the store's
// locks before it refuses: long enough to outlast another reader's reload
// or a local step of a sync, far short of a wiki save holding the lock
// across the network.
var unregisterWait = 2 * time.Second

// renameUnderLocks: Unregister renames the store directory away while it
// still holds the store's file locks, so no other process runs on the
// directory in between. Windows cannot rename a directory that holds an
// open file, so there the lock files close first; a call another process
// already had waiting on the store lock can then run on the directory in
// the moment before the rename, and a call on the network (a fetch, in any
// process) holds the network lock file open in the directory throughout.
// There the rename is retried while refused (renameWindow).
var renameUnderLocks = runtime.GOOS != "windows"

// renameDir renames a store directory away; a test replaces it to refuse
// the rename as Windows does while a file in the directory is open.
var renameDir = os.Rename

// renameWindow bounds how long Unregister, renaming after the locks'
// release, retries a rename refused because a file in the directory is
// open: long enough to outlast a background check's fetch. A rename still
// refused then puts the registry entry back, and Unregister fails.
var renameWindow = 5 * time.Second

// renameRetryEvery is the pause between two attempts within renameWindow.
const renameRetryEvery = 50 * time.Millisecond

// The kinds of an UnregisterRefusal.
const (
	// UnregisterNeedsConfirm: the store has no remote and the confirmation
	// did not name it.
	UnregisterNeedsConfirm = "confirm"
	// UnregisterBusy: a write, or a local step of a check or sync, holds
	// the store.
	UnregisterBusy = "busy"
	// UnregisterUnpushed: the clone holds own events or wiki changes the
	// remote does not, and a sync can push them.
	UnregisterUnpushed = "unpushed"
	// UnregisterStuck: the clone holds own events no sync can push.
	UnregisterStuck = "stuck"
)

// UnregisterRefusal is Unregister's answer when it left the store exactly
// as it was. Reason is a full sentence for the user.
type UnregisterRefusal struct {
	Store  string
	Kind   string
	Reason string
}

func (r *UnregisterRefusal) Error() string {
	return fmt.Sprintf("tracker: store %q was not unregistered: %s", r.Store, r.Reason)
}

func refuseBusy(id string) error {
	return &UnregisterRefusal{Store: id, Kind: UnregisterBusy,
		Reason: "a check, sync, or write of the store is in flight; try again when it has finished"}
}

// unpushedRefusal is the refusal for a clone that holds what its remote
// does not; nil when it holds nothing.
func unpushedRefusal(id string, u wsledger.Unpushed) error {
	p := u.Pending
	var held []string
	if p.Count == 1 {
		held = append(held, "1 own event that is not pushed yet")
	} else if p.Count > 1 {
		held = append(held, fmt.Sprintf("%d own events that are not pushed yet", p.Count))
	}
	if len(u.Wikis) > 0 {
		held = append(held, "unpushed wiki changes ("+strings.Join(u.Wikis, ", ")+")")
	}
	if len(held) == 0 {
		return nil
	}
	what := "this machine's copy holds " + strings.Join(held, " and ")
	stuck := ""
	switch {
	case p.Count == 0:
	case p.IDTaken:
		stuck = "another join took this machine's member id, so nothing of this copy can be pushed; the store has to be registered under another member id"
	case p.KeyNotRegistered:
		stuck = "this machine's key is not registered in the store, so its events wait until an admin adds the key"
	case p.ForeignAdvance:
		stuck = "the remote holds writes under this copy that it did not make, so it cannot push until its unpushed events move to a fresh clone id"
	}
	if stuck != "" {
		return &UnregisterRefusal{Store: id, Kind: UnregisterStuck,
			Reason: what + ", and no sync can push them: " + stuck + ". Unregistering would delete them"}
	}
	return &UnregisterRefusal{Store: id, Kind: UnregisterUnpushed,
		Reason: what + ", which unregistering would delete; sync the store first"}
}

// tryFlock takes fl within unregisterWait.
func tryFlock(ctx context.Context, fl *flock.Flock) bool {
	lockCtx, cancel := context.WithTimeout(ctx, unregisterWait)
	defer cancel()
	locked, err := fl.TryLockContext(lockCtx, 20*time.Millisecond)
	if err != nil || !locked {
		fl.Close() //nolint:errcheck // a failed attempt leaves the file open
		return false
	}
	return true
}

// Unregister removes store id from this machine: its registry entry and its
// directory, the local clone. It writes nothing to the store and leaves the
// remote, this member's join and key included, as it is, so registering the
// id again with the same member id finishes without a new join.
//
// A store with a remote is refused (an *UnregisterRefusal) while this clone
// holds an own event or a wiki change the remote does not (a push in
// flight included: what it sends counts as unpushed until it is recorded),
// or while a write of it is in flight in any process; confirm is ignored.
// A store with no remote holds its only copy here, so it is removed only
// when confirm is the store id. A refusal leaves the store as it was.
//
// A fetch in flight, in any process, does not hold the store: it is on the
// network under the network lock, and Unregister does not refuse for it
// (but where the rename follows the locks' release, below). The store is
// removed under it, and the step that would apply what it fetched finds
// the registration gone and applies nothing (applyGate).
//
// The removal is safe against the other processes sharing the root. It
// holds the store's write lock and wsledger's store lock, read the unpushed
// state under them, and takes the entry out of the registry before
// releasing either, so a write that another process had waiting finds the
// registration gone (checkRegistered) instead of landing in the removed
// clone. The directory is then renamed to a hidden name and deleted there:
// every process keeps the stores it opened open, so deleting in place would
// let another process's write recreate files inside a half-deleted
// directory. Nothing recreates the directory afterwards, and every Host
// re-checks the registration before it reuses an open store (lockStore).
//
// Where the rename follows the locks' release (renameUnderLocks false), a
// file another call holds open in the directory refuses it: a fetch's
// network lock file, or a reader's. The rename is then retried for up to
// renameWindow, holding no lock of the store, so a fetch in flight can end
// meanwhile. A rename still refused then, or failing otherwise, puts the
// registry entry back and Unregister fails, telling the user to try again.
// The store is then registered as before: a process that found it gone
// meanwhile and dropped its handle opens a fresh one on its next call
// (lockStore), and no directory is ever left without an entry, which
// Register would refuse.
func (h *Host) Unregister(ctx context.Context, id, confirm string) error {
	h.sweepRemoved()
	e, at, aside, err := h.removeRegistration(ctx, id, confirm)
	if err != nil {
		return err
	}
	if !renameUnderLocks {
		if err := renameRetrying(ctx, e.Dir, aside); err != nil {
			_ = h.restoreEntry(e, at)
			if !errors.Is(err, fs.ErrPermission) {
				return fmt.Errorf("tracker: unregister store %q: %w", id, err)
			}
			return fmt.Errorf("tracker: store %q was not unregistered: its directory is in use; try again when the check, sync, or program using it has finished: %w", id, err)
		}
	}
	// A failed delete leaves a hidden directory the next sweep removes.
	_ = os.RemoveAll(aside)
	return nil
}

// removeRegistration is Unregister up to the rename that follows the
// locks' release: it refuses, or takes store id's entry out of the
// registry, renames the directory to aside when renameUnderLocks, and
// retires this process's handle. It returns the entry, its registry
// position, and aside, holding no lock of the store.
func (h *Host) removeRegistration(ctx context.Context, id, confirm string) (StoreEntry, int, string, error) {
	fail := func(err error) (StoreEntry, int, string, error) { return StoreEntry{}, -1, "", err }
	st, err := h.lockStore(ctx, id, unregisterWait, true)
	if errors.Is(err, ErrStoreBusy) {
		return fail(refuseBusy(id))
	}
	if err != nil {
		return fail(err)
	}
	defer st.mu.Unlock()
	e := st.entry
	if e.Remote == "" && confirm != id {
		return fail(&UnregisterRefusal{Store: id, Kind: UnregisterNeedsConfirm,
			Reason: "it has no remote, so this machine holds its only copy and unregistering deletes its data permanently; confirm by giving the store id"})
	}
	wl := flock.New(filepath.Join(e.Dir, writeLockFile))
	if !tryFlock(ctx, wl) {
		return fail(refuseBusy(id))
	}
	defer wl.Unlock() //nolint:errcheck
	if st.ledger == nil {
		// Opening waits for the store lock as long as ctx allows, so a lock
		// another process holds across a wiki save is refused here first.
		probe := flock.New(filepath.Join(e.Dir, wsledger.LockFile))
		if !tryFlock(ctx, probe) {
			return fail(refuseBusy(id))
		}
		probe.Unlock() //nolint:errcheck
		if err := h.open(ctx, st); err != nil {
			return fail(err)
		}
	}
	// Held around the entry's removal, and the rename under the locks, so
	// a seen mark of this process (MarkSeen) reads the entry before it is
	// gone and is written before the directory moves, or finds it gone.
	h.seenMu.Lock()
	defer h.seenMu.Unlock()
	aside := filepath.Join(h.root, fmt.Sprintf("%s%s-%d", removedPrefix, id, time.Now().UnixNano()))
	at := -1
	err = st.ledger.Exclusive(ctx, unregisterWait, func(u wsledger.Unpushed) error {
		if e.Remote != "" {
			if err := unpushedRefusal(id, u); err != nil {
				return err
			}
		}
		var err error
		if at, err = h.removeEntry(id, e.Nonce); err != nil {
			return err
		}
		if !renameUnderLocks {
			return nil
		}
		if err := renameDir(e.Dir, aside); err != nil {
			// The store stays registered rather than leaving a directory
			// with no entry, which Register would refuse.
			_ = h.restoreEntry(e, at)
			return fmt.Errorf("tracker: unregister store %q: %w", id, err)
		}
		return nil
	})
	if errors.Is(err, wsledger.ErrBusy) {
		return fail(refuseBusy(id))
	}
	if err != nil {
		return fail(err)
	}
	h.retire(st)
	return e, at, aside, nil
}

// renameRetrying renames a store directory from to to, retrying while the
// rename is refused for a file open in it (Windows reports it as a
// permission error), for up to renameWindow or until ctx ends. It returns
// the last attempt's error.
func renameRetrying(ctx context.Context, from, to string) error {
	deadline := time.Now().Add(renameWindow)
	for {
		err := renameDir(from, to)
		if err == nil || !errors.Is(err, fs.ErrPermission) || !time.Now().Before(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(renameRetryEvery):
		}
	}
}

// sweepRemoved deletes the directories an Unregister renamed away and then
// failed to delete (it was interrupted, or a file was still open).
func (h *Host) sweepRemoved() {
	entries, err := os.ReadDir(h.root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), removedPrefix) {
			_ = os.RemoveAll(filepath.Join(h.root, e.Name()))
		}
	}
}

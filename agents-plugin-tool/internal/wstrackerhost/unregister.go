package wstrackerhost

import (
	"context"
	"errors"
	"fmt"
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
// locks before it refuses: long enough to outlast another reader's reload,
// far short of a sync holding the lock across the network.
var unregisterWait = 2 * time.Second

// renameUnderLocks: Unregister renames the store directory away while it
// still holds the store's file locks, so no other process runs on the
// directory in between. Windows cannot rename a directory that holds an
// open file, so there the lock files close first; a call another process
// already had waiting on the store lock can then run on the directory in
// the moment before the rename.
var renameUnderLocks = runtime.GOOS != "windows"

// The kinds of an UnregisterRefusal.
const (
	// UnregisterNeedsConfirm: the store has no remote and the confirmation
	// did not name it.
	UnregisterNeedsConfirm = "confirm"
	// UnregisterBusy: a check, sync, or write holds the store.
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
// holds an own event or a wiki change the remote does not, or while a
// check, sync, or write of it is in flight in any process; confirm is
// ignored. A store with no remote holds its only copy here, so it is
// removed only when confirm is the store id. A refusal leaves the store as
// it was.
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
func (h *Host) Unregister(ctx context.Context, id, confirm string) error {
	h.sweepRemoved()
	st, err := h.lockStore(ctx, id, unregisterWait, true)
	if errors.Is(err, ErrStoreBusy) {
		return refuseBusy(id)
	}
	if err != nil {
		return err
	}
	defer st.mu.Unlock()
	e := st.entry
	if e.Remote == "" && confirm != id {
		return &UnregisterRefusal{Store: id, Kind: UnregisterNeedsConfirm,
			Reason: "it has no remote, so this machine holds its only copy and unregistering deletes its data permanently; confirm by giving the store id"}
	}
	wl := flock.New(filepath.Join(e.Dir, writeLockFile))
	if !tryFlock(ctx, wl) {
		return refuseBusy(id)
	}
	defer wl.Unlock() //nolint:errcheck
	if st.ledger == nil {
		// Opening waits for the store lock as long as ctx allows, so a lock
		// another process holds across a sync is refused here first.
		probe := flock.New(filepath.Join(e.Dir, wsledger.LockFile))
		if !tryFlock(ctx, probe) {
			return refuseBusy(id)
		}
		probe.Unlock() //nolint:errcheck
		if err := h.open(ctx, st); err != nil {
			return err
		}
	}
	// Held around the entry's removal and the rename, so a seen mark of
	// this process (MarkSeen) cannot land in between.
	h.seenMu.Lock()
	defer h.seenMu.Unlock()
	aside := filepath.Join(h.root, fmt.Sprintf("%s%s-%d", removedPrefix, id, time.Now().UnixNano()))
	at := -1
	moveAside := func() error {
		if err := os.Rename(e.Dir, aside); err != nil {
			// The store stays registered rather than leaving a directory
			// with no entry, which Register would refuse.
			_ = h.restoreEntry(e, at)
			return fmt.Errorf("tracker: unregister store %q: %w", id, err)
		}
		return nil
	}
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
		if renameUnderLocks {
			return moveAside()
		}
		return nil
	})
	if errors.Is(err, wsledger.ErrBusy) {
		return refuseBusy(id)
	}
	if err != nil {
		return err
	}
	h.retire(st)
	wl.Unlock() //nolint:errcheck
	if !renameUnderLocks {
		if err := moveAside(); err != nil {
			return err
		}
	}
	// A failed delete leaves a hidden directory the next sweep removes.
	_ = os.RemoveAll(aside)
	return nil
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

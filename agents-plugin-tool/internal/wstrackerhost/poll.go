package wstrackerhost

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsledger"
)

const pollLockFile = "poll.lock"

// HasStores reports whether the registry holds a store; an unreadable
// registry reads as none.
func (h *Host) HasStores() bool {
	stores, err := h.Stores()
	return err == nil && len(stores) > 0
}

// PollerRunning reports whether some process holds the poll lock, that is,
// runs StartPoll (the dashboard daemon). The lock is released when that
// process exits, however it exits.
func (h *Host) PollerRunning() bool {
	fl := flock.New(filepath.Join(h.root, pollLockFile))
	locked, err := fl.TryLock()
	if err != nil {
		return false // no root yet, so no poller either
	}
	if locked {
		_ = fl.Unlock()
		return false
	}
	return true
}

// checkThrottled fetches store id unless it has no remote or any process
// attempted a check or sync of it within the check interval. Only a store
// that cannot be opened, or a lock held past the caller's bound
// (WithLockWait), is an error; a failed fetch is recorded.
func (h *Host) checkThrottled(ctx context.Context, id string, timeout time.Duration) error {
	due := func(e StoreEntry) bool {
		if e.Remote == "" {
			return false
		}
		// A last attempt in the future (the clock stepped back) does not throttle.
		if s := readSyncState(e.Dir); !s.AttemptedAt.IsZero() {
			if since := h.now().Sub(s.AttemptedAt); since >= 0 && since < h.checkInterval {
				return false
			}
		}
		return true
	}
	// Asked before the network mutex is: a throttled check never waits for
	// a fetch or push in flight.
	fetch := false
	if err := h.with(ctx, id, func(st *store) error {
		fetch = due(st.entry)
		return nil
	}); err != nil || !fetch {
		return err
	}
	return h.withNet(ctx, id, func(ctx context.Context, st *store, e StoreEntry, l *wsledger.Ledger) error {
		// Asked again: the call this one waited behind may have been the
		// attempt.
		if due(e) {
			_ = h.attempt(ctx, st, timeout, l.Fetch)
		}
		return nil
	})
}

// PreRead is the check before a tracker read when no daemon polls: a fetch
// of store id, throttled by the check interval. While a process holds the
// poll lock it does nothing, since that process's poll owns the network and
// a read reloads what it fetched. A failed fetch is the offline path: it is
// recorded, and the read serves the local state, so only a store that
// cannot be opened returns an error.
func (h *Host) PreRead(ctx context.Context, id string) error {
	if h.PollerRunning() {
		return nil
	}
	return h.checkThrottled(ctx, id, preReadTimeout)
}

// CheckAll is the lead-session-start check: a throttled fetch of every
// registered store, whether or not a daemon polls and whatever the tracker
// knob says. It reports nothing; a store that fails stays as it was.
func (h *Host) CheckAll(ctx context.Context) {
	h.forEachStore(ctx, func(id string) { _ = h.checkThrottled(ctx, id, networkTimeout) })
}

// PollOnce is one round of the background poll: an unthrottled fetch of
// every registered store with a remote. It reports whether the round moved
// anything a page shows (pollStamp): a round that fetched nothing new and
// kept every store's sync standing changed nothing.
func (h *Host) PollOnce(ctx context.Context) (changed bool) {
	h.forEachStore(ctx, func(id string) {
		_ = h.withNet(ctx, id, func(ctx context.Context, st *store, e StoreEntry, l *wsledger.Ledger) error {
			before := pollStamp(e.Dir)
			_ = h.locked(st, func() error {
				h.trySweep(st)
				return nil
			})
			if e.Remote != "" {
				_ = h.attempt(ctx, st, networkTimeout, l.Fetch)
			}
			if pollStamp(e.Dir) != before {
				changed = true
			}
			return nil
		})
	})
	return changed
}

// pollStamp is what a poll round can move that a page shows: the store's
// ledger state (storeStamp) and its sync standing (whether it ever reached
// the remote, the last error, the wiki warnings), but not the attempt and
// success times every round stamps.
func pollStamp(dir string) string {
	s := readSyncState(dir)
	standing, _ := json.Marshal(struct {
		Synced bool
		Error  string
		Wiki   []string
	}{!s.SyncedAt.IsZero(), s.Error, s.WikiWarnings})
	return fmt.Sprintf("%x\x00%s", storeStamp(dir), standing)
}

// forEachStore runs fn on the id of every registered store, one at a time.
func (h *Host) forEachStore(ctx context.Context, fn func(id string)) {
	stores, err := h.Stores()
	if err != nil {
		return
	}
	for _, e := range stores {
		if ctx.Err() != nil {
			return
		}
		fn(e.ID)
	}
}

// StartPoll runs the background poll in this process: it creates the root,
// takes the poll lock (retrying each tick while another process holds it),
// and runs PollOnce at once and then every interval while it holds the
// lock, re-reading the registry each round. onRound, when not nil, runs after
// each round this process ran (that is, only while it holds the poll lock)
// that changed something (PollOnce), and after the first one it ran
// whatever it found, since pages read before the poll began.
// stop ends the poll and releases the lock. The dashboard daemon runs it, so
// while the daemon runs, PreRead leaves the network to it.
func (h *Host) StartPoll(interval time.Duration, onRound func()) (stop func()) {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = os.MkdirAll(h.root, 0o700) // so the lock can be taken before the first registration
		fl := flock.New(filepath.Join(h.root, pollLockFile))
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		held := false
		defer func() {
			if held {
				_ = fl.Unlock()
			}
		}()
		ran := false
		round := func() {
			if !held {
				// Fails while another process polls.
				if locked, err := fl.TryLock(); err == nil && locked {
					held = true
				}
			}
			if held {
				changed := h.PollOnce(ctx) || !ran
				ran = true
				if changed && onRound != nil && ctx.Err() == nil {
					onRound()
				}
			}
		}
		// The first round runs as soon as the lock is held: holding it turns
		// every process's PreRead into a no-op, so waiting a full interval
		// would leave reads that stale after the daemon starts.
		round()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				round()
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

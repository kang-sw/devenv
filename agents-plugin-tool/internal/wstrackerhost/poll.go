package wstrackerhost

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
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

// checkThrottled fetches st unless any process attempted a check or sync of
// it within the check interval. Callers hold st.mu.
func (h *Host) checkThrottled(ctx context.Context, st *store, timeout time.Duration) {
	if st.entry.Remote == "" {
		return
	}
	// A last attempt in the future (the clock stepped back) does not throttle.
	if s := readSyncState(st.entry.Dir); !s.AttemptedAt.IsZero() {
		if since := h.now().Sub(s.AttemptedAt); since >= 0 && since < h.checkInterval {
			return
		}
	}
	_ = h.attempt(ctx, st, timeout, func(ctx context.Context) error {
		_, err := st.ledger.Fetch(ctx)
		return err
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
	return h.with(ctx, id, func(st *store) error {
		h.checkThrottled(ctx, st, preReadTimeout)
		return nil
	})
}

// CheckAll is the lead-session-start check: a throttled fetch of every
// registered store, whether or not a daemon polls and whatever the tracker
// knob says. It reports nothing; a store that fails stays as it was.
func (h *Host) CheckAll(ctx context.Context) {
	h.forEachStore(ctx, func(st *store) { h.checkThrottled(ctx, st, networkTimeout) })
}

// PollOnce is one round of the background poll: an unthrottled fetch of
// every registered store with a remote.
func (h *Host) PollOnce(ctx context.Context) {
	h.forEachStore(ctx, func(st *store) {
		if st.entry.Remote == "" {
			return
		}
		_ = h.attempt(ctx, st, networkTimeout, func(ctx context.Context) error {
			_, err := st.ledger.Fetch(ctx)
			return err
		})
	})
}

// forEachStore runs fn on every registered store this process can open,
// one at a time, skipping the ones it cannot.
func (h *Host) forEachStore(ctx context.Context, fn func(*store)) {
	stores, err := h.Stores()
	if err != nil {
		return
	}
	for _, e := range stores {
		if ctx.Err() != nil {
			return
		}
		_ = h.with(ctx, e.ID, func(st *store) error {
			fn(st)
			return nil
		})
	}
}

// StartPoll runs the background poll in this process: it creates the root,
// takes the poll lock (retrying each tick while another process holds it),
// and runs PollOnce at once and then every interval while it holds the
// lock, re-reading the registry each round. stop ends it and releases the lock. The dashboard
// daemon runs it, so while the daemon runs, PreRead leaves the network to
// it.
func (h *Host) StartPoll(interval time.Duration) (stop func()) {
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
		take := func() {
			if !held {
				// Fails while another process polls.
				if locked, err := fl.TryLock(); err == nil && locked {
					held = true
				}
			}
		}
		// The first round runs as soon as the lock is held: holding it turns
		// every process's PreRead into a no-op, so waiting a full interval
		// would leave reads that stale after the daemon starts.
		take()
		if held {
			h.PollOnce(ctx)
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				take()
				if held {
					h.PollOnce(ctx)
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

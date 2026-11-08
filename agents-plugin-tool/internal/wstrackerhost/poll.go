package wstrackerhost

import (
	"context"
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
	if s := readSyncState(st.entry.Dir); !s.AttemptedAt.IsZero() && h.now().Sub(s.AttemptedAt) < h.checkInterval {
		return
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

// StartPoll runs the background poll in this process: it takes the poll
// lock (waiting a tick at a time while another process holds it) and then
// runs PollOnce every interval, re-reading the registry each round. stop
// ends it and releases the lock. The dashboard daemon runs it, so while the
// daemon runs, PreRead leaves the network to it.
func (h *Host) StartPoll(interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		fl := flock.New(filepath.Join(h.root, pollLockFile))
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		held := false
		defer func() {
			if held {
				_ = fl.Unlock()
			}
		}()
		for {
			if !held {
				// Fails while another process polls, or before the first
				// registration creates the root.
				if locked, err := fl.TryLock(); err == nil && locked {
					held = true
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
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

// Immediate checks, sync reports, and persisted sync state.
package wstrackerhost

import (
	"context"
	"github.com/kang-sw/devenv/internal/wsledger"
	"path/filepath"
	"strings"
	"time"
)

// Check is the immediate check (a receive-only fetch) the drawer runs when
// a store's drawer opens. It is never throttled; its outcome is recorded
// for every process. A store with no remote only reloads.
func (h *Host) Check(ctx context.Context, id string) (wsledger.SyncReport, error) {
	var rep wsledger.SyncReport
	err := h.withNet(ctx, id, func(ctx context.Context, st *store, e StoreEntry, l *wsledger.Ledger) error {
		if e.Remote == "" {
			return h.locked(st, func() error {
				var err error
				rep, err = l.Reload(ctx)
				return err
			})
		}
		return h.attempt(ctx, st, networkTimeout, func(ctx context.Context) (wsledger.SyncReport, error) {
			var err error
			rep, err = l.Fetch(ctx)
			return rep, err
		})
	})
	return rep, err
}

// Sync is the apply path: close the open batch, push this clone's writes,
// and fetch. It runs at an explicit apply and when the drawer panel
// collapses; its outcome is recorded for every process. With no remote it
// closes the batch.
func (h *Host) Sync(ctx context.Context, id string) (wsledger.SyncReport, error) {
	var rep wsledger.SyncReport
	err := h.withNet(ctx, id, func(ctx context.Context, st *store, e StoreEntry, l *wsledger.Ledger) error {
		if e.Remote == "" {
			var err error
			rep, err = l.Sync(ctx)
			return err
		}
		return h.attempt(ctx, st, networkTimeout, func(ctx context.Context) (wsledger.SyncReport, error) {
			var err error
			rep, err = l.Sync(ctx)
			return rep, err
		})
	})
	return rep, err
}

// syncState is a store's last check or sync outcome, persisted in its
// directory so every process sharing the store agrees on it.
type syncState struct {
	// AttemptedAt is the last check or sync that ran, success or not; the
	// throttle counts from it.
	AttemptedAt time.Time `json:"attemptedAt"`
	// SyncedAt is the last one that reached the remote.
	SyncedAt time.Time `json:"syncedAt,omitempty"`
	// Error is the last attempt's error; empty when it reached the remote.
	Error string `json:"error,omitempty"`
	// WikiWarnings are the last attempt's wiki sync failures (a wiki fetch
	// or push the ledger's sync went on without), one line each. The
	// ledger re-reports them until the wiki syncs, so the next attempt
	// after that clears them; a background poll's report is read here too.
	WikiWarnings []string `json:"wikiWarnings,omitempty"`
}

const syncStateFile = "host-sync.json"

// wikiWarnings renders the wiki sync failures among warns, one line each
// naming the wiki (or the wiki namespace, for a failed fetch).
func wikiWarnings(warns []wsledger.Warning) []string {
	var out []string
	for _, w := range warns {
		if w.Kind == wsledger.WarnWikiSyncFailed {
			out = append(out, strings.TrimPrefix(w.Ref, "refs/heads/")+": "+w.Detail)
		}
	}
	return out
}

func readSyncState(dir string) syncState {
	var s syncState
	_ = readJSON(filepath.Join(dir, syncStateFile), &s)
	return s
}

// attempt runs one network call on st within timeout and records its
// outcome for every process. Callers hold st's network mutex and not
// st.mu (withNet): the call takes st.mu for its local steps (applyGate),
// and the outcome is recorded under it, unless the store was unregistered
// while the call was on the network, whose directory is no longer st's to
// write.
func (h *Host) attempt(ctx context.Context, st *store, timeout time.Duration, call func(context.Context) (wsledger.SyncReport, error)) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	rep, err := call(ctx)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.retired || h.checkRegistered(st) != nil {
		return err
	}
	s := readSyncState(st.entry.Dir)
	s.AttemptedAt = h.now()
	s.WikiWarnings = wikiWarnings(rep.Warnings)
	if err != nil {
		s.Error = err.Error()
	} else {
		s.SyncedAt, s.Error = s.AttemptedAt, ""
	}
	_ = writeJSON(filepath.Join(st.entry.Dir, syncStateFile), s)
	return err
}

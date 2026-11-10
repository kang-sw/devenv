package wstrackerhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

// reload is one background re-read of a store for ViewWithin. Its fields
// are guarded by the store's viewMu; v and err are set before done closes.
type reload struct {
	done chan struct{}
	// read: the reload has started re-reading the store directory, so a
	// write landing now may be missed; a later read waits for the next
	// reload instead.
	read bool
	// late: a read stopped waiting and served the last state, so the
	// change callback runs when this reload lands with a view that differs
	// from one served.
	late bool
	// served are the distinct views the late reads were handed; a nil
	// entry is a late read that was handed nothing (its context ended),
	// which always counts as a difference.
	served []*StoreView
	v      *StoreView
	err    error
}

func newReload() *reload { return &reload{done: make(chan struct{})} }

// ViewWithin is View for a reader that would rather see a briefly stale
// view than wait: the dashboard daemon's page, whose store reload can wait
// behind another process's (or the daemon's own) push or fetch holding the
// store's lock for up to a network timeout.
//
// The reload runs in the background, at most one per store at a time, on
// a context Close cancels. ViewWithin waits for it at most wait; past that
// it returns the view the last completed reload folded (its sync state and
// time-relative fields as of that reload), and when the reload it left
// behind completes with a view that differs from the one it served
// (viewIdentity), the change callback StartWatch set runs so the page
// reads again. A late reload that folds the same view signals nothing: on
// a machine where every reload outlasts wait, a signal per late reload
// would have each page read queue the next reload forever. A store with
// no completed reload yet waits as View does. A reload that fails within
// the wait returns its error, and one that fails later signals nothing.
// Every caller of one Host passes the same staleDays: a reload in flight
// serves every reader that joins it.
//
// The returned view's embedded View, Sync.Unpushed, and Clones may be
// shared with other readers and are read-only; Redact is safe.
func (h *Host) ViewWithin(ctx context.Context, id string, staleDays int, wait time.Duration) (*StoreView, error) {
	st, err := h.store(id)
	if err != nil {
		return nil, err
	}
	st.viewMu.Lock()
	r := st.cur
	switch {
	case r == nil:
		h.mu.Lock()
		if h.closed {
			h.mu.Unlock()
			st.viewMu.Unlock()
			return nil, errors.New("tracker: host closed")
		}
		h.wg.Add(1)
		h.mu.Unlock()
		r = newReload()
		st.cur = r
		go h.reloadLoop(st, id, staleDays)
	case r.read:
		if st.next == nil {
			st.next = newReload()
		}
		r = st.next
	}
	stale := st.snap != nil
	st.viewMu.Unlock()

	var timeout <-chan time.Time
	if stale {
		t := time.NewTimer(wait)
		defer t.Stop()
		timeout = t.C
	}
	select {
	case <-r.done:
		return r.v.shared(), r.err
	case <-timeout:
	case <-ctx.Done():
	}
	st.viewMu.Lock()
	defer st.viewMu.Unlock()
	select {
	case <-r.done: // landed meanwhile
		return r.v.shared(), r.err
	default:
	}
	r.late = true
	if err := ctx.Err(); err != nil {
		r.serve(nil)
		return nil, err
	}
	r.serve(st.snap)
	return st.snap.shared(), nil
}

// serve records v as a view a late read was handed; the caller holds the
// store's viewMu.
func (r *reload) serve(v *StoreView) {
	if !slices.Contains(r.served, v) {
		r.served = append(r.served, v)
	}
}

// differs reports whether v, the view a late reload landed, differs from
// one of the views its late reads were handed. It runs outside viewMu: the
// views are read-only once folded.
func (r *reload) differs(v *StoreView) bool {
	id := viewIdentity(v)
	for _, s := range r.served {
		if s == nil || !bytes.Equal(viewIdentity(s), id) {
			return true
		}
	}
	return false
}

// viewIdentity is the identity of a folded view for the late-reload
// change test: its JSON with View.Now cleared, since Now differs on every
// reload while nothing a page shows changed. It covers every input the
// view is built from (the folded ledger with its uncommitted events, the
// sync state, the commit counts), so a journal-only write by another
// process or a host-sync.json change, which reach the page through this
// signal alone, still signal.
func viewIdentity(v *StoreView) []byte {
	if v == nil {
		return nil
	}
	cp := *v
	if v.View != nil {
		inner := *v.View
		inner.Now = ""
		cp.View = &inner
	}
	raw, err := json.Marshal(&cp)
	if err != nil {
		// Unreachable for a folded view; a nil identity is never equal to a
		// marshalled one, so the change still signals.
		return nil
	}
	return raw
}

// reloadLoop runs st's current reload, then the one queued behind it, until
// none is left.
func (h *Host) reloadLoop(st *store, id string, staleDays int) {
	defer h.wg.Done()
	for {
		st.viewMu.Lock()
		r := st.cur
		st.viewMu.Unlock()
		// The same bound a write waits for the store (writeLockWait): the
		// lock holder may be a sync pushing and fetching.
		ctx, cancel := context.WithTimeout(h.ctx, writeLockWait)
		v, err := h.view(ctx, id, staleDays, func() {
			st.viewMu.Lock()
			r.read = true
			st.viewMu.Unlock()
		})
		cancel()
		st.viewMu.Lock()
		if err == nil {
			st.snap = v
		}
		r.v, r.err = v, err
		close(r.done)
		late := r.late && err == nil
		st.cur, st.next = st.next, nil
		more := st.cur != nil
		st.viewMu.Unlock()
		h.reloads.Add(1)
		if r.late {
			h.lateReloads.Add(1)
		}
		if late && r.differs(v) {
			h.lateChanges.Add(1)
			h.changed()
		}
		if !more {
			return
		}
	}
}

// ReloadStats counts the background reloads behind ViewWithin since the
// host was created.
type ReloadStats struct {
	// Reloads counts the reloads that completed, failed ones included.
	Reloads int64
	// Late counts the completed reloads a read stopped waiting for.
	Late int64
	// LateChanged counts the late reloads that ran the change callback:
	// they folded a view other than one a late read was handed.
	LateChanged int64
}

// ReloadStats returns the reload counters.
func (h *Host) ReloadStats() ReloadStats {
	return ReloadStats{Reloads: h.reloads.Load(), Late: h.lateReloads.Load(), LateChanged: h.lateChanges.Load()}
}

// changed runs the change callback, when one is set.
func (h *Host) changed() {
	h.mu.Lock()
	f := h.onChange
	h.mu.Unlock()
	if f != nil {
		f()
	}
}

// shared returns a copy of v whose own fields (the store, the sync state's
// scalars and warnings) a reader may change, as Redact does; the rest is
// shared with v. nil stays nil.
func (v *StoreView) shared() *StoreView {
	if v == nil {
		return nil
	}
	cp := *v
	cp.Sync.WikiWarnings = slices.Clone(v.Sync.WikiWarnings)
	return &cp
}

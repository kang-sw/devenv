package wstrackerhost

import (
	"context"
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
	// change callback runs when this reload lands.
	late bool
	v    *StoreView
	err  error
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
// behind completes, the change callback StartWatch set runs so the page
// reads again. A store with no completed reload yet waits as View does. A
// reload that fails within the wait returns its error, and one that fails
// later signals nothing.
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
		return nil, err
	}
	return st.snap.shared(), nil
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
		if late {
			h.changed()
		}
		if !more {
			return
		}
	}
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

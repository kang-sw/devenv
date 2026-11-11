package wstrackerhost

import (
	"context"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
)

// ActivityEvent is one event of an ActivityRead.
type ActivityEvent struct {
	ID wsledger.EventID
	TS time.Time
	// Chosen: the read's want chose the event, so Settled and Activity
	// are filled; both are zero otherwise.
	Chosen bool
	// Settled: the event's verdict is final, so its Activity is too
	// (wstracker.Tracker.Activity); an unsettled event comes back in a
	// later read once its verdict changes.
	Settled  bool
	Activity []wstracker.Activity
}

// ActivityRead is one read of a store's events for the dashboard daemon's
// event hooks.
type ActivityRead struct {
	// Entry is the store's registry entry as the read found it.
	Entry StoreEntry
	// Cursor is the read position after this read, for the next one.
	Cursor wsledger.Cursor
	// CatchUp: the read's cursor was of another epoch (another Ledger
	// instance, or the zero cursor), so Events holds every event the store
	// holds, not only those since it.
	CatchUp bool
	// Events are the events the read returned, in display order.
	Events []ActivityEvent
	// View is the store's view state at the host's now.
	View *wstracker.View
}

// ReadActivity reads store id's events since after (wsledger.Ledger.Events)
// and, for each one want chooses, what it did (wstracker.Tracker.Activity),
// with the view at the host's now. want is told whether the read is a
// catch-up (ActivityRead.CatchUp). It reads the store directory as every
// process left it and never calls the remote.
func (h *Host) ReadActivity(ctx context.Context, id string, after wsledger.Cursor, want func(id wsledger.EventID, ts time.Time, catchUp bool) bool) (ActivityRead, error) {
	var out ActivityRead
	err := h.with(ctx, id, func(st *store) error {
		if _, err := st.ledger.Reload(ctx); err != nil {
			return err
		}
		evs, cur := st.ledger.Events(after)
		out = ActivityRead{Entry: st.entry, Cursor: cur, CatchUp: after.Epoch != cur.Epoch, View: st.tracker.View(h.now(), 0)}
		out.Events = make([]ActivityEvent, 0, len(evs))
		for _, e := range evs {
			ae := ActivityEvent{ID: e.ID, TS: e.TS}
			if want(e.ID, e.TS, out.CatchUp) {
				ae.Chosen = true
				ae.Activity, ae.Settled = st.tracker.Activity(e.ID)
			}
			out.Events = append(out.Events, ae)
		}
		return nil
	})
	return out, err
}

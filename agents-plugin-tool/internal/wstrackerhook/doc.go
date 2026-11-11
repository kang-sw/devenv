// Package wstrackerhook runs the dashboard daemon's tracker event hooks:
// local commands the machine's user names in the hooks file (FileName, in
// the daemon's state directory), each run with one tracker event as JSON
// on its stdin (Event). A hook's scope picks its events: the ones that
// concern the store's local member (ScopeMine, the default), or every
// event of the store (ScopeAll). Each hook applies to every registered
// store.
//
// # Delivery
//
// The daemon reads every store when a store may have changed (Nudge, from
// its store watch and poll), and every scanInterval for the due_soon clock.
// Each store keeps a state file under <state dir>/tracker-hooks/<store id>.json:
// the ledger cursor the next read starts at, the ids of the events
// processed (delivered or filtered out) in the last 24 hours, and the
// due_soon events fired. A ledger's cursor places events only within one
// Ledger instance, so after a daemon start every read is a catch-up that
// returns every event: the 24-hour window drops the old ones and the
// processed ids the delivered ones. A store with no state file starts at
// its current head, as does every store when the hooks file appears while
// the daemon runs.
//
// The cursor and the processed ids advance once an event is queued to
// every hook. Each hook has its own serial queue, so its events arrive in
// order; a run is bounded by RunTimeout, a failure is logged and not
// retried, and the events still queued at shutdown are dropped.
//
// Whether an event concerns the member is judged by wstracker.Activity
// against the state folded from the event's causal past, so a read after a
// restart reaches the verdict the first read did.
package wstrackerhook

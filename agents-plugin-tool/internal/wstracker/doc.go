// Package wstracker is the tracker's data-structure layer on top of the
// payload-agnostic wsledger protocol: event types and their rules, the fold
// into items, assignments, and comments, and the derived per-viewer view
// state. The behavior contract is the tracker mockup in
// ai-docs/ref/tracker-mockup (README "Data model", "Event types",
// "Assignments", "Derived state", "View state", "Validation and
// concurrency", and its engine.js), which the golden parity tests pin.
//
// # Layers
//
// The protocol owns the identity events (member.join and the roles.* events
// in the wsledger package doc) and the envelope verdicts: broken, waiting,
// signer and membership flags, and the revocation cut. An event the protocol
// marks Waiting, Flagged, or Ignored keeps that verdict here. A
// protocol-Valid domain event (roles.set_lead, roles.set_priority_levels,
// item.*, comment.*) then gets the domain verdict, judged against the state
// folded from its causal past only, so a late arrival never changes it.
//
// The domain folds the identity events itself from each event's causal
// past (admins, removal, keys, joins); genesis leads and priority come from
// the roles.json passthrough (IdentityState.Genesis). A joined member is a
// root with no lead.
//
// # Reading
//
// The package reads the protocol through Source, which *wsledger.Ledger
// satisfies. It computes the fold order itself from each event's seen (the
// same order wsledger uses: causal, ties by ts, member, clone, seq), and asks
// the source only whether two events are concurrent: for two events in fold
// order, the earlier is in the later's causal past exactly when they are not
// concurrent.
//
// A verdict folds only the events that can affect it: the roles and identity
// events, the threads the event names, the comments it names, and every
// item.assign (which re-applies live "everyone within N" rules everywhere).
// That equals folding the whole causal past, at a fraction of the cost.
package wstracker

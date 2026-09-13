---
title: "Owner-held `/audit` view hides the ownership modal behind Enter"
related:
  260908-feat-ws-pi-subagent-audit-window-and-owner-steering: introduced persistent owner-held steering and the hold/finish/interrupt modal
---

# Owner-held `/audit` view hides the ownership modal behind Enter

## Background

Owner-live Phase 2 acceptance verified steering, settle-time owner attention, reopen, and finish. It also exposed a discoverability mismatch: `/audit` reopens a child in read-only `view` mode even when the durable record remains owner-held from an earlier owner send or `hold`. In that state, Esc follows view-mode behavior and closes the overlay directly. The owner must press Enter to reveal the editor, then press Esc again before the `hold / finish / interrupt` ownership modal becomes reachable.

The child ownership state is real and persists independently of the overlay's temporary mode, so hiding its release actions behind an unrelated editor transition is counterintuitive and makes owner-held state easy to strand accidentally.

## Decisions

- In an owner-held `/audit` view, Esc opens the existing ownership modal directly even when the overlay is still in read-only view mode.
- In a lead-owned read-only view, Esc continues to close the overlay immediately.
- Enter continues to raise the same view instance into interactive mode; it is not required merely to access ownership lifecycle actions.
- Reuse the existing modal and hold/finish/interrupt semantics. Do not create a second view-only modal or a new ownership state.

## Constraints

- Determine modal routing from the authoritative record ownership state, not from a stale overlay-local copy or liveness label that loses owner identity while running.
- `hold` must close without releasing ownership; `finish` must release or reconcile ownership exactly as it does from interactive mode; `interrupt` remains enabled only while the child is running.
- A child the owner has never written to retains the Phase 1 Esc-to-close behavior.
- Preserve scroll position, transcript contents, live subscriptions, and editor state when opening or dismissing the modal.

## Phases

### Phase 1: Make the ownership modal reachable from owner-held view mode

Expose the current record's owner-held state through the existing audit conversation binding and route Esc to the existing ownership modal whenever that state is true, regardless of whether the editor has been opened. Add focused regressions for lead-owned view close, owner-held idle and running views, hold, finish, interrupt enablement, repeated reopen, and no implicit mode or ownership change merely from opening the modal.

---
title: "Owner-held `/audit` view hides the ownership modal behind Enter"
related:
  260908-feat-ws-pi-subagent-audit-window-and-owner-steering: introduced persistent owner-held steering and the hold/finish/interrupt modal
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a9d74078ac56f2f4
sage-review-completeness-reviewed: a9d74078ac56f2f4
completed: 2026-09-13
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

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/audit.ts and agents-plugin-pi/test/audit.test.ts |
| scope.surface | public-interface | existing /audit owner-facing overlay behavior in agents-plugin-pi/src/audit.ts |
| scope.new_public_symbol | no | no new command or exported symbol is specified |
| scope.new_type_contract | no | no new type or signature is specified |
| scope.test_surface | existing | agents-plugin-pi/test/audit.test.ts covers view-mode Esc, modal actions, and ownership transitions |
| complexity.reuse_points | confirmed | existing createAuditChannel, OwnerSteeringComponent, and isOwnerHeld in agents-plugin-pi/src/audit.ts and agents-plugin-pi/src/spawner.ts |
| complexity.side_effect_risk | moderate | Esc routing must preserve owner lifecycle behavior and lead-owned view closing |
| risk.correctness | moderate | a wrong ownership predicate could expose release actions or close the wrong view |
| risk.fit | moderate | the change must retain the shared audit conversation binding and modal semantics |
| risk.test | moderate | existing unit seams cover the view and modal paths, but both ownership states need regressions |
| risk.security_or_contract | moderate | changes the owner-facing lifecycle-control contract of the existing /audit command |

## Phases

### Phase 1: Make the ownership modal reachable from owner-held view mode

Expose the current record's owner-held state through the existing audit conversation binding and route Esc to the existing ownership modal whenever that state is true, regardless of whether the editor has been opened. Add focused regressions for lead-owned view close, owner-held idle and running views, hold, finish, interrupt enablement, repeated reopen, and no implicit mode or ownership change merely from opening the modal.

### Result (93ffda9e) - 2026-09-13

- Routed read-only `/audit` Esc through the existing owner-steering modal when the live registry record is owner-held; lead-owned and never-owner views still close directly.
- Preserved the existing modal lifecycle: hold keeps ownership, finish performs the existing handoff, and interrupt remains limited to running children.
- Added regressions for idle and running owner-held views, modal cancel/reopen, no implicit mode or ownership change, absent ownership, and replacement of the registry record after open to prove Esc reads current ownership.
- Verification: `cd agents-plugin-pi && npm test -- test/audit.test.ts` (47 passed); full `npm test` ran 1,798 tests with one unrelated `test/session-bootstrap-guard.test.ts` launcher assertion failure (`write EPIPE` instead of the expected process-exit text).
- Decisions: left the ownership callback optional because `/answer` is always interactive and does not require an audit-specific registry dependency.


## Resolution (2026-09-13)

Phase 1 complete: owner-held `/audit` views now expose the existing ownership modal directly on Esc, with regression coverage for current registry ownership.

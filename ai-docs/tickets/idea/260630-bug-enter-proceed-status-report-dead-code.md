---
title: "Bug: enter.proceed status-report route is dead code"
---

## Finding

Static analysis of `proceed_resolver.go` shows `status-report` is a valid
`NEXT:` enum value and has a corresponding `NextInstruction` case, but no
routing branch in `selectProceedRoute` ever sets `next = "status-report"`.
The route label is unreachable at runtime.

Surfaced during lead-proceed playbook diet (2026-06-30, session
`glazing-recapture-cedar-facecloth-50`). Explorer found no call site producing
this value across all routing conditions.

## Options

1. **Remove** — delete the `status-report` case from `proceedNextInstruction`
   and its enum entry. If the route was intentional but never implemented,
   document the intent in a separate ticket.
2. **Implement** — add a routing branch that produces `status-report` when
   the target is a status query rather than an implementation request.

## Decision

Remove. The `status-report` route was added during autonomous route design
(no known real use case ever wired it up), and pure status queries are
already served by `ws/tickets.status` and `lead-discuss`'s git-log-based
session-continuity path. Delete the `status-report` `NEXT:` enum value and
its `proceedNextInstruction` case; do not implement a routing branch for it.

## Related

- `260630-epic-skill-playbook-diet` — diet session that surfaced this

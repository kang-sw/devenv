---
title: "Bug: enter.proceed status-report route is dead code"
sage-review-completeness: completed
sage-review-design: completed
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

Note for the implementer: `next` is a plain string, not a separate declared
enum/const — the only two sites are the `proceedNextInstruction` switch case
in `proceed_resolver.go` and the direct assertion in
`agents-plugin-tool/internal/mcp/session_state_test.go:1196-1197`, which must
be removed or updated alongside the case. The route was deliberately reserved
by `260627-feat-enter-proceed-deterministic-verdict-engine`'s original closed
`NEXT` set, not an accidental leftover — this removal closes out that unused
part of the contract, not merely deletes stray code.

## Phases

### Phase 1: Remove the dead status-report route

Delete the `status-report` case from `proceedNextInstruction` in
`agents-plugin-tool/internal/mcp/proceed_resolver.go`, and update or remove
the direct assertion at
`agents-plugin-tool/internal/mcp/session_state_test.go:1196-1197` that
depends on it. No routing branch anywhere ever produces this value, so no
other call site changes. Verification: `go test ./...` in
`agents-plugin-tool/` passes; grep for `status-report`/`status_report` across
the repo turns up no remaining production or test references to the removed
case (the `status_report` fact enum value on the `category` fact itself is
unrelated caller input shape and is out of scope for this removal).

## Spec Impact

No spec change. The `status-report` route was never reachable at runtime (no
caller ever observed it), so removing it is not a caller-visible behavior
change and no existing spec documents it as an observable contract.

## Related

- `260630-epic-skill-playbook-diet` — diet session that surfaced this

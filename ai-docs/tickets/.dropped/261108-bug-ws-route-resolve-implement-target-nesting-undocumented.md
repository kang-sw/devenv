---
title: route.resolve_implement rejects a top-level target; worker playbook does not say it nests under params
dropped: 2026-11-10
---

# route.resolve_implement rejects a top-level target; worker playbook does not say it nests under params

## Background

Dogfood surprise reported by the disk-assets dev-mode worker (2026-11-08,
ticket 261108-feat-ws-dashboard-disk-assets-dev-mode): calling
`ws/route.resolve_implement` with a top-level `target` was refused; the call
only succeeds as `params: {target: {...}}`. The worker playbook text that
instructs the call does not state the nesting, so a worker following it
literally fails the first call.

## Direction (unconfirmed)

Either accept a top-level `target` as an alias, or state the `params`
nesting where the playbook names the call. Verify which shape the schema
advertises in `tools/list` before choosing.


## Resolution (2026-11-10)

Does not reproduce: since 2026-10-30 (4d6b807d2) the route.resolve_implement schema accepts a top-level `target` (`additionalProperties: true`; `routeEnvelope` in `internal/mcp/session_state.go` accepts the unwrapped form), the playbooks document the top-level form, and this session's workers called it that way successfully. User, 2026-11-10 board (571) ok.

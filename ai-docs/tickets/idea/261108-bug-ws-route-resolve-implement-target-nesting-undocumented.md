---
title: route.resolve_implement rejects a top-level target; worker playbook does not say it nests under params
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

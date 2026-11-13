---
title: "Ticket promotion to ready accepts a Route Facts value the route parser rejects"
---

# Promotion accepts an invalid route-fact enum

Duplicate of 261007-bug-ready-gate-accepts-invalid-route-fact-values.

## Background

Dogfood, 2026-11-11. `261110-bug-ws-tracker-store-lock-network-split` reached
`ready/` with both sage stages stamped `completed` while its `## Route Facts`
held `scope.new_public_symbol | likely`. The fact populator wrote that value,
and neither `tickets.sage_gate` (which refuses only a missing section),
`tickets.sage_stamp`, nor `tickets.move` validated it. The first worker's
`ws/route.resolve_implement` then failed with
`facts.scope.invalid new_public_symbol "likely": want one of yes, no, unknown`,
treated every fact as `unknown`, and would have allocated a lite review to a
high-risk lock split; the worker stopped (c) instead.

## Open questions

- Should the ready gate (`sage_gate` or `tickets.move` to ready) parse Route
  Facts with the same parser `route.resolve_implement` uses and refuse an
  invalid value?
- Should the fact-populator playbook list the allowed values per fact so it
  cannot write `likely`?
- Should one invalid row degrade only that fact to `unknown` rather than the
  whole table?

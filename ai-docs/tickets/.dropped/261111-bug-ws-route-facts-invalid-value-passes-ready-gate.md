---
title: "Route Facts with an invalid enum value pass the ready gate and fail only at run time"
---

# Route Facts invalid value passes the ready gate

Duplicate of 261007-bug-ready-gate-accepts-invalid-route-fact-values.

## Observation

2026-11-11 dogfood: `261110-feat-ws-tracker-daemon-event-hook` reached
`ready/` with `scope.test_surface | new` in its `## Route Facts` table. The
allowed values are `none`, `existing`, `new-files`, `unknown`. Neither the
fact populator nor `tickets.sage_gate(landing: "ready")` (which already
refuses a ticket without the section) flagged it. The error surfaced only
when a worker called `route.resolve_implement`:
`facts.scope.invalid test_surface "new": want one of none, existing, new-files, unknown`,
and the route then treated every fact as `unknown` (review allocation
`lite` despite high-risk facts). The worker stopped (stop c), costing a
spawn round.

## Direction

- Validate the Route Facts table with the same parser `route.resolve_implement`
  uses at `tickets.sage_gate(landing: "ready")` (and/or `tickets.move` to
  ready), refusing with the parser's message.
- Optionally have the fact-populator playbook list the allowed values per
  fact key so it does not invent near-miss values.

---
title: "claude-mod dashboard engine tests flake under machine load (first flush never reaches the intake)"
---

# claude-mod dashboard engine tests flake under load

## Observation (2026-11-10)

Reported by the delegate of the dashboard meter cadence hotfix (f56fa8d42):
`agents-plugin/hooks/claude-mod/dashboard-engine.test.ts` (and its wsflow
copy) passes 38/38 when it has the machine to itself, but runs that overlap
another test process fail 1-2 tests, a different one each run. Each failure
lands at about 1.1 s with the intake never reached, including permission and
whiteboard tests unrelated to status. The full ws suite
(`claude plugin test agents-plugin`) failed one unrelated test the same way
at the pre-change HEAD, so the flake predates the hotfix.

## Directions (not decided)

- Find the wall-clock dependency: the commit 7e6b2a712 AI Context notes the
  engine's `now` edge is wall time while timers run on the mock clock; a
  first-flush wait bounded by wall time would explain load sensitivity.
- Drive the first flush from the mock clock, or widen the bound only where a
  real-time wait is intended.

## Constraints

- Mirror test changes to `agents-plugin-wsflow/hooks/claude-mod/`.
- Run tests niced (`nice -n 19`) and scoped locally.

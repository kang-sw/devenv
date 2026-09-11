---
title: "ticket worker reports stop none while later phases remain unfinished"
related:
  260911-feat-ws-git-merge-lead-owned-merge-authority: surfaced when Phase 1 landed while Phase 2 remained ready
---

# ticket worker reports stop none while later phases remain unfinished

## Background

During a `lead-run` cycle, the elevated ticket worker implemented and recorded
Phase 1 of `260911-feat-ws-git-merge-lead-owned-merge-authority`, deliberately
left Phase 2 outside that invocation's earliest-unfinished-phase scope, and
returned `stop: none`.

The lead-run contract currently interprets `stop: none` as meaning that the
ticket is closed on its branch. Authoritative ticket state instead remained
`ready`, with Phase 1 carrying a Result and Phase 2 unresolved. This makes the
terminal report ambiguous to its lead caller and can cause session notes or
queue-terminal decisions to be recorded against a ticket that is still active.

## Evidence

- Worker report: `stop: none` and `omitted: Phase 2 remains ready, outside this
  invocation's earliest-unfinished-phase scope`.
- `tickets.query` immediately after the report returned status `ready`,
  `result_present: true`, and unresolved Phase 2.
- The goal branch was clean at `20863462`, so the mismatch was not caused by
  uncommitted ticket state.

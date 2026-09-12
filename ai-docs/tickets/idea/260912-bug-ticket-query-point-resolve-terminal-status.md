---
title: "Make tickets.query exact-stem resolution consistent for terminal tickets"
---

# Make tickets.query exact-stem resolution consistent for terminal tickets

## Background

After closing `260909-epic-ws-worker-interpreter-refoundation`, calling
`tickets.query` with only its exact `ticket_stem` returned `ticket not found`.
The tool schema says that an exact stem given alone point-resolves the ticket
and returns its status metadata, without stating that terminal inventories are
excluded. This makes post-close verification unexpectedly fail unless callers
already know to add terminal-inventory flags.

## Phases

### Phase 1: Reconcile exact-stem lookup behavior and its contract

Reproduce the terminal-ticket lookup, determine from existing contract evidence
whether the implementation or schema description is wrong, and align behavior,
documentation, and regression coverage without changing discovery-query defaults.

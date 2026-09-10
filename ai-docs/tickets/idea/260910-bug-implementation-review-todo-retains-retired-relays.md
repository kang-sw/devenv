---
title: Align implementation review todos with the current ticket-worker protocol
parent: 260909-epic-ws-worker-interpreter-refoundation
---

# Align implementation review todos with the current ticket-worker protocol

## Background

During execution of `260910-refactor-ready-only-actionable-ticket-gates`,
`route.resolve_implement` installed a partitioned review todo that still tells
the worker to use `implementer-relay`, permits three reviews, and escalates a
remaining Critical finding through `implementer-elevated`. The rendered
`ticket-worker` instead owns its fixes, allows two review rounds, and stops on
an unresolved Critical finding after round two.

The obsolete clauses are emitted by `implementReviewRelayClause` and
`implementReviewCriticalBranchClause` in
`agents-plugin-tool/internal/mcp/session_state.go`; tests in
`session_state_test.go` currently pin them. The current prompt contract is in
`agents-plugin/rsrc/ticket-worker/ticket-worker.md` and its worker-protocol
include. A caller following the installed todo can take a different review
path from the playbook that created the route.

## Proposed scope

Reconcile the runtime-generated review instructions and their tests with the
current worker protocol, retaining independent review and a single unambiguous
round budget. Confirm the intended authority before changing review semantics.

## Verification

Exercise implementation routing for each review allocation and compare the
emitted todo instructions with the shipped worker's review ownership, round
budget, and stop condition.

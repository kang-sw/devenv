---
title: Research table-driven authoring for lead-proceed route context and route selection
related-mental-model:
  - workflow-skills
related:
  260605-research-ws-native-subagent-pivot: lead-proceed is the front door that protects the pivot from touching code before routing through tickets, specs, or explicit direct-implementation gates
---

# Research table-driven authoring for lead-proceed route context and route selection

## Background

Dogfooding `lead-proceed` exposed that its invoke-time route context and route
selection guidance is difficult to scan. The current prose reads like a long
chain of conditionals: it is precise enough to execute, but the reader must
track several facts, dependent judgments, and early-stop cases at once.

The proposed cleanup direction is to split the route logic into two table-driven
layers:

- **Route Facts**: a normalized table of facts such as `target-kind`,
  `has-ticket`, `status`, `category`, `actionable`, `freshness`,
  `discussion-needed`, and `scope-blocked`, with allowed values and the judge or
  source that sets each value.
- **Route Matrix**: a priority-ordered route table where rows map combinations
  of normalized facts to `NEXT` actions such as `lead-discuss`,
  `lead-write-ticket`, `lead-implement`, or a stop/report outcome. Empty cells
  would mean "any value" so the table stays readable without becoming a full
  cross-product truth table.

This ticket keeps the idea at research level until the authoring shape is
validated against the existing `lead-proceed` semantics.

## Research question

Would a Route Facts table plus a priority Route Matrix make `lead-proceed`
easier to read and more reliable to execute without changing the actual routing
contract?

Evaluate:

- Whether every existing `lead-proceed` branch can be expressed as a normalized
  fact, a matrix row, or a short derivation-order rule.
- Which facts are independent and which must be derived after earlier reads
  (for example ticket status before freshness, category before implementation
  scope, and discussion blockers before code routing).
- Whether a priority matrix with "first matching row wins" is less ambiguous
  than the current prose while preserving all early-stop behavior.
- Whether table wording helps fresh readers distinguish ticket-path routing,
  inline brief routing, container-ticket stops, stale-ticket refreshes, and
  direct implementation skips.
- Whether the cleanup should live only in the playbook body or also affect
  mirrored wsflow resources, generated runtime artifacts, or tests that assert
  shipped prompt content.

## Candidate authoring shape

The likely final shape is:

1. A short "Derive facts in this order" list for dependencies that cannot be
   captured by a flat table alone.
2. A Route Facts table with columns such as `fact`, `values`, `source`, and
   `judge`.
3. A Route Matrix with columns such as `priority`, `target-kind`, `has-ticket`,
   `status`, `category`, `freshness`, `discussion`, `scope-blocked`, `NEXT`, and
   `action`.
4. A semantic preservation checklist that maps old prose branches to new matrix
   rows before any playbook edit is merged.

## Non-goals

- Do not change `lead-proceed` routing behavior as part of the research itself.
- Do not promote this directly to implementation-ready without a semantic drift
  audit against the current playbook.
- Do not collapse route facts into one giant truth table; dependent judgments
  still need a small derivation order.

## Promotion notes

Promote this only after a reader can compare the current playbook to a draft
Route Facts / Route Matrix rewrite and identify either:

- a semantics-preserving cleanup that should become a focused refactor ticket;
  or
- a reason table-driven authoring would hide important ordering constraints and
  should be rejected or narrowed.

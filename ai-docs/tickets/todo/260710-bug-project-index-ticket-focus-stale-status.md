---
title: project index Ticket Focus advertises absent ready tickets
related:
  260605-epic-ws-playbook-factory-pivot: workflow routing depends on accurate project-memory status
sage-review-completeness: required
sage-review-design: blocked
---

# project index Ticket Focus advertises absent ready tickets

## Background

Codex dogfooding loaded `ai-docs/_index.md` before using wsflow discovery.
Its `## Ticket Focus` section lists multiple tickets as `ready`, including
`260702-bug-config-unset-asymmetry`. In the same checkout,
`ai-docs/tickets/ready/` does not exist and `wsflow/tickets.list` returns an
empty ready list. `wsflow/project_tree` agrees with the live ticket state.

The index therefore conflicts with both filesystem-backed workflow discovery
and the repository's current branch, risking an implementation route toward a
nonexistent ticket.

## Phases

### Phase 1: Reconcile active ticket focus with the live ticket inventory

Determine whether the stale focus entries should be removed, replaced by the
current active tickets, or restored from an omitted history boundary. Update
`ai-docs/_index.md` to match the resolved live inventory.

Add a focused guard or documented regeneration procedure so Ticket Focus does
not claim an absent ticket is implementation-ready. Verify the result against
`wsflow/tickets.list(statuses: ["ready"])` and the ticket directories.

## Blocked (2026-07-13)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Recurrence-prevention mechanism (automated guard vs. documented manual regeneration procedure vs. some other shape) is an unresolved open design question, not something to leave to implementer discretion | important | missing |

The mechanical reconciliation half of Phase 1 (fix the 7 confirmed stale
entries against current ticket directories) is autonomous and decidable. Only
the guard/mechanism half needs the user to pick a direction before this can
promote further:

- (a) an automated check wired into `tickets.close`/`tickets.move` or an
  MCP-level verification, or
- (b) a documented manual regeneration procedure (e.g. an addition to
  `WORKFLOW.md` or `ai-docs/spec/documentation-system.md`), or
- (c) some other shape.

`WORKFLOW.md` already states the rule ("only `ready/` entries are direct
implementation targets") but that rule alone evidently did not prevent this
drift, so re-stating it is unlikely to satisfy the "guard" half by itself.

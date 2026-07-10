---
title: project index Ticket Focus advertises absent ready tickets
related:
  260605-epic-ws-playbook-factory-pivot: workflow routing depends on accurate project-memory status
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

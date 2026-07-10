---
title: lead-write-ticket playbook uses a stale tickets.create schema
related:
  260605-epic-ws-playbook-factory-pivot: playbooks are runtime-distributed contracts and must match their exposed MCP schemas
sage-review-design: required
sage-review-completeness: required
---

# lead-write-ticket playbook uses a stale tickets.create schema

## Background

Codex dogfooding of `wsflow:lead-discuss` rendered the installed
`lead-write-ticket` playbook. Its new-ticket procedure instructs callers to
invoke `wsflow/tickets.create(session_key, type, title, status)`. The live
`wsflow/tickets.create` tool instead accepts `session_key`, `initial_state`,
and `stem`; it has no `type`, `title`, or `status` parameters.

The discrepancy makes the documented creation path fail before a ticket body
can be populated. The runtime tool itself successfully created this ticket
with `initial_state: "idea"` and the semantic stem.

## Phases

### Phase 1: Align the ticket-authoring playbook with the runtime schema

Update the shipped `lead-write-ticket` playbook so its `tickets.create`
invocation exactly uses the public runtime schema. Preserve the existing
follow-up flow that fills the generated ticket body and commits it through
`wsflow/git.commit`.

Verify the rendered playbook against the installed `tickets.create` tool
schema and add or update a drift-focused regression test where the project
has an established playbook/schema contract test surface.

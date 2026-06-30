---
title: lead-write-ticket dogfood bypassed tickets.create
related:
  260625-feat-ws-session-state-machine: typed ticket/create and session-state surfaces should reduce manual file authoring mistakes
related-mental-model:
  - workflow-skills
  - mcp-runtime
sage-review: required
completed: 2026-06-30
---

# lead-write-ticket dogfood bypassed tickets.create

## Background

During ticket authoring dogfood, the lead loaded `lead-write-ticket`, read the
ticket conventions and template, then manually created a ready ticket file
instead of discovering and calling `ws.tickets.create`.

This is a workflow issue rather than just an operator mistake. The playbook
mentions templates and conventions prominently, but the creation path did not
make the MCP creation primitive sufficiently salient at execution time. The
result is that a lead can bypass the runtime's intended creation surface, losing
centralized behavior such as default frontmatter, sage-review initialization, and
future creation-time policy.

## Notes

- `ws.tickets.create` exists and creates dated ticket stubs under
  `ai-docs/tickets/<status>/`.
- The failure happened while creating
  `260627-feat-enter-implement-deterministic-verdict-engine`.
- The follow-up should clarify `lead-write-ticket` so Create Ticket invokes
  `ws.tickets.create` before body authoring, and so leads discover the tool
  before falling back to manual file creation.
- The fallback path should remain available only when the MCP create tool is
  unavailable or fails with a blocker.

## Decision (260629 sweep)

Fix (A+B combined): A — In lead-write-ticket's `On: Create Ticket` procedure, call `ws/tickets.create` before manual body drafting; fall back to manual only when the tool is unavailable or errors. B — In lead-discuss and lead-sprint, add an explicit constraint that ticket creation must route through lead-write-ticket, not through convention.read + Write directly. Both changes are prose-only playbook edits.


## Resolution (2026-06-30)

A: Added Create Stub step to lead-write-ticket On: Create Ticket calling ws/tickets.create before body drafting, with manual fallback. B: Added ticket-creation routing constraint to lead-discuss and lead-sprint invariants: creation must route through lead-write-ticket, not convention.read + Write directly.

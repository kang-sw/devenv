---
title: lead-write-ticket dogfood bypassed tickets.create
related:
  260625-feat-ws-session-state-machine: typed ticket/create and session-state surfaces should reduce manual file authoring mistakes
related-mental-model:
  - workflow-skills
  - mcp-runtime
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

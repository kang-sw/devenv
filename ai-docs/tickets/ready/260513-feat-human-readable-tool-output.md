---
title: Human-readable workflow tool output
parent: 260513-epic-workflow-question-loop-hygiene
spec:
  - 260519-workflow-command-readable-output-defaults
related-mental-model:
  - git-workflow-tools
  - mcp-runtime
---

# Human-readable workflow tool output

## Background

Several workflow tools already default to compact text output, but some MCP and
CLI surfaces still return JSON by default. This creates unnecessary friction in
agent-facing and human-facing workflow loops, especially around Git and
documentation discovery commands.

This ticket is backlog capture only for now. It should not be implemented as
part of the immediate workflow-skill cleanup unless explicitly promoted.

## Phases

### Phase 1: Audit remaining JSON-default surfaces

Identify MCP and CLI commands whose default output is still JSON even though a
human-readable text form would be the normal interactive output.

Acceptance criteria:

- Audit `git.commit`, Git CLI mirrors, config display, ticket/spec/mental-model
  discovery CLI mirrors, and reference tracing CLI output.
- Preserve explicit JSON compatibility paths for structured consumers.
- Record which surfaces should remain JSON because they are protocol payloads,
  cache files, or internal state files rather than interactive output.

### Phase 2: Add human-readable defaults

Implement text defaults for the audited workflow command surfaces while keeping
explicit JSON output available.

Acceptance criteria:

- `git.commit` MCP output reports the created hash, title, staged paths, and
  ticket-result or edition detections in compact text by default.
- CLI mirrors print readable text by default and offer an explicit JSON mode
  where compatibility requires it.
- Git CLI mirrors preserve the original Git output shape as much as practical
  instead of wrapping native Git output in ws-specific JSON by default.
- Tests cover both default text output and explicit JSON output.
- Specs and mental models describe the default text / explicit JSON rule.

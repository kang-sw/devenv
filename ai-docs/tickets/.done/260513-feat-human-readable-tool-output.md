---
title: Human-readable workflow tool output
parent: 260513-epic-workflow-question-loop-hygiene
spec:
  - 260519-workflow-command-readable-output-defaults
completed: 2026-05-19
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

### Result (3f52c36) - 2026-05-19

Audited and updated the remaining workflow output surfaces named by the ticket:
`git.commit` MCP output, Git CLI mirrors, `config show`, ticket/spec/mental-model
discovery CLI mirrors, and `references trace`.

Protocol and compatibility surfaces remain structured: MCP response envelopes,
runtime capability/metadata payloads, API async job state, agent runtime state,
cache files, and explicit `format=json` / `--format json` paths.

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

### Result (3f52c36) - 2026-05-19

Implemented readable defaults for the audited surfaces. `git.commit` now returns
a compact text summary by default and structured JSON only when requested.
`ws-mcp` CLI mirrors for Git, `config show`, ticket/spec/mental-model discovery,
and reference tracing now default to text and keep `--format json` compatibility.

Git CLI read mirrors preserve native Git text where practical: `status`, `log`,
and `merge-base` print direct Git output by default, while `diff` keeps the
existing diff output path including the workflow's range-less untracked-file
behavior. Tests cover text defaults and JSON compatibility.

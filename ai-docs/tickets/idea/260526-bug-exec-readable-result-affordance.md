---
title: Exec result readability and wait affordance
parent: 260524-epic-async-exec-job-surface
related:
  260524-feat-exec-job-core-text-readers: introduced the current exec core, JSON response shape, key format, and non-waiting result behavior
  260524-feat-exec-output-ask: adjacent large-output UX, but does not own basic result/status affordances
spec:
  - 260512-mcp-llm-readable-output-defaults
  - 260524-exec-job-mcp-tools
related-mental-model:
  - mcp-runtime
---

# Exec result readability and wait affordance

## Background

Dogfooding the `exec.*` MCP tools exposed three lead-facing usability problems:

- `exec.spawn`, `exec.shell`, `exec.status`, `exec.result`, and raw-reader
  responses always use JSON serialized into MCP text content. When stdout itself
  is JSON, the default response becomes escaped JSON inside escaped JSON, which
  is hard for a lead model to scan.
- `exec_key` values currently include a nanosecond timestamp plus 16 hex
  characters, for example `exec-1779771339137812722-bcd0ce257405f478`. The
  token is longer than the model needs and increases retyping mistakes. Actor-
  scoped or worktree-scoped uniqueness plus a shorter random suffix should be
  enough for ordinary follow-up calls.
- `exec.result` does not wait for a running job. It returns an immediate
  `not terminal` error with no `timeout_seconds` affordance, so callers tend to
  loop through `exec.status` polling even when they intended to wait for the
  terminal result.

These are not correctness failures in the process substrate. They are
lead-facing UX issues that make the new exec surface harder to use reliably.

## Evidence

- `agents-plugin-tool/internal/mcp/server.go` dispatches every `exec.*` tool
  through `toolJSONResponse`.
- `agents-plugin-tool/internal/execjob/execjob.go` generates keys as
  `exec-<unix-nano>-<16 hex>` and validates that exact shape.
- `agents-plugin-tool/internal/execjob/execjob.go` makes `Result` return an
  error immediately when the job is not terminal.
- Live dogfood on 2026-05-26 reproduced escaped JSON output, long keys, and the
  immediate non-terminal `exec.result` response.

## Phases

### Phase 1: Improve exec follow-up readability and waiting

Audit and improve the lead-facing exec follow-up surface while preserving the
durable job model:

- add compact readable default formatting for exec launch/status/result/abort
  responses, with explicit JSON retained for structured callers if needed;
- shorten new exec keys to the minimum token that preserves actor/worktree-local
  practical uniqueness and keep any required legacy-key compatibility for
  existing persisted jobs;
- decide whether `exec.result` should accept `timeout_seconds`, provide a
  separate wait primitive, or return stronger follow-up guidance that prevents
  status polling loops;
- update MCP tests, execjob tests, runtime docs, and related specs for the
  chosen caller-visible behavior.

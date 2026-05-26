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
completed: 2026-05-26
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

## Decisions

- Do not keep a `format: json` escape hatch for the lead-facing `exec.*`
  surface. The point of this fix is to make the default and only MCP response
  shape readable to models.
- Format `exec.result` as text with metadata above a clear separator and raw
  command output below it. The separator may use an obvious marker such as
  `==========` so JSON stdout remains visually raw instead of becoming escaped
  payload inside response JSON.
- Keep metadata compact and stable enough for a model to reuse the `exec_key`,
  inspect status, and decide whether it needs `exec.raw.*`, without requiring a
  JSON parser.

## Phases

### Phase 1: Improve exec follow-up readability and waiting

Audit and improve the lead-facing exec follow-up surface while preserving the
durable job model:

- replace JSON text responses for the lead-facing exec MCP tools with compact
  readable text responses;
- make `exec.result` render compact metadata above a separator and raw stdout
  and stderr content below the separator when inline output is available;
- shorten new exec keys to the minimum token that preserves actor/worktree-local
  practical uniqueness and keep any required legacy-key compatibility for
  existing persisted jobs;
- decide whether `exec.result` should accept `timeout_seconds`, provide a
  separate wait primitive, or return stronger follow-up guidance that prevents
  status polling loops;
- update MCP tests, execjob tests, runtime docs, and related specs for the
  chosen caller-visible behavior.

### Result (a9a80660) - 2026-05-26

Implemented readable text output for the exec MCP lifecycle and raw-reader
surface, including separator-delimited inline stdout/stderr for `exec.result`
and raw-reader text/match sections. `exec.result` now accepts
`timeout_seconds`: omitted or zero timeout returns prompt running guidance
without an MCP error, while a positive timeout waits for terminal completion or
returns the same running guidance on timeout.

New exec jobs now use short `exec-<8hex>` keys with collision checks against
SQLite metadata and legacy job directories. Legacy
`exec-<unix-nano>-<16hex>` keys remain accepted so existing persisted jobs and
raw readers still work after upgrade.

Verification covered manager-level key/wait behavior, MCP readable lifecycle
and raw-reader formatting, unescaped JSON-shaped stdout, non-blocking and
waiting `exec.result`, and wsflow no-agent visibility:

- `cd agents-plugin-tool && go test ./internal/execjob ./internal/mcp ./cmd/ws-mcp`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `ws/spec_index.verify`

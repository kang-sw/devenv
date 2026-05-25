---
title: Async exec output reader
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
  - api-documentation-cache
  - plugin-runtime
---

# Async exec output reader

## Background

Large shell command output can accidentally pollute the lead context when a tool
returns stdout or stderr inline. Add a ws MCP exec job surface that starts
commands asynchronously, stores raw output outside the tool result, and lets a
light reader agent answer focused questions about large outputs.

The primary goal is mechanical context-bloat prevention. The default result path
must never return more than a small bounded output payload to the lead, even when
the underlying command produces much larger stdout or stderr.

## Decisions

- Use separate launch tools for argv execution and shell execution:
  `exec.spawn` starts a command without shell parsing, while `exec.shell` starts
  an explicit shell command string.
- Share the rest of the lifecycle across both launch modes:
  `exec.status`, `exec.result`, `exec.ask`, and `exec.abort` all operate on the
  same `exec_key` job records.
- Prefer `exec.abort` over `exec.cancel` so the operation reads as terminating
  an execution job rather than cancelling result retrieval or overlapping with
  `agents.cancel` semantics.
- Hide all `exec.*` tools in wsflow no-agent mode. Exposing only the non-agent
  subset would still add an arbitrary shell execution surface to the reduced
  product mode and leave `exec.ask` unavailable.

## Constraints

- `exec.spawn` must be the safe default path for structured argv execution.
  Shell syntax such as pipes, redirects, glob expansion, and compound commands
  belongs only in `exec.shell`.
- `exec.shell` should accept one command string, not a mixed command-plus-args
  shape, so escaping responsibility remains clear.
- Raw stdout and stderr must be persisted to job-owned files with byte counts.
  Normal MCP tool results should return only bounded text plus metadata.
- If combined stdout and stderr are at most 4096 bytes, `exec.result` may return
  the captured output inline. Larger output must return metadata such as total
  size, per-stream sizes, exit status, and follow-up guidance without including
  the raw payload.
- `exec.ask` must use the `light` model alias and read from the persisted output
  files. With `agent_resume: true` it should reuse the same reader session for
  follow-up questions; with `agent_resume: false` it should answer in a fresh
  reader context.
- `exec.abort` must stop the running process while preserving job state and any
  captured partial output for later `exec.result` and `exec.ask` inspection.

## Prior Art

- `subquery` already returns an async key immediately and collects work through a
  later result surface.
- `api.ask_async`, `api.status`, `api.result`, and `api.cancel` provide the
  closest recoverable async job precedent.
- `agents.result`, `agents.tail`, and debug stream handling provide precedent for
  bounded normal output with separate raw diagnostics.

## Phases

### Phase 1: Add exec job state and launch tools

Add a file-backed exec job manager and MCP launch surfaces:

- `exec.spawn(cmd, args?, cwd?, env?, timeout_seconds?, stdin?)`
- `exec.shell(command, cwd?, env?, timeout_seconds?, stdin?, shell?)`

Both tools should return an `exec_key` immediately and store a shared job record
with launch mode, command metadata, resolved cwd, process id or process group
metadata, timestamps, timeout configuration, stdout/stderr paths, byte counts,
and terminal status when available.

The launch result should include only bounded metadata and follow-up guidance.
It must not inline command output. Tool schemas, dispatch, profile gates,
runtime capability metadata, and CLI mirrors should stay aligned with the MCP
surface selected for the implementation slice.

### Phase 2: Add status, bounded result, and abort

Add shared lifecycle tools for launched jobs:

- `exec.status(exec_key)`
- `exec.result(exec_key, timeout_seconds?)`
- `exec.abort(exec_key)`

`exec.status` should reconcile dead or terminal workers and return compact job
metadata. `exec.result` may optionally wait, then return inline output only when
combined stdout and stderr are at most 4096 bytes. For larger outputs it should
return `output_truncated: true`, stream byte counts, total output size, exit
status, duration, and `exec.ask` follow-up guidance.

`exec.abort` should terminate an active execution job and preserve the job files
and partial output. Already terminal jobs should report their existing terminal
state rather than failing as if abort were impossible.

### Phase 3: Add light output-reader questions

Add `exec.ask(exec_key, query, agent_resume?: true)`.

The tool should register or reuse a light reader agent that receives only the
query, job metadata, and references to the persisted stdout/stderr files. The
reader prompt should answer from those files and avoid pasting large raw output
back to the lead unless the answer genuinely requires a short excerpt.

When `agent_resume` is true or omitted, follow-up questions for the same
`exec_key` should reuse the reader session. When false, the runtime should use a
fresh reader context so stale assumptions from earlier questions do not affect
the answer.

### Phase 4: Gate product modes and verify integration

Hide every `exec.*` tool and any CLI mirror in wsflow no-agent mode. Add tests
for advertised tool lists, explicit hidden-tool calls, runtime capability
metadata, and launcher contract drift.

Add integration tests for small-output inline results, large-output metadata
results, stdout/stderr persistence, shell-vs-spawn command shapes, abort
preserving partial output, process timeout behavior, reader-agent reuse, fresh
reader contexts, and profile visibility. Update MCP, named-agent, plugin
runtime, and prompt-bundle documentation as needed before promoting this ticket
to `ready/`.

## Drop Reason

Dropped on 2026-05-24 because the ticket mixed epic-level decomposition with
multiple implementation contracts. The scope was absorbed by
`260524-epic-async-exec-job-surface`.

The first actionable child is `260524-feat-exec-job-core-text-readers`, covering
the non-model exec job core and bounded text readers. The model-backed
`exec.ask` design is intentionally deferred for follow-up discussion and a later
child ticket.

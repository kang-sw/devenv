---
title: Exec job core and text readers
parent: 260524-epic-async-exec-job-surface
related:
  260513-feat-async-exec-output-reader: original broad ticket absorbed by parent epic
  260513-research-streamable-http-mcp-transport: adjacent daemon and reconnect lifecycle discussion
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
  - plugin-runtime
---

# Exec job core and text readers

## Background

The first async-exec slice should deliver the usable non-model core: launch
commands, preserve stdout and stderr in durable job-owned files, return small
completed outputs inline when safe, and expose bounded text readers for larger
outputs. This keeps context-bloat prevention useful before adding a model-backed
`exec.ask` layer.

## Decisions

- Expose separate launch tools:
  - `exec.spawn(cmd, args?, working_dir?, env?, stdin?)`
  - `exec.shell(command, working_dir?, env?, stdin?, shell?)`
- Launch tools are always job-backed. They create an `exec_key`, start the
  process, and wait up to a fixed 5-second foreground window before returning.
- The 5-second foreground window is not caller-configurable and is not named as
  a timeout. Long-running jobs continue asynchronously and are inspected through
  `exec.status`, `exec.result`, `exec.abort`, and text readers.
- If a job exits during the foreground window and combined stdout plus stderr is
  at most 4096 bytes, the launch response may include the completed output
  inline with the `exec_key`, exit status, and metadata.
- If the job is still running after the foreground window, or if output exceeds
  4096 bytes, the launch response returns compact metadata, stream sizes, and
  follow-up guidance without raw output.
- `exec.result(exec_key)` uses the same fixed 4096-byte inline budget and has
  no `max_output_bytes` parameter. Larger results return metadata and guidance
  for text readers.
- Keep `exec.abort` as the process/job termination verb. It is distinct from
  MCP request cancellation and from result retrieval cancellation.
- Provide text-reader tools in this first slice:
  - `exec.tail(exec_key, stream?, lines?)`
  - `exec.read(exec_key, stream?, offset?, limit?)`
  - `exec.grep(exec_key, pattern, stream?, before?, after?, max_matches?, regex?)`
- `exec.grep` defaults to literal matching. Regex behavior requires
  `regex: true`.
- `exec.read` is byte-offset based and returns `next_offset` so callers can
  continue without rereading large files.

## Constraints

- Omitted `working_dir` resolves to the current ws worktree root through the
  existing ws root resolver. Relative `working_dir` values resolve beneath that
  root, not beneath the plugin cache cwd.
- `env` overlays the inherited environment. Replacement or deletion semantics
  are out of scope for this child unless implementation discovers they are
  needed for a coherent minimum surface.
- `stdin` is textual input for this child. Binary stdin support is out of scope.
- Persist raw stdout and stderr under a durable job-owned directory in the
  current worktree state. Normal MCP responses expose only bounded excerpts and
  metadata.
- Implement the file reading and searching logic as reusable internal helpers.
  The `exec.*` tools should primarily map `exec_key` to persisted stream file
  paths and call those helpers, so later agent logs, transcripts, or other
  text-backed surfaces can reuse them.
- Hide every introduced `exec.*` tool in wsflow no-agent mode for both
  `tools/list` and explicit `tools/call`.
- Update runtime capabilities, runtime manifests, CLI mirrors where included,
  and tests in the same implementation slice as the MCP tool surface.

## Prior Art

- `api.ask_async` provides the closest durable async job precedent: persisted
  job records, status/result/cancel surfaces, restart reconciliation, and
  process-local live-worker cancellation hints.
- `wsagent` already contains cross-platform async process setup and best-effort
  process tree cancellation helpers.
- Existing agent tail/debug stream tools show the local bounded-output pattern,
  but exec readers should be implemented as generic reusable text-file helpers
  rather than agent-specific code.

## Phases

### Phase 1: Implement exec job core and text readers

Add a durable exec job manager, MCP tools, optional CLI mirrors, runtime
metadata, wsflow no-agent hiding, and tests for:

- spawn vs shell command shapes;
- default worktree-root `working_dir` resolution and relative working
  directories;
- foreground-window completed small output;
- foreground-window running handoff;
- large-output metadata without inline raw output;
- fixed 4096-byte `exec.result` budget;
- stdout and stderr persistence;
- `exec.tail`, `exec.read`, and literal/regex `exec.grep`;
- `exec.abort` preserving partial output and terminal state;
- Unix process-group behavior and best-effort Windows behavior;
- runtime capability and manifest drift.

Update `mcp-tools`, `plugin-runtime`, and relevant mental-model docs before
promoting this child to `ready`.

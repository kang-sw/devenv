# Brief: 260524-feat-exec-job-core-text-readers

## Intent

Implement the first async exec child: durable command jobs for trusted lead
workflows, bounded inline output, lifecycle inspection, best-effort abort, and
raw fallback text readers over persisted stdout and stderr. This slice must
create the substrate that a later `exec.ask` child can query without adding
model-backed answering now.

## Scope Boundary

Selected scope is Phase 1 of `260524-feat-exec-job-core-text-readers`.

In scope:

- `exec.spawn`, `exec.shell`, `exec.status`, `exec.result`, `exec.abort`.
- Raw fallback readers `exec.raw.tail`, `exec.raw.read`, `exec.raw.grep`.
- Durable job records and stream files under the current ws worktree state.
- Runtime contract updates for full ws, and wsflow no-agent hiding for tools
  list, explicit calls, and runtime capability output.
- Focused tests covering launch shapes, working directory resolution, inline
  budget behavior, raw readers, abort, runtime manifests, and no-agent hiding.

Out of scope:

- `exec.ask`, model-backed output questions, context modes, or reader agents.
- Interactive PTY/session management.
- Binary stdin or binary stream formatting.
- Remote execution, authority semantics, dashboard projection, or harness
  integration beyond runtime capabilities.

## Caller-Visible Contract

`exec.spawn(cmd, args?, working_dir?, env?, stdin?)` runs structured argv.
`cmd` is an executable/argv0, not a shell command line.

`exec.shell(command, working_dir?, env?, stdin?, shell?)` runs one explicit shell
command string. Shell selection may be minimal but must stay explicit; default
shell behavior should be platform-appropriate and tested enough not to break
Unix.

Both launch tools:

- create and return an `exec_key`;
- persist stdout and stderr in job-owned files;
- wait up to a fixed 5-second foreground window before responding;
- do not expose a caller-configurable timeout parameter;
- inline output only when the process completed within that foreground window
  and combined stdout plus stderr is at most 4096 bytes;
- otherwise return compact metadata, stream sizes, status, and guidance that
  names future `exec.ask` first and `exec.raw.*` fallback readers second.

`exec.status(exec_key)` returns lifecycle status, pid if available, timestamps,
exit metadata, stream sizes, and whether a terminal result is ready.

`exec.result(exec_key)` returns terminal job metadata and at most the same fixed
4096-byte inline output budget. It has no `max_output_bytes` argument. Larger
results return metadata and guidance, not raw output.

`exec.abort(exec_key)` best-effort terminates a running job, preserves partial
output, and returns updated terminal or cancel-requested status metadata.

`exec.raw.tail(exec_key, stream?, lines?)` returns bounded tail text. `stream`
selects `stdout`, `stderr`, or `combined`, with a sensible default documented in
the implementation tests.

`exec.raw.read(exec_key, stream?, offset?, limit?)` reads bytes from a selected
stream and returns `next_offset`.

`exec.raw.grep(exec_key, pattern, stream?, before?, after?, max_matches?,
regex?)` defaults to literal matching. Regex mode is opt-in with `regex: true`.

All normal MCP responses are text content; JSON text is acceptable for
machine-readable job metadata if it matches existing mcp test patterns.

## Contract Instructions

Primary implementation files and modules:

- `agents-plugin-tool/internal/mcp/server.go` for tool schemas, dispatch, root
  resolution, profile/no-agent filtering, and MCP response formatting.
- New internal package is allowed for reusable text file readers and/or exec job
  management if it keeps `server.go` thin.
- `agents-plugin-tool/internal/mcp/api_async.go` is prior art for durable job
  keys, records, status/result/cancel shapes, and worktree-state layout.
- `agents-plugin-tool/internal/wsagent/cancel_process_*.go` and
  `async_command_*.go` are prior art for process creation/cancellation.
- `agents-plugin-tool/internal/wsstate/paths.go` owns durable worktree state
  layout; use `wsstate.Manager.Ensure(root)` rather than ad hoc cache roots.
- `agents-plugin-tool/cmd/ws-mcp/main.go` only needs CLI mirrors if the
  implementation chooses to include them in this child. If included, update
  `runtimeCapabilityCommandNames` and tests. If not included, do not add exec
  commands to `runtime.json.commands`.
- `agents-plugin/runtime.json` must include new full-ws MCP tools.
- `agents-plugin-wsflow/runtime.json` must omit the new tools because wsflow
  no-agent mode hides `exec.*`.

Public parameter names must use `working_dir`, not `root`, for command
execution location. The existing root resolver still supplies the ws worktree
root when `working_dir` is omitted. Relative `working_dir` resolves beneath that
root; do not resolve it relative to the plugin cache cwd or process cwd.

`env` overlays the inherited process environment. Do not add environment
deletion/replacement semantics in this child unless a minimal coherent
implementation requires it.

`stdin` is text only.

Implement generic, reusable text file helpers for tail/read/grep. The
`exec.raw.*` MCP tools should map `exec_key` to persisted stream paths and call
those helpers. Avoid making these helpers agent-specific.

Command output is untrusted data. Do not execute or interpret output as
instructions. This child does not add model-backed summarization.

## Integration Test Instructions

Add or extend tests under:

- `agents-plugin-tool/internal/mcp/server_test.go` for tools/list, no-agent
  hiding, explicit hidden-call errors, and root/working_dir behavior visible
  through MCP.
- New package tests for exec job management and text readers if implemented
  outside `internal/mcp`.
- `agents-plugin-tool/cmd/ws-mcp/main_test.go` for runtime capabilities. Include
  CLI command tests only if CLI mirrors are included.
- `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py` and
  `agents-plugin-wsflow/runtime.json` for exact no-agent contract drift.

Required verification:

- `cd agents-plugin-tool && go test ./...`
- `python3 -m unittest discover agents-plugin-wsflow/tests`

Tests must cover:

- spawn vs shell launch shapes;
- omitted and relative `working_dir`;
- quick small output inlined;
- running handoff after the foreground window;
- large output metadata without raw inline output;
- fixed 4096-byte `exec.result` budget;
- stdout and stderr persistence;
- `exec.raw.tail`, `exec.raw.read`, literal grep, and regex grep;
- abort preserving partial output and reaching a terminal/cancelled state;
- wsflow no-agent omits and rejects `exec.*`;
- runtime capability and manifest drift.

## Implementation Strategy Decisions

Use a foreground wait window of 5 seconds. This is a response-shaping window,
not a command timeout.

Use an inline output budget of 4096 bytes for launch responses and
`exec.result`. Do not expose a `max_output_bytes` parameter.

Persist job state and stream files before returning the launch response.

Keep `exec.abort` distinct from MCP request cancellation and result retrieval.

Name large-output guidance in this order: future `exec.ask` first, `exec.raw.*`
fallback readers second.

## Rejected Alternatives

- Do not call the command location parameter `root`.
- Do not make launch tools synchronous-only.
- Do not add caller-configurable launch timeouts or result byte limits.
- Do not expose raw readers as `exec.read`/`exec.grep`; keep them under
  `exec.raw.*`.
- Do not implement `exec.ask` in this child.

## Approach

- Add exec job records and file layout under worktree state.
- Add process launch helpers with separate argv and shell paths.
- Add reusable text file helpers for tail/read/grep.
- Add MCP schemas and dispatch.
- Add no-agent hiding and runtime manifest updates.
- Add tests before or alongside each behavior, using short local commands and
  bounded sleeps.

## Constraints

- No new loose root-level modules or scripts.
- Keep source comments sparse and only where lifecycle ordering is non-obvious.
- Cross-platform process cancellation should use best-effort Unix process-group
  behavior and Windows behavior consistent with existing wsagent helpers.
- Work with existing dirty-state rules; do not revert unrelated user changes.

## Details

Suggested state fields include `exec_key`, `status`, `root`, `working_dir`,
`argv` or shell command metadata, pid, timestamps, exit code/signal information,
error text, cancel flag, and stdout/stderr byte sizes. Exact field names may be
implementation-local if MCP output remains stable and tests cover it.

Use an `exec-...` key pattern distinct from `api-...`.

Stream names should include `stdout` and `stderr`; `combined` may be synthesized
from captured stream files or a combined persisted file. Pick one and test it.

## Verification Contract

Implementation is acceptable only after the required Go and Python test commands
pass with output read in full.

Docs closeout must remove planned `🚧` markers or convert the new spec text to
implemented text for `260524-exec-job-mcp-tools` and
`260524-exec-runtime-contract-surface`, then update the ticket with a Result
entry referencing the implementation commit.

## References

- [Must] `ai-docs/tickets/ready/260524-feat-exec-job-core-text-readers.md` -
  selected ticket and phase.
- [Must] `ai-docs/spec/mcp-tools.md` - planned caller-visible MCP contract.
- [Must] `ai-docs/spec/plugin-runtime.md` - planned runtime contract behavior.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP registry, filtering, and
  runtime capability coupling.
- [Must] `ai-docs/mental-model/plugin-runtime.md` - runtime manifest and
  wsflow contract coupling.
- [Must] `agents-plugin-tool/internal/mcp/server.go` - MCP schema and dispatch.
- [Must] `agents-plugin-tool/internal/mcp/api_async.go` - durable async job
  precedent.
- [Must] `agents-plugin-tool/internal/wsstate/paths.go` - worktree state layout.
- [Maybe] `agents-plugin-tool/internal/wsagent/async_command_unix.go` and
  `agents-plugin-tool/internal/wsagent/async_command_windows.go` - process
  setup precedent.
- [Maybe] `agents-plugin-tool/internal/wsagent/cancel_process_unix.go` and
  `agents-plugin-tool/internal/wsagent/cancel_process_windows.go` - best-effort
  termination precedent.
- [Maybe] `agents-plugin-tool/cmd/ws-mcp/main.go` - runtime capabilities and
  optional CLI mirror patterns.

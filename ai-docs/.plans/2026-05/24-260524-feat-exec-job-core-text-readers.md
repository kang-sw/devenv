# Implementation Plan: 260524-feat-exec-job-core-text-readers Phase 1

## Status

[ok] Research is not required before implementation. The brief and referenced code define the public contract, storage precedent, tool registry points, process setup precedent, and runtime contract coupling clearly enough for Phase 1.

## Phase 1 Scope

Implement only the durable exec job core and raw text readers described by `ai-docs/.plans/2026-05/24-260524-feat-exec-job-core-text-readers.brief.md` and ticket Phase 1 in `ai-docs/tickets/ready/260524-feat-exec-job-core-text-readers.md#L78-L94`.

Do not implement `exec.ask`, model-backed output questions, interactive PTY/session management, binary stream formatting, dashboard projection, or harness integration beyond runtime capability metadata.

## Grounding References

- Public MCP contract: `ai-docs/spec/mcp-tools.md#L322-L350`.
- Runtime/wsflow contract: `ai-docs/spec/plugin-runtime.md#L53-L59`.
- MCP registry and filtering coupling: `ai-docs/mental-model/mcp-runtime.md#L24-L55` and `ai-docs/mental-model/mcp-runtime.md#L62-L70`.
- Plugin runtime manifest coupling: `ai-docs/mental-model/plugin-runtime.md#L35-L45`.
- Existing MCP dispatch, response, and schema locations: `agents-plugin-tool/internal/mcp/server.go#L321-L423` and `agents-plugin-tool/internal/mcp/server.go#L1608-L1739`.
- Durable async job precedent: `agents-plugin-tool/internal/mcp/api_async.go#L515-L617`.
- Worktree state layout: `agents-plugin-tool/internal/wsstate/paths.go#L32-L55` and `agents-plugin-tool/internal/wsstate/paths.go#L120-L154`.
- Process-group setup/cancel precedent: `agents-plugin-tool/internal/wsagent/async_command_unix.go#L10-L12`, `agents-plugin-tool/internal/wsagent/async_command_windows.go#L10-L13`, `agents-plugin-tool/internal/wsagent/cancel_process_unix.go#L19-L55`, `agents-plugin-tool/internal/wsagent/cancel_process_windows.go#L7-L15`.
- Runtime capability path: `agents-plugin-tool/cmd/ws-mcp/main.go#L190-L215` and command list at `agents-plugin-tool/cmd/ws-mcp/main.go#L218-L240`.
- No-agent hiding tests and capability tests: `agents-plugin-tool/internal/mcp/server_test.go#L518-L554`, `agents-plugin-tool/cmd/ws-mcp/main_test.go#L38-L130`, `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L12-L36` and `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L85-L109`.

## Implementation Steps

### 1. Add reusable text-file reader helpers

Create a small internal package, e.g. `agents-plugin-tool/internal/textreader`, with helpers for:

- `Tail(path, lines)` with bounded default/max behavior and text output.
- `Read(path, offset, limit)` with byte offsets and `next_offset`.
- `Grep(path(s), pattern, before, after, max_matches, regex)` with literal default and opt-in regex.

Use `agents-plugin-tool/internal/wsagent/agent.go#L2304-L2324` only as prior art for line-tail scanning; do not couple the new helpers to agent layouts or sanitizer behavior.

Minimum package tests should cover default bounds, offset continuation, literal search, regex search, context lines, max matches, missing/empty files, and invalid regex errors.

### 2. Add exec job manager package

Create a package such as `agents-plugin-tool/internal/execjob` to keep `server.go` thin. It should own:

- `exec-...` key generation distinct from `api-...`.
- Job-owned state under `wsstate.Manager.Ensure(root)` in a directory like `<layout.WorktreeDir>/exec-jobs/<exec_key>/`.
- `state.json`, `stdout`, `stderr`, and either a persisted `combined` stream or synthesized combined reads with tested ordering/metadata.
- Record fields for `exec_key`, status, root, working_dir, argv or shell metadata, pid, started/updated/completed timestamps, exit metadata, error text, cancel flag, and stdout/stderr sizes.
- Atomic state writes following the `api_async.go` pattern at `agents-plugin-tool/internal/mcp/api_async.go#L515-L617`.

Statuses should distinguish running, succeeded, failed, cancel requested/cancelled, and terminal readiness. Startup must persist the record and open stream files before returning from launch.

### 3. Implement process launch and lifecycle behavior

In the exec manager:

- Implement structured `spawn` using `exec.CommandContext`/`exec.Command` with `cmd` plus `args` and no shell parsing.
- Implement `shell` using explicit shell selection, with platform-appropriate default if `shell` is omitted; keep the shell path/argv encoded in job metadata.
- Resolve `working_dir` from the MCP-resolved worktree root: omitted means root, relative means beneath root, absolute should be cleaned and used only if it resolves coherently inside the intended worktree policy chosen by the implementer. The contract specifically forbids resolving relative paths against process/plugin cwd.
- Overlay `env` on `os.Environ()` without deletion/replacement semantics.
- Treat `stdin` as text only.
- Apply process group/new group setup matching `wsagent` prior art; either export/reuse helpers from `wsagent` if appropriate or duplicate minimal platform-specific helpers in the new package with tests.
- Wait up to the fixed 5-second foreground window for launch responses; this is not a command timeout.
- Continue long-running jobs in the background and update state on process exit.
- Implement best-effort abort that marks cancel requested, terminates the running process/process group if process-local handle or pid is available, preserves partial streams, and returns reconciled status.

### 4. Shape exec responses and inline budgets

Add response structs/formatters in the exec manager or MCP layer for:

- Launch response with `exec_key`, status, pid, timestamps, stream sizes, terminal readiness, and optional inline stdout/stderr only when completed within the foreground window and combined stream bytes are <= 4096.
- `exec.status` with lifecycle metadata, pid when available, exit metadata, stream sizes, timestamps, and `result_ready`.
- `exec.result` that only returns terminal job metadata and uses the same fixed 4096-byte inline budget with no `max_output_bytes` argument.
- Oversized/running guidance that names future `exec.ask` first and `exec.raw.*` fallback readers second.

Normal MCP responses may be JSON text through `toolJSONResponse` as used by API async tools at `agents-plugin-tool/internal/mcp/server.go#L387-L423`.

### 5. Wire MCP schemas and dispatch

Update `agents-plugin-tool/internal/mcp/server.go`:

- Add dispatch cases near the `api.*` cases for `exec.spawn`, `exec.shell`, `exec.status`, `exec.result`, `exec.abort`, `exec.raw.tail`, `exec.raw.read`, and `exec.raw.grep`.
- Use `resolveToolRoot` at `agents-plugin-tool/internal/mcp/server.go#L1456-L1494` for the worktree root, but public schemas for exec launch tools must expose `working_dir`, not `root`.
- Add tool schemas in `tools()` near the existing `api.*` tool schemas at `agents-plugin-tool/internal/mcp/server.go#L1634-L1739`.
- Add `exec.*` to `noAgentHiddenTool` at `agents-plugin-tool/internal/mcp/server.go#L2387-L2397` so wsflow hides tools/list and rejects explicit calls before dispatch.
- Review role/profile filtering in `agents-plugin-tool/internal/mcp/server.go#L2282-L2325`; no special leaf/delegate allowance should accidentally override no-agent hiding.

### 6. Update runtime manifests and capability tests

- Add the eight new MCP tool names to `agents-plugin/runtime.json#L33-L82` with the current version range.
- Do not add exec commands to `agents-plugin/runtime.json#L83-L122` unless CLI mirrors are intentionally included in this child.
- Keep `agents-plugin-wsflow/runtime.json#L36-L63` without `exec.*` because wsflow no-agent must hide the surface exactly.
- Extend hidden-tool sets in `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L12-L36` to include every `exec.*` tool.
- Extend `agents-plugin-tool/cmd/ws-mcp/main_test.go#L38-L130` to require full ws capabilities include `exec.*` tools and no-agent capabilities omit them.

### 7. Add MCP integration tests

Extend `agents-plugin-tool/internal/mcp/server_test.go` for:

- `tools/list` includes all `exec.*` tools in full ws mode.
- No-agent tools/list omits all `exec.*`, and explicit `exec.spawn`/one raw reader call returns the disabled-surface error. Existing no-agent test pattern is at `agents-plugin-tool/internal/mcp/server_test.go#L518-L554`.
- `exec.spawn` vs `exec.shell` launch shapes.
- Omitted `working_dir` runs from the resolved git worktree root.
- Relative `working_dir` resolves under that root and not plugin/process cwd.
- Small quick output is inlined.
- A command running beyond the foreground window returns metadata/guidance while remaining inspectable.
- Large completed output returns metadata/guidance without raw inline output.
- `exec.result` enforces the fixed 4096-byte budget.
- stdout and stderr are both persisted and reported with sizes.
- `exec.raw.tail`, `exec.raw.read`, literal grep, and regex grep return bounded expected text/metadata.
- `exec.abort` preserves partial output and reaches terminal/cancelled status.

Prefer portable test commands built through the same Go test binary where shell quoting is fragile; on Unix, small `sh -c` snippets are acceptable for shell-only behavior, with Windows-specific skips or helper binaries where needed.

### 8. Verification and docs closeout for implementer

After source changes, run and read full output from:

```sh
cd agents-plugin-tool && go test ./...
python3 -m unittest discover agents-plugin-wsflow/tests
```

Docs closeout is lead/implementation wrap-up owned after code verification: remove or convert planned `🚧` markers for `260524-exec-job-mcp-tools` and `260524-exec-runtime-contract-surface`, then add the ticket Result entry referencing the implementation commit.

## Risk Signals Requiring Attention

- `agents-plugin-tool/internal/mcp/server.go#L1456-L1494` — The existing root resolver only knows `root`; exec public schemas must not expose `root`, so launch dispatch needs a separate `working_dir` resolver layered after worktree-root resolution.
- `agents-plugin-tool/internal/wsagent/cancel_process_unix.go#L19-L55` — Cancellation helper is unexported and agent-scoped; reusing it may require a small exported/common process helper or duplication, otherwise abort could degrade to killing only the parent process.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L218-L240` and `agents-plugin/runtime.json#L83-L122` — CLI command mirrors are optional in the brief; adding only MCP tools avoids command-list drift, but if any exec CLI is added the command list and runtime manifests must be updated together.
- `agents-plugin-wsflow/runtime.json#L8-L10` and `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L85-L109` — wsflow uses exact runtime capabilities, so any full-ws tool addition must also be hidden from no-agent capability output or wsflow tests will fail.

## Completion Checklist

- [ ] Text reader helpers and tests added.
- [ ] Exec job manager persists state and streams under `wsstate` worktree layout.
- [ ] Launch, status, result, abort, and raw reader APIs covered by package tests.
- [ ] MCP schemas and dispatch added for all eight `exec.*` tools.
- [ ] Full ws runtime manifest includes all eight MCP tools.
- [ ] wsflow runtime contract omits and rejects all eight MCP tools.
- [ ] Required Go and Python verification commands pass.
- [ ] Docs/ticket closeout handled by the lead-owned implementation wrap-up.

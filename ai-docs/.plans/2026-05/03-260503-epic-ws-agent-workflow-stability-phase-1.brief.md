# Brief: 260503-epic-ws-agent-workflow-stability Phase 1

## Intent

Harden the first named-agent runtime failure slice found while dogfooding
`write-code`: async wait timeouts must remain recoverable, status must expose
enough lifecycle state for a lead to act without raw tail spelunking, cancel must
target the runtime-owned process tree rather than only a single PID, and runtime
diagnostics must leave a durable event trail under the existing ws project state.

## Approach

- Keep the public `ws/agents.*` tool names stable.
- Add structured lifecycle details to agent status output while preserving the
  existing text format for current skills.
- Make wait timeout output explicit that the backend may still be running and
  that callers should follow up with status/wait/print.
- Introduce runtime-owned process-tree cancellation helpers for Unix process
  groups and a conservative Windows fallback.
- Append diagnostics for wait timeouts, cancellation attempts/results, and
  cleanup-needed states to the existing agent event/runtime logs.
- Cover the behavior with focused Go tests using fake runners, fake starters,
  and fake liveness where possible.

## Constraints

- Do not update repository specs or mental models on this branch.
- Do not mutate `claude-plugin/`.
- Do not kill unrelated processes; process cleanup must be limited to the
  runtime-owned PID or process group recorded in current call state.
- Keep changes under `agents-plugin-tool/` plus ticket/plan closeout.
- Preserve the durable named-agent model: registering a task-scoped name resets
  or creates that name, and repeated async calls continue the conversation.

## Out of scope

- Lead-context compression beyond enough status/wait text to avoid ambiguous
  failures; broader summary ergonomics are Phase 2.
- `ws/git.commit`, ticket graph tools, or project-specific test runners.
- Native Windows smoke beyond compile/test coverage.

## Details

The current async worker starter puts `ws-mcp agents run-current` in a separate
process group on Unix, but `Manager.Cancel` calls `os.FindProcess(pid).Kill()`,
which only kills the worker PID and can leave its Codex child alive. Add a small
platform abstraction for cancelling an owned async process tree and use it from
`Cancel`. Record whether cancellation succeeded, failed, or may need manual
cleanup.

`Wait` currently returns `"timeout\n" + status`, which is not wrong but is too
easy to interpret as a terminal failure. Make the timeout status actionable and
diagnostic: include current call state and a clear follow-up hint. Keep it text
based for compatibility, but add fields such as `active: true`,
`follow_up: agents.wait|agents.status|agents.cancel`, and relative paths for
output/runtime logs where available.

## References

- `[Must]` `agents-plugin-tool/internal/wsagent/agent.go` - agent lifecycle,
  async call, wait, status, tail, cancel, and diagnostics.
- `[Must]` `agents-plugin-tool/internal/wsagent/async_command_unix.go` - async
  worker process group setup.
- `[Must]` `agents-plugin-tool/internal/wsagent/runner_command_unix.go` - Codex
  runner process group cancellation prior art.
- `[Must]` `agents-plugin-tool/internal/wsagent/agent_test.go` - lifecycle tests
  and fake worker/runner patterns.
- `[Maybe]` `agents-plugin-tool/internal/mcp/server.go` - MCP text surface for
  agent tools.
- `[Maybe]` `agents-plugin-tool/cmd/ws-mcp/main.go` - CLI fallback surface.

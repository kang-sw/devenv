# Brief: 260505-feat-ws-mcp-result-readiness-api

## Intent

Replace the current result-returning `agents.wait` / immediate `agents.print`
split with clearer primitives:

- `agents.result` consumes one agent's final output and optionally waits for it.
- `agents.wait` waits for readiness across one or more agents and returns
  status metadata, not final output.
- `agents.print` is removed or left only as a compatibility alias, depending on
  what keeps the implementation coherent before release.

This should make subquery fan-out cheaper to orchestrate and give temporary
helper agents one result-consumption point for cleanup.

## Approach

- Add runtime support for explicit result consumption in `wsagent`.
- Change wait semantics so waiting and result consumption are separate code
  paths.
- Add multi-name wait support for MCP and CLI callers.
- Mark subquery-created agents as ephemeral through agent metadata, not name
  parsing.
- Rename generated subquery names to a temporary-looking hint such as
  `subquery-tmp<id>`, while keeping cleanup policy metadata-driven.
- Consume and erase successful ephemeral agents only through `agents.result`.
- Update MCP schemas, CLI fallbacks, runtime references, skills, prompt
  guidance, tests, and runtime contract metadata together.

## Required Behavior

`agents.result`:

- Public MCP tool name: `agents.result`.
- CLI fallback: `ws-mcp agents result --root <repo> --name <name> [--timeout 10m]`.
- Accepts one `name`.
- Accepts optional `timeout_seconds` in MCP and `--timeout` in CLI.
- If timeout is omitted or zero, do not wait; return already-available output
  only when the current call is completed.
- If timeout is positive, wait up to that bound for completion and then return
  output when completed.
- If the call is running and no wait was requested, return actionable status
  text rather than blocking.
- If the call fails, is cancelled, or times out, return status/timeout text and
  keep the agent available for diagnostics.
- If the agent metadata marks it ephemeral, erase the agent only after
  successfully reading completed output.

`agents.wait`:

- Keep MCP tool name `agents.wait`.
- Accept `name` for compatibility and `names` for the new multi-agent path.
- CLI fallback should accept repeated `--name` and/or positional names.
- Wait for one or more agents to reach a terminal state, timeout, or context
  cancellation.
- Return concise readiness metadata, not final output.
- Include enough fields for callers to decide whether to call `agents.result`,
  `agents.status`, `agents.tail`, or `agents.cancel`.
- Do not erase ephemeral agents.

Subquery:

- Return `subquery_key` matching the generated temporary name.
- Follow-up should point to `agents.result(name: "<key>", timeout_seconds: 600)`
  plus status/tail/cancel as diagnostics.
- Do not emphasize automatic deletion in user-facing follow-up text.
- Delegate-profile scoped access must allow `agents.result` for generated
  subquery agents, just like status/tail/cancel access.

## Constraints

- Do not modify `claude-plugin/`; it is compatibility reference only.
- Do not stage or commit the in-progress forge-spec archive move under
  `ai-docs/spec/` and `ai-docs/ref/old-spec/260505/`.
- Preserve failed/cancelled/timed-out agent state for diagnostics.
- Keep result cleanup centralized. `wait`, `status`, `tail`, and `cancel` must
  not consume or erase output.
- Avoid adding a TTL/GC cleanup mechanism in this ticket; leave abandoned
  ephemeral-agent GC as a possible follow-up.
- Specs are currently archived for forge-spec reconstruction. Update runtime
  references and ticket docs; do not recreate behavioral specs here.
- Update embedded prompt bundle hash in `agents-plugin/runtime.json` if any
  embedded prompt text changes.

## Likely Source Files

- `agents-plugin-tool/internal/wsagent/agent.go`
- `agents-plugin-tool/internal/wsagent/agent_test.go`
- `agents-plugin-tool/internal/mcp/server.go`
- `agents-plugin-tool/internal/mcp/server_test.go`
- `agents-plugin-tool/cmd/ws-mcp/main.go`
- `agents-plugin/runtime.json`
- `agents-plugin/skills/lead-workflow/SKILL.md`
- `agents-plugin/skills/lead-discuss/SKILL.md`
- `agents-plugin/skills/lead-sprint/SKILL.md`
- `agents-plugin/skills/lead-forge-spec/SKILL.md`
- `agents-plugin/skills/lead-forge-mental-model/SKILL.md`
- `agents-plugin/skills/lead-write-spec/SKILL.md`
- `agents-plugin/skills/lead-write-ticket/SKILL.md`
- `agents-plugin/skills/lead-edit/SKILL.md`
- `agents-plugin/skills/lead-write-code/SKILL.md`
- `agents-plugin-tool/internal/wsprompt/prompts/implementer.md`
- `ai-docs/ref/ws-mcp.md`
- `ai-docs/ref/ws-agent-runtime.md`

## Tests

Run from `agents-plugin-tool/`:

```sh
go test ./internal/wsagent ./internal/mcp ./cmd/ws-mcp
go test ./...
```

Add focused tests for:

- `agents.result` immediate completed output.
- `agents.result` timeout-positive wait then output.
- `agents.result` non-ready status without waiting.
- ephemeral agent erase only after successful result consumption.
- failed/cancelled/timed-out ephemeral agents remain inspectable.
- `agents.wait` multi-name readiness output does not include final output.
- `agents.wait` still accepts legacy single `name`.
- delegate profile can retrieve `agents.result` only for generated
  `subquery-*`/temporary subquery agents, not arbitrary names.
- subquery follow-up points to `agents.result`.
- CLI `agents result` and multi-name `agents wait` behavior.

## References

- `ai-docs/tickets/todo/260505-feat-ws-mcp-result-readiness-api.md` - source
  ticket and settled direction.
- `ai-docs/tickets/todo/260503-epic-ws-agent-workflow-stability.md` - parent
  lifecycle and containment roadmap.
- `ai-docs/ref/ws-agent-runtime.md` - current durable agent runtime contract.
- `ai-docs/ref/ws-mcp.md` - public MCP schema and CLI runtime reference.
- `ai-docs/ref/skill-authoring.md` - required when editing skills or prompts.
- `ai-docs/mental-model/executor-wrapup.md` - delegated implementation wrapup
  and review-file protocol.

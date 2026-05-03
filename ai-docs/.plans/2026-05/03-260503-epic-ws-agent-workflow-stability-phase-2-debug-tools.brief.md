# Brief: 260503-epic-ws-agent-workflow-stability Phase 2 debug tools

## Intent

Add explicit debug-namespaced agent diagnostic tools so normal lead orchestration
continues through `agents.status`, `agents.wait`, and `agents.print`, while raw
tails and streams are opt-in through `agents.debug.*`. Preserve `agents.tail` as
a compatibility alias during the transition.

## Approach

- Add MCP tools for `agents.debug.tail`, `agents.debug.stdout`,
  `agents.debug.stderr`, `agents.debug.runtime_log`, and `agents.debug.events`.
- Keep `agents.tail` behavior unchanged as a deprecated compatibility alias.
- Add matching CLI fallbacks under `ws-mcp agents debug <tail|stdout|stderr|runtime-log|events>`.
- Reuse existing agent state layout and tailing helpers; avoid duplicating raw
  file reading logic.
- Update runtime metadata so hosts can detect the new debug tools and commands.
- Add tests for MCP tools/list, MCP calls, CLI debug subcommands, and wsagent
  diagnostic stream selection.

## Constraints

- Do not remove or rename existing `agents.tail` in this slice.
- Do not change normal `agents.status`, `agents.wait`, or `agents.print`
  semantics except where tests need to account for existing Phase 1 fields.
- Do not update repository specs or mental models on this branch.
- Keep raw diagnostic output bounded by the existing `lines` parameter.

## Out of scope

- JSON structured status output.
- Changing reviewer prompt behavior beyond enabling the new debug namespace.
- Removing deprecated aliases.
- Fixing synchronous `agents.oneshot` host timeout behavior; the dogfood survey
  still timed out and should be recorded as a remaining runtime issue.

## Details

The debug tools should make tool names carry the usage policy:

- `agents.debug.tail`: same sectioned output as existing `agents.tail`.
- `agents.debug.stdout`: recent lines from current call stdout.
- `agents.debug.stderr`: recent lines from current call stderr.
- `agents.debug.runtime_log`: recent lines from current call runtime log.
- `agents.debug.events`: recent lines from agent events log.

Use short descriptions that clearly mark these as debugging/raw diagnostics, not
normal workflow status tools.

## References

- `[Must]` `agents-plugin-tool/internal/wsagent/agent.go` - agent layout, tail
  helper, status/wait/print semantics, and diagnostic stream files.
- `[Must]` `agents-plugin-tool/internal/mcp/server.go` - MCP tool routing and
  tool metadata.
- `[Must]` `agents-plugin-tool/cmd/ws-mcp/main.go` - CLI fallback routing.
- `[Must]` `agents-plugin/runtime.json` - advertised runtime tool and command
  surface.
- `[Must]` `ai-docs/tickets/todo/260503-epic-ws-agent-workflow-stability.md` -
  Phase 2 raw-output containment decision.

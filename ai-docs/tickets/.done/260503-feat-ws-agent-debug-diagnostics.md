---
title: ws agent debug diagnostics
parent: 260503-epic-ws-agent-workflow-stability
related:
  260503-epic-ws-agent-workflow-stability: parent stabilization epic
completed: 2026-05-03
---

# ws agent debug diagnostics

## Background

Delegated workflows needed better raw-output containment. Normal operation
should show compact state and next actions, while raw `stdout`, `stderr`,
runtime logs, events, and full review findings should stay in files unless the
lead explicitly asks for debugging detail.

This child ticket preserves the completed Phase 2 scope split out of
`260503-epic-ws-agent-workflow-stability`.

## Result (481dd78) - 2026-05-03

Implemented the first raw-output containment slice by adding debug-namespaced
agent diagnostic surfaces. MCP now advertises `agents.debug.tail`,
`agents.debug.stdout`, `agents.debug.stderr`, `agents.debug.runtime_log`, and
`agents.debug.events`; CLI fallbacks are available under `ws-mcp agents debug
<tail|stdout|stderr|runtime-log|events>`.

The existing `agents.tail` MCP and CLI surfaces remain as compatibility aliases.
The implementation added `wsagent.DiagnosticStream` so MCP and CLI routes share
the same agent layout and bounded tailing helper instead of reading diagnostic
files directly.

Runtime metadata now advertises the debug tools and commands. Tests cover
diagnostic stream selection, bounded output, MCP tools/list and calls, and CLI
debug subcommands.

Dogfooding exposed a follow-up runtime issue: an implementer used `ws:edit`
internally, launched a reviewer, and then blocked on `agents.wait`. Because the
then-current `ws-mcp` stdio server handled requests sequentially, that long wait
blocked later `agents.status` calls on the same MCP server. The source commit
and verification completed, but the lead had to inspect files and processes
directly, then manually terminate the stuck implementer/reviewer process tree.
This finding led to the nonblocking MCP orchestration child ticket.

Verification covered `go test ./...` from `agents-plugin-tool`, runtime JSON
parsing, `claude plugin validate agents-plugin`, and `git diff --check`. No
spec or mental-model updates were made on this branch.

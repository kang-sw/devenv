---
name: mcp-server-repair
description: Recover when the wsflow/* MCP tools are absent from the tool list, or a wsflow/* tool call fails to connect. Keep working through wsflow-cli and relay the reconnect steps to the user.
---

# MCP Server Repair

## Invariants

- This skill makes no MCP call; it must run with the MCP server down, so every step here uses `wsflow-cli`, never a tool call.
- Keep executing the interrupted workflow through `wsflow-cli`; do not abandon the task because the tools vanished from the list.
- The agent cannot re-enable the MCP server itself; relay the reconnect steps to the user and keep working meanwhile.
- Map every `wsflow/x.y(a: b)` call to `wsflow-cli call x.y '{"a": "b"}'`: same tool name, arguments collapsed into one JSON object.
- A stale runtime can make the first `wsflow-cli` call slow because repair runs on cold start; treat that delay as expected, not a failure.

## On: tools missing or a call failing to connect

1. List the surface: `wsflow-cli tools` prints the mapping rule and every tool name with its description.
2. Inspect one tool: `wsflow-cli tools <name>` prints that tool's input schema.
3. Invoke a tool: `wsflow-cli call <name> '<json>'` — for example `wsflow-cli call workflow_state '{"session_key": "<your key>"}'`.
4. Cold start with no session: `wsflow-cli call workflow_manual '{"session_key": "obsidian-latch", "root": "<abs worktree>"}'` bootstraps the primitives, exactly as the tool call would.
5. Resume the interrupted workflow through these calls, translating each intended tool call with the mapping rule above.

## On: wsflow-cli not resolving on PATH

- Call the launcher directly, PATH-independent: `python3 <plugin-root>/bin/ws-mcp-launcher.py tools` (and `... tools <name>`, `... call <name> '<json>'`). `<plugin-root>` is the installed plugin directory that contains `bin/`.

## On: relaying reconnect steps to the user

- Hand the user the Reconnect Steps template verbatim; keep making progress through `wsflow-cli` until they confirm the server is back.

## Templates

### Reconnect Steps (relay verbatim to the user)

> The workflow's MCP server dropped and I cannot restart it from here — it is a plugin-provided stdio server, which the host does not auto-reconnect. To bring it back:
> 1. Run `/mcp` to view server status; this workflow's server will show as failed or disconnected.
> 2. Toggle that server off and back on in the `/mcp` panel, or run `/reload-plugins` to restart the plugin's servers.
> 3. If it still does not reconnect, restart the session so the launcher process can start again (this keeps needing `python3` on PATH).
>
> The `/mcp` retry/toggle is stable across recent versions, but exact button text or placement can differ; if there is no visible retry, toggling the server off then on in `/mcp` is the reliable path. Meanwhile I will keep making progress through the CLI fallback.

## Doctrine

The finite resource is a live tool channel. When the MCP server is gone, the one thing that must not also vanish is forward progress: the CLI fallback preserves every tool as a subprocess call, so the workflow continues while the user restores the channel. When ambiguous, keep working through `wsflow-cli` rather than stalling on the missing tools.

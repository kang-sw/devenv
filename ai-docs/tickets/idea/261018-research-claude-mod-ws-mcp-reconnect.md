---
title: Bring a dropped ws MCP server back from the Claude mod
related:
  261017-feat-claude-mod-lead-compaction-timing: establishes the single Claude mod directory (claude-mod/) a reconnect feature would join, and the request-then-act-at-turn-end lever shape
  261017-feat-claude-mailbox-active-push-mod: first Claude mod; mod-registered tools are served by the mod, not by the ws MCP server
---

# Bring a dropped ws MCP server back from the Claude mod

## Background

On Windows the ws stdio MCP server (`ws-mcp`) sometimes drops for unknown
reasons, and Claude Code does not bring it back: the lead falls back to
`ws:mcp-server-repair`, which relays manual `/mcp` steps to the user and keeps
working through `ws-cli`. The question is whether the ws Claude mod can
restore the server itself, either on the model's request (a lever) or
automatically (a watchdog).

A 2026-10-18 probe answered the mechanism on macOS with Claude Code 2.1.302: a
throwaway plugin (a PID-reporting stdio server plus a probe mod), then a copy
of `agents-plugin` with a probe mod as its single hooks module, run headless
(`claude -p --plugin-dir`) and interactively (tmux). Its logs were left at
`/tmp/mcp-reconnect-probe.5qJMuk/` (`runs/`, `wsrun/`; volatile).

## Probe results

Interactive session, after `kill -9` of the server:

- The server's tools leave `$.tool.list` within about 1.5 s, and no new
  process starts within about 60 s. A model-side call fails with "No such tool
  available"; a `tool.call` hook on that tool does not fire, so a hook cannot
  catch the failure and retry in place.
- `$.mcp.connect(<server>)` returns `{ isConnected: false, reason: "failed",
  message: "... did not connect: no reason given." }` and starts nothing
  (also for the real ws copy, `plugin:ws:ws`).
- `$.command.run({ command: "mcp", args: "reconnect <server>" })` is refused
  from a hook the turn waits on (`tool.call`, `prompt.submit`: "it would wait
  on the turn this hook is holding; run it from a later event
  (turn.complete)"), and works from `turn.complete` or a `$.clock.every`
  timer: it runs once the session is idle, the transcript shows "Successfully
  reconnected to <server>", a new server PID starts, the tools return, and the
  model's next call succeeds. Against the ws copy: 64 `mcp__plugin_ws_ws__*`
  tools dropped to 0 and came back after the reconnect, with a new ws-mcp PID.

Headless (`-p`) session:

- A killed server is restarted lazily on the next call (new PID, tools never
  leave the list).
- If that restart fails, after two attempts the server stays dead for the
  session; calls return `isError` "MCP server ... is not connected", and
  `$.mcp.connect` misreports `{ isConnected: true }` while starting nothing.
- The failure is cached in `~/.claude/mcp-needs-auth-cache.json` for 15
  minutes; a new session in that window skips the server
  (`$.mcp.connect`: `failed`, "recent failure cached; retries automatically in
  15 min").
- `/mcp reconnect` is unavailable ("Reconnect, enable, and disable aren't
  available in this session.").

The 2.1.302 declarations have no MCP status, lifecycle, or failure event; the
levers are `$.command.run` (queued until idle), `$.tool.list`, `$.clock.every`
/ `$.clock.after`, and `turn.complete`.

## Lever shape

Mod-registered tools are served by the mod itself (they appear as
`mcp__ws__*`, separate from `mcp__plugin_ws_ws__*`), so a reconnect tool should
survive the server's death. A lever would mirror the compaction lever: the
tool records a request and tells the model to end its turn; at
`turn.complete` the mod queues `/mcp reconnect` through `$.command.run`; once
the server is back it submits a one-line continuation. `ws:mcp-server-repair`
would gain a Claude-specific branch that calls the tool before relaying the
manual steps; its invariant "The agent cannot re-enable the MCP server itself"
would no longer hold on Claude.

## Outcome Ledger

### Verified Findings

- In an interactive Claude Code 2.1.302 session on macOS, a dead plugin stdio
  server is not restarted: its tools leave `$.tool.list` and no process starts
  within about 60 s (probe run 8).
- `$.mcp.connect` does not restart a dead server: interactive returns
  `reason: "failed"`; headless after a failed restart returns
  `isConnected: true` while calls keep failing (probe runs 4, 8, 9, wsrun).
- `$.command.run({ command: "mcp", args: "reconnect <server>" })` from
  `turn.complete` or a `$.clock.every` timer restores the server (new PID,
  tools back, model call succeeds), including the real ws plugin copy (probe
  runs 7, 9, 10, wsrun).
- `$.command.run` from a `tool.call` or `prompt.submit` hook is refused by the
  host; a `tool.call` hook does not fire for a call to a tool that has left
  the list (probe runs 5, 9).
- Headless sessions restart a killed server lazily; a failed restart sticks
  for the session and is cached for 15 minutes across sessions; `/mcp
  reconnect` is unavailable headless (probe runs 1-4, 4a, 6).

### Confirmed Decisions

- Capture the probe as research (user, 2026-10-18).

### Proposals

- Sequence: the lever first, after the single Claude mod structure of
  261017-feat-claude-mod-lead-compaction-timing has landed, then the
  watchdog only if needed.
- A model-invoked reconnect lever (tool + `turn.complete` reconnect +
  continuation) in the single Claude mod, shipped in both `ws` and `wsflow`,
  with a Claude branch in `ws:mcp-server-repair`.
- A watchdog (`$.clock.every`, counting the server's tools in
  `$.tool.list`) that queues the same reconnect without waiting for the model;
  deferred behind the lever as more moving parts.
- Resolve the server name from `$.mcp.connect("ws")`'s `server` at start
  rather than hard-coding `plugin:ws:ws`.

### Open Questions

- Does a mod-registered tool stay callable after the ws MCP server dies?
  (Inferred from the tool naming; not probed.)
- Does the `$.command.run` result reliably tell reconnect success from
  failure, so the continuation can say which?
- Windows: does the dropped server reach the same interactive state, does the
  reconnect work there, and what kills ws-mcp in the first place?
- Does an interactive failure also write the 15-minute failure cache?
- What should the lever do headless, where `/mcp reconnect` is unavailable
  (report and fall back to `ws-cli`)?

### Rejected Alternatives

- A `tool.call` hook that catches the transport error and retries in place:
  the hook does not fire once the tool has left the list, and the host refuses
  `/mcp reconnect` from a hook the turn waits on.
- `$.mcp.connect` as the reconnect: it never restarts a dead server.

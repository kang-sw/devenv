---
title: "claude-mod MCP watchdog left a killed server down on native Windows"
dropped: 2026-10-30
---

# claude-mod MCP watchdog left a killed server down on native Windows

## Observation

Dogfood on native Windows, Claude Code 2.1.314, wsflow 0.48.7, 2026-10-30
(transcript `C:\Users\user\.claude\projects\D--Temp-nightwatch\f520aed5-ad93-4180-ae9e-d10ce6a660eb.jsonl`):

- 09:46:01Z `claude.mcp_watchdog_enabled` tuned to `true` at global scope;
  interval left at the default 10 minutes.
- 09:46:15Z `/reload-plugins` (no new session) to pick the knob up.
- 09:46:59Z the wsflow server tree (cmd -> python launcher -> ws-mcp) was
  killed with `taskkill /F`; the harness marked the server failed and its 65
  tools left the list. The turn ended normally at 09:47:06Z.
- By 10:30Z (43 minutes, four intervals) the server had not come back, and
  the transcript holds neither the watchdog success line nor its failure
  notice. No debug log was captured.

The shipped live probe passed on Linux (Claude Code 2.1.314): a fresh
session armed at start, a server killed with no prior turn, a 1-minute
interval.

## Candidate causes (unverified)

- The watchdog never armed: the knob was picked up through
  `/reload-plugins`, not a fresh session start, and the reload path was not
  part of the live probe.
- Ticks ran but skipped: `turnRunning()` stuck true after the reload, or
  `$.tool.list()` still listing the dead server's tools.
- An attempt ran and both the command and its transcript notice were refused
  by the host's plugin-input hold. Claude Code 2.1.314 refuses plugin input
  with "the session was stopped and nobody has sent input since" (flag
  `tengu_plugin_input_refusal`, default on) after a Stop; the same session
  hit that refusal on a lever reconnect at 09:45:17Z, after a rejected tool
  call at 09:45:02Z.

## Next evidence

Re-run on Windows from a fresh session started with `claude --debug`, the
interval tuned to 1 minute at project scope, no turn interrupted, and read
the `mcp watchdog:` debug lines (`armed`, `tick skipped: ...`, `not
reconnected (...)`).


## Resolution (2026-10-30)

Not a watchdog bug. A fresh Windows session started with `--debug` (10:44:46Z) armed the watchdog at the 1-minute interval; the server was killed at 10:45:12Z, the 10:45:50Z tick found the tools unlisted, ran `/mcp reconnect plugin:wsflow:wsflow`, and the server was back at 10:45:51Z with the transcript success line and no model turn. The earlier misses were test artifacts: the knob was changed and picked up through `/reload-plugins`, which on Claude Code 2.1.314 (Windows, unchanged plugin code) re-spawned the MCP server but did not re-run the mod's `session.start` (no `armed` line after it), so the watchdog stayed off or on its start-time 10-minute interval, and the debug-logged run ended at 10:44:12Z before its first tick (10:45:48Z). Forward: this contradicts the 2.1.302 macOS probe note that a hot reload re-runs `register` and `session.start` (261016-research-claude-mod-mailbox-active-push); the knob descriptions already say a change applies from the next session, so no shipped text changes.

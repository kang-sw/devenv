---
title: "Pi worker's pi.wait MCP request times out before its requested wait budget"
---

# Pi worker's pi.wait MCP request times out before its requested wait budget

## Background

During the dashboard spawn-tree ticket's independent review on 2026-11-13,
a Pi worker called `pi.wait` for its reviewer with `timeout_s: 1000`. The tool
returned `MCP request timed out after 60000ms`, rather than waiting for the
requested budget or returning the documented running state. The reviewer was
not stopped.

## Cause (code evidence)

The deadline is not in ws-mcp. Ruled out:

- `piForwardTimeout = 60s` (`agents-plugin-tool/internal/mcp/pi_forward.go:30`)
  bounds only spawn/list/send/stop/transcript. `pi.wait` passes
  `timeout+piForwardSlack` (`pi_forward.go:247`, slack 30 s at `:27`), and
  `piCall` reissues a disconnected wait until its own deadline
  (`pi_forward.go:61-124`).
- The Pi bridge client in this repo (`agents-plugin-pi/src/mcp-stdio-client.ts`,
  `request`, `:278`) has no request timer, and the string `MCP request timed
  out` appears nowhere in this repository.

Remaining site: the Pi host's own MCP client. A Pi child gets ws-mcp through a
bridge extension that calls `pi.registerMcpServer(name, config)`
(`agents-plugin-tool/internal/wsagent/pi.go:210-217`, `writePiBridge` `:224`);
the ws-mcp entry carries only `command`, `args`, `env`, `cwd`, `exposure`, no
per-server request timeout, so Pi applies its default 60000 ms. (The same
60 s default is why the Codex plugin sets `tool_timeout_sec: 600`,
`agents-plugin/.mcp.json:11`.) The default 300 s `pi.wait`
(`agents-plugin-tool/internal/mcp/pi_tools.go:60`) has the same mismatch; any
wait above about 60 s is cut. The exact Pi config key is not in this repo;
Phase 1 confirms it against the installed Pi version before coding.

## Decisions

Compatibility tier: T0 (a bridge registration field and description text; no
tool, argument, store, or protocol change), per
`ai-docs/manuals/release-compatibility.md`.

Owner decision, 2026-11-13, option c: keep `pi.wait`, fix its timeout, and
narrow its description.

- Rejected: removing `pi.wait`. It would be T2 (a removal without an alias,
  minor-only); it breaks Codex sessions without mailbox push (Stop hook
  untrusted or no address registered) and Pi workers waiting synchronously on
  their own child; and it reverses epic Decision 6, which paired mail wake-up
  with a synchronous `pi.wait`.
- Rejected: fixing only, leaving the description as is. The description today
  presents `pi.wait` as a general way to collect a child's result, which pulls
  push-capable hosts into blocking calls.
- Never stop or respawn a healthy child because a wait request timed out;
  completion delivery stays asynchronous.

## Phases

### Phase 1: lift the Pi-host request deadline and narrow the descriptions

1. Find the Pi MCP server config key for the per-request/tool timeout in the
   installed Pi version (Pi's `registerMcpServer` docs or source). In
   `piMCPServers` (`agents-plugin-tool/internal/wsagent/pi.go:210`) set it on
   the ws-mcp entry to `piWaitMax + piForwardSlack` plus margin, from one
   shared constant derived from `internal/mcp/pi_tools.go:60-61` so the cap and
   the deadline cannot drift (if `wsagent` cannot import `mcp`, define the
   constant in `wsagent` and have `mcp` reference it). Fallback only if Pi
   exposes no such key: clamp a Pi worker's per-call `pi.wait` to 50 s in
   `handlePiForward` (`pi_forward.go:231`), return the documented `running`
   status when it elapses, and say so in the description.
2. Regression tests: (a) a `wsagent` test that the bridge source written by
   `writePiBridge` carries the timeout on the ws-mcp server and that it
   exceeds `piWaitMax`; (b) an `mcp` test that a forwarded `pi.wait` with
   `timeout_s` above 60 s gets a deadline of `timeout+piForwardSlack`, not
   `piForwardTimeout`.
3. Narrow descriptions. `pi.wait` in `pi_tools.go:587` (session form) and
   `pi_forward.go:264` (Pi worker form): state that it is for a host that does
   not wake on mailbox push, or for a Pi worker that must have its own child's
   result before continuing; otherwise end the turn and let the completion mail
   or report arrive. Align the sentences that mention it in `pi.spawn`
   (`pi_tools.go:518`; spawn answer text `:325`) and `pi.send` (`pi_tools.go:534`).
   Read `ai-docs/manuals/skill-authoring.md` first; shipped text must not name
   repo-only things.
4. Run `go test ./internal/mcp/... ./internal/wsagent/...` from
   `agents-plugin-tool/`.

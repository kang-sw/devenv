---
title: "Launchers fall to the direct path (exec the exe) while the daemon is replaced"
---

# Launchers fall to the direct path (exec the exe) while the daemon is replaced

## Background

Dogfood 2026-11-13: right after merge 0b3daf8e1 ("only the dashboard daemon
runs the ws exe"), `python scripts/dev.py rerun daemon` replaced the daemon
(new pid 24748). Sessions whose launchers predate the merge ended up on a
standalone ws-mcp process; `pi.spawn` from the lead answered "this MCP server
is not hosted by the dashboard daemon (... after the daemon refused its
connection)". A read-only diagnosis found no refusal in `dashboard.log`
(no "MCP session refused" line, `internal/wsdashboard/proxy.go:208`); no old
launcher ever reached the hello. The merge did not change the hello,
protocol (`ProxyProtocol` 1) or minor check.

Inferred cause (code and timing, not observed): the old launcher's
`McpProxy.recover` connects with a 1.5 s grace; a replacement gap longer than
that falls through to `start_daemon()` and then `ProxyFallback` ("continuing
on the direct path"), or the 3-quick-closes limit, both of which exec the ws
exe. The `piUnavailable` message (`internal/mcp/pi_tools.go:125`) names a
refusal whatever the actual reason.

## Idea

- A planned replacement keeps an endpoint answering until the successor is
  healthy, so an in-flight launcher's reconnect never reaches
  `start_daemon` or the direct path; or the successor is up before the old
  daemon drains.
- Record in `release-compatibility.md` that launchers older than 0b3daf8e1
  need one restart after updating (they still carry the exe fallback).
- Reword `piUnavailable` so it does not claim a refusal it cannot know.

- Neighbouring risk found the same day: daemon-process paths that reach the
  launcher (`agentMCPCommand`, `cmd/ws-mcp/agent_mcp.go:19-31`) resolve it
  from the daemon's `WS_RSRC_ROOT`, which for an installed plugin points into
  a versioned cache dir; a plugin update that removes that dir while the
  daemon keeps running breaks `pi.spawn` until the daemon is replaced.
  (`scripts/dev.py` lacked `WS_RSRC_ROOT` entirely; fixed in fbd1cd6a9.)

## Open

- Confirm which fallback fired: reproduce with an old-launcher session and
  its stderr captured.
- Whether the new launcher (no fallback, `DaemonUnavailable`) rides out the
  same gap or surfaces an error to the host.

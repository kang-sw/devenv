---
title: "Dev launcher rebuild replaces the dashboard daemon mid-work"
---

# Dev launcher rebuild replaces the dashboard daemon mid-work

## Background

Observation, 2026-11-13: the dashboard daemon restarted on its own twice, at
06:17:42Z and 06:31:37Z (pid 16160, binary mtime 06:31:36Z, command
`ws-mcp-dashboard-<hash>.exe dashboard --port 29417 --build-time ...`). Both
followed Go merges into the main checkout's develop (06:25:55Z `1073074d0`,
06:27:59Z `ea25ed915`). Effects seen: peer lead sessions got "dashboard daemon
not reachable" from `mailbox.send`, and one Pi completion mail was lost across
the earlier restart (the result had to be read via `pi.transcript`). Pi turns
themselves survived.

Cause (source evidence), `agents-plugin/bin/ws-mcp-launcher.py`:

- `local_devenv_source_fingerprint` (~543-589) stat-walks the repo tool_dir.
- `runtime_install_forced` (~535-539) forces install on a dev install.
- The decision at ~1798-1807 prints "local devenv source changed ...
  rebuilding from source"; `build_local_devenv_runtime` (~620-644) builds it.
- `needs_start` (~1298-1322) replaces the daemon when its build_time is older
  and the content hash differs.

So the next launcher start after any source change in the checkout (any new
MCP session, including a Pi spawn) rebuilds and replaces the machine's daemon.
The launcher does not check running Pi turns or connected sessions; its own
docstring says a replacement drains every session on the machine. Go-side
drain behavior in `wsdashboard.Ensure` is unverified.

Dogfood data point (2026-11-13, during the 3badee1b6 rebuild window, ~08:13Z):
a peer lead's two consecutive `tickets.sage_gate` calls failed fast with
"dashboard daemon not reachable; request was not sent", and its turn stalled
until the user said the daemon was back. Owner comment (verbatim): "데몬 리로딩되는 동안 작업이 멈춰버리는 게 문제네요"

## Idea

In dev mode, do not replace a daemon mid-work automatically.

- MCP tool calls should wait (bounded, e.g. tens of seconds) for a daemon that
  is mid-reload/replacement instead of failing fast.
- Compatibility: launcher-local behavior, T0; a config knob would also be T0.
  Changing replacement semantics is an observable workflow behavior change:
  ask before ready.

## Open

- Defer replacement while Pi turns or hosted sessions are active and report
  "rebuild pending" instead.
- An explicit rerun command to apply the pending rebuild.
- An opt-in auto-replace knob.
- Making completion mail durable across a replacement.
- Verify drain behavior in `wsdashboard.Ensure`.

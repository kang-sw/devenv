---
title: "Daemon replace ended two other projects' hosted lead sessions with a masked exit reason"
related:
  - 261113-bug-ws-launcher-dev-rebuild-replaces-daemon-mid-work
---

# Daemon replace ended two other projects' hosted lead sessions

This ticket is about the session-loss consequence and the holder mechanism. The
launcher trigger (auto rebuild and replace in dev mode) is the related ticket.

## Background

Observation, 2026-11-13. A dev-launcher rebuild replaced the dashboard daemon
at ~06:15:30Z. At 06:15:32.33Z and 06:15:32.43Z (0.09 s apart) two other
projects' dashboard-hosted Claude lead sessions stopped together. Both
`sessions/<slug>/registration.json` files say `host_stopped: true`,
`host_exit_code: 0`, `host_exit_reason: "child_exited"`; both transcripts ended
cleanly at idle. About 25 sessions' `agent.json` were rewritten at 06:15:30-32Z
(the successor daemon rejoining). `dashboard.log` was reset by the next daemon,
so there is no daemon-side line for the stop.

Scan of all 181 session dirs for `host_stopped_at` on this date (4 rows):

- 05:35:29Z `stop` (claude): a deliberate stop, unrelated.
- 06:15:32Z x2 `child_exited` (claude, hosted): the casualties, within 2 s of
  the replacement.
- 07:17:25Z `child_exited` (claude, hosted): 3 min before the 07:20:45Z
  replacement; not near any replacement, probably the user quitting.
- Nothing within +-10 s of the later replacements (06:17:42, 06:31:37,
  07:06:52, 07:20:45). One casualty slug was relaunched at 07:06:39Z, 13 s
  before 07:06:52Z; its new holder survived.

The devenv lead (hosted, claude, holder started 04:54:53Z) and its Pi children
survived every replacement. Every live holder descriptor (22: leads, Pi
workers, terminals, started 04:27Z-07:20Z) has `restart_safe: true`, so
restart_safe does not distinguish survivors from casualties. The casualties'
holder directories are gone, so their own restart_safe is unknowable now.

## Mechanism

Confirmed from code (graceful replace does not stop holders):

- `wsdashboard/ensure.go:139-146` replaces via `RequestShutdown` (graceful
  `/shutdown`), no kill.
- `wsdashboard/daemon.go:601-604` ("Detaches only: the holders and their
  children outlive the daemon") calls `Manager.Close`, `wsholder/manager.go:237-251`,
  which only closes clients.
- `wsagent/runner.go:2878-2890` `letGo`: a holder-backed lead is `Detach`ed
  (`wsagent/stream.go:360`, returns true) and runs on; only a process with no
  holder falls to `stopProcess` -> `proc.Close()`, which closes the child's
  stdin and so ends claude with exit 0 (`wsagent/backend.go:100,323`).
- Nothing in the shutdown path reads `restart_safe`; it is only recorded
  (`holder.py:1283,1299`) and shown (`dashboard.js:3347`).

Not confirmed (inference, medium-low confidence):

- Breakaway refused: `wsholder/manager.go:632-636` retries without
  `CREATE_BREAKAWAY_FROM_JOB` plus `--restart-unsafe`; such a holder lives in
  the daemon's job and dies with it (`spawn_windows.go:12-30`). But a killed
  holder would not write an exit record with `exit_code 0`; the record exists,
  so the holder was alive when claude exited 0.
- The 0 exit code fits claude exiting on stdin EOF or a console control event
  (holders share a hidden console, `wsholder/spawn_windows.go:15-19`), not a
  kill. Which of `letGo` fallback (holder nil), a rejoin failure, or a console
  event closed claude's input is not established.
- Masking: `holder.py:1238-1246` `request_stop` records `child_exited` whenever
  the child already exited, and `holder.py:824` defaults to `stop`; neither can
  say "daemon replaced", so a daemon-induced end looks like a normal exit.

## Idea

Compatibility: T0 for an added exit reason or a launcher guard; a change to
replace semantics is an observable behavior change, ask first.

- Find the real cause first: log, in the daemon, every holder detach/stop and
  every `letGo` fallback to `stopProcess`, in a file that survives the
  daemon's log reset (append across daemons).
- Record an honest reason: when the daemon closes a child's input or a holder
  stops during replace, write `daemon_replaced` (distinct from `child_exited`)
  into the exit record and the registration.
- Refuse or defer a replace while any hosted lead holder is live and
  restart-unsafe, and report "rebuild pending" (pairs with the launcher
  ticket's defer idea).
- On Windows, make holders survive the daemon unconditionally; if breakaway is
  refused, say so loudly (dashboard row, not just a descriptor flag).

## Open questions

- Which path ended the two leads: holder-less lead processes (started before
  holders existed or by an older daemon build) hitting `letGo`'s fallback, a
  console control event, or a restart-unsafe holder? Reproduce with a hosted
  lead under a daemon replace and capture the holder log before it deletes
  its directory.
- Why did both die within 0.09 s yet nothing else (22 live holders) did? A
  shared trait (older daemon build, project, start time) is the lead.
- Dashboard `holder.log` removal on exit loses the evidence; keep it with the
  retained exit details (`89ad004ec`).
- There is no launcher knob to disable the dev auto-rebuild: only
  `WS_MCP_BOOTSTRAP_BINARY`/`WS_MCP_BOOTSTRAP_URL` (which force install, the
  opposite) and `WS_MCP_RUNTIME_DIR` exist; local-devenv mode follows the
  contract file (`ws-mcp-launcher.py:532,535-539`).

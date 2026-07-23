---
title: "dashboard terminal: detached Windows terminal-helper inherits the daemon's listening TCP socket, blocking same-port daemon restart"
related:
  260723-feat-dashboard-terminal-lifetime-daemon-decouple: introduced-by
---

## Symptom

Surfaced during the native-Windows end-to-end acceptance walk of
`260723-feat-dashboard-terminal-lifetime-daemon-decouple` on 2026-07-23
(a Windows daemon binary rebuilt from goal tip `70a18a14`, driven from
PowerShell). After hard-killing the first daemon (`Stop-Process -Force`,
the SIGKILL-equivalent path with no graceful shutdown) while a terminal
helper was alive, restarting a daemon on the SAME port `4300` failed to
bind with Rust `os error 10048` (WSAEADDRINUSE, "Only one usage of each
socket address (protocol/network address/port) is normally permitted").
The detached terminal-helper had inherited the dead daemon's listening
TCP socket, so the port stayed occupied while the helper lived.

## Finding

Decisive evidence gathered on native Windows:

- Daemon #1 (PID 154976) was hard-killed with `Stop-Process -Force`; the
  terminal-helper (PID 207612) stayed alive as designed.
- Restarting a daemon on the same port 4300 failed to bind: Rust
  `os error 10048` (WSAEADDRINUSE).
- `Get-NetTCPConnection -LocalPort 4300` showed `State=Listen,
  OwningProcess=154976` — the listen socket persisted, still attributed
  to the already-dead daemon PID, while helper 207612 was alive.
- Restarting the daemon on a DIFFERENT port (4301, same state home)
  worked fine and reconciled/adopted the helper correctly.
- DECISIVE: closing the terminal via
  `DELETE /api/dashboard/terminals/{id}` made helper 207612 exit, and
  port 4300 then FREED immediately (`Get-NetTCPConnection -LocalPort 4300`
  returned no listener). This proves the surviving helper was holding the
  daemon's old listen socket the whole time.

Confirmed root cause (in source): `crates/daemon/src/terminal_platform.rs`,
`pub mod windows`, function `spawn_detached` (around lines 148-153). It
sets `command.creation_flags(CREATE_BREAKAWAY_FROM_JOB |
CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW)` then `command.spawn()`. Rust
`std::process::Command` on Windows spawns with `bInheritHandles=TRUE`, so
ALL inheritable handles in the daemon — including the daemon's listening
TCP socket (the mio/tokio `TcpListener`, which is not marked
non-inheritable via `WSA_FLAG_NO_HANDLE_INHERIT` or
`SetHandleInformation(handle, HANDLE_FLAG_INHERIT, 0)`) — are inherited by
the helper. The existing code/comment only addresses Job-Object breakaway,
not handle inheritance.

Unix is not affected: on Unix `std::process::Command` sets CLOEXEC on file
descriptors, so the listener is not inherited across exec. This is why the
Unix E2E (`crates/daemon/tests/terminal_lifetime.rs`) and the dogfood
walk on a different port never surfaced the bug — they use
ephemeral/different ports rather than rebinding the same fixed port.

## Impact

Severity: HIGH. This directly undermines the feature's core promise —
"terminals survive a daemon restart" — in the realistic production case,
where the daemon restarts on its SAME fixed configured port. The surviving
helper holds the old listen socket, so the daemon cannot rebind its normal
port until EVERY terminal helper exits — i.e. the operator would have to
kill the very terminals the feature exists to preserve. The mechanism only
appears to work in tests/dogfood because those use ephemeral/different
ports.

This gates the currently-held `goal/drain-ready-queue -> ws-dashboard-dev`
merge decision: the survival + reconcile mechanisms are verified on native
Windows, but the same-port-restart defect must be fixed or explicitly
accepted before the Windows surface is production-correct.

## Fix direction (not decided)

- (a) Mark the daemon's TCP listener socket non-inheritable on Windows via
  `SetHandleInformation(handle, HANDLE_FLAG_INHERIT, 0)` right after bind.
- (b) Spawn the helper with handle inheritance disabled
  (`bInheritHandles=FALSE`) — the helper communicates over a
  named-pipe/socket PATH, not inherited fds, so it needs no inherited
  handles at all.
- (c) Audit all daemon-held inheritable handles similarly, in case the
  listen socket is not the only leaked handle.

Prefer whichever is smallest and platform-scoped. None of the above is
decided; this ticket captures the finding for triage.

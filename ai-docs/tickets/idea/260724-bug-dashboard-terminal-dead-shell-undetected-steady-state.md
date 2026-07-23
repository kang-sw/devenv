---
title: "dashboard terminal: a dead/silently-exited shell is not detected during steady-state daemon operation, leaving a zombie pane that must be closed by hand"
related:
  260723-feat-dashboard-terminal-lifetime-daemon-decouple: introduced-by
---

## Symptom

Observed during dogfooding the Windows gateway (2026-07-24). On Windows the
shell (powershell) inside a terminal sometimes dies while the daemon keeps
running (the user's hypothesis: powershell/ConPTY exits after a stretch with
no IO). When that happens the frontend still shows the terminal as a live
entry occupying a workbench slot, and the only way to clear it is a manual
close. The dead shell is never auto-detected or retired.

## Finding

Traced end-to-end (paths under `ws-dashboard/`):

- **Exit detection hinges entirely on PTY EOF.** The helper's only child-exit
  trigger is the PTY master read side: `crates/daemon/src/terminal_helper_process.rs:457-472`
  — `read()==Ok(0)` (EOF) -> `transition(Exited)`, `read()==Err` ->
  `transition(Error)`. There is **no independent `child.try_wait()`/poll** of
  the shell process; `child.wait()` is only reached inside
  `kill_shell_if_running` (`:114-121`) after an explicit kill. If the shell
  dies but the PTY master never signals EOF — a known Windows/ConPTY failure
  mode, where conhost can hold the pipe open — the helper stays stuck
  reporting `Running` and never emits the `Exit` IPC message.
- **No steady-state liveness probe on the daemon side.** The daemon reacts
  only to (i) an inbound helper IPC message or (ii) the IPC connection
  dropping (`spawn_ipc_reader_task`, `crates/daemon/src/terminal.rs:1040-1076`;
  socket-close -> `mark_ipc_closed` -> status `Error`, `:1069-1071`). There is
  no heartbeat, ping/keepalive, or periodic process-alive check. Full
  PID-identity + IPC-reachability probing exists **only at daemon boot**
  (`boot_reconcile`, `terminal.rs:196-270`, wired at `server.rs:99-102`). So a
  live helper whose shell is silently dead is invisible to the daemon while it
  keeps running.
- **No idle/no-IO kill anywhere in daemon or helper.** The only timeout
  constants are the post-exit reattach `GRACE_WINDOW = 30s`
  (`terminal_helper_process.rs:32`), the `IDLE_ACCEPT_POLL = 2s` accept
  re-poll cadence (`:33`), the daemon-side `DAEMON_GRACE_WINDOW_MS`
  (`terminal.rs:45`), and handshake connect timeouts (`terminal.rs:46-47`).
  None terminates a live shell for inactivity. **Powershell dying "on no IO"
  is therefore external OS/ConPTY behavior with zero daemon involvement** — the
  daemon neither causes it nor notices it.
- **No frontend auto-reap.** Even when status *does* flip to
  `exited`/`error`/`terminated`, no code retires the pane; it keeps rendering
  and holding its slot until a manual close button ->
  `DELETE /api/dashboard/terminals/{id}` (`frontend/src/terminalPaneBody.tsx:731-734`
  -> `App.tsx:5608-5628`). `listTerminals` is fetched only on work-root
  switch/mount, not on an interval (`App.tsx:4357-4419`), so even the
  daemon-side list drop-off (`admits_attach()` filtering,
  `terminal.rs:272-280`) never propagates spontaneously.

## Impact

Severity: MEDIUM (user called it minor at the point of observation). It does
not lose data or corrupt state, but it degrades the terminal UX exactly on the
Windows surface the decouple feature targets: dead shells accumulate as zombie
panes that silently occupy slots and require manual cleanup, and the operator
cannot tell a wedged shell from a live one. The core failure — detection
hinging solely on PTY EOF — means any shell death that doesn't produce EOF is
undetectable, which is precisely the reported Windows/ConPTY case.

## Fix direction (not decided)

- (a) **Helper-side independent child-liveness probe.** Add a periodic
  `child.try_wait()` (or a Windows process-handle/job-object wait) alongside
  the PTY reader, so shell death is caught even when the master never signals
  EOF. This targets the root cause directly and is the smallest change that
  fixes the actual reported symptom.
- (b) **Daemon-side steady-state liveness check.** A lightweight periodic
  probe (PID-alive and/or IPC ping) of each adopted helper, transitioning to
  `Error`/`Exited` on failure — reuses `boot_reconcile`'s probing logic but on
  an interval instead of only at boot.
- (c) **Frontend retirement of dead panes.** Gray-out and/or auto-remove panes
  whose `status` is `exited`/`terminated`/`error`, and/or add a periodic
  `listTerminals` re-poll so daemon-side drop-off propagates without a manual
  root switch.

(a) fixes detection at the source; (c) fixes the user-visible slot-occupancy
regardless of detection latency; (b) is the general safety net. Likely (a)+(c)
together. None decided; this ticket captures the finding for triage.

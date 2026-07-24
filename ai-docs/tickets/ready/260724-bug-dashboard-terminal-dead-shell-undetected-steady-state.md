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

## Approach (decided)

Detection is fixed at the source with an **event-driven** mechanism, not a
poll: the helper already holds the shell's raw Windows process HANDLE (it is
duplicated for job-object assignment at `terminal_helper_process.rs:403`). A
dedicated reaper thread blocks on that handle via
`WaitForSingleObject(handle, INFINITE)`; it sleeps in the kernel (zero idle
CPU, zero poll-interval latency) and wakes the instant the shell process dies —
**independent of whether ConPTY ever signals PTY EOF**. On wake it drives the
same `SharedState::transition(Exited)` path the PTY-EOF reader already uses, so
the existing `Exit` IPC -> daemon -> WS `exit` frame pipeline is reused
unchanged. This is preferred over a periodic `child.try_wait()` loop: same
correctness, but no polling and instant detection.

Scope and rationale:

- **Windows-only reaper.** Unix already surfaces shell death reliably via PTY
  EOF (that is why this bug is Windows-specific), and Unix `waitpid` can reap a
  child only once — a second reaper thread would race/steal the reap from
  portable_pty's own `wait()`. Windows process handles allow many concurrent
  waiters, so the reaper coexists cleanly with `kill_shell_if_running`'s
  `child.wait()` (`:117`). Gate the whole mechanism behind `#[cfg(windows)]`.
- **Idempotent, kill-aware transition.** A daemon-initiated close
  (`kill_shell_if_running` -> status `Terminated`) also unblocks the reaper;
  the reaper must not clobber a `Terminated`/already-exited status with
  `Exited`. Guard on current status so intentional close stays `Terminated`.
- **Frontend dead-pane retirement (independent of detection latency).**
  Retire panes whose `status` is `exited`/`terminated`/`error` (gray-out and
  offer/auto one-click clear), and add a bounded periodic `listTerminals`
  re-poll (or reuse an existing tick) so the daemon-side list drop-off
  propagates without a manual work-root switch.

Not doing (b) from the original triage (daemon-side steady-state heartbeat):
the Windows handle-wait reaper closes the root cause directly and the IPC
socket-close path already covers helper-process death, so a separate daemon
poll adds cost without covering a new failure mode. Left as a future option if
a non-EOF, non-process-death wedge is ever observed.

## Phases

### Phase 1: Windows helper-side process-handle reaper (detection at source)

Add a `#[cfg(windows)]` reaper thread in `terminal_helper_process.rs` that
duplicates the shell child's raw process HANDLE (alongside the existing
job-object handle grab near `:403`) and blocks on
`WaitForSingleObject(handle, INFINITE)`. On wake, call
`SharedState::transition(Exited)` guarded so it is a no-op when the current
status is already `Terminated`/`Exited`/`Error` (i.e. it must not override a
daemon-initiated close). Verify the transition still fans out through the
existing per-connection `notify` arm (`:343-367`) -> `Exit` IPC ->
`apply_helper_status` (`terminal.rs:1010`) unchanged. Unix path untouched.
Handle the duplicated handle's lifetime/close correctly (no leak, no
double-close vs `kill_shell_if_running`).

### Phase 2: Frontend retirement of dead panes

In `frontend/src`, make panes whose `session.status` is
`exited`/`terminated`/`error` visually retire (gray-out + a clear affordance),
and either auto-remove them or make the retirement obvious enough that they no
longer read as live slots. Add a bounded periodic `listTerminals` re-poll (or
piggyback an existing interval) so the daemon-side `admits_attach()` drop-off
(`terminal.rs:272-280`) propagates without a manual work-root switch. Keep the
manual close path working; avoid double-DELETE races between auto-reap and the
close button.

### Phase 3: Verification (Unix regression + native-Windows acceptance)

- Unix: extend/adjust `crates/daemon/tests/terminal_lifetime.rs` so the
  existing PTY-EOF exit path still flips status to `exited` (no regression from
  the reaper wiring being compiled out on Unix).
- Native Windows: using the established dogfood harness (rebuild the Windows
  binary from the goal tip, drive via PowerShell), reproduce a shell death that
  does NOT produce PTY EOF (e.g. terminate the shell process directly, leaving
  ConPTY holding the pipe) and confirm the terminal now flips to `exited` and
  the frontend pane retires — where before it stayed `running` forever. Record
  the acceptance walk in this ticket's Result (do not edit frozen text).

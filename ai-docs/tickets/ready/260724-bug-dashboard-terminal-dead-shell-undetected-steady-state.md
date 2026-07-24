---
title: "dashboard terminal: a dead/silently-exited shell is not detected during steady-state daemon operation, leaving a zombie pane that must be closed by hand"
related:
  260723-feat-dashboard-terminal-lifetime-daemon-decouple: introduced-by
sage-review-design: completed
sage-review-completeness: completed
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
poll: the helper already accesses the shell's raw Windows process HANDLE for
job-object assignment at `terminal_helper_process.rs:403` — but note
`portable_pty::Child::as_raw_handle()` returns a **borrowed** handle owned by
the `Child`, which is `CloseHandle`d when `kill_shell_if_running` takes and
drops the `Child` (`:114-118`). The reaper therefore **must hold its own
`DuplicateHandle` copy** (not the borrowed raw handle, which could be closed
and recycled out from under a blocking wait). A dedicated reaper thread blocks
on that duplicated handle via `WaitForSingleObject(handle, INFINITE)`; it
sleeps in the kernel (zero idle CPU, zero poll-interval latency) and wakes the
instant the shell process dies — **independent of whether ConPTY ever signals
PTY EOF**. On wake it drives the same `SharedState::transition(Exited)` path
the PTY-EOF reader already uses, so the existing `Exit` IPC -> daemon -> WS
`exit` frame pipeline is reused unchanged.

**Why event-driven over a `try_wait()` poll thread.** A poll thread
(`Child::try_wait()`, non-blocking) is materially simpler — no
`DuplicateHandle`, no `#[cfg(windows)]`, and it would also cover the Unix
grandchild-fd residual below. It is rejected only for the zero-latency /
zero-idle-CPU property (owner preference for the elegant path); the poll
variant is an acceptable fallback if the handle-wait wiring proves fiddly.

Scope and rationale:

- **Windows-only reaper.** Unix `waitpid` can reap a child only once — a
  second reaper thread would race/steal the reap from portable_pty's own
  `wait()` (`kill_shell_if_running:117`). Windows process handles allow many
  concurrent waiters, so the reaper coexists cleanly with that `wait()` (they
  wait on independent handle copies). Gate the whole mechanism behind
  `#[cfg(windows)]`. **Unix is *mostly* covered by PTY EOF, not immune:** the
  master EOFs only when all slave fds close, so a grandchild that inherited the
  slave fd and outlives the shell reproduces the same zombie-pane symptom on
  Unix. This residual is knowingly accepted here; the clean Unix analog if it
  ever matters is `pidfd_open` + `poll` (event-driven, non-reaping, does not
  race `waitpid`).
- **Kill-path ordering (make the guard real).** The reaper must not stamp
  `Exited` over a daemon-initiated close — but note the helper ring is **never**
  set to `Terminated` today: `Terminated` is daemon-side only
  (`terminal.rs:976`), while `kill_shell_if_running` (`:114-118`) and the
  `GracefulShutdown` arm (`:337-340`) set no helper status at all. So a naive
  "no-op when already Terminated" guard is a no-op that never fires — the
  reaper would wake on the intentional kill and stamp `Exited` while the ring
  is still `Running`, and a `select!` race (`:293`) could deliver that
  `Exit{Exited}` over the wire and overwrite the daemon's `Terminated`. Fix:
  transition the helper ring to `Terminated` **before** `child.kill()` in the
  kill path, so the ring is non-`Running` first and the reaper's
  `transition(Exited)` becomes a genuine no-op.
- **Frontend dead-pane retirement.** The reaper's `Exited` already reaches the
  pane live over the WebSocket status frame (`terminalPaneBody.tsx:589-594`), so
  no poll is needed for *detection*. Retire panes whose `status` is
  `exited`/`terminated`/`error` by **gray-out + an explicit clear affordance**,
  preferring retain-with-clear over auto-remove so the exited shell's final
  scrollback stays readable. A bounded, coarse (seconds, not the existing 120ms
  output poll) `listTerminals` re-poll is a **reconciliation backstop** for the
  daemon-side `admits_attach()` drop-off (`terminal.rs:272-280`) when the WS is
  in fallback — not the primary detection path. `list_for_work_root` reads only
  the in-memory registry (no git), so the poll is cheap.
- **Idempotent close (avoid the auto-reap/manual-close race).**
  `close_terminal` returns `404 NOT_FOUND` when the terminal is already gone
  (`terminal.rs:737-745`), and `closeTerminalPane` (`App.tsx:5608-5628`) surfaces
  any rejection as "terminal close failed". Whichever of {auto-reap, manual
  close} loses the race would raise a spurious error. Treat `404` on close as
  success (idempotent removal) and/or have retirement remove the pane locally
  without a competing DELETE.

Not doing (b) from the original triage (daemon-side steady-state heartbeat):
the Windows handle-wait reaper closes the root cause directly and the IPC
socket-close path already covers helper-process death, so a separate daemon
poll adds cost without covering a new failure mode. Left as a future option if
a non-EOF, non-process-death wedge is ever observed.

## Phases

### Phase 1: Windows helper-side process-handle reaper (detection at source)

Add a `#[cfg(windows)]` reaper thread in `terminal_helper_process.rs` that
blocks on `WaitForSingleObject(handle, INFINITE)` and, on wake, calls
`SharedState::transition(Exited)`. Implementation guardrails (all evidence-based
against portable-pty-0.8.1 `src/win/mod.rs`):

- **Own a duplicated handle.** Capture the shell child's raw handle at
  `spawn_shell` **before** the child is moved into `shared.child` (near `:431`),
  `DuplicateHandle` it with `DUPLICATE_SAME_ACCESS` (carries `SYNCHRONIZE`,
  required by `WaitForSingleObject`), and store the dup in a
  `std::os::windows::io::OwnedHandle` so its close is automatic and distinct
  from the `Child`'s own handle (no double-close).
- **Hold an `Arc<SharedState>` clone** in the reaper (like `spawn_reader_thread`
  at `:457`) so `SharedState` outlives the wait; the thread is detached (never
  joined).
- **Kill-path ordering:** transition the ring to `Terminated` before
  `child.kill()` in the kill path (see Approach) so the reaper's `Exited` is a
  real no-op, not a status the ring can never actually hold.

Verify the transition still fans out through the existing per-connection
`notify` arm (`:343-367`) -> `Exit` IPC -> `apply_helper_status`
(`terminal.rs:1010`) unchanged. Unix path untouched. Exactly one shell per
helper (`shell_started` compare_exchange at `:298-302`) means exactly one reaper
thread; every helper-exit path runs `kill_shell_if_running` (`:195`) which kills
the shell and unblocks the reaper, so no thread/handle leak.

### Phase 2: Frontend retirement of dead panes

In `frontend/src`, make panes whose `session.status` is
`exited`/`terminated`/`error` visually retire via **gray-out + an explicit
clear affordance** (retain-with-clear, not immediate auto-remove, so final
scrollback stays readable). Detection already arrives live over the WS status
frame (`terminalPaneBody.tsx:589-594`); add only a **coarse** (seconds)
`listTerminals` reconciliation backstop for the daemon-side `admits_attach()`
drop-off (`terminal.rs:272-280`) when the WS is in fallback — keep it far
coarser than the existing 120ms output poll (`App.tsx:441`). Make close
idempotent: treat `404` from `close_terminal` (`terminal.rs:737-745`) as
success in `closeTerminalPane` (`App.tsx:5608-5628`) so the auto-reap/manual-
close race does not raise a spurious "terminal close failed".

Phase-local verification: a terminal whose helper reports
`exited`/`error`/`terminated` visually retires without a work-root switch, the
clear affordance removes the pane, and a manual close on an already-retired or
already-gone pane does not surface "terminal close failed" (the `404`-as-success
path); the reconciliation poll only needs asserting when the WS is forced into
fallback.

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

## Spec Impact

Governing spec: `ai-docs/spec/ws-web-dashboard/index.md`.

- **Exit-status detection (Phase 1) is already within the existing contract.**
  `#260516-ws-web-dashboard-terminal-websocket-transport` already states the WS
  "carries ordered PTY output, status, and exit data to the browser", and
  `#260723-terminal-attach-grace-window` covers the post-exit grace/drop. A
  shell that dies without PTY EOF surfacing as `exited` is that contract holding
  on Windows/ConPTY, not a new behavior — **no new spec text; closeout only.**
- **Frontend dead-pane retirement (Phase 2) is new caller-visible behavior.**
  `#260516-ws-web-dashboard-terminal-pane` describes pane rendering but says
  nothing about how an `exited`/`terminated`/`error` session is visually retired
  or cleared. That anchor will need a sentence covering retain-with-clear
  (gray-out + explicit clear affordance, final scrollback preserved) and that
  close is idempotent. `judge: contract-first-spec` = no (UI refinement, final
  shape settles during implementation) — **address at post-implementation
  closeout, not before.**


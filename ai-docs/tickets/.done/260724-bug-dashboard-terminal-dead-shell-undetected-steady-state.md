---
title: "dashboard terminal: a dead/silently-exited shell is not detected during steady-state daemon operation, leaving a zombie pane that must be closed by hand"
related:
  260723-feat-dashboard-terminal-lifetime-daemon-decouple: introduced-by
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-07-25
---

## Native-Windows leg verified — closing (2026-07-25)

Both legs of Phase 3 are now complete, so this ticket is closed to `.done/`:

- **Unix-regression leg**: COMPLETE (`e2990574`).
- **Native-Windows acceptance leg**: VERIFIED by the dedicated end-of-drain
  acceptance ticket `260724-chore-dashboard-windows-terminal-reaper-native-acceptance`
  (which `verifies:` this one). Its `#[cfg(windows)]` live-ConPTY integration
  test `crates/daemon/tests/terminal_windows_reaper_acceptance.rs` (Result
  `f5891a7e`) spawns the real `terminal-helper` subprocess, drives the IPC
  handshake to spawn a real ConPTY shell, kills that shell's OS process
  out-of-band via `taskkill` so the PTY master never observes EOF, and proves
  the Phase-1 `#[cfg(windows)]` reaper wakes and flips status to `Exited` on a
  real Windows host — non-vacuity confirmed by mutation (reaper `transition`
  neutralized → 15s hang/FAIL; restored → PASS). This exercises exactly the
  non-PTY-EOF shell-death path this ticket's root-cause analysis identified.

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

### Result (b07f40ad) - 2026-07-24

Shipped the Windows helper-side event-driven exit reaper as designed.

- **Reaper mechanism** (`b07f40ad`): added `terminal_platform::windows::duplicate_process_handle`
  (`DuplicateHandle` with `DUPLICATE_SAME_ACCESS`, so the copy carries `SYNCHRONIZE`)
  and `wait_for_process_exit` (`WaitForSingleObject(INFINITE)`). In
  `terminal_helper_process::spawn_shell`'s `#[cfg(windows)]` block the shell's
  process handle is duplicated into an `OwnedHandle` before the `Child` moves into
  shared state and handed to a detached `spawn_process_exit_reaper` thread that
  blocks on it and, on wake, drives the same `SharedState::transition(Exited)` the
  PTY-EOF reader uses — reusing the existing `Exit` IPC -> daemon -> WS exit-frame
  pipeline unchanged. Detection therefore no longer hinges solely on PTY-master
  EOF, closing the Windows/ConPTY "conhost holds the pipe open" root cause.
- **Kill-path reorder** (`b07f40ad`): `kill_shell_if_running` now stamps the ring
  `Terminated` BEFORE `child.kill()` (cross-platform) so a reaper waking on the
  intentional death finds a non-`Running` ring and its `transition(Exited)` is a
  genuine no-op instead of overwriting the daemon's `Terminated`. The ring was
  never actually set to `Terminated` before this change, so the original
  "no-op when Terminated" guard was a guard that could never fire; this makes it
  real. PTY master / writer channel are still torn down AFTER child death (writer
  starvation ordering, unchanged).
- **Guard tests** (`e8f9f603`): three non-vacuous, non-windows-gated unit tests in
  a `#[cfg(test)] mod kill_path_guard_tests`: `transition(Exited)` from `Terminated`
  stays `Terminated` (guard no-op); `transition` from `Running` still reaches
  `Exited` (negative control proving the gate is non-`Running`, not a blanket
  freeze); `kill_shell_if_running` leaves the ring `Terminated` (the load-bearing
  stamp). Verified non-vacuous by mutation: dropping the `Running`-only guard fails
  test 1, dropping the pre-kill stamp fails test 3, a never-mutating `transition`
  fails test 2.

Verification: both gates green — `cargo test -p ws-dashboard-daemon` and
`cargo check --target x86_64-pc-windows-gnu -p ws-dashboard-daemon`. Honest
limitation: the stamp-before-kill *ordering* against a real racing reaper is not
unit-testable without a live PTY child plus a concurrent reaper thread; the
`Running`-only guard test and the stamp-happens test together cover the
pure-state reliance ("`Exited` becomes a no-op once the ring has left `Running`")
that the ordering exists to guarantee. Native-Windows runtime acceptance (a shell
death with no PTY EOF flipping the pane to `exited`) is explicitly deferred to
Phase 3 and requires a real Windows host — not executable in this environment.

Spec: closeout only, no new spec text. Exit-status detection on Windows/ConPTY is
the existing WS transport contract holding
(`#260516-ws-web-dashboard-terminal-websocket-transport` +
`#260723-terminal-attach-grace-window`), not new behavior.

Mental model: extended `ai-docs/mental-model/ws-web-dashboard/terminal.md` with a
Module Contract capturing the dual exit-detection path (event-driven Windows
handle-wait reaper independent of ConPTY EOF) and the make-the-guard-real kill
ordering (`Terminated` stamped before `child.kill()`, reaper `Exited` a real
no-op). `terminal-render.md` was assessed and correctly left untouched — it is
the frontend render-batching sub-domain, not daemon-side lifecycle.

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

### Result (2b4d0e0b) - 2026-07-24

Landed the three frontend behaviors (range `862d58b0..2b4d0e0b`, all under
`ws-dashboard/frontend/src`, zero daemon/Rust edits):

- **Idempotent close** — `closeTerminal` (`terminals.ts`) now returns normally
  on a `404` response instead of throwing, so the auto-reap/manual-close race
  no longer surfaces "terminal close failed". The daemon already returns `404`
  correctly in both `close_terminal` guard branches, so the fix is entirely
  client-side and needed no server change.
- **Retain-with-clear retirement** — a pane whose `pane.session.status` is
  `exited`/`terminated`/`error` grays out (new `.terminal-pane-retired` rule in
  `styles.css` reusing `--ws-color-text-disabled`, no new raw color) and its
  control relabels to "Clear"; the pane is retained (scrollback preserved) until
  the owner clears it, not auto-removed.
- **Coarse reconciliation backstop** — a new 5s per-work-root `useEffect`
  (`terminalListReconciliationPollIntervalMs`) that, only while any pane is on
  the `socketStatus === "fallback"` path, re-lists terminals and prunes panes
  the daemon no longer lists, via a shared `applyListedTerminalSessions`
  apply-path now used by both the mount effect and the backstop (reuse of the
  existing `reconcileListedTerminalSessions`, far coarser than the 120ms output
  poll).

Anchor corrections against this ticket's frozen Phase 2 text (line numbers had
drifted after Phase 1 landed): `close_terminal` is `terminal.rs:784-799` (not
737-745, which is `terminal_resize`); `closeTerminalPane` is `App.tsx:5749-5775`
(not 5608-5628); the WS status-frame handler is `terminalPaneBody.tsx ~614-619`.
Design refinement over the plan: the retirement gate reads the `pane` prop's
`pane.session.status`, **not** the component-local `displaySession` mirror. A
mental-model footgun note (`terminal-render.md`, `5c95a5b8`) records the
source-verified reason — Dockview panel-param forwarding is suppressed while the
socket is `connecting`/`connected` (`shouldUpdateDockviewWorkbenchPanelParams`),
so `displaySession` freshness is not guaranteed — correcting the plan's looser
"stale during fallback polling" phrasing while keeping the same guidance.

Verification: `npm run build` clean; `npm run test:terminals` exit 0, with two
added `terminals.test.ts` cases (`closeTerminal` resolves on a mocked `404`; the
reconciliation prune boundary). The UI gray-out/clear affordance and the
fallback-gated backstop are dogfood-level (no DOM/browser harness in
`test:terminals`; `test:browser` is independently RED on unrelated 260713), so
only the two pure-function units are pinned — matching the plan's scoped ask, not
overclaiming UI coverage. Reviews: correctness clean, fit clean, test clean with
one accepted minor (the prune predicate's exact-equality boundary
`localCreatedAtMs === pruneStartedAtMs` is unpinned on pre-existing, unmodified
code). Spec closeout: `#260724-terminal-pane-dead-session-retire` (`0fbbceb4`).

Deferred: Phase 3 (Unix regression + native-Windows acceptance) remains — ticket
stays in `ready/`.

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

### Result (e2990574) - 2026-07-24

Phase 3 split by leg: the Unix-regression leg is COMPLETE; the native-Windows
acceptance leg is NOT (blocked — see the `## Blocked (2026-07-24)` note).
Branch `impl/terminal-pty-eof-exit-regression`, merging into
`goal/drain-ready-queue`.

- **Unix-regression leg — DONE.** Added `#[tokio::test]
  terminal_live_pty_eof_exit_flips_status_to_exited` to
  `crates/daemon/tests/terminal_lifetime.rs` (commit `e2990574`). It guards the
  LIVE single-daemon steady-state path: create a terminal, attach a WebSocket,
  drive the shell to exit normally so the PTY master EOFs, and assert a WS
  exit/status frame reports `exited`. This proves the Phase-1
  `#[cfg(windows)]`-gated reaper being compiled OUT on Unix (plus the kill-path
  reorder) did NOT regress the pre-existing PTY-master-EOF exit-detection path.
  Distinct from the two existing tests: `terminal_survives_*` keeps the shell
  alive across a restart; `terminal_boot_reconcile_*` discovers exit via
  adoption during a daemon-down window.
- **Reliability hardening (commit `131a9ffb`).** The first cut used a novel
  "quiet-gap" readiness heuristic that was ~40% flaky in isolation on
  interactive `zsh`+powerlevel10k hosts (an 8s startup-output deadline that
  times out under load, false-RED). Replaced it with the proven `echo <marker>`
  + `poll_output_until_contains` readiness handshake (same idiom as
  `terminal_survives_*`) → 20/20 isolation-green. Also: added a `HelperReaper`
  `Drop` guard that reaps the detached setsid+double-fork terminal-helper on
  panic paths (the daemon's `kill_on_drop` cannot reach it, and leaked live
  shells self-amplified the flake); pinned `status == "exited"` on the
  exit-frame branch; broke the drain loop on stream-end.
- **Reaper directory fix (commit `6d8a5575`).** The `HelperReaper` initially
  scanned `WS_DASHBOARD_STATE_HOME` flat, but registry entries live at
  `<state_home>/terminals/<terminal_id>.json`, so it never found a pid and never
  reaped. Pointed it at the `terminals/` subdir; verified it actually reaps on a
  panic path (panic injection: helper pid reaped, no leak).
- **Reviews.** Correctness+test review confirmed the assertion is non-vacuous
  and targets the right live PTY-EOF path, and surfaced the flake + leak (both
  fixed). A focused re-review of the `HelperReaper` SIGKILL guard confirmed the
  `/proc/<pid>/stat` field-22 starttime parse (`rsplit_once(')')` then
  `.nth(19)`) and the recycled-PID safety (starttime match before kill; all
  `None` paths skip) are sound.
- **Docs.** Mental-model footguns captured in
  `ai-docs/mental-model/ws-web-dashboard/terminal.md` (commit `c6b4c87d`): (1)
  do not drive terminal input during the interactive-shell startup window — use
  the marker/poll handshake; (2) the helper detaches and survives daemon kill,
  so tests must reap it. Spec unchanged (test-only; the dead-pane behavior was
  specced in Phase 2). A pre-existing, independent flaky test
  (`terminal_boot_reconcile_...`, which violates footgun #1) was spun off as
  idea ticket
  `260724-idea-dashboard-daemon-terminal-lifetime-test-interactive-shell-timing`
  (commit `d140f536`) — NOT fixed here (changing an existing test's timing
  CONTRACT is out of scope).
- **Native-Windows acceptance leg — NOT DONE (blocked).** Rebuilding the
  Windows binary from the goal tip and driving it via PowerShell to reproduce a
  non-PTY-EOF shell death and confirm the `#[cfg(windows)]` reaper flips status
  to `exited` requires the user's real Windows environment; see the
  `## Blocked (2026-07-24)` note.

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



## Resolution (2026-07-25)

Both Phase 3 legs complete: Unix-regression leg `e2990574`; native-Windows acceptance leg verified by the dedicated chore ticket `260724-chore-dashboard-windows-terminal-reaper-native-acceptance` (Result `f5891a7e`), whose live-ConPTY test proves the Phase-1 `#[cfg(windows)]` reaper wakes and flips status to `Exited` on a non-PTY-EOF out-of-band shell death on a real Windows host. Phase 1 (Windows reaper `b07f40ad`) and Phase 2 (frontend dead-pane retirement `2b4d0e0b`) shipped earlier.

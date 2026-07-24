# Plan: dashboard terminal: runtime-verify the #[cfg(windows)] dead-shell reaper on a real Windows host via the native cargo harness (end-of-drain acceptance) — Phase 1: Native-Windows reaper acceptance test

## Relevant Ticket Contract
- Add a `#[cfg(windows)]` acceptance test that: (1) spawns a real ConPTY-backed
  shell child through the same helper path the product uses (not a mocked
  handle), (2) terminates that child **out-of-band** (kill the OS process
  directly, not through the daemon's own kill path) so the PTY master does
  not observe EOF, (3) asserts the reaper wakes and the terminal transitions
  to `Exited` within a bounded timeout.
- Ticket explicitly leaves open: co-located `#[cfg(test)]` unit module in
  `terminal_helper_process.rs` vs. a dedicated `crates/daemon/tests/`
  integration test — resolving this is this survey's job.
- Must run on real Windows via the validated `powershell.exe`
  scratch-worktree harness in `ai-docs/_index.local.md`; never the primary
  `D:\dbg-ws-dashboard-dev` checkout; `command git worktree remove --force`
  on completion; never touch the `:4300` production gateway daemon.
- Must prove non-vacuity by mutation: suppress the reaper's
  `transition(Exited)` wake, confirm the test hangs-to-timeout/fails; restore
  it, confirm the test passes. Record the mutation evidence in the ticket's
  `## Result` (this happens during Phase 1 *execution* on the Windows host,
  not as a shipped source change from this plan).
- Keep the Linux build green: `cargo test -p ws-dashboard-daemon` on Linux
  must be unaffected by the new test.
- Test-only; no spec impact (ticket's own "Spec Impact" section: closeout-only).
- Sage review was explicitly skipped for this ticket — any newly surfaced
  binding design decision must be surfaced conservatively, not assumed
  pre-authorized. (See risk signal under Codebase Findings: the Job-Object
  self-kill hazard is exactly such a decision, resolved here with concrete
  evidence, but the executor/lead should still read it before proceeding.)

## Out of Scope
- Any behavior/product change to the reaper itself (already shipped at
  `b07f40ad`); this ticket is test-only.
- The dead-shell ticket's other Phase 3 material or the Unix-side EOF
  regression coverage — already covered by
  `terminal_live_pty_eof_exit_flips_status_to_exited` in `terminal_lifetime.rs`.
- The full HTTP/daemon/WebSocket surface (`spawn_real_daemon`,
  `create_terminal`, WebSocket reattach) — not needed here, since this
  acceptance scenario only depends on the helper's own IPC protocol, not the
  daemon's HTTP layer.
- Permanently committing the non-vacuity mutation — it is a temporary,
  reverted local edit made during Windows execution to gather evidence, not
  part of this plan's deliverable diff.
- Adding any new Cargo dependency (e.g. `windows-sys` Toolhelp/CIM bindings)
  to drive the out-of-band kill; the plan below uses only external OS tools
  (`powershell.exe`, `taskkill`) already relied on by the harness, avoiding
  any Cargo.toml change.

## Codebase Findings
- `ws-dashboard/crates/daemon/src/terminal_helper_process.rs#L391-L481`
  (`spawn_shell`) — on Windows, calls
  `terminal_platform::windows::create_kill_on_close_job()`, which assigns
  **the calling process itself** into a fresh `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`
  job (`terminal_platform.rs#L184-L215`, `AssignProcessToJobObject(job,
  GetCurrentProcess())`). **Risk signal**: calling `spawn_shell` directly from
  a `#[cfg(test)] mod` inside `terminal_helper_process.rs` (the unit-module
  option) runs it inside the `cargo test` process itself — that process would
  get assigned into the kill-on-close job, and dropping the stored
  `OwnedHandle` (`SharedState.job`) at end of test would `CloseHandle` the job
  and, per its own doc comment ("dropping/closing it ... tears down every
  process still assigned to the job"), terminate the **entire test binary
  process**, not just the shell — including any other tests running
  concurrently in the same binary. This rules out the co-located unit-module
  option as unsafe under normal `cargo test` execution; a dedicated
  integration test that spawns a real, separate helper *subprocess* keeps the
  job's blast radius confined to that subprocess exactly as in production.
- `ws-dashboard/crates/daemon/src/terminal.rs#L801-L852`
  (`TerminalSession::spawn`) — the daemon's real invocation shape:
  `Command::new(helper_binary).arg("terminal-helper").arg("--registry-dir")
  ...` then `terminal_platform::spawn_detached(command)`, then
  `connect_and_handshake(&socket_path, connect_timeout)`. The new test should
  mirror this (minus the detach wrapper, which is orthogonal to the reaper
  under test).
- `ws-dashboard/crates/daemon/src/terminal.rs#L373-L395`
  (`connect_and_handshake`) — retry-connect-with-deadline loop
  (`terminal_ipc_transport::connect` retried every 20ms until a timeout,
  since the helper's IPC listener bind is not synchronous with
  `Command::spawn()` returning), then a timed
  `reader.read_message::<HelperToDaemonMessage>()` for the `Handshake`. Reuse
  this idiom directly in the new test.
- `ws-dashboard/crates/daemon/src/lib.rs#L21-L26` — `terminal_helper_process`,
  `terminal_helper_protocol`, `terminal_helper_ipc`, `terminal_ipc_transport`,
  and `cli` are all `pub mod`. `run_terminal_helper` (`terminal_helper_process.rs#L172`)
  is `pub async fn`, and `TerminalHelperArgs` (`cli.rs#L31-L50`) has all-`pub`
  fields. A dedicated `tests/*.rs` integration test can therefore drive the
  real helper end-to-end using only already-`pub` items — no private-module
  access is required, so the "not a mocked handle" requirement is fully
  satisfiable from a dedicated integration test, not just from the unit-module
  option.
- `ws-dashboard/crates/daemon/src/terminal_helper_process.rs#L594-L629`
  (`spawn_process_exit_reaper`) — `#[cfg(windows)]`, detached, blocks on
  `wait_for_process_exit` then calls `shared.transition(TerminalHelperStatus::Exited)`.
  `transition` (`#L150-L160`) is a no-op unless the ring is still `Running`;
  `kill_shell_if_running` (`#L114-L140`) stamps `Terminated` *before*
  `child.kill()` specifically to make a racing reaper wake a no-op on a
  daemon-initiated kill. Because the new test's out-of-band kill never calls
  `kill_shell_if_running`/`child.kill()` (it kills the shell's real OS PID
  directly via an external tool), the ring stays `Running` until *only* the
  reaper's wake can flip it — this is exactly what makes the assertion
  non-vacuous once the reaper's `transition` call is mutated away.
- `ws-dashboard/crates/daemon/src/terminal_helper_protocol.rs#L42-L62`
  (`HelperToDaemonMessage::Handshake{pid, start_time}`) — carries the
  **helper's own** real PID, never the shell's; no message anywhere in this
  protocol exposes the shell's real OS PID. The test must discover the shell
  PID independently (Implementation Plan step 4); this mirrors how a real
  out-of-band actor (Task Manager, an OOM killer, `taskkill` run by a human)
  would identify the process, so it is not a shortcut — it is the realistic
  scenario.
- `ws-dashboard/crates/daemon/tests/terminal_lifetime.rs#L1-L130,L580-L654,L687-L843` —
  reusable precedent: `temp_fixture_path` helper, `Command::new(env!("CARGO_BIN_EXE_ws-dashboard"))`
  subprocess spawn (works because the `ws-dashboard` bin target lives inside
  the same `ws-dashboard-daemon` package, `Cargo.toml#L7-L9`, and
  `main.rs#L19` dispatches `terminal-helper` args straight into
  `run_terminal_helper`), the `HelperReaper` `Drop`-guard leak-safe cleanup
  pattern (verifies pid+startTime from the registry `.json` before
  signalling), and the readiness-marker technique used by
  `terminal_live_pty_eof_exit_flips_status_to_exited` (pins the assertion on
  the exact status string, not merely "non-running").
- `ws-dashboard/crates/daemon/src/terminal_ipc_transport.rs#L33-L50,L108-L115`
  — Windows named-pipe name is `\\.\pipe\ws-dashboard-terminal-<socket-path-file-stem>`,
  derived purely from the `--socket-path` argument's file stem; the actual
  file need not exist on Windows, only the stem matters.
- `ws-dashboard` shell selection (`terminal.rs#L71-L135`, `default_shell`,
  `select_terminal_shell`) — on Windows the shell is `powershell.exe` (if on
  PATH) or `cmd.exe` fallback; useful as a filter/sanity check when
  discovering the shell's child PID by parent-PID query (see Implementation
  Plan step 4) in case more than one child process ever shows up.
- `portable-pty` 0.8.1 source (`~/.cargo/registry/.../portable-pty-0.8.1/src/win/psuedocon.rs#L110-L154`)
  — `spawn_command` calls `CreateProcessW` directly with the pseudoconsole
  attribute; the shell is a **direct child** of the calling (helper) process
  in the Win32 process tree (no intermediary child process to account for),
  so `ParentProcessId=<helper pid>` should resolve to exactly the shell.
- `ai-docs/mental-model/ws-web-dashboard/terminal.md` (reaper contract
  paragraph, anchors `{#260516-ws-web-dashboard-terminal-websocket-transport}`
  `{#260723-terminal-attach-grace-window}`) — confirms the dual
  exit-detection path and kill-path-ordering already documented; no edit
  needed here (test-only, closeout-only spec/doc impact per the ticket).
- `ai-docs/_index.local.md#L280-L328` — validated native-Windows `cargo test`
  harness steps (fetch via the Linux filesystem path, scratch `git worktree
  add /mnt/d/<scratch-name> FETCH_HEAD`, run via `powershell.exe`, `command
  git worktree remove --force` on completion); confirmed end-to-end in under
  3 minutes for a prior Windows-only test.

## Implementation Plan
1. Create `ws-dashboard/crates/daemon/tests/terminal_windows_reaper_acceptance.rs`
   with `#![cfg(windows)]` as the file's first line — excludes the entire
   file from the Linux test binary (an established pattern for platform-only
   integration test files), simpler than annotating every item individually
   given nothing in this file is cross-platform.
2. Spawn the real helper subprocess directly (mirroring
   `terminal.rs::TerminalSession::spawn`'s arg shape, `terminal_lifetime.rs`'s
   `Command::new(env!("CARGO_BIN_EXE_ws-dashboard"))` idiom): `.arg("terminal-helper")
   .arg("--registry-dir").arg(<temp dir>) .arg("--terminal-id").arg(<generated id>)
   .arg("--work-root-id").arg(<dummy id>) .arg("--cwd").arg(<temp cwd>)
   .arg("--title").arg("windows-reaper-acceptance") .arg("--columns").arg("80")
   .arg("--rows").arg("24") .arg("--socket-path").arg(<temp socket path>)`,
   `stdin/stdout/stderr(Stdio::null())`, `kill_on_drop(true)` (tokio Command).
   This runs the exact same `spawn_shell`/`spawn_process_exit_reaper` code the
   product uses, in its own OS process — sidestepping the Job-Object
   self-assign hazard above (Codebase Findings item 1).
3. Connect to the helper's IPC transport with the same retry-with-deadline
   idiom as `connect_and_handshake` (`terminal_ipc_transport::connect`, retry
   every ~20ms up to a startup deadline), `split`, `NdjsonReader::new`. Read
   `HelperToDaemonMessage::Handshake{pid, start_time}` first (capture for
   cleanup in step 7) — no need to wait for the helper's proactive initial
   `Status{Running}` message (it reflects the ring's optimistic default, sent
   before the shell has actually spawned, so it is not a "shell is up"
   signal); then `write_ndjson(&mut write_half, &DaemonToHelperMessage::HandshakeAck)`
   to trigger `spawn_shell`.
4. Discover the real shell child PID with no new Cargo dependency: poll (a
   handful of attempts, short sleep between, small total budget e.g. ~2s)
   `std::process::Command::new("powershell.exe").args(["-NoProfile", "-Command",
   &format!("(Get-CimInstance Win32_Process -Filter \"ParentProcessId={helper_pid}\").ProcessId")])`,
   parse the returned PID. This indirection exists because no IPC message
   ever carries the shell's PID (Codebase Findings item on
   `terminal_helper_protocol.rs`) — matches how a genuine external actor
   would identify the process. If more than one child PID is ever returned,
   fail loudly with a clear message rather than guessing (portable-pty spawns
   the shell as a direct child with no intermediary, so this should not
   normally happen — see Codebase Findings).
5. Terminate the discovered shell PID **out-of-band**, entirely outside the
   crate's own kill paths: `std::process::Command::new("taskkill").args(["/F",
   "/PID", &shell_pid.to_string()]).status()`. This never calls
   `kill_shell_if_running`/`child.kill()`, so the ring is never pre-stamped
   `Terminated` — only the reaper can flip status afterward.
6. Assert the reaper wakes: keep reading NDJSON messages off the same IPC
   connection under a bounded `tokio::time::timeout` (recommend ~15s,
   consistent with the generous 5-8s drain windows already used elsewhere in
   `terminal_lifetime.rs`) until `HelperToDaemonMessage::Exit{status:
   TerminalHelperStatus::Exited, ..}` arrives (per `handle_connection`'s
   `notify.notified()` arm, a `Running -> Exited` transition is always
   reported via the `Exit` variant, never a bare `Status`). Fail on timeout or
   on any other terminal status, pinning the exact path being guarded (mirror
   `terminal_live_pty_eof_exit_flips_status_to_exited`'s `== "exited"`
   pinning rather than "non-running").
7. Cleanup via a `Drop`-guard declared before the child process/connection
   (so it runs on every exit path, including a panicking assertion),
   analogous to `HelperReaper`: best-effort verified-kill the helper process
   using the pid/start_time captured from its own `Handshake` in step 3 (a
   Windows identity-verified kill exists at
   `terminal_platform::windows::kill_verified`, but it is `pub(crate)`-only
   reachable from inside the crate — from an external `tests/*.rs` file
   either shell out to `taskkill /F /PID <helper_pid>` after a `tasklist`
   sanity check, or re-derive start-time verification inline with `powershell.exe`
   `Get-CimInstance`; do not skip identity verification given tests may run
   concurrently). Remove the temp registry/cwd dirs.
8. Non-vacuity proof (execution-time only, not committed): on the Windows
   host, temporarily neutralize the single `shared.transition(TerminalHelperStatus::Exited)`
   call inside `spawn_process_exit_reaper`
   (`terminal_helper_process.rs#L624-L629`), rebuild, rerun the new test and
   confirm it fails/times out at the step-6 deadline; revert, rebuild, rerun,
   confirm it passes. Record both outcomes (pass/fail, wall-clock) in the
   ticket's `## Result`.
9. Confirm Linux is unaffected: `cargo test -p ws-dashboard-daemon` (this
   session, Linux) must stay green with the new file compiled out; e.g.
   `cargo test -p ws-dashboard-daemon --test terminal_windows_reaper_acceptance`
   should report zero tests / a cfg-excluded target on Linux.

## Verification Plan
- Linux (this session, no Windows host needed): `cargo test -p
  ws-dashboard-daemon` stays green; the new integration-test binary reports
  zero tests under `#![cfg(windows)]` (confirms Implementation Plan step 9).
- Real Windows host (ticket's own Phase 1 execution, per
  `ai-docs/_index.local.md#L280-L328`, not performed by this survey):
  1. `git fetch /home/swkang/devenv/.worktree/ws-dashboard-dev <branch>`.
  2. `command git worktree add /mnt/d/<scratch-name> FETCH_HEAD` (detached;
     never `D:\dbg-ws-dashboard-dev`).
  3. `powershell.exe -NoProfile -Command "cd D:\<scratch-name>\ws-dashboard;
     cargo test -p ws-dashboard-daemon terminal_windows_reaper_acceptance"` —
     confirm the new test passes.
  4. Apply the Implementation Plan step 8 mutation, rerun the same filtered
     `cargo test`, confirm hang-to-timeout/failure; revert, rerun, confirm
     pass again.
  5. `command git worktree remove /mnt/d/<scratch-name> --force`.
  6. Record pass/fail evidence and wall-clock timings in the ticket's
     `## Result`.

## Escalations
- None.

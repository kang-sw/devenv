# Plan: 260725-bug-dashboard-terminal-platform-macos-unsupported — Phase 2: Native macOS runtime acceptance for the helper lifecycle

## Relevant Ticket Contract

- Exit criterion (ticket `## Phases > Phase 2`): all four lifecycle legs —
  spawn through the detached helper, daemon-restart re-adopt via
  `boot_reconcile`, identity-verified termination on explicit close, and
  dead-shell detection — pass on a real macOS host, each with a non-vacuity
  proof that its assertions can actually fail. Record the outcome under spec
  anchor `#260516-ws-web-dashboard-terminal-cross-platform-evidence`,
  including any leg that does not pass as an explicit OS-scoped limitation
  (never silently dropped, never satisfied by a Linux run standing in for
  macOS).
- "macOS PTYs are expected to deliver EOF like Linux rather than needing the
  Windows-style reaper — confirm rather than assume": the dead-shell leg must
  be an actual observed pass on this host, not an assumption carried over from
  the ticket's own prose.
- Depends on Phase 1 (`### Result (1aca7993)`, already landed): macOS
  `terminal_platform` leaf compiles and unit-verifies; `terminal_lifetime.rs`
  already runs 3/3 on macOS after two Phase-1 test-fixture fixes (`/tmp`
  socket-path base, `HelperReaper` routed through the cfg-independent
  `process_start_time` re-export instead of a Linux-only `/proc` parser).
- Non-vacuity technique to reuse: the closed Windows precedent
  (`ai-docs/tickets/.done/260724-chore-dashboard-windows-terminal-reaper-native-acceptance.md`,
  `### Result (f5891a7e)`, `## Resolution`) — mutate the exact line the
  assertion depends on, confirm FAIL/timeout, revert via `git checkout --`,
  confirm PASS again. That ticket's mutation touched
  `terminal_helper_process.rs`'s reader/reaper `transition(Exited)` call, the
  same file and pattern this phase reuses for the dead-shell leg.
- Hard boundaries from the launch prompt: Phase 2 only (no Phase 1 rework);
  never weaken/rewrite an assertion to force a pass — a failing leg is
  recorded as a limitation instead; every mutation touches production source
  temporarily and must be reverted with a verified-clean tree; any new test
  must reap what it spawns (`HelperReaper` pattern), independent of the
  already-tracked `routes.rs` leak (`260725-bug-dashboard-routes-test-terminal-helper-leak-no-reaper`).

## Out of Scope

- Phase 1's port itself (`terminal_platform.rs` macOS leaf, the
  `terminal_helper_process.rs` fail-loud identity fix) — already landed and
  verified; this phase only exercises it at runtime.
- Browser/Playwright UI-WebSocket gate: the launch prompt is explicit this
  runs "headless, via cargo. No browser, no credentials, no human." The spec
  anchor's existing "browser-facing UI/WebSocket gate" caveat stays in place
  after this phase's edit — only the process/socket-level lifecycle-legs
  sentence changes from "explicit gap" to "verified."
- `tests/routes.rs`'s ~8-helper-per-run leak (`260725-bug-dashboard-routes-test-terminal-helper-leak-no-reaper`) — do not fix; only ensure this phase's own new/reused tests do not add to it.
- `discovery.rs::canonical_or_normalized` work-root-id instability
  (`260725-bug-dashboard-workroot-id-unstable-when-path-canonicalize-fails`,
  the 2 pre-existing `routes.rs` failures) — unrelated, pre-existing, not
  touched here.
- Integration-level pid-mismatch/pid-reuse race testing — judged infeasible
  without deliberately racing OS pid allocation (see Codebase Findings); the
  existing unit-level negative case is reused instead of adding a new one.

## Codebase Findings

- `ws-dashboard/crates/daemon/tests/terminal_lifetime.rs#L166-L182` (`create_terminal`) — spawn-leg coverage today: all three tests call this, asserting `response.status() == OK` and `created["status"] == "running"`. Non-vacuity target: `unix::spawn_detached`.
- `ws-dashboard/crates/daemon/tests/terminal_lifetime.rs#L237-L420` and `#L436-L596` — the two daemon-restart tests. Both reach the adopt path in `TerminalRegistry::reconcile_entry` and assert the adopted terminal appears in `list_terminals` (`#L299-L303`, `#L493-L500`). Non-vacuity target: `terminal.rs::reconcile_entry`'s adopt arm.
- `ws-dashboard/crates/daemon/tests/terminal_lifetime.rs#L699-L855` (`terminal_live_pty_eof_exit_flips_status_to_exited`) — dead-shell leg: sends `exit` over a live WebSocket, asserts a `status == "exited"` (or `exit` with `status == "exited"`) frame arrives within 5s (`#L825-L827`). This is the "confirm rather than assume" test for macOS PTY EOF behavior; Phase 1 already ran it green (3/3 in `terminal_lifetime`), but Phase 2 must produce its own non-vacuity proof, not just cite Phase 1's pass.
- `ws-dashboard/crates/daemon/tests/terminal_lifetime.rs#L403-L415`, `#L578-L591`, `#L835-L849` — the three existing `DELETE .../terminals/{id}` call sites. Each asserts only `NO_CONTENT` (or `NO_CONTENT | NOT_FOUND` in the live-EOF test). None reads the registry identity before closing or verifies the OS process is actually gone afterward — this is the uncovered fourth leg.
- `ws-dashboard/crates/daemon/tests/terminal_lifetime.rs#L619-L666` (`HelperReaper`) — the in-repo pattern for reading a terminal's `pid`/`startTime` out of `<state_home>/terminals/<terminal_id>.json` and verifying identity through the cfg-independent `ws_dashboard_daemon::terminal_platform::process_start_time` re-export before signalling. Reuse this exact read pattern (not a hand-rolled `/proc` parse — the mental model's "Common Mistakes" section calls that out by name) for the new close-leg test's pre-close identity capture and post-close death check.
- `ws-dashboard/crates/daemon/src/terminal.rs#L784-L799` (`close_terminal`) — `DELETE` handler: removes the session from the registry map, then `.await`s `session.terminate()` before returning `NO_CONTENT`. Because this is awaited synchronously, the HTTP response does not return until `terminate()`'s full sequence (including the fallback kill attempt) has run.
- `ws-dashboard/crates/daemon/src/terminal.rs#L1053-L1071` (`TerminalSession::terminate`) — unconditionally 2-tier: writes `GracefulShutdown` over IPC, sleeps 200ms, then **always** calls `terminal_platform::kill_verified(pid, start_time)` regardless of whether the graceful path appeared to succeed.
- `ws-dashboard/crates/daemon/src/terminal_helper_process.rs#L358-L361` (`GracefulShutdown` handler) — on receipt, the helper calls `kill_shell_if_running()` (kills the shell child) and returns `Ok(false)`, which unwinds `serve_connections`'s loop and lets `run_terminal_helper` finish, self-exiting the helper process. **Risk signal**: under normal healthy load this completes well inside the 200ms window, so `terminate()`'s fallback `kill_verified` call is a structural no-op in the common case (`read_bsdinfo`/`process_start_time` already returns `None` for the exited pid → `Ok(false)`). A black-box test that only checks "process is gone after DELETE" would not distinguish "graceful path did it" from "identity-verified kill_verified path did it" — the lead's finding specifically wants the latter exercised, so the new test must force the graceful path to miss its window (see Implementation Plan step 4).
- `ws-dashboard/crates/daemon/src/terminal_platform.rs#L234-L307` (`macos::kill_verified`) — re-verifies via `proc_pidinfo`, then `kill(pid, SIGKILL)`, then does a best-effort post-kill re-read. Mutation target for the new close-leg test's non-vacuity proof: inserting an early `return Ok(false);` (e.g. right after the `pid == 0 || pid > i32::MAX as u32` guard at `#L244-L246`) makes the fallback kill a permanent no-op without touching its surrounding logic.
- `ws-dashboard/crates/daemon/src/terminal_platform.rs#L55-L71` (`unix::spawn_detached`) — shared Unix leaf (Linux + macOS). Mutation target for the spawn leg: make the `pre_exec` closure (`#L57-L66`) return an `Err` immediately, before `setsid()`/`fork()`; per `std::os::unix::process::CommandExt::pre_exec` semantics this propagates as an `Err` from `command.spawn()`, which `terminal.rs`'s spawn path (`#L845-L848`) turns into `TerminalError::BadRequest`, failing `create_terminal`'s `assert_eq!(status, OK)`.
- `ws-dashboard/crates/daemon/src/terminal.rs#L221-L269` (`reconcile_entry`) — adopt arm at `#L242-L255` ends with `self.insert_unchecked(session);` (`#L255`). Mutation target for the restart-adopt leg: commenting out that single line breaks both restart-adopt tests simultaneously (`AdoptLive` and `AdoptGrace` share this arm), since the session is verified/connected but never registered, so `list_terminals` on daemon #2 never finds it.
- `ws-dashboard/crates/daemon/src/terminal_helper_process.rs#L518-L594` (`spawn_reader_thread`) — EOF branch (`Ok(0)` at `#L524-L534`) ends with `shared.transition(TerminalHelperStatus::Exited);` at `#L592`. Mutation target for the dead-shell leg, structurally identical in shape to the Windows precedent's mutation of the sibling reaper's `transition(Exited)` call.
- `ws-dashboard/crates/daemon/src/terminal_platform.rs#L563-L616` (`platform_identity_tests`) — already `#[cfg(all(test, any(target_os = "linux", target_os = "macos")))]`-gated and already ran green on macOS in Phase 1 (part of the 124/0/2 `--lib` result). `kill_verified_refuses_to_kill_on_start_time_mismatch` (`#L598-L616`) is the existing pid-mismatch negative case, at the `kill_verified` function level. **Risk signal resolved**: reproducing a genuine pid-reuse race at the HTTP/integration level deterministically (without racing real OS pid allocation, which would be flaky by construction) is not feasible; this phase reuses the existing unit-level coverage rather than adding a new integration-level negative case, and records that judgment explicitly under the spec anchor rather than leaving the "feasible?" question open.
- `ai-docs/spec/ws-web-dashboard/index.md#L1739-L1744` — the exact sentence this phase must edit: "Live-lifecycle and browser-gate evidence (spawn, daemon-restart re-adopt, identity-verified close, dead-shell detection against a running dashboard) is an explicit gap on macOS, deferred to a later phase... `terminal_lifetime` exercises the real lifecycle at the process/socket level but not through the browser-facing UI/WebSocket gate." This phase closes the process/socket-level half (all four legs) while the browser-facing UI/WebSocket-gate half must stay recorded as still open — do not overclaim closure of the part this phase does not touch.
- `ai-docs/spec/ws-web-dashboard/index.md#L1690-L1737` — the existing Phase-1 evidence block's format (per-target pass/fail counts, exact commands, explicit non-macOS-attribution notes) is the pattern to match when appending Phase 2's evidence paragraph.

## Implementation Plan

1. Add a new integration test to `ws-dashboard/crates/daemon/tests/terminal_lifetime.rs` (not a new file — reuses `spawn_real_daemon`, `open_work_root`, `create_terminal`, `HelperReaper`'s identity-read pattern already in scope), e.g. `terminal_close_kills_verified_process_via_fallback_kill`:
   - Spawn one real daemon, open a work root, create a terminal.
   - Read `<state_home>/terminals/<terminal_id>.json` directly (mirror `HelperReaper`'s parse, not a fresh implementation) to capture `pid`/`startTime` before closing.
   - Send `SIGSTOP` to that `pid` via `std::process::Command::new("kill").arg("-STOP").arg(pid.to_string())`, then poll `ps -o state= -p <pid>` (or equivalent) until it reports the stopped state `T`, so the helper genuinely cannot service its IPC socket before the daemon's `terminate()` 200ms fallback timer fires (`terminal.rs#L1053-L1071`) — this forces the identity-verified `kill_verified` SIGKILL branch to be the one that actually terminates the process, rather than racing the graceful `GracefulShutdown` path (`terminal_helper_process.rs#L358-L361`), which would otherwise win under normal load.
   - Call `DELETE .../terminals/{terminal_id}`; assert `NO_CONTENT` (existing pattern). Because `close_terminal` `.await`s `terminate()` before responding (`terminal.rs#L784-L799`), the fallback kill has already run by the time this returns.
   - Poll (bounded, generous deadline — consistent with this file's existing "err generous" margin philosophy, e.g. up to 2-3s) `ws_dashboard_daemon::terminal_platform::process_start_time(pid)` until it is no longer `Some(start_time)` (expect `None`, since the process is orphaned to the OS's init process at this point and reaped quickly, not held as an un-reaped zombie by a still-alive parent). Assert this happens — this is the "OS process was actually killed" evidence the lead's finding asked for.
   - Cleanup: this test does not need `HelperReaper`'s reap-on-drop for the helper (it is already dead by the time the test body finishes), but keep a `HelperReaper`-style drop guard anyway for panic-safety on the poll's own deadline, consistent with the "reap what it spawns" hard boundary. Note in a code comment that the shell child is expected to receive `SIGHUP`/EOF-of-master-close when the session-leader helper dies (the same PTY-master-close guarantee `spawn_detached`'s `setsid()` already relies on per the terminal mental model — not a new risk this test introduces).
2. Run the new test plus the existing three `terminal_lifetime` tests as a green baseline: `cargo test -p ws-dashboard-daemon --test terminal_lifetime` (native aarch64-apple-darwin, no cross-compile).
3. Non-vacuity proof, spawn leg: temporarily edit `terminal_platform.rs`'s `unix::spawn_detached` `pre_exec` closure (`#L57-L66`) to `return Err(io::Error::other("mutation"));` as its first statement. Run `cargo test -p ws-dashboard-daemon --test terminal_lifetime terminal_live_pty_eof_exit_flips_status_to_exited` (single-daemon, fastest of the three to isolate). Expect failure at `create_terminal`'s `assert_eq!(response.status(), OK, "create terminal")`. Revert via `git checkout -- crates/daemon/src/terminal_platform.rs`; confirm `git status`/`git diff` clean for that file; rerun to confirm PASS.
4. Non-vacuity proof, restart-adopt leg: temporarily comment out `self.insert_unchecked(session);` at `terminal.rs#L255`. Run `cargo test -p ws-dashboard-daemon --test terminal_lifetime terminal_survives_simulated_daemon_restart_and_reattaches_by_id terminal_boot_reconcile_adopts_grace_row_and_delivers_final_output_on_reattach`. Expect both to fail at their respective "adopted/grace-adopted terminal missing from list" panics (`#L299-L302`, `#L493-L496`). Revert via `git checkout -- crates/daemon/src/terminal.rs`; confirm clean; rerun to confirm PASS.
5. Non-vacuity proof, dead-shell leg: temporarily comment out `shared.transition(TerminalHelperStatus::Exited);` at `terminal_helper_process.rs#L592`. Run `cargo test -p ws-dashboard-daemon --test terminal_lifetime terminal_live_pty_eof_exit_flips_status_to_exited`. Expect failure at `assert!(saw_exited, ...)` (`#L825-L827`) after the test's own 5s drain deadline. Revert via `git checkout -- crates/daemon/src/terminal_helper_process.rs`; confirm clean; rerun to confirm PASS.
6. Non-vacuity proof, identity-verified-close leg: temporarily insert `return Ok(false);` in `macos::kill_verified` right after the `pid == 0 || pid > i32::MAX as u32` guard (`terminal_platform.rs#L244-L246`). Run the new test from step 1 alone. Expect it to fail at the process-death poll's deadline (the SIGSTOP'd helper is never sent `SIGKILL`, so it stays alive-but-stopped). **Before reverting**, manually clean up the leftover stopped process (`kill -CONT <pid>` then `kill -KILL <pid>`, using the pid captured in the test's own failure output/panic message) so the mutation run does not leak a process. Revert via `git checkout -- crates/daemon/src/terminal_platform.rs`; confirm clean; rerun to confirm PASS and that no leftover helper/shell process remains (`pgrep -f terminal-helper` or equivalent, compared before/after).
7. After all four mutation round-trips, run `git status` and `git diff` across `ws-dashboard/crates/daemon/src/` to confirm the tree is byte-identical to its pre-mutation state (only `tests/terminal_lifetime.rs`'s new test should show as a real, intentional diff).
8. Run the full baseline once more as the final recorded pass: `cargo test -p ws-dashboard-daemon --test terminal_lifetime` (expect 4/4 now) plus `cargo build -p ws-dashboard-daemon --all-targets` (sanity that nothing else regressed).
9. Edit `ai-docs/spec/ws-web-dashboard/index.md#L1739-L1744` under `#260516-ws-web-dashboard-terminal-cross-platform-evidence`: replace the "explicit gap on macOS, deferred to a later phase" sentence with a Phase-2 evidence paragraph recording all four legs verified on native aarch64-apple-darwin via `terminal_lifetime.rs`, each with a stated non-vacuity mutation and its exact failure mode (mirroring the Phase-1 block's format at `#L1690-L1737`), the dead-shell leg specifically stated as a *confirmed* macOS PTY-EOF observation (not the Windows-style reaper), and the pid-mismatch/pid-reuse negative case recorded as covered only at the unit level (`kill_verified_refuses_to_kill_on_start_time_mismatch`) with integration-level reproduction recorded as infeasible without racing. Keep the existing browser-facing UI/WebSocket-gate caveat intact — this phase does not close that gap.
10. If any leg fails to pass and cannot be made to pass without weakening an assertion, do not "fix" it into passing — record it explicitly as an OS-scoped limitation in the same spec paragraph, naming the exact failure observed, per the hard boundary in the launch prompt.

## Verification Plan

- `cargo build -p ws-dashboard-daemon --all-targets` (native aarch64-apple-darwin).
- `cargo test -p ws-dashboard-daemon --test terminal_lifetime` — expect 4/4 passing after the new test is added (baseline before/after each mutation round-trip per Implementation Plan steps 3-6).
- Four manual mutation/revert round-trips (steps 3-6 above), each recording: exact mutated line, command run, observed FAIL mode (assertion/panic text or timeout), revert command, confirmed clean `git diff`, and re-confirmed PASS.
- `git status` / `git diff` on `ws-dashboard/crates/daemon/src/` after all mutations are reverted, to prove the tree is clean before the final commit.
- Process-hygiene check: compare live `terminal-helper` process count (`pgrep -f terminal-helper` or `ps aux | grep terminal-helper`) before the test run and after, to confirm this phase's tests reap everything they spawn (independent of the already-tracked `routes.rs` leak).
- Spec-anchor edit reviewed for accuracy against what was actually observed (no overclaiming the browser-gate leg as closed).

## Escalations

- None.

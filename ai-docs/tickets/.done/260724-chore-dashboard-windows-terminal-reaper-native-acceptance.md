---
title: "dashboard terminal: runtime-verify the #[cfg(windows)] dead-shell reaper on a real Windows host via the native cargo harness (end-of-drain acceptance)"
related:
  260724-bug-dashboard-terminal-dead-shell-undetected-steady-state: verifies
sage-review-design: skipped
sage-review-completeness: skipped
completed: 2026-07-25
---

## Context

The Windows dead-shell reaper shipped in Phase 1 of
`260724-bug-dashboard-terminal-dead-shell-undetected-steady-state` (merged at
`b07f40ad`) is currently verified only up to a **compile-and-unit ceiling on
Linux**:

- `cargo check --target x86_64-pc-windows-gnu -p ws-dashboard-daemon` confirms
  the `#[cfg(windows)]` reaper compiles.
- `mod kill_path_guard_tests` (`e8f9f603`) unit-tests the pure-state kill-path
  guard (`transition(Exited)` becomes a no-op once the ring has left `Running`;
  `kill_shell_if_running` stamps `Terminated` before `child.kill()`).

What is **not** verified: that on a real Windows host a shell process which dies
**without ConPTY signalling PTY EOF** actually wakes the reaper thread
(`WaitForSingleObject` over the `DuplicateHandle` copy of the child handle) and
drives the same `SharedState::transition(Exited)` path. That native-Windows
runtime acceptance was recorded as deferred in the dead-shell ticket's Phase 3
("blocked on a real Windows host"). It is no longer blocked: a validated Windows
dogfood harness exists (`ai-docs/_index.local.md`), so this gap is now
runtime-testable.

This is deliberately a **standalone end-of-drain acceptance ticket** rather than
folding back into the dead-shell ticket's frozen Phase 3 plan, because it needs
the native-Windows harness and is best run **after the other terminal fixes have
landed** so a single native pass can also exercise them. Sage review was
**explicitly skipped** by user directive during the goal run; consequently the
sage-settled-design pre-authorization does **not** apply to this ticket — treat
any newly surfaced binding design decision or sensitive action conservatively
and surface it rather than assuming pre-authorization.

## Unblocked (2026-07-24)

The sole blocker recorded here — the standing "surface a confirmation before
firing cargo at the Windows box" gate — is now lifted by an explicit user goal
directive: "windows dashboard is now freely disposable (user is offline),
freely test windows side dashboard for E2E test. repeat until all autonomously
proceedable tickets drained." That is the green-light this ticket's prior
Blocked note named as its unblock condition. The native-cargo scratch-worktree
harness in `ai-docs/_index.local.md` is validated and low-risk (`cargo test` on
a scratch worktree launches no daemon and needs no bind-mode/token handling).

Proceeding autonomously per the goal run: author the `#[cfg(windows)]`
live-ConPTY acceptance test, fetch this branch into a `/mnt/d/<scratch>`
worktree, run it on real Windows via `powershell.exe`, prove non-vacuity by
mutation, and record the Result. Constraints still in force (from
`_index.local.md`): scratch worktree ONLY — never the primary
`D:\dbg-ws-dashboard-dev` checkout; `command git worktree remove --force` on
completion; never touch the `:4300` production gateway daemon. Because sage
review was explicitly skipped for this ticket, any newly surfaced binding
design decision or additional sensitive action beyond this scratch-worktree
`cargo test` run is still surfaced conservatively rather than assumed
pre-authorized.

## Goal

Add a `#[cfg(windows)]` acceptance test in the daemon crate that:

1. Spawns a **real ConPTY-backed shell child** through the same helper path the
   product uses (not a mocked handle).
2. Terminates the child process **out-of-band** — kill the OS process directly so
   the PTY master does **not** observe EOF (reproducing the Windows/ConPTY
   pipe-held-open failure mode the reaper exists to cover).
3. Asserts the reaper thread wakes and the terminal transitions to `Exited`
   (surfaced over the existing WS transport / `Exit` IPC), within a bounded
   timeout — i.e. exit detection no longer hinges on PTY EOF.

Run it on real Windows via the native-cargo scratch-worktree harness documented
in `ai-docs/_index.local.md`. `cargo test` on the scratch worktree does **not**
launch a daemon and needs no bind-mode/token handling, so it is low-risk and
needs no fresh public-bind confirmation. Follow every security and worktree-hygiene
constraint recorded in `_index.local.md` (scratch worktree only, never the
primary `D:\dbg-ws-dashboard-dev` checkout; `command git worktree remove --force`
on completion; never touch the `:4300` production gateway daemon).

## Phases

### Phase 1: Native-Windows reaper acceptance test

- Author a `#[cfg(windows)]` acceptance test (co-located with the reaper in
  `ws-dashboard/crates/daemon/src/terminal_helper_process.rs`, or a dedicated
  `#[cfg(windows)]` integration test if the live-child setup is too heavy for a
  unit module) implementing the three-step scenario above.
- Fetch this branch into a `/mnt/d/<scratch>` worktree and run the test via
  `powershell.exe -NoProfile -Command "cd D:\<scratch>\ws-dashboard; cargo test
  -p ws-dashboard-daemon <filter>"`, per the proven `_index.local.md` steps.
- Prove non-vacuity: with the reaper's `transition(Exited)` wake suppressed the
  test must hang-to-timeout / fail; with it present it must pass. Record the
  mutation evidence in the Result.
- Keep the Linux build green: the new test is `#[cfg(windows)]`-gated so
  `cargo test -p ws-dashboard-daemon` on Linux is unaffected; confirm it.

### Result (f5891a7e) - 2026-07-24

Implemented and ran the `#[cfg(windows)]` acceptance test on a real Windows
host. Added
`ws-dashboard/crates/daemon/tests/terminal_windows_reaper_acceptance.rs` as a
dedicated integration test (not a co-located `#[cfg(test)] mod` inside
`terminal_helper_process.rs` — see the survey plan's Job-Object self-kill
hazard finding): it spawns the real `ws-dashboard terminal-helper` subprocess,
drives the IPC handshake so it spawns a real ConPTY-backed shell, kills that
shell's OS process out-of-band via `taskkill` (never through
`kill_shell_if_running`/`child.kill()`), and asserts the reaper wakes and
reports `Exit{status: Exited}` over IPC within a bounded 15s deadline.

**Commit range**: `3841ee6b..f5891a7e` on `impl/terminal-windows-reaper-native-acceptance`
(base `7b1faba5`):
- `3841ee6b` — initial test file.
- `8f081e38` — fix: filter the ConPTY host process out of shell-pid
  discovery (see finding below).
- `f5891a7e` — fix: borrow-checker error in the same discovery helper,
  only surfaced by the real-Windows compile (see finding below).

**Linux verification** (this session, before every Windows round-trip):
`cargo test -p ws-dashboard-daemon` stayed green throughout (124 lib + 166
routes + 15 server + 3 terminal_lifetime tests passing); the new file
reported `0 passed; 0 failed` under `#![cfg(windows)]` at every commit,
confirming the Linux build is unaffected.

**Real-Windows execution**: scratch worktree `/mnt/d/dbg-ws-reaper-acceptance`
(detached, fetched over the Linux filesystem path from this WSL worktree per
`ai-docs/_index.local.md`), built/run via
`powershell.exe -NoProfile -Command "cd D:\dbg-ws-reaper-acceptance\ws-dashboard; cargo test -p ws-dashboard-daemon --test terminal_windows_reaper_acceptance"`.
Removed with `command git worktree remove --force` on completion; the primary
`D:\dbg-ws-dashboard-dev` checkout and the `:4300` production gateway daemon
were never touched.

- **Baseline PASS** (commit `f5891a7e`): `test result: ok. 1 passed; 0
  failed; ... finished in 1.15s` (a second run after the mutation-revert
  below; the first baseline run measured `1.12s`). Wall-clock for the full
  `cargo test` invocation including a cold `cargo test --no-run` dependency
  compile was `4m 11s`; subsequent incremental reruns after small source
  edits were `2-13s` wall-clock each.
- **Non-vacuity mutation** (Implementation Plan step 8): temporarily replaced
  the single `shared.transition(TerminalHelperStatus::Exited);` line in
  `spawn_process_exit_reaper` (`terminal_helper_process.rs`) with a comment,
  as an uncommitted, local-only edit in the scratch worktree (never
  committed, reverted via `git checkout --` immediately after). Rebuilt and
  reran: `test result: FAILED. 0 passed; 1 failed; ... finished in 16.63s`,
  with the panic `windows dead-shell reaper did not wake within 15s: the
  out-of-band-killed shell's exit was never observed over IPC` — hangs to
  exactly the bounded `EXIT_WAIT_TIMEOUT` deadline as expected, confirming
  the assertion is genuinely pinned on the reaper's wake and not vacuously
  satisfied by some other path.
- **Revert + re-PASS**: reverted the mutation (`git checkout --
  crates/daemon/src/terminal_helper_process.rs`, confirmed working tree
  clean), rebuilt, reran: `test result: ok. 1 passed; 0 failed; ... finished
  in 1.15s`. Non-vacuity proof complete: PASS -> mutated FAIL/timeout ->
  reverted PASS.

**Findings surfaced during the Windows run** (both fixed in the commits
above, no reaper/product code touched):
1. **ConPTY host process is a sibling child, not an intermediary.** The
   survey plan's Codebase Finding (citing `portable-pty`'s
   `CreatePseudoConsole` call) expected the shell to be the *only* direct
   child of the helper process. On real Windows,
   `Get-CimInstance -Filter "ParentProcessId=<helper>"` legitimately
   returned **two** direct children: the real shell and `OpenConsole.exe`
   (the modern Windows Terminal ConPTY host, successor to `conhost.exe`),
   confirmed by name via a temporary uncommitted diagnostic edit to the
   scratch copy's discovery query (reverted before applying the real fix).
   Fixed by filtering discovered children to the known shell executables
   `select_terminal_shell` (`terminal.rs`) can pick
   (`pwsh.exe`/`powershell.exe`/`cmd.exe`) before asserting exactly one
   match — this is precisely the fallback the plan's own Codebase Findings
   pre-authorized ("useful as a filter/sanity check ... in case more than
   one child process ever shows up"), now confirmed necessary rather than
   theoretical.
2. **Borrow-checker error invisible on Linux.** The first fix for (1)
   introduced a real `E0505` "cannot move out of `children` because it is
   borrowed" compile error, which never surfaced on Linux because the whole
   file is `#![cfg(windows)]`-gated and Linux never type-/borrow-checks the
   gated body — only a real-Windows compile catches it. This is a durable
   ceiling on what Linux-side verification can prove for this file: it can
   only confirm the file *compiles to an empty, valid test binary*, not that
   the gated body itself is even well-typed. Recorded here as a forward
   note for anyone extending this file.

No reaper or product-code behavior change resulted from this ticket; the
reaper genuinely wakes on a real out-of-band Windows process kill, exactly as
designed.

## Spec Impact

Test-only. This ticket adds a runtime acceptance test for behavior already
shipped within the existing WS transport contract (dead-shell Phase 1); it
introduces no new observable behavior, protocol, or API surface and therefore
addresses no spec entry. Closeout-only spec impact — no spec text is added or
changed.


## Resolution (2026-07-25)

Native-Windows `#[cfg(windows)]` live-ConPTY reaper acceptance test landed (`crates/daemon/tests/terminal_windows_reaper_acceptance.rs`, Result `f5891a7e`). Ran on a real Windows host via the scratch-worktree `powershell.exe` cargo harness: baseline PASS (1.15s), non-vacuity mutation (reaper `transition(Exited)` neutralized) → FAIL/hang to the 15s deadline, revert → PASS. Linux stays green (file `#![cfg(windows)]`-gated → 0 tests). Reviews (correctness + test) clean, 3 accepted minors (all fail-loud, never false-green). Test-only; no product code touched; closeout-only spec impact.

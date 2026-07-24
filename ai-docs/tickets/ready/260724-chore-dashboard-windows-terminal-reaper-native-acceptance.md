---
title: "dashboard terminal: runtime-verify the #[cfg(windows)] dead-shell reaper on a real Windows host via the native cargo harness (end-of-drain acceptance)"
related:
  260724-bug-dashboard-terminal-dead-shell-undetected-steady-state: verifies
sage-review-design: skipped
sage-review-completeness: skipped
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

## Blocked (2026-07-24)

This end-of-drain acceptance ticket is blocked on the user's real Windows
dogfood harness. Its Phase 1 is a single coherent acceptance activity —
author a `#[cfg(windows)]` live-ConPTY acceptance test, run it on real
Windows via `powershell.exe` in a `D:\<scratch>` worktree, AND prove
non-vacuity by mutation — that is inherently runtime-iterated against real
ConPTY behavior; authoring it blind on Linux (cross-compile only) would land
an unverified test and violate evidence-before-claims, so the whole ticket
is gated together on the harness rather than split. It must NOT be advanced
autonomously: per the standing goal-run Windows-harness commitment, do not
fire cargo at the user's Windows box without explicit user confirmation;
sage review was explicitly skipped for this ticket so there is no
sage-settled pre-authorization for the native run. Unblock condition: the
user confirms/enables the Windows dogfood harness (per
`ai-docs/_index.local.md`) and green-lights the native run; then a single
session authors + runs + mutation-proves + records the Result. This note
exists so goal-drain selection skips the ticket until the user clears it.

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

## Spec Impact

Test-only. This ticket adds a runtime acceptance test for behavior already
shipped within the existing WS transport contract (dead-shell Phase 1); it
introduces no new observable behavior, protocol, or API surface and therefore
addresses no spec entry. Closeout-only spec impact — no spec text is added or
changed.

---
title: Daemon terminal_lifetime test `terminal_boot_reconcile_adopts_grace_row_and_delivers_final_output_on_reattach` is not robust to interactive-shell startup timing and fails on interactive-zsh hosts
related:
  260724-bug-dashboard-terminal-dead-shell-undetected-steady-state: surfaced during that ticket's Phase 3, but is an independent test-infrastructure bug — not caused by the dead-shell Phase 1/2/3 work
related-mental-model:
  - ws-web-dashboard
---

# Daemon terminal_lifetime test is fragile to interactive-shell startup timing

## Symptom

`cargo test -p ws-dashboard-daemon --test terminal_lifetime` reports
`test result: FAILED. 2 passed; 1 failed` on this WSL2 host. The failing test
is `terminal_boot_reconcile_adopts_grace_row_and_delivers_final_output_on_reattach`.
It asserts that the boot-reconcile-adopted grace row has status `exited`, but
observes `running` instead — it fails around the adopted-status assertion,
approximately line 479 of
`ws-dashboard/crates/daemon/tests/terminal_lifetime.rs`.

## Not a regression

The failure is pre-existing and environment-dependent, NOT introduced by the
dead-shell steady-state work:

- It fails at commit `d65d9335`, the commit that INTRODUCED the test.
- `d65d9335` predates the Phase 1 reaper (`b07f40ad`) and the kill-path
  reorder (`e8f9f603`).
- It reproduces on a pristine branch and in isolation.

So this is a longstanding environment-dependent test failure, not a
consequence of the `260724-bug-dashboard-terminal-dead-shell-undetected-steady-state`
Phase 1/2/3 changes.

## Root cause (diagnosed via captured WS frames)

The default shell on this host is interactive `zsh` with a heavy async startup
(powerlevel10k prompt, ZLE, bracketed-paste). Input delivered during the
startup window is buffered and re-processed unreliably — ZLE re-types the
buffer one keystroke at a time with prompt redraws — so an `exit` sent early
sits unexecuted for seconds.

The boot-reconcile test sends `delayed_exit_marker_command`
(`printf <marker>; sleep 1; exit`) IMMEDIATELY after terminal create, i.e.
mid-startup. Its `exit` therefore never fires, and the shell is still
`running` when daemon B boots and tries to adopt it as an already-exited grace
row. Captured WS frames confirm the input is buffered/re-typed rather than
executed within the expected window.

## Suggested fix direction (not prescriptive)

- Apply the same readiness-before-input treatment the sibling test
  `terminal_live_pty_eof_exit_flips_status_to_exited` (commit `e2990574`) now
  uses: wait for the shell's startup-output burst to settle into a quiet gap
  before sending input.
- Additionally, the test's marker poll appears to match the command ECHO
  rather than real execution output; tighten it so it waits for actual command
  execution.

Changing an existing test's timing CONTRACT is an "ask first" change, so this
is captured as an idea ticket rather than fixed inline.

## Impact

Makes the daemon test suite RED on interactive-shell dev hosts, obscuring real
regressions — a CI robustness / dogfoodability concern. Related area:
`260724-bug-dashboard-terminal-dead-shell-undetected-steady-state` (this
surfaced during its Phase 3), but this is an independent test-infrastructure
bug.

---
title: hop-1 default-spawn env regression guard is fragile and platform-partial
related:
  260725-feat-dashboard-pty-agent-attention-notification: found-during
---

# hop-1 default-spawn env regression guard is fragile and platform-partial

## Background

`ws-dashboard/crates/daemon/src/terminal.rs`, test
`helper_spawn_default_no_command_matches_existing_arg_shape` (around :2064),
asserts on the `Debug` rendering of `std::process::Command` to detect an
accidental `env_clear()` on the default (no-argv) spawn path — the daemon's
hop-1 spawn of the helper process. It needs this indirection because
`Command::get_envs()` cannot distinguish "no env manipulation happened" from
"`env_clear()` was called, then everything was re-added" on any platform;
only the `Debug` string encodes the clear flag at all (rendered as `env -i`
on Unix).

This is debt for two reasons:

1. It couples a test to `std::process::Command`'s `Debug` format, which is
   unstable and undocumented. A std change could break the assertion loudly
   (acceptable), or — worse — reshape the format so the assertion keeps
   passing while silently no longer detecting the regression it exists for.
2. It is `#[cfg(unix)]`-gated, because only Unix's `Debug` impl encodes the
   clear flag as `env -i`. Windows has **no coverage** for this regression at
   hop 1.

Why it matters: the regression this guard exists to catch is that ordinary
shell terminals — the most-used path in the product — would spawn with no
inherited environment at all (no `PATH`, no `HOME`), because hop 1 sits
upstream of every terminal spawn, not just agent-profile ones.

This was surfaced by the Phase 1 review of
`260725-feat-dashboard-pty-agent-attention-notification` (the phase that
introduced explicit argv/env passthrough at both spawn hops) and was honestly
scoped by the implementer rather than hidden — this ticket records the
limitation so it is not later read as an oversight.

## Possible directions (not decided here)

- Make the env decision an observable value the test can assert on directly
  (e.g. an explicit "did this call clear the base env" flag threaded through
  the construction path) instead of inferring it from a `Debug` string.
- Cover it with a real spawn-and-inspect integration test that spawns a
  process and checks its actual environment, rather than static-inspecting
  the unexecuted `Command`.
- Accept the `Debug`-string fragility as a known trade-off and add a
  std-version canary test that fails loudly if the format the assertion
  depends on ever changes, so silent breakage is at least converted to loud
  breakage.

Any of these should also close the Windows coverage gap, not just the
fragility.

---
title: "git_exec reader-cap test is load-sensitive: green alone, red under a loaded full-suite run"
---

## Symptom

`git_exec::tests::capture_refuses_to_spawn_once_the_outstanding_reader_cap_is_reached`
fails during a full `cargo test` on a loaded machine and passes when run alone.

The test wedges git readers until the outstanding-reader cap refuses the next
spawn, and asserts the cap figure (32). Under full-suite load it observes 28-30 -
not all the readers it launched have reached the wedged state by the time it
asserts.

## Finding

Observed while landing Phase 1 of
`260729-bug-dashboard-agent-profile-gc-destroys-concurrent-daemon-state`, and
proven unrelated to it three ways:

- the implementing commits touch no line of `git_exec.rs`;
- `cargo test --lib capture_refuses_to_spawn` passes (`1 passed; 380 filtered
  out`);
- it fails identically on the pre-change commit with those changes checked away.

So this is pre-existing and load-triggered, not a regression. It became visible
because that work added ~25 tests, several of which spawn real helper processes,
raising steady-state load during the lib run.

## Why it is worth a ticket

It will be misread. The failure surfaces in the middle of an unrelated change's
verification run, names a cap that looks like a real invariant, and is red only
in the configuration people actually run. The next person to hit it either
chases it into the wrong subsystem or learns to tolerate a red suite - both are
worse than the flake.

## Directions worth considering

- Wait for the wedged state rather than asserting on a count sampled at an
  arbitrary moment - most likely the real fix, since the test's subject is the
  refusal, not the arrival rate.
- Assert the refusal behaviour without pinning the exact number of readers
  observed in flight.
- If the count genuinely is the property under test, make the spawn barrier
  explicit instead of timing-dependent.

Reducing the cap or adding a sleep would hide it rather than fix it.

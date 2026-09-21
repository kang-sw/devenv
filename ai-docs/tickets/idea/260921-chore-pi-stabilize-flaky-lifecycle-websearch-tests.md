---
title: Stabilize flaky pi real-process and timing tests
---

# Stabilize flaky pi real-process and timing tests

## Background

During the 0.46.15 ship-gate review (`59140d29..10150d41`), the
`agents-plugin-pi` test suite showed two load-sensitive flaky failures across
repeated full-suite runs, each passing in isolation and on a clean rerun:

- `agents-plugin-pi/test/claude-lifecycle.test.ts` — a real-process cleanup
  confirmation that is timing/scheduling sensitive under load.
- `agents-plugin-pi/test/web-search.test.ts` — an assertion that an operation
  completes in `<3000ms`, i.e. a wall-clock threshold that a loaded CI/dev host
  can miss.

These are pre-existing (not introduced by the reviewed range) and did not block
the ship: the suite reaches 1621/1623 pass (2 pre-existing skips) on a clean run,
and the blocker that gate actually fixed was the stale bundled-tool contract in
`bridge.test.ts`. Captured as non-blocking observation B during that audit.

## Background — why fix

Load-sensitive flakes erode the ship gate's signal: a green suite is the release
gate's evidence, and intermittent red forces reruns and desensitizes readers to
real failures.

## Phases

### Phase 1: Remove the wall-clock and real-process race sensitivity

For `web-search.test.ts`, replace the absolute `<3000ms` timing assertion with a
deterministic signal (assert completion/ordering, or a fake clock), not a tighter
real-time bound. For `claude-lifecycle.test.ts`, make the cleanup-confirmation
wait on the actual lifecycle event/condition rather than a scheduling-dependent
window. Verify by running the full `agents-plugin-pi` suite repeatedly (e.g. a
handful of back-to-back runs) with zero flaky failures; do not merely rerun until
green. Do not delete or skip the tests to hide the flake.

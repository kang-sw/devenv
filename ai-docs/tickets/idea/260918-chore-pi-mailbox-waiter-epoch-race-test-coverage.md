---
title: pi mailbox waiter epoch-race guard has no regression test (lifecycle harness missing)
related:
  260917-feat-ws-pi-mailbox-waiter-slug-wake: origin — the guard this ticket wants to test
---

# pi mailbox waiter epoch-race guard has no regression test (lifecycle harness missing)

## Background

Captured from the `v0.46.11` ship-gate review sweep (range
`4c5c2d43..develop`, Test partition, Important). Code was accepted for release
by explicit user override with this follow-up; correctness was independently
verified, so this is a **regression-protection gap, not a live defect**.

`260917-feat-ws-pi-mailbox-waiter-slug-wake` added an `await
resolveMailboxSelfSlug(...)` before `mailboxWaiterHandle` is assigned at the pi
`session_start` arm site. That await opened a window where a rapid double
`session_start` (`/reload`) — or a `session_start` racing a `session_shutdown` —
could leak a waiter subprocess or leave two live waiters double-admitting mail.
The fix (commit `b75c4075`) added a monotonic `mailboxWaiterEpoch` counter,
bumped at every reset site (arm site, `session_shutdown`); the post-await
`armEpoch === mailboxWaiterEpoch` check drops a stale arm attempt.

The guard's logic was verified correct by the ship review's Correctness
partition, but **no test exercises it**, and the harness needed to write one
does not exist yet: no test in `agents-plugin-pi/test/*.ts` constructs a fake pi
`ExtensionAPI` and calls `wsPiBridgeExtension()` to drive `session_start` /
`session_shutdown` directly. So a future edit that reorders the epoch-bump vs
the await could silently regress the fix with nothing in CI to catch it; today
only a live two-`/reload` dogfood run would surface it, and that manual check
was recorded as not performed.

## Suggested scope

- Stand up a minimal pi `ExtensionAPI` fake + `wsPiBridgeExtension()` lifecycle
  test harness (new capability; reusable for other lifecycle races, e.g. the
  ones tracked in `260908-research-ws-pi-lifecycle-race-monitoring`).
- Cover: (a) two overlapping arm attempts where the first's
  `resolveMailboxSelfSlug` await resolves *after* a second `session_start` has
  re-armed — assert the first attempt neither clobbers the second nor leaks its
  subprocess; (b) an arm attempt in flight when `session_shutdown` fires —
  assert no waiter is assigned afterward.

## Notes

- Minor sibling gap from the same review (separate, pre-acknowledged, likely
  not worth its own ticket): `buildMailboxWaitRearmCommand`'s `timeout <= 0`
  branch (omits `--timeout`) is untested because a `--timeout 0` re-arm would
  hang the test; deferred deliberately in `260917-feat-mailbox-wait-rearm-...`
  Phase 1 Result.
- Fit-partition awareness item, not a defect: the re-arm command string is
  built independently in `cmd/ws-mcp/mailbox.go` and `internal/mcp/playbook_tools.go`
  with no shared helper; both are test-covered but could drift. A future
  consolidation into `internal/wsmailbox` would remove the hand-sync.

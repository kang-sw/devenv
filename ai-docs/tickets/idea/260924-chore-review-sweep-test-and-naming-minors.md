---
title: Review-sweep test and naming minors (Pi usage rollup, worktree.list, config isolation)
related:
  260924-bug-ticket-index-read-timeout-below-ssh-roundtrip: the same sweep's release-blocking index fixes; these minors were left out as non-blocking
---

# Review-sweep test and naming minors (Pi usage rollup, worktree.list, config isolation)

## Background

Non-blocking findings from the develop review sweep 0a361491..4a2f4721 that
fall outside the index fix ticket:

- **Pi usage rollup tests.**
  - `agents-plugin-pi/test/agent-usage-rollup.test.ts`, the "telemetry reset
    on a revived record" test, deletes `revived.descendantUsage` before
    refreshing. It never exercises the `descendantUsageOrder` gate its title
    names and passes with the gate removed (mutation-confirmed). The gate is
    covered by later tests in the same file. Either seed an older in-memory
    value or retitle.
  - `agent-usage-rollup.integration.test.ts`, the "parent restart" test, now
    calls `restoreDescendantUsage` itself, so it no longer shows the
    production revival path restores the value (unit tests cover it).
- **worktree.list.** The "skip prunable entries" rule, the empty-pool
  "worktrees: none" output, and the lease-read-failure warning have no test.
- **Config isolation.** `TestWorktreeListIncludesLegacyInTreePoolUnderDefaultConfig`
  and the `git_merge_test.go` `legacy-pool-holder` dispatch depend on the
  default `worktree_pool` without isolating `WS_CONFIG_HOME`; a host
  `~/.ws/config.json` pool override would break them (same class as the two
  known agent-config tests that need a clean `HOME`).
- **Effort text test.** `server_test.go`'s `config.resolve_agent` case builds
  its expected text with `formatEffortForText`, the helper under test, with
  no independent "no injected line" assertion.
- **F3 naming.** `ChildApprovalGate.EARLY_DECISION_CAP`
  (`agents-plugin-pi/src/approval-protocol.ts`) now also caps the `consumed`
  list; the name says early decisions only.

## Open questions

- Whether the two known HOME-dependent agent-config tests and the
  `WS_CONFIG_HOME` leak above share one fix (a test-wide config isolation
  helper) worth doing together.

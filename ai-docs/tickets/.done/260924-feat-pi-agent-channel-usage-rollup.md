---
title: Roll up cumulative descendant usage over the Pi parent-child control channel
blocked-by: 260924-feat-pi-agent-channel-transport
related:
  260923-research-pi-parent-child-loopback-control-channel: source research; its Outcome Ledger is this ticket's authority
  260924-feat-pi-agent-channel-transport: prerequisite transport
  260924-bug-pi-retention-cross-owner-checkpoint-fold: blocked by this ticket; consumes its fold rule and checkpoint schema
  260924-bug-pi-durable-write-hygiene: also edits the refreshAgentTelemetry ownership write; whichever lands second includes the descendant-usage field in the change comparison
  260913-bug-ws-pi-cost-footer-cpu-saturation: prior recursive 250-ms polling regression this must not reintroduce
  260912-feat-ws-pi-custom-footer-cost-telemetry: introduced footer cost and the checkpoint
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: eb0729af858637e4
sage-review-completeness-reviewed: eb0729af858637e4
completed: 2026-09-24
---

# Roll up cumulative descendant usage over the Pi parent-child control channel

## Background

The parent's `refreshAgentTelemetry` re-reads and parses its direct child's whole `session.jsonl` on each refresh (`agents-plugin-pi/src/spawner.ts`, `src/agent-telemetry.ts`). The footer counts only direct child records, and it arms only in a TUI lead or fork (`shouldArmAgentFooter`, `agents-plugin-pi/src/agent-footer.ts`). Grandchild and deeper usage therefore never reaches the root footer. An earlier recursive design with 250-ms polling saturated the CPU.

## Decisions

- The channel carries only each hop's **descendant** usage: the total below the hop, excluding the hop's own usage. Deltas are not used, because they break replay safety.
  - "Usage" means the token and cost fields of the existing telemetry that the footer renders.
  - A child's own usage stays where it is today. The parent keeps its existing reduction of the direct child's `session.jsonl` in `refreshAgentTelemetry` (`agents-plugin-pi/src/spawner.ts`), with its current cadence, prefix anchor, and context-token fields. That reduction already excludes a fork's inherited parent-history prefix (17dea8a3), and the widget, audit, sidecar, and fork-resume records keep reading its context fields unchanged.
  - The subtree total of a direct child is therefore its own usage from that reduction plus its last reported descendant value. No hop ever reads a session other than its direct children's.
- A hop's descendant value has two parts:
  - the subtree totals of its registry-resident direct children, dormant ones included;
  - its owner checkpoint's evicted baseline.

  After a restart, each hop recomputes this sum from its children's ownership records and its checkpoint. Restarts must not double count.
- Ordering.
  - Each descendant-usage message carries the launch generation from `260924-feat-pi-agent-channel-transport` (the record's `launchGeneration`) and a per-generation sequence number.
  - Within a generation, the parent keeps the highest sequence and ignores older messages, so a late stale value can neither inflate nor lower the total.
  - When the parent accepts a new generation, the first value from that generation replaces the stored value. A message from an older generation that arrives after that is ignored.
- A hop sends its descendant value whenever the computed value changes. It evaluates the value at two points: when the hop's own `refreshAgentTelemetry` reduction of a direct child runs, and when a direct child's descendant report arrives. The hop's own turns play no part, so a hop that is parked waiting on its children still forwards their usage. Sends are coalesced at those events and never happen per streamed delta. After a reconnect, the latest value goes in the resume section of the transport's hello.
- The parent stores the reported descendant value in the child's ownership telemetry, next to the existing own-usage fields. The parent is already the single writer of that record. A child that exits without a final report keeps its last stored descendant value; the own-usage reduction still runs at exit as it does today.
- The root footer shows the whole subtree's usage. Intermediate hops run in RPC mode without a footer, but they still compute and forward their descendant value.
- No recursive JSONL scanning or polling is reintroduced.
- Eviction fold rule. It applies at every hop that owns children, whether or not the hop renders a footer. Eviction runs at every hop (`evictForCapacity`), and the hop's owner checkpoint already holds an evicted baseline (`persistEvictedAgentCost`).
  - A hop counts only its direct children's subtree totals.
  - Evicting a direct child adds that child's subtree total to the hop's evicted baseline: its own usage, as `persistOwnedTelemetryRollup` folds today, plus its last stored descendant value.
  - A fold happens at most once per actual removal of a child home. Tie the fold to the removal rather than to an unbounded set of already-folded IDs, so the checkpoint stays bounded to the live registry identities (2f833ec8).
  - The worker chooses the checkpoint schema that realizes this rule and records it in the Result.
- If `260924-bug-pi-durable-write-hygiene` lands first, its write-on-change comparison against the persisted record must include the new descendant field.
- `260924-bug-pi-retention-cross-owner-checkpoint-fold` consumes this rule and its schema. If the fold requires a checkpoint schema change that affects that ticket's cross-owner writer, record the change in the Result.

## Prior Decisions

- 260913-bug-ws-pi-cost-footer-cpu-saturation (2026-09-13, commit 2f833ec8): "Exact lifetime accounting and recursive nested-child traversal were rejected in favor of the ticket's bounded, monotonic cosmetic estimate." — bearing: constrains
- 260912-feat-ws-pi-custom-footer-cost-telemetry (2026-09-13, commit a075db31): "Child telemetry is persisted in ownership metadata and identity-keyed owner roll-ups so eviction, retention, reload, and retry preserve totals without double counting." — bearing: supports
- 260923-research-pi-parent-child-loopback-control-channel (2026-09-23, Confirmed Decisions): "Scope. The following move to the channel: - execute-approval decisions (A5); - the whole subtree snapshot (A6) ...; - descendant usage (A9), both live and durable cumulative" — bearing: supports
- 260924-bug-pi-retention-cross-owner-checkpoint-fold (2026-09-24, Decisions): "The fix follows the usage-rollup eviction fold rule: a fold happens at most once per actual removal of a child home." — bearing: constrains
- 260924-feat-pi-agent-channel-subtree-state (2026-09-24, Decisions): "The child sends full revisioned snapshots, and the parent keeps the last revision it has seen." — bearing: supports
- 260924-bug-pi-durable-write-hygiene (2026-09-24, Decisions): "`ownership.json` is written only when its content changes. ... `refreshAgentTelemetry` writes only when the telemetry differs from the persisted record." — bearing: constrains
- 260924-research-pi-root-single-authority-durable-state (2026-09-24, Confirmed Decisions): "This question is investigated separately from the parent-child channel research, not inside its scope." — bearing: constrains
- 17dea8a3 (2026-09-10, commit): "Use durable SDK session-entry IDs and an immutable pre-first-prompt prefix anchor because responseId is optional and streaming usage is cumulative." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/agent-telemetry.ts, agents-plugin-pi/src/agent-footer.ts, agents-plugin-pi/src/agent-storage.ts, plus the channel protocol module pending 260924-feat-pi-agent-channel-transport |
| scope.surface | cross-module | spawner refreshAgentTelemetry, footer CostCheckpoint and persistOwnedTelemetryRollup, ownership telemetry in agent-storage, and the channel protocol all change together |
| scope.new_public_symbol | yes | a descendant-usage channel message type, kept separate from approval and subtree messages per the research Non-authoritative notes |
| scope.new_type_contract | yes | AgentTelemetry gains a reported descendant-usage value stored in ownership.json, and the checkpoint evictedBaseline fold may change the CostCheckpoint schema |
| scope.test_surface | existing | agents-plugin-pi/test/agent-footer.test.ts, agent-telemetry.test.ts, agent-telemetry-contract.test.ts, agent-telemetry-lifecycle.test.ts; a three-level tree case likely adds new cases |
| complexity.reuse_points | unconfirmed | existing CostCheckpoint evictedBaseline and updateOwnership telemetry path read in the tree; the channel transport is not landed yet |
| complexity.side_effect_risk | high | changes durable ownership telemetry and checkpoint state and touches the footer path that previously saturated the CPU |
| risk.correctness | high | last-wins descendant values must stay replay safe across parent and child restarts, reordering, and a subtree eviction folded exactly once |
| risk.fit | moderate | depends on the unlanded transport and must leave a fold design that 260924-bug-pi-retention-cross-owner-checkpoint-fold consumes |
| risk.test | high | needs multi-process three-level tree, restart, and reorder coverage; prior round-one review found tests that did not fail on evicted-baseline restore |
| risk.security_or_contract | moderate | durable ownership telemetry and checkpoint schema are read by retention and by another ticket's cross-owner writer |

## Phases

### Phase 1: Cumulative per-hop usage roll-up

Add the descendant-usage message, which carries a generation and a sequence number. Store it in the child's ownership telemetry, and include the evicted baseline at every hop. Leave the direct-child own-usage reduction in place. The tree and restart cases below are multi-process tests.

Verification:

- A three-level tree (root → child → grandchild) shows grandchild usage in the root footer, while the intermediate child is parked waiting on the grandchild and takes no turn of its own.
- Parent restart, child restart, and resume each preserve totals without double counting.
- Duplicate or reordered usage messages neither inflate nor lower totals. That includes a message from the previous generation arriving after the new generation's first value.
- An intermediate hop evicts its child that has descendants and then restarts. The root total is unchanged, because the fold happened exactly once, in that hop's evicted baseline.
- A dormant direct child's subtree total stays in its parent's sum.
- A fork hop's own usage still excludes its inherited parent-history prefix, and the widget and audit still show each direct child's context tokens.
- No hop reads any session other than its direct children's. Unchanged values and streamed deltas send no usage message.
- These tests fail when the fold or the checkpoint restore is removed.

### Result (19e57c98) - 2026-09-24

Landed in `aba83809..19e57c98` on `impl/develop/wrath-slam-never`.

**What landed**

- New module `agents-plugin-pi/src/agent-usage-rollup.ts`.
  - **Message.** `descendant-usage`, with the payload `{t, gen, seq, usage}`. `usage` is a `CumulativeCost` (`knownUsd`, `knownContributors`, `unknownContributors`, `descendants`), the footer's own unit; tokens are not rolled up.
  - **Parent side** (`acceptDescendantUsage`, `attachDescendantUsage`):
    - it keeps the highest seq within a launch generation;
    - the first value from a newer generation replaces the stored one;
    - a report from an older generation is ignored;
    - it drops a report whose channel generation is not the record's current launch.
    - The accepted value is stored on the record and in the child's ownership telemetry as `telemetry.descendantUsage`. The ordering state is live-only, because launch generations restart with the parent process.
  - **Child side** (`createDescendantUsageReporter`): one reporter per channel, so seq stays monotonic across session replacements. It sends only when the value changes; a zero value never sends. Its latest report rides the reconnect hello's `resume.descendantUsage` and is sent again once after the reconnect completes; the seq check drops the duplicate.
  - **Evaluation points:**
    - after the hop's own reduction of a direct child (`refreshAgentTelemetry`);
    - after a direct child's report changes its stored value;
    - once at `session_start` after revival.
    - Streamed deltas never reach an evaluation point.
- **Per-hop value** (`descendantUsageValue`, `agent-footer.ts`): the subtree totals of the registry-resident direct children, dormant ones included, plus the owner checkpoint's evicted baseline. A subtree total is own usage plus the last stored descendant value. Every hop that owns children has a cost estimate (`costEstimateFor`), footer or not, and the footer reuses it. The direct-child own-usage reduction is unchanged, so no hop reads any session other than its direct children's.
- **Channel.** New `ChildChannel.provideResume(key, provider)` for feature resume state; `readiness` is a reserved key.
- **Fold rule.** A fold happens once per actual removal:
  - capacity eviction (`foldAndPersist`);
  - retention (`persistOwnedTelemetryRollup`).
  - Each folds the whole subtree: own plus the stored descendant value. `persist`/`reconcile` now drop, not fold, identities that have left the registry.
  - Sidecar revival (`parseOrphans` and `reviveOrphans`) skips an owned entry whose home no longer exists. Its removal already folded it; without the skip, a non-root hop would have counted it twice after the root lead's cross-owner retention.

**Checkpoint schema (for 260924-bug-pi-retention-cross-owner-checkpoint-fold)**

- The file is unchanged at `ws-agents/<owner>/.cost-estimate/checkpoint.json`, `version: 1`, `{lead, evictedBaseline, agents[]}`.
- `agents[].cost` now holds each direct child's **subtree** total, not own-only usage.
- `evictedBaseline` accumulates whole removed subtrees.
- Nothing folds at persist time. The only writers that fold are eviction, and retention's `beforeRemove`. Retention's read-modify-write still takes no lock; that race remains that ticket's.
- The durable descendant value per child is `ownership.json` `telemetry.descendantUsage`, written only by the parent.

**Verification**

- Test files:
  - `test/agent-usage-rollup.test.ts` (unit, 16 cases);
  - `test/agent-usage-rollup.integration.test.ts` (multi-process, 8 cases);
  - `test/fixtures/usage-hop.ts` (hop process);
  - spawner wiring cases in `test/agent-channel-launch.test.ts`;
  - a real-Pi `session_start` revival-and-report case in `test/agent-channel.integration.test.ts`;
  - a footer-reuse case in `test/agent-footer.test.ts`.
- Mapping to the verification list:
  - **Three-level tree:** the root footer shows the grandchild's and great-grandchild's usage only through the channel while M is parked.
  - **Restarts and resume:** child restart on a new generation, parent restart rebuilt from ownership telemetry, and resume through the reconnect hello each keep the total, with no double count.
  - **Ordering:** duplicate, reordered and late previous-generation reports are unit-tested, and the generation replacement is also covered over the wire.
  - **Eviction then restart:** the checkpoint `evictedBaseline` equals G own plus L, exactly once, and the root total is unchanged.
  - **Dormant child:** it stays in the sum.
  - **Fork hop:** its own usage still excludes the inherited prefix and keeps `contextTokens`.
  - **Session reads:** hop-recorded read paths show only direct children's sessions, including across a restart.
  - **Send suppression:** unchanged values send nothing.
- Mutations, each confirmed failing and then reverted:
  - fold own-only in `foldAndPersist` (unit and integration);
  - `loadCheckpoint` returning empty (unit and integration);
  - removing either spawner `attachDescendantUsage` call;
  - removing `evaluateDescendantUsage` from the change handler;
  - removing the `index.ts` reporter install, `setSource`, or the `session_start` evaluate;
  - removing the telemetry-reset restore;
  - removing the `parseOrphans` removed-home skip;
  - removing the `reviveOrphans` gate;
  - removing the reconnect resend.
- Full `npm test`: 1731 tests, 1729 pass, 0 fail, 2 skipped.
  - Two known unrelated flakes appeared intermittently in earlier runs: `recursive-worker.test.ts` "channel notifications propagate a nested parent edge through two process hops", and "iterator abort-result cannot mutate settled output". Both fail on the baseline too, and both pass on rerun.
- The multi-process middle hops are fixture processes that run the production modules, channel, sidecar and checkpoint paths, not real Pi processes. A real Pi child cannot be made to spawn a grandchild without an LLM turn. The real-Pi integration case covers the `index.ts` wiring separately.

**Known edges (accepted)**

- A child with no own-usage telemetry keeps its descendant value in memory only.
- While a child's telemetry is reset (for example after a session change), the durable descendant copy is absent until the next refresh recreates telemetry.
- A crash that loses the sidecar under-counts.
- Final reports sent during a child's shutdown are lost when the channel closes.
- A `reload` re-imports the extension with a fresh reporter ref and no bootstrap, so reporting stops for that launch. RPC children have no reload command.
- A lead-less hop's checkpoint stores a zero lead with `found: true`. The footer would show `$0.00`, not unknown, only if that session is later mounted with a footer. This was already possible after any eviction at a hop.
- `agents-plugin-pi/` is not listed in AGENTS.md's Implementation Conventions table for `shipped-surface-boundary.md`, although its `src/` ships.

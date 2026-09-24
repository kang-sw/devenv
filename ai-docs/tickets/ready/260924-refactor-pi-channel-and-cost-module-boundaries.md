---
title: Pi channel resume registration and cost-module boundaries cleanup
related:
  260924-bug-pi-review-sweep-correctness-fixes: behavior fixes from the same sweep; land first, this ticket must not change behavior
  260924-feat-pi-agent-channel-usage-rollup: introduced provideResume and grew agent-footer.ts
  260924-feat-pi-agent-channel-approval-decisions: placed the approval parent half in spawner.ts
  260924-bug-pi-retention-cross-owner-checkpoint-fold: eviction-record values now living in agent-footer.ts
  260924-bug-pi-removal-claim-abandonment-and-generation: forwarded removal-gate behavior fixes split out of this ticket
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 40369659a50ff695
sage-review-completeness-reviewed: 40369659a50ff695
---

# Pi channel resume registration and cost-module boundaries cleanup

## Background

The release-gate review sweep of develop `a976a2b1..0a361491` found structural
drift across the Pi channel tickets. Each ticket was locally reasonable; together
they left duplicated mechanisms and modules whose role no longer matches their
name (AGENTS.md Code Standard 3). None of this is a behavior defect; it is
scheduled after `260924-bug-pi-review-sweep-correctness-fixes`.

Findings (fit reviewer, file:line at 0a361491):

- **Two hello-resume registration paths.** Subtree and approval state enter the
  hello through `ChildChannel.connect(..., { resume })`, composed by hand in
  `index.ts:408`; usage enters through `ChildChannel.provideResume(key,
  provider)` (`agent-channel.ts:673-677`, used at `agent-usage-rollup.ts:112`).
  They merge at `agent-channel.ts:735` with different guarantees: the reserved
  `readiness` key guard and per-provider try/catch isolation apply only to
  `provideResume`, and a key collision is settled silently by spread order.
  Wiring also differs (explicit `sessionCtx` threading versus the module-global
  `descendantUsageReporterRef`). Doc comments at `index.ts:404-407` and
  `agent-channel.ts:593` no longer describe what a hello carries.
- **agent-footer.ts owns more than the footer.** It holds every hop's cost
  estimate, eviction-record values (`capacityEvictionCost`,
  `retentionEvictionCost`, `persistEvictedAgentCost`), the removed-agent gate
  `isRemovedAgent` (imported by spawner.ts and ask.ts), and
  `descendantUsageValue`; its `reconcile` deletes registry entries and stops
  their observers (`:173-189`), so a value read mutates registry lifecycle.
- **Three "is this owned child removed" predicates.** Sidecar
  (`!existsSync(home) || hasEvictionRecord`, `agent-sidecar.ts:263`/`:395`),
  footer (`isRemovedAgent`, record only, `agent-footer.ts:367`), storage
  (`isOwnedHomeGone`, claim-aware, `agent-storage.ts:289`). The correctness
  ticket makes the sidecar claim-aware. (Decisions below keep three named
  predicates rather than unifying them.)
- **Approval's parent half lives in spawner.ts** (`attachApprovalChannel` and
  `ApprovalChannelHost`, around `:2688`), while subtree and usage keep both halves
  in their own modules.
- Small items: `WEB_HOME_ENV` defined in `web-readiness.ts:12` but not used
  there; it is read by `web-tools.ts:29` and set by `spawner.ts:2440`
  (`git grep WEB_HOME_ENV 0a361491 -- agents-plugin-pi/src`); leftover alias `export type PendingApproval =
  PendingApprovalState` (`execute-gateway.ts:273`); missing space at
  `agent-sidecar.ts:264`.

## Decisions

Settled at promotion against develop 04e95b4e (after 99f8ac90). Line numbers
below are at that commit.

- **Resume registration: keep both entry points, one documented composition.**
  `ChildChannelOptions.resume` stays the source for state that exists at
  connect time (it is the only source in the first hello); `provideResume`
  stays the entry for features that register after `connect()` resolves.
  The merge in `awaitWelcome` (`agent-channel.ts:729-735`) becomes one private
  helper with the precedence written in its doc comment, unchanged: constructor
  keys, then provider keys in registration order, then the channel's own
  `readiness` on a reconnect. Correct the stale comments
  (`agent-channel.ts:593` says "every reconnect hello" but the option runs for
  every hello; `index.ts:406-407` says the same and omits the usage key).
  `approval-protocol.ts:13-14` ("in every hello; acted on for reconnect
  hellos") matches the code and stays unchanged. The wiring in
  `index.ts:408` does not thread `sessionCtx`; the background's "sessionCtx
  threading" wording is inaccurate and needs no change.
  - Rejected: moving subtree and approval onto `provideResume`. Registration is
    only possible after `connect()` resolves (`agent-channel.ts:620-626`), so the
    first hello would lose `approval: {consumed: []}` (a wire change, although
    the parent ignores first hellos, `spawner.ts:2747`); and `options.resume`
    still could not be removed, because `agent-channel.test.ts:125,153` and
    `execute-gateway.test.ts:536-545` use it directly.
  - Rejected: retiring `provideResume`. `agent-usage-rollup.test.ts:170-218`
    calls it directly and relies on the reporter registering itself on a bare
    channel, so the constraint below would be broken.
  - Rejected: new collision or reserved-key enforcement on the constructor
    source. It changes behavior only for inputs no caller produces, and the
    constraint below forbids behavior change.
- **Cost accounting leaves agent-footer.ts.** A new module
  `agent-cost.ts` takes the cost estimation and checkpoint state
  (`CostEstimateState`, `registryStorage`, `registryEstimates`,
  `costEstimateFor`, `loadCheckpoint`, `registerAgentCostOwner`,
  `persistAgentCostCheckpoint`), the eviction-record values
  (`capacityEvictionCost`, `persistEvictedAgentCost`,
  `retentionEvictionCost`), `descendantUsageValue`, and `isRemovedAgent`
  (which reads `registryStorage`, so it stays with that state).
  `agent-footer.ts` keeps rendering, the controller, and the session
  lifecycle, importing what the controller needs from `agent-cost.ts`.
  - **Import direction is one-way: `agent-footer.ts` -> `agent-cost.ts`.**
    `CostEstimateState.presentation()` calls `formatCumulativeCost`
    (`agent-footer.ts:246-252`), so `formatCumulativeCost` moves next to
    `CumulativeCost` in `agent-telemetry.ts` (which imports only node
    builtins) and `agent-footer.ts` re-exports it, keeping
    `agent-footer.test.ts:14` unchanged. `agent-cost.ts` imports nothing from
    `agent-footer.ts`.
  - The controller today builds `CostEstimateState` directly and reads and
    writes `registryEstimates` (`agent-footer.ts:478-583`, set at `:495`,
    conditional delete in `stop()` at `:580`). `agent-cost.ts` exposes that as
    a small attach/release pair for the footer's estimate (names are the
    implementer's) rather than exporting the mutable WeakMap. These are
    package-internal exports, not plugin surface.
  - Callers and tests switch import paths: `spawner.ts:124`, `ask.ts:135`,
    `index.ts:220`, `agent-usage-rollup.test.ts:27`,
    `agent-usage-rollup.integration.test.ts:22`, `eviction-records.test.ts:34`,
    the string `FOOTER_URL` child-process scripts import from
    (`eviction-records.test.ts:77`), and the dynamic import in
    `test/fixtures/usage-hop.ts:57`.
  - `removedAgentMessage` already lives in `agent-storage.ts:262` and stays.
  - Registry pruning stays triggered at the same points and in the same order,
    because tests pin it (`eviction-records.test.ts:281-285`, `:465`,
    `:476-477`: `descendantUsageValue` drops a recorded entry whose home is
    gone). The pruning half of `reconcile` becomes a separately named method
    of the cost state in `agent-cost.ts` (for example `pruneRemovedAgents`).
    It stops each dropped entry's observer through the record's own
    `ownershipObserverStop` field (`agent-footer.ts:186-187`), so it needs no
    footer import. Every entry point that reconciles today
    (`CostEstimateState`'s constructor, `evictionCost`, `persist`,
    `descendantUsageValue`, `persistEvictedAgentCost`, and the controller's
    `refreshAgents`) runs refresh records -> prune -> compute against one
    record snapshot: the prune uses the records the same call just refreshed,
    never the previous call's set, and records are not re-read between prune
    and compute. The mutation is visible at the call site and documented,
    not hidden inside a value read.
  - Rejected: making the value reads side-effect free. It would change what
    the pinned tests observe.
- **Removal predicates: three named predicates, not one.** A single function
  cannot serve every call site without a behavior change:
  - `ownedHomeRemovalState` (`agent-storage.ts:322-330`, three-state,
    claim-aware) stays the sidecar parse and revival gate, where a held claim
    may still roll back.
  - `isOwnedHomeGone` (`agent-storage.ts:300-305`, home absence, claim-aware)
    stays the registry-drop test inside pruning; `ownedHomeRemovalState` would
    drop removal-pending entries whose home is still present
    (`eviction-records.test.ts:161,199,465`).
  - `isRemovedAgent` (record only, fail-closed, works from `agentId` alone)
    stays the relaunch and rehydrate refusal (`spawner.ts:3513,3534`,
    `ask.ts:1486`): "removed, or is removing" must refuse even under a held
    claim.

  Each predicate's doc comment states its meaning and names the call sites
  that need that meaning and why the others must not be used there.
  `isRemovedAgent` moves to `agent-cost.ts` with `registryStorage` (above);
  the other two stay in `agent-storage.ts`.
  - Rejected: one unified predicate (above).
  - The two behavior issues forwarded here by 4128ae93 and 5c5ae07f (an
    abandoned claim keeps an entry claimed and revived every session; a full
    claim cycle between the gate's record and claim reads is read as removal)
    are behavior changes, so they move to
    `260924-bug-pi-removal-claim-abandonment-and-generation`, not this ticket.
- **Approval's parent half stays in spawner.ts.** Rejected: moving
  `attachApprovalChannel` and `ApprovalChannelHost` to
  `approval-protocol.ts`. 306197ba placed reconciliation in spawner.ts on
  purpose and gave the protocol module no imports (its header,
  `approval-protocol.ts:26-30`); the release and reissue paths differ (release
  syncs protection and refreshes the widget, reissue only syncs,
  `spawner.ts:2732-2736` vs `:2753`), so injection needs two callbacks for
  65 lines; and the premise is weaker than reported: subtree's record side
  also stays in spawner (`observeChildSubtree`, `spawner.ts:2673-2691`), with
  only protocol parsing in `subtree-lifecycle.ts`.
- **Small items.**
  - `WEB_HOME_ENV` stays in `web-readiness.ts`: it is shared by the parent side
    (`spawner.ts:121` imports it, `:2476` sets it) and the child side
    (`web-tools.ts:29` reads it), so a neutral module is the right home. The
    background finding is withdrawn.
  - Remove the `PendingApproval` alias (`execute-gateway.ts:273`); its uses
    (`execute-gateway.ts:288`, `execute-gateway.test.ts:75,135,140,380,556`)
    switch to `PendingApprovalState`.
  - Fix the missing space at `agent-sidecar.ts:271`.

## Constraints

- No behavior change: the existing test suite passes unchanged except for
  import-path and type-name updates (a moved symbol's import, the
  `FOOTER_URL` string and fixture import, the `PendingApproval` rename); any test that must change
  semantically means scope creep and stops the work.
- Out of scope: the forwarded behavior fixes (see Decisions), reserved-key or
  collision enforcement on the constructor resume source, and side-effect-free
  cost reads.
- `approval-protocol.ts` keeps importing nothing.

## Prior Decisions

- 4128ae93 (2026-09-24, commit): "Correctness Minor 2 (abandoned claim on a detached home keeps the entry "claimed" forever) is not changed: reconcile's isOwnedHomeGone has the same semantics ... the two predicates the refactor ticket unif[ies]" — bearing: constrains
- 5c5ae07f (2026-09-24, commit): "a full claim cycle that writes and rolls back a record entirely between the gate's record read and claim read is still read as removal; closing it needs a claim-generation marker, left to the predicate-unification refact[or]" — bearing: constrains
- 03305a62 (2026-09-24, commit): "A crash after detach that leaves a dead-pid lock keeps the entry excluded (not dropped) ... accepted over reading lock liveness in the hot reconcile path." — bearing: constrains
- 260924-feat-pi-agent-channel-approval-decisions (2026-09-24, Result 306197ba): "Reconciliation lives in `spawner.ts` (the parent record owner) so `execute-gateway.ts` keeps importing from `spawner.ts` only, never the reverse; the protocol module imports from neither." — bearing: constrains
- aba83809 (2026-09-24, commit): "ChildChannel.provideResume(key, provider) is a small generic addition so features can contribute resume state after the factory-time connect; a throwing provider is skipped rather than failing the hello." — bearing: constrains
- 61303b5a (2026-09-24, commit): "index.ts keeps the combined subtree+approval resume callback and adds the descendant-usage reporter (its resume rides the separate provideResume key, merged by ChildChannel alongside options.resume ...)" — bearing: supports
- 94c87426 (2026-09-24, commit): "Import cycle: mergeAgentCost moved to agent-telemetry.ts as mergeCumulativeCost, so agent-storage can merge a record rewrite with the existing record without importing agent-footer." — bearing: constrains
- d65040e4 (2026-09-24, commit): "removeOwnerArtifact's duplicated symlink check and the isRemovedAgent / removedAgentMessage module split were left as is (minor, no behavior gain)." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/index.ts, agent-channel.ts, agent-usage-rollup.ts, agent-footer.ts, agent-sidecar.ts, agent-storage.ts, spawner.ts, ask.ts, approval-protocol.ts, web-readiness.ts, web-tools.ts, execute-gateway.ts |
| scope.surface | cross-module | moves exports from agent-footer.ts to a new agent-cost.ts (registerAgentCostOwner, persistAgentCostCheckpoint, capacityEvictionCost, persistEvictedAgentCost, retentionEvictionCost, descendantUsageValue, isRemovedAgent); removes the PendingApproval alias; ChildChannel resume composition refactored internally |
| scope.new_public_symbol | no | symbols move modules with unchanged names and signatures; the prune method, the resume composition helper, and the footer estimate attach/release pair are package-internal, not plugin surface |
| scope.new_type_contract | no | hello resume wire content, provideResume and ChildChannelOptions.resume signatures, and all three removal predicates keep their shapes (Decisions) |
| scope.test_surface | existing | agents-plugin-pi/test/agent-channel.test.ts, agent-channel.integration.test.ts, agent-footer.test.ts, agent-sidecar.test.ts, eviction-records.test.ts, agent-usage-rollup.test.ts, execute-gateway.test.ts; merged-resume pin may be a new case in an existing file |
| complexity.reuse_points | confirmed | ChildChannel.provideResume agent-channel.ts:673 and ownedHomeRemovalState agent-storage.ts:323 read in the current tree |
| complexity.side_effect_risk | moderate | reconcile's registry deletion and observer stop (agent-footer.ts:173-189) move into agent-cost.ts and split into a named prune step; the refresh -> prune -> compute order on one record snapshot must be kept at every entry point |
| risk.correctness | moderate | pruning order and record-snapshot sharing across six reconcile entry points (pinned by eviction-records.test.ts); predicates keep their bodies, only isRemovedAgent moves |
| risk.fit | moderate | decisions settled; the new agent-cost.ts must not create an import cycle with agent-storage.ts or spawner.ts (94c87426 moved mergeCumulativeCost to agent-telemetry.ts to avoid one), and approval-protocol.ts keeps no imports |
| risk.test | moderate | no-semantic-test-edit constraint plus a required pin of the merged hello resume object across subtree, approval, usage, readiness |
| risk.security_or_contract | low | hello resume wire content and precedence are unchanged and newly pinned; both registration entry points and their signatures stay |

## Phases

### Phase 1: Consolidate resume registration and cost-module boundaries

Implement `## Decisions` and the small items.

Verification expectations:

- Full `npm test` in `agents-plugin-pi` passes with only the test edits the
  Constraints allow.
- A new test pins the reconnect hello's merged `resume` object when the
  constructor source and `provideResume` both contribute: the key set, key
  order (constructor keys, provider keys in registration order, then
  `readiness`), and values. No existing test pins the merged object across
  both sources today (`agent-channel.test.ts:131,144,178`,
  `agent-usage-rollup.test.ts:199` each pin one source).
- Each entry point that reconciled before (listed in Decisions) still runs
  refresh -> prune -> compute on one record snapshot, evidenced by the
  unchanged `eviction-records.test.ts` pins.
- `agent-footer.ts` no longer defines cost estimation, eviction-record values,
  `descendantUsageValue`, `isRemovedAgent`, or `formatCumulativeCost` (it
  re-exports the last).
- `agent-cost.ts` has no import from `agent-footer.ts`, and
  `approval-protocol.ts` has no imports (checked by reading the import lines;
  a test load alone cannot catch a runtime-tolerated ESM cycle).
- The three removal predicates' doc comments each state their meaning, their
  call sites, and why the other two must not be used there.

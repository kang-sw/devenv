---
title: Pi channel resume registration and cost-module boundaries cleanup
related:
  260924-bug-pi-review-sweep-correctness-fixes: behavior fixes from the same sweep; land first, this ticket must not change behavior
  260924-feat-pi-agent-channel-usage-rollup: introduced provideResume and grew agent-footer.ts
  260924-feat-pi-agent-channel-approval-decisions: placed the approval parent half in spawner.ts
  260924-bug-pi-retention-cross-owner-checkpoint-fold: eviction-record values now living in agent-footer.ts
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
  ticket makes the sidecar claim-aware; unifying them is here.
- **Approval's parent half lives in spawner.ts** (`attachApprovalChannel` and
  `ApprovalChannelHost`, around `:2688`), while subtree and usage keep both halves
  in their own modules.
- Small items: `WEB_HOME_ENV` defined in `web-readiness.ts:12` but used only by
  `web-tools.ts`; leftover alias `export type PendingApproval =
  PendingApprovalState` (`execute-gateway.ts:273`); missing space at
  `agent-sidecar.ts:264`.

## Open Decisions (settle at promotion)

- One resume registration path: migrate subtree and approval onto
  `provideResume`, or retire `provideResume` in favor of the constructor
  option.
- Module split for cost accounting and eviction values out of agent-footer.ts,
  and where registry pruning moves so reads stop mutating the registry.
- Shape of the single removal predicate and which call sites need the
  claim-aware versus record-only meaning (if any must differ, name why).
- Whether approval's parent half moves to approval-protocol.ts with
  `syncOwnershipProtection` and widget refresh injected as callbacks.

## Constraints

- No behavior change: the existing test suite passes unchanged except for
  import-path updates; any test that must change semantically means scope
  creep and stops the work.

## Phases

### Phase 1: Consolidate resume registration and cost-module boundaries

Implement the settled Open Decisions and the small items. Verification: full
`npm test` in `agents-plugin-pi`, no semantic test edits, and the hello content
of subtree, approval, and usage unchanged (a test pins the merged resume
object).

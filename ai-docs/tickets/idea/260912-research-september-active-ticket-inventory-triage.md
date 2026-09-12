---
title: "Triage the September active-ticket inventory after the refoundation release sweep"
related:
  260909-research-ws-refoundation-evidence-audit: retained binding anchor and context for the refoundation-era subset
  260911-research-batch-promotion-cross-ticket-coherence-gap: one open item selected for actionable derivation before this inventory was recorded
---

# Triage the September active-ticket inventory after the refoundation release sweep

## Background

During the 2026-09-12 pre-release sweep, older `idea/` and `todo/` tickets kept
surfacing one at a time after related implementation had already landed. The
owner asked for one durable inventory of every active ticket dated 2026-09-01 or
later so later triage can decide what to implement, retain, close, or drop with
the surrounding context visible at once.

This research does not assign priority or authorize implementation or closure.
It records a read-only comparison against `develop` at `717b1086`.

## Inventory

| status | stem | assessment | evidence summary |
|---|---|---|---|
| idea | `260901-bug-enter-proceed-misplaced-facts-silent-unknown-status` | Live implementation candidate | The current proceed resolver can still return a bare unknown status without the requested caller diagnostic. |
| idea | `260901-bug-ticket-scanner-silently-skips-noncanonical-status-dir` | Open research | The warning-versus-tolerant-mapping decision remains open; scanners still enumerate canonical directories only. |
| idea | `260901-research-enter-tool-direct-call-affordance-rename` | Cleanup candidate | Its direction landed through `260904-refactor-enter-affordance-rename-route-opaque`; the runtime now exposes `route.resolve_*`. |
| idea | `260904-bug-stale-proceed-dispatch-contract-test` | Cleanup candidate | The related pre-refoundation contract-test ticket was dropped and the tested lead surface was retired. |
| idea | `260904-bug-subagent-worktree-head-detach-orphans-commits` | Cleanup candidate | `260909-bug-code-reviewer-delegate-switches-shared-worktree-branch` landed the reviewer-side protection; broader isolation remains in the separate worktree ticket. |
| idea | `260904-refactor-cli-subcommand-verb-alignment` | Open research | CLI independence versus MCP verb alignment remains undecided and unimplemented. |
| idea | `260904-refactor-mental-model-doc-drift-epic-renames` | Cleanup candidate | `260909-refactor-retire-spec-mental-model-layers` removed the live mental-model layer. |
| idea | `260904-research-mental-models-query-reconciliation` | Cleanup candidate | The same retirement removed the queried tool surface. |
| idea | `260907-feat-ws-tickets-query-pagination-and-keyword-ranking` | Open research | It remains demoted with unresolved pagination and ranking decisions and no implementation. |
| idea | `260908-bug-todo-add-accepts-call-without-required-title` | Live implementation candidate | Current todo creation still accepts an unchecked or empty title. |
| idea | `260909-research-ws-refoundation-evidence-audit` | Retained reference | `AGENTS.md` names it as the binding anchor for the refoundation topics. |
| idea | `260910-feat-lead-run-worktree-parallel-route` | Live candidate with open decisions | Resource/worktree root separation landed, but worktree acquire/release and the gated parallel route do not exist; dispatch and reporting choices remain unsettled. |
| idea | `260911-bug-implement-route-reuses-prior-phase-branch` | Live implementation candidate | The captured prior-phase branch selection defect has no closing implementation or superseding ticket. |
| idea | `260911-bug-lead-run-stop-c-edition-before-result` | Live implementation candidate | `lead-run` still requires an Edition for stop-(c) recovery without distinguishing a phase that has no Result. |
| idea | `260911-research-batch-promotion-cross-ticket-coherence-gap` | Actionable derivation selected | Its single batch-design-review direction was confirmed on 2026-09-12 and is being derived separately. |
| idea | `260911-research-epic-close-on-last-child-prompt` | Open research | The current lead-run nudge is explicitly interim; trigger and predicate choices remain open. |
| idea | `260911-research-golden-fixture-verification-gap` | Open research | The observed fixture was repaired, but no general discovery or verification mechanism was selected. |
| idea | `260911-research-lead-commit-guard-during-worker-run` | Open research | Shared-worktree lead mutation remains unguarded; prose, MCP, liveness, and override choices remain open. |
| idea | `260911-research-ticket-decision-state-convention-gap` | Cleanup candidate | `260911-feat-research-outcome-ledger-derivation-contract` implemented the Outcome Ledger and derivation rules. |
| idea | `260912-bug-lead-review-fix-relay-ticket-only-run` | Live implementation candidate | `lead-review` still hands a review artifact to ticket-only `lead-run`; the replacement contract remains undecided. |
| idea | `260912-bug-ticket-query-point-resolve-terminal-status` | Cleanup candidate pending runtime check | Current source point-resolves terminal tickets; the observed failure likely came from stale installed runtime and needs one post-reload check. |
| idea | `260912-research-git-merge-epic-develop-boundary` | Cleanup candidate | `260912-feat-git-merge-generic-branch-promotion` implemented the confirmed generic-merge direction. |
| idea | `260912-research-git-merge-release-target-policy` | Cleanup candidate | `260912-bug-git-merge-release-target-diagnostics` implemented diagnostics and OID-bound acknowledgement. |
| todo | `260911-research-impl-lifecycle-merge-authority-goal-loop-rehoming` | Cleanup candidate with residual question | Its merge, selector, and goal-trigger children landed; only an explicitly deferred host-loop concern remains and should become fresh scope if pursued. |

## Immediate Disposition

`260912-bug-design-reviewer-explore-failure-direct-inspection-fallback` was
captured during this authoring run and dropped after the owner clarified that
direct inspection is an acceptable fallback when Explore is unavailable. The
failed dispatch was transient harness capacity and succeeded on retry. It is not
part of the active inventory above.

## Outcome Ledger

### Verified Findings

- The active-inventory survey contains 24 tickets: 23 in `idea/` and one in
  `todo/`.
- The resulting ledger classifies seven as open research, six as live
  implementation candidates, ten as cleanup candidates, and one as an
  intentionally retained binding anchor.
- Ten active tickets have strong evidence of implementation, supersession, or
  loss of their target surface, but remain active pending explicit lifecycle
  decisions.
- The retained evidence-audit ticket is not clutter: current `AGENTS.md`
  requires it as the binding anchor.

### Confirmed Decisions

- Keep this research ticket as the single context record for later September
  inventory triage.
- Derive the batch-promotion cross-ticket design review as a separate actionable
  implementation ticket.
- Treat the Explore failure as transient harness capacity; direct inspection is
  acceptable when Explore is unavailable, so the captured fallback bug is
  dropped.
- Do not treat this inventory classification alone as approval to implement,
  close, or drop any other ticket.

### Proposals

- Review the ten cleanup candidates as one lifecycle batch rather than letting
  them resurface individually.
- Evaluate the remaining research and implementation candidates by release
  impact and reproducibility; the inventory does not currently establish that
  all or most are critical.

### Open Questions

- Which cleanup candidates should close as implemented versus drop as obsolete
  or superseded?
- Which of the remaining thirteen open items belong before the next release,
  and which should stay deferred?
- Does a fresh installed-runtime check reproduce the terminal point-resolution
  failure?

### Rejected Alternatives

- None recorded. The owner requested consolidated context before making the
  implementation and lifecycle choices.

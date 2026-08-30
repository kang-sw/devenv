---
title: lead-review range/watermark scenario — parameterize diff selection, add landing lens
parent: 260824-epic-review-watermark-model
related:
  260824-feat-review-watermark-ledger: dependent — the marker/ledger sweep consumes this range scenario
  260824-feat-review-release-gate-policy: dependent — the release gate reviews a range through this scenario
sage-review-design: completed
sage-review-completeness: completed
---

# lead-review range/watermark scenario — parameterize diff selection, add landing lens

## Background

`lead-review` today has a single scenario: branch/PR review with diff selection
hardcoded to a checked-out branch (`On: invoke [branch?]` → checkout → diff).
The epic's sweep and release-gate layers need to review an arbitrary commit
range (`marker..HEAD`) instead. The underlying `git.diff` MCP tool already
accepts a `range` (`wsgit.DiffOptions.Range`, exposed in the schema); the gap is
purely that the playbook only knows "branch," never "range." This ticket is the
**prerequisite** that unblocks the marker/sweep (③) and the release gate (④).

Circled numbers denote the epic's sibling children: ② = this ticket
(`260824-feat-lead-review-range-scenario`), ③ =
`260824-feat-review-watermark-ledger`, ④ = `260824-feat-review-release-gate-policy`
(see `related:`).

## Decisions

- Reuse, do not rebuild. The existing phase machinery
  (intent/alignment/risk, `judge: is-large-diff`/Deep Review, verdict routing
  LGTM/NEEDS FIX/OPEN) is diff-content-agnostic and stays as-is; only how the
  target diff is determined becomes scenario-parameterized.
- Add a **landing lens** (convention adherence + spec/mental-model update
  completeness — were docs authored per each doc's function). This is the
  implementation-side symmetry of sage's completeness stage. **Scope it to the
  range/watermark scenario (own integrated work), not unconditionally across the
  branch/PR scenario** — an external contributor's PR must not be flagged for
  spec/mental-model updates they were never expected to author. Captured as a
  folded required-check; whether it later splits into its own separate posture is
  deferred (epic Deferred note).
- **Config-load behavior is scenario-scoped (2026-08-30 discuss).** Today's
  Invariant "load `ai-docs/_review.local.md`; **run setup if absent**" forces a
  9-question interactive setup when the config file is missing. That is correct
  only for the **branch/PR scenario** — an explicit human invocation whose config
  is dominated by **collaboration/remote mechanics** (Remote fetch, Branch Naming,
  Comment/Merge-Approval/Notification Method, Contributor Workflow) that only
  interactive setup can supply. The **range scenario** is caller-invoked (the
  sweep/gate in ③/④), touches no checkout/remote/merge, and needs only
  **review-substance** settings that **already ship with built-in defaults**
  (intent/alignment/risk phases, deep-review threshold 20 files/500 lines, plus
  the landing lens). Therefore the range scenario **runs on built-in defaults when
  `_review.local.md` is absent and never forces interactive setup**; it still
  *respects* the config's review-substance sections when present. Forced setup
  stays exclusive to the branch scenario. Rationale: the automated sweep/gate path
  — and the git-naive brute-persona relief valve that rides it
  (`260829-research-review-checkpoint-relief-valve`) — must not be blocked by an
  interactive setup, and the collaboration/remote half of the config is
  meaningless for a checkout-free range review. This makes "range mode × config
  presence" genuinely orthogonal (the prior coupling was a branch-scenario
  assumption baked into a shared Invariant).

## Phases

### Phase 1: Scenario-kind diff selection (branch vs range)

- Parameterize `lead-review`'s target selection by scenario kind: the existing
  branch scenario, plus a `range` scenario that reviews `git.diff(range: "<base>..<head>")`
  / `git.log(range:)` rather than a checked-out branch. Base/head are supplied
  by the caller (the sweep/gate in ③/④); this ticket does not own the marker.
- Keep the downstream phase machinery and verdict routing unchanged; only the
  diff/commit-enumeration source differs.

Verification: a range scenario over a known `base..head` produces the same
phase/verdict flow as a branch scenario over the equivalent branch diff;
`is-large-diff`/Deep Review threshold still trips on a large range. **A range
scenario with no `ai-docs/_review.local.md` present runs on built-in defaults and
never enters `On: setup`; the branch/PR scenario with no config still forces
setup as today; a present config's review-substance sections are honored by
both.**

### Phase 2: Landing lens as a review-config required-check

- Add the landing lens (convention adherence + spec/mental-model update
  completeness) to the review config (`_review.local.md` template) as a
  required-check that runs within the review phases.
- Depends on Phase 1 (the lens rides the range-scenario review path).

Verification: a range review over a diff that changed behavior without a
matching spec/mental-model update surfaces a landing-lens finding; a
convention-conformant, doc-complete diff passes it.

## Spec Impact

Target: the `lead-review` behavior area in `ai-docs/spec/workflow-skills.md`.
Expected caller-visible change: `lead-review` gains a range/watermark scenario
(diff selected by explicit `base..head` rather than only branch checkout) and a
landing-lens required-check in its review config. The range scenario runs on
built-in review-substance defaults when `_review.local.md` is absent and never
forces interactive setup (setup-when-absent stays exclusive to the branch/PR
scenario). No change to verdict vocabulary or the existing branch scenario.

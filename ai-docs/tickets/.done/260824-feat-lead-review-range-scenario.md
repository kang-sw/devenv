---
title: lead-review range/watermark scenario — parameterize diff selection, add landing lens
parent: 260824-epic-review-watermark-model
related:
  260824-feat-review-watermark-ledger: dependent — the marker/ledger sweep consumes this range scenario
  260824-feat-review-release-gate-policy: dependent — the release gate reviews a range through this scenario
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-30
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

### Result (01fd2fe3) - 2026-08-30

Landed on `impl/goal/develop/copper-lantern-drizzle/issue-drew-spent` at
`01fd2fe3` (skill + spec + regenerated manifests/mirror), with a follow-on
mental-model drift fix at `3571116b`.

- **Scenario-kind diff selection.** `lead-review`'s target selection is now
  parameterized up front by scenario kind: the existing `branch` scenario, plus a
  `range` scenario that selects the diff via `git.diff(range: "<base>..<head>")`
  and enumerates commits via `git.log(range: "<base>..<head>")` from
  caller-supplied `base`/`head` (the sweep/gate in ③/④ own the marker; this ticket
  does not). Both supplied → range precedence. The downstream phase machinery
  (intent/alignment/risk, `is-large-diff`/Deep Review, verdict routing
  BLOCKED/LGTM/NEEDS FIX/OPEN) is untouched and diff-content-agnostic, so Deep
  Review still trips on a large range.
- **Scenario-scoped config-load.** Branch scenario, absent `_review.local.md` →
  still forces `On: setup` (unchanged). Range scenario, absent config → runs on
  built-in review-substance defaults (Review Phases, Deep Review threshold) and
  never enters setup; it ignores the collaboration/remote config half (no
  checkout/remote/merge surface). A present config's review-substance sections are
  honored by both scenarios. This makes "range mode × config presence" orthogonal.
- **Artifacts.** Only the canonical `agents-plugin/rsrc/lead-review/lead-review.md`
  was hand-edited; `agents-plugin-wsflow/rsrc/lead-review/lead-review.md` and both
  `manifest.json` files were regenerated (mirror confirmed byte-identical). The
  `lead-review` behavior area in `ai-docs/spec/workflow-skills.md` was updated
  (anchor id unchanged), and the mental-model `workflow-skills.md` lead-review
  contract bullet was corrected (it had implied unconditional forced setup).
- **Golden constraint honored.** The doctrine sentence pinned by
  `TestPlaybookPrintGoldenLeadReview` was not altered.

Review (partitioned: correctness / fit / test) — all clean; one accepted minor
(correctness): the word "branch" is overloaded (conditional branch vs git branch)
in the scenario-dispatch sentence at `lead-review.md:23`; non-blocking wording
nit, not worth churning the manifest/mirror/golden pipeline for.

Verification: `TestPlaybookPrintGoldenLeadReview` PASS (doctrine untouched);
`go test ./internal/wsrsrc/...` PASS (incl. `TestShippedManifestUpToDate`,
`TestWsflowRsrcMirrorUpToDate`); `python3 -m unittest discover
agents-plugin-wsflow/tests` 10/10 PASS; `spec_index_verify` ok; `go build ./...`
clean.

Phase 2 (landing lens as a review-config required-check) remains; it rides this
range-scenario path.

### Phase 2: Landing lens as a review-config required-check

- Add the landing lens (convention adherence + spec/mental-model update
  completeness) to the review config (`_review.local.md` template) as a
  required-check that runs within the review phases.
- Depends on Phase 1 (the lens rides the range-scenario review path).

Verification: a range review over a diff that changed behavior without a
matching spec/mental-model update surfaces a landing-lens finding; a
convention-conformant, doc-complete diff passes it.

### Result (cb777b56) - 2026-08-30

Landed on `impl/goal/develop/copper-lantern-drizzle/issue-drew-spent` at
`cb777b56`.

- **Landing lens as a required review phase.** `lead-review` gains a `landing`
  Review Phase (convention adherence + spec/mental-model update completeness —
  were docs authored per each doc's function) folded into the existing phase
  execution: no new posture, no new pipeline, no new verdict state. Placed last in
  phase order (after any custom config phases).
- **Range-scenario-scoped, enforced.** The lens runs ONLY in the range/watermark
  scenario. The branch/PR scenario never runs it — regardless of the `Contributor
  Workflow` config — and no config section can re-enable it there. This keeps an
  external contributor's PR from being flagged for spec/mental-model updates they
  were never expected to author. The scoping is expressed scenario-gated (not
  contributor-type-gated) consistently across playbook, spec, and mental-model.
- **Wording trap avoided.** The shared Invariant "a present config's Review
  Phases / Checklist / Blocked Paths / Deep Review are honored by both scenarios"
  does NOT absorb the landing lens; the lens's range-only scope is stated
  separately.
- **Artifacts.** Only canonical `agents-plugin/rsrc/lead-review/lead-review.md`
  hand-edited; wsflow mirror + both `manifest.json` regenerated (byte-identical);
  `ai-docs/spec/workflow-skills.md` (`#260513-review-workflow-skill`) and the
  mental-model `workflow-skills.md` updated. Golden-pinned doctrine sentence
  unchanged.

Review (partitioned: correctness / fit / test) — all clean, no relays.

Verification: `TestPlaybookPrintGoldenLeadReview` PASS (doctrine byte-identical);
`go test ./internal/wsrsrc/...` drift guards (`TestShippedManifestUpToDate`,
`TestWsflowRsrcMirrorUpToDate`) PASS; `python3 -m unittest discover
agents-plugin-wsflow/tests` 10/10 PASS; `spec_index_verify` ok; `go build ./...`
clean.

Both phases are complete; this ticket is done. It unblocks ③
(`260824-feat-review-watermark-ledger`) and ④
(`260824-feat-review-release-gate-policy`), which consume this range scenario.

## Spec Impact

Target: the `lead-review` behavior area in `ai-docs/spec/workflow-skills.md`.
Expected caller-visible change: `lead-review` gains a range/watermark scenario
(diff selected by explicit `base..head` rather than only branch checkout) and a
landing-lens required-check in its review config. The range scenario runs on
built-in review-substance defaults when `_review.local.md` is absent and never
forces interactive setup (setup-when-absent stays exclusive to the branch/PR
scenario). No change to verdict vocabulary or the existing branch scenario.

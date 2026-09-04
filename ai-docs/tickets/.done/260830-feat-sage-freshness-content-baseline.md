---
title: Sage freshness content baseline — record reviewed-body digest so a re-stamp clears staleness
related:
  260824-epic-review-watermark-model: surfaced here — the epic's ready-promotion of child tickets wedged on this freshness gap; not an epic child, a general sage-tooling fix
  260824-feat-review-watermark-ledger: unblocked by this — its stale design flag (baseline pinned to original completion) cannot be cleared today
  260824-feat-review-release-gate-policy: unblocked by this — same stale-flag wedge at ready promotion
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-30
---

# Sage freshness content baseline — record reviewed-body digest so a re-stamp clears staleness

## Background

Sage-review "freshness" flags a `completed` stage whose ticket body has drifted
since it was reviewed, so the lead re-reviews before the stage is trusted. The
detection is sound, but the **baseline it diffs against is unrecordable and
cannot be advanced by a re-review**, so a re-reviewed ticket stays flagged
forever.

Traced to source (Explore, 2026-08-30):

- `sageReviewStageBaseline` (`agents-plugin-tool/internal/wsdoc/tickets_sage_freshness.go:71-109`)
  derives the baseline from git history as the **earliest** commit where the
  stage transitioned into `completed`, iterating oldest→newest and returning on
  the first match (`:81-96`).
- `sageReviewFreshnessCheck` (`:31-69`) compares the **normalized** current
  content against that baseline commit's content; any body change (frontmatter
  sage fields stripped) marks the stage stale.
- `SageRecord`/`sage_stamp` (`tickets_sage.go` `SageRecord` path) writes **only
  the posture field** — no baseline SHA, digest, or timestamp is recorded
  anywhere (confirmed: no baseline write exists).

Consequence: once a ticket is reviewed (T0), materially re-adjusted, and
re-reviewed (T1), the baseline stays pinned at T0. Re-stamping at T1 is a
`completed → completed` write, which creates **no** new completed-transition, so
the baseline never advances — and the loop returns the *earliest* transition
regardless. The re-reviewed body reads as permanently stale.

Observed live (2026-08-30): epic `260824` children `260824-feat-review-watermark-ledger`
and `260824-feat-review-release-gate-policy` had design re-reviewed at `03b6e2bd`
on content **byte-identical to HEAD**, yet `sage_gate(landing: ready)` returned
`check_review_required` against baseline `cc828d74` (their creation commit). The
gate short-circuits on the stale completed stage **before** surfacing the pending
`completeness` `ask`, so the promotion to `ready` is wedged with no tool
affordance to clear it. (Freshness is only a soft WARN at `tickets.verify` /
`git.commit` — `tickets_verify.go:173-200` — so it does not block commits; the
block is specifically the gate return on the promotion path.)

This is a tooling gap, not a workflow-model change; it is **not part of the
`260824` epic** but was surfaced while promoting that epic's children.

## Decisions

- **Record an explicit content baseline at stamp time (primary).** When
  `sage_stamp` writes `sage-review-<stage>: completed`, it also writes a
  companion machine-managed field `sage-review-<stage>-reviewed: <digest>`, where
  `<digest>` is a hash of the **normalized reviewed body** — sha256, truncated to
  a fixed 16-hex-char prefix (the field is machine-managed and opaque, so the
  width only needs to be stable; 16 hex is ample against accidental collision for
  a single ticket's body). Freshness compares the recorded digest against the current
  normalized-body digest: equal → fresh, differ → stale. No git-history walk on
  the primary path. A re-stamp rewrites the digest, so **the re-review flow
  (warning → re-review → re-write `completed` → digest updated → warning clears)
  advances the baseline with zero forced commits and no dependence on a committed
  `pending` dip.** This is the "advance the stamp" fix, done explicitly rather
  than inferred from commit topology.
- **Body-only normalization (frontmatter excluded).** The digest and the
  freshness comparison hash the **markdown body below the frontmatter fence**,
  trimmed — the entire frontmatter is excluded, not only the sage fields. Today's
  normalization strips only `sage-review*` keys and still includes `title` /
  `related` / `parent`, so link/title housekeeping falsely triggers staleness.
  Frontmatter is metadata and cross-references, not review substance; excluding
  it is a deliberate scope narrowing. Primary (digest) and fallback (below) must
  use the **same** body-only normalization so the two paths agree.
- **Git-transition fallback, corrected to latest (legacy).** Tickets with no
  recorded digest fall back to the git-history baseline, but corrected to the
  **latest** completed-transition (not the earliest) so a committed
  reset→re-stamp advances it. Legacy tickets **self-heal**: their next stamp
  writes a digest and moves them onto the primary path. No migration pass.
- **Separate field, not an inline posture value.** The posture value stays
  exactly `completed`; the digest lives in a sibling `sage-review-<stage>-reviewed`
  field. Rejected inline `completed (<token>)`: it would touch every posture
  parser (`effectiveSageReviewPostures`, `resolveStage`, the freshness field
  reads) and raise regression risk for no gain.
- **The new field is itself excluded from the digest** (body-only already
  guarantees this, since it lives in frontmatter) and from any posture parsing.
- **Backward compatible by construction.** Absent digest → fallback path;
  presence is detected per stage. No existing ticket needs editing; the corpus
  converges as tickets are re-stamped.

## Non-Scope

- **No lead "dismiss without re-review" affordance.** An earlier idea — a
  `sage_gate` answer that records "inspected, prior review still current, do not
  re-flag" — is not included: the digest mechanism makes a re-stamp the natural,
  honest clear path (re-stamping *is* recording the review as current). A pure
  dismiss (accept staleness with no re-review) can be a separate ticket if a real
  need appears.
- **No change to `sage_gate` stage ordering.** Resolving a stale completed stage
  before the pending completeness `ask` is retained as correct; only the
  staleness *signal* is fixed.
- **No wall-clock timestamp as the freshness key.** Rejected (consistent with the
  epic's "range key is a commit SHA, never wall-clock"): a timestamp cannot detect
  content divergence and mixes clocks with git. A content digest is exact.

## Phases

### Phase 1: Record and consume a body-digest freshness baseline

- In the `sage_stamp` write path (`SageRecord` and its single/combined helpers),
  when a stage is written `completed`, also write
  `sage-review-<stage>-reviewed: <digest>` computed from the normalized body.
- Add a shared normalization that returns the **body below the frontmatter fence,
  trimmed** (whole frontmatter excluded), and a digest helper over it. Refactor
  `normalizeTicketForSageFreshness` and the freshness comparison to use it so
  primary and fallback agree.
- In `sageReviewFreshnessCheck` / `sageReviewStageBaseline`: if the current
  frontmatter carries `sage-review-<stage>-reviewed`, compare digests (no git
  walk) for that stage; otherwise fall back to the git-history baseline **corrected
  to the latest completed-transition** and compare body-only.
- Ensure the new field is stripped anywhere posture/frontmatter is parsed so it
  never perturbs postures or the digest.

Verification:
- A ticket reviewed, body-edited with a `completed → completed` re-stamp (no
  `pending` dip), then left unedited, reads **fresh** at `sage_gate(ready)` (the
  wedge that motivated this ticket); a subsequent body edit without a re-stamp
  reads **stale**.
- A legacy ticket (no recorded digest) with a committed reset→re-stamp on the
  current body reads **fresh** via the latest-transition fallback; the same ticket
  edited after its last stamp reads **stale**.
- A frontmatter-only edit (e.g. a `related:` or `title` change) after a stamp does
  **not** mark the stage stale.
- Existing freshness tests (single-transition stale/uncommitted/staged/status-move
  cases) stay green under body-only normalization.

### Result (29cb2795) - 2026-08-30

Landed on `impl/goal/develop/copper-lantern-drizzle/yam-corny-elite`, range
`29cb2795^..3b33136e` (two commits: `29cb2795` code+tests, `3b33136e` spec).

- **Digest write on stamp.** Both completed-posture write sites in
  `SageRecord` (`sageRecordSingle`, `sageRecordCombined`,
  `agents-plugin-tool/internal/wsdoc/tickets_sage.go`) now emit a sibling
  `sage-review-<stage>-reviewed: <digest>` alongside every `completed` posture,
  where `<digest>` is sha256 of the body-only-normalized ticket truncated to a
  fixed 16-hex prefix.
- **Digest-primary freshness, git fallback corrected to latest.**
  `sageReviewFreshnessCheck` compares the recorded digest against the current
  body digest with **no git walk** when the field is present; absent it,
  `sageReviewStageBaseline` (`tickets_sage_freshness.go`) now resolves the
  **latest** completed-transition (rewritten from earliest, newest→oldest
  precompute-then-scan) so a committed reset→re-stamp advances the baseline.
- **Body-only normalization.** `normalizeTicketForSageFreshness` now excludes
  the **whole** frontmatter block (was: strip only `sage-review*` keys), shared
  by both the digest and fallback paths so they agree. A frontmatter-only edit
  (`title:`, `related:`) no longer triggers staleness.
- **Backward compatible.** Absent digest → fallback; legacy tickets self-heal on
  their next stamp; no migration pass. The new field is read by exact key only
  and never perturbs posture parsing.
- **Spec.** `ai-docs/spec/mcp-tools.md` updated in two spots (`tickets.verify`
  freshness-warning summary and `tickets.sage_gate` mechanism paragraph) to
  match; no `ticket-conventions.md` edit (it carries no sage mentions).

Verification: `go build ./...` clean; `go test ./...` (agents-plugin-tool) all
`ok`; the three new tests (`TestSageGateDigestRestampClearsFreshness`,
`TestSageGateLegacyFallbackFollowsLatestTransition`,
`TestSageGateFrontmatterOnlyEditStaysFresh`) each map to a ticket verification
bullet and the test reviewer confirmed all three **fail** against the reverted
`29cb2795^` production code; the pre-existing `TestSageGate*` suite is
unmodified and green. All three review partitions (correctness/fit/test) clean —
one non-blocking correctness minor (a rare staged≠worktree state degrades to the
git fallback rather than the digest compare; safe, no wrong verdict).

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md` (the sage-review freshness contract for
`sage_gate` / `sage_stamp`) and the ticket-conventions frontmatter contract (the
new machine-managed `sage-review-<stage>-reviewed` field, and the body-only
freshness scope). Caller-visible changes: re-stamping a re-reviewed ticket clears
its freshness flag; a frontmatter-only edit no longer triggers staleness; a new
machine-managed frontmatter field appears on stamped tickets; the freshness
baseline advances on re-stamp instead of pinning to the first review.

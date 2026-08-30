---
title: Per-phase review lightening — remove unknown-risk bias, reaffirm single floor, narrow lead-only
parent: 260824-epic-review-watermark-model
related:
  260627-feat-enter-implement-deterministic-verdict-engine: substrate — owns deriveImplementReviewAlloc / implementReviewPartitions / materialRisk that this ticket edits
  260726-bug-lead-implement-lost-review-relay-cycle-cap: interaction — review-cycle budgets (2 vs 3) are keyed on the review_alloc label this shifts toward single/lead-only
  260729-research-implement-router-prose-only-dimension: motivating incident — documents the unknown/moderate-bias over-escalation this removes
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-08-25
---

# Per-phase review lightening — remove unknown-risk bias, reaffirm single floor, narrow lead-only

## Background

The default per-phase review pass over-allocates reviewers. In
`enter.implement`'s resolver (`agents-plugin-tool/internal/mcp/implement_resolver.go`,
which owns `materialRisk`, `implementReviewPartitions`,
`deriveImplementReviewAlloc`, and `automaticLeadOnlyReviewEligible`),
`materialRisk` treats `unknown` (not just `moderate`/`high`) as material, so any
un-derived risk fact escalates toward a partition. The result is the "three reviewers on a three-line change" pain and
the documented dogfood incident where a two-sentence prose change triggered a
full partitioned review pass (`260729-research-implement-router-prose-only-dimension`).

This ticket is the **independent, immediate pain relief** — it stands alone and
does not depend on the rest of the epic (range scenario, marker, gate). Under
the epic's target three-layer model it is the per-phase layer: slice
self-consistency only, kept light.

## Decisions

- Change the default *bias*, not add a new input. A `num_reviewer` field and a
  count→role priority table were rejected at the epic level (less expressive
  than risk-keyed partition selection; re-introduces non-determinism the verdict
  engine removed). This ticket only re-tunes the existing derivation.
- `single` (one delegated full-scope reviewer) is the default floor for the
  ~99% case. `lead-only` (no delegate) is **retained** but narrowed to genuinely
  trivial changes — a lead-performed review burns the lead context the workflow
  exists to save, so it must not become the common path.

## Phases

### Phase 1: Stop treating `unknown` risk as material; reaffirm floor and narrow lead-only

Goal: the default `review_alloc` resolves to `single` for low-signal changes,
so a partition (correctness/fit/test) is added only on a *positive* signal — an
all-`unknown`/un-derived fact set must land on `single`, not `partitioned`.

Constraint discovered in design review: making `unknown` non-material in
`materialRisk` alone is **insufficient**. In `implementReviewPartitions`
(`agents-plugin-tool/internal/mcp/implement_resolver.go`) two arms fire on
`unknown` independently of `materialRisk` — `ReusePoints == "unknown"` (fit) and
`TestSurface == "unknown"` (test) — and every fact defaults to `"unknown"`, so
an all-unknown set produces fit + test = `partitioned` regardless of the
`materialRisk` edit.

- Treat `unknown` as a **non-signal across all three partition arms**: drop
  `unknown` from `materialRisk` (keep `moderate`/`high`), and stop the
  `ReusePoints == "unknown"` and `TestSurface == "unknown"` arms from adding a
  partition. A partition is added only on a positive signal — `moderate`/`high`
  risk, a new type-contract / public symbol, `cross-module` surface,
  `ReusePoints == "unconfirmed"`, or `TestSurface == "new-files"`. With no
  positive signal, zero partitions resolve to `single` via
  `deriveImplementReviewAlloc`.
- **Do not broaden `lead-only`.** `automaticLeadOnlyReviewEligible` continues to
  require genuine `low` (not `unknown`), so an all-unknown set lands on `single`
  (one delegated full-scope reviewer), never `lead-only` — this matches the
  settled floor (single = default; lead-only = narrowed to trivial). `single`
  remains the existing generic `reviewer` over the full-scope contract.
- Do **not** change the review-cycle budget wiring (2 vs 3 cycles keyed on the
  `review_alloc` label owned by `260726`); this ticket only changes which label
  is derived, not what the label triggers downstream.

Verification: resolver unit tests covering the fact matrix — an all-`unknown`
fact set resolves to `single` (not `partitioned`, not `lead-only`); a positive
correctness/fit/test signal produces the matching partition (assert at the
`implementReviewPartitions` list level, since a *single* partition still
collapses to `single` at `deriveImplementReviewAlloc` — use ≥2 signals to see
`partitioned:` at the alloc level); a genuine all-`low` set still qualifies for
`lead-only`; a mixed set adds only the signalled partitions. Update the spec
prose (see Spec Impact) to match.

### Result (fe940130) - 2026-08-25

`materialRisk` now treats only `moderate`/`high` as material (`unknown` dropped).
`implementReviewPartitions` no longer fires the fit arm on `ReusePoints ==
"unknown"` or the test arm on `TestSurface == "unknown"`; each of the three arms
requires a positive signal (moderate/high risk, new type contract / public
symbol, `cross-module` surface, `ReusePoints == "unconfirmed"`, or `TestSurface
== "new-files"`). An all-`unknown`/un-derived fact set therefore yields zero
partitions and resolves to `single` via `deriveImplementReviewAlloc`.
`automaticLeadOnlyReviewEligible` was left untouched — it still requires genuine
`low` on all four risk facts — so the same all-`unknown` set lands on `single`,
never `lead-only`. The review-cycle budget wiring (260726) was not touched.

- Verification: `go test ./internal/mcp/...` pass (22.5s); `go vet` clean. New
  coverage: `TestImplementReviewPartitionsTreatUnknownAsNonSignal`,
  `TestAutomaticLeadOnlyReviewEligibleRequiresGenuineLow`, and two table cases in
  `TestDeriveImplementReviewAllocProportionalPartitions` (all-unknown→single,
  ≥2 signals→partitioned, all-low→lead-only, mixed→only signalled partitions).
- Review: correctness `clean`; fit `non-clean: 1 important` — the omitted spec
  prose sync — resolved in the doc pre-pass (spec commit 2b5b6be6). No unresolved
  findings carried.
- Deviation: the `## Spec Impact` sync landed as a separate `docs(spec)` commit
  (2b5b6be6) during the doc pre-pass rather than inside the code commit; the fit
  reviewer flagged its absence from `fe940130`, which is the expected runbook
  ordering (code then doc), not a defect.

## Spec Impact

Target: the automatic-review-allocation description at
`ai-docs/spec/mcp-tools.md` (the review-allocation paragraph for
`enter.implement`, currently around the "Automatic review allocation derives
independent correctness, fit, and test partitions from material risk…"
sentence). Expected caller-visible change: `unknown`/un-derived facts no longer
add a partition on any arm; a partition requires a positive signal only —
`moderate`/`high` risk, new type-contract / public symbol, `cross-module`
surface, `ReusePoints == "unconfirmed"`, or `TestSurface == "new-files"` (the
current prose's "reuse uncertainty" and "unknown test surfaces" wording goes
stale and must be updated). `single` and `lead-only` semantics are otherwise
unchanged.

---
title: "review_phase lite tier as the default: one-round correctness and test-integrity review"
related:
  261001-feat-opt-in-design-and-phase-review: prerequisite (landed on develop, unreleased); this ticket revises its review_phase default and value domain
  260909-research-ws-refoundation-evidence-audit: binding anchor (topics worker interpreter, stop conditions); its 261001 addendum is updated here
---

# review_phase lite tier as the default: one-round correctness and test-integrity review

## Background

`261001-feat-opt-in-design-and-phase-review` added `review_phase` (`on|off`,
builtin `off`). `on` keeps the original risk-keyed allocation
(`single` or `partitioned: correctness, fit, test`) and the two-round
protocol. Re-examining the default right after that merge, the owner and the
lead concluded that `off` leans too hard on two things: the user reading the
diff, and the advisory `lead-review` sweep. Two reasons:

- **What a sprint-style human review replaces.** A human skimming a diff
  catches fit problems (conventions, structure, style) quickly. It rarely
  checks what test assertions actually prove, and it misses subtle
  correctness defects. Those are the implementer's structural blind spots:
  the same author writes and verifies, so tests confirm what was built
  (implementation-mirroring assertions, always-passing checks, mocks that
  bypass the real path). The `code-review-test` checklist (tautological
  assertions, unreachable asserts, mock integrity) targets exactly this.
- **This session's own evidence (n=1 each).** The design review caught a
  dead-by-default epic path that the discussion missed. The worker's
  round-1 code review caught an Important test gap the implementer missed.

The `lead-review` sweep is a weaker net for low-quota users: the
watermark nudge is advisory, low-quota users are the likeliest to skip
sweeps, and a range review reads a large diff at lower per-line density,
finding defects later when fixes cost more.

`review_phase` is not in any release yet (no tag contains the merge
`02f402d0d`; latest tag `v0.46.24`), so its value domain can change without
a compatibility alias.

## Decisions

1. **Value domain `off | lite | full`, builtin default `lite`.** `full` is
   exactly today's `on` behavior (risk-keyed allocation, two rounds,
   stop (e) after round 2). `off` is today's `off`.
   *Rejected: keep builtin `off`* - see Background.
   *Rejected: default `full`* - its cost (up to three reviewers, two rounds,
   per phase) is the cost problem `261001-feat-opt-in-design-and-phase-review`
   was opened for.
2. **`lite` is one fresh reviewer, one round, medium tier, covering the
   correctness and test-integrity checklists; fit is excluded.** Fit is the
   part a human diff skim replaces. Medium is enough because test deception
   is close to pattern recognition, and the goal is low-quota cost.
   *Rejected: large tier for lite* - better on subtle correctness, but
   against the cost goal; heavy users who want it pick `full`.
3. **`lite` ignores the risk partition.** Under `lite` the allocation is
   always the single lite reviewer, whatever the route facts say. An
   explicit `policy.review.override` of `single` or `partitioned` still wins
   over the knob (Decision 8 of the prerequisite, unchanged).
4. **No compatibility alias for `on`.** The knob is unreleased.
5. **Naming.** The new wrapper is `code-review-lite` and the allocation
   value is `lite`, following the existing `code-review-*` wrapper and
   allocation naming.
6. **Lite fix pass and stops.** Under `lite` the worker fixes Critical and
   Important findings in one pass with no re-review. A Critical the worker
   cannot fix is stop (e), so the elevation ladder stays reachable; an
   Important it does not fix goes to `unresolved:` with a reason. The worker
   judging "I could not fix it" is not grading its own fix as correct, so
   this does not reintroduce self-review.
   *Rejected: an unfixable Critical goes to `unresolved:` with no stop* - it
   would bypass the ladder that exists for exactly this case.
7. **The binding-anchor addendum is amended in place.** The prerequisite's
   dated addendum in the research anchor is not a frozen Result; it is
   updated to name `lite` as the default.

## Constraints

- Read every manual the `AGENTS.md` `### Implementation Conventions` rows
  match before editing (`shipped-surface-boundary.md`, `skill-authoring.md`,
  `wsflow-mirroring.md`, `ws-mcp.md`). Shipped text names no devenv-only
  path or ticket.
- Regenerate `agents-plugin-wsflow/rsrc/` and the manifests per
  `wsflow-mirroring.md`; do not hand-edit mirrors.
- `ticket-worker.md` and `ticket-worker-elevated.md` change together.
- The shared reviewer contract (`code-reviewer.md`), the existing
  `reviewer` and `code-review-*` wrappers, `lead-review`, and the design
  and completeness knobs are unchanged.

## Phases

### Phase 1: lite tier and default

- `wsconfig` / `config.tune` registry: `review_phase` domain becomes
  `off|lite|full`, builtin `lite`; update the `config.list` description and
  `lead-tune.md`'s catalog line (it names `on`/`off` today).
- Resolver (`implement_resolver.go`): normalize to `off|lite|full`; `full`
  is today's `on` path; `lite` allocates a new allocation value for the lite
  reviewer (Decision 3). Update the review-phase verdict condition, the
  review-off and review-on instruction strings, and their golden tests.
  Existing tests that enable review switch from `on` to `full`.
- New render wrapper `code-review-lite` (`kind: render`, `delegates: true`,
  `tier: medium`), including the shared `code-reviewer.md` contract, with
  the correctness and test checklists from `code-review-correctness` and
  `code-review-test`.
- `ticket-worker.md`, `ticket-worker-elevated.md`: map the lite allocation
  to `code-review-lite`.
- `worker-stop-protocol.md`: Review Rounds describes the lite round, and
  the stop (e) wording covers both the `full` case (Critical open after
  round 2) and the `lite` case (a Critical the worker could not fix)
  (Decision 6).
- Binding anchor `260909-research-ws-refoundation-evidence-audit`: amend
  the 261001 addendum so it says the default per-phase review is `lite`.

Verification: Go tests showing (i) builtin resolves `lite` and allocates
the lite reviewer regardless of route-fact risk, (ii) `full` reproduces the
previous `on` allocation table, (iii) `off` yields no review, (iv) an
explicit `policy.review.override` wins under each value, (v) `config.list`
shows `off|lite|full` with default `lite`, (vi) `on` is rejected as an
unknown value, (vii) the lite verdict's installed review todo states the
one-pass rule and the unfixable-Critical stop. Full `go test ./...` for `agents-plugin-tool` and the plugin
package tests after wsflow regeneration.

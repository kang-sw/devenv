---
title: plan-populator survey unilaterally reduces a fully-specified requirement to a subset (runtime-fallback misread as implementation-fallback)
related:
  260729-bug-survey-plan-drops-verbatim-contract-text: sibling survey-plan fidelity gap — different failure (verbatim-marked text summarized out) and different fix; linked, not absorbed
  260831-refactor-severity-graded-per-slice-review-relay: sibling review-load lever (weight); this fix must not re-add always-on ceremony that lever removed
related-mental-model:
  - workflow-skills
sage-review-design: completed
sage-review-design-reviewed: 356941a69752e3ec
sage-review-completeness: completed
sage-review-completeness-reviewed: 63ca190ef9f17e0b
completed: 2026-08-31
---

# plan-populator survey unilaterally reduces a fully-specified requirement to a subset (runtime-fallback misread as implementation-fallback)

## Background

A downstream project running this ws workflow reported an "intent drift": on a
ticket whose phase specified a multi-part requirement, `plan-populator-survey`
reduced it to a single sub-clause and justified the reduction in the plan as a
"ticket-traceable first cut." The dropped elements included the ticket's primary
path; only a narrow runtime-fallback sub-clause survived into the plan, and the
implementer (which never sees the ticket, only the plan) faithfully built that
subset.

Root mechanism — a modality/category error, not an importance-ranking error.
The ticket expressed a **conjunctive** requirement: build A (primary, targeting a
niche) *and* B (fallback), and at runtime select B when A does not apply. This is
a **runtime fallback** — a required execution branch (graceful degradation). The
planner read it as an **implementation fallback** — a permitted scope shortcut
("first cut") — and built only B.

Two compounding factors, confirmed against source (survey of the delegate
prompts and the implement flow):

- **Homonym trap in the delegate's own vocabulary.**
  `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md` lists
  "fallback behavior" and "temporary implementation paths" as *shortcut-risk
  signals* — i.e. within the planner's charter "fallback" already denotes a
  permissible reduced path. When a ticket uses "fallback" in the
  runtime-composition sense, the two senses collide and the planner is primed to
  treat a required runtime branch as a droppable shortcut.
- **No guard on unilateral scope reduction.** The survey delegate's only
  escalation channel is `[escalate-to-research]`, scoped to strategy/contract
  *uncertainty*. (The research delegate already carries a second, lead-directed
  channel, `[escalate-to-lead]`, but it is scoped to shortcut-blocked targets, not
  to disclosing a *confident* scope reduction.) So there is no channel for "I am
  confidently choosing to implement a subset of a fully-specified requirement,"
  and the planner performs the reduction silently and labels it "first cut" — a
  phrase its own charter blesses. Nothing between prep
  and edit re-checks the plan's reading of the ticket; the implementer receives
  only `PlanPath`; and the partitioned reviewers check phrase-traceability
  (correctness), codebase-belonging (fit), and assertion validity (test), none of
  which owns "every specified requirement element survived."

Scope note: the drift here entered purely at plan authoring — the source ticket
correctly specified both A and B as required, so this is not a ticket-authoring
defect. Ticket-authoring-side over-elaboration and per-clause consent are a
separate concern raised in the same discussion and are out of scope here.

## Decisions

- **Fix locus is plan-side (survey/research delegate prompts + the shared
  reviewer frame), not a new lead checkpoint.** Rejected: a mandatory lead
  adjudication step between prep and edit (the downstream report's original
  suggestion). It re-adds always-on ceremony directly against the just-shipped
  per-slice weight reduction (`260831-refactor-severity-graded-per-slice-review-relay`)
  and the granularity-load direction, and it conflicts with the project doctrine
  that upstream prevention is cheaper and more fundamental than a new signal.
- **No structural modality tag in the plan contract template (deferred).**
  Rejected for now: adding a mandatory / runtime-conditional / deferrable modality
  field to `## Relevant Ticket Contract`. The behavioral rule in Phase 1 is the
  direct-cause fix and is cheaper; escalate to the structural tag only if the
  behavioral rule proves to rely too heavily on the planner recognizing modality
  it was never forced to record.
- **`260729` is linked, not absorbed.** It is a different mechanism (verbatim-
  marked contract text summarized out, breaking plan self-containment) with a
  different fix (literal transcription). Dropping or generalizing it is out of
  scope here.

## Phases

### Phase 1: Forbid unilateral down-scoping in the survey/research planners and add a reviewer-frame coverage check

Three changes, all round-0 (prompt/rsrc edits and an operationalized existing
check; no new workflow stage):

1. **Behavioral rule — survey + research delegate prompts.** In
   `agents-plugin/rsrc/plan-populator-survey/plan-populator-survey.md` and
   `agents-plugin/rsrc/plan-populator-research/plan-populator-research.md`: a
   fully-specified, multi-part requirement must be carried whole into the plan;
   the planner may not silently implement a subset. If the planner judges that
   only a subset should be built this slice, that is a **scope-reduction decision
   for the lead**, surfaced as an explicit escalation *distinct from*
   `[escalate-to-research]` (which stays scoped to strategy/contract
   uncertainty). A "first cut" / phased subset is legitimate only when the ticket
   or lead already authorized the phasing — never when the planner invents it.
   Suggested approach: model this on the research delegate's existing
   `[escalate-to-lead]` channel and extend that lead-directed signal to the survey
   delegate and to the confident-subset case, rather than minting an unrelated
   third token; the exact token/wording is an implementation choice, but it must
   stay separate from `[escalate-to-research]`.
2. **Terminology hygiene — same two prompts.** Distinguish in the prompt text a
   **runtime fallback** (a required execution branch / graceful degradation —
   must be built) from an **implementation fallback** (a scope shortcut / temporary
   path the planner may not take unilaterally). Ensure the "shortcut-risk signals"
   list cannot be read as licensing the omission of a ticket's required runtime
   branches.
3. **Reviewer-frame coverage check — cheap second net.** Operationalize the
   shared reviewer frame's existing "Binding authority decisions were not omitted
   or violated" line (`agents-plugin/rsrc/lead-implement/lead-implement.md`) into a
   concrete obligation: *each specified authority requirement is either implemented
   or carries an explicit, authorized deferral.* Phrase it against "authority," not
   "ticket," so it binds both reviewer-frame modes (`Authority: Ticket path` and
   `Authority: Inline contract`) — the inline mode is exactly the surface where the
   sibling `260729` verbatim-drop occurred and where an inline review is told not to
   read a ticket. This catches a dropped primary element at review even when it
   slips the planner.

Constraints:
- These are `agents-plugin/rsrc/` edits: apply the wsflow mirror + manifest
  regen per `ai-docs/manuals/wsflow-mirroring.md` (byte-identity mirror plus
  manifest.json hashes).
- If any binding execution text in
  `agents-plugin-tool/internal/mcp/session_state.go` references the reviewer
  coverage line, preserve the existing shared-clause convergence pattern rather
  than forking per-branch copies.
- Do not widen `[escalate-to-research]`'s meaning; the scope-reduction escalation
  is a separately named signal.

Verification:
- Delegate-prompt rendered/golden tests assert (a) the down-scope-forbidden rule
  text is present in both plan-populator surfaces, (b) the runtime-vs-
  implementation-fallback distinction is present, and (c) `[escalate-to-research]`
  remains uncertainty-scoped (not the scope-reduction channel).
- Reviewer-frame rendered test asserts the concrete per-requirement coverage
  obligation is present.
- wsflow mirror byte-identity + manifest-hash tests green; full Go suite green.

Deliverable (spec/doc pass, not optional drift-on-contact): update the
`{#260505-implementation-workflow-skills}` anchor in
`ai-docs/spec/workflow-skills.md` (the section covering the plan-populator
survey/research contract) to record the new lead-directed scope-reduction
escalation signal and the reviewer per-requirement coverage obligation; reconcile
the `workflow-skills` mental-model on contact.

### Result (4e795761) - 2026-08-31

All three changes landed as planned. The suggested approach held: rather than a
new token, the survey delegate gained a lead-directed `[escalate-to-lead]`
scope-reduction signal (distinct from the un-widened `[escalate-to-research]`),
and the research delegate's existing `[escalate-to-lead]` was broadened to cover
the same confident-subset case. Both planners now distinguish an implementation
fallback (a forbidden unilateral shortcut) from a ticket's required runtime
fallback (must be planned in full), closing the homonym trap. The reviewer-frame
line reads "Each specified authority requirement is implemented, or carries an
explicit, authorized deferral" — phrased against `authority` so it binds both
`Authority: Ticket path` and `Authority: Inline contract` modes.

`session_state.go` was left untouched (it holds only a name-pointer to the
reviewer frame, not the coverage-line text), so the shared-clause convergence
constraint was satisfied without any Go source change beyond test assertions.
Golden/rendered tests now lock the down-scope rule and the new signal, assert
`[escalate-to-research]` stays uncertainty-scoped, and forbid the removed vague
line. Docs reconciled in-range: spec `{#260505-implementation-workflow-skills}`,
mental-model `workflow-skills` (new local anchor
`{#260831-forbid-unilateral-scope-reduction}`) and `prompt-bundle`.

Verification: full Go suite (13 packages) green, plus wsflow mirror byte-identity
and manifest-hash drift tests. Partitioned review: correctness / fit / test all
clean. One Minor recorded, not fixed: the new mental-model anchor
`{#260831-forbid-unilateral-scope-reduction}` resolves to no spec anchor, but
this follows a tolerated local convention (five pre-existing unresolved
mental-model anchors; `spec_index_verify` ok), so it was left as-is rather than
break the pattern. Commit range `049e31af..5cad50c0` (feat `4e795761`, spec
`3f8d7bc4`, mental-model `5cad50c0`).

## Spec Impact

The caller-visible workflow behavior added is (1) a new planner escalation signal
for a confident, unilateral scope reduction, distinct from `[escalate-to-research]`,
and (2) a reviewer obligation that every specified authority requirement is
implemented or explicitly, authorizedly deferred. This is addressed by the
`{#260505-implementation-workflow-skills}` anchor in
`ai-docs/spec/workflow-skills.md`, which already governs the plan-populator
survey/research contract; the Phase 1 doc-pass deliverable above updates that
anchor to cover both additions. No new spec stem is created and none is removed.

## Non-Goals

- A new lead adjudication checkpoint between prep and edit.
- Structural modality tagging in the plan-contract template (deferred lever, used
  only if the Phase 1 behavioral rule proves insufficient).
- Ticket-authoring-side over-elaboration or per-clause consent gating (separate
  concern from the same discussion).
- Dropping or generalizing `260729-bug-survey-plan-drops-verbatim-contract-text`.

---
title: Severity-graded per-slice review relay — restore Critical iteration + elevate, lighten the rest
parent: 260824-epic-review-watermark-model
related:
  260828-refactor-per-slice-review-relay: reshapes its landed one-relay model — replaces the uniform single-relay cap with a severity-graded budget
  260726-bug-lead-implement-lost-review-relay-cycle-cap: restores its multi-cycle iteration + elevate, scoped to Critical only (adjudicator routing not restored)
related-mental-model:
  - workflow-skills
  - mcp-runtime
spec:
  - 260612-reviewer-allocation-tier-default
sage-review-design: completed
sage-review-design-reviewed: 3f98ca423ccd9194
sage-review-completeness: completed
sage-review-completeness-reviewed: 3f98ca423ccd9194
---

# Severity-graded per-slice review relay — restore Critical iteration + elevate, lighten the rest

## Background

The landed per-slice review model (`260828-refactor-per-slice-review-relay`,
commit `4575f634`) replaced the multi-cycle relay budget with a uniform **one
repair relay after review #1, then closeout**, plus a Critical-only branch of
`one relay -> Critical-scoped review #2 -> hard stop` if a Critical still
stands. Two behavioral guardrails were dropped together with the multi-cycle
*routing machinery*:

1. **Ceiling semantics flipped from complete-the-run to halt.** `260726`'s
   contract was "the final cycle completes the run; it does not halt it"; the
   landed model instead hard-stops an unresolved Critical (no merge). In
   dogfooding this makes implementation runs abort more often.
2. **Iteration depth for Critical was cut to one relay + one scoped re-review.**
   A genuinely hard Critical no longer gets the runway (or the escalation to a
   stronger implementer) it had under `260726`.

The observed pain is a *product*, not a single cause: the AI tends to split a
ticket into many granular phases, and each phase then draws the full per-phase
review weight. Uniform per-finding weight × many phases is unmanageable. The
right lever is to make review weight **track finding severity** so a typical
phase (mostly Minor/Important findings) is cheap, while the rare Critical that
must not escape gets restored iteration. This *lowers* typical per-phase load
relative to the uniform one-relay model while removing the stall.

This does not overturn epic `260824`'s "keep per-phase review light" thesis — it
recalibrates child ⑤ (`260828`) to be severity-keyed rather than uniform. Per
the epic, per-phase findings are fixed inline before the slice lands; the
sweep/gate ticketizes cross-slice accumulation. This ticket keeps that split:
Critical is an inline must-fix; Important/Minor stay light and lean on the
integration net for accumulation.

## Decisions

Severity-graded relay budget (replaces the uniform single relay):

| Severity | Fix obligation | Relay budget | On remaining non-clean |
|----------|----------------|--------------|------------------------|
| **Critical** | must-fix | bounded **3 review rounds** (review #1 + up to 2 re-reviews = up to 2 relays) | **unconditional elevate** to `implementer-elevated`; elevate owns final resolution — no hard stop |
| **Important** | best-effort | at most **1 relay** | record `[not fixed: <reason>]` in the fix commit `## AI Context`; not blocking |
| **Minor** | note only | **0 relays** | recorded in the review summary / commit only |

- **Relay-round accounting.** A relay round is one implementer dispatch that
  dispositions every non-clean finding of the current review at once — the
  per-severity budget governs how many *rounds/re-reviews each severity may
  drive*, not separate parallel relays. Concretely: relay #1 handles all
  severities; Important consumes its single relay there and is **not**
  re-reviewed, so its post-relay `[not fixed]` state is the implementer's
  self-reported disposition, not a re-review verdict. Only a Critical from
  review #1 drives the Critical-scoped re-review (review #2) and, if still
  non-clean, the second Critical relay before the elevate ceiling. Minor never
  drives a relay round.
- **Ceiling = elevate, not halt.** The user chose restoring the original
  unconditional-elevate behavior over defining a narrow "cannot-proceed" halt
  class. A Critical unresolved after the bounded budget escalates to a stronger
  implementer (`implementer-elevated`) and the run continues; it does not stop
  the slice with a durable-evidence hard stop.
- **Disposition durability defaults to the commit message, not a ticket.**
  Unresolved Important/Minor findings are recorded in the fix commit
  `## AI Context` (`git log --grep` recoverable), reusing the existing
  per-finding disposition record. **Ticketization is a lead-judgment exception**
  for a genuinely worth-tracking follow-up, never the default — this is the
  explicit guard against ticket explosion the user flagged.
- **Restore `implementer-elevated`, not `review-adjudicator`.** Only the
  stronger-implementer escalation path is revived, reachable *only* at the
  Critical ceiling. The cycle-counting/re-review *routing* delegate
  (`review-adjudicator`) stays dormant; its adjudication role is lead-owned in
  the shared clause. Removing that routing machinery was the genuine diet win
  and is preserved. (Its orphan-deletion is tracked separately.)
- **Keep `260828`'s shared-clause convergence.** The `single`, `partitioned:`,
  and bare-`partitioned` allocations continue to share one clause set; the
  severity-graded budget + ceiling become parameters of that shared clause. The
  `lead-only` branch stays untouched and relay-vocab-free.

Rejected alternatives:
- **Uniform bounded-3** (an earlier proposal in discussion): makes every finding
  heavy again and worsens the load × phase-granularity product. Rejected in
  favor of severity grading.
- **A "cannot-proceed" halt class** as the ceiling: the user judged unconditional
  elevate the cleaner original behavior; a bespoke blocker-severity gate is not
  introduced.
- **Reviving `review-adjudicator`**: its multi-cycle routing was the fat the
  diet correctly cut; iteration is restored via a parameterized relay budget
  instead.

## Constraints

- The generated review todo `Instruction` is the binding execution surface, not
  the playbook prose. The severity grading must land in the runtime that
  generates that Instruction (`implementReviewInstruction` and its relay/
  disposition/Critical-branch clause consts in
  `agents-plugin-tool/internal/mcp/session_state.go`); changing
  `lead-implement.md` prose alone is insufficient (the constraint `260828`
  already established).
- `lead-only` remains a no-delegate path and must not gain relay vocabulary.
- Preserve the settled disposition-marker vocabulary
  (`[fixed]` / `[won't fix: …]` / `[deferred: …]` / `[escalate: …]`) and add the
  Important record marker `[not fixed: …]`; do not introduce a new severity
  label (`Critical`/`Important`/`Minor` stay the per-slice vocabulary).
- Preserve the risk-based reviewer allocation resolver (`review_alloc`); this
  ticket changes post-allocation loop behavior only.
- After editing any `rsrc` file, regenerate `agents-plugin-wsflow/rsrc`
  byte-for-byte (`WS_REGEN_WSFLOW_RSRC=1`) and both `manifest.json` hashes
  (`WSRSRC_REGEN=1`) per `ai-docs/manuals/wsflow-mirroring.md`.
- This is an atomic change: do not land the Important/Minor lightening without
  the Critical iteration+elevate path in the same slice, so no intermediate
  release weakens Critical verification.

## Prior Art

- `260726-bug-lead-implement-lost-review-relay-cycle-cap` (.done) — the
  multi-cycle (3 review rounds) model with `review-adjudicator` (cycle-2) and
  `implementer-elevated` (capacity/root-cause escalation) delegates, and the
  "final cycle completes the run; it does not halt it" ceiling. This ticket
  restores its iteration + elevate for the Critical path only.
- `260828-refactor-per-slice-review-relay` (.done, `4575f634`) — the uniform
  one-relay model this ticket reshapes; its shared-clause convergence is kept.
- The dormant delegate playbooks live under `agents-plugin/rsrc/` (search
  `implementer-elevated` and `review-adjudicator`).

## Phases

### Phase 1: Replace the uniform relay cap with a severity-graded budget

- Rework `implementReviewInstruction` and its clause consts in
  `session_state.go` so the generated review Instruction expresses the
  severity-graded budget above across the `single`, `partitioned:`, and
  bare-`partitioned` allocations, differing only in reviewer-dispatch wording;
  `lead-only` unchanged.
- Critical path: bounded 3 review rounds (up to 2 relays), then unconditional
  elevate to `implementer-elevated`; remove the `260828` Critical hard-stop.
  Re-activate the `implementer-elevated` routing prose in `lead-implement.md`
  scoped to this ceiling only; leave `review-adjudicator` unreferenced.
- Important path: at most one relay; remaining non-clean Important recorded as
  `[not fixed: <reason>]` in the fix commit `## AI Context`, not ticketized by
  default.
- Minor path: no relay; recorded in the review summary only.
- Reconcile `lead-implement.md` review prose (Review invariants, Review relay
  dispatch, Re-review prompt) with the graded model; regenerate the
  `agents-plugin-wsflow/rsrc` mirror and both `manifest.json` hashes.
- Update spec anchor `{#260612-reviewer-allocation-tier-default}` (and the
  `{#260619-stateless-implement-review-continuity}` backstop sentence) in
  `ai-docs/spec/workflow-skills.md` to the severity-graded model; update the
  mental-model `review-adjudicator`/`implementer-elevated` bullets
  (`implementer-elevated` becomes reachable-at-Critical-ceiling, adjudicator
  stays dormant).

Verification: generated instructions plus focused tests
(`session_state_test.go`, `playbook_tools_test.go`) prove that Critical issues
up to 2 relays across 3 review rounds then elevates (never hard-stops), Important
issues at most one relay then records `[not fixed]`, Minor issues zero relays,
the disposition-marker vocabulary is preserved, no default-path auto-ticketization
appears, `lead-only` stays relay-vocab-free, and the three relaying allocations
share the graded clause set. `go build ./... && go vet ./... && go test ./...`
green; `agents-plugin-wsflow` python tests pass; regenerated manifests verified.

## Spec Impact

Target: the implementation-review contract in
`ai-docs/spec/workflow-skills.md` (`{#260612-reviewer-allocation-tier-default}`,
with the `{#260619-stateless-implement-review-continuity}` backstop). Expected
caller-visible change: per-slice review weight becomes severity-graded — Critical
regains a bounded multi-round budget plus unconditional elevate (replacing the
one-relay-then-hard-stop), Important is a single best-effort relay then a
`[not fixed]` record, and Minor drives no relay; unresolved findings default to a
commit-message record rather than a ticket.

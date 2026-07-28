---
title: lead-implement lost the review-relay cycle cap, so a fix loop has no numeric
  backstop and no defined escalation point
related:
  260619-refactor-stateless-implement-review-continuity: its D6 records the cap as
    "the existing hard cap" and lists lead-implement.md as updated with the
    convergence invariants; only the dedup half landed
  260503-feat-agents-plugin-write-code-port: records "Review relay remains capped at
    three cycles" for the skill lead-implement succeeded
  260525-bug-implement-review-fix-owner: its constraints say re-review and cycle caps
    stay equivalent unless the implementation finds reason to change them, so the cap
    was expected to survive that refactor too
---

# lead-implement lost the review-relay cycle cap

## Background

Found by dogfooding `ws:lead-implement` on Phase 1 of
`260726-refactor-ws-dashboard-git-fs-watch-invalidation` (2026-07-26). The lead
ran three review-relay cycles and was about to dispatch a fourth; the user asked
whether a hard cap existed. It does — but not anywhere the executing lead can
see it.

Documented intent, in two places, both non-live:

- `260619-refactor-stateless-implement-review-continuity` (`.done`), decision D6:
  "the existing hard cap (partitioned 3 cycles / single 2, lead adjudicates at
  cycle 2, caller escalation at cycle 3) is the backstop for the pathological
  case of a reviewer inventing new distinct findings each cycle — where hitting
  the cap is then a true 'contentious' signal."
- `ai-docs/.old/spec/260505/workflow-skills.md`: "Runs a relay loop (max 3
  cycles) … At cycle 2, the lead adjudicates maintained disputes; at cycle 3,
  unresolved disputes escalate to the caller."

Live text, verified by grep over `agents-plugin/rsrc/`:

- `agents-plugin/rsrc/lead-implement/lead-implement.md` contains **no numeric
  cycle cap and no escalation point**. Its only convergence invariants are the
  dedup ones: "Use Review relay and Re-review prompts only for genuinely new
  non-clean Critical/Important findings" and "Preserve settled or deferred
  dispositions".
- The only live "Max three cycles" in the whole `rsrc/` tree is
  `lead-skill-authoring.md:120`, which governs skill-authoring review, not code
  implementation review.

So `260619` shipped the dedup half of D6 and left the numeric backstop half
unwritten in the very playbook it lists as updated. The cap most likely dates
from the `write-code` era (`260503-feat-agents-plugin-write-code-port`: "Review
relay remains capped at three cycles") and was dropped when `write-code` became
`lead-implement`.

## Why it matters

Dedup and the cap guard different failure modes, so the surviving half does not
cover the lost half:

- Dedup stops **re-litigation** — the same finding returning after being settled.
- The cap stops **divergence** — each cycle producing genuinely new, genuinely
  valid findings with no convergence in sight. Dedup permits that loop
  indefinitely, because every finding legitimately passes the "is this new?" test.

The observed run is exactly the second case: cycles 1, 2, and 3 each returned a
new, real, distinct Critical/Important finding, all in the same function, and one
of them was caused by the *lead's own* prior-cycle disposition rather than by
implementer error. Nothing in the live playbook says when that stops being a fix
loop and starts being a signal to escalate.

Secondary loss: D6 frames hitting the cap as "a true 'contentious' signal" — the
cap is not only a budget guard but the mechanism that converts a churning review
into an explicit owner decision. Without it there is no defined moment at which
the lead must hand the design question back.

## Topics

### Direction: put the cap clause in the review todo's Instruction text

Owner direction (2026-07-26), idea-level: the cap language should land in the
**review-step description of the todo checklist the lead executes from**, not
only in playbook prose. Exact wording is TBD.

What makes this the correct surface rather than merely a convenient one:
`lead-implement.md` instructs the lead to "Treat the installed todo list as the
ordered runbook" and says "each todo's `Instruction` field is a complete how-to
for that step — **do not restate or supplement it from memory**." Under that
contract, a rule absent from the review todo's Instruction is a rule the lead is
positively instructed not to supply. That is why the observed run had no cap in
effect: not because the lead could not have known of one, but because the runbook
it was told to follow exhaustively did not carry it.

Edit surface, verified: this text is generated, not authored in markdown —
`implementReviewInstruction` in
`agents-plugin-tool/internal/mcp/session_state.go:563-577`. It already branches
on `verdict.ReviewAlloc`:

- `:568` lead-only review
- `:571` `partitioned:` — receives the partition list via `formatReviewPartitions`
- `:574` `single` — already worded differently ("Relay only new non-clean …")
- `:576` fallback

Two consequences worth carrying into implementation:

- The generator already knows which of the two documented budgets applies, so the
  emitted text can state it directly instead of leaving the lead to infer it.
  That settles the per-partition question below **at the point of generation**,
  which is the cheapest place to settle it.
- The change is localized to three of four branches plus the string pins in
  `session_state_test.go`.

Constraints on doing it: the changed lines are prompt text, so
`agents-plugin/skills/lead-skill-authoring/SKILL.md`'s invariant checklist
applies to each one; and because the change touches `agents-plugin-tool/`, the
dev-merge version bump through `agents-plugin-tool/scripts/bump-ws-version.sh`
applies.

Wording questions left open on purpose:

- Whether the clause states a number, or states the escalation *condition* (a
  finding whose root cause repeats an already-relayed one) and leaves the number
  as a pure backstop. The observed run argues the condition is the load-bearing
  half — the count only caught it incidentally.
- Whether it names what happens at the cap (stop and hand the design question to
  the user with the disposition record) or only that the loop stops. D6 treats
  reaching the cap as a "contentious" signal, which implies the former.
- Whether the `single` branch needs the same clause at its own budget, or whether
  its existing shorter wording is deliberate.

### Where the cap belongs, and whether it is per-partition

`260619` states it as "partitioned 3 cycles / single 2", and its own summary says
"per-path cycle cap as backstop", which reads as per-review-path rather than
per-slice. Worth settling before writing text: with `review_alloc: partitioned:
correctness, test`, is the budget 3 cycles per partition or 3 for the slice? The
observed run had a clean test partition and three correctness cycles, so the two
readings differ materially.

Per the topic above this is answerable inside `implementReviewInstruction`, which
holds the allocation at generation time — so the decision needs to be made once,
in text, rather than re-derived by each lead.

### Whether the impl-playbook repetition check already covers it, and why it did not fire

`impl-playbook` carries a stronger, non-numeric rule: "**Repetition check**
(mechanical, every failure): Before fixing, ask and answer: 'Is this the same
root cause as a previous failure in this session?' If yes, stop and escalate
immediately. Do not attempt another fix."

In the observed run that rule should have fired at cycle 3 — all three findings
share one root cause (bounding a subprocess's output collection). It did not fire
because the rule is addressed to the *implementer* handling a test failure, and
each relay went to a fresh implementer spawn with no view of prior cycles. The
lead, the only party that sees the whole sequence, has no equivalent
instruction.

That suggests the fix is not only a number in `lead-implement.md` but a
lead-side root-cause check: relaying a finding whose root cause matches an
already-relayed finding is itself the escalation signal, independent of cycle
count.

### Interaction with stateless dispatch

`260619`'s premise is that relay dispatch is stateless and self-contained. A
lead-side root-cause check is compatible with that, since the lead holds the
disposition record — but a cap phrased as implementer-visible state is not. Any
new text must keep the delegate prompt self-contained.

## Non-Goals

- Changing what counts as `clean`. `260619` D5 explicitly rejected a machine gate
  ("no Critical/Important == clean") in favor of lead judgment; this ticket adds a
  stopping rule, not an automated verdict.
- Reintroducing the retired `.old/spec/260505` text wholesale.
- Blocking the dashboard FS-watch ticket. That work continues; this ticket only
  records why its Phase 1 review loop ran to the cap without anything noticing.

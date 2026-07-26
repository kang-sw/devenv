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

### Where the cap belongs, and whether it is per-partition

`260619` states it as "partitioned 3 cycles / single 2", and its own summary says
"per-path cycle cap as backstop", which reads as per-review-path rather than
per-slice. Worth settling before writing text: with `review_alloc: partitioned:
correctness, test`, is the budget 3 cycles per partition or 3 for the slice? The
observed run had a clean test partition and three correctness cycles, so the two
readings differ materially.

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

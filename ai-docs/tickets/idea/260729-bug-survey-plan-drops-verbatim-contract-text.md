---
title: plan-populator-survey paraphrases verbatim-mandated contract text out of the plan
related:
  260605-research-ws-native-subagent-pivot: plans are the self-contained artifact a fresh delegate reads; this is a hole in that premise
related-mental-model:
  - workflow-skills
---

# plan-populator-survey paraphrases verbatim-mandated contract text out of the plan

## Background

Observed while dogfooding `lead-implement` on the `lead-prefer-subagent`
delegate-continuation change (merge `82e5f6ff`).

The lead passed an `inline_contract` to `plan-populator-survey` containing an
`EXACT REQUIRED EDITS` section with two user-approved paragraphs marked verbatim
("copy character-for-character"). The survey wrote a high-quality plan — but it
**summarized** those paragraphs into `## Relevant Ticket Contract` bullets
("Required edit 1 (verbatim, user-approved): ... the two-sentence replacement
given in the contract") while its own `## Implementation Plan` step 2 still said:

> both quoted in full in the "EXACT REQUIRED EDITS" section of the inline
> contract — copy them character-for-character

No such section existed in the plan artifact. The inline contract is visible to
the survey agent but **not** to the fresh implementer, so the plan pointed at
text that existed only in the lead conversation.

The delegated implementer caught it, refused to fabricate prose for an edit
marked verbatim, and stopped with a correct escalation. Cost was one wasted
implementer dispatch plus a lead-authored plan fix commit (`5bbee455`).

## Why this is worth a ticket

The failure is silent at the point it occurs. The survey returned
`confidence: high`, no `[escalate-to-research]`, and a plan that reads complete —
the omission is only detectable by an agent that lacks the contract, i.e. one
step too late. A less careful implementer would have paraphrased the wording,
and a verbatim, user-approved prose contract would have shipped altered with
nobody noticing.

## Suspected shape of the fix

Not yet decided; options seen so far:

- Make `plan-populator-survey`'s playbook state that any contract text marked
  verbatim/exact must be transcribed into the plan literally, never summarized —
  the plan is the only authority the implementer can read.
- More generally: the survey's job is to make the plan **self-contained against
  the contract**, and "self-contained" currently has no check. A mechanical
  guard is conceivable (contract substrings the plan must contain) but may be
  over-engineering for a prose-only failure mode.
- Consider whether `implementer` should be told it may read the plan's cited
  authority when the plan self-describes as incomplete, instead of hard-stopping.
  Weak preference against: the stop was the correct behavior and produced a
  clean, cheap recovery.

## Notes

- Do not generalize this into "plans should quote everything." The survey's
  summarizing is normally correct and keeps plans readable; the defect is
  specific to text the contract marked as verbatim-binding.

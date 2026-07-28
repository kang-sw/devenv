---
title: the planned-marker retirement's downstream migration instruction has no mechanical check
related:
  260728-research-parallel-workflow-guide-divergence: same shape - a hand-maintained surface asserted true with no mechanism keeping it so
  260726-refactor-retire-spec-planned-marker-mechanism: the retirement whose sub-step 2.7 wrote the instruction this ticket questions the verification of
---

# the planned-marker retirement's downstream migration instruction has no mechanical check

## Background

`260726-refactor-retire-spec-planned-marker-mechanism` sub-step 2.7 and its
verification clause required the bootstrap template's v0045 entry to give
downstream projects an instruction covering the `features:`-frontmatter marker
form. That instruction was needed because the same retirement's clause 2.1
deleted `project_tree`'s WIP/ticket-ref counting — the only in-tree tool that
surfaced that marker form. Once that counting tool is gone, the only thing
telling a downstream project what to do about `features:`-frontmatter markers
is prose in a hand-maintained template.

## Measured 2026-07-28

- `specStats` narrowed from `(int, int, []string)` to `int` — the two counters
  and the string slice that carried WIP/ticket-ref detail are gone.
- `ticketRefRE` no longer exists anywhere in the tree.

So the retirement's own premise (this detection capability is being removed
from the tool) is confirmed, and it did anticipate the gap by writing a v0045
template instruction rather than leaving downstream silently uncovered.

## Why this is worth deciding rather than just fixing

There is nothing to hotfix here — the instruction exists, as designed. What is
missing is verification, on two separate axes, and neither is a code change
that could be made "correct" by itself:

- **Adoption**: nothing checks that any downstream project that pulled v0045
  actually applied the instruction to its own `features:`-frontmatter markers.
- **Durability**: nothing checks that the instruction stays present and
  correct as the bootstrap template accumulates later version entries. A
  future template edit could drop or garble it with no test noticing.

This is the same shape already captured in
`260728-research-parallel-workflow-guide-divergence`: a hand-maintained surface
(there, three WORKFLOW.md copies; here, one template version entry) that is
asserted to be true and load-bearing, with no generator, test, or playbook step
keeping it so. That ticket's open questions about generator-vs-structural-test
vs accepted-divergence-manifest are the same menu of answers this instruction
would need, so this ticket does not re-derive them — it records this as a
second, narrower instance for whichever session resolves the general question.

## Non-Scope

- Does not propose a specific verification mechanism for this instruction.
  Whatever is decided for the WORKFLOW.md divergence question should be
  checked against this narrower case rather than solved independently of it.
- Does not audit the bootstrap template for other similar one-off migration
  instructions that share this gap; this one was found because it was the
  direct subject of the regression review.

---
title: "Research tickets do not distinguish evidence, proposals, and confirmed decisions strongly enough"
related:
  260911-feat-impl-derivation-hardening-branch-aware-select: trigger; a slug-legibility proposal with no durable confirmation marker was copied into an actionable ticket as a required contract
  260911-research-batch-promotion-cross-ticket-coherence-gap: sibling; both failures passed through the same child-derivation and ready-review batch
---

# Research-ticket decision-state convention gap

## Background

The impl-lifecycle research and its derived branch-aware-selection ticket exposed
a recurring authoring failure: a research document can mix verified observations,
candidate improvements, and user-confirmed decisions in the same declarative
voice. A later child-derivation pass then has no reliable boundary for deciding
which statements may become executable contract.

In this incident, the research correctly established that
`wskey.Derive(ticketStem, 3)` is deterministic, then described readable slugging
as a legibility improvement. A later research edit coupled that proposal to
branch-owner lookup by treating “not reversible” as “not candidate-matchable.”
The actionable child copied the coupling as a settled prerequisite, even though
active ticket stems can be enumerated and passed through the existing derivation.
Git history establishes when the proposal and coupling appeared, but the stored
artifacts do not establish which individual statements the user explicitly
confirmed during the originating discussion.

## Incident chain

1. Commit `18e6c0ad` recorded the existing derivation invariants as holding and
   introduced readable-prefix + hash slugging under “residual defects” as a
   legibility proposal.
2. Commit `b7a26e75` added the branch-aware selector design and coupled owner
   lookup to readable slugging, despite also naming deterministic candidate
   derivation as the lookup mechanism.
3. Commit `e17e54be` derived and promoted the actionable child with Phase 2
   replacing the existing slug. Its AI Context explicitly treated
   “readable-slug -> reverse-map” as the reason to combine the work. Cross-ticket
   design and per-ticket completeness review did not challenge that premise.
4. Implementation stopped first on an unrelated pre-Select API sequencing
   contradiction. Reframing ownership as active-inventory enrichment on
   `git.status`, followed by a fresh fact pass and Sage review, exposed that the
   new slug contract was both underspecified and unnecessary.

## Failure mode

- Research prose had no durable decision-state distinction, so a plausible
  proposal read like a settled conclusion.
- “Cannot decode the stem from one slug” was conflated with “cannot enumerate
  stems, re-derive their slugs, and compare.”
- Child derivation preserved the research wording but did not re-check whether
  the claimed dependency was logically necessary or explicitly confirmed.
- The recorded review outcomes did not challenge the premise or establish its
  confirmation provenance before stamping the actionable contract.

## Direction to investigate

Define a research-ticket authoring convention that makes decision state explicit
without eliminating research's freeform topic structure. At minimum, determine:

- how verified findings, candidate directions, confirmed decisions, rejected
  alternatives, and open decisions are visibly distinguished;
- whether only confirmed decisions may be copied into actionable ticket contracts,
  with every other mechanism choice returned to the Open Decision Queue;
- whether child derivation or Sage design review must verify the provenance of each
  durable API, naming, workflow, or compatibility contract; and
- how to migrate existing research tickets without implying that old unmarked
  prose was user-confirmed.

The resulting rule must live on the shipped ticket-authoring/convention surface,
not only in a project-local manual, so downstream leads receive it through the
same convention path used for other ticket semantics.

---
title: Sage design review curates related-mental-model — prose recommendation, lead-owned frontmatter edit
sage-review-design: required
related:
  260622-feat-sage-review-ticket-gate: substrate — introduced the two-reviewer sage gate this extends
  260716-feat-mental-model-openup-injection: consumer — injection reads the curated related-mental-model associations
---

# Sage design review curates related-mental-model — prose recommendation, lead-owned frontmatter edit

## Background

Mental-model docs are stored well but delivered probabilistically: whether a
sage reviewer actually consults the relevant domain doc is unmeasured, and the
ticket `related-mental-model:` frontmatter that future deterministic injection
(`260716-feat-mental-model-openup-injection`) will consume is populated only
opportunistically at authoring time.

Attaching an association-curation duty to the design reviewer solves both at
once: the reviewer is the highest-capability agent in the ticket path and
already reads the ticket deeply, and the output obligation itself forces a
real mental-model corpus lookup — the "is it read at sage time" gap closes as
a byproduct rather than by an instruction to read.

## Decisions

- **The design reviewer outputs a prose recommendation, never a file edit.**
  Design and completeness reviewers can run concurrently (combined mode), so
  giving any reviewer write access to the ticket file is a race. The lead
  applies the final frontmatter edit as part of its existing sage-gate result
  handling in `lead-write-ticket`.
- Recommendation shape: additions and removals to `related-mental-model:`,
  **each with a one-clause justification**. No cap on entry count — precision
  is enforced by the per-item justification, not a limit. Removals deserve
  extra caution in the justification: under future injection, a wrong removal
  becomes a silent false negative.
- **Non-blocking.** Association curation is metadata repair, not a verdict
  input; it must not change pass/concern/block semantics. The natural
  exception needs no new rule: if a missing mental-model reveals the ticket
  ignores a documented trap, that is already a legitimate design-review
  concern/block through the existing verdict path.
- Scope boundary: only tickets that pass through the sage gate get curated
  associations. Ticket-less flows (sprint edits, direct edits) remain a known
  gap monitored by `260716-feat-ws-doc-condition-diagnostics`.

## Phases

### Phase 1: Reviewer output + lead-side application

Extend the `ticket-reviewer-design` playbook with the curation duty and
output contract (recommended additions/removals, one-clause justification
each, grounded in an actual `mental_models.find`/corpus lookup during the
review). Extend `lead-write-ticket`'s sage-gate result handling to parse the
recommendation and apply the frontmatter edit alongside the existing
`sage-review-design` posture write, in the same lead-owned commit.
Verification: run a design review on a ticket with a deliberately incomplete
`related-mental-model` list and confirm the reviewer recommends the missing
domain with justification and the lead commit carries the corrected
frontmatter; confirm reviewer output contains no file mutations.

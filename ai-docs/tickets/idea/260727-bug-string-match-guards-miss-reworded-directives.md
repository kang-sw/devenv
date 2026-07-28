---
title: regression guards written as forbidden-substring checks pass a reworded
  version of the defect they guard
related:
  260726-bug-sage-ready-enforcement-single-chokepoint: its Phase 1 shipped the
    guards, and its own cycle-1 fix is the demonstrated evasion
  260725-bug-sage-stamp-swallows-unrelated-ticket-edits: the earlier instance of
    the same underlying defect, which the 260726 guards exist to prevent recurring
---

# Forbidden-substring regression guards pass a reworded defect

## Topic

Phase 1 of `260726-bug-sage-ready-enforcement-single-chokepoint` deleted
`tickets.sage_gate`'s ability to emit a commit directive, and guarded the deletion
with three tests that assert the response text contains none of `ws/git.commit`,
`git.commit(`, `pending_commit`, or `chore(sage)`.

A test-partition reviewer mutation-tested them: it reworded the `skip` branch of
`sageGateNextInstruction` to reintroduce a commit directive using none of those
four substrings. All three guards passed.

## Why it matters

This is not a hypothetical evasion. It is the exact move that already happened
once inside this ticket: cycle 1 fixed the auto-commit by *rewording* it into
`pending_commit_*` lines plus a ready-to-paste `ws/git.commit(...)`, which
reproduced both defects one hop out and was only caught because a human-directed
re-review looked at the behavior rather than the string. The guards were then
written against the strings that particular evasion happened to use.

The general shape: a guard that pins the *wording* of a defect protects against
re-pasting it, not against re-deciding it. Prose surfaces — tool
`next_instruction` text, playbook step bodies, warning strings — are exactly where
a behavior is easiest to reintroduce under a new name, and exactly where
substring assertions are the tempting guard to write.

## Direction

The open question is what a behavior-class guard would even look like for a prose
surface. Candidates, none obviously right:

- Assert on structure rather than content: no `next_instruction` on a
  non-terminal gate action may name any `ws/` tool that mutates. This needs a
  notion of "mutating tool" the test can consult.
- Move the invariant off prose: if a commit directive is data rather than a
  sentence, its absence is a struct-shape assertion, which is checkable. This is
  what deleting `CommitTitle`/`CommitPaths`/`AIContext` already achieved for the
  *payload*; the residue is that the sentence can be rebuilt by hand.
- Accept the substring guard and add a review-time rule instead — but the ticket
  it came from establishes that a rule the executing agent does not read is a rule
  that does not apply.

Also worth scoping: how many other guards in this repo are substring assertions
over prose, and which of them guard a defect that has already recurred once.

## Prior art

- `260726-bug-sage-ready-enforcement-single-chokepoint` — where this was found;
  its `### Result` carries the disposition.
- `260721-bug-review-partition-empty-artifact` — an adjacent case of a check that
  passes without the thing it is checking for actually being present.

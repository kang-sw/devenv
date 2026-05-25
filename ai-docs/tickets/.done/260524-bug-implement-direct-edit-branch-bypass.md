---
title: Implement direct-edit mode can bypass expected implementation branch isolation
related:
  260523-bug-implement-merge-target-discovery: adjacent implementation branch lifecycle risk
  260523-chore-implement-branch-cleanup-guidance: adjacent implementation branch lifecycle cleanup gap
related-mental-model:
  - workflow-skills
completed: 2026-05-24
---

# Implement direct-edit mode can bypass expected implementation branch isolation

## Background

Dogfooding showed a mismatch between caller expectation and current
`ws:lead-implement` routing. The historical delegated implementation workflow
created `implement/<scope>` before execution, while the current unified
`lead-implement` skill creates an implementation branch only for delegated mode
and runs direct-edit mode on the current branch.

The current spec and mental model document this split, but the behavior may be
too permissive for callers who expect every `lead-implement` invocation to keep
implementation work off the base branch until the final action gate. Triage
should decide whether direct-edit current-branch mode is intentional, should
require an explicit opt-in, or should be replaced by always creating
`implement/<scope>` outside existing implementation branches.

## Result

Implemented in `a3cf3f7c`. `ws:lead-implement` now continues existing
`implement/*` branches and otherwise creates `implement/<scope>` before source
edits. Direct-edit remains an edit-mode choice only; it no longer bypasses
branch isolation. Merge guidance now allows fast-forward only when every branch
commit is workflow-owned, message-clean, and worth preserving directly in
target history, with squash or no-fast-forward used for non-trivial histories.

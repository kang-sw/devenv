---
title: Skeleton branch workflow
spec:
  - 260512-skeleton-inside-implement-branch
  - 260512-skeleton-draft-and-final-commits
related-mental-model:
  - workflow-skills
  - git-workflow-tools
---

# Skeleton branch workflow

## Background

`lead-proceed` currently treats skeleton writing as a prefix stage before
implementation. That makes `lead-proceed` route-only in wording but source-editing
in effect, and it puts skeleton commits outside the branch lifecycle owned by
`lead-implement`.

Skeleton writing should remain a separate primitive, but every code-editing
workflow step should execute under the implementation harness so branch creation,
commit range capture, documentation updates, and merge handling stay in one
place.

## Decisions

- `lead-proceed` decides whether skeleton work is needed, then passes that
  directive to `lead-implement`; it does not invoke `lead-write-skeleton`.
- `lead-implement` owns implementation branch setup before skeleton or
  implementation edits run.
- `lead-write-skeleton` operates on the current branch only and produces a
  lead-authored draft commit plus a final populated skeleton commit.
- Ticket `skeletons:` records only final skeleton commit hashes. Draft commits
  remain history checkpoints, not ticket artifacts.

## Phases

### Phase 1: Move skeleton execution under implement

Update workflow skill text and matching docs so skeleton execution is controlled
by `lead-implement`, not `lead-proceed`.

The implementation should:

- revise `lead-proceed` pipeline wording and execution steps so it passes a
  skeleton directive into `lead-implement`;
- revise `lead-implement` task flow so delegated implementation branches are
  created before optional skeleton writing, while direct-edit mode can still run
  on the current branch;
- revise `lead-write-skeleton` commit steps so it commits a draft checkpoint
  before population and a final skeleton checkpoint after review;
- keep `lead-write-code` and `lead-edit` as skeleton consumers only;
- update spec and mental-model text for the implemented behavior.

Success criteria:

- `lead-proceed` no longer invokes `lead-write-skeleton` directly.
- `lead-implement` documents optional skeleton execution before edit/write-code.
- `lead-write-skeleton` records draft and final commit boundaries without using
  amend.
- Specs no longer mark the new behavior as planned after implementation.

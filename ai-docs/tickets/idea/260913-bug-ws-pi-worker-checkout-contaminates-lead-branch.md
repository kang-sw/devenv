---
title: "Pi worker checkout can contaminate lead-owned commits"
related:
  260912-feat-ws-pi-bounded-web-access-for-explore: dogfood run that exposed the shared-worktree branch leak
  260912-feat-ws-pi-recursive-worker-subtree-lifecycle: worker lifecycle must preserve parent completion and state boundaries
---

# Pi worker checkout can contaminate lead-owned commits

## Background

During the Pi goal run for bounded web access, the ticket worker created and checked out its `impl/goal/...` branch in the same Git worktree used by the lead. The checkout was worktree-global, not agent-process-local. After the worker returned stop `(c)`, the lead correctly authored ticket Editions and a localized hotfix but did not first restore its goal branch, so six lead-owned commits landed after the unfinished implementation tip.

Recovery required preserving the contaminated tip, selectively transplanting only lead-owned commits to the goal branch, excluding worker implementation ancestry and review findings, and resetting the inactive impl ref to the worker's last intended checkpoint.

## Decisions

- **Treat checkout state as shared.** Pi lead/worker workflow must not assume a worker's branch checkout is isolated merely because the model/RPC process is separate.
- **Restore before lead writes.** After every ticket-worker terminal report from an impl branch, lead-run must capture the reported impl ref and head, restore and verify the invocation's recorded base branch before any lead-owned ticket edit, hotfix commit, follow-up dispatch, or merge decision.
- **Fail closed on unsafe restoration.** If the worktree has worker-owned uncommitted changes, unresolved Git state, an unexpected base tip, or a branch/ref mismatch, preserve diagnostics and stop instead of switching, committing, resetting, or importing implementation content.
- **Keep the impl ref intact.** Restoring the lead checkout must not reset or merge the implementation branch. The lead retains the exact impl head in its assignment note until normal merge or blocker disposition.
- **Guard each lead mutation.** Lead-owned commit paths should verify the current branch against the recorded base rather than relying only on a one-time post-worker restoration.
- **No implicit worktree allocation.** Do not solve this by silently creating temporary worktrees; this repository requires explicit approval for worktree creation and cleanup. A future isolated-worktree design may be considered separately.

## Phases

### Phase 1: Add branch restoration and mutation guards to Pi ticket runs

Make the ticket-worker handoff record both base and impl branch identities, restore the base checkout before stop handling performs lead-owned mutations, and reject unsafe or ambiguous Git state. Apply the guard to resumed workers and recursive-worker terminal reports as well as first-run completion.

Verify stop `(b)` through `(e)`, successful phase/ticket completion, worker-created impl checkout, clean base restoration, dirty/unmerged refusal, changed ref/tip refusal, unrelated untracked-file preservation, assignment-note recovery after compaction, no implementation ancestry in lead-only commits, and unchanged normal `ws/git.merge` ownership. Include a regression reproducing the 2026-09-13 contamination shape: worker commits followed by lead ticket/hotfix commits must leave the latter only on the restored goal branch.

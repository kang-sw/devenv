---
title: Implement merge flow lacks branch cleanup guidance
related:
  260523-bug-implement-merge-target-discovery: merge-target safety and cleanup both affect implementation branch lifecycle
sage-review: skipped
---

# Implement merge flow lacks branch cleanup guidance

## Background

Local dogfood exposed many stale implementation branches after completed work.
`lead-implement` documents the final merge step, but it does not say whether the
merged implementation branch should be deleted, reported as retained, or left to
manual cleanup. That omission makes branch accumulation the default outcome even
after the implementation branch is safely contained in its merge target.

Branch cleanup is destructive enough to require explicit approval or a clearly
bounded post-merge rule. The workflow should not silently delete active
worktree-checked-out branches, unmerged branches, or branches whose target was
ambiguous.

## Spec Impact

Target spec area: workflow-skills — lead-implement post-merge branch lifecycle
Expected caller-visible change: Adds a `### 9. Branch Cleanup` step to lead-implement; callers get explicit cleanup guidance with safety guards after a confirmed merge.
Contract-first spec: no

## Phases

### Phase 1: Add post-merge branch cleanup step to lead-implement

Constraints:
- Edit `agents-plugin/rsrc/lead-implement/lead-implement.md` only.
- Add `### 9. Branch Cleanup` after `### 8. Merge`.
- Safety guards required: skip if the branch is currently checked out, linked to an active worktree, the merge target was ambiguous, or unmerged commits exist. Ask user before deleting. Never delete without explicit approval.
- Keep prose command-shaped and concise; one numbered list.
- After editing rsrc, regen wsflow rsrc mirror and verify `TestWsflowRsrcMirrorUpToDate` passes.

Verification:
- `go test ./internal/mcp/...` passes.
- Fresh read confirms cleanup guidance is clear and safety guards are unambiguous.

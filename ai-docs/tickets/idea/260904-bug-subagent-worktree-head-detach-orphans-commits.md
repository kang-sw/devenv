---
title: "Delegated subagents can leave shared-worktree HEAD detached, orphaning lead commits"
related:
  - 260605-epic-ws-playbook-factory-pivot
  - 260904-refactor-enter-affordance-rename-route-opaque
---

## Background

During layer ① Phase 1 (`260904-refactor-enter-affordance-rename-route-opaque`),
the lead's doc-commit-gate commit for the ticket Phase 1 Result was silently
orphaned. Observed symptom:

- The implementer subagent committed the rename on branch
  `impl/.../hunk-mumps-zippy`, advancing it to `7e35db10`.
- Two review subagents then ran (correctness, test), read-only by role.
- The lead next edited the ticket and ran `git add <ticket> && git commit`,
  producing `e50cc8bf` — but with parent `b17813283` (a stale ③-era commit),
  **not** the impl tip `7e35db10`. HEAD was detached at `b17813283` at commit
  time.
- Result: `e50cc8bf` never advanced the impl branch (deleted "was 7e35db10"),
  the Phase 1 merge (`8134f0c0`) did not carry it, and the ticket landed on the
  goal branch without its Result section. Recovered by re-authoring the Result
  directly on goal (`d1270d13`).

The rename code itself was correct and merged cleanly; only the lead's
subsequent commit was misrouted.

## Hypothesis

Delegated subagents (Agent tool) execute shell in the **same working directory
and git repo** as the lead. A subagent that runs `git checkout <ref>` (e.g. a
reviewer inspecting pre-rename state to confirm "the old tokens were equally
untested") leaves the shared repo HEAD detached, and the lead's next commit
lands there instead of on the intended branch — with no error surfaced.

## Open Questions

- Which subagent moved HEAD, and can it be confirmed from transcripts? (The
  test reviewer explicitly compared pre/post-rename token coverage.)
- Is the right fix a guardrail in the reviewer/implementer playbooks ("never
  leave HEAD detached; restore the branch you found"), a lead-side invariant
  ("verify `git rev-parse --abbrev-ref HEAD` is the intended branch before every
  commit after delegated work"), or isolation (subagents operate on a detached
  copy / separate worktree)?
- Does the ws git-commit MCP tool (`ws/git.commit`) guard against a detached
  HEAD, and should it refuse or warn when HEAD is not on the expected branch?

## Notes

- Immediate mitigation already adopted for layer ① Phase 2: the lead verifies the
  checked-out branch before committing after any delegated subagent returns.
- This is a workflow-integrity hazard, not specific to this ticket — any
  delegated flow that commits after read-only reviewers is exposed.

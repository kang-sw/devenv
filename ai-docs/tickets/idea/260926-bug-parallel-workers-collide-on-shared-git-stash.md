---
title: Parallel ticket workers collide on the repository-wide git stash
---

# Parallel ticket workers collide on the repository-wide git stash

## Background

Dogfood surprise (2026-09-26, `ws:lead-run` parallel route, four pooled
worktrees). `refs/stash` is shared by every worktree of a repository. Two
workers in separate pooled worktrees each ran `git stash push` / `git stash
pop` at nearly the same moment to take a test baseline, and each popped the
other's stash:

- the `260926-bug-pi-children-inherit-mailbox-identity` worker's worktree
  ended up holding the other worker's uncommitted `agents-plugin-pi` waiter
  edits;
- the `260926-bug-mailbox-idle-owner-presence-goes-stale` worker's worktree
  lost those edits and gained a duplicate of the first worker's already
  committed spawner diff as uncommitted changes.

Nothing was lost only because the first worker noticed and saved both sides
as patches. A worker that commits without noticing would land a foreign
diff on its impl branch and silently drop its own edits.

## Direction (unsettled)

- The worker playbooks (`ticket-worker`, `ticket-worker-elevated`, and any
  other playbook that may run in a pooled worktree) forbid `git stash` and
  name a safe baseline method instead: a throwaway worktree, or running the
  baseline at the branch base commit before editing.
- Alternatively or additionally, `ws/worktree.acquire` or the worker
  preamble could flag the shared-stash hazard only when parallel worktrees
  are live.
- Check whether any shipped playbook or tool itself calls `git stash`.

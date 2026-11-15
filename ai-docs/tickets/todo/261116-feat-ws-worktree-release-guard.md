---
title: "worktree.release refuses another holder's or ownerless-dirty worktree unless force"
---

# worktree.release refuses another holder's or ownerless-dirty worktree unless force

## Background

Incident: a released worktree was re-acquired by another session, and a
second release of it wiped that session's work. EXEC4 implemented a guard
on branch `exec4/worktree-release-guard`, head `172b6bbe3` (pushed at
handoff). Its parent commits are `3b2fe2596` and `90a592286`.

Behavior on that head:
- `worktree.release` refuses, by default, a worktree that another session
  holds or that has ownerless changes. The disk is left as is.
- `force: true` keeps today's cleanup.
- Release is serialized with acquire, so another session cannot slip in
  between the check and the cleanup.
- A test replays the incident: the second release is refused and the draft
  survives.
- Old daemons ignore the new argument and behave as before (T0).

## Decisions

- (1172)=ok, 2026-11-16: the owner approved this tool-behavior tightening for
  the next patch release.

## Phases

### Phase 1: Land the reviewed head

- Rebase or merge `172b6bbe3` onto current develop, resolving conflicts in
  `internal/mcp` worktree tools against the later acquire-facts change
  (`0c5f1b43a`).
- Run the scoped mcp tests (`-run 'Worktree|Release|Acquire'`), never the
  full package. Then go through the merge flow and swap the daemon.

---
title: git.commit rename status summary mismatch
related-mental-model:
  - git-workflow-tools
  - documentation-system
---

# git.commit rename status summary mismatch

## Background

During dashboard daemon ticket promotion, `ws/git.commit` successfully committed
a `todo/` to `ready/` ticket move, and follow-up `ws/tickets.status` plus the
filesystem confirmed the ticket lived under `ready/`.

The `ws/git.commit` result payload still reported the ticket change as
`to_status: todo` for that rename. This is surprising because callers use the
commit result as workflow evidence after ticket status moves.

## Next Check

Inspect the structured ticket-change detection used by `ws/git.commit` for
renamed paths that also have worktree modifications. Confirm whether it reads
the old path, the staged rename source, or a stale status inference when a moved
ticket is edited before commit.

The desired behavior is that the commit result reports the destination status
for ticket moves, matching `ws/tickets.status` after the commit.

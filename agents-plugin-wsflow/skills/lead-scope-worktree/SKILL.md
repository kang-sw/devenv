---
name: lead-scope-worktree
description: Scope this worktree's ticket board to one work line via git sparse-checkout, hiding out-of-topic ready/todo/idea tickets. Use when the user wants a worktree dedicated to one work line, topic, or ticket subset.
---

# Scope Worktree

Call `wsflow/playbook.read(name: "lead-scope-worktree")` and execute the returned procedure
inline against the current user request. If the playbook cannot be loaded, stop
and report that blocker.

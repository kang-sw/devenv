---
title: "Tracker item.move: reparent and reorder an existing item"
---

# Tracker item.move: reparent and reorder an existing item

## Background

User question on the 2026-11-09 board (item (151) note): when a topic is no
longer assigned to its creator, what happens to it, "and can a task be
reparented at all?" Answer (152) b: capture an idea ticket for moving items.

Current state (agents-plugin-tool/internal/wstracker/state.go): an item's
`parent` and its sibling anchor `after` (decision 528) are set only by
`item.create` (the create fold reads `p.str("parent")` and `p.str("after")`);
`item.edit` folds only title, body, project, due, priority, and stage. So a
misplaced item can only be recreated, losing its history, comments, and
assignment.

## Idea

A `item.move` operation (or `item.edit` fields) that sets a new `parent`
and/or `after` on an existing item, exposed through `tracker.update` and the
dashboard (drag in the tree, or a "move to" picker in the item menu).

Open questions for promotion:

- Who may move: the author only, the parent's author, an assignee?
- Subtree assignment rules (`{root, include_root, levels?}`): does a moved
  subtree leave rules rooted above its old position, and enter the new
  parent's rules?
- Cycle refusal (moving an item under its own descendant) and concurrent
  moves from two processes (fold order by happens-before, like other
  ledger ops).
- Kind constraints, interacting with the planned topic/task kind changes
  ((153) b, (154) a on the same board).

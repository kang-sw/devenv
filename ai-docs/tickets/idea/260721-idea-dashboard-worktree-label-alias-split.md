---
title: Dashboard workspace label is basename-derived, so differently-named path aliases to the same physical directory still split into separate buckets
related:
  260721-bug-dashboard-worktree-create-duplicate-add: origin ticket - fixed WorkspaceKey.id canonicalization; this ticket captures the sibling label gap that fix left out of scope
---

# Workspace label derived from literal basename splits same-physical-dir aliases

## Background

After the `260721-bug-dashboard-worktree-create-duplicate-add` fix,
`WorkspaceKey.id` (and the other `stable_path_hash` derivations in
`ws-dashboard/crates/daemon/src/discovery.rs`) route through a shared
`canonical_or_normalized` helper before hashing, so the same physical
directory reached via divergent textual or symlink paths now correctly
dedups to one `WorkRootId`.

`WorkspaceKey.label`, however, is derived by `label_for_path` in the same
file directly from the literal candidate path's basename, independent of
canonicalization. So two **differently-named** symlink aliases pointing at
the same physical directory still produce two different labels, and
therefore still land in two separate workspace buckets in the dashboard UI
- even though their underlying `WorkRootId` now matches.

## Scope note

This is a narrow edge case: it requires two aliases with *different*
basenames pointing at the same physical directory (a same-named alias would
already coincide). It was explicitly out of scope for
`260721-bug-dashboard-worktree-create-duplicate-add`, whose fix was `.id`
(hash-key) only, per that ticket's scope. Captured here so the gap is not
lost.

## Related

- `260721-bug-dashboard-worktree-create-duplicate-add`

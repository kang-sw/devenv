---
title: ws dashboard WorkRoot polishing backlog
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
spec:
  - 260524-ws-dashboard-git-worktree-creation
  - 260524-ws-dashboard-git-aware-workroot-toolbar
---

# ws dashboard WorkRoot polishing backlog

## Background

The WorkRoot management MVP is complete enough for the current milestone:
opened roots persist, linked Git worktrees are discovered and creatable,
workspace/root removal has an owner-facing UI, and selected WorkRoots expose a
Git-aware toolbar with branch/status/fetch/push/fast-forward pull behavior.

This ticket keeps remaining WorkRoot management polish out of the MVP critical
path. Future sessions should add concrete dogfood issues here or split them
when they become implementation-ready.

## Phases

### Phase 1: WorkRoot lifecycle polish

Improve owner operations for active, unavailable, linked, remembered, pinned,
recoverable, and prunable WorkRoots without turning the root picker into a
generic file manager.

Candidate areas include clearer unavailable/recovery states, remove/forget copy,
refresh timing, pinned directory behavior, workspace grouping clarity, and
worktree lifecycle edge cases.

- **Git worktree deletion is missing entirely** (found 2026-07-11 dogfood
  discussion): `crates/daemon/src/git_worktree.rs` only implements
  `git_worktree_add_options`/`_preview`/`_submit` (create-only); there is no
  `remove`/`prune`/`list` daemon operation, and the frontend has no
  delete/remove worktree command (only "Add worktree"). Deleting a worktree
  today requires bypassing the dashboard entirely and running
  `git worktree remove` manually in a terminal. Worse, `root_picker.rs`'s
  `remove_workspace` is misleadingly named — it only unregisters the
  workroot from the dashboard's own `OpenedWorkRoots` registry
  (`persistent_state.rs`) and never touches disk or runs
  `git worktree remove`. The two "delete" concepts are fully decoupled:
  removing via the dashboard leaves the real worktree directory and
  `.git/worktrees/` metadata behind (stale entries in `git worktree list`
  forever), while removing via terminal leaves a zombie entry in the
  dashboard registry. A real delete operation should atomically run
  `git worktree remove` (with an explicit force option gated on
  uncommitted/untracked changes) and clean up the dashboard registry entry
  together, and must account for active terminal sessions still using that
  worktree.

Verification should include resource model tests where possible plus browser
coverage for any visible navigation or lifecycle behavior changed.

### Phase 2: Git toolbar polish

Tune the selected WorkRoot Git surface as dogfood issues appear. Candidate areas
include branch dropdown ergonomics, new-branch flow feedback, status polling
load, push/pull/fetch progress affordances, long branch names, and stale status
recovery.

This phase should preserve the current safety decision that dashboard-triggered
pulls use `git pull --ff-only` and never attempt conflict resolution.

Verification should include daemon Git tests where practical and browser
coverage for any changed Git interaction.

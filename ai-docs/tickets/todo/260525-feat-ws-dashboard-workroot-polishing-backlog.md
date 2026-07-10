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

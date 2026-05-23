---
title: worktree local index context is missing
related-mental-model:
  - workflow-skills
---

# worktree local index context is missing

## Background

During Windows SSH verification from an implementation worktree, the expected
local machine context was absent because `ai-docs/_index.local.md` is ignored
and was not copied into the Git worktree. The canonical main worktree still had
the needed host record for `ki608@192.168.33.6`.

This makes worktree-based dogfood runs lose machine-local context such as
approved SSH test hosts, local browser-gate notes, and other ignored
environment records. Agents then either re-derive local setup unnecessarily or
try the wrong default account/path.

## Direction

Investigate how ws-created worktrees should carry ignored local context without
committing it. A likely direction is copying selected ignored local files during
worktree setup, with clear ownership and no accidental staging.

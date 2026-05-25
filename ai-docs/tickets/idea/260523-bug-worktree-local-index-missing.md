---
title: dashboard-managed worktree local context
related-mental-model:
  - ws-web-dashboard
---

# dashboard-managed worktree local context

## Background

During Windows SSH verification from an implementation worktree, the expected
local machine context was absent because `ai-docs/_index.local.md` is ignored
and was not copied into the Git worktree. The canonical main worktree still had
the needed host record for `ki608@192.168.33.6`.

This makes worktree-based dogfood runs lose machine-local context such as
approved SSH test hosts, local browser-gate notes, and other ignored
environment records. Agents then either re-derive local setup unnecessarily or
try the wrong default account/path.

Git worktree itself does not distinguish ignored build artifacts from ignored
local workflow context. Copying all ignored files would drag build outputs,
caches, and possibly secrets into implementation worktrees. Adding broad
worktree-management APIs to ws core would also pull workflow orchestration into
general Git workspace management.

## Direction

Treat this as a dashboard/workroot management problem, not a ws core workflow
primitive. A future dashboard surface could make local context propagation a
human-visible action:

- show which ignored local context files exist for the current workroot;
- distinguish allowlisted local context from build artifacts and caches;
- let the user copy selected local context into a new or existing worktree;
- default to skip existing destination files unless the user explicitly
  overwrites;
- keep copied files ignored and unstaged;
- warn before copying files likely to contain secrets.

The likely allowlist starts with `ai-docs/_index.local.md` and
`ai-docs/**/*.local.md`, but the dashboard design should decide the manifest,
preview, and safety policy before implementation.

---
title: worktree local context propagation
---

# worktree local context propagation

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
caches, and possibly secrets into implementation worktrees. Any broad API for
worktree management must avoid coupling workflow context propagation to general
Git workspace management.

## Direction

The mechanism is undecided. The dashboard/workroot surface that previously
assumed this responsibility no longer exists. A future solution must preserve
the observed safety constraints: distinguish allowlisted local workflow context
from build artifacts and caches, keep copied files ignored and unstaged, avoid
overwriting existing destinations without explicit confirmation, and warn before
copying files likely to contain secrets.

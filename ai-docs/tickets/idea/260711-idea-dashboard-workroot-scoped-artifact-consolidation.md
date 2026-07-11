---
title: "Consolidate dashboard-managed workroot files under .ws-dashboard/, shared across worktrees"
parent: 260525-feat-ws-dashboard-workroot-polishing-backlog
related:
  260711-idea-dashboard-command-bus-quick-open-shortcuts: first concrete consumer — custom command definitions are proposed to live at <root>/.ws-dashboard/scripts/
  260711-idea-dashboard-agent-facing-mcp-control-surface: a future consumer if worktree-management or custom-command MCP calls need workroot-scoped config
---

# Consolidate dashboard-managed workroot files under .ws-dashboard/, shared across worktrees

## Background

Owner direction (2026-07-11): dashboard-managed, workroot-scoped files
(starting with custom command definitions from
`260711-idea-dashboard-command-bus-quick-open-shortcuts`) should live
under a single `<workroot>/.ws-dashboard/` directory in the repo, rather
than each feature inventing its own location. Two concrete asks:

1. Files under `.ws-dashboard/` should be conditionally git-tracked: e.g.
   a filename pattern like `*.local.*` marks a file as local-only
   (`.gitignore`d), everything else tracks normally — mirroring the
   `.env`/`.env.local` convention, so shareable dashboard config (like
   team-wide custom commands) can be committed while personal overrides
   stay local.
2. `.ws-dashboard/` should be **shared across all worktrees of the same
   repo**, rooted at the root workroot (the original clone), not
   duplicated per worktree — so a custom command or config defined once
   is visible from every worktree checked out from that repo.

Today, confirmed by investigation: `.ws-dashboard/` does not exist
anywhere in the codebase — this is a new concept. All current
dashboard-managed state (`DashboardStateStore` /
`persistent_state.rs:485-503`: `OpenedWorkRoots` registry, root-picker
pins, linked servers) lives in a single global JSON file in the user's
XDG state dir / `%LOCALAPPDATA%`, not inside any workroot — so there is
no existing in-repo precedent to extend, only a global-state precedent
to diverge from for anything meant to be workroot-scoped and shareable.

## Findings relevant to the cross-worktree sharing mechanism

- `resolve_workspace_git` (`git_worktree.rs:429-474`) already resolves
  `git rev-parse --path-format=absolute --git-common-dir`, which — called
  from *any* worktree — always points at the original repo's `.git`. Its
  parent directory is therefore a ready-made "root workroot" anchor;
  the daemon does not need new bookkeeping to find where the root clone
  lives.
- Default worktree placement today is `<root>/.git/ws-worktree/<name>`
  (`git_worktree.rs:338-341,473`).
- git worktrees do not require touching `.git` itself to add a shared
  directory (each worktree's `.git` is already just a gitdir-pointer
  file) — a new `.ws-dashboard` entry is orthogonal to that structure, so
  no conflict there.
- **Windows**: true symlinks need admin rights or Developer Mode
  (`SeCreateSymbolicLinkPrivilege`). Directory **junctions** (`mklink /J`)
  need no elevated privilege but only work within the same volume — a
  worktree on a different drive/volume than the root clone would break a
  junction-based approach. This needs an explicit fallback decision (skip
  linking cross-volume? fall back to a real symlink attempt with a clear
  error? copy instead of link?).
- Ordering matters: `git worktree add` can fail if the target directory
  already has unexpected entries, so the link should be created *after*
  worktree creation succeeds, not before.
- Cleanup: `git worktree remove` will delete the worktree directory
  including any symlink/junction entry inside it (that's fine, the link
  is disposable), but the daemon is responsible for not leaving orphaned
  link state anywhere else and for handling worktrees that were removed
  outside the dashboard (see the sibling worktree-deletion gap in
  `260525-feat-ws-dashboard-workroot-polishing-backlog`, Phase 1).
- No `GitWorktreeBlockerCode` variant currently exists for a symlink/
  junction creation failure — the blocker-code schema would need a new
  variant if this is surfaced as a user-facing error during worktree
  creation.

## Open Questions (owner flagged this needs more UX/policy discussion — not decided yet)

- **When** does the link get created: at dashboard-driven worktree
  creation time (daemon controls the whole flow, simplest), at worktree
  *discovery* time (covers worktrees created outside the dashboard, e.g.
  by a plain `git worktree add` in a terminal — more robust but requires
  the daemon to detect and backfill missing links whenever it discovers
  a worktree), or both?
- What happens on a linking failure (cross-volume worktree, missing
  Windows privilege, filesystem without symlink support)? Silently skip
  (worktree just doesn't get the shared directory, falls back to nothing
  or a local-only stub) vs. surface a blocking error vs. copy-then-diverge?
- Does "shared across worktrees" mean strictly read/write-through the
  same files (a real link), or would a sync/replicate model be
  acceptable/preferable for cross-volume cases?
- Should `.ws-dashboard/` have an internal structure decided now (e.g.
  `scripts/`, and whatever future MCP-surface or workroot-config data
  needs), or should this ticket only settle the top-level consolidation +
  sharing mechanism and let each consumer (starting with the command-bus
  ticket) claim its own subdirectory independently?

## Non-Goals

- Deciding the internal layout of `.ws-dashboard/scripts/` itself — owned
  by `260711-idea-dashboard-command-bus-quick-open-shortcuts`.
- Migrating the existing global `DashboardStateStore` (opened-workroots
  registry, pins, linked servers) into `.ws-dashboard/` — that state is
  intentionally global/cross-repo today; this ticket is about new
  workroot-scoped, shareable data only, not a wholesale storage migration.

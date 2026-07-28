---
title: "Worktree removal may be blocked by a live daemon handle/terminal on that workRoot"
sage-review-design: required
---

# Worktree removal may be blocked by a live daemon handle/terminal on that workRoot

## Background

Dogfood observation (2026-07-27, owner report, unconfirmed mechanism): deleting
a git worktree sometimes fails or leaves residue, and the suspicion is that the
ws-dashboard daemon still holds a reference into that worktree's path — e.g. an
open terminal/PTY session, a helper process with its cwd inside the worktree,
or some other live handle — which either locks files on Windows or otherwise
keeps the daemon's own state pointing at a path that's about to disappear.

This has NOT been verified as the actual mechanism. Before implementing
anything, Phase 1 must confirm whether the daemon actually holds a handle that
would block OS-level deletion (Windows file/directory locks are the most
likely culprit given this is dogfooded from WSL onto a Windows clone over
`/mnt/d`), or whether the daemon merely keeps stale in-memory references that
don't block deletion but cause a different downstream symptom (e.g. a
dangling terminal pane pointing at a now-missing path, garbage in the
workRoot registry, etc). The fix shape depends entirely on which of these is
actually happening.

## Phases

### Phase 1: Confirm the mechanism

Use an exploration pass (read-only) to answer, with evidence from the daemon
source (`crates/daemon/`) and the worktree-removal call path (frontend
trigger + backend handler):

- Does the daemon open any OS-level handle (file, directory, process cwd) that
  stays open for the lifetime of a terminal session, scoped to a worktree's
  path? On Windows specifically, does an open file/process with cwd inside a
  directory prevent that directory from being deleted/renamed by another
  process (e.g. `git worktree remove`, or Explorer/PowerShell doing the
  deletion)?
- Where does worktree removal actually get triggered from — is it initiated
  by the dashboard itself (an API path in the daemon), or always external
  (owner runs `git worktree remove` by hand outside the dashboard)? If
  external, the daemon has no opportunity to pre-close anything unless it's
  polling/watching for the removal.
- Does the daemon already have any registry of "live terminals/handles per
  workRoot" that a pre-removal check could query (reuse rather than build
  new bookkeeping)?
- Reproduce if feasible: open a terminal in a worktree via the dashboard,
  attempt to remove that worktree while the terminal is still open, and
  observe the actual failure mode (locked file error, silent partial
  deletion, no issue at all, etc).

### Phase 2: Close live handles before removal (scope depends on Phase 1 findings)

If Phase 1 confirms the daemon holds a blocking handle and removal can be
daemon-mediated: close all live terminals/helper processes for the target
workRoot before deletion proceeds, mirroring the existing "close all
terminals" pattern in `AdvancedSection`'s danger-zone (see
`ws-dashboard/frontend/src/settingsSections.tsx`'s `killAllTerminals` call,
scoped per-workRoot instead of globally).

If Phase 1 finds removal is always external (owner-driven, outside the
dashboard's control), the fix instead needs to be: detect when a workRoot's
directory has disappeared out from under an open terminal/session and clean
up daemon-side state gracefully (close the stale session, surface a clear
error) rather than trying to preempt an external `git worktree remove` call.

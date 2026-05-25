---
title: Add Git-aware WorkRoot toolbar chips and branch controls
completed: 2026-05-25
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-linked-worktree-discovery: toolbar chips apply to Git primary roots and linked worktrees discovered as workRoots
  260524-feat-ws-dashboard-add-git-worktree-ui: branch/worktree creation controls should share Git-aware workspace/workRoot semantics
  260523-feat-ws-dashboard-workroot-registry-activation: Git status is visible only for online available Git-aware workRoots
spec:
  - 260524-ws-dashboard-git-aware-workroot-toolbar
related-mental-model:
  - ws-web-dashboard
---

# Add Git-aware WorkRoot toolbar chips and branch controls

## Background

The WorkRoot toolbar currently shows resource state, activity, availability,
activation, and workbench actions, but it is not Git-aware beyond the workRoot
kind. Git primary roots and linked Git worktrees are now first-class workRoots,
so the selected WorkRoot toolbar should expose high-signal branch and change
state without forcing the owner to inspect a terminal for ordinary Git context.

This ticket adds compact Git-aware toolbar chips for selected Git workRoots and
basic branch/sync controls. It should follow Git's default safety behavior and
avoid inventing merge, rebase, force, stash, conflict-resolution, or branch
policy semantics inside the dashboard.

## Decisions

- Show Git chips only when the selected workRoot is Git-aware
  (`gitPrimaryRoot` or `gitLinkedWorktree`), online, and currently available.
  Non-Git, offline, unavailable, moved, or inaccessible workRoots should not
  render the Git chip controls beyond possibly bounded unavailable diagnostics.
- Add a branch chip that shows the checked-out branch, or a bounded detached
  `HEAD` label when detached. Clicking the branch chip opens a branch dropdown.
- The branch dropdown should show `+ New branch...` at the top, followed by
  daemon-resolved local branches. Branches checked out in another worktree
  should be disabled when known, and every mutation must revalidate server-side
  to handle races.
- `+ New branch...` opens a modal with new branch name and base branch. The
  default base branch is the current branch when available. The first pass
  creates and switches to the new branch; create-without-switch is deferred.
- Branch switching and branch creation should follow Git defaults. The
  dashboard does not force, stash, automatically resolve, or choose merge/rebase
  policies. Git command failures become bounded errors and a refreshed status.
- Add one compact Git status pill with a dark/black background. Its segment
  text colors classify state: `+<added-line-count>` in green,
  `-<removed-line-count>` in red, `*<modified-file-count>` in yellow,
  `?<untracked-file-count>` in indigo-blue, then `|`, then
  `↑<pending-push-count>` in green and `↓<pending-pull-count>` in yellow.
- The Git status pill should include a small always-rendered refresh icon. The
  refresh icon runs `git fetch` and then refreshes the toolbar Git status.
- The `↑` segment should be interactive when there are pending pushes and an
  upstream exists. It runs plain `git push`, never force push, then refreshes
  status.
- The `↓` segment should be interactive only for safe fast-forward pulls. It
  runs `git pull --ff-only`, not plain `git pull`, so dashboard-triggered pull
  cannot leave the worktree in a merge/rebase conflict state. FF-only failure
  is a bounded error plus status refresh.
- Modified and untracked status segments are read-only in the first pass. The
  status pill may hide zero-value segments, but the refresh icon should remain
  visible while the Git-aware chip is shown.
- Polling should stay host-light: immediately refresh Git status on selected
  workRoot change, explicit dashboard refresh, branch mutations, fetch, push,
  and pull; poll only the selected visible workRoot at a conservative interval;
  pause polling while the document is hidden and refresh immediately when it
  becomes visible again.
- Git status and branch APIs must remain owner-authenticated, route through
  opaque `workRootId`, run Git work off async workers, and avoid exposing host
  paths in command logs or bounded browser-visible errors.

## API Sketch

Git toolbar status should be a daemon-owned read model:

```ts
type WorkRootGitStatus = {
  available: boolean;
  reason?: string;
  branch?: {
    name?: string;
    detachedOid?: string;
    upstream?: string;
  };
  changes: {
    addedLines: number;
    removedLines: number;
    modifiedFiles: number;
    untrackedFiles: number;
  };
  sync: {
    ahead: number;
    behind: number;
    upstream?: string;
  };
  operations?: {
    fetching?: boolean;
    pushing?: boolean;
    pulling?: boolean;
  };
  refreshedAtMs: number;
};
```

Branch list and branch mutation requests should stay minimal:

```ts
type GitBranchList = {
  current?: string;
  detachedOid?: string;
  branches: Array<{
    name: string;
    current: boolean;
    checkedOut: boolean;
    upstream?: string;
    ahead?: number;
    behind?: number;
    disabledReason?: string;
  }>;
};

type SwitchBranchRequest = {
  branchName: string;
};

type CreateBranchRequest = {
  branchName: string;
  baseBranch?: string;
  switchTo: true;
};
```

Candidate routes:

```text
GET  /api/dashboard/work-roots/{workRootId}/git/status
GET  /api/dashboard/work-roots/{workRootId}/git/branches
POST /api/dashboard/work-roots/{workRootId}/git/switch-branch
POST /api/dashboard/work-roots/{workRootId}/git/branches
POST /api/dashboard/work-roots/{workRootId}/git/fetch
POST /api/dashboard/work-roots/{workRootId}/git/push
POST /api/dashboard/work-roots/{workRootId}/git/pull-ff-only
```

Each mutation should return an updated `WorkRootGitStatus` when possible, or a
bounded command result plus a follow-up status refresh. `pull-ff-only` is
deliberately named after its safe semantics.

## Phases

### Phase 1: Add Git-aware toolbar status and branch controls

Add daemon-backed Git status and branch APIs for online available Git workRoots.
The read model should summarize current branch/detached state, tracked line
changes, modified-file count, untracked-file count, upstream ahead/behind
counts, and branch checked-out state. Status computation should follow Git
status semantics and should not scan non-Git workRoots.

Add the WorkRoot toolbar branch chip, branch dropdown, new-branch modal, and
compact Git status pill. Branch switch and create-new-branch actions should
follow Git defaults and revalidate on submit. The status pill should render
the colored segment grammar, always include a small fetch/refresh icon, and
expose push and fast-forward-only pull interactions on the upstream segments.

Polling should refresh the selected workRoot immediately on selection changes
and on focus/visibility return, then poll conservatively only while the page is
visible. Explicit toolbar fetch, push, pull-ff-only, branch switch, and branch
create actions should refresh status after completion or failure.

Deferred scope: merge/rebase pull, conflict resolution, abort handling, stash
workflows, force push, set-upstream creation, remote-branch checkout/tracking
UX, branch deletion/rename, file-level Git operations, status chips for
non-selected roots, and watcher-driven correctness. Later watchers may only act
as refresh hints unless a separate ticket defines their safety model.

Verification should cover Git-aware visibility gating, branch chip/dropdown,
new branch creation from current/base branch, checked-out branch disabling,
dirty branch switch/create bounded failures under Git defaults, status counts
for added/removed/modified/untracked/ahead/behind, fetch refresh behavior,
plain push success/failure behavior, `git pull --ff-only` success and
non-fast-forward failure without conflict state, polling pause while hidden,
immediate refresh on selected workRoot switch, no host-path leakage, and
browser-level evidence against a daemon-served Git workRoot toolbar.

### Result (3ac64908) - 2026-05-25

Implemented daemon-backed Git status, branch listing, branch creation/switching,
fetch, plain push, and fast-forward-only pull routes for selected online
available Git workRoots. The toolbar now renders the branch chip, branch menu,
new-branch modal with base selection, compact status pill, and safe sync
interactions through dashboard commands and opaque `workRootId` routing.

Review follow-up tightened typed route failures, stale cross-root state
clearing, route-test coverage for duplicate/base branch and ff-only behavior,
polling scheduler coverage, CSS token usage, browser fetch evidence, and
visible-workbench-root close confirmation handling. Verification passed daemon
tests, command tests, Git toolbar route tests, production frontend build, and
browser acceptance after the close-confirmation fix.

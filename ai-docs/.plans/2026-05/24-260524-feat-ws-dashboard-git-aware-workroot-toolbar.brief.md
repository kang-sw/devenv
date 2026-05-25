# Brief: 260524-feat-ws-dashboard-git-aware-workroot-toolbar

## Intent

Add selected-WorkRoot Git awareness to the dashboard top toolbar so an owner can
see branch/status context and run safe branch/sync operations without leaving
the browser, while preserving Git's default safety behavior and dashboard
privacy boundaries.

## Scope Boundary

Implement only Phase 1 of
`260524-feat-ws-dashboard-git-aware-workroot-toolbar`.

In scope:

- Owner-authenticated, workRoot-scoped daemon routes for Git status, branch
  list, branch switch, create-and-switch branch, fetch, plain push, and
  fast-forward-only pull.
- Git controls visible only for the selected online, available
  `gitPrimaryRoot` or `gitLinkedWorktree` WorkRoot.
- A toolbar branch chip/dropdown, `+ New branch...` modal, compact Git status
  pill, fetch/refresh action, and interactive push / ff-only pull segments.
- Conservative frontend refresh: immediate on selected WorkRoot change,
  explicit refresh, visibility/focus return, and Git mutations; polling only
  while the page is visible and only for the selected WorkRoot.
- Browser-level evidence against a daemon-served real Git WorkRoot.

Out of scope:

- Merge/rebase pull, conflict resolution, abort handling, stash workflows,
  force push, set-upstream creation, remote-branch checkout/tracking UX,
  branch deletion/rename, file-level Git operations, status chips for
  non-selected roots, and watcher-driven correctness.
- Generic Git panel redesign, multi-server forwarding, and broad visual-system
  refresh.

## Caller-Visible Contract

For the selected WorkRoot:

- Non-Git, offline, unavailable, missing, moved, or inaccessible roots do not
  render Git branch/status controls.
- Git-aware online available roots render:
  - a branch chip with current branch or a bounded detached `HEAD` label;
  - a branch dropdown with `+ New branch...` and local branches;
  - disabled branch entries for branches already checked out in another
    worktree when known;
  - a compact status pill using the segment grammar
    `+<added-lines> -<removed-lines> *<modified-files> ?<untracked-files> |
    ↑<ahead> ↓<behind>`;
  - a small always-visible fetch/refresh action;
  - push and `pull --ff-only` affordances only when upstream state makes them
    applicable.

Mutations follow Git defaults:

- branch switch uses Git checkout/switch semantics and may fail on dirty state;
- branch create creates and switches to the new branch from a selected base;
- fetch runs normal `git fetch`;
- push runs plain `git push`, never force;
- pull runs `git pull --ff-only`, never plain merge/rebase pull.

Failures are bounded and followed by status refresh when possible.

## Contract Instructions

Backend:

- Add routes under the owner-auth protected dashboard router.
- Route by opaque `workRootId`; do not expose Git roots or host paths in route
  identity, command logs, or bounded browser-visible errors.
- Reuse existing known WorkRoot access resolution and activation/availability
  gates. Only online, available Git WorkRoots are actionable.
- Run Git subprocess work off async workers.
- Status should report branch/detached state, upstream name when present,
  added/removed line totals, modified file count, untracked file count, ahead,
  behind, operation availability, and refreshed timestamp.
- Branch list should report local branches, current branch, checked-out state
  when known through `git worktree list`, and optional upstream/ahead/behind.
- Every mutation must revalidate the target WorkRoot and relevant branch state
  server-side before running Git.
- Return updated `WorkRootGitStatus` after mutations when possible; otherwise
  return bounded error plus enough frontend state to refresh status.
- `pull-ff-only` must use `git pull --ff-only`.

Frontend:

- Add command ids/builders for visible Git toolbar controls. Keep command
  payloads path-free.
- Fetch status for only the selected WorkRoot and ignore stale responses after
  selection changes.
- Pause polling while `document.hidden`; refresh immediately when visible again.
- Trigger immediate status refresh after fetch, push, pull-ff-only, branch
  switch, and branch creation, including failures.
- Use existing dashboard dark semantic tokens and toolbar/chip/menu/modal
  vocabulary; do not add raw palette colors.
- Keep branch/status UI inside the selected WorkRoot toolbar, not the left nav
  or a new top-level panel.

Temporary, fallback, or mock-only success paths are forbidden.

## Integration Test Instructions

Backend tests:

- Cover owner auth for every Git toolbar route.
- Cover visibility/gating for non-Git, offline, unavailable, unknown, and Git
  WorkRoots.
- Cover status counts for added lines, removed lines, modified files,
  untracked files, ahead, and behind.
- Cover branch list current branch and checked-out branch disabling.
- Cover branch switch success and dirty-state bounded failure.
- Cover new branch creation from current/base branch and checked-out race
  revalidation.
- Cover fetch refresh behavior, plain push success/failure, and
  `git pull --ff-only` success plus non-fast-forward failure without leaving a
  conflict state.
- Cover no host-path leakage in bounded errors.

Frontend tests:

- Extend `commands.test.ts` for new command builders and path-free payloads.
- Add focused helper tests if new API/polling modules are introduced.

Browser gate:

- Extend daemon-served Playwright acceptance to open a real Git WorkRoot and
  verify visibility gating, branch chip/dropdown, new branch modal, status pill
  segments, fetch/refresh, safe push/pull affordance behavior where practical,
  and immediate selected-root refresh after switching WorkRoots.

Expected verification commands:

- `cargo test -p ws-dashboard-daemon`
- `npm run test:commands`
- any new focused frontend helper test
- `npm run build`
- `npm run test:browser`

## Implementation Strategy Decisions

- Prefer extending or sharing the daemon Git helper introduced for worktree
  creation where it reduces duplication, but do not contort its add-worktree
  validation into toolbar status/mutation semantics.
- Use porcelain/status commands or equivalent structured Git output rather than
  ad hoc human output parsing where possible.
- Treat watcher-driven correctness as future scope; this slice uses explicit
  refresh and conservative polling as freshness hints.
- Keep push/pull behavior intentionally narrow: no force push, no set-upstream,
  no merge/rebase, and no conflict cleanup.

## Rejected Alternatives

- Rendering Git controls for all remembered roots regardless of availability.
- Running plain `git pull`.
- Adding branch delete/rename/remote-tracking workflows in the first pass.
- Adding a separate Git panel or making Git status a left-nav chip.
- Using host paths as command payloads or public route identity.

## Approach

- Add daemon Git status/branch/mutation helpers with route tests first.
- Add frontend API helpers and command ids.
- Add toolbar chip/dropdown/modal UI, then selected-root refresh/polling.
- Add browser acceptance over a temporary Git repo with an upstream where
  needed for ahead/behind and push/pull assertions.
- Run the full required verification set.

## Constraints

- Public vocabulary stays `workRootId`.
- Owner auth guards every new route.
- Server-side revalidation is mandatory for all mutations.
- Browser-visible Git failures must be bounded and path-free.
- UI changes require daemon-served browser evidence.

## Out of scope

See Scope Boundary.

## Details

Candidate route shapes:

```text
GET  /api/dashboard/work-roots/{workRootId}/git/status
GET  /api/dashboard/work-roots/{workRootId}/git/branches
POST /api/dashboard/work-roots/{workRootId}/git/switch-branch
POST /api/dashboard/work-roots/{workRootId}/git/branches
POST /api/dashboard/work-roots/{workRootId}/git/fetch
POST /api/dashboard/work-roots/{workRootId}/git/push
POST /api/dashboard/work-roots/{workRootId}/git/pull-ff-only
```

Candidate response/request shapes:

```ts
type WorkRootGitStatus = {
  available: boolean;
  reason?: string;
  branch?: { name?: string; detachedOid?: string; upstream?: string };
  changes: {
    addedLines: number;
    removedLines: number;
    modifiedFiles: number;
    untrackedFiles: number;
  };
  sync: { ahead: number; behind: number; upstream?: string };
  operations?: { fetching?: boolean; pushing?: boolean; pulling?: boolean };
  refreshedAtMs: number;
};

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

type SwitchBranchRequest = { branchName: string };
type CreateBranchRequest = {
  branchName: string;
  baseBranch?: string;
  switchTo: true;
};
```

## Verification Contract

Implementation is complete only when backend route tests, frontend
command/helper tests, production build, and daemon-served browser acceptance
pass, or when a real environment blocker is reported with exact failing
command/output.

The implementation report must list source commits, test results, any route or
type deviations, safe-pull evidence, host-path leakage coverage, and browser
evidence for visibility gating and selected-root refresh.

## References

- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard auth,
  command, resource, browser-gate, and WorkRoot/Git contracts.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md#260524-ws-dashboard-git-aware-workroot-toolbar`
  - planned caller-visible toolbar contract.
- [Must] `ws-dashboard/crates/daemon/src/router.rs`, `auth.rs` - protected
  route placement.
- [Must] `ws-dashboard/crates/daemon/src/resources.rs`,
  `persistent_state.rs`, `discovery.rs`, `git_worktree.rs` - WorkRoot access,
  Git kind gating, and existing Git helper patterns.
- [Must] `ws-dashboard/frontend/src/App.tsx`, `commands.ts`,
  `resourceModel.ts`, `resourceRefresh.ts` - toolbar UI, command dispatch, and
  selected-root refresh.
- [Must] `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`,
  `daemonHarness.ts` - daemon-served browser acceptance.

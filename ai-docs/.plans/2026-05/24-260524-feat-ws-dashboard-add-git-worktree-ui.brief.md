# Brief: 260524-feat-ws-dashboard-add-git-worktree-ui

## Intent

Add the first WorkRoot management operation for the dashboard: from a workspace
row, open a workspace-scoped overflow menu, preserve the existing remove
workspace action inside it, and create linked Git worktrees through daemon-owned
Git routes with branch/path preview and canonical resource refresh.

## Scope Boundary

Implement only Phase 1 of `260524-feat-ws-dashboard-add-git-worktree-ui`.

In scope:

- Replace direct workspace trash affordances with a workspace overflow menu.
- Add `Add worktree...` and `Remove workspace...` menu items.
- Preserve the existing `workspace.remove` command id, confirmation text, and
  registry-only remove semantics.
- Add owner-authenticated workspace-scoped daemon routes for Git worktree add
  options, preview, and submit.
- Add a frontend modal that collects worktree name, branch mode/name, path
  mode/path, daemon-backed preview status, and submit/cancel actions.
- Run `git worktree add -b <branchName> <targetPath>` for new branches and
  `git worktree add <targetPath> <branchName>` for existing branches.
- After successful submit, refresh/reconcile canonical dashboard resources and
  select/focus the created linked workRoot when the daemon can identify it.

Out of scope:

- Git worktree remove, branch deletion, branch rename, remote tracking setup,
  prunable cleanup, repair/recover flows, direct child workRoot forget/remove,
  broad file-manager actions, and multi-server forwarding.
- Git-aware selected WorkRoot toolbar chips, fetch/push/pull, branch switching,
  and new branch controls outside the add-worktree modal.
- General root picker redesign; custom target paths may reuse existing picker
  helpers only if this can stay inside the add-worktree target-path need.

## Caller-Visible Contract

Workspace rows expose an overflow/menu action instead of a standalone trash
button. The menu has stable command-routed behavior and contains:

- `Add worktree...` for Git-capable workspaces.
- `Remove workspace...` preserving current remove confirmation and behavior.

The add-worktree modal:

- Uses `worktreeName` as the primary input.
- Supports branch auto mode and manual existing-branch selection/input.
- Supports path auto mode using the Git root's
  `.git/ws-worktree/<filesystem-compatible-branch-name>` convention and custom
  target path entry.
- Shows green "new branch will be created", yellow "existing branch will be
  checked out", or red blocked preview severity from the daemon.
- Disables submit for blocked previews.
- Revalidates preview server-side on submit.
- On success, updates the resource tree from daemon-owned resources and selects
  the created workRoot id when present.

Host paths may appear in authenticated modal input/preview fields as owner-only
request data, but stable command payloads, command logs, browser-visible errors,
and diagnostics must avoid private absolute path leakage.

## Contract Instructions

Backend:

- Add routes inside the existing owner-auth protected dashboard router, not the
  top-level public router.
- Route by opaque `workspaceId`; do not introduce public `worktreeId` identity.
- Reuse existing resource/registry/discovery code as the source of truth.
- Ensure non-Git, unavailable, moved, or not-found workspaces get bounded route
  errors or blocked previews.
- Resolve Git data from the workspace's root Git workRoot. Branches already
  checked out in another worktree should be disabled when known.
- Preview and submit must share validation logic. Submit must re-resolve and
  revalidate rather than trusting prior frontend preview state.
- Return aggregated `DashboardResourcesView` plus optional `createdWorkRootId`.

Frontend:

- Add command ids/builders for new visible actions where the command model
  applies. Keep command payloads path-free.
- Menu/modal actions must be keyboard/mouse compatible and not bypass the
  existing command dispatcher for visible dashboard actions.
- Use existing dark dashboard tokens and overlay/menu/modal styling vocabulary.
- Preserve selected-resource reconciliation through the canonical resource
  refresh path; do not infer the created id from labels or paths.

Temporary, fallback, or mock-only behavior is forbidden for the main success
path. Fixture updates are acceptable only if existing fixture-backed tests
require them.

## Integration Test Instructions

Backend tests:

- Extend `ws-dashboard/crates/daemon/tests/routes.rs` or nearby daemon route
  tests to cover auth for options/preview/submit, branch-create preview/submit,
  existing-branch checkout preview/submit, checked-out branch blocking, invalid
  name blocking, target conflict blocking, unavailable/non-Git workspace
  handling, resource refresh, created workRoot selection, and bounded errors.

Frontend unit/route tests:

- Extend `commands.test.ts` for new command ids/builders/log labels.
- Add or extend focused frontend tests for preview/request helpers if new
  helper modules are introduced.

Browser gate:

- Extend `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` to exercise
  the daemon-served overflow menu and add-worktree modal against a real
  temporary Git workspace. Evidence must include menu visibility, remove action
  preservation, preview severity, successful creation, resource refresh, and
  selected created workRoot when available.

Expected verification commands:

- `cargo test -p ws-dashboard-daemon`
- `npm run test:commands`
- any new focused frontend helper test
- `npm run build`
- `npm run test:browser`

## Implementation Strategy Decisions

- Treat `Add worktree...` as a Git operation, not as root-picker open.
- Use Git's default safety behavior. Do not force, stash, merge, rebase,
  resolve conflicts, or invent branch policy.
- For auto branch/path, derive deterministic branch-compatible and
  filesystem-compatible names from `worktreeName`.
- Use `.git/ws-worktree/<filesystem-compatible-branch-name>` for auto path
  preview and submit.
- Use existing linked-worktree discovery and resource refresh to project the
  created workRoot into the workspace tree.
- Keep root path data daemon-owned and request-local; public API paths remain
  opaque ids.

## Rejected Alternatives

- Leaving the trash button as a direct workspace-row affordance.
- Treating add-worktree as a generic folder picker/open-root operation.
- Adding worktree removal or direct child workRoot removal in this slice.
- Inferring created workRoot selection in the browser from path or display
  label.
- Running plain branch checkout/switching or toolbar Git controls in this
  ticket.

## Approach

- Add shared daemon Git worktree preview/submit helpers, then route handlers.
- Add route tests for validation before wiring the UI deeply.
- Add frontend API helpers and command ids.
- Replace workspace row destructive button with an overflow menu and modal.
- Reuse resource refresh/reconciliation after submit.
- Extend browser acceptance once the end-to-end path works.

## Constraints

- No private absolute host paths in command payloads, logs, bounded errors, or
  browser diagnostics.
- Owner-auth must guard every new route.
- Public vocabulary stays `workspaceId`/`workRootId`, not Git worktree-specific
  ids.
- Submit must revalidate preview input server-side.
- Browser-facing UI changes require daemon-served browser evidence.

## Out of scope

See Scope Boundary.

## Details

Candidate backend route shapes:

```text
GET  /api/dashboard/workspaces/{workspaceId}/git-worktree-add/options
POST /api/dashboard/workspaces/{workspaceId}/git-worktree-add/preview
POST /api/dashboard/workspaces/{workspaceId}/git-worktree-add
```

Candidate frontend/public request shapes:

```ts
type GitWorktreeAddOptions = {
  workspaceId: string;
  git: { available: boolean; reason?: string; rootLabel: string };
  branches: Array<{
    name: string;
    checkedOut: boolean;
    current: boolean;
    disabledReason?: string;
  }>;
  defaults: { worktreeBaseDirLabel: string };
};

type GitWorktreeAddPreviewRequest = {
  worktreeName: string;
  branch: { mode: "auto" } | { mode: "manual"; name: string };
  path: { mode: "auto" } | { mode: "custom"; targetPath: string };
};

type GitWorktreeAddPreview = {
  branchName: string;
  filesystemName: string;
  targetPathLabel: string;
  status: "willCreateBranch" | "willCheckoutExisting" | "blocked";
  message: string;
  blockers: Array<{
    code:
      | "invalidWorktreeName"
      | "invalidBranchName"
      | "branchAlreadyCheckedOut"
      | "targetExists"
      | "targetParentMissing"
      | "notGitWorkspace";
    field?: "worktreeName" | "branch" | "path";
    message: string;
  }>;
};

type AddGitWorktreeRequest = GitWorktreeAddPreviewRequest & {
  activate: boolean;
};

type AddGitWorktreeResponse = {
  resources: DashboardResourcesView;
  createdWorkRootId?: string;
};
```

## Verification Contract

Implementation is complete only when backend, frontend command/helper tests,
production build, and daemon-served browser acceptance pass, or when a real
environment blocker is reported with exact failing command/output.

The implementation report must list:

- source commits;
- tests run with pass/fail output summary;
- any deviations from the route/type sketches;
- whether created workRoot selection came from daemon response/resource refresh;
- confirmation that host-path leakage checks were covered.

## References

- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - auth boundary, command
  model, resource refresh, Git/workRoot identity, browser-gate requirements.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md#260524-ws-dashboard-git-worktree-creation`
  - planned caller-visible contract.
- [Must] `ws-dashboard/crates/daemon/src/router.rs`, `auth.rs` - protected
  route placement.
- [Must] `ws-dashboard/crates/daemon/src/resources.rs`,
  `persistent_state.rs`, `discovery.rs` - registry, live resources, linked
  worktree projection.
- [Must] `ws-dashboard/frontend/src/App.tsx`, `commands.ts`,
  `resourceModel.ts`, `resourceRefresh.ts` - workspace row UI, command routing,
  resource reconciliation.
- [Must] `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`,
  `daemonHarness.ts` - daemon-served browser acceptance.
- [Maybe] `ws-dashboard/frontend/src/rootPicker.ts`, `openWorkRoot.ts` -
  target-path helper patterns.

---
title: Add Git worktree creation from the dashboard workspace menu
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-linked-worktree-discovery: created worktrees should appear through the existing linked-worktree discovery projection
  260524-feat-ws-dashboard-workspace-forget-remove-ui: workspace remove moves behind the same overflow menu
  260523-feat-ws-dashboard-workroot-registry-activation: created worktrees should reuse durable workspace/workRoot activation and availability
spec:
  - 260524-ws-dashboard-git-worktree-creation
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-24
---

# Add Git worktree creation from the dashboard workspace menu

## Background

The dashboard already detects Git primary roots and linked Git worktrees, keeps
workspace roots separate from derived child workRoots, and exposes workspace
removal as a dashboard-only owner action. The remaining WorkRoot Management
gap is a practical Git-aware creation flow: users should be able to add a new
linked worktree from a workspace row without dropping to a shell or using the
root picker as a generic file manager.

The current left navigation exposes workspace removal directly as a trash icon
on workspace or compact-workRoot rows. That single destructive affordance should
move behind a workspace overflow menu so additional workspace-scoped actions
can share one conventional entrypoint.

## Decisions

- Replace the workspace-row trash button with a `...` overflow menu. The menu
  should include `Add worktree...` and `Remove workspace...`.
- Keep `Remove workspace...` on the existing `workspace.remove` command and
  confirmation flow. Moving it into the menu is an affordance change, not a
  filesystem or registry behavior change.
- `Add worktree...` is a Git operation, not a root-picker open operation. It
  should create a linked Git worktree through `git worktree add`, then rely on
  the existing linked-worktree discovery and resource refresh pipeline so the
  created child workRoot appears under the same workspace.
- The add-worktree modal should use `worktreeName` as the primary input. Auto
  branch and auto path previews derive from that name unless the user disables
  the relevant auto checkbox.
- Auto branch naming means "derive a branch-compatible candidate from the
  worktree name", not "always create a new branch". The daemon preview decides
  whether the candidate creates a new branch, checks out an existing branch, or
  is blocked.
- Auto path naming should resolve to the workspace Git root's
  `.git/ws-worktree/<filesystem-compatible-branch-name>` convention. Custom
  path selection may reuse the existing explorer-style picker in a target-path
  or parent-directory mode, but it must not expose generic delete, rename,
  move, copy, or recursive filesystem operations.
- Existing branch selection should come from daemon-resolved Git branch data.
  Branches that are already checked out in another worktree should be disabled
  when known, and submit must still revalidate to handle races.
- The modal should show branch/path resolution status with visible severity:
  green for "new branch will be created", yellow for "existing branch will be
  checked out", and red for "cannot check out/create with the given inputs".
- Successful creation should activate the new workRoot by default, refresh the
  canonical resource tree, and select or focus the created linked workRoot when
  the daemon can identify it after refresh.
- Do not add `git worktree remove`, branch deletion, branch rename, remote
  branch tracking, prunable cleanup, repair/recover flows, or direct child
  workRoot forget/remove controls in this ticket.
- Host paths may be authenticated request data inside the modal and daemon
  route, but command payloads, logs, and browser-visible diagnostics must avoid
  leaking private absolute paths.

## API Sketch

Workspace-scoped add-worktree options should provide Git availability,
branch data, and base convention metadata:

```ts
type GitWorktreeAddOptions = {
  workspaceId: string;
  git: {
    available: boolean;
    reason?: string;
    rootLabel: string;
  };
  branches: Array<{
    name: string;
    checkedOut: boolean;
    current: boolean;
    disabledReason?: string;
  }>;
  defaults: {
    worktreeBaseDirLabel: string;
  };
};
```

The preview route resolves the user's current modal inputs before submit:

```ts
type GitWorktreeAddPreviewRequest = {
  worktreeName: string;
  branch:
    | { mode: "auto" }
    | { mode: "manual"; name: string };
  path:
    | { mode: "auto" }
    | { mode: "custom"; targetPath: string };
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
```

Submit should re-resolve and revalidate the same inputs before running Git:

```ts
type AddGitWorktreeRequest = {
  worktreeName: string;
  branch:
    | { mode: "auto" }
    | { mode: "manual"; name: string };
  path:
    | { mode: "auto" }
    | { mode: "custom"; targetPath: string };
  activate: boolean;
};

type AddGitWorktreeResponse = {
  resources: DashboardResourcesView;
  createdWorkRootId?: string;
};
```

Green preview status means submit will run a create-branch worktree operation
such as `git worktree add -b <branchName> <targetPath>`. Yellow preview status
means submit will run a checkout-existing operation such as
`git worktree add <targetPath> <branchName>`. Red preview status disables
submit and must also be enforced by the daemon route.

## Phases

### Phase 1: Add workspace Git worktree creation UI

Add a workspace overflow menu in the left navigation, move existing workspace
removal into `Remove workspace...`, and add `Add worktree...` for Git-capable
workspaces. The modal should collect worktree name, branch resolution, and path
resolution with auto defaults and daemon-backed preview status.

The backend should add owner-authenticated workspace-scoped options, preview,
and submit routes. Submit should create a linked worktree through Git only after
revalidating the preview inputs, persist or synchronize any required registry
state through existing resource refresh paths, then return the aggregated
dashboard resources view and the created workRoot id when available.

The frontend should refresh/reconcile resources after success, select or focus
the created workRoot when possible, and keep existing linked-worktree discovery
as the source of truth for child workRoot projection. Existing branch reuse is
a normal yellow path; branch creation is a normal green path; checked-out
branches, invalid names, unavailable Git roots, and path conflicts are blocked
with bounded red status.

Deferred scope: Git worktree removal, branch deletion, branch rename, remote
branch tracking, prunable cleanup, worktree repair/recover flows, direct child
workRoot forget controls, broad filesystem manager actions, and multi-server
forwarding of Git operations.

Verification should cover workspace overflow menu behavior, preservation of
the existing remove confirmation flow, options/preview/submit route auth,
branch creation, existing-branch checkout, checked-out branch blocking, invalid
name blocking, target-path conflict blocking, resource refresh and selection,
activation defaulting, no direct child workRoot remove action, and no private
host-path leakage in command logs or bounded errors. Browser-level evidence
should exercise the daemon-served modal against a real temporary Git workspace.

### Result (c8695a70) - 2026-05-24

Implemented owner-authenticated workspace Git worktree creation from the
dashboard left navigation. Workspace rows now expose an overflow menu that
contains `Add worktree...` and preserves `Remove workspace...` through the
existing confirmation path. The daemon exposes workspace-scoped
options/preview/submit routes, validates branch/path inputs, blocks checked-out
branches, invalid names, non-Git roots, and path conflicts, revalidates on
submit, runs the corresponding `git worktree add` operation, and returns
canonical resources with `createdWorkRootId` when it can identify the linked
workRoot.

The frontend adds path-free command builders, request helpers, a modal with
daemon-backed preview severity, stale preview/submit guards, semantic-token
preview styling, canonical resource reconciliation, and selection of the
daemon-created workRoot id. Browser acceptance covers the workspace menu,
remove action preservation, new-branch preview/submit, resource refresh, and
created row selection against a real temporary Git workspace. Review fixes
tightened submit-time blocked preview surfacing, branch-checkout assertions,
blocked submit no-side-effect coverage, and browser fixture isolation after
opening multiple workRoots.

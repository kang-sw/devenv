# Survey Implementation Plan: 260524-feat-ws-dashboard-add-git-worktree-ui Phase 1

Brief: `ai-docs/.plans/2026-05/24-260524-feat-ws-dashboard-add-git-worktree-ui.brief.md`
Ticket: `ai-docs/tickets/ready/260524-feat-ws-dashboard-add-git-worktree-ui.md`
Scope: Phase 1 only — workspace overflow menu, Git worktree creation modal, owner-authenticated workspace-scoped options/preview/submit routes, existing resource/discovery/registry refresh, frontend command model, and browser tests.

## Reusable Components

- `ws-dashboard/frontend/src/commands.ts`
  - Reuse the dashboard command builder/dispatcher/observer path for visible workspace controls.
  - Add command identities for opening the workspace overflow menu / add-worktree modal only if current command taxonomy requires explicit IDs for these controls.
  - Keep path-bearing data out of command payloads and command logs.

- `ws-dashboard/frontend/src/App.tsx`
  - Existing left-nav workspace rows, compact workRoot rows, root picker modal integration, remove workspace confirmation, and resource selection/reconciliation likely live here.
  - Replace direct trash affordance with a workspace overflow entrypoint while preserving the existing `workspace.remove` command and confirmation flow.

- `ws-dashboard/frontend/src/resourceModel.ts`
  - Reuse resource hierarchy helpers and compact workspace/workRoot selection helpers.
  - Do not invent Git-worktree identity in frontend resource paths; linked worktrees remain workRoots.

- `ws-dashboard/frontend/src/resourceRefresh.ts`
  - Reuse canonical resource refresh/reconcile behavior after successful worktree creation.
  - Success should apply returned resources and select/focus `createdWorkRootId` when present, then rely on canonical resources as source of truth.

- `ws-dashboard/frontend/src/rootPicker.ts` and existing root-picker UI code
  - Reuse only for custom target path or parent-directory selection if practical.
  - Do not expand this ticket into generic file manager actions.

- `ws-dashboard/crates/daemon/src/router.rs` and `auth.rs`
  - Add new routes under the existing owner-auth protected dashboard router.
  - Preserve bearer/cookie auth behavior and Host/Origin protections.

- `ws-dashboard/crates/daemon/src/resources.rs`, `persistent_state.rs`, and `discovery.rs`
  - Reuse existing registry, live resource aggregation, activation, availability, and linked-worktree discovery.
  - Submit should return aggregated dashboard resources, not only a just-created candidate.

- Existing browser acceptance harness under `ws-dashboard/frontend/e2e/`
  - Reuse daemon-served production frontend flow, owner pairing, temporary workRoot setup, and browser assertions.

## Existing Patterns

- Owner-authenticated dashboard APIs live behind the protected router; `/pair` is the only unauthenticated browser entrypoint.
- Public API/resource identity uses opaque `serverId`, `workspaceId`, and `workRootId`; absolute host paths are daemon-private except where authenticated picker/request data is explicitly needed.
- Workspace/workRoot rows are browser presentation over a full `server -> workspace -> workRoot` resource model; singleton compaction must not become daemon-side pre-collapse.
- Resource-changing operations should persist/synchronize daemon registry state before advertising success, then refresh or return the canonical resource view.
- Existing open-workRoot behavior reconciles immediate successful responses and then treats `/api/dashboard/resources` as canonical; Git worktree submit should follow the same shape.
- Visible controls should route through stable dashboard command IDs so future keyboard bindings can invoke the same behavior.
- UI-facing work requires browser-level evidence against the daemon-served production frontend; unit tests and build checks are not sufficient by themselves.

## Relevant Interfaces

- Backend options route, workspace scoped:
  - Provides Git availability, branch data, checked-out/current branch metadata, and default base path label.
  - Must distinguish non-Git/unavailable workspace from usable Git root.

- Backend preview route, workspace scoped:
  - Input shape: `worktreeName`, branch mode (`auto` or `manual`), path mode (`auto` or `custom`).
  - Output shape: resolved branch name, filesystem name, target path label, status, message, and bounded blockers.
  - Status semantics:
    - `willCreateBranch` — green path; submit would create branch and linked worktree.
    - `willCheckoutExisting` — yellow path; submit would checkout existing branch.
    - `blocked` — red path; submit disabled and daemon-enforced.

- Backend submit route, workspace scoped:
  - Re-resolves and revalidates preview inputs before running Git.
  - Runs the safe equivalent of:
    - `git worktree add -b <branchName> <targetPath>` for create-branch path.
    - `git worktree add <targetPath> <branchName>` for existing-branch path.
  - Activates created workRoot by default when requested.
  - Refreshes/synchronizes canonical dashboard resources.
  - Returns `DashboardResourcesView` plus optional `createdWorkRootId`.

- Frontend modal:
  - Collects worktree name, branch resolution, and path resolution.
  - Auto branch derives a branch-compatible candidate from worktree name.
  - Auto path derives `.git/ws-worktree/<filesystem-compatible-branch-name>` label from daemon-side resolution.
  - Existing-branch checkout is a normal yellow path, not an error.
  - Blockers must map to fields and disable submit.

- Browser tests:
  - Workspace overflow menu replaces direct trash affordance.
  - Remove workspace menu entry preserves existing confirmation flow.
  - Add-worktree modal loads options and preview.
  - Branch creation succeeds.
  - Existing branch checkout succeeds when not checked out elsewhere.
  - Checked-out branch, invalid names, unavailable Git root, and target conflict block with bounded errors.
  - Successful submit refreshes resources and selects/focuses created workRoot when daemon can identify it.
  - No direct child workRoot remove action appears.
  - Command logs and browser-visible errors do not leak private host paths.

## Constraints

- Phase 1 only. Do not add Git worktree removal, branch deletion, branch rename, remote branch tracking, cleanup/prune, repair/recover flows, direct child workRoot forget/remove controls, or broad filesystem manager actions.
- Keep workspace removal behavior unchanged except for moving its affordance into the overflow menu.
- Git worktree creation is a Git operation, not root-picker open semantics.
- Do not expose absolute host paths in command payloads, logs, browser-visible diagnostics, public resource IDs, or route identity.
- Custom path selection may reuse picker UI only for target path / parent-directory selection; it must not add delete, rename, move, copy, or recursive filesystem operations.
- Submit must not trust stale preview state; it must revalidate immediately before Git execution.
- Unknown workspace/workRoot IDs, offline activation, degraded availability, non-Git roots, and path conflicts should produce bounded not-found/offline/unavailable/blocked responses.
- Browser resource authority remains daemon-owned; frontend must not infer created IDs from labels, paths, or row order.
- Tests should use real temporary Git repositories/worktrees where behavior depends on Git, not only fixtures.

## Risk Signals

- Direct click handler mutation instead of command dispatch would fork future keyboard behavior and lose command observer evidence.
- Returning only the created workRoot from submit would drop other known registry entries or desynchronize the tree.
- Inferring `createdWorkRootId` in the browser from label/path/order can select the wrong row when labels collide.
- Moving remove workspace into a menu could accidentally skip the existing confirmation path.
- Auto branch/path preview can become misleading if submit does not share the exact resolver/revalidator.
- Existing branch checkout is intentionally yellow; treating it as blocked would violate the ticket.
- Checked-out branch detection is race-prone; daemon submit must still block even if preview was green/yellow.
- Route errors or logs that include raw command lines with absolute paths would violate dashboard privacy boundaries.
- React Aria/menu/modal focus can interfere with Dockview/xterm focus; browser evidence should cover open, close, Escape, focus restore, and ordinary row selection.
- Polling/refresh races can overwrite newer submit results; reuse the resource refresh coordinator’s stale-result guards.

## Opinion

Implement this as a narrow vertical slice with shared backend resolution logic first: one resolver should power options-derived defaults, preview, and submit revalidation so UI status and Git execution cannot drift. On the frontend, make the overflow/menu/modal mostly an adapter around existing command dispatch and resource refresh rather than a new resource authority. For tests, prioritize daemon-served browser coverage over isolated component assertions because the highest risks are auth, command/log privacy, focus/menu behavior, and real Git/resource refresh reconciliation.

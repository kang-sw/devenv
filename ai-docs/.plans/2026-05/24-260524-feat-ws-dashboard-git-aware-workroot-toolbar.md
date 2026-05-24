# Survey Implementation Plan: 260524-feat-ws-dashboard-git-aware-workroot-toolbar Phase 1

Brief: `ai-docs/.plans/2026-05/24-260524-feat-ws-dashboard-git-aware-workroot-toolbar.brief.md`
Ticket: `ai-docs/tickets/ready/260524-feat-ws-dashboard-git-aware-workroot-toolbar.md`
Scope: Phase 1 only — selected WorkRoot Git toolbar branch/status chips; daemon status/branches/switch/create/fetch/push/pull-ff-only routes; conservative polling; no merge/rebase/force/stash or file-level Git operations.

## Reusable Components

- `ws-dashboard/crates/daemon/src/router.rs` and `auth.rs`
  - Add Git toolbar APIs under the existing owner-auth protected dashboard router.
  - Preserve bearer/cookie auth behavior, Host/Origin protections, and the rule that `/pair` is the only unauthenticated browser entrypoint.

- `ws-dashboard/crates/daemon/src/resources.rs`, `persistent_state.rs`, and `discovery.rs`
  - Reuse daemon-owned `workRootId` lookup, registry membership, activation, availability, and Git kind classification.
  - Gate all Git toolbar routes to known, online, available Git-aware WorkRoots (`gitPrimaryRoot` or `gitLinkedWorktree`).

- `ws-dashboard/crates/core/src/ids.rs`, `resources.rs`, and `view_model.rs`
  - Preserve public resource vocabulary: `workRoot`, `workRootId`, and opaque route identity.
  - Do not introduce public `worktreeId` or host-path identity for Git toolbar behavior.

- Existing daemon Git execution seams
  - Add a narrow dashboard Git executor/read-model seam for status, branch list, switch, create, fetch, push, and ff-only pull.
  - Prefer an injectable command runner/test seam so route tests can cover Git output parsing and failures without depending entirely on live Git fixtures.

- `ws-dashboard/frontend/src/App.tsx`
  - Reuse the selected WorkRoot topbar/toolbar region for branch chip, status pill, dropdown, and new-branch modal lifecycle.
  - Keep Git toolbar rendering selected-WorkRoot-scoped, not workspace-wide.

- `ws-dashboard/frontend/src/commands.ts`
  - Route visible toolbar actions through stable dashboard command IDs: branch menu open, branch switch, new branch modal open/submit, fetch/refresh, push, and pull-ff-only.
  - Keep command payloads limited to logical targets such as `workRootId`; no host paths or Git roots.

- `ws-dashboard/frontend/src/resourceModel.ts`
  - Reuse resource kind/availability/activation helpers to decide whether Git controls should render.

- `ws-dashboard/frontend/src/resourceRefresh.ts`
  - Coordinate selected WorkRoot changes and canonical resource refreshes with Git status refreshes without making browser polling the authority.

- New or existing frontend Git helper module
  - Add typed wrappers for status, branches, switch branch, create branch, fetch, push, and pull-ff-only routes.
  - Keep route URLs workRoot-id scoped and request bodies minimal.

- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` and `daemonHarness.ts`
  - Extend daemon-served Playwright coverage with a real temporary Git WorkRoot.
  - Cover visible chips, dropdown/modal, mutations, polling pause/resume, and no private path leakage in UI/command evidence.

## Existing Patterns

- Protected dashboard APIs are registered inside the owner-auth router, not beside `/pair`.
- Browser-visible resource identity is daemon-owned and opaque; host paths/Git roots remain private except authenticated request data where explicitly allowed.
- WorkRoot `activation` and `availability` are distinct. Online activation alone is not enough; Git routes also require current availability and Git-aware kind.
- Visible browser controls route through `commands.ts`, giving click behavior and future keybindings one action path and observer evidence.
- Resource refresh is canonical and daemon-owned. Polling is a freshness hint and must not overwrite newer explicit mutation results.
- UI-facing work closes only with browser-level evidence against the daemon-served production frontend; unit tests, API curls, and Vite builds are supporting checks, not sufficient evidence alone.
- Git operations should follow Git defaults and return bounded failures rather than implementing dashboard-specific policy machinery.

## Relevant Interfaces

- `GET /api/dashboard/work-roots/{workRootId}/git/status`
  - Returns a daemon-owned `WorkRootGitStatus` read model.
  - Includes availability/reason, current branch or detached OID, line/file change counts, upstream ahead/behind, operation flags if implemented, and `refreshedAtMs`.
  - Must not scan non-Git WorkRoots.

- `GET /api/dashboard/work-roots/{workRootId}/git/branches`
  - Returns current branch or detached OID plus local branches.
  - Branch rows include `name`, `current`, `checkedOut`, optional upstream/ahead/behind, and optional `disabledReason`.
  - Checked-out branches are disabled when known; mutations still revalidate server-side.

- `POST /api/dashboard/work-roots/{workRootId}/git/switch-branch`
  - Body: `{ branchName: string }`.
  - Runs Git branch switch using Git defaults.
  - Revalidates branch existence/checked-out/operability before mutation.
  - Returns updated status when possible, or bounded command result plus follow-up refresh path.

- `POST /api/dashboard/work-roots/{workRootId}/git/branches`
  - Body: `{ branchName: string; baseBranch?: string; switchTo: true }`.
  - Creates and switches to the new branch from current/base branch.
  - Create-without-switch is out of scope.

- `POST /api/dashboard/work-roots/{workRootId}/git/fetch`
  - Runs a safe fetch/refresh operation and returns updated status when possible.
  - Also backs the always-rendered refresh icon in the status pill.

- `POST /api/dashboard/work-roots/{workRootId}/git/push`
  - Runs plain `git push` only when upstream/pending-push state makes the segment applicable.
  - Never force pushes.

- `POST /api/dashboard/work-roots/{workRootId}/git/pull-ff-only`
  - Runs `git pull --ff-only` only when safe pull interaction is applicable.
  - Non-fast-forward failure becomes bounded error plus refreshed status; no merge/rebase state.

- Frontend status pill
  - Segment grammar: `+<added-lines> -<removed-lines> *<modified-files> ?<untracked-files> | ↑<ahead> ↓<behind>`.
  - Colors: green additions/ahead, red removals, yellow modified/behind, indigo-blue untracked.
  - Zero-value segments may hide, but refresh icon stays visible while Git controls render.

- Frontend polling
  - Refresh immediately on selected WorkRoot change, focus/visibility return, explicit refresh/fetch, branch switch/create, push, and pull-ff-only.
  - Poll conservatively only for the selected visible WorkRoot.
  - Pause while document is hidden and ignore stale in-flight responses after selection changes or newer mutation results.

## Constraints

- Phase 1 only. Do not implement merge/rebase pull, conflict resolution, abort handling, stash workflows, force push, set-upstream creation, remote branch checkout/tracking UX, branch deletion/rename, file-level Git operations, status chips for non-selected roots, or watcher-driven correctness.
- Git controls render only for selected Git-aware WorkRoots that are known, online, and currently available.
- All routes are owner-authenticated and workRoot-id scoped.
- Git work stays off async workers in this phase.
- Browser command payloads, logs, URLs, tooltips, bounded errors, screenshots, and test notes must avoid private host paths and Git roots.
- Mutating routes must revalidate immediately before running Git; frontend disabled states are not security or correctness boundaries.
- Dirty working trees and Git command failures should remain Git-default bounded failures plus status refresh, not dashboard-invented remediation flows.
- Polling is host-light and selected-root-only; no broad repo scanning or all-workRoot status polling.
- Linked worktrees are WorkRoot kind metadata, not a new public identity vocabulary.

## Risk Signals

- Registering routes outside the protected router would bypass owner auth.
- Collapsing activation and availability can show or run Git controls for offline/unavailable roots.
- Parsing Git status loosely can miscount line/file changes, ahead/behind, detached HEAD, or untracked files.
- Branch checked-out state can race; submit-time revalidation is mandatory even if the dropdown disabled the row.
- Plain `git pull` or any merge/rebase fallback would violate the safe ff-only contract.
- Adding push with force flags, stash, cleanup, file-level operations, or branch deletion/rename would exceed Phase 1.
- Polling every known WorkRoot or polling while hidden can make the dashboard host-heavy.
- Stale poll responses after selection/mutation can display the wrong branch/status for the selected WorkRoot.
- Command logs or browser diagnostics can leak absolute paths if raw Git command lines or stderr are surfaced directly.
- Browser-only UI assertions without a real temporary Git WorkRoot can miss route auth, real Git behavior, and resource gating bugs.
- Dropdown/modal focus behavior can conflict with existing workbench/terminal focus; browser evidence should cover open, close, Escape, selection, and focus restore.

## Opinion

Implement the backend read model and mutations as the spine first, with shared workRoot gating and a single Git execution/parsing seam used by all routes. Keep route responses small, bounded, and frontend-shaped so the toolbar does not parse raw Git output. On the frontend, treat the toolbar as selected-resource decoration plus command-routed actions, not as a second resource authority. Build polling last, after explicit refresh/mutation paths work, because stale-response handling and selected-root scoping are the highest frontend correctness risks. For verification, combine route/parser tests with one daemon-served browser test against a real temporary Git WorkRoot that proves visibility gating, branch controls, sync actions, polling pause/resume, and no private path leakage.

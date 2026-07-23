# Plan: 260525-feat-ws-dashboard-workroot-polishing-backlog — Phase 3: Worktree removal & hide UX (B-1 confirm modal, B-2 branch-delete checkbox + unmerged detection, B-3 hide/unhide) + prerequisite git worktree remove daemon op from Phase 1

## Relevant Ticket Contract

- Phase 1 prerequisite: add the missing daemon op — atomic `git worktree
  remove` (force-gated on uncommitted/untracked changes), dashboard registry
  cleanup, and active terminal/agent-session accounting for sessions rooted
  in the removed worktree. `root_picker.rs`'s `remove_workspace` only
  unregisters from `OpenedWorkRoots`; it never runs `git worktree remove` —
  the new op must do both atomically.
- B-1: worktree removal **always** shows a confirmation modal (never
  conditional on dirty state — worktree add/remove is heavy regardless).
  When the worktree has uncommitted/untracked changes, the modal
  additionally shows a **red** data-loss warning, distinct from baseline
  confirmation copy.
- B-2: the same modal has a "delete branch too" checkbox, **default OFF**.
  Before submit, check whether the branch has unmerged/dangling commits —
  the same condition under which plain `git branch -d` would refuse. If so,
  show a **red** parenthetical warning next to the checkbox (example UI
  string: "아직 머지되지 않았습니다"). Checking the box on a safe (merged/no
  unique commits) branch simply deletes it. **Never silently force-delete a
  branch with dangling commits** — this must never become `git branch -D`.
- B-3: hide is presentation-only — worktree directory and branch are
  untouched; hiding only removes the row from the dashboard's visible list.
  This is NOT `git worktree remove` and NOT a "forget" op. Restore path: the
  root workRoot's right-side "..." settings menu gains a "hidden worktrees"
  submenu; clicking a hidden entry there un-hides it. Confirmed pure
  browser-local UI state (supersedes the invisible-worktree rejection in
  `260523-research-ws-dashboard-persistable-ui-state-map`, reversed
  2026-07-22).
- Both daemon op and Phase 3 UX are to be implemented together, per the
  ticket's 2026-07-22 priority note — not sequenced as daemon-first /
  UX-follow-up.
- Verification boundary (ticket text): resource model tests for the branch
  unmerged/dangling check; browser coverage for the confirmation modal (both
  warning states) and the hide/unhide flow.

## Out of Scope

- Phase 1's worktree-placement migration (`.git/ws-worktree/<name>` →
  `.ws-dashboard/worktrees/<name>`) — separate future slice per the ticket's
  "Long-term worktree placement direction" note.
- Phase 1's general WorkRoot lifecycle polish beyond the remove op (recovery
  states, refresh timing, pinned-directory behavior, workspace grouping
  clarity).
- Phase 2 (git toolbar polish / `.git/index.lock` contention / status
  polling single-flight guard) entirely.
- Removing/forgetting the *primary* Git root or a whole workspace — that is
  the existing `workspace.remove` op (`root_picker.rs::remove_workspace`,
  frontend `workspace.remove` command) and is untouched by this plan. This
  plan's remove op targets `WorkRootKind::GitLinkedWorktree` rows only.
- New hotkey bindings beyond filling the already-reserved `"g w h"` gap
  comment in `hotkeys.ts` for opening the hidden-worktrees view (see Finding
  below) — no other new keybindings are implied by the ticket.
- Remote/SSH-linked-server manual QA — server-scoped wiring is added to
  preserve the existing architectural invariant (every mutating dashboard op
  is server-scoped, per spec `## Remote Dashboard Resource Gatewaying`), but
  is mechanical (mirrors `remove_workspace`/`git-worktree-add` 1:1) and not
  independently browser-tested against a real linked daemon.

## Codebase Findings

**Daemon — existing worktree-add op to mirror**
- `ws-dashboard/crates/daemon/src/git_worktree.rs#L155-L285` —
  `git_worktree_add_submit`: the exact shape to mirror — resolve Git
  context, run `git` via `std::process::Command`, on success
  register/unregister the registry entry under
  `state.registry_persist_lock` with rollback-on-persist-failure, then
  return a fresh `live_dashboard_resources(&state.opened_work_roots)`.
- `ws-dashboard/crates/daemon/src/git_worktree.rs#L429-L491` —
  `resolve_workspace_git`: resolves a *workspace id* to its primary/
  available Git root path + `common_dir` + branch inventory. The new remove
  op is addressed by `work_root_id` (the specific linked worktree), not
  `workspace_id`, so this needs a sibling resolver that (a) finds the
  workspace containing the given `work_root_id` via
  `live_dashboard_resources`, then (b) reuses this same primary-root
  resolution — because `git worktree remove <path>` and `git branch -d`
  must run with `-C <primary-root-path>`, not `-C <worktree-being-removed>`
  (you cannot remove a worktree from a `-C` context rooted inside itself).
  **Risk signal**: this primary-root-vs-target-worktree path distinction is
  not obvious from the ticket text and is the single most important
  plumbing detail to get right.
- `ws-dashboard/crates/daemon/src/git_worktree.rs#L212-L240` — the
  `git worktree add` `Command` construction pattern (`-C <root>`, arg
  building per status) to mirror for `git worktree remove [--force]
  <target_path>`.

**Daemon — dirty-state check (reuse, do not reimplement)**
- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L440-L482` —
  `changes_for_path`: already computes `modified_files`/`untracked_files`
  via `git --no-optional-locks status --porcelain=v1 --untracked-files=all`
  against the **worktree's own path** (not the primary root). This is
  exactly B-1's uncommitted/untracked signal and the daemon-side force-gate
  signal for Phase 1's prerequisite — reuse `changes_for_path`, do not
  hand-roll a second porcelain parser. The existing `git_status` route
  (`git_toolbar.rs#L134-148`) is already fetchable per-`work_root_id` and
  could either be reused directly by the frontend before opening the modal,
  or the new preview endpoint can call `changes_for_path` server-side and
  bundle it into one payload — prefer the latter (one request, no dirty
  read/modal-open race).

**Daemon — branch-unmerged detection (no existing helper; must add)**
- No existing code computes "would `git branch -d` refuse". Git's own
  semantics (see `git-branch(1)`): a branch is safe to delete with `-d` if
  it is an ancestor of its configured upstream (`@{upstream}`), or, if no
  upstream is configured, an ancestor of the *invoking repo's* current
  `HEAD`. The non-destructive equivalent is `git merge-base --is-ancestor
  <branch> <ref>` (exit 0 = safe/merged, exit 1 = unmerged/dangling), run
  with `-C <primary-root-path>` against `ref = branch's configured upstream
  if any, else the primary root's current HEAD`. Do **not** implement this
  by actually invoking `git branch -d` as a "dry run" — it has no dry-run
  flag, and this must be a non-mutating preview available before the owner
  submits.
- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L502-L509` — `rev_counts`
  and the `rev-list --left-right --count` pattern are the existing
  ahead/behind idiom; `merge-base --is-ancestor` is the analogous
  non-mutating primitive to add alongside `git_text`/`run_git` in whichever
  module hosts the new op.

**Daemon — session/terminal accounting (reuse directly)**
- `ws-dashboard/crates/daemon/src/root_picker.rs#L323-L372` —
  `remove_workspace`: the exact registry-cleanup + rollback +
  session-accounting pattern to mirror for the new op: `_persist_guard =
  state.registry_persist_lock.lock().await`, `unregister` +
  `persist_opened_work_roots`, roll back registered entries on persist
  failure, then unconditionally (on success) call
  `state.terminals.remove_for_work_roots(&ids)`,
  `state.codex_sessions.remove_for_work_roots(&ids)`,
  `state.claude_sessions.remove_for_work_roots(&ids)`. For a single-worktree
  remove, `ids` is the one-element `BTreeSet` containing the removed
  `work_root_id`.
- `ws-dashboard/crates/daemon/src/terminal.rs#L181`,
  `ws-dashboard/crates/daemon/src/codex_app_server.rs#L664`,
  `ws-dashboard/crates/daemon/src/claude_cli.rs#L642` — the three
  `remove_for_work_roots(&BTreeSet<WorkRootId>) -> usize` signatures, all
  already `BTreeSet`-keyed and directly reusable without modification.

**Daemon — route registration + server-scoped mirroring**
- `ws-dashboard/crates/daemon/src/router.rs#L157-L172` and `#L290-L306` —
  the unscoped + `{server_route}`-scoped route-pair registration pattern
  for `workspaces/{workspace_id}` (DELETE) and `git-worktree-add` (GET
  options / POST preview / POST submit). New routes should follow the same
  pairing, e.g. `work-roots/{work_root_id}/git-worktree-remove/preview`
  (GET or POST) and `work-roots/{work_root_id}/git-worktree-remove`
  (POST or DELETE with JSON body — DELETE-with-body is supported by the
  existing `parse_json_alias_body` helper, which is method-agnostic).
- `ws-dashboard/crates/daemon/src/servers.rs#L610-L643` —
  `ServerScopedForwardOperation::remove_workspace` /
  `git_worktree_add_submit` construction plus
  `server_scoped_remove_workspace` / `server_scoped_git_worktree_add_submit`
  handlers (`#L1019-L1078`) — the exact local-dispatch-vs-forward pattern
  to mirror. `ForwardResponseRewrite::Resources` (bare
  `DashboardResourcesView`, `servers.rs#L525-529`) is the correct rewrite
  variant since the new submit response is a bare resources view like
  `remove_workspace`, not the nested-wrapper shape `GitWorktreeAdd` uses.

**Daemon — view-model action hint plumbing**
- `ws-dashboard/crates/core/src/view_model.rs#L57-L69` — `WorkRootView`
  already carries `pub actions: Vec<ActionHint>` (currently populated only
  by `activation_actions`, below) — no schema change needed, just populate
  a `worktree.remove` hint.
- `ws-dashboard/crates/daemon/src/discovery.rs#L185-L235` (`push`) and
  `#L574-595` (`activation_actions`) — where `WorkRootView.actions` is
  built per row. `activation_actions(active, available)` doesn't currently
  see `discovered.kind`; either pass `discovered.kind` in or add the
  `worktree.remove` hint directly in `push` after the existing call, gated
  on `discovered.kind == WorkRootKind::GitLinkedWorktree`.

**Frontend — modal pattern to mirror (rich modal, not `window.confirm`)**
- `ws-dashboard/frontend/src/App.tsx#L1454-L1467` — the *existing*
  `workspace.remove` handler uses a bare `window.confirm(...)`. **Risk
  signal**: this is not a rich modal and must NOT be copied for B-1/B-2 —
  the ticket requires a real modal with a conditional red data-loss banner
  and a checkbox with a conditional red unmerged warning, which
  `window.confirm` cannot render. Build a new component instead.
- `ws-dashboard/frontend/src/App.tsx#L2735-L3121` — `GitWorktreeAddModal`:
  the complete pattern to mirror for a new `GitWorktreeRemoveModal` —
  target state (`{serverRoute, workRootId}` or similar) driving a fetch-on-
  open effect, `ModalOverlay`/`Modal`/`Dialog` from `react-aria-components`,
  `InlineNotice tone="error"` for red warnings (already used at
  `App.tsx#L3059-L3065` for a comparable red-banner case), a debounced
  preview-on-input-change effect, and command-wrapped open/close/submit
  (`onCommand(buildX..., { "cmd.id": handler })`) so command-id dispatch,
  recent-command evidence, and programmatic invocation stay consistent with
  every other dashboard control.

**Frontend — action-hint gating + row menu wiring**
- `ws-dashboard/frontend/src/App.tsx#L10184-L10186` — `hasWorkspaceRemove`
  derives from `actions.some(action.enabled && action.id ===
  "workspace.remove")` on the **workspace/compact-root** row only.
- `ws-dashboard/frontend/src/App.tsx#L10073-L10109` — child worktree rows
  (`presentation="workRoot"`, depth 1, one per
  `WorkRootKind::GitLinkedWorktree`) currently render `actions={[]}` and get
  **no** overflow menu at all — only the close-`X` button when open. This is
  the row that needs a new "..." menu (or reused affordance) exposing
  "Remove worktree..." (and, per B-3, "Hide worktree"), gated on the new
  `worktree.remove` action hint from the daemon.
- `ws-dashboard/frontend/src/App.tsx#L10313-L10371` — the existing
  `workspace-row-menu` overflow (rendered only on the workspace/compact-root
  row, i.e. the "root workRoot's right-side '...' settings menu" the ticket
  names) is where the B-3 "hidden worktrees" submenu belongs, alongside the
  existing "Add worktree..." / "Remove workspace..." items.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1012-L1041` —
  confirms `gitRow.locator('[data-command-id="workspace.menu.open"]')` /
  `.workspace-row-menu` is exactly this root-row menu, and that a created
  worktree becomes a `.resource-row[data-resource-presentation="workRoot"]`
  row — the concrete locator shape for both the new remove-modal test and
  the hidden-worktrees submenu test.

**Frontend — B-3 pure-UI hide/unhide: direct precedent to reuse**
- `ws-dashboard/frontend/src/workNavOrder.ts` (whole file) — this is
  effectively the exact pattern B-3 needs, already built and tested: a pure,
  versioned, `browserStorage()`-backed, scope-keyed id map
  (`worktreeOrderByWorkspace: Record<scopeKey, string[]>`) with a defensive
  malformed-data-tolerant parser (`loadWorkNavOrderSnapshot`) and a
  best-effort saver (`saveWorkNavOrderSnapshot`), applied at render time over
  server-supplied arrays without mutating the source
  (`applySiblingOrder`). A `hiddenWorktreesByWorkspace: Record<scopeKey,
  string[]>` map of the same shape, in a new module (or added to
  `workNavOrder.ts` itself, given the identical scope-keying via
  `serverScopedIdentity(serverId, workspace.id)`), is the natural
  implementation — filtered out of `childWorkRoots` at
  `App.tsx#L9991-L9994` before `applySiblingOrder` is applied, with the
  filtered-out ids surfaced in the new hidden-worktrees submenu for
  un-hiding.
- `ws-dashboard/frontend/src/hotkeys.ts#L729-L745` — **key finding**: the
  comment directly above the existing `gitWorktreeAdd.open` (`g w a`) /
  `workspace.remove` (`g w x`) / `workspace.menu.open` (`g w m`) bindings
  reads `// "g w h" hidden-worktrees is a GAP`. This is a pre-existing,
  already-reserved keybinding slot for exactly this feature — confirms this
  exact UX was anticipated and gives a concrete hotkey id/binding to fill
  as part of B-3 (open the workspace menu focused on / scrolled to the
  hidden-worktrees section, or a dedicated toggle), consistent with the
  `activeRootBinding` pattern used by the three neighboring bindings.

**Frontend — command id plumbing**
- `ws-dashboard/frontend/src/commands.ts#L1-100` — `DashboardCommandId` /
  `DashboardCommandPayload` union: every new interactive affordance
  (`worktreeRemove.open/close/submit`, `worktreeHidden.menu.open` or
  similar, `worktree.hide`, `worktree.unhide`) needs a new command id +
  payload variant here, plus a `buildXCommand` builder function following
  the existing `buildGitWorktreeAddOpenCommand` /
  `buildWorkspaceRemoveCommand` shape (`commands.ts#L218-L359`).

**Frontend — API client module to mirror**
- `ws-dashboard/frontend/src/gitWorktreeAdd.ts` (whole file, 170 lines) —
  the exact shape for a new `gitWorktreeRemove.ts`: typed request/response,
  a `*Base` route-builder using `localCompatibleDashboardApiRoute`, a
  typed submit-error class carrying the bounded server error/preview back
  to the modal, and `fetch...`/`preview...`/`submit...` functions. Route
  scoping (`server-local` vs `{serverRoute}`) should reuse
  `localCompatibleDashboardApiRoute` exactly as `gitWorktreeAddBase` does.

**Test conventions**
- `ws-dashboard/crates/daemon/tests/routes.rs#L7247-L7308` —
  `workspace_remove_route_forgets_workspace_without_deleting_files_or_paths`:
  the concrete integration-test shape (temp fixture dirs, `open_work_root_for_test`,
  asserting the removed id is gone from resources, the path/host-path never
  leaks into the response body, and `store.load_opened_work_roots()`
  reflects the change) to mirror for the new remove-route tests, plus
  `#L6554-6861` (`git_worktree_add_*` tests, using `skip_without_git(...)`
  to no-op when the test host lacks `git`) for preview/submit/blocked-input
  coverage shape.
- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L589-625` —
  `init_fixture_repo` + `changes_for_path_reports_modified_and_untracked...`:
  the in-module unit-test fixture pattern (real `git init` in a temp dir) to
  mirror for a new unit test of the `merge-base --is-ancestor`
  unmerged-branch check.
- `ws-dashboard/frontend/package.json` `scripts` — `test:git` already runs
  `gitToolbar.test.js` + `gitWorktreeAdd.test.js`; a new
  `gitWorktreeRemove.test.ts` should be added to that same script entry (or
  a new `test:worktree-remove` script following the same
  `tsc -p tsconfig.route-tests.json && node
  ./node_modules/.tmp/route-tests/<name>.test.js` shape). `test:resource-model`
  is the right place for a `workNavOrder`-adjacent hidden-worktree filter
  unit test if it lands in `workNavOrder.ts`.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L982-L1042` — the
  `git workspace overflow adds linked worktree` step is the direct
  precedent/anchor to extend with a new step exercising: open the created
  worktree's new row menu → "Remove worktree..." → modal shows baseline
  copy (clean worktree, no red banner) → cancel; then repeat with a dirty
  worktree (write an untracked file first) asserting the red data-loss
  banner renders; then a B-2 case with the checkbox checked against an
  unmerged branch asserting the red parenthetical warns and does not
  silently proceed as `-D`; then a B-3 case hiding a worktree (row
  disappears from the nav), opening the root row's "..." → hidden
  worktrees submenu, and un-hiding it (row reappears, nothing on disk/registry
  changed — no daemon call observed for the hide/unhide step itself).

## Implementation Plan

1. **Daemon: branch-unmerged non-mutating check.** Add a `branch_unmerged`
   (or similarly named) helper next to `git_toolbar.rs`'s `git_text`/
   `run_git`/`rev_counts` (or in the new remove-op module) implementing
   `git merge-base --is-ancestor <branch> <ref>`, where `<ref>` is the
   branch's `@{upstream}` if configured, else the primary root's current
   `HEAD`. Return `true` (unmerged/dangling) when the ancestor check exits
   non-zero for the chosen ref. Cover with a fixture-repo unit test mirroring
   `git_toolbar.rs#L589-625` (a branch with a unique unmerged commit vs. a
   fully-merged branch).

2. **Daemon: worktree-remove preview + submit handlers.** New module (or
   extend `git_worktree.rs`) with:
   - A resolver that, given a `work_root_id`, finds its owning workspace via
     `live_dashboard_resources`, confirms the target root's
     `kind == GitLinkedWorktree`, and resolves the *workspace's primary
     root path* the same way `resolve_workspace_git` does (reuse/extract
     shared logic rather than duplicating the primary-root search).
   - A preview (non-mutating) response bundling: dirty-state summary (reuse
     `changes_for_path` from `git_toolbar.rs`, run against the *target
     worktree's own path*) and branch-unmerged status (Step 1's helper, run
     with `-C <primary-root-path>`).
   - A submit handler: revalidate availability, run `git -C <primary-root>
     worktree remove [--force] <target-path>` — `--force` only when an
     explicit force flag is set by the caller after the owner has seen the
     dirty-state warning and still confirms (never force implicitly); on
     success, optionally run `git -C <primary-root> branch -d <branch>`
     only when the request's `deleteBranch` flag is true AND Step 1's check
     reports "safe" — if `deleteBranch` is true but the branch is unmerged,
     do **not** run `-D`; leave the branch and surface that outcome in the
     response instead (never silently force-delete). Then unregister the
     work root under `registry_persist_lock` with rollback-on-persist-
     failure (mirror `root_picker.rs#L323-L372`), call
     `state.terminals.remove_for_work_roots`,
     `state.codex_sessions.remove_for_work_roots`,
     `state.claude_sessions.remove_for_work_roots` for the one removed id,
     and return a fresh `live_dashboard_resources(...)`.

3. **Daemon: route registration.** Add unscoped routes (mirroring
   `router.rs#L290-L306`) and `{server_route}`-scoped mirrors (mirroring
   `router.rs#L157-L172` plus `servers.rs#L610-L643` handlers/operations,
   using `ForwardResponseRewrite::Resources`) for the new preview/submit
   endpoints, e.g. under `work-roots/{work_root_id}/git-worktree-remove/...`.

4. **Daemon: `worktree.remove` action hint.** In `discovery.rs`'s `push`
   (`#L185-235`), add a `worktree.remove` `ActionHint` (label "Remove
   worktree...", `enabled: available && active` or similar) gated on
   `discovered.kind == WorkRootKind::GitLinkedWorktree`, alongside the
   existing `activation_actions(...)` call — either extend
   `activation_actions` to take `kind` or push the extra hint directly in
   `push`.

5. **Frontend: `gitWorktreeRemove.ts` API client.** New module mirroring
   `gitWorktreeAdd.ts`: typed preview/submit request+response, a typed
   submit-error class, `fetchGitWorktreeRemovePreview`/
   `submitGitWorktreeRemove` using `localCompatibleDashboardApiRoute`.

6. **Frontend: command plumbing.** Add command ids + payload variants to
   `commands.ts` (`worktreeRemove.open` / `.close` / `.submit`, and for B-3
   `worktree.hide` / `worktree.unhide` / a hidden-worktrees-submenu open
   command) with matching `buildXCommand` builders, following
   `commands.ts#L218-359`'s shape.

7. **Frontend: `GitWorktreeRemoveModal` component.** New component mirroring
   `GitWorktreeAddModal` (`App.tsx#L2735-3121`): on open, fetch the preview
   (Step 2's endpoint) for the target `work_root_id`; render the baseline
   confirmation copy always; render an `InlineNotice tone="error"` (or
   equivalent red-styled block) data-loss banner only when the preview
   reports dirty state; render the "delete branch too" checkbox
   (`checked` state defaulting to `false`) with a red parenthetical warning
   rendered only when the preview reports the branch unmerged; submit posts
   `{ deleteBranch }` to the submit endpoint and, on success, feeds the
   returned resources back through the same `onCreated`/resource-merge path
   `GitWorktreeAddModal` uses.

8. **Frontend: row menu wiring for child worktree rows.** In the
   `ResourceRow` render for `presentation === "workRoot"` child rows
   (`App.tsx#L10073-10109`), add an overflow ("...") affordance analogous
   to the existing `workspace-row-menu` button/menu
   (`App.tsx#L10313-10371`), gated on the new `worktree.remove` action hint
   from Step 4, with a "Remove worktree..." item opening the Step 7 modal
   and a "Hide worktree" item invoking the Step 9 hide state update.

9. **Frontend: B-3 hidden-worktree browser-local state.** Extend
   `workNavOrder.ts` (or a new sibling module reusing its
   load/save/versioned-JSON pattern) with a `hiddenWorktreesByWorkspace:
   Record<scopeKey, string[]>` map, scoped identically to
   `worktreeOrderByWorkspace` (`serverScopedIdentity(serverId,
   workspace.id)`). Filter hidden ids out of `childWorkRoots` at
   `App.tsx#L9991-9994` before `applySiblingOrder`. Persist via the same
   `browserStorage()`-backed save-on-change effect pattern as
   `App.tsx#L1711-1716`.

10. **Frontend: "hidden worktrees" submenu.** In the root-row
    `workspace-row-menu` (`App.tsx#L10313-10371`), add a submenu/section
    (flat labeled list within the existing menu — no nested-flyout pattern
    exists elsewhere in this codebase, don't introduce one for this) listing
    currently-hidden worktree ids/labels for that workspace scope; clicking
    an entry removes it from the hidden set (un-hide), immediately restoring
    the row. Section renders only when the scope's hidden set is non-empty.

11. **Frontend: fill the reserved `"g w h"` hotkey gap.** In `hotkeys.ts`
    (`#L729-745`), add an `activeRootBinding` for `["g", "w", "h"]` wired to
    whatever command Step 6/10 exposes for revealing the hidden-worktrees
    section, following the same three-binding pattern already present.

12. **Wire up test scripts.** Add the new frontend unit test file(s) to
    `package.json`'s `test:git` (or a new `test:worktree-remove` script) per
    the existing `tsc -p tsconfig.route-tests.json && node ...` convention.

## Verification Plan

- `cd ws-dashboard && cargo test -p ws-dashboard-daemon` — full daemon
  suite; at minimum the new `merge-base --is-ancestor` unit test (co-located
  with the new module, mirroring `git_toolbar.rs`'s fixture-repo tests) and
  new integration tests in `ws-dashboard/crates/daemon/tests/routes.rs`
  covering: clean-worktree remove (registry + `git worktree list` both
  reflect removal), dirty-worktree remove without force is blocked, force
  remove of a dirty worktree succeeds, `deleteBranch: true` on a merged
  branch deletes it, `deleteBranch: true` on an unmerged branch does **not**
  delete it (never silently `-D`), and terminal/codex/claude session
  accounting is cleared for the removed work root id (mirror
  `workspace_remove_route_forgets_workspace_without_deleting_files_or_paths`,
  `routes.rs#L7247-7308`).
- `cd ws-dashboard/frontend && npm run test:git` (extended with the new
  `gitWorktreeRemove.test.ts`, or a new equivalent `npm run` script) — unit
  coverage for the new API client and modal-state logic.
- `cd ws-dashboard/frontend && npm run test:resource-model` — covers any
  `workNavOrder.ts`-adjacent hidden-worktree filter logic if it lands
  there.
- `cd ws-dashboard/frontend && npm run build` — `tsc -b && vite build`,
  confirms the new command/type plumbing type-checks end to end.
- `cd ws-dashboard/frontend && npm run test:browser` — builds, builds the
  daemon, and runs Playwright, including the extended
  `dashboard-acceptance.spec.ts` step(s) covering: B-1's baseline vs.
  red-data-loss-banner modal states, B-2's checkbox default-OFF + red
  unmerged warning, and B-3's hide → hidden-worktrees submenu → unhide
  round trip (gated the same way the existing worktree-add step is gated on
  `gitWorkRoot` being configured on the test host).

## Escalations

- None.

---
title: "Dashboard rail: delete a linked worktree with a preview-and-confirm modal"
---

# Dashboard rail: delete a linked worktree with a preview-and-confirm modal

## Background

Owner request (2026-11-13, dashboard UX side track; whiteboard topic
`wt-delete`, items (123)-(126)). The session rail lists worktree rows (for
example `분리된 HEAD · single-exe-pi-base`, a detached-HEAD worktree at
`D:/Temp/single-exe-pi-base` outside the ws pool). The user wants to delete
such a worktree from the rail. It is treated as an extremely dangerous
operation.

Research (2026-11-13):

- The archived Rust dashboard defined a worktree deletion policy: spec at
  `git show 01333c9e9:ai-docs/spec/ws-web-dashboard/index.md`, plan in commit
  `4a5b5c84d`, dropped follow-up `260525-feat-ws-dashboard-workroot-polishing-backlog`.
  Summary: linked worktrees only; a preview -> submit two-step with
  re-validation at submit; a real modal (never `window.confirm`); a dirty
  worktree could still be deleted through an explicit force flag; branch
  deletion was an opt-in checkbox (`git branch -d` only); on success the
  worktree's sessions were cleaned up.
- The current `worktree.remove` MCP tool (`internal/mcp/worktree_remove.go`
  `removeWorktree`, `worktree_tools.go` `ownedPoolWorktree`) refuses the
  primary worktree, any worktree outside the ws pool, dirty trees, unreached
  commits, and live leases, and keeps the branch. Because it refuses foreign
  worktrees unconditionally, the dashboard needs its own route.
- Current rail: `RailWorktree` (`pageview_rail.go`), `railworktrees.go`
  (`git worktree list --porcelain`); no delete action and no pool/foreign
  distinction. The row's change count is a stale snapshot, so deletion must
  re-check. `/s/<rid>/worktrees` GET is read-only; `worktrees/cleanup` only
  messages the session. Route template: `handleWorktreeAdd` in
  `workspace_routes.go` (`workspaceBody`, `railRepository`, `beginGitAction`,
  `d.rail.reread`, `broker.wake`; `workspaceBody` checks `sameOrigin` only, not
  `localViewer`: `agents-plugin-tool/internal/wsdashboard/workspace_routes.go#L62-L81`,
  so decision 5's local-viewer gate is new code in the delete route). Existing confirm UI is `window.confirm`;
  `railPopOpen` (`dashboard.js`) is the popover pattern.

## Decisions

1. **Scope: every linked worktree except the primary** ((123)=a), including
   worktrees outside the ws pool such as `single-exe-pi-base`.
   Rejected: ws pool worktrees only - the owner's example is a foreign
   worktree.
2. **The branch is always kept** ((124)=a); only the worktree is removed. A
   detached HEAD has no branch to keep.
   Rejected: the old spec's opt-in "also delete branch" checkbox.
3. **Deletion conditions follow the archived Rust dashboard policy as is**
   ((125), owner: "just the old dashboard behaviour"): the worker reads the old
   spec above and implements its deletion conditions, force flag, re-validation
   and session cleanup, except where decision 2 overrides branch handling.
   Rejected: the stricter clean-only proposal with no force path (lead's
   (125) draft).
4. **Confirmation UX: a small modal cycle** ((126), owner: "ok, a small modal
   cycle instead of the popover"): the rail row's delete action opens a modal
   that shows a fresh server-side check result (dirty files, unreached
   commits, sessions in the worktree, lock state), requires typing the
   worktree folder name to enable the delete button, and the server
   re-validates at submit.
   Rejected: a popover; `window.confirm`.
5. **Access**: same-machine viewer and same-origin only (`localViewer` +
   `sameOrigin`, the terminal card's rule); a LAN token alone cannot delete.
6. **Execution**: `git worktree remove` run from the primary worktree's root;
   afterwards the rail re-reads (`d.rail.reread`, `broker.wake`) like the add
   route.
7. **Compatibility tier: T0** - a new dashboard route and asset content; no
   MCP tool or store change.
8. **A ws pool worktree with a live lease is refused, even under the old
   policy's force flag** ((127), technical, acknowledged): the archived
   dashboard predates the worktree pool, so its policy has no lease rule, and
   deleting a leased worktree breaks a running worker.
   Rejected: letting force override a live lease.

## Constraints

- Reuse the existing check logic from `worktree_remove.go` where it fits;
  do not loosen `worktree.remove` itself (it stays pool-only).
- Daemon code under `agents-plugin-tool/internal/` follows
  `ai-docs/manuals/release-compatibility.md`.
- Never run deletion tests against real user worktrees; use temp repos under
  the test's temp dir.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- f0c07b19 (2026-11-13, commit): "Settled on the board: (123)=a all linked worktrees incl. foreign, (124)=a branch always kept, (125) owner chose the archived Rust dashboard's deletion policy as is over the lead's stricter clean-only/no-force draft" — bearing: supports
- 6c5fe1f2 (2026-11-10, commit): "The worktree routes act only on a repository the rail shows (a stored one or a listed session's), so the routes cannot be pointed at an arbitrary folder." — bearing: constrains
- 6c5fe1f2 (2026-11-10, commit): "The preview is a POST because the same-origin gate needs an Origin header, which a GET does not carry." — bearing: constrains
- 6c5fe1f2 (2026-11-10, commit): "Refusals git should never see (option-like or invalid branch names, a taken or checked-out branch, a non-empty folder) are checked by the daemon first" — bearing: constrains
- eaa6a798 (2026-10-20, commit): "worktree.remove lacked the primary/non-pool guards release already has (worktree_tools.go:425-429) and could delete a live worker's clean, commit-less worktree" — bearing: supports
- 261020-feat-worktree-profiles (2026-10-20, Decisions): "### `worktree.remove` and the nudge" (worktree.remove is pool-only; this ticket leaves it unchanged) — bearing: constrains
- d1021d42 (2026-05-24, commit): "Preview state is keyed by the current modal request and submit is disabled unless the displayed preview matches that request." — bearing: supports
- 1ab62ba2 (2026-05-24, commit): "Inputs remain editable during submit, so blocked submit previews now check the current request key before mutating modal preview/error state." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/workspace_routes.go (route registration), a new delete route file, pageview_rail.go (RailWorktree), assets/dashboard.js, dashboard.css, dashboard.tmpl, i18n tables, tests |
| scope.surface | cross-module | new daemon POST route plus rail row action; reusing mcp/worktree_remove.go checks crosses packages, and internal/mcp imports internal/wsdashboard (mcp/dashboard_config.go), so wsdashboard cannot import mcp |
| scope.new_public_symbol | unknown | route constant and handlers are unexported like worktreeAddRoute; an extraction of the check logic into a shared package (for example wsgit) is unsettled and would export symbols |
| scope.new_type_contract | yes | preview and submit request and answer JSON, a RailWorktree field for delete eligibility |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/workspace_routes_test.go, rail_test.go, railscript_test.go; mcp/worktree_remove_test.go for the reused checks; new test files possible |
| complexity.reuse_points | unconfirmed | checks live in unexported mcp functions (removeWorktree, countUnreachedCommits, porcelainPaths, readWorktreeLease) that wsdashboard cannot import |
| complexity.side_effect_risk | high | runs git worktree remove on arbitrary linked worktrees including ones outside the ws pool, deleting the directory and any uncommitted files |
| risk.correctness | high | force semantics, locked and submodule worktrees, unreached detached-HEAD commits, live sessions and leases, and submit-time re-validation must all hold on a destructive path |
| risk.fit | moderate | follows the add-worktree route pattern but the dashboard has no typed-confirm modal yet (confirm UI is window.confirm) |
| risk.test | high | needs temp-repo matrix for dirty, unreached, lease, lock, submodule and primary cases plus a LOOK capture of the modal |
| risk.security_or_contract | high | destructive route; workspaceBody enforces sameOrigin only, so localViewer must be added; the target path must be validated against git worktree list of a rail-known repository |

## Phases

### Phase 1: Rail worktree delete route and modal

- Daemon route (POST, guarded per decision 5): a preview call returning the
  fresh check result and a submit call that re-validates and runs the
  removal per decisions 3 and 6.
- Rail: a delete action on non-primary worktree rows; the modal per
  decision 4.

Verification: Go tests over temp repos - preview reports dirty / unreached /
session / lock state; submit refuses the primary worktree and anything the
old policy refuses without force; force behaves as the old policy defines;
branch survives; access guard refuses a non-local viewer. A LOOK capture of
the modal with a dirty and a clean worktree.

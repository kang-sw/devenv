---
title: "Dashboard rail: delete a linked worktree with a preview-and-confirm modal"
sage-review-design: required
sage-review-completeness: required
related:
  261020-feat-worktree-profiles: "worktree.remove stays pool-only; profile pre_remove hooks do not apply: pooled worktrees are refused (decision 12, (519))"
  260525-feat-ws-dashboard-workroot-polishing-backlog: "dropped follow-up of the archived Rust deletion policy"
  261113-feat-ws-dashboard-iframe-shell: "moves the rail, and this ticket's row action and modal, into the shell document"
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
   worktrees outside the ws pool such as `single-exe-pi-base`; pooled
   worktrees are refused (decision 12).
   Rejected: ws pool worktrees only - the owner's example is a foreign
   worktree.
2. **The branch is always kept** ((124)=a); only the worktree is removed. A
   detached HEAD has no branch to keep.
   Rejected: the old spec's opt-in "also delete branch" checkbox.
3. **Deletion conditions follow the archived Rust dashboard policy as is**
   ((125), owner: "just the old dashboard behaviour"): the worker reads the old
   spec above and implements its deletion conditions, force flag and
   re-validation, except where decision 2 overrides branch handling and
   decisions 9-10 fill its gaps (decision 10 replaces its session cleanup).
   Rejected: the stricter clean-only proposal with no force path (lead's
   (125) draft).
4. **Confirmation UX: a small modal cycle** ((126), owner: "ok, a small modal
   cycle instead of the popover"): the rail row's delete action opens a modal
   that shows a fresh server-side check result (dirty files, unreached
   commits, sessions in the worktree, lock state), requires typing the
   worktree folder name to enable the delete button, and the server
   re-validates at submit. Every delete has exactly one typed-name step; the
   submit carries `force` only when the preview showed a risk that needs it
   (dirty or untracked files, a detached HEAD's unreached commits,
   initialized submodules), and the modal says force will be used.
   Rejected: a popover; `window.confirm`.
5. **Access**: same-machine viewer and same-origin only (`localViewer` +
   `sameOrigin`, the terminal card's rule); a LAN token alone cannot delete.
6. **Execution**: `git worktree remove` run from the primary worktree's root;
   afterwards the rail re-reads (`d.rail.reread`, `broker.wake`) like the add
   route.
7. **Compatibility tier: T0** - a new dashboard route and asset content; no
   MCP tool or store change.
8. **(Superseded by decision 12: pooled worktrees are refused outright.)**
   **A ws pool worktree with a live lease is refused, even under the old
   policy's force flag** ((127), technical, acknowledged): the archived
   dashboard predates the worktree pool, so its policy has no lease rule, and
   deleting a leased worktree breaks a running worker.
   Rejected: letting force override a live lease.
9. **Gaps the archived policy does not cover** ((499)=a, owner 2026-11-13):
   commits a detached HEAD would lose (decision 2 keeps no branch for it) join
   dirty and untracked changes behind the same typed force confirmation, and
   the modal shows their count (only a detached HEAD is gated this way: a
   branch worktree's commits stay on the kept branch, so they are neither
   refused nor gated); a locked worktree is refused (unlock it
   first); a worktree with initialized submodules gets `--force` only when the
   user forced. Rejected: (b) refusing unreached commits even with force;
   (c) showing them with no gate.
10. **Live sessions in the worktree block deletion** ((500)=a, owner
    2026-11-13): the archived policy's "session cleanup" has no counterpart
    today, so instead deletion is refused while any live ws session, hosted
    lead, or dashboard terminal has its cwd inside the worktree; the modal
    lists them, and ended sessions are listed only. A terminal is judged by
    the cwd it was spawned with (its current cwd is not tracked). Path
    containment compares cleaned absolute paths, case-insensitively on
    Windows. Rejected: (b) stopping
    and closing them from the modal; (c) listing only and deleting anyway.
11. **Shared check helpers and path validation** ((505), technical,
    owner-acknowledged 2026-11-13; narrowed by decision 12): the
    `worktree.remove` checks the dashboard still needs (unreached commits,
    porcelain status: `countUnreachedCommits`, `porcelainPaths`) move from
    package `mcp` into a shared internal package, because `mcp` imports
    `wsdashboard` and not the reverse; the delete route also verifies the
    target path is in that repository's `git worktree list`. The lease check
    and the `worktree.nudge_stale_days` window stay in `mcp`: decision 12
    refuses every pooled worktree before any lease could matter, and a
    foreign worktree has no ws lease. Rejected: duplicating the checks in
    `wsdashboard` (two copies drift).
12. **Pooled worktrees are refused** ((519), owner 2026-11-13: dashboard
    deletion never touches ws pool worktrees; those stay with
    `worktree.remove`). A target is pooled when its cleaned absolute path is
    under the repository's resolved `worktree_pool` root or under the default
    pool root (`<git root>/../.ws-worktrees/<git root dir name>`), so a
    changed config never exposes the old pool; the route resolves the knob
    per repository without a session key. The rail row carries a deletable
    flag and shows no delete action on pooled rows (the rail hides pooled
    rows except while one holds a listed session, `pageview_rail.go`); the
    preview and the submit both refuse a pooled target, and the modal says the
    worktree belongs to the ws pool and is removed with `worktree.remove`.
    Profile `pre_remove` hooks therefore never apply. Rejected: running the
    profile's `pre_remove` hooks from the dashboard (the lead's (519) premise,
    rejected by the owner).

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
- (519) (2026-11-13, owner on MAIN's board): "the worktrees the dashboard shows are not pooled, so they are outside the helper's reach" — the lead's pre_remove-hook premise was rejected; recorded as decision 12 — bearing: constrains
- 261020-feat-worktree-profiles (2026-10-20, Decisions): "### `worktree.remove` and the nudge" (worktree.remove is pool-only; this ticket leaves it unchanged) — bearing: constrains
- d1021d42 (2026-05-24, commit): "Preview state is keyed by the current modal request and submit is disabled unless the displayed preview matches that request." — bearing: supports
- 1ab62ba2 (2026-05-24, commit): "Inputs remain editable during submit, so blocked submit previews now check the current request key before mutating modal preview/error state." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/workspace_routes.go (route registration), a new delete route file, pageview_rail.go (RailWorktree), assets/dashboard.js, dashboard.css, dashboard.tmpl, i18n tables, tests |
| scope.surface | cross-module | new daemon POST route plus rail row action; reusing mcp/worktree_remove.go checks crosses packages, and internal/mcp imports internal/wsdashboard (mcp/dashboard_config.go), so wsdashboard cannot import mcp |
| scope.new_public_symbol | yes | decision 11 extracts the worktree.remove checks into a shared internal package, exporting its helpers; route constant and handlers stay unexported like worktreeAddRoute |
| scope.new_type_contract | yes | preview and submit request and answer JSON, a RailWorktree deletable flag (false for the primary and pooled rows, decision 12) |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/workspace_routes_test.go, rail_test.go, railscript_test.go; mcp/worktree_remove_test.go for the reused checks; new test files possible |
| complexity.reuse_points | confirmed | decision 11: countUnreachedCommits and porcelainPaths move from package mcp into a shared package both callers import; the lease check stays in mcp (decision 12) |
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
- Rail: a delete action on non-primary, non-pooled worktree rows (decision
  12); the modal per decision 4.
- Shared check helpers per decision 11.
- The preview also reports initialized submodules (decision 9 needs force for
  them).

Verification: Go tests over temp repos - preview reports dirty / unreached /
session / lock / submodule state; submit refuses the primary worktree, a path
not in `git worktree list`, a locked worktree, a worktree with a live session,
a pooled worktree (under the resolved or the default pool root, decision
12; the preview refuses it too and the rail row has no delete action), and anything the old policy refuses without
force; a detached HEAD's unreached commits need force while a branch
worktree with commits no other branch reaches deletes without force and the
branch survives; a worktree with initialized submodules is removed only with
force; submit re-validates (state changed between preview and submit, for
example a file dirtied, is refused or needs force again); force behaves as
the old policy defines; access guard refuses a non-local viewer. A node test
that the delete button stays disabled until the typed name matches. A LOOK
capture of the modal with a dirty and a clean worktree.

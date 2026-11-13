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
  `d.rail.reread`, `broker.wake`). Existing confirm UI is `window.confirm`;
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

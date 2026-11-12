# Dashboard rail: drag to reorder sessions and workspaces

## Goal

User request 2026-11-12 (board topic rail-order, screenshot of the rail's
`develop · 루트` worktree group with three session rows): drag rows in the
dashboard's session rail to reorder them.

Today: the rail region (`r-rail` / `rsessions` / `rrow` in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`, built by
`pageview_rail.go`) lists workspaces (`section.rws`, sorted by path,
`pageview_rail.go` ~311), each repo's worktrees (`.rwt`, root first), and
each group's sessions (`.rs`, live by last exchange, ended in a
`details.rended` fold). Starred rows are pinned to the top of their group
client-side (`WsRail.pin` in `agents-plugin-tool/internal/wswhiteboard/assets/rail.js`,
applied from `dashboard.js` `railMarks` ~4988-5010); stars and workspace
collapse state live in this browser through `railStore` (load/save).

## Decisions (settled)

- (408)=a The custom order is stored in this browser only, next to the
  stars and collapse state (`railStore`), with the same try/catch posture
  (a storage failure falls back to the server order). No daemon route, no
  store shape change.
- (409)=b Draggable: live session rows within their own group (a session
  belongs to its worktree / folder workspace; it cannot be dropped into
  another group), and workspace sections (`.rws`) among themselves.
  Worktrees inside a repo keep their server order.
- (410) A session not yet in the stored order (new since the last drag)
  appears at the top of its group. Stars still win: the custom order
  applies within the starred rows and within the unstarred rows of a group
  separately. Rows in the ended fold are not draggable and keep their
  order. Workspaces not in the stored order go after the ordered
  ones, in server order (adding a workspace is an explicit user action,
  and the end is predictable). Stale ids in the stored order are ignored and pruned on
  the next save.
- The order must survive every rail region swap (the server re-renders
  the rail region; reapply the order after each swap the way stars are
  reapplied) and must not fight the existing row animations
  (`rail.js` move/marker animations) or the click-to-open / star / new
  / forget buttons: a press that never moves past a small threshold stays
  a click. Pointer-based drag (mouse and touch), with a visible drop
  indicator in quiet `--wb-*` tone, light and dark; keyboard reorder is
  not required. Do not let a drag start from the buttons inside a row.
- Dashboard asset change only (rail.js, dashboard.js, dashboard.css as
  needed): compatibility tier T0. Keep `rail.js` free of dashboard
  globals as it is today (it is shared with the whiteboard page).

## Verification

- Extend the Go-driven node tests for the rail (search `*_test.go` for
  `WsRail`, `pin(`, `railStore`, `rail.js`): applying a stored order
  within a group, new sessions on top, stars first with order inside each
  part, ended rows untouched, workspace order with unknown ones at the
  end, stale ids pruned, cross-group drop refused, storage throwing falls
  back to server order.
- Headless check with the real CSS/JS (playwright-core under the
  scratchpad's `pw/node_modules`; chromium
  `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`; scratchpad
  /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  drag a session within a group and a workspace section, mid-drag
  indicator, order kept after a simulated region swap, light and dark;
  shots into scratchpad/rail-drag-order/.

## Constraints

- A parallel branch (delegate/rail-hotkey-frame) edits dashboard.js's
  Ctrl+` handler / frame message handler and the session label class;
  stay out of those spots.
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wsdashboard/... ./internal/wswhiteboard/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/rail-drag-order` in this worktree; do not
  merge or push.

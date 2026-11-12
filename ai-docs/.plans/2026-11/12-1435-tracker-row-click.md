# Tracker list rows: each part opens its own picker; only the title opens the item

## Goal

User feedback 2026-11-12 (board topic tracker-row-click, items (403)-(406)
confirmed): a task row in the tracker list should behave like the new-task
row, whose chips (status, assignee, priority, due, labels) each open their
own picker. Today (`agents-plugin-tool/internal/wsdashboard/assets/tracker.js`)
the row (`irow`, ~970) opens the status menu from the status icon
(`stBtn`, act `mstatus`) and the item window (`showItem`, the click
handler's `[data-item]` fallback ~5776) from everything else: title,
assignee avatars (`whoHtml`), priority (`priHtml`, e.g. "Task"/"Urgent"),
due (`dueHtml`), and empty space.

## Decisions (settled)

- (403) Only the title (`a.title`) opens the item window. The status icon
  keeps the status menu; the assignee avatars, the priority text and the
  due text each open the matching existing item-menu step (`assign`,
  `pri`, `due` in `STEPS`, through `itemMenu(id, anchor, null, {step})`),
  the same pickers and save paths the new-task row and the item menu use.
  An element the viewer may not change (the same permission checks the
  item menu applies to those steps) stays inert: tooltip only, no pointer
  cursor, click does nothing.
- (404) A click on the row's empty space no longer opens the item window;
  it only focuses the row. Enter on a focused row still opens it (keyboard
  path). Unchanged: selection-mode row clicks (`mselClick`), the touch
  first-tap select, drag move, the "⋯" menu, the fold controls.
- (405) When the assignee or due slot is empty and the viewer may set it,
  hovering the row shows a dim icon in that slot (person / calendar, the
  icons the new-row chips use, the dim tone of a default `.nch.dflt`
  chip); hidden otherwise so the list stays quiet. Clicking it opens that
  step. Topics have no priority/due (as today).
- (406) Label chips on the row keep their current click (label search,
  `lsearch`). Added: hovering a row the viewer may edit shows a dim label
  (tag) icon; clicking it opens the label edit popup for that item (the
  `labels` step/picker the property card uses since 68126c07f, which
  stays open across toggles). Same visibility rule as (405).
- Applies wherever `irow` draws (list, tree, closed section). Keep the row
  height and layout stable (no shift on hover). Quiet `--wb-*` tone, light
  and dark. Keep `tracker.js` host-neutral.
- Dashboard asset change only: compatibility tier T0.

## Verification

- Extend the node-driven tests (`tracker_quickadd_test.go` harness or the
  row tests near it): title click opens the item window; empty-space click
  does not; Enter does; avatar/priority/due click opens the matching step;
  a no-permission element is inert; hover icons present only when
  editable and empty; label icon opens the labels step; label chip still
  searches.
- Headless check with the real CSS/JS (playwright-core under
  `<scratchpad>/pw/node_modules`; chromium
  `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  a hovered row with empty assignee/due and the label icon, and each
  picker opened from a row, light and dark, into scratchpad/tracker-row-click/.

## Constraints

- A parallel branch (delegate/tracker-assigner-confirm-below) edits the
  assign picker's completion radio in tracker.js (~3942-4090); do not
  restructure that area.
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wsdashboard/... ./internal/wsi18n/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/tracker-row-click` in this worktree; do not
  merge or push.

---
title: "Whiteboard one-line view survives a toolbar state-filter switch"
---

# Whiteboard one-line view survives a toolbar state-filter switch

## Background

Owner request (relayed by MAIN, 2026-11-15, board item (881), corrected
from an initial topic-click reading): on the whiteboard, while the board
toolbar's one-line view (Overview; the leftmost collapse toggle
`.board-overview`, `aria-pressed="true"`) is active, switching the toolbar's
right-side state-filter category (All / Confirm / Discuss / Implement /
Wait / Closed, from the TASK5 state filter merged as `40f96ea17`) exits
one-line view. It must instead keep one-line view on and show the
re-filtered list. Topic clicks inside one-line view keep exiting and
expanding exactly as today.

Cause: `cards.js` `selectFilter` hands its apply step to the page hook
`opt.toolbar.filter(run)` ("Let Overview finish restoring its anchor before
visibility changes"), and `dashboard.js` implements that hook as
`afterOverview(run)`, i.e. `WsOverview.after(run)`, which calls
`exit(null, run)` while Overview is on
(`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js` toolbar
`filter`, `agents-plugin-tool/internal/wsdashboard/assets/overview.js`
`after`). Overview already filters itself by visibility: `items()` and
`snapshot()` skip `[data-state-filtered]` nodes, so arrows rove only
eligible topics (`agents-plugin-tool/internal/wsdashboard/testdata/board-state-filter-browser.cjs`
Overview traversal checks).

## Decisions

- **(881) = d, owner's words:** "오른쪽의 이 토픽 카테고리 전환에서만
  살아남고 토픽 선택은 지금대로 펼치기입니다" (only the right-side topic
  category switch survives; topic selection stays expand-as-today).
  Rejected: a/b/c, which changed the topic click inside one-line view.
- **Only the filter hook changes path.** Topic/card clicks, rail and
  empty-space clicks, palette picks (`WsOverview.pick`), Enter, Escape,
  Ctrl+` and the toolbar toggle keep their current exit behavior.
- **`cards.js` contract unchanged.** `selectFilter` keeps deferring its
  apply step through `opt.toolbar.filter(run)`
  (`board_state_filter_test.go` "filter waits for overview restoration
  callback" stays valid). The page hook decides: when Overview is neither on
  nor animating, `run()` at once (as today); while Overview is on and
  settled, apply `run()` in place and re-settle Overview without exiting;
  while Overview is animating (`on` or not, `busy` true), defer the apply
  until the animation finishes and then follow the same two cases (`after`
  is not usable as that barrier: while entering it would exit). Overview
  exposes the deferral and re-settle step as a new `WsOverview` method (for
  example `filter(run)`); the exact name is the implementer's.
- **Re-settle rules (technical, implementer may refine within them):**
  the scroll stays anchored on the topmost visible row that remains
  eligible, through the existing single scroll writer (`keep`), so the list
  does not jump; if the roved item became filtered, the roving `tabindex`
  moves to the first eligible item at or below the old position (else the
  last eligible item), but keyboard focus is not stolen from the filter
  control the user is operating (`selectFilter` focuses the radio or
  trigger after `run`). Newly eligible items join the roving set. Exit never
  lands on a filtered node: when the stored entry topic is filtered at exit
  time, the landing falls back to the first eligible item at the top of the
  one-line list, or the toolbar when none is eligible. Session restore
  (`ws.dashboard.overview.<rid>`) keeps working.
  Rejected: exiting and re-entering Overview around the filter (visible
  double animation, loses the user's place); applying the filter without
  re-settling (roving focus and exit landing can sit on hidden nodes).

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (dashboard assets
  ship downstream).
- No new UI strings, so no `wsi18n` change.
- Not a cross-version surface (dashboard asset behavior only).
- Owner rules for the run: offline env `GOPROXY=off GOSUMDB=off
  GOTOOLCHAIN=local`; never the full `./internal/wsdashboard` package, only
  `-run`; never touch the shared dashboard daemon or port 29417 (isolated
  daemons with their own `WS_CACHE_HOME` on free ports only); end with an
  owned process scan and signal only processes the run started.

## Phases

### Phase 1: Keep Overview on through a state-filter switch

Change the toolbar `filter` hook in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js` and add the
Overview method in
`agents-plugin-tool/internal/wsdashboard/assets/overview.js`, per the
decisions above.

Verification:
- Extend the Overview JS tests in
  `agents-plugin-tool/internal/wsdashboard/overview_test.go` (or the
  nearest existing Overview node test): with Overview on, a filter switch
  keeps `data-overview` and `aria-pressed="true"`; a roved item that became
  filtered hands the roving `tabindex="0"` to an eligible item; exit after
  filtering out the entry topic lands on an eligible item, never a
  `[data-state-filtered]` node; with Overview off the apply runs at once;
  while Overview is animating (entering and exiting) the apply waits for
  the animation and does not exit an entering Overview.
- Update `agents-plugin-tool/internal/wsdashboard/testdata/board-state-filter-browser.cjs`
  (isolated Chromium fixture) where it asserts the old exit, and add a
  check: switch the filter with one-line view on, it stays on, rows
  re-filter, and arrow traversal covers only eligible topics.
- `go test ./internal/wsdashboard -run 'TestOverview|TestBoardStateFilter|TestBoardToolbar|TestCards'`
  from `agents-plugin-tool/` with the offline env; never the full package.

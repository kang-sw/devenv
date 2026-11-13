---
title: "Dashboard: the board's .oq-wrap.top pill wrapper inherits the header's .top margin"
---

# Dashboard: the board's .oq-wrap.top pill wrapper inherits the header's .top margin

## Background

Found by the empty-board-render fix (merged aa3c2634f, 2026-11-14).

- The whiteboard board's sticky pill wrapper is `.oq-wrap.top`
  (`wswhiteboard/assets/page.css:667-668`; set by `rail.js:246`).
- The dashboard's page header uses the bare class `.top`
  (`wsdashboard/assets/dashboard.css:66`:
  `.top { gap: 10px; margin-bottom: 10px; }`).
- On the dashboard's session page both stylesheets apply. The pill wrapper,
  meant to be `height: 0`, picks up `margin-bottom: 10px`, a 10px offset at
  the top of the board column (`.bcol`).
- No visible defect today. The empty-board fix sidesteps it by pinning the
  empty board to the column box instead of sizing by `height: 100%`. Any
  future layout that sums the column's children (a fill, a toolbar card at
  the top) meets the stray 10px again.

## Direction

- Scope the header rule to its element (e.g. `header.top` or a dedicated
  class), or rename the wrapper modifier (`.oq-wrap.is-top`), so the two
  generic `.top` uses stop meeting. Check the CSS collision guard test
  for why it did not flag this, and extend it if the guard is meant to.
- Verify the board column's metrics before and after: only the 10px
  offset should change.

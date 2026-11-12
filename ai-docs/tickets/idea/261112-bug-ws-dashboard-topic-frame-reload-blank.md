---
title: "Dashboard board: an open topic's body frame blanks while it reloads"
---

# Dashboard board: an open topic's body frame blanks while it reloads

## Background

Found while fixing the fold flicker (merge 8c441efda, delegate
`board-fold-flicker`). When a topic that stays open is updated, its body
frame reloads: blank for about two frames, a jump to an in-between height,
then one more blank frame. The capture before that fix shows the same, so it
is older than it. The dash-polish round had also listed "a changed topic's
frame blanks while it reloads" as unchecked.

Headless Chromium repro script and frame sheets from that round:
the lead's scratchpad `pw/repro.js` and `pw/out/SHEET-*.png` (session-local).

## Direction (unconfirmed)

Keep the old frame visible until the new one has loaded and reported its
height, then swap (double-buffer the body frame), in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js` and the
whiteboard topic frame sizing.

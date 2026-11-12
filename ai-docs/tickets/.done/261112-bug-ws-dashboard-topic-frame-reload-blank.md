---
title: "Dashboard board: an open topic's body frame blanks while it reloads"
completed: 2026-11-12
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

## Result

The board morph double-buffers a changed open topic's body frame
(`dashboard.js` `frameGraft`): when the topic's old section would stay where
it stands, that section is kept, its content becomes the new section's
except the path down to the old frame, and the new frame loads beside it,
unseen and out of the layout (`.doc-frame-next`, `dashboard.css`). Once the
new frame has loaded and two more frames have drawn, it replaces the old one
in the same frame and is asked for its height (easing to it in a moving
swap); one that never loads shows after 1.5 s. Headless captures of an
in-place update show no blank frame and no in-between height, where the old
build showed two blank frames, a shrink to an in-between height, and a third
blank frame.

A changed topic whose old section would move (an update that raises a topic
to the head from below it) still takes its new section and reloads as
before: moving a frame reloads it, and keeping the old section there would
move it or push the kept topics around it (`frameGrafts`, which replays the
card settle to tell). Covering that case would need an atomic DOM move
(`moveBefore`) in the card settle.

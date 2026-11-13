---
title: "Dashboard: every topic update reloads the topic's panel frames, restarting their scripts"
related:
  261113-feat-ws-whiteboard-board-frontend-performance: "also changes how board frames mount (lazy srcdoc, viewport mount/unmount); a fix here must agree with it"
---

# Dashboard: every topic update reloads the topic's panel frames, restarting their scripts

## Background

Found by the panel-float fix (`fix/ux-panel-float`, 010886611, 2026-11-14),
checked with nothing maximized.

- A board swap that updates a topic grafts the new section into the old one
  only along the path of the topic's **body** frame:
  `frameGraftable` picks the first `iframe.doc-frame` of the section, and
  `frameGraft` keeps only the elements on that frame's path
  (`wsdashboard/assets/dashboard.js`, ~806-880).
- Every other element of the old section is replaced by the new section's
  children. A whiteboard panel's `iframe.doc-frame` (inside
  `details.entry-panel`) is therefore a new node with the same `srcdoc`, so
  it loads again and any script in it starts over.
- Visible effect: an interactive mockup in a panel (a counter, an animation,
  a toggled state, a scroll position inside the frame) resets whenever the
  lead posts anything to that topic, even a one-line log entry that does not
  touch the panel.

## Direction

- Keep every panel frame whose document is unchanged across the update
  (same `data-doc-key` or the same `srcdoc`), not only the body frame: graft
  along each such frame's path, or carry the old frame node into the new
  section before it replaces the old one, as `moveBefore` would.
- A panel overwritten under the same id is a new entry and should load fresh.
- Verify headless: a panel with a counting script keeps counting across a
  non-panel entry on its topic, across another topic's update, and reloads
  only when its own panel is overwritten.

# Dashboard wide layout: tracker "dedicated site" view wraps below the session rail

## Bug (user report, 2026-11-12)
In the dashboard's three-column wide view, switching the tracker panel to its
dedicated site view (the mode whose header shows the site name, e.g.
"mylocaltest", with a "뒤로" back button, and lays out tracker / notifications /
chat side by side) breaks the row: the session rail stays alone in the first
row (top-left, with an empty area to its right) and the site's three panels
wrap into a second row below it, spanning the full width. Screenshot:
/home/swkang/.cache/ws@kang-sw-devenv/dashboard/sessions/camcordersenoritahandgunpatchwork/inbox/attachments/640098605fbb177f-1.png

## Expected
The site view stays in the same row as the session rail: the rail keeps its
column and the site view fills the remaining width at full height, the same
geometry the tracker panel had before the switch. Other layouts (split, tabs,
narrow/phone width) are unchanged, and switching back restores the previous
layout.

## Where to look
- agents-plugin-tool/internal/wsdashboard/assets/dashboard.css (the .dgrid.live
  layout rules around L620-680, data-layout variants) and tracker.css.
- dashboard.js / tracker.js where the site view is entered and what it changes
  on the grid (classes, data-layout, which element hosts it).
- Find the root cause (e.g. an element with no grid-area / a grid-column span
  that forces an implicit new row) rather than patching with a fixed size.

## Constraints
- Peer session shares /home/swkang/devenv; you work in this worktree only.
  Do not touch the topic-note code in dashboard.js (~L770-810) or the composer
  answer-block code (another delegate is changing it).
- Never run heavy go build/test locally; use
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- test ./internal/wsdashboard/...`.

## Verification
- Headless Chromium capture (playwright, as earlier dashboard rounds did) of
  the wide three-column view before and after entering the site view, plus the
  split layout and a phone-width view, saved under your scratch dir; report
  the paths.
- wsdashboard tests pass via ws-rgo.
- One commit on this branch including this plan; do not merge or push.

# Plan: post-swap CSS regressions (ticketless, (527)=a)

Branch `fix/ux-postswap-css-regressions` off develop. The UX lead delegates; EXEC merges.
Keep it surgical: a perf Phase 1 worker is editing the wswhiteboard/dashboard
assets in parallel, so touch only the rules named here and add no refactors.

## Must fix

1. **Topic-note image chips have no inset.** c69946c02 moved `.tnote` into
   `.tophead`, so `.topic > .nimgs` and `.topic.quiet > .nimgs`
   (agents-plugin-tool/internal/wsdashboard/assets/dashboard.css ~508-509) no
   longer match the chip row of a topic note. First confirm where `.nimgs` is
   inserted for a topic note (grep `nimgs` in dashboard.js/cards.js). Then
   retarget it so the chip row aligns with the note: `.tophead > .nimgs` and
   the quiet variant. The note's own inset is `margin: 6px 10px` and the quiet
   variant `4px 6px` (wswhiteboard/assets/page.css:152-154).
2. **The empty board is not full height when #bpins exists.** cards.js mount
   (~848-864) inserts an empty `#bpins.bpins` as the board's first child, so
   `.r-board:has(> .empty:only-child)` and `.r-board > .empty:only-child`
   (dashboard.css ~1488-1489) never match. Make the "No whiteboard yet" area
   fill the height again while an empty pin list is present. CSS-only is
   preferred, for example matching `.empty` when the only other child is an
   empty `.bpins` (`:has(> .bpins:empty)` or `:not(:has(> .topic))`). Do not
   change when or where #bpins is mounted; cards.js pins logic relies on it.
   If #bpins carries pseudo-content (data-label/data-drop), check that it
   still shows no strip on a topicless board.
3. **The stuck topic head is too tall at short heights.** The sticky
   `.tophead` (page.css ~697) now includes the `.tsub` recap (page.css ~407).
   At 700x640 the stuck head takes 208 of 387px of the scroller; at 420px
   wide, 270px. While stuck (`.tophead.wb-stuck`), clamp the recap: line-clamp
   `.tsub > .trecap` to 2 lines, or hide `.tsub` while stuck if the clamp
   alone still leaves the head over ~35% of a 640px-tall viewport. The
   unstuck head keeps the full recap.

## Cheap minors (do them only if each is a few lines; otherwise report and skip)

4. The pinned column starts ~10px above the board's first topic. Find the
   top padding/margin mismatch between `#bpins` in the pinned column and the
   board's first topic, and align them.
5. With every check answered and "hide answered" on, clicking the stuck check
   pill opens an empty 0px-tall popover (`.tophead > .checks`, page.css
   ~713). Do not open, or do not show, the popover when it has no visible
   check rows.

## Verification

- `go test ./internal/wsdashboard/... ./internal/wswhiteboard/...` from
  agents-plugin-tool passes, including the CSS collision guard that just
  landed on develop.
- Headless Chromium (cached Playwright at
  ~/.npm/_npx/705bc6b22212b352/node_modules/playwright) against the live
  daemon http://127.0.0.1:29417/. It runs develop, not this branch, so inject
  the branch's CSS (page.addStyleTag, or route the asset URL to the branch
  file) and measure:
  1. topic-note chip left edge == note left edge;
  2. the empty-board area height on a topicless session (e.g. the
     audacity…/regroup… session pages) fills the board;
  3. stuck-head height at 700x640 and 420x800.
- The live daemon is read-only: abort every non-GET request at the route
  level, and never send, submit, stop or close anything.

## Return boundary

Commit on the branch with `## AI Context` and stop. If a fix needs JS changes
beyond a guard for item 5, or touches files outside dashboard.css, page.css
and (for item 5 only) the one JS function that opens the checks popover,
stop and report instead.

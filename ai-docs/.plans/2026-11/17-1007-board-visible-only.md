# Dashboard board: render work only for visible topics (content-visibility)

## Context

Owner 2026-11-17 09:59 / 10:04: in the Electron dashboard the whiteboard
board panel stutters when scrolling with 44 topic cards and lags while typing
a note; the cost grows with the card count. Owner: visible-only processing is
required.

Trace analysis of the owner's DevTools traces (Trace-20261117T095746 scroll,
Trace-20261117T100328 typing; develop at 4ccb25cca):

- Scroll: 7.79 s main-thread busy of ~9.2 s. Style recalc 2.90 s (87
  whole-document recalcs of ~13k elements, avg 30 ms) and layout 2.84 s (all
  layouts rooted at `#document`, avg 22.7 ms) = ~71% of busy. ~1.4
  whole-document layouts per rAF frame; 657 dropped frames. DOM ~83-85k nodes,
  16-17.5k layout objects; `#bcol` is 10272 px tall vs a 1073 px viewport
  (~10-15% of topics on screen).
- Layout cost barely depends on the dirty set: 14 dirty objects of 17.5k
  still cost 26-36 ms. Typing pays ~2.3 such layouts per keystroke (~58 ms),
  from Blink edit/caret handling, not JS.
- Remaining forced reads (source at 4ccb25cca):
  - `wswhiteboard/assets/pill.js` ~207 `update` (rAF, `getBoundingClientRect`);
  - `wswhiteboard/assets/rail.js` ~147 (rect reads in `found.forEach`) and
    `draw` ~218 (`wb-stuck` class toggle; top scheduler of whole-doc
    recalcs, 19 recalcs / 623 ms);
  - `wsdashboard/assets/cards.js` `floatToolbar update` ~145;
  - `dashboard.js` `notePanes` ~315, `visible` ~214 (via `pinned`,
    `pinConvo`, `scrollState` in `swap`);
  - `dashboard-board.js` `boardViewedScroll` ~1350 (`scrollTop` per scroll
    event);
  - `dashboard-frames.js` `applyFrameHeights` ~356;
  - `dashboard-composer.js` `follow` ~1212 (scroll listener).
- Paint/raster are minor. No `content-visibility: auto` exists on the page
  today; overview mode already sets `content-visibility: visible !important`
  (`dashboard-board.css` ~120, ~138).

## Outcome

1. Off-screen topic bodies skip style, layout and paint:
   `content-visibility: auto` with `contain-intrinsic-size: auto <px>` on
   topic bodies only (`section.topic > :not(.tophead)` and the `.bcard`
   equivalent `:not(.bc-hd)`), in the stylesheets that style them
   (`dashboard-board.css`, `page-items.css`, `page-sticky.css` per the
   analysis; confirm). Headers stay out so sticky headers and the stuck
   shadow keep working. Pick the intrinsic size from typical folded/open
   heights.
2. Always fully rendered: overview mode (keep the existing overrides),
   `[data-max]` topics, the topic holding the focused note editor (`.tnote`
   focus), and a topic under an open popover.
3. Forced reads no longer defeat the skip:
   - pill/rail read the `section.topic` box, and item rects only for topics
     near the viewport (IntersectionObserver);
   - drop the per-scroll `scrollTop` read in `boardViewedScroll`; use the
     existing IntersectionObserver for `notePanes`/`visible`/`scrollState`;
   - fold pill, rail and `floatToolbar` rAF work into one read-then-write
     pass; stop `rail.js` `draw` from restyling the whole document with the
     `wb-stuck` toggle (scope the class to the element that needs it).
4. No behaviour change: anchor scroll / `scrollIntoView` to a topic or item,
   rail and pill positions, fold/open state, IME note drafts, stream
   auto-follow, frame cards (iframes still mount/unmount as today), and
   `stream.js` visibility checks (`scrollerOf`, `readyHeight`) all still work.

Not in scope: real DOM virtualization (unmounting topics). Only if
content-visibility proves insufficient after re-trace; report that instead.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `agents-plugin-tool/`.
- A parallel small change moves the topic state icon in the topic header
  (branch impl/develop/topic-state-icon-left); do not restructure the header
  markup.
- NO local `go test`, Go test exes, ws/ws-mcp executables, or launcher on this
  PC (owner AV rule). Do not start the ws daemon or Electron. Go tests on
  remote w5090 only:
  `D:/sys_lock.sh remote w5090 run <who> <min> --sync '<this worktree>' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wswhiteboard ./internal/wsdashboard'`.
  Accepted reds: only TestTopicStateBadgeResearchingMetadata and
  TestTopicStateBadgeClockAndReconciliation (need Node). Local Node/JS tests
  are fine.
- LOOK and measure: build a static fixture page with ~45 topics (reuse an
  existing test fixture/generator if one exists) in a local headless
  Chromium; capture before/after screenshots (top of board, scrolled middle
  with sticky header, overview mode, an anchor jump) and a before/after
  performance trace of a scripted scroll plus a few keystrokes in a note.
  Targets: style recalcs under ~2k elements per frame, layouts under ~5 ms,
  keystroke layout under ~10 ms. Report the numbers; if missed, say which
  reader still forces whole-board work.
- Temp only under `D:/Temp/board-visible-only/`, delete after (keep the
  screenshots and a short numbers file until the lead has seen the report).
- Commits with `## AI Context`. Do not push develop or main.

## Verification

- Before/after screenshots identical apart from intended changes.
- Before/after trace numbers on the fixture.
- Affected Go packages pass on w5090 (accepted reds above); JS tests pass.
- Report: files changed, commits, numbers, screenshots paths, anything left
  for a re-trace by the owner in Electron.

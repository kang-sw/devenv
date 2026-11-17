# Dashboard: remove forced layouts in pill, rail and shell post

## Context

Owner 2026-11-17: dashboard frontend is very slow (Electron and PWA). Trace
`Trace-20261117T091601` (scrolling a 27-topic board, ~81k DOM nodes) showed
the main thread spends ~3.4 s of 6.8 s busy in whole-document style recalc +
layout (40-50 ms per full pass), multiplied by read-after-write forced layouts
inside rAF/scroll handlers. Owner picked (1274)=a: fix the localized forced
layout sites now (items 2, 3, 5 below); DOM containment (content-visibility)
and the history/frame-height admission path are a separate ticket.

Measured sites (bundle line mapping is approximate for pill.js, +/-1):

2. `internal/wswhiteboard/assets/pill.js` `update` (~182, driven by
   `redrawSoon` rAF) and `show` (~130; `p.offsetWidth` in the capsule-grow
   block ~160-166), called twice (top and bottom) per update: ~0.77 s total,
   ~0.5 s forced layout. Per scroll/rAF it reads every item's rect, then
   writes (`classList.toggle`, `hidden`, `textContent`, `style.overflow`),
   then reads `offsetWidth` again. `later` (`pill.js:216`) re-arms a 120 ms
   timer on every scroll (54 times in the trace).
3. `internal/wswhiteboard/assets/rail.js` `run` (:133), `draw` (:177),
   `after` (:328, per-cap `getBoundingClientRect` after writes),
   `readInset` (:80, `getComputedStyle().getPropertyValue` on every run),
   `kick` (:422) rAF requests: ~0.34 s.
5. `internal/wsdashboard/assets/dashboard-lifecycle.js:708` `post`
   (MutationObserver on `.dwrap > .top` and root attributes): reads
   `getComputedStyle`, `offsetHeight`, `getClientRects` on modal/popover
   selectors after every board mutation: ~0.24 s.

## Outcome

1. pill: all layout reads for an update (item rects, both capsules' widths)
   happen in one read phase before any write; `update` skips writes when the
   visible set and labels did not change; the scroll path no longer re-arms a
   timer per scroll event (coalesce to one rAF or keep one pending timer).
2. rail: `readInset` value cached and refreshed only on resize (or when its
   inputs change); `run`/`draw`/`after` do all rect reads before the first
   write in a frame.
3. shell `post`: does no layout reads unless a popover/dialog/modal is
   actually open; coalesced to at most once per frame.
4. Behaviour unchanged: pill positions/labels, rail caps, popover placement
   look and act exactly as before.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing
  `agents-plugin-tool/`.
- View-only (T0); no API, store, or template contract change.
- No new CSS class names colliding across modules.
- NO local `go test`, Go test exes, ws/ws-mcp executables, or launcher on this
  PC. Node tests run locally. Go only on remote w5090:
  `D:/sys_lock.sh remote w5090 run <who> <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/guiding-dragonish-consumer' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wswhiteboard ./internal/wsdashboard'`.
  Accepted reds there: only TestTopicStateBadgeResearchingMetadata and
  TestTopicStateBadgeClockAndReconciliation (need Node).
- Temp only under `D:/Temp/dashboard-forced-layout/`, deleted after.
- Commits carry no Co-Authored-By trailer. Do not push develop or main.

## Verification

- Existing Node tests for pill/rail/lifecycle (testdata/*.cjs, *.test.cjs)
  pass; add a small test per site where a pure helper is extracted (e.g. the
  pill visible-set diff decides "no write").
- Go tests for wswhiteboard and wsdashboard on w5090.
- Evidence of the win: a Playwright/Chromium script under the temp dir that
  loads a static harness (or a page rendered by the Go test fixtures on
  w5090) with many topics, scrolls it, and counts forced layouts or long
  tasks before vs after via the Performance API / tracing; report the numbers.
- R1 review; fixes-only R2 for substantive findings.

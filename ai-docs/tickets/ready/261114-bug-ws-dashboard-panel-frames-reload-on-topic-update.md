---
title: "Dashboard: topic updates can reload panel frames, restarting their scripts"
related:
  261113-feat-ws-whiteboard-board-frontend-performance: "done; landed lazy placeholders, keyed entry-frame reuse and viewport mount/unmount; a fix here must agree with it (277615827; its Phase 1 and Phase 2 Results)"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 35f0cd6b31540316
sage-review-design-reviewed: 35f0cd6b31540316
---

# Dashboard: topic updates can reload panel frames, restarting their scripts

## Background

Found by the panel-float fix (`fix/ux-panel-float`, 010886611, 2026-11-14),
checked with nothing maximized.

- The current owner is
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js`:
  `frameGraftable` picks the first `.doc-frame:not(.doc-frame-next)` and
  matches an incoming `.doc-frame` (including a lazy placeholder), and
  `frameGraft` retains that frame's ancestor path; `morphBoard` separately
  preserves unchanged entry frames with `frameKeep` / `frameKept`
  (agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js#L425-L445,
  agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js#L564-L574,
  agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js#L617-L643).
- Since 277615827, a changed topic's mounted entry frames are paired with
  incoming placeholders using both equal `data-doc-key` and equal
  `data-srcdoc`, then moved through a connected hidden holder using
  `Element.moveBefore`; without that API they are not kept. Thus the
  historical claim that every panel frame is replaced on every topic update
  no longer describes all browsers
  (agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js#L497-L541;
  agents-plugin-tool/internal/wsdashboard/board_frame_reuse_test.go#L14-L95).
  The focused `TestBoardKeepsUnchangedEntryFrames` passed on 2026-11-15
  (`cd agents-plugin-tool && GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsdashboard -run '^TestBoardKeepsUnchangedEntryFrames$' -count=1`).
- Historical visible effect recorded by 010886611: panel scripts restarted
  on unrelated updates of their own topic. A present-day interactive
  counter/animation/toggle/scroll reproduction has not been run in this
  fact pass; current source explicitly retains the reload fallback without
  `moveBefore` and intentionally unmounts far-off frames
  (agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js#L509-L511;
  agents-plugin-tool/internal/wsdashboard/assets/dashboard-frames.js#L85-L100,
  agents-plugin-tool/internal/wsdashboard/assets/dashboard-frames.js#L136-L147).

## Prior Decisions

- 27761582 (2026-11-14, commit): "Decision 6 (keyed reuse): the key alone is not identity because image inlining and relay lists change an entry's document, so the pair key is data-doc-key plus data-srcdoc" — bearing: contradiction-candidate
- 261113-feat-ws-whiteboard-board-frontend-performance (2026-11-14, Result 232f25ded): "The keep scenario was narrowed to topics that stay mounted: accepted. Late heights from a far topic's update now arrive as a placeholder, which is the design." — bearing: constrains
- 01088661 (2026-11-14, commit): "Known, pre-existing, not changed: any topic update re-creates the topic's panel frames (frameGraft keeps only the body frame's path)" — bearing: supports
- 0c46ba10 (2026-11-15, commit): "New upstream direct source readers now use existing logical assemblers; original assertions, historical revision reads and fixture behavior are retained." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 1 adds one browser test and, only if it reproduces, edits dashboard-board.js; current morph owner is agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js, with existing coverage in agents-plugin-tool/internal/wsdashboard/board_frame_reuse_test.go. |
| scope.surface | internal | Direction concerns dashboard DOM/frame lifecycle; no proposed MCP, store or stream API change. |
| scope.new_public_symbol | no | None proposed; frameKeep / frameKept / frameGraft already exist in agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js. |
| scope.new_type_contract | no | No new type or signature proposed; current identity pair is data-doc-key plus data-srcdoc (agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js#L515-L524). |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/board_frame_reuse_test.go#L14-L95 passes; scripts/board-headless.mjs is the existing Chromium harness, but no counting-script scenario is specified here. |
| complexity.reuse_points | confirmed | Read morphBoard, frameKeep / frameKept and frameGraft in agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js#L322-L643, plus mount/unmount in agents-plugin-tool/internal/wsdashboard/assets/dashboard-frames.js#L14-L147. |
| complexity.side_effect_risk | high | Disconnecting or re-sourcing a mounted iframe discards its live document; reuse must occur between connected nodes before graft/settle detaches it (agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js#L502-L509). |
| risk.correctness | high | F1 keeps the key-and-data-srcdoc pair; overwrite must produce a fresh entry while unchanged documents, including a panel-only topic's first panel (dashboard-board.js#L564-L574), retain live script state. |
| risk.fit | moderate | Related performance work is done and already implements reuse; the remaining FIX scope must distinguish an uncovered regression from the explicit no-moveBefore fallback and viewport eviction. |
| risk.test | moderate | The passing Node DOM model asserts simulated reloads, not native script continuity; Phase 1 adds the native counting-script test for body+panel and panel-only topics; the no-moveBefore fallback is out of scope (F3). |
| risk.security_or_contract | moderate | Identity matching must not retain stale content across entry/document changes; preserve sandbox-before-connect behavior in agents-plugin-tool/internal/wsdashboard/assets/dashboard-frames.js#L33-L51. |

## Decisions

- (F1) Frame identity stays the landed pair: equal `data-doc-key` and
  equal `data-srcdoc` (277615827, Decision 6). Rejected: key-or-srcdoc
  matching from the original Direction - image inlining and relay lists
  change an entry's document under the same key, so either one alone
  would keep a stale document.
- (F2) A panel overwritten under the same id carries a new document and
  loads fresh; this follows from F1.
- (F3) Out of scope, intended behaviour: viewport unmount/remount of
  far-off topics (`dashboard-frames.js#L85-L147`, accepted in the
  frontend-performance Result 232f25ded) and the reload fallback in a
  browser without `Element.moveBefore`.
- (F4) The fix is reproduction-first: a native Chromium test with a
  counting script decides whether panel frames inside
  `details.entry-panel` still reload on an unrelated update of their own
  topic or of another topic. If they keep counting, the test lands as the
  regression guard and the ticket closes as fixed by 277615827. If they
  reset, panel frames are routed through the same keep path
  (`frameKeep` / `frameKept`, `dashboard-board.js#L497-L541`) as entry
  frames. A topic with no body frame is its own case: `frameGraftable`
  then selects the first panel (`dashboard-board.js#L564-L574`), which
  `frameKeep` excludes (`#L513-L523`) and `frameGraft` replaces even with
  identical bytes (`#L621-L632`); body double-buffering applies only to an
  actual topic-body frame, and an unchanged selected panel frame is kept.
  Rejected: changing code before the browser reproduction - the
  Node DOM model cannot show native script continuity.

## Phases

### Phase 1: Reproduce in Chromium and keep panel frames

Add a browser test on the disposable page fixture pattern
(`selection_note_browser_test.go`, `startPageFixture`, Playwright via
`WS_PLAYWRIGHT_MODULE`) with a topic carrying a body and a panel whose
`srcdoc` script increments a counter, and a second mounted topic with
panels only (no body frame). Assert for both topics, by reading the counter
inside the frame: (a) it keeps counting across a log entry added to its
own topic, (b) across an update of another topic, and (c) it restarts when
its own panel is overwritten. Keep the topic mounted (in view) throughout.
Closure as already fixed needs (a) and (b) to pass for both topics. If any
fails, fix `dashboard-board.js` so unchanged panel frames take
the keep path per F1, without breaking `TestBoardKeepsUnchangedEntryFrames`.
Verify: the new browser test, `go test ./internal/wsdashboard -run
'BoardKeeps|FrameReuse|BoardTopic'` (focused, never the whole
package), with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

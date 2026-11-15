---
title: "Green the known pre-existing dashboard and whiteboard test reds"
related:
  261115-feat-ws-whiteboard-viewed-panel-anchor: "introduced TestViewedPerformanceBrowser (4afc78add)"
  261113-feat-ws-dashboard-session-four-state-lifecycle: "Phase 2 Result records TestRootKeepsSessionsInTheRailOnly red after the landing logo (d2921cf02)"
completed: 2026-11-16
---

# Green the known pre-existing dashboard and whiteboard test reds

## Background

Whiteboard topics `ship-gate` (배포 검토 관문) and `ship-wrapup`. During the
v0.50.0 wrap-up, several tests were known red or flaky on develop and
carried forward as "pre-existing". Workers listed them as not-PASS and moved
on. One of them also failed in the pre-release CI run ("나머지 하나는 이
맥에서도 원래 실패하던 시험입니다", ship-gate). No ticket owns them, so every
worker re-diagnoses them, and a CI-gated release can block on them.

Known reds:

- `TestGearHoldsTheThemeSection` (`internal/wsdashboard/theme_test.go:144`).
  VERIFIED red on develop `e2725a2d6`: for `/s/livesession?frame=1` in
  several locales it reports missing `gear.theme*` text and missing
  `data-theme-set` items. Earlier tickets already listed it as failing
  (`261115-bug-ws-worktree-acquire-concurrent-pool-claim`,
  `261114-refactor-ws-dashboard-whiteboard-asset-split`).
- `TestRootKeepsSessionsInTheRailOnly` (`internal/wsdashboard/pages_test.go:414`).
  VERIFIED red on develop `e2725a2d6` with "root main panel is not empty".
  This follows from the landing logo and guide at `/` (`d2921cf02`, then the
  illustrated guide `bf96b4b13`). The test's expectation may simply be stale.
- `TestViewedPerformanceBrowser` (`internal/wswhiteboard/viewed_performance_test.go:14`).
  MAIN reports it red. It is opt-in and skips unless
  `WS_VIEWED_PERF_PLAYWRIGHT` and `WS_VIEWED_PERF_CHROMIUM` are set, so it
  was not reproduced here.
- `TestHistoryLoadAnchorBrowser` (`internal/wswhiteboard/history_load_anchor_browser_test.go:18`).
  MAIN reports it flaky. It skips unless `WS_PLAYWRIGHT_MODULE` is set, so it
  was not reproduced here.

## Decisions

- For each test, first decide whether the product or the expectation is
  wrong. Do not weaken an assertion just to make it pass. A test whose
  intent was superseded by an owner-approved change (such as the landing
  content at `/`) gets its expectation updated, citing that change.

## Phases

### Phase 1: Diagnose and green each known red

Acceptance:

- Each listed test passes on develop, or is fixed in product code, with the
  cause recorded per test in this ticket's Result.
- The two browser tests run with their opt-in environment against cached
  local Chromium. `TestHistoryLoadAnchorBrowser` passes several consecutive
  runs (for example `-count=5`), and its flake cause is named. No retry has
  been added around an assertion.
- The CI-red test from the v0.50.0 pre-release run is identified, and it
  is on this list or added to it.

### Result (cd477b5a8) - 2026-11-16

Triaged on develop `cd477b5a8` (macOS, cached Chromium 1223, `-run` only).

- `TestGearHoldsTheThemeSection`: PASS on `cd477b5a8`, with no change here.
  Stale expectation, fixed by `234cef037`: the theme controls moved to the
  shell-owned Settings menu (accepted in `0bc7fcf24`), and the test now checks
  every locale there.
- `TestRootKeepsSessionsInTheRailOnly`: PASS on `cd477b5a8`, with no change
  here. Stale expectation, fixed by `c918577da` ("update root tests for landing
  content in main"): the owner-approved landing content at `/` (`d2921cf02`,
  `bf96b4b13`) replaced the empty main panel.
- `TestViewedPerformanceBrowser`: red on `cd477b5a8`, with two causes.
  - **Product regression (fixed).** `20972a5f8` added the initial
    1000px `fill()` after `init()` applied its shared per-scroller capture.
    Each `fill()` -> `progress()` then opened its own `kit.keep()` capture,
    one per history with a gap. That undid the one-capture-per-scroller
    batching of `69c7e50fa`. On the 100-topic fixture, initial load ran 102
    captures and about 106k `getBoundingClientRect` calls; after the fix it
    runs 2 captures and about 4.5k calls.
  - **The fix.** `assets/stream.js` runs the fill loop before the shared
    `kept.apply()`, so each fill's `keep()` joins the batch.
  - **Stale fixture.** The test's board still used the pre-`15e44e235` history
    markup (`.history-clip` and `.history-older`), so the product's
    RO -> rAF path never ran (`raf: 0` in every stage).
    `testdata/viewed-performance.cjs` now renders the timeline markup the
    server emits (`page.tmpl` `timelinerow` and `.history-gap` with an inert
    template). One gap row holds a frame with no height yet, so a window
    stays pending.
  - **Stages.**
    - "initial load", "viewport resize" and a new "late frame height" stage
      must each exercise reading, RO delivery and rAF.
    - "panel resize" of a settled panel must now do zero capture work. The
      reason is the documented "Settled membership never changes on resize"
      rule (`stream.js` above `progress`).
    - A new per-stage bound of at most 2 captures fails the unfixed source:
      "initial load: 102 captures > 2".
    - The existing per-capture, quiet-frame and `max` bounds are unchanged.
- `TestHistoryLoadAnchorBrowser`: PASS, with no change. It passed 5 of 5 runs
  with `-count=5` and 16 of 16 runs as 4 parallel runners × `-count=4`. Under
  CDP CPU throttling at 4×, 8× and 16× it passed every run. On `a882ca00d`,
  the test's version when MAIN reported it flaky, 16 of 16 parallel runs
  passed. **The flake cause is not identified, because it never reproduced.**
  INFERRED candidate: the fixed 1500ms pin cap (`PIN_MS`, a non-renewing cap
  chosen in `a882ca00d`) against late images arriving at 100/300/600ms. A
  heavily starved runner could deliver an image after the cap, and the anchor
  would then move. CPU throttling up to 16× did not trigger this. No retry
  was added.
- **CI red from the v0.50.0 pre-release run.** Run 43706357281 (ship-gate log
  e4883: "나머지 하나는 이 맥에서도 원래 실패하던 시험입니다") failed
  `TestWhiteboardEffectsKnownCreationBatchAndLegacySeparation`. Its three
  companions were Windows-only. It was not on this ticket's list.
  `0500a7690` fixed it as a stale expectation (a new topic records its initial
  topic-state line), and it passes 3 of 3 runs on `cd477b5a8`.
- **Verification on the fixed head.**
  - `go test ./internal/wswhiteboard` (whole package): ok.
  - `-run 'Browser$'` with the browser env set: every whiteboard browser test
    passed.
  - The wsdashboard `(Board|Whiteboard|History|Timeline|Viewed|Overview).*Browser`
    run passed. `TestBoardViewedReloadBrowser`, `TestBoardViewedLiveBrowser`
    and `TestBoardStateFilterBrowser` passed, and
    `TestBoardViewedUpdatePerfBrowser` passed 3 of 3 runs.


## Resolution (2026-11-16)

Merged as develop 28cf9c7bc (head 75990f973). Gear/RootKeepsSessions were already green (234cef037, c918577da). TestViewedPerformanceBrowser fixed by moving the init 1000px fill inside the shared capture (102 -> 2 captures; before/after screenshots byte-identical) plus fixture markup update. The CI red TestWhiteboardEffectsKnownCreationBatchAndLegacySeparation was already fixed by 0500a7690. TestHistoryLoadAnchorBrowser was not reproduced (5 sequential, 16 parallel, CDP 4x/8x/16x throttle); lead decision: carried as known-unreproduced, inferred candidate the fixed 1500ms PIN_MS cap; a future CI failure line routes a fix task.

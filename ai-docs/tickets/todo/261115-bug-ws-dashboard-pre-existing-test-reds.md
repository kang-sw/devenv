---
title: "Green the known pre-existing dashboard and whiteboard test reds"
related:
  261115-feat-ws-whiteboard-viewed-panel-anchor: "introduced TestViewedPerformanceBrowser (4afc78add)"
  261113-feat-ws-dashboard-session-four-state-lifecycle: "Phase 2 Result records TestRootKeepsSessionsInTheRailOnly red after the landing logo (d2921cf02)"
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

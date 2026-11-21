---
title: "보드 ask 클릭 시 dashboard 렌더러 크래시(STATUS_BREAKPOINT) 및 content-visibility 트리거 라이브 이분 추적"
completed: 2026-11-21
---

# Dashboard renderer crash (STATUS_BREAKPOINT) on board ask click, live-bisecting content-visibility triggers

## Background

The owner views the dashboard as an Edge 157 PWA (Edge 157.0.4341.60, Default
profile). Clicking the body or label of an open TECHNICAL ask row on a session
board crashes the whole renderer with `STATUS_BREAKPOINT`. The shell and all its
`?frame=1` session iframes share one renderer, so one crash takes everything
down.

- The crash is intermittent and flaky. Clicking the checkbox square itself
  crashes less often than clicking the row body.
- On 2026-11-17 around 15:35 the frequency rose sharply, to the point where the
  dashboard was unusable.
- The board involved has 59 topics, 224 panels and is 2.25 MB.

### Evidence

Dumps are in `D:/Temp/pwa-crash/` (local only, not tracked).

- Two renderer dumps, from 15:31 and 15:32.
- Both show exception `0x80000003` (int3) at the same site on the main thread.
- No OOM annotation and no stack overflow, so this is a hard Blink CHECK.
- They cannot be symbolized: Edge PDBs are not public. The dumps cannot name the
  failing function, which is why the investigation moved to live bisection.
- One dump's heap held the capsule class string `note tnote wsc-backing`
  (`capsules.js:84`).

### Suspect: content-visibility skipping from `de4b8a083`

This is the baseline at develop `eed79fc4c`.

`wsdashboard/assets/dashboard-board.css`:
- `:93` `section.topic[data-topic] > :not(.tophead) { contain-intrinsic-block-size: auto 480px; }`
- `:94` `section.topic:not([data-max]) > :not(.tophead):not(:has(details.entry-panel[data-max])) { contain: layout style paint; }`
- `:95` `content-visibility: auto` on `section.topic:not([data-max]):not(:focus-within) > [data-wbcv]:not(.tophead):not(:has(...))`
- `:96` the same on `.bcard:not([data-max]):not(:focus-within) > .bc-in > .bc-body`

`wsdashboard/assets/dashboard-frames.js`:
- `cvRO`/`cvWatch` set `data-wbcv` from a ResizeObserver.
- The `contentvisibilityautostatechange` double-rAF `wbUnskip` handler.

Merge `5e5c7835f`: only `de4b8a083` adds skipping or containment. `e828db7b7`
(`:has` drop) and `dcf405248` (scrollTop) are not involved.

The capsule attach happens on the topic note (`.tophead > .tnote`), so it is
outside the contained/skipped parts. But focusing it flips `:focus-within` on
the section, just as the label click does.

Working hypothesis: a label click moves focus into the topic. That flips
`section.topic:not(:focus-within)`, which un-skips every part at once, together
with `:has()` invalidation. The hypothesis is not confirmed.

## Decisions

- **Keep visible-only rendering.** Owner 2026-11-17 15:56: do not blind-revert
  content-visibility skipping. Without it the dashboard lags to the point of
  being unusable, so find the root cause.
  - Rejected: the blanket revert of `de4b8a083` skipping. It was commit
    `ddae6abc3` and was reset off the branch.
- **Live bisection instead of dump analysis.** Owner 2026-11-17 15:59: edit the
  code directly to locate the cause.
  - The live daemon runs in dev-assets mode
    (`WS_MCP_DASHBOARD_DEV_SOURCE=<worktree>/agents-plugin-tool`), which re-reads
    dashboard assets from disk per request with no rebuild.
  - The owner's real PWA is the repro.
  - Rejected: a headless Edge repro. The owner's PWA reproduces it reliably and
    a headless repro was never achieved.
- **One change per variant.** Each variant is a single change and keeps CV
  skipping on.
- **Pass bar.** A variant passes when the owner sees no crash over sustained
  normal use or 10 ask-row body clicks after a reload. Any crash fails it.

## Phases

### Phase 1: Live-bisect the trigger and ship the minimal fix

Status as of 2026-11-17 16:15:
- The live daemon was restarted at 16:0x as pid 33664 in dev-assets mode,
  reading from worktree `groove-animosity-playroom` (branch
  `impl/develop/pwa-crash-hotfix`).
- V1 is applied there, uncommitted.
- The owner is using the dashboard normally and will report the next crash.

Variants, in order:
- **V1** (applied): drop `:not(:focus-within)` from the topic-part CV selector
  (`:95` only).
- **V2**: `contain: layout style paint` → `contain: layout paint` (`:94`).
- **V3**: disable the `contentvisibilityautostatechange` `wbUnskip` handler.
- **V4**: drop `:not(:has(details.entry-panel[data-max]))` from `:94` and `:95`.
- **V5**: drop `data-wbcv` gating, so CV applies from the start and no attribute
  changes mid-layout from the ResizeObserver.
- **V6**: also drop `:not(:focus-within)` from the `.bcard` body rule (`:96`).

Procedure:
1. If V1 survives the pass bar, commit V1 as the fix.
2. If not, apply the next variant on top of a clean baseline, one at a time.
3. Once the cause is confirmed, commit only the minimal fix and run the w5090
   check:
   `go vet` + `go test ./internal/wsdashboard ./internal/wswhiteboard ./internal/mcp`.
   - Update any test that pins the changed rule.
   - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
     TestTopicStateBadgeClockAndReconciliation.
4. Merge, then swap the daemon back to embedded assets: restart without
   `WS_MCP_DASHBOARD_DEV_SOURCE`.

Constraints:
- Tier T0: served CSS/JS only.
- No local `go test`, ws exes or launcher on the owner's PC (antivirus rule).
- Until this phase ends, every daemon swap for other merges must keep
  `WS_MCP_DASHBOARD_DEV_SOURCE` pointing at the bisect worktree. That worktree
  must not be released, because the live daemon reads assets from it.
- Dev-assets mode re-reads assets per request. That adds file I/O, so it is
  temporary and must be removed at the end.


## Resolution (2026-11-21)

V1 fix (`eb29c9997`, merged `f1b4e1866`) dropped `:not(:focus-within)` from board topic-part `content-visibility` in `dashboard-board.css:91-99` with `ResizeObserver` / double-rAF unskip handling in `dashboard-frames.js:122-152`. Owner confirmed on 2026-11-21 that the Edge/PWA renderer crash is resolved in live use; closed to `.done/`.

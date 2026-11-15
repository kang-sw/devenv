---
title: Web toolbar popover uses viewport coordinates inside an absolute slot
related:
  261114-feat-ws-whiteboard-compact-floating-toolbar: 'baseline defect discovered during source acceptance; explicitly nonblocking for that ticket'
---

# Web toolbar popover uses viewport coordinates inside an absolute slot

## Background

**VERIFIED:** The real Web URL/path popover extends beyond the viewport on the exact `395242d80` baseline and the compact-toolbar candidate. At a 320px browser width its right edge is 380.125px (menu width 304px); at 800px it is 853.71875px. The retained assertion is `Web Enter opens the real viewport-fitting URL/path popover` in `scripts/board-headless.mjs` (search that string).

The 652 continuation authority from owner-reassigned MAIN `id:sweatradartakingoverwrite` at 2026-11-14 10:59 explicitly treats proven equivalent baseline containment failures as nonblocking for 652 and requests this narrow follow-up. It does not authorize a Web redesign or claim the compact-toolbar candidate free of other regressions.

**INFERRED:** `placePop()` in `agents-plugin-tool/internal/wsdashboard/assets/webcard.js` calculates viewport-relative left/top values and assigns them to `.tpop.wpop`, which is absolutely positioned inside `.board-web-slot`. The terminal popover's existing placement uses slot-relative adjustments and is prior art, not a mandate to copy constants.

## Evidence

- Historical 800px baseline/current diagnostic: `/tmp/compact-toolbar-evidence/web-baseline.cjs`, `/tmp/compact-toolbar-evidence/web-baseline.json` and `/tmp/compact-toolbar-evidence/web-baseline.log`.
- Matched 320px/face266 probe: `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@2530e43b/review-paths/ea2fa236-compact-toolbar-652/final-adapter-320-face266/web-containment-comparison.json`.
- Matched 800px probe: `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@2530e43b/review-paths/ea2fa236-compact-toolbar-652/final-adapter-800/web-containment-comparison.json`.
- Reproduction uses the isolated source fixture and `node scripts/board-headless.mjs toolbar --url <fresh-adapter-url> --width 320 --face-width 266 --baseline-ref 395242d80 --capture-dir <new-evidence-dir>`; repeat at width800 without the constrained face. Keep the full containment predicate and record its false result rather than blanket-ignore failures.

**Limits:** These local cache paths are recorded evidence, not portable tracked fixtures. Browser-substituted baseline probes establish static Web geometry only; route-fulfilled navigation is subject to Chromium local-network restrictions, so it is not live WebSocket acceptance. The compared semantic fields are menu left/right, menu width/height and opener width; viewport-relative top legitimately differs between compact and nonfloating states. No fix or general regression clearance is claimed here.

## Constraints

Preserve existing availability, Enter/Escape behavior, opener focus, URL/path handling and horizontal action layout. A future fix should stay T0 dashboard asset content, with no protocol, store, API or tracker-event change. Read the declared shipped-surface and release-compatibility manuals before editing tooling internals.

## Phases

### Phase 1: Correct and verify Web popover placement

Ground a minimal coordinate-space correction in production source. Retain full viewport/column containment assertions and verify top/floating, desktop/narrow and document/board scrollers, real keyboard opening/dismissal and opener focus. Compare the terminal menu as a regression boundary without replacing its independently owned anchor constants. Capture fresh before/after evidence; this idea ticket is not implementation-ready.

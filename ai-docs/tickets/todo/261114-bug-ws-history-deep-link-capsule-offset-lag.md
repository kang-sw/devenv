---
title: Synchronize history deep-link arrival with the capsule offset
related:
  261114-feat-ws-whiteboard-compact-floating-toolbar: 'discovery evidence; Lane A2 executes after 652 merges'
---

# Synchronize history deep-link arrival with the capsule offset

## Background

**VERIFIED:** The real `p1` panel summary can land beneath `DIV.tbar` when deep-link navigation computes its offset before the top capsule stack appears. This was reproduced in an adapter page at 1900×900 on both compact-toolbar source `1975e3be465d3bde667529b5bc0af497e8fdf3bf` and independently built exact baseline `395242d80815f01d4aad5661547b42676f021ff5`.

The targeted `history-paint` precondition opens real history pages, grows an older payload by 420px, temporarily moves the topic horizontally out of view to force the first genuine IntersectionObserver zero, activates a real `#p1` link, then restores the topic after the zero report. The target materializes, the clip remains at scrollTop0 and the keep session remains active, but the summary-center hit belongs to the sticky topic bar.

- Candidate: summary y127.609375–160.609375; sticky head y130–162; center `(1290.828125, 144.109375)` hits `DIV.tbar → DIV.tophead.wb-stuck`. Computed navigate offset changes **78→122px** after capsules appear.
- Baseline: summary y93.609375–126.609375; head y96–128; center `(1290.828125, 110.109375)` hits the same ancestry. Offset changes **44→88px**.
- Both show the same **44px offset lag**; the candidate's 34px toolbar rail inset accounts for the vertical difference. This is specific baseline equivalence, not general regression clearance.

Full logs and diagnostics:

- `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@2530e43b/review-paths/ea2fa236-compact-toolbar-652/post1975-history-candidate.log`
- `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@2530e43b/review-paths/ea2fa236-compact-toolbar-652/post1975-history-candidate/measurements.json`
- `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@2530e43b/review-paths/ea2fa236-compact-toolbar-652/post1975-history-candidate/adapter-deep-link-visible.png`
- Corresponding exact-baseline artifacts are in `post1975-history-baseline.log` and `post1975-history-baseline/` beside those files. The harness is `node scripts/board-headless.mjs history-paint --url <fresh-adapter-url> --width 1900 --artifacts <new-directory>` with the same seeded history fixture; preserve the complete arrival/ownership predicate.

## Decisions

- **MAIN technical assignment 690, 2026-11-14 11:31:** this is an owned **Lane A2** bug, executed immediately **after 652 merges and before Lane B**. Capture it in todo now; no ready promotion or implementation occurs during 652. This supersedes an unowned idea disposition for this precise defect.
- Keep the correction minimal: synchronize the existing private navigation offset with the capsule geometry, preserving shared keep/scroll ownership. Add a regression and independent lite review. Compatibility target is **T0** asset content; no new API, protocol, tracker event or store contract is authorized.
- This is not the other lane's first real-Tab `e320` system-summary occlusion by `.oq-pill`. No equivalence to `9854ff350`, that `e320` case, or the fixed tracker/git handle collision is claimed.

## Constraints

Read `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md` before tooling edits. Preserve full assertions, history chunk boundaries, actual frame identity, keyboard focus, user-scroll cancellation and existing rail ownership. Do not add a competing scroll writer or weaken ownership checks. Source/runtime activation remains separately lead-owned.

## Phases

### Phase 1: Synchronize deep-link offset and verify arrival

Ground the ordering race in `navigate()` in `agents-plugin-tool/internal/wswhiteboard/assets/stream.js` and capsule offset publication in `assets/rail.js`. Apply the smallest supported synchronization correction after 652 integration. Extend the existing harness/test contract so the forced-first-IO-zero `p1` summary is exposed and owns its center after materialization and late height changes. Verify adapter/document scrollers and ordinary navigation without that precondition, preserve existing keep ownership and cancellation, run applicable package/browser checks and independent lite review. Record exact tested source pins and evidence; do not fold unrelated pill or Web-popover fixes into this ticket.

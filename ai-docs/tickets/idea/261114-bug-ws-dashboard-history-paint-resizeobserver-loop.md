---
title: "board-headless history-paint fails on develop with a native ResizeObserver loop error"
---

# history-paint ResizeObserver loop error

## Background

Found by the EXEC lead on 2026-11-14 while verifying the board performance
Phase 1 merge (21c3d4262).

- `node scripts/board-headless.mjs history-paint --url <adapter page> --width 1800`
  fails with `native ResizeObserver loop errors: ResizeObserver loop completed
  with undelivered notifications.` (VERIFIED)
- The page was served by `serve --topics 40 --history-entries 180`.
- It fails twice out of two runs on develop 534fe3297, before the perf merge,
  and twice out of two after it. The perf merge did not cause it. (VERIFIED)
- On the same page, `history`, `history-cards` and `history-regressions` pass.
  `keep` and `observer` pass after harness fix 3a8ecd7b3. (VERIFIED)
- The topic-history merge (be41211bb) fixed a loop of this kind: stream.js
  moved its control visibility writes out of the history ResizeObserver
  callback. So the loop may have come back with a later develop change, most
  likely the iframe shell or the session-rail tree merges (534fe3297). (INFERRED)
  This has not been bisected.

## Notes

- First step: bisect between be41211bb and 534fe3297 with the same scenario.
- The scenario runs in the shell's child frame (`/s/<rid>?frame=1`), so a
  ResizeObserver in the shell or the rail could also be the source.

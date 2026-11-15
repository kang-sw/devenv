---
title: Viewed board shows one 50-57 ms long task on the first update or a panel insert near the view
related:
  261115-feat-ws-whiteboard-viewed-panel-anchor: follow-up accepted by MAIN after the Viewed refix (exec4/viewed-refix)
---

# Viewed board shows one 50-57 ms long task on the first update or a panel insert near the view

## Background

The Viewed refix (261115-feat-ws-whiteboard-viewed-panel-anchor, "Viewed
Refix - 2026-11-15") brought steady-state log-append updates to 25-38 ms
synchronous work with no long task (baseline 95-132 ms plus a 105-122 ms long
task) and board swaps to 30-40 ms. One residue remains and was accepted as a
follow-up rather than a gate failure:

- About 1 in 12 updates still produces a single post-dispatch long task of
  50-57 ms (pre-refix baseline for the same update: about 62 ms).
- It shows on the first update after load, or on an update that inserts a
  panel frame near the viewport.

## Candidates (unverified)

- Post-swap requestAnimationFrame / forced layout in the dashboard board
  swap path (`boardWithSnap`, exposure-unit scan over live panels).
- Panel iframe remount on insert: frame document parse plus the doc-frame
  height reporter's first ResizeObserver round.

## Reproduction

The opt-in browser fixtures from the refix reproduce it:
`TestBoardViewedLiveBrowser` and `TestBoardViewedUpdatePerfBrowser` in
`agents-plugin-tool/internal/wsdashboard` (skipped unless
`WS_VIEWED_PERF_PLAYWRIGHT` / `WS_VIEWED_PERF_CHROMIUM` are set). Run them
with `-run`, never the full wsdashboard package. The live fixture gates on
long tasks under 50 ms per update; a trace of the offending update attributes
the task.

## Open Questions

- Is the long task in the parent document or in the inserted frame's process
  (out-of-process iframe vs same-process)?
- Is the first-update case the same cause as the panel-insert case?

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - Minor leftover performance issue where a roughly 50 ms long task occasionally occurs on the first update; only candidate causes known.

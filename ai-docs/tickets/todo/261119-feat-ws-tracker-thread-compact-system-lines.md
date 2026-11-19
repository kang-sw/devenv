---
title: tracker 스레드 시스템 메시지 compact 표시와 연속 구간 접기
related:
  261119-feat-ws-tracker-org-chart-editor: sibling tracker UI work from the same owner session; independent, no ordering
---

# Tracker thread: compact system lines and collapsible runs

## Background

In a tracker item thread, system events (assignment, CC, field changes and
similar non-comment events) render at the same row height and font size as
user comments, so long runs of system events bury the conversation. The owner
asked (2026-11-19 10:13) that system messages be visually compact and distinct
from comments, and that consecutive system messages can be folded.

Today the thread renders one line per event with no grouping:
`evLine` in `agents-plugin-tool/internal/wsdashboard/assets/tracker-window.js`
(around 628-632) and in `tracker.js` (around 647-652). Both render paths must
behave the same.

Approved mockup (owner check (1389)=ok, 2026-11-19 10:31):
`ai-docs/assets/mockups/261119/tracker-thread-compact/thread.html` (root class
`.tktc`).

## Decisions

1. **Compact system lines.** Non-comment events render as compact lines:
   smaller row height and font size than comments, muted colour, as in the
   mockup. Comments keep their current look.
2. **Run definition.** A run is 2 or more consecutive non-comment events
   between comments. Rejected: grouping by time window — the owner asked for
   "consecutive" events, and time windows split or merge runs unpredictably.
3. **Default fold.** A run of 4 or more events renders collapsed to a single
   summary line ("시스템 이벤트 N개" style, see mockup) that expands on click; runs
   of 2-3 render expanded. Rejected: always collapsed — short runs carry little
   noise and folding them costs a click.
4. **Runs that stay open.** A run containing a rejected or conflict event
   renders expanded by default, so problems are never hidden.
5. **Status changes break runs.** Status-change events are not folded: they
   end the current run, render as a visible compact line, and a new run may
   start after them. The mockup does not draw this case; render it as a compact
   line with status emphasis consistent with the mockup's palette.
6. **Fold state is in memory only.** Expanded/collapsed state per run lives in
   page memory and resets on reload. Rejected: persisting fold state — it would
   add file or store writes on an interactive path for little value
   (`ai-docs/manuals/performance.md`).
7. **Compatibility: T0.** Rendering only; no event, store, route or API shape
   changes.

## Constraints

- Applicable manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/performance.md`, `ai-docs/manuals/release-compatibility.md`
  (T0 statement only).
- Every new user-visible string goes into all locale tables under
  `agents-plugin-tool/internal/wsi18n/tables/`.
- No local `go test`, Go tools, gofmt, ws/ws-mcp exes, launcher or Electron on
  the owner's Windows PC (antivirus). Go tests run on remote w5090 only
  (`D:/sys_lock.sh remote w5090 run ...`; w5090 has no Node). Run only touched
  JS test files locally with node.
- Do not start, stop, rebuild or swap the shared ws daemon; the LOOK capture
  uses an asset-level fixture page that loads the real tracker assets with
  fixture events, not the live daemon.
- Temp files only under `D:/Temp/<purpose>/`.

## Phases

### Phase 1: Compact system lines and foldable runs

Implement Decisions 1-7 in both thread render paths (`tracker-window.js` and
`tracker.js`), sharing the grouping logic rather than duplicating it where the
existing module structure allows.

Verification:
- A pure-function test (node) for run grouping: comment boundaries, the 2+ run
  threshold, 4+ default collapse, a rejected/conflict event keeping a run open,
  and status-change events breaking runs.
- Existing dashboard Go tests pass on w5090.
- LOOK: capture the fixture thread (collapsed and expanded states, a run with
  a rejected event, a status-change break) and compare it side by side with the
  approved mockup; report differences and fix any that are not justified by
  real data.

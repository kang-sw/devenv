---
title: ws dashboard frontend performance - profile and optimize
---

# ws dashboard frontend performance - profile and optimize

## Background

Owner report 2026-11-13: "the dashboard frontend feels very heavy". No
measurements exist yet. The goal is to measure where the cost is on the live
dashboard, then fix only the measured top hotspots without changing what the
dashboard looks like.

Frontend surface: `agents-plugin-tool/internal/wsdashboard/assets/`
(`dashboard.js` 6926 lines, `tracker.js` 6438, `cards.js` 921, `files.js` 855,
`terminal.js` 631, `rail.js` 355, `mention.js` 310, `preview.js` 255,
`dashboard.css` 1772, `tracker.css` 1160; vendored `xterm/`).

### Observations (unmeasured)

Light read-only pass; these are candidates to look at in Phase 1, not
established causes.

- `dashboard.js:97` - `setInterval(tickTimes, 1000)`, a permanent 1 s timer.
- `dashboard.js:533`, `:1051` - `innerHTML` replacement of whole regions on
  update (`:1051` has a keyed-diff path via `box`/`nbox` and a full-replace
  fallback); `:4849`, `:4895` - conversation live/entry regions set via
  `innerHTML`.
- `dashboard.js:173`, `:979`, `:3078` - rAF-scheduled work; `dashboard.js:5321`
  - WebSocket with 3 s reconnect (`:5353`); `terminal.js:193` - per-terminal
  WebSocket.
- `dashboard.js:6841` - 400 ms draft-save debounce on typing.
- Two very large single-file scripts (`dashboard.js`, `tracker.js`) are loaded
  as a whole; parse/execute cost at load is unknown.

## Decisions

- Measure first, fix only measured hotspots. Rejected: speculative rewrite or
  framework migration.
- Implementer is a Claude (Opus) worker: the owner directed that frontend
  adjustments go to Opus agents, and the change needs visual-regression
  judgment. Rejected: Pi worker.
- Builds and tests run on the sys_lock remote w5090; live measurement runs on
  the owner's Windows machine dashboard.

## Constraints

- No server-side protocol or store change unless Phase 1 proves a server
  payload is the hotspot; then it must be T0 additive per AGENTS.md
  `### Compatibility Tiers`.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing, per AGENTS.md
  `### Implementation Conventions` (paths under `agents-plugin-tool/internal/`
  and `agents-plugin*/`).
- No intended visual change.

## Phases

### Phase 1: Measure

Reproducible scenarios on the live dashboard:

- (a) initial load and refresh.
- (b) idle CPU with many sessions and live streaming.
- (c) long whiteboard/conversation scroll and typing.
- (d) session/tab switching.

Tools: browser Performance trace, Performance API, long-task counts, plus
network request counts and payload sizes. Record numbers per scenario in the
Result and rank hotspots. Scenario priority is set by (417) in the queue
below.

### Phase 2: Optimize measured hotspots

Optimize only the top measured hotspots from Phase 1, using the simplest fix
for each. Re-measure with the same scenarios and record before/after numbers.
No intended visual change; LOOK before/after captures confirm no regression.

## Open Decision Queue

Last ID: (417)

(417) [open] [policy] Which scenario feels heaviest, setting Phase 1
measurement priority: a initial load, b idle CPU with sessions/streaming,
c whiteboard/long conversation scroll and typing, d session/tab switching,
e all - measure every scenario. Recommendation: e.

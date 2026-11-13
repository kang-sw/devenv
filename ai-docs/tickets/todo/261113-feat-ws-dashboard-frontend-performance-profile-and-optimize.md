---
title: ws dashboard frontend performance - profile and optimize
related:
  261113-feat-ws-dashboard-iframe-shell: session-switch cost is structural (full document reload per navigation) and is addressed there; this ticket measures (d) but does not restructure navigation
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
- (417) Measure all four scenarios (a-d), with priority on (b) idle CPU and
  especially whiteboard NOTE typing latency. Owner answer 2026-11-13, recorded
  as the lead's reading (assumed): the board note pen editor feels much slower
  than typing in the main left composer. Rejected: measuring only one scenario.
- Owner input 2026-11-13: the whiteboard page gets extremely long; the owner
  asks for rendering/caching logic for long content (e.g. not rendering
  hidden/collapsed boxes, caching long content efficiently). The dashboard is
  not React: it is server-rendered HTML plus vanilla JS assets with no build
  step, regions replaced via `innerHTML` (see Observations). Keep the
  measure-first decision: techniques are picked by measured effect. The
  in-flight wb-stream mockup-G work (latest 3 panels expanded, height-limited
  stream with "load older") already reduces the initial DOM; measure after it
  lands.

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
Result and rank hotspots. Priority per (417): (b) idle CPU first.

- Compare the two input paths: typing latency in the whiteboard note pen editor
  versus the main left composer; find why the note editor is slower.
- Scenario (d) baseline: session switching currently takes about 500-1000 ms
  (owner observation). Session-switch cost is structural (full document reload
  per navigation) and is addressed by
  `261113-feat-ws-dashboard-iframe-shell`; this ticket measures (d) but does
  not restructure navigation.
- Long-whiteboard case: on a very long board, record DOM node count,
  layout/style recalculation time, and per-keystroke cost while typing a board
  note. Take it both before and after the wb-stream mockup-G work.

### Phase 2: Optimize measured hotspots

Optimize only the top measured hotspots from Phase 1, using the simplest fix
for each. Re-measure with the same scenarios and record before/after numbers.
No intended visual change; LOOK before/after captures confirm no regression.

Candidate techniques for long whiteboard content, to evaluate against the
Phase 1 long-whiteboard measurements, cheapest first; pick by measured effect,
not all by default:

1. CSS `content-visibility: auto` with `contain-intrinsic-size` on panels and
   topic sections, so off-screen boxes skip layout and paint.
2. Lazy DOM: collapsed panels, hidden `<details>`, and settled-item bodies are
   not built until opened.
3. Keyed incremental updates instead of whole-region `innerHTML` replacement
   for the board.
4. A per-topic rendered-HTML cache keyed by content hash.

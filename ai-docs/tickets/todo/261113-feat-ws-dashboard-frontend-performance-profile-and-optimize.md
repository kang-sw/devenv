---
title: ws dashboard frontend performance - profile and optimize
related:
  261113-feat-ws-whiteboard-board-frontend-performance: "absorbs this ticket's board scope ((417) board-note typing, long whiteboard, the board candidate techniques) and the (473) measurement; this ticket keeps the non-board surfaces"
  261113-feat-ws-dashboard-iframe-shell: historical shell prerequisite; new cache removal is owned by the hidden-cached-page ticket
  261116-bug-ws-dashboard-hidden-cached-page-cost: "P3: remove hidden session cache; visible-only shared clocks; fresh-switch load follow-up"
  261116-bug-werkschaft-electron-high-resolution-lag-diagnostics: "P2: isolate native high-resolution presentation from shared renderer costs"
  261111-bug-ws-dashboard-daemon-idle-cpu-windows: "backend profile/discovery costs, measured separately"
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

- Board scope moved (owner (532)=a, 2026-11-13): the whiteboard board's
  frames, swaps, note typing and idle timers, and the measurement harness,
  belong to `261113-feat-ws-whiteboard-board-frontend-performance`; this
  ticket keeps the non-board surfaces (tracker, conversation regions,
  script parse cost) and reuses that ticket's harness.

- No server-side protocol or store change unless Phase 1 proves a server
  payload is the hotspot; then it must be T0 additive per AGENTS.md
  `### Compatibility Tiers`.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing, per AGENTS.md
  `### Implementation Conventions` (paths under `agents-plugin-tool/internal/`
  and `agents-plugin*/`).
- No intended visual change.

## Additional measured evidence — 2026-11-16 (P1/P4)

Duplicate capture: retain P1 shared render-transaction and P4 main-composer
work in this existing non-board frontend backlog rather than create overlapping
tickets. Board-local frame/history/note work remains with its existing owner;
coordinate the changed-topic postprocessing boundary rather than duplicate it.
All paths below are under `agents-plugin-tool/`.

Evidence source read: `D:/Temp/dash-scroll-perf/SUMMARY.md`. The summary's
Windows isolated browser probes are not a reproduction of the owner's aged,
exposed Electron lag. The initial unmeasured observations above are historical;
these are fresh measurements, not an assertion that one root cause is proven.
No visual change is intended for either performance-only refactor.

### P1: Shared render transaction (priority P1, size M)

- `internal/wsdashboard/assets/dashboard-stream.js:449-461` buffers latest
  canonical input per region while hidden, but visible events call `swap`
  immediately. There is no shared rAF render transaction (`:153-175`). The
  transport is WebSocket despite SSE naming in source/task descriptions.
- A single-topic patch triggers board-wide postprocessing at
  `internal/wsdashboard/assets/dashboard-board.js:137-150`: frame indexing,
  forms, capsule notes, chips and sent markers. Existing morph paths reuse
  unchanged nodes; full DOM replacement is not the claim.
- Initial board data measured 383,900 B wire (~384 KB), 291,852 B HTML.
  `internal/wsdashboard/board_topics.go:20-30,104-119` emits the initial full
  board and complete changed-topic HTML/order, not append-only log fragments.
  No steady board event was captured in the 47.851-second fresh-session sample;
  earlier whiteboard burst cost remains unmeasured.

Reproduction: in an isolated profile, replay identical ordered region/topic
bursts and a large topic update; count parses, swaps, postprocessing visits,
geometry/scroll work and frame/long-task timings. Retain the current hidden
latest-per-region behavior as a control; do not posit an unbounded FIFO.

Candidate direction: parse ordered patches into canonical state in order,
coalesce only render commits to the latest canonical region per rAF, batch
geometry reads/scroll capture/writes, and postprocess only changed units.
Coalescing must not discard ordered deltas before they have been applied.
Preserve drafts, history, selection, focus, scroll anchoring and hash ordering.
Budgeted/unit rendering is a later option only if one large transaction remains
measurably expensive.

Open questions: which regions can share one transaction without breaking
ordering/scroll contracts; how are canonical dependencies and removal/full-board
fallbacks represented; and what measured transaction budget is appropriate?
These mechanism/acceptance choices are not settled by the performance report.

### P4: Composer layout thrash and slash guard (priority P4, size M; guard S)

- `internal/wsdashboard/assets/dashboard-composer.js:386-396` writes height
  auto, reads scrollHeight, then writes height on each input (`:1651`).
- In an isolated headless long-prefix case (5,500 characters plus 104 typed),
  bypassing real scrollHeight/autosizing reduced outer handler mean from
  0.919 to 0.272 ms/key (~70%), grow-group 0.677 to 0.029 ms and total layout
  200.40 to 99.91 ms. This diagnostic bypass is not a shippable fixed-height fix.
  The original mean was below 1 ms: severe typing freeze was not reproduced.
- Slash autocomplete enumerates commands before checking the trigger at
  `dashboard-composer.js:820-845`. Removing popup sync saved only ~11% of the
  outer handler and did not reduce layout in the measured case. Topic lookup
  is cached; ordinary typing is not proven to perform network ticket lookups.

Reproduction: compare short and long drafts using real input without sending;
measure handler/layout and visible latency separately. Include slash/non-slash,
IME, selection/caret, pasted/fragmented long drafts, dense results and live
updates. CDP dispatch latency is not key-to-photon latency.

Candidate direction: batch autosize and popup placement to avoid synchronous
write/read/write thrash; check the slash trigger before enumeration; consider
cached command metadata/unchanged-popup skips only when measured. Preserve
actual autosizing, IME/caret, keyboard selection, slash results and send behavior.
Open questions: what scheduling boundary preserves immediate caret/popup
correctness, and which dense/fragmented inputs warrant further work? No fixed
height, virtualization or editor rewrite is preselected.

### Compatibility and phase integration

Compatibility tier: **n/a** for client-only scheduling/input refactors with
unchanged wire payloads, stores, manifest fields and draft format. Existing
T0-only constraint still governs any proposed server-surface extension; no such
extension is chosen here. Use the existing Phase 1 to establish matched replay
and typing baselines, then Phase 2 for the measured P1/P4 corrections. Assert
ordered final DOM and preserved drafts/history/selection/hash/scroll plus
unchanged visual output, and report before/after counts rather than only a
subjective speed claim. No builds, tests or browser/daemon actions occur in
this docs capture.

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

---
title: ws dashboard frontend performance - profile and optimize
sage-review-design: skipped
sage-review-completeness: skipped
related:
  261113-feat-ws-whiteboard-board-frontend-performance: "absorbs this ticket's board scope ((417) board-note typing, long whiteboard, the board candidate techniques) and the (473) measurement; this ticket keeps the non-board surfaces"
  261113-feat-ws-dashboard-iframe-shell: historical shell prerequisite; new cache removal is owned by the hidden-cached-page ticket
  261116-bug-ws-dashboard-hidden-cached-page-cost: "P3: cache removal and fresh-switch load follow-up; clocks dropped after measured trace"
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

Owner priority update (2026-11-16 10:05): performance is top priority in
parallel with `261116-bug-ws-exe-launch-elimination`. The P1 render transaction
and P4 input work below belong to that priority set; P1/P4 retain the original
finding labels, not lower scheduling authority. Input scope includes composer
autosize and both slash/`#` trigger guards. This is backlog authority only;
implementation awaits the owner's explicit assignment.

- Measure first, fix only measured hotspots. Rejected: speculative rewrite or
  framework migration.
- Historical frontend/visual implementer preference: Claude (Opus). The direct
  2026-11-16 exposure assignment below authorizes this worker's bounded
  behavior-preserving performance implementation; it does not transfer the
  independent composer work or authorize a visual redesign.
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
- 2026-11-16 exposure assignment: only Phase 3 is promoted for implementation
  in the assigned isolated worktree. Do not edit dashboard-composer.js or
  capsules.js (Exec 5 owns typing), hidden-cache source, daemon lifecycle or
  unrelated phases. No local Go/test binary/ws executable/launcher runs;
  scoped Go checks on w5090 only. Local offline Node contracts spawn no ws
  executable. Live traces and daemon swaps are lead/owner-arranged.

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

### P4: Composer layout thrash and slash/# guards (finding P4, size M; guards S)

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
measure handler/layout and visible latency separately. Include slash, `#` and
non-trigger typing, IME, selection/caret, pasted/fragmented long drafts, dense results and live
updates. CDP dispatch latency is not key-to-photon latency.

Candidate direction: batch autosize and popup placement to avoid synchronous
write/read/write thrash; guard slash and `#` paths before unnecessary source
lookup/enumeration or popup work. The measured pre-trigger enumeration defect
is slash-specific; audit `#` before claiming the same defect there. Consider
cached command metadata/unchanged-popup skips only when measured. Preserve
actual autosizing, IME/caret, keyboard selection, slash/topic results and send behavior.
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

## Exposure assignment and trace decision — 2026-11-16

Read the complete supplied analysis
`C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac/review-paths/f2812641-01-perf-trace-analysis-20261116.md`.
Trace A: 13.18/14.46 seconds main-thread busy, 584 layouts / 6.69 seconds,
449 forced layouts / 5.81 seconds; board exposure accounts for **44% of busy
main-thread time**, up to **140 full-document layouts in one rAF** (2.41 seconds).
The scroll-path `swap("board", ..., still=true)` re-run costs 3.1 seconds
inclusive. The chain is whiteboard `stream.js:566` exposed, dashboard `visible`,
`boardExposureUnits`, `boardReleaseDue`, `boardFlushViewed`, `boardKeep` and
history reading capture. These are measured priorities, superseding the broad
candidate ranking above. Postprocessing is not hot (<0.3% self); clocks cost
32 ms over this trace; slash/# enumeration is refuted. Composer grow is a
separate directly assigned branch and must not be touched here.

Promote **Phase 3 only**. Compatibility: **n/a**, private client exposure/read
batching and release gating; no protocol/store/draft/event/config shape changes.
Preserve exact one-pixel, clipped/sticky reading-boundary and per-scroller
viewed/unread semantics, top-origin click-only holds, immutable canonical input,
owner receipt admission and explicit release. No framework/config change.

Trace C read before implementation (2026-11-16 10:47):
`C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac/review-paths/cc07bc5e-01-perf-trace-c-board-scroll.md`.
Layout is 84.1% of 11.98 seconds busy time: 661 layouts / 10.085 seconds at
exposed's getBoundingClientRect, including 548 / 8.275 seconds from the scroll
gate and 113 / 1.810 seconds from frame-height keep/reading capture. Each read
re-dirties full-document layout (~9,250 objects, ~15 ms), without JS writes
between reads. Closed-details display-lock forcing is the likely mechanism;
closed-details filtering after the read is too late. **Batching reads alone is
not a fix.** The scroll re-swap did not occur in C; the gate itself is the cost.

Updated light design sanity pass: reject closed-details/non-rendered nodes
before geometry and candidate enumeration, preserving the details' own visible
summary and collapsed panel box. Move scroll release checks to existing
IntersectionObserver change reports, not a raw scroll rAF. The exact existing
one-pixel/sticky clipping check still decides release on those changes. Use a
reading-line hit-test (or observed candidates) before any fallback scan in
shared keep/reading; never measure hidden fallback candidates. Resolve the
frame-height batch's scrollers before writing heights and capture once per
scroller. Do not add content-visibility:auto or virtualization: Forward only,
after hidden-read elimination. Offline Node contracts must demonstrate hidden
geometry is never read and a 1,000-unit scroll does not scan every unit. The
owner performs live re-tracing after merge.

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

### Phase 3: Remove board-exposure layout amplification from scrolling

Implement the measured exposure phase above without pulling in composer or
unmeasured generic render-transaction changes. First reject hidden/closed-details
descendants before any geometry read in shared exposed/reading and dashboard
candidate lists. Drive release checks
from observer exposure changes, not every scroll rAF; retain exact clipping at
those change boundaries. Optimize the frame-height keep/reading path as well.
A scroll must not measure all units or re-run a whole board swap when no held
content/placement can release. Preserve
per-panel holds, cross-topic frozen intervals, pins/closed-page transfers,
user-scroll ownership, top-origin click-only pending counts and explicit release.

Verification: an offline Node contract must reject the old per-unit synchronous
scroll path and count layout reads for a synthetic 1,000-entry board. Cover
closed details (including summary exceptions), non-rendered ancestors,
unchanged exposure, partial/actual release, cross-topic spans, DOM replacement,
independent scrollers, reading-line hit-test and the no-IntersectionObserver
fallback. Include hidden historical versions/system runs in the 1,000-unit
fixture. Record content-visibility:auto/virtualization only as a future phase.
Run the scoped Go regressions on w5090; Node-dependent Go cases skip there, so execute the
Node contracts locally without ws processes. Source-only checks must not claim
native forced-layout timing or the owner's post-merge trace. One R1 review and
fixes-only R2; report HEAD/range at this phase boundary.

### Result (286495f00) - 2026-11-16

Implemented the assigned exposure Phase 3 on `impl/develop/defy-afoot-dares`,
base `1682b0c49`; other phases are not claimed complete.

- Shared `rendered()` rejects closed-details descendants, hidden attributes,
  display-none/display-locked states and detached nodes before geometry. A
  closed details box and its own first summary remain eligible. Apply this
  filter to exposed(), reading candidates, sticky headers, dashboard live
  panels and exposure units. Observing a hidden node skips scroller inspection.
- Modern raw scroll events no longer scan/re-stage the board. Topic/panel/card/
  group observer exposure transitions queue one exact release check; repeated
  identical reports do nothing. Disclosure toggles and the coalesced old-engine
  fallback remain supported. Existing canonical/viewed/unread staging logic,
  one-pixel/sticky/clip checks and top-origin click-only behavior are retained.
- Shared reading capture first hit-tests the reading line and preserves the
  original hidden-safe fallback for gaps/overlays/old engines. Rects and snapshot
  visibility are memoized per read-only pass. Frame-height batches share cached
  ancestor/scroller resolution, capture once per actual scroller, and reuse the
  pre-write result rather than inspecting keep after each height write.
- No dashboard.js, dashboard-composer.js, capsules.js, clock, hidden-cache,
  style/virtualization or daemon lifecycle edits in this branch.

Verification actually executed:

- Local `node --test agents-plugin-tool/internal/wsdashboard/testdata/board-exposure.test.cjs`:
  PASS. Parses full shipped dashboard/shared-history sources; runs hidden and
  summary/fallback tests, actual observer subscription/coalescing, old-engine
  fallback, existing shared viewKit contracts and static viewed-panel contracts.
- Synthetic 1,000-unit board with 50 closed descendants: **0 getBoundingClientRect
  calls and 0 release checks from 100 raw scroll events**; **4 rect calls** for
  the actual `applyFrameHeights -> boardKeep -> keep -> reading` batch. No hidden
  node rect was read. Counts are VM instrumentation of rect calls, not measured
  native forced layouts, hit-test latency or the owner's browser frame rate.
- Red controls run from `git show 1682b0c49:<source>` piped into the same offline
  tests: shared-history base fails `hidden geometry was measured`; dashboard
  base fails `raw scroll ticks must not recompute exposure`. No local Go/ws
  executable or test-built fixture is involved.
- Remote w5090 job `18241e3a64dd478b91324c730999fa28`:
  `go test ./internal/wsdashboard/ ./internal/wswhiteboard/ -run "^Test(BoardViewed|BoardKeep|BoardFrame|BoardExposure|ViewKit|StreamScript)"`
  PASS (0.196 s / 0.018 s). Node-only Go cases skip on the Node-less remote;
  their direct offline Node runs above are separate evidence. Go-rendered
  dynamic fixture cases were not independently run under local Node.
- `git diff --check`: PASS. R1 and owner live re-trace remain pending. No daemon
  swap/rebuild/restart, local Go/ws/launcher/test binary, browser/account action
  or push performed.

**Forward:** evaluate content-visibility:auto/contain-intrinsic-size or lazy
DOM/virtualization only AFTER this hidden-measurement guard lands and the owner
re-traces unavoidable long-document layout cost. Do not add them here or infer
that batched reads alone make display-locked subtree measurements safe. Broader
render transactions and per-layout DOM cost remain later measured work; composer
implementation belongs to Exec 5's independent branch.

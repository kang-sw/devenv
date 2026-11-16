---
title: Composer autosize forces a full-document layout on every keystroke
sage-review-design: skipped
sage-review-completeness: skipped
related:
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: parent perf ticket; this splits out the measured composer typing cost
completed: 2026-11-16
---

# Composer autosize forces a full-document layout on every keystroke

## Background

Owner report 2026-11-16: typing in the dashboard composer is very laggy
(Chrome SPA and Electron). Owner trace B (Trace-20261116T100857, ~5.7 s of
IME typing) measured key-to-presentation latency of median 1136 ms / max
2304 ms for keydown, decaying to 165-500 ms late in the run.

Measured cause (evidence:
`C:\Users\user\.cache\ws@kang-sw-devenv\proj\de04b4ac\review-paths\f2812641-01-perf-trace-analysis-20261116.md`,
machine-local):

- `agents-plugin-tool/internal/wswhiteboard/assets/capsules.js:175-181`
  `notify` dispatches a synthetic `input` event (nested inside the native
  one, doubling input dispatches).
- The composer listener at
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard-composer.js:1651`
  runs `grow(); syncMarkers(); trackMentions(); paintMentions();`.
- `grow()` (`dashboard-composer.js:386-396`, editor branch ~:390) sets
  `editor.style.height = "auto"` then reads `editor.scrollHeight`: one forced
  full-document layout (~17 ms; layout root #document, ~7700 layout objects)
  per input, plus another ~17 ms layout next frame because the height write
  dirties layout again. 918 of the 958 ms input chain is `grow()`.
- IME composition produces 3+ events per character, so ~35 ms/event queues.
- Not a factor: slash/# enumeration (`commandList` ~:820, 0.5 ms);
  syncMarkers/trackMentions/paintMentions (~40 ms total).

## Decisions

- Simplest fix first: no forced layout per keystroke. Preferred: CSS
  `field-sizing: content` with the existing max-height (Chromium/Electron
  support it), removing the JS height write; if the composer element is a
  contenteditable where that does not apply, measure at most once per
  animation frame, only when content could change line count, and write the
  height only when it differs.
- Remove the duplicate work from the nested synthetic `input` (capsules.js
  notify) if it re-runs `grow()` for the same change.

- Per the owner's lightweight-promotion instruction, skip formal Sage stages;
  use the quick design sanity pass below and lead-arranged independent review.
- Design sanity pass: the live capsule editor is a contenteditable `div`,
  not a textarea. Its existing block layout already grows/shrinks naturally
  within the CSS 38–160px bounds with internal overflow; no JS measurement
  or `field-sizing` is needed for it. Use native `field-sizing: content`
  for the plain textarea, with a frame-coalesced measuring-layer fallback
  only on browsers without support.
- `capsules.js` is now under `wsdashboard/assets/`. Its native editor input
  stops propagation and forwards once to the backing field; retain that
  notification contract rather than removing the composer's only input.
- No cross-version tool, protocol, config, or stored-data surface changes.

## Constraints

- Read and follow `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` for internal dashboard edits.
- Do not edit dashboard board/stream exposure code (a parallel executor owns it).
- Local verification is Node-only without ws executable spawning; Go tests
  run only remotely on w5090. No local ws binary, launcher, daemon restart,
  Go command, or test binary. Live performance verification belongs to the lead.

## Phases

### Phase 1: Keystroke path without forced layout

- Implement the decision above; keep visual behavior (min/max height,
  scroll inside the editor beyond max) identical.
- Verify: a node-vm or existing composer client test pinning that input
  handling performs no height=auto/scrollHeight read per event (or at most
  one per frame); a before/after DevTools trace or Performance-panel note
  from a live check arranged with the lead (no local Go / ws exe runs on the
  owner's PC; Go tests remote on w5090).

### Result (f96bb52ec) - 2026-11-16

- The live contenteditable composer now relies entirely on its existing
  CSS block sizing, 38px minimum, 160px maximum, and internal overflow.
  `grow()` performs no height write or layout read and installs no resize
  observer/font-ready measurement for that path.
- Plain textareas use `field-sizing: content`. Unsupported browsers measure
  the existing hidden layer at most once per animation frame, never reset
  visible height to `auto`, and write visible height only when it differs.
  Placeholder, trailing-newline, width-change, and font-ready behavior are
  covered by deterministic tests; actual rendered layout remains a live-check
  responsibility.
- Retained capsule input forwarding: source and the 30-input regression show
  that the native editor event stops propagation and produces exactly one
  backing-field notification. Nested dispatch is not a second composer input
  in this checkout, so removing `notify()` would lose the input contract.
- Local PASS: `node agents-plugin-tool/internal/wsdashboard/testdata/composer-autosize.test.cjs`
  and `node agents-plugin-tool/internal/wsdashboard/testdata/capsules.test.cjs`.
  The new node-vm fixture executes production autosize setup and the actual
  input listener with visible-field geometry getters that throw; CSS-sized
  paths produce zero measurements/height writes/frame callbacks across an
  input burst. The legacy fallback coalesces the burst and skips unchanged
  visible-height writes. `git diff --check` passed.
- Remote PASS on w5090, via the owner-prescribed sys_lock sync/run command:
  `go test ./internal/wsdashboard/ -run "TestComposer|TestInlineCapsules" -count=1 -v`.
  Node-dependent and opt-in browser cases skipped as expected (no Node there);
  asset ownership, placeholder/width wiring, and no-note-row checks passed.
- No local Go/ws/test binary, launcher, or daemon lifecycle operation ran.
  No board/stream exposure changes or worker-created temporary files.
- Pending lead acceptance: independent review plus a before/after typing/IME
  Performance-panel trace in Chrome SPA/Electron, including wrapping,
  shrink-after-delete, long placeholder, max-height scrolling, and width/font
  changes. No live latency improvement is claimed from the deterministic tests.
- Workflow surprise: `tickets.move` refused the occupied assigned worker
  checkout; used the documented `git mv` fallback to promote the ticket.


## Resolution (2026-11-16)

Owner live check (1213)=ok on 2026-11-16 12:36 KST against daemon 7bdf11e74: composer autosize no longer forces layout on input; Edge PWA verified, Electron deferred by owner.

---
title: Composer autosize forces a full-document layout on every keystroke
related:
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: parent perf ticket; this splits out the measured composer typing cost
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

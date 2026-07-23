---
title: "dashboard terminal frontend: unmemoized O(N) pane scan re-runs on every PTY output chunk from any terminal"
related-mental-model:
  - ws-web-dashboard
---

## Symptom

Contributes to dashboard terminal lag that scales with the number of open
terminals. Surfaced as a secondary suspect during the investigation for
`260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation` (the
primary root cause there is a daemon-side blocking PTY write; this ticket
tracks a distinct frontend render-cost contributor found during the same
read-only sweep).

## Finding

Every PTY output chunk from ANY open terminal calls `setTerminalPanes` (via
`applyTerminalSocketMessage`/`markTerminalOutputCursor`, `App.tsx:5508-5524`),
which re-runs the whole `App` render function (~7600 lines). That render
includes an UNMEMOIZED `Object.values(terminalPanes).filter(...).map(...)`
scan over ALL panes at `App.tsx:4833-4848`, regardless of which single
terminal produced the output.

Cost scales as O(N terminals × aggregate output events/sec) — one terminal's
output tick forces a full pane-scan render, and the cost compounds with both
terminal count and output chatter (build logs, watch tasks, etc). Confidence:
medium.

## Fix direction (not decided)

- Memoize the `App.tsx:4833-4848` pane scan (e.g. `useMemo` keyed on the
  parts of `terminalPanes` that actually affect its output) so an output-only
  update to one pane doesn't force a full re-scan of all panes.
- Coalesce/batch `setTerminalPanes` output-cursor updates — e.g. rAF or
  microtask batching — instead of one state update per output chunk per
  terminal.
- Consider moving terminal-output-cursor state out of the giant `App` render
  entirely so output ticks don't re-render unrelated UI.

None of the above is decided; this ticket captures the finding for triage.

## Out of scope note

The daemon-side blocking-write root cause (the primary fix) is handled by
`260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation`. This
ticket is scoped to the frontend render-cost contributor only.

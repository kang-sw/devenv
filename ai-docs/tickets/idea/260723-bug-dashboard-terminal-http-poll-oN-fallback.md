---
title: "dashboard terminal frontend: per-pane HTTP short-poll fallback is O(N) when WebSockets drop"
related-mental-model:
  - ws-web-dashboard
---

## Symptom

If WebSockets drop (plausibly Windows security/EDR software interfering — see
the reframed hypothesis in the parent ticket), N open terminals each fall
back to independent HTTP short-polling instead of a shared WS stream. This is
an N-scaling cost that only manifests once WS health degrades. Surfaced as a
secondary, conditional suspect during the investigation for
`260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation`.

## Finding

`terminalOutputPollIntervalMs = 120` (`App.tsx:441`); the per-pane poll loop
at `App.tsx:4859-4916` issues one `fetchTerminalOutput` call per pane in
`livePollPanesRef.current`, i.e. one HTTP request per open terminal every
120ms while polling is active. This path is gated by
`shouldPollTerminalOutput` (`terminals.ts:616-623`), so it is idle under a
healthy WebSocket connection and only engages as a fallback.

Confidence: medium — the mechanism is real and reproducible by observing the
poll loop, but it is contingent on the WS connection actually dropping, which
was not independently confirmed during this sweep.

## Fix direction (not decided)

- Coalesce the N per-pane polls into a single batched request per tick
  (server-side endpoint accepting multiple terminal IDs, or a client-side
  request-merging layer).
- Back off the poll interval as N grows, rather than holding a fixed 120ms
  cadence per pane regardless of terminal count.
- Separately investigate WHY the WebSocket drops on Windows (security
  software interference is the leading hypothesis) — that is the real
  trigger that pushes terminals onto this fallback path in the first place,
  and fixing the trigger may make the fallback's scaling cost moot in
  practice.

None of the above is decided; this ticket captures the finding for triage.

## Out of scope note

The daemon-side blocking-write root cause (the primary fix) is handled by
`260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation`. This
ticket is scoped to the frontend HTTP-fallback scaling contributor only.

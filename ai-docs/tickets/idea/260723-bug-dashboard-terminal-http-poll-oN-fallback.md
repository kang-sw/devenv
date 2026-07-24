---
title: "dashboard terminal frontend: per-pane HTTP short-poll fallback is O(N) when WebSockets drop"
spec:
  - 260516-ws-web-dashboard-browser-terminal-emulator-behavior
related:
  260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation: parent
    investigation that surfaced this finding as a secondary, conditional
    suspect
related-mental-model:
  - ws-web-dashboard
---

# dashboard terminal frontend: per-pane HTTP short-poll fallback is O(N) when WebSockets drop

## Background

If a terminal's WebSocket drops (plausibly Windows security/EDR software
interfering — see the reframed hypothesis in the parent ticket), that pane
falls back to independent HTTP short-polling via `shouldPollTerminalOutput`
(`terminals.ts:616-623`) instead of a shared WS stream. Confirmed in the live
poll loop: `App.tsx:4859-4916`'s `poll()` iterates
`livePollPanesRef.current` and issues one `fetchTerminalOutput` call per pane
on every tick of `window.setInterval(poll, terminalOutputPollIntervalMs)`
(`terminalOutputPollIntervalMs = 120`, `App.tsx:441`, timer at
`App.tsx:4918`). With N panes on the fallback path simultaneously, this is N
independent HTTP requests every ~120ms — an O(N) request-rate cost that only
manifests once WS health degrades for multiple open terminals at once.

Separately, `appendTerminalOutput` (`terminals.ts:539-551`), the state
updater applied on every fallback poll response, concatenates unboundedly:
`pane.output + output.chunks.map((chunk) => chunk.data).join("")`, with no
length cap. The daemon already bounds its own retained output ring at
`MAX_OUTPUT_CHUNKS = 1024` (`crates/daemon/src/terminal.rs:33`, evicted
oldest-first at `terminal.rs:1001-1002`, surfaced to the client as a
`[terminal output gap: ...]` marker on truncation), but that only bounds what
the daemon retains for backfill — it does not bound what the frontend
accumulates in `pane.output` once received. A long-running terminal pane
under repeated fallback polling therefore keeps growing `pane.output` as a
JS string for the lifetime of the pane, independent of daemon-side retention.

Confidence: high for both findings — both are direct reads of the current
poll loop and state-update code, not conditional on reproducing a live WS
drop.

## Constraints

- The WebSocket-drop root cause (leading hypothesis: Windows security/EDR
  interference) is **not** confirmed and is **out of scope** for this
  ticket. This ticket only addresses the fallback path's own scaling
  behavior once a terminal is already polling; it does not attempt to
  prevent or reduce how often terminals end up on that path. A follow-up
  ticket should independently investigate the WS-drop trigger if it
  continues to reproduce.
- No batched multi-terminal output endpoint exists today. The current daemon
  route is single-ID only: `GET /api/dashboard/terminals/{terminalId}/output`
  (handler `terminal_output`, `crates/daemon/src/terminal.rs:655-667`,
  registered at `router.rs:358-359`), plus its server-scoped mirror `GET
  /api/dashboard/servers/{serverRoute}/terminals/{terminalId}/output`
  (`router.rs:260-261`, spec anchor
  `260525-ws-dashboard-remote-link-auth-handshake` /
  [Remote Terminal HTTP Lifecycle](../../spec/ws-web-dashboard/index.md#remote-terminal-http-lifecycle)).
  Adding a batched endpoint is new daemon work, not a client-only fix.

## Phases

### Phase 1: Batch fallback polls and bound frontend output growth

Chosen direction (of the candidates raised during triage): batch the N
per-pane fallback polls into a single request per tick, and independently
cap `pane.output` growth on the fallback path. Reject "back off the poll
interval as N grows" as the *primary* fix — it only trades request rate for
latency and does not fix the O(N) shape — but keep it as a secondary,
smaller guard (see below). Reject "fix the WS-drop root cause" as in-scope
here per the Constraints section above.

1. **New daemon endpoint: batched terminal output poll.**
   Add a multi-ID output route (both unscoped and server-scoped, mirroring
   the existing single-ID pair) that accepts a list of `{terminalId, after}`
   cursors in one request and returns a map of `terminalId ->
   TerminalOutputView` (reusing the existing `output_after`/
   `TerminalOutputView` shape per ID, `terminal.rs:895-912`). Unknown
   terminal IDs in the batch are reported per-ID (e.g. omitted from the
   response map, or an explicit not-found marker) rather than failing the
   whole batch, so one closed/unknown terminal does not block polling for
   the rest of the batch. Auth/work-root-availability gating stays
   per-terminal, matching the existing single-ID handler's
   `resolve_online_available_work_root` check.

2. **Frontend: single batched request per poll tick.**
   Replace the per-pane `fetchTerminalOutput` loop in `poll()`
   (`App.tsx:4859-4916`) with one batched call per tick carrying the cursor
   for every pane currently in `livePollPanesRef.current` (still respecting
   the existing `inFlight` per-terminal in-flight guard, now tracked as one
   in-flight batch rather than N). Apply the batched response as a single
   `setTerminalPanes` update, preserving the existing per-pane skip logic
   (`canApplyTerminalOutputPoll`, `terminalOutputPollChangedState`) so a
   quiet pane still does not trigger a re-render. This collapses the
   request count from O(N) to O(1) per tick regardless of how many panes
   are on the fallback path.

3. **Secondary guard: back off the tick interval as N grows.**
   Once request count is O(1) per tick, the remaining per-tick cost is
   response payload size, which still scales with N. Scale
   `terminalOutputPollIntervalMs` up (poll less often) once the number of
   fallback-polling panes exceeds a small threshold (e.g. keep the existing
   120ms base up to 8 concurrently-polling panes, then increase — exact
   curve and threshold are an implementation-time choice, not fixed by this
   ticket) so a large number of simultaneously-dropped-WS terminals cannot
   produce an unbounded per-tick payload even after batching.

4. **Cap `pane.output` growth on the fallback path.**
   In `appendTerminalOutput` (`terminals.ts:539-551`), trim the
   concatenated result to a bounded trailing length (e.g. keep only the
   last N characters, discarding from the front) instead of unbounded
   `pane.output + ...`. Use the same class of bound the daemon already
   applies to its own retention (`MAX_OUTPUT_CHUNKS`) as a sizing reference,
   translated to a frontend character/byte budget appropriate for a
   terminal pane's visible scrollback. `appendTerminalWebSocketMessage`
   (`terminals.ts:586-614`) has the same unbounded-concatenation shape on
   the live-socket path; leave it unchanged in this ticket (out of scope —
   this ticket is HTTP-fallback-path scoped per the original finding) but
   flag it as a same-shape follow-up if this trim proves worth generalizing.

Verification:

- Unit test: with N (>1) panes simultaneously satisfying
  `shouldPollTerminalOutput`, one `poll()` tick issues exactly one batched
  HTTP request (not N independent `fetchTerminalOutput` calls), and the
  batched response is applied to all N panes' state correctly (cursor
  advancement, quiet-pane skip, and error handling per pane).
- Unit test: `appendTerminalOutput` applied repeatedly with output exceeding
  the bound produces a `pane.output` that stays at or below the bound after
  each append, and retains the most recent output (trims from the front,
  not the back).
- Existing single-ID output-route tests continue to pass unchanged (batched
  endpoint is additive, not a replacement).

## Out of scope note

The daemon-side blocking-write root cause (the primary fix for the parent
investigation) is handled by
`260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation`
(already `.done/`). This ticket is scoped to the frontend HTTP-fallback
scaling contributor only. The WebSocket-drop trigger itself (Constraints
above) and the live-socket-path output-growth shape (Phase 1, step 4) are
explicitly deferred, not fixed here.

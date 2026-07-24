---
title: "dashboard terminal frontend: per-pane HTTP short-poll fallback is O(N) when WebSockets drop"
spec:
  - 260516-ws-web-dashboard-browser-terminal-emulator-behavior
  - 260703-ws-dashboard-server-route-scoped-operation-endpoints
related:
  260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation: parent
    investigation that surfaced this finding as a secondary, conditional
    suspect
related-mental-model:
  - ws-web-dashboard
sage-review-design: completed
sage-review-completeness: completed
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
  [Remote Terminal HTTP Lifecycle](../../spec/ws-web-dashboard/index.md#remote-terminal-http-lifecycle),
  a subsection of `260703-ws-dashboard-server-route-scoped-operation-endpoints`).
  Adding a batched endpoint is new daemon work, not a client-only fix, and it
  extends that same spec anchor (see Phase 1 step 1 for the chosen HTTP
  method and envelope).
- The single-ID route's cursor (`?after=`) is a query parameter because a
  single ID plus one cursor fits a `GET`. A multi-cursor batch request
  (`[{terminalId, after}, ...]`) needs a request body, so it cannot reuse the
  existing `GET .../output?after=` shape as-is — the batch endpoint's method
  and envelope are a Phase 1 design decision, not an assumed extension of the
  existing route (see Phase 1 step 1).

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
   Add `POST /api/dashboard/terminals/output/batch` (unscoped) and
   `POST /api/dashboard/servers/{serverRoute}/terminals/output/batch`
   (server-scoped), mirroring the existing single-ID pair's
   scoped/unscoped split. `POST` is chosen (an intentionally unRESTful read,
   noted as such) because a multi-cursor request cannot fit the existing
   `GET .../output?after=` query-parameter shape. Request body:
   `{"cursors": [{"terminalId": "<id>", "after": <u64>}, ...]}`. Response
   body: `{"results": {"<terminalId>": <TerminalOutputView>, ...}}`, reusing
   the existing `output_after`/`TerminalOutputView` shape per ID
   (`terminal.rs:895-912`). Unknown or inaccessible terminal IDs in the batch
   are simply omitted from the `results` map (not a batch-wide error and not
   a per-ID error object) so one closed/unknown terminal does not block
   polling for the rest of the batch and the frontend's existing per-pane
   "unknown terminal" handling stays unchanged for that ID. Auth/work-root-
   availability gating stays per-terminal, matching the existing single-ID
   handler's `resolve_online_available_work_root` check — a cursor for a
   terminal the caller cannot access is likewise omitted from `results`
   rather than failing the whole batch. This route extends the
   [Remote Terminal HTTP Lifecycle](../../spec/ws-web-dashboard/index.md#remote-terminal-http-lifecycle)
   spec anchor (`260703-ws-dashboard-server-route-scoped-operation-endpoints`),
   which documents the existing single-ID output route; update it to also
   document the batch route's method, path, and envelope.

2. **Frontend: single batched request per poll tick.**
   Replace the per-pane `fetchTerminalOutput` loop in `poll()`
   (`App.tsx:4859-4916`) with one `POST .../terminals/output/batch` call per
   tick carrying the `{terminalId, after}` cursor for every pane currently
   in `livePollPanesRef.current` (still respecting
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

4. **Cap `pane.output` growth on the fallback path, consumer-coherently.**
   In `appendTerminalOutput` (`terminals.ts:539-551`), trim the concatenated
   result to a bounded trailing length (e.g. keep only the last N
   characters, discarding from the front) instead of unbounded
   `pane.output + ...`. Use the same class of bound the daemon already
   applies to its own retention (`MAX_OUTPUT_CHUNKS`) as a sizing reference,
   translated to a frontend character/byte budget appropriate for a
   terminal pane's visible scrollback.

   Front-trimming `pane.output` is **not** a client-local change in
   isolation: `pane.output` has exactly one consumer, the delta-write effect
   in `terminalPaneBody.tsx:664-679`, and that effect assumes `pane.output`
   only ever grows at the tail or resets to a shorter prefix — it tracks
   `writtenLengthRef`, a raw `pane.output.length` already written to xterm,
   and compares it against the current `pane.output.length` to decide
   between a tail-delta write and a full `terminal.clear()` + redump.
   Front-trimming breaks that assumption: once the buffer is capped, a
   trim-then-append cycle can return the buffer to the same length it was
   before (new content pushed in, an equal amount trimmed off the front),
   so `pane.output.length === writtenLengthRef.current` even though new
   tail content arrived — neither branch fires and the new output is
   silently never written to the emulator. This is therefore an in-scope,
   required cross-file change alongside the trim itself, not an
   independent follow-up:

   - Add a cumulative trim counter to pane state (e.g. `outputTrimOffset:
     number`, starting at 0, incremented by the trimmed character count K on
     every front-trim in `appendTerminalOutput`). This tracks the absolute
     stream position of `pane.output[0]` — i.e. how many characters have
     ever been trimmed off the front over the pane's lifetime.
   - In `terminalPaneBody.tsx`, replace the raw-length `writtenLengthRef`
     comparison with an absolute-offset comparison. Track the absolute
     stream position already written (e.g. rename/repurpose the ref to hold
     `outputTrimOffset + writtenLength-at-last-write` instead of a bare
     length). On every `pane.output` change, compute the current absolute
     end position `currentEnd = pane.outputTrimOffset + pane.output.length`:
     - If `currentEnd > writtenAbsoluteRef.current`: new content exists.
       Compute the local slice start
       `localStart = Math.max(0, writtenAbsoluteRef.current - pane.outputTrimOffset)`
       (clamped to 0 so a previously-written position that has since been
       trimmed away never produces a negative index), write
       `pane.output.slice(localStart)` — calling `terminal.clear()` first
       only if `localStart` was clamped (the writer had fallen behind by
       more than one buffer's worth of trimming, so only the
       currently-retained window can be shown) — then set
       `writtenAbsoluteRef.current = currentEnd`.
     - If `currentEnd < writtenAbsoluteRef.current`: unchanged existing
       behavior (shorter buffer, e.g. reattach) — `terminal.clear()`, write
       the full `pane.output`, set `writtenAbsoluteRef.current = currentEnd`.
     - If equal: no-op, unchanged.
   - This keeps the fix confined to the delta-write effect's own bookkeeping
     (`writtenLengthRef` and its comparisons at `terminalPaneBody.tsx:671-677`)
     and the new `outputTrimOffset` field. The live-socket direct-write path
     (`terminalPaneBody.tsx:583-585`, which writes chunks straight to xterm
     and increments the same ref) is unaffected in the ordinary case because
     `appendTerminalWebSocketMessage` (`terminals.ts:586-614`) is untouched
     by this ticket and never trims — `pane.outputTrimOffset` only advances
     on the fallback path, so a pane's live-socket regime keeps the ref
     behaving exactly as a raw length (offset stays 0) unless that pane has
     previously spent time on the fallback path and accumulated a nonzero
     `outputTrimOffset`, in which case the same offset-aware formula above
     still applies correctly to the live-write increment site.
     `appendTerminalWebSocketMessage`'s own unbounded-concatenation growth
     stays out of scope, as before.

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
- Unit test (the load-bearing regression case for step 4): drive a pane
  through enough fallback-poll `appendTerminalOutput` calls to reach the
  cap and front-trim at least once, then continue appending further tail
  output past that point. Assert the delta-write effect's emulator writes
  cover every appended character exactly once across the whole sequence —
  no gap at the trim boundary (silent loss) and no spurious full-buffer
  `terminal.clear()` + redump triggered merely by a trim keeping
  `pane.output.length` flat. This is the direct regression test for the
  `writtenLengthRef` vs. `outputTrimOffset` interaction described above.
- Unit test: the step-3 adaptive poll interval backs off (increases) once
  the number of concurrently fallback-polling panes exceeds the chosen
  threshold, and recovers (returns to the 120ms base) once the count drops
  back at or below threshold — e.g. as panes reconnect to a live WebSocket
  or close. Assert both directions (back off under sustained high-N, and
  recover once N drops), not just the back-off direction alone.
- Existing single-ID output-route tests continue to pass unchanged (batched
  endpoint is additive, not a replacement).
- Daemon test: `POST .../terminals/output/batch` returns per-terminal
  results keyed by `terminalId` for a mixed batch of valid, unknown, and
  inaccessible IDs, omitting the latter two from `results` without failing
  the request for the valid IDs in the same batch.

## Out of scope note

The daemon-side blocking-write root cause (the primary fix for the parent
investigation) is handled by
`260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation`
(already `.done/`). This ticket is scoped to the frontend HTTP-fallback
scaling contributor only. The WebSocket-drop trigger itself (Constraints
above) and the live-socket-path output-growth shape (Phase 1, step 4) are
explicitly deferred, not fixed here.

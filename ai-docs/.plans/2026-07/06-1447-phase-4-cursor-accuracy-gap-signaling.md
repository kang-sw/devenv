# Plan: 260703-feat-dashboard-workroot-session-keepalive — Phase 4: Cursor accuracy and gap signaling for reconnect

## Relevant Ticket Contract

- Fix 1 (frontend): update `pane.nextSequence` directly from each `output`
  frame's own chunk sequence instead of waiting for a trailing `status` frame,
  to remove the race where a socket closed mid-batch leaves the cursor stale
  and causes duplicate output on resume.
- Fix 2 (backend): in `terminal_socket_task`/`send_output_backfill`, detect
  when a client-supplied `after` cursor is older than the oldest chunk
  currently retained in the ring buffer (already evicted) and include an
  explicit truncation signal (e.g. `truncated: true`) on the response instead
  of silently serving only the retained tail. Frontend renders this as a
  visible gap marker rather than stitching the stream silently.
- Non-goal: no buffer retention-policy change (chunk-count vs. byte-size vs.
  time-based) is in scope; flag as a follow-up candidate only if the gap
  signal fires often in practice.
- Spec Impact: Contract-first: no for this phase — internal lifecycle/protocol
  behavior change, no new browser-visible persisted contract, no spec file
  edit required.

## Out of Scope

- Phase 5/6 layout and terminal-visual-buffer persistence (separate phases,
  not needed for this fix).
- The HTTP `GET .../terminals/:id/output` route (`terminal_output`,
  `crates/daemon/src/terminal.rs#L383-394`) and `TerminalOutputView` JSON
  shape — the ticket names only `terminal_socket_task`/`send_output_backfill`
  for the truncation signal; the HTTP backfill/poll route stays untouched.
- Any change to `MAX_OUTPUT_CHUNKS` eviction policy itself.
- Ticket line numbers are stale (predate Phases 1-3 landing); this plan uses
  current, re-surveyed locations.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx#L4428-4446` — `applyTerminalSocketMessage`
  is the actual current site of the bug the ticket describes (moved from the
  ticket's stale `App.tsx:4194-4212`): it early-returns (no-op) on
  `message.type === "output"` and only calls
  `appendTerminalWebSocketMessage` for `status`/`exit` frames. This is called
  from the live socket's `message` listener.
- `ws-dashboard/frontend/src/App.tsx#L6528-6550` — the WebSocket `message`
  listener in `TerminalPaneBody`. For `output` frames it writes directly to
  the xterm instance via `terminalRef.current?.write(...)` and bumps
  `writtenLengthRef.current` by the chunk length *synchronously*, then always
  calls `liveRef.current.actions.onSocketMessage(pane, message)` (→
  `applyTerminalSocketMessage`) regardless of frame type. This direct-write
  path is why `output` frames were special-cased out of `setTerminalPanes` —
  reusing `appendTerminalWebSocketMessage`'s full output branch (which also
  appends to `pane.output`) would be safe re: double-write (the `pane.output`
  effect at L6578-6591 diffs against `writtenLengthRef`, already advanced),
  but would restore a `setTerminalPanes` re-render per output chunk and grow
  `pane.output` a second time for the same bytes. The ticket's fix only asks
  for `nextSequence` tracking, not restoring the full output-string path, so
  the plan updates only the numeric cursor field.
- `ws-dashboard/frontend/src/terminals.ts#L538-562` —
  `appendTerminalWebSocketMessage` already has fully correct, already-tested
  cursor logic for `output` frames
  (`nextSequence: Math.max(pane.nextSequence, message.chunk.sequence + 1)`),
  it is simply never invoked for that frame type today. Reuse the same
  formula in a new lightweight helper rather than inventing new math.
- `ws-dashboard/frontend/src/terminals.ts#L522-536` — existing pattern for
  small, no-op-when-unchanged pane mutators (`markTerminalSocketStatus`,
  `markTerminalPaneVisibilityGated`) to follow for a new
  `markTerminalOutputCursor`-style helper.
- `ws-dashboard/frontend/src/terminals.ts#L20-24,36-43` — wire types
  `TerminalOutputChunk` and `TerminalWebSocketServerMessage`. The `status`/
  `exit` variant needs a new `truncated` field to carry the backend signal.
- `ws-dashboard/frontend/src/terminals.test.ts#L264-310` — existing coverage
  pattern for `appendTerminalOutput`/`appendTerminalWebSocketMessage`
  (`withOutput`, `withSocketOutput`, `withSocketExit` fixtures); extend here
  for the new cursor helper and the truncated-gap-marker behavior.
- `ws-dashboard/crates/daemon/src/terminal.rs#L27` —
  `const MAX_OUTPUT_CHUNKS: usize = 1024;` ring buffer bound.
- `ws-dashboard/crates/daemon/src/terminal.rs#L205-214` —
  `TerminalSessionInner.output: VecDeque<TerminalOutputChunk>`; eviction is
  `pop_front()` in `append_output` (`#L632-651`), so `output.front()` is
  always the oldest retained chunk when non-empty.
- `ws-dashboard/crates/daemon/src/terminal.rs#L563-576` — `output_after`
  filters `chunk.sequence > after` under `self.inner.lock()`; this is the
  method to add a companion truncation check next to (or fold into), reusing
  the same lock acquisition to avoid a race against the reader thread's
  `append_output` evicting between two separate locks.
- `ws-dashboard/crates/daemon/src/terminal.rs#L251-273` —
  `TerminalWebSocketServerMessage` enum: `Status` and `Exit` variants both
  carry `status`/`next_sequence`; both need a new `truncated: bool` field
  since both are constructed from the same `send_terminal_socket_status`
  function.
- `ws-dashboard/crates/daemon/src/terminal.rs#L771-811` —
  `send_output_backfill` (loops chunks, mutates `*cursor`) and
  `send_terminal_socket_status` (builds the `Status`/`Exit` frame). The
  truncation check must use the cursor value **as requested at call entry**,
  before the backfill loop advances `*cursor` to the last sent chunk's
  sequence — otherwise the check always sees a caught-up cursor and never
  fires.
- Risk signal (resolved, not escalated): a naive `after < oldest_retained`
  check would false-positive on every **first-time** attach to a long-lived,
  busy terminal. Fresh `TerminalPaneState`s always start `nextSequence: 0`
  (`terminals.ts#L322`), so `terminalWebSocketCursor` sends `after=0` for a
  first-time view, not just for a genuine resume. If the daemon's terminal
  has already produced >1024 chunks by then, a naive check would report
  truncation even though the client never had that data to lose. Decision
  for this plan: only signal truncation when `after > 0` (i.e., the client is
  actually resuming from a previously-observed cursor) **and** the oldest
  retained chunk's sequence exceeds `after + 1` (there is a real gap between
  what the client last saw and what is now retained). `after == 0` always
  means "send me everything you have," never a gap.

## Implementation Plan

1. **Frontend cursor fix** — `ws-dashboard/frontend/src/terminals.ts`:
   add an exported helper near `markTerminalPaneVisibilityGated`
   (`#L530-536`), e.g.:
   ```ts
   export function markTerminalOutputCursor(
     pane: TerminalPaneState,
     chunkSequence: number,
   ): TerminalPaneState {
     const nextSequence = Math.max(pane.nextSequence, chunkSequence + 1);
     if (nextSequence === pane.nextSequence) return pane;
     return { ...pane, nextSequence, localCreatedAtMs: Date.now() };
   }
   ```
   Mirrors the existing cursor math already proven in
   `appendTerminalWebSocketMessage`'s output branch (`#L545-552`).

2. **Wire it in** — `ws-dashboard/frontend/src/App.tsx#L4428-4446`
   (`applyTerminalSocketMessage`): replace the `if (message.type === "output") { return; }`
   no-op with a `setTerminalPanes` update that calls
   `markTerminalOutputCursor(current[pane.logicalKey], message.chunk.sequence)`,
   keeping the existing `status`/`exit` branch (via
   `appendTerminalWebSocketMessage`) unchanged. Do not touch the direct
   `terminalRef.current?.write(...)` / `writtenLengthRef` path at
   `App.tsx#L6528-6550` — this fix only needs the cursor field, not a second
   copy of the output text into `pane.output`.

3. **Backend truncation type** — `ws-dashboard/crates/daemon/src/terminal.rs#L253-273`:
   add `truncated: bool` to both `TerminalWebSocketServerMessage::Status` and
   `::Exit` variants (`#[serde(rename_all = "camelCase")]` already applies at
   the enum level, so the field serializes as `truncated`).

4. **Backend truncation detection** — `ws-dashboard/crates/daemon/src/terminal.rs`:
   add a method on `TerminalSession` alongside `output_after` (`#L563-576`),
   e.g. `fn is_range_truncated(&self, after: u64) -> bool`, taking
   `self.inner.lock()` and returning
   `after > 0 && inner.output.front().is_some_and(|c| c.sequence > after + 1)`.
   Prefer computing this under the same lock acquisition as `output_after`
   (either inline both checks in one locked block, or accept two sequential
   locks if simpler — the eviction race window is negligible for a UI hint,
   but note the ordering constraint from finding 4 below either way).

5. **Wire detection into the socket path** —
   `ws-dashboard/crates/daemon/src/terminal.rs#L771-811`:
   - In `send_output_backfill` (`#L771-789`): capture
     `let requested_after = *cursor;` as the **first statement**, before the
     backfill loop mutates `*cursor`. Compute
     `let truncated = session.is_range_truncated(requested_after);` using
     that captured value. Pass `truncated` through to
     `send_terminal_socket_status`.
   - Update `send_terminal_socket_status` (`#L791-811`) to accept a
     `truncated: bool` parameter and set it on both the `Exit` and `Status`
     branches it constructs.

6. **Frontend wire type + gap marker** —
   `ws-dashboard/frontend/src/terminals.ts#L36-43`: add `truncated: boolean`
   to the `status`/`exit` member of `TerminalWebSocketServerMessage`. In
   `appendTerminalWebSocketMessage` (`#L538-562`, the non-output branch,
   `#L554-561`), when `message.truncated` is true, append a visible gap
   marker string to `pane.output` (e.g. a bracketed note such as
   `"\r\n[terminal output gap: some history was not retained]\r\n"`) in
   addition to the existing status/nextSequence update, so the existing
   xterm-write effect (`App.tsx#L6578-6591`, diffing `pane.output` against
   `writtenLengthRef`) renders it into the pane without new plumbing.

## Verification Plan

- `cd ws-dashboard && cargo test -p ws-dashboard-daemon` — add/extend a
  `crates/daemon/tests/routes.rs` websocket case (existing precedent at
  `#L985-1154`, `open_socket_request`/`connect_async` pattern) that opens a
  terminal socket with `after` set below the oldest retained chunk once
  `MAX_OUTPUT_CHUNKS` has been exceeded, and asserts the resulting `status`
  frame has `truncated: true`; add a companion case asserting `after=0`
  against a busy terminal (chunks already evicted) reports `truncated: false`.
- `cd ws-dashboard/frontend && npm run test:terminals` — extend
  `terminals.test.ts` (existing `withSocketOutput`/`withSocketExit` fixtures
  at `#L277-310`) with cases for `markTerminalOutputCursor` (advances only on
  a higher sequence, no-ops otherwise) and for the truncated gap marker being
  appended to `pane.output` when `appendTerminalWebSocketMessage` receives a
  `truncated: true` status/exit frame.
- `cd ws-dashboard/frontend && npm run build` (tsc -b + vite build) — confirm
  the new wire-type field compiles through both ends.
- Playwright e2e (`dashboard-acceptance.spec.ts`) is expected to remain
  non-runnable in this sandbox per Phases 1-3 Results (`libasound.so.2`
  missing, no Chromium binary) — fall back to the structural/unit verification
  above, consistent with prior phases.

## Escalations

- None.

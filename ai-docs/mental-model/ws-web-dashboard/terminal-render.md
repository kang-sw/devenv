---
domain: terminal-render
description: "Frontend terminal render-state batching: the rAF-deferred pending-cursor accumulator over per-chunk PTY output, and the flush-or-pending-read contract every pane teardown/persist site must honor to avoid a stale-cursor duplicate-output race."
sources:
  - ws-dashboard/frontend/src/
---

# Terminal Frontend Render (ws Web Dashboard)

Sub-domain of `ws-web-dashboard`; read `ws-web-dashboard/index.md` first — the
terminal pane/WebSocket wire contract, xterm emulator behavior, and workbench
placement stay documented there, and daemon-side helper-process/registry/
reconcile mechanics are `ws-web-dashboard/terminal.md`. This file covers only
the frontend render-state batching layer sitting between per-chunk PTY
output and committed `terminalPanes` React state.

## Entry Points

- `ws-dashboard/frontend/src/terminals.ts` — `createOutputCursorFlushScheduler`: the framework-agnostic `accumulate`/`flushNow`/`cancel`/`pendingNextSequenceFor` factory.
- `ws-dashboard/frontend/src/App.tsx` — constructs one scheduler instance lazily per `WorkbenchShell` (ref-if-null), wires the `"output"` WebSocket branch's `accumulate` call, every `flushPendingOutputCursorsNow()`/`pendingNextSequenceFor()` call site, and the revalidation effect's identity-signature memoization.

## Module Contracts

- A WebSocket `"output"` frame no longer calls `setTerminalPanes` synchronously. `applyTerminalSocketMessage` instead calls `scheduler.accumulate(logicalKey, chunkSequence)`, which coalesces same-key advances via `Math.max` and commits the whole pending `Map<logicalKey, maxChunkSequence>` at most once per animation frame (`applyBatch` -> `setTerminalPanes(current => flushPendingOutputCursors(current, pending))`). This bounds `App` re-render and the workbench revalidation effect to one commit per frame regardless of PTY output volume or open-terminal count.
- Because the commit is deferred to the next frame, `pane.nextSequence` on committed React state can be stale at any point between chunks. Every site that reads or persists a pane's cursor, or tears down/reattaches a pane, MUST either call `scheduler.flushNow()` first (synchronously commits the pending batch and cancels+nulls the scheduled frame) or read the pending-adjusted value through `scheduler.pendingNextSequenceFor(pane)`. Skipping this reintroduces the stale-cursor duplicate-output-on-resume race `markTerminalOutputCursor`'s own doc comment (`ws-web-dashboard/terminal.md`) describes — at the batching layer instead of the per-chunk layer.
- The enumerated flush/pending-read call sites — adding a new pane teardown or persist path requires adding one here too: the non-`"output"` socket branch (`flushPendingOutputCursorsNow()` before its own `appendTerminalWebSocketMessage` merge), `closeTerminalPane` (flush before the async `closeTerminal` call), the work-root-close effect (flush before `removeTerminalPanesForWorkRoot` drops panes that may reattach by the same `logicalKey` later), the debounced visual-capture read in `terminalPaneBody.tsx` (via `TerminalPaneActions.getPendingNextSequence`, since that effect reads off its own `liveRef` inside a `window.setTimeout` outside App's rAF scheduling — a same-tick flush-then-read ordering isn't reachable across that boundary), and the `WorkbenchShell` unmount-cleanup effect (`scheduler.cancel()`, no flush — a torn-down tree must not receive a late `setTerminalPanes`).
- The revalidation effect (`App.tsx`, the `openWorkRootKeys` effect around the pane-reconciliation logic) depends on memoized `terminalPaneIdentities`/`agentChatPaneIdentities` arrays — built from a sorted, `,`-joined, `|`-delimited per-pane signature string of `paneId`/`workRootId`/`serverRoute` — instead of raw `terminalPanes`/`agentChatPanes` state, so a cursor-only flush does not re-trigger the O(open roots × panes) rescan.

## Extension Points & Change Recipes

- **Add a new pane teardown/persist site that reads a cursor**: add it to the flush/pending-read enumeration above, and call `flushPendingOutputCursorsNow()` (or the scheduler's `pendingNextSequenceFor`/`TerminalPaneActions.getPendingNextSequence` if flushing App state isn't reachable from that call site) before reading — never read `pane.nextSequence` straight off committed state at a teardown/persist boundary.
- **Change the revalidation-effect identity signature**: keep a delimiter-safe leading token (`paneId` first) when composing the join string; a signature built without one risks a false-negative collision if a `workRootId`/`serverRoute` value happens to contain the field or item delimiter.

## Common Mistakes

- Reading `pane.nextSequence` (or persisting a cursor) from committed `terminalPanes` state at a teardown/reattach/persist site without first flushing or reading the pending-adjusted value — this silently reintroduces the duplicate-output race the batching layer must not regress.

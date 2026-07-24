---
title: "dashboard terminal frontend: unmemoized O(N) pane scan re-runs on every PTY output chunk from any terminal"
related:
  260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation: prior investigation found this as a secondary render-cost contributor (that ticket's daemon-side blocking-write root cause is already fixed, in `.done/`)
related-mental-model:
  - ws-web-dashboard
---

# dashboard terminal frontend: unmemoized O(N) pane scan re-runs on every PTY output chunk from any terminal

## Background

Confirmed as the TOP suspect for "the whole dashboard UI freezes during a
compile / heavy terminal output" during a 2026-07-24 end-to-end terminal-path
investigation (distinct from the daemon-side blocking-PTY-write root cause
already fixed by `260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation`).

Every PTY output chunk from ANY open terminal calls `setTerminalPanes` from
the `"output"` branch of `applyTerminalSocketMessage`
(`ws-dashboard/frontend/src/App.tsx:5508-5524`, via `markTerminalOutputCursor`
in `ws-dashboard/frontend/src/terminals.ts:577-583`). Each call produces a new
`terminalPanes` object identity, which re-runs the whole `App` render
function (~7600 lines) AND re-fires the workbench pane-revalidation
`useEffect` at `App.tsx:3881-4063`, because its dependency array
(`App.tsx:4049-4063`) includes raw `terminalPanes` (and `agentChatPanes`)
wholesale.

That effect, on every fire, loops over every currently open work root
(`openWorkRootKeys`) and for each one:

- rebuilds `Object.values(terminalPanes).filter(...).map(...)` and
  `Object.values(agentChatPanes).filter(...).map(...)`
  (`App.tsx:3898`, `3922`) to derive live pane ids,
- calls `revalidateWorkbenchLayoutForRoot` twice,
- compares results with `JSON.stringify(...) !== JSON.stringify(...)`
  (`App.tsx:4027`, `4043`).

None of this work depends on *which* pane's `nextSequence`/output cursor
advanced — it only needs pane *membership* (which pane ids exist for which
root/route) — but it re-runs in full on every single chunk from every
terminal, for every open root. Cost is O(output chunks/sec × open work
roots), all on the React main thread, and compounds directly with terminal
count and output chatter (build logs, watch tasks). A separate, cheaper
unmemoized scan for the same reason exists at `App.tsx:4833-4848`
(`livePollPanesRef.current = Object.values(terminalPanes).filter(...).map(...)`,
recomputed on every render with no `useMemo`).

## Spec Impact

Internal frontend render-performance fix; no change to the terminal output
delivery contract. `ai-docs/spec/ws-web-dashboard/index.md`
§`260516-ws-web-dashboard-terminal-registry-pty-spawn` and its WebSocket/PTY
output sections (ordering, transport, cursor/replay semantics) are
unaffected — this ticket only changes when/how often the existing React
state updates are committed and re-rendered, not what the terminal transport
delivers or how output is applied to pane state. No caller-visible behavior
change is intended.

## Phases

### Phase 1: batch output-cursor writes and stop the revalidation effect from keying on per-chunk cursor churn

Two changes, both required — the first alone still leaves the revalidation
effect firing once per animation frame per output-active terminal even when
other open terminals/roots are idle; the second alone still leaves one
`setTerminalPanes` call (and one `App` render) per PTY output chunk:

1. **Batch/coalesce the per-chunk `setTerminalPanes` write.** In
   `applyTerminalSocketMessage`'s `"output"` branch (`App.tsx:5508-5524`),
   stop calling `setTerminalPanes` synchronously for every `"output"`
   message. Instead, accumulate pending `(logicalKey -> chunkSequence)`
   advances in a `ref` (a `Map<string, number>`) and flush them into a
   single `setTerminalPanes` call — applying `markTerminalOutputCursor` for
   every pending logicalKey in one updater — on the next
   `requestAnimationFrame`. This bounds the state-commit rate for terminal
   output to at most one per frame, regardless of chunk volume or terminal
   count, and bounds `App` re-render frequency the same way.
   `markTerminalOutputCursor` (`terminals.ts:577-583`) itself is a pure
   function and needs no change; only its call site and cadence change.

2. **Stop the `App.tsx:3881-4063` revalidation effect from re-running on
   cursor-only `terminalPanes`/`agentChatPanes` changes.** Even with
   batching, one commit per frame still re-fires this effect on every frame
   that has output, which is wasteful since the effect body only needs pane
   *membership*, not per-pane cursor state. Derive a small
   `useMemo`-computed "live pane identity signature" for each of
   `terminalPanes` and `agentChatPanes` — e.g. a stable string key built
   from the sorted list of `{paneId, workRootId, serverRoute}` per pane —
   and use that memoized value (not the raw `terminalPanes`/`agentChatPanes`
   objects) as the effect's dependency, replacing `terminalPanes` /
   `agentChatPanes` at `App.tsx:4050` / `4054`, and as the source the effect
   reads to build `liveTerminalPaneIds` / `liveAgentChatPaneIds`
   (`App.tsx:3897-3905`, `3921-3929`). Because the `useMemo` dependency is
   the identity signature, an output-cursor-only update (same pane ids,
   different `nextSequence`) produces the same memoized value and the effect
   does not re-run.

Leave the `App.tsx:4833-4848` `livePollPanesRef` scan alone for this phase —
it is a plain per-render assignment (no `JSON.stringify`, no cross-root
loop) and is already bounded by the render-rate cap from change (1);
revisit only if the Verification measurement below shows it still
contributes measurably.

**Verification:**

- Unit test (in `ws-dashboard/frontend/src/terminals.test.ts`, alongside the
  extracted batching helper): given several pending `(logicalKey,
  chunkSequence)` pairs, including duplicates and out-of-order sequences for
  the same key, one flush produces exactly one `terminalPanes` patch with
  each pane's cursor advanced to the max sequence seen — proving the batched
  updater is equivalent to N sequential `markTerminalOutputCursor` calls
  with fewer commits.
- Behavior/perf measurement (manual or e2e; `App.tsx` has no existing
  component-level unit harness — see `ws-dashboard/frontend/e2e/`): with 2+
  terminals open across 2+ work roots, drive a burst of output chunks (e.g.
  a build/watch command) on one terminal only, and confirm via a temporary
  render/effect-fire counter (or React DevTools Profiler) that:
  - the `App.tsx:3881-4063` effect fires at most once per animation frame
    during the burst (not once per output chunk), and
  - its fire count does not scale with the number of *other* open
    terminals/roots that received no output during the burst.

## Out of Scope

The daemon-side blocking-PTY-write root cause (the primary fix for
dashboard-freezes-during-heavy-output) was already handled by
`260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation`
(`.done/`). This ticket is scoped to the frontend render-cost contributor
only.

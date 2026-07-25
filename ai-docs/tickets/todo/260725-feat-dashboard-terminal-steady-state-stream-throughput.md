---
title: Steady-state open-terminal streaming throughput hardening
sage-review-design: completed
related:
  260723-bug-dashboard-terminal-frontend-output-oN-rerender: prior steady-state batch this extends
  260724-bug-dashboard-terminal-utf8-multibyte-read-boundary-corruption: output-fidelity behavior all phases must preserve
related-mental-model:
  - ws-web-dashboard/terminal
  - ws-web-dashboard/terminal-render
sage-review-completeness: completed
---

# Steady-state open-terminal streaming throughput hardening

## Background

The `260723-bug-dashboard-terminal-*` batch hardened steady-state terminal
plumbing (rAF-batched `setTerminalPanes`, O(1) daemon `output_after`, PTY writes
off the Tokio worker, batched HTTP-poll fallback). Four per-output-chunk costs
on the **open, actively-streaming** path remain — most visible when a
full-screen TUI (e.g. Claude Code TUI, vim, htop) repaints many frames/sec.

This ticket is deliberately scoped to **open-state throughput only**. It is the
result of rejecting a retention/replay-on-open redesign, which would have
collided with two architecture invariants: the gapless-contiguous output
sequence invariant that the daemon's O(1) `output_after` depends on
(`terminal.md:35`), and xterm.js owning its own scrollback buffer. Every phase
here is **behavior-preserving throughput work** that touches neither retention
semantics nor the replay-on-open contract.

Investigation (2026-07-25) confirmed the frontend `pane.output` full-string
concat/`.slice()` is NOT on the live-socket hot path — it is already bypassed
for connected panes by the 260723 rAF cursor-accumulator, and only runs on the
disconnected/HTTP-poll fallback (already batched). It is therefore excluded.

## Constraints

- Behavior-preserving: no change to retention semantics, the gapless-contiguous
  sequence invariant (`terminal.md:35`), or the replay-on-open contract.
- Output fidelity must be preserved, including the `260724` UTF-8 read-boundary
  carry behavior — no phase may reorder, drop, or re-split output chunks.
- Verification standard per phase: a before/after throughput observation on a
  fast full-screen TUI repaint plus a no-regression check on output fidelity.
- Phases are independent and may land in any order; Phase 1 is the highest
  leverage and is being shipped ahead of the others as a hotfix.

## Phases

### Phase 1: xterm WebGL renderer with canvas/DOM fallback

`terminalPaneBody.tsx:166-183` initializes `new Terminal(...)` with only
`FitAddon` + `SerializeAddon`; no GPU renderer addon is loaded, so xterm 5.x
falls back to its DOM renderer, which re-touches DOM row nodes per repaint and
dominates full-screen-TUI throughput. `package.json` pins no
`@xterm/addon-webgl`/`@xterm/addon-canvas`.

Load `WebglAddon` (after `terminal.open()`), with a `webglcontextlost` handler
that disposes it and falls back to `CanvasAddon`, then plain DOM, so the
terminal never blanks on GPU context loss; guard construction in try/catch for
WebGL-less environments; dispose in the existing unmount cleanup. Render backend
only — no data-path change.

Verification: TUI repaint throughput improves; no visual regression; context-loss
falls back gracefully.

### Phase 2: Port the daemon O(1) output_after fix to the helper ring

The helper process's own ring still linear-scans on every wake:
`RingState::backfill_after` (`terminal_helper_process.rs:62-68`) and the inlined
filter in the `notify` arm (`terminal_helper_process.rs:362-376`) both do
`ring.output.iter().filter(|c| c.sequence > after)` over the full ring
(`MAX_OUTPUT_CHUNKS = 1024`) — the exact pattern already replaced by
index-arithmetic on the daemon side (`terminal.rs` `output_after`). Port the
same index-arithmetic skip, preserving the gapless-contiguous invariant the
shortcut relies on.

Verification: unit test alongside the existing ring tests; scan replaced by index
math; behavior identical.

### Phase 3: Batched Output frame (array of chunks) across both hops

Both IPC hops emit one frame + one serialize per chunk even though the fetch is
already batched: daemon->browser `send_output_backfill` loops one
`serde_json::to_string` + one `Message::Text` per chunk (`terminal.rs:1239-1256`,
single-chunk `TerminalWebSocketServerMessage::Output`), and the helper's
`notify` arm writes one NDJSON `HelperToDaemonMessage::Output` per chunk. A fast
TUI repaint spans several 4096-byte read chunks, so N frames fire per wake.

Add a batched Output variant carrying an array of chunks and coalesce each send
loop into one frame per wake, preserving chunk order (gapless). Additive wire
change — keep or version the single-chunk variant so mixed old/new ends do not
break.

Verification: fewer frames under high chunk rate; frontend applies the batched
array in order; no reordering or gaps.

#### Spec Impact

- Target spec area: `ws-web-dashboard/index.md` terminal-io-transport anchor
  (`260516-ws-web-dashboard-terminal-io-transport`), the terminal output message
  contract.
- Expected caller-visible change: the WS/IPC output message gains a batched
  form (array of chunks) alongside or replacing the single-chunk form.
- Contract-first spec: no — the exact batched-frame shape and back-compat
  strategy will be settled during Phase 3 implementation; spec text is updated at
  implementation close, not authored ahead.

### Phase 4: Debounce refocusActiveTerminal off the per-chunk path

`refocusActiveTerminal` (`terminalPaneBody.tsx:145-154`) schedules a fresh
`setTimeout(0)` doing a DOM `querySelector` + two `.focus()` calls on **every**
output message for the active pane (`terminalPaneBody.tsx:623-625`), with no
debounce — flooding the event loop under fast output while the adjacent cursor
accumulator is already rAF-batched. Fold the refocus into the same rAF/debounce
mechanism.

Verification: focus still restored on the active pane; per-chunk timer/DOM churn
eliminated.

---
title: Server-side terminal screen emulation for TUI transmission reduction (mosh-style)
related:
  260725-feat-dashboard-terminal-steady-state-stream-throughput: raw-byte-forwarding layer this sits above; complement, do not re-litigate
  260725-research-ws-dashboard-pty-agent-pivot: pivot that makes repainting agent-CLI TUIs the terminal's primary workload but never addresses transmission perf
  260725-bug-dashboard-terminal-utf8-residual-multibyte-corruption: a server-side parser faces the same multibyte/ANSI boundary hazard
  260426-feat-claude-dash: in-repo prior art using termwiz for client-side VT screen state
related-mental-model:
  - ws-web-dashboard/terminal
  - ws-web-dashboard/terminal-render
---

# Server-side terminal screen emulation for TUI transmission reduction (mosh-style)

## Background

The dashboard forwards the raw PTY byte stream verbatim to xterm.js (a "dumb
pipe"). For a full-screen TUI (Claude Code TUI, vim, htop, spinners, progress
bars) that repaints in place many frames/sec, every redundant repaint frame is
transmitted in full. This is the dominant terminal cost and it is about to get
worse: the PTY-agent pivot (`260725-research-ws-dashboard-pty-agent-pivot`)
makes an agent CLI — itself a heavily-repainting TUI — the terminal's primary
workload, yet that pivot treats the raw-byte substrate as a reused black box and
never raises transmission cost (a gap this ticket records).

Trigger context: on a corporate host where a DLP inspection hook fires on
(nearly) every transmission, per-terminal lag correlates exactly with whether
that terminal is running an active repainting TUI. Idle terminals are fine; a
spinner-driven TUI is unusable. So the bottleneck is transmission cost
(count and/or bytes), not CPU.

Question: can we cut TUI transmission cost by emulating terminal screen state
server-side and transmitting throttled screen snapshots / cell diffs instead of
the raw byte stream — and if so, how far is worth going?

## Core insight: novel content vs overdraw

Two output regimes must be separated:

- **Novel content** (`cat biglog`, scrolling build output): every byte is new
  information; nothing is droppable. Only transmission *batching* helps here.
- **Overdraw** (alt-screen TUI, spinner, progress bar): bytes pour out but
  mostly overwrite the same bounded grid; intermediate frames are never
  perceived. This is where reduction is possible.

alt-screen (`ESC[?1049h`) is the marker of overdraw: no scrollback, the visible
state is bounded by rows×cols, repaints overwrite. It is therefore
time-collapsible — only the latest frame matters.

**Hard constraint that forces emulation:** a raw VT byte stream is a *stateful
command stream*, not independent snapshots. Dropping a middle chunk loses cursor
moves / SGR state and corrupts the screen. So exploiting repaint redundancy
requires tracking screen state — i.e. a (bounded) emulator. There is no safe
"recognize alt-screen and throttle raw bytes" shortcut. "alt-screen
optimization" and "server-side emulation" are the same mechanism; alt-screen
scoping is just the smaller, tractable half (bounded grid, no scrollback).

## Option ladder

- **L0 — time-window transmission coalescing (no emulator).** Concatenate all
  bytes that arrived in a fixed window (~16–50 ms) into one frame. Safe (bytes
  are appended, never dropped). Cuts transmission *count* hard (the DLP
  per-transmission win) but not bytes. Generalizes Phase 3 of the
  stream-throughput ticket (which coalesces per-wake) to a fixed cadence.
- **L1 — alt-screen-scoped bounded emulator.** Run a VT emulator only while in
  alt-screen; maintain the rows×cols grid (no scrollback to model); on a
  throttled tick emit the latest grid as a repaint or cell diff. Normal
  (scrollback) mode stays lossless raw passthrough. Cuts count *and* bytes for
  TUIs. This is the tractable first real slice of the twin strategy.
- **L2 — full twin / mosh model.** A server-side emulator becomes authoritative
  for all screen state and transmits cell diffs everywhere; scrollback splits
  into a separate append stream; the client applies diffs (via xterm buffer API
  or a grid renderer). Biggest change; deferred.

## Which lever depends on DLP cost shape (open)

If the DLP hook is a fixed per-transmission overhead → L0 alone may suffice
(collapse count). If it also scans bytes (cost scales with payload) → L1/L2
byte reduction is needed. "Hook on nearly every transmission" suggests
per-transmission cost dominates, so L0 is likely the immediate high-ROI move;
confirm the cost shape before committing to L1+.

## Crate options for L1/L2

- `termwiz` — wezterm's VT emulator; in-repo prior art in `260426-feat-claude-dash`
  (client-side VT screen state for a ratatui multiplexer), so team familiarity
  exists. Preferred starting candidate for divergence-risk reasons.
- `vt100` — lightweight Screen with built-in diff.
- `alacritty_terminal` / `vte` — heavier / lower-level.

## Constraints to reconcile (avoid re-litigating settled work)

- Gapless-contiguous output sequence invariant that the daemon's O(1)
  `output_after` depends on (`terminal.md:35`) and the replay-on-open contract:
  a screen-diff transport changes what "output" and "backfill" mean.
- xterm.js currently owns authoritative scrollback / rendered state (spec anchor
  `#260516-ws-web-dashboard-browser-terminal-emulator-behavior`); L2 moves that
  authority server-side.
- The completed/queued phases of
  `260725-feat-dashboard-terminal-steady-state-stream-throughput` (WebGL
  renderer, helper-ring O(1), batched frame, refocus debounce) — complement, do
  not duplicate; L0 is adjacent to its Phase 3.
- A server-side parser inherits the same multibyte/ANSI read-boundary hazard as
  `260725-bug-dashboard-terminal-utf8-residual-multibyte-corruption`.

## Twin-emulator divergence risk

xterm.js is already an emulator. Any server-side emulator whose cell-grid
interpretation diverges from xterm's causes render corruption. The escape-
sequence surface (scroll regions, alt-screen, wide/combining chars, DEC modes,
SGR) is large; L2 must define how the two agree (or how the client stops using
xterm's parser and applies cells directly).

## Open questions

- DLP cost shape: per-transmission fixed vs byte-proportional (gates L0-vs-L1).
- Throttle cadence and whether to send full-grid-repaint or cell-diff per tick.
- Client apply path: xterm buffer API vs a dedicated grid renderer.
- How scrollback history coexists with a diff-synced visible grid (append stream
  split).
- Whether L1 alone (alt-screen only) captures most of the real-world win, given
  agent-CLI TUIs run predominantly in alt-screen.

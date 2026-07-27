---
title: "Terminal refocus steals IME composition focus, corrupting Korean input"
related:
  260725-feat-dashboard-terminal-steady-state-stream-throughput: shares the refocusActiveTerminal call site; that ticket's Phase 4 debounce work must compose with this fix rather than reintroduce an unguarded refocus
related-mental-model:
  - ws-web-dashboard/terminal-render
sage-review-design: required
---

# Terminal refocus steals IME composition focus, corrupting Korean input

## Background

Dogfooding on Windows surfaced intermittent Korean IME corruption while typing
into an open terminal pane: composed text loses its leading character(s) (e.g.
"안녕하세요" arrives as "녕하세요" or "녕 세요"), the corruption is
deterministic for a given retyped string/timing (not random), and it stops
happening after navigating to a different work root and back (masks the race,
does not fix it).

Root cause, confirmed from source (`ws-dashboard/frontend/src/terminalPaneBody.tsx`):

- `refocusActiveTerminal()` (~160-169) schedules a `setTimeout(..., 0)` that
  calls `terminalRef.current?.focus()` (= xterm's `Terminal.focus()`, which is
  just `this.textarea.focus({preventScroll:true})`) and separately re-queries
  and re-focuses the same `.xterm-helper-textarea` element.
- It fires unconditionally, with no IME-composition guard, from two sites:
  - `sendInputBytes` (~306, ~310) — every time xterm's `onData` forwards input
    to the daemon (i.e. on every keystroke/composed-character send).
  - the WebSocket `"output"` message handler (~699) — every PTY output chunk
    for the active pane.
- The codebase already tracks composition state (`composingInput`, set/cleared
  by `compositionstart`/`compositionend` listeners at ~314-320/~338-339) and
  already gates a different handler on it (`keydownFallback`, ~356: `if
  (event.isComposing || event.key === "Process" || composingInput) return;`)
  — but neither guard is wired into `refocusActiveTerminal` or its two call
  sites above.
- Cross-checked against xterm.js's own composition finalization logic
  (`node_modules/@xterm/xterm/src/browser/input/CompositionHelper.ts`,
  `_finalizeComposition`): it slices the composed string out of the live
  textarea value using position bookmarks set at `compositionstart`/read back
  after its own `setTimeout(..., 0)`, and depends on the textarea not being
  redundantly refocused mid-composition. `refocusActiveTerminal`'s
  `setTimeout(..., 0)` calls race in the same macrotask queue as xterm's own
  composition-finalization timers; a refocus landing near the start of a new
  composition is consistent with the observed "leading character(s) dropped"
  pattern and its retype-deterministic (race, not random-loss) behavior.
- Not a transport/WebSocket issue: `sendInputBytes` sends one atomic
  `socket.send(JSON.stringify({ type: "input", data }))` per `onData` event
  with no client-side byte splitting: whatever gets sent is already whatever
  `CompositionHelper` computed as `data`, so the loss happens before the send,
  at the DOM/IME layer.

This regression was anticipated but never ticketed: `260725-feat-dashboard-terminal-steady-state-stream-throughput`'s
Phase 1 Result (2026-07-25) has a "Forward" note flagging that
`refocusActiveTerminal` churn is an IME-composition-corruption regression and
that "Phase 4 must couple the debounce with an IME/composition guard, or that
fix lands in the dedicated bug ticket tracking the refocus-vs-IME
interaction" — but only names the output-chunk call site (~699). This ticket
also covers the input-send call sites (~306/~310), found during this
investigation.

## Constraints

- Must not regress existing non-IME focus-restore behavior: when composition
  is NOT active, typing/output must still restore focus to the terminal
  exactly as today.
- Must compose with `260725` Phase 4 (debounce `refocusActiveTerminal` off the
  per-chunk output path) rather than conflict with it — whichever lands first,
  the other must build on it, not reintroduce an unguarded call.
- No caller-visible wire/contract change; this is a client-local focus-timing
  fix only.

## Phases

### Phase 1: Gate refocusActiveTerminal on active IME composition

Make `refocusActiveTerminal` (or its call sites) a no-op while IME composition
is active, using the existing `composingInput` tracking (and/or
`event.isComposing`) already present in this component — extend it to cover
both unguarded call sites (`sendInputBytes` ~306/~310, and the WS `"output"`
handler ~699), not just `keydownFallback`. Exact wiring (guard inside the
function vs. at each call site) is an implementation choice; preserve the
non-IME focus-restore behavior unchanged.

Verification: manual dogfood repro on Windows — type Korean text via IME into
an active terminal pane while output is streaming (e.g. during shell echo or a
running command), confirm no dropped/corrupted leading characters across
repeated attempts. No automated composition-simulation coverage exists in this
repo's test suite (jsdom does not simulate real OS/browser IME composition
timing), so this phase's verification boundary is manual/dogfood-based, not a
new unit test.

## Spec Impact

No spec change: this restores an unintended focus-stealing side effect to the
documented non-stealing behavior; the terminal WS/IPC wire contract and
retention/replay semantics are untouched. Contract-first spec: no.

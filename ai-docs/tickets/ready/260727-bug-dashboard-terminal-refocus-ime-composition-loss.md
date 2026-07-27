---
title: "Terminal refocus steals IME composition focus, corrupting Korean input"
related:
  260725-feat-dashboard-terminal-steady-state-stream-throughput: shares the refocusActiveTerminal call site; that ticket's Phase 4 debounce work must compose with this fix rather than reintroduce an unguarded refocus
related-mental-model:
  - ws-web-dashboard/terminal-render
sage-review-design: completed
sage-review-completeness: completed
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
- It fires unconditionally, with no IME-composition guard, from **three**
  sites across **two different effects**:
  - Mount effect (`useEffect` at line 175, cleanup at 581) — `sendInputBytes`
    (~306, ~310, every input send) and the 100ms `focusWatchdog`
    (`setInterval` at ~523-539, call at ~538). The watchdog partially
    self-guards via `if (container.contains(document.activeElement)) return;`,
    but that check does not know about composition state either — it only
    happens to usually no-op during composition because the IME textarea
    normally stays `document.activeElement` and inside `container`; if focus
    genuinely left the container mid-composition it would refocus
    unconditionally on its next 100ms tick, same bug at lower frequency.
  - Separate socket effect (`useEffect` at line 646, cleanup at 745) — the
    WebSocket `"output"` message handler (~699), every PTY output chunk for
    the active pane.
- The codebase already tracks composition state (`composingInput`, a `let`
  local **inside the mount effect**, set/cleared by `compositionstart`/
  `compositionend` listeners at ~314-320/~338-339) and already gates a
  different handler on it (`keydownFallback`, ~356: `if (event.isComposing ||
  event.key === "Process" || composingInput) return;`) — but neither guard is
  wired into `refocusActiveTerminal` or any of its three call sites above.
  Because `composingInput` is effect-local, it is not usable as-is for a
  single guard: `refocusActiveTerminal` is defined at the component-body level
  (~160, outside every effect) and one of its call sites (~699) lives in an
  *entirely separate* effect that never sees the mount effect's local
  variable — only a component-level ref (the same pattern already used for
  `keepTerminalFocusRef` at line 143, set by `markFocusedTerminal`/
  `clearFocusedTerminal` in the mount effect and read by both effects) can
  bridge composition state across this boundary.
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
also covers the input-send and focus-watchdog call sites (~306/~310, ~538),
found during this investigation.

Evaluated and rejected: deleting the per-keystroke/per-chunk refocus calls
(~306/~310/~699) entirely and relying solely on the 100ms `focusWatchdog` for
restoration. Rejected for this ticket because the watchdog's up to 100ms
restore latency would be an observable behavior change for the non-IME case,
which the Constraints below require to stay exactly as today; consolidating
onto the watchdog alone is a separate throughput/simplification move left to
`260725` Phase 4, not this bug fix.

## Constraints

- Must not regress existing non-IME focus-restore behavior: when composition
  is NOT active, typing/output must still restore focus to the terminal
  exactly as today (same call sites, same latency).
- Must compose with `260725` Phase 4 (debounce `refocusActiveTerminal` off the
  per-chunk output path) rather than conflict with it. This ticket lands
  first: it fixes a live, user-reported input-corruption bug and `260725` is a
  throughput-hardening ticket already sitting in `ready/` with Phase 4 not yet
  implemented — Phase 4 must build its debounce on top of this ticket's
  composition guard, not the reverse.
- No caller-visible wire/contract change; this is a client-local focus-timing
  fix only.

## Phases

### Phase 1: Gate refocusActiveTerminal on active IME composition

Promote composition state to a component-level ref (e.g. `composingRef =
useRef(false)`, set/cleared by the existing `compositionstart`/
`compositionend` listeners at ~338-339 instead of today's effect-local `let
composingInput`) — this is required, not a style choice: `refocusActiveTerminal`
is defined outside every effect (~160) and one call site (~699) lives in the
separate socket effect, so a plain local variable cannot reach all three call
sites. Gate `refocusActiveTerminal` itself (single choke point) on
`composingRef.current`, covering all three call sites (`sendInputBytes`
~306/~310, `focusWatchdog` ~538, WS `"output"` handler ~699) through the one
guard. Update `keydownFallback` (~356) to read the same ref instead of the
local `composingInput` it uses today, so there is one source of truth.

Trailing edge: on `compositionend`, invoke `refocusActiveTerminal()` once
(after clearing `composingRef`) so focus is restored immediately when
composition finishes, rather than waiting for the next output chunk or the
next `focusWatchdog` tick (up to 100ms later) to notice.

Preserve the non-IME focus-restore behavior unchanged (same call sites, same
`setTimeout(...,0)` timing) when composition is not active.

Verification:
- Unit test: extract the guard as a DOM-free predicate (e.g.
  `shouldRefocusTerminal({ composing, keepFocus, visible, active })`),
  following the existing `keydownSuppression.ts`/`keydownSuppression.test.ts`
  pattern in this same directory (predicate extracted from DOM state, tested
  in the repo's node/tsc harness without jsdom). This pins the guard behavior
  so `260725` Phase 4's rework of this function cannot silently drop it.
- Manual dogfood repro on Windows: type Korean text via IME into an active
  terminal pane while output is streaming (e.g. during shell echo or a running
  command), confirm no dropped/corrupted leading characters across repeated
  attempts, and confirm focus is still restored promptly after composition
  ends. The end-to-end IME race itself stays manual/dogfood-verified — jsdom
  does not simulate real OS/browser IME composition timing — but the guard
  logic itself is no longer untested.

### Result (cc33309b) - 2026-07-27

Implemented as planned: `composingRef = useRef(false)` promoted to
component-level (`terminalPaneBody.tsx`), `refocusActiveTerminal` rewritten to
build a `RefocusGuardState` at `setTimeout(0)` fire time and delegate to the
new `shouldRefocusTerminal` predicate (`terminalRefocusGuard.ts`), covering
all three call sites (`sendInputBytes`, `focusWatchdog`, WS `"output"`
handler) through that one choke point; `clearComposing` now fires a
trailing-edge `refocusActiveTerminal()` after clearing the ref;
`keydownFallback` reads `composingRef.current` instead of the old effect-local
variable. `terminalRefocusGuard.test.ts` added alongside the predicate,
following the `keydownSuppression.ts` pattern.

Partitioned review (fit/test: clean; correctness: non-clean, 1 important + 2
minor) caught a real regression risk in the first pass: `composingRef` had no
reset path outside `compositionend`, so a dropped `compositionend` (pane
hidden/re-parented, alt-tab) would latch composing state true forever and
permanently disable both the watchdog and `keydownFallback` for that pane.
Fixed in a follow-up commit: `focusWatchdog` now clears `composingRef` when
focus has genuinely left the container, restoring its unconditional-safety-net
property; also restored a cheap composing/keepFocus short-circuit ahead of the
layout-triggering `offsetParent` read (perf, no forced reflow per output
chunk on an inactive pane). One narrow minor finding (the `active` gate
folded into `sendInputBytes` is a practical no-op) was accepted without a code
change. Re-review confirmed clean with 2 minor remaining (no new
critical/important findings).

Deviation from `## Spec Impact` below: doc-pre-pass's mental-model-updater
flagged that `{#260517-ws-dashboard-terminal-ime-and-line-editing-fidelity}`
listed focus-stability cases without naming in-progress IME composition, even
though this fix exists specifically to preserve it during composition. Judged
as a small clarity edit rather than a new-behavior entry (the underlying
behavior is not new) and applied directly — see
`ai-docs/spec/ws-web-dashboard/index.md`.

Verification: `npm run test:terminal-refocus` (new predicate unit test),
`npm run build` (type-checks the `.tsx` edit), and
`npm run test:keydown-suppression` (neighboring pattern regression check) all
passed at each checkpoint. The end-to-end IME race itself stays
manual/dogfood-verified per the ticket — not yet exercised on the Windows
dogfood clone as of this Result (pending a frontend-only redeploy).

## Spec Impact

No spec change: this restores an unintended focus-stealing side effect to the
documented non-stealing behavior; the terminal WS/IPC wire contract and
retention/replay semantics are untouched. Contract-first spec: no.

(Superseded in part by the Result above: a small clarifying edit to
`{#260517-ws-dashboard-terminal-ime-and-line-editing-fidelity}` was made
during doc-pre-pass — not a new-behavior spec entry, but this section's "no
spec change" framing did not anticipate even a clarity-only touch.)

# Plan: Terminal refocus steals IME composition focus, corrupting Korean input — Phase 1: Gate refocusActiveTerminal on active IME composition

## Relevant Ticket Contract

- Promote composition state to a component-level `composingRef = useRef(false)`
  (currently an effect-local `let composingInput`), set/cleared by the existing
  `compositionstart`/`compositionend` listeners, because `refocusActiveTerminal`
  is defined outside every effect and one call site lives in a separate effect
  that cannot see an effect-local variable.
- Gate `refocusActiveTerminal` itself as the single choke point so all three
  call sites (`sendInputBytes` ~306/~310, `focusWatchdog` ~538, WS `"output"`
  handler ~699) are covered by one guard.
- `keydownFallback` (~356) must read the same ref instead of its local
  `composingInput`.
- Trailing edge: on `compositionend`, invoke `refocusActiveTerminal()` once
  (after clearing `composingRef`) so focus restores immediately when
  composition finishes.
- Non-IME behavior must stay byte-for-byte identical: same call sites, same
  `setTimeout(...,0)` timing, when composition is not active.
- Verification: extract the guard as a DOM-free predicate, e.g.
  `shouldRefocusTerminal({ composing, keepFocus, visible, active })`, following
  the existing `keydownSuppression.ts`/`keydownSuppression.test.ts` pattern
  (predicate module + assertEqual-style test, no jsdom) in the same directory.
- No wire/contract change; client-local focus-timing fix only.

## Out of Scope

- `260725` Phase 4's debounce of the WS-output refocus call — this ticket
  lands first; Phase 4 builds on top of this guard, not the reverse.
- Deleting the per-keystroke/per-chunk refocus calls and relying solely on the
  100ms watchdog — evaluated and rejected in the ticket's Background.
- Automated end-to-end IME composition simulation — the ticket explicitly
  keeps the real race manual/dogfood-verified (jsdom cannot simulate real
  IME timing); only the extracted predicate gets a unit test.
- Removing the existing redundant per-call-site pre-checks (`isActivePane` at
  ~532 and `document.activeElement` containment at ~535 inside
  `focusWatchdog`; `isActivePane` at ~698 in the WS handler) — these become
  partially redundant once `refocusActiveTerminal` absorbs an `active` check,
  but leaving them in place is a no-op early-exit, not a regression; removing
  them is an unnecessary diff for this phase.

## Codebase Findings

- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L143` — `keepTerminalFocusRef
  = useRef(false)`, the existing component-level-ref pattern the ticket names
  as precedent; add the new `composingRef` next to it.
- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L160-169` — `refocusActiveTerminal`,
  today: `window.setTimeout(() => { if (keepTerminalFocusRef.current &&
  containerRef.current?.offsetParent) { ...focus calls... } }, 0)`. This is
  the single choke point to extend with the composing/active guard.
- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L302-311` — `sendInputBytes`,
  call sites ~306 (socket-open branch) and ~310 (fallback branch); no
  independent pre-check today (relies solely on the internal guard).
- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L313-320` — `inputDisposable
  = terminal.onData(sendInputBytes)`, then effect-local `let composingInput =
  false` plus `markComposing`/`clearComposing` closures — this local variable
  is what must become `composingRef.current` writes.
- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L338-339` — listener wiring:
  `container.addEventListener("compositionstart", markComposing)` /
  `("compositionend", clearComposing)`. `clearComposing` is where the trailing-
  edge `refocusActiveTerminal()` call must be added, after clearing the ref.
- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L349-406` — `keydownFallback`;
  line 356 reads `event.isComposing || event.key === "Process" ||
  composingInput` — replace `composingInput` with `composingRef.current`.
- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L523-539` — `focusWatchdog`
  (`setInterval`, 100ms); existing guards at ~526 (`keepTerminalFocusRef`),
  ~529 (`container.offsetParent`), ~532 (`isActivePane`), ~535
  (`document.activeElement` already inside container -> early return), call
  site at ~538.
- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L686-707` — WS `"output"`
  message branch; `terminalRef.current?.write(...)` at ~693, existing
  `isActivePane` guard at ~698, call site at ~699.
- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L541-580` — mount-effect
  cleanup; listener removal list (~549-550 for
  `compositionstart`/`compositionend`) needs no change (still tears down the
  same two listeners), confirming no new listener/cleanup pairing is required.
- `ws-dashboard/frontend/src/keydownSuppression.ts` (36 lines) — the exact
  pattern to follow for the new predicate module: a DOM-free input type
  (`SuppressibleKeydownEvent`), a small exported pure function, and a header
  comment naming the originating ticket/phase.
- `ws-dashboard/frontend/src/keydownSuppression.test.ts` (267 lines) — the
  exact test-harness pattern: no test framework, a local `assertEqual`
  helper, plain `assertEqual(...)` calls per case, ending in
  `console.log("<file>.test.ts passed")`; imports the module via its
  compiled `./keydownSuppression.js` specifier (NodeNext ESM).
- `ws-dashboard/frontend/package.json` — `"test:keydown-suppression": "tsc -p
  tsconfig.route-tests.json && node
  ./node_modules/.tmp/route-tests/keydownSuppression.test.js"` is the
  dedicated-script precedent for a same-directory predicate+test pair; add an
  analogous script for the new module.
- `ws-dashboard/frontend/tsconfig.route-tests.json` — `include` array lists
  every route-test source/test file pair explicitly (e.g.
  `"src/keydownSuppression.ts"`, `"src/keydownSuppression.test.ts"`); the new
  predicate module and test file must be added here or `tsc -p
  tsconfig.route-tests.json` will not compile them. Note `terminalPaneBody.tsx`
  itself is NOT in this include list (only `src/workbench/**/*.ts` and an
  explicit list) — the `.tsx` edits are type-checked by `npm run build` (`tsc
  -b`), not by the route-tests config.

## Implementation Plan

1. Create `ws-dashboard/frontend/src/terminalRefocusGuard.ts`: export a
   DOM-free `RefocusGuardState` type (`{ composing: boolean; keepFocus:
   boolean; visible: boolean; active: boolean }`) and `shouldRefocusTerminal
   (state: RefocusGuardState): boolean` returning `!state.composing &&
   state.keepFocus && state.visible && state.active` — mirrors today's
   `keepTerminalFocusRef.current && containerRef.current?.offsetParent` check
   inside `refocusActiveTerminal` (L160-169) plus the new composing gate, plus
   the `isActivePane` check already performed ad hoc at two of the three call
   sites (~532, ~698) now folded into the one choke point. Follow
   `keydownSuppression.ts`'s header-comment/export style.
2. In `terminalPaneBody.tsx`, add `const composingRef = useRef(false);` next
   to `keepTerminalFocusRef` (L143).
3. Import `shouldRefocusTerminal` from `./terminalRefocusGuard.js` and rewrite
   `refocusActiveTerminal` (L160-169) so the `setTimeout(..., 0)` callback
   builds a `RefocusGuardState` (`composing: composingRef.current, keepFocus:
   keepTerminalFocusRef.current, visible:
   Boolean(containerRef.current?.offsetParent), active:
   liveRef.current.actions.isActivePane(liveRef.current.pane)`), calls
   `shouldRefocusTerminal`, and only performs the existing
   `terminalRef.current?.focus()` + `.xterm-helper-textarea` refocus when it
   returns `true`. Keep the function's outer shape (still schedules via
   `window.setTimeout(..., 0)`, still called unconditionally by all three
   call sites) unchanged — the state is read at fire time, not at call time,
   so a composition that starts after the timeout was scheduled but before it
   fires is still caught (this addresses the race described in the ticket's
   Background).
4. Replace the effect-local `let composingInput = false` (L314) and its
   `markComposing`/`clearComposing` closures (L315-320) with writes to
   `composingRef.current`. In `clearComposing`, after `composingRef.current =
   false`, call `refocusActiveTerminal()` once (trailing-edge restore per the
   ticket).
5. In `keydownFallback` (L356), replace the `composingInput` read with
   `composingRef.current`.
6. No changes needed at the three call sites themselves (L306, L310, L538,
   L699) — they already call `refocusActiveTerminal()` unconditionally or
   behind their existing pre-checks; the new gating is entirely internal to
   the function per the "single choke point" requirement.
7. Create `ws-dashboard/frontend/src/terminalRefocusGuard.test.ts` following
   `keydownSuppression.test.ts`'s pattern (local `assertEqual`, no test
   framework): cases for all-true -> `true`; each of `composing=true`,
   `keepFocus=false`, `visible=false`, `active=false` individually -> `false`;
   and a combined-false case. End with
   `console.log("terminalRefocusGuard.test.ts passed")`.
8. Add `"src/terminalRefocusGuard.ts"` and
   `"src/terminalRefocusGuard.test.ts"` to the `include` array in
   `ws-dashboard/frontend/tsconfig.route-tests.json`.
9. Add a dedicated npm script in `ws-dashboard/frontend/package.json`,
   mirroring `test:keydown-suppression`: `"test:terminal-refocus": "tsc -p
   tsconfig.route-tests.json && node
   ./node_modules/.tmp/route-tests/terminalRefocusGuard.test.js"`.

## Verification Plan

- `cd ws-dashboard/frontend && npm run test:terminal-refocus` — new predicate
  unit test.
- `cd ws-dashboard/frontend && npm run build` — type-checks the
  `terminalPaneBody.tsx` edits (this file is outside
  `tsconfig.route-tests.json`'s include list, so only the full app build
  (`tsc -b`) type-checks it).
- `cd ws-dashboard/frontend && npm run test:keydown-suppression` — regression
  check that the neighboring same-directory pattern this phase copies is
  still intact.
- Manual dogfood repro on Windows (per ticket): type Korean text via IME into
  an active terminal pane while output is streaming; confirm no
  dropped/corrupted leading characters and that focus restores promptly after
  composition ends. This end-to-end race stays manual/dogfood-only per the
  ticket.

## Escalations

- None.

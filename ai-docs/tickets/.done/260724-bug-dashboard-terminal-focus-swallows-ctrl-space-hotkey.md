---
title: Ctrl+Space hotkey swallowed when a terminal pane has focus
spec:
  - 260722-ws-dashboard-which-key-hint-overlay
related:
  260521-feat-ws-dashboard-command-dispatch-spine: prerequisite
  260722-feat-dashboard-hotkey-config-framework: prerequisite
  260721-feat-dashboard-suppress-browser-shortcuts: related
  260517-bug-ws-dashboard-terminal-focus-browser-gate-regression: related
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-07-24
---

# Ctrl+Space hotkey swallowed when a terminal pane has focus

## Background

Dogfood observation (owner, 2026-07-24): when a terminal pane has focus, the
`Ctrl+Space` hotkey does not open the dashboard's leader/command mode.

Root cause is confirmed in code (not the original xterm-capture hypothesis).
The global leader/hotkey dispatcher is a capture-phase `document` `keydown`
listener installed in `ws-dashboard/frontend/src/App.tsx`
(`document.addEventListener("keydown", handleKeydown, true)` at
`App.tsx:1715`). Capture-phase listeners on `document` fire before a focused
descendant element's own `keydown` handling, so xterm.js's internal textarea
keydown handling is not actually in the way — the dispatcher already sees
every keydown, including ones fired while a terminal pane's xterm textarea
has DOM focus.

The actual swallow happens inside `handleKeydown` itself
(`App.tsx:1625-1719`):

- `App.tsx:1637-1639` computes `targetInsideTerminalPane` from
  `document.activeElement?.closest(".terminal-pane")`.
- `App.tsx:1663-1672` calls `shouldSkipHotkeyCapture` (defined at
  `hotkeys.ts:446-457`), which unconditionally returns `true` — and the
  caller then `return`s out of `handleKeydown` — whenever
  `targetInsideTerminalPane` is `true`. This guard exists so that ordinary
  keys (e.g. typing `t`) pass through untouched to the focused terminal as
  literal input (`hotkeys.test.ts:835-844`: "focus already inside a
  terminal pane skips capture (terminal passthrough)").
- The leader-trigger check, `isLeaderTriggerKeydown` (`hotkeys.ts:398-404`,
  called at `App.tsx:1674-1691`), which is what recognizes `Ctrl+Space` and
  calls `enterLeaderPending`, sits *after* that early return. It is
  therefore unreachable for any keydown fired while a terminal pane is
  focused — including `Ctrl+Space` itself.

By contrast, the leader-*pending* continuation branch
(`App.tsx:1641-1661`, handling the next key after leader mode is already
active) is checked *before* `shouldSkipHotkeyCapture` and is unaffected by
terminal focus. Only the leader-*entry* trigger is misordered relative to
the terminal-passthrough guard. `hotkeys.ts:395-397`'s own design comment
("`Ctrl+Space` press enters the transient dashboard command mode") states
no exception for terminal focus, confirming this ordering is the bug, not
intended behavior.

## Spec Impact

`260722-ws-dashboard-which-key-hint-overlay` (## Which-Key Leader Hint
Overlay) states: "the existing leader-press capture path (including its
terminal-focus/IME guard) is unchanged and remains the sole
input-handling surface." That line currently documents the terminal-focus
guard as applying uniformly to the whole leader-press capture path,
which is the buggy behavior this ticket fixes. Phase 1 corrects that
sentence to state that the terminal-focus/IME guard applies to
leader-*continuation* keys and standalone bindings, but the leader-*entry*
trigger (`Ctrl+Space` idle -> pending) is checked before that guard and is
never blocked by terminal focus.

## Phases

### Phase 1: Let Ctrl+Space enter leader mode regardless of terminal focus

Reorder the checks in `handleKeydown` (`App.tsx:1625-1719`) so the
leader-entry trigger check (`isLeaderTriggerKeydown`) runs before the
`shouldSkipHotkeyCapture` terminal/editable-target passthrough guard,
mirroring how the leader-*pending* continuation branch
(`App.tsx:1641-1661`) already runs unconditionally first. Concretely:

- Keep the existing IME-composition skip (`event.isComposing ||
  event.key === "Process"`) ahead of the leader-trigger check — IME
  composition still suppresses `Ctrl+Space` recognition.
- Move (or duplicate) the `isLeaderTriggerKeydown` check to run before
  `targetInsideTerminalPane`/`targetIsEditable` are consulted, so a
  terminal-focused or editable-focused `Ctrl+Space` still calls
  `enterLeaderPending` + `event.preventDefault()` +
  `event.stopPropagation()`. `stopPropagation()` at the document
  capture phase prevents the event from ever reaching xterm's internal
  textarea keydown handling, so no stray control byte
  (e.g. NUL for Ctrl+Space) leaks into the terminal session as a side
  effect of the fix.
- Do not change `shouldSkipHotkeyCapture` itself or its behavior for any
  other key: standalone bindings and all non-leader-trigger keydowns must
  keep passing through to a focused terminal pane exactly as today
  (`hotkeys.test.ts:835-844` stays green unmodified).
- Update the `260722-ws-dashboard-which-key-hint-overlay` spec sentence
  per `## Spec Impact` above.

No new interaction model: this fits the existing capture-phase
document-listener + `shouldSkipHotkeyCapture`/`isLeaderTriggerKeydown`
architecture from `260722-feat-dashboard-hotkey-config-framework` and
`260721-feat-dashboard-suppress-browser-shortcuts`; it only fixes the
check ordering for the one already-designed leader-entry chord.

Verify:

- Unit: extend `hotkeys.test.ts`'s "Terminal-focus / IME capture guard"
  section (or the `App.tsx` handleKeydown decision logic, if extracted
  into a pure function as part of this fix) with a case asserting that a
  leader-trigger chord (`Ctrl+Space`) is NOT skipped when
  `targetInsideTerminalPane: true`, while a non-trigger key (e.g. `"t"`)
  with the same `targetInsideTerminalPane: true` still IS skipped
  (regression guard for both directions of this ordering fix).
- E2E: in `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`, using
  the existing `createTerminalPane`/`terminalSurface` helpers, focus a
  terminal pane (click into `.terminal-surface`, matching the existing
  `.terminal-surface`.click() pattern at line 311), then
  `page.keyboard.press("Control+Space")` and assert the
  `.which-key-overlay` (`role="status"`, `WhichKeyOverlay.tsx:87`)
  becomes visible — confirming leader mode was entered — and that no
  stray control byte was written into the terminal's rendered output as
  a side effect.

### Result (334f41cf) - 2026-07-24

- **Fix.** Reordered `ws-dashboard/frontend/src/App.tsx#handleKeydown` so the
  leader-*entry* trigger (`Ctrl+Space`) is evaluated before the
  `shouldSkipHotkeyCapture` terminal/editable-target passthrough guard, so a
  terminal-focused (or editable-focused) `Ctrl+Space` now opens the dashboard
  leader/which-key mode instead of being swallowed. The IME-composition skip
  still precedes the trigger, so a composing `Ctrl+Space` is still suppressed.
  `shouldSkipHotkeyCapture`'s body is untouched: non-trigger keys and standalone
  bindings still pass through to a focused terminal exactly as before, and the
  leader-*continuation* branch is unchanged and still checked first.
- **Structure.** The guarded-stage ordering was extracted into an exported pure
  function `decideKeydownGuardStage(evt)` in
  `ws-dashboard/frontend/src/hotkeys.ts` (decision
  `"enter-leader" | "skip-passthrough" | "fall-through"`) as the single source
  of truth; `handleKeydown` dispatches side effects
  (`enterLeaderPending` + `preventDefault` + `stopPropagation` / return /
  fall-through to standalone matching) based on the returned decision. This
  extraction is byte-for-byte behavior-preserving vs the inline reorder
  (confirmed by correctness re-review).
- **Tests.** `hotkeys.test.ts` gained four runnable ordering-regression cases
  directly against `decideKeydownGuardStage` (Ctrl+Space + terminal focus ->
  `enter-leader`; `"t"` + terminal focus -> `skip-passthrough`; Ctrl+Space +
  composing -> `skip-passthrough`; plain `"t"` with no guarded target ->
  `fall-through`) — these would fail if the ordering regressed. The pre-existing
  terminal-passthrough case is unchanged. An E2E step was added to
  `dashboard-acceptance.spec.ts` (focus a terminal pane, press
  `Control+Space`, assert the `.which-key-overlay` becomes visible and the
  terminal still round-trips input, i.e. no stray control byte leaked).
- **Verification.** `npm run build` clean (no type errors);
  `npm run test:hotkeys` green (incl. the four new cases); `npx tsc -b --noEmit`
  exit 0. The E2E was structurally verified (parses/lists) but NOT run, because
  the Playwright browser suite is independently RED on the unrelated bug
  `260713`; the ordering is nonetheless runnably guarded now by the new
  `hotkeys.test.ts` unit cases (not E2E-only).
- **Reviews.** Correctness review clean (plus a re-review confirming the
  extraction is behavior-preserving — `preventDefault` + `stopPropagation` both
  retained on `enter-leader`, no argument-shape mismatch). Test review clean
  after resolving two minors (removed a byte-for-byte duplicate assertion; made
  the ordering guard runnable via the extraction).
- **Spec.** `260722-ws-dashboard-which-key-hint-overlay` sentence in
  `ai-docs/spec/ws-web-dashboard/index.md` updated: the terminal-focus/IME guard
  applies to leader-continuation keys and standalone bindings, but the
  leader-entry trigger is checked before that guard and is never blocked by
  terminal/editable focus.
- **Commits.** Plan `a7a89f55`; implementation `334f41cf` (reorder + tests +
  spec + E2E) and `491e1c30` (pure-function extraction + runnable ordering unit
  guard + duplicate removal). Branch `impl/terminal-focus-ctrl-space-hotkey`,
  merging into `goal/drain-ready-queue`. No plugin version bump (only
  `ws-dashboard/frontend/` + `ai-docs/`).

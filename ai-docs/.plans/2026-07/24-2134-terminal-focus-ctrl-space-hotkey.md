# Plan: 260724-bug-dashboard-terminal-focus-swallows-ctrl-space-hotkey — Phase 1

## Relevant Ticket Contract
- Reorder `handleKeydown` so the leader-entry trigger check
  (`isLeaderTriggerKeydown`) runs before the `shouldSkipHotkeyCapture`
  terminal/editable-target passthrough guard, mirroring how the
  leader-*pending* continuation branch already runs unconditionally first.
- Keep the IME-composition skip (`event.isComposing || event.key ===
  "Process"`) ahead of the leader-trigger check — IME composition still
  suppresses `Ctrl+Space` recognition.
- A terminal-focused or editable-focused `Ctrl+Space` must still call
  `enterLeaderPending` + `event.preventDefault()` + `event.stopPropagation()`,
  so no stray control byte (e.g. NUL) leaks into the terminal.
- Do NOT change `shouldSkipHotkeyCapture` itself or its behavior for any
  other key: standalone bindings and all non-leader-trigger keydowns must
  keep passing through to a focused terminal pane exactly as today
  (`hotkeys.test.ts:835-844` stays green unmodified).
- Update the `260722-ws-dashboard-which-key-hint-overlay` spec sentence
  (`ai-docs/spec/ws-web-dashboard/index.md:881-883`) to state the
  terminal-focus/IME guard applies to leader-continuation keys and
  standalone bindings, while the leader-entry trigger is checked before
  that guard and is never blocked by terminal focus.
- Verify: unit regression covering both directions (trigger key not
  skipped, non-trigger key still skipped) in `hotkeys.test.ts`, plus an
  E2E case in `dashboard-acceptance.spec.ts` proving `Control+Space` opens
  the which-key overlay while a terminal pane has focus, with no stray
  byte written into the terminal.

## Out of Scope
- Changing `shouldSkipHotkeyCapture`'s behavior for non-trigger keys
  (standalone bindings, ordinary typed keys) — it must keep gating those
  exactly as today.
- The leader-*pending* continuation branch (`App.tsx:1651-1671`) — already
  unconditional, untouched by this fix.
- Any change to `isLeaderTriggerKeydown`'s own matching logic (which chord
  counts as the leader trigger) — only its position in the check order
  changes.
- The `260724-idea-dashboard-hotkey-leader-dispatch-gap` `terminal.create`
  dispatch-wiring gap noted in the existing which-key-overlay E2E step
  (`dashboard-acceptance.spec.ts:2820-2830`) — unrelated pre-existing gap,
  not this ticket's concern.
- Any other spec section besides the one sentence identified in `## Spec
  Impact` of the ticket.

## Codebase Findings
- `ws-dashboard/frontend/src/App.tsx#L1634-1729` — the capture-phase
  `document` `keydown` listener (`handleKeydown`, installed at line 1725).
  Current check order inside it, after the early
  `leaderStateRef.current.kind === "pending"` continuation branch
  (L1651-1671, already unconditional and returns before any guard):
  1. Compute `targetIsEditable` / `targetInsideTerminalPane` (L1640-1649,
     already computed before the pending branch — no change needed here).
  2. `shouldSkipHotkeyCapture({...})` (L1673-1682) → `return` if true. This
     is the guard that currently blocks everything below it, including the
     leader-trigger check, for any keydown while a terminal pane is focused.
  3. `isLeaderTriggerKeydown({...})` (L1684-1701) → calls
     `enterLeaderPending`, `preventDefault`, `stopPropagation`, `return`.
     **This is the check that must move ahead of step 2 for the trigger
     case.**
  4. `findStandaloneMatch` (L1703-1723) — standalone bindings, unaffected,
     must stay after `shouldSkipHotkeyCapture`.
  - Note: actual line numbers here (handler body L1634-1729,
    `isLeaderTriggerKeydown` call at L1684-1701) run about 9 lines later
    than the ticket text's cited range (App.tsx:1625-1719 / 1663-1691) —
    likely minor doc drift from an intervening edit. The check order and
    logic the ticket describes match exactly; use these confirmed line
    numbers for the edit.
- `ws-dashboard/frontend/src/hotkeys.ts#L446-457` — `shouldSkipHotkeyCapture`
  (pure function, three independent OR'd conditions: composing/Process,
  editable target, terminal-pane focus). No change needed to its body or
  signature; only its call-site position in `App.tsx` moves relative to
  `isLeaderTriggerKeydown`.
- `ws-dashboard/frontend/src/hotkeys.ts#L398-404` — `isLeaderTriggerKeydown`
  (pure function, already imported and called in `App.tsx`). No signature
  change needed; it already takes only `{key, ctrlKey, metaKey, shiftKey,
  altKey}`, none of which depend on `targetIsEditable`/
  `targetInsideTerminalPane`, so hoisting the call above the guard check is
  a pure reorder with no new data dependency to thread through.
- `ws-dashboard/frontend/src/hotkeys.test.ts#L803-844` — "Terminal-focus /
  IME capture guard" section, four `assertEqual` calls against
  `shouldSkipHotkeyCapture` directly (pure-function unit style, no DOM/React
  harness). The passthrough case at L835-844 ("focus already inside a
  terminal pane skips capture (terminal passthrough)") must stay green
  unmodified — it exercises `shouldSkipHotkeyCapture` alone, not the
  `handleKeydown` ordering, so this fix does not touch it.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L2751-2891` —
  existing which-key-overlay E2E step. Notably, its `focusNeutral()` helper
  (L2767-2773) has a comment stating "every leader press below first
  refocuses this always-present, non-editable, non-terminal toolbar button"
  because "[t]he leader-trigger keydown handler skips capture while focus
  sits inside an editable element or a `.terminal-pane`" — this comment
  currently documents the exact bug being fixed here. It is not the
  location for the new regression test (that belongs in a new, separate
  step per the ticket) and the existing step's own assertions do not need
  to change, but the comment becomes stale/inaccurate for the leader-entry
  case once this fix lands. Recommend a one-line comment correction as a
  minor hygiene follow-up in this same change (see Implementation Plan
  step 5) — optional, does not block the ticket's Done-when.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L261-266` —
  `terminalSurface(page)` helper: locates `.terminal-surface`, asserts
  visible plus its `.xterm` child visible. Use for the new E2E case instead
  of a bare `page.locator(".terminal-surface")` call, matching existing
  helper usage conventions.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L330-338` —
  `expectTerminalNotBlocked(page, tag)` helper: clicks the (first)
  `.terminal-surface`, types `commandPlan.echo(tag)`, presses Enter, asserts
  the tag appears in `.xterm-rows`. Reusable as the "no stray byte leaked"
  assertion after the new Ctrl+Space-while-terminal-focused case, in place
  of a bespoke raw-output-byte check — it already proves the terminal
  round-trips real input cleanly after a leader interaction, which is
  exactly the ticket's "no stray control byte" verification intent, and is
  the established pattern the existing which-key dismissal-path assertions
  (L2857, 2868, 2879, 2890) already use for the same purpose.
- `ws-dashboard/frontend/src/WhichKeyOverlay.tsx#L87` — `<div
  className="which-key-overlay" role="status" aria-live="polite">`, the
  overlay root the ticket's E2E assertion should target
  (`page.locator(".which-key-overlay")`, matching the existing overlay step
  at spec L2757).
- `ai-docs/spec/ws-web-dashboard/index.md#L878-883` — the exact spec
  paragraph to edit. Current sentence: "It does not capture or consume
  keyboard input; the existing leader-press capture path (including its
  terminal-focus/IME guard) is unchanged and remains the sole
  input-handling surface." Replace only the guard-scope clause per the
  ticket's `## Spec Impact` section — do not touch anything else in this
  spec anchor block (L846-889).
- **Pure-function-extraction trade-off** (ticket explicitly allows either):
  the ticket's own Verify wording says extend `hotkeys.test.ts` "(or the
  `App.tsx` handleKeydown decision logic, if extracted into a pure
  function as part of this fix)". Recommendation: do an in-place reorder
  inside `handleKeydown`, do not extract. Rationale — `isLeaderTriggerKeydown`
  and `shouldSkipHotkeyCapture` are already independently pure and already
  independently unit-tested (`hotkeys.test.ts`); the only thing changing is
  call order plus the `enterLeaderPending`/`preventDefault`/
  `stopPropagation`/`return` side effects that must stay inline in
  `handleKeydown` (they touch `leaderStateRef`/`setLeaderUiState`, which
  are React-effect-local). Extracting a new "decision" pure function would
  need to either duplicate those side-effect calls' shape or return a
  discriminated result for `handleKeydown` to act on — real refactor
  surface for a single-branch reorder, not proportional to a bug fix whose
  own ticket calls it optional. The two-direction unit regression the
  ticket asks for is expressible directly against the two existing pure
  functions (`isLeaderTriggerKeydown` + `shouldSkipHotkeyCapture`) without
  needing a new extraction: assert per-input that a terminal-focused
  `Ctrl+Space` chord is `isLeaderTriggerKeydown() === true` (so it will be
  checked first and reach `enterLeaderPending` before
  `shouldSkipHotkeyCapture` is ever consulted for it), while confirming
  `shouldSkipHotkeyCapture` still independently returns `true` for a
  terminal-focused non-trigger key. This documents the ordering contract
  under test without needing a third combinator function.

## Implementation Plan
1. `ws-dashboard/frontend/src/App.tsx` — in `handleKeydown` (current
   L1634-1729), move the `isLeaderTriggerKeydown(...)` block (current
   L1684-1701, including its `enterLeaderPending` / `setLeaderUiState` /
   `preventDefault` / `stopPropagation` / `return`) to run immediately after
   the leader-pending continuation branch (L1651-1671) and before the
   `shouldSkipHotkeyCapture(...)` block (current L1673-1682). Leave the
   IME-composition inputs (`event.isComposing || event.key === "Process"`)
   as part of `shouldSkipHotkeyCapture`'s own evaluation, but since
   `isLeaderTriggerKeydown` itself does not consult IME state, keep (or add,
   if the reorder drops it) an explicit composing check ahead of the
   leader-trigger block so an in-progress IME composition still suppresses
   `Ctrl+Space` recognition — the simplest form is guarding the moved block
   with the same `event.isComposing || event.key === "Process"` condition
   used inside `shouldSkipHotkeyCapture`, matching the ticket's explicit
   instruction to keep that skip ahead of the leader-trigger check.
   `targetIsEditable`/`targetInsideTerminalPane` computation (L1640-1649)
   stays where it is — both the moved trigger check and the now-later
   `shouldSkipHotkeyCapture` call still need them, and neither uses them for
   this reorder (`isLeaderTriggerKeydown` never reads them at all).
2. Do not modify `ws-dashboard/frontend/src/hotkeys.ts` — no changes to
   `shouldSkipHotkeyCapture` or `isLeaderTriggerKeydown` bodies/signatures.
3. `ws-dashboard/frontend/src/hotkeys.test.ts` — extend the "Terminal-focus
   / IME capture guard" section (after L844) with two new `assertEqual`
   cases: (a) `isLeaderTriggerKeydown({key: " ", ctrlKey: true, metaKey:
   false, shiftKey: false, altKey: false})` is `true` regardless of
   terminal focus (proving the trigger check is reachable/independent of
   the guard's inputs), paired with an assertion that
   `shouldSkipHotkeyCapture({..., key: " ", targetInsideTerminalPane:
   true})` alone would have returned `true` — documenting why the ordering
   (not the guard body) is what fixes the bug; and (b) confirm
   `shouldSkipHotkeyCapture({isComposing: false, key: "t", targetIsEditable:
   false, targetInsideTerminalPane: true})` still returns `true` (this is
   the existing L835-844 case — leave it unmodified, just note it as the
   "still skipped" half of the two-direction regression pair required by
   the ticket).
4. `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` — add a new
   `test.step` (near the which-key overlay step, e.g. immediately after the
   L2751-2891 step) that: clicks `.terminal-surface` (via
   `terminalSurface(page)`, L261-266) to focus the terminal, presses
   `Control+Space` (`page.keyboard.press`), asserts
   `page.locator(".which-key-overlay")` becomes visible (`role="status"`,
   `WhichKeyOverlay.tsx:87`) confirming leader mode was entered despite
   terminal focus, dismisses (e.g. `Escape`) to return to idle, then calls
   `expectTerminalNotBlocked(page, <tag>)` (L330-338) to prove no stray
   control byte leaked into the terminal session as a side effect.
5. (Optional hygiene, non-blocking) `ws-dashboard/frontend/e2e/
   dashboard-acceptance.spec.ts#L2767-2770` — adjust the `focusNeutral()`
   comment so it no longer states the leader-trigger keydown itself skips
   capture on terminal focus (only continuation/standalone keys do now);
   the helper's behavior and every existing call site stay unchanged.
6. `ai-docs/spec/ws-web-dashboard/index.md#L881-883` — replace "the
   existing leader-press capture path (including its terminal-focus/IME
   guard) is unchanged and remains the sole input-handling surface." with
   wording stating the leader-press capture path is unchanged and remains
   the sole input-handling surface, and that its terminal-focus/IME guard
   applies to leader-continuation keys and standalone bindings while the
   leader-entry trigger (`Ctrl+Space` from idle) is checked before that
   guard and is never blocked by terminal or editable-target focus.

## Verification Plan
- Unit: `cd ws-dashboard/frontend && npm run test:hotkeys` (route-test
  build + run of `hotkeys.test.ts`; this is the correct focused command —
  do NOT run `npm run test:browser`, the Playwright suite, for this check).
- E2E: the new `dashboard-acceptance.spec.ts` step runs under `npm run
  test:browser` (builds frontend + daemon, then `playwright test`). Flag
  for the executor: this Playwright suite is independently RED right now
  from an unrelated pre-existing bug (260713); a run may report failures
  outside the new step. Confirm the new step's own assertions pass (or
  isolate it, e.g. `playwright test -g "<new step name>"`, if the full-file
  run is too noisy) rather than requiring full-suite green.
- Manual sanity (optional, matches ticket's dogfood-observed scenario):
  focus a terminal pane in the running dashboard, press Ctrl+Space, confirm
  the which-key overlay opens and no stray character appears in the
  terminal.

## Escalations
- None.

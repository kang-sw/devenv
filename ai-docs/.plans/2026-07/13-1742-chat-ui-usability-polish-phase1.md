# Plan: 260713-fix-ws-dashboard-agent-chat-ui-usability-polish — Phase 1: Fix prompt-history recall requiring caret-at-column-0

## Relevant Ticket Contract

- `handlePromptKeyDown` only advances ArrowUp/ArrowDown prompt-history recall
  when the caret sits exactly at `selectionStart === 0 && selectionEnd === 0`;
  otherwise the keypress is silently ignored.
- A recalled entry leaves the caret at the end of the inserted text, so a
  second immediate ArrowUp does nothing today.
- Fix must reset/select the caret position on recall (e.g. select-all or move
  caret to column 0) so repeated ArrowUp keeps walking further back through
  history, matching standard shell/chat-history conventions, without removing
  the existing caret-at-start guard itself (that guard is intentional — see
  the code comment at `App.tsx:7325-7331` — so normal in-text editing at a
  non-zero caret position is left alone).
- Verification boundary (from ticket): extend the e2e assertion in
  `dashboard-acceptance.spec.ts`'s agent-chat history-traversal coverage
  (~line 2658) to assert two-or-more consecutive ArrowUp presses recall
  distinct, progressively-older history entries. Manual live-browser
  confirmation is nice-to-have, not required, given no browser tool is
  available to this execution context (same limitation noted for the sibling
  `260713-...adapter-wiring` ticket's Phase 4).

## Out of Scope

- Phase 2 (auto-scroll queued/pending bubble into view) and Phase 3
  (resume-history popover styling) — separate phases in the same ticket, not
  touched here.
- Removing or loosening the `caretAtStart` guard itself; the ticket only asks
  for caret-position reset on recall, not a change to when recall triggers.
- `revertPending`'s caret behavior when invoked directly from the manual
  "되돌리기" (revert) button click (as opposed to via `navigateHistory`
  landing on a still-pending entry) — not mentioned in the ticket and not
  exercised by the cited verification boundary.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx#L7360-7376` — `handlePromptKeyDown`: the
  exact bug site. Computes `caretAtStart` from `input.selectionStart`/
  `selectionEnd` on `event.currentTarget` (typed `HTMLInputElement`), returns
  early if not at start, otherwise calls `navigateHistory(...)`. No caret
  reset after recall.
- `ws-dashboard/frontend/src/App.tsx#L7332-7358` — `navigateHistory`: pure
  history-index bookkeeping; calls `setPromptValue(text)` (React state) to
  swap in the recalled text, or `revertPending(stillPending.id)` when the
  target index's text still matches a live pending/queued entry.
- `ws-dashboard/frontend/src/App.tsx#L7265-7274` — `revertPending`: also
  calls `setPromptValue(entry.text)`; reached both from `navigateHistory`
  (recall landing on a pending entry) and from the manual revert button's
  `onClick`.
- `ws-dashboard/frontend/src/App.tsx#L7426-7438` — the prompt `<input>` is a
  plain single-line `type="text"` controlled input (`value={promptValue}`,
  `onKeyDown={handlePromptKeyDown}`), with no existing `ref` attached. Since
  it is single-line, ArrowUp/ArrowDown carry no native browser caret-movement
  behavior on this element type — the `caretAtStart` guard exists purely as
  this feature's own "don't hijack the key inside an offset caret" REPL-style
  convention (see comment at `App.tsx:7325-7331`), not to preserve any
  browser default.
- `ws-dashboard/frontend/src/App.tsx#L7168-7170` — `useRef` is already
  imported/used extensively nearby (e.g. `pendingRef`, `streamHandlesRef`),
  so no new import is needed to add an input ref if the fix wants one.
- **Risk signal / confirms the bug**: `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L2759-2777`
  is the current history-traversal coverage. It explicitly presses `"Home"`
  before every single `ArrowUp`/`ArrowDown` press
  (`await promptInput.press("Home"); await promptInput.press("ArrowUp");`,
  repeated 4x) — i.e. the existing test manually works around today's bug by
  re-homing the caret before each arrow press, rather than exercising
  consecutive presses. This test must change to remove the `Home` presses
  between consecutive same-direction arrows (or otherwise assert two ArrowUp
  presses in a row recall two distinct entries) for the coverage to actually
  regress-test the fix; leaving the `Home` presses in place would keep
  masking a caret-reset regression.
- Controlled-input timing constraint: `setPromptValue` is a React state
  update; the DOM `value`/selection are not necessarily already updated by
  the time `navigateHistory(...)` returns inside `handlePromptKeyDown`. A
  caret reset written directly after the `navigateHistory(...)` call must
  not assume the new value has already committed to the DOM synchronously —
  schedule the `setSelectionRange` call for after the browser's next paint
  (e.g. `requestAnimationFrame`) using the `event.currentTarget` /
  `input` reference already captured in the handler, rather than relying on
  a `useEffect` timing guess. This avoids adding a new ref/effect pair and
  keeps the change localized to `handlePromptKeyDown`.

## Implementation Plan

1. In `ws-dashboard/frontend/src/App.tsx`'s `handlePromptKeyDown`
   (`App.tsx:7360-7376`), after the existing
   `navigateHistory(event.key === "ArrowUp" ? -1 : 1);` call, add a caret
   reset scheduled via `requestAnimationFrame` (or equivalent post-commit
   scheduling) that calls `input.setSelectionRange(0, 0)` on the same
   `input` (`event.currentTarget`) reference already captured earlier in the
   function at line 7369. Add a short comment explaining why the reset is
   deferred (DOM value/selection updates from `setPromptValue` land after
   the handler returns) and why resetting to column 0 (rather than
   select-all) keeps repeated ArrowUp/ArrowDown passing the `caretAtStart`
   guard on the next press.
2. Do not touch `navigateHistory` or `revertPending` — the caret reset lives
   entirely in `handlePromptKeyDown`, applying uniformly regardless of
   whether `navigateHistory` resolved via a plain `setPromptValue` or via
   `revertPending`.
3. In `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`, update the
   history-traversal block (`e2e/dashboard-acceptance.spec.ts:2759-2777`):
   remove the `Home` presses that precede each `ArrowUp`/`ArrowDown` press
   and instead press the arrows consecutively, asserting the value after
   each press. Concretely: `ArrowUp` → `"second phase-3 message"`, `ArrowUp`
   again (no `Home` in between) → `"first phase-3 message"`, `ArrowDown` →
   `"second phase-3 message"`, `ArrowDown` again → `""`. Update the trailing
   `note(...)` call's description text to describe consecutive-press
   traversal rather than the old per-press-Home behavior.

## Verification Plan

- `cd ws-dashboard/frontend && npx playwright test dashboard-acceptance.spec.ts`
  (or the project's existing e2e run command) — confirms the updated
  history-traversal assertions (consecutive ArrowUp/ArrowDown without
  intervening `Home`) pass against the fixed `handlePromptKeyDown`.
- Manual live-browser confirmation (queue several prompt-history entries,
  press ArrowUp twice in a row, confirm distinct progressively-older values)
  is nice-to-have per the ticket but not required — no browser tool is
  available in this execution context.

## Escalations

- None.

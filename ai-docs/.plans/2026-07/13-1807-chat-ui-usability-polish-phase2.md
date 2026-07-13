# Plan: 260713-fix-ws-dashboard-agent-chat-ui-usability-polish — Phase 2: Auto-scroll the queued/pending message bubble into view

## Relevant Ticket Contract

- When a user queues a message mid-turn, the pending ("steering…"/"queued for
  next turn") bubble can land below the fold with no auto-scroll on a tall
  transcript — queuing a message currently has no visible confirmation.
- Fix by scrolling the newly-queued pending bubble into view when it is added
  (e.g. a ref + effect on the pending bubble element).
- Explicit ticket note: there is no existing scroll-into-view/auto-scroll
  precedent anywhere in the transcript path today
  (`.agent-chat-pane-transcript` is a plain `overflow: auto` grid with no JS
  scroll management) — this is net-new behavior, not a mirror of an existing
  pattern.
- Verification boundary: manually confirm in a live browser (not available to
  this agent, consistent with sibling-ticket limitations); add a test
  asserting the pending bubble's scroll-into-view call (or resulting
  visibility) fires when a message is queued mid-turn.

## Out of Scope

- Phase 1 (prompt-history caret recall) — already implemented and closed
  (`### Result (7171aa84)`).
- Phase 3 (resume-history popover styling) — separate, independent concern
  gated on a live-browser recheck; not touched here.
- Any general transcript auto-scroll-to-bottom behavior on new agent-turn
  content — the ticket scopes this fix specifically to the pending/queued
  bubble, not a broader "always scroll to bottom" feature.
- Changing the FIFO queue mechanics, `beginSimulatedTurn`, `revertPending`, or
  the steer control-call wiring — untouched, pre-existing, and out of this
  phase's contract.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx#L7161-L7170` — `AgentChatPaneBody`'s
  Phase-3 queuing state block: `pendingMessages` (`useState<PendingChatMessage[]>`)
  is the rendered array, `pendingRef` is the FIFO mirror used inside
  handlers/closures. This is where a new ref for the pending-bubble DOM node
  and a length-tracking ref belong, following the file's existing
  `useState` + parallel `useRef` convention (see `paneRef`,
  `pendingRef`/`pendingMessages` pairing already in place).
- `ws-dashboard/frontend/src/App.tsx#L7276-L7304` — `submitPrompt`'s
  mid-turn branch: `pendingRef.current = [...pendingRef.current, entry]` then
  `setPendingMessages(pendingRef.current)`. New entries are always appended
  at the end, so the *last* element of `pendingMessages` is always the most
  recently queued one — the element to scroll into view.
- `ws-dashboard/frontend/src/App.tsx#L7220-L7233` — `onTurnComplete` dequeues
  via `pendingRef.current = rest; setPendingMessages(rest)` (shifts the
  front off). `revertPending` (`#L7265-L7274`) also shrinks the array. Both
  of these decrease `pendingMessages.length`; only `submitPrompt`'s queue
  branch increases it. An auto-scroll effect must trigger only on the
  increase (new-message-queued) case, not on every `pendingMessages`
  reference change, or it will also re-fire (harmlessly, but pointlessly)
  on dequeue/revert.
- `ws-dashboard/frontend/src/App.tsx#L7403-L7436` — pending-bubble render
  site: `.map((entry) => <div ... data-testid="agent-chat-pending-bubble"
  key={entry.id}>...)` inside `.agent-chat-pane-transcript`. Every pending
  bubble shares the same `data-testid`, so a `.last()` (Playwright) or
  index-based ref-callback (React) is required to target specifically the
  newest one, not just "a" pending bubble.
- `ws-dashboard/frontend/src/styles.css#L3787-L3791` — confirms the ticket's
  claim: `.agent-chat-pane-transcript { display: grid; gap: ...; overflow:
  auto; }` — no `scroll-behavior`, no JS scroll listener/management anywhere
  else in the file for this container. Net-new behavior, as the ticket says.
- `ws-dashboard/frontend/src/App.tsx` (file-wide) — `useRef`/`useEffect` are
  used pervasively (100+ call sites) with an established local idiom:
  declare a `useState` for render + a parallel `useRef` for
  synchronous/closure-safe reads, and effects keyed on a narrow dependency
  (e.g. `activeActivityId`) rather than broad object identity. The new
  effect should follow this same shape: `useRef<HTMLDivElement | null>(null)`
  for the last-pending-bubble node, a `useRef(0)` (or similar) tracking the
  previous `pendingMessages.length`, and a `useEffect` on `[pendingMessages]`
  that only acts when length increased.
- **Test-infra constraint (risk signal carried over from Phase 1's
  findings, independently reconfirmed)**: `ws-dashboard/frontend/package.json`
  has no `vitest`/`jsdom`/`@testing-library` dependency; every `test:*`
  script (`test:agent-chat-tabs`, `test:agent-chat-bubbles`, etc.) is
  `tsc -p tsconfig.route-tests.json && node ./node_modules/.tmp/route-tests/*.js`
  — a plain compiled-Node script, no DOM. `src/agentChatBubbles.test.ts`
  confirms the one DOM-adjacent pattern that exists: `renderToStaticMarkup`
  (server-side static HTML string diffing) — this performs no commit
  phase, so refs never attach and effects never run. **There is no way to
  unit-test an actual `scrollIntoView` call or ref-attachment from this
  route-tests harness.** The only place a real DOM + real effect commit +
  real scroll geometry exists is Playwright
  (`e2e/dashboard-acceptance.spec.ts`), which already runs against the live
  built app in a real browser.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L2656-L2710`
  ("agent chat send input, mid-turn queuing, revert, and history
  traversal") — the existing mid-turn-queuing test step already sends a
  first message, then a second message while the first streams, and
  asserts the second renders as `[data-testid="agent-chat-pending-bubble"]`
  with the "steering…" badge. This is the natural place to extend (or add
  a sibling step) for a scroll-into-view assertion, following its existing
  locator conventions (`pane.locator(...)`, `transcript.locator(...)`).
- **Known environment blocker (Phase 1 finding, applies identically
  here)**: per Phase 1's `### Result`, `dashboard-acceptance.spec.ts` fails
  before reaching the agent-chat transcript assertions in this environment
  (Codex tile click leaves the transcript hidden — filed as
  `260713-bug-dashboard-acceptance-codex-tile-transcript-hidden`, confirmed
  pre-existing and unrelated to Phase 1's diff by the reviewer). Any new
  Playwright assertion added for this phase will likely hit the same
  pre-existing blocker and not run to completion in this environment either
  — this is an execution-environment limitation, not a reason to change the
  implementation approach.

## Implementation Plan

1. In `ws-dashboard/frontend/src/App.tsx`, inside `AgentChatPaneBody`
   (near the existing `pendingMessages`/`pendingRef` declarations at
   `#L7163-L7166`), add:
   - `const lastPendingBubbleRef = useRef<HTMLDivElement | null>(null);`
   - `const pendingCountRef = useRef(0);`
2. Add a new `useEffect(() => { ... }, [pendingMessages]);` near the other
   pending-queue effects (after the `activeActivityId` reset effect at
   `#L7184-L7191` is a reasonable spot) that:
   - Reads `pendingMessages.length`.
   - If the new length is greater than `pendingCountRef.current` (a message
     was just queued, not dequeued/reverted), calls
     `lastPendingBubbleRef.current?.scrollIntoView({ block: "nearest" })`
     (or `"end"` — pick whichever keeps the bubble fully visible without
     over-scrolling past the input box; `"nearest"` is the safer default
     for a bottom-anchored, already-mostly-visible container).
   - Unconditionally updates `pendingCountRef.current = pendingMessages.length`
     at the end, so both increases and decreases stay tracked correctly.
3. In the pending-bubble render block (`#L7411-L7435`), attach the ref only
   to the last element of the map:
   ```tsx
   {pendingMessages.map((entry, index) => (
     <div
       ...
       key={entry.id}
       ref={index === pendingMessages.length - 1 ? lastPendingBubbleRef : undefined}
     >
   ```
   Keep every other prop/attribute (including
   `data-testid="agent-chat-pending-bubble"`) unchanged.
4. Add a short inline comment at the new effect (matching the file's
   existing comment density for Phase 3 queuing code) noting: net-new
   auto-scroll behavior added for this ticket's Phase 2, scoped to the
   newest pending bubble only, guarded to fire only on queue-growth (not
   dequeue/revert), because there is no other scroll-management precedent
   in this transcript container to follow.
5. Extend `e2e/dashboard-acceptance.spec.ts`'s existing mid-turn-queuing
   test step (`#L2656-L2710` and its continuation) with an assertion that
   the pending bubble is scrolled into view when queued on a tall
   transcript. Two viable approaches — pick based on what's simplest to
   wire without new test infra:
   - **Preferred**: before triggering the queue action, pad the transcript
     tall enough that the pending bubble would start outside the viewport
     (e.g. send enough turns/or resize the pane short), then after queuing,
     assert `await expect(pendingBubble).toBeInViewport()` (Playwright's
     built-in viewport-intersection assertion) — this directly verifies the
     *outcome* (visibility) rather than the mechanism, and needs no
     monkey-patching.
   - **Alternative** (only if the transcript can't easily be made tall
     enough in the existing test's flow): use `page.addInitScript` (or
     `page.evaluate` before the click) to wrap
     `Element.prototype.scrollIntoView` and record calls on elements
     matching the pending-bubble selector, then assert it was called after
     queuing. Prefer the viewport-based approach first since it tests
     actual user-visible behavior, not the implementation detail.

## Verification Plan

- `cd ws-dashboard/frontend && npm run build` (type-check + build; no new
  route-tests script needed since no pure/DOM-free logic was added — the
  change is a ref/effect on a component, which the compiled-Node
  `test:*` scripts cannot exercise per the test-infra finding above).
- Extend `dashboard-acceptance.spec.ts` per Implementation step 5; run via
  `npm run test:browser`. Expect this to hit the same pre-existing
  Codex-tile-transcript-hidden blocker noted in Phase 1's result
  (`260713-bug-dashboard-acceptance-codex-tile-transcript-hidden`) in this
  environment — if so, this is not a regression from this diff; verify
  correctness instead by direct code reading/manual trace of the new
  effect's guard logic (increase-only trigger, last-element-only ref),
  same as Phase 1's fallback verification approach.
- Manual browser confirmation (queue a message on a transcript tall enough
  to push the pending bubble below the fold, confirm it scrolls into view)
  is not available to this agent — same limitation as prior phases in
  sibling tickets; leave for a human/live-browser pass.

## Escalations

- None.

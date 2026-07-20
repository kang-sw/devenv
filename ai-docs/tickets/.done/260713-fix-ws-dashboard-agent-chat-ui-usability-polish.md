---
title: "Agent chat UI usability polish from manual browser walkthrough"
related:
  260711-feat-ws-dashboard-agent-activity-chat-ui: prerequisite
  260713-feat-ws-dashboard-agent-chat-real-adapter-wiring: related
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-07-20
---

# Agent chat UI usability polish from manual browser walkthrough

## Background

`260711` (agent activity chat UI) shipped with full automated Playwright E2E
coverage, but no human/manual exploratory pass had been done against the
running UI. A dedicated manual walkthrough (real daemon + real browser,
clicking through the chat surface on branch `impl/chat-ui-server`) surfaced
several usability issues that targeted automated assertions did not catch.
Screenshots from the walkthrough are not preserved (were left in a session
scratchpad, not committed); reproduce fresh if visual reference is needed.

None of these block core functionality (tile launch, fork-from-here, revert,
markdown rendering all worked correctly in the walkthrough) — this ticket is
purely about interaction/discoverability polish.

## Phases

### Phase 1: Fix prompt-history recall requiring caret-at-column-0

`App.tsx`'s `handlePromptKeyDown` (around line 7201) only advances the
ArrowUp/ArrowDown prompt-history recall when the caret sits exactly at
`selectionStart === 0 && selectionEnd === 0`; otherwise the key press is
silently ignored. Since a recalled entry leaves the caret at the end of the
inserted text, a second immediate ArrowUp press does nothing — walkthrough
screenshots showed identical input content across two consecutive ArrowUp
presses. This contradicts standard shell/chat-history-recall conventions
(repeated Up should keep walking further back regardless of caret position).
Fix by resetting/selecting caret position on recall (e.g. select-all or move
caret to 0) so repeated ArrowUp continues traversing history, matching
common chat-input UX conventions.

**Verification**: add/extend an e2e assertion in
`dashboard-acceptance.spec.ts`'s agent-chat history-traversal coverage
(around line 2658) asserting two-or-more consecutive ArrowUp presses recall
distinct, progressively-older history entries; manually confirm in a live
browser that repeated ArrowUp keeps walking back.

### Result (7171aa84) - 2026-07-13

Fixed by resetting the caret to column 0 after history recall.

- `handlePromptKeyDown` (`App.tsx`) now schedules
  `input.setSelectionRange(0, 0)` via `requestAnimationFrame` right after
  `navigateHistory(...)`, deferred because `setPromptValue`'s DOM commit
  hasn't landed synchronously inside the handler. Column-0 (not select-all)
  was chosen so the reset itself satisfies the existing `caretAtStart`
  guard, letting the next ArrowUp/ArrowDown keep walking history. The guard
  itself is untouched, per the ticket's explicit constraint.
  `navigateHistory`/`revertPending` were not touched.
- `dashboard-acceptance.spec.ts`'s history-traversal block previously pressed
  `Home` before every single ArrowUp/ArrowDown — an active workaround that
  was masking this exact bug. Removed those `Home` presses so the test now
  exercises genuinely consecutive presses; review confirmed via manual
  state-machine tracing that the test would fail without the fix (not
  tautological).
- Review: single full-scope pass, clean, no fix cycle needed.
- Verification: `npm run build` passed clean. The full
  `dashboard-acceptance.spec.ts` playwright run could not reach the modified
  assertions in this environment — it fails earlier (line ~2578, transcript
  stays hidden after the Codex tile click) regardless of this diff; the
  reviewer independently reproduced the identical failure against both HEAD
  and the pre-diff baseline commit, confirming it's pre-existing and
  unrelated. Filed as its own ticket:
  `260713-bug-dashboard-acceptance-codex-tile-transcript-hidden` (idea/).
  Given the harness couldn't run to completion, the fix's correctness was
  verified by direct code reading plus manual trace of the modified test's
  expected state machine (documented in the review findings), not by a
  passing CI run — this is a real environment limitation, not a defect in
  this diff.
- Manual live-browser confirmation (nice-to-have per the ticket) was not
  performed — no browser tool available to this agent, same limitation
  noted elsewhere in this workspace's sibling tickets.

### Phase 2: Auto-scroll the queued/pending message bubble into view

When a user queues a message mid-turn (while the agent is still streaming),
the pending ("steering...") bubble can end up below the fold with no
auto-scroll, especially once the transcript has grown tall. A user can queue
a message and have no visible confirmation it happened. Fix by scrolling the
newly-queued pending bubble into view when it's added (e.g. a ref + effect
on the pending bubble element). Note: there is no existing scroll-into-view/
auto-scroll precedent anywhere in the transcript path today
(`.agent-chat-pane-transcript` is a plain `overflow: auto` grid with no JS
scroll management) — this phase is net-new transcript auto-scroll behavior,
not a mirror of an existing pattern; design review confirmed the fix itself
is still clear and implementable from scratch.

**Verification**: manually queue a message on a transcript tall enough to
push the pending bubble below the fold and confirm it scrolls into view;
add a test asserting the pending bubble element's scroll-into-view call (or
resulting visibility) fires when a message is queued mid-turn.

### Result (1edd3bec) - 2026-07-13

Implemented as net-new behavior, no existing precedent to mirror.

- `AgentChatPaneBody` (`App.tsx`) gained `lastPendingBubbleRef` and
  `pendingCountRef`, plus a `useEffect` on `[pendingMessages]` that calls
  `scrollIntoView({ block: "nearest" })` on the newest pending bubble only
  when `pendingMessages.length` increased versus the tracked count (a
  genuine queue-growth event), staying silent on dequeue
  (`onTurnComplete`)/revert (`revertPending`), which only shrink the array.
  The ref is attached only to the last mapped pending-bubble element.
  `submitPrompt`/`onTurnComplete`/`revertPending`/FIFO mechanics were not
  touched, per the plan's guardrails.
- Extended `dashboard-acceptance.spec.ts`'s mid-turn-queuing test step with
  a viewport-shrink-then-`toBeInViewport()` assertion (Playwright's
  outcome-level check) rather than monkey-patching `scrollIntoView` —
  verifies the user-visible result, not the implementation mechanism.
- Review: correctness clean; test clean with 1 informational minor (no
  `try/finally` around the viewport shrink/restore — matches 4 pre-existing
  instances of the same pattern elsewhere in the file, not a functional
  defect in this codebase's single-monolithic-test execution model). No fix
  cycle needed.
- Verification: `npm run build` passed clean. The full e2e run hit the same
  pre-existing, unrelated blocker documented in Phase 1's Result and filed
  as `260713-bug-dashboard-acceptance-codex-tile-transcript-hidden` — the
  test reviewer independently re-ran the suite and confirmed the identical
  pre-existing failure point, then verified the new effect's correctness by
  direct code trace (guard logic, ref-attachment timing, CSS scroll-chain
  analysis) instead of a passing CI run.
- Manual live-browser confirmation (nice-to-have per the ticket) was not
  performed — no browser tool available to this agent.
- No spec entry added: the underlying mid-turn message-queuing UI behavior
  itself has no dedicated spec section yet; this phase is UI polish on an
  already-shipped, currently-unspecced interaction.

### Phase 3: Add visible container styling to the resume-history popover

The resume-session popover (listing cross-harness session history by
harness tag + title + timestamp) was observed during the manual walkthrough
rendering with no visible container border/shadow. **Design review found
this may already be fixed**: `styles.css:3683-3694`'s
`.agent-chat-history-popover` already has `border`/`background`/`box-shadow`,
applied unconditionally in `App.tsx` (~line 7308) across all render states,
present since `260711` Phase 1 (`414d8805`), unmodified at HEAD. Before
writing any code, open the live UI and confirm whether the popover still
looks unstyled (possibly a stale-build/cache artifact from the walkthrough)
or whether a different rendering bug (positioning/z-index/interaction, not
missing chrome) is the real cause — then fix whatever is actually observed,
or close this phase as a no-op if the popover already renders correctly.

### Result (no-op) - 2026-07-13

Closed as a no-op with respect to code: no CSS/JSX changes were made.

- Static analysis fully confirms design review's claim. `styles.css:3683-3694`
  declares `border`, `background`, and `box-shadow` on
  `.agent-chat-history-popover` unconditionally, with no other selector
  overriding them. The popover has a single render site
  (`App.tsx:7511-7516`), gated only by `historyOpen`, using a fixed
  (non-conditional) `className` with no inline `style` override. There is
  no `z-index` conflict (all declared values are `1`, `4`, `20`, `30`, `35`,
  or `1000`, and nothing paints over this popover's stacking context).
  `git blame` shows the block unchanged since its introduction in
  `414d8805` (260711 Phase 1). Color contrast between
  `--ws-color-border-strong`/`--ws-color-panel-raised` and the surrounding
  `--ws-color-panel` background was also checked and ruled out as a
  near-invisible-contrast explanation.
- One open finding, flagged honestly as a hypothesis, not a confirmed
  defect: `.agent-chat-history-popover` uses `position: absolute` inside
  ancestors with `overflow: auto`/`hidden` (`.agent-chat-pane`, and
  dockview's bundled `.dv-groupview` CSS), with no protection against
  ancestor-overflow clipping. The sibling popover
  `.workbench-close-popover` (`styles.css:2020-2031`) deliberately avoids
  this by using `position: fixed` with a JS-computed cursor-anchored
  `top`/`left`. Whether `.agent-chat-history-popover` actually clips in
  practice depends on live runtime layout (panel width/height, scroll
  position) that cannot be determined from source alone, so this is
  recorded as a plausible mechanism worth remembering, not a verified bug.
  Per the plan's guidance, no `idea/` ticket is filed for this yet since it
  has not been confirmed as an actual observed defect.
- The phase's own required precondition — "before writing any code, open
  the live UI and confirm whether the popover still looks unstyled..." —
  was **not performed**. No browser automation tool is available in this
  environment, the same recurring limitation already recorded in this
  ticket's Phase 1 and Phase 2 Results and in the sibling ticket
  `260713-feat-ws-dashboard-agent-chat-real-adapter-wiring`'s Phase 4
  Result. This closure rests entirely on static source analysis (CSS/JSX
  inspection, `git blame`, z-index audit, dockview bundled-CSS inspection),
  which matches design review's independent suspicion but is not the live
  confirmation the phase text explicitly asks for.
- No test was added or changed: since no code changed, there is nothing new
  to verify by test, and no test claims are made here.
- Given the live-UI check genuinely was not done, this phase's no-op
  conclusion is **not** a substitute for it. Consistent with how Phase 1
  and Phase 2 of this ticket were left (both explicitly documented the
  same manual-live-browser-confirmation gap as an accepted, non-blocking
  limitation rather than reopening or holding up those phases), this
  phase's closure follows the same precedent: the gap is accepted and
  documented here rather than treated as a blocker on this phase itself.
  The ticket stays in `ready/` (unchanged from before this phase) with the
  live-UI check (Verification Plan steps 1-2 in the corresponding plan
  file) recorded as outstanding follow-up for whenever a browser tool
  becomes available, rather than as a precondition that must be satisfied
  before this phase's Result can be recorded.

## Non-goals

- The stub chat response being identical text across all turns is a known,
  intentional limitation of the stub provider (not this ticket's scope) —
  resolved as a side effect of `260713-feat-ws-dashboard-agent-chat-real-adapter-wiring`
  once real Codex/Claude adapters are wired in; no fix needed here.
- "Resume from here" rendering as fully absent (not a visible disabled
  button) is the deliberate capability-gated design from `260711` Phase 3;
  not a defect, no change needed here.


## Resolution (2026-07-20)

All 3 phases carry Result sections. Phase 1 (7171aa84) and Phase 2 (1edd3bec) are confirmed reachable from goal/drain-ready-queue, goal/ws-dashboard-tickets, ws-dashboard-dev, and remotes/origin/ws-dashboard-dev — fully merged and pushed. Phase 3 is a documented no-op closed on its own terms. The shared manual-live-browser-confirmation gap across all three phases is accepted and non-blocking per each phase's own Result wording.

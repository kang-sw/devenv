---
title: "Agent chat UI usability polish from manual browser walkthrough"
related:
  260711-feat-ws-dashboard-agent-activity-chat-ui: prerequisite
  260713-feat-ws-dashboard-agent-chat-real-adapter-wiring: related
sage-review-design: completed
sage-review-completeness: completed
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

## Non-goals

- The stub chat response being identical text across all turns is a known,
  intentional limitation of the stub provider (not this ticket's scope) —
  resolved as a side effect of `260713-feat-ws-dashboard-agent-chat-real-adapter-wiring`
  once real Codex/Claude adapters are wired in; no fix needed here.
- "Resume from here" rendering as fully absent (not a visible disabled
  button) is the deliberate capability-gated design from `260711` Phase 3;
  not a defect, no change needed here.

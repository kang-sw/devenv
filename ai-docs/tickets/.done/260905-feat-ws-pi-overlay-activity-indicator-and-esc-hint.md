---
title: Owner overlay shows a working indicator while the respondent thinks and states what Esc does
related:
  260904-feat-ws-pi-side-thread-fork-question-surface: owns the overlay chat component
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 9210cb30c41eda84
sage-review-completeness-reviewed: 9210cb30c41eda84
completed: 2026-09-06
---

# Owner overlay shows a working indicator while the respondent thinks and states what Esc does

## Background

Acceptance run 2026-09-05, scenario E3: after answering a fork-raised
question in the overlay, the owner saw no sign that the fork was processing
the answer, and could not tell whether `Esc` cancelled the exchange or only
closed the view. The spec is unambiguous (`Esc` closes the view only; the
thread stays open and the fork keeps running), and the overlay does already
render that sentence: `overlay-chat.ts` prints a dim footer line under the
input (`/done closes the thread · Esc closes this view (the thread keeps
running)`) and has since the component landed. The owner still missed it.
The likely reason is layout, not wording: the overlay is capped at
`maxHeight: "80%"` and renders up to 24 transcript rows plus the box, the
input line, and the footer, so on a short terminal the footer is the first
line pushed out of view. The header, which is always on screen, carries no
key hint.

Separately, the overlay renders activity only through streamed text deltas,
so a respondent that is thinking or running a tool shows nothing at all.

## Decisions

- **Activity marker in the streaming slot.** While the respondent's turn is
  running and the streaming tail is empty, the overlay renders one line
  `working…` where the tail would appear; the first text delta replaces it
  and settle clears it. The running state is read from
  `ForkChannel.isStreaming()` at render time, which is backed by the
  registry's streaming flag set the instant the prompt is issued. Rejected:
  deriving the state from `agent_start`/`agent_settled` events the component
  receives, because two of the paths this ticket exists for never deliver a
  start event to the component: attaching to a fork-raised thread whose run
  started before the overlay subscribed, and a dormant thread's first message,
  where the channel attaches its listener only after the prompt is sent.
  Rejected: a spinner animation, which would need a timer and re-render on
  every tick for no information gain.
- **Key hint moves from the footer to the header.** The footer line is
  removed and the same fact is stated in the header, directly after the
  `opened <time>` line when present, as
  `Esc: close view (thread stays open) · /done: end thread`. The header
  wraps to width like the title line does (`wrapLine`), so at narrow widths
  the hint takes two rows rather than clipping. Rejected: keeping the footer
  and adding the header line, which states one fact twice and leaves the
  original cause (footer scrolled out) undiagnosed. Rejected: a toast on
  `Esc`, since the owner has already left the overlay by then.
- **No behavior change.** `Esc` and `/done` keep their current semantics; this
  ticket only makes them visible.

## Constraints

- Both additions stay inside the existing `render(width)` width bound
  (`visibleWidth(line) <= width`).
- The marker never appears in the persisted transcript (it lives in the
  render-time streaming slot, which `onTranscriptChange` never reports).
- The footer hint is not duplicated: after this ticket exactly one line in
  the overlay states what `Esc` does.

## Spec Impact

`pi-adapter-runtime`: amend the "Overlay chat" bullet of the owner-question
surface anchor with the working marker and the header hint (replacing any
mention of the footer hint).

## Phases

### Phase 1: Working marker and Esc hint

In `overlay-chat.ts`: read `channel.isStreaming()` in `render`, draw the
marker when it is true and the streaming tail is empty, move the key hint
from the footer to the header. Tests: marker present when the channel
reports streaming with no tail, replaced by the first delta, absent once
the channel reports not streaming after settle; marker present on the very
first render of a component whose channel already reports streaming (the
attach-mid-turn and dormant-relaunch cases); header hint present exactly
once and width-bounded at 40/80/120, footer line gone; transcript
persistence unchanged. Amend the spec bullet. Live check (owner-run): open a
fork-raised thread with `/answer` while the fork is mid-turn and confirm the
marker shows immediately; then send a message that makes the fork run a
tool and confirm the marker shows before any text arrives.

### Result (fd36f541) - 2026-09-06

Landed as `7aeb5d2a` (survey plan), `fd36f541` (overlay change and tests),
`6911aae3` (spec bullet), `8cb384f0` (review relay #1) on the
implementation branch under the goal branch. Adapter-only change, confined
to `overlay-chat.ts`, its test file, and the spec bullet.

- `transcriptRows` draws one `working…` row in the streaming slot when
  `channel.isStreaming()` is true and the tail is empty; the first delta
  replaces it and settle clears it. The row is built at render time and
  never reaches `entries` or `onTranscriptChange`.
- The key hint moved from the footer to the header, directly after the
  `opened <time>` row, as `Esc: close view (thread stays open) · /done: end
  thread`, wrapped by `wrapLine`; the footer block is gone, so exactly one
  line states what `Esc` does. `Esc` and `/done` semantics unchanged.
- Relay #1 corrected a premise in this ticket: `record.streaming` flips on
  `agent_start`, not "the instant the prompt is issued" (`promptAgent`
  sets `running`). With the render cache keyed on width only and no
  refresh on `agent_start`, the frame cached at submit time hid the marker
  until the first delta. The cache now misses when `isStreaming()` differs
  from the cached value, `handleEvent` refreshes on `agent_start`, and
  `deliver` refreshes after `send()` resolves, which also covers the
  dormant-relaunch first message and the stale-true mirror when the flag
  clears without an event.
- Tests: the ticket's cases plus three render-time discriminators (bare
  flag flip and re-render, `agent_start` alone requests a render, settle
  mirror without an event); header-hint tests pin the row position after
  `opened`. Adapter suite 770 pass, 0 fail.
- Spec: "Overlay chat" bullet amended with the marker and the header hint;
  anchor id unchanged.

Review (single, full scope): one Critical (marker never shown in the E3
scenario, see above), one Important (no test could catch it), three Minor.
Critical and Important fixed in relay #1; two Minor fixed, one recorded: a
live task fork (`summarizeOnDone: false`) shows `working…` throughout its
own task work, which the ticket wording ("the respondent's turn is
running") covers. The Critical-scoped re-review confirmed the fix with a
probe of E3, dormant relaunch, and the stale-true mirror, and left one
Minor (the post-send refresh in `deliver` is unpinned by a test).

Owner-run live check outstanding: open a fork-raised thread with `/answer`
mid-turn and confirm the marker, then trigger a tool run and confirm the
marker precedes any text.

## Blocked (2026-09-06) — owner sign-off pending, not a work item

Phase 1 carries a Result; no autonomous work remains. Closing waits on the
owner-run live check above. Once confirmed, close the ticket to `.done/`.


## Resolution (2026-09-06)

Owner-run live check on 2026-09-06 passed (owner reported all four overlay checks pass: marker on /answer mid-turn, marker before tool-run text, header hint present once, footer gone).

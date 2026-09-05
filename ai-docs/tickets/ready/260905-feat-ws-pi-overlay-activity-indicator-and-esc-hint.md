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

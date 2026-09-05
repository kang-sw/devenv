---
title: "Pi adapter: always-visible list of live agents and threads in the lead TUI"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260802-research-ws-pi-native-framework: research anchor — names an always-visible workflow surface as deferred expansion scope (the "always-visible TODO" item)
  260903-feat-ws-pi-subagent-rpc-ux: the RPC agent registry (`ws-agent-list` status) this widget renders
  260904-feat-ws-pi-side-thread-fork-question-surface: the `aboveEditor` pending-question widget this ticket folds into one panel
  260905-feat-ws-pi-push-only-child-reports: sibling — once reports are pushed, the widget is how the lead and owner see what is still running
  260905-feat-ws-pi-agent-alias-park-and-registry-cap: prerequisite — rows name children by alias/title, and its automatic park at idle decides which rows exist (running, awaiting approval, awaiting owner; never dormant)
related-mental-model:
  - plugin-runtime
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: efa51a1840b28444
sage-review-completeness-reviewed: efa51a1840b28444
---

# Pi adapter: always-visible list of live agents and threads in the lead TUI

## Background

Owner request (2026-09-05): while the lead drives several workers, forks and
discussion threads, there is no place on screen that shows what is alive.
`ws-agent-list` is a tool the model calls; the owner has nothing but the
`ws: N pending question(s)` line from `260904`. The `260802` research already
reserved an "always-visible workflow surface" as expansion scope, with the
exact shape left to implementation time.

Pi's extension UI (0.84.4) offers no side panel: the TUI is a vertical stack.
The available always-visible slots are `ctx.ui.setWidget(key, lines | factory,
{ placement: "aboveEditor" | "belowEditor" })`, `ctx.ui.setStatus(key, text)`
(a short segment in the built-in footer), `ctx.ui.setFooter(factory)` and
`ctx.ui.setHeader(factory)` (full replacements of the built-in footer/header
components). A right-hand column would have to be drawn inside a widget's own
`render(width)` — feasible but it would push the chat narrower on every line.

## Decisions

- **One compact `belowEditor` widget, not a footer replacement.** The widget
  lists live agents and threads in one line each: `name · role
  (worker/execute/fork/thread) · state (running/awaiting
  approval/awaiting owner) · elapsed`, where `name` is the alias when set,
  else the title, else a shortened uuid (owner decision 2026-09-05, via
  `260905-feat-ws-pi-agent-alias-park-and-registry-cap`). Idle is not a row
  state: that ticket parks a child the moment it settles, so a finished
  child's row disappears when its `final` is delivered; dormant children are
  `ws-agent-list`'s business, not the widget's. `explore` leaves are not
  rows: they are synchronous tool calls outside the RPC registry (design
  review, 2026-09-05), so the role list has no `explore`. Rows are ordered
  `awaiting owner`, then `awaiting approval`, then `running` by elapsed
  descending; the cap is 5 with a `+N more` tail, and rows in the two
  awaiting states are never folded into the tail (they are the cues the
  owner must act on), so the cap only ever trims `running` rows. The widget
  disappears when nothing is live. `setStatus` additionally shows one segment
  (`ws: 3 agents · 1 question`, the question part only while a thread is
  pending) so the built-in footer stays informative when the widget is
  hidden; this segment owns the footer's agent count, and the sibling's
  goal-loop yield status (`260905-feat-ws-pi-push-only-child-reports` Phase
  2) writes its own key with yield wording only, not a second count. Rejected: `setFooter` replacement (drops Pi's
  own footer data: model, git branch, context usage) and a header panel
  (scrolls away). Rejected for now: a right-hand column (narrows every chat
  line; revisit if Pi grows a side-panel primitive).
- **The `260904` pending-question line merges into this widget.** Pending and
  open threads are rows of the same list (`awaiting owner` state carries the
  `/answer <id>` hint), so there is one ws panel above/below the editor, not
  two.
- **One row per child; a thread collapses onto its respondent.** Every open
  thread's respondent is also an RPC record (a fork-raised thread's task
  fork, a `lead-ask` thread's discussion fork), so `buildAgentRows` dedupes
  by agent id: a `threadBound` record renders once, as role `thread` when
  the thread is a `lead-ask` discussion and as its own role otherwise, with
  state precedence `awaiting owner` > `awaiting approval` > `running`. The
  `setStatus` count is the deduped row count.
- **Source of truth is the RPC registry plus the thread registry; no widget
  model.** The widget re-renders on registry transitions (spawn, settle,
  report, stop, thread open/close) and on a 10-second timer for the elapsed
  column (armed only while the widget has rows); `ws-agent-list` keeps
  returning the same data for the model. `elapsed` is time since the
  current turn started: one new record field `runStartedAt`, stamped by
  `promptAgent` at every prompt site (lead send, overlay line, nudge), is the
  clock; thread rows use `ThreadRecord.touchedAt`.
- **Lead only, TUI only.** Child processes and headless leads render nothing
  (`ctx.mode !== "tui"` guard, `isChildProcess` guard).

## Constraints

- Widget rows must respect `visibleWidth(line) <= width` like the overlay; the
  rendering helpers in `overlay-chat.ts` are reused, not duplicated.
- The widget must not flicker on every child `text_delta`; re-render only on
  state transitions and the timer tick.

## Spec Impact

`pi-adapter-runtime`: new anchor for the live-agent widget (placement, row
shape, states, hide-when-empty, `setStatus` segment) and an amendment to the
side-thread owner question surface anchor replacing the standalone pending
line with a row in this widget.

## Phases

### Phase 1: Live-agent widget

Add `agents-plugin-pi/src/agent-widget.ts` with pure row-building
(`buildAgentRows(records, threads, now)`) and the `setWidget`/`setStatus` IO
glue wired from `index.ts` on `session_start` (TUI lead only), subscribed to
registry transitions; replace `refreshPendingWidget` in `ask.ts` with the
merged widget; stamp `runStartedAt` in `promptAgent`. Tests: row shape and
states, name precedence (alias > title > short uuid), ordering and the cap
never folding an awaiting row, dedupe of a thread-bound record with state
precedence, hide-on-empty, width bound across 40/80/120, thread rows
carrying the `/answer` hint, the footer segment with and without a pending
question. Live
check: spawn two workers and a fork, open a thread, confirm the rows update
without a lead turn and vanish when everything settles.

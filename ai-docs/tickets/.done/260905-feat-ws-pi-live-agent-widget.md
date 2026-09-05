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
completed: 2026-09-05
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

### Result (07d24cdc) - 2026-09-05

Behavioral delta (commits `ae03bcc2`, `07d24cdc`):

- `agents-plugin-pi/src/agent-widget.ts` (new): pure `buildAgentRows`,
  `buildWidgetLines`, `buildStatusSegment`, `shouldArmAgentWidget`, and the
  `createAgentWidgetController` IO glue. Rows come from the union of the RPC
  registry and the thread registry: a pending or open thread with no
  thread-bound respondent (a `ws-ask` before `/answer`, or a fork-raised
  question whose respondent was revived dormant after a restart) stands as
  its own `thread · awaiting owner` row; a thread-bound respondent collapses
  the thread onto its own row. The `/answer <id>` hint and `touchedAt` clock
  follow the `awaiting owner` state for either origin; only the `thread` role
  label is lead-ask-only.
- `setWidget` receives the `(tui, theme) => Component` factory so
  `render(width)` sees the real terminal width; `buildWidgetLines` stays pure
  and width-agnostic. The repaint is guarded so a throwing surface loses one
  paint, not the process. The 10 s timer is armed only while rows exist and
  is stopped on `session_shutdown` and before re-arming on `/reload`.
- `spawner.ts`: `runStartedAt` stamped unconditionally in `promptAgent`;
  `agentWidgetRefreshRef` (same seam as `leadIdleRef`) fires on spawn, spawn
  failure, exit, stop, `agent_start`, settle and auto-park, and the
  gated-exec/report branch, so neither `spawner.ts` nor `ask.ts` imports the
  widget module.
- `ask.ts`: standalone `aboveEditor` pending-question widget removed; all six
  former `refreshPendingWidget` sites route through the shared refresh ref;
  headless `notify` baseline unchanged.
- `index.ts`: controller armed on `session_start` via `shouldArmAgentWidget`
  (lead-or-fork spawn role and TUI mode), stopped on shutdown.

Verification: `npm test` in `agents-plugin-pi` at `07d24cdc`, 725 passing,
0 failing (baseline 696). Reviews: partitioned correctness/fit/test on
`ae03bcc2` found one Critical (rows built from the RPC registry alone) and
four Important (fork-raised rows lacked the hint and thread clock, fixed
80-column width, unguarded timer repaint, lost TUI-only arming coverage), all
fixed in relay #1; a fresh Critical-scoped re-review on `07d24cdc` returned
clean. The fit Important (spec not updated in the implementer's range) was
resolved by the lead's doc pre-pass in the same branch.

Minor findings recorded, not acted on: `ws: 1 agents` is not singularized;
`ws-approve`, the `leadSend` thread-bound release, and the streaming
steer/followUp branch fire no refresh (the timer self-heals within 10 s);
the auto-park path fires the refresh twice; the arming gate uses the
lead-or-fork role check rather than the ticket's literal `isChildProcess`
wording, matching the existing push-renderer precedent.

Owner-run live check (outstanding): spawn two workers and a fork, open a
thread, and confirm the rows update without a lead turn, render at the real
terminal width, tick every 10 s, survive `/reload`, and vanish when
everything settles.

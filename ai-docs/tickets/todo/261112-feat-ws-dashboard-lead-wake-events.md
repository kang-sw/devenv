---
title: Show what woke the lead, and background/monitor counts, in the dashboard conversation
related:
  261112-feat-ws-mixed-version-compat-guards: new adapter event kinds and fields are cross-version surfaces; this ticket's additions must stay T0
---

# Show what woke the lead, and background/monitor counts, in the dashboard conversation

## Background

The dashboard conversation shows the lead's turns, but a turn that no person
started (a subagent's report or completion, a background shell exiting, a
Monitor event, mail, a scheduled wakeup) appears only as an origin-less turn
line ("turn · N tool calls"), and its cause is invisible. The user asked
(2026-11-12) for those causes to be drawn clearly in the conversation, and
for "N background" / "M monitors" badges right of the subagent badge.

A 2026-11-12 exploration of the feed found:

- The wslive adapter event kinds are reset, turn_start, turn_end, agent,
  permission, reply, user, status, compact; `OptionalKinds = [status,
  compact]` are negotiated through the intake result's `accepts:` line
  (`wslive/state.go:16-62`, `mcp/live_snapshot.go:487`). `turn_start`
  `origin` is validated to terminal, whiteboard, or mail.
- Claude mod: `userPrompt` (`agents-plugin/hooks/claude-mod/dashboard.ts:191-204`,
  called from `prompt.submit`, `register.ts:1505-1515`) drops every prompt
  whose origin is outside `PERSON_ORIGINS` (dashboard.ts:182), which includes
  harness-injected task notifications and other-session messages; the turn
  then starts with no origin. Subagent completion is inferred from
  `turn.complete` with an `agentId`. `tool.call` already receives call args
  (so `run_in_background` and Monitor calls are observable).
- Pi bridge (`agents-plugin-pi/src/dashboard-feed.ts`): extension pushes
  (`ws-background-task`, agent reports via `pushToLead`) are custom messages
  that never become `user` entries; `countBackgroundTasks()` in
  `background-tasks.ts` already returns `{timed, monitors}` but nothing in
  the feed reads it.
- Hosted runner (`wsagent/claude.go`): `background_tasks_changed` sets
  `Host.BackgroundTasks` (one count: shells, monitors, background subagents),
  which drives the existing `#bgbadge`; turns it did not start begin with no
  origin.
- Rendering: `turn` ring entries draw `.turnline` with an origin pill only for
  whiteboard, mail, or terminal (`dashboard.tmpl:197-199`). There is no entry
  kind that names a notification or its sender.

Mockup: the session scratchpad's `mock-conv-events/index.html` (section 2,
event entries) is the agreed look; its screenshot was posted on the board
topic `conv-events`.

## Decisions

- **Event display (board 348 a).** Mixed: completions and exits
  (subagent done/failed, background shell exit) render as a thin divider
  chip in the conversation; content-bearing causes (a subagent's report or
  message, a Monitor event, mail, a scheduled wakeup) render as a two-line
  card with a left accent, expandable to the full text, followed by a
  "↳ lead turn · N tool calls · time" line for the turn it caused.
  Rejected: all thin dividers (content hidden behind a click); all cards
  (completions would break the conversation flow).
- **Badges (board 347 a).** Background shells and monitors get separate pill
  badges, right of the subagent badge, in the existing floating strip between
  the conversation and the composer (the user likes that floating position
  as is). A zero count draws no badge. Counts come from the harness's own
  background tasks on Claude and from the ws Pi extension's background tasks
  on Pi. Rejected: one segmented control (each pill opens a different list).
- **Delivery is added where missing.** Each adapter that can observe a wake
  cause sends it: the Claude mod, the Pi bridge, and the hosted runner.
- **Compatibility tier: T0.** New causes travel as an optional event kind
  and new counts as optional fields, sent only when the daemon's `accepts:`
  line lists them, so an older daemon never receives them (AGENTS.md
  `### Compatibility Tiers`). A new `turn_start` origin value is not used for
  this, because an older daemon rejects an unknown origin.
- **Out of scope:** auto-collapsing the agents panel
  (`261112-feat-ws-dashboard-agent-panel-autocollapse`, board 349 a) and the
  strip's left/right arrangement (delegated separately, board 354).

## Open Decision Queue

Last ID: (361)

(360) [open] [technical] Carry a wake cause as a new optional event kind `wake` (cause kind, source label, one-line caption, full text) that the daemon turns into a ring entry, over new `turn_start` fields
- A separate kind is negotiated through the existing `accepts:` mechanism and can arrive before the turn it causes (the harness injects the notification, then the turn starts); new `turn_start` fields would tie cause and turn in one event that some adapters cannot assemble.

(361) [open] [technical] Carry background and monitor counts as optional fields on the existing `status` event (`background`, `monitors`), with the hosted runner splitting its one count by task type where stream-json exposes it, over a new event kind
- `status` already carries per-session live numbers and is already `accepts:`-gated; a count is state, not an occurrence.

## Constraints

- Applies `ai-docs/manuals/ws-mcp.md` (intake contract) and
  `ai-docs/manuals/wsflow-mirroring.md` (the Claude mod is mirrored byte
  for byte into `agents-plugin-wsflow/hooks/claude-mod`).
- Applies `ai-docs/manuals/release-compatibility.md`: every addition is T0.
- The Claude Code mod engine's full hook list is not vendored here; which
  `classic.*` events exist beyond those `register.ts` uses is unverified, so
  the Claude side builds on `prompt.submit`, `tool.call`, and
  `turn.complete` unless a probe shows a better hook.
- Heavy Go build/test runs through the project's remote build wrapper
  (machine note `remote-go-build`).

## Phases

### Phase 1: Wake events end to end

- wslive: the optional wake kind and its ring entry; intake `accepts:`
  lists it.
- Claude mod: classify non-person prompts in `prompt.submit` (task
  notification: subagent report/complete/fail, background shell exit,
  Monitor event; other-session message; scheduled wakeup) into wake events
  with a caption, instead of dropping them; mirror to wsflow.
- Pi bridge: send wake events for extension pushes (background task
  results, agent reports, mail wakes).
- Hosted runner: send wake events where stream-json exposes the cause.
- Dashboard: render dividers and cards per the display decision, and the
  "↳ lead turn" line on the turn that follows a wake entry.
- Verification: wslive unit tests for the kind and accepts gating; mod and
  Pi feed tests for classification (each cause kind, and a person prompt
  still producing a `user` entry); a page render test for divider and card
  markup; a headless visual check against the mockup.

### Phase 2: Background and monitor badges

- `status` count fields per (361); Claude mod counts `run_in_background`
  shells and Monitor tasks from `tool.call` and their completion
  notifications; Pi samples `countBackgroundTasks()` at flush; the hosted
  runner splits its count.
- Dashboard: separate background and monitor pills right of the subagent
  badge; zero draws nothing; the existing `#bgbadge` becomes the background
  pill.
- Verification: count tests per adapter (start, finish, abort), page render
  test for zero/non-zero pills.

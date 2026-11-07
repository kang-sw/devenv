---
title: "Dashboard: interrupt the lead's running turn from the session page"
parent: 261107-epic-ws-dashboard-messenger-round2
related:
  261107-feat-ws-dashboard-steer-promotion: precedent for an inbox flag, a drain mode, a wait mode, and both host adapters acting mid-turn
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 36ed1665e93f0191
sage-review-completeness-reviewed: 36ed1665e93f0191
completed: 2026-11-08
---

# Dashboard: interrupt the lead's running turn from the session page

## Background

The live session page has no way to stop the lead once a turn runs, so a
mistyped message cannot be cut off from the page (user, 2026-11-07: "뭔가
잘못 입력했을 때 interrupt 수단이 없는 게 좀 불편하네요"). The user asked for a
round interrupt badge with red text that rises on the border between the
conversation and the composer while the lead is working ("lead 생각 중 같은
거 빙글빙글 돌아갈 때 interrupt 버튼 하나 동그랗게 올라오면 좋을 거 같아요.
위치는 대화창이랑 입력창 사이에 있는 경계선에 빨간 글씨 뱃지가 하나 올라오는
느낌으로").

A feasibility probe (2026-11-07, not run live) found:

- Claude: the mod kit has `$.turn.abort({ turnId })`, which cancels the main
  loop's running model turn and its running tools and rejects when the id is
  not the running turn's. Unlike Esc it leaves no interruption marker (only
  the aborted tool rows draw `Interrupted`), and per the mod-kit doc the
  prompt the plugin submits next is the context; whether a later submit is
  dropped the way it is after Esc is not verified live. The id
  is the one the main loop's `turn.start` hook hands the mod
  (`e.agentId === undefined`); the mod does not store it today
  (`agents-plugin/hooks/claude-mod/register.ts`, `turn.start` handler).
- Pi: `ExtensionContext.abort()` ("Abort the current agent operation") in
  the installed `@earendil-works/pi-coding-agent`; the adapter does not call
  it today. In TUI mode it runs the TUI's abort handler
  (`interactive-mode.js` `restoreQueuedMessagesToEditor({abort: true})`),
  which clears Pi's queues: untaken text steers move into the Pi terminal
  editor and queued custom messages (push batches sent through
  `sendMessage` as steers) are dropped. Without that handler a plain
  `session.abort()` leaves the queues and `continue()` starts a new run from
  them (`agent-session.js` `_handlePostAgentRun`). `ExtensionContext`
  exposes no queue withdrawal, only `abort()` and `hasPendingMessages()`.
- Dashboard: the page reaches the host only through the per-session inbox
  (`agents-plugin-tool/internal/wsdashboard/inbox_routes.go`,
  `agents-plugin-tool/internal/wsinbox`) and the
  adapter-only `dashboard.inbox_drain`; the lead's phase (`thinking`,
  `replying`, `tool`, `permission`, `compacting`, `idle`) is already in the
  live snapshot (`agents-plugin-tool/internal/wslive`).
- Both APIs stop the main loop only; subagents keep running.

## Decisions

Board IDs refer to the 2026-11-07 session whiteboard topic
"대시보드 interrupt 버튼".

- **Round-2 child, run at once** ((257) ok). Rejected: a standalone ticket
  outside the epic.
- **Held messages return to the composer** ((253) a). Interrupting returns
  every held (undrained) dashboard message, steered or not, to the composer
  with its text and image chips, as the existing cancel does, so the user can
  fix and resend it. Rejected: submitting held messages right after the
  interrupt ((253) b); discarding them ((253) c).
- **One click** ((254) a). The badge interrupts on the first click, with no
  confirm step. Rejected: a second "click again to interrupt" tap ((254) b).
- **Channel and host actions** ((255) ok). A new `POST /s/<rid>/interrupt`
  writes an interrupt entry into the session inbox. The host adapter handles
  it as soon as its wait wakes, without waiting for a tool boundary or the
  turn end. Claude stores the main loop's `turn.start` turn id and calls
  `$.turn.abort` with it; Pi calls `ctx.abort()`. An interrupt entry never
  merges with messages. A rejected abort (the turn already ended) is ignored.
  Rejected: waiting for a tool boundary, which is useless during long
  thinking.
- **When the badge shows** ((256) ok). Visible while the lead's phase is
  thinking, replying, tool, or permission; hidden while idle or compacting
  (compaction is not the main turn). After a click it shows a disabled
  "interrupting" state until the turn ends, so a second click cannot cut the
  next turn.

- **Atomic claim of held messages** ((259) ok). The interrupt route claims
  every held entry under the daemon's inbox lock in the same step that
  writes the interrupt entry, and returns their text and image refs in its
  response for the page to restore into the composer (as a cancel's response
  does). Rejected: the page cancelling each held bubble before interrupting,
  since a turn-end flush in between would submit what (253) a returns.
- **No stale interrupt** ((260) ok). An interrupt entry records its creation
  time; the adapter aborts only a main turn that started before it. An entry
  older than the running turn, or drained while idle, is consumed and
  dropped. Rejected: aborting whatever turn runs at drain time, which lets a
  click that lands as a turn ends cut the next turn.
- **Delivery follows steer promotion** ((261) ok,
  261107-feat-ws-dashboard-steer-promotion). `dashboard.inbox_drain` gains a
  mode that claims only interrupt entries; `ws-mcp mailbox wait` gains a mode
  that wakes on an interrupt entry mid-turn, armed by both adapters during a
  turn (a held follow-up still does not wake it); the drain tool's minimum
  version is raised in the runtime.json files of all three packages that list
  it (ws, wsflow, and Pi; `agents-plugin/runtime.json#L19`,
  `agents-plugin-wsflow/runtime.json#L22`, `agents-plugin-pi/runtime.json#L19`,
  each `>=0.48.11-dev <0.49.0` today) so an adapter never sends the new mode
  to a ws-mcp that predates it. A full (turn-end)
  drain never returns an interrupt entry as message text.
  Further refinements (lead, auto-confirmed from design and completeness
  review):
  - Only the interrupt drain acts on an interrupt entry, and only the full
    drain (turn end or idle) drops one; the steer-only drain, which Claude
    runs at every main-thread tool boundary and Pi under `--board-steer`,
    leaves it in place so a tool boundary between the click and the
    interrupt wake cannot swallow the interrupt. The full drain drops it and
    never writes it to `drained/`, so it never
    holds a `DrainedCap` slot, never shows as a sent mark, and never keeps an
    idle board wait waking. An entry no drain takes is dropped by the inbox's
    existing sweep. The page never renders an interrupt entry as a bubble,
    and the cancel and steer routes refuse one.
  - The interrupt wake combines with every in-turn arm: Claude's flagless
    re-arm after a board wake, and Pi's `--board-inbox` and `--board-steer`
    arms (which cannot be combined with each other today). The wait result
    reports it under its own `kinds` value, so an interrupt is never read as
    an ordinary held board message; the per-wait AbortController re-arm rule
    (001ba50c) applies.
  - Mode, flag, and response names follow the precedents: the steer-only
    drain mode, `--board-steer`, and the cancel route's response shape.
  - The creation time and the turn start compare on the same host's wall
    clock (daemon and adapter run on one machine).
  - The raised minimum version is the version this change ships in.
  - Restoring several held entries joins their texts as paragraphs in send
    order and renumbers image refs across the entries and any existing
    draft, as `wsinbox.Merge` does on the server.
- **Turn-end submits after an interrupt are held** ((265) a; raised by
  design review). After a dashboard interrupt, what the adapters would
  submit at that turn's end (mail queued during the turn, a steered row the
  turn never read, carried steered texts) does not start a turn: it takes
  the existing "submit refused, append as a row" fallback the Claude
  watcher already uses after Esc, so it reaches the model with the user's
  next input. On Claude, `turnCompleted` learns the turn was interrupted and
  the mail delivery, the board flush, and `carrySteer` take that fallback.
  On Pi ((269) a) the interrupt follows Pi's own Esc behavior: `ctx.abort()`
  in TUI mode moves untaken text steers, the dashboard steer included, into
  the Pi terminal editor, so they go out with the user's next terminal input;
  what the adapter still owns (its `carry` and the push FIFO's held items) is
  queued with `sendMessage(..., {deliverAs: "nextTurn"})` instead of
  started; and push batches the adapter steered but the run did not take,
  which the host's queue clear drops, are tracked by the adapter and
  re-sent whole as `nextTurn` after the abort so nothing drained is lost (a
  `ws-push-batch` snapshots the held queue, so it may mix mail with child
  reports, questions, approvals, and goal replacements; none is filtered
  out). The held FIFO, its batch sends, and the push wake line live in
  `agents-plugin-pi/src/spawner.ts` (`heldPushQueue`, `submitHeldPushBatch`,
  `admitPush`, `requestPushWake`); the wake line must not start a turn after
  the interrupt. As on Claude, the adapter keeps an interrupted flag from
  before `ctx.abort()` until the next run starts, so a `steerBoard` already
  in flight that pushes to `carry` after the abort, and the following
  `leadSettled` drain, send that text as `nextTurn` instead of injecting a
  turn. `nextTurn` items ride only on a `prompt()` turn (terminal input or
  the page's `sendUserMessage` inject); a custom-message turn such as idle
  mail after the interrupt runs first and they wait for the user's next
  input, and a session reload or `/new` drops them; a code-site comment
  records this ordering. Whether
  Pi signals that a pushed batch was taken is unverified; the worker
  establishes the signal and, without one, re-sends every batch pushed since
  the run's last observed message (a duplicate is preferred over a loss). In
  a non-TUI Pi mode the abort keeps the queues and Pi's `continue()` starts a
  new run from them; the adapter cannot withdraw queued items, so that mode
  is a recorded limit, out of scope. Rejected for Pi: the adapter tracking
  and re-sending every queued text steer too ((269) b), which duplicates the
  host's editor restore. A message the user sends afterwards (page or terminal)
  starts a turn normally. Refinements (lead, auto-confirmed from delta
  design review): at an interrupted turn's end the board flush holds only
  the stranded and carried steered texts; held messages the full drain newly
  claims were sent after the click (the route claimed every earlier one) and
  still submit. The interrupted mark is set before `$.turn.abort` is called,
  since `turn.complete` can fire before the abort settles, and lasts until
  the next main-thread `turn.start`, so a late steer drain or `carrySteer`
  after the turn end still holds. Items already held when an abort is
  rejected stay held. Mail arriving while idle after the interrupt is
  outside this decision and submits normally. Rejected: returning claimed steered texts to the
  composer ((265) b), which needs a new adapter-to-page path; letting them
  start the next turn ((265) c), which resubmits an already steered
  mistyped message the moment the turn is cut.
- **Epic exception** ((258) ok). The round-2 epic's "no host adapter change"
  decision lists this ticket as its third exception.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Out of scope: interrupting subagents or teammates; interrupting compaction;
  a keyboard shortcut.
- Strings the page shows go through the i18n tables; every table carries the
  new keys (`agents-plugin-tool/internal/wsi18n/keys_test.go`).
- Read `ai-docs/manuals/wsflow-mirroring.md` before editing the Claude mod
  under `agents-plugin/hooks/`, and `ai-docs/manuals/ws-mcp.md` before
  editing `agents-plugin-tool/internal/mcp/` (AGENTS.md Implementation
  Conventions).

## Prior Decisions

- 261107-epic-ws-dashboard-messenger-round2 (2026-11-07, Cross-Child Decisions): "No host adapter change in this round, except steer promotion ... The interrupt badge is the third: both adapters abort the main turn on an interrupt inbox entry" — bearing: supports
- 261106-feat-ws-dashboard-claude-adapter (2026-11-07, 001ba50c): "Re-arm is a per-wait AbortController linked to the loop signal: an own abort re-arms the loop instead of ending it; boardChanged() re-arms a running wait" — bearing: constrains
- 261106-feat-ws-dashboard-claude-adapter (2026-11-07, 001ba50c): "A stop during a turn-end drain records the claimed messages and the waiting mail instead of submitting them into the next conversation." — bearing: constrains
- 261018-feat-claude-mod-mid-turn-steer-delivery (2026-10-18, Decisions): "Main-thread turn running is tracked from turn.start / turn.complete with agentId === undefined" — bearing: supports
- 261018-feat-claude-mod-mid-turn-steer-delivery (2026-10-18, Result 7fe04e8e6): "an interrupt needs no new turn ... the host forbids plugin-started turns after a stop anyway" — bearing: constrains
- d34ef5c2 (2026-11-07, commit): "A carried steer whose row was refused after the turn ended (an interrupt during the hook) is flushed at once rather than waiting a turn." — bearing: constrains
- 261107-feat-ws-dashboard-composer-image-attach (2026-11-07, 0801c68d): "Decoding and sniffing run before the inbox lock; sweep, ref checks, the merged-cap count, file writes, and Send run under it." — bearing: constrains
- 260908-feat-ws-pi-claude-code-lead-provider (2026-09-08, Decisions): "options.signal abort -> Query.interrupt(); the current Pi turn ends with stopReason aborted" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/ (inbox_routes.go, assets/dashboard.js, dashboard.tmpl, dashboard.css), internal/wsinbox/inbox.go, cmd/ws-mcp/mailbox.go, internal/mcp/dashboard_inbox.go, internal/wsi18n, agents-plugin/hooks/claude-mod/register.ts and its wsflow mirror, agents-plugin-pi/src/mailbox-waiter.ts, agents-plugin-pi/src/spawner.ts, three runtime.json files |
| scope.surface | cross-module | new POST /s/<rid>/interrupt route, dashboard.inbox_drain mode, ws-mcp mailbox wait mode, and both host adapters change together |
| scope.new_public_symbol | yes | interrupt route, interrupt drain mode of dashboard.inbox_drain, interrupt wait flag of ws-mcp mailbox wait |
| scope.new_type_contract | yes | interrupt inbox entry with creation time; route response carrying held text and image refs |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/inbox_routes_test.go, internal/wsinbox/inbox_test.go, internal/wsi18n/keys_test.go, agents-plugin/hooks/claude-mod/dashboard-engine.test.ts, agents-plugin-pi/test/mailbox-waiter.test.ts, agents-plugin-pi/test/spawner.test.ts |
| complexity.reuse_points | confirmed | steer-only drain (DrainSteer, steer_only arg in internal/mcp/dashboard_inbox.go) and --board-steer wait mode in cmd/ws-mcp/mailbox.go, cancel route and sameOrigin in inbox_routes.go |
| complexity.side_effect_risk | high | aborts a running host turn and claims held entries atomically under inboxMu; a race at turn end could cut the next turn or lose held messages |
| risk.correctness | high | stale-interrupt timing, atomic claim, and mid-turn wake across two hosts; the mod kit's turn.abort was not found in the tree (host-provided) |
| risk.fit | moderate | follows the steer-promotion precedent, but adds a third adapter-visible mode and a stale-context concern on Pi |
| risk.test | high | the live two-host abort and turn-end race checks are manual and cannot be fully automated |
| risk.security_or_contract | moderate | new write route behind Origin check, and a runtime.json minimum-version contract across ws, wsflow, and Pi |

## Phases

### Phase 1: Interrupt badge, route, and abort on both hosts

One behavior slice: the page badge and route, the inbox entry, the drain and
wait modes, and both adapters land together.

- **Page.** A round badge with red text on the border between the
  conversation and the composer, visible per "When the badge shows". One
  click posts the interrupt, switches the badge to its disabled
  interrupting state, and restores the returned held messages into the
  composer (text and image chips, as cancel does). A refused request (409
  without a live adapter, or another error) restores the badge and shows the
  page's usual error notice. Badge strings in every i18n table. Works in the
  light and dark themes and at narrow widths.
- **Daemon.** `POST /s/<rid>/interrupt` behind the same Origin check and
  live-adapter requirement as the other inbox routes; the atomic claim and
  interrupt-entry write; the entry is never a merge target and never counts
  toward the attachment cap.
- **Claude adapter.** Store the main loop's turn id at `turn.start`
  (`agentId === undefined`) and clear it at `turn.complete`; on an interrupt
  entry, abort with `$.turn.abort` when the turn started before the entry,
  catching a rejection; arm the interrupt wait mode during a turn; at that
  turn's end, append queued mail and steered texts as rows instead of
  submitting them (the held turn-end decision). Mirror to
  wsflow per the mirroring manual.
- **Pi adapter.** On an interrupt entry, call `ctx.abort()` when the run
  started before the entry, with a context that is not stale across session
  reload; arm the interrupt wait mode during a run; after the abort, leave
  the host's restore of text steers to the editor, queue the adapter's own
  carry and held FIFO items as `nextTurn`, and re-send untaken mail push
  batches as `nextTurn` ((269) a).

Verification: Go tests for the route (Origin, 409, the atomic claim
returning held text and image refs, the entry written), the inbox (an
interrupt entry is not merged into, is consumed and dropped by the full
drain without a `drained/` record, is skipped by the attachment cap, is the
only thing the interrupt drain takes, is left in place by the steer-only
drain, and is refused by cancel and steer),
and the wait mode (wakes on an interrupt entry mid-turn under every in-turn
arm with its own `kinds` value, not on a held follow-up);
an i18n key test pass; Claude mod engine tests for the turn-id capture, the
abort call, the stale-entry drop, a rejected abort being ignored, and an
interrupted turn's end appending queued mail and steered texts as rows
instead of submitting them while still submitting a held message drained
fresh at that turn end; Pi tests for the abort call, the stale-entry
drop, the adapter's carry and held FIFO items queued as `nextTurn` after an
abort, an untaken push batch re-sent whole as `nextTurn` after an abort,
no push wake turn after the interrupt, and a carry made by an in-flight
steer after the abort sent as `nextTurn`; `node --check` on changed
page scripts; a headless Chromium check that the badge appears only in the
working phases, renders no bubble for an interrupt entry, and restores
several held messages with renumbered images; and a live check on Claude and
Pi that a click during long thinking stops the turn within a few seconds,
returns a held message to the composer, that a message resent from the page
after the interrupt starts a new turn (escalate to the lead if it does not),
and that a click as the turn ends does not cut the next turn.

### Result (07674bfaf) - 2026-11-08

Landed in d480394df (Go), 003f3050e (Claude mod + wsflow mirror), 6b9e08afa
(Pi), 4dc309263 (page), 07674bfaf (help text).

- **Daemon / inbox.** `wsinbox.Entry.Interrupt` (carries only `SentAt`, the
  click's epoch ms); `wsinbox.Interrupt` claims every held message (steered
  or not) and writes the entry under `inboxMu`, putting claims back when the
  write fails; `DrainInterrupt`, `MessageNames`, `InterruptNames`. The full
  drain deletes an interrupt entry without a `drained/` record, the
  steer-only drain leaves it, `MergeTarget`/`Held` skip it, cancel and steer
  refuse it. `POST /s/<rid>/interrupt` answers
  `{"held":[{id,text,chips,images,nums}]}` oldest first (cancel's shape,
  shared helper). `dashboard.inbox_drain` gains `interrupt_only` (error with
  `steer_only`; records carry `interrupt: true`, `sent_at`).
  `ws-mcp mailbox wait --board-interrupt` combines with `--board-inbox`,
  `--board-steer`, or stands alone; the result gains `interrupt` and the
  `interrupt` kind; `board` never counts interrupt entries.
- **Claude mod.** Main-thread `turn.start` stores the turn id and start time;
  the wait carries `--board-interrupt` whenever board submission is on; an
  interrupt drain aborts only a running main turn that started before the
  newest `sent_at`, setting the interrupted mark before `$.turn.abort`
  (rejection logged and ignored); the mark lasts until the next main
  `turn.start` (or a stop). At an interrupted turn end queued mail and
  carried steer texts are appended as rows, stranded steer rows are not
  resubmitted, and messages the turn-end drain newly claims still submit.
- **Pi.** The run start is stamped at owner-lead `agent_start`; a context
  ref refreshed at `session_start`/`agent_start` feeds `ctx.abort()`. While
  the mark stands (abort to next `agent_start`) the push FIFO is held, no
  `agent_end` boundary batch or wake line starts a turn, carried steer texts
  are queued as a `nextTurn` custom message, and at the interrupted settle
  the pushes the run did not take (tracked by `details` identity through
  `message_start`, verified in the installed pi-coding-agent) and then the
  FIFO are queued as `nextTurn`. `pi.dashboard_enabled` description updated.
- **Page.** Round red badge on the conversation/composer border, shown only
  while live and the convlive `data-phase` is thinking, replying, tool, or
  permission; one click posts, holds a disabled state until the phase leaves
  that set, and restores held messages through `restoreHeld` (shared with
  cancel: paragraphs in send order, images renumbered above the draft's). A
  refusal restores the badge and shows `js.interrupt.fail`. Six `js.interrupt*`
  keys in all 11 tables.

Verification: `go build ./...`, `go vet`, `GOOS=windows go vet` clean; `go
test ./internal/wsinbox ./internal/wsdashboard ./internal/mcp
./internal/wsmailbox ./cmd/ws-mcp ./internal/wsrsrc ./internal/wsi18n` ok
(TestStickyFallbackPort flaked once in a leaf run, file untouched); `claude
plugin test` 677/677 and `claude plugin validate` on ws and wsflow; Pi
`node --test` mailbox-waiter 98/98, spawner interrupt tests 7/7, full
`npm test` no failures beyond the pre-existing baseline; `node --check
dashboard.js`; headless Chromium against a test daemon (badge only in the
working phases, no bubble for the entry, three held messages restored with
renumbered images against a draft with an image, disabled state until idle,
409 restores the badge, light/dark at 1280/390/320 px). Lite review: no
Critical or Important findings.

Decisions taken in this phase:
- runtime.json unchanged: `>=0.48.11-dev` already is the ships-in version
  (0.48.11 unreleased).
- `--board-interrupt` is armed whenever board submission is on, idle
  included, so an idle-armed wait spanning into a turn cannot miss the
  interrupt; an idle wake consumes and drops the entry.
- No entry sweep exists; the full drain drops an interrupt entry no
  interrupt drain took.
- Pi re-sends untaken pushes at the interrupted run's settle, not at the
  abort: a non-TUI session keeps its queues and its continuation would take
  them, so an abort-time re-send would duplicate.
- Pi's ordering note corrected: idle mail after an interrupt wakes through
  the push wake line, a `prompt()` turn, so `nextTurn` items ride it; only a
  turn started by a custom message alone carries none, and a reload or `/new`
  drops them.

Open (not verified here): the live two-host check (click during long
thinking stops the turn, held message returns, a resent message starts a
turn, a click as the turn ends does not cut the next) is manual. Minor review
notes left: Claude `wakeKinds` reads unreadable output of an interrupt-armed
wait as `both` without the interrupt (delayed, not lost; Pi counts it); in
the Claude notice drain mode a notice held at an interrupted turn end leaves
the loop suspended until the model's recv, so a page message sent after the
interrupt waits for terminal input (same gap Esc has).

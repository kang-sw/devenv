---
title: "Dashboard: promote a held message to steering"
parent: 261107-epic-ws-dashboard-messenger-round2
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 2f337dfa2fc657d8
sage-review-completeness-reviewed: 2f337dfa2fc657d8
---

# Dashboard: promote a held message to steering

## Background

A message sent from the live session page while the lead is mid-turn is held
and delivered when the turn ends (follow-up). In the terminal the user can
steer: a message typed mid-turn reaches the model at the next tool boundary.
The user wants the page to default to follow-up but to let them promote a
held message to steering with a button (2026-11-07: "제가 입력한 메시지가
기본적으로는 follow up으로 들어가되, 제가 특정 버튼으로 steering 시점으로
승격시키는? 그런 수단이 있으면 좋겠어요").

Both hosts already have the mechanism:

- Claude: the mod's mailbox steer appends mail queued mid-turn with
  `$.session.append` at the next main-thread tool boundary (the `tool.call`
  hook in `agents-plugin/hooks/claude-mod/register.ts`).
- Pi: `pi.sendMessage(..., {deliverAs: "steer"})`, already used for pushes
  (`PushDeliverAs` in `agents-plugin-pi/src/spawner.ts`).

## Decisions

- **In this epic, as an adapter-touching child** (user, 2026-11-07, option (a)
  of (29)): the epic's "no host adapter change" cross-child decision exempts
  this ticket. Rejected: deferring to a later epic.
- **Promotion is a button on a held bubble** (user, 2026-11-07, "(30) ok"): the
  held message is marked for steering and the adapter injects it at the next
  main-thread tool boundary; when the turn ends with no boundary in between,
  it is delivered as a follow-up as today. Claude uses the mailbox-steer
  `$.session.append` path; Pi uses `deliverAs: "steer"`. Rejected: a send-time
  mode toggle - the request is "follow-up by default, promote by button", and
  the button also promotes a message already held.
- **Steer route and flag** (user, 2026-11-07, "(39) ok"): the button calls
  `POST /s/<rid>/steer {id}` (same Origin check, 409 without a live adapter),
  which writes the held entry under a fresh name with `steer: true`; a
  steered entry is no longer a merge target, so a later send starts a new held
  entry. Rejected: flagging in place - inbox entry files are never rewritten
  (5a0a0a14), and keeping later sends out of a steered entry stops a promoted
  message from absorbing text the user did not promote.
- **Steer-only drain** (user, 2026-11-07, "(40) ok"): `dashboard.inbox_drain`
  gains a `steer_only` argument that drains only steer-marked entries; the
  turn-end drain still drains everything, which is the follow-up fallback.
  Rejected: a separate steer queue directory - one inbox and one claim
  protocol.
- **Injection per host** (user, 2026-11-07, "(41) ok"): Claude - at each
  main-thread tool boundary while the dashboard feed is active, the mod calls
  the steer-only drain and appends any returned text with `$.session.append`,
  as mailbox steer does. Pi - the mailbox waiter's board wake
  (`agents-plugin-pi/src/mailbox-waiter.ts`, wired in `index.ts`) drains
  steer-only on a steer-marked entry and sends it with `deliverAs: "steer"`.
  Rejected: pushing from the daemon - it has no channel into either host; the
  adapters already pull through the drain. A Claude steer-only drain call per
  main-thread tool boundary is far rarer than the streaming intake calls, so
  its cost needs no gating.
- **Pi mid-turn wake on a steer entry** (lead, 2026-11-07, auto-confirmed
  from fact population): `ws-mcp mailbox wait` gains a steer-board mode that
  wakes only on a steer-marked inbox entry, and the Pi waiter arms it during a
  turn where it now re-arms without `--board-inbox`
  (`mailbox-waiter.ts#L224-L232`), so a held follow-up still does not wake it
  mid-turn. Rejected: polling the steer-only drain on a timer - it adds
  latency and an MCP call per tick, while the wait already peeks the inbox.
- **Pi mid-run steer drain safety** (lead, 2026-11-07, auto-confirmed from
  fact population): the steer-only drain runs only while the extension's run
  flag is set and no compaction holds the session, where `deliverAs: "steer"`
  queues into the running agent. This narrows the idle-only drain rule of
  0d680abd ("a drained entry that fails to inject is lost") to non-steer
  drains; the worker verifies how Pi handles a steer sent as the run ends and,
  if it would be dropped, delivers that text through the follow-up send path
  instead, so a drained steer entry is never lost. Rejected: draining steer
  entries only when idle - that is the follow-up path and defeats promotion.
- **A steered entry stays held until claimed** (lead, 2026-11-07,
  auto-confirmed from fact population; follows "held means undrained" of
  261106-feat-ws-dashboard-browser-interaction): until an adapter drains it, a
  steered entry can still be cancelled like any held entry; its bubble shows a
  steer marker and no steer button, and it accepts no merge. Rejected:
  locking a steered entry against cancel - nothing has reached the model yet.
- **A steered message is recorded in the conversation** (lead, 2026-11-07,
  auto-confirmed from design review): neither injection path records a user
  entry today - Claude's feed records user messages only on `prompt.submit`
  or the whiteboard submit path, not on `$.session.append`, and Pi's feed
  records only role "user" messages, which a `deliverAs: "steer"` custom
  message is not. Each host records the steered text as a whiteboard-origin
  user entry in the conversation ring at its injection point (on Pi,
  preferably by injecting it as a user message with steer delivery, if Pi
  accepts that), so the message does not vanish from the page when its held
  bubble clears. Rejected: leaving the record to the turn-end path - a
  steered entry is already drained by then.
- **No hot loop while a steer entry cannot be drained** (lead, 2026-11-07,
  auto-confirmed from design review): the wait only peeks, so while a
  steer-marked entry exists and the drain is refused (a compaction hold, or
  between run end and settle) a re-armed steer-mode wait would wake at once.
  The Pi waiter treats it as it treats a held follow-up today: it leaves the
  entry, re-arms without steer mode, and restores steer mode when draining
  is allowed again. Rejected: a backoff on the immediate wake - it still
  spawns a wait process per tick.
- **No loss after the claim on Claude** (lead, 2026-11-07, auto-confirmed
  from design review): if `$.session.append` is refused after the steer-only
  drain claimed the entry, the mod keeps the text and submits it through the
  turn-end submit path, matching the Pi guarantee. Rejected: the current
  `keepBoard` log-only fallback - it drops the message.
- **Version gate for `steer_only`** (lead, 2026-11-07, auto-confirmed from
  design review): the drain tool's input schema is open, so an older ws-mcp
  ignores `steer_only` and drains everything, which would append held
  follow-ups as steers. Raise the minimum version for `dashboard.inbox_drain`
  in both runtime.json files (ws and wsflow) so an adapter never sends
  `steer_only` to a ws-mcp that predates it. Rejected: echoing the mode in
  the response - the adapter would learn of the full drain only after the
  entries are claimed.
- **Promotion overtakes held items** (lead, 2026-11-07, auto-confirmed from
  review; a consequence of "Promotion is a button on a held bubble"): a
  promoted message may reach the model before earlier held dashboard
  follow-ups and held mail pushes; on Pi it is sent directly, not joined to a
  held push batch (08c3eba1 keeps mail steers behind held mail). Rejected:
  queuing it behind held items - that is the follow-up path.
- **Steer rewrite under the inbox lock** (lead, 2026-11-07, auto-confirmed
  from review): the steer route claims the original held file and writes
  the fresh-name steered entry in one step under the daemon's inbox lock, the
  same lock the attachment sweep uses, and copies the entry's image paths
  (261107-feat-ws-dashboard-composer-image-attach). `Send`'s merge-target
  choice and the attachment cap count skip steered entries. The brief claim
  window (a hidden claim file in neither the held nor the drained list) is
  one more instance of the trim-time gap the image-attach ticket accepts.
  The wait flag follows `--board-inbox` naming (for example
  `--board-steer`).

## Constraints

- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Decisions

- 261106-feat-ws-dashboard-browser-interaction (2026-11-06, Decisions): "Held means undrained; claims are atomic. A message that arrives while the session's turn runs is held and submitted when the turn ends. Held means left undrained in the inbox: the adapter drains only at turn end, so merge and cancel act on the entry until then." — bearing: constrains
- 261106-feat-ws-dashboard-browser-interaction (2026-11-06, Decisions): "The drain tool is adapter-only. Like the intake tool, its short description marks it adapter-only, since a model call would claim a held message that is then never injected." — bearing: constrains
- 0d680abd (2026-11-07, commit): "Turn state is the extension's own agent_start..agent_settled flag plus isOwningAgentIdle (compaction hold)... a drained entry that fails to inject is lost, so the drain only runs when both say idle." — bearing: contradiction-candidate
- 261106-feat-ws-dashboard-claude-adapter (2026-11-06, Decisions): "Hold until turn end. A message that arrives while the turn runs is left undrained in the inbox and submitted by the watcher's turn-end flush, so merge and cancel act on it until then. Because `mailbox wait` only peeks, the adapter re-arms it without the inbox flag while an entry is held" — bearing: constrains
- 261018-feat-claude-mod-mid-turn-steer-delivery (2026-10-18, 7fe04e8e): "The flush hook is a catch-all tool.call registered first, so it wraps the mod-served tools too... It awaits the append before the tool result returns... It acts only on next.origin.plugin === 'engine'" — bearing: supports
- 260911-feat-ws-pi-held-push-batch-delivery (2026-09-13, 08c3eba1): "a steer arriving after a held follow-up could overtake it because Pi drains steering before follow-ups; any existing held prefix now causes later pushes to join the same boundary batch." — bearing: constrains
- 261106-epic-ws-dashboard-serve-daemon (2026-11-06, Cross-Child Decisions): "The daemon writes each sent message as one file in a per-session board-answer inbox on disk... outside the mailbox store." — bearing: supports
- 261107-epic-ws-dashboard-messenger-round2 (2026-11-07, Cross-Child Decisions): "No host adapter change in this round, except steer promotion: everything reaches the session through the existing inbox drain as text." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsinbox/inbox.go, agents-plugin-tool/internal/wsdashboard/inbox_routes.go, agents-plugin-tool/internal/wsdashboard/pages.go, agents-plugin-tool/internal/wsdashboard/assets/dashboard.js, agents-plugin-tool/internal/mcp/dashboard_inbox.go, agents-plugin/hooks/claude-mod/register.ts (plus wsflow mirror), agents-plugin-pi/src/mailbox-waiter.ts and index.ts |
| scope.surface | cross-module | new HTTP route, new MCP tool argument, wsinbox entry schema, two host adapters |
| scope.new_public_symbol | yes | POST /s/<rid>/steer route; dashboard.inbox_drain steer_only argument; wsinbox entry steer field |
| scope.new_type_contract | yes | wsinbox.Entry steer flag; dashboard.inbox_drain argument schema at agents-plugin-tool/internal/mcp/server.go#L4940 |
| scope.test_surface | existing | agents-plugin-tool/internal/wsinbox/inbox_test.go, agents-plugin-tool/internal/wsdashboard/inbox_routes_test.go, agents-plugin-tool/internal/mcp/dashboard_inbox_test.go, agents-plugin/hooks/claude-mod/dashboard-engine.test.ts, agents-plugin-pi/test/mailbox-waiter.test.ts |
| complexity.reuse_points | confirmed | watcher toolCompleted in the tool.call hook (register.ts#L1177), wsinbox.Drain, Pi sendBatchToLead steer path (index.ts#L995-L1000) |
| complexity.side_effect_risk | high | a mid-turn drain claims entries before injection, so a failed inject loses the message, and the Pi wait does not wake on a held entry during a turn |
| risk.correctness | high | claim-then-inject race at a tool boundary, a steered entry no longer merge or cancel target, ordering against earlier held follow-ups |
| risk.fit | high | Pi waiter re-arms without --board-inbox during a turn (mailbox-waiter.ts#L224-L232), so how a steer entry wakes it is unsettled |
| risk.test | high | two host adapters need a live check that engine tests cannot cover |
| risk.security_or_contract | moderate | new write route behind the same Origin check; adapter-only drain tool gains an argument and listings in runtime.json |

## Phases

### Phase 1: Steer button, steer-only drain, and injection on both hosts

One behavior slice: the page button and route, the inbox flag, the drain
argument, and both adapters land together.

Verification: Go tests for the route (Origin, 409, unknown id, the fresh-name
write), the inbox (a steered entry is not merged into; steer-only drain takes
only steered entries; the full drain takes the rest; a steered entry is
skipped as merge target and in the attachment cap count); a Go test for the
steer-mode wait (wakes on a steer-marked entry, not on a held follow-up);
Claude mod engine tests for the tool-boundary append, the conversation
record, and the turn-end fallback after a refused append; Pi tests for the
steer delivery, the conversation record, and re-arming without steer mode
while the drain is refused; a live check on Claude and Pi that a promoted
message reaches the model before the turn ends, appears in the page
conversation, and that an unpromoted one arrives after it.

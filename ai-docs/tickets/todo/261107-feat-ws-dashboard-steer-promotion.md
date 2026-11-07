---
title: "Dashboard: promote a held message to steering"
parent: 261107-epic-ws-dashboard-messenger-round2
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
  as mailbox steer does. Pi - the bridge's board wake drains steer-only on a
  steer-marked entry and sends it with `deliverAs: "steer"`. Rejected: pushing
  from the daemon - it has no channel into either host; the adapters already
  pull through the drain.

## Constraints

- Convention: ai-docs/manuals/ws-mcp.md (the drain tool lives under
  `agents-plugin-tool/internal/mcp/`).
- Convention: ai-docs/manuals/wsflow-mirroring.md (the Claude mod under
  `agents-plugin/hooks/` has a wsflow mirror).
- Convention: ai-docs/manuals/shipped-surface-boundary.md.

## Phases

### Phase 1: Steer button, steer-only drain, and injection on both hosts

One behavior slice: the page button and route, the inbox flag, the drain
argument, and both adapters land together.

Verification: Go tests for the route (Origin, 409, unknown id, the fresh-name
write), the inbox (a steered entry is not merged into; steer-only drain takes
only steered entries; the full drain takes the rest); Claude mod engine tests
for the tool-boundary append; Pi tests for the steer delivery; a live check on
Claude and Pi that a promoted message reaches the model before the turn ends
and an unpromoted one arrives after it.

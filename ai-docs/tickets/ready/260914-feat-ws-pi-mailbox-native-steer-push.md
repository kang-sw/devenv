---
title: "Pi native mailbox push: steer incoming mail into the live conversation"
related:
  260913-research-cross-session-mailbox: parent research; Decision 7 names this adapter
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: e86a51f9ee9f589c
sage-review-completeness-reviewed: e86a51f9ee9f589c
---

# Pi native mailbox push: steer incoming mail into the live conversation

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/push-protocol.ts, agents-plugin-pi/src/execute-gateway.ts |
| scope.surface | internal | reused pushToLead/admitPush are agents-plugin-pi/src-internal, no external package boundary crossed |
| scope.new_public_symbol | likely | a session-bound mail-arrival waiter/detector; exact name unfixed |
| scope.new_type_contract | yes | a new `PushBatchItem` customType `"ws-mailbox"` (state informational, display true) rides the existing push-batch wire contract |
| scope.test_surface | existing | agents-plugin-pi/test/spawner.test.ts, push-wake.test.ts, push-render.test.ts, execute-gateway.test.ts already cover admitPush/pushToLead/push-batch machinery |
| complexity.reuse_points | confirmed | pushToLead (agents-plugin-pi/src/spawner.ts#L1580), admitPush (agents-plugin-pi/src/spawner.ts#L993), heldPushQueue (agents-plugin-pi/src/spawner.ts#L1169), PUSH_BATCH_CUSTOM_TYPE (agents-plugin-pi/src/push-protocol.ts#L2), steer precedent (agents-plugin-pi/src/execute-gateway.ts#L576) |
| complexity.side_effect_risk | moderate | a session-bound background `mailbox wait` detector plus live-turn interruption via steer; lifecycle settled (session open/close) |
| risk.correctness | moderate | held-batch admission ordering and active/idle dual-path reuse must match the existing approval-relay path; waiter backoff detail deferred to implementation |
| risk.fit | low | Decisions mandates riding the existing admitPush/pushToLead FIFO rather than a parallel delivery path |
| risk.test | moderate | existing push-batch/spawner test patterns extend, but no test precedent yet for the new mail-arrival background-worker detection path |
| risk.security_or_contract | moderate | steers externally-authored mail content into a live turn; Open Questions confirms adapter-only/best-effort scope, never the host-neutral mailbox contract (ai-docs/tickets/idea/260913-research-cross-session-mailbox.md#L301-L303) |

## Background

Cross-session mailbox delivery today is passive on every harness: Codex and
Claude only surface incoming mail at a turn boundary through stop-hooks. The
owner wants pi to be different — the pi extension should *actively* inject an
arriving mailbox message into the running conversation as a steering message,
so mail can preempt a live turn rather than wait for the next boundary.

## Evidence

- Mailbox logic lives entirely in the Go MCP server, not in pi:
  `agents-plugin-tool/internal/mcp/mailbox_*.go`, and the CLI dispatch
  `ws-mcp mailbox wait|codex-stop-hook|claude-stop-hook`. A grep for `mailbox`
  across `agents-plugin-pi/src` returns zero hits — pi has no mailbox-aware
  code today.
- The steer pathway pi would reuse already exists and is proven: `pushToLead()`
  (`spawner.ts`) funnels into `pi.sendMessage(msg, {deliverAs: "steer"})`, and
  `steer` is the delivery mode that interrupts a live streaming turn (vs.
  `followUp`, which only queues at the next turn boundary). The approval relay
  (`execute-gateway.ts`, `pushToLead(..., "steer")`) is the existing precedent
  for interrupting an active conversation.
- Steer pushes are **not** raw one-off sends: pi already batches system
  messages through a shared FIFO. `admitPush()` (`spawner.ts`) holds pushes in
  `heldPushQueue`, and `submitHeldPushBatch()` releases the queue at a turn
  boundary as one versioned message, `PUSH_BATCH_CUSTOM_TYPE = "ws-push-batch"`
  (`push-protocol.ts`), so the TUI renders one batch rather than many separate
  structured items. Only never-held busy steers stay immediate.
- `260913-research-cross-session-mailbox` Decision 7 lists "pi native push"
  among the harness adapters, all best-effort per the shipped-surface
  boundary (`ai-docs/tickets/idea/260913-research-cross-session-mailbox.md#L301-L303`;
  an earlier, unnumbered "Delivery model" bullet in the same ticket also
  names it "pi native active push",
  `ai-docs/tickets/idea/260913-research-cross-session-mailbox.md#L75-L76`).
  Codex and Claude received hook adapters and hook tests
  (`agents-plugin-tool/cmd/ws-mcp/mailbox_codex_hook_test.go`,
  `agents-plugin-tool/cmd/ws-mcp/mailbox_claude_hook_test.go`); a search for
  `mailbox` across `agents-plugin-pi/src` returns zero hits — pi has neither.

## Decisions

- **Ride the existing push batch, do not send raw.** Arriving mail must be
  admitted through the shared push FIFO (`admitPush` / `sendToLead` /
  `pushToLead`) so it joins the held-push batch and ships inside the existing
  `ws-push-batch` alongside other system messages. Do not call raw
  `pi.sendMessage(..., {deliverAs: "steer"})` directly — that produces a
  separate structured TUI item and stretches steering. Mail is one more
  `PushBatchItem`, not a parallel delivery mechanism.
- **Arrival detection = a session-bound background waiter.** Run a long-running
  `ws-mcp mailbox wait` background worker for the session; on delivery it admits
  the mail through the FIFO above. Its lifecycle is bound to the session
  (start on session open, stop on close). Reuse `pushToLead`'s existing
  active-vs-idle dual path — admit as `steer` when the session is live (subject
  to the held-batch rules), fall through to the idle-wake path when dormant —
  rather than inventing a second delivery mechanism.
- **Mail is always `informational`.** The mailbox `Envelope` is
  `{From, ReplyTo, Content, SentAt}` (`agents-plugin-tool/internal/mcp/mailbox_tools.go`
  `handleMailboxSend`) — free-text `Content` only, with no structured
  instruction/kind field. So mail cannot be classified as an instruction from
  the envelope, and adding such a field would breach the adapter-only boundary
  (below). The mail `PushBatchItem` is therefore `customType: "ws-mailbox"`,
  `state: "informational"`, `display: true`; whether the text is an instruction
  is left to the receiving agent to read, exactly as remote-control already
  works.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Adapter-only boundary: per Decision 7 this is best-effort adapter behavior,
  never part of the host-neutral mailbox contract. Do not add an
  instruction/kind field to the shared `Envelope` to serve this feature.

## Phases

### Phase 1: Session-bound mail waiter + push-batch admission

Add a session-bound background waiter that drives `ws-mcp mailbox wait` for the
session's mailbox and, on delivery, admits the mail through the existing FIFO
(`admitPush`/`pushToLead`) as a `PushBatchItem` with `customType: "ws-mailbox"`,
`state: "informational"`, `display: true` — so it rides the existing
`ws-push-batch` and reuses the active(`steer`)/idle-wake dual path. Start the
waiter on session open, stop it on close. No raw `pi.sendMessage(..., steer)`.
Do not alter the shared `Envelope`.

Verification: extend the existing push-batch/spawner tests
(`agents-plugin-pi/test/spawner.test.ts`, `push-wake.test.ts`,
`push-render.test.ts`) to assert an arriving mail item is admitted through the
FIFO (not a raw send), renders inside a single `ws-push-batch`, and takes the
idle-wake path when the session is dormant. Add a fake/stubbed waiter so the
detection path is testable without a live mailbox.

## Open Questions

- Waiter cadence detail: exact `ws-mcp mailbox wait` invocation/backoff and how
  it composes with compaction/pending-start — resolvable at implementation
  against the existing wait CLI, no product decision left.

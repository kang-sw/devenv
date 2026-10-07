---
title: Pi adapter messages reach the model as unlabeled user text
related:
  261007-feat-pi-compaction-milestone-wake-and-reread-lists: origin; its follow-up labeled only the compaction messages
sage-review-completeness: completed
sage-review-completeness-reviewed: 539d05f67005aea5
sage-review-design: completed
sage-review-design-reviewed: c1d51878ca941f7e
completed: 2026-10-07
---

# Pi adapter messages reach the model as unlabeled user text

## Background

Pi's `convertToLlm` hands every custom message to the model as plain
user-role text (`pi-coding-agent/dist/core/messages.js`, `case "custom"` ->
`role: "user"`), and `pi.sendUserMessage` is user text by construction. The
model therefore cannot tell an adapter notice from the human's message by
structure. Dogfood evidence: a downstream lead's compaction summary quoted the
adapter's 70% milestone notice as "the latest advisory request verbatim" under
Immediate next step.

The 261007 follow-up (`b2b42f690`) prefixed only the compaction messages
(advisory, milestone, hard cut, `/compact` reroute) with
`ADAPTER_MESSAGE_LABEL` (`src/lead-compaction.ts`). Every other message the
adapter injects is still unlabeled; this ticket closes that gap.

## Decisions

- **Label.** Every adapter-injected message that reaches the model as
  user-role text opens with the same first line,
  `[system message from ws-pi-plugin]` (the existing `ADAPTER_MESSAGE_LABEL`).
  Rejected: rewording the lever's `next_step` field to say "the human's
  request" - it treats one symptom and leaves every other reader of these
  messages guessing.
- **Scope.** All remaining adapter injections: every `pi.sendMessage` /
  `pi.sendUserMessage` call site under `agents-plugin-pi/src/` whose content
  reaches the model, swept exhaustively. The known sites are: mailbox
  messages (`WS_MAILBOX_CUSTOM_TYPE`, `src/mailbox-waiter.ts`), push messages
  and push batches (`src/spawner.ts` `pi.sendMessage` sites,
  `PUSH_BATCH_CUSTOM_TYPE`), the push wake line (`buildPushWakeLine`),
  orphaned-agent notices (`ws-agent-orphaned`, `src/index.ts`), the ask
  thread summary (`THREAD_SUMMARY_CUSTOM_TYPE`, `src/ask.ts`), the goal
  reminder and goal announcement, the post-compaction resume prompt
  (`buildCompactionResumeMessage`, `src/goal-loop.ts`), and the execute-gateway
  approval prompt (`buildApprovalPromptText`, `src/execute-gateway.ts`, sent
  as a `ws-agent-approval` push through `pushToLead`, so the push sites cover
  it).
- **Label once, at the send.** The label goes on where `pi.sendMessage` /
  `pi.sendUserMessage` is called (the direct raw send, `sendPush`, the push
  batch send, the orphaned-agent notices), not inside each content builder,
  so a push batch carries it once on the batch and not again on every inner
  `<message>`.
- **The label is the dialog filter's recognizer.** The compaction `## Dialog`
  extraction (`src/lead-compaction.ts`, the human-text filter near
  `GOAL_REMINDER_MARKER_PREFIX` / `isPushWakeLine`) treats a user-role message
  whose text starts with the label as adapter traffic and drops it, replacing
  the per-shape matching of the push wake line and the goal reminder where the
  label now covers them. The two existing shape matchers stay only as a
  fallback for unlabeled entries recorded before this change. Rejected:
  keeping per-shape matching as the recognizer and only tolerating a leading
  label - every new adapter message would need its own recognizer again.
  Accepted trade-off: a human who types the label verbatim is filtered as
  adapter traffic.
- **The goal announcement collapses to what the human typed.** A labeled
  goal announcement ("Goal armed: <goal>", `buildGoalAnnouncement`) renders in
  `## Dialog` as `/goal <goal>`, the way a `/skill:` expansion collapses to the
  `/skill:<name> <args>` the human typed, instead of being dropped by the
  label filter. This keeps 261002's (94cdf8bd) reason for leaving "Goal armed:"
  in the dialog - it carries the user's goal - while the model sees the label.
  Rejected: leaving the announcement unlabeled (the model would still read it
  as the human's own words); dropping it from the dialog (loses the user's
  goal).

## Constraints

- Pi `/skill:` body expansions are host-generated, not adapter messages; their
  existing recognition stays.
- Message shapes other code or tests parse (push batch XML, goal reminder
  marker, push wake line) change only by the leading label line; any parser of
  those shapes is updated in the same change.

## Prior Decisions

- b2b42f690 (2026-10-07, commit): "User decision: label the source in the message itself rather than rewording the next_step field; scope (i) covers only the compaction messages here. Labeling every adapter-injected message ... is a separate todo" — bearing: supports
- 261002-feat-pi-lead-ws-owned-compaction (94cdf8bd, commit): "Human messages are selected newest-first under the budget and displayed chronologically; "Goal armed:" stays as human text because it carries the user's goal." — bearing: contradiction-candidate
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, Decisions): "No system- or developer-role injection. ws messages stay user-role. Pi offers no role choice for injected messages, Anthropic has no mid-conversation system role" — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (94cdf8bd, commit): "the wake-line text is shared from spawner so exclusion cannot drift." — bearing: constrains
- 261003-feat-pi-lead-compaction-dialog-transcript (2026-10-03, Decisions): "Human text is classified exactly as today (humanTextOf). ... Custom messages (push batches, mailbox, preparation messages) stay excluded, as today." — bearing: constrains
- 261004-feat-pi-compaction-interactive-ux (2026-10-04, Decisions): "D4 — Select at most the twenty most recent eligible user/assistant messages ... Exclude tool calls/results, thinking, adapter control messages" — bearing: constrains
- 261006-feat-pi-compaction-advisory-standing-intent-milestones (2026-10-06, Decisions): "Milestones are not preparation messages. They use their own customType ... Adapter custom messages are already excluded from the" dialog — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/lead-compaction.ts, src/mailbox-waiter.ts, src/spawner.ts, src/index.ts, src/ask.ts, src/goal-loop.ts; src/compaction-history.ts#L33-L38 also matches "Goal armed: " and the resume prompt by shape and consumes humanTextOf |
| scope.surface | cross-module | model-visible text of every adapter injection changes; exported humanTextOf in lead-compaction.ts is consumed by compaction-history.ts |
| scope.new_public_symbol | no | reuses ADAPTER_MESSAGE_LABEL agents-plugin-pi/src/lead-compaction.ts#L800; note lead-compaction.ts already imports spawner.ts#L34, so spawner importing the label from lead-compaction would form an import cycle |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-pi/test/lead-compaction.test.ts, goal-loop.test.ts, mailbox-waiter.test.ts, spawner.test.ts, push-wake.test.ts, push-render.test.ts, compaction-history.test.ts |
| complexity.reuse_points | confirmed | ADAPTER_MESSAGE_LABEL agents-plugin-pi/src/lead-compaction.ts#L800 and its use in buildContextMilestoneMessage and buildPreparationMessage |
| complexity.side_effect_risk | moderate | every push, mailbox, goal, and resume message text changes and the dialog and compaction-history filters switch recognizer |
| risk.correctness | moderate | the filter swap can drop human text or keep adapter traffic, and the labeled goal announcement must collapse to `/goal <goal>` rather than drop |
| risk.fit | moderate | import direction between spawner.ts and lead-compaction.ts needs resolving (the label may move to a leaf module) |
| risk.test | moderate | pinned message texts across seven test files must gain the label line without loosening assertions |
| risk.security_or_contract | moderate | push batch XML, goal reminder marker, and push wake line shapes are parsed by code and tests |

## Phases

### Phase 1: Label every adapter injection and recognize by label

Prefix the label on every site in the scope above, switch the dialog filter
to the label, collapse a labeled goal announcement to `/goal <goal>`, and
update every parser of the changed shapes (including
`src/compaction-history.ts`, whose `"Goal armed: "` drop must keep dropping
the labeled announcement once `humanTextOf` returns `/goal <goal>` for it). `src/lead-compaction.ts` already imports
`src/spawner.ts`, so move `ADAPTER_MESSAGE_LABEL` into a leaf module that both
import rather than create a cycle. Every labeled site gets a test asserting
its labeled form: existing pinned-text tests gain the label line, and a site
with no such test today (for example the orphaned-agent notice, the ask
thread summary, or the execute-gateway approval push) gets a new
assertion, and a push batch carries the label exactly once. Also test the label filter, the shape fallback for pre-change unlabeled
entries, the goal collapse, and that ordinary human text stays in.

### Result (030f07d50) - 2026-10-07

Landed. `ADAPTER_MESSAGE_LABEL` moved to the leaf module
`agents-plugin-pi/src/adapter-label.ts` (with `labelAdapterText`,
`labelAdapterContent`, `adapterLabeledBody`), so spawner.ts and
lead-compaction.ts both import it with no cycle. The label is applied once at
each send: the push wake (`requestPushWake`), individual pushes (`sendPush`,
which covers the execute-gateway approval), the push batch (covers mailbox and
held thread summaries; inner items stay unlabeled), raw summaries sent on their
own (`labelRawSend`), both orphaned-agent notices (`buildOrphanNoticeMessage`
in agent-sidecar.ts), the goal reminder, and `sendAdapterPrompt` (goal
announcement, post-compaction resume, including prompts held for the next
start). `humanTextOf` recognizes labeled messages as adapter traffic and
collapses a labeled announcement to `/goal <goal>`. The reminder-marker and
push-wake shape checks stay as the fallback for unlabeled pre-change entries.
`compaction-history.ts` drops every labeled user message. `push-render.ts`
skips the label line when it parses a single push's head.

Verification: `npm test` in agents-plugin-pi ran 2065 tests: 2030 pass and 31
fail. The failing set is identical to the pre-change baseline (2057 tests,
31 fail). All 31 are environmental failures in agent-channel-launch,
agent-channel.integration, persistent-explore, two spawner Explore tests,
web-search, and web-startup, because pi-web-access is missing in the worktree.
The new or tightened assertions cover:
- adapter-label.test.ts
- push-wake: the wake line, batch label once, mailbox, individual pushes, and
  raw string/part sends
- ask: the thread summary and queued answers
- execute-gateway: the approval batch
- agent-sidecar: the orphan notice
- goal-loop: the announcement, reminder, and resume
- lead-compaction: the label filter, legacy fallback, `/goal` collapse, and
  human text kept
- compaction-history
- push-render

Decisions:
- The fork-lead orphan notice used to send the payload object as content. It
  now sends the same JSON text as the worker site, because a label cannot
  prefix an object.
- `deliverQueuedAnswers` also rides `ws-thread-summary`, so it is labeled
  with its batch.
- Unlabeled legacy "Goal armed:" entries keep their old dialog rendering.

Lite review: no Critical or Important findings. One minor observation: a fork's
first input is framed by `frameForkInput`, so a labeled adapter prompt arriving
as that first input would lose its first-line label. This is unlikely in
practice.

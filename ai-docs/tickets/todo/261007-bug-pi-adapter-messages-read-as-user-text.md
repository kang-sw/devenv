---
title: Pi adapter messages reach the model as unlabeled user text
related:
  261007-feat-pi-compaction-milestone-wake-and-reread-lists: origin; its follow-up labeled only the compaction messages
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
- **Scope.** All remaining adapter injections, including at least: mailbox
  messages (`WS_MAILBOX_CUSTOM_TYPE`, `src/mailbox-waiter.ts`), push messages
  and push batches (`src/spawner.ts` `pi.sendMessage` sites,
  `PUSH_BATCH_CUSTOM_TYPE`), the push wake line (`buildPushWakeLine`),
  orphaned-agent notices (`ws-agent-orphaned`, `src/index.ts`), the ask
  thread summary (`THREAD_SUMMARY_CUSTOM_TYPE`, `src/ask.ts`), the goal
  reminder and goal announcement, and the post-compaction resume prompt
  (`buildCompactionResumeMessage`, `src/goal-loop.ts`).
- **The label is the dialog filter's recognizer.** The compaction `## Dialog`
  extraction (`src/lead-compaction.ts`, the human-text filter near
  `GOAL_REMINDER_MARKER_PREFIX` / `isPushWakeLine`) treats a user-role message
  whose text starts with the label as adapter traffic and drops it, replacing
  the per-shape matching of the push wake line and the goal reminder where the
  label now covers them. Rejected: keeping per-shape matching and only
  tolerating a leading label - every new adapter message would need its own
  recognizer again. Accepted trade-off: a human who types the label verbatim
  is filtered as adapter traffic.

## Constraints

- Pi `/skill:` body expansions are host-generated, not adapter messages; their
  existing recognition stays.
- Message shapes other code or tests parse (push batch XML, goal reminder
  marker, push wake line) change only by the leading label line; any parser of
  those shapes is updated in the same change.

## Phases

### Phase 1: Label every adapter injection and recognize by label

Prefix the label on every site in the scope above, switch the dialog filter
to the label, and pin each message's labeled form in tests (the existing
pinned-text tests for these messages gain the label line).

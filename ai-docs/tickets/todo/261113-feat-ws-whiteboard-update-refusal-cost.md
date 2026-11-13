---
title: "whiteboard.update: one malformed command throws away a large batch"
---

# whiteboard.update: one malformed command throws away a large batch

## Background

Owner question 2026-11-13 ("a whiteboard format error is expensive because
the bodies are large"). `whiteboard.update` validates a batch whole: any
problem refuses all of it, the lead must resend every body, and the refusal
repeats the command syntax and the whole command section (~10 KB) into the
lead's context. In one session three refusals came from three patterns:

1. `wb:edit` anchor ambiguity: a generic `wb:old` (`## `) matched three
   places in a body the lead had not queried first.
2. `wb:edit` on a topic that has no body yet ("set one with `<wb:body>`").
3. A note-only board answer to a policy item. The answer block allows
   `41=^<note>` (a note alone), but `wb:answer` on a policy item requires the
   value to start with an option letter, so the lead had to invent a letter
   and mark it assumed.

## Idea

- Refusal output lists only the problems (and a pointer to the playbook),
  not the full command reference.
- Apply-what-validates or per-topic atomicity, or keep the refused batch's
  bodies server-side so a retry can reference them instead of resending.
- `wb:edit` on an empty body with no match: allow an append form (e.g.
  `wb:append`) or treat an empty-body edit as setting the body.
- `wb:answer` accepts a note-only value for a policy item (answered with a
  note, no option chosen), matching the answer block's `^<note>` form.

## Owner Direction (2026-11-13)

- The full syntax/command reprint on refusal goes: delegated as a hotfix
  (branch `impl/develop/wb-refusal-no-syntax-dump`), not part of this
  ticket's remaining scope.
- Incremental model: apply what validates; return the failed parts as an
  XML tree of the failed commands, so only those are resent.
- Heuristics for patterns 1 and 2, and a direction for pattern 3, to be
  discussed.

## Decisions

- Owner 2026-11-13 (board 43=b): the atomic unit is one command, not a
  topic group. A wrong answer must not discard large bodies; the lead sees
  the failed-command tree and resends only those next turn. Lead technical
  guard: a `wb:close` is skipped (reported as failed) when an earlier
  command in the same batch for the same topic failed, so a close never
  withdraws an item whose answer just failed.
- Owner 2026-11-13 (board 44=b): a note-only answer to a policy item gets a
  new status meaning "user commented, lead's move": it leaves the user's
  waiting list until the lead re-asks, records an option letter, or
  withdraws it. Compatibility: a new item status in the whiteboard store is
  a store change; settle its tier at promotion.

## Candidate Design (lead, for discussion)

- Atomic unit: a `wb:topic` group (top-level commands each on their own),
  so an answer-then-close or edit-then-recap sequence inside one topic never
  half-applies; one change stamp per call stays.
- Failure echo: a skeleton tree with the failed command's tag, attributes,
  line, and error, without echoing large content the caller already holds:
  `<wb:failed><wb:topic name="x"><wb:edit line="7" error="..."/></wb:topic></wb:failed>`.
- Pattern 1: on several matches, refuse that command only and return each
  candidate's line number and a one-line context; add heading-scoped edits
  (e.g. `wb:section heading="..." op="replace|append|before|after"`) so the
  common "add or replace a section" needs no substring anchor.
- Pattern 2: `wb:append` (creates the body when there is none); a
  `wb:edit` against an empty body applies `wb:new` as the body, since there
  is nothing to lose.
- Pattern 3 options: (a) a note-only answer is recorded as answered with no
  option chosen; (b) a new status meaning "user commented, lead's move"
  that leaves the item off the user's waiting list until the lead re-asks,
  records a letter, or withdraws it.

## Open

- Whether partial apply breaks the "one batch, one change stamp" model.
- Pattern 3 choice.

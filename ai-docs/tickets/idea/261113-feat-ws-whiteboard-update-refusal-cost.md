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

## Open

- Whether partial apply breaks the "one batch, one change stamp" model.

---
title: "사용자가 직접 대안을 제시하는 whiteboard 정책 응답"
dropped: 2026-11-21
---

# Whiteboard policy answer that proposes the user's own alternative

## Background

Dogfood surprise (2026-11-06, devenv lead session): the user answered a
policy item with an alternative none of its options named ("(202) note:
store each ring entry as its own file ..."). `whiteboard.update` refused
the answer: "a policy answer starts with its option letter (a-c)". The
lead had to patch the item's `options` to append the user's alternative as
option d in a separate update (since `answer` applies before `patch` in one
batch), and then record `d` in a second update, breaking the one-update-
per-turn rule.

The `whiteboard` playbook's `#### Answers` covers letters, notes, and
`assumed`, but not an answer that is a new alternative. Users do this often
in discussion, so every lead hits the same two-update workaround.

Open for design: accept a free-text policy answer (for example `answer:
"other, <text>"` or no letter), apply `patch` before `answer` within a batch
so one update can add and select the option, or document the two-step
workaround in the playbook.

## Additional dogfood evidence — 2026-11-16 (#14)

The owner again reported that a whiteboard policy item cannot record an answer
outside its listed options. This matches the original incident above; reuse
this ticket and accept it into todo instead of creating a duplicate. Today's
item ID and exact answer/refusal were not supplied, so the original concrete
refusal remains the detailed reproduction evidence. Do not treat a note-only
commented status as necessarily equivalent to recording a new policy decision.

## Reproduction

Create an isolated policy item with options a–c, then record an owner answer
that selects none of them and describes a different alternative. Observe the
option-letter refusal. Compare existing letter selection, note-only/commented
answers and the historical two-update append-option/select-option workaround.
Keep failed updates atomic; do not alter a live policy item for this capture.

## Candidate fix directions

- Support an explicit off-option/free-text alternative answer.
- Permit an atomic option amendment followed by selection with defined ordering.
- If existing semantics remain, provide a supported explicit alternative-answer
  workflow rather than silently mapping the owner to an unchosen option.

## Decisions

Compatibility tier: **unsettled; target T0** if existing answer forms and stored
meaning remain usable by old clients. A new answer kind/event must be checked
against actual old parsers/writers and new reopen behavior before readiness;
unknown data being dropped is not T2 without retained recovery. Any required
old-version boundary is T3 and needs approval and a minor bump. No answer syntax,
policy authority or batch-order change is selected by this capture.

## Open questions

- Is an off-option answer a new selectable alternative, a separate free-text
  decision kind, or an explicit patch-and-answer operation?
- How should it differ from a note-only/commented item, and what closes the
  policy decision versus leaving discussion open?
- What history, old-client rendering/writing and atomic update ordering are
  required so the owner's exact intent is not lost or coerced?

## Phases

### Phase 1: Record owner alternatives without coercing a listed option

Trace current answer validation, state/history writers and readers, settle the
alternative-answer representation and compatibility, then implement the approved
bounded path. Future fixtures cover an ordinary letter, off-option owner text,
note-only/commented state, option amendment/answer ordering, atomic failure,
replay and old-write/new-reopen preservation. Preserve existing policy answers
and the owner's literal intent. No whiteboard mutation, build or test occurs
during this docs capture.


## Resolution (2026-11-21)

Superseded by policy note-only comments (`StatusCommented`, commit `e8cfa2167`, `wswhiteboard/store.go:37-49,1013-1070`). Owner confirmed on 2026-11-21 that the existing note/comment workflow sufficiently covers this need; dropped per owner direction.

---
title: Resolve ws-queue-question live dogfood findings
related:
  260911-feat-ws-pi-async-question-queue: feature under live acceptance
  260906-workset-ws-pi-dogfood-ux: Pi dogfood UX collection
---

# Resolve ws-queue-question live dogfood findings

## Background

Live Pi acceptance of the newly landed `ws-queue-question` flow confirmed its fork-less registration, `/thread` discovery, anchored answer injection, draft persistence, Korean IME input, multiline editing, per-question drafts, Tab navigation, partial multi-question submission, and blank-question retention.

Keep the remaining findings from this dogfood run together in this rolling actionable ticket rather than creating one ticket per small observation. Split later only if implementation proves that a finding has an independently meaningful contract or delivery boundary.

## Findings

1. **Weak modal hierarchy.** The `Qn/N` and answered-count header, answer editor separator, and shortcut footer do not establish enough visual hierarchy. The header and separator need clearer semantic foreground or background treatment; shortcut help should be dimmed without becoming illegible.
2. **Counter-spatial confirmation arrows.** In the final `Yes [No]` confirmation, Right from the initial right-hand `No` moves selection left to `Yes`, and Left from `Yes` moves selection right to `No`.
3. **Inaccessible long-question truncation.** Long question/context content is replaced by `question truncated`; Up/Down and PageUp/PageDown cannot reveal the omitted text. The marker uses the same foreground treatment as ordinary prose and does not clearly communicate the inaccessible boundary.
4. **Bare `/answer` enters at the newest item.** With `q6`, `q7`, and `q8` pending, bare `/answer` opened on `q8`. A sequential oldest-first queue should begin at the oldest answerable item unless the owner explicitly names an ID.
5. **One model turn per answer from one modal submission.** The owner submitted `q6` and `q7` together while the lead was working, but the two answers later popped as separate owner messages across separate lead turns. The modal's batch boundary was lost, adding avoidable model-turn cost.
6. **Explicit terminal target falls through to another question.** `/answer q6` on an already answered question displayed `Warning: ws: question q6 was already answered or withdrawn — showing the rest of the queue instead.` and then opened another pending question. The warning was not visually salient enough to prevent the owner from entering `adf`, which was submitted as the answer to `q10`. The explicit target was not honored, and fallback changed which question received the input.

## Live Acceptance Status (2026-09-11)

Passed in the current Pi session:

- fork-less single-question registration and anchored answer injection;
- Korean IME input, multiline editing, Esc/reopen draft restoration, and exact answer preservation;
- multi-question Tab/Shift+Tab navigation, independent drafts, commit-and-advance, partial submission, and blank-question retention;
- pending-question withdrawal and post-answer withdrawal no-op.

Remaining live coverage for a later session:

- withdrawal while the owner is actively editing an open question;
- the fork-raised `kind:"question"` path, existing-fork attachment, `/done`, and final `Decisions:` delivery;
- optional post-compaction and restart persistence/anchor recovery.

## Constraints

- Keep `No` as the confirmation's initial safe default.
- Use bounded spatial confirmation navigation: Left moves toward the visually left option, Right moves toward the visually right option, and input at an outer edge does not wrap.
- Use Pi theme semantics rather than fixed colors so hierarchy remains usable across light, dark, and custom themes.
- Replace destructive question/context truncation in this modal type with a bounded, scrollable question region. Keep the answer editor and shortcut footer visible while making the full original question reachable. Any remaining overflow cue must be visually distinct from ordinary prose.
- Answer commands are target-strict:
  - bare `/answer` starts at the oldest answerable lead-ask item;
  - `/answer qN` opens exactly `qN` when it is answerable;
  - `/answer qN` for an answered or withdrawn question reports that state and returns without opening another modal;
  - `/answer qN` for an unknown ID reports an error and returns without opening another modal.
  `Ctrl+Shift+A` behavior is not decided by this finding.
- Answers accepted by one lead-ask modal submission return in one injected lead follow-up and consume one lead turn. Preserve every included question's full question, context, verbatim answer, ask-time commit hash, and entry anchor in deterministic queue order; exclude blank questions that remain pending.
- Preserve fork-less registration, draft and withdrawal semantics, and the separate fork-raised question path. Do not generalize batching to unrelated user input or child-message families without separately deciding their ordering and wake behavior.

## Phases

### Phase 1: Close accepted question-queue dogfood gaps

Resolve the accepted findings while preserving the behaviors that passed live acceptance.

Verify confirmation positions and both arrow keys, including edge no-ops; long questions at constrained viewport heights with complete scroll reach and fixed editor/footer visibility; oldest-first bare `/answer`, exact explicit-ID targeting, and terminal/unknown explicit-ID refusal without fallback; semantic hierarchy in a live Pi theme; and single-turn delivery of multiple answers submitted together while the lead is busy. Retest the already-passing single answer, Korean multiline draft, partial multi-question submission, blank retention, and per-question provenance paths for regression.

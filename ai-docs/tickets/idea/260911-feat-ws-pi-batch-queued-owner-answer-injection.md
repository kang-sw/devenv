---
title: Batch queued owner answers into one lead turn
related:
  260911-feat-ws-pi-async-question-queue: follow-up to multi-question modal submission and answer injection
  260906-workset-ws-pi-dogfood-ux: Pi dogfood efficiency follow-up
---

# Batch queued owner answers into one lead turn

## Background

During live Pi dogfood, the owner answered `q6` and `q7` together through one multi-question `ws-queue-question` modal submission while the lead was working. Both answers were preserved correctly, but they were later injected as separate owner messages in separate lead turns. Each answer therefore consumed its own model turn even though the owner submitted them as one batch.

The sequential modal should preserve its batching boundary on return: answers submitted together should pop into the lead together rather than serially consuming one model turn per question.

## Constraints

- Coalesce answers accepted by one lead-ask modal submission into one injected lead follow-up and therefore one model turn.
- Preserve each question's full question, context, verbatim answer, ask-time commit hash, and entry anchor in deterministic queue order.
- Exclude blank questions that remain pending.
- Do not change the fork-less registration contract, draft preservation, withdrawal behavior, or the separate fork-raised question path.
- Do not generalize batching to unrelated user input or child-message families without separately deciding their ordering and wake semantics.

## Phases

### Phase 1: Preserve modal submission batching through lead injection

Deliver the answers committed by one modal submission as one lead follow-up while retaining per-question provenance and deterministic order. Ensure one submission causes one lead wake/turn even when the lead was busy when the owner submitted it.

Verify single-answer compatibility, two-or-more answered questions in one submission, mixed answered and blank pending questions, and delivery while the lead is busy. Live-test that one multi-question owner submission appears to the lead as one follow-up turn and that every included answer remains individually recoverable.

---
title: Improve lead-ask modal hierarchy and confirmation navigation
related:
  260911-feat-ws-pi-async-question-queue: live dogfood follow-up to the sequential prose-modal queue
  260906-workset-ws-pi-dogfood-ux: Pi dogfood UX follow-up
---

# Improve lead-ask modal hierarchy and confirmation navigation

## Background

Live Pi dogfood of `ws-queue-question` at `f160975f` confirmed the fork-less question round trip, `/thread`, `/answer`, safe-default confirmation, and anchored answer injection. It also exposed two presentation problems in the lead-ask queue:

- The `Qn/N` and answered-count header, answer editor separator, and shortcut footer do not establish enough visual hierarchy. The header and separator need clearer semantic foreground or background treatment, while shortcut help should be dimmed so it remains legible without competing with the question and answer.
- The final `Yes [No]` confirmation maps arrow keys counter to the visible spatial order. From the initial right-hand `No`, Right moves selection left to `Yes`; from left-hand `Yes`, Left moves selection right to `No`.

## Constraints

- Keep `No` as the initial safe default.
- Confirmation arrows must use bounded spatial navigation: Left moves toward the visually left option, Right moves toward the visually right option, and input at an outer edge does not wrap.
- Use Pi theme semantics rather than fixed colors so the hierarchy remains usable across light, dark, and custom themes.
- Preserve the queue's submission, draft, withdrawal, and answer-injection behavior. This ticket changes only presentation and confirmation navigation.

## Phases

### Phase 1: Correct modal hierarchy and navigation

Refine the lead-ask queue header, answer separator, and shortcut footer styling, and correct the final confirmation's horizontal key behavior without changing its visible option order or safe default.

Verify focused component behavior for both confirmation positions and both arrow keys, including edge no-ops. Exercise the modal live in Pi to confirm that the header, editor boundary, and dimmed shortcut help remain distinguishable in the active theme and that the corrected navigation matches the visible `Yes [No]` order.

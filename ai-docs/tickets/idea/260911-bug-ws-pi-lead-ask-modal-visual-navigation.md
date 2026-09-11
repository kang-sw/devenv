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
- Long question/context content is replaced by a `question truncated` line with no way to reveal the omitted text. Up/Down and PageUp/PageDown do not expose it, so instructions or decision context past the viewport boundary become inaccessible. The marker uses the same foreground treatment as ordinary prose and does not read clearly as a boundary notice.
- With several newly queued questions, a bare `/answer` focuses the newest question (`q8`) instead of beginning at the oldest unanswered question (`q6`). That entry point conflicts with the modal's sequential, oldest-first queue model and can skip the context needed for the later questions.

## Constraints

- Keep `No` as the initial safe default.
- Confirmation arrows must use bounded spatial navigation: Left moves toward the visually left option, Right moves toward the visually right option, and input at an outer edge does not wrap.
- Use Pi theme semantics rather than fixed colors so the hierarchy remains usable across light, dark, and custom themes.
- Replace destructive question/context truncation in this modal type with a bounded, scrollable question region. The answer editor and shortcut footer must remain visible while the owner can reach the complete original question. Any remaining overflow or boundary cue must be visually distinct from ordinary prose.
- A bare `/answer` must enter a multi-question lead-ask queue at its oldest answerable item. Explicit `/answer qN` continues to focus the requested item.
- Preserve the queue's submission, draft, withdrawal, and answer-injection behavior. This ticket changes presentation, question accessibility, and confirmation navigation only.

## Phases

### Phase 1: Correct modal hierarchy and navigation

Refine the lead-ask queue header, answer separator, shortcut footer, and overflow-cue styling; make long question/context content scrollable instead of inaccessible; make bare `/answer` enter at the oldest answerable item; and correct the final confirmation's horizontal key behavior without changing its visible option order or safe default.

Verify focused component behavior for both confirmation positions and both arrow keys, including edge no-ops. Cover a long question at a constrained viewport: every line remains reachable, scrolling does not displace the answer editor or shortcut footer, and editor input routing remains intact. With multiple pending questions, verify that bare `/answer` starts at the oldest item while `/answer qN` starts at the requested item. Exercise the modal live in Pi to confirm that the header, editor boundary, overflow cue, and dimmed shortcut help remain distinguishable in the active theme and that the corrected navigation matches the visible `Yes [No]` order.

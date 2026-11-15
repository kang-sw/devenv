---
title: "Whiteboard history uses one 1000px cut and folds closed Q/A like ordinary log entries"
related:
  fix/history-autofold-loadmore-capsule: "branch carrying the history auto-fold regression fix; this ticket lands after it"
---

# Whiteboard history uses one 1000px cut and folds closed Q/A like ordinary log entries

## Background

Board topic `history-fold-rule`, owner items 1086 and 1087. The history "load
more" expand appends whole rows until the accumulated height reaches or
exceeds 1000px, so the row straddling 1000px is included and the result is
always slightly over 1000px. Code: `agents-plugin-tool/internal/wswhiteboard/assets/stream.js`.

## Decisions

Three changes are settled:

1. The initial view uses the same cut, counted from the bottom: 1000px plus
   the straddling row.
2. Closed questions, answers and older user inputs fold like ordinary log
   entries ((1087)=a).
3. Only OPEN questions inside a folded older history are lifted out and
   shown.

- Deferred past tonight by the 20:45 wrap-up ((1109)=a). Implement after
  v0.50.0 and after the history auto-fold regression fix lands (branch
  `fix/history-autofold-loadmore-capsule`).
- The change must preserve the anchor-settle behaviour from `a9a71e099`:
  re-pin within a 1500ms settle window, cancelled on user input.
- Compatibility tier: T0. Frontend rendering only; no store or protocol
  change.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`). Shipped text must be downstream-neutral.
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`).
- Start only after v0.50.0 and the history auto-fold fix have landed.

## Phases

### Phase 1: Apply the single 1000px cut and fold closed Q/A and old inputs

In `stream.js`, make the initial history view use the same bottom-up cut as
"load more" (1000px plus the straddling row). Fold closed questions, answers
and older user inputs like ordinary log entries, and lift out and display
only open questions from within a folded older history. Keep the anchor-settle
behaviour intact.

Acceptance:

- Browser tests for the initial cut, folding of closed Q/A and old user
  inputs, open-question lift-out from folded history, and anchor delta 0
  across expand.
- Screenshots of the initial view and an expanded view.

---
title: "Center whiteboard reference navigation and reveal targets in closed topics"
---

# Center whiteboard reference navigation and reveal targets in closed topics

## Background

The owner requested an idea-only UX change for whiteboard item/choice-number
and reference navigation. Implementation details remain deferred while work is
paused.

## Decisions

- Clicking an item/choice number consistently navigates to its target using
  vertical viewport-center alignment.
- If the target belongs to an already closed topic, reveal and expand any
  collapsed UI containers needed to show it, then center and visually highlight
  that exact item.
- Revealing a closed topic's UI does not reopen its workflow/status or alter
  recorded answers.

## Constraints

- Capture only: no implementation, promotion, run, or review is authorized.
- Animation, highlight duration, implementation approach, and unrelated
  navigation behavior are not specified by this request.

## Phases

### Phase 1: Center reference targets and reveal closed-topic items

Deferred idea: preserve the requested behavior above. Define implementation and
verification details only when the owner resumes and authorizes this work.

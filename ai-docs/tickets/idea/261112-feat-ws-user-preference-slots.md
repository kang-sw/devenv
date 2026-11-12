---
title: "User preferences as named slots that layer across config levels"
---

# User preferences as named slots that layer across config levels

## Background

User feedback (whiteboard, 2026-11-12), recorded as feedback only; nothing
here is a settled decision yet.

Today the user preference is a single value that the workflow manual prints
under `### User preferences`. The user asked for it to become a set of named
slots:

- The existing default note moves into a `default` slot.
- Slots override per slot, and keys accumulate across config levels, so each
  layer can add slots of its own.
- A lower layer clears a slot set by a higher one by setting it to the empty
  string; a cleared slot disappears from the manual entirely.
- How existing downstream configs that set the single value stay compatible
  is to be discussed.

## Open Questions

- Compatibility for downstream configs that set the single preference value
  today (alias into `default`, migration, or both), and the tier per
  `ai-docs/manuals/release-compatibility.md` (the config manifest is a
  cross-version surface).
- Slot ordering in the rendered manual.
- Which config levels take part and their precedence (the existing layering
  of the config knob family).

## Phases

### Phase 1: Slotted user preferences

To be settled at capture through the Open Decision Queue.

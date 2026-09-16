---
title: "Place Pi agent active time beside total elapsed time"
---

# Place Pi agent active time beside total elapsed time

## Background

The Pi live-agent gutter currently renders total elapsed time and active time as separate fields, with the latter shown later in the row as `active <active-time>`. This separates two related durations and spends horizontal space on the `active` prefix.

Current shape:

```text
gutter-probe | worker | running | 1m | gpt-5.6-luna (high) | 17k | active 25s | $0.33
```

Desired shape:

```text
gutter-probe | worker | running | 1m (25s) | gpt-5.6-luna (high) | 17k | $0.33
```

The first duration remains total elapsed time. The parenthesized duration is active time.

## Decisions

- Relocate the existing active-time entry from its separate trailing position; do not delete or replace that entry.
- Remove only its `active ` prefix and render the same active-time value immediately after total elapsed time, enclosed in parentheses as `<total-time> (<active-time>)`.
- Preserve the active-time display's current color and other styling after it moves.
- Keep the existing `worker` label unchanged.

## Constraints

- This is a presentation-only relocation of the existing active-time entry: do not change total-time or active-time accounting semantics.
- Do not move, remove, rename, or restyle any other gutter field.

## Phases

### Phase 1: Relocate the active-time display

Update the Pi live-agent gutter rendering to produce the confirmed duration layout and retain the moved active-time value's existing styling.

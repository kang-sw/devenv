---
title: claude-watch scroll performance — cache visual rows and Arc rendered_lines
related:
  260426-feat-claude-watch-mouse: prerequisite
completed: 2026-04-26
---

# claude-watch scroll performance — cache visual rows and Arc rendered_lines

## Background

With large sessions, scrolling in the claude-watch content panel is sluggish.
Two per-frame O(n) operations are the bottleneck:

1. `scroll_to_bottom_offset_for_width(pw)` iterates all `rendered_lines` and calls
   `visual_rows()` on each — called up to twice per frame (once for
   `needs_scroll_to_bottom` resolution, once for `max_scroll` clamp).
2. `rendered_lines.clone()` copies the entire `Vec<Line<'static>>` into
   `Paragraph::new()` every frame at 60 fps.

## Phases

### Phase 1: Cache total visual rows

Add `cached_visual_rows: Option<(usize, u16)>` to `App` — stores
`(total_visual_rows, panel_width)`. Invalidate on any of:

- `rendered_lines` reassigned (load, reload, toggle_thinking)
- `right_panel_inner_width` changes (terminal resize)

`scroll_to_bottom_offset_for_width(pw)` checks the cache: if the stored
`panel_width` matches `pw`, return `cached.saturating_sub(content_panel_height)`
directly; otherwise recompute, store, and return.

Call sites in `draw_content_panel` pass the same `pw` on every frame, so the
cache hits every frame after the first load or resize.

### Result (01af02a) - 2026-04-26

Implemented as scoped. `cached_visual_rows: Option<(usize, u16)>` added to `App`.
`scroll_to_bottom_offset_for_width` changed to `&mut self`. Cache invalidated at
all three `rendered_lines` reassignment sites. Explicit ui.rs width-change guard
removed — the function-level `cached_pw == pw` check is the sole invalidation path
(the explicit guard was inert because main.rs pre-writes `right_panel_inner_width`
before every draw).

### Phase 2 [dropped]: Arc rendered_lines to eliminate per-frame clone

Change `rendered_lines: Vec<Line<'static>>` to
`rendered_lines: Arc<Vec<Line<'static>>>`.

Dropped: ratatui 0.30 `Paragraph::new` requires owned `Vec<Line>` via `Into<Text>`.
`Arc` forced `(*arc).clone()` — a full deep clone with zero allocation savings.
Phase 1 alone resolves the dominant per-frame bottleneck (the O(n) visual_rows sum).
The per-frame clone remains; addressing it requires a ratatui API change or a
custom `Paragraph` wrapper.

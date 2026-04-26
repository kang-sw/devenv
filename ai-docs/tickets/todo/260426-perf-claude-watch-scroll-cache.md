---
title: claude-watch scroll performance — cache visual rows and Arc rendered_lines
related:
  260426-feat-claude-watch-mouse: prerequisite
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

### Phase 2: Arc rendered_lines to eliminate per-frame clone

Change `rendered_lines: Vec<Line<'static>>` to
`rendered_lines: Arc<Vec<Line<'static>>>`.

In `draw_content_panel`, replace `app.rendered_lines.clone()` with
`Arc::clone(&app.rendered_lines)` — O(1) reference count bump instead of
O(n) deep copy.

Constraint: `Paragraph::new()` accepts `impl Into<Text<'static>>`.
`Vec<Line<'static>>` implements `Into<Text<'static>>` via ownership.
With Arc we cannot move out, so wrap with `(*arc).clone()` only when
ratatui requires ownership — or check if `Paragraph` can accept a
`Text` constructed from a borrow. If ratatui requires owned `Vec`, keep
a lightweight `Arc<Vec<…>>` and call `.as_ref().to_vec()` only when the
inner pointer changes (i.e., cache the `Text` alongside the Arc pointer
identity). If that is too complex, Phase 2 may be simplified to
`Rc<Vec<Line<'static>>>` with a single-threaded clone-on-write approach
or simply accept that Phase 1 alone resolves the dominant bottleneck.

Constraint: all mutation sites (`load_selected_session`, `reload_content_if_loaded`,
`maybe_reload_content`) must replace the Arc with a new `Arc::new(...)` —
never mutate through the Arc.

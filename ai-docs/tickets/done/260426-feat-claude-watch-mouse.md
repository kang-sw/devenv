---
title: claude-watch mouse support
spec:
  - 260426-claude-watch-mouse-input
related:
  260426-feat-claude-watch: prerequisite
completed: 2026-04-26
---

# claude-watch mouse support

## Background

`claude-watch` currently accepts only keyboard input. Mouse scroll and left-click session selection would make it more ergonomic, particularly in terminal environments where mouse capture is standard.

Scope is narrow: enable crossterm mouse capture, wire scroll events to the existing `scroll_offset` field in `App`, and wire left-click on the left panel to `selected_session` selection. No other mouse gestures.

## Phases

### Phase 1: Mouse capture and event handling

Enable `crossterm::event::EnableMouseCapture` on startup and `DisableMouseCapture` on exit, symmetric with the existing terminal init/restore flow in `main.rs`.

Wire two event kinds:

- `MouseEventKind::ScrollDown` / `ScrollUp` — increment / decrement `scroll_offset` in `app.rs`, clamped by existing bounds logic.
- `MouseEventKind::Down(MouseButton::Left)` — if the click's column falls within the left panel boundary, map the row coordinate to a list index (accounting for the current scroll offset of the session list) and set `selected_session`. Clicks outside the left panel are ignored.

Constraints:
- Mouse capture must be disabled before process exit in all paths (normal quit, Ctrl+C panic handler).
- The left-panel click hit-test must use the actual rendered panel width, not a hardcoded constant.
- No behavior change for existing keyboard bindings.

### Result (50f3f02) - 2026-04-26

Implemented as scoped. `EnableMouseCapture` on startup, `DisableMouseCapture` on all exit paths via augmented panic hook. `compute_session_index` extracted as pure function (8 unit tests). `LEFT_PANEL_PERCENT` shared constant across app.rs and ui.rs.

Also included: event-drain loop refactor (poll(ZERO) inner drain + thread::sleep(FRAME_MS=16ms)) eliminating 100ms input latency — prompted by user-observed sluggishness during review. Session-switch latency (JSONL parse blocking) was not addressed; background I/O thread remains a follow-up if large sessions feel slow.

---
title: "Tracker item window silently drops unsaved edits when it closes"
---

# Tracker item window drops unsaved edits on close

## Observation (2026-11-10)

Reported out of scope by the delegate that implemented the tracker
`wiki:<name>/<path>` links: "The item window drops its own unsaved edits
silently when it closes; that is outside this ticket and was not changed."

A user editing an item's body or a field in the tracker item window, then
closing the window (close button, Escape, clicking away, or opening another
item), loses the edit with no prompt or draft.

## Directions (not decided)

- Keep a per-item draft (the shared markdown editor already exists) and
  restore it on reopen, or
- confirm before closing a window with unsaved edits, or
- autosave on close.

## Constraints

- Frontend change in the tracker dashboard assets; `nice -n 19 node --check`
  locally, Go tests via `~/.local/bin/ws-rgo`.

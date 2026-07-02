---
title: "Research: enter/exit stack-based todo-list nesting model"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260625-feat-ws-session-state-machine: motivating-feature
---

# Research: enter/exit stack-based todo-list nesting model

## Background

The session-state machine keeps ONE flat todo list per session, and every
`ws.enter.*` call REPLACES it (mode switch discards the prior list). This is
correct for today's flows:

- forge skills (`lead-forge-spec`, `lead-forge-mental-model`) are standalone —
  nothing routes into them mid-mode, so they own the list cleanly.
- `proceed -> implement` is a transition, not a nesting: by the time
  `lead-proceed` hands off, the proceed checklist is effectively done, so
  replace is the right semantic.

The gap is true mode NESTING: a parent mode whose step spawns a child mode that
should resume the parent afterward. The flat-replace model cannot represent
"run a sub-mode, then come back to where the parent left off." This ticket
records the long-term direction surfaced during the 260625 dogfood; it is
idea-level and needs design discussion before any implementation.

## Direction: enter/exit stack

Model the todo state as a STACK of mode frames rather than a single list:

- `ws.enter.*` PUSHES a new frame (its derived checklist) onto the stack.
- A corresponding EXIT pops the frame and restores the parent frame's list.
- `ws.todo.*` mutations target the active (top) frame.

This preserves parent progress across a nested sub-mode and generalizes beyond
two levels.

## Hard sub-problems

1. **Parent binding / pop trigger.** The child frame must be bound to its parent
   so exit pops it with the parent restored. Open question: is exit explicit (a
   new `ws.exit`/`ws.enter.*` returns) or inferred (child checklist fully
   `done`)? Implicit pop risks premature/never popping.
2. **Active-only rendering.** Alternatively, keep a richer structure but render
   only the active frame's todos at `workflow_manual` load / `todo.list`, to
   avoid drowning the reader in the full stack. This adds rendering complexity.
3. **Depth.** Real nesting exceeds two levels (`proceed -> implement -> review
   -> fix`), so a fixed 2-level structure is insufficient — favor an arbitrary
   stack.

## Rejected for now

- **Fixed 2-level (parent + one child).** Conceptually simpler but leaks at
  depth >2; if we change the core model we should generalize to a stack.
- **Inline migration into 260625 Phase 2.** Foundational change to a
  just-shipped machine not yet validated in its flat form; out of scope for the
  Phase 2 remainder.

## Open questions

- Does any current flow actually require parent restoration, or is transition
  semantics sufficient indefinitely? (If the latter, this stays idea-only.)
- Interaction with compaction recovery: how does `workflow_manual` render and
  restore a multi-frame stack?

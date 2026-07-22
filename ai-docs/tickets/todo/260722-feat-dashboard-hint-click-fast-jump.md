---
title: Dashboard hint-click / fast-jump on-screen labels
parent: 260711-epic-ws-dashboard-command-surface
related:
  260722-feat-dashboard-hotkey-config-framework: provides the leader-key
    capture and binding-dispatch spine this layer's trigger and label-key
    resolution bind onto
  260722-feat-dashboard-which-key-hint-overlay: sibling discoverability
    layer; both render on-screen overlays but for different purposes (key
    discovery vs. mouse-target jumping)
related-mental-model:
  - ws-web-dashboard
---

# feat: Dashboard hint-click / fast-jump on-screen labels

## Background

Agenda A UX review (owner, 2026-07-22) finalized the dashboard's layered
keyboard-interaction stack (leader-only, `Ctrl+Space`, no modal - full
decision record in `260722-feat-dashboard-hotkey-config-framework`). This
ticket is layer 4, the **last** layer in the sequence: a Vimium/flash/
leap-style hint-click fast-jump system that lets the keyboard reach any
visible on-screen mouse target without touching the mouse.

Motivating use case: a user is editing inside a terminal pane and wants to
jump directly to a different server's worktree elsewhere in the dashboard,
without leaving the keyboard to hunt for it with the mouse.

## Decisions (final - Agenda A, 2026-07-22)

- Framed as a **keyboard extension of mouse actions**, not a new command
  vocabulary: hint labels mark clickable/interactive elements, and selecting
  a label performs the same action a mouse click on that element would.
- Coverage: the **full visible (non-occluded) viewport** - not scoped to a
  single panel or pane. Any interactive element currently visible and not
  covered by another element is a fast-jump target.
- **Performance-gated**: label generation/rendering must be gated so it does
  not degrade interaction latency on a dashboard with many panes/targets
  open. The exact gating strategy (e.g. lazy label computation, capping
  visible target count, throttling recomputation on layout change) is an
  implementation detail, not decided here - but the requirement that it be
  gated, not naive, is final.
- Depends on `260722-feat-dashboard-hotkey-config-framework`: the trigger to
  enter fast-jump mode is itself a leader-sub binding (or user-configured
  standalone hotkey) dispatched through the framework's registry, and label
  key-sequence capture reuses the same terminal-passthrough guard pattern so
  it does not eat terminal input when triggered while a terminal pane has
  focus.
- **Sequencing: last layer.** This ships after the hotkey config framework,
  the which-key hint overlay, and the command bar integration are in place,
  per the parent epic's ordering. It is not a prerequisite for any earlier
  layer.

## Non-Goals

- Replacing the which-key hint overlay
  (`260722-feat-dashboard-which-key-hint-overlay`) - that overlay shows
  available leader-sub *key bindings*; this feature shows on-screen *labels
  over clickable targets*. Different purpose, different trigger.
- Replacing the command bar's go-to-file/go-to-line/symbol-search prefixes
  (`260711-idea-dashboard-command-bus-quick-open-shortcuts`) - fast-jump
  targets currently-visible interactive elements, not arbitrary
  files/symbols outside the current viewport.
- Deciding the specific performance-gating mechanism - left to
  implementation, constrained only by the "must be gated" requirement above.

## Phases

### Phase 1: Viewport-wide hint labeling and fast-jump dispatch

- Add a fast-jump trigger bound through the hotkey config framework
  (`260722-feat-dashboard-hotkey-config-framework`) that enters a transient
  hint-label mode: label every visible, non-occluded interactive element in
  the current viewport (not just the focused pane) with a short selectable
  key sequence, Vimium/flash/leap-style.
- Selecting a label's key sequence performs the same action a mouse click
  on that target would (focus, activate, navigate - whatever the target's
  native click behavior is), then exits hint-label mode.
- Gate label computation/rendering for performance: avoid naive per-frame
  recomputation across a dashboard with many open panes/worktrees/terminals;
  document the chosen gating approach (lazy computation, target-count cap,
  recompute throttling, or similar) as part of this phase's implementation
  notes.
- Reuse the terminal-passthrough guard pattern so triggering or cancelling
  fast-jump mode does not leak into terminal raw input, and so a terminal
  pane's own content is still a labelable jump target rather than an
  input-capture exception.
- Verify: labels appear over every visible non-occluded interactive element
  across multiple simultaneously visible panes/worktrees (not just the
  focused one); selecting a label reaches and activates the correct target,
  including the motivating cross-worktree jump (terminal-focused editing to
  another server's worktree); fast-jump mode cancels cleanly (e.g. Escape)
  without residual overlay state; interaction latency stays acceptable with
  a realistically large number of open panes/targets.

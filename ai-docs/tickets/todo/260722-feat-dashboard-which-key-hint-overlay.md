---
title: Dashboard which-key-style leader hint overlay
parent: 260711-epic-ws-dashboard-command-surface
related:
  260722-feat-dashboard-hotkey-config-framework: provides the binding
    registry this overlay reads to render available leader-sub bindings
  260711-idea-dashboard-command-bus-quick-open-shortcuts: sibling command-bar
    layer; this overlay surfaces leader-sub bindings, the command bar
    surfaces prefix-triggered actions - both dispatch through the same
    command bus
related-mental-model:
  - ws-web-dashboard
---

# feat: Dashboard which-key-style leader hint overlay

## Background

Finalized behavior: see 260722-feat-dashboard-hotkey-config-framework §
Default Keymap & Interaction Spec (which-key overlay behavior).

Agenda A UX review (owner, 2026-07-22) finalized a tmux-style, leader-only
(no modal) dashboard keyboard model with `Ctrl+Space` as the leader key; see
`260722-feat-dashboard-hotkey-config-framework` for the full decision record
this ticket builds on. That framework ticket is the foundation: it defines
the binding registry, the leader-press dispatch mechanism, and the terminal
passthrough guard. This ticket is layer 2 of the intended stack — a
which-key/lazyvim-style hint overlay that appears on leader press and shows
the user what follow-up keys are available.

This is a discoverability layer, not a new dispatch mechanism: it reads the
hotkey config framework's binding registry and renders it, it does not
define new bindings or a new command path.

## Decisions (final - Agenda A, 2026-07-22)

- Depends on `260722-feat-dashboard-hotkey-config-framework`; must not ship
  before or independently of that framework, since it has nothing to render
  without the binding registry.
- Purely a presentation layer over the existing registry: on leader
  (`Ctrl+Space`) press, show a transient overlay (which-key/lazyvim style)
  listing the currently reachable leader-sub bindings and their bound
  actions/labels.
- Must respect the same no-modal, tmux-style transient-mode semantics as the
  framework: the overlay is visible only during the transient
  dashboard-command-mode window and dismisses when that window resolves,
  times out, or is cancelled (e.g. Escape) - it does not introduce a second,
  independent mode of its own.
- Must not interfere with the terminal passthrough guard already established
  by the framework ticket; the overlay is purely additive UI, not a new
  input-capture path.
- Layer sequencing: this sits between the framework (layer 1) and the
  command bar integration (layer 3, already specced in
  `260711-idea-dashboard-command-bus-quick-open-shortcuts`) and before
  hint-click/fast-jump (layer 4,
  `260722-feat-dashboard-hint-click-fast-jump`). Implement sequentially,
  after the framework ships, per the parent epic's ordering.

## Non-Goals

- Defining new bindings or a new dispatch path - all actions shown come from
  the framework's existing binding registry.
- Rewriting the command bar's prefix grammar or its own UI - the command bar
  remains its own surface, specced in
  `260711-idea-dashboard-command-bus-quick-open-shortcuts`.
- The hint-click/fast-jump on-screen label system
  (`260722-feat-dashboard-hint-click-fast-jump`) - visually similar in spirit
  (on-screen hints) but a distinct, later feature with a different trigger
  and purpose (mouse-target jumping vs. leader-sub key discovery).

## Phases

### Phase 1: Leader-press hint overlay

- Render a transient overlay on `Ctrl+Space` leader press that lists
  currently reachable leader-sub bindings (key, bound command label) sourced
  from the hotkey config framework's binding registry.
- Overlay lifecycle follows the framework's transient dashboard-command-mode
  window: appears on leader press, updates or narrows as the user types a
  partial sequence (if the framework supports multi-key leader-sub
  sequences), and dismisses on resolution, timeout, or cancel.
- Ensure the overlay does not capture or consume terminal input itself; it
  is a read-only visualization of the framework's existing capture state.
- Verify: overlay appears within a normal input-latency budget on leader
  press, reflects live registry contents (including user-configured
  rebindings from the framework's persistence layer), and disappears
  cleanly on every dismissal path (match, timeout, Escape) without leaving
  stale UI or blocking subsequent terminal input.

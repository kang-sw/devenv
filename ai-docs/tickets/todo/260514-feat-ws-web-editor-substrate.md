---
title: ws web browser-native editor substrate
parent: 260514-epic-ws-web-dashboard-mvp
related-mental-model:
  - developer-environment-tools
---

# ws web browser-native editor substrate

## Background

The primary editor path should avoid Windows PTY instability by using a
browser-native editor surface. CodeMirror 6 is the initial base, with Vim-like
modal editing through an existing extension or a custom ws modal layer.

Frontend UI implementation delegated through ws named agents should use
`model: "opus"` unless the user overrides that choice for this ticket.

## Phases

### Phase 1: Add CodeMirror editor panel

Create an editor panel contribution that can open text files, render buffers,
track dirty state, and save through daemon file APIs.

### Phase 2: Add Vim-like modal baseline

Add normal/insert/visual mode behavior through an existing CodeMirror Vim
extension or a minimal custom modal layer, with clear room to replace behavior
incrementally.

### Phase 3: Add editor workspace affordances

Add tabs or splits, search, open-file command integration, close/reopen flows,
and basic keyboard command registration through the frontend substrate.

### Phase 4: Prepare advanced editor hooks

Define extension points for diagnostics, completion, file watching, agent edit
actions, and future custom modal semantics without implementing the full IDE
surface in this ticket.

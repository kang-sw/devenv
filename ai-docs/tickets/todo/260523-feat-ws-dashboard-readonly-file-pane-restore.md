---
title: Restore dashboard read-only file panes after refresh or daemon restart
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-persist-open-workroots: remembered workRoots are the parent resource required before file panes can restore
  260523-feat-ws-dashboard-terminal-tab-restore: adjacent browser descriptor replay pattern for non-resumable surfaces
spec:
  - 260516-ws-dashboard-readonly-text-pane
  - 260516-ws-web-dashboard-workroot-io-restore-model
related-mental-model:
  - ws-web-dashboard
---

# Restore dashboard read-only file panes after refresh or daemon restart

## Background

Dashboard restart persistence currently restores opened workRoots and terminal
tab descriptors, but read-only editor/file panes remain React memory only. A
refresh or daemon restart loses preview and pinned file context even when the
owning workRoot is remembered.

This should follow the existing file-open and command-spine model rather than
persisting daemon-private file content. Restored panes re-read through the
authenticated file API, so deleted, binary, oversized, unreadable, or moved
files degrade through the normal read-only file pane unavailable/error states.

## Decisions

- Restore file panes quietly after browser refresh or daemon restart. Do not
  use toast-style global alerts for every pane restore failure.
- Keep failed restores pane-local: the tab/pane may come back, but its body
  shows the normal bounded unavailable or error state for missing, unreadable,
  binary, oversized, or moved files.
- Do not force selected workRoot changes during restore. A restored pane remains
  owned by the workRoot in its descriptor.
- Preserve current preview and pinned semantics exactly: one replaceable preview
  per workRoot, pinned panes keyed by workRoot-relative path, and
  preview-to-pinned removal of the matching preview pane.

## Phases

### Phase 1: Restore read-only file pane descriptors

Persist browser-visible read-only file pane descriptors per workRoot:
workRoot id, workRoot-relative path, preview/pinned mode, title/metadata needed
for tab reconstruction, and enough pane-order hint to avoid dumping every file
back into the first group. Do not persist absolute host paths, file contents,
daemon filesystem handles, or stale response bodies.

Restore should dispatch or share the same file-open command path used by normal
file explorer interactions so future keyboard bindings can target the same
behavior. Duplicate preview and pinned semantics must remain intact: one
replaceable preview per workRoot, pinned panes keyed by file path, and
preview-to-pinned removal of the matching preview pane.

Restore failures should use pane-local unavailable/error rendering rather than
global notification fan-out. Restoring a pane must not change the selected
workRoot merely because the pane belongs to another root.

Verification should cover pure descriptor storage, stale/missing file
degradation, duplicate preview/pinned behavior after restore, and browser-level
refresh/restart evidence.

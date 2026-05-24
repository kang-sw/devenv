---
title: Rework root picker as React Aria explorer-style folder picker
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260524-feat-ws-dashboard-root-picker-modal: completed local modal substrate that this ticket revises for interaction and visual quality
  260524-research-ws-dashboard-react-aria-ui-primitives: broader adoption research that should consume pilot findings
spec:
  - 260516-ws-web-dashboard-root-picker-empty-directory-creation
  - 260516-ws-web-dashboard-open-workroot-resource-refresh
  - 260524-ws-dashboard-root-picker-modal
  - 260524-ws-dashboard-react-aria-root-picker-pilot
  - 260524-ws-dashboard-root-picker-pins
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-24
---

# Rework root picker as React Aria explorer-style folder picker

## Background

The current dashboard root picker replaced the temporary left-nav path input
with a local modal over the authenticated root picker APIs. The behavior is
verified, but the UI still feels awkward: it is closer to a form plus directory
list than a desktop folder selection dialog.

The accepted direction is to run a focused React Aria pilot on the root picker
instead of declaring a dashboard-wide UI primitive migration. The picker should
feel closer to a Windows File Explorer folder selection dialog in folder-only
mode while preserving dashboard command semantics, owner-authenticated backend
routes, and open-workRoot resource reconciliation.

## Decisions

- Use React Aria Modal/Dialog for the picker shell and a controlled React Aria
  GridList for the current-folder details list.
- Use GridList as a details-style row list, not as a thumbnail grid. The initial
  visual mode is a one-dimensional folder list with room for columns such as
  name, kind/type, modified time, and future sort keys.
- Keep folder-only as a filter mode rather than a hard-coded data model. The
  first implementation may show only directories, but picker entries should
  remain compatible with future file rows, metadata columns, and alternate
  filters.
- Preserve `commands.ts` as the authoritative dashboard action contract. React
  Aria keys and events are adapted into existing or new dashboard commands.
- Host paths may be local component keys, navigation history entries, or
  authenticated request arguments; they must not appear in loggable dashboard
  command payload fields.
- Keep the root picker a folder selection dialog, not a broad file manager.
  Delete, rename, move, copy, chmod, recursive deletion, and arbitrary file
  open operations remain out of scope.
- Add built-in known places before persisted pinned directories. Persisted pins
  require dashboard persistence semantics and should be implemented only after
  the picker interaction model is stable.

## Phases

### Phase 1: React Aria explorer-style folder picker

Replace the local root picker modal shell and row list with a React Aria-based
folder selection dialog while preserving the existing backend routes and command
model.

The visible layout should be explorer-like:

- top ribbon with Back, Forward, Up, Refresh when useful, and an address field;
- central left sidebar for built-in places such as home, root, mounted/system
  paths, and platform-specific drive roots when available;
- central right details-style GridList for the current folder's visible entries;
- bottom selected-path input that can hold an absolute typed path or the
  selected entry path;
- bottom-right Open and Cancel buttons.

Navigation behavior:

- Back and Forward are browser-local picker navigation history, not backend
  state.
- Up and address submit call the authenticated root picker listing route through
  `rootPicker.navigate`.
- Selecting a row dispatches `rootPicker.selectDirectory`.
- Row action, Enter, or double click navigates into the selected folder.
- Footer Open dispatches `workRoot.open` for the selected or typed path.
- Cancel/close dispatches `rootPicker.close` and restores focus to the opener.

Entry model and filtering:

- The initial filter is folder-only.
- The picker model should be column-ready, with optional metadata such as
  kind/type label, modified time, size, and future sort keys.
- Metadata may be absent in the first backend response; the UI must degrade
  cleanly without fake values.
- File rows, alternate filters, sorting, and generic file-open behavior are
  deferred unless already needed to keep the model honest.

Built-in places:

- Provide a platform-aware known-places model for the left sidebar. It should
  include home when available, root or filesystem root when applicable, common
  Unix mount locations such as `/mnt` or `/media` when present, and Windows
  drive roots when reported by the daemon.
- Places should be derived through daemon-owned data rather than brittle
  frontend-only host assumptions.
- Unavailable places should either be hidden or shown as disabled/degraded
  without exposing private diagnostics.

Verification should include package/build evidence, command tests proving path
payload invariants, focused picker model tests for history/filter/places
behavior, and browser-level evidence against the daemon-served production
frontend. Browser evidence must cover mouse open, keyboard row selection and
row action, address-field navigation, exact typed path open, focus restore on
close, and no regressions to Dockview or xterm focus in adjacent dashboard
surfaces.

Deferred scope: persisted pinned directories, file rows, sort UI, destructive
filesystem actions, generic file manager behavior, and dashboard-wide React Aria
standardization.

### Result (0eb8525e) - 2026-05-24

Implemented Phase 1 root picker pilot:

- Added `react-aria-components` and replaced the local modal shell/list with a
  controlled React Aria `ModalOverlay`/`Dialog` plus details-style `GridList`.
- Reworked the visible picker into an explorer-style layout with Back/Forward
  local history, Up/Refresh/address navigation, daemon-derived built-in places,
  current-folder details rows, a selected/typed path footer, Open/Cancel
  actions, and retained single-segment empty-folder creation.
- Extended the authenticated root picker view with available known places and
  optional row metadata while keeping folder-only filtering and history helpers
  in the frontend model.
- Preserved `commands.ts` as the loggable command contract: host paths remain
  local picker keys or authenticated request data and are not included in
  `rootPicker.*` or `workRoot.open` payload fields.
- Split production vendor chunks so the React Aria package impact is visible
  and the Vite build remains warning-free.

Verification:

- `npm run test:commands`
- `npm run test:root-picker`
- `cargo test -p ws-dashboard-daemon root_picker`
- `npm run test:browser`

### Phase 2: Persist pinned picker directories

Add owner-managed pinned directory support for the root picker sidebar after the
React Aria picker interaction model is stable.

Pinned directories should be stored in dashboard-local persistence rather than
browser-only memory. The persisted model should avoid command payload path
leakage, distinguish built-in places from owner pins, allow unavailable pins to
degrade without breaking picker open, and provide explicit pin/unpin controls.

Pinned directories are selection/navigation affordances only. They must not
delete files, remove Git worktrees, rename directories, or silently create
workspace roots. Opening a pinned directory still routes through normal
root-picker listing and `workRoot.open` behavior.

Verification should cover persistence round trips, unavailable pinned
directories, pin/unpin command identities, no host path leakage in command
payloads, and browser evidence that pinned directories remain available across
refresh or daemon restart when persistence supports it.

### Result (f86e46c4) - 2026-05-24

Implemented Phase 2 pinned picker directories:

- Added daemon-local root picker pin persistence in the existing dashboard
  state file while preserving workRoot registry state on pin writes and pins on
  registry writes.
- Added authenticated pin/unpin routes returning refreshed picker places, with
  available pins navigable and unavailable pins degraded but still removable.
- Added `rootPicker.pinDirectory` and `rootPicker.unpinDirectory` command
  builders with path-free payloads, plus sidebar pin/unpin controls in the
  React Aria picker.
- Kept pins as navigation/selection affordances only. Pin/unpin does not open,
  create, delete, rename, move, or otherwise mutate filesystem/workRoot
  resources.

Verification:

- `npm run test:commands`
- `npm run test:root-picker`
- `cargo test -p ws-dashboard-daemon root_picker`
- `npm run test:browser`

---
title: Replace open workRoot path input with root picker modal
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260514-research-ws-web-dashboard-direction: dashboard direction keeps workRoot opening as a focused owner workflow rather than a broad file manager
spec:
  - 260516-ws-web-dashboard-root-picker-empty-directory-creation
  - 260516-ws-web-dashboard-open-workroot-resource-refresh
related-mental-model:
  - ws-web-dashboard
---

# Replace open workRoot path input with root picker modal

## Background

The dashboard currently exposes `OpenWorkRootControl` as an always-visible
manual path input above the resource navigation. That was useful as a temporary
substrate, but the accepted dashboard UX should let the owner pick a directory
through an explorer-like modal backed by the existing authenticated root picker
routes.

The replacement should preserve the current open-workRoot backend contract:
successful opens still route through the dashboard command layer, call the
open-workRoot API, consume the daemon-returned opened workRoot id when present,
reconcile the returned aggregated resource view, and re-fetch canonical
dashboard resources through the existing refresh coordinator.

## Decisions

- Replace the always-visible path input with an `Open...` entrypoint in the left
  navigation chrome.
- Open a modal directory picker for normal workRoot selection. The modal should
  provide current-path context, parent/up navigation, directory rows,
  keyboard-friendly selection, selected-directory footer actions, and bounded
  loading/error states.
- Keep a paste/type path affordance inside the modal for remote, long, or exact
  paths. The path field is a secondary affordance, not the primary left-nav
  surface.
- Keep the existing narrow `Create empty folder` backend capability available
  only as a single-segment empty-directory action. Do not add delete, rename,
  move, copy, chmod, recursive folder deletion, or generic file-manager
  operations.
- Prefer a small accessible dialog primitive or a local modal implementation
  over introducing a broad component framework. The directory tree/list remains
  dashboard-specific because it is backed by the root picker contract and
  workRoot open semantics.

## Phases

### Phase 1: Replace path input with modal picker

Implement a dashboard root picker modal that uses the authenticated root picker
listing route, open-workRoot route, and create-empty-folder route. The visible
left-nav opener routes through the dashboard command layer with the existing
`workRoot.open` command id when a directory is opened. The modal should support
mouse and keyboard selection, parent navigation, a secondary exact-path field,
loading/error/empty states, and a clear selected-directory footer.

The implementation must preserve the existing resource reconciliation rules:
the open response may update the browser immediately, the
`x-ws-dashboard-opened-work-root-id` header remains the preferred selector, and
the canonical resources endpoint remains the source of truth after open.

Deferred scope: broad file-manager operations, destructive filesystem changes,
bookmark/recent-root management, and cross-host root federation.

Verification should include focused frontend route/model tests for picker state
transitions and browser-level acceptance evidence against the daemon-served
production frontend. UI evidence must cover opening an existing workRoot
through the modal and the fallback exact-path field.

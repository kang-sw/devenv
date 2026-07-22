---
title: Dashboard git-status/diff inspector surface
parent: 260711-epic-ws-dashboard-command-surface
related:
  260722-feat-dashboard-hotkey-config-framework: keymap that focuses this
    surface via `<leader> g s`; this idea is the dependency that binding
    needs to land against
related-mental-model:
  - ws-web-dashboard
---

# idea: Dashboard git-status/diff inspector surface

## Background

While finalizing the hotkey keymap (`260722-feat-dashboard-hotkey-config-framework`,
2026-07-22), the owner raised a new UI surface: a source-control/editor-style
view for inspecting changed files' contents and diffs directly in the
dashboard, rather than only through the existing `git.*` command family
(refresh, fetch, push, pull-ff-only, branch menu/create).

The keymap's `g` (Git) group already reserves `<leader> g s` to focus this
surface once it exists (see that ticket's v2 draft, group `g`, leaf `s`).
That binding is currently a forward reference with no backing surface —
this ticket is the placeholder that binding depends on.

## Idea

Add a dashboard-native surface (modeled on VSCode's Source Control /
diff-editor UX) that shows:

- The set of changed files for the currently-active work root (status:
  modified/added/deleted/untracked, mirroring what `git.refresh` already
  polls for the Git toolbar).
- Per-file diff content, viewable inline or in a dedicated diff pane.

## Placement constraint

This surface must **not** live in the main pane, because the main pane's
position varies per layout (dockview panes can be rearranged, and different
users/layouts put different content there — there is no stable "the main
pane" to anchor a status/diff view to). A **dedicated right sidebar** is the
candidate placement, analogous to how VSCode docks Source Control in a side
panel independent of the main editor area. Exact docking/resize/dockview
integration details are open and left to implementation.

## Scale note

Acknowledged up front as a larger-scale addition, not a small polish item:
it likely needs its own file-list data source (or reuse of `git.refresh`'s
polling), a diff-rendering component (reuse an existing document
viewer/diff primitive if one exists, else build one), and dockview layout
integration for the new sidebar region. Should be scoped into its own
phased ticket before implementation starts, not attempted as a quick add-on.

## Related

- `260711-epic-ws-dashboard-command-surface` — parent epic coordinating the
  dashboard command-surface work this surface's `git.*` command family
  lives under.
- `260722-feat-dashboard-hotkey-config-framework` — keymap ticket whose
  `<leader> g s` binding targets this surface.
- The `git.*` command family in `ws-dashboard/frontend/src/commands.ts`
  (`git.refresh`, `git.fetch`, `git.push`, `git.pullFfOnly`,
  `git.branchMenu.open`, `git.branchCreate.open`) this surface complements.

## Open Points

- Exact sidebar docking mechanics (resizable, collapsible, dockview panel
  vs. fixed region) — deferred to implementation ticket authoring.
- Whether diff rendering reuses an existing document viewer primitive or
  needs a new one.
- Whether file-list data comes from a new polling source or reuses the
  existing Git-toolbar status poll.

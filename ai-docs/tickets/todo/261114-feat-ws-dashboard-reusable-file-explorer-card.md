---
title: "Deliver the reusable stateful file explorer card"
parent: 261114-epic-ws-dashboard-file-explorer
blocked-by: 261114-refactor-ws-dashboard-file-explorer-instance-isolation
related:
  261114-refactor-ws-dashboard-file-explorer-instance-isolation: "explicit prerequisite: first technical slice that isolates reusable explorer instances"
  261110-feat-ws-dashboard-file-preview: "existing read-only Files-tab and preview behavior"
  261110-feat-ws-dashboard-terminal: "existing shared board-card framework context"
---

# Deliver the reusable stateful file explorer card

## Background

The dashboard needs an owner-approved reusable file explorer card rather than
one singleton Files surface. This first user-facing slice builds on isolated
instances and captures the approved common card behavior. The epic continues
to represent the other approved explorer scope without assigning delivery
order here.

The approved tracked visual reference is [mockup.html](../assets/261114-epic-ws-dashboard-file-explorer/mockup.html).
Owner visual check 759 is APPROVED with the new design axis. Before ready
promotion, the approved tracked reference must be linked and verification must
require a side-by-side result/reference comparison; implementation acceptance
performs that comparison.

## Decisions

- The first user-facing delivery includes internal tabs with middle-click
  opening an inner background tab, details/list/icon/gallery view modes,
  per-window filename and content search with split search targeting the active
  pane, float and fullscreen presentation, and reuse from the conversation
  Files tab.
- The explorer remains read-only. Its operations are opening and previewing,
  searching, copying a path, and sending a path reference to chat; it does not
  modify the filesystem.
- Each explorer card stores its own browser state in browser storage. This is
  additive T0 browser behavior and does not create daemon or shared-store
  authority. The tracked visual asset and control-attachment UI are additive
  T0 content; an additive endpoint is T0 only when necessary. No existing root
  or API semantic rewrite is authorized.
- Code syntax highlighting and PDF preview are required in this first
  user-facing delivery. Their implementation mechanisms remain open.
- Existing normal Files guarded roots remain the access boundary. The
  separately approved workspace picker is local-viewer-only, directories-only,
  may browse home and local-volume roots labeled with the daemon host, and is
  not blanket file-content root widening. Do not add new symlink permissions.
- The reusable explorer core exposes selection/navigation events and a
  swappable bottom attachment slot. Browse cards offer `Send to session` and
  `Copy path`. The workspace picker uses the same core, hides files, shows
  directories only, and has a Select/Cancel row with hidden entries
  translucent. No separate picker UI is authorized.
- Technical event names, renderer libraries, and store fields remain
  unselected unless a real gap requires a new decision.
- The approved tracked visual reference is an acceptance authority. Before
  ready promotion, it must remain linked and verification must require a
  side-by-side result/reference comparison; implementation acceptance performs
  that comparison.

## Constraints

- Reuse the isolated explorer component for the conversation Files tab rather
  than creating a second Files-tab-specific contract.
- Do not add filesystem mutation, widen normal Files roots, add new shared
  persistence, or introduce unapproved storage fields, endpoints, dependencies,
  renderer libraries, API names, budgets, or root permissions.
- Preserve the existing read-only `/fs` list, preview, raw, find, and search
  authority and its local/remote guarded-root behavior. Browse-root validation
  is discoverable in `fs_routes.go` through `browseRootParam` and `containedDir`;
  the separately approved local-viewer, directories-only workspace picker is
  not blanket file-content root widening. Host-action and capability checks
  remain in `fs_act_route.go`, with narrow outside-linked-file grants in
  `fs_outside.go`; do not add new symlink permissions.
- Keep unresolved mechanism and grouping choices as unnumbered gaps for the
  lead; do not write them as settled decisions.

## Prior Art

- Current singleton ownership and browser state are evidenced by `dashboard.js`
  around the `#files` setup and storage/card-state helpers, and `cards.js`
  around the conversation and toolbar references.
- Existing filesystem authority is evidenced by the `/fs` list, preview, raw,
  find, and search surfaces. Browse-root validation is discoverable in
  `fs_routes.go` through `browseRootParam` and `containedDir`; the separately
  approved workspace picker is local-viewer-only and directories-only, not
  blanket file-content root widening. Host-action and capability checks remain in
  `fs_act_route.go`, with narrow outside-linked-file grants in `fs_outside.go`.
- Existing Files-tab and read-only preview behavior is recorded in
  `261110-feat-ws-dashboard-file-preview`.

## Phases

### Phase 1: Deliver the approved reusable explorer card base

Build the reusable card-facing behavior on isolated instances. Include the
approved internal tabs with middle-click opening an inner background tab,
details/list/icon/gallery modes, per-window filename and content search with
split search targeting the active pane, float/fullscreen presentation,
conversation Files-tab reuse, per-card browser storage, PDF preview, code
syntax highlighting, and the approved control-attachment axis. The reusable
core exposes selection/navigation events and a swappable bottom attachment
slot. The approved workspace-picker consumer uses that same core with
local-viewer-only, directories-only browsing, hidden files omitted, and a
Select/Cancel row. Keep all operations within their approved boundaries and do
not create a separate picker UI.

Verification should cover independent per-card state, the approved user-facing
entry points and modes, inner-background-tab middle-click behavior, active-pane
split search, conversation reuse, the control-attachment slot and actions,
the local-viewer directories-only workspace-picker consumer, PDF preview, code
highlighting, preservation of the existing normal Files root guards, and a
side-by-side result/reference comparison during implementation acceptance
against the approved tracked reference. Product source changes, tests,
browser/runtime verification, and visual acceptance belong to the later
implementation and review work; this draft records no Result.

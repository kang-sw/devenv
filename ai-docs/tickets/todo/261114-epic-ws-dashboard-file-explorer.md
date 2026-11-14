---
title: "Reusable stateful file explorer cards"
sage-review-design: required
related:
  261110-feat-ws-dashboard-file-preview: "existing read-only Files-tab and board-card evidence"
  261110-feat-ws-dashboard-terminal: "existing shared board-card framework context"
---

# Reusable stateful file explorer cards

## Scope

Deliver a reusable, stateful file explorer card for the dashboard. Explorer
instances are isolated rather than sharing the current singleton `#files`
instance. The first user-facing delivery covers internal tabs, details/list/icon/gallery
view modes, per-window filename and content search, float and fullscreen
presentation, and reuse from the conversation Files tab. The inner-tab scope
is approved, and middle-click opens an inner background tab.

The explorer remains read-only. Its operations are opening and previewing,
searching, copying a path, and sending a path reference to chat. It does not
modify the filesystem. Each card stores its own browser state in browser
storage. Code syntax highlighting and PDF preview are part of the first
user-facing feature delivery.

The existing guarded roots remain the access boundary. The design may leave
room for a future Tauri shell without widening roots now. Browse-root validation
is discoverable in `fs_routes.go` through `browseRootParam` and `containedDir`;
host-action and capability checks are in `fs_act_route.go`, and narrow,
expiring outside-linked-file grants are in `fs_outside.go`. The separately
approved workspace picker is local-viewer-only and directories-only, not blanket
file-content root widening. The current singleton ownership is evidenced by
`dashboard.js` around the `#files` card setup and `cards.js` around the
conversation and toolbar references; current browser card state is evidenced
by `dashboard.js` around its storage and card state helpers.

The approved tracked visual reference is [mockup.html](../assets/261114-epic-ws-dashboard-file-explorer/mockup.html).
Owner visual check 759 is APPROVED with the new design axis. Before ready
promotion, the approved tracked reference must be linked and verification must
require a side-by-side result/reference comparison; implementation acceptance
performs that comparison.

## Non-Scope

- Filesystem mutation, including create, rename, move, delete, copy, chmod,
  or write-back editing.
- Widening normal Files guarded roots or treating the separately approved
  local-viewer, directories-only workspace picker as file-content authority.
- A new daemon or shared persistence store for explorer state.
- Unapproved endpoints, dependencies, renderer libraries, storage fields, API
  names, budgets, or permission changes.
- Treating the approved visual reference as authorization for unapproved
  mechanisms, permissions, or API semantics.

## Owner-Approved Provenance

MAIN relayed the owner answers at `2026-11-14T12:55:59Z`: decision IDs 741,
744, and 746 are `a`; 742 is `ok`; and 745 is `a` with room for future
Tauri-shell widening. MAIN's final answers at 13:22, 13:23, and 13:25 confirm
755=`a` (browse home and local-volume roots, labeled with the daemon host),
756=`a` (directory picker is local-viewer-only), 757=`b` (hidden entries are
shown translucent, with no new symlink permissions), 759=`ok` (the tracked
visual reference is approved with the new design axis), 760=`a` (middle-click
opens an inner background tab), and 761=`a` (split search targets the active
pane). At 13:23:42, the owner approved the control-attachment design: the
reusable explorer core exposes selection/navigation events and a swappable
bottom attachment slot; browse cards offer `Send to session` and `Copy path`,
and the workspace picker uses that same core with directories only, hidden
files omitted, and a Select/Cancel row. There is no separate picker UI. These
IDs record provenance only and do not settle technical event names, renderer
libraries, or store fields.

## Child Tickets

- `261114-refactor-ws-dashboard-file-explorer-instance-isolation` - first
  technical slice: isolate explorer instances while preserving current
  behavior and security.
- `261114-feat-ws-dashboard-reusable-file-explorer-card` - first user-facing
  slice: deliver the reusable card base, approved tabs and view modes,
  per-window search, per-card browser state, conversation reuse, PDF preview,
  code syntax highlighting, and the approved control-attachment axis.
- Approved scope to be represented by a child: add the agent change-tracking
  view.
- Approved scope to be represented by a child: add Git badges and a diff tab.
- Approved scope to be represented by a child: add worktree/root switching
  without widening guarded roots.
- Approved scope to be represented by a child: support drag-to-chat path
  references.
- Approved scope to be represented by a child: add ticket and board links.
- Approved scope to be represented by a child: add a path bar.
- Approved scope to be represented by a child: add tree and column views.
- Approved scope to be represented by a child: add space quick-look.
- Approved scope to be represented by a child: add bookmarks/recent entries
  and a hidden/ignored toggle.
- Approved scope to be represented by a child: add split dual-pane browsing.
- Approved scope to be represented by a child: add keyboard navigation.
- Approved scope to be represented by a child: virtualize large directories.

These approved scope items are not ordered here; their grouping and
mechanisms are not settled. The approved workspace-picker consumer depends on
the reusable card and is not a separate child ticket; owner 771 owns new
picker authoring.

## Cross-Child Decisions

- Explorer instances are isolated. No child may restore the current singleton
  as the state owner for all explorer cards.
- Browser storage is additive per-card browser behavior and is T0. It stores
  browser-owned view state rather than daemon authority or a new shared store.
- Normal Files browsing remains read-only and keeps the existing guarded-root
  behavior. The separately approved workspace picker is local-viewer-only,
  directories-only, may browse home and local-volume roots labeled with the
  daemon host, and is not blanket file-content root widening. Do not add new
  symlink permissions.
- Compatibility is T0 only for additive UI or an additive endpoint when
  necessary; no existing root or API semantic rewrite is authorized. Technical
  event names, renderer libraries, and store fields remain unselected unless a
  real gap requires a new decision.
- The conversation Files tab reuses the same explorer component rather than
  creating a second explorer contract.
- Internal tabs, middle-click inner background tabs,
  details/list/icon/gallery modes, per-window filename and content search with
  split search targeting the active pane, float/fullscreen presentation, PDF
  preview, and code syntax highlighting are part of the approved first
  user-facing delivery.
- The reusable core exposes selection/navigation events and a swappable bottom
  attachment slot. Browse cards offer `Send to session` and `Copy path`; the
  workspace picker uses the same core, hides files, shows directories only, and
  has a Select/Cancel row with hidden entries translucent. No separate picker
  UI is authorized.
- The approved scope includes room for a future Tauri-shell expansion, but
  this epic does not widen normal Files content roots; the separately approved
  local directories-only picker keeps its stated boundary.
- The approved tracked visual reference is linked above and is an acceptance
  authority. Before ready promotion, it must remain linked and verification
  must require a side-by-side result/reference comparison; implementation
  acceptance performs that comparison.
- No child may introduce an unconfirmed implementation mechanism or persist an
  unconfirmed decision as settled. Exact event names, renderer libraries, and
  store fields remain technical choices, not new policy questions, unless a
  real gap emerges.

## Completion Criteria

- Done: the isolated reusable explorer card and all approved explorer-scope
  children are implemented without changing normal Files root guards, and the
  read-only operations, per-card browser state, conversation reuse, approved
  view/search behavior, workspace-picker consumer, PDF preview, and code
  syntax highlighting are verified by their child tickets.
- Dropped: the owner replaces this direction, or the required instance
  isolation and read-only security boundary cannot be preserved.
- Deferred: any mechanism or child grouping not explicitly settled above;
  the required side-by-side result/reference comparison during implementation
  acceptance; and any future backend surface until its additive semantics are
  separately settled without rewriting existing root or API semantics.

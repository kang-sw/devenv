---
title: Unwire the workroot activity ("agents") badge to drop its always-on load
sage-review-design: required
---

# Unwire the workroot activity ("agents") badge to drop its always-on load

## Background

The workbench toolbar renders a `WorkbenchActivityBadge` (`App.tsx:6459`,
component in `workbenchChips.tsx:115-149`) — the "agents" indicator. Its data
comes from React state `workRootActivityState` (`App.tsx:3717-3721`), fed by
`fetchWorkRootActivity` → `GET /api/dashboard/work-roots/{id}/activity`.

The ws-mcp agents feature this badge reflects is effectively obsolete, so the
badge is stale weight. It also adds an **always-on fetch**: every workroot
selection triggers a 300 ms-debounced one-shot activity fetch (**Effect A**,
`App.tsx:4517-4554`) regardless of whether the Activity Console pane is open.
The goal is to remove that added load while **keeping the rendering/projection
logic dormant in the tree** for a possible future revival (user directive: do
not delete the logic, just cut the wiring).

## Decisions

- **Cut Effect A only** (`App.tsx:4517-4554`) — the sole always-on, badge-only
  load surface. Removing/disabling its debounced `fetchWorkRootActivity` call
  eliminates the per-selection traffic. The daemon endpoints are purely
  request-driven, so removing the client caller removes the load with **no
  daemon change**.
- **Leave the badge component, `workRootActivityBadge` projection, and the
  daemon routes/projector intact** (dormant). Reviving the feature later means
  restoring the one wiring point.
- **Hide the badge when unwired** rather than leave it stuck rendering
  `"agents" / loading`. With Effect A gone, `workRootActivityState` would sit at
  `{phase:"loading"}` forever and the badge would show a permanent spinner;
  suppress the badge in that dormant state so it disappears from the toolbar in
  normal use.
  - *Rejected:* leaving a static dormant placeholder badge. User chose full
    removal of the visual, not a placeholder.
- **Do NOT touch Effects B/C** (`App.tsx:4560-4771` SSE stream;
  `App.tsx:4773-4853` 3 s poll fallback). They are already self-gated on
  `activityPaneOpenForSelected`, so they add **zero idle load** when the Activity
  Console pane is closed, and they also power that pane's live transcript/items —
  cutting them would degrade the still-available pane beyond just the badge.

## Constraints

- Sibling toolbar badges (`StateBadge` ← `root.state`, `WorkRootGitToolbar` ←
  git status, availability/activation chips ← `root.availability`/`.activation`)
  do **not** read `workRootActivityState`; the cut must not touch them.
- The Activity Console pane must remain functional when explicitly opened (it
  drives its own fetch/SSE via Effects B/C).

## Prior Art

- `workRootActivity.ts:997` `workRootActivityBadge` — the projection kept dormant.
- `work_root_activity.rs` / `router.rs:412-423` — request-driven daemon routes
  kept intact; no server-side polling to remove.

## Phases

### Phase 1: Remove the always-on activity fetch and hide the dormant badge

Disable Effect A's per-selection `fetchWorkRootActivity` so no activity request
fires on workroot selection, and suppress `WorkbenchActivityBadge` while
`workRootActivityState` is in its dormant/loading state so it no longer renders.
Keep the badge component, projection, Effects B/C, and daemon routes unchanged.

Verification boundary: selecting a workroot issues no
`GET .../work-roots/{id}/activity` request (verify via network/daemon logs) and
the "agents" badge is absent from the toolbar; sibling badges (state, git,
availability/activation) still render; opening the Activity Console pane still
loads activity through Effects B/C.

## Spec Impact

Target spec area: none in the workflow spec set — downstream ws-dashboard UI/load
behavior with no workflow-system contract. Caller-visible change is limited to
the toolbar (badge removed) and the elimination of one client-initiated fetch.

Contract-first spec: no.

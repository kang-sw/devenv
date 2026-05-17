---
title: ws dashboard WorkRoot Activity projection
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260513-feat-async-exec-output-reader: future running-command activity source
  260514-research-ws-web-dashboard-direction: prior dashboard activity and instance projection research
spec:
  - 260517-ws-dashboard-workroot-activity-projection
  - 260517-ws-dashboard-workroot-activity-topbar-badge
  - 260517-ws-dashboard-workroot-activity-pane
skeletons:
  phase-1: 43049cb
plans:
  phase-1: 2026-05/17-260517-feat-ws-dashboard-workroot-activity-phase-1
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
---

# ws dashboard WorkRoot Activity projection

## Background

The dashboard needs a workRoot-scoped runtime activity view before deeper
agent-facing UI work. Named agents and future async command jobs are stored and
addressed through filesystem-backed workRoot/worktree state, so their dashboard
summary should be owned by the selected workRoot rather than by a terminal pane
or a future agent GUI pane.

The first slice should expose read-only named-agent activity from daemon-owned
wsstate/wsagent projections, render a compact summary in the existing workRoot
top-bar badge row, and open a reversible WorkRoot Activity pane for detail. This
ticket is a child of the dashboard MVP, not a new epic: it defines one cohesive
runtime-context surface and leaves command execution, agent control, and future
agent GUI work to later tickets.

## Decisions

- WorkRoot Activity is workRoot-owned runtime context. Terminal panes and future
  agent GUI panes may link to it, but they must not own or fork the activity
  model.
- The compact projection renders in the existing workRoot top-bar badge row.
  Implementation must not add a new toolbar row or increase the top-bar height.
- The detail surface is a reversible WorkRoot Activity workbench pane. Closing it
  requires no confirmation and does not affect daemon agent state.
- Opening the WorkRoot Activity detail pane defaults to group 1, the
  agent/terminal-side split, even though it is a reversible projection surface.
  This is an explicit placement exception to the usual opened/support surface
  default.
- Running Commands remain reserved until `260513-feat-async-exec-output-reader`
  provides the underlying exec job model.

## Constraints

- The browser must not read `~/.cache/ws@kang-sw-devenv/` directly. The daemon
  owns any wsstate/wsagent reads and exposes a stable authenticated projection
  view model.
- The activity badge must fit the current top-bar density. At narrow widths,
  truncate, compact, or hide secondary text rather than wrapping the badge row
  and growing the toolbar.
- The badge label may use compact user-facing wording such as `agents: N active`,
  but implementation names should avoid treating these records as children of a
  specific main agent instance.
- The initial implementation is read-only. Starting, cancelling, interrupting,
  or erasing agents from the dashboard belongs to later control tickets.

## Prior Art

- `260513-feat-async-exec-output-reader` defines the future persisted command
  job source that can later feed the same WorkRoot Activity pane.
- `260514-research-ws-web-dashboard-direction` anticipated dashboard runtime
  projections but did not define a concrete WorkRoot Activity surface.
- Existing dashboard workbench policy already distinguishes durable
  daemon-owned panes from reversible projection panes; this ticket adds one
  placement exception rather than changing the general policy.

## Phases

Implement Phase 1 first. Phases 2 and 3 consume the projection from Phase 1 and
touch separate browser layout and workbench placement surfaces.

### Phase 1: Add read-only WorkRoot activity projection

Add a daemon-owned authenticated projection for the selected/opened workRoot's
named-agent activity. The projection should summarize agent identity, backend or
model metadata when available, terminal status, current-call state, last-call
time, and bounded follow-up/detail hints without exposing host cache paths as
browser API identity.

The projection should derive from wsstate/wsagent state rather than introducing
a parallel dashboard cache. Missing, malformed, or stale agent records should
produce bounded unavailable or diagnostic states instead of failing the whole
workRoot activity response.

### Result (7c49130) - 2026-05-17

Implemented the authenticated
`GET /api/dashboard/work-roots/{workRootId}/activity` route for opened
workRoots. The daemon now derives wsstate-compatible Git worktree agent
directories, scans read-only named-agent metadata and current-call state, and
returns bounded `WorkRootActivityView` rows without exposing host paths, cache
paths, session ids, pids, or stream paths.

Malformed or missing agent/current-call records degrade individual rows rather
than failing the whole route. Non-Git, bare-repository, or no-agent workRoots
return an empty `ok` projection for Phase 1. Verification covered daemon route
auth, unknown roots, empty projections, fixture agent records, malformed rows,
linked-worktree layout, Windows/non-UTF-8 hash compatibility, and frontend route
helpers.

Forward: consider a shared daemon Git subprocess/path-discovery seam before more
features duplicate Git probing logic; review accepted keeping that refactor out
of Phase 1.

### Phase 2: Add top-bar activity badge projection

Render a compact named-agent activity badge in the existing workRoot top-bar
badge row. The badge should summarize counts such as active, blocked, or failed
agents and remain a summary/entrypoint only.

The top bar must keep its current height. Add browser evidence that the badge
does not introduce a new row, does not wrap the toolbar under the covered
viewports, and does not reduce terminal/workbench usable height beyond the
existing toolbar footprint.

### Phase 3: Add WorkRoot Activity workbench pane

Add a reversible WorkRoot Activity pane that shows the detailed read-only
projection and opens or focuses when the top-bar activity badge is clicked.

The pane should use a distinct workbench surface kind, close immediately without
confirmation, and default to group 1. Duplicate opens for the same selected
workRoot should focus the existing pane instead of creating duplicates. The
browser gate should prove badge click, group-1 placement, duplicate focus, and
ordinary close behavior.

Running-command rows should remain absent or explicitly empty until
`260513-feat-async-exec-output-reader` lands.

---
title: Backend "watched work-root" subscription/push channel to replace fixed-interval polling
related:
  260714-feat-dashboard-multi-server-workbench-keepalive: follow-up optimization - that ticket's Non-Goals explicitly defers this as a separate idea ticket and confirms polling needs no change for its own core fix
related-mental-model:
  - ws-web-dashboard
---

# Backend "watched work-root" subscription/push channel to replace fixed-interval polling

## Background

This is a follow-up to
`260714-feat-dashboard-multi-server-workbench-keepalive` (keeping multiple
linked servers' workbenches mounted-but-hidden in parallel instead of tearing
them down on server focus switch). That ticket's Non-Goals section explicitly
carves this out:

> Backend "watched work-root" push channel is a SEPARATE follow-up idea ticket
> (out of scope here).

and its Decisions/Constraints confirm polling is explicitly untouched by that
work: "Polling: NO changes. Only the visible/active root polls today and that
stays. Non-focused On servers retain their last cached tree until re-focused;
they do not begin background polling." In other words, **this idea is a
possible future optimization, not a dependency of the parallel-server fix** -
the owner confirmed the core parallel-server fix needs no polling changes at
all.

Today, the frontend refreshes git/resource/activity state for the focused work
root via fixed-interval client-side polling (e.g. the ~5s git status timer
noted in `260525-feat-ws-dashboard-workroot-polishing-backlog` Phase 2, plus
similar interval-driven refresh paths for resources/activity elsewhere in
`App.tsx`). The idea explored here: instead of the client blindly polling on a
timer, let the backend expose which work root is currently "watched"/focused
by a connected client and push updates to it (a subscription or event-stream
model) so refreshes become event-driven rather than interval-driven.

## Required First Step: Prior-Art Check

Before designing anything, **check existing live-streaming/subscription prior
art in this codebase** - there is already a live single-subscription concept
for activity data in the frontend:

- `currentActivityStreamRequest` (`App.tsx:3505-3509`, a
  `useRef<ActivityConsoleStreamRequest>` tracking `{ serverRoute, workRootId,
  requestId }`) appears to already model "which work root/server route is
  currently being live-streamed," with usages threading through
  `App.tsx:4252-4434` (stream-request sequencing/staleness guards).
- The spec area `ai-docs/spec/ws-web-dashboard/index.md` has an existing
  `## Activity Console Watch Stream` section
  (anchor `260521-ws-dashboard-activity-console-watch-stream`) and a related
  `## Activity Console Live UX` section
  (`260521-ws-dashboard-activity-console-live-ux`) that likely already
  document a live/watch-stream contract for activity data specifically.

Any design here must first establish: does the existing activity watch-stream
already cover (or partially cover) the "which work root is being watched"
concept this idea wants for git/resource polling too? If so, this ticket may
reduce to "generalize/reuse the existing activity watch-stream subscription
model for git/resource refresh," rather than inventing a second, parallel
subscription mechanism. If the existing stream is narrowly activity-only and
architecturally cannot generalize, note why before proposing a new channel.

## Direction (not yet scheduled)

- Backend exposes a notion of "this client is watching work root X" (likely
  tied to the existing per-server-route/work-root focus concept already used
  for activity streaming).
- Backend pushes updates (git status changes, resource-tree changes, activity
  events) for the watched work root instead of the client polling on a fixed
  interval.
- Client-side fixed-interval polling (git status timer, resource/activity
  refresh timers) is replaced or supplemented by handling pushed events.
- Scope question to resolve during design: does this cover git status only,
  the broader resource tree, activity, or all three uniformly through one
  channel vs. per-concern channels.

## Constraints

- This is framed as an **optimization**, not a blocker or prerequisite for any
  currently-planned feature. `260714-feat-dashboard-multi-server-workbench-keepalive`
  explicitly does not depend on it and needs no polling changes for its own
  core fix to land.
- No phases yet - this ticket captures the idea and its required first
  investigation step; it is not implementation-ready.

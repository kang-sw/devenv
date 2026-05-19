---
title: ws dashboard WorkRoot Activity SSE refresh
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260518-epic-ws-dashboard-activity-console: broader Activity Console epic that absorbs this narrow refresh scope
  260518-feat-ws-dashboard-activity-watch-stream: replacement child ticket for watcher and feed stream work
  260517-feat-ws-dashboard-workroot-activity: dogfood showed the implemented pane needs live refresh after named-agent cache changes
  260513-feat-async-exec-output-reader: future persisted command activity may share the same refresh/stream path
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard WorkRoot Activity SSE refresh

## Background

After `260517-feat-ws-dashboard-workroot-activity` landed the daemon projection,
top-bar badge, and detail pane, dogfood with a newly registered/called named
agent showed that the Activity view does not grow automatically while the
dashboard stays open.

The immediate hotfix is bounded polling while the Activity pane is open: the
frontend keeps the initial full projection, then periodically asks the daemon
for only the recently updated agent rows and merges them client-side. That keeps
the UI usable without repeatedly fetching a monotonically growing full list.

The durable solution should replace that polling bridge with a workRoot-scoped
SSE or WebSocket activity invalidation stream. Since the dashboard daemon and
wsagent cache are local, the daemon can watch the resolved workRoot agents
directory and push bounded invalidation or row-update events when agent
directories, `agent.json`, `current/state.json`, or output/runtime files change.

The implementation must be cross-platform. File watching should handle atomic
renames and nested current-call files on macOS, Linux, and Windows, degrade to a
bounded polling fallback when watcher support is unavailable, and connect only
for the selected workRoot instead of watching every opened root eagerly.

Acceptance should prove that a newly registered named agent appears in the
top-bar badge/detail pane without reloading the browser, call status transitions
update while a call is running or completes, and the polling hotfix can be
removed or reduced to a watcher fallback.

## Dropped

This narrow idea was absorbed into the broader Activity Console cascade. The
durable work now lives in `260518-feat-ws-dashboard-activity-watch-stream`,
which covers the same polling replacement plus the feed event stream needed by
the reusable Activity Ribbon and selected transcript viewer.

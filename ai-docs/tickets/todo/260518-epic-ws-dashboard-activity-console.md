---
title: ws dashboard Activity Console
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260517-feat-ws-dashboard-workroot-activity: completed first read-only named-agent projection, badge, and pane substrate
  260517-feat-ws-dashboard-workroot-activity-live-refresh: narrow polling-replacement idea absorbed into the Activity Feed stream child
  260513-feat-async-exec-output-reader: future exec jobs should become feed items and transcript sources
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
  - mcp-runtime
---

# ws dashboard Activity Console

## Scope

Promote WorkRoot Activity from a read-only named-agent list into a reusable
read-only Activity Console for live/latest workRoot activity. The console
centers on a compact activity ribbon and a selected transcript viewer rather
than a vertically dumped list of agent cards.

The milestone covers:

- A workRoot-scoped Activity Feed snapshot API that generalizes the current
  named-agent projection into selectable activity items.
- A cross-platform daemon watch and stream path so live/latest feed state
  updates without browser reloads or repeated full-list polling.
- A selected Activity Transcript API that returns normalized transcript blocks
  from daemon-owned sources without exposing cache, session, process, or host
  paths.
- A reusable frontend Activity Console made from an Activity Ribbon and a
  Transcript Block viewer, usable inside the WorkRoot Activity pane now and
  popup-style main-agent surfaces later.

## Non-Scope

- Agent start, interrupt, cancel, erase, retry, or other control actions.
- Making the dashboard the ws MCP or named-agent session authority.
- Full async exec job implementation; exec work may reserve feed item kinds but
  remains blocked on `260513-feat-async-exec-output-reader`.
- Writable editor, terminal control, or generic file-manager behavior.
- Public internet or multi-user authorization semantics.

## Child Tickets

- `260518-feat-ws-dashboard-activity-feed-api` - todo; define and implement the
  Activity Feed snapshot model, item identity, ordering semantics, and
  compatibility path from the current WorkRoot Activity projection.
- `260518-feat-ws-dashboard-activity-watch-stream` - todo; replace the bounded
  polling hotfix with a workRoot-scoped cross-platform watcher plus feed event
  stream and fallback mode.
- `260518-feat-ws-dashboard-activity-transcript-api` - todo; resolve selected
  activity items to normalized transcript blocks, starting from ws named-agent
  state and preparing backend-native Codex/Claude/Gemini adapters.
- `260518-feat-ws-dashboard-activity-console-ui` - todo; build the reusable
  Activity Ribbon and Transcript Block viewer and migrate the WorkRoot Activity
  pane to the Activity Console shape.

## Cross-Child Decisions

- Use **Activity Feed** for the workRoot-scoped live/latest item list,
  **Activity Item** for a selectable feed entry, **Activity Ribbon** for the
  horizontal item UI, **Transcript Block** for a normalized render unit, and
  **Activity Console** for ribbon plus selected transcript viewer.
- Keep the console read-only. If a future terminate action is needed, it must
  be a separate high-friction control ticket and must not broaden this
  milestone into an agent-control surface.
- Feed ordering defaults to live/active/attention items first, then latest
  updated activity. A-Z ordering is not the primary user value.
- The browser must not read wsstate, wsagent, backend session, process, or host
  paths directly. All transcript and activity source resolution is daemon-owned.
- Prefer SSE for read-only Activity Feed and transcript streams unless a child
  ticket records a concrete need for bidirectional WebSocket semantics.
- Watchers should connect only for selected or otherwise visible workRoots and
  degrade to bounded polling when platform watcher behavior is unavailable or
  unreliable.
- The console component must not be named-agent-specific. Named agents are the
  first feed source; main agent sessions, exec jobs, diagnostics, and other
  runnable/readable activity must fit the same item and transcript contracts.
- Reuse the existing instance event envelope only where it clarifies stream
  shape. Do not expose the older `instance-events` scaffold name as the Activity
  Feed product vocabulary.

## Completion Criteria

- Done: selected workRoot activity renders as a reusable Activity Console with a
  horizontal live/latest ribbon, selected transcript blocks, live update
  behavior, and browser-level verification.
- Dropped: dashboard activity visibility moves to a different primary UX
  concept and the ribbon/transcript model is no longer desired.
- Deferred: agent controls, broad exec job support, main-agent popup
  integration, and backend adapters beyond the accepted transcript source scope
  may continue in later children.

---
title: ws dashboard Activity Console
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260517-feat-ws-dashboard-workroot-activity: completed first read-only named-agent projection, badge, and pane substrate
  260517-feat-ws-dashboard-workroot-activity-live-refresh: narrow polling-replacement idea absorbed into Activity Console live update children
  260513-feat-async-exec-output-reader: future exec jobs should become feed items and transcript sources
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
  - mcp-runtime
---

# ws dashboard Activity Console

## Scope

Promote WorkRoot Activity from a read-only named-agent list into a full-scale
read-only Activity Console for live/latest workRoot activity. The console
centers on a compact activity ribbon and a selected transcript viewer rather
than a vertically dumped list of agent cards.

The milestone covers:

- A backend Activity Read Model that combines the Activity Feed snapshot and
  selected transcript backfill needed by the console.
- A polished reusable Activity Console UI shell with an Activity Ribbon and
  Transcript Block viewer inside the WorkRoot Activity pane.
- A cross-platform daemon watch and read-only stream path for live Activity
  Feed and transcript invalidation events.
- Frontend live update adoption that merges daemon stream events without stale
  workRoot updates or always-on full-list polling.
- Transcript source expansion beyond the MVP ws named-agent backfill, starting
  with native Codex session resolver support where stable evidence exists.

## Non-Scope

- Agent start, interrupt, cancel, erase, retry, or other control actions.
- Making the dashboard the ws MCP or named-agent session authority.
- Full async exec job implementation; exec work may reserve feed item kinds but
  remains blocked on `260513-feat-async-exec-output-reader`.
- Writable editor, terminal control, or generic file-manager behavior.
- Public internet or multi-user authorization semantics.

## Child Tickets

- `260521-feat-ws-dashboard-command-dispatch-spine` - done; Phase 0 before
  Activity Console UI work, establishing the shared command dispatch path that
  future tmux-like keybindings and visible controls invoke.
- `260518-feat-ws-dashboard-activity-read-model` - done; implement the backend
  feed snapshot plus selected transcript backfill contract that the console
  reads.
- `260518-feat-ws-dashboard-activity-console-ui` - done; built the reusable
  Activity Ribbon, Transcript Block viewer, and WorkRoot Activity pane shell
  against the read model.
- `260518-feat-ws-dashboard-activity-watch-stream` - done; implemented the
  daemon-side SSE feed event stream, cursor/reset behavior, and bounded
  polling fallback mode.
- `260518-feat-ws-dashboard-activity-live-ux` - done; adopted the live stream in
  the Activity Console frontend with merge, stale-root, transcript-refresh, and
  fallback behavior.
- `260518-feat-ws-dashboard-activity-transcript-api` - todo; expand transcript
  sources and block-level live transcript behavior after the read model and
  live update foundation are in place.

## Cross-Child Decisions

- Use **Activity Feed** for the workRoot-scoped live/latest item list,
  **Activity Item** for a selectable feed entry, **Activity Ribbon** for the
  horizontal item UI, **Transcript Block** for a normalized render unit, and
  **Activity Console** for ribbon plus selected transcript viewer.
- The Activity Console is a dense operational surface. The ribbon optimizes
  scan and selection through compact item rows, while the selected transcript
  optimizes reading the chosen activity without turning the ribbon into a full
  transcript.
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
- Activity Console controls must keep the dashboard command path intact:
  ribbon selection, transcript refresh/load-more, pane open/focus, and any
  future visible control need stable command ids that dispatch through the same
  behavior future tmux-like keybindings will invoke. Background stream or
  polling merges are data effects, not user commands.
- Activity Console UI implementation starts after
  `260521-feat-ws-dashboard-command-dispatch-spine` lands, so new controls do
  not add raw callback paths that a future keybinding layer must rediscover.
- Reuse the existing instance event envelope only where it clarifies stream
  shape. Do not expose the older `instance-events` scaffold name as the Activity
  Feed product vocabulary.

## Completion Criteria

- Done: selected workRoot activity renders as a reusable Activity Console with a
  horizontal live/latest ribbon, selected transcript blocks, live update
  behavior, browser-level verification, and at least one native transcript
  source expansion beyond the basic ws named-agent read model.
- Dropped: dashboard activity visibility moves to a different primary UX
  concept and the ribbon/transcript model is no longer desired.
- Deferred: agent controls, broad exec job support, main-agent popup
  integration, and backend adapters beyond accepted transcript source scope may
  continue in later epics or children.

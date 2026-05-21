---
title: ws dashboard Activity Console UI shell
parent: 260518-epic-ws-dashboard-activity-console
related:
  260521-feat-ws-dashboard-command-dispatch-spine: prerequisite command spine for Activity Console controls
  260518-feat-ws-dashboard-activity-read-model: supplies Activity Feed items and selected transcript blocks
  260518-feat-ws-dashboard-activity-live-ux: later adds stream-backed updates to this shell
  260517-feat-ws-dashboard-workroot-activity: existing pane to migrate from list dump to console
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard Activity Console UI shell

## Background

The current WorkRoot Activity pane renders every named agent as a full-width
card in one vertical list. This makes a populated workRoot hard to scan and
does not provide a useful transcript reading surface. The desired UI shell is a
reusable Activity Console: a roughly 60px horizontal Activity Ribbon showing
live/latest items and a selected Transcript Block viewer below it.

The same component family should later support popup-style transcript views
from a main agent surface, so it must not be coupled to WorkRoot Activity pane
placement or named-agent-specific metadata.

This ticket should start after `260521-feat-ws-dashboard-command-dispatch-spine`
lands. Activity Console adds enough new controls that implementing it before
the command spine would grow keybinding debt.

## Decisions

- Build reusable `ActivityRibbon`, `TranscriptBlockViewer`, and
  `ActivityConsole` components or equivalent local modules with dependency
  injection for item rendering and transcript loading.
- The ribbon defaults to live/active/attention items first, then latest updated
  activity. Live items use a green outline or equivalent semantic active
  treatment without turning the whole UI into a single-hue theme.
- Selecting a ribbon item renders transcript blocks below. Selection should
  survive snapshot refreshes where the selected item still exists.
- The WorkRoot Activity pane remains a reversible read-only workbench surface
  and should continue to close immediately without daemon side effects.

## Constraints

- No visible in-app tutorial text explaining the feature. Use normal labels,
  statuses, and affordances.
- Text must fit inside ribbon buttons and transcript blocks across desktop and
  mobile viewports; the ribbon scrolls horizontally rather than wrapping.
- UI work must include browser-level visual and interaction verification
  against the daemon-served frontend, not only TypeScript tests.
- Activity Ribbon selection, transcript viewer commands, pane open/focus, and
  refresh or load-more controls must expose stable command ids and route through
  the dashboard command dispatch path; the clicked behavior must be the same
  behavior future tmux-like keybindings invoke.
- Do not add agent control buttons. Future terminate behavior, if accepted,
  belongs to a separate ticket.
- Live SSE consumption is out of scope for this shell ticket; it belongs to
  `260518-feat-ws-dashboard-activity-live-ux`.

## Phases

### Phase 1: Build the route-backed Activity Console UI shell

Implement the horizontal Activity Ribbon, normalized Transcript Block viewer,
and Activity Console composition inside the WorkRoot Activity pane. Use the
Activity Console read model for route-backed data and deterministic fixtures
for component states. Default-select the first live/attention item, or the
latest item when no live item exists.

Verification should cover ordering, selection preservation, horizontal
overflow, active/live styling, long text wrapping, tool/status/output blocks,
empty/degraded/running/completed states, duplicate pane focus, immediate close,
root switching without stale activity, command-id and dispatch parity for
visible controls, and desktop/mobile browser screenshots or DOM assertions.

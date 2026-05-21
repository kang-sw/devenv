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
- Ribbon items use a compact three-line shape: small source/kind text
  (`Agent`, `Cmd`, or later source kinds), a primary name/title line, and small
  status or recency text. The internal text area should stay around 2.5 font
  heights, truncate rather than wrap, and remain horizontally scrollable at
  constrained widths.
- A small green breathing indicator may mark newly updated or attention-worthy
  items. It is not merely the persistent running/live status indicator; it is a
  short-lived attention cue that turns off when the user selects or otherwise
  acknowledges the item.
- Selecting a ribbon item renders transcript blocks below. Selection should
  survive snapshot refreshes where the selected item still exists.
- The UI shell may keep a browser-local acknowledgement watermark per
  workRoot/activity item. On initial feed load, compare that watermark with the
  daemon-provided item update timestamp or cursor to mark items dirty and show
  the breathing attention cue. Selecting or explicitly acknowledging the item
  clears the local dirty state. This is local UI state, not daemon read-receipt
  authority.
- Transcript rendering is source-aware. Exec activity renders like a terminal
  output view. Agent activity renders normalized action-unit blocks: dialogue
  and assistant output are expanded by default, while tool calls, MCP activity,
  and command runs default to one-line summaries.
- Transcript block details expand inline when selected, exposing MCP, command,
  or other backend action detail without opening a modal or making the browser
  understand backend-private paths.
- Transcript backfill and load-more behavior should be scroll-position driven.
  Explicit refresh or load-more affordances are fallback/error controls rather
  than the primary reading flow.
- The WorkRoot Activity pane remains a reversible read-only workbench surface
  and should continue to close immediately without daemon side effects.

## Constraints

- No visible in-app tutorial text explaining the feature. Use normal labels,
  statuses, and affordances.
- Text must fit inside ribbon buttons and transcript blocks across desktop and
  constrained-width views; the ribbon scrolls horizontally rather than
  wrapping.
- Mobile layout is not a target for this ticket. Narrow desktop or constrained
  widths should keep the ribbon horizontally scrollable and preserve the
  transcript viewer below it.
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
overflow, the three-line ribbon item shape, breathing attention indicator
acknowledgement, browser-local dirty-state initialization from update
timestamps or cursors, active/live styling, long text wrapping,
tool/status/output blocks, exec terminal-output rendering, agent action-unit
blocks, inline detail expansion, scroll-position transcript loading,
empty/degraded/running/completed states, duplicate pane focus, immediate close,
root switching without stale activity, command-id and dispatch parity for
visible controls, and desktop screenshots or DOM assertions.

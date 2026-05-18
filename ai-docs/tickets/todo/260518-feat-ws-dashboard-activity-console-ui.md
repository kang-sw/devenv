---
title: ws dashboard Activity Console UI
parent: 260518-epic-ws-dashboard-activity-console
related:
  260518-feat-ws-dashboard-activity-feed-api: supplies Activity Feed items for the ribbon
  260518-feat-ws-dashboard-activity-transcript-api: supplies selected transcript blocks
  260518-feat-ws-dashboard-activity-watch-stream: supplies live update behavior for feed and transcript state
  260517-feat-ws-dashboard-workroot-activity: existing pane to migrate from list dump to console
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard Activity Console UI

## Background

The current WorkRoot Activity pane renders every named agent as a full-width
card in one vertical list. This makes a populated workRoot hard to scan and
does not provide a useful transcript reading surface. The desired UI is a
reusable Activity Console: a roughly 60px horizontal Activity Ribbon showing
live/latest items and a selected Transcript Block viewer below it.

The same component family should later support popup-style transcript views
from a main agent surface, so it must not be coupled to WorkRoot Activity pane
placement or named-agent-specific metadata.

## Decisions

- Build reusable `ActivityRibbon`, `TranscriptBlockViewer`, and
  `ActivityConsole` components or equivalent local modules with dependency
  injection for item rendering and transcript loading.
- The ribbon defaults to live/active/attention items first, then latest updated
  activity. Live items use a green outline or equivalent semantic active
  treatment without turning the whole UI into a single-hue theme.
- Selecting a ribbon item renders transcript blocks below. Selection should
  survive feed updates where the selected item still exists.
- The WorkRoot Activity pane remains a reversible read-only workbench surface
  and should continue to close immediately without daemon side effects.

## Constraints

- No visible in-app tutorial text explaining the feature. Use normal labels,
  statuses, and affordances.
- Text must fit inside ribbon buttons and transcript blocks across desktop and
  mobile viewports; the ribbon scrolls horizontally rather than wrapping.
- UI work must include browser-level visual/interaction verification against
  the daemon-served frontend, not only TypeScript tests.
- Do not add agent control buttons. Future terminate behavior, if accepted,
  belongs to a separate ticket.

## Phases

### Phase 1: Add reusable Activity Ribbon

Implement the horizontal ribbon component with stable dimensions, overflow
scrolling, selected state, live state, latest ordering, and injected item
rendering metadata. It should be usable with deterministic fixtures before live
feed streaming is complete.

Verification should cover ordering, selection preservation, horizontal overflow,
active/live styling, and narrow viewport behavior.

### Phase 2: Add Transcript Block viewer

Implement normalized transcript block rendering for assistant/user/tool/status
and output-style blocks. The viewer should support bounded loading, empty,
degraded, running, and completed states without assuming named-agent-only
content.

Verification should cover long text wrapping, tool call/result blocks, running
append state, degraded blocks, and viewport-bounded scrolling.

### Phase 3: Compose Activity Console in the WorkRoot Activity pane

Replace the current single-list WorkRoot Activity pane body with the reusable
Activity Console using Activity Feed and selected transcript data. The pane
should default-select the first live/attention item, or the latest item when no
live item exists.

Verification should cover a populated feed, duplicate pane focus, immediate
close, selected item transcript rendering, root switching without stale
activity, and browser screenshots/DOM assertions at desktop and mobile sizes.

### Phase 4: Prove popup-ready reuse without adding controls

Add a fixture or narrow UI harness that proves the Activity Console components
can render as a popup-style transcript viewer with injected feed/transcript
data. This phase should not add a main-agent product surface unless a later
ticket owns that integration.

Verification should prove the component contract without introducing
agent-control affordances or WorkRoot-specific assumptions.

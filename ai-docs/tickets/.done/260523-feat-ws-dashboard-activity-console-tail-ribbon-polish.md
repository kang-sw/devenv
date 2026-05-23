---
title: Polish Activity Console tail loading and ribbon metadata
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-bug-ws-dashboard-activity-console-dogfood-usability: earlier dogfood repair that left pagination direction and ribbon density follow-ups
  260523-bug-ws-dashboard-dockview-split-scroll-reset: separate split-wide scroll reset bug
spec:
  - 260521-ws-dashboard-activity-console-read-model
  - 260521-ws-dashboard-activity-console-ui-shell
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-23
---

# Polish Activity Console tail loading and ribbon metadata

## Background

Dogfood feedback after the Activity Console usability repair found that the
surface still feels awkward for real transcript inspection. Selecting a ribbon
item scrolls the transcript view to the bottom, but the backing endpoint still
loads from the beginning and the automatic load-more path only fires after a
manual scroll event. That makes the UI look tail-oriented while the data model
is head-first.

The ribbon also spends vertical space on a separate summary chip row and repeats
similar text on the first and second item lines. Item cards should convey source
channel, name, current state, recency, and elapsed duration without increasing
their vertical footprint.

## Decisions

- Initial transcript reads should return the latest bounded tail window.
- Older transcript history should load from the top edge and prepend blocks
  while preserving the visible scroll position.
- The Activity Console summary chip row should be removed from the pane body.
- Ribbon first-line text should be a source discriminator such as `agent.codex`
  or `cmd.exec`, not another copy of the display label.
- Ribbon third-line text should combine status, relative update time, and
  completed-duration information where space permits.

## Phases

### Phase 1: Tail-first transcript pagination

Change the selected transcript route and frontend loading policy so initial
loads return the latest bounded tail window. The browser should follow the tail
for newly selected/live activity, load older blocks when the user scrolls near
the top, prepend those older blocks without shifting the user's visible content,
and keep explicit load-more as a fallback command rather than the normal path.

Verification should cover backend pagination bounds, frontend scroll-trigger
policy, and browser evidence that selecting a long transcript starts at the
latest blocks while upward scrolling loads older history.

### Result (36c64f8) - 2026-05-23

Implemented tail-first selected transcript loading and older-history prepend.
The transcript route now returns the latest bounded tail window by default,
keeps legacy forward `cursor` pagination available, and accepts `before` for
older windows. The frontend requests older history from the current earliest
cursor, triggers older loading at the top edge, prepends returned blocks, and
preserves the visible scroll position after prepend. The explicit fallback
button moved to the top of the scroll region and now says "Load earlier
transcript".

Verification covered backend tail/default and `before` pagination route tests,
frontend endpoint/scroll-trigger tests, production frontend build, and browser
acceptance for the WorkRoot Activity pane.

### Phase 2: Ribbon density and timing polish

Remove the Activity Console summary chip row. Update ribbon cards so the first
line renders a source discriminator, the second line remains the primary title,
and the third line combines status with relative update time and completed
duration when known. Keep the three-line card footprint and horizontal overflow
behavior stable.

Verification should cover source discriminator formatting, relative time and
duration formatting, summary-row removal, and narrow-width ribbon truncation.

### Result (36c64f8) - 2026-05-23

Implemented with Phase 1 in one pass. The Activity Console no longer renders
the redundant summary chip row above the ribbon. Ribbon item first lines now
show source discriminators such as `agent.codex`, `agent.claude`, and
`cmd.exec`; the title remains on the second line; and the status line combines
status, relative update time, and completed duration when timestamps are
available. Frontend helper tests cover source labels and timing formatting, and
browser acceptance verifies the summary row is absent and source discriminator
text appears for agent and exec items.

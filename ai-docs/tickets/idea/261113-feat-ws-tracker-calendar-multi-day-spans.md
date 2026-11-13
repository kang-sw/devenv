---
title: "Tracker calendar renders items across start..due as multi-day spans"
related:
  261113-feat-ws-tracker-start-date-range-picker: "prerequisite (adds the start field); owner answer (480)"
---

# Tracker calendar renders items across start..due

## Background

Owner, 2026-11-13, answer (480)=a on
`261113-feat-ws-tracker-start-date-range-picker`: that ticket adds an
optional item `start` and makes gantt bars span start..due, but the calendar
view stays due-day only (one cell per item on its due day; `calendarBody` in
`agents-plugin-tool/internal/wsdashboard/assets/tracker.js`). The owner asked
to record, not design now, the follow-up: the calendar month grid renders an
item with a start as a multi-day span from start to due.

## Open

- Span layout in the month grid (lanes, week wrap, overflow with many
  spans) needs its own design and mockup.
- Depends on the `start` field landing with the range-picker ticket.

---
title: "Tracker items get an optional start date, picked with a date-range picker"
---

# Tracker items get an optional start date, picked with a date-range picker

## Background

Owner feedback 2026-11-13 on the dashboard tracker: the due-date picker is a
small popover with a native `YYYY-MM-DD` input and a set button (item menu
`'due'` step, `agents-plugin-tool/internal/wsdashboard/assets/tracker.js`
~3861; new-item composer due chip ~4409 and step ~4467). The owner wants a
calendar-based picker, and on seeing mockups asked for a date *range*: an item
has a start and a due date, both picked on one calendar.

Tracker items have no user-set start date today. The only date field is
`due` (`internal/wstracker/payload.go:75`, validated by `validDue` at
`payload.go:438` and `check.go:168`, applied at `state.go:1281` / `1323`,
viewed at `view.go:120`, filtered at `query.go:192` / `310`, MCP arguments at
`internal/mcp/tracker_tools.go:141-142`, `293`, `326-349`, hook event
`internal/wstrackerhook/event.go:68`). `ItemView.Started`
(`view.go:134`, computed at `view.go:746-793`) is a read-only value derived
from the first `doing` transition; tracker.js uses it only as the gantt bar
start (`tracker.js` 2364-2397, `it.started || it.ts`).

Owner decision (whiteboard (13), option a): add an optional start-date field
and build the range picker; captured as this ticket.

The picker the owner described (mockups rendered 2026-11-13, not tracked):

- One highlighted span on the month grid from start to due. Its left end is a
  left half-circle handle (start), its right end a right half-circle handle
  (due); dragging either handle along the grid moves that end.
- When the range must cross into another month, the picker extends to two
  panels: the left panel shows the start's month and the right panel the
  due's month, each paged on its own, so they need not be adjacent (owner:
  "left September, right December" is possible).
- Fallback under the calendar: two date input rows, start and due, stacked
  when the picker is narrow and side by side (start left, due right) when it
  shows two panels.
- A due-only item shows only the right half-circle on its due day plus a
  dashed ghost left half-circle; dragging the ghost out creates a start.

The row's pencil edit (in flight 2026-11-13, plan
`ai-docs/.plans/2026-11/13-1025-tracker-row-edit-uses-composer.md`) reuses the
composer's chips; it keeps today's due step until this ticket lands.

## Decisions

- Compatibility is T0 provided every reader of a tracker event tolerates an
  unknown optional payload field; the item event payload gains `start`. If an
  older ws version's replay or `check.go` validation rejects unknown fields,
  the change is a tracker event change (T2) and returns to the owner before
  implementation; fact population verifies this at promotion. Rejected:
  assuming T0 unverified.
- Field name `start` (`YYYY-MM-DD`, same format and validation as `due`); the
  derived read-only view field `started` keeps its name and meaning, and the
  item view gains `start` beside it. Rejected: `start_date`, `begin` (do not
  pair with `due`).
- The server refuses an item whose start is after its due, on create and on
  any edit that sets either side; clearing `due` leaves `start` as is.
  Rejected: enforcing it only in the picker (MCP writers bypass the picker).
- Gantt and calendar bars begin at `start`, falling back to the derived
  `started`, then the creation time. Rejected: replacing `started` (existing
  items without a start would lose their bars' start).
- One range-picker component serves the item menu's due step, the new-item
  composer's due chip, and the pencil edit row. Rejected: a picker per entry
  point (behaviour drifts; ad8bde38e applied the same rule to the label
  checklist).
- Picks apply in one write when the picker closes, no write when nothing
  changed. Rejected: a write per drag (writes on every day crossed).
- The picker has at most two month panels: one panel normally; when
  extended, a left panel for the start side and a right panel for the due
  side, each paged independently with its own month arrows, so they need not
  be adjacent (e.g. September | December). The span is highlighted from the
  start to the left panel's month end and from the right panel's month start
  to the due; dragging a handle past its panel's edge pages that panel.
  Dragging uses pointer events (mouse and touch); the two input rows are the
  keyboard path. Rejected: adjacent consecutive months, unbounded extension
  (owner (21): the extension exists to pick the start and the due).
- The hook event gains an optional `Start` beside `Due`; a `start` query
  filter is out of scope. Rejected: adding the filter now (no requested view
  filters by start).
- Standalone `todo/` feat with no parent: no tracker epic exists, and the
  pencil edit row is a plan in flight, not a ticket.

## Constraints

- Manuals: `ai-docs/manuals/release-compatibility.md` (the tier is settled
  before release), `ai-docs/manuals/shipped-surface-boundary.md` (MCP tool
  argument text), `ai-docs/manuals/ws-mcp.md`.
- Every new user-facing string goes into all `internal/wsi18n/tables/*.json`.
- Dashboard theme: dark; the picker fits a ~280 px pane in its one-month
  state.

## Phases

### Phase 1: Optional start date and the range picker

- Store and API: optional `start` on item create and edit, `clear` support,
  validation per the decisions above, the item view field, the hook event
  field.
- Dashboard: the range picker described in Background replaces the due
  picker at every entry point; the row due badge shows the range when a start
  is set (exact format is the worker's call, matching the existing D-N / D-day
  / D+N badge); gantt and calendar use the start date.
- Tests: `wstracker` store and validation tests (including start after due
  refused, clear, replay), MCP argument tests, tracker asset tests for the
  picker's one-write-on-close behaviour.
- Verification: `go test` for the touched packages on the remote build host;
  a headless render of the picker's three states (one month with a range, two
  panels (non-adjacent months) with a range across them, due-only with the ghost handle); state
  what was not checked interactively.

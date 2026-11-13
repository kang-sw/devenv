---
title: "Tracker items get an optional start date, picked with a date-range picker"
---

# Tracker items get an optional start date, picked with a date-range picker

## Background

Owner feedback 2026-11-13 on the dashboard tracker: the due-date picker is a
small popover with a native `YYYY-MM-DD` input and a set button (item menu
`'due'` step, `agents-plugin-tool/internal/wsdashboard/assets/tracker.js`
3939, menu entry 4008; new-item composer due chip 4492 and step 4553). The owner wants a
calendar-based picker, and on seeing mockups asked for a date *range*: an item
has a start and a due date, both picked on one calendar.

Tracker items have no user-set start date today. The only date field is
`due` (`internal/wstracker/payload.go:75`, validated by `validDue` at
`payload.go:438` and `check.go:168`, applied at `state.go:1281` / `1323`,
viewed at `view.go:120`, filtered at `query.go:192` / `310`, MCP arguments at
`internal/mcp/tracker_tools.go:141-142`, `293`, `326-349`, hook event
`internal/wstrackerhook/event.go:68`). `ItemView.Started`
(`view.go:134`, computed at `view.go:744-793`) is a read-only value derived
from the first `doing` transition; tracker.js uses it only as the gantt bar
start (`tracker.js` 2449-2454 and 2473, `it.started || it.ts`; the calendar view places an item on its due day only, `calendarBody` at `tracker.js:2485-2490`, with no bar).

Owner decision (whiteboard (13), option a): add an optional start-date field
and build the range picker; captured as this ticket.

The picker the owner described and approved ((497)=ok, owner 2026-11-13).
Approved visual reference: `ai-docs/tickets/assets/261113-feat-ws-tracker-start-date-range-picker/range-picker-mockup.html`
(MAIN board panel p4, latest version e104, "목업 v1 (스크립트 오류 수정본):
한 달 · 두 패널 · due만 있는 경우"; the HTML fragment is wrapped in a minimal
standalone shell):

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

The row's pencil edit (landed 2026-11-13 in `8acecdba1`, `startRowEdit` in
`tracker.js`) reuses the composer's chips (`nrowChips`, `newMenu`); it keeps
today's due step until this ticket lands.

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
- Gantt bars begin at `start`, falling back to the derived `started`, then
  the creation time. Rejected: replacing `started` (existing items without a
  start would lose their bars' start).
- (480)=a (owner, 2026-11-13): the calendar view stays due-day only (one
  cell per item on its due day). Rejected: multi-day start..due spans in the
  month grid now; recorded for later as
  `261113-feat-ws-tracker-calendar-multi-day-spans` (idea).
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
- Compatibility (475, announced default, verified at fact population
  2026-11-13): T0. Older readers decode payloads leniently
  (`wstracker/payload.go` `parsePayload`), `checkFields` never enumerates
  keys, and the fold reads named keys only, so `start` is ignored on create
  and on edits that also set a known field. Caveat: an older reader flags a
  start-only `item.edit` as "nothing to edit" (`check.go`, the `itemFields`
  check) and does not apply it; the flag is non-fatal and matches how
  `labels` shipped (`bd3517fc9`, v0.49.0). Rejected: bundling an unchanged
  `due` into start-only edits to dodge the flag (it would overwrite a
  concurrent `due` write).
- Reasons parity is in scope (announced default, fact population): every
  new flagged reason (invalid `start`, start after due, the edit-key list
  message gaining `start`) is added to `wstracker/testdata/reasons.json` and
  to the reference engine `ai-docs/ref/tracker-mockup/engine.js`, because
  `TestFlaggedReasons` pins each reason against both.
- Standalone `todo/` feat with no parent: no tracker epic fits (the only tracker
  epic, `261110-epic-ws-tracker-team-feedback`, scopes the first team-introduction
  feedback, not owner picker feedback), and the pencil edit row is a plan in
  flight, not a ticket.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Manuals: `ai-docs/manuals/release-compatibility.md` (the tier is settled
  before release), `ai-docs/manuals/shipped-surface-boundary.md` (MCP tool
  argument text), `ai-docs/manuals/ws-mcp.md`.
- Every new user-facing string goes into all `internal/wsi18n/tables/*.json`.
- Dashboard theme: dark; the picker fits a ~280 px pane in its one-month
  state.

## Prior Decisions

- 3634ebdf (2026-11-13, commit): "Compatibility recorded as T0 conditional on readers tolerating an unknown optional event field; fact population verifies at promotion." — bearing: supports
- 8acecdba (2026-11-13, commit): "Rejected: leaving the due step as a date input only is unchanged (range picker is out of scope)." — bearing: supports
- 13cb3f0a (2026-11-13, commit): "Removed the LW per-item in-flight write queue (labelWrite/labelFlush): a single write on close makes it unnecessary." — bearing: supports
- 1b1d0a9d (2026-11-13, commit): "One save = one commit batch; item.edit carries only changed fields, status/assign events reuse the item menu's events and are sent only when changed and permitted." — bearing: constrains
- 261110-feat-ws-dashboard-tracker-quick-add-fields (2026-11-10, Decisions): "A due date set at quick-add travels in item.create's existing due field (ItemCreate.Due, internal/wstracker/payload.go), not as a separate item.edit." — bearing: constrains
- 261110-feat-ws-tracker-daemon-event-hook (2026-11-10, Decisions): "Event JSON fields (user, (281)). v; id; kind; ts; store; item {id, alias, title, kind}; actor; target (optional); detail {status, comment, due...}" — bearing: constrains
- 261108-feat-ws-tracker-caller (2026-11-08, Result): "tk:item.edit clears with clear=\"body due\". A due cleared this way is written as null." — bearing: constrains
- 4449ed91 (2026-11-08, commit): "Every flagged reason has a reasons.json case checked against the engine by the generator and against Go by TestFlaggedReasons, so the strings cannot drift between the two." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wstracker (payload.go, check.go, state.go, view.go, query.go), mcp/tracker_tools.go, wstrackerhook/event.go, wsdashboard/assets/tracker.js and tracker.css, wsi18n/tables/*.json, wstracker/testdata/reasons.json and ai-docs/ref/tracker-mockup/engine.js (parity) |
| scope.surface | cross-module | new item.create and item.edit payload field start, MCP tracker_update start attribute and clear value, ItemView.start, hook Detail |
| scope.new_public_symbol | yes | ItemCreate.Start, ItemView.Start, wstrackerhook Detail.Start, start argument on tk item.create and item.edit |
| scope.new_type_contract | yes | ItemCreate.Start string, ItemView.Start *string, start-after-due refusal reason, clear accepts start |
| scope.test_surface | existing | wstracker/tracker_test.go, wstracker/testdata/reasons.json, mcp/tracker_tools_test.go, wsdashboard/tracker_quickadd_test.go, wsdashboard/tracker_draft.test.js |
| complexity.reuse_points | confirmed | due field path (validDue, checkFields, itemFields fold, buildItemCreate and buildItemEdit, clear handling) read for reuse |
| complexity.side_effect_risk | moderate | item.edit with only start is flagged nothing to edit by older readers (check.go:738-743); picker replaces three due entry points |
| risk.correctness | moderate | cross-field start-after-due rule on edits that set one side, with concurrent due writes and clear semantics |
| risk.fit | moderate | two-panel pointer-drag range picker is new UI shared by menu, composer and pencil edit row (plan in flight) |
| risk.test | moderate | drag and paging behaviour has no existing asset-test harness beyond tracker_draft.test.js; headless render only |
| risk.security_or_contract | moderate | tracker event field and MCP argument change; tier T0 holds for readers but not for a start-only edit on older readers, and the golden reasons parity must be extended |

## Phases

### Phase 1: Optional start date and the range picker

- Store and API: optional `start` on item create and edit, `clear` support,
  validation per the decisions above, the item view field, the hook event
  field.
- Dashboard: the range picker described in Background replaces the due
  picker at every entry point; the row due badge shows the range when a start
  is set (exact format is the worker's call, matching the existing D-N / D-day
  / D+N badge); gantt uses the start date; the calendar stays due-day only.
- Tests: `wstracker` store and validation tests (including start after due
  refused, clear, replay), reasons parity (`reasons.json`, reference
  `engine.js`, `TestFlaggedReasons`), MCP argument tests, tracker asset tests
  for the picker's one-write-on-close behaviour.
- Verification: `go test` for the touched packages on the remote build host;
  a headless render of the picker's three states (one month with a range, two
  panels (non-adjacent months) with a range across them, due-only with the ghost handle), each
  compared side by side against the matching state of the approved reference
  `ai-docs/tickets/assets/261113-feat-ws-tracker-start-date-range-picker/range-picker-mockup.html`
  (list both paths and the result per state); state what was not checked
  interactively.

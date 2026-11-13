---
title: "Tracker items get an optional start date, picked with a date-range picker"
sage-review-design: completed
sage-review-completeness: completed
related:
  261113-feat-ws-tracker-calendar-multi-day-spans: "calendar multi-day spans split out by (480)=a; this ticket keeps the calendar due-day only"
sage-review-design-reviewed: 66b4af1df77dccd5
sage-review-completeness-reviewed: 66b4af1df77dccd5
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
start (`tracker.js` 2449-2454 and 2473, `it.started || it.ts`; the calendar view places an item on its due day only, `calendarBody` at `tracker.js:2485-2494`, with no bar). Both views draw only due-dated rows (`dueRows`, `tracker.js:2430`).

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
- Fallback under the calendar: two date input rows, start and due, each with
  a trailing × clear button, stacked when the picker is narrow and side by
  side (start left, due right) when it shows two panels.
- A due-only item shows only the right half-circle on its due day plus a
  dashed ghost left half-circle; dragging the ghost out creates a start.

The mockup's weeks run Sunday-first and its two-panel state carries a
"skipped months" note between the panels; owner (529)=c (below) keeps the
Sunday-first weeks and drops the note. The mockup's header text is hard-coded
Korean; the shipped picker takes it from i18n keys.

The row's pencil edit (landed 2026-11-13 in `8acecdba1`, `startRowEdit` at
`tracker.js:755`) reuses the composer's chips (`nrowChips`, `newMenu`); it keeps
today's due step until this ticket lands.

## Decisions

Source tags: owner = an owner answer relayed by the MAIN lead; UX review and
design review = the lead's reviews of this ticket on 2026-11-13; autonomous =
a worker or reviewer call, open to the lead.

1. [owner whiteboard (13)=a] Add an optional start-date field and build the
   range picker; this ticket captures it. The picker the owner approved is
   (497)=ok, the mockup in Background.
2. [announced default (475), verified at fact population 2026-11-13]
   Compatibility T0. The item event payload gains an optional `start`; older
   readers decode payloads leniently (`wstracker/payload.go` `parsePayload`),
   `checkFields` never enumerates keys, and the fold reads named keys only, so
   `start` is ignored on create and on edits that also set a known field.
   The start <= due rule (decision 4) is write-time only, so no reader ever
   flags or reverts an event over it: old and new readers fold the same store
   to the same `due`. Caveat: an older reader flags a start-only `item.edit`
   as "nothing to edit" (`check.go:738-742`, the `itemFields` check) and does
   not apply it; the flag is non-fatal and matches how `labels` shipped
   (`bd3517fc9`, v0.49.0). Rejected: bundling an unchanged `due` into
   start-only edits to dodge the flag (it would overwrite a concurrent `due`
   write); assuming T0 unverified.
3. [ticket creation] Field name `start` (`YYYY-MM-DD`, same format and
   validation as `due`); the derived read-only view field `started` keeps its
   name and meaning, and the item view gains `start` beside it. Rejected:
   `start_date`, `begin` (do not pair with `due`).
4. [ticket creation; placement by UX review 2026-11-13] The store refuses an
   item whose start is after its due, on `item.create` and on any `item.edit`
   that sets either side (the side the edit does not set is read from the
   folded item). Clearing either side never refuses. The rule lives in
   `checkWrite` (`wstracker/check.go:600`, "rules a replay never applies",
   run by Dispatch at `tracker.go:177`) and in the reference engine's
   `writeRefusal` (`ai-docs/ref/tracker-mockup/engine.js:567`), never in the
   fold-time `check()`. Rejected: a `check()` rule (an older reader would apply
   a due edit that a newer reader flags, so one store would show two `due`
   values); enforcing it only in the picker (MCP writers bypass the picker).
   Concurrent register writes can still fold to start > due (one replica's
   `due` edit against another's `start` edit), so every reader tolerates the
   pair: the view carries both values unflagged, the gantt clamps (its
   `Math.min(..., e - 1)` at `tracker.js:2453` already yields a one-day bar at
   the due), and the picker opens on it showing the inline error of decision
   13 until one end is fixed.
5. [design review 2026-11-13, autonomous] `item.create` accepts `start`
   without `due`; the refusal of decision 4 fires only when both are present.
   Start-only items exist (clearing `due` leaves `start`). Gantt and calendar
   draw only due-dated rows (`dueRows` `dated()`, `tracker.js:2430`), so a
   start-only item has no bar and no calendar cell; the row badge
   (`dueHtml`, `tracker.js:713`, which returns an empty slot without a due)
   shows `from MM-DD` in the neutral badge class: text key `js.tk.due.from`
   ("from {0}", {0} the date from the fifth character), tooltip key
   `js.tk.due.tip.from` ("start {0}"), both new in all 11 tables; an item
   with both ends keeps today's due badge forms, the range display being the
   worker's call. In the picker a start-only item shows
   the left handle on its start day and a dashed ghost right handle that
   creates a due when dragged out, mirroring the due-only case. Rejected: a
   bar for a start-only item (no honest right end: extending to today reads as
   a due, a one-day marker reads as a due-less milestone nobody asked for).
6. [ticket creation] Gantt bars begin at `start`, falling back to the derived
   `started`, then the creation time. Rejected: replacing `started` (existing
   items without a start would lose their bars' start).
7. [owner (480)=a, 2026-11-13] The calendar view stays due-day only (one cell
   per item on its due day). Rejected: multi-day start..due spans in the month
   grid now; recorded for later as
   `261113-feat-ws-tracker-calendar-multi-day-spans` (idea).
8. [owner (529)=c, 2026-11-13; owner: "the calendar view should switch to
   Sunday-first"] Weeks run Sunday-first in the range picker (as the mockup
   draws them) and in the existing tracker calendar view, so both agree. The
   calendar is Monday-first today (`calendarBody`: the `dow` offset at
   `tracker.js:2486`, the header order at `tracker.js:2494`). Header text goes
   through the existing `js.tk.cal.sun` ... `js.tk.cal.sat` keys (all seven
   exist in all 11 tables; the picker reuses them, no new weekday strings),
   not the mockup's hard-coded Korean. The mockup's × clear buttons stay;
   its "skipped months" note between the panels is dropped. The gantt axis's
   Monday tick labels (`tracker.js:2464`) are a label cadence, not a week
   grid, and are out of this change. Rejected: Monday-first (the owner chose
   Sunday-first); a Sunday-first picker beside a Monday-first calendar (two
   week grids in one dashboard).
9. [ticket creation] One range-picker component serves the item menu's due
   step, the new-item composer's due chip, and the pencil edit row. Rejected:
   a picker per entry point (behaviour drifts; ad8bde38e applied the same rule
   to the label checklist). Topics have no due chip (`nrowChips`), so they get
   no start either.
10. [owner (541)=b, 2026-11-13, with the owner's addition below; supersedes
    the UX review's Set/Enter-only rule and the completeness review's
    outside-click rule] The picker follows the label picker's single write on
    close (13cb3f0a, `labelPending` called from `closeMenu`,
    `tracker.js:3870` and `5075`): **any close applies the pending pick once**
    (outside click, focus loss out of the picker, Enter in an input row, the
    back row), and there is **no Set button**. **Esc** and the red floating
    discard capsule of decision 21 discard the pick and write nothing; Esc in
    the item menu's sub-step goes back to the main menu through `menuBack`
    (`tracker.js:4386`), with the pick dropped. The apply sends only the
    fields that differ from the item's current values (as `editRowEvents`
    does), so a changed start never carries an unchanged due (decision 2's
    no-bundling rule); a close with nothing changed writes nothing. The back
    row exists only on the range picker's due sub-step opened from the main
    menu (`from` set); ArrowLeft on a non-field control there calls `menuBack`
    too (`tracker.js:4422`), and counts as the back row: it applies, and is
    refused when the pair is inverted. A back row that applies a changed pick
    writes, and the written batch closes the item menu as every item-menu
    write does; with nothing changed it steps back to the main menu. Focus
    loss counts as an apply-close only when the new focus target is outside
    both the menu and the capsule; focus lost because a picker re-render
    removed the focused element (month arrows, ×, the collapse after a drag
    end, a panel opening) does not count. `wrote()` closes every item menu
    except the labels step after a written batch (`tracker.js:3713-3715`), so
    a late label write could close the range picker mid-pick: the range step
    joins labels in that `keep` test (and, unlike labels, is not redrawn).
    Rejected: listing a forced close on a foreign write. Stale
    closes follow the label picker as it is today: a resize or scroll close
    (`tracker.js:6250`, `6374`) applies a valid pick like any close, because
    `labelPending` does; a close because the item or the viewer's edit right
    is gone (`mnStale`, `tracker.js:3650` and `5422`) writes nothing, as
    `labelPending` returns null then. An inverted pick is refused by every
    apply-close (decision 13). Owner words: the owner prefers a red
    "[trash icon] discard" floating capsule hovering near the popup, and
    "losing focus counts as saving by default; tag saving and such UX are
    already reflected that way, aren't they?" (verbatim Korean: "저는 차라리
    빨간색 \"[휴지통아이콘] 버리기\"를 floating capsule로 팝업 근처 허공에 띄우는 게
    더 좋긴 해요. 기본적으로는 포커스 풀리는 건 저장으로 치고. 지금 태그 저장이나
    그런 UX도 그렇게 반영되고 있잖아요?"). Rejected: (a) apply only on Set or
    Enter with a Set button (the earlier decision: an outside click or focus
    loss would drop a pick, unlike a label pick); Escape applying (no way to
    back out of a drag). The mockup caption "closing saves once" now reads
    literally. The mockup has no Set button, so none is drawn.
11. [completeness review 2026-11-13, autonomous; × added by owner (529)=c]
    Emptying an input row or pressing its × clears that field in the pending
    pick; the clear applies with the pick on any apply-close, as decision 10 says.
    The item menu's separate "Clear due date" row (`m-due-clear`, which writes
    at once, `tracker.js:4330`) goes away with the ×. Clearing due leaves
    start as is (decision 4). Rejected: a × that writes at once (it would
    break the one-write rule). The × and an emptied input never close the
    picker themselves.
12. [UX review 2026-11-13] In the composer and the pencil edit row, an apply-close
    changes only the draft (`n.start`, `n.due`, as `m-due-set` does at
    `tracker.js:4584`); the write rides the row's save: `createNew` adds
    `cp.start` beside `cp.due` (`tracker.js:4647`), and `editRowEvents`
    (`tracker.js:779`) sends `start` in the same `item.edit` as the other
    changed fields only when it differs from both the editor's opening value
    (`n.orig`) and the item as it is now, as it does for `due`. A pencil row
    save stays one commit batch carrying only changed fields (1b1d0a9d). The
    row edit survives every picker close: `menuKey` takes Esc first and stops
    it (`tracker.js:4386`, routed at `6309`), so it never reaches the row's own
    Esc handler (`endRowEdit`, `6333`); a discarded pick (Esc or the capsule) leaves the draft as it was, and the
    capsule never discards the row edit. A start-only edit is the older-reader flag of decision 2. A `checkWrite`
    refusal after a concurrent `due` change (start now after the other
    viewer's due) refuses the whole pencil-row save batch, not just the
    `item.edit` (`engine.js:2351-2353`; Dispatch returns the refusal with its
    event index), and surfaces through the existing commit-failure path
    (`commit`'s `!res.ok` toast and `fail` callback, `tracker.js:3701`); the
    draft stays open for the user to fix.
13. [UX review 2026-11-13] Edge handling. Dragging start past due clamps at
    due (a one-day range), and dragging due before start clamps at start;
    the ends never swap or push each other. A typed start after due (or due
    before start) shows an inline error on the input rows and writes nothing.
    Every apply-close of an inverted pick (Enter, outside click, focus loss,
    the back row) is refused: the picker stays open, focus goes to the
    offending input, and the inline error shows; nothing is written and the
    pick is not dropped. The refusal swallows the triggering event: the document
    click handler (`tracker.js:5989-5993`) calls `preventDefault` and returns
    before the click's own action, and a focus-loss refusal puts focus back on
    the offending input. Otherwise a click on another item's chip would run
    `openMenu`, which replaces `MN` without `closeMenu` (`tracker.js:3842`),
    and silently drop the kept pick. Only Esc and the capsule leave it, discarding the
    pick. A resize, scroll or stale close cannot keep the picker open and
    discards an inverted pick without a write. Rejected: an outside click on an
    invalid pick discarding it (a mis-click would lose a typed range); writing
    it (the store refuses it).
    The same error shows when the picker opens on a folded start > due
    (decision 4), where closing without a fix is Esc or the capsule; the text is the new key `js.tk.range.err` ("Start is after
    the due date"), set on the inputs with `aria-invalid` and `aria-describedby`.
    The handle's visual width stays the mockup's 10 px, with a hit area of at
    least 24 px and `touch-action: none` on the handles so a touch drag is not
    taken for a scroll. Rejected: swapping the ends mid-drag (a handle would
    change role under the finger); pushing the other end along (silently
    rewrites a field the user did not touch).
14. [UX review 2026-11-13] Accessibility. The picker's menu root takes
    `role="dialog"` as the assign step does (`drawMenu`, `tracker.js:3893`).
    Day cells stay out of the Tab ring: `menuKey`'s Tab loop gathers every
    enabled `button`, `input`, `select` in the menu regardless of
    `tabindex` (`tracker.js:4387`), so day cells are not `<button>` elements.
    The keyboard path is the two native `<input type="date">` rows plus the
    month arrows, × buttons, and the discard capsule. Enter in an input row
    closes the picker and applies (decision 10); Esc discards; the capsule is
    reachable by Tab, after the ×. Dragging uses pointer events (mouse and
    touch) with pointer capture.
15. [owner (21) and (497)=ok; panel rules by UX review 2026-11-13] At most
    two month panels. A left panel shows the start side and a right panel the
    due side, each paged by its own month arrows, so they need not be
    adjacent (September | December). The span is highlighted from the start
    to the left panel's month end and from the right panel's month start to
    the due. The second panel **opens** when the start and the due fall in
    different months (on open, after a typed date, a ×, or a drag end), and
    in the one-panel state when a handle is dragged past the panel's edge (the neighbouring month of
    that end opens in the second panel and the drag continues there). In the two-panel
    state a handle dragged past its own panel's edge pages that panel only;
    the other panel stays put (mockup state B), and the clamp of decision 13
    still bounds the handle. It **collapses**
    when the start and the due fall in the same month, or when only one end
    is set, and only on a drag end or an input change, never mid-drag.
    Rejected: adjacent consecutive months; unbounded extension (the extension
    exists to pick the start and the due).
16. [UX review 2026-11-13] Width. `.tk.tk-menu` caps at
    `min(340px, calc(100vw - 12px))` (`tracker.css:827`) and two month panels
    need about 560 px. The two-panel state takes a wide menu variant beside
    the existing `.wide` and `.spopw` (`tracker.css:830-831`, chosen in
    `drawMenu`, `tracker.js:3892`), about `min(580px, calc(100vw - 12px))`
    with `max-width: none`; the one-month state keeps the default width
    (~280 px). At a viewport of 640 px or less (the file's existing
    breakpoint, `tracker.css:223`) the two panels stack vertically and the
    input rows stack. Rejected: the `@container iw (max-width: 759px)` trigger
    from the UX review: the menu is a body-level sibling of the item window
    (`#tkwin` at `tracker.js:5923`, `#tkmenu` at `5929`), not inside `.iwin`, so
    that container query never reaches it; and a JS-measured trigger (EXEC
    lead, technical: the menu is positioned against the viewport, so the
    viewport width is what bounds it).
17. [ticket creation] The hook event gains an optional `Start` beside `Due`
    (`wstrackerhook/event.go:68`); a `start` query filter is out of scope.
    Rejected: adding the filter now (no requested view filters by start).
18. [fact population 2026-11-13; corrected by UX review] Reasons parity is in
    scope. The new fold-time reasons (an invalid `start`, and the "nothing to
    edit (...)" list gaining `start`: `start` goes before the retired `stage` in `itemFields` and
    the `editFields` slice rises from `[:6]` to `[:7]` at `state.go:23-24`;
    the same in `ITEM_FIELDS` and `EDIT_FIELDS = ITEM_FIELDS.slice(0, 7)` at
    `engine.js:43`, so the list reads title, body, project, due, start,
    priority, labels) get flagged
    cases in `wstracker/testdata/reasons.json` (the "edit nothing" case at
    line 61 changes to the new list). The start-after-due refusal is a write-time refusal, so
    it gets `"write": true` cases like the assign-below cases at
    `reasons.json:316-318`; `TestFlaggedReasons` checks the Go refusal and the
    replay's `applied` status, and `gen-golden.js` checks the engine's words.
19. [UX review 2026-11-13] Touch points beyond the store: `EDIT_FIELDS` and
    `FIELD_KEY` (`tracker.js:500` and `504`) gain `start` and
    `js.tk.field.start` (the event sentence at `tracker.js:524` reads them);
    the MCP `tracker_update` item.create and item.edit specs gain `start`
    (`tracker_tools.go:141-142`), `buildItemCreate` reads it (`:293`),
    `buildItemEdit` reads it and `clear` accepts it (`:326-351`: the
    `clear="start"` with a `start` attribute conflict message, and the
    "must list body or due" message at `:351` names start). Every new
    user-facing string (picker labels, the inline error, the start field name,
    a start tooltip) goes into all 11 `wsi18n/tables/*.json`.
20. [ticket creation; corrected by design review 2026-11-13] Standalone
    `todo/` feat with no parent: no tracker epic fits (the only tracker epic,
    `261110-epic-ws-tracker-team-feedback`, scopes the first
    team-introduction feedback, not owner picker feedback). The pencil edit row
    is landed code (`8acecdba1`), not a plan in flight; this ticket's picker
    replaces its due step.

21. [owner (541)=b, 2026-11-13; EXEC technical call] The discard capsule is a
    shared tk-menu helper used by the range picker and by the existing label
    picker, so both behave the same (the owner assumed they already did).
    It is a small red (danger style, `--tk-fail`) capsule with a trash icon
    and the new i18n key `js.tk.menu.discard` (en "Discard"; new in all 11
    tables), floating just below the menu outside its box, above it when
    there is no room below, kept inside the viewport and repositioned with
    `placeMenu` (`tracker.js:3902`). The menu clips its children
    (`overflow: auto`, `tracker.css:827`), so the capsule is a separate
    fixed element beside `#tkmenu`, and every handler that treats the menu as
    "inside" also treats the capsule so: the document click handler's
    `closest('#tkmenu')` test (`tracker.js:5989`); the document keydown router,
    which sends keys to `menuKey` only when `$('tkmenu').contains(t)`
    (`tracker.js:6309`), so capsule keys are routed to `menuKey` too (else Esc
    on the capsule falls to the global Esc tail, `closeItemWindow` /
    `mselClear` / `collapse` at `tracker.js:6368-6373`, and Tab leaves the
    `menuKey` loop); the Tab loop (`tracker.js:4387`), which appends the
    capsule after the menu's controls; the `#tkmenu` `focusout` handler
    (`tracker.js:6281`), whose `contains(to)` test also accepts the capsule,
    plus a `focusout` on the capsule itself (focus leaving the capsule never
    reaches the `#tkmenu` listener); and the mousedown guard that keeps focus.
    The capsule carries the `tk` class like `#tkmenu` and handles its own
    click, since the `[data-act]` dispatch (`tracker.js:6022`) needs `own(t)`
    (`tracker.js:5940`, a `.tk` ancestor). Without all of that, clicking or
    tabbing to the capsule would count as an outside click or focus loss and
    apply the pick it is meant to discard. The `focusout` rule widens from the
    labels step to the range picker. Discard (the capsule or Esc) writes nothing, and where it lands is the
    same for both pickers: the range picker's due sub-step opened from the
    main menu (`from` set) steps back to the main menu through `menuBack`, and
    focus lands where `drawMenu` puts it (the first control); every picker with
    no parent step (the due step opened from a row badge, the item labels step,
    which always opens with `from: null` from the row icon `tracker.js:728` or
    the property card `2757`, and the composer and pencil row chips) closes
    the menu through `closeMenu(true)`, and focus returns to the opener (the
    chip, the badge or icon). Label picker behaviour change, to match: Esc
    discards its pending pick (today `closeMenu` applies it). The labels step
    has no back row to change (the `itemMenu` sub-step path needs an anchor
    inside the menu, `menuHtml` draws no back row for it, and neither does the
    composer's labels step, `tracker.js:4558`); `menuBack` with `from` null
    already calls `closeMenu(true)`. In the new-item
    composer and pencil row the label toggles edit the draft live
    (`newAct` `m-ltog`), so the picker snapshots `n.labels` when it opens and
    discard restores that snapshot. When neither below nor above the menu has
    room (the stacked state at a viewport of 640 px or less), the capsule is
    pinned inside the viewport over the menu's bottom edge and stays visible.
    Rejected: a capsule inside the menu box (clipped by the menu's overflow); a range picker capsule only (the two
    pickers would differ on Esc and the capsule).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Every new user-facing string goes into all 11 `internal/wsi18n/tables/*.json`.
- Dashboard theme: dark; the picker fits a ~280 px pane in its one-month
  state and a ~580 px wide menu in its two-panel state (decision 16).
- The start <= due rule is a write-time refusal only (`checkWrite` and the
  engine's `writeRefusal`); `check()` and the fold never flag or revert on it
  (decision 4).
- A close applies the pending pick once; Esc and the capsule discard (decisions
  10, 21); there is no Set button.
- The week start is Sunday in the picker and in `calendarBody` (decision 8).

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
| scope.span | multi-file | wstracker (payload.go, check.go incl. checkWrite, state.go, view.go, query.go), mcp/tracker_tools.go, wstrackerhook/event.go, wsdashboard/assets/tracker.js and tracker.css, wsi18n/tables/*.json, wstracker/testdata/reasons.json and ai-docs/ref/tracker-mockup/engine.js (parity) |
| scope.surface | cross-module | new item.create and item.edit payload field start, MCP tracker_update start attribute and clear value, ItemView.start, hook Detail, calendar week start |
| scope.new_public_symbol | yes | ItemCreate.Start, ItemView.Start, wstrackerhook Detail.Start, start argument on tk item.create and item.edit |
| scope.new_type_contract | yes | ItemCreate.Start string, ItemView.Start *string, start-after-due refusal reason, clear accepts start |
| scope.test_surface | existing | wstracker/tracker_test.go, wstracker/testdata/reasons.json, mcp/tracker_tools_test.go, wsdashboard/tracker_quickadd_test.go, wsdashboard/tracker_draft.test.js |
| complexity.reuse_points | confirmed | due field path (validDue, checkFields, itemFields fold, buildItemCreate and buildItemEdit, clear handling) read for reuse |
| complexity.side_effect_risk | moderate | item.edit with only start is flagged nothing to edit by older readers (check.go:738-743); picker replaces three due entry points |
| risk.correctness | moderate | cross-field start-after-due rule as a write-time refusal (checkWrite) on edits that set one side; concurrent writes can still fold to start > due and every reader tolerates it; clear semantics |
| risk.fit | moderate | two-panel pointer-drag range picker is new UI shared by menu, composer and pencil edit row (landed in 8acecdba1); also flips the calendar view to Sunday-first |
| risk.test | moderate | drag and paging behaviour has no existing asset-test harness; editRowEvents and startRowEdit already load into the node harness in wsdashboard/tracker_quickadd_test.go (lines 416, 530), the start delta extends it; headless render for the rest |
| risk.security_or_contract | moderate | tracker event field and MCP argument change; tier T0 holds for readers but not for a start-only edit on older readers, and the golden reasons parity must be extended |

## Phases

### Phase 1: Optional start date and the range picker

- Store and API: optional `start` on item create and edit, `clear` support,
  the start <= due write-time refusal (decision 4), the item view field, the
  hook event field, the MCP and `EDIT_FIELDS` touch points (decision 19).
- Dashboard: the range picker described in Background replaces the due
  picker at every entry point, with the close semantics of decisions 10-12, no Set button, and the shared
  discard capsule on the range picker and the label picker (decision 21),
  the edges of decisions 13-14, and the panel and width rules of decisions
  15-16; the row due badge shows the range when a start is set (exact format
  is the worker's call, matching the existing D-N / D-day / D+N badge); gantt
  uses the start date; the calendar stays due-day only; the calendar view
  switches to Sunday-first (decision 8).
- Tests: `wstracker` store and validation tests (including start after due
  refused on create and on one-sided edits, clear, replay folding an inverted
  pair without a flag), reasons parity (`reasons.json`, reference `engine.js`,
  `TestFlaggedReasons`, decision 18), MCP argument tests (`start`,
  `clear="start"`, the conflict and list messages), tracker asset tests: the
  picker's close-applies behaviour (Enter, outside click, focus loss, the back
  row, resize/scroll each apply once; Esc and the capsule each discard; an item
  gone writes nothing; an inverted pick refused and the picker kept open; in
  the item menu and the draft contexts), the capsule on the label picker (it
  discards the pending labels, Esc now discards, the composer label draft
  restores its snapshot, clicking or tabbing to the capsule does not apply),
  Esc and Tab with the capsule focused (Esc discards and the item window and
  selection stay; Tab cycles back into the menu and applies nothing), an
  inverted pick plus a click on another item's chip leaving the picker and the
  pick in place, paging with focus on a month arrow or an input applying
  nothing (a re-render losing focus is not a focus loss), a late label write
  not closing the range picker mid-pick, and ArrowLeft in the due sub-step
  applying a valid pick and refusing an inverted one, the
  `editRowEvents` start delta (changed start only, start with due in one
  `item.edit`, unchanged start sends nothing, a start another viewer wrote
  meanwhile is kept) in `wsdashboard/tracker_quickadd_test.go`'s harness, a hook
  event for an item with a start carrying `Detail.Start` (decision 17, in
  `wstrackerhook`'s tests), the gantt `own()` bar beginning at `start`, else
  `started`, else creation, and clamping to one day at the due for an
  inverted pair (decision 6), a `checkWrite` refusal after a concurrent `due`
  change refusing the whole pencil-row batch and reaching the commit-failure
  toast with the draft kept (decision 12), and
  `calendarBody` placing the first of a month on the Sunday-first column with
  the header Sun..Sat.
- Verification: `go test` for the touched packages (on the project's remote build host where the local machine context names one);
  a headless render of the picker's three mockup states (one month with a range, two
  panels (non-adjacent months) with a range across them, due-only with the ghost handle), each
  compared side by side against the matching state of the approved reference
  `ai-docs/tickets/assets/261113-feat-ws-tracker-start-date-range-picker/range-picker-mockup.html`
  (list both paths and the result per state); a headless render of the
  calendar view for one month, checked Sunday-first (header and first-day
  offset) and against the picker's grid for the same month. The mockup has no
  discard capsule, no start-only state, no narrow (stacked, 640 px or less) or
  adjacent-month state, and none with an inverted pair or an inline error:
  render those (including the capsule on the range picker and on the label
  picker, and the start-only row badge of decision 5), and check that the
  capsule is visible in the narrow stacked state, describe
  them, and say they were not compared against it. State what was not checked
  interactively.

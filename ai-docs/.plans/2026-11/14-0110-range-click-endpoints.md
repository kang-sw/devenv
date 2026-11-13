# Range picker: a plain click on a day sets an end (ticketless UX fix, owner (591)=ok)

Follow-up to 261113-feat-ws-tracker-start-date-range-picker (merged 7ee0abb10).

## Problem

The tracker's start/due range picker (`agents-plugin-tool/internal/wsdashboard/assets/tracker.js`,
`rangeOf` .. `rangeUp`, ~4460-4620) moves an end only by dragging its handle. When the
panels are paged so that an end's day is in no visible panel (e.g. left 2026-11, right
paged to 2027-05, due 2027-03-05), that end has no handle and cannot be set from the
grid. Today a plain press on a day does something only when neither end is set (it sets
the due, `rangeDown`).

## Owner's rules (board (591), confirmed verbatim `591=ok`)

1. Two panels showing different months: a plain click on a day in the **right** panel
   sets the **due** to that day; in the **left** panel it sets the **start**.
2. When the clicked panel's side is ambiguous - one panel (`r.rm` null), or both panels
   show the same month - and that month contains **neither** the start nor the due, a
   click moves whichever end is **nearer** to the clicked day.
3. Existing drag behaviour is unchanged (pressing a handle still drags it).

## Settled details (lead's technical calls; apply as written)

- A click never inverts the pair: under rule 1 the moved end is clamped at the other end,
  exactly as `rangeSet` clamps a drag (ends never swap or push each other).
- Rule 2 with only one end set: the click never inverts either. Only start set: a day on
  or after it sets the due, a day before it moves the start. Only due set: a day on or
  before it sets the start, a day after it moves the due.
- Rule 2 with both set, equal distance: the due moves.
- Rule 2 does not apply when the shown month contains an end (its handle is there to
  drag): a click there keeps today's behaviour (nothing).
- Neither end set: keep today's behaviour (a click sets the due).
- After a click-set, the panels refit (`rangeFit`) and the error state recomputes, like a
  drag end. `r.last` is the end that moved.
- The click acts where today's tap acts (in `rangeDown`, primary button only, not on a
  handle). It must not break the capsule focus rule at the top of `rangeDown`.

## Boundary

- Files: `tracker.js` (the range-picker functions and their comment block above
  `monAdd`, which must describe the new click rules) and
  `agents-plugin-tool/internal/wsdashboard/tracker_range_test.go`. Nothing else.
- Another session has an uncommitted edit in the shared checkout renaming the picker's
  `bad` class to `tk-rbad` in `rangeHtml`, `rangeDraw` and one test line (~369). Do not
  touch those lines, so the two changes merge cleanly.
- Never contact 127.0.0.1:29417, never run `scripts/dev.py`, kill nothing you did not
  start. A headless check on your own daemon is optional: build `ws-mcp` to `/tmp` and run
  `WS_CACHE_HOME=<tmp> WS_CONFIG_HOME=<tmp> HOME=<tmp> <bin> dashboard --port 0`; stop it
  by PID.

## Verify

- New cases in `tracker_range_test.go` (same harness style as the existing range checks):
  rule 1 on both sides incl. the clamp; rule 2 one-panel and same-month-two-panel, both
  ends set (nearer and tie), one end set (both directions), month containing an end (no
  change); neither end set (due); a press on a handle still starts a drag.
- `go test ./internal/wsdashboard/...` from `agents-plugin-tool`; report the ok/FAIL lines.
- Commit on branch `fix/ux-range-click-endpoints` with the plan file.

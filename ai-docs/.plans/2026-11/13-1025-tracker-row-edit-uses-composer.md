# Dashboard tracker: the row pencil opens the composer-style edit row

## Problem

Owner feedback 2026-11-13 on ad8bde38e (hover pencil renames the title in
place): clicking the pencil (`D:/Temp/tracker-row-edit/pencil.png`, tooltip
"편집") should not open a bare title input; it should switch the row into the
same blue inline editor the new-item composer uses
(`D:/Temp/tracker-row-edit/composer.png`: title input "새 항목 제목: Enter로
추가" plus chips for status (구상), assignee, priority (중간), due date
(calendar icon), labels (tag icon), and a close x), prefilled with the item's
current values.

## Outcome

- Pencil click (and the same keyboard affordance the pencil has today) turns
  that row into the composer-style edit row, reusing the new-item composer's
  markup/component and chip steps (no second copy of the chip UI); prefilled
  with the item's title, status, assignee, priority, due, labels.
- Changing chips only changes the local draft. Enter (or the composer's own
  confirm affordance) saves: one `item.edit` carrying only the fields that
  changed (no write if nothing changed; blank title keeps the old one or is
  refused like the composer does). Escape or the x cancels with no write.
  Blur/outside click: follow the composer's existing convention and say
  which.
- Follow the label-picker rule from ad8bde38e: chip picks apply only when the
  edit row is saved, not per click.
- If the composer's status step has transitions that are only valid on
  create (or the edit API needs a different event for status/assignee, e.g.
  a separate status event rather than `item.edit`), reuse whatever the item
  detail/menu already sends for those fields; one user save may then mean
  more than one event, each only when that field changed. State this in the
  report.
- Replaces the bare title input from 163997099; keep `TestTrackerRowTitleEdit`
  updated to the new behaviour.
- Assets only (`agents-plugin-tool/internal/wsdashboard/assets/`), plus i18n
  tables if a string is added (all `internal/wsi18n/tables/*.json`). T0.

## Out of scope

- The due-date range picker / start-date field (separate ticket); the edit
  row keeps today's due step.
- Server changes; restarting the daemon or sessions.

## Verification

- `go test ./internal/wsdashboard/... ./internal/wsi18n/... -count=1` on w5090
  (clone note `remote-build-w5090`).
- Headless Edge render of a row in edit mode next to the composer, same
  approach as `D:/Temp/tracker-row/` in ad8bde38e's check; state what was not
  checked interactively.

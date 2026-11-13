# Dashboard tracker: batched row label picks, hover pencil for the title

Owner feedback 2026-11-13 on the dashboard tracker list (screenshots, not
tracked, under `D:/Temp/tracker-feedback/`).

## 1. Row label picker applies once, on close

### Problem

The label button on a tracker row (`row-label-popover.png`) opens a popover
of label chips; every click applies at once (one server write per click), so
each pick lags visibly and several picks in a row feel sluggish.

### Outcome

- The row's label picker reuses the checklist UI the new-item label menu
  already has (`new-item-label-menu.png`: checkbox, colour dot, name per
  label) for toggling. Whether the edit/delete pencil and x buttons and
  "+ 라벨 추가" also show there follows what that menu component already
  supports without new branches; keep it the same component.
- Toggles only change the local selection (immediate visual feedback, no
  write). When the picker closes (focus leaves it, outside click, Escape, or
  the button again) the net change is applied in one write; no change means
  no write. Escape still applies (owner: "포커스 풀릴 때 한번에 적용"), unless
  the existing menu has a cancel convention, in which case follow it and say
  so.
- If the apply fails, the row shows its server state again (no stuck local
  selection).

## 2. Hover pencil to rename the title

### Problem

The title is the field the owner changes most, and renaming it takes several
steps.

### Outcome

- Hovering a tracker row shows a pencil button immediately right of the
  title text (`title-pencil-spot.png`: the spot where the label tag icon
  sits; place the pencil next to it, before or after, whichever matches the
  row's existing hover-icon pattern). Keyboard focus on the row shows it too,
  like the row's other hover affordances.
- Clicking it edits the title in place: Enter saves (one write, skipped when
  unchanged or empty after trim), Escape cancels, blur saves. Reuse the
  item edit form's title save path; no new server API.
- Use the dashboard's existing lucide icon set for the pencil.

## Both

- Assets only under `agents-plugin-tool/internal/wsdashboard/assets/`
  (tracker.js / tracker.css and i18n tables if a label/tooltip string is
  added: add it to every `internal/wsi18n/tables/*.json`). T0.
- Two commits, one per section.

## Out of scope

- The detail-pane label editor, other tracker views, server-side changes.
- Restarting the dashboard daemon or Claude sessions (lead does the live
  check).

## Verification

- `go test ./internal/wsdashboard/... ./internal/wsi18n/... -count=1` on the
  remote w5090 (clone note `remote-build-w5090`), and any existing JS test
  for tracker assets.
- Headless Edge render (as in 6b4407b62's check) or equivalent showing the
  picker checklist and the hover pencil; state what could not be checked
  visually.

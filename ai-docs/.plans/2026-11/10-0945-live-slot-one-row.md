# Plan: agents panel shows one live row per running agent

## Request

User, 2026-11-10 whiteboard, with a screenshot of a running worker showing two
live rows ("Bash" and "Bash 인자 작성 중"): "이거 그냥 한 줄씩만 뜨게 하죠" -
show just one line. Lead reading (board (584), assumed): a running agent's
fixed live rows drop from two to one.

## Change

- `agents-plugin-tool/internal/wsdashboard/pageview.go`: `liveSlotCount`
  2 -> 1. `liveSlots` already keeps the latest entry, and a pending permission
  prompt takes the slot when present (keep drops to 0, so no entry shows
  beside it). Update the comments that say "two" or describe a permission
  prompt sitting above the newest entry.
- Tests in `internal/wsdashboard/` that assume two rows (slot counts,
  placeholder rows, permission-above-entry order) are updated to the one-row
  behavior; keep the assertions meaningful (latest entry wins; a permission
  prompt replaces it; an idle running agent shows one placeholder).
- Leave the lead row's `liverows` and the peer tree unchanged (not in the
  screenshot; their variable rows were a separate decision).
- CSS: if a fixed two-row height exists for `ol.brows.live.bguide`, adjust it.

Supersedes 5e42cb819's "exactly two live rows" by user direction.

## Verification

`go test ./internal/wsdashboard/... -count=1` under `agents-plugin-tool`, and
`go vet ./internal/wsdashboard/...`.

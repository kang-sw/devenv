# Plan: whiteboard assumed mark as a badge, Korean wording "승인으로 해석"

## Request

User, 2026-11-10 whiteboard (topic "화이트보드: assumed 표시"):
- (599) ok: draw the settled row's assumed mark as a small badge (pill)
  instead of bare faint-pink text, so it separates from the ID and text.
- (600) a: Korean wording "승인으로 해석" (tooltip "승인으로 해석함, 확인하거나
  수정하세요"). English "assumed" stays.

## Changes

1. `agents-plugin-tool/internal/wswhiteboard/assets/page.css` ~:272
   `.srow .amark`: keep the 10.5px bold text; make it an inline pill: light
   tint of `--wb-accent-assumed` as background (e.g. `color-mix(in srgb,
   var(--wb-accent-assumed) 18%, transparent)`), text colour from the same
   accent (keep it readable in light and dark), `border-radius: 999px`, small
   horizontal padding, vertical-align matching the row. Follow the existing
   `.chip` shape conventions; no new tokens. Letter-spacing may go if it looks
   off in Korean. Check any other place that renders the assumed mark (grep
   `amark` across wswhiteboard and wsdashboard assets, templates, and embed
   output) so every surface gets the same badge.
2. `agents-plugin-tool/internal/wsi18n/tables/ko.json`: `chip.assumed` ->
   "승인으로 해석"; `wb.assumed.title` -> "승인으로 해석함, 확인하거나
   수정하세요". Grep ko.json for other strings using "가정" for the assumed
   status and align them only if they name this same mark.
3. Update any test that pins the old Korean string or the old CSS.

## Verification

- Go tests only remotely: `~/.local/bin/ws-rgo -C
  /home/swkang/.ws-worktrees/devenv/gamma-washcloth-everyone/agents-plugin-tool
  -m 30 -- test ./internal/wswhiteboard/ ./internal/wsi18n/
  ./internal/wsdashboard/ -count=1 -timeout 30m`. Never run go build/test/vet
  locally.
- Never touch processes or ports 29417 / 45743.
- Commit on branch `feat/assumed-badge`; do not push.

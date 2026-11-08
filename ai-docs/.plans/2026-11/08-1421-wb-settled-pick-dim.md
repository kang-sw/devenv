# Plan: tone down the picked-option highlight on settled whiteboard items

Branch `delegate/wb-pick-dim` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`). Commit here; do not
merge or push. Read `ai-docs/manuals/shipped-surface-boundary.md` first.

## User feedback (2026-11-08, screenshot of settled items (428), (422))

"이미 선택한 선택지가 너무 표현이 강하다 ... 이 경계선 표현을 이미 완성된
선택지에서는 조금 dim하게 표현하던지 톤을 죽이는 게 좋아 보여요."

## Scope

`agents-plugin-tool/internal/wswhiteboard/assets/page.css` L194 gives the same
strong accent ring plus glow (`box-shadow: 0 0 0 1.5px accent, 0 0 10px ...`)
and `font-weight: 600` to four states: an open item's pressed option
(`.opt[aria-pressed="true"]`), and the picked option on a settled item - the
settled-row chip `.pick` (template `srow`, page.tmpl L52) and the expanded
settled card's `.opt.static.picked` (page.tmpl L46).

- Keep the open item's pressed option exactly as is (it is live input).
- For the settled states (`.pick`, `.opt.static.picked`): drop the glow and
  replace the strong ring with a thin, low-contrast border (e.g. the accent
  mixed toward transparent at roughly 35-45%), normal or 500 weight. The
  picked option must stay distinguishable from unpicked static options
  (`.opt.static:not(.picked)` is already dimmed to opacity .75), in light and
  dark themes. Exact values are your call; note them.
- CSS only. Do not touch `markdown.go` (a parallel worker edits it). Update
  any test that pins the old CSS text, if one exists.

## Verification

- `cd agents-plugin-tool && go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/`.
- Before/after screenshots, light and dark, of a page with a settled row
  carrying a picked chip, an expanded settled card with a picked option, and
  an open item with a pressed option, to `/tmp/wb-pick-dim/` (headless
  Chromium via cached Playwright under `~/.npm/_npx/*/node_modules/playwright`;
  render a fixture page through a throwaway test or the existing render
  helpers, not committed).

## Return

Commit hash, the values chosen, screenshot paths, test output.

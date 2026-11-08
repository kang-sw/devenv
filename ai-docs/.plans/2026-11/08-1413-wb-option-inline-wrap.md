# Plan: whiteboard option text breaks into columns around inline code and links

Branch `delegate/wb-option-inline-wrap` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`). Commit on this
branch; do not merge or push. Read
`ai-docs/manuals/shipped-surface-boundary.md` before editing under
`agents-plugin-tool/`.

## Bug (user dogfood, 2026-11-08, dashboard screenshot)

A settled policy item's option (board item (422) option a) whose text mixes
plain text, inline code spans (`tracker.query`, `tracker.update`), and an item
reference link (`{{421}}`) renders as side-by-side narrow columns: each text
run, code span, and link sits in its own column and wraps inside it ("MCP /
도구 2 / 개 | tracke / r.quer / y | , | tracker / .update | ..."), instead of
one flowing paragraph. User: "대시보드에서 코드 블록이 중간에 섞이면 이런
식으로 희한하게 행바꿈이 되네요".

Likely area: the non-stacked option markup in
`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl` (L38 and L46:
`<span class="opt ...">` / `<button class="opt">` with `<span>{{Block}}</span>`)
and `.opt` / `.opts` in `assets/page.css` (L188-L202: `.opt` is a 2-column
grid, `.opts` a wrapping flex row). The stacked variant wraps the text in
`<div class="md">` and may not show the bug. Confirm the actual cause; the
dashboard may embed the page with its own CSS (`internal/wsdashboard/assets`),
so check where the screenshot's rendering comes from.

## Fix

Option text renders as one flowing block (wrapping normally, code spans and
links inline) in both the stacked and non-stacked variants, on the
whiteboard page and wherever the dashboard shows it. Keep the letter column
and the picked / recommended styling unchanged. A parallel worker is editing
`internal/wswhiteboard/markdown.go` (inline-tag rendering) on another
branch; stay out of markdown.go unless the cause is there, and say so if it
is.

## Verification

- A rendered-page test (Go, in the existing wswhiteboard or dashboard test
  style) pinning that an option with inline code and a link produces a
  single wrapping text container, if the structure is testable that way.
- Headless Chromium screenshot (cached Playwright under
  `~/.npm/_npx/*/node_modules/playwright`) before and after of an item with
  such an option, open and settled, light and dark, saved to
  `/tmp/wb-option-inline-wrap/`.
- `cd agents-plugin-tool && go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/`.

## Return

Commit hash, the root cause, changed files, screenshot paths, test output.

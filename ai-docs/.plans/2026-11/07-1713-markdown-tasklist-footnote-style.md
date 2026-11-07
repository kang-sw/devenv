# Plan: rich markdown - task lists, footnotes, and table and blockquote styling

## Outcome

The user dogfooded a numbered render showcase on the dashboard (2026-11-07)
and reported:

- Task-list checkboxes `- [ ]` / `- [x]` show as raw text everywhere: in the
  conversation (lead replies), in the Now slot, and in whiteboard item fields
  (options, rationale, recommendation).
- Footnotes (`text[^1]` with a `[^1]: body` definition) show as raw text in
  the conversation.
- Tables render, but "the table style itself reads poorly". Restyle them on
  every surface: clear header row, cell padding, subtle borders or row
  separation, long cells wrapping, wide tables scrolling horizontally inside
  their block instead of overflowing.
- Blockquote style should improve. User's words: "지금 테마의 파란색 스타일로
  왼쪽 회색 띠가 은은하게 칠해지게". Lead reading: a left bar in the theme's
  accent (blue) colour, with a faint tinted background, muted text kept
  readable, in both light and dark themes.

Everything else in the showcase was confirmed fine: headings, inline marks,
links, code blocks, mermaid, images, details, SVG, canvas, raw-HTML-as-text
in board fields. Do not regress them.

## Evidence (lead-verified)

- `agents-plugin-tool/internal/wswhiteboard/markdown.go` ~88-128 and
  `store.go:63` build goldmark with only `extension.Table` and
  `extension.Strikethrough`. TaskList and Footnote are not enabled.
- Find every goldmark construction that renders these surfaces, including the
  lead-reply markdown in `internal/wsdashboard` if it has its own. Enable the
  extensions consistently. The `store.go` `nowParser` (used for parse-only
  checks such as the full-document check) should stay consistent too.
- Styles: `internal/wswhiteboard/assets/page.css`,
  `internal/wsdashboard/assets/dashboard.css`, and the CSS inlined into the
  sandboxed Now frame (merged 229596b4b, `assets/now-frame.js` / `page.css`
  inlining, selectors under `.now-body`).

## Settled constraints

- Use the existing CSS variables (accent, muted, border, background). No new
  colour literals except where no variable exists.
- Task-list checkboxes render disabled (read-only). Footnote ids must not
  collide when several messages on one page each have a `[^1]`. Pick a fix,
  such as a per-message id prefix, and test it. If goldmark's footnote
  extension cannot be prefixed per render, say how you solved it.
- Keep the HTML sanitization or escaping posture of each surface unchanged.
  Board fields still show raw HTML as text.
- Shipped-surface boundary applies (`ai-docs/manuals/shipped-surface-boundary.md`).
- Another delegate is editing claude-mod, the Pi feed, `internal/wslive`, and
  the conversation/agents-tree rendering for a compaction row. Keep
  `internal/wsdashboard` changes limited to markdown setup and CSS.

## Boundary

Commit on branch `delegate/markdown-tasklist-footnote-style` only; do not
merge or push.

## Verification

- agents-plugin-tool: `go build ./... && go vet ./internal/... ./cmd/...`;
  `go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/mcp/`.
  Add tests for task lists and footnotes, including two messages with `[^1]`.
- Headless Chromium (cached Playwright under
  ~/.npm/_npx/*/node_modules/playwright): render a board and a session page
  with a task list, a footnote, a long-cell table, a wide table, and a
  blockquote in the conversation, the Now frame, and a board option. Check
  light and dark. Take screenshots for yourself and judge readability.
- Report what was not checked.

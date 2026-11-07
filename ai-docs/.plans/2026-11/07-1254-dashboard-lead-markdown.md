# Plan: render lead replies as markdown in the dashboard conversation

## Outcome

The live session page's conversation (`r-conversation` in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`) shows lead
replies as plain text, so headings, lists, code blocks, and tables from the
lead are unreadable. Render lead reply bubbles as markdown: finished `reply`
entries and the streaming reply (`.Conversation.Reply`). User messages, mail
bodies, and turn lines stay plain text.

## Settled constraints

- Reuse the board's goldmark setup in
  `agents-plugin-tool/internal/wswhiteboard/markdown.go` rather than adding a
  second markdown stack. If it needs a small exported entry point usable
  without a whiteboard page, add the narrowest one; keep the board's own
  rendering unchanged.
- Safety matches the board: raw HTML renders as text, never as markup; links
  are http(s) or render as path text; local images follow the board's rule on
  the served page (inside the session root through `/s/<rid>/image?path=`,
  otherwise path text). Do not add mermaid to conversation bubbles.
- The streaming reply is partial markdown; render it the same way (an
  unclosed fence simply renders as an open code block). Keep the caret.
- The region hash must still change only when the rendered content changes
  (render deterministically).
- Style rendered markdown inside `.bub` compactly (paragraph spacing, code
  block overflow scrolls horizontally, tables scroll) in both themes, reusing
  board CSS where it applies.
- Tests: render cases for a heading/list/code/table reply, raw HTML escaped,
  a streaming reply with an unclosed fence, and a user entry still plain.

## Boundary

Touch only the conversation rendering path (template entry/reply parts, the
Go that builds conversation view data, the markdown helper, CSS for `.bub`
markdown). Another delegate is concurrently reworking the page shell (composer
buttons, gear menu input mode, agents/conversation split, lead status rows) in
the same template/JS/CSS files; do not touch those parts. Return to the lead
if this needs a snapshot or adapter schema change.

## Verification

`go test ./internal/wsdashboard/ ./internal/wswhiteboard/ -count=1`,
`go vet ./...`, `go test ./... -count=1` in `agents-plugin-tool`.

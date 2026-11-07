# Plan: kanban ticket card - category chip inline with the title

## Outcome

The whiteboard Tickets panel kanban cards (c0384c306) put the category chip
(`feat`, `epic`, ...) on its own line above the title. The user wants no line
break between them: the chip sits at the start of the title line and the title
text follows it directly on the same line (2026-11-07: "캡슐이랑 본문 개행 안
되게 해주세요. 캡슐 뒤에 타이틀이 바로 따라붙게요").

- The chip is inline at the start of the title's first line; the title wraps
  after it and keeps its two-line clamp (the chip counts as part of the first
  line). The muted stem line stays below.
- Chip vertical alignment matches the title text baseline/center, and the
  chip never wraps alone onto its own line.
- An empty-title card (stem shown as title) behaves the same way.

## Settled constraints

- Touch only `agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl`,
  `page.css`, and `internal/mcp/whiteboard_panels.go` / tests if the markup
  must move. No change to lanes, which tickets are listed, links, or tooltips.
- Do not touch `internal/wsdashboard` (another delegate is merging there).

## Boundary

Commit on this branch only; do not merge or push.

## Verification

`go build ./... && go vet ./internal/... ./cmd/...`;
`go test -count=1 ./internal/wswhiteboard/ ./internal/mcp/` in
agents-plugin-tool. Render a sample board and check in headless Chrome (cached
Playwright under ~/.npm/_npx/*/node_modules/playwright) light and dark: chip
and title on one line, two-line clamp holds with a long title, no overflow.

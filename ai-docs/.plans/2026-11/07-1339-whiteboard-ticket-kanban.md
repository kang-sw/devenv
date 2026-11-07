# Plan: whiteboard Tickets panel as a vertical-stack kanban

## Outcome

The whiteboard's Tickets panel renders today as one flat list: each row is a
status badge line, then the title and the full repo-relative path as a long
inline code span, which wraps and dominates the row. The user finds it hard to
read and asked for a "vertical-stack kanban" layout (2026-11-07: "tickets
보드가 약간 보기 별로인데 수직 스택 칸반? 느낌으로 배치하는 게 어떨까 싶어요").

Lead's reading of that request, which is the target:

- One lane per status (`ready`, then `todo`), side by side as kanban columns,
  each lane a vertical stack of compact cards. When the panel is too narrow
  for side-by-side lanes (phone, narrow board column), the lanes stack
  vertically instead, ready first.
- Lane header: the status name and its count (the todo lane keeps today's
  "latest N" cap, and says so when it truncates).
- Card: the title (clamped to two lines) as the primary text, and a small muted
  second line with the stem (date + category). The category (feat, bug,
  epic, research, chore, refactor) shows as a small chip so epics stand out.
  The full path no longer shows as body text. The card keeps today's link
  behavior: on a disk board the card links to the file; on a daemon-served
  page, where file paths render as text, keep the path reachable without
  showing it as body text (e.g. title tooltip) - do not invent a new route.
- A ticket whose title is empty shows its stem as the title (already true).

## Settled constraints

- Code: `agents-plugin-tool/internal/mcp/whiteboard_panels.go` (`ticketPanel`
  builds generic `wswhiteboard.Panel` rows) and the whiteboard render/assets in
  `agents-plugin-tool/internal/wswhiteboard/` (`render.go`, `assets/`). Choose
  the minimal structure: a panel-specific layout hook or a lanes field on the
  panel model, whichever fits the existing render with the least new surface.
  Other panels (mailbox, todos, agendas) stay unchanged.
- It must render on both the disk board (full page + embed snapshot) and the
  daemon-served session/board pages, light and dark themes, using existing CSS
  variables.
- No change to which tickets are listed (ready all, todo latest N, idea
  hidden) or to the panel's count chip.
- Another worker is concurrently editing the dashboard session page
  (`internal/wsdashboard/assets/`, `pages.go`, conversation code); stay out of
  `internal/wsdashboard` unless a served-page test needs an expectation update.
- Read `ai-docs/manuals/shipped-surface-boundary.md` first (whiteboard output
  ships downstream).

## Boundary

Commit on this branch only; do not merge or push. Return to the lead if the
layout needs a new route, a protocol/payload change, or touches other panels.

## Verification

`go build ./... && go vet ./internal/... ./cmd/...`;
`go test -count=1 ./internal/wswhiteboard/ ./internal/mcp/ ./internal/wsdashboard/`
in agents-plugin-tool (update render expectations that change). Render a board
with a few ready and todo tickets (including an epic and an empty-title
ticket) to an HTML file and check it in headless Chrome at a wide (~1400px)
and narrow (~400px) width, light and dark: lanes side by side when wide,
stacked when narrow, no horizontal overflow, titles clamp. Report what was
and was not checked.

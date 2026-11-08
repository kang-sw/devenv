# Plan: agenda panel - stale entry diagnosis and a hierarchical tree card

Branch `delegate/agenda-panel-tree` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`), based on
develop. Commit here; do not merge or push.

Source: user on the whiteboard, 2026-11-08: "대시보드 UI에서 아직
dashboard-round2 아젠다가 보여서요. 이건 그러면 렌더링 쪽에 뭔가 부실한 게
숨어있는 걸까요? 그리고 아젠다 쪽 렌더링도 사실 거의 반만 되어있는
상태잖아요? 이건 delegate한테 맡겨서 적당히 봐줄만한 계층 트리 카드로
보이게 자율적으로 맡겨볼 수 있을까요?" The visual design is left to you.

Read `ai-docs/manuals/shipped-surface-boundary.md` and
`ai-docs/manuals/ws-mcp.md` before editing under `agents-plugin-tool/`.

## 1. Diagnose the stale agenda entry

The lead cleared agenda key `dashboard-round2` with `ws/agenda.clear` on
session `trickily-shrivel-emotion` (the merged `overnight` blob was set right
before it), and the user still sees `dashboard-round2` in the dashboard's
agenda panel. `internal/mcp/whiteboard_tools.go:118-120` lists `agenda.clear`
in the agenda panel's `dirtiedBy`, so on paper it re-renders.

- Find why the entry survives. Candidates, not conclusions: the installed
  ws-mcp binary predates the dirtiedBy wiring (the user has not run
  `./install.sh update` recently; compare the installed binary's version with
  the commit that added it); the dirty match compares a name form that
  `agenda.clear` does not produce; clear does not re-render when it leaves
  the last change to another blob; the dashboard daemon serves a cached page
  or snapshot, or its SSE does not push this change; the page reads agenda
  from another source than the session record.
- Read-only checks against the live session are fine (the session record on
  disk, the rendered page file, the daemon page over HTTP at
  `http://127.0.0.1:29417/s/affluentxboxspongeprelaw`). Do not call
  `agenda.*` or `whiteboard.update` on the lead's session `trickily-shrivel-emotion`; reproduce in a test or a
  scratch session instead.
- If it is a source bug, fix it with a test that fails before the fix. If it
  is only the installed binary being old, say so with evidence and change
  nothing for it.

## 2. Agenda panel as a hierarchical tree card

Today `agendaPanel` (`whiteboard_tools.go:155-172`) shows one row per key
with the one-line `agendaSummary` (e.g. `{agent_tickets, board_last_id, ...`),
which reads as half-done. Replace it with a readable hierarchical tree card:

- Each agenda key is a top-level node; a blob's JSON object fields become
  child nodes, nested objects and arrays nest further; scalar values show as
  text (long strings wrap or truncate with a way to see the whole value).
- Collapsible nodes; a sensible default (for example keys expanded one level,
  deeper levels folded). Keep it compact enough beside the rest of the page.
- Design, markup, and CSS are your call; follow the existing panel and
  card styling and the theme variables (light, dark, pinned theme). Any new UI
  string goes through `wsi18n` tables like the existing panel title.
- The comment at `whiteboard_tools.go:155-157` records the earlier choice to
  never show the blob body. The user's request above supersedes it; update
  the comment and record the reversal in the commit's `## AI Context`.
- Decide sensibly for the embed snapshot (it may stay compact) and say what
  you chose.
- `WS_MCP_DASHBOARD_DEV_SOURCE` (dev mode, see `ws-mcp.md`) serves dashboard
  assets from a source tree for UI tuning; headless Chromium via cached
  Playwright under `~/.npm/_npx/*/node_modules/playwright` can screenshot the
  result. Run any test daemon on a free port and a scratch session, not the
  user's running daemon.

## Out of scope

Other panels, the todo panel, agenda storage format, and the `agenda.*` tool
contracts.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/mcp/ ./internal/wswhiteboard/ ./internal/wsdashboard/ && go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/ && go test -count=1 -run 'Whiteboard|Agenda|Dashboard|Panel' ./internal/mcp/`,
plus a rendering test for the tree (nested object, array, long string, empty
agenda), and headless screenshots in light and dark saved under `/tmp/agenda-tree/`.

## Return

The stale-entry diagnosis with evidence (and fix or none), commit hashes,
changed files, test output, screenshot paths, and anything left open.

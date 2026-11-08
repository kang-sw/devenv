# Plan: dashboard session page top-bar git badge (daemon-side read)

Branch `delegate/topbar-git-badge` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`), based on develop.
Commit here; do not merge or push.

Read `ai-docs/manuals/shipped-surface-boundary.md` and
`ai-docs/manuals/ws-mcp.md` before editing under `agents-plugin-tool/`.

## Source and the decision it reverses

User, whiteboard 2026-11-08: "daemon에서 git을 안 쓰기로 이전에 결정했다고
했는데, 지금 보니까 생각보다 불편해서요. 상단 줄? 톱니바퀴와 실시간 마크
사이? 현재 폴더의 git 상태를 보여주는 (시각 문법은 status band랑 비슷하게)
뱃지가 하나 있으면 좋겠는데요. 주기는 한 3분 정도 한 번씩 git status를
호출하면 부담이 안 되지 않을까요"

This reverses `ai-docs/tickets/.done/261107-feat-ws-dashboard-status-band.md`
(Decisions ~L84-90: git fields computed by the session's ws-mcp; rejected
"daemon-side git per render (cost grows with sessions served)"). That ticket
stays closed; record the reversal in the commit's `## AI Context`, citing
board (461)-(464). The status band itself is unchanged.

## Confirmed decisions (board)

- (464) a: the badge appears on the **session page top bar only**, between
  the settings gear and the live mark, for that session's root folder. Not
  on the hub / session list.
- (461): the daemon reads git only for sessions whose page is open right now
  (at least one live page/SSE subscriber), and reads one root once even when
  several open pages share it. Nothing is read for sessions nobody views.
- (462): read once immediately when a page opens (or the first subscriber
  arrives), then every 3 minutes while it stays open; clicking the badge
  re-reads at once. Never `git fetch`; behind reflects the last fetch.
- (463): content follows the status band's grammar: branch, ahead/behind
  (no-upstream told apart from in-sync, as `wsgit.BranchStatus.Upstream`
  does), and working-tree counts `~M +S ?U`; a clean tree shows no change
  chip; a failed read (or a non-repository root) shows the badge dimmed or
  hidden, your call, with a title saying why. Reuse `wsgit.BranchSync` and
  `wsgit.WorkingTreeCounts` (`--no-optional-locks`) as they are; reuse the
  status band's CSS classes (`.sb-*`) or share them rather than copying.

## Implementation notes (yours to decide)

- How the value reaches the page (SSE event, a field on an existing frame,
  a small endpoint) is your call; prefer the existing session SSE stream and
  an existing auth gate over a new channel. A click-refresh request must be
  owner-authenticated like the other session endpoints and rate-limited
  enough that repeated clicks cannot spawn concurrent git processes for one
  root.
- Bound each git run with a timeout and do not hold any daemon-wide lock
  while git runs. Stop the poll for a root when its last viewer leaves.
- Any new UI string goes through `wsi18n` tables (all languages present).
- Theme: light, dark, and pinned theme via the existing variables.
- Another delegate is reworking the agenda panel in `internal/mcp/whiteboard_tools.go`
  and its whiteboard CSS in parallel; stay out of the agenda panel.
- `WS_MCP_DASHBOARD_DEV_SOURCE` dev mode and cached Playwright under
  `~/.npm/_npx/*/node_modules/playwright` are available for a headless check.
  Run any test daemon on a free port with a scratch session, never the
  user's running daemon at 127.0.0.1:29417, and do not call any ws tool on
  the lead's session `trickily-shrivel-emotion`.

## Tests

- Poller: starts on first viewer, reads immediately, re-reads on the
  interval (inject the clock/interval), stops when the last viewer leaves,
  dedupes shared roots, coalesces concurrent refresh requests.
- Render: badge markup for clean, dirty, ahead/behind, no upstream, detached,
  non-repository, and read failure.
- Refresh endpoint auth (refuses without the owner credential).
- Headless screenshots, light and dark, under `/tmp/git-badge/`.

## Out of scope

The hub / session list, the status band, `git fetch`, and branch actions.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./... && go test -count=1 ./internal/wsdashboard/ ./internal/wsgit/ && go test -count=1 -run 'Dashboard|Session' ./internal/mcp/`
plus `ai-docs/manuals/ws-mcp.md` updated for the daemon's new git read.

## Return

Commit hashes, changed files, test output (which new tests failed before the
change), screenshot paths, and anything left open.

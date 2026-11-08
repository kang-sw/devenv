# Plan: merge develop into the drawer UI branch and resolve conflicts

Worktree `/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`,
branch `impl/develop/bribe-diner-candy` (ticket
`261108-feat-ws-tracker-drawer-ui`, closed done on this branch). Do not
merge into develop, do not push. Do not run git stash. Do not rebase.

## Task

`git merge develop` (develop is at c9f642e17 or later) into the branch and
resolve every conflict so both sides' behavior survives. Conflicts reported
by `git merge-tree`:

- `agents-plugin-tool/internal/mcp/tracker_tools.go`, `tracker_tools_test.go`
  - develop: XML entity decoding via `wsxmlcmd` grammar flags
    (`Spec.DecodeBody` / `DecodeFields`, merge ba2f482fd), redaction and
    review-fix changes; branch: redaction moved into `wstrackerhost`,
    batched dispatch.
- `agents-plugin-tool/internal/wstrackerhost/poll.go`: develop review fixes
  vs branch's poll round hook.
- `agents-plugin-tool/internal/wsdashboard/daemon.go`, `access_test.go`,
  `assets/dashboard.js`, `assets/dashboard.tmpl`: develop added the
  whiteboard topic-header work (closed-topics route `/s/<id>/closed`, topic
  note pen sent with the composer, `ago.js`), plus earlier dashboard
  changes; branch added `/tracker/*` routes, tracker drawers, SSE
  `event: tracker`.
- `ai-docs/manuals/ws-mcp.md`: keep both sections (develop added "Session
  key store" and the closed-topics route; branch added the tracker host
  paragraph).

Also note develop merged the session key split (c9f642e17):
`mutateRecord`/`readState` in `internal/mcp` gained a parts argument. If
anything on the branch calls them, update the calls.

Use `git log develop --oneline -15` and each side's commit `## AI Context`
to understand intent. When both sides changed the same logic, keep both
behaviors; if they genuinely contradict, stop and report rather than drop
one.

## Verification

`cd agents-plugin-tool && go build ./... && gofmt -l . && go vet ./... && go test -count=1 ./internal/wstrackerhost/ ./internal/wsdashboard/ ./internal/wsxmlcmd/ ./internal/wswhiteboard/ ./internal/wsi18n/ ./internal/wsrsrc/ ./cmd/ws-mcp/ && go test -count=1 ./internal/mcp/`
plus `node --check` on `internal/wsdashboard/assets/dashboard.js`,
`tracker.js`, and `internal/wswhiteboard/assets/form.js`. If cheap, one
headless Playwright smoke (cached Playwright under
`~/.npm/_npx/*/node_modules/playwright`, isolated WS_CACHE_HOME and
WS_CONFIG_HOME under /tmp) that the session page loads with no page errors,
the tracker handle opens, and the whiteboard "show more closed topics"
button and topic pen still render.

## Return

The merge commit hash, how each conflict was resolved (one line each),
verification output, anything left open.

# Brief: remove the agy lead-call usage trigger (owner-approved removal)

## Outcome

Remove the "refresh agy usage on lead tool calls" mechanism that
261120-feat-ws-agy-usage-activity-trigger Phase 2 added. Its only purpose was
a user-started, non-hosted agy CLI lead, and the owner has stated that case is
not supported. The mechanism resolves to nothing for every supported setup.

Keep:
- Phase 1, the hosted agy turn-end trigger: `wsagent.Options.AgyActivity`,
  the `endTurn` call in `internal/wsagent/runner_events.go`,
  `Daemon.agyActivity` and `refreshAgy`. Do not touch any of it.
- The test isolation in `internal/mcp/server_test.go` `runTestMain` (always a
  temp WS_CACHE_HOME), from commit 545226868. It is independent and must stay.
- The popover-driven probe trigger in `plan_usage_probe.go`.

## What Phase 2 added (develop commits 37c4fcc76, f1ca9d1b1, be834f9ae, 86bad7ff0)

Each commit's diff: `git show <sha>`. Files:
- internal/mcp/plan_usage_activity.go (new) and plan_usage_activity_test.go (new)
- internal/mcp/dashboard_sessions.go: the hook in dashboardTouch
- internal/mcp/server.go: the server fields
- internal/wsagent/registration.go: the `PlanUsageActivity` descriptor field
- internal/wsdashboard/plan_usage_activity.go: the daemon host method
- cmd/ws-mcp/dashboard.go (a one-line change) and cmd/ws-mcp/agent_mcp_test.go (+5)

Remove exactly these additions, and nothing Phase 1 or other work added. If a
piece looks shared with Phase 1 or other code, keep it and report it.

## Compatibility

Confirm by reading the source that `PlanUsageActivity` on the backend
registration is in-process only: not serialized to any store, the hello, an
MCP argument, or a manifest. If it is, stop and report before removing it.
Expected tier: T0.

## Permitted actions

- Edit Go source and tests under agents-plugin-tool/.
- One commit on impl/develop/agy-lead-trigger-removal, with `## AI Context`
  and no Co-Authored-By line. Do not edit tickets: the Phase 2 ticket is in
  `.done/` and is frozen.
- No push, no branch switch, no daemon, no Electron, no real agy binary, no
  credentials.

## Verification

Run every Go command from agents-plugin-tool/ with the prefix
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local:
- `go build ./...`
- `go vet ./internal/mcp ./internal/wsagent ./internal/wsdashboard ./cmd/ws-mcp`
- `go test -count=1 -timeout 20m -run 'PlanUsage|AgyActivity|Dashboard' ./internal/mcp`.
  It is safe only because runTestMain now isolates the cache; confirm that
  fix is present before running.
- `go test -count=1 -timeout 20m -run 'PlanUsage|AgyActivity' ./internal/wsdashboard ./internal/wsagent`
- Narrow `-run` on the cmd/ws-mcp test you touched.
- Never run the whole ./internal/mcp or ./internal/wsdashboard package.
- Real-cache proof: count `~/.cache/ws@kang-sw-devenv/dashboard/sessions/*/registration.json`
  and the rows whose project path is under `/var/folders`, `/private/var` or
  `/tmp`, before and after your test runs.
- `grep -rn 'PlanUsageActivity\|planUsageActivity' agents-plugin-tool` must
  come back empty, apart from anything you report as intentionally kept.

## Return boundary

Return after the single commit, or earlier if removal would touch Phase 1,
the runTestMain isolation, or a serialized shape. Report:
- the commit sha
- the files and lines removed
- every command run, with its result
- the cache counts before and after
- the compatibility finding with file:line

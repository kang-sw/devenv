# Opt-in pprof on the dashboard daemon

User decision (board (111) a, 2026-11-09): add an opt-in pprof endpoint to the dashboard daemon so its CPU use can be profiled at function level.

## Context

The Windows dashboard daemon (`ws-mcp dashboard --port 29417`) holds ~0.64 core steadily; the WSL one averages ~16%. A 10 s kernel file ETW trace of the Windows daemon showed ~1,500 file opens/s, ~2,600 reads/s (9.3 MB/s), ~7,000 query-info/s, no child processes, 23 open connections. Opens concentrate on other projects' `ai-docs/tickets/{ready,todo}` (directory listings plus each ticket file), `~/.cache/ws@.../mailbox-history/<key>/*.json`, `mailbox-replyids.json`, `dashboard/sessions/<rid>/{ring,inbox/drained,snapshot.json}`, and `sessions/<key>/{agenda,todos,whiteboard,key}.json`. File names alone do not say which code path re-reads them; pprof will.

## Outcome

- The daemon serves `net/http/pprof` handlers under `/debug/pprof/` only when an environment variable (name it in the `WS_` family, e.g. `WS_DASHBOARD_PPROF=1`) is set in the daemon's environment. Off by default: without it the routes do not exist (404 through the normal mux, not a special refusal).
- The pprof routes are reachable only from loopback, even in LAN mode (the LAN token must not open them). Wire them through the daemon's existing access guard / mux in `internal/wsdashboard` so `TestLANModeGuardsEveryRoute` (or its equivalent) covers them; extend that test or add one that pins: off -> 404; on + loopback -> 200 on `/debug/pprof/`; on + LAN request -> refused.
- Make sure the variable reaches the daemon: check how `ws-mcp` launches the daemon (`ensure.go`, the copied `ws-mcp-dashboard-*` binary) and whether the child inherits the environment; if it does not, pass the variable through. Document the variable where the dashboard's other env/config knobs are documented (find it; e.g. `ai-docs/manuals/ws-mcp.md` launcher environment), one or two lines.
- No other behavior changes.

## Verification

From `agents-plugin-tool/`: `gofmt -l ./internal ./cmd`, `go vet ./internal/... ./cmd/...`, `go test ./internal/wsdashboard/ ./cmd/ws-mcp/ -count=1` in the foreground. Then build a local binary and run a dev daemon with isolated `WS_CACHE_HOME`/`WS_CONFIG_HOME` and the variable set, and confirm `curl http://127.0.0.1:<port>/debug/pprof/` answers and `go tool pprof -top http://127.0.0.1:<port>/debug/pprof/profile?seconds=5` works; stop the dev daemon afterwards. Do not touch the user's running daemons.

Return when committed on branch `dashboard-pprof` with verification passing, and state the exact command a user runs to profile the daemon.

# Plan: dashboard daemon sticky fallback port

## Outcome

When the configured dashboard port (`dashboard.port`, default 29417) is taken
by another program, the daemon today falls back to an OS-assigned port and
forgets it (`agents-plugin-tool/internal/wsdashboard/daemon.go` `listen`).
Every daemon restart then lands on a new origin, so the page's localStorage
(`ws.dashboard.layout`, expanded peers) is lost and a LAN bookmark breaks.

Change port selection to: configured port -> last-used port -> OS-assigned.

## Settled constraints

- After a successful bind, record the actually bound port in its own small
  file in the dashboard state directory (`DaemonOptions.Dir`), not in
  `endpoint.json`: `endpoint.json` signals a live daemon and must keep that
  single meaning.
- Try order: `opts.Port` when > 0; then the recorded last port when it differs
  from `opts.Port` and is valid; then port 0. A missing, unreadable, or
  malformed record is ignored (no error), so the fallback never fails startup
  for a reason it does not today.
- When the configured port is free again, the daemon binds it (configured
  port always wins), and the record then holds the configured port.
- `opts.Port == 0` (OS-assigned requested): keep today's behavior; do not try
  the recorded port. Tests use port 0 and must stay deterministic.
- Write the record while the single-instance lock is held (after bind), with
  the same file-permission posture as the other state files; a write failure
  is logged or ignored, never fatal.
- Host and LAN mode are unchanged; the record is one port for whichever bind
  host the daemon uses.

## Files

- `agents-plugin-tool/internal/wsdashboard/daemon.go` (`listen`, `StartDaemon`)
- possibly `endpoint.go` for the file-name constant
- tests beside them (`daemon_test.go` or a new `*_test.go`)
- `ai-docs/manuals/ws-mcp.md` where it documents `dashboard.port`, one
  sentence on the sticky fallback, if such a sentence fits there.

## Verification

- Tests: configured port free -> binds it and records it; configured port held
  by a foreign listener -> falls back, records the fallback; restart with the
  configured port still held -> binds the recorded port; configured port freed
  -> binds the configured port again; recorded port also held -> OS-assigned;
  malformed record -> ignored.
- `go test ./internal/wsdashboard/ -count=1`, `go test -race ./internal/wsdashboard/`,
  `go vet ./...`, `GOOS=windows go vet ./internal/wsdashboard/`, then
  `go test ./... -count=1` in `agents-plugin-tool`.

## Boundary

Return to the lead without expanding scope if the change needs anything
beyond the daemon's own port selection (for example Ensure, the client, the
pointer line, or a config knob).

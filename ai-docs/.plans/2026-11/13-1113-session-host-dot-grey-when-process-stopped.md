# Session header harness chip: grey dot when the session's process is stopped

Owner 2026-11-13, screenshot `C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\sessions\shrankhumpbackuncannyelaborate\inbox\attachments\5ded1c877446534d-1.png`:
the dashboard session page shows a harness chip `● claude ▾` with a green
dot. The owner wants the dot grey when the session's harness process is not
running (for example a hosted Claude lead whose process exited after its idle
timeout, or any session whose process is down), and green only while the
process is actually alive. The owner also asks whether the green comes from an
already-deleted session; answer that in the report.

## Do

1. Find the chip (dashboard.tmpl / dashboard.js / dashboard.css and the Go page
   view in `agents-plugin-tool/internal/wsdashboard/`) and what drives the
   dot's colour today (registration heartbeat `Live`, snapshot freshness,
   `host_stopped`, hosted holder state, ...). State it in the report.
2. Make the dot reflect "process running" vs "process stopped": green only
   while the session's harness process is running; grey when it is stopped
   but the session still exists (hosted: holder/process exited, e.g.
   `host_stopped:true` or no live process; non-hosted: no live heartbeat).
   Use data the page already has where possible; if the daemon must expose
   one more boolean to the page, add it (optional field, T0). Keep a tooltip
   that says which state it is, with any new string in all
   `internal/wsi18n/tables/*.json`.
3. The dot must update live (the page already streams session state; use the
   same path, no page reload needed).

## Out of scope

- Restarting the daemon or any session; creating sessions (the lead does the
  live check).

## Verification

- Go tests on the remote w5090 (`D:/sys_lock.sh remote w5090 run <who> <min>
  --sync <worktree> -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd
  agents-plugin-tool && go test ./internal/wsdashboard/... ./internal/wsi18n/...
  -count=1'`): a page-view test for running -> green class and stopped ->
  grey class.
- If JS changes, run the touched node-backed tests locally with node at
  `C:/Program Files/nodejs` under the local lock (`D:/sys_lock.bat`/`sys_lock.sh run`).
- State what was not checked live. Commit with `## AI Context` naming the root
  cause; temp files only under D:/Temp/<purpose>/, deleted when done.

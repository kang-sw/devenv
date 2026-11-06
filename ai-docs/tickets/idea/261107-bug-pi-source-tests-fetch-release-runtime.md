---
title: "Pi source-tree tests fetch the release ws-mcp, so a runtime.json tool addition breaks real-child tests"
---

## Observation (2026-11-07, dogfood)

After `261106-feat-ws-dashboard-browser-interaction` merged (it added
`dashboard.inbox_drain` to `agents-plugin-pi/runtime.json`), `node --test`
in `agents-plugin-pi/` on develop went from 1 failure to 11. The 10 new
failures are all real-child spawn tests (agent-channel, web-startup, nested
hop, execute-worker decisions), each failing with
`ws-mcp-launcher: incompatible ws-mcp runtime after repair`.

- The source-tree Pi package has no `.local-devenv-runtime` marker (that path
  needs the plugin under a cache package), so the launcher does not build from
  source. When the contract changes, the stamp misses and the launcher installs
  the `v0.48.10` release asset, which lacks the tools develop's `runtime.json`
  lists (`dashboard.intake`, `mailbox.history`, `dashboard.inbox_drain`).
- A plain `go build` of develop does not help: the Pi bridge refuses
  `serverInfo.version` `0.48.10-dev` against `plugin_version` `0.48.10`.
- Workaround that yields the true result (2541 pass, only the known stale
  65-tool contract check failing): build develop with
  `-ldflags "-X main.version=0.48.10"` into a scratch dir as
  `ws-mcp-0.48.10-<hash>` (the name the launcher picks) and run with
  `WS_MCP_RUNTIME_DIR=<scratch dir>`.

## Question

Between releases, any tool added to `runtime.json` makes the source-tree Pi
suite fail for reasons unrelated to the change. Options: let the source-tree
launcher build from source under test (a test-only local-devenv seam), have the
Pi test harness build and inject a matching runtime, or accept a dev-suffixed
server version when the contract tools match. Also note the stale
`captured tool set exactly matches the bundled 65-tool contract` check (now 70).

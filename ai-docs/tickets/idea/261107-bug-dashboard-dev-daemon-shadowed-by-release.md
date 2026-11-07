---
title: "A same-core release dashboard daemon shadows a local dev build, and a reinstalled Claude session keeps its old serve"
---

## Observation (2026-11-07, dogfood)

After `./install.sh update`, `/reload-plugins`, and `/mcp reconnect all`, the
running dashboard daemon was still the previous day's binary
(`agents-plugin-pi/.runtime/.../ws-mcp-0.48.10-46b67f88402c`, version
`0.48.10`, the release asset the Pi launcher fetched). The freshly installed
local build reports `0.48.10-dev`, and `CompareVersions` orders a pre-release
below its release core, so `Ensure` kept the old daemon as "newer". The daemon
has no idle exit, so it would have served stale page code indefinitely. Getting
the new code required `POST /shutdown` plus starting `ws-mcp dashboard` by hand.

Separately, the Claude session's plugin MCP `serve` (pid started at session
launch) survived `/reload-plugins` and `/mcp reconnect all`: the mod-spawned
`mailbox wait` used the new binary, but every `ws/*` tool still ran on the old
serve (`claude.dashboard_enabled` read as unset; no `dashboard.inbox_drain`).
Only a full Claude restart picks the new serve up.

## Question

- Should a dev build replace a release daemon of the same core when the
  build differs (e.g. compare a build id in `/health`), or is the documented
  manual shutdown enough for maintainers? Downstream release-to-release
  upgrades already work; this mainly bites source-tree dogfooding, and is
  compounded by the Pi source tree fetching the release runtime
  (`261107-bug-pi-source-tests-fetch-release-runtime`).
- Whether the install/reload path should tell the user a session restart is
  needed for the MCP serve to change.

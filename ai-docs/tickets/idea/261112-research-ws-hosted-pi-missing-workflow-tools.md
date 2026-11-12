---
title: "Hosted Pi dogfood: workflow tools absent from the lead's active surface"
related:
  261110-epic-ws-dashboard-agent-host: hosted Pi lead dogfood
---

# Hosted Pi workflow tools absent during dogfood

## Observation

On 2026-11-12, the user opened a dashboard-hosted Pi lead and requested
`lead-discuss` plus a whiteboard update. The lead received only the native
read/bash/edit/write tools and no ws bridge or delegation tools. It could
update the board only by starting the bundled MCP launcher from bash and
calling `whiteboard.query` / `whiteboard.update` over stdio with the existing
hosted key. No new session key was minted.

The session registration and agent record agree on `harness: pi`,
`profile: lead-pi`, `hosted: true`, and `host_pid: 242829` (reply ID
`theaterbotchsubatomicpolar`). These are runtime evidence, not portable
reproduction constants.

## Investigation

`agents-plugin-tool/internal/wsagent/pi.go` `piLeadArgs` loads ambient Pi
extensions rather than explicitly adding the ws Pi adapter. This machine's
`~/.pi/agent/settings.json` lists a local `pi-agent-dev` package and
`npm:pi-openai-usage`; whether that local package includes or loads the ws
adapter was not established. Missing installation, resource discovery, or
adapter startup are hypotheses, not a diagnosed runner defect.

Compare ordinary terminal and dashboard-hosted Pi startup on the same
machine, inspect extension diagnostics and effective active tools, and decide
whether this is a setup/documentation gap or a missing hosted preflight.
The fallback restores board access but does not validate ws bootstrap,
delegation, hosted-mode suppression of duplicate input/feed, or compaction
revival.

No behavior or compatibility-surface change is authorized by this capture.

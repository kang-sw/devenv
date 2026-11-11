---
title: "Launcher never replaces a running -dev daemon after a reinstall of the same release version"
---

# Launcher never replaces a running -dev daemon after a reinstall of the same release version

## Observation (2026-11-11)

The user reinstalled ws into Claude from develop (a local-devenv build) and
reconnected the MCP server. The new runtime binary
(`.runtime/linux-amd64/ws-mcp-0.48.17-02f63d4d0a82`) has a different content
hash and a newer mtime than the running dashboard daemon. The daemon reports
`version 0.48.17-dev`, build_time 2026-11-10 22:25, content_hash
`7e108883...`. The launcher still connected the new MCP session to the old
daemon, so none of the newly merged dashboard routes (holders, fs/cap,
terminal) were served.

## Cause (confirmed by reading the installed launcher)

- `DaemonLink.needs_start` compares the daemon's reported version with
  `runtime_version`, which comes from `runtime.json` `plugin_version`. That
  value is `0.48.17`, with no suffix.
- A local-devenv binary reports `0.48.17-dev`.
- `compare_daemon_versions` mirrors `wsdashboard.CompareVersions`, where a
  pre-release suffix sorts after none.
  `compare_daemon_versions("0.48.17-dev", "0.48.17")` returns 1, so the
  running daemon reads as newer than the plugin and `needs_start` returns
  False.
- The build-time and content-hash tie-break that would detect the new build
  is never reached.

The fault is in the launcher's version source, not in the session-holder
change (e5720ffda only passed the interpreter env).

## Directions (not decided)

- Compare against the runtime binary's own version (the staged binary's
  `version` output, or a value recorded at install time) instead of the
  contract's `plugin_version`. Then a -dev daemon meets a -dev runtime and
  falls through to the build-time and content-hash tie-break.
- Alternatively, treat versions with an equal core and different suffixes
  as equal for replacement, and let the content-hash tie-break decide. This
  needs a check that a real release install still replaces a -dev daemon.
- Add a launcher test for -dev daemon versus release contract versus newer
  -dev runtime.

## Workaround

`POST /shutdown` on the daemon (endpoint.json token), then reconnect the MCP
server; the next launcher start launches the new binary.

## Constraints

- `agents-plugin/bin/ws-mcp-launcher.py` and its wsflow copy per
  `ai-docs/manuals/wsflow-mirroring.md`; launcher tests run locally with
  `nice -n 19`.

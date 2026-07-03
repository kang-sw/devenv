---
title: Claude plugin cache stuck two versions behind source; MCP refuses to start
related:
  260627-bug-playbook-render-uses-stale-plugin-cache-during-source-dogfood: adjacent stale-cache class, but that case still started the MCP server
---

# Claude plugin cache stuck two versions behind source; MCP refuses to start

## Background

Dogfood surprise on 2026-07-03 while running `/ws:lead-discuss` from `main` in
this repo. `claude mcp list` showed `plugin:ws:ws` as `✘ Failed to connect`, so
no `ws/*` tools were registered in the session at all (not just a stale-render
case like the related ticket).

Root cause chain, confirmed by running the launcher directly with
`WS_MCP_LAUNCHER_DEBUG=1`:

- The installed Claude plugin snapshot at
  `~/.claude/plugins/ws-plugin/ws/runtime.json` was pinned to
  `plugin_version: 0.30.17`, while `main` HEAD (`agents-plugin/.claude-plugin/plugin.json`
  and `agents-plugin-tool`) was already at `0.32.0` (two release bumps ahead:
  `0.30.17 -> 0.31.1 -> 0.32.0`, per `docs(changelog)` commits on `main`).
- The repo-local `.local-devenv-runtime` marker forces the launcher to build
  from `agents-plugin-tool` source instead of downloading a release asset.
- The build succeeds (`exit=0`) but the built binary reports `0.32.0`
  capabilities against the installed plugin's `0.30.17` runtime contract, so
  `runtime_fully_compatible()` rejects it as a version mismatch, and the
  launcher fails with "local devenv runtime was forced but no compatible local
  runtime could be installed".
- `claude plugin marketplace update kang-sw-devenv` and
  `claude plugin update ws@kang-sw-devenv` both reported "already at the
  latest version (0.30.17)" — the CLI's own update path did not detect that
  the local-marketplace source (`/home/swkang/devenv/agents-plugin`) had
  advanced past the cached snapshot version.

## Impact

Any `/ws:*` skill that depends on `ws/playbook.print` or other MCP tools is
unusable from a fresh session until the user manually uninstalls/reinstalls
the plugin (uninstall was blocked by the auto-mode classifier as
self-modification outside task scope, correctly — this needs an explicit user
action). This is worse than the stale-render case in the related ticket: there
the MCP server started and served a stale artifact; here it refuses to start
at all, so the whole skill is blocked, not just degraded.

## Research Direction

- Confirm whether `claude plugin update` for a local-path marketplace source
  is expected to re-diff the source's `plugin.json` version, or whether it
  only compares against a marketplace-manifest-cached version that itself
  needs a separate refresh trigger.
- Decide whether the local-devenv-runtime forced-build path should treat a
  runtime capability mismatch against an out-of-date installed
  `runtime.json` as a signal to also refresh the installed plugin manifest
  (not just the compiled binary), or whether this is purely a Claude Code
  plugin-cache bug to report upstream.
- Consider a dogfood runbook step (or `ws doctor`-style self-check) that
  detects "source version > installed plugin_version" before a session
  starts relying on `ws/*` tools, so the failure surfaces as an actionable
  message instead of a silent MCP connect failure.

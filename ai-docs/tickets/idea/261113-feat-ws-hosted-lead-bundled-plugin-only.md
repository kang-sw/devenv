---
title: "Dashboard-hosted leads use only the daemon-bundled ws/wsflow plugin"
related:
  - 261110-feat-ws-hosted-session-mode-flag
---

# Dashboard-hosted leads use only the daemon-bundled ws/wsflow plugin

## Background

Owner request 2026-11-13: dashboard-hosted sessions (Claude and Pi) should use
the ws/wsflow plugin bundled with the daemon and exclude any user-installed
ws/wsflow copy; the extreme option (hosted: disable all extensions, enable only
ws/wsflow) is acceptable.

## Current behavior (investigation evidence)

- Hosted children are already isolated. Claude: `claudeArgs`
  agents-plugin-tool/internal/wsagent/claude.go:128-187 passes
  `--setting-sources ""` (:157), `--settings` disabling builtin plugins
  (:141,:158), `--strict-mcp-config --mcp-config` (:159),
  `--disable-slash-commands` unless profile.Skills (:163), `--plugin-dir` only
  for profile.Plugins (:166); `verifyInit` (:300-310) refuses launch if any
  plugin outside the profile loads. Pi child (wsagent/pi.go:241-276):
  `--no-extensions` then `--extension builtin:mcp`, the generated MCP bridge
  (piMCPServers :198-220), profile plugins.
- Hosted leads are not isolated. `claudeLeadArgs` (claude.go:97-125)
  deliberately loads user/project settings, plugins, hooks, MCP servers; env
  scrub keeps CLAUDE_CONFIG_DIR (:59-95). On the owner machine the lead
  therefore loads the marketplace cache copy `wsflow@kang-sw-devenv` 0.49.0,
  not the daemon's bundled package. Pi lead (pi.go:283-297) passes only
  `--mode rpc` and session flags, so `~/.pi/agent/settings.json` packages (the
  devenv git package) and local extensions (orca-*) load.
- No duplicate load observed; no guard exists on the lead path.

## Mechanisms (Claude 2.1.323)

`--plugin-dir`, `--settings <file-or-json>`, `--setting-sources`,
`--strict-mcp-config`, `--disable-slash-commands`. Unverified (probe required):
(1) whether `--settings {"enabledPlugins":{"wsflow@...":false}}` overrides a
user-source true; (2) whether a `--plugin-dir` plugin with the same name as an
installed one loads twice or dedupes; (3) whether Pi `--no-extensions` also
suppresses settings `packages` entries.

## Idea

- Hosted Claude lead: add `--plugin-dir <bundled ws|wsflow>` plus `--settings`
  disabling the installed ws/wsflow entries; fallback if probe (1) fails is the
  child-style flag set.
- Pi lead: exclude the installed ws package, add the bundled one; extreme mode
  `--no-extensions --extension <bundled>`.
- Optional knob for the extreme mode (name TBD, e.g.
  `hosted_isolated_extensions`).
- "Bundled" = the plugin root of the same package (ws or wsflow) as the running
  daemon; repo `agents-plugin*` in dev builds.
- Compatibility tier: T0 (additive optional config key + launch args); note the
  lead-observable behavior change.

## Open questions (board items, pending owner)

- (393) Default isolation scope for hosted leads: replace only ws/wsflow vs
  isolate everything vs knob (default replace-only, knob for extreme).
  Recommendation: knob.
- (394) In extreme mode keep user permissions/hooks/CLAUDE.md memory vs
  `--setting-sources ""`. Recommendation: keep, if probe (1) passes.
- Pending acks: (395) bundled copy always wins on version mismatch; (396) child
  launch path unchanged; (397) probe (1)-(3) before implementing; (398) bundled
  = daemon's own package root.

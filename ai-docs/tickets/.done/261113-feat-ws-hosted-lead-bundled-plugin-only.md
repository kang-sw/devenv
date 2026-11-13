---
title: "Dashboard-hosted leads use only the daemon-bundled ws/wsflow plugin"
completed: 2026-11-13
related:
  - 261110-feat-ws-hosted-session-mode-flag
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0d14d9ba63b16c6d
sage-review-completeness-reviewed: 0d14d9ba63b16c6d
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
  for profile plugins (:166); `checkClaudeInit` refuses plugins outside the
  profile except builtins (agents-plugin-tool/internal/wsagent/claude.go#L255-L260, agents-plugin-tool/internal/wsagent/claude.go#L298-L309). Pi child (wsagent/pi.go:241-276):
  `--no-extensions` then `--extension builtin:mcp`, the generated MCP bridge
  (piMCPServers :198-220), profile plugins.
- Hosted leads are not isolated. `claudeLeadArgs` (claude.go:97-125)
  deliberately loads user/project settings, plugins, hooks, MCP servers; env
  scrub keeps CLAUDE_CONFIG_DIR (:59-95). The lead therefore loads the
  installed `wsflow@kang-sw-devenv` copy, not the daemon's bundled package (on
  the owner machine the local marketplace already resolves to the repo path,
  0.49.1; see Probe Results). Pi lead (pi.go:283-297) passes `--mode rpc`,
  session flags, and optional model and thinking flags, so `~/.pi/agent/settings.json` packages (the devenv git
  package) and local extensions (orca-*) load.
- No duplicate load observed; no guard exists on the lead path.

## Mechanisms (Claude 2.1.323)

`--plugin-dir`, `--settings <file-or-json>`, `--setting-sources`,
`--strict-mcp-config`, `--disable-slash-commands`. Probes (1)-(3): whether
`--settings` `enabledPlugins` false overrides a user-source true; whether a
same-name `--plugin-dir` plugin dedupes; whether Pi `--no-extensions` also
suppresses settings `packages` entries. Results below.

## Probe Results (2026-11-13, Claude Code 2.1.323, Pi 1.5.0)

Claude runs (`claude -p --output-format stream-json --verbose`, plugin list from
the init event):

- R0 baseline loaded clangd-lsp, `wsflow@kang-sw-devenv` (path
  D:/Workspace/Repos/devenv/agents-plugin-wsflow, 0.49.1 - on the owner machine
  the local marketplace already resolves to the repo path, correcting the
  earlier "cache 0.49.0" claim), and 4 builtin cc-plugin-*.
- R1 `--settings {"enabledPlugins":{"wsflow@kang-sw-devenv":false}}` removed
  wsflow and kept the others: probe (1) PASS (CLI settings override a
  user-source true).
- R2 R1 + `--plugin-dir <repo wsflow>` loaded exactly one wsflow, as
  `wsflow@inline`.
- R3 baseline + `--plugin-dir` (no disable) showed one wsflow `@inline`
  (dedupe), but both MCP names `plugin:wsflow:wsflow` and `wsflow` appeared.
  Same path on this machine, so precedence between two different paths is
  untested: probe (2) inconclusive; avoided by always disabling installed IDs
  first.
- R4 every enabled ID false + `--plugin-dir` -> only `wsflow@inline`;
  `permissionMode` still came from user settings and the SessionStart hook ran,
  so settings sources stay active. claude.ai connector MCP servers remained
  (plugin isolation does not isolate MCP servers).

Pi probe (3), by source inspection of pi-coding-agent 1.5.0: `--no-extensions`
keeps only CLI `--extension` sources (dist/core/resource-loader.js:424-427),
excluding settings-package extensions and auto-discovered
`~/.pi/agent/extensions` and project `.pi/extensions`
(dist/core/package-manager.js:2149-2151, 2168-2169); argument order is
irrelevant (dist/cli/args.js:20-35,155-162); packages are still resolved and may
be installed/refreshed (package-manager.js:731-745, 1053-1134). PASS, not
empirically load-tested.

## Decisions

Compatibility tier: T0 (additive per-session field and optional config key;
launch args only; lead-observable behavior change noted). Owner answers
2026-11-13 and probe-derived lead decisions.

- Per-session isolation level `default|isolated`, chosen in the dashboard
  session-spawn modal (owner (392) = c). Default `default` = replace only
  ws/wsflow with the bundled copy, keep other user plugins/extensions;
  `isolated` = disable all user plugins/extensions, enable only bundled
  ws/wsflow. An optional config key may supply the modal's default; the modal
  choice is the authority. Rejected: replace-only with no choice;
  always-isolated.
- Isolated mode keeps user/project settings sources (permissions, hooks,
  CLAUDE.md memory); only plugins are disabled ((393) = a; probe (1) passed, so
  no fallback is needed). Rejected: `--setting-sources ""`.
- The bundled copy always wins on version mismatch (394).
- The child launch path is unchanged (395).
- "Bundled" = plugin root of the daemon's own package (ws or wsflow); repo
  `agents-plugin*` in dev builds (397).
- Claude default mode: keep setting sources; `--settings` with `enabledPlugins`
  false for every enabled plugin ID whose plugin name is `ws` or `wsflow`
  (discover IDs from the effective user/project settings, do not hardcode a
  marketplace name) plus `--plugin-dir <bundled package root>`. Rejected:
  `--plugin-dir` alone (duplicate-name precedence untested, R3 shows a
  duplicate MCP name).
- Claude isolated mode: `enabledPlugins` false for every enabled plugin ID
  including builtins, plus `--plugin-dir <bundled>`; user MCP servers (e.g.
  claude.ai connectors) and settings sources stay, consistent with (393) = a.
  Rejected: `--strict-mcp-config` in isolated mode (owner scoped isolation to
  extensions/plugins).
- Pi isolated mode: `--no-extensions --extension <bundled Pi extension entry>`.
- Pi default mode: `--no-extensions --extension <bundled>` plus each non-ws
  extension re-added explicitly (scope per the Pi default-mode scope decision
  below). Rejected: plain launch (loads the installed ws package); relying on a
  Pi per-package filter (none known).
- Pi default-mode scope (auto-confirmed, raised by fact population): restore
  every non-ws extension the plain launch would load: user
  `~/.pi/agent/extensions` entries, settings `packages` extension entries, and
  project `.pi/extensions` entries of the session cwd. Skip only the ws/wsflow
  Pi package. Rejected: restoring user-level extensions only (would silently
  drop project extensions in default mode).
- Bundled Pi extension entry (auto-confirmed, raised by fact population): the
  `agents-plugin-pi` package beside the daemon's own plugin package root (same
  checkout/release; repo sibling `agents-plugin-pi/` in dev builds). Resolve it
  relative to the bundled package root and use the extension entry declared in
  its manifest (e.g. `src/index.ts`). If it is not found, launch the Pi lead
  with the child-style ws MCP bridge (`--extension builtin:mcp` plus the
  generated bridge, as `piMCPServers` does in wsagent/pi.go) and record a
  warning on the session. Rejected: requiring a separately installed Pi package
  (defeats the bundled-only goal).
- Per-session field: the session spawn request carries an isolation level
  `default|isolated` set by the dashboard session-spawn modal (UI owned by the
  UI-track session); an optional config key may supply the modal's default.

## Constraints

- Read before editing: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md` (T0), and
  `ai-docs/manuals/ws-mcp.md` if `agents-plugin-tool/internal/mcp/` is touched.
- Child launch path (`claudeArgs`, `checkClaudeInit`, Pi child) unchanged (agents-plugin-tool/internal/wsagent/claude.go#L128-L130, agents-plugin-tool/internal/wsagent/claude.go#L255-L260).
- Spawn-modal UI is out of scope (UI track); this ticket adds the backend
  plumbing and the per-session field.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- unavailable: wsflow/rationale.query is not exposed in this session; prior-decision search must be completed before routing.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/claude.go, pi.go, http.go, runner.go, record.go, backend.go |
| scope.surface | public-interface | lead spawn request and persisted session field change; backend launch behavior is observable |
| scope.new_public_symbol | no | no new exported symbol specified; request adds an isolation field |
| scope.new_type_contract | yes | lead spawn request and persisted record gain default or isolated isolation level |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/claude_test.go, pi_test.go, lead_http_test.go, lead_recovery_test.go |
| complexity.reuse_points | confirmed | wsrsrc.ResolveRoot and agentMCPCommand derive package resource and launcher paths; the bundled Pi extension entry is resolved beside the same package root (agents-plugin-pi, fallback to builtin:mcp bridge) |
| complexity.side_effect_risk | high | changes plugin loading and persistence for hosted Claude and Pi leads |
| risk.correctness | high | installed-plugin enumeration and Pi extension restoration must preserve other extensions and resume behavior |
| risk.fit | high | Pi distribution and discovery must match daemon packaging and existing hosted-session contracts |
| risk.test | high | two harnesses and live plugin loading require coverage beyond argv tests |
| risk.security_or_contract | high | user plugin isolation and a persisted spawn request field affect trust and compatibility |

## Phases

### Phase 1: hosted-lead launch args + per-session isolation field

Code sites (agents-plugin-tool/internal/wsagent/):
- `claude.go` `claudeLeadArgs` (:97-125): add `--settings` disabling installed
  ws/wsflow (all IDs in isolated mode) and `--plugin-dir <bundled>`.
- `pi.go` `piLeadArgs` (:283-297): add `--no-extensions --extension <bundled>`
  where `<bundled>` is the `agents-plugin-pi` manifest entry resolved beside the
  bundled package root; if missing, switch to the `--extension builtin:mcp` +
  `piMCPServers` bridge and record a session warning. In default mode, also
  re-add Pi's full resolved, enabled extension loadout minus ws: user
  `~/.pi/agent/extensions`, settings `packages` and explicit `extensions`
  entries, enabled builtins, and the session cwd `.pi/extensions` (per Pi's
  package-manager.js and resource-loader.js), preserving Pi's trust handling,
  ordering, and deduplication.
- Field: `leadRequest` (`http.go:87`; the leads route at `http.go:157-162` calls
  `CreateLead`) -> `LeadOptions` (`runner.go:496`) -> `CreateLead`
  (`runner.go:514`) -> `Record` (`record.go:27`, next to `PermissionMode` :57,
  set at `runner.go:584-585`) -> `LeadSpec` (`backend.go:69`, built at
  `runner.go:2051`). Default `default`; persist so a resumed session relaunches
  in the same mode.
- Bundled package root: `wsrsrc.ResolveRoot` (`internal/wsrsrc/loader.go:30`,
  `WS_RSRC_ROOT`, else exe-relative `../rsrc`); the package root is its parent,
  as `cmd/ws-mcp/agent_mcp.go:20-41` derives the launcher path.

Verification:
- Unit tests for argv in both modes and both backends: installed ws/wsflow IDs
  disabled, bundled plugin-dir present, non-ws user Pi extensions re-added in
  default mode; field default `default`.
- The isolation level persists on the session record; a resumed or relaunched
  session starts in the same isolation mode (default or isolated) it was
  created with.
- Unit test: Pi default-mode argv includes a project `.pi/extensions` entry of
  the session cwd.
- Unit test: when the bundled `agents-plugin-pi` entry is missing, the Pi lead
  argv falls back to `--extension builtin:mcp` with the generated bridge, and
  the session records a warning.
- One live check: a hosted Claude lead's init event shows only one wsflow, from
  the bundled path.

### Result (52dc582b4) - 2026-11-13

Implemented in `52dc582b4`, with independent-review fixes in `f7b2d3af7`.

- Added HTTP `plugin_isolation` (`default|isolated`, omission means `default`),
  propagated through LeadOptions, persisted Record and response State, and
  LeadSpec. Legacy records and resumed/relaunched sessions retain the correct
  mode. Spawn-modal UI remains owned by the separate UI track; no optional
  configuration policy was added.
- Claude disables effective installed ws/wsflow IDs in default mode, or all
  enabled IDs including builtins in isolated mode, then loads the daemon's
  bundled package. Settings sources, hooks, permissions, memory and user MCP
  remain active. Its POSIX canonical local-settings store is included.
- Pi loads the sibling package's manifest entries. Default mode re-adds enabled
  non-ws resources through Pi's native in-process resolver rather than a
  preflight enumeration: this preserves trust hooks, package filtering,
  ordering, deduplication and reload without initializing extensions twice.
  The adapter uses the SDK graph adjacent to the actual CLI. Default mode
  therefore requires that JavaScript SDK; standalone binaries can use isolated
  mode. Missing bundled entries use the native MCP bridge, with hosted key
  context and a persisted/exposed `plugin_warning`.
- Child launch paths are unchanged. Existing Claude/Pi lead holders reattach
  without re-resolving launch-time plugin resources.
- Independent R1 found the Pi bundled/unbundled module-graph mismatch and the
  Claude reattachment dependency on launch-time plugin resources. Both were
  fixed with regressions; fixes-only R2 confirmed both resolved, no Forwards.

Verification:

- Final w5090 full `go test ./... -count=1`: every package passed except the
  owner-excluded `TestHostedConnectionSeeingALANChangeReplacesTheDaemon`
  predecessor-exit timeout. `internal/wsagent` and `internal/wsholder` passed.
  No new failures; the known exclusion was not changed or suppressed in code.
- Local Node-only adapter/bridge checks: 11 passed, zero failed or skipped,
  against installed Pi 1.5.0. Actual npm-bin startup restored user, explicit,
  package and trusted project extensions while excluding installed ws. An
  old-SDK-graph negative control reproduced the R1 failure.
- `git diff --check`: clean. Temporary local fixtures/logs were removed.
- Forward: the live hosted Claude init-event check remains unrun because the
  permitted remote verification host has no Claude/Pi executable. Per lead
  steering, the full check with its known exclusion closes this phase.
- Workflow routing/acquire/close APIs were not exposed in this session; the
  supplied branch/ticket were retained, and closure used the documented Git
  move fallback. No merge or push was performed.

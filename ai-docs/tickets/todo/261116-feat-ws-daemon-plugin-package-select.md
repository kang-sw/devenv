---
title: "Select the hosted-lead plugin package (ws or wsflow) with an explicit daemon setting"
related:
  261116-feat-ws-dashboard-config-settings-ui: "a later dashboard editor for the setting; its G6 covers per-package manifest defaults"
  261112-feat-ws-mixed-version-compat-guards: "same shared-daemon starter/replacement mechanics; tier vocabulary"
---

# Select the hosted-lead plugin package (ws or wsflow) with an explicit daemon setting

## Background

Owner report (2026-11-16, verbatim):

> 22:56 이거 그냥 데몬 설정으로 플러그인 ws 쓸지 / wsflow 쓸지 설정하게 해줄 수 있나요?
>
> 22:56 보니까 wsflow 쓰게 되면서 다 어그러진거 가틍ㄴ데.

Gist: can the daemon get a setting that chooses whether the ws or the wsflow
plugin package is used? Things seem to have gone wrong since wsflow came into use.

Owner answer at 23:04, when asked how to clean this up (the owner chose
"ticket only", with no global-config workaround in the meantime):

> 일단 이렇게 동전 뒤집기 방식으로 동작하는 걸 고치는 게 시급해 보입니다 ...

Gist: today the package that hosted lead sessions get is effectively a coin
flip, and fixing that is urgent.

### How the package is chosen today (verified at drafting; line numbers drift)

- **One daemon serves every session.** Both packages ship the same launcher
  byte for byte (`agents-plugin/bin/ws-mcp-launcher.py` and the wsflow copy
  are identical). The package difference is only the env each package's MCP
  server entry passes: wsflow sets `WS_MCP_NO_AGENT=1`, `WS_MCP_NAMESPACE=wsflow`,
  `WS_MCP_CLI_NAME`, and its own `WS_MCP_CONFIG_MANIFESTS`
  (`agents-plugin-wsflow/.claude-plugin/plugin.json`, `agents-plugin-wsflow/.mcp.json`).
  ws passes only its manifests (`agents-plugin/.mcp.json`).
- **The daemon's package is its starter's.** The launcher sets `WS_RSRC_ROOT`
  to its own package's `rsrc/` (`apply_rsrc_root_env`) and starts the daemon
  with that env (`start_daemon` -> `env = daemon_start_env(self.env, ...)`,
  launcher ~1627-1658). The Pi package's start path picks its owner the same
  way, from its own `runtime.json` `plugin` field
  (`pi_daemon_workflow_root`, launcher ~1223-1240; `daemon_start_env` ~1342-1369).
  The desktop app starts the daemon from the one package bundled into it, or
  from a development root (`agents-plugin-tool/desktop-electron/daemon-selection.js`,
  `selectDaemon`). This repository's own `scripts/dev.py rerun daemon`
  hard-codes the wsflow package (`DAEMON_PACKAGE`, introduced by `fbd1cd6a9`),
  which matches the owner's "since wsflow came into use" observation.
- **Hosted leads take the daemon's package.** `bundledPluginRoot`
  (`agents-plugin-tool/internal/wsagent/lead_plugins.go:16-34`) is
  `dir(wsrsrc.ResolveRoot())`, that is, `dir(WS_RSRC_ROOT)` of the daemon
  process. `claudeLeadPluginArgs` (`claude_lead_plugins.go:25-49`) passes it as
  `--plugin-dir` and disables any installed `ws`/`wsflow` plugin entry. Pi
  leads use the same root (`pi_lead.go:19`, `piBundledEntries` ~130). A hosted
  lead drops the daemon's `WS_MCP_*`/`WS_RSRC*` env (`leadDropEnv`,
  `claude.go:82`), so its namespace, skill prefix, no-agent profile and config
  defaults all come from that one `--plugin-dir`. Daemon-spawned conversations
  attach the ws MCP through `dir(WS_RSRC_ROOT)/bin/ws-mcp-launcher.py`
  (`agents-plugin-tool/cmd/ws-mcp/agent_mcp.go:19-42`).
- **Nothing chooses it.** No config key, daemon flag, `/health` field
  (`HealthInfo`, `wsdashboard/daemon.go:298-315`), session record field
  (`Record`, `wsagent/record.go`, which has only `plugin_isolation` and
  `plugin_warning`), or new-session form field selects or reports the package.
- **When it changes.** The first launcher to start the daemon on a cold cache
  wins. A launcher replaces a running daemon only when the daemon has an older
  numeric core (`needs_start`, launcher ~1618-1625; `keepRunning`,
  `wsdashboard/ensure.go:213-220`), when LAN mode differs, or on an explicit
  `daemon-start --replace` (dev rebuild). Each replacement re-picks the
  package from the replacing starter. A hosted lead reads the package at every
  process start (`claudeLeadArgs` -> `claudeLeadPluginArgs`, `claude.go:103`).
  A process that kept running under its holder is reattached unchanged
  (`conv.reattach`, `wsagent/runner.go` ~2510). So a resumed or relaunched
  conversation can switch namespace (`ws/...` vs `wsflow/...`) and defaults
  in the middle of its history.
- **Sessions the daemon did not launch are unaffected.** A plugin-started
  Claude/Codex session, a standalone Pi session, and an MCP-only caller send
  their own env in the hello (`proxy_hello`, launcher ~1371-1390). The
  daemon builds each connection's server from that env: manifests at
  `process_env.go:142`, rsrc root at ~228-233, no-agent flag at ~363. The Pi
  tool gate reads the knob from the connection's own manifest (`piKnobOn`,
  `pi_tools.go:135-146`).

### What differs between the packages

- Config defaults (same keys, only `default` differs): `dashboard.pi_spawn_enabled`
  true -> false; `claude.compaction_advisory_enabled` true -> false;
  `claude.mailbox_drain` direct -> notice; `claude.mcp_watchdog_enabled`
  true -> false; `claude.status_band_enabled` true -> false (ws -> wsflow,
  from each package's `config-manifest.json` / `dashboard-config-manifest.json`).
- Not config-reachable: MCP server and tool names (`ws/...` vs `wsflow/...`),
  the skill prefix (`ws:` vs `wsflow:`), the `wsflow-cli` name, and the
  no-agent profile, which hides some tools and config knobs
  (`noAgentHiddenTool`, `mcp/server.go` ~5853).

### Goal

The hosted-lead package becomes deterministic and explicit. The owner can set
it as a daemon setting (ws or wsflow), and it stays the same across daemon
restarts and replacements, whichever launcher (ws, wsflow, Pi, desktop app,
dev rebuild) started the daemon.

## Decisions

- The owner wants a daemon setting that chooses ws vs wsflow for hosted
  sessions (owner, 22:56).
- The fix is urgent (owner, 23:04).
- There is no global-config workaround in the meantime. The owner chose
  "ticket only" over copying ws values into the global config (owner, 23:04).

Owner answers to the decision queue (whiteboard, 2026-11-16 23:21 KST):

- **(1209)=a:** The package selection is one global-scope config key stored in
  the global `config.json` overrides, written through `config.tune`, and read
  by the daemon at each hosted process start. The daemon has no settings store
  of its own, and global config is already the daemon-wide settings source
  (`dashboard.*` keys resolve from project/global/built-in values;
  `DashboardSettings`, `mcp/dashboard_config.go:54`). A dashboard editor can
  later write the same key through `261116-feat-ws-dashboard-config-settings-ui`.
  Rejected: a daemon CLI flag (every starter - ws, wsflow, Pi, desktop app,
  dev script - would have to pass it, which keeps the decision with the
  starter: the bug itself); the desktop app's own selection file
  (`daemon-selection.json`; desktop-app-only and lost when a plugin launcher
  starts the daemon); dashboard settings storage only (no daemon config route
  exists yet).
- **(1210)=c:** When the key is unset, hosted leads use the fixed default
  `wsflow`, not the package that started the daemon. The owner chose `wsflow`
  over the lead's recommended `ws`. A ws-only install (for example a Claude
  install that has only the `ws` package) keeps working without setting
  anything through the fallback in (1214). Rejected: fixed default `ws` (the
  lead's recommendation); unset keeps the starter's package (safest for
  single-package installs, but keeps the coin flip for anyone with both
  packages installed unless the owner sets the key).
- **(1211)=a:** A changed setting applies at each hosted process's next start
  (new sessions and relaunched/resumed ones). Running and holder-reattached
  processes keep their package, and no daemon restart is needed. Package args
  are already computed per process start (`claude.go:103`), so this needs no
  new lifecycle; the mid-conversation switch becomes owner-initiated instead
  of random. Rejected: pinning the package per session at creation in the
  session record (keeps a conversation's namespace stable across a flip, but
  needs a new `Record` field; `writeRecord` serializes the struct whole,
  `wsagent/record.go:295-303`, so an older daemon would drop the field -
  scenario-sheet row 10 shape); reading the setting only at daemon start (a
  flip would need a restart).
- **(1212)=a:** The setting governs only daemon-launched processes: hosted
  Claude and Pi leads, and the ws MCP the daemon attaches to conversations it
  spawns. Plugin-started Claude/Codex sessions, standalone Pi sessions and
  MCP-only callers keep their own package and defaults; they load their tools
  from their own plugin and send their own env in the hello (`proxy_hello`;
  `process_env.go:142`), and the daemon cannot swap their skills or tool
  namespace. Any display-only mismatch note is outside this ticket. Rejected:
  refusing or warning a non-hosted connection whose package differs from the
  setting (a new hello-time policy that risks a T3-like refusal for no
  data-safety reason).
- **(1213)=a:** The setting selects the whole plugin package: plugin dir, MCP
  namespace, skill prefix, CLI name, no-agent profile and config defaults. The
  owner asked to choose "plugin ws / wsflow", and several differences are not
  config-reachable (see Background). Rejected: keeping the daemon's package and
  letting the setting pick only which manifest defaults apply (tool names,
  skill prefix and no-agent hiding would still follow the starter, so part of
  the coin flip stays).
- **(1214)=a:** When the selected package's root is not available to the
  daemon, the hosted process starts with the daemon's own package and a
  visible `plugin_warning` naming the fallback is persisted. `Record.PluginWarning`
  and the Pi fallback warning path already exist (`pi_lead.go`,
  `PluginWarning`). Under the `wsflow` default from (1210), this covers a
  ws-only install. Rejected: refusing the launch with an actionable error
  (stricter, but breaks single-package installs under the fixed default).
- **(1215)=ok:** The daemon learns package roots from connecting hellos and
  persists the last-seen root per package name in daemon state. Every
  launcher hello already carries its `WS_RSRC_ROOT` (`proxy_hello` env;
  `process_env.go` ~228-233), and `validateBundledPlugin`
  (`lead_plugins.go:36-51`) already checks the manifest name. Rejected:
  scanning host plugin caches (host cache layouts are harness-specific and
  would put host paths into shipped Go).
- **(1216)=ok:** All three daemon package consumers (`claudeLeadPluginArgs`,
  `piBundledPluginRoot`, `agentMCPCommand`) route through one setting-aware
  resolver. Rejected: patching each site separately (a hosted lead and the
  conversations it spawns could get different packages).
- **(1217)=ok:** The key is a ws-mcp built-in key outside the `dashboard.`
  namespace, visible on the no-agent surface. A built-in key cannot share the
  manifest-owned `dashboard.` namespace (`parseConfigManifest`
  namespace-collision check, `mcp/config_manifest.go` ~172-200). Rejected:
  adding it to both packages' `dashboard-config-manifest.json` (a manifest
  key's default is per package by construction, so the default itself would
  reintroduce the starter dependency).
- **(1218)=ok:** Compatibility tier T0. Old writers keep the new override:
  global `Overrides` is `map[string]string` (`wsconfig/config.go:34-40`), and
  `setOverrideInFileRMW` read-modify-writes that map
  (`wsconfig/resolver.go:331-346`). An old daemon never reads the key and
  keeps today's starter behavior. A new daemon reading a config without the
  key resolves the default. No hello, protocol or `Record` shape changes (the
  per-session pin was rejected in (1211), because `writeRecord` drops unknown
  fields). An optional `/health` package field, if added, follows
  `HealthInfo`'s existing omitted-field pattern (`wsdashboard/daemon.go:298-315`).
  The persisted root registry from (1215) is a new daemon-owned file that old
  binaries never read. Rejected: any compatibility boundary (T3 or a
  minor-bump gate); nothing in the change requires one.

## Constraints

- Read before editing, per AGENTS.md `### Implementation Conventions`:
  `ai-docs/manuals/shipped-surface-boundary.md` (all `agents-plugin*/` paths),
  `ai-docs/manuals/release-compatibility.md` (`agents-plugin-tool/internal/`),
  `ai-docs/manuals/ws-mcp.md` (`agents-plugin-tool/internal/mcp/`, for the key),
  and `ai-docs/manuals/wsflow-mirroring.md` plus
  `ai-docs/manuals/skill-authoring.md` if any manifest, launcher, rsrc or
  skill text changes.
- Compatibility tier: the change touches config keys, shared daemon state and
  possibly `/health`. The tier is settled as T0 with cited read/write sites in
  (1218) (AGENTS.md `### Compatibility Tiers`). An implementation that changes
  any of those read/write sites re-grades it before release; a T3 boundary
  needs a minor bump and explicit owner approval.
- No functionality removal: the `ws` and `wsflow` packages, `plugin_isolation`
  modes, the Pi bundled-extension fallback, and non-hosted sessions'
  own-package behavior all stay.
- Shipped Go and text must not name this repository's layout or the dev
  script (Architecture Rule 4). Package roots come from runtime evidence
  (hellos, `WS_RSRC_ROOT`), never from source-tree paths (see `a3fb49a5b`,
  which removed such a fallback).
- Design for the downstream project, including single-package installs. This
  repository's single-maintainer release cadence and its `scripts/dev.py`
  wsflow starter are not design constraints. `scripts/dev.py` may be adjusted
  repo-locally if dogfooding needs it.
- The desktop app is one of the daemon starters above. A separate Electron
  daemon-launch work item is being settled for the same release (owner
  comment on (1214), 2026-11-16); it is not decided in this ticket.

## Phases

### Phase 1: Setting-driven hosted package resolution

Implements (1209)-(1218):

- Declare a built-in key (name chosen by the implementer, outside
  `dashboard.`) in ws-mcp, visible on the no-agent surface, with values `ws`
  and `wsflow` and the fixed default `wsflow`. It is set at global scope
  through `config.tune` and stored in the global `config.json` overrides.
- Persist a per-package root registry in a new daemon-owned state file, fed by
  the `WS_RSRC_ROOT` each connecting hello carries. Record a root under a
  package name only after `validateBundledPlugin` confirms the manifest name.
- Add one setting-aware resolver. At each hosted process start it reads the
  key and returns the selected package's registered root. When that root is
  not available, it returns the daemon's own package (`dir(WS_RSRC_ROOT)`)
  plus a warning that names the fallback.
- Route `claudeLeadPluginArgs`, `piBundledPluginRoot` (Pi bundled entries) and
  `agentMCPCommand` through the resolver, and persist the fallback warning in
  the session's existing `plugin_warning`. The launch is never refused for a
  missing package root.
- Leave running and holder-reattached processes alone; a setting change takes
  effect at their next process start.
- Leave non-hosted connections unchanged: hello-built servers keep their own
  manifests, namespace and no-agent flag.
- Add no `Record` field and no hello or protocol change. An optional `/health`
  package field, if added, uses `HealthInfo`'s omitted-field pattern.
- Update the `bundledPluginRoot` comment so it no longer claims "the daemon's
  package".

Verification expectations:

- Determinism by starter: with two temporary package roots (`ws`, `wsflow`)
  both registered, start the daemon-side runner with `WS_RSRC_ROOT` at either
  root. With the key set to either value, a hosted Claude lead's args carry
  `--plugin-dir` of the setting's package in both cases. Do the same for Pi
  bundled entries and for the spawned-conversation MCP command.
- Unset key: `wsflow` wins regardless of the starter root, including a daemon
  started from the `ws` root. A global config without the key resolves the
  `wsflow` default.
- Restart/replacement: the root registry and the setting survive a daemon
  restart, and a daemon replaced from the other package's env still resolves
  the setting's package.
- Fallback: when the selected root is not registered (for example only `ws`
  is registered under the unset `wsflow` default), the lead starts from the
  daemon's own package and the persisted `plugin_warning` names the fallback.
  The launch is not refused.
- Registry validation: a hello whose `WS_RSRC_ROOT` manifest name does not
  match is not recorded under that package name.
- Running processes: a holder-reattached lead is not relaunched by a setting
  change. Its next process start uses the new setting.
- Non-hosted callers: a hello-built connection from either package keeps its
  own manifests, namespace and no-agent flag whatever the setting says.
- Key surface: `config.tune` writes the key at global scope; the key is
  listed on the no-agent (wsflow) surface; both packages' manifests still pass
  the `dashboard.` namespace-collision check.
- Regression: the existing `wsagent` hosted-lead tests (which set
  `WS_RSRC_ROOT` in `TestMain`), `pi_tools` gate tests, the launcher Python
  tests and the wsflow package tests pass. `go test ./...` passes under
  `agents-plugin-tool`.

---
title: "P0: Shared daemon Claude plugin root depends on the launcher that starts it"
related:
  261113-feat-ws-hosted-lead-bundled-plugin-only: "preserve bundled-only isolation while correcting package resolution"
  261115-bug-ws-claude-binary-provenance-and-resume-model: "neighboring launcher-environment issue; not the same binary/model-selection defect"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0cd526ff0670053b
sage-review-completeness-reviewed: 0cd526ff0670053b
---

# P0: Shared daemon Claude plugin root depends on the launcher that starts it

## Background

Dogfood finding #12, 2026-11-16, priority P0. The machine-wide daemon's ability
to launch Claude leads depends on which harness launcher won daemon startup.
This also affects downstream mixed-harness installations, not just this checkout.

At approximately 09:10 KST, a Pi launcher using the freshly installed v0.50.1
release runtime in `agents-plugin-pi/.runtime` replaced the older 0.50.0-dev
daemon under the newer-core-wins rule. Every subsequent hosted CLAUDE lead
relaunch failed. The owner's messages to an idle hosted Claude session remained
pending; Pi and Codex sessions were unaffected. Restarting through
`scripts/dev.py` restored Claude launches by pinning the wsflow resource root.
These runtime observations are owner-supplied evidence, not a new reproduction
performed during ticket capture.

Source evidence:

- `agents-plugin-pi/bin/ws-mcp-launcher.py#L840-L850`, `apply_rsrc_root_env`, sets
  `WS_RSRC_ROOT=<agents-plugin-pi>/rsrc` when unset. Startup calls it at
  `agents-plugin-pi/bin/ws-mcp-launcher.py#L1993-L1994` and `agents-plugin-pi/bin/ws-mcp-launcher.py#L2065-L2066`.
- `agents-plugin-pi/bin/ws-mcp-launcher.py#L1458-L1502`,
  `DaemonLink.start_daemon`, copies its inherited environment, adds the holder interpreter when absent, and passes it to `daemon-start`.
- `agents-plugin-tool/internal/wsagent/lead_plugins.go:16-31`,
  `bundledPluginRoot`, derives the plugin package from the parent of the resource
  root; `:36-46`, `validateBundledPlugin`, requires
  `<root>/.claude-plugin/plugin.json`. `agents-plugin-pi` has no such manifest.
- The incident's reported launch failures correspond to the current start failure path at
  `agents-plugin-tool/internal/wsagent/runner.go#L2003-L2011`; the incident itself was not reproduced.
- `scripts/dev.py#L54-L54` and `scripts/dev.py#L196-L202` choose `agents-plugin-wsflow` and explicitly set
  its resource/skills roots, explaining the observed recovery.

## Reproduction

1. In an isolated mixed-harness installation, start an older compatible shared
   daemon, with an idle hosted Claude lead and working Pi/Codex leads.
2. Start a Pi launcher with `WS_RSRC_ROOT` initially unset and a newer release
   runtime, allowing the normal replacement rule to select it.
3. Send a dashboard message to the idle Claude lead or relaunch it. Observe the
   missing `agents-plugin-pi/.claude-plugin/plugin.json` error and pending mail.
4. Compare Pi/Codex operation, then start the daemon with a valid ws/wsflow
   package root and confirm Claude recovery. Do not replace the owner's daemon
   merely to collect this reproduction.

## Decisions

- **Owner (1219)=a, 2026-11-16:** ship a producer-owned, self-contained complete
  workflow bundle for every startup lane. Construct a distinct daemon-start
  subprocess environment using existing `WS_RSRC_ROOT` and `WS_SKILLS_ROOT`;
  preserve the caller's Pi resources and hello environment. Plain Pi retains
  its existing ws identity; prepared native Pi retains the actual ws/wsflow
  owner and defaults. A complete bundle includes the referenced Claude hooks,
  agents, configuration, launcher and workflow resources, not just a manifest.
  Rejected: arbitrary installed/project/marketplace or downstream sibling
  lookup, and a manifest-only shim. A native cache's owner-name marker alone
  does not supply the package bytes.
- **Owner (1220)=a, 2026-11-16:** complete provisioning and correct startup
  context in this patch, retaining existing backend-time refusal and pending
  input handling. Add no global startup or hello refusal. Pre-drain admission
  is deferred to a separately approved compatibility/availability boundary.
  Rejected for this patch: expanding a Claude launch failure into global
  Pi/Codex MCP unavailability or changing mail schemas/retry policy.
- **Owner (1221)=a, 2026-11-16:** cover plain Pi, prepared-native ws and wsflow,
  and the Electron package plan. Preserve binary `content_hash`, shared cache
  and `daemon_dir`, startup lock, newer-core arbitration and holder behavior.
  Rejected: a checkout/dev.py-only repair or silently substituting ws for
  wsflow. Electron already stages a complete supplied workflow package; align
  the package contents and test its consumer boundary rather than adding a
  second lifecycle/admission implementation.
- **Compatibility: T0 per lane, using existing startup seams.** The existing
  writer passes resource/skills environment to `daemon-start`
  (`agents-plugin-pi/bin/ws-mcp-launcher.py`, `DaemonLink.start_daemon`); old
  readers resolve `WS_RSRC_ROOT` and its package parent
  (`agents-plugin-tool/internal/wsrsrc/loader.go`, `ResolveRoot`;
  `agents-plugin-tool/internal/wsagent/lead_plugins.go`, `bundledPluginRoot`;
  `agents-plugin-tool/cmd/ws-mcp/agent_mcp.go`, `agentMCPCommand`).
  - Existing ws/wsflow launchers to the new runtime keep their valid package
    roots, explicit-root precedence, identity and existing refusal semantics.
  - New plain-Pi startup supplies its complete ws bundle through those same
    fields, so both old and new daemon readers understand it; caller Pi
    resources remain unchanged.
  - New prepared-native startup supplies the complete bundle of its actual
    owner through the same fields. Materialization must include bundle inputs
    in freshness/identity checks and preserve older payload compatibility;
    no old helper may erase the only retained representation of new data.
  - Electron supplies its complete staged ws/wsflow package through the same
    existing fields; its independently owned lifecycle guards are unchanged.
  - Old Pi packages retain their existing behavior rather than acquiring a
    new startup/connection refusal. This patch does not claim that a new Go
    binary can invent absent files in an old installation.
  No new tool argument, hello field, persistent session field or compatibility
  boundary is authorized. Record exact old/new writer/reader source lines and
  executable fixture evidence in the implementation Result; escalate any
  concrete design that cannot meet these directional T0 premises.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Preserve bundled-only Claude isolation, both plugin-isolation modes, settings,
  hooks, memory and non-plugin MCP. Preserve native Pi actual-host SDK identity
  and package-only installation without downstream npm/network or repo siblings.
- Invalid explicit roots must not silently fall back to a different package.
  Normalize starter-relative paths in their defining context; no daemon-state
  cwd or machine-specific path may select the product.
- Exec1 owns scripts/dev.py and scripts/test_dev.py (lock/hash confirmation),
  unchanged root selection. Exec3 owns Electron packaging/start/replace and Go
  lifecycle guards (cmd/ws-mcp/proxy.go and internal/wsdashboard ensure/spawn).
  Coordinate any actual overlap before editing; no duplicate admission policy.
- Shared root is read-only; all edits/commits use the canonical full worktree.
  No local Go/test binaries, ws/ws-mcp executables, launcher runs, account-backed
  acceptance or live daemon replacement. Remote gates run on w5090 through
  sys_lock; temporary files stay under D:/Temp/exec2/ and stop below 20 GB free.
- Pi commits have no Co-Authored-By. If agents-plugin-pi/src changes, rebuild
  dist in the same change and verify the freshness guard. Keep file touches
  minimal; do not regenerate dependency closures unless inputs require it.

## Prior Decisions

- 261113-feat-ws-hosted-lead-bundled-plugin-only (2026-11-13, Decisions): "Bundled = plugin root of the daemon's own package (ws or wsflow)" — bearing: constrains
- 261113-bug-ws-release-gate-sweep-fixes (2026-11-13, commit): "the fallback named this repository's layout in shipped Go ... dev builds get WS_RSRC_ROOT from the launcher" — bearing: constrains
- 261113-bug-ws-pi-spawn-key-bound-to-lead-root-not-cwd (2026-11-13, commit): "Pi worker MCP bridges no longer exec the daemon exe ... they start the plugin's Python launcher with WS_MCP_PROJECT_ROOT" — bearing: constrains
- 261018-feat-claude-mod-compaction-threshold-knob (2026-10-21, Result): "Claude Code launches the server from .claude-plugin/plugin.json ... not .mcp.json" — bearing: constrains
- 260513-feat-wsflow-agentless-plugin-scaffold (2026-05-13, Result): "wsflow/ with Codex and Claude manifests, package-local MCP configuration, shared launcher copies" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Pi package and producer, mirrored launchers and native materializers, Electron package consumer |
| scope.surface | cross-module | startup environment connects launcher, native materializer, daemon and Electron |
| scope.new_public_symbol | no | no new public symbol specified; existing environment fields are used |
| scope.new_type_contract | no | no new public or cross-module type contract authorized; internal package layout is implementation detail |
| scope.test_surface | existing | agents-plugin/tests/test_native_pi_payload.py, agents-plugin/tests/test_ws_mcp_launcher_proxy.py, agents-plugin-tool/internal/wsagent/lead_bundled_plugin_test.go |
| complexity.reuse_points | confirmed | DaemonLink.start_daemon copies env; ws-pi-payload.py materialize hashes owner and overlays; ResolveRoot and bundledPluginRoot consume resource root |
| complexity.side_effect_risk | high | daemon startup context changes affect hosted sessions across harnesses |
| risk.correctness | high | package freshness, explicit paths and wsflow identity must remain correct across lanes |
| risk.fit | high | complete self-contained bundles must fit independent package producers without changing lifecycle ownership |
| risk.test | high | copied package-only layouts, version directions and cross-harness failure retention need isolated fixtures |
| risk.security_or_contract | high | Claude plugin isolation and cross-version startup compatibility must remain intact |

## Phases

### Phase 1: Provision complete owning bundles and separate daemon startup resources

Current source grounding (inspection only; no runtime acceptance):

- `agents-plugin-pi/package.json#L12-L29` inventories Pi files but not a Claude package. `agents-plugin/bin/ws-pi-payload.py#L20-L21` and `agents-plugin/bin/ws-pi-payload.py#L288-L325` overlay rsrc, skills, runtime, dashboard config and launcher; its cache key includes owner identity, first-party manifest and overlay hashes, and `workflow-owner.json` contains only `plugin`. `agents-plugin-pi/src/workflow-owner.ts#L7-L16` applies that identity. Bundle bytes and their freshness must be added without assuming the marker supplies them. `agents-plugin-wsflow/rsrc/` is the mirrored resource tree.
- `agents-plugin-pi/bin/ws-mcp-launcher.py#L1183-L1206` constructs caller hello separately; `agents-plugin-tool/internal/mcp/process_env.go` consumes caller roots, while `agents-plugin-tool/internal/wsagent/lead_plugins.go#L16-L50` and `agents-plugin-tool/cmd/ws-mcp/agent_mcp.go#L16-L41` use daemon-global roots. Validator checks manifest JSON and name, not full assets. `agents-plugin-tool/internal/wsagent/runner.go#L1983-L2019` starts before inbox drain; this does not establish automatic retry.
- Electron's complete-package staging and JavaScript validator are on the unlanded Exec3 branch `impl/develop/cape-barn-hence`: `agents-plugin-tool/desktop-electron/daemon-selection.js` validates referenced plugin files and rsrc/skills trees and returns existing environment roots; `agents-plugin-tool/desktop-electron/scripts/build.cjs` calls `copyPackage` for the supplied package (pending 261115-feat-werkschaft-electron-daemon-override). This checkout's `agents-plugin-tool/desktop-electron/scripts/build.cjs` is still attach-only packaging; coordinate against the branch, not a duplicate implementation.
- Directional T0 remains a premise to prove with fixtures, not a verified result: old ws/wsflow starter to new reader must retain valid roots; new starter to old reader must provide a complete parent package via existing roots; old Pi packages cannot be repaired by absent bytes; old materializers must not erase the only new representation. Existing cross-minor hello refusal remains unchanged (`agents-plugin-tool/internal/wsdashboard/proxy.go`).
- `agents-plugin-pi/scripts/bundle.mjs` and `agents-plugin-pi/dist/index.js` are generated-artifact inputs/output; w5090 has no Node per coordinator, so a Pi source change needs a separately provisioned Node-capable freshness/build check, not an assumed remote Node test.

Implement the owner-approved complete workflow package in plain Pi and both
prepared-native owner lanes, using producer-owned package-local contents.
Select that package only for the daemon-start subprocess through the existing
resource/skills fields; retain caller resources and connection-local behavior.
Preserve valid ws/wsflow startup and explicit-root semantics. Align with
Electron's complete-package staging/validation boundary and report any missing
consumer requirement to its owner, without duplicating lifecycle changes.

Verification uses isolated fixtures, no owner daemon or credentials: source
and copied package-only ws/wsflow/plain-Pi/native-Pi layouts, with no reachable
checkout siblings; unset, valid explicit, relative and invalid roots; referenced
Claude assets/config and correct ws/wsflow defaults; old/new launcher-reader
and materializer directions; cold/warm native payloads and bundle freshness;
Claude isolation, cross-harness relaunch and retained pending input on existing
backend failure. Use fake executables and captured subprocess environments,
not local launcher execution. Retain Pi/Codex behavior and actual-host SDK
identity. Add Electron package-consumer controls without new global refusal.

Run one coherent passing remote check on w5090 covering changed Python/Go and
any generated-artifact freshness inputs, not unscoped slow Go packages. Run
independent R1 after implementation; use fixes-only R2 only for substantive
findings and retain minor findings as Forward. Record commands/results, per-lane
T0 evidence, unresolved acceptance boundaries and coordination in the Result.
Report the phase boundary to the coordinating lead; do not merge/push develop.


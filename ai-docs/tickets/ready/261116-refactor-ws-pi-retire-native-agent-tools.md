---
title: "Retire native Pi agent tools in favor of daemon Pi agents"
related:
  - 261116-feat-ws-pi-bundle-hardening-and-notices
sage-review-design: completed
sage-review-completeness: completed
---

# Retire native Pi agent tools in favor of daemon Pi agents

## Background

The owner retires the extension's public native `ws-agent-*`, `explore`, and `ws-fork` tools in favor of daemon `pi.*`. Specific model/effort selection was a footgun; use capability tiers. Do not port write scopes: Profile has no write-scope field. Both sequencing prerequisites landed by develop d7534a5f7: Pi idle/background-shell handling and bundle hardening/notices. Work only in `groove-animosity-playroom`, branch `impl/develop/retire-ws-agent`.

The initial no-daemon-dependency compatibility premise failed; d7d88bfd5 records the authorized pre-removal stop. The owner authorized continuation on 2026-11-16 at 13:58 +09:00 with a corrected cross-version contract: retain every daemon API old plugins call, and make new/old rendered terminology interoperable. This supersedes that blocker; it is not a waiver of compatibility analysis.

## Decisions

- Owner (1225): daemon `pi.spawn/send/list/stop/transcript/wait` replaces public native agent management; no concrete-model or per-spawn effort selection, aliases, write-scope argument or new Profile field. Forward: separately design valuable write-path limiting with actual enforcement and an honest shell limitation, not a prompt-only sandbox.
- Owner (1227)=b: add a minimal **builtin** `explore` profile in `internal/wsagent/builtin.go` / `BuiltinProfiles`, using existing Profile fields: backend Pi; read/grep/find/ls only; evidence-report instructions; normally caller tier small. T0 additive builtin: old daemon does not know this new name, so the updated adapter must handle that precise refusal rather than assume profile presence.
- Network gap: native Explore's `web_search` and `ws_web_fetch` are extension facades (`agents-plugin-pi/src/delegation-policy.ts:12`, `web-tools.ts:32`). The inspected installed Pi `dist/core/tools/index.js:19-50` has read/bash/powershell/edit/write/grep/find/ls only. There is no core web-search/fetch counterpart for profile `tools`. Keep the builtin local/read-only; do not invent a network field or claim web equivalence.
- Owner (1228)=a: drop `ws-fork` outright, including its public registration and startup wiring; no migration of its question/thread feature.
- Owner continuation (1229)=b, **old plugin + new daemon: T0**, conditioned on retaining every daemon API reached by old native-agent/explore/fork paths unchanged in arguments and result shape. Known direct path: `ws-agent-spawn` passes `bridge.client` (`spawner.ts:4676-4697`); spawn model resolution (`:3771,347-354`) calls `config.resolve_agent` (`:297-306`); `McpStdioClient.callTool` sends `tools/call` (`mcp-stdio-client.ts:341-342`); daemon handler `internal/mcp/server.go:1275-1296` stays unchanged. Inventory shared helper/bridge calls before implementation and preserve their handlers too. No daemon tool removal, rename or argument/result change is authorized.
- **New plugin + old same-minor daemon is possible.** `version-check.ts:39-57` accepts equal bundled/server versions. Launcher `bin/ws-mcp-launcher.py:1449-1456,2004-2005` and daemon `internal/wsdashboard/ensure.go:208-218` retain healthy equal numeric cores regardless of code/content freshness. Numeric minimum is not a guarantee of post-retirement daemon code. No version bump/connection refusal will paper over this case.
- Reverse-direction mitigation, **T0/T1 without an old-version boundary**: new clients opt in via standard MCP `initialize.capabilities.experimental.wsPiDaemonAgents: true`; old daemons ignore unknown experimental metadata. New daemon uses daemon-agent terminology only for a marked client whose effective Pi-tool gate permits it; unmarked legacy Pi clients receive neutral delegation prose, so old default-disabled plugins are not told to call nonexistent Pi tools. MCP-only workers retain record/profile-based capability checks. This is not a config/rsrc/Profile/manifest field.
- On a new adapter with an old daemon, make returned legacy native-tool wording harmless by translating daemon guidance to available `pi.*` idioms and/or a clear system-level translation. Account for rendered prompt artifacts as well as displayed return text. For only the exact unknown-builtin-`explore` refusal, use the pre-existing `search-answer-pi` profile with the same caller tier and evidence brief only after verifying the effective fallback is still read-only; whole-profile overrides may change its capabilities. Refuse clearly if that cannot be proved. Never retry transport/unknown-outcome errors. Preserve explicit Pi disable and availability/version gates rather than force spawning.
- Pi default enablement: change only the existing Pi package's `dashboard.pi_spawn_enabled` default to true; canonical ws is already true, while wsflow's explicit conservative false default remains. Explicit configured false still wins. Default-value-only change is T0; no key/shape addition.
- Safety-preserving scope refinement: keep a **live execution-only** native registry/engine for retained `ws-execute` / `ws-approve`, including approval correlation, intermediate progress, descendant/status projection and execute-worker recovery. Replacing that approval transport is not needed to retire general native-agent tools and would enlarge this ticket dangerously. Supersedes the earlier all-registry migration sketch in f9aaaa3bd/d7d88bfd5. Do not retain a dead registry or a second public general-agent path. Keep `ws-report-to-lead` only for internal execution workers if needed. Existing general/fork sidecars are quarantined before revival, files preserved with an explicit non-recovery notice; do not silently revive or claim daemon adoption of native processes/history.
- Shipped guidance must use namespace-correct `pi.*` / `pi.spawn(profile: "explore")`, capability tiers and the real retained execution tools. Preserve non-Pi harness wording. Mirror canonical rsrc through the documented generator; rebuild Pi dist/notices and both first-party native payloads together. Keep closure archives unchanged if dependencies are unchanged.

## Retained Daemon API Inventory

Pre-retirement call/handler citations refer to develop d7534a5f7; deleting extension callers does not authorize changing these daemon contracts. Native send/list/stop/transcript/report use the local RPC registry, but dormant send re-enters shared startup. Arbitrary inherited ws tools dispatch discovered APIs (`spawner.ts:3733-3739`, `fork.ts:330`, `bridge.ts:1188`), so **all existing daemon APIs stay**, not only the automatic fixed calls below.

| API | Extension caller | Retained daemon handler (agents-plugin-tool/) |
|---|---|---|
| config.resolve_agent | spawner.ts:306,3771,4691,4875; fork.ts:214-230 | internal/mcp/server.go:1275-1296 |
| MCP initialize / tools/list / tools/call | mcp-stdio-client.ts:327-342; bridge.ts:1075-1081 | Existing MCP transport/dispatcher; standard experimental init metadata is additive/ignored by old readers |
| ferrule | bridge.ts:865,1024-1052,1241-1251 | internal/mcp/server.go:882-883,2382; handleLeadLogin |
| workflow_manual | bridge.ts:493,1188,1300 | internal/mcp/server.go:878-879; workflow_manual.go:311 |
| workflow_state | bridge.ts:514 | internal/mcp/server.go:880-881; workflow_manual.go:273 |
| playbook.read | bridge.ts:1308; index.ts:810 | internal/mcp/server.go:2177-2246 |
| playbook.render | bridge.ts:1129-1142,1188-1200 | internal/mcp/server.go:2248-2326 |
| config.get | adapter-config.ts:108-129; index.ts:582-583 | internal/mcp/server.go:1247-1273 |
| dashboard.intake | index.ts:960-976; dashboard-feed.ts:86,822 | internal/mcp/server.go:2338-2339; live_snapshot.go:425 |
| mailbox.lookup_peers | index.ts:1047-1050; mailbox-waiter.ts:1282 | internal/mcp/server.go:2334-2335; mailbox_tools.go:329 |
| mailbox.recv | mailbox-waiter.ts:1193 | internal/mcp/server.go:2332-2333; mailbox_tools.go:146 |
| dashboard.inbox_drain | mailbox-waiter.ts:1214-1247 | internal/mcp/server.go:2340-2341; dashboard_inbox.go:60 |
| CLI mailbox wait / streaming | mailbox-waiter.ts:773; index.ts:1054+ | cmd/ws-mcp/main.go:35; mailbox.go:69,106 |

## Prior Decisions

- f9aaaa3bd (2026-11-16): initial native retirement inventory and all-registry migration sketch — superseded where the owner answered (1225), (1227), (1228), and where retaining the live execution-only approval path avoids changing an unretired contract.
- d7d88bfd5 (2026-11-16): stop on failed daemon-independence/embedded-text predicates — owner explicitly authorized the corrected compatibility argument above; do not repeat the obsolete predicate as a gate.
- 51c75ee61 / d7534a5f7: retain bundle freshness, deterministic third-party notices, runtime initialization completion signal and loud failure reporting while removing public tools. These remain preservation constraints.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md`, `skill-authoring.md`, `wsflow-mirroring.md`, `release-compatibility.md`, and `ws-mcp.md` for matching paths; Pi docs before harness/API edits.
- No local Go build/test, ws/ws-mcp/test executable, Pi launcher (including synthetic adapter startup tests), or scripts/dev.py. Go checks only via approved `bash D:/sys_lock.sh remote w5090 run exec11 25 --sync <this-worktree> -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/<pkg>/... -run "<pattern>"'`. w5090 has no Node. Only pure local Node tests that never spawn a launcher.
- Temporary artifacts only under `D:/Temp/exec11`; set TMP/TEMP/TMPDIR and PYTHONUTF8=1 for producer runs and remove owned caches/dependencies/review artifacts. Minimize file touches.
- No new Profile/config/rsrc/first-party manifest field or daemon API break. If unavoidable T3 is demonstrated, stop for owner approval; do not infer one merely from dependency on an unchanged API.
- Sage design/completeness before ready promotion. R1 then fixes-only R2 for substantive findings; minor findings go to Forward. One passing focused verification set; no full-suite/real-host rollout requirement under these safety restrictions. No Co-Authored-By, merge or push. Report phase commits.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Pi index/spawner/bridge/bootstrap, builtin profiles, terminology, guidance and generated artifacts |
| scope.surface | cross-module | Client initialize metadata and rendered terminology interoperate with existing daemon APIs |
| scope.new_public_symbol | yes | Add builtin explore profile, no new wire tool or Profile field |
| scope.new_type_contract | no | Existing Profile, standard MCP experimental map and generic asset inventories |
| scope.test_surface | existing | builtin/profile/playbook tests, Pi bridge/native agent tests and bundle-freshness test |
| complexity.reuse_points | confirmed | BuiltinProfiles, read-only search-answer-pi, piListed, existing bridge result mapping, execute-worker engine, native producer |
| complexity.side_effect_risk | moderate | Tool retirement and persisted native sidecar quarantine; no shared store migration |
| risk.correctness | high | No orphan approvals, accidental legacy revival, stale daemon instructions or duplicate completion |
| risk.fit | moderate | Preserve safe execution path rather than redesign daemon approval transport |
| risk.test | high | Mixed client/daemon versions, capability gates and retired registrations need focused regressions |
| risk.security_or_contract | high | Preserve profile/tier ceilings and every old daemon call contract |

## Sage Gate (2026-11-16)

- Design: pass after independent source/19-ready-ticket comparison; no blocking contradiction or parent. One autonomous minor: exercise effective overridden fallback profiles, addressed by the fail-closed read-only verification decision above. Omitted rationale.query currency lookup because that tool is unavailable; runtime/explorer subprocess probes are outside the no-launcher boundary.
- Completeness: independent pass; no issues or omissions.
- The exposed tool inventory has no tickets.sage_gate/sage_record, and the parent session's workflow tools remain bound to its released worktree. Applied the reviewed gate/postures and status move directly in this assigned worktree; no local ws executable was used. All actual review verdicts are recorded here, not self-certified.

## Phases

### Phase 1: Builtin explorer, defaults and compatible daemon terminology

Add the existing-fields-only builtin explorer; update Pi/default terminology with actual-client/effective-tool gating. Preserve non-Pi behavior, explicit disable, old APIs and same-minor compatibility. Acceptance: focused remote Go checks cover builtin fields/read-only instructions, profile/tier ceilings, conditional terminology/namespace output, old/unmarked clients, MCP-only workers and configured opt-out; source-backed API inventory and both version-direction verdicts are recorded.

### Phase 2: Retire public native tools and isolate retained execution

Remove public native general-agent/explore/fork registrations and fork wiring. Preserve the live execution-only registry, approval decisions/progress/status/recovery without broad protocol changes; quarantine old general/fork sidecars with explicit preserved-file diagnostics. Adapt legacy daemon guidance and exact unknown-explorer fallback without retrying uncertain calls. Acceptance: pure Node tests cover registration absence, partial/active loadouts, old/new daemon text and prompt artifacts, exact profile fallback, tier/read-only preservation, sidecar classification and retained approval/progress flows. No duplicate daemon/native child projection or completion claim.

### Phase 3: Ship guidance/mirrors and close after review

Update shipped guides/playbooks/conventions to actual transports; regenerate wsflow rsrc if canonical rsrc changes. Rebuild dist/THIRD_PARTY_NOTICES and both first-party native copies using the existing producer. Acceptance: one final focused remote Go/local pure-Node set, freshness hashes and mirrored notices pass; R1 substantive findings fixed and fixes-only R2 clean. Record Results and Forward, close ticket; no merge/release implied.

---
title: "Retire native Pi agent tools in favor of daemon Pi agents"
related:
  - 261116-feat-ws-pi-bundle-hardening-and-notices
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-11-16
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
- R1 preservation refinement: every fresh execution record pins its approval relay across dormant /audit resume. Retired owned histories receive intentional durable `liveness.ownerHeld:true` protection before quarantine and during global retention scans; this existing field is understood by old readers/pruners (`agent-storage.ts` read/update/removal checks), not a new schema or a live-process claim. Unknown/unreadable/unquarantinable source data blocks sidecar writes, fresh persistent execution and that startup's retention, with explicit notice, until preservation is established. No daemon API/Profile/manifest shape changes are involved.
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
| scope.new_type_contract | yes | Optional standard MCP experimental capability semantics and internal sidecar recovery outcome; existing Profile and all persisted/API shapes remain |
| scope.test_surface | existing | builtin/profile/playbook tests, Pi bridge/native agent tests and bundle-freshness test |
| complexity.reuse_points | confirmed | BuiltinProfiles, read-only search-answer-pi, piListed, existing bridge result mapping, execute-worker engine, native producer |
| complexity.side_effect_risk | moderate | Tool retirement and persisted native sidecar quarantine; no shared store migration |
| risk.correctness | high | No orphan approvals, accidental legacy revival, stale daemon instructions or duplicate completion |
| risk.fit | moderate | Preserve safe execution path rather than redesign daemon approval transport |
| risk.test | high | Mixed client/daemon versions, capability gates and retired registrations need focused regressions |
| risk.security_or_contract | high | Preserve profile/tier ceilings and every old daemon call contract |

## Forward

- Owner (1225): separately design write-path limiting, not a shell sandbox or a new field in this ticket.
- R1 M1 (minor, not an R2 blocker): `execute-gateway.ts` still has live descriptions/abort wording referring to retired native spawn/explore/stop names (d609726e2:686,714,834). Update them to daemon/retained-execution wording in a later change and rebuild artifacts then. System-level legacy translation already mitigates this; left untouched per the owner's minor-to-Forward rule.

## Sage Gate (2026-11-16)

- Design: pass after independent source/19-ready-ticket comparison; no blocking contradiction or parent. One autonomous minor: exercise effective overridden fallback profiles, addressed by the fail-closed read-only verification decision above. Omitted rationale.query currency lookup because that tool is unavailable; runtime/explorer subprocess probes are outside the no-launcher boundary.
- Completeness: independent pass; no issues or omissions.
- The exposed tool inventory has no tickets.sage_gate/sage_record, and the parent session's workflow tools remain bound to its released worktree. Applied the reviewed gate/postures and status move directly in this assigned worktree; no local ws executable was used. All actual review verdicts are recorded here, not self-certified.

## Phases

### Phase 1: Builtin explorer, defaults and compatible daemon terminology

Add the existing-fields-only builtin explorer; update Pi/default terminology with actual-client/effective-tool gating. Preserve non-Pi behavior, explicit disable, old APIs and same-minor compatibility. Acceptance: focused remote Go checks cover builtin fields/read-only instructions, profile/tier ceilings, conditional terminology/namespace output, old/unmarked clients, MCP-only workers and configured opt-out; source-backed API inventory and both version-direction verdicts are recorded.

### Result (d609726e2) - 2026-11-16

- Added builtin `explore` with Pi backend, read/grep/find/ls, minimal evidence instructions and no model/effort/network/write fields. Caller tier selects the model; no Profile field was added. Pi package spawn default is now true; ws stays true, wsflow stays false, and explicit false remains authoritative.
- Daemon-rendered idioms use actual capability/availability, not numeric freshness. Marked eligible owner leads and record/profile-backed MCP-only callers with both spawn/send permissions get namespace-correct daemon terms; legacy/unavailable callers get neutral prose. Non-Pi idioms are preserved.
- Old plugin + new daemon is T0 under the owner's corrected scenario: the inventoried APIs and all other existing daemon handlers/schemas remain unchanged. New plugin + old equal-core daemon is handled without a T3 boundary: standard optional initialize metadata is ignored by old daemons; displayed/returned/rendered-file legacy guidance is superseded explicitly; only an exact pre-allocation unknown-explore refusal permits a same-tier fallback after effective read-only profile verification. Uncertain/unsafe failures never retry. Old text can be display-degraded (T1), not a nonexistent-tool/version lockout.
- Focused remote Go wsagent/mcp checks passed (44.438s / 0.391s, job `4fcca51c641f43c9ad3c49e28a2a5acd`). Patterns included builtin/Profile, Pi capability/rendering/tool gates/spawn resolution, shipped defaults and retained config.resolve_agent. Independent R1 reviewed both version directions and found no daemon API or Profile/manifest shape break.

### Phase 2: Retire public native tools and isolate retained execution

Remove public native general-agent/explore/fork registrations and fork wiring. Preserve the live execution-only registry, approval decisions/progress/status/recovery without broad protocol changes; quarantine old general/fork sidecars with explicit preserved-file diagnostics. Adapt legacy daemon guidance and exact unknown-explorer fallback without retrying uncertain calls. Acceptance: pure Node tests cover registration absence, partial/active loadouts, old/new daemon text and prompt artifacts, exact profile fallback, tier/read-only preservation, sidecar classification and retained approval/progress flows. No duplicate daemon/native child projection or completion claim.

### Result (d609726e2) - 2026-11-16

- Removed public native spawn/send/list/stop/transcript, Explore and fork registration/wiring, including lazy discussion entry points. Retired permissions are filtered and denied; progress reporting is internal to execution workers. The live execution registry preserves command approval correlation, progress, dashboard identity and audit-style recovery rather than masquerading as daemon child management.
- Updated the four Pi guides and bootstrap/bridge guidance to actual transports. General daemon completion is ordinary final-answer/mail delivery, not intermediate approval/progress transport. Rendered legacy prompt files are adapted before provenance capture, not just their displayed response.
- Initial launcher-free focused Node set passed 198 tests with one Windows symlink-permission skip; full output read. R1 found three Important preservation/resume composition gaps, corrected in the Edition below; M1 is explicitly Forward.

#### Edition (160bd9d01) - 2026-11-16

- I1: every fresh execution record pins its approval relay so process loss and /audit dormant resume cannot strand a new command. Regression uses the production marked registry and registered gateway, with only transport/working-context execution stubbed; it checks exactly-correlated approval, stale-id rejection and a valid denial.
- I2: preserve actual owned transcript/prompt bytes, not only registry metadata. Existing durable `liveness.ownerHeld:true` holds are established with locked identity checks before quarantine and across retirement-aware all-owner retention scans; existing old/new readers honor that field. Tests use expired owned homes and an eligible execution control, then prove raw sidecar/history bytes survive real pruning and a later startup 100 days later.
- I3: explicit writable/retentionSafe outcomes block every sidecar writer, new persistent execution (before even the optional initial command), and unsafe startup retention when read/protection/consume/quarantine fails. Shutdown cannot overwrite retained source bytes or falsely claim recovery. Read/rename fault tests restore filesystem access before attempting nonempty shutdown snapshots, then assert exact original bytes and subsequent safe recovery.
- After-fix filesystem/in-process RPC regressions: 56/56 passed; final compatibility/bootstrap/health/freshness set: 45 passed, 0 failed, 1 Windows symlink skip. No actual or synthetic launcher ran. Independent fixes-only R2 reviewed committed `160bd9d01ffe413a47a012d5f60ba50a8ca548a0` and is CLEAN for I1-I3; no remaining substantive findings, M1 Forward. No third review round.

### Phase 3: Ship guidance/mirrors and close after review

Update shipped guides/playbooks/conventions to actual transports; regenerate wsflow rsrc if canonical rsrc changes. Rebuild dist/THIRD_PARTY_NOTICES and both first-party native copies using the existing producer. Acceptance: one final focused remote Go/local pure-Node set, freshness hashes and mirrored notices pass; R1 substantive findings fixed and fixes-only R2 clean. Record Results and Forward, close ticket; no merge/release implied.

### Result (d609726e2) - 2026-11-16

- Canonical skills/rsrc/conventions had no searched literal retired-tool instruction; their harness-specific wording is supplied by the changed Go renderer. No unnecessary canonical rsrc edit or wsflow rsrc regen was performed. Both native products carry the updated first-party guides and dist.
- Source/dist/native payloads were regenerated together, retaining prior bundle input/header/notice guards and the awaited initialization-complete signal. Third-party notices and both closure ZIPs/manifests remain byte-identical. No dependency, package version, API shape or persisted schema field change.

#### Edition (160bd9d01) - 2026-11-16

- R1 fix source and regenerated dist/native artifacts landed in the same commit. Final source/native input hash is `d3f8b9e54a51ad2dea18981b52c87e012bd0948ccf13b16295953edf2ff9478b`; notices remain SHA-256 `efcf20f46412a1c4f28bb0a7889783e3f179fdbf8483cd9850a2008478538884` in all three distributions.
- Final remote bundle/prepared-payload checks passed 38.538s, job `0108a5453d584c719978f1182adf8030`. Command: approved sys_lock w5090 run exec11 25 with this worktree sync, Go PATH, `cd agents-plugin-tool && go test ./internal/wsagent/... -count=1 -run "TestPiBundleCommittedIsFresh|TestPiPreparedPackageOnlyLayouts|TestPiPreparedSourceDrift"`.
- Final local commands (TMP/TEMP/TMPDIR under D:/Temp/exec11, PYTHONUTF8=1, role/policy env unset): `node --test` on native-retirement-r1, native-agent-retirement, persistent-explore, session-retention, fork, approval-protocol and session-bootstrap-guard test files passed 56; daemon-agent-compat, lead-bootstrap, testdata/pi-lead-health and bundle-freshness passed 45 with one skip. Logs were read in full. `git diff --check` passed.
- Review: R1 had I1-I3 Important and M1 Minor; fixes-only R2 is clean for I1-I3 on reviewed implementation HEAD `160bd9d01ffe413a47a012d5f60ba50a8ca548a0`. M1 remains Forward as directed. Detailed temporary review reports/logs were read and their outcomes recorded here before cleanup.
- Surprise: the first remote sync attempted during concurrent source edits failed SHA256 validation before tests started. A frozen-source retry recovered successfully; later final sync was clean. No implementation test failed. Missing local SDK imports during early inspection were resolved through a locked-dependency junction under the approved temp root; no launcher was run to resolve them.
- Omitted: full npm suite, launcher-dependent integration tests (actual and synthetic), remote Node tests (w5090 has no Node), live installed-plugin/daemon rollout and provider probes under owner restrictions. No merge/push or prohibited local executable/build occurred. Owned dependency junction/caches, logs and review artifacts are removed at close. Compatibility settled T0/T1, with no unavoidable T3 boundary.

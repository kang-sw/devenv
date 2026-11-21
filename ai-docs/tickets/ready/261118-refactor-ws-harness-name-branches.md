---
title: "공통 레이어의 하네스 이름 분기를 하네스별 등록 데이터로 이전"
parent: 261114-epic-ws-common-session-foundation
related:
  261118-feat-ws-agy-tool-names-and-worker-role: "(143)=a introduced the BackendRegistration descriptor seam this umbrella extends"
  261118-research-ws-pi-hosted-connector: "stage 2 targets full common-interface support by Pi; this leak inventory is its baseline"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3302b5eedd080007
sage-review-completeness-reviewed: 3302b5eedd080007
---

# Move harness-name branches out of the common layer into per-harness registration data

## Background

The harness common interface (epic `261114-epic-ws-common-session-foundation`)
is only partly isolated from per-harness implementations. The refreshed source
read used `develop` at merge `da8930600` on 2026-11-21 and covered
non-test Go code under `agents-plugin-tool/internal/wsagent`, `internal/mcp`,
`internal/wsdashboard`, and the `cmd/ws-mcp` registration assembly.

The old estimate of about 75 `BackendClaude|BackendPi|BackendAgy` references is
retired. It mixed raw references, omitted Codex and newer dashboard/MCP sites,
and predated the Agy activity hook. The inventory below has **70 grouped
decision sites**: 8 declarative descriptor-data sites, 37 backend-behavior
sites, 20 candidate legacy-compatibility exceptions, and 5 intentionally
adapter-owned sites (`agents-plugin-tool/internal/wsagent/`, `agents-plugin-tool/internal/mcp/`, `agents-plugin-tool/internal/wsdashboard/`, `agents-plugin-tool/cmd/ws-mcp/dashboard.go`). A site is one branch or one adjacent table/branch cluster;
all lines in a cluster share one migration decision. The classes route work;
candidate exceptions remain only where old persisted bytes or external
identifiers cannot be interpreted through registration data ((1881)).

### Refreshed inventory

| Class | Site on `develop` at `da8930600` | Current name-dependent behavior |
|---|---|---|
| declarative descriptor data | `cmd/ws-mcp/dashboard.go:305-322` | Registers the Claude, Pi, Codex, and Agy backends and their current capability values. |
| declarative descriptor data | `internal/wsagent/builtin.go:49` | Selects the Claude versus non-Claude search-answer native tool names. |
| declarative descriptor data | `internal/wsagent/builtin.go:242-320` | Builds support, legacy Pi, agent-worker, search-answer, and lead profile rows from backend-name tables, including a Claude-only support-tool branch. |
| declarative descriptor data | `internal/wsagent/support.go:43-44` | Orders the built-in Claude and Pi tracker-support profiles. |
| declarative descriptor data | `internal/wsagent/catalog.go:571-575` | Supplies keyless compaction fallbacks from a backend-name-to-catalog table plus the Pi descriptor. |
| declarative descriptor data | `internal/mcp/playbook_terminology.go:16-43` | Stores Claude, Codex, Pi, and neutral playbook terminology rows. |
| declarative descriptor data | `internal/mcp/config_registry.go:24-25,151` | Repeats the closed harness lists used by prompt selectors, agent-tier selectors, and stored harness buckets. |
| declarative descriptor data | `internal/mcp/compaction.go:34` | Repeats the closed harness enum for compaction tuning. |
| backend behavior method | `internal/wsagent/permission.go:39-52` | Maps common permission levels to Claude native permission modes. |
| backend behavior method | `internal/wsagent/permission.go:54-68` | Derives legacy common permission levels differently for Pi, Claude, and other backends. |
| backend behavior method | `internal/wsagent/permission.go:72-75,232-237` | Labels Claude plan-mode enforcement and its pending-observation diagnostic. |
| backend behavior method | `internal/wsagent/permission.go:97` | Admits common lead permissions only for Claude and Pi despite registration metadata. |
| backend behavior method | `internal/wsagent/permission.go:333-341` | Applies Agy's user-only bypass transition inside common permission handling. |
| backend behavior method | `internal/wsagent/permission.go:380-392` | Reconciles an explicit selection only for Claude native modes. |
| backend behavior method | `internal/wsagent/agent_tools.go:220-224` | Runs the extension child check only for Pi spawn admission. |
| backend behavior method | `internal/wsagent/runner_events.go:361-362` | Calls `AgyActivity` after a non-replay, non-compaction Agy turn settles; added by merge `dec0d7d6a`. |
| backend behavior method | `internal/wsagent/runner_summary.go:59-68` | Starts and projects terminal reasoning summaries only for hosted Pi leads. |
| backend behavior method | `internal/wsagent/runner_process.go:70-75` | Rechecks the parent capability ceiling only for a fresh Pi worker start. |
| backend behavior method | `internal/wsagent/runner_process.go:110-115` | Binds a Pi worker key to its working directory rather than the profile root. |
| backend behavior method | `internal/wsagent/runner_process.go:221-226` | Persists the checked launch definition only for fresh Pi workers. |
| backend behavior method | `internal/wsagent/runner_process.go:310-317` | Composes hosted lead instructions for Claude, Agy, and Codex while Pi obtains them through its extension/bridge. |
| backend behavior method | `internal/wsagent/runner_process.go:410-418` | Marks unfinished Agy/Codex HTTP-only starts stale on attach. |
| backend behavior method | `internal/wsagent/runner_process.go:420-440` | Selects HTTP MCP for Claude/Agy/Codex and additionally supplies Claude's stdio fallback. |
| backend behavior method | `internal/wsagent/runner_create.go:172-178` | Mints Pi spawned-session keys from `spawn.Cwd`. |
| backend behavior method | `internal/wsagent/runner_operations.go:118` | Marks spawned Pi-child input as a native steer. |
| backend behavior method | `internal/wsagent/spawn.go:178-197` | Enforces the parent launched-profile ceiling only for native Pi child profiles. |
| backend behavior method | `internal/wsagent/spawn.go:225-238` | Rechecks a Pi child spawned under a lead against a concurrent parent close. |
| backend behavior method | `internal/wsagent/handoff.go:83-85` | Refuses terminal takeover for Codex. |
| backend behavior method | `internal/wsagent/handoff.go:98-102` | Requires a regular session file before taking over Pi. |
| backend behavior method | `internal/wsagent/handoff.go:158-168` | Clears non-Claude native permission mode and stores Pi's file identity versus other backends' session ID. |
| backend behavior method | `internal/wsagent/handoff.go:204-206` | Refuses terminal handoff for Codex. |
| backend behavior method | `internal/wsagent/handoff.go:307-327` | Builds Claude, Agy, and Pi terminal resume commands. |
| backend behavior method | `internal/wsagent/backend.go:543-550` | Gives a Claude-specific Windows cmd-shim remediation. |
| backend behavior method | `internal/mcp/playbook_terminology.go:88-100` | Computes Claude's namespace-dependent spawn idiom outside the terminology row. |
| backend behavior method | `internal/mcp/playbook_terminology.go:151-177` | Detects Claude native-subagent removal and Pi MCP-only records by backend name. |
| backend behavior method | `internal/mcp/playbook_render.go:111` | Treats Pi client metadata as a daemon-agent capability alongside descriptor and record evidence. |
| backend behavior method | `internal/mcp/summary_owner.go:18` | Enables terminal summary intake only for Pi. |
| backend behavior method | `internal/mcp/live_snapshot.go:527` | Accepts negotiated reasoning-summary events only from the Pi adapter. |
| backend behavior method | `internal/wsdashboard/plan_usage.go:195` | Accepts pushed usage only from Claude live snapshots. |
| backend behavior method | `internal/wsdashboard/plan_usage.go:437` and `internal/wsdashboard/daemon_runtime.go:20` | Scans only Claude sessions for pushed usage snapshots. |
| backend behavior method | `internal/wsdashboard/plan_usage.go:454` | Adds the separate Agy history projection only when an Agy usage entry exists. |
| backend behavior method | `internal/wsdashboard/pageview_rail.go:311` | Offers terminal takeover in the rail only for Claude/Pi registrations. |
| backend behavior method | `internal/wsdashboard/plan_usage_probe.go:114-120` | Routes Codex five-hour samples to its separate history file. |
| backend behavior method | `internal/wsdashboard/plan_usage_history.go:412-423` | Projects only Codex entries into the separate five-hour history view. |
| backend behavior method | `internal/wsdashboard/pageview_hud.go:230-237` | Accepts only the four built-in harness names in HUD normalization. |
| legacy-compatibility exception | `internal/wsagent/runner_startup.go:228` | Preserves old direct non-Pi orphan admission while held workers use the former Pi cleanup. |
| legacy-compatibility exception | `internal/wsagent/runner_create.go:126` | Retains the legacy `pi.spawn` rule that a tiered non-worker profile must be Pi. |
| legacy-compatibility exception | `internal/wsagent/runner_operations.go:83-90` | Retries a send by reopening a closed spawned Pi child. |
| legacy-compatibility exception | `internal/wsagent/children.go:366-389` | Revives only closed Pi children and validates their historical session file. |
| legacy-compatibility exception | `internal/wsagent/children.go:856` | Treats legacy Pi children under lead parents as descendants alongside common agent-worker profiles. |
| legacy-compatibility exception | `internal/wsagent/worker_rejoin.go:15` | Accepts the legacy `holderKindPiWorker` descriptor with an absent/Pi backend. |
| legacy-compatibility exception | `internal/wsagent/runner_process.go:116-122` | Falls back to the root-bound key written by 0.49.x Pi children. |
| legacy-compatibility exception | `internal/wsagent/runner_process.go:533-540` | Reattaches Pi from the persisted launch definition while older held records best-effort resolve their profile. |
| legacy-compatibility exception | `internal/wsagent/registration.go:135-148` | Accepts legacy Pi permission values and supplies keyless Claude/Agy/Codex permission-mode fallbacks. |
| legacy-compatibility exception | `internal/mcp/server_harness.go:89-96` | Recognizes the Pi extension's exact `ws-pi-bridge` client name before descriptor lookup. |
| legacy-compatibility exception | `internal/mcp/server_harness.go:105-110` | Detects Codex and Claude from historical raw initialize substrings. |
| legacy-compatibility exception | `internal/mcp/server_harness.go:115-117` | Infers Codex from `x-codex-turn-metadata`. |
| legacy-compatibility exception | `internal/mcp/server_harness.go:127-145` | Normalizes the built-in Codex/Claude/Pi names before accepting registered manual-harness names. |
| legacy-compatibility exception | `internal/wsdashboard/registry.go:455-460` | Protects a handed-off Pi conversation stored inside the daemon session directory from retirement deletion. |
| legacy-compatibility exception | `internal/wsdashboard/registry.go:483` and `internal/wsdashboard/page_reader.go:311` | Checks and watches the historical Pi session-file location for child revival. |
| legacy-compatibility exception | `internal/wsdashboard/plan_usage_history.go:69-78` | Validates the persisted Claude/Codex history source and window combinations. |
| legacy-compatibility exception | `internal/wsdashboard/plan_usage_history.go:391` | Excludes Agy entries from the older shared history projection because Agy has its own grouped history. |
| legacy-compatibility exception | `internal/wsdashboard/proxy_http.go:287-297` | Replays harness-specific MCP `clientInfo.name` values for Pi, Claude, Codex, and Agy. |
| legacy-compatibility exception | `internal/wsdashboard/conversation_presentation.go:69-75` | Recognizes stored peer-lead prefixes containing the historical Claude/Codex/Pi labels. |
| legacy-compatibility exception | `internal/wsdashboard/pageview_hud.go:240-249` | Infers Pi from the historical Pi agent-kind marker when a live agent lacks a harness name. |
| intentionally adapter-owned | `internal/wsagent/agy_bypass.go:16,64` | Keeps Agy bypass storage/native-mode translation inside the Agy adapter helper. |
| intentionally adapter-owned | `internal/wsagent/agy_watchdog.go:86,134` | Keeps Agy model-wait loading and arming inside the Agy watchdog. |
| intentionally adapter-owned | `internal/wsagent/pi_launch.go:112` | Keeps Pi extension/MCP-only classification inside `PiMCPOnly`. |
| intentionally adapter-owned | `internal/wsdashboard/plan_usage_agy.go:63,568` | Keeps Agy-specific grouped usage/history validation inside the Agy usage provider. |
| intentionally adapter-owned | `internal/wsagent/hosted_choices.go:116-139` | Keeps the Agy model-listing command in an AgyBackend method. |

Backend implementation files (`claude*.go`, `codex*.go`, `agy*.go`, and
`pi*.go`) remain the owners of genuine adapter behavior. The umbrella removes
name checks from common callers by having registration data or narrow backend
methods expose that behavior; it does not make adapter implementations
name-free.

## Decisions

- **(144)=a, 2026-11-18: capture only.** The cleanup is captured in `todo/`
  under the epic and executed later; Claude usage is scarce this week.
  Rejected: no ticket, moving sites only as feature work touches them.
- **Umbrella and last, owner 2026-11-20.** This ticket is the umbrella for
  removing Claude/Codex/Agy/Pi string and `Backend*` branches from common code.
  Its implementation runs after the migration-relevant prerequisites land.
  Backend-specific adapter files remain adapter-owned.
- **(1880)-(1883), owner (1880)=a, 2026-11-20.** The owner accepted all four
  worker recommendations in one answer.
- **(1880): one registration seam.** Put declarative harness differences on
  `BackendRegistration` values and executable differences behind narrow
  `Backend` methods. Rejected: a second catalog; one callback-heavy descriptor.
- **(1881): source-backed survivors only.** Retain a common-code name branch
  only when old persisted bytes or external identifiers (including historical
  `clientInfo`, holder kinds, and Pi paths) cannot be interpreted through
  registration data. Give each survivor a regression fixture and a site comment.
  Rejected: retaining every candidate with a rationale; migrating every name
  check.
- **(1882): migration-relevant promotion gate.** At promotion, name an explicit
  prerequisite list refreshed against the parent epic, limited to tickets that
  can add or move sites in this inventory. Rejected: waiting for every active
  child of the parent epic.
- **(1883): T0 ceiling.** This umbrella is T0-only unless a prerequisite ticket
  already settled a higher tier. Split newly discovered T1+ behavior into its
  own ticket. Rejected: allowing T1/T2 inside this umbrella; splitting every
  observable change regardless of tier.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)
- Manuals for the touched paths: `ai-docs/manuals/performance.md`,
  `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/ws-mcp.md`, and
  `ai-docs/manuals/shipped-surface-boundary.md` where shipped surfaces change.
- Prerequisite list ((1882), refreshed against the parent epic on
  2026-11-21 at `da8930600`): `261115-feat-ws-permission-profile` Phase 3 (the
  Codex per-run permission launch path, in flight) is the only
  migration-relevant prerequisite, because it adds Codex permission mapping to
  `support.go`/launch sites in this inventory. Keep this ticket in `todo/`
  until that phase has landed on `develop`. Not prerequisites: that ticket's
  Phase 4 (agy, owner-account gated) and Phase 5, and
  `261118-feat-ws-agy-tool-names-and-worker-role` Phase 3 (knob-gated probes),
  because they wait on external gates and would hold this umbrella
  indefinitely; any site they add must use the (1880) seam, and Phase 4's
  residual audit catches what they move. `261117-feat-ws-exec-tools` and the
  child-session tickets add new code under the same seam rule and are not
  prerequisites. Neither are `261114-feat-ws-live-reasoning-summary` and
  `261115-feat-ws-claude-reasoning-summary-producer`, although they touch
  `live_snapshot.go`, `summary_owner.go`, and `runner_summary.go`: they are
  stage-4 candidates with no landed phase, so ordering them first would hold
  this umbrella indefinitely. Phase 4's source-scan regression test binds any
  of their later changes to the seam instead.
- Preserve Decision (144) and use the descriptor seam already established by
  `261118-feat-ws-agy-tool-names-and-worker-role`; do not add a parallel
  harness registry. Put declarative differences on `BackendRegistration` and
  executable differences behind narrow `Backend` methods ((1880)).
- Genuine behavior remains in backend/adapter-owned files. Common code may use
  registration data or backend methods but may not branch on a harness name.
- Retain a common-code name branch only when old persisted bytes or external
  identifiers cannot be interpreted through registration data. Each survivor
  needs a regression fixture and a site comment naming that boundary ((1881)).
- Go tests and harness parity runs execute on remote w5090 when run from the
  office PC; on the Mac clone they run locally. Follow the performance
  manual's file-operation budget and measure the affected daemon lifecycle
  paths on the machine that ran the tests.
- A compatibility verdict is directional and source-backed: identify the old
  and new writer/reader, the write or refusal, and the new-version reopen path.
  Keep this umbrella at T0 unless a prerequisite already settled a higher tier;
  split newly discovered T1+ behavior into its own ticket ((1883)). Do not
  infer T2 from retained append-only data alone.

## Prior Decisions

- 261114-refactor-ws-common-session-interface (2026-11-14, Decisions): "The single composition root stays `cmd/ws-mcp/dashboard.go`." — bearing: constrains
- 261118-feat-ws-agy-tool-names-and-worker-role (2026-11-18, Decisions): "agy behavior lives in a harness descriptor." — bearing: supports
- 261115-feat-ws-permission-profile (2026-11-17, commit): "Model/effort choices and permission modes are deliberately absent from catalog Values" — bearing: constrains
- 261119-feat-ws-codex-hosted-worker (2026-11-19, commit): "Common code carries no harness-name branch: Codex specifics live in the registration descriptor and the adapter, mirroring the agy worker." — bearing: supports
- 261117-feat-ws-hosted-prompt-assembly (2026-11-18, commit): "The system prompt is the one channel the daemon controls that such a policy does not filter." — bearing: constrains
- 261117-feat-ws-hosted-compaction-lifecycle (2026-11-19, Result (c65451174)): "Added `CompactionProtocol` on `HarnessProfile` / `BackendRegistration`" — bearing: supports
- 261120-feat-ws-agy-usage-activity-trigger (2026-11-20, Result (86bad7ff0)): "Added `BackendRegistration.PlanUsageActivity` and enabled it only on agy's daemon registration." — bearing: supports
- 2ca6ff3d (2026-11-20, commit): "Placed the signal in conv.endTurn beside settleTurn so replayed events, non-turn endings, compaction commands, and watchdog-intercepted continuations cannot trigger it." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/, internal/mcp/, internal/wsdashboard/, cmd/ws-mcp/dashboard.go |
| scope.surface | cross-module | BackendRegistration, Backend methods, MCP prompt and detector behavior, and dashboard consumers cross packages |
| scope.new_public_symbol | yes | Phase 1 adds exported fields on BackendRegistration and narrow Backend methods; exact names unsettled |
| scope.new_type_contract | yes | BackendRegistration and Backend method signatures change in Phase 1 |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/registration_test.go, internal/mcp/playbook_terminology_test.go, internal/wsdashboard/plan_usage_test.go, cmd/ws-mcp/dashboard_test.go |
| complexity.reuse_points | confirmed | BackendRegistration in internal/wsagent/registration.go and composition in cmd/ws-mcp/dashboard.go |
| complexity.side_effect_risk | high | Runner lifecycle, permission, MCP detection, and dashboard history branches affect active sessions |
| risk.correctness | high | Four-harness lifecycle and permission behavior must retain parity across migration |
| risk.fit | high | Adapter ownership, single registration seam, and source-backed survivor boundary constrain the refactor |
| risk.test | high | Four-harness parity, downgrade fixtures, race checks, and remote file-operation measurements are required |
| risk.security_or_contract | high | Permission admission, MCP client identity, persisted holder paths, and T0 compatibility are contract boundaries |

## Phases

### Phase 1: Extend the descriptor/catalog seam

First check the explicit promotion prerequisite list against the parent epic
and refresh the inventory on its landed head ((1882)). Add the reusable
declarative values and narrow backend methods settled in (1880), then move the
inventory's descriptor-data construction behind that single seam. Primary files:
`internal/wsagent/registration.go`, `backend.go`, `catalog.go`, `builtin.go`,
`support.go`, the affected backend-owned files, and
`cmd/ws-mcp/dashboard.go`. Keep runtime registration data out of persisted
profile/config shapes unless separately approved.

Verification: registration-copy and descriptor-default unit tests; built-in
profile/catalog fixtures for Claude, Codex, Agy, and Pi; a focused search that
shows no new parallel harness catalog; and Go tests for `internal/wsagent`
and `cmd/ws-mcp` packages affected by the final design. Record the explicit
prerequisite list and inventory refresh against the parent epic ((1882)).

Compatibility verification ((1883)): show that additions are runtime-only and
invisible to MCP/config/store readers (T0). If a directional old/new
reader-writer trace reveals T1+ behavior, split it into its own ticket.

### Result (2c2ee3478) - 2026-11-21

- **VERIFIED — prerequisite (1882).** The parent epic's Child Tickets and
  Extension dependency map leave the promotion list at one item,
  `261115-feat-ws-permission-profile#phase3`. Its implementation (`090d72e04`)
  and observation correction (`53349aeb5`) landed in develop merge
  `66c7792d3`, an ancestor of `684b2c834`. Codex's registration advertises
  the landed levels (`cmd/ws-mcp/dashboard.go:311` at the baseline,
  `internal/wsagent/codex.go:33-38` after Phase 1). Permission Phases 4–5
  and the gated Agy probes remain follow-ons. The exec and child-session
  work uses the existing seam, and live reasoning remains stage 4
  (`261114-epic-ws-common-session-foundation`, Child Tickets).
- **VERIFIED — inventory refresh at `684b2c834`.** Rechecked the 70 grouped
  decisions in the non-test Go scope using a name scan and the diff from
  `da8930600`. The Codex prerequisite expands the mode mapping
  (`permission.go:40-90`) and quality/nudge cluster (`permission.go:92-105,265-270`).
  Lead permission admission and reconciliation now read the registration or
  mapping (`permission.go:123-125,414-420`), removing two direct name checks.
  Two backend-behavior sites replace those rows: live-only Codex default
  projection (`permission.go:204-209`) and preserving an omitted next-launch
  mode when Codex reports an active mode (`runner_events.go:169-176`).
  Agy settlement moves to `runner_events.go:365-366`. The grouped totals
  at the refreshed baseline are 8 descriptor, 37 behavior, 20 candidate
  legacy, and 5 adapter-owned. Codex's omission policy remains a behavior
  migration candidate under (1881).
- **VERIFIED — implementation.** The adapter descriptor method
  (`internal/wsagent/backend.go:32-37`) feeds one ordered, in-memory floor
  (`registration.go:92-151`) and the daemon assembler
  (`cmd/ws-mcp/dashboard.go:304-310`). The eight declarative sites now
  derive from it: daemon registration, search-answer tools, built-in profile
  rows, tracker-support order, keyless compaction, static playbook terms,
  config harness selectors/buckets, and the compaction selector
  (`builtin.go:221-246`, `support.go:236-247`, `catalog.go:568-572`,
  `internal/mcp/playbook_terminology.go:44-60`,
  `internal/mcp/config_registry.go:24-25,151`, `internal/mcp/compaction.go:31-37`).
  Keyless lead and permission defaults also derive from the floor. The Pi
  permission mode reader retains the existing legacy acceptance with
  `TestPermissionModesLegacyFallback` coverage (`registration.go:246-255`).
  Dynamic Claude terminology and runner policy remain assigned to the
  later phases. The new descriptor builders and lookups use memory only
  and introduce zero file operations on the affected paths by source
  inspection. No syscall trace was collected.

**VERIFIED — T0, in both directions.** The baseline and Phase 1 write and
read the same structured override bytes through `config_structured.go:146-168`
and `wsconfig/structured.go:27-64,118-138` at both commits. The new floor
marshals only `BuiltinProfileRegistration.Definition` as the `Profile`
that old and new `profile.go:80-132,222-239` decode. Opening an old
config on Phase 1 and opening a Phase 1 config after a baseline write
resolve project, repo, global, and built-in in the same order
(`profile.go:378-425`, `wsconfig/structured.go:69-100`). Bucketed knobs
write and reopen under the same keys at both commits
(`mcp/server_dispatch.go:481-495`, `mcp/config_registry.go:203-239`).
Both sides read and write the same `Record`, including any embedded
`*Profile`, on reopen (`record.go:30-38,309-335`). The new descriptor
fields live on the in-process registration (`registration.go:64-75`).
An old caller talking to Phase 1 and a Phase 1 caller talking to the baseline
see the same schemas and run the existing dispatch/argument checks
(`mcp/server.go:386-406`, `mcp/tool_arguments.go:65-128` at both commits).
A Go-overlay probe against `684b2c834` compared all built-in profile
JSON, keyless defaults, pre-existing daemon descriptor fields and capability
catalogs, `tools()` JSON, selector order, and terminology for `ws` and
`wsflow`. All three package snapshots were byte-identical.

**VERIFIED — local checks.** Commands run from `agents-plugin-tool/` with
a writable temporary Go cache. Build, vet, formatting, and the focused
suites passed. The two full-suite commands reached tests and stopped at
the sandbox's loopback bind restriction.

```sh
go build ./...
go vet ./internal/wsagent ./internal/mcp ./cmd/ws-mcp
git diff 684b2c834 --relative --name-only -z -- '*.go' | xargs -0 gofmt -l
go test ./internal/wsagent -count=1 -run 'Test(Builtin|LeadProfileForBuiltinFallback|RegistrationChoices|PermissionModesLegacyFallback|RegisteredPermissionMode|.*(Catalog|StreamFacts)Fixture)'
go test ./internal/wsagent -count=1 -run 'Test(ClaudeCatalogValues|CodexCatalog|AgyCatalogValues|BuiltinSupportProfilesResolve|BuiltinAgentWorkerProfiles|BuiltinLeadProfilesResolve|BuiltinSupportProfileOverriddenWhole|SupportLastChoiceIsTheDefaultAndPersists|SupportDefaultSkipsAnUnavailableHarness|SupportChoicesListEachHarness|PermissionModesLegacyFallback|BuiltinBackendRegistration.*|RegistrationChoicesBackendRegistrationsDefensiveCopies)$'
go test ./cmd/ws-mcp -count=1 -run 'Test(AgentRunnerCatalogs|CallFallbackRegistrationAndRenderDescriptor|HostedCLIExistingShapesAndOldHello)$'
go test ./internal/mcp -count=1 -timeout 45m -run 'Test(CompactionRegistryManifestParity|CompactionConfigGetTuneAndLegacy|ConfigRegisteredAgentProfile.*|ConfigTuneAgentProfile|ConfigListAgentProfileKnob|ConfigTuningCatalogProjectsPromptAndSchemaKnobs|ConfigPrompt.*|Config.*Harness.*|ConfigCatalogAgentsTierResetWriter|ServeStdioConfigAgentsTier.*|TerminologyTableCoverage|TermsDifferThreeWay|ReservedToolVarNamesContainsRequiredNames|PlaybookTerminology.*|PlaybookRenderUnrecordedCallerKeepsHarnessRow|PlaybookPrint(Claude|Codex|Unknown|Pi)Harness.*|PlaybookPrintGoldenDelegateSample.*Harness)$'
go test ./internal/wsagent -count=1
go test ./cmd/ws-mcp -count=1
```

The full suites stopped at `TestAgyWorkerLaunchApprovalAndResume`
(`internal/wsagent/agy_worker_test.go:157`) and
`TestAgentMCPHostedWorkerKeepsTokenForwarding`
(`cmd/ws-mcp/agent_mcp_test.go:137`) respectively. Both reported
`listen tcp6 [::1]:0: bind: operation not permitted`. The hosted-launcher
asset test was not reached in that run.

The focused source search combined these patterns across
`internal/wsagent/{builtin,catalog,registration,support}.go`,
`internal/mcp/{compaction,config_registry,playbook_terminology}.go`, and
`cmd/ws-mcp/dashboard.go`:

```sh
rg -n 'builtinRegistration|BuiltinBackendRegistrations|BuiltinHarnessNames|Backend(Claude|Codex|Agy|Pi)|"(claude|codex|agy|pi)"' \
  internal/wsagent/{builtin,catalog,registration,support}.go \
  internal/mcp/{compaction,config_registry,playbook_terminology}.go cmd/ws-mcp/dashboard.go
```

It found one ordered built-in registration floor, the derived daemon
map and enum projections, the legacy Pi mode check, and the existing MCP
Claude and Pi behavior checks.

### Phase 2: Migrate runner, process, permission, and lifecycle behavior

Replace common name checks with Phase 1 data/methods across
`internal/wsagent/permission.go`, `agent_tools.go`, `runner_startup.go`,
`runner_create.go`, `runner_operations.go`,
`runner_process.go`, `runner_events.go`, `runner_summary.go`, `children.go`,
`spawn.go`, `handoff.go`, and `worker_rejoin.go`. Implement behavior in the
matching Claude/Codex/Agy/Pi adapter files. Preserve historical
holder/record/session-file name checks only when their old bytes cannot be
interpreted through registration data ((1881)).

Verification: `internal/wsagent` package tests plus focused fixtures for
permissions, worker admission, cwd/key ownership, holder restart/rejoin,
handoff/takeover, native steer and child revival, Pi summaries, and the Agy
settled-activity callback. Run four-harness lead and worker parity checks and
measure file operations on start, turn settlement, restart, rejoin, and
handoff; the migration must not add interactive-path file I/O.

Compatibility verification ((1883)): prove T0 for record/holder JSON, legacy Pi
session paths, downgrade rejoin, and process behavior with directional writer,
reader, and reopen evidence. Split any newly found T1+ behavior into its own
ticket.

### Phase 3: Migrate MCP detection, prompt terminology, and dashboard consumers

Move reusable connection/prompt capabilities behind the registered descriptor
and remove common harness-name policy from
`internal/mcp/playbook_terminology.go`, `playbook_render.go`,
`summary_owner.go`, `live_snapshot.go`,
`server_harness.go`, `config_registry.go`, and `compaction.go`; and from
`internal/wsdashboard/plan_usage.go`, `plan_usage_probe.go`,
`plan_usage_history.go`, `daemon_runtime.go`, `pageview_hud.go`,
`pageview_rail.go`, `registry.go`, `page_reader.go`, `proxy_http.go`, and
`conversation_presentation.go`. Keep external identifier and persisted-format
parsers only where old bytes or external identifiers cannot be interpreted
through registration data ((1881)); keep Agy usage logic in
`plan_usage_agy.go`.

Verification: `internal/mcp` and `internal/wsdashboard` package tests;
initialize/clientInfo and metadata detection fixtures for all four harnesses
plus unknown clients; prompt terminology/daemon-agent selection fixtures;
HTTP replay, plan-usage history, rail revive, retirement, and peer-lead display
fixtures; and MCP schema/legacy-call checks showing unchanged tool names,
arguments, and default behavior.

Compatibility verification ((1883)): prove T0 acceptance of old `clientInfo`,
calls, config selectors, history files, registrations, and replay names with
directional old/new caller and reader-writer evidence. Split any newly found
T1+ behavior into its own ticket. Tool-list refresh does not prove a removed
call compatible.

### Phase 4: Residual audit and parity, compatibility, and performance gate

Repeat the source inventory on the landed prerequisite head. Search non-test Go
code in the original scope for Claude/Codex/Agy/Pi literals and
`BackendClaude|BackendCodex|BackendAgy|BackendPi`; require zero common-code
behavior branches except the exact (1881) allowlist and declarative registration
rows. Confirm that each common-code survivor parses old persisted bytes or
external identifiers that registration data cannot interpret, and update this
inventory if prerequisite tickets moved a site.

Make the residual gate durable: add a Go source-scan regression test over the
same common-code scope that fails on any harness-name literal or
`BackendClaude|BackendCodex|BackendAgy|BackendPi` branch outside adapter-owned
files, declarative registration rows, and the exact (1881) allowlist, which the
test itself carries. Work that lands after this umbrella (for example the ready
reasoning-summary tickets) then has to route harness differences through the
(1880) seam or extend the allowlist under (1881)'s rule in its own diff.

Verification: the Phase 1-3 package suites together; four-harness hosted lead
and `agent.spawn` worker parity; restart/rejoin/handoff and downgrade fixtures;
MCP legacy-call and detector fixtures; race coverage for touched lifecycle
paths; and syscall/file-operation measurements against the pre-change
baseline. For each retained common-code name check, verify its regression
fixture and site comment naming the preserved compatibility boundary. Record
the residual search command, output, and allowlist in the phase result.

Compatibility verification ((1883)): record the directional T0 verdict across
the implemented phases, citing source lines for each affected old/new writer,
reader/refusal, and reopen path. Split newly discovered T1+ behavior into its
own ticket; a prerequisite's already settled higher tier remains that ticket's
decision.

---
title: "공통 레이어의 하네스 이름 분기를 하네스별 등록 데이터로 이전"
parent: 261114-epic-ws-common-session-foundation
related:
  261118-feat-ws-agy-tool-names-and-worker-role: "(143)=a introduced the BackendRegistration descriptor seam this umbrella extends"
  261118-research-ws-pi-hosted-connector: "stage 2 targets full common-interface support by Pi; this leak inventory is its baseline"
---

# Move harness-name branches out of the common layer into per-harness registration data

## Background

The harness common interface (epic `261114-epic-ws-common-session-foundation`)
is only partly isolated from per-harness implementations. The refreshed source
read used current `develop` at merge `dec0d7d6a` on 2026-11-20 and covered
non-test Go code under `agents-plugin-tool/internal/wsagent`, `internal/mcp`,
`internal/wsdashboard`, and the `cmd/ws-mcp` registration assembly.

The old estimate of about 75 `BackendClaude|BackendPi|BackendAgy` references is
retired. It mixed raw references, omitted Codex and newer dashboard/MCP sites,
and predated the Agy activity hook. The inventory below has **66 grouped
decision sites**: 8 declarative descriptor-data sites, 35 backend-behavior
sites, 19 candidate legacy-compatibility exceptions, and 4 intentionally
adapter-owned sites. A site is one branch or one adjacent table/branch cluster;
all lines in a cluster share one migration decision. The classes route work;
candidate exceptions remain only where old persisted bytes or external
identifiers cannot be interpreted through registration data ((1881)).

### Refreshed inventory

| Class | Site on `develop` at `dec0d7d6a` | Current name-dependent behavior |
|---|---|---|
| declarative descriptor data | `cmd/ws-mcp/dashboard.go:305-322` | Registers the Claude, Pi, Codex, and Agy backends and their current capability values. |
| declarative descriptor data | `internal/wsagent/builtin.go:49` | Selects the Claude versus non-Claude search-answer native tool names. |
| declarative descriptor data | `internal/wsagent/builtin.go:237-315` | Builds support, legacy Pi, agent-worker, search-answer, and lead profile rows from backend-name tables, including a Claude-only support-tool branch. |
| declarative descriptor data | `internal/wsagent/support.go:43-44` | Orders the built-in Claude and Pi tracker-support profiles. |
| declarative descriptor data | `internal/wsagent/catalog.go:571-575` | Supplies keyless compaction fallbacks from a backend-name-to-catalog table plus the Pi descriptor. |
| declarative descriptor data | `internal/mcp/playbook_tools.go:34-53` | Stores Claude, Codex, Pi, and neutral playbook terminology rows. |
| declarative descriptor data | `internal/mcp/config_registry.go:24-25,151` | Repeats the closed harness lists used by prompt selectors, agent-tier selectors, and stored harness buckets. |
| declarative descriptor data | `internal/mcp/compaction.go:34` | Repeats the closed harness enum for compaction tuning. |
| backend behavior method | `internal/wsagent/permission.go:39-50` | Maps common permission levels to Claude native permission modes. |
| backend behavior method | `internal/wsagent/permission.go:54-68` | Derives legacy common permission levels differently for Pi, Claude, and other backends. |
| backend behavior method | `internal/wsagent/permission.go:72-75,180-182` | Labels Claude plan-mode enforcement and its pending-observation diagnostic. |
| backend behavior method | `internal/wsagent/permission.go:95` | Admits common lead permissions only for Claude and Pi despite registration metadata. |
| backend behavior method | `internal/wsagent/permission.go:233-235` | Applies Agy's user-only bypass transition inside common permission handling. |
| backend behavior method | `internal/wsagent/permission.go:278-297` | Reconciles an explicit selection only for Claude native modes. |
| backend behavior method | `internal/wsagent/agent_tools.go:250-254` | Runs the extension child check only for Pi spawn admission. |
| backend behavior method | `internal/wsagent/runner_events.go:359-361` | Calls `AgyActivity` after a non-replay, non-compaction Agy turn settles; added by merge `dec0d7d6a`. |
| backend behavior method | `internal/wsagent/runner_summary.go:59-68` | Starts and projects terminal reasoning summaries only for hosted Pi leads. |
| backend behavior method | `internal/wsagent/runner_process.go:70-74` | Rechecks the parent capability ceiling only for a fresh Pi worker start. |
| backend behavior method | `internal/wsagent/runner_process.go:97-99` | Binds a Pi worker key to its working directory rather than the profile root. |
| backend behavior method | `internal/wsagent/runner_process.go:200-205` | Persists the checked launch definition only for fresh Pi workers. |
| backend behavior method | `internal/wsagent/runner_process.go:289-297` | Composes hosted lead instructions for Claude, Agy, and Codex while Pi obtains them through its extension/bridge. |
| backend behavior method | `internal/wsagent/runner_process.go:391-398` | Marks unfinished Agy/Codex HTTP-only starts stale on attach. |
| backend behavior method | `internal/wsagent/runner_process.go:401-419` | Selects HTTP MCP for Claude/Agy/Codex and additionally supplies Claude's stdio fallback. |
| backend behavior method | `internal/wsagent/runner.go:915-917` | Mints Pi spawned-session keys from `spawn.Cwd`. |
| backend behavior method | `internal/wsagent/runner.go:1201` | Marks spawned Pi-child input as a native steer. |
| backend behavior method | `internal/wsagent/children.go:605-608` | Drains marked inbox messages through Pi's native steer path. |
| backend behavior method | `internal/wsagent/spawn.go:178-197` | Enforces the parent launched-profile ceiling only for native Pi child profiles. |
| backend behavior method | `internal/wsagent/spawn.go:225-238` | Rechecks a Pi child spawned under a lead against a concurrent parent close. |
| backend behavior method | `internal/wsagent/handoff.go:83-85` | Refuses terminal takeover for Codex. |
| backend behavior method | `internal/wsagent/handoff.go:98-102` | Requires a regular session file before taking over Pi. |
| backend behavior method | `internal/wsagent/handoff.go:158-168` | Clears non-Claude native permission mode and stores Pi's file identity versus other backends' session ID. |
| backend behavior method | `internal/wsagent/handoff.go:204-206` | Refuses terminal handoff for Codex. |
| backend behavior method | `internal/wsagent/handoff.go:307-327` | Builds Claude, Agy, and Pi terminal resume commands. |
| backend behavior method | `internal/wsagent/backend.go:543-547` | Gives a Claude-specific Windows cmd-shim remediation. |
| backend behavior method | `internal/mcp/playbook_tools.go:102-111` | Computes Claude's namespace-dependent spawn idiom outside the terminology row. |
| backend behavior method | `internal/mcp/playbook_tools.go:163-180` | Detects Claude native-subagent removal and Pi MCP-only records by backend name. |
| backend behavior method | `internal/mcp/playbook_tools.go:1042` | Treats Pi client metadata as a daemon-agent capability alongside descriptor and record evidence. |
| backend behavior method | `internal/mcp/summary_owner.go:18` | Enables terminal summary intake only for Pi. |
| backend behavior method | `internal/mcp/live_snapshot.go:527` | Accepts negotiated reasoning-summary events only from the Pi adapter. |
| backend behavior method | `internal/wsdashboard/plan_usage.go:194` | Accepts pushed usage only from Claude live snapshots. |
| backend behavior method | `internal/wsdashboard/plan_usage.go:431` and `internal/wsdashboard/daemon.go:653` | Scans only Claude sessions for pushed usage snapshots. |
| backend behavior method | `internal/wsdashboard/plan_usage.go:445` | Adds the separate Agy history projection only when an Agy usage entry exists. |
| backend behavior method | `internal/wsdashboard/pageview_rail.go:260` | Offers terminal takeover in the rail only for Claude/Pi registrations. |
| legacy-compatibility exception | `internal/wsagent/runner.go:474` | Preserves old direct non-Pi orphan admission while held workers use the former Pi cleanup. |
| legacy-compatibility exception | `internal/wsagent/runner.go:884` | Retains the legacy `pi.spawn` rule that a tiered non-worker profile must be Pi. |
| legacy-compatibility exception | `internal/wsagent/runner.go:1166-1174` | Retries a send by reopening a closed spawned Pi child. |
| legacy-compatibility exception | `internal/wsagent/children.go:366-389` | Revives only closed Pi children and validates their historical session file. |
| legacy-compatibility exception | `internal/wsagent/children.go:833` | Treats legacy Pi children under lead parents as descendants alongside common agent-worker profiles. |
| legacy-compatibility exception | `internal/wsagent/worker_rejoin.go:15` | Accepts the legacy `holderKindPiWorker` descriptor with an absent/Pi backend. |
| legacy-compatibility exception | `internal/wsagent/runner_process.go:103-108` | Falls back to the root-bound key written by 0.49.x Pi children. |
| legacy-compatibility exception | `internal/wsagent/runner_process.go:512-519` | Reattaches Pi from the persisted launch definition while older held records best-effort resolve their profile. |
| legacy-compatibility exception | `internal/wsagent/registration.go:135-148` | Accepts legacy Pi permission values and supplies keyless Claude/Agy/Codex permission-mode fallbacks. |
| legacy-compatibility exception | `internal/mcp/server_harness.go:89-96` | Recognizes the Pi extension's exact `ws-pi-bridge` client name before descriptor lookup. |
| legacy-compatibility exception | `internal/mcp/server_harness.go:105-110` | Detects Codex and Claude from historical raw initialize substrings. |
| legacy-compatibility exception | `internal/mcp/server_harness.go:115-117` | Infers Codex from `x-codex-turn-metadata`. |
| legacy-compatibility exception | `internal/mcp/server_harness.go:127-145` | Normalizes the built-in Codex/Claude/Pi names before accepting registered manual-harness names. |
| legacy-compatibility exception | `internal/wsdashboard/registry.go:455-458` | Protects a handed-off Pi conversation stored inside the daemon session directory from retirement deletion. |
| legacy-compatibility exception | `internal/wsdashboard/registry.go:481` and `internal/wsdashboard/pages.go:1169` | Checks and watches the historical Pi session-file location for child revival. |
| legacy-compatibility exception | `internal/wsdashboard/plan_usage_history.go:60-62` | Validates the persisted Claude/Codex history source and window combinations. |
| legacy-compatibility exception | `internal/wsdashboard/plan_usage_history.go:374` | Excludes Agy entries from the older shared history projection because Agy has its own grouped history. |
| legacy-compatibility exception | `internal/wsdashboard/proxy_http.go:270-277` | Replays harness-specific MCP `clientInfo.name` values for Pi, Claude, Codex, and Agy. |
| legacy-compatibility exception | `internal/wsdashboard/conversation_presentation.go:69` | Recognizes stored peer-lead prefixes containing the historical Claude/Codex/Pi labels. |
| intentionally adapter-owned | `internal/wsagent/agy_bypass.go:16,64` | Keeps Agy bypass storage/native-mode translation inside the Agy adapter helper. |
| intentionally adapter-owned | `internal/wsagent/agy_watchdog.go:86,134` | Keeps Agy model-wait loading and arming inside the Agy watchdog. |
| intentionally adapter-owned | `internal/wsagent/pi.go:335` | Keeps Pi extension/MCP-only classification inside `PiMCPOnly`. |
| intentionally adapter-owned | `internal/wsdashboard/plan_usage_agy.go:63,568` | Keeps Agy-specific grouped usage/history validation inside the Agy usage provider. |

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

- Manuals for the touched paths: `ai-docs/manuals/performance.md`,
  `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/ws-mcp.md`, and
  `ai-docs/manuals/shipped-surface-boundary.md` where shipped surfaces change.
- Before promotion, refresh the parent epic's child list and name the exact
  migration-relevant prerequisite tickets that can add or move inventory sites.
  Keep this ticket in `todo/` until that list is explicit and has landed; do
  not gate on unrelated epic children ((1882)).
- Preserve Decision (144) and use the descriptor seam already established by
  `261118-feat-ws-agy-tool-names-and-worker-role`; do not add a parallel
  harness registry. Put declarative differences on `BackendRegistration` and
  executable differences behind narrow `Backend` methods ((1880)).
- Genuine behavior remains in backend/adapter-owned files. Common code may use
  registration data or backend methods but may not branch on a harness name.
- Retain a common-code name branch only when old persisted bytes or external
  identifiers cannot be interpreted through registration data. Each survivor
  needs a regression fixture and a site comment naming that boundary ((1881)).
- Go tests and harness parity runs execute on remote w5090 only. Follow the
  performance manual's file-operation budget and measure the affected daemon
  lifecycle paths there.
- A compatibility verdict is directional and source-backed: identify the old
  and new writer/reader, the write or refusal, and the new-version reopen path.
  Keep this umbrella at T0 unless a prerequisite already settled a higher tier;
  split newly discovered T1+ behavior into its own ticket ((1883)). Do not
  infer T2 from retained append-only data alone.

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
shows no new parallel harness catalog; and w5090 Go tests for `internal/wsagent`
and `cmd/ws-mcp` packages affected by the final design. Record the explicit
prerequisite list and inventory refresh against the parent epic ((1882)).

Compatibility verification ((1883)): show that additions are runtime-only and
invisible to MCP/config/store readers (T0). If a directional old/new
reader-writer trace reveals T1+ behavior, split it into its own ticket.

### Phase 2: Migrate runner, process, permission, and lifecycle behavior

Replace common name checks with Phase 1 data/methods across
`internal/wsagent/permission.go`, `agent_tools.go`, `runner.go`,
`runner_process.go`, `runner_events.go`, `runner_summary.go`, `children.go`,
`spawn.go`, `handoff.go`, and `worker_rejoin.go`. Implement behavior in the
matching Claude/Codex/Agy/Pi adapter files. Preserve historical
holder/record/session-file name checks only when their old bytes cannot be
interpreted through registration data ((1881)).

Verification: w5090 `internal/wsagent` package tests plus focused fixtures for
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
`internal/mcp/playbook_tools.go`, `summary_owner.go`, `live_snapshot.go`,
`server_harness.go`, `config_registry.go`, and `compaction.go`; and from
`internal/wsdashboard/plan_usage.go`, `plan_usage_history.go`, `daemon.go`,
`pageview_rail.go`, `registry.go`, `pages.go`, `proxy_http.go`, and
`conversation_presentation.go`. Keep external identifier and persisted-format
parsers only where old bytes or external identifiers cannot be interpreted
through registration data ((1881)); keep Agy usage logic in
`plan_usage_agy.go`.

Verification: w5090 `internal/mcp` and `internal/wsdashboard` package tests;
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

Verification: the Phase 1-3 package suites together; four-harness hosted lead
and `agent.spawn` worker parity; restart/rejoin/handoff and downgrade fixtures;
MCP legacy-call and detector fixtures; race coverage for touched lifecycle
paths; and w5090 syscall/file-operation measurements against the pre-change
baseline. For each retained common-code name check, verify its regression
fixture and site comment naming the preserved compatibility boundary. Record
the residual search command, output, and allowlist in the phase result.

Compatibility verification ((1883)): record the directional T0 verdict across
the implemented phases, citing source lines for each affected old/new writer,
reader/refusal, and reopen path. Split newly discovered T1+ behavior into its
own ticket; a prerequisite's already settled higher tier remains that ticket's
decision.

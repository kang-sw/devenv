---
title: Codex hosted adapter range-versioned catalog contract and bounded lead plan
parent: 261114-epic-ws-common-session-foundation
related:
  261110-epic-ws-dashboard-agent-host: "preserve hosted lead ownership, trusted-origin approval and holder survival"
  261114-refactor-ws-common-session-interface: "done; its Phase 2 prerequisite is satisfied (the earlier blocked-by is removed)"
  261117-feat-ws-harness-capability-catalog: prerequisite
  261117-feat-ws-hosted-antigravity-adapter: "next stage-3 harness, ordered after Codex"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1f03b99ebfd35647
sage-review-completeness-reviewed: 1f03b99ebfd35647
---

# Codex hosted adapter contract and first lead slice

> Revised 2026-11-17 under the hosted-first plan (epic `261114-epic-ws-common-session-foundation`, stage 3): the exact-version contract below is replaced by the range-based catalog version policy; Codex is the first stage-3 harness, and the Pi hosted backend will be ported from this adapter (25).

## Background

Codex already has ws plugin/MCP, configuration and mailbox-hook support, but that is not a daemon-hosted conversation backend. The former short app-server account/rate-limit usage probe is no longer in the tree: Codex usage acquisition moved to file-selected OAuth REST at `4c50ad056` (2026-11-16, an ancestor of this branch), and no non-test Go file under `agents-plugin-tool/internal` mentions `app-server` (search returned nothing); it was never a sustained conversation owner. There is no Codex `Backend` / `Process` under `agents-plugin-tool/internal/wsagent` (no codex file there; checked 2026-11-17 on `track/agent-v2`).

The ticket was first captured on 2026-11-14 as a docs-only plan while the owner was asleep, with (799)=a assumed rather than confirmed. The foundation prerequisite (the common interface's Phase 2) has since landed. Under the hosted-first plan the capability/protocol/fixture contract is expressed as capability-catalog values per `harness@version` range; the first implementation is one hosted lead conversation, after the catalog ticket; execution is approved (Decision 4).

(799)=a was "assumed while the owner was asleep" and was not confirmed (03). Owner 2026-11-17 22:10 answered (03): the protocol is the Codex **app-server** (sustained JSON-RPC control), and (70)=b moved Codex and Antigravity ahead of the single session holder, approving execution of this ticket now (with (66)=a, (67)=a). The installed CLI on the home machine is `codex-cli 0.160.0`.

## Decisions

1. **Reuse the current execution boundary.** A future adapter attaches at `Backend.Start` / `Process` / normalized events beneath the existing Runner and holder. Do not create a second daemon/runtime, use `execjob` as a conversation owner, or duplicate mailbox/input delivery.
2. **Protocol: app-server (owner (03), 2026-11-17 22:10).** The adapter drives a sustained `codex app-server` process over its JSON-RPC protocol. Rejected: per-invocation `exec --json` / `exec resume` (no sustained bidirectional control for interrupt, approvals or mid-turn delivery). Method signatures and the instruction-injection mechanism are pinned in Phase 1 from the installed version and its official documentation; for every claimed capability record the version range it holds for, the build/platform evidence and the matching documentation.
3. **Bound the first implementation to one hosted lead conversation.** The intended first slice is create/resume/send/interrupt, ordered normalized output and holder reattach. Approval handling must preserve supported decisions and stable correlation rather than silently approve or reinterpret a richer choice. Unsupported handoff, permission modes, steer and native-subagent telemetry must remain explicit until their exact support is settled.
4. **Gates.** `261114-refactor-ws-common-session-interface#phase2` has landed. Execution approval was granted by the owner on 2026-11-17 ((70)=b, (66)=a). Phase 1 maps Codex values onto the catalog item list already settled in the catalog ticket's Decision 3 and may run beside the catalog's Phase 1; Phase 2 depends on the catalog's Phases 1-3 (schema, HTTP MCP admission, startup check).
5. **Range-based version policy replaces the exact-version contract (20)=a, (21)=a, (22).** Codex values live in the catalog per `harness@version` with `since` / `until` ranges; version branches in adapter code only where data cannot express the difference (protocol translation). At session start read the version and lightly check the init shape, demoting mismatched capabilities to `unsupported`. Verification is per-version recorded stream fixtures; the guaranteed upper bound is the newest version with a fixture. Out of range, above or below the lower bound, allow loudly with the nearest-version profile, an "unverified version" dashboard warning and an error only on actual failure.
6. **Stage ordering (25).** Claude is the reference, then Codex (this ticket), then Antigravity, then Pi. The Pi hosted backend is a port of this Codex adapter and catalog profile (Pi is driven only with GPT models), not a separate Pi-first design.
7. **Delegation is `ws/agent.*` (06).** Hosted sessions delegate through `ws/agent.*`, not native subagents. Owner (08)=c: whether a harness's native subagents can be removed is decided per harness at stage 3, so Phase 1 probes and records it for Codex.
8. **Compatibility target: T0.** Preserve current Pi/Claude controls, native integrations, identities, readable state and existing API/holder/event semantics. Phase 1 records the concrete tier verdict for every new descriptor/config/serialization surface in its `### Result`, with source lines, including an old daemon reopening a hosted Record whose new Codex `Backend` value it does not know. T2, root-key policy changes and a store/protocol rewrite are not authorized.

9. **Phase 2 settlements of the Phase 1 gaps (owner, 2026-11-18).**
   - (101)=a, E1: approvals stay binary. Allow maps to `accept` and deny to
     `decline` (`{permissions:{}, scope:"turn"}` for a permissions request),
     with stable correlation. `acceptForSession`, the policy amendments,
     `cancel` and elicitation `persist` are reported unsupported, never
     mapped onto another choice. Rejected: (b) offered choices on
     `ApprovalRequest` with a dashboard UI change in Phase 2, a cross-harness
     change outside this slice. The owner plans permission bypass as the
     usual default for Codex and agy, with a later ws-side auto-mode hook
     (for example a decision model) rather than a richer manual choice set;
     the richer set is follow-up work, not this ticket.
   - (95), E2: `item/tool/requestUserInput` is answered with an error
     telling the model to write the question as plain chat text and end the
     turn, the same wording the Claude hosted lead uses for `AskUserQuestion`
     (`agents-plugin-tool/internal/wsagent/claude.go` L616-621, L1311); the
     dashboard has no question form.
   - (96): an old daemon reopening a hosted Record with `backend: "codex"` is
     T1, the Phase 1 verdict above; T1 permits a patch.
   - (97), G1: the version comes from `thread.cliVersion`;
     `parseHarnessVersion` is not widened to strip `codex-cli `. Rejected:
     widening the shared parser, which touches every harness for a value the
     protocol already supplies.
   - (98), G2: the catalog's `usage_query` text is reworded to "ws obtains
     the harness's usage (asked or pushed)", and Codex grades native by push.
     Rejected: a separate push-only item.
   - G3 follows (101)=a and (95): no catalog slot for the approval decision
     set or the user-question request, since neither adds a capability the
     adapter offers.
   - (99): the ws plugin's stdio MCP is blocked by shadowing - the HTTP entry
     is injected under the plugin's server name (`ws`; `wsflow` for that
     package). Rejected: `plugins."<id>".enabled=false`, which drops the
     plugin's skills and hooks and needs the installation-specific key.
     Phase 2 verifies by process count that no stdio `ws-mcp` is spawned.
   - (100): Codex hooks stay user-controlled (Claude-lead parity); the
     adapter does not set `features.hooks=false`. Rejected: disabling hooks,
     which silently drops user-configured behavior only in hosted leads.

## Capability Matrix (range-based)

Current target protocol: **Codex app-server** (Decision 2); installed version `codex-cli 0.160.0` (`codex --version`). The supported version range and per-row evidence are **not yet verified** (Phase 1). This is the finite contract matrix to complete, not a declaration of API support. Each completed row maps to a catalog item and carries the version range (`since` / `until`) it holds for, official contract citation, fixture identity and `native` / `ws-substitute` / `unsupported` classification. Historical versions below are evidence labels, not the chosen hosted target.

| Capability | Recorded evidence and version boundary | Required contract / fixture before readiness |
|---|---|---|
| Process protocol | `ai-docs/manuals/codex-integration.md` records `exec --json` / `exec resume`; their original invocation notes are not tied to a new hosted target. No Go app-server client remains in the tree (usage moved to OAuth REST, `4c50ad056`); the installed `codex app-server` exists (`codex app-server --help`: default `--listen stdio://`, `generate-json-schema` / `generate-ts` subcommands), so nothing about its sustained protocol is proven by existing ws code. | Pin app-server framing, initialization handshake and sustained bidirectional control at the installed version (protocol already settled, Decision 2). |
| Create / resume / attach | Recorded exec notes describe CLI-assigned thread IDs and nonexistent-thread failure. No hosted holder/session adoption is proven. | New/resumed thread identity, existing holder reattach without duplicate launch, missing/invalid session and changed-version resume fixtures. |
| Send / interrupt / steer | The 2026-09-13 Codex **0.154.0** macOS re-probe found `PostToolUse` side-effect-only; Stop-hook reentry is a turn-boundary path, not active-turn steer. | Pin real send/cancel controls; distinguish next-turn queueing, active-turn steer, cancellation, turn end and process exit. Unsupported steer is not fabricated support. |
| Approvals / pause | Existing manual bypass flags are probe history, not production policy. No current hosted approval contract is established. | Request/answer/reject/pause IDs, richer choice mapping, duplicate/stale answers and trusted-origin checks; settle any loss of expressiveness. |
| Ordered events / recovery | Recorded `--json` stdout differs from rollout JSONL. Native transcript parsing is not a live event mapper. | Content/tool/usage/error lifecycle, ordering, unknown messages, EOF, replay/gaps, reconnect and queued-input release fixtures. |
| Config home / authentication | The **0.154.0** re-probe says `--ignore-user-config` still uses `CODEX_HOME` for authentication. Account-slot support is not implemented by this plan. | Correct harness-owned config/account home, scrubbed inherited ws identity, MCP/key/physical-PID adoption and isolation; no login or credential mutation now. |
| Instructions / workflow access | The local manual records `model_instructions_file` replacement semantics, not the other adapters' append-system-prompt contract. | Pin how approved workflow/context reaches the session without silently replacing native defaults; preserve the adopted common access boundary. |
| Subagents / idle safety | The **0.154.0** ephemeral re-probe did not establish daemon-backed subagent hooks. Missing activity is not known zero. | Supported producer/parent/status evidence or explicit unknown/unsupported capability; conservative idle behavior until actual running-state evidence exists. |
| Model / effort / UI admission | Common-session Phase 1 is being refined for registration/admission/profile/choice metadata. Codex model names are not the harness shorthand. | Capability-consistent defaults, choices and unsupported-operation behavior through the settled descriptor; no hard-coded claim of full Pi/Claude parity. |

## Constraints

- Probes and provider calls against the installed `codex` CLI and the owner's existing login are authorized (owner 2026-11-17 22:10). Do not log in, log out, switch accounts or mutate credentials or the user's Codex config home; run probes with an isolated or read-only configuration where the protocol allows.
- Consult exact-version official documentation for the installed version. Existing reports/manuals are supporting prior evidence; do not present them as current installed-binary or account-backed acceptance.
- Do not use `PostToolUse` to implement mid-turn delivery on the contradicted 0.154.0 assumption, reuse dangerous bypass flags as approval policy, or treat rollout-file events as stdout protocol events.
- Keep ws identity, parent lineage, single input/mail ownership, approval correlation and holder replay/recovery in their current owners. One lead slice does not authorize daemon-spawned native children or account switching.
- Implementation manuals later include `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/release-compatibility.md` and `ai-docs/manuals/ws-mcp.md` where applicable; authored/mirrored surfaces retain their matching manuals.
- Go test location (owner (68), 2026-11-17 22:10): run Go tests locally on the macOS home machine. The earlier "remote w5090 only" rule belonged to the owner's company Windows setup and does not apply here.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches at the root; files for backup/restore only; deltas (append) not rewrites; no heartbeat rewrites; interactive paths never wait on file I/O.
- No main push, version bump or release. Work lands on `track/agent-v2` through the lead. Replacing the shared daemon on this machine is the lead's call, not a worker's.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Art

- `ai-docs/manuals/codex-integration.md`: existing plugin/MCP operations, distinct stdout/rollout formats, instruction replacement, and explicitly versioned 0.154.0 hook re-probe superseding 0.128.0 steering claims.
- `agents-plugin-tool/internal/wsdashboard/plan_usage_probe.go`: Codex usage probe; it now reads usage over file-selected OAuth REST (`plan_usage_probe.go#L215`, commit `4c50ad056`), so it is no longer app-server prior art. `plan_usage_cmd_unix.go` does not exist in the tree (`git ls-files | grep plan_usage_cmd` returned nothing; it was removed with the app-server launcher).
- A 2026-11-14 harness-readiness inventory (`shared/docs/harness-interface-readiness.md` at `951af006a`) is not in this repository; do not depend on it. Re-ground boundaries from current source.

## Prior Decisions

- 261117-feat-ws-harness-capability-catalog (2026-11-17, Decisions): "Admission conditions. A harness is unusable without them: (0) headless JSON event streaming (the adapter converts the format); (3) HTTP MCP server injection (24)." — bearing: supports
- 261116-research-ws-hosted-session-first-tier (2026-11-16, Confirmed Decisions): "(25) Implementation and verification order is Claude, Codex, Antigravity, then Pi; Pi is driven only with GPT models, so the Pi hosted backend is a port of the Codex adapter" — bearing: supports
- 261114-epic-ws-common-session-foundation (2026-11-14, Cross-Child Decisions): "Codex and Antigravity are both tier-1 hosted harnesses (02). (799)=a for Codex was assumed while the owner was asleep and is not confirmed (03)" — bearing: supports
- 261117-feat-ws-hosted-antigravity-adapter (2026-11-17, Decisions): "Stage-3 child, ordered after Codex (25), ahead of the holder ((70)=b). Implementation and verification order is Claude, Codex, Antigravity, then Pi." — bearing: constrains
- 261116-bug-ws-exe-launch-elimination 74924b4c (2026-11-16, commit): "item 4 (codex app-server -> REST, (1207)=a) stays parked at parked/exe-phase1-item4-codex d9fd94543 and follows after the swap acceptance." — bearing: constrains
- 0086eb14 (2026-09-13, commit): "Left open: whether Codex subagent turns fire hooks at all — the isolated ephemeral exec could not spawn a subagent (needs the shared app-server daemon); flagged for a daemon-backed probe." — bearing: supports
- c6267c3f (2026-11-15, commit): "Explicitly retain an unexpected side effect: cleanup tracked a codex app-server process parented by the baseline fixture despite intended provider isolation." — bearing: constrains
- 261113-feat-ws-dashboard-rail-usage-gutter (2026-11-13, Decisions): "primary source is a short-lived `codex app-server` process answering `account/rateLimits/read` (the Codex CLI manages its own token)" — bearing: context only; superseded by OAuth REST at `4c50ad056`, so it is not working app-server prior art

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 2 touches agents-plugin-tool/internal/wsagent (new Codex backend, fixtures), the catalog profile and the startup check; Phase 1 edits ai-docs manual/ticket evidence |
| scope.surface | cross-module | Backend/Process boundary at agents-plugin-tool/internal/wsagent/backend.go#L21-L28 and #L99-L116, plus Runner, holder and the capability catalog |
| scope.new_public_symbol | yes | a Codex Backend registered beside the existing backends; the exported name is not named in the ticket |
| scope.new_type_contract | yes | Codex values in the harness capability catalog per harness@version range and a Process implementation of the existing interface; no exact signature named yet |
| scope.test_surface | new-files | recorded app-server stream fixtures and adapter tests; precedent agents-plugin-tool/internal/wsagent/testdata/pi-tool-turn.jsonl; no Codex test or fixture exists (search of wsagent for codex files returned only test mentions) |
| complexity.reuse_points | confirmed | existing Backend/Process interface in agents-plugin-tool/internal/wsagent/backend.go read at #L21-L28 and #L99-L116 |
| complexity.side_effect_risk | high | spawns a sustained external process that owns approvals, input delivery and holder replay beneath the live Runner |
| risk.correctness | high | ordered event mapping, interrupt versus turn end, reattach without duplicate input, and unverified 0.160.0 protocol details |
| risk.fit | moderate | the Backend/Process boundary exists, but app-server JSON-RPC is bidirectional and may not fit the one-way Events/Send shape without extension |
| risk.test | high | acceptance needs a real hosted Codex conversation with the owner's login; fixtures must be recorded from the installed CLI |
| risk.security_or_contract | high | approval correlation and trusted-origin checks, credential and CODEX_HOME handling, and catalog compatibility tier (T0 target) |

## Phases

### Phase 1: Settle the range-based capability catalog values and fixture contract

With the app-server protocol settled (Decision 2) and the installed `codex-cli 0.160.0`: probe the app-server and complete the finite matrix with the selected version range, official contract evidence and recorded stream fixtures. Map each row to a catalog item (catalog Decision 3) with its grade and `since` / `until`. Cover thread create/resume/send/interrupt/optional steer, permissions, normalized event order, holder recovery, config/account home and workflow access. Check both admission conditions (headless JSON event streaming; HTTP MCP server injection) and decide native subagent removability (08)=c. Keep unsupported operations and unresolved telemetry visible. Return choices that change product/permission/compatibility contracts to the lead rather than selecting an API from a different version or assuming parity. If app-server's bidirectional JSON-RPC (server-initiated approval requests, for example) does not fit the existing `Backend` / `Process` shape, describe the needed extension and return it to the lead; do not extend the common interface in this phase.

Deliverables: the completed matrix, the admission evidence, the removability verdict and the compatibility verdict (Decision 8) go in this phase's `### Result`; recorded fixtures go under `agents-plugin-tool/internal/wsagent/testdata/` beside the existing Pi fixture.

Verification: every claimed supported capability has a matching version-ranged source and a recorded fixture or a finite planned-fixture expectation, with explicit unknowns; planned fixtures are distinguished from recorded ones; both admission conditions carry probe evidence.

### Result (6a44e921d) - 2026-11-17

Probed the installed `codex-cli 0.160.0` (macOS arm64, the owner's ChatGPT
login, default `CODEX_HOME`) by driving `codex app-server` (default
`--listen stdio://`) from a stdlib Python client, with a loopback
streamable-HTTP MCP server standing in for the daemon. No login, logout,
account switch, credential or `config.toml` write. Probe threads ran in a
scratch git repo under an already-trusted home path, and the probe client
removed `WS_*`/`CLAUDE*`/`ORCA_*`/`PI_*` from the child environment.

**Evidence sources.**
- Exact-version contract: `codex app-server generate-json-schema` and
  `generate-ts` from the installed binary. Official documentation states
  that this output is "specific to the Codex version you ran".
- Official prose: `developers.openai.com/codex/app-server` (redirects to
  `learn.chatgpt.com/docs/app-server`) and `.../codex/config-reference`
  (redirects to `learn.chatgpt.com/docs/config-file/config-reference`),
  fetched 2026-11-17.
- Recorded fixtures: `agents-plugin-tool/internal/wsagent/testdata/codex-0.160.0-*.jsonl`
  (nine files). They use the claude-lead `>> ` client-line convention, and
  `codex_fixture_test.go` pins the protocol facts cited below.
- Probe-side observations that are not in the fixtures carry the label
  *(probe log)*. They come from the probe MCP server's request log and a
  grep of the rollout file, which were not committed because they hold the
  bearer token.

**Fixture index (recorded).**
- `handshake`: a request before `initialize` gets `-32600 Not initialized`, and a second `initialize` gets `Already initialized`.
- `lead-session`: covers thread/start with HTTP MCP and `developerInstructions`, then six turns:
  - T1: MCP tool approval through `mcpServer/elicitation/request`, declined.
  - T2: command approval, declined.
  - T3: `turn/interrupt` during `sleep 30`.
  - T4: steer (stale id error, accepted steer, ZEBRA in the reply).
  - Steer while idle (error).
  - T6: tool self-report. During T6 the interrupted T3 item also completes late.
- `resume`: covers these cases:
  - Resume of a missing thread fails.
  - Resume with a re-issued token works. The fixture shows a resume under the same `bearer_token_env_var` name; that the value was new is *(probe log)*.
  - `developerInstructions` persist across the resume.
  - An accepted MCP elicitation, followed by the tool result.
  - `features.multi_agent=false` on resume does not remove subagents: `subAgentActivity` and `collabAgentToolCall` items still appear.
  - `thread/turns/list` works.
  - A required HTTP MCP server answering 401 makes `thread/start` fail.
- `stale-approval`: resume with `agents.enabled=false` (answer `NO-SUBAGENT-TOOL`). An interrupt while an approval waits gives `turn/completed` (interrupted), then `serverRequest/resolved`. The recording ends with two late answers. That they got no reply and created no file is *(probe log)*.
- `subagent-baseline` (ephemeral; spawns one), `subagent-removed` (`agents.enabled=false`), `multi-agent-feature-off` (`features.multi_agent=false` plus `features.shell_tool=false`: no shell; the subagent tool is still advertised but spawn fails).
- `ws-mcp-shadow`: a thread `mcp_servers.ws` HTTP entry replaces the plugin's `ws` server for that thread. The stream shows one `ws` `starting` and two `ready` events. That the HTTP server took the handshake is *(probe log)*.
- `plugin-mcp-block`: bare `mcp_servers.ws.enabled=false` is refused with `invalid transport`. Disabling the plugin drops the `ws` startup.

**Admission conditions (catalog Decision 2): both pass at 0.160.0.**
- (0) Headless JSON event streaming: app-server stdio is newline-delimited JSON-RPC 2.0 with the `jsonrpc` member omitted on the wire (all fixtures). It is sustained and bidirectional: client requests and notifications, plus server notifications and server requests.
- (3) HTTP MCP injection: `thread/start` / `thread/resume` `config.mcp_servers.<name> = {url, bearer_token_env_var, required: true}`. The probe server saw `Authorization: Bearer` on every POST with Host `127.0.0.1` *(probe log)*.
  - A wrong token on a `required` server fails `thread/start` with `-32603 ... required MCP servers failed to initialize ... HTTP 401` (`resume`), so the failure is loud.
  - A re-issued token on the next process start works (`resume`).
  - Neither the token nor the MCP url is written to the rollout (grep of the thread's rollout file: 0 hits, *(probe log)*), so the MCP config is re-passed on every resume.
  - Static `http_headers` also exist (doc), but they would carry the token in config. `bearer_token_env_var` keeps it in the child environment only.

**Capability matrix: catalog values for the Phase 2 Codex profile.**
- Shape: verbatim the `catalog.go` schema landed at `d20202cd6` (branch `impl/track/agent-v2/rust-stank-cedar`, read only).
- Profile range: `Oldest: "0.160.0", Newest: "0.160.0"`, the only version with fixtures. Entries carry no `Since`/`Until`, like `ClaudeCatalog`. `Resolve` already pins out-of-range versions to the nearest bound, marked unverified.
- Version source: `thread/start`/`thread/resume` response `thread.cliVersion` (`"0.160.0"`). The raw `codex --version` line `codex-cli 0.160.0` does not parse with `parseHarnessVersion`, which cuts at the first space.
- Roles: every entry is `Role: RoleLead`. Worker is left undeclared, so it resolves `unsupported`, because Phase 2 is lead-only (C2).
- Grades follow catalog Decision 10 (what the ws adapter will deliver once Phase 2 wires it). Until Phase 2 registers the backend, there is no Codex profile.

| Item | Grade (lead) | Values | Evidence (0.160.0) | Fixture |
|---|---|---|---|---|
| `skill_list` | unsupported | - | `skills/list` exists (schema). No ws wiring in the lead slice. | schema only |
| `skill_injection` | native | - | Codex plugin skills, which stay loaded under the recommended block mechanism B (below). Under mechanism A it becomes `unsupported` until prompt assembly. `UserInput {type:"skill"}` exists (schema). | planned: Phase 2 lead with the plugin loaded |
| `compaction` | native | - | Planned, not probed: `thread/compact/start` request + `thread/compacted` notification (schema). | planned: `/compact` -> `thread/compact/start` -> `thread/compacted` -> turn end |
| `rewind` | unsupported | - | `thread/revert` exists (schema). `thread/rollback` was removed (doc). No ws wiring. | none |
| `tool_removal` | unsupported | - | The lead keeps the user's tool setup, as `ClaudeCatalog` does. Removable set, for a later worker row: `shell` (`features.shell_tool=false`; `multi-agent-feature-off`: `NO-SHELL-TOOL`, no `commandExecution` item), `web_search` (`web_search="disabled"`, doc), `view_image` (`features.view_image`, doc). | recorded (shell); others doc only |
| `subagent_removal` | native | `spawn_agent` (the `collaboration` namespace: `spawn_agent`, `send_message`, `wait_agent`, `followup_task`, `interrupt_agent`, `list_agents`) | `agents.enabled=false` in `thread/start` and in `thread/resume` config removes it. `features.multi_agent=false` (thread config or `--disable multi_agent`) does **not**: the tool stays advertised and spawn fails. | `subagent-removed`, `stale-approval` (resume), vs `subagent-baseline` |
| `model_effort` | native | choices from `model/list` via ChoiceLister (not catalog) | `turn/start` `model`/`effort` override "this turn and subsequent turns", so SetModel/SetEffort apply on the next `turn/start` in-stream with no restart. The effort override persisted into the resumed thread (`reasoningEffort: "low"`). | recorded (effort); planned (model switch) |
| `approval` | native | modes stay in `BackendRegistration.PermissionModes` (`approvalPolicy` x `sandbox`) | `item/commandExecution/requestApproval` -> answer -> `serverRequest/resolved` -> `item/completed` (`declined`). Delivered binary (allow/deny), per interface gap E1. MCP tool approval arrives as `mcpServer/elicitation/request` with `_meta.codex_approval_kind: "mcp_tool_call"`. | `lead-session`, `resume`, `stale-approval` |
| `steer` | native | - | `turn/steer {threadId, input, expectedTurnId}` -> `{turnId}`. The steered text becomes a `userMessage` item in the same turn, with no new `turn/started`. A wrong id gets `-32600 expected active turn id ...`. Idle gets `no active turn to steer`. Schema error `activeTurnNotSteerable` covers review/compact turns. | `lead-session` |
| `interrupt` | native | - | `turn/interrupt {threadId, turnId}` -> `{}`, then `turn/completed` `status: "interrupted"` | `lead-session`, `stale-approval` |
| `follow_up` | ws-substitute | - | The runner holds the message and sends it at turn end (`Runner.Send`). The experimental `thread/queue/*` is not used. | n/a (runner) |
| `resume` | native | - | `thread/resume {threadId}` with config re-passed. A missing thread gets `-32600 no rollout found for thread id ...`. Needs a non-ephemeral thread. | `resume`; planned: resume across Codex versions (only one installed) |
| `reasoning_stream` | unsupported | - | Codex streams `item/reasoning/summaryTextDelta` and `reasoning` items (summaries were empty at `effort: low`). No ws wiring; owned by the reasoning-summary line. | none |
| `usage_query` | native | - | `thread/tokenUsage/updated {tokenUsage: {total, last, modelContextWindow}}` after every model response, plus `account/rateLimits/updated`. Nothing is asked (see schema gap G2). | `lead-session` |

**Non-catalog event and recovery contract (for the Phase 2 mapper).**
- Scope every notification by `threadId`. One app-server stream carries the
  lead thread and any subagent thread it spawned. In `resume`, the subagent's
  own `thread/status/changed` (`idle`), `turn/started` and `turn/completed`
  (another `threadId`) interleave inside the lead's running turn
  (`TestCodexSubagentRemovalFixture`). A mapper that skips this check
  would report the lead idle mid-turn, or end the lead's turn on the
  subagent's turn end. This also applies to `hook/*`, `thread/goal/*` and
  item events. Removing subagents (below) lowers the risk, but the rule
  stays.
- Turn lifecycle:
  - `turn/start` response (`inProgress`) -> `turn/started` -> items -> `turn/completed {turn.status: completed|interrupted|failed, turn.error}`.
  - `thread/status/changed` maps to the harness state: `idle`, `active` (running), `active` with `activeFlags: ["waitingOnApproval"]` (`HarnessWaiting`).
  - Items: `item/started` / `item/completed` with types `agentMessage` (streamed through `item/agentMessage/delta`; the final message is marked `phase: "final_answer"`), `commandExecution`, `mcpToolCall`, `fileChange`, `reasoning`, `userMessage`, `subAgentActivity`, `collabAgentToolCall`, and others (schema `ThreadItem`).
  - An unknown notification is ignored.
  - `error {error, willRetry}` with `codexErrorInfo` (`usageLimitExceeded`, `unauthorized`, ...) maps to Retry/Alert.
- Interrupt leaves items open: the interrupted turn's `commandExecution` item gets no `item/completed` before `turn/completed`. Its completion arrives later, during the next turn, and still names the old `turnId` (`TestCodexInterruptLeavesItemOpenFixture`). The mapper must close open items at turn end and attribute every item event by `turnId`, never by "the current turn".
- Approvals: correlation is the server's JSON-RPC request id, which maps to `ApprovalRequest.ID` as a string. `serverRequest/resolved {requestId}` maps to `ApprovalDone`. On interrupt the order is turn end, then resolved. Stale or duplicate answers got no reply and had no effect *(probe log)*. `TestCodexStaleApprovalFixture` pins the order up to the late answers.
- Unhandled server requests: every server request the adapter does not handle must get a JSON-RPC error reply, never be dropped, or the turn stalls. Schema set at 0.160.0: `item/fileChange/requestApproval`, `item/permissions/requestApproval`, `item/tool/requestUserInput`, `item/tool/call`, `account/chatgptAuthTokens/refresh`, `attestation/generate`, legacy `applyPatchApproval`/`execCommandApproval`.
- Holder reattach: the holder keeps the app-server's stdio, as for Claude and Pi. Client request ids are client-chosen (`RequestId` is string or integer), so a reattaching daemon must not reuse an in-flight id. Planned: string ids with a per-attach prefix.
  - Planned fixtures: reattach mid-turn; reattach with an approval pending; ring gap. On a gap, `thread/turns/list` / `thread/read` can rebuild the last turn's final message and status (`resume` records a `thread/turns/list` reply).
- Instructions / workflow access: `developerInstructions` on `thread/start` is additive and persists across resume (`resume` T1 answers the marker without re-sending it).
  - `baseInstructions` / `model_instructions_file` replace the built-in instructions and are not used.
  - Project `AGENTS.md` loads natively (`instructionSources`).
- Config / account home: `initialize` reports `codexHome` (the user's `~/.codex`). Auth, rollouts and logs live there, and the lead keeps them, as the Claude lead keeps user settings.
  - The user's `config.toml` defaults apply (the thread opened at `reasoningEffort: "high"` from user config).
  - The user's `hooks.json` hooks fire in app-server (`hook/started`/`hook/completed`), and `features.hooks=false` suppresses them (`resume`).
  - The process is the harness PID (stdio). With HTTP MCP there is no ws-mcp child.
- Subagent telemetry, if not removed: `subAgentActivity {kind: started|completed, agentThreadId, agentPath}` and `collabAgentToolCall` (`resume`, `subagent-baseline`).

**Removability verdict (08)=c: removable.** At 0.160.0, `config.agents.enabled=false` on `thread/start` and on `thread/resume` removes the native subagent tools. Evidence:
- The model reports `NO-SUBAGENT-TOOL` twice on fresh threads and once on a resumed one.
- No `subAgentActivity` / `collabAgentToolCall` item appears with the setting, while the same prompt spawns one on the ephemeral baseline.

`features.multi_agent=false` is not a removal: the tool stays visible and calls fail. The tool list is not in any init event, so this item is gated by version only (no init-shape demotion source). Phase 2 applies the removal for the hosted lead, because delegation is `ws/agent.*` (Decision 7).

**Compatibility verdict (Decision 8).**
- Phase 1 surfaces are T0: test-only fixtures and a `_test.go` file, with no shipped, stored or protocol change.
- Phase 2 surfaces, as verdicts to confirm at implementation:
  - The Codex catalog profile is in-code daemon data, so T0.
  - The thread id is stored in the existing `Record.SessionID`. Phase 2 must add no new `Record` field: the old binary's `readRecord` + `writeRecord` round trip (`agents-plugin-tool/internal/wsagent/record.go:281-305`) drops unknown fields, which would make any such field at best T2.
  - The old-daemon scenario is **T1**, not T0. The old binary reads a hosted Record with `backend: "codex"` without validating the backend (`record.go:281-294`). `Runner.Start` -> `startConv` -> `reattach` (`runner.go:266-304`, `runner.go:607-616`) reaches `startLead` -> `startBackend` -> `Runner.backend`, which returns `ErrBackendUnavailable` (`runner.go:924-929`). `reattach` only logs it and keeps `HolderID` in the record (`runner_process.go:378-381`). The row is listed but the old daemon cannot drive the session, and nothing in the record changes. Reopening with the new daemon reattaches the same holder.
  - The HTTP MCP endpoint and token verdict belongs to catalog Phase 2.

**Interface gaps returned to the lead (no common-interface change made).**
- E1, approval expressiveness: `approvalAnswerer.AnswerApproval(req, allow bool, message)` (`backend.go`) is binary. Codex offers richer decisions:
  - Commands: `accept`, `acceptForSession`, `acceptWithExecpolicyAmendment`, `applyNetworkPolicyAmendment`, `decline`, `cancel`.
  - File changes: `accept`, `acceptForSession`, `decline`, `cancel`.
  - Permissions: a granted subset plus `scope: turn|session`.
  - MCP elicitation: `accept`/`decline`/`cancel` with `persist: ["session","always"]`.

  Phase 2 can map allow to `accept` and deny to `decline` (`{permissions:{}, scope:"turn"}` for permissions), with stable correlation. Every other choice would be listed unsupported, which is a loss of expressiveness that Decision 3 asks the lead to settle. The alternative is an extension: offered choices on `ApprovalRequest`, and an answer that names one.
- E2, user questions: the default tool set includes `request_user_input`, which arrives as the server request `item/tool/requestUserInput`. No `Event`/`Process` form exists for it. Phase 2 options: answer with an error (the model sees a failed tool), or extend the interface. Whether that tool can be removed was not probed.
- Everything else fits the existing shape without extension:
  - `Send` -> `turn/start`
  - `Interrupt` -> `turn/interrupt` (the process tracks the active turn id)
  - `Steer` -> `turn/steer`
  - `SetModel`/`SetEffort` -> next `turn/start` overrides
  - server requests -> `Event.Approval`, `serverRequest/resolved` -> `ApprovalDone`

**Schema gaps (catalog `d20202cd6`).**
- G1: `parseHarnessVersion` cannot read `codex-cli 0.160.0`. Phase 2 passes `thread.cliVersion` instead (no schema change needed).
- G2: `ItemUsageQuery` reads "ws asks the harness". Codex pushes usage and is never asked. It is graded native on delivery, which the lead may want to re-word.
- G3: the item list has no slot for the approval decision set (E1) or the user-question request (E2).
- Admission conditions are not items (by design). Their evidence is recorded above.

**Decisions taken (for veto).**
- The ws-plugin MCP block mechanism (the analogue of catalog Decision 12) is a recommendation for Phase 2 to settle by probe:
  - (B, recommended) shadow the plugin server by injecting the HTTP entry under the plugin's server name (`ws`; `wsflow` for that package). This keeps the plugin's skills and hooks.
  - (A) `plugins."<id>".enabled=false`, which drops the whole plugin and needs the installation-specific `<plugin>@<marketplace>` key.

  `ws-mcp-shadow` shows a single `ws` `starting` event, and *(probe log)* shows the HTTP server took the handshake. Not verified: whether the plugin's stdio `ws-mcp` process was also spawned. Phase 2 checks the process count.
- Hooks stay user-controlled (Claude-lead parity). `features.hooks=false` is available, if the lead prefers isolation.
- The probe thread `01a1e9b5-2c24-7f90-b6f3-4b7d0c71a317` remains in the user's `~/.codex/sessions/2026/11/17/`. It was left in place rather than deleted (`thread/delete`), to avoid a further write to the Codex home.

**Unknowns (explicit).**
- Resume across Codex versions.
- `thread/compact/start` behaviour.
- Mid-turn model change.
- Holder reattach and gap fixtures.
- Whether `request_user_input` can be removed.
- Whether the stdio ws-mcp is spawned under mechanism B.
- Behaviour below 0.160.0. No older binary was available, so the lower bound is 0.160.0 by fixture.

### Phase 2: Implement one hosted lead conversation after the foundation gate

Depends on this ticket's settled Phase 1 and the capability catalog ticket's Phases 1-3 (`261117-feat-ws-harness-capability-catalog`); execution approval is granted (Decision 4). Implement only the agreed hosted-lead slice through the existing Runner/backend/holder boundary, the catalog profile and the startup version/init-shape check. Preserve truthful unsupported controls, one input/mail owner, approvals, identity and ordered recovery. No implicit native-child, multi-account or terminal-handoff parity expansion. The backend serves the lead role only: it must not make `codex.<tier>` spawnable as an `agent.spawn` worker (batch-review coherence C2; the tier listing rule lives in `261117-feat-ws-agent-tools-mailbox-routing`). Decision 9 settles the Phase 1 gaps (E1, E2, G1-G3), the ws-plugin MCP block mechanism and hooks; apply the Phase 1 removal verdict (`config.agents.enabled=false` on `thread/start` and `thread/resume`) for the hosted lead.

Verification: isolated protocol and real-adapter/fake-holder fixtures for create/resume/send/interrupt, ordered content/tools/usage/errors, approvals, unknown events, holder reattach/replay gaps and no duplicate input. Run Pi/Claude regressions against the exact candidate. A real hosted Codex lead conversation on the installed CLI with the owner's login is the acceptance check; no fixture result is invented.

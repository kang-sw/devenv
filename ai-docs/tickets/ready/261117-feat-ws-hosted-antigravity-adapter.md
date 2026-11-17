---
title: "Antigravity hosted adapter: fill catalog values per harness@version range and implement the backend (stage 3, after Codex)"
parent: 261114-epic-ws-common-session-foundation
related:
  261114-feat-ws-hosted-antigravity-acp-adapter: "dropped prior work (immutable in .dropped); this ticket revives its scope under the hosted-first plan"
  261117-feat-ws-harness-capability-catalog: prerequisite
  261115-feat-ws-hosted-codex-adapter-contract: "stage 3 ordering: Codex first, then Antigravity (25)"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5adf930d038dd542
sage-review-completeness-reviewed: 5adf930d038dd542
---

# Antigravity hosted adapter

## Background

The earlier ticket `261114-feat-ws-hosted-antigravity-acp-adapter` was dropped on
2026-11-15 (owner (1057)) as far-future work because no Antigravity account was
available; it lives in `.dropped/` and is immutable, so this is a new stem. What
it planned, in short:

- assess the official ACP path (`agy acp`, ACP v1 JSON-RPC over stdio, with
  headless NDJSON, the Python SDK and the IDE extension as distinct alternatives)
  against the common hosted contract;
- settle an exact-version capability/protocol/fixture matrix (session
  create/load/resume, send/cancel/steer, permission pause/answer correlation,
  ordered events, holder reattach, auth/config home, subagent telemetry);
- then implement the chosen public control path through the existing
  Runner/backend/holder boundary.

Antigravity is revived by (02) (correction: Codex and Antigravity will both be
tier-1 hosted harnesses after hosted common-ization) and (05) (bring Antigravity
back as a stage-3 child of the epic). The ticket's official-documentation links
and capability checklist remain the starting evidence there; they are not
re-verified here.

## Decisions

1. **Stage-3 child, ordered after Codex (25), ahead of the holder ((70)=b).**
   Implementation and verification order is Claude, Codex, Antigravity, then
   Pi. Owner (70)=b, 2026-11-17 22:10, moved Codex and Antigravity ahead of
   `261117-feat-ws-single-session-holder`. Phase 1 starts after the Codex
   ticket's Phase 1 lands; Phase 2 also depends on the catalog's Phases 1-3.
2. **Catalog-based (07).** The work is: fill the capability catalog values per
   `harness@version` range (`since` / `until`), and implement the backend. The
   adapter is range-versioned per the catalog version policy (20)=a, (21)=a, (22):
   startup version and init-shape check, nearest-profile "unverified version"
   warning when out of range, per-version recorded stream fixtures.
3. **Admission conditions apply.** Headless JSON event streaming (the adapter
   converts the format) and HTTP MCP server injection (24). Whether the Antigravity
   path satisfies them is determined in Phase 1.
4. **Delegation is `ws/agent.*` (06).** Hosted sessions delegate through
   `ws/agent.*`. Owner (08)=c: whether a harness's native subagents can be
   removed is decided per harness at stage 3, so Phase 1 probes and records it
   for Antigravity.
5. **Account and access confirmed (owner 2026-11-17 22:10).** The owner logged
   in on the home machine (installed `agy` 1.5.9) and approved provider calls
   for probes, fixtures and smoke tests. Do not log in, log out, switch
   accounts or mutate credentials; keep account details out of tracked files.
   Rejected: keeping the earlier "recheck at stage-3 start" gate, which this
   answer settles.

## Constraints

- Go test location (owner (68), 2026-11-17 22:10): run Go tests locally on the macOS home machine. The earlier "remote w5090 only" rule belonged to the owner's company Windows setup and does not apply here.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; files for backup/restore only; deltas (append) not rewrites; no
  heartbeat rewrites; interactive paths never wait on file I/O.
- Keep machine-local credentials and account details out of tracked tickets and
  manuals. Do not mutate the user's agy config home; run probes with an isolated
  or read-only configuration where the CLI allows.
- No main push, version bump or release. Work lands on `track/agent-v2` through
  the lead. Replacing the shared daemon on this machine is the lead's call, not
  a worker's.
- Do not edit the dropped prior ticket; link it by stem only.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Compatibility

Additive adapter and catalog data: target T0. Phase 1 records the concrete tier
verdict for every new surface (descriptor, config, protocol) in its `### Result`,
with source lines, per `ai-docs/manuals/release-compatibility.md`, including an
old daemon reopening a hosted Record whose new Antigravity `Backend` value it
does not know.

## Prior Decisions

- 261114-feat-ws-hosted-antigravity-acp-adapter (2026-11-14, Decisions): "A future adapter must coexist with Pi/Claude without reducing their supported controls or native integrations. MCP attachment alone does not establish sustained agent-process control." — bearing: constrains
- 261114-feat-ws-hosted-antigravity-acp-adapter (2026-11-14, Decisions): "The future design must prefer an additive T0 adapter. Any actual protocol/store/config/MCP change is classified and settled before promotion; T2 is not authorized by this deferred idea." — bearing: constrains
- 449e2602 (2026-11-15, commit): "Keep exact-version support unknown and distinguish ACP from headless NDJSON, SDK and IDE-private transport. Record required session/control/approval/event/identity/holder/config/telemetry evidence" — bearing: constrains
- 261117-feat-ws-exec-tools (2026-11-17, commit 1d04941a): "Owner 19:02: harness+tier strings (pi.small, codex.large) and user aliases (mockup -> agy.large) instead of model/effort args; rejected: spawn_ex(model, effort, harness) and an override object." — bearing: constrains
- 261114-epic-ws-common-session-foundation (2026-11-17, commit 6615ab3f): "(29)=ok extends native subagent removal to hosted workers; owner added a per-harness 'Never use {{NativeToolScheme}}' playbook line because Codex/Antigravity removability is still unprobed ((08)=c)." — bearing: supports
- 261116-research-ws-hosted-session-first-tier (2026-11-16, Confirmed Decisions): "(02) Codex and Antigravity will both be supported as tier-1 hosted harnesses after hosted common-ization; Antigravity is not dropped (new ticket `261117-feat-ws-hosted-antigravity-adapter`" — bearing: supports
- 261115-feat-ws-hosted-codex-adapter-contract (2026-11-15, Decisions): "Stage ordering (25). Claude is the reference, then Codex (this ticket), then Antigravity, then Pi. The Pi hosted backend is a port of this Codex adapter and catalog profile" — bearing: supports
- 261114-refactor-ws-hosted-common-contract-pi-claude-parity (2026-11-14, Decisions): "No Antigravity install, authentication, provider call or live probe" — scoped to that first unit and lifted for this ticket by Decision 5 (owner 2026-11-17 22:10) — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 2 touches the Runner/backend/holder boundary (agents-plugin-tool/internal/wsagent/backend.go, registration.go, holder.go), catalog values and fixtures; no agy/antigravity file exists in wsagent yet (ls grep found none); exact files are settled in Phase 1 |
| scope.surface | cross-module | wsagent backend plus holder, catalog profile and startup version check, possibly MCP injection under agents-plugin-tool/internal/mcp/ |
| scope.new_public_symbol | unknown | the backend registration and catalog profile shape are settled in Phase 1 and the catalog schema has not landed (pending 261117-feat-ws-harness-capability-catalog) |
| scope.new_type_contract | unknown | the Backend interface exists at agents-plugin-tool/internal/wsagent/backend.go#L24 but whether agy needs a new contract depends on the Phase 1 matrix |
| scope.test_surface | new-files | Phase 2 verification names recorded-stream fixture tests and an account-backed smoke; wsagent has existing Claude and fake backend tests but no agy test |
| complexity.reuse_points | unconfirmed | Backend interface and BackendRegistration exist (wsagent/backend.go#L24, registration.go#L14) but the catalog schema they plug into has not landed (pending 261117-feat-ws-harness-capability-catalog) |
| complexity.side_effect_risk | moderate | additive backend behind registration, but it spawns a provider-calling subprocess under the owner's login and runs inside the shared holder |
| risk.correctness | high | agy 1.5.9 protocol, session resume, permission correlation and event ordering are unprobed; the ticket has no recorded matrix yet |
| risk.fit | high | fit to the catalog and Codex-first contract is unsettled; both predecessors have unlanded phases (pending 261115-feat-ws-hosted-codex-adapter-contract) |
| risk.test | high | needs recorded fixtures per version plus provider-backed smoke on a single owner account that automated tests cannot reproduce |
| risk.security_or_contract | high | credential home and account state, permission mapping, HTTP MCP token injection and a version-ranged catalog contract are all in play |

## Phases

### Phase 1: Settle the version-ranged contract and fixtures

With access confirmed (Decision 5): select the target version range from the
installed `agy`; assess the documented public ACP path against the catalog's
admission conditions and items; record `native` / `ws-substitute` / `unsupported`
per item with `since` / `until`; decide native subagent removability (08)=c; plan
the per-version fixtures. Return product or API choices to the lead, and also
which roles (hosted lead, `agent.spawn` worker, or both) the first Phase 2 slice
should deliver, with the evidence for each. If the chosen path does not fit the
existing `Backend` / `Process` shape, describe the needed extension and return
it to the lead; do not extend the common interface in this phase.

Deliverables: the matrix, admission evidence, removability verdict, role
recommendation and compatibility verdict go in this phase's `### Result`;
recorded fixtures go under `agents-plugin-tool/internal/wsagent/testdata/`.

Verification: a recorded matrix where every claimed capability carries a version
and a source; a fixture plan distinguishing planned fixtures from ones run.

### Result (25e34507c) - 2026-11-17

Probed the installed `agy 1.5.9` (macOS arm64, the owner's Google login)
by driving `agy acp` from a stdlib Python client, with a loopback
streamable-HTTP MCP server standing in for the daemon. No login, logout,
account switch or credential write. Every probe after the first passed
`--gemini-dir <scratch>`: conversations, `brain/` transcripts, logs and the
MCP schema cache landed in a scratch home, and auth still came from the
system keychain. The first handshake probe ran without that flag and left
one empty conversation (a `session/new` with no prompt) in the user's agy
home, which was left in place rather than deleted. One headless attempt
under a scratch `HOME` printed a login URL and exited (stdin was
`/dev/null`; no login happened), so `HOME` isolation is not usable. Probe
threads ran in a scratch git repo outside any project, and the probe
client removed `WS_*`/`CLAUDE*`/`ORCA_*`/`CODEX_*`/`PI_*` from the child
environment. `agy changelog` lists 1.5.10 as available; nothing was
updated.

**Evidence sources.**
- Exact-version behaviour: the installed binary's `initialize` reply,
  `agy --help`, `agy help acp` and `agy changelog` (release notes through
  1.5.10).
- Official prose, fetched 2026-11-17: `antigravity.google/docs/cli/acp/`,
  `.../docs/ide/extensions/acp/`, `.../docs/cli/headless/`,
  `.../docs/subagents?tab=cli`, `.../docs/permissions?tab=cli`.
- Recorded fixtures: `agents-plugin-tool/internal/wsagent/testdata/agy-1.5.9-*.jsonl`
  (eleven files), cut with the codex `>> ` client-line convention.
  `agy_fixture_test.go` pins most protocol facts cited below; the
  compaction usage drop and the permission-option shape outside
  `lead-session` are visible in the fixtures but not asserted.
- Probe-side observations that are not in the fixtures carry the label
  *(probe log)*: the probe MCP server's request log, greps of the scratch
  home, and runs not committed as fixtures.

**Fixture index (recorded).**
- `handshake`: `session/new` before `initialize` succeeds (no init gate). `initialize` reports `agentInfo.version` `1.5.9`, `sessionCapabilities` `resume`/`load`/`close`/`list`, `_meta.antigravity.systemPrompt.append: true`, and no `mcpCapabilities`. A second `initialize` succeeds; an unknown method gets `-32601`.
- `http-mcp`: `session/new` with an HTTP MCP server (bearer token in `headers`) and a system-prompt append; the model answers the marker and runs `mcp_ws_probe_echo` after an approval.
- `lead-session`: tool self-report, MCP tool denied, command denied, `session/cancel` during `sleep 30`, steer by a second prompt during `sleep 12`, `session/set_config_option` (thought_level, model), `session/set_mode` twice, `session/set_model`, then a marker turn.
- `resume`: resume of a missing id fails; resume of the lead session in a new process (MCP re-passed with a new token) continues it; `/compact`; `session/close`; `session/new` with an HTTP MCP server refusing the token.
- `load-replay`: `session/load` of the lead session in a fresh process.
- `stale-approval`: string request ids; a held permission request, `session/cancel`, a late answer, then a normal turn.
- `resume-marker`: resume of the `stale-approval` session in a new process with no append re-sent.
- `subagent-baseline`: default agent spawns the built-in `research` subagent; `ask_question` with no client elicitation.
- `subagent-removed`: the same prompts under `--agent ws-nosub` (a scratch-workspace agent whose `tools` list leaves out the subagent tools). The flag is a CLI argument and not in the transcript.
- `custom-agent-bad-tool`: the first `ws-nosub` draft naming `list_resources`/`read_resource`, which are not registry tools.
- `custom-agent-mcp`: a custom agent with `mcpServers: [{name: ws}]` in its frontmatter.

**Control path (returned to the lead): ACP, not headless.**
- `agy acp` (documented since 1.5.0, changelog) is sustained,
  bidirectional JSON-RPC over stdio with permission requests, cancel,
  steer, resume/load and in-session model and mode changes.
- Headless `-p --input-format stream-json --output-format stream-json`
  (since 1.1.15) keeps one conversation open, but the headless doc states
  approvals that cannot be obtained are soft-denied. It has no cancel other
  than killing the process and no steer, so it cannot carry approval,
  interrupt or steer. Rejected.
- The SDK and the IDE app are separate products and were not probed.

**Admission conditions (catalog Decision 2): both pass at 1.5.9, with one
gap.**
- (0) Headless JSON event streaming: ACP over stdio, newline-delimited
  JSON-RPC 2.0 with the `jsonrpc` member present (all fixtures). Sustained
  and bidirectional: client requests and notifications (`session/cancel`),
  agent notifications (`session/update`) and agent requests
  (`session/request_permission`).
- (3) HTTP MCP injection: `session/new` / `session/resume` `mcpServers:
  [{type: "http", name, url, headers: [{name: "Authorization", value:
  "Bearer <token>"}]}]` (`http-mcp`, `resume`). The probe server saw the
  bearer on every POST *(probe log)*.
  - `initialize` advertises no `mcpCapabilities.http`, yet HTTP servers
    work. The ACP doc says `--process-per-session` is "required for HTTP
    MCP servers the editor configures". At 1.5.9 the server worked both
    with and without the flag *(probe log: identical runs)*.
  - The token travels in the stdio request, not the environment, and
    nothing persists it. After four probe tokens, a grep of the scratch
    home found 0 hits for the tokens and the MCP URLs *(probe log)*. MCP
    servers are therefore re-passed on every resume.
  - **Gap: auth failure is silent.** A session/new with a wrong bearer
    succeeds; its tools are just absent and the model answers `NO-WS-TOOL`
    (`resume`). That the server answered 401 is *(probe log)*. There is no
    `required` flag in ACP.
  - The MCP client first POSTs `server/discover` (protocol `2026-07-28`),
    then `initialize`, and GETs `/.well-known/oauth-protected-resource[/mcp]`
    even after a successful handshake. The probe answered method-not-found
    and 405, and agy proceeded *(probe log)*. The daemon's HTTP MCP route
    must answer them cleanly.

**Capability matrix: catalog values for the Phase 2 agy profile.**
- Shape: the `catalog.go` schema on `impl/track/agent-v2/rust-stank-cedar`
  at `4ebcf2ed2` (last catalog change `92b4c835b`; read only).
- Profile range: `Oldest: "1.5.9", Newest: "1.5.9"`, the only version with
  fixtures. Entries carry no `Since`/`Until`, like `ClaudeCatalog`.
  `Resolve` pins any other version to 1.5.9, marked unverified.
- Version source: `initialize` `agentInfo.version` (`"1.5.9"`, a bare
  dotted version that `parseHarnessVersion` reads; `agy --version` prints
  the same *(probe log)*).
- Roles: every entry is `Role: RoleLead`. Worker stays undeclared and
  resolves `unsupported` under the role recommendation below (C2).
- Grades follow catalog Decision 10 (what the ws adapter will deliver once
  Phase 2 wires it).

| Item | Grade (lead) | Values | Evidence (1.5.9) | Fixture |
|---|---|---|---|---|
| `skill_list` | unsupported | - | `available_commands_update` lists agy's commands and skills (`goal`, `compact`, `learn`, ...). No ws wiring. | `lead-session` (no ws reader) |
| `skill_injection` | native | - | agy's own skills, invoked by a `/name` prompt and listed in `available_commands_update`. ws's skills are **not** present: `agy plugin list --json` is `[]` on this machine *(probe log)*, so ws workflow skills reach an agy lead only after a delivery choice (decision 4 below). | planned: Phase 2 lead with the chosen delivery |
| `compaction` | native | - | The dashboard's `/compact` maps to a `session/prompt` with text `/compact` (listed as a command). The turn ends `end_turn` and carries only a `usage_update`: there is no compaction event, and the next turn's `used` drops (18682 -> 10508). | `resume` |
| `rewind` | unsupported | - | No ACP method. | none |
| `tool_removal` | unsupported | - | The lead keeps the user's tool setup, as `ClaudeCatalog` does. For a later worker row, a custom main agent's `tools` allowlist removes built-in tools (`subagent-removed`). | recorded (worker mechanism) |
| `subagent_removal` | unsupported (recommended; see verdict) | `invoke_subagent`, `define_subagent`, `manage_subagents`; `swarm`, `run_workflow` also fan out subagents | Removal exists only through a custom main agent, with costs that argue against it for the lead (verdict below). | `subagent-baseline` vs `subagent-removed` |
| `model_effort` | native | choices from `configOptions` (`model`, `thought_level`) via ChoiceLister | Effort is recorded: `session/set_config_option {configId: "thought_level", value: "low"}` answers with the updated `configOptions` in-session, with no restart. A model change is **not** evidenced. The recorded `model` call re-set the current `gemini-3.8-flash`, and the unstable `session/set_model gemini-3.8-flash-low` answered `{}` with `configOptions` unchanged. Two model vocabularies exist: `configOptions` values (`gemini-3.8-flash`) and `models.availableModels` ids (`gemini-3.8-flash-high`). Phase 2 must probe which one switches the model. | `lead-session` (effort); planned: model switch |
| `approval` | native | modes stay in `BackendRegistration.PermissionModes`: `default`, `accept-edits`, `auto`, `plan` (`session/set_mode` -> `current_mode_update`) | `session/request_permission {toolCall, options}` offers exactly `allow` (`allow_once`) and `deny` (`reject_once`) for MCP tools and commands. The answer is `{outcome: {outcome: "selected", optionId}}`. A denied call ends `failed`. Binary, so it fits `AnswerApproval(allow bool)`. | `lead-session`, `http-mcp`, `stale-approval` |
| `steer` | native | - | A `session/prompt` sent while a turn runs steers it. The earlier prompt answers `end_turn` immediately, and the rest of the run belongs to the later prompt: the tool's completion and the reply that uses the steer. | `lead-session` |
| `interrupt` | native | - | `session/cancel` (notification) -> open tool calls `failed` -> prompt answers `stopReason: "cancelled"`. No item outlives the turn (unlike Codex). | `lead-session`, `stale-approval` |
| `follow_up` | ws-substitute | - | The runner holds the message and sends it at turn end (`Runner.Send`). A mid-turn prompt would steer instead, so holding it is required, not optional. | n/a (runner) |
| `resume` | native | - | `session/resume {sessionId, cwd, mcpServers}` continues without replay. A missing id gets `-32603 Failed to read session ...`. The system-prompt append persists across a process restart without being re-sent (`resume-marker`). `session/load` replays instead (below). | `resume`, `resume-marker`; planned: across agy versions |
| `reasoning_stream` | unsupported | - | agy streams `agent_thought_chunk`. No ws wiring; owned by the reasoning-summary line. | `lead-session` (no ws reader) |
| `usage_query` | native | - | Pushed: `usage_update {used, size}` at session start and after each model call, plus `usage` on every `session/prompt` response. Nothing is asked (same as Codex schema gap G2). | `lead-session` |

**Non-catalog event and recovery contract (for the Phase 2 mapper).**
- Id spaces: the client's request ids and the agent's request ids both start
  at 1 (`custom-agent-mcp`: permission request ids 1 and 2 beside client
  ids 1-3). A line with a method is a request, and one without is a
  response; match responses by direction.
  - Agent request ids are JSON **strings** (`"1"`), while the probe's client
    ids were numbers. Echo the raw id; never decode it as an integer
    (pinned in `TestAgyStaleApprovalFixture`).
- Turn end: a `session/prompt` response ends the turn only when no later
  prompt is outstanding (steer). Stop reasons seen are `end_turn` and
  `cancelled`.
  - Updates do not arrive after the prompt's response (changelog 1.5.1), and
    the recordings agree.
  - A background task or subagent finishing after a turn wakes the agent,
    which then streams with no prompt outstanding (changelog 1.5.10, not in
    1.5.9's notes; unrecorded).
- Updates (`session/update.update.sessionUpdate`):
  - `agent_message_chunk`, `agent_thought_chunk`, `user_message_chunk`.
    The last appears in every replay. Live, it was seen once, in a fresh
    session's first turn *(probe log)*, and in no recorded live turn, so a
    mapper must not rely on a live echo.
  - `tool_call` / `tool_call_update`: `toolCallId`, `status` `in_progress` ->
    `pending` (awaiting approval) -> `in_progress` -> `completed` | `failed`,
    with `kind`, `title`, plus non-standard `name` and `args` carrying the
    agy tool name and input. MCP tools appear as `mcp_<server>_<tool>`.
  - `usage_update`, `available_commands_update`, `current_mode_update`,
    `config_option_update` and `session_info_update` (title).
  - Ignore unknown kinds.
- Approvals: the correlation is the agent's JSON-RPC request id, which maps
  to `ApprovalRequest.ID`. Pick the option by `kind` (`allow_once` /
  `reject_once`), never by a fixed id. On cancel, nothing withdraws a
  pending request: the call goes `failed`, the prompt answers `cancelled`,
  and a late answer gets no reply and has no effect (no file was created,
  *(probe log)*). The process must synthesize `ApprovalDone` at prompt end.
- Unhandled agent requests: with `fs`/`terminal` client capabilities false,
  none arrived. Any request the adapter does not handle still gets a
  JSON-RPC error reply.
- `ask_question` (the user-question tool) without a client elicitation
  capability renders as agent text and the turn ends with the call left
  `pending`, so the next prompt is the answer (`subagent-baseline`). This
  fits `Send` with no interface extension. The elicitation path (client
  advertising it) was not probed.
- Subagent telemetry, if not removed: the parent's `invoke_subagent` /
  `manage_subagents` tool calls only (`args`: `TypeName`, `Role`,
  `Prompt`). No update names another session, and the subagent's result
  arrives inside the same prompt.
- Holder reattach: the holder keeps `agy acp`'s stdio, as for Claude and Pi.
  Client ids are client-chosen, and string ids are accepted
  (`stale-approval`: `attach1-1`), so a per-attach prefix avoids reuse.
  - On a ring gap, `session/load` replays the history before its response:
    user, thought and agent text plus `tool_call` lines with final status
    only, and no `tool_call_update` (`load-replay`).
  - Whether `session/load` is safe on a session already open in the same
    process was not probed.
- Instructions / workflow access: `session/new`
  `_meta.antigravity.systemPrompt.append` is additive and persists across
  resume. Project rules and agy skills load natively.
- Config / account home *(probe log: directory listing of the default and
  scratch homes)*: default `--gemini-dir` (`~/.gemini`) holds
  `antigravity-cli/settings.json` (permission rules, mode), `conversations/<id>.db`,
  `brain/<id>/` transcripts, `log/` and the MCP schema cache. Auth comes from
  the system keychain.
  - A hosted lead keeps the user's home and settings, matching Claude and
    Codex parity.
  - A worker could run under an isolated `--gemini-dir` and keep auth (probe
    evidence).
- Process: `agy acp` is the harness PID. It can serve many sessions, and
  the adapter opens one; `--process-per-session` adds a worker process per
  session.

**Removability verdict (08)=c: removable, but not recommended for the lead.**
At 1.5.9 the only removal found is a custom main agent: `agy acp --agent
<name>`, with the file in the workspace's `.agents/agents/` or under
`<gemini-dir>/config/agents/`. Its `tools` allowlist omits the subagent
tools. Evidence: the removed run answers `NO-SUBAGENT-TOOL` and makes no
`invoke_subagent` call, while the same prompt spawns one on the baseline.
Costs:
- The agent's markdown body replaces agy's default system prompt. The first
  prompt's input dropped from 9135 to 3748 tokens (baseline vs removed).
- Session-injected MCP tools vanish under a `tools` allowlist
  (`subagent-removed` lists no `mcp_ws_*`). Naming `mcp_ws_probe_echo` or
  `mcp(ws/*)` in `tools` fails, the same way `custom-agent-bad-tool` does
  *(probe log)*. `mcpServers: [{name: ws}]` restores them only through the
  lazy `call_mcp_tool` gateway, with an extra read approval of the schema
  cache file (`custom-agent-mcp`). `eager: true` changed nothing
  *(probe log)*.
- A bad `tools` entry fails every turn as agent text, not as an error
  (`custom-agent-bad-tool`). A malformed `mcpServers: [ws]` silently fell
  back to the default agent, with all subagent tools back *(probe log)*.
- The agent file must be written into the user's workspace or agy home.
- No flag or setting removes a tool. The permission rules have no subagent
  action (doc); `--settings` was not probed for this.

Recommendation: the lead keeps the subagent tools (`unsupported`), and the
per-harness "Never use {{NativeToolScheme}}" playbook line (epic (29))
covers it. A worker profile, where ws owns the whole tool set, can use the
custom agent.

**Role recommendation for the first Phase 2 slice: hosted lead only.**
- Lead: every lead control is native over ACP and recorded: approval,
  steer, interrupt, resume, model/effort, compaction and usage. It keeps the
  user's home, like the Claude and Codex leads.
- Worker (`agent.spawn agy.<tier>`): technically feasible (an isolated
  `--gemini-dir` keeps auth; a custom agent removes tools), but it needs
  three things first:
  - a ws-written agent file, with its location and the system-prompt
    replacement cost decided;
  - a worker permission posture (no Claude `dontAsk` equivalent seen; the
    candidates are `auto` mode plus rules, or
    `--dangerously-skip-permissions`);
  - the tier-listing rule in `261117-feat-ws-agent-tools-mailbox-routing`
    (C2).

  Leave the worker out until those are settled, so `agy.<tier>` is not
  advertised as spawnable.

**Compatibility verdict.**
- Phase 1 surfaces are T0: test-only fixtures and a `_test.go` file, with
  no shipped, stored or protocol change.
- Phase 2 surfaces, as verdicts to confirm at implementation:
  - The agy catalog profile is in-code daemon data, so T0.
  - The ACP session id is stored in the existing `Record.SessionID`. Phase 2
    must add no new `Record` field: the old binary's `readRecord` +
    `writeRecord` round trip (`agents-plugin-tool/internal/wsagent/record.go:281-305`)
    drops unknown fields, which would make any such field at best T2.
  - The old-daemon scenario is **T1**, by the same chain as Codex,
    re-checked on this branch:
    - The old binary reads a hosted Record whose `backend` it does not know
      (the Phase 2 name; `agy` recommended, matching the `agy.<tier>`
      strings of 261117-feat-ws-exec-tools) without validating it
      (`record.go:281-294`).
    - `Runner.Start` -> `startConv` -> `reattach` (`runner.go:266-304`,
      `runner.go:607-616`) reaches `Runner.backend`, which returns
      `ErrBackendUnavailable` (`runner.go:924-929`).
    - `reattach` only logs it and keeps `HolderID` (`runner_process.go:372-381`).
    - So the row is listed, the old daemon cannot drive the session, and
      the record does not change. Reopening with the new daemon reattaches
      the same holder.
  - Any agy launcher config key Phase 2 adds is an additive manifest entry,
    so T0.
  - The HTTP MCP endpoint and token verdict belongs to catalog Phase 2.

**Interface fit (no common-interface change needed or made).**
- The mapping onto the existing shape:
  - `Start` -> spawn `agy acp`, `initialize`, `session/new` or
    `session/resume` with `mcpServers` and the append
  - `Send` -> `session/prompt`
  - `Steer` -> a `session/prompt` while running (the process tracks
    outstanding prompts)
  - `Interrupt` -> `session/cancel`
  - `SetModel` / `SetEffort` -> `session/set_config_option`
  - permission requests -> `Event.Approval`; the prompt response or the
    call's terminal status -> `ApprovalDone`
- The approval decision set is binary at 1.5.9, so Codex gap E1 does not
  arise. The user question is text, so Codex gap E2 does not arise.

**Schema gaps (catalog `92b4c835b`).**
- G1 (Codex) does not apply: `agentInfo.version` parses.
- G2: `ItemUsageQuery` reads "ws asks the harness". agy pushes usage; it is
  graded native on delivery, as for Codex.
- G3: `ItemCompaction` has no slot for "compaction leaves no event". The
  mapper sees only the usage drop.

**Decisions returned to the lead (product/API choices).**
1. Control path: ACP (`agy acp`). Headless stream-json is rejected (no
   approval channel, cancel or steer).
2. Role scope: lead only for the first slice (above).
3. Lead subagent removal: keep the tools (recommended) or use a ws-owned
   custom main agent (above).
4. ws skill delivery into an agy lead: none exists today. The options are
   catalog prompt assembly, or `agy plugin import claude`, which writes the
   user's agy home. Recommended: wait for prompt assembly.
5. `--process-per-session`: the doc calls it required for HTTP MCP, and the
   probe works without it. Technical recommendation: pass it (documented
   contract), and confirm the process tree in Phase 2.
6. Silent MCP auth failure: Phase 2 needs a ws-side check, for example the
   daemon expecting the hosted session's MCP handshake within a deadline and
   alerting otherwise.

**Fixture plan: planned, not run.**
- Holder reattach mid-turn.
- Reattach with a permission request pending.
- Ring gap rebuilt through `session/load`.
- Resume across agy versions (needs a second installed version).
- Model switch (which vocabulary switches it, and across families).
- The elicitation path for `ask_question`.
- A worker profile (custom agent plus isolated `--gemini-dir`), if the
  worker role comes into scope.
- Phase 2 acceptance on the owner's login.

**Unknowns (explicit).**
- Behaviour below 1.5.9 and at 1.5.10.
- Resume across versions.
- `session/load` on an already-open session.
- Whether `auto` mode still asks for MCP tools.
- `allow_always` options, never offered in `default` mode at 1.5.9.
- `thought_level` values beyond low/medium/high on other models (the doc
  names Extra and Max).
- `--settings`-based tool removal.

### Phase 2: Implement the backend under the settled contract

Before this phase is dispatched, the lead settles the role scope from Phase 1's
recommendation and records it here as an `#### Edition`; a worker does not pick
the roles. A role not in scope must not be advertised: if the worker role is
out, `agy.<tier>` is not spawnable (batch-review coherence C2).

Implement the chosen control path through the Runner/backend/holder boundary with
the catalog values, startup check and loud nearest-profile policy.

Verification: recorded-stream fixture tests, regression of the
Claude and Codex backends, and an account-backed smoke on the owner's login
(authorized, Decision 5). The smoke passes when every capability Phase 1 graded
`native` or `ws-substitute` for the settled roles works end to end on the
installed version, and every `unsupported` one is refused cleanly.

#### Edition (5a0cd8500) - 2026-11-18

Role scope and control-path decisions settled before dispatch (owner board
answers 2026-11-18: (89)=a, (87), (88), (90), (91) ok):

- (87) **Control path is ACP (`agy acp`).** Phase 1 evidence: stream-json
  soft-denies approvals and has no cancel or steer, while ACP carries every
  lead control natively (25e34507c).
- (88) **First slice is the hosted lead role only.** The worker role needs a
  ws-written agent file (location and system-prompt replacement cost), a
  worker permission posture, and the tier-listing rule first. `agy.<tier>`
  is therefore not spawnable: agy registers with no `agent-worker-agy`
  profile, so `agent.tiers` lists no agy tier and a bare agy tier is refused
  (the worker-role rule of `261117-feat-ws-agent-tools-mailbox-routing`,
  Phase 1 Result).
- (89)=a **The lead keeps agy's built-in subagent tools.** The catalog grades
  `subagent_removal` `unsupported` for the agy lead; the per-harness "Never
  use {{NativeToolScheme}}" playbook line (`261117-feat-ws-hosted-prompt-assembly`)
  is the steer. Rejected: a custom main agent (`--agent`), which replaces the
  default system prompt and hides session-injected MCP tools behind the lazy
  `call_mcp_tool` gateway with an extra approval. Whether to allow
  system-prompt replacement is decided again for the worker slice.
- (90) **ws skills reach an agy lead only through the common prompt
  assembly.** Do not run `agy plugin import claude`: it writes into the
  user's agy home.
- (91) **Always pass `--process-per-session`, and detect HTTP MCP auth
  failure on the ws side.** The doc requires the flag for HTTP MCP (the probe
  worked without it). An auth failure only drops the tools, with no error, so
  ws detects it on its own side (for example, the session's token never
  reaching the daemon's HTTP route with an `initialize`) and reports the
  start as failed loudly rather than running a lead without ws tools; the
  worker picks the exact signal from the probe evidence.

---
title: "agy completion: dotted tool-name aliases, harness detection, and the agy worker role"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-hosted-antigravity-adapter: "its Phase 2 (88) shipped the hosted agy lead only and deferred the worker role, naming three prerequisites; Phase 2 here picks them up"
  261117-feat-ws-agent-tools-mailbox-routing: "owns the agent.tiers() rule (C2) under which a lead-only backend has no spawnable tiers"
  261118-research-ws-agy-custom-agent-prompt: "probes 1-3 (custom agent file location, silent fallback, excludeDefaultComponents) feed Phase 2's agent file and system-prompt replacement"
  261117-feat-ws-hosted-prompt-assembly: "renders the agy lead row of the workflow manual; Phase 1 harness detection lets that render name agy"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a4e111c868975597
sage-review-completeness-reviewed: a4e111c868975597
---

# agy completion: dotted tool-name aliases, harness detection, and the agy worker role

## Background

The owner wants to move daily lead work to a hosted Antigravity (agy) lead
because Claude usage is scarce this week. Owner, 2026-11-18: "agy 워커까지
들어와야 마무리" - the agy switch is complete only when agy workers land, not
at the lead alone.

Two gaps stand in the way:

1. **Dotted tool names (original gap, fixed in Phase 1).** agy originally
   forwarded dotted ws MCP names (for example `mcp_ws_tickets.query`), which
   the Anthropic API rejected against `^[a-zA-Z0-9_-]{1,128}$`; Gemini and
   gpt-oss worked (adapter Phase 2 Result, Forward). The working tree now
   advertises `tickets_query` on hosted agy lead connections and reverse-maps
   it before the hosted-key rule
   (`agents-plugin-tool/internal/mcp/tool_aliases.go#L78-L121`,
   `agents-plugin-tool/internal/mcp/server_dispatch.go#L33-L41`,
   `agents-plugin-tool/internal/mcp/tool_aliases_test.go#L33-L57`).
   Connection-harness detection still has no agy case, deliberately;
   `renderLeadManual` now takes the descriptor's `ManualHarness`, and
   `playbookHarness()` supplies the descriptor fallback for renders
   (`agents-plugin-tool/internal/mcp/server_harness.go#L82-L135`,
   `agents-plugin-tool/cmd/ws-mcp/agent_mcp.go#L71-L100`,
   `agents-plugin-tool/cmd/ws-mcp/dashboard.go#L295-L322`,
   `agents-plugin-tool/internal/mcp/tool_aliases.go#L124-L132`).
2. **No agy worker.** `261117-feat-ws-hosted-antigravity-adapter` Phase 2
   decided (88) that the first slice is the hosted lead only
   (`ai-docs/tickets/.done/261117-feat-ws-hosted-antigravity-adapter.md`
   L358-374, L484-490, Result L542). `agentWorkerBackends` is
   `{claude, pi}` (`agents-plugin-tool/internal/wsagent/agent_tools.go`
   L44-47; refusal at L243-244; `agy.go` L21-24; pinned by `TestAgyIsLeadOnly`,
   `agents-plugin-tool/internal/wsagent/agy_lead_test.go#L422-L440`). The adapter ticket left three prerequisites
   for the worker role: a ws-written agy agent file, a worker permission
   posture, and the C2 tier-exposure rule.

## Decisions

- **(106)=a, 2026-11-18: underscore aliases on agy connections only.** The
  hosted HTTP route exposes `_` names (`tickets_query`) only to agy sessions
  and maps a called alias back to the dotted name before dispatch. Rejected:
  renaming the tools for every harness (breaks every other caller and the
  shipped playbooks' canonical names).
- **(132)=a, 2026-11-18: fix first, then one daemon swap.** Phase 1 lands
  before the owner's PC daemon moves to the track build, so a single swap
  serves both Gemini and Claude models in agy. The owner performs the swap and
  the live agy-lead check at home. Rejected: swapping right after the develop
  sync and swapping again after the fix (two swaps, each with start-time
  cleanup and failed in-flight calls).
- **(133)=no, 2026-11-18: no agy CLI on the office PC.** agy live checks and
  the custom-agent probes run on the owner's other machine.
- **(135), 2026-11-18: placement.** A `todo/` child of epic
  `261114-epic-ws-common-session-foundation` with the `related:` links above;
  the adapter ticket deferred the worker role without naming a ticket, and
  every agy ticket sits under this epic.
- **(136), 2026-11-18: agy detection from the hosted lead record.** An agy
  connection is recognized by the hosted lead record's `Backend == agy` (the
  record the hosted key already resolves for `hostedSessionKeyRule`).
  Rejected: a URL marker added in `agyHTTPServer` (`agy.go` L291-300), a
  second identity channel that can disagree with the record.
- **(137), 2026-11-18: one alias rule.** The alias mapping mirrors
  `agents-plugin-pi/src/bridge.ts` `sanitizeToolName` (L567); a collision is
  refused when the alias table is built, pinned by a test. Rejected: a new
  agy-specific naming scheme (two naming rules across harnesses).
- **(138), 2026-11-18, reworded by (145): harness detection in Phase 1.**
  The hosted agy lead's harness (the server's harness for that connection
  and the `renderLeadManual` render) lands with the aliases, since (102)=a
  ordered tool names (G2) and harness detection (G3) together. Per (145) it
  comes from the descriptor's `ManualHarness` ((143)), not from a new agy
  case in `server_harness.go`'s clientInfo list.
- **(139), 2026-11-18: Phase 1 is compatibility tier T0.** Aliases exist only
  in the agy connection's `tools/list` and are reverse-mapped server-side; no
  stored shape, hello field, or tool argument changes, and non-agy callers
  see dotted names unchanged.
- **(140), 2026-11-18: Phase 2 waits for probe evidence.** The agy agent file
  location and system-prompt replacement for workers come from probes 1-3 of
  `261118-research-ws-agy-custom-agent-prompt`; Phase 2 is not promoted to
  `ready/` before they land. Rejected: choosing them now, since (08)=c showed
  a malformed agent config silently falls back to the default agent.
- **(141)=a, 2026-11-18: agy worker permission posture.** A spawned agy
  worker runs with the same posture as today's spawned Claude and Pi workers
  (unattended, no approval prompts); `261115-feat-ws-permission-profile`
  later tightens every backend uniformly. Rejected: holding Phase 2 for that
  ticket's workdir-write cap.

- **(143)=a, 2026-11-18: agy behavior lives in a harness descriptor.** A
  small descriptor on `BackendRegistration` (`internal/wsagent/registration.go`
  L14-24) carries `ToolNameScheme`, `ManualHarness`, and `WorkerCapable` plus
  the worker profile; the common layer reads it, so the new work adds no
  `agy` branch to common code. A code read on 2026-11-18 found the existing
  seams (the launch-only `Backend` interface, `BackendRegistration`, the
  capability catalog) and about 50 harness-name branches in common code; those
  stay, captured in `261118-refactor-ws-harness-name-branches`. Rejected:
  adding an `agy` branch at each site, the existing leak pattern.
- **(105)=a, 2026-11-18: the agy worker runs on agy's default agent.** The
  ws worker instructions go in `_meta.antigravity.systemPrompt.append`, the
  hosted lead's mechanism (`internal/wsagent/agy.go` L247-254); ws writes no
  agent file. Probes 1-3 of `261118-research-ws-agy-custom-agent-prompt`
  found no custom-agent configuration with both the native tools and direct
  MCP tools. Rejected: (b) a custom agent with `excludeDefaultComponents`
  and an explicit tool list (ws calls through the `call_mcp_tool` gateway,
  silent fallback on a malformed file) for Phase 2; (c) holding Phase 2 for
  the quality A/B probe 4.
- **(149)=a, 2026-11-18: native subagent tools stay but are not used.** agy
  cannot remove them from the default agent (it ignores ACP
  `disableBuiltInTools` and has no tool flag,
  `ai-docs/tickets/.dropped/261115-research-ws-unified-agent-tool-surface.md`
  L640-644) and probe runs `b4-explicit`/`b9-explicit-collect` show
  `invoke_subagent` completing with no `session/request_permission`, so the
  daemon cannot deny it either. The worker instructions direct child work to
  `agent.spawn` and forbid native subagents; no daemon-side detection.
  Rejected: (b) letting the worker use native agy subagents, which run
  outside ws (no trace, mailbox, or tier control).
- **(150), 2026-11-18: ws over the daemon's HTTP MCP route.** The agy worker
  attaches ws through `attachMCP` as the Claude worker does
  (`internal/wsagent/runner_process.go` L158-163), selected by a descriptor
  field rather than a `rec.Backend == BackendClaude` check ((143)); agy
  reaches ws only over HTTP MCP (`agy.go` L137-139).
- **(151), 2026-11-18: the agy worker is not holder-held**, like the Claude
  worker; a daemon restart ends it. Rejected: holding it like the Pi worker
  (a new holder kind and reattach path). The hosted lead's held path is
  unchanged.
- **(152), 2026-11-18: posture (141) by auto-allow.** The daemon answers
  every agy worker `session/request_permission` with allow through the
  existing `AnswerApproval`, leaving each approval on the trace. Rejected:
  agy's `auto` session mode, whose description ("ask before irreversible
  actions") still prompts. Owner, 2026-11-18, with (101): bypass is the
  usual default for agy and Codex, with a later ws-side auto-mode hook.
- **(153), 2026-11-18: the agy worker runs in the user's agy home** (no
  `--gemini-dir`), like the hosted lead. Rejected: an isolated per-worker
  `--gemini-dir` (per-worker state no requirement asks for).
- **(154), 2026-11-18: `agy` is a tier-config harness key.** `agents.tier`
  maps `agy.<tier>`; today `normalizedHarness`
  (`internal/wsconfig/config.go` L713-723) knows only codex, claude and pi.
  agy is not added to the connection-harness detection that Phase 1 kept out
  (Result deviation (b)). Compatibility T0 expected (an old binary resolves
  no `agy` key); old-binary write preservation is verified per
  `ai-docs/manuals/release-compatibility.md` and recorded.
- **(155)=b, 2026-11-18: builtin agy tier defaults.** Like Claude's builtin
  tiers (`config.go` L640-664), agy ships builtin small..xlarge mappings,
  overridable through `config.tune`; the mapping is (157). Rejected: (a) no
  builtin, configured by the owner like Pi.
- **(156), 2026-11-18: the custom-agent path is Phase 3, behind a knob.**
  Phase 2 adds no knob: a knob with a single working value is a config
  surface nothing can exercise. The knob covers the agy worker only; the
  hosted lead keeps the default agent ((08)=c). Owner: "기본 에이전트 경로 +
  서브에이전트로 두고, knob를 둬서 나중에 custom agent 경로를 추가로
  테스트해보는 방향으로 갑시다".
- **(157), 2026-11-18: agy models are exposed without the level suffix, and
  effort is composed separately.** agy's thinking level is ws's effort, so
  ws's agy model vocabulary is the base name (`gemini-4-hydrogen`, not
  `gemini-4-hydrogen-high`) and a tier's `AgentTier.Effort` supplies the
  level at launch. Owner: "하네스 인터페이스에서 모델 이름을 뒤의
  -small|medium|high를 뺀 이름으로 노출하고, 그 뒤에 effort를 합성하는
  방향으로 가야 일관성이 있겠습니다". This matches the adapter's finding
  that the ACP model vocabulary is the `model` config option's base values,
  with `thought_level` a separate option
  (`ai-docs/tickets/.done/261117-feat-ws-hosted-antigravity-adapter.md`
  L251, L594-599). Builtin tiers under the `agy` harness key:
  small `gemini-4-hydrogen` effort `medium`, medium `gemini-4-hydrogen`
  effort `high`, large `gemini-4-argon` effort `high`, xlarge
  `gemini-4-carbon` effort `high` (owner: "수소(med) - 수소(high) -
  아르곤(high) - 카본", effort high). `agy models` (1.5.10, 2026-11-18) lists
  `gemini-4-hydrogen-{low,medium,high,xhigh}`,
  `gemini-4-argon-{low,medium,high}` and `gemini-4-carbon-{low,medium,high}`.
  The worker composes the effort through `session/set_config_option
  thought_level` where it is offered, or through the suffixed id otherwise,
  and records which one agy honored. Rejected: tier models that carry the
  level in the id (two spellings of one axis, conflicting with effort).
- **(158), 2026-11-18: the underscore aliases cover the agy worker's
  connection.** Phase 1 resolves the alias table through the hosted lead's
  connection identity only; the agy worker's HTTP MCP connection gets the
  same aliases, selected through its backend registration, so a Claude
  model inside an agy worker can call ws. Rejected: leaving workers on
  dotted names (every Claude-model turn in an agy worker would fail).
- **(159), 2026-11-18: tests run where the worker runs.** Phase 2 runs on
  the owner's macOS agy machine, so Go builds and tests run locally there;
  the w5090-only rule applies to the owner's Windows office PC. Rejected:
  routing this phase's tests to w5090, which has no agy CLI for the live
  check.
- **Phase 2 compatibility verdict, 2026-11-18: T0 for the agy tier bucket.**
  Against pre-change `b397590c2`, `internal/wsconfig/config.go:52` stores
  unrestricted nested maps, `:108-121` decodes them, `:159-170` merges every
  harness key, `:330-332` removes only the selected leaf/empty parent, and
  `:423` edits only the selected leaf. `:777-795` retains and serializes
  those maps. Old normalization (`:713-723`) cannot resolve agy but never
  filters its stored bucket; new reopen (`config.go:153-155,159-170`)
  overlays retained mappings on defaults. No recovery from lost bytes is
  needed. Project/global preservation tests also passed against the exact
  old source through a temporary Go overlay. This verdict covers existing
  `AgentTier` fields, not arbitrary future JSON members. The descriptor,
  worker connection hint and catalog add no persisted field or hello/tool
  argument change; existing dotted calls remain accepted.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)
- Manuals for the touched paths: `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/performance.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`.
- On the owner's Windows office PC, Go builds and tests run on remote w5090
  only, with temp files under `D:/Temp/<purpose>/`, deleted after use; on the
  owner's macOS agy machine they run locally ((159)).
- No agy CLI on the office PC ((133)); the live agy check runs on the
  owner's macOS agy machine.

## Prior Decisions

Local ticket/history fallback on 2026-11-18: `rationale.query` is unavailable
in this session; these pointers are not an exhaustive rationale-index result.

- 261118-research-ws-agy-custom-agent-prompt (2026-11-18, Confirmed Decisions): "(105)=a, 2026-11-18: the agy worker ships on agy's default agent with the ws instructions as a system-prompt append; the custom-agent path is tested later behind a knob" — bearing: supports
- 261118-research-ws-agy-custom-agent-prompt (2026-11-18, Verified Findings): "both <workspace>/.agents/agents/<name>.md and <gemini-dir>/config/agents/<name>.md are discovered; the home-scoped file loads from any workspace." — bearing: constrains
- 261118-refactor-ws-harness-name-branches (2026-11-18, Decisions): "(144)=a, 2026-11-18: capture only. The cleanup is captured in todo/ under the epic and executed later; Claude usage is scarce this week." — bearing: constrains
- 261117-feat-ws-hosted-antigravity-adapter (2026-11-18, Phase 2 Result): "The model vocabulary is the model config option's values, not availableModels' variant ids." — bearing: constrains
- 261117-feat-ws-agent-tools-mailbox-routing (2026-11-18, Phase 1 Result): "Admission is recomputed against the parent's current level at create and on every wake (Decision 14), using that ticket's Phase 2 admission machinery." — bearing: constrains
- 261117-feat-ws-hosted-prompt-assembly (2026-11-18, Decisions 8): "children get no lead workflow manual." — bearing: constrains
- 261115-feat-ws-permission-profile (2026-11-15, Decisions 2): "Inability to enforce the level never blocks either a lead or worker, including Pi." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/registration.go, agent_tools.go, agy.go, runner_process.go; internal/wsconfig/config.go; internal/mcp/tool_aliases.go; cmd/ws-mcp/agent_mcp.go; Phase 3 adds worker-agent config/render plumbing |
| scope.surface | cross-module | backend registration, Runner worker admission, MCP spelling/transport, tier configuration and worker instructions cross wsagent, mcp, wsconfig and daemon composition |
| scope.new_public_symbol | yes | Phase 2 adds exported BackendRegistration.WorkerCapable and a worker-profile descriptor field (exact latter spelling unspecified); Phase 3 adds a user-visible config knob (name unspecified) |
| scope.new_type_contract | yes | BackendRegistration gains worker eligibility/profile and HTTP-attachment selection metadata; agy becomes an accepted tier-config harness; Phase 3 knob selects default versus custom agent |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/agy_lead_test.go#L422-L440 must replace TestAgyIsLeadOnly; existing internal/mcp/tool_aliases_test.go covers Phase 1; Phase 3 test filenames are not specified |
| complexity.reuse_points | confirmed | Read BackendRegistration, AgentWorkerProfile/spawnableHarness, runner_process.go attachMCP path, AgyBackend.Start/open/AnswerApproval, wsconfig.normalizedHarness/defaultModelAliases and custom-agent research Q1-Q3 |
| complexity.side_effect_risk | high | unattended provider-backed child processes use the user's agy home, auto-allow permission requests and later write/select custom agents |
| risk.correctness | high | worker start currently requires lead/holder state; descriptor admission, restart/wake, HTTP connection aliases and per-model tier/effort selection must work together |
| risk.fit | high | Phase 2 must stay descriptor-driven and preserve authority/tool ceilings; lead-only alias lookup currently excludes children; custom-agent selection relies on version-specific log evidence |
| risk.test | high | unit tests plus eight live tier spawns/mailbox reports and Phase 3 quality A/B require installed harnesses/provider access; fallback/resume log behavior remains unprobed |
| risk.security_or_contract | high | auto-allow and native subagent availability interact with authority intent; shared agy home, HTTP credentials, tier-config downgrade preservation and fail-loud custom-agent fallback are contract boundaries |

## Phases

### Phase 1: Underscore tool-name aliases and agy harness detection

- Add the harness descriptor fields `ToolNameScheme` and `ManualHarness` to
  `BackendRegistration` ((143)); the agy registration declares the sanitize
  scheme and harness `agy`, the Claude registration declares harness
  `claude`, and every other registration keeps the identity scheme.
- `advertisedToolName` (`agents-plugin-tool/internal/mcp/server_inventory.go`
  L174-176, today an identity stub) reads the connection's alias table, built
  by one generic collision-refusing builder from the connection's
  `ToolNameScheme` ((137)); a connection without a hosted lead record keeps
  dotted names.
- `callTool` (`server_dispatch.go`, about L36-71) maps an alias back to its
  dotted name after `hostedSessionKeyRule` and before `toolAllowed`, so
  allowlists and dispatch keep seeing canonical names.
- The hosted lead's `Backend` ((136)) selects its descriptor; `renderLeadManual`
  (`cmd/ws-mcp/agent_mcp.go` L71-100) takes the harness from `ManualHarness`
  in place of its `if backend == BackendClaude` line, so the agy lead's manual
  renders with harness agy. Common code gains no `agy` branch.
- Verification: unit tests for the alias table (round trip, collision refusal,
  identity scheme unchanged), for the descriptor-driven harness render
  (Claude unchanged, agy new), run on w5090.
  Then the owner swaps the daemon to the track build once ((132)) and checks a
  hosted agy lead live: a Gemini model first, then a Claude model calling a
  dotted ws tool through its alias.

### Result (e69a4a620) - 2026-11-18

- Landed: `ToolNameScheme` and `ManualHarness` on `BackendRegistration`
  (`internal/wsagent/registration.go`, with `ApplyToolNameScheme`); Claude
  declares harness `claude`, agy declares harness `agy` and `sanitize`, Pi
  declares neither. One generic `buildToolAliasTable`
  (`internal/mcp/tool_aliases.go`) refuses a collision; the connection's
  descriptor is read once through the hosted lead record's `Backend` and
  `s.pi.agents.BackendRegistrations()`. `filteredTools` advertises aliases,
  `callTool` maps an alias back, `renderLeadManual` takes the harness from
  the descriptor (`newAgentRunner` builds the registry once). A note on the
  descriptor is in `ai-docs/manuals/ws-mcp.md`.
- Deviations: (a) the alias is mapped before `hostedSessionKeyRule`, since
  that rule fills an omitted `session_key` by canonical tool name; (b) the
  declared harness is a separate `playbookHarness()` for playbook render, not
  the connection's detected harness, because "agy" in `currentHarness()`
  would break a harness-defaulted `config.tune` (harness enum) and enter the
  harness buckets; `normalizedHarness` and `server_harness.go` gain no agy
  case; (c) alias spelling follows (137) with the `ws__` prefix, not the bare
  `tickets_query` of (106).
- w5090: `go build ./...`, `go vet` and `go test` of `cmd/ws-mcp`,
  `internal/mcp`, `internal/wsagent` pass; the new tests cover the alias round
  trip over the real inventory, collision refusal, identity, an alias call
  without `session_key`, and the descriptor-driven render.
- Forward (Phase 2): `WorkerCapable` and the worker profile go on the same
  descriptor; the registry literal now lives in `newAgentRunner`'s `backends`
  variable. The live agy check (Gemini, then a Claude model through an alias)
  is still the owner's after the daemon swap.

#### Edition (d902c66b8) - 2026-11-18

- The alias drops the `ws__` prefix: `tickets.query` is `tickets_query`.
  agy supplies `mcp_ws_` itself and (106) names the bare form; the character
  mapping, collision refusal, and API-pattern check of (137) are unchanged,
  and a test pins that every alias plus `mcp_ws_` fits 128 characters.
  Deviation (c) of the Result above is superseded.

### Phase 2: agy worker role

- Replace the global `agentWorkerBackends` map
  (`internal/wsagent/agent_tools.go` L44-47) with a `WorkerCapable` descriptor
  field plus the worker profile on `BackendRegistration` ((143)); the Claude
  and Pi registrations declare it, and the agy registration declares it with
  a new `agent-worker-agy` profile, so `agent.spawn agy.<tier>` is accepted
  and `agent.tiers()` lists agy tiers under the C2 rule of
  `261117-feat-ws-agent-tools-mailbox-routing`.
- `AgyBackend.Start` accepts a worker start (today it refuses any start
  without `spec.Lead` and `spec.Holder`, `agy.go` L133-135): the same
  `agy acp --process-per-session` launch in the user's agy home ((153)),
  default agent, worker instructions through the system-prompt append
  ((105)), not holder-held ((151)).
- The worker reaches ws over the daemon's HTTP MCP route ((150)); the
  descriptor, not a backend-name check, selects it in
  `runner_process.go`. That connection advertises and reverse-maps the
  underscore aliases ((158)).
- The daemon auto-allows every agy worker permission request ((152)).
- The agy worker profile's instructions direct child work to `agent.spawn`
  and forbid agy's native subagent tools ((149)).
- `agy` becomes a tier-config harness key with builtin small..xlarge
  defaults ((154), (155)): base model names with a separate effort, mapped
  per (157), the effort composed at launch and the honored mechanism
  recorded. Record the compatibility verdict with source lines.
- Replace `TestAgyIsLeadOnly` with tests that pin the worker role.
- Verification: unit tests run locally, then a live check from a hosted
  lead: `agent.spawn` of each of `claude.small`..`claude.xlarge` and
  `agy.small`..`agy.xlarge`, each worker reporting back through the mailbox.
  The live check runs on a scratch daemon with isolated state; the shared
  daemon is swapped only after this phase lands ((103), owner 2026-11-18).

### Result (b3e2e5e65) - 2026-11-18

- Landed: descriptor-driven `WorkerCapable`, `WorkerProfile`,
  `WorkerHTTPMCP`, and `WorkerAutoApprove`; Claude/Pi retain their worker
  roles and transports. agy admits all four builtin tiers, launches the
  default ACP agent in the user's home without a holder, appends the worker
  instructions, and auto-allows permission requests through the ordinary
  traced `AnswerApproval` path. Native child tools remain available but the
  appended instructions forbid their use and name `agent.spawn` and
  `mailbox.send` instead. No custom-agent file or knob was added.
- Worker HTTP MCP advertises/reverse-maps underscore aliases through the
  same backend descriptor. `WS_MCP_AGENT_SESSION_KEY` identifies the worker's
  record for spelling only: it grants neither lead authority nor omitted-key
  substitution. Canonical allowlists and the existing authority admission
  on each wake remain enforced. Worker tokens stay in memory, rotate on
  resume, and are revoked on process/daemon exit.
- Base model IDs and separate effort land in `3aebdf29a`. Launch prefers
  acknowledged `thought_level`, otherwise an offered suffixed config/ACP
  model ID, and logs the honored mechanism. Unsupported or ignored worker
  effort fails loudly; leads retain an unavailable effort for their next
  model. A base-only selection without explicit effort selects the
  harness's offered variant. Compatibility evidence is in Decisions above.
- Implementation decisions: reserve the complete `agent-worker-` namespace
  in compatibility spawn guards rather than keep a second backend map;
  declare worker catalog entries because a lead-only catalog otherwise
  disables the new worker's controls. Both changes have executable tests.
- Verification on macOS: per-file `gofmt -l` clean and `go build ./...` pass;
  `go vet ./internal/wsagent ./internal/wsconfig ./internal/mcp ./cmd/ws-mcp`
  passes. `go test -timeout 30m ./internal/wsagent ./internal/wsconfig
  ./internal/mcp ./cmd/ws-mcp` passes (128.032s, 19.740s, 918.028s,
  170.099s respectively; `/tmp/agy-worker-unit-final.log`). The initial
  broad run found two stale profile/namespace test expectations (corrected)
  and exceeded MCP's default 10-minute package timeout, not a failing test
  case. New tests exercise actual provider-free ACP subprocesses, approval
  answers, HTTP token/PID context, resume/revocation, alias allowlists,
  effort mechanisms and refusals, and config preservation.
- Live verification: `/tmp/ws-agy-phase2.20t3eN/live.py` used separate
  `WS_CACHE_HOME`, `WS_CONFIG_HOME`, a scratch Git root and an OS-assigned
  port. Calls over a real hosted agy lead's MCP connection spawned
  `agy.small..xlarge` and `claude.small..xlarge`; all eight workers completed
  and each sent a mailbox message received by the lead. All agy tiers
  honored `thought_level` with the required base models/efforts. Evidence:
  `live-output.log` and sanitized `summary.json` in that scratch directory.
  Every worker had no holder or persisted MCP token. Scratch sessions and
  daemon were closed; the shared daemon and account credentials were not
  changed, and nothing was pushed.
- Independent one-pass lite review of `b397590c2..b3e2e5e65`: clean, no
  findings (`/tmp/agy-phase2-lite-review.md`).
- I/O scope: no custom-agent file, new recovery file, or hot-path file
  operation; descriptor identity is read once per connection, and agy reads
  the existing composed instructions once per process launch. No Windows
  AV baseline measurement was run under this macOS-only verification.
- Omitted: Phase 3 and its custom-agent quality A/B, the owner's shared
  daemon swap, and a repository-wide test sweep beyond the four affected
  packages. The worker profile does not expose `workflow_manual`; the
  attempted call was refused, so repository manuals and the rendered worker
  contract supplied workflow context. No implementation/review finding
  remains. The ticket stays in `ready/` for Phase 3.

#### Edition (a5bc02ce0) - 2026-11-19

Owner directive: medium is now `gemini-4-neon`/`high` and large is
`gemini-4-neon`/`xhigh`; small and xlarge remain unchanged.

### Phase 3: Custom-agent path for the agy worker behind a knob

- A config knob selects the agy worker's agent: the default agent (default)
  or a ws-written custom agent ((156)). The hosted lead is unaffected.
- The custom agent uses `excludeDefaultComponents: true`, an explicit native
  tool list without the subagent tools, and `mcpServers: [{name: ws}]`; its
  body carries the worker instructions plus the schemas of the ws tools the
  worker may call, so the `call_mcp_tool` gateway needs no schema-cache read
  (research Proposal, `261118-research-ws-agy-custom-agent-prompt`).
- A start whose agent falls back to the default agent fails loudly, keyed on
  agy's fallback log line (research Q3); the agent file location follows
  research Q3.
- Verification: unit tests for the knob and the agent file render, then the
  research's quality A/B (probe 4) on the same task with each knob value.

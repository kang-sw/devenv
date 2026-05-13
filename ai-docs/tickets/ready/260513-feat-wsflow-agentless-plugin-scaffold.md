---
title: wsflow agentless plugin scaffold
parent: 260513-epic-wsflow-agentless-plugin
related:
  260513-feat-wsflow-agentless-runtime-mode: prerequisite shared runtime mode
  260429-research-host-neutral-ws-plugin: host-neutral plugin architecture anchor
spec:
  - 260513-wsflow-agentless-plugin-package
  - 260513-wsflow-agentless-skill-surface
  - 260513-wsflow-claude-compatible-package
related-mental-model:
  - plugin-runtime
  - mcp-runtime
  - named-agent-runtime
  - workflow-skills
  - claude-compatibility
---

# wsflow agentless plugin scaffold

## Background

The full `ws` plugin now ships as `agents-plugin/` with Codex and Claude
metadata, a launcher-managed `ws-mcp` runtime, a runtime contract in
`runtime.json`, and workflow skills that use `ws/<tool>` MCP notation plus
`ws:lead-*` plugin skill invocations.

Internal users need a separate lightweight distribution named `wsflow` that
keeps the project workflow, documentation, Git, ticket, spec, and setup tools,
but removes ws named-agent and subquery orchestration. The package should be a
derivative copy rather than a separate product line: specs and workflow meaning
remain anchored to the canonical ws runtime, while wsflow records only variant
constraints in the existing specs.

## Decisions

- Create the package under `agents-plugin-wsflow/`.
- Use `wsflow` as the plugin name, MCP server key, skill namespace prefix, and
  user-facing MCP notation stem.
- Reuse the same `ws-mcp` binary and launcher rather than building a separate
  runtime.
- Depend on the shared runtime mode that supports:
  `WS_MCP_NO_AGENT=1`, `WS_MCP_NAMESPACE=wsflow`, and
  `WS_MCP_SETUP_TOOL=setup`.
- Keep actual MCP tool names stable where they are already generic, such as
  `project_tree`, `git.status`, `tickets.list`, and `specs.find`. The host MCP
  server key supplies the `wsflow/<tool>` namespace.
- Treat `ws.setup` as the special legacy name. wsflow should advertise `setup`
  and may keep `ws.setup` only as hidden compatibility dispatch if that keeps
  shared runtime reuse simple.
- Keep `agents-plugin/` as the canonical full distribution. wsflow follows the
  same semantic changes when copied or caller-visible surfaces are touched, but
  wsflow does not get an independent spec tree.
- Treat the ws relationship as internal maintenance context only. Distributed
  wsflow manifests, skills, default prompts, and ordinary user-facing guidance
  should present wsflow as its own workflow plugin, not as a ws-aware or
  ws-lite product.
- Keep one-shot native subagent guidance where it improves investigation or
  review. Native subagents have been verified able to use MCP documentation and
  discovery tools, so wsflow skills may instruct them to use wsflow read tools
  such as `wsflow/convention.read`, `wsflow/infra.read`,
  `wsflow/project_tree`, `wsflow/specs.*`, `wsflow/tickets.*`,
  `wsflow/mental_models.*`, and `wsflow/git.*`.
- Include `lead-ship` in the shipped wsflow skill set for workflow
  centralization and documentation completeness.
- Include forge workflows in the shipped wsflow skill set because legacy
  project bootstrap depends on spec and mental-model reconstruction. Rewrite
  their survey and validation steps around self-contained native subagents or
  direct exploration instead of wsflow-managed agent sessions.
- Exclude persistent multi-turn orchestration skills and upstream-maintenance
  authoring helpers from the shipped wsflow skill set: `lead-write-code`,
  `lead-write-skeleton`, `lead-sprint`, `lead-salvage`, and
  `lead-skill-authoring`.
- Keep direct, documentation, bootstrap, and reconstruction workflows in the
  shipped wsflow skill set:
  `lead-workflow-manual`, `lead-discuss`, `lead-write-spec`,
  `lead-write-ticket`, `lead-proceed`, `lead-implement`, `lead-edit`,
  `lead-update-spec`, `lead-bootstrap`, `lead-add-rule`, `lead-exit-session`,
  `lead-ship`, `lead-verify-discussion`, `lead-forge-spec`, and
  `lead-forge-mental-model`.

## Constraints

- Do not expose `agents.*`, `subquery`, `config.agents_tier`, agent-backed API
  ask tools, agent debug tools, agent CLI commands, agent prompt requirements,
  or follow-up text that tells users to call ws named-agent tools when
  `WS_MCP_NO_AGENT=1` is active.
- Do not use `WS_MCP_TOOL_PROFILE=leaf` as the product mechanism. Tool profiles
  are containment filters; wsflow needs a distribution contract.
- Runtime compatibility checks must compare against the wsflow tool and command
  contract, not the full ws contract.
- wsflow skill text must not instruct users to call `ws/subquery`,
  `ws/agents.*`, or `ws:lead-*`.
- wsflow user-facing package text must avoid requiring ws knowledge. Use
  `wsflow`, `wsflow:lead-*`, and `wsflow/<tool>` notation in distributed
  wsflow files; keep ws references only in repository maintenance docs,
  tests, compatibility comments, or hidden implementation details where they are
  unavoidable.
- Claude compatibility must use a package-local `.claude-plugin/plugin.json`
  and the shared launcher pattern. Do not revive `claude-plugin/`.
- If a copied wsflow surface cannot be updated with a full ws change, create a
  follow-up ticket rather than leaving untracked drift.

## Phases

### Phase 1: Package and runtime contract scaffold

Create `agents-plugin-wsflow/` as a derivative package with Codex and Claude
metadata, package-local MCP config, a wsflow `runtime.json`, and launcher
coverage for the wsflow runtime contract.

Suggested approach:

- Copy the full plugin package structure only where wsflow needs it.
- Set the Codex plugin name and Claude manifest name to `wsflow`.
- Configure the MCP server key as `wsflow`.
- Inject `WS_MCP_NO_AGENT=1`, `WS_MCP_NAMESPACE=wsflow`, and
  `WS_MCP_SETUP_TOOL=setup` through wsflow `.mcp.json`.
- Remove `agents.*`, `subquery`, and agent CLI commands from the wsflow
  `runtime.json` requirements.
- Keep version and launcher compatibility aligned with the full ws release
  mechanism unless a later ticket intentionally separates them.

Acceptance criteria:

- Codex and Claude manifests exist under `agents-plugin-wsflow/`.
- wsflow `.mcp.json` starts the shared launcher with the wsflow env contract.
- wsflow `runtime.json` represents the agentless required tool and command
  surface.
- Launcher capability validation can distinguish full ws and wsflow contracts.

### Result (9f066d9) - 2026-05-13

Created `agents-plugin-wsflow/` with Codex and Claude manifests, package-local
MCP configuration, shared launcher copies, an agentless `runtime.json`, an empty
tracked skills root for later normalization, and package tests that verify the
runtime contract against `ws-mcp runtime capabilities` under the same no-agent
environment used by `.mcp.json`.

The scaffold intentionally does not copy workflow skills yet. Phase 3 remains
responsible for curated semantic rewrites under the `wsflow:lead-*` namespace.

### Phase 2: Package runtime contract integration

Wire the wsflow package to the runtime mode delivered by
`260513-feat-wsflow-agentless-runtime-mode`.

Suggested approach:

- Verify wsflow `runtime.json` matches no-agent `runtime.capabilities`.
- Keep `api.list` if the runtime mode exposes it as read-only cache discovery.
- Exclude `api.ask`, `api.ask_async`, `api.status`, `api.result`, and
  `api.cancel` from the wsflow runtime contract.
- Keep the setup tool name aligned with `WS_MCP_SETUP_TOOL=setup`.

Acceptance criteria:

- wsflow package validation compares against the no-agent runtime surface, not
  the full ws runtime surface.
- wsflow `runtime.json` excludes agent-backed tools and commands while keeping
  non-agent workflow tools.
- wsflow startup does not require or advertise ws named-agent capabilities.

### Result (c7cd740) - 2026-05-13

Added an exact runtime capability contract mode to the shared launcher and
enabled it for `agents-plugin-wsflow/runtime.json`. The full ws package keeps
the existing required-surface compatibility behavior, while wsflow now requires
the runtime-reported tool and command sets to exactly match its agentless
contract and skips weaker fallback probes after exact capability mismatch.

Package tests verify wsflow `.mcp.json` selects `WS_MCP_NO_AGENT=1`,
`WS_MCP_NAMESPACE=wsflow`, and `WS_MCP_SETUP_TOOL=setup`; verify
`runtime.json` opts into exact matching; and compare the package contract
against no-agent `ws-mcp runtime capabilities`.

### Phase 3: wsflow skill normalization

Create or trim wsflow skills so users see agentless workflow instructions under
the `wsflow:lead-*` namespace.

Suggested approach:

- Replace `ws/` MCP notation with `wsflow/` in wsflow skill text.
- Replace `ws:lead-*` plugin-skill invocations with `wsflow:lead-*`.
- Remove instructions that require `ws/subquery` or `ws/agents.*`.
- Prefer native host agent/subagent guidance for broad exploration or review
  when the host offers it, and direct local search/read/edit guidance when it
  does not.
- In native subagent prompts, point workers at wsflow read tools for project
  context and conventions instead of assuming wsflow-managed agent sessions.
- Rewrite `lead-implement` as a direct-edit harness. Remove skeleton routing,
  delegated write-code routing, ws-managed mental-model updater calls, and
  delegated merge-path assumptions.
- Rewrite `lead-edit` to use native subagent review when available, or a
  lead-only risk review with rationale for low-risk changes.
- Keep `lead-ship` in the wsflow package, but use wsflow naming and package
  configuration once wsflow ship configuration exists.
- Do not include `lead-skill-authoring` in the wsflow package. It is an
  upstream maintenance helper for authoring and auditing workflow skills, not a
  downstream wsflow workflow surface.
- Rewrite `lead-forge-spec` and `lead-forge-mental-model` so survey fan-out,
  verification, and ticket-association checks use self-contained native
  subagents or direct exploration. The lead still owns conventions, anchor
  generation, document authorship, final judgment, and commits.
- Keep detailed implementation decisions in child tickets and specs rather than
  creating a separate wsflow doctrine.

Acceptance criteria:

- The shipped wsflow skill set includes only:
  `lead-workflow-manual`, `lead-discuss`, `lead-write-spec`,
  `lead-write-ticket`, `lead-proceed`, `lead-implement`, `lead-edit`,
  `lead-update-spec`, `lead-bootstrap`, `lead-add-rule`, `lead-exit-session`,
  `lead-ship`, `lead-verify-discussion`, `lead-forge-spec`, and
  `lead-forge-mental-model`.
- The shipped wsflow skill set excludes `lead-write-code`,
  `lead-write-skeleton`, `lead-sprint`, `lead-salvage`, and
  `lead-skill-authoring`.
- wsflow forge workflows use native self-contained survey or audit workers only
  for read-only investigation. They do not refer to wsflow-managed subquery
  keys, persistent agents, or result collection through `agents.*`.
- wsflow skills do not mention ws named-agent or subquery MCP calls.
- wsflow workflow manual documents the wsflow MCP notation and the absence of
  wsflow-managed named-agent primitives.
- wsflow skill descriptions and optional host metadata point to `wsflow`
  invocations.
- wsflow distributed skill text and manifests do not describe the package as a
  ws variant or require ws-aware user behavior.
- Native subagent guidance is one-shot and host-native. It tells workers to use
  wsflow MCP read/context tools where useful, and never describes a persistent
  wsflow-managed agent session.

### Phase 4: Documentation and drift guard

Record the wsflow derivative-maintenance rule in the existing specs and project
memory without creating a parallel spec set. The repository already has
`ai-docs/ref/wsflow-mirroring.md` and an index reminder to read it before
wsflow-relevant full ws edits; this phase keeps those rules current and adds
the remaining automated verification guards so future full `agents-plugin/`
skill edits evaluate wsflow drift automatically during normal workflow
hygiene.

Suggested approach:

- Update `plugin-runtime`, `mcp-tools`, `workflow-skills`, and
  `claude-compatibility` specs with wsflow variant constraints.
- Update relevant mental models so future implementation changes check wsflow
  when copied, packaged, or caller-visible surfaces change.
- Update project memory to list `agents-plugin-wsflow/` as an active derivative
  distribution after the package exists.
- Keep `ai-docs/ref/wsflow-mirroring.md` and the `ai-docs/_index.md`
  read-before-editing reminder aligned with the final shipped wsflow package.
  Editing a full `agents-plugin/skills/lead-*` skill that is included in the
  shipped wsflow skill set must either update the corresponding
  `agents-plugin-wsflow/skills/lead-*` skill in the same logical change or
  record an explicit follow-up ticket explaining why it cannot be mirrored.
  Editing a full skill excluded from wsflow must still check whether wsflow
  docs, workflow manual, or exclusion rationale drifted.
- Keep `lead-skill-authoring` self-contained. Do not make it read
  repository-local reference files at invocation; that would break downstream
  plugin use. Mirror policy belongs in repository authoring guidance,
  workflow-maintenance instructions, and static checks, not in distributed skill
  execution.
- Add a static wsflow skill-bundle verification command and wire it into the
  relevant local or release verification path. The check should fail when
  distributed wsflow skills contain forbidden full-ws references such as
  `ws/`, `ws:`, `ws.`, `subquery`, `agents.register`, `agents.call`,
  `agents.result`, `mental-model-updater`, `lead-write-code`,
  `lead-write-skeleton`, `lead-sprint`, or `lead-salvage`, except for explicit
  internal-maintenance allowlist paths.
- Add a drift inventory check for the shipped wsflow skill set. The check
  should report included full skills that have no wsflow counterpart, wsflow
  skills that are not in the shipped allowlist, and excluded full skills that
  accidentally appear in `agents-plugin-wsflow/skills/`.
- Document that wsflow is not a generated mirror. It is a curated derivative:
  automatic checks should force review and drift detection, while semantic
  rewrites remain lead-owned.

Acceptance criteria:

- Existing specs describe wsflow as an internal derivative distribution.
- Future full ws changes have a documented rule to evaluate wsflow drift.
- No separate wsflow spec corpus is introduced.
- `ai-docs/ref/wsflow-mirroring.md` remains accurate for the final shipped
  wsflow skill inventory and tells future skill edits how to update or
  explicitly defer wsflow mirrors.
- `lead-skill-authoring` remains self-contained and does not depend on
  `ai-docs/ref/skill-authoring.md` at invocation.
- A static verification path checks the wsflow distributed skill set for
  forbidden full-ws references and shipped-skill inventory drift.
- The verification path distinguishes curated semantic rewrites from mechanical
  mirroring; it should not require wsflow skills to be text-identical to full
  ws skills.

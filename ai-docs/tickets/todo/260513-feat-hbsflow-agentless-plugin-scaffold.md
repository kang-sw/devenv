---
title: hbsflow agentless plugin scaffold
parent: 260513-epic-hbsflow-agentless-plugin
related:
  260513-feat-hbsflow-agentless-runtime-mode: prerequisite shared runtime mode
  260429-research-host-neutral-ws-plugin: host-neutral plugin architecture anchor
related-mental-model:
  - plugin-runtime
  - mcp-runtime
  - named-agent-runtime
  - workflow-skills
  - claude-compatibility
---

# hbsflow agentless plugin scaffold

## Background

The full `ws` plugin now ships as `agents-plugin/` with Codex and Claude
metadata, a launcher-managed `ws-mcp` runtime, a runtime contract in
`runtime.json`, and workflow skills that use `ws/<tool>` MCP notation plus
`ws:lead-*` plugin skill invocations.

Internal users need a separate lightweight distribution named `hbsflow` that
keeps the project workflow, documentation, Git, ticket, spec, and setup tools,
but removes ws named-agent and subquery orchestration. The package should be a
derivative copy rather than a separate product line: specs and workflow meaning
remain anchored to the canonical ws runtime, while hbsflow records only variant
constraints in the existing specs.

## Decisions

- Create the package under `agents-plugin-hbsflow/`.
- Use `hbsflow` as the plugin name, MCP server key, skill namespace prefix, and
  user-facing MCP notation stem.
- Reuse the same `ws-mcp` binary and launcher rather than building a separate
  runtime.
- Depend on the shared runtime mode that supports:
  `WS_MCP_NO_AGENT=1`, `WS_MCP_NAMESPACE=hbsflow`, and
  `WS_MCP_SETUP_TOOL=setup`.
- Keep actual MCP tool names stable where they are already generic, such as
  `project_tree`, `git.status`, `tickets.list`, and `specs.find`. The host MCP
  server key supplies the `hbsflow/<tool>` namespace.
- Treat `ws.setup` as the special legacy name. hbsflow should advertise `setup`
  and may keep `ws.setup` only as hidden compatibility dispatch if that keeps
  shared runtime reuse simple.
- Keep `agents-plugin/` as the canonical full distribution. hbsflow follows the
  same semantic changes when copied or caller-visible surfaces are touched, but
  hbsflow does not get an independent spec tree.
- Treat the ws relationship as internal maintenance context only. Distributed
  hbsflow manifests, skills, default prompts, and ordinary user-facing guidance
  should present hbsflow as its own workflow plugin, not as a ws-aware or
  ws-lite product.
- Keep one-shot native subagent guidance where it improves investigation or
  review. Native subagents have been verified able to use MCP documentation and
  discovery tools, so hbsflow skills may instruct them to use hbsflow read tools
  such as `hbsflow/convention.read`, `hbsflow/infra.read`,
  `hbsflow/project_tree`, `hbsflow/specs.*`, `hbsflow/tickets.*`,
  `hbsflow/mental_models.*`, and `hbsflow/git.*`.
- Include `lead-ship` in the shipped hbsflow skill set for workflow
  centralization and documentation completeness.
- Include forge workflows in the shipped hbsflow skill set because legacy
  project bootstrap depends on spec and mental-model reconstruction. Rewrite
  their survey and validation steps around self-contained native subagents or
  direct exploration instead of hbsflow-managed agent sessions.
- Exclude persistent multi-turn orchestration skills from the shipped hbsflow
  skill set: `lead-write-code`, `lead-write-skeleton`, `lead-sprint`,
  and `lead-salvage`.
- Keep direct, documentation, bootstrap, and reconstruction workflows in the
  shipped hbsflow skill set:
  `lead-workflow-manual`, `lead-discuss`, `lead-write-spec`,
  `lead-write-ticket`, `lead-proceed`, `lead-implement`, `lead-edit`,
  `lead-update-spec`, `lead-bootstrap`, `lead-add-rule`, `lead-exit-session`,
  `lead-skill-authoring`, `lead-ship`, `lead-verify-discussion`,
  `lead-forge-spec`, and `lead-forge-mental-model`.

## Constraints

- Do not expose `agents.*`, `subquery`, `config.agents_tier`, agent-backed API
  ask tools, agent debug tools, agent CLI commands, agent prompt requirements,
  or follow-up text that tells users to call ws named-agent tools when
  `WS_MCP_NO_AGENT=1` is active.
- Do not use `WS_MCP_TOOL_PROFILE=leaf` as the product mechanism. Tool profiles
  are containment filters; hbsflow needs a distribution contract.
- Runtime compatibility checks must compare against the hbsflow tool and command
  contract, not the full ws contract.
- hbsflow skill text must not instruct users to call `ws/subquery`,
  `ws/agents.*`, or `ws:lead-*`.
- hbsflow user-facing package text must avoid requiring ws knowledge. Use
  `hbsflow`, `hbsflow:lead-*`, and `hbsflow/<tool>` notation in distributed
  hbsflow files; keep ws references only in repository maintenance docs,
  tests, compatibility comments, or hidden implementation details where they are
  unavoidable.
- Claude compatibility must use a package-local `.claude-plugin/plugin.json`
  and the shared launcher pattern. Do not revive `claude-plugin/`.
- If a copied hbsflow surface cannot be updated with a full ws change, create a
  follow-up ticket rather than leaving untracked drift.

## Phases

### Phase 1: Package and runtime contract scaffold

Create `agents-plugin-hbsflow/` as a derivative package with Codex and Claude
metadata, package-local MCP config, a hbsflow `runtime.json`, and launcher
coverage for the hbsflow runtime contract.

Suggested approach:

- Copy the full plugin package structure only where hbsflow needs it.
- Set the Codex plugin name and Claude manifest name to `hbsflow`.
- Configure the MCP server key as `hbsflow`.
- Inject `WS_MCP_NO_AGENT=1`, `WS_MCP_NAMESPACE=hbsflow`, and
  `WS_MCP_SETUP_TOOL=setup` through hbsflow `.mcp.json`.
- Remove `agents.*`, `subquery`, and agent CLI commands from the hbsflow
  `runtime.json` requirements.
- Keep version and launcher compatibility aligned with the full ws release
  mechanism unless a later ticket intentionally separates them.

Acceptance criteria:

- Codex and Claude manifests exist under `agents-plugin-hbsflow/`.
- hbsflow `.mcp.json` starts the shared launcher with the hbsflow env contract.
- hbsflow `runtime.json` represents the agentless required tool and command
  surface.
- Launcher capability validation can distinguish full ws and hbsflow contracts.

### Phase 2: Package runtime contract integration

Wire the hbsflow package to the runtime mode delivered by
`260513-feat-hbsflow-agentless-runtime-mode`.

Suggested approach:

- Verify hbsflow `runtime.json` matches no-agent `runtime.capabilities`.
- Keep `api.list` if the runtime mode exposes it as read-only cache discovery.
- Exclude `api.ask`, `api.ask_async`, `api.status`, `api.result`, and
  `api.cancel` from the hbsflow runtime contract.
- Keep the setup tool name aligned with `WS_MCP_SETUP_TOOL=setup`.

Acceptance criteria:

- hbsflow package validation compares against the no-agent runtime surface, not
  the full ws runtime surface.
- hbsflow `runtime.json` excludes agent-backed tools and commands while keeping
  non-agent workflow tools.
- hbsflow startup does not require or advertise ws named-agent capabilities.

### Phase 3: hbsflow skill normalization

Create or trim hbsflow skills so users see agentless workflow instructions under
the `hbsflow:lead-*` namespace.

Suggested approach:

- Replace `ws/` MCP notation with `hbsflow/` in hbsflow skill text.
- Replace `ws:lead-*` plugin-skill invocations with `hbsflow:lead-*`.
- Remove instructions that require `ws/subquery` or `ws/agents.*`.
- Prefer native host agent/subagent guidance for broad exploration or review
  when the host offers it, and direct local search/read/edit guidance when it
  does not.
- In native subagent prompts, point workers at hbsflow read tools for project
  context and conventions instead of assuming hbsflow-managed agent sessions.
- Rewrite `lead-implement` as a direct-edit harness. Remove skeleton routing,
  delegated write-code routing, ws-managed mental-model updater calls, and
  delegated merge-path assumptions.
- Rewrite `lead-edit` to use native subagent review when available, or a
  lead-only risk review with rationale for low-risk changes.
- Keep `lead-ship` in the hbsflow package, but use hbsflow naming and package
  configuration once hbsflow ship configuration exists.
- Rewrite `lead-forge-spec` and `lead-forge-mental-model` so survey fan-out,
  verification, and ticket-association checks use self-contained native
  subagents or direct exploration. The lead still owns conventions, anchor
  generation, document authorship, final judgment, and commits.
- Keep detailed implementation decisions in child tickets and specs rather than
  creating a separate hbsflow doctrine.

Acceptance criteria:

- The shipped hbsflow skill set includes only:
  `lead-workflow-manual`, `lead-discuss`, `lead-write-spec`,
  `lead-write-ticket`, `lead-proceed`, `lead-implement`, `lead-edit`,
  `lead-update-spec`, `lead-bootstrap`, `lead-add-rule`, `lead-exit-session`,
  `lead-skill-authoring`, `lead-ship`, `lead-verify-discussion`,
  `lead-forge-spec`, and `lead-forge-mental-model`.
- The shipped hbsflow skill set excludes `lead-write-code`,
  `lead-write-skeleton`, `lead-sprint`, and `lead-salvage`.
- hbsflow forge workflows use native self-contained survey or audit workers only
  for read-only investigation. They do not refer to hbsflow-managed subquery
  keys, persistent agents, or result collection through `agents.*`.
- hbsflow skills do not mention ws named-agent or subquery MCP calls.
- hbsflow workflow manual documents the hbsflow MCP notation and the absence of
  hbsflow-managed named-agent primitives.
- hbsflow skill descriptions and optional host metadata point to `hbsflow`
  invocations.
- hbsflow distributed skill text and manifests do not describe the package as a
  ws variant or require ws-aware user behavior.
- Native subagent guidance is one-shot and host-native. It tells workers to use
  hbsflow MCP read/context tools where useful, and never describes a persistent
  hbsflow-managed agent session.

### Phase 4: Documentation and drift guard

Record the hbsflow derivative-maintenance rule in the existing specs and project
memory without creating a parallel spec set.

Suggested approach:

- Update `plugin-runtime`, `mcp-tools`, `workflow-skills`, and
  `claude-compatibility` specs with hbsflow variant constraints.
- Update relevant mental models so future implementation changes check hbsflow
  when copied, packaged, or caller-visible surfaces change.
- Update project memory to list `agents-plugin-hbsflow/` as an active derivative
  distribution after the package exists.

Acceptance criteria:

- Existing specs describe hbsflow as an internal derivative distribution.
- Future full ws changes have a documented rule to evaluate hbsflow drift.
- No separate hbsflow spec corpus is introduced.

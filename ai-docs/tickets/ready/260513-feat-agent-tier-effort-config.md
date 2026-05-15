---
title: Agent tier effort configuration
spec:
  - 260513-harness-local-agent-tier-config
  - 260508-model-alias-config-tools
  - 260508-agents-register-model-alias-field
  - 260508-harness-aware-model-aliases
  - 260505-codex-agent-session-jsonl-handling
  - 260505-claude-agent-runner
plans:
  phase-2: 2026-05/15-260513-feat-agent-tier-effort-config
related-mental-model:
  - named-agent-runtime
  - mcp-runtime
---

# Agent tier effort configuration

## Background

Named-agent model selection already flows through portable `light`, `core`, and
`deep` model aliases configured by `config.agents_tier`. Reasoning effort should
follow the same path instead of adding another agent registration option or
backend-specific workflow knob.

The default effort behavior should remain unset: if an alias mapping has no
configured effort, ws must not force a backend effort value. Users who want a
specific effort should configure it on the alias mapping through
`config.agents_tier`.

## Decisions

- Extend `config.agents_tier` with an optional `effort` field for alias mapping
  updates.
- Keep `agents.register` free of direct effort input. Agents select `model:
  "light" | "core" | "deep"` or the legacy `tier` field, and the resolved alias
  mapping supplies backend, model, and effort.
- Treat empty effort and explicit `none` as no forced backend effort. Persist the
  no-override state as an omitted or empty value, not as a backend argument.
- Support portable effort values that can map cleanly across current runners:
  `none`, `low`, `medium`, `high`, and `xhigh`.

## Constraints

- Preserve `backend` as the execution backend in alias mappings; do not make
  effort selection change backend inference.
- Preserve harness-aware alias resolution. Codex, Claude, and default mappings
  may each carry their own effort value.
- Do not introduce an effort field on `agents.register`, `subquery`, or prompt
  frontmatter in this ticket.
- Do not make default alias mappings force effort. Existing users should keep
  current backend defaults until they explicitly configure effort.
- Validate or normalize unsupported values before runner invocation so invalid
  effort names do not become backend-specific surprises.

## Prior Art

- `config.agents_tier` already persists harness-aware backend/model mappings for
  `light`, `core`, and `deep`.
- Codex CLI exposes general config overrides through `-c/--config`; the Codex
  config reference includes `model_reasoning_effort`.
- Claude CLI exposes session model and effort controls through `--model` and
  `--effort`.

## Phases

### Phase 1: Extend alias configuration and metadata

Add optional effort storage to the ws agent alias configuration shape, MCP
schema, config display output, registration resolution, agent metadata, and
tests. `config.agents_tier` should be the only public configuration surface for
model effort.

Acceptance criteria:

- `config.agents_tier(tier: "core", effort: "high")` stores effort on the
  selected harness alias mapping without requiring a model change.
- `config.agents_tier(tier: "core", model: "gpt-5.5", effort: "medium")` and
  `config.agents_tier(tier: "deep", model: "gpt-5.5", effort: "high")` support
  the user's current Codex alias override setup.
- `config.show` and `agents.status` make the resolved effort visible enough to
  diagnose alias routing.
- Existing config files without effort continue loading with no behavior change.
- Invalid effort values are rejected or normalized consistently.

### Result (cbec9ec) - 2026-05-15

Implemented optional effort metadata on harness-aware model alias mappings.
`config.agents_tier` accepts portable effort values, treats omitted, empty, and
`none` as the no-override state, and preserves existing backend/model defaults
for config files without effort. `config.show` exposes stored effort in JSON and
readable output, and `agents.status` exposes the resolved registration effort
without adding any direct effort input to `agents.register`. Runner invocation
remains deferred to Phase 2.

### Phase 2: Apply effort in backend runners

Pass resolved effort into runner requests and map it to backend-specific
invocation flags only when non-empty.

Acceptance criteria:

- Codex-backed calls with resolved effort add a Codex config override for
  `model_reasoning_effort`.
- Claude-backed calls with resolved effort pass `--effort`.
- Calls with no resolved effort do not pass any effort override.
- Runner tests cover Codex, Claude, and no-override behavior.

### Result (8cc0c5e) - 2026-05-15

Implemented runner application for resolved alias effort. Registered agent
effort now flows into `RunnerRequest`; Codex-backed calls add
`model_reasoning_effort=<effort>` through Codex configuration only when effort
is non-empty, and Claude-backed calls pass `--effort <effort>` only when effort
is non-empty. Manager handoff, Codex invocation, Claude invocation, and
no-override behavior are covered by `go test ./internal/wsagent`.

### Phase 3: Update workflow docs and release metadata

Update specs, mental models, runtime metadata, and any relevant user-facing
configuration guidance so model aliases are documented as the single route for
named-agent effort selection.

Acceptance criteria:

- `mcp-tools` and `named-agent-runtime` specs describe alias effort resolution
  and runner application.
- Mental models record the single-entry-point rule and the default no-override
  behavior.
- Runtime metadata and tests remain aligned with the public MCP schema.

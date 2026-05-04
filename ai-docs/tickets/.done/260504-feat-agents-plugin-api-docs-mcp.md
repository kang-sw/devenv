---
title: agents-plugin API documentation MCP migration
related:
  260429-feat-api-deps: Claude plugin prior art for ws-ask-api and ai-docs/.deps cache behavior
  260503-epic-agents-plugin-skill-porting: skill migration roadmap that should expose the API docs primitive to worker agents
  260503-feat-agents-plugin-runtime-boundary: MCP runtime and launcher boundary that will host the API docs tool
spec:
  - 260429-ws-ask-api-tool
  - 260429-ws-ask-api-exit-code
  - 260429-api-deps-cache-layout
  - 260429-pre-router-domain-resolution
  - 260429-per-domain-executor-session
  - 260429-per-domain-lock
  - 260429-worker-agent-contract
related-mental-model:
  - executor-wrapup
  - personal-devenv
completed: 2026-05-04
---

# agents-plugin API documentation MCP migration

## Background

The Claude plugin already has working prior art for external API documentation:
`ws-ask-api`, `ws-ask-api-internal`, `pre-router`, `api-doc-manager`, and
workflow guidance that directs worker agents away from direct web browsing for
third-party API lookup. The Agents plugin currently lacks the equivalent
runtime surface. `agents-plugin/skills/workflow/SKILL.md` still treats API
documentation routing as planned/specialized, which leaves downstream Codex
workers without the cached, auditable API docs path.

This ticket ports the behavior into the Agents/MCP runtime without mutating
`claude-plugin/`, which now remains an out-of-support compatibility tree.

## Decisions

- Preserve the two-layer design: lightweight domain pre-router first, then
  persistent per-domain API doc manager sessions.
- Preserve the project-local `ai-docs/.deps/` cache layout and direct-access
  prohibition for ordinary worker agents.
- Prefer MCP tools as the shared Agents contract instead of reintroducing
  PATH-only `ws-ask-api` helpers.
- Reuse the Claude prompt content as prior art, but normalize it into embedded
  `agents-plugin-tool` prompts.
- Keep per-domain locking in the runtime/tool layer, not inside the manager
  prompt.
- Do not expose refresh or stale-check management commands as first-pass MCP
  tools. Staleness is a per-domain manager responsibility: the manager creates
  the check script during bootstrap and runs it at the start of each session
  before answering.

## Constraints

- Do not modify `claude-plugin/` for this migration.
- Keep `ai-docs/.deps/` out of `AGENTS.md`, `CLAUDE.md`, and project indexes
  except where this ticket or specs document the hidden cache contract.
- API docs answers must cite cached doc files or official fetched sources; do
  not allow uncited API claims.
- Domain locks must be per-domain so unrelated API doc queries can proceed in
  parallel.
- The workflow skill and delegate orientation must steer third-party API lookup
  through the MCP tool once available.

## Phases

### Phase 1: Embedded prompts

Port `pre-router` and `api-doc-manager` into
`agents-plugin-tool/internal/wsprompt/prompts/`.

Keep these contracts:

- `pre-router` is a light one-shot prompt whose output is canonical domain
  slugs, one per line, with no prose.
- `api-doc-manager` owns exactly one `ai-docs/.deps/<domain>/` tree.
- The manager can fetch official docs and write l1/l2/l3 cache files,
  subdomain drill-down docs, `README.md`, `meta.yaml`, and executable scripts.
- The manager creates a staleness check script during bootstrap and runs it at
  the start of each session. When stale, it refreshes or reports the stale cache
  state according to the prompt contract instead of requiring a separate public
  command.
- The manager answers from cached docs and cites the files/sections used.

### Phase 2: MCP API docs tool surface

Add the host-neutral runtime surface for API documentation operations.

Suggested tool contracts:

- `ws/api.ask(prompt: "...", domain_hint: "...")`
- `ws/api.list()`

The tool implementation should replace the Python/Bash orchestration from the
Claude prior art with native runtime behavior:

- locate the repository root and `ai-docs/.deps/`;
- list existing domain directories;
- skip pre-router when `domain_hint` exactly matches an existing domain;
- otherwise invoke `pre-router` through the prompt/runtime path;
- register or resume `api-doc-<domain>` persistent agents;
- hold a per-domain lock around each domain manager call;
- dispatch multiple resolved domains in parallel and concatenate results in
  resolution order.

Multi-domain synthesis should remain an agent responsibility, but the tooling
layer should provide guardrails that keep per-domain calls from being blocked by
formatting, aggregation, or partial-failure ambiguity. The runtime should keep
domain result boundaries, report failed domains distinctly, return failure when
every resolved domain fails, allow partial success when at least one domain
answers, and preserve enough structured metadata for the lead agent to combine
or qualify the returned material.

### Phase 3: Workflow exposure

Update shared Agents workflow guidance after the MCP tool exists.

Required updates:

- Replace the planned/specialized API docs wording in `ws:workflow` with the
  concrete API docs MCP calls.
- Update delegate orientation or worker-facing prompt text so third-party API
  lookup uses the API docs MCP tool instead of direct web browsing.
- Keep `ai-docs/.deps/` hidden from ordinary worker instructions; callers see
  the tool, not the cache layout.

### Phase 4: Verification and compatibility

Add tests and smoke coverage for the migrated surface.

Acceptance checks:

- Prompt resolution includes `pre-router` and `api-doc-manager`.
- MCP tools appear in `tools/list` and `runtime.json`.
- `api.list` works with no cache and with existing cache directories.
- `api.ask` exact-hint path skips pre-router.
- `api.ask` no-hint path invokes pre-router and dispatches one or more domains.
- Multi-domain responses preserve per-domain boundaries and failure metadata so
  callers can continue when only some domains fail.
- The manager prompt instructs each per-domain session to create and run its
  staleness check before answering.
- Per-domain locks prevent concurrent manager calls for the same domain.
- Error reporting preserves non-zero behavior when all domain calls fail.
- Plugin validation and local cache sync succeed.

### Result (5705077, 232fdf6) - 2026-05-04

Implemented the API documentation MCP surface in `agents-plugin-tool` with
embedded `pre-router` and `api-doc-manager` prompts, `api.ask` and `api.list`
MCP tools, per-domain dispatch boundaries, partial-failure reporting, and
per-domain locking. The implementation preserves the existing hidden cache
layout while exposing ordinary callers only to the `ws/api.*` tool contract.

Workflow guidance, delegate orientation, runtime references, and prompt tests
now describe the MCP-based API docs path. Review-cycle fixes tightened domain
slug validation, removed worker-facing cache-path leakage, and added MCP
dispatch/error-formatting and prompt-contract coverage.

Verification:

- `cd agents-plugin-tool && go test ./...`
- `git diff --check`
- correctness, fit, and test reviewer partitions re-reviewed cleanly.

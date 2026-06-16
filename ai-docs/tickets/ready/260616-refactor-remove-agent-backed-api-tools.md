---
title: remove agent-backed api MCP tools from the playbook pivot
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260605-research-ws-native-subagent-pivot: original pivot decision that api.ask spawn machinery should disappear
  260609-refactor-ws-api-ask-corpus-routing: dropped predecessor; replaced by deletion-only M4 scope
  260616-epic-api-namespace-documentation-memory-tooling: deferred follow-up for a pure-tooling api namespace
  260616-refactor-wsflow-product-mode-convergence: follow-up; wsflow convergence waits until the agent-backed api surface is removed
spec:
  - 260505-api-documentation-mcp-tools
  - 260505-api-docs-mcp-surface
  - 260505-api-docs-worker-guidance
  - 260505-claude-api-docs-cli-fallback
  - 260505-workflow-primitive-reference
spec-remove:
  - 260508-api-documentation-async-mcp-tools
  - 260508-api-docs-async-jobs
  - 260505-api-docs-domain-routing
  - 260505-api-docs-synchronous-aggregation
  - 260505-api-docs-manager-sessions
  - 260512-api-doc-agent-backend-selection
  - 260505-api-docs-staleness-fetch-bootstrap
  - 260505-api-docs-conditional-prompts
related-mental-model:
  - api-documentation-cache
  - mcp-runtime
  - workflow-skills
---

# Remove agent-backed api MCP tools from the playbook pivot

## Background

M4 of `260605-epic-ws-playbook-factory-pivot` originally aimed to redesign
`api.ask` into a corpus-routed dependency-documentation playbook. The settled
scope is narrower: the playbook pivot should remove the agent-backed `api.*`
surface from the core workflow product, not design a new documentation-memory
system inside this milestone.

The current `api.ask` family owns model delegation: a pre-router, per-domain
manager sessions, synchronous aggregation, and recoverable async jobs. That
violates the post-pivot boundary that workflow agents should delegate reasoning
through host-native subagents or explicit mercenary calls, while MCP tools own
deterministic project and workflow data operations.

The replacement documentation/memory direction is tracked outside this epic by
`260616-epic-api-namespace-documentation-memory-tooling`.

## Decisions

- **Remove, do not redesign inside M4.** M4 removes `api.ask`,
  `api.ask_async`, `api.status`, `api.result`, and `api.cancel` from the full ws
  tool surface. It does not introduce a playbook-driven replacement.
- **No compatibility shim by default.** Keeping `api.ask` as a diagnostic shim
  would preserve a stale public tool name and invite continued use. Remove the
  tool unless implementation finds a release-blocking caller that needs a short
  deprecation window.
- **`api.list` is not a model surface.** If retained, it must be only a
  read-only local cache/domain discovery tool. If the current cache convention
  no longer has a supported caller after removing `api.ask`, removal is allowed.
- **Workflow guidance falls back to native exploration.** Until the future api
  namespace exists, workflow manual and delegate guidance should tell agents to
  use scoped host-native exploration for external dependency/API documentation,
  with official-source citation and explicit staleness caveats.
- **Future `api.*` is pure tooling.** The long-term `api` MCP namespace is
  reserved for deterministic document, corpus, hierarchical memory, and agent
  playbook-manual tooling. It must not own agent delegation or model routing.

## Constraints

- Keep this ticket scoped to deletion and guidance cleanup. Do not design the
  future api documentation/memory system here.
- Update runtime capabilities, manifests, tests, workflow manual text, delegate
  orientation text, specs, and mental models so no shipped guidance points at a
  removed `api.ask` surface.
- Coordinate with `260616-refactor-wsflow-product-mode-convergence`: wsflow
  convergence should run after this deletion so product-mode rendering does not
  need to preserve dead api guidance.

## Phases

### Phase 1: remove agent-backed api tools and stale guidance

Remove `api.ask`, `api.ask_async`, `api.status`, `api.result`, and `api.cancel`
from MCP schemas, dispatch, runtime metadata, CLI/runtime contracts, and tests.
Remove the pre-router, per-domain manager-session orchestration, API async job
state, and api-doc prompt call sites when no other supported surface uses them.
Retain or remove `api.list` based on whether it still has a deterministic
read-only cache-discovery role after the ask surface is gone.

Update shipped workflow guidance to route external dependency/API documentation
questions through scoped host-native exploration rather than `ws/api.ask`.
Update specs and mental models to mark the removed tool family as retired and
to point future api namespace work at
`260616-epic-api-namespace-documentation-memory-tooling`.

Verification: full ws and wsflow package tests pass; runtime capabilities and
`tools/list` omit removed api tools; no shipped playbook, skill, delegate
orientation, spec, or mental model instructs agents to call `ws/api.ask` or its
async job tools; any retained `api.list` test proves it performs only
deterministic read-only discovery.

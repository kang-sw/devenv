# Brief: 260616-refactor-remove-agent-backed-api-tools

## Intent

Remove the agent-backed API documentation question surface from the ws
playbook-factory pivot. The change retires MCP-owned model routing for API
documentation while preserving deterministic workflow tooling and leaving the
future pure-tooling `api.*` namespace to a separate epic.

## Scope Boundary

Implement only Phase 1 of `260616-refactor-remove-agent-backed-api-tools`.
Remove `api.ask`, `api.ask_async`, `api.status`, `api.result`, and
`api.cancel`, plus stale shipped guidance that tells agents to call them.
Retain `api.list` only if it remains deterministic read-only cache/domain
discovery. Do not design or implement the future documentation/memory system.

## Caller-Visible Contract

- Full ws MCP `tools/list`, direct `tools/call`, runtime capabilities, and
  package runtime metadata no longer expose `api.ask`, `api.ask_async`,
  `api.status`, `api.result`, or `api.cancel`.
- wsflow/no-agent mode continues to omit those removed tools and must not
  advertise stale runtime requirements.
- Any retained `api.list` returns only local cache-domain discovery and performs
  no model routing, manager dispatch, fetching, staleness check, or answer
  synthesis.
- Shipped workflow guidance no longer instructs workers to call `ws/api.ask` or
  async API job tools. External dependency/API documentation lookup falls back
  to scoped native exploration with official-source citation and staleness
  caveats until future `api.*` tooling exists.

## Contract Instructions

- Remove MCP schemas, dispatch cases, role/product gates, async job handlers,
  runtime metadata entries, and tests for removed tools.
- Remove or orphan-proof code paths for pre-router, per-domain manager sessions,
  API async job state, and api-doc prompt rendering when no retained surface uses
  them.
- Update `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md` and
  `agents-plugin/rsrc/delegate-orientation.md` to remove `ws/api.ask` guidance.
- Keep wsflow generated mirrors aligned when rsrc content changes.
- Do not introduce a compatibility shim unless implementation discovers a
  release-blocking caller. If such a caller exists, stop and report before
  changing the ticket contract.

## Integration Test Instructions

Run at minimum:

- `cd agents-plugin-tool && go test -count=1 ./...`
- `python3 -m unittest discover agents-plugin/tests`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `git diff --check`
- `ws/spec_index.verify`

Add or update targeted tests proving removed tools are absent from `tools/list`,
direct calls fail as unknown or unavailable, `runtime.capabilities` omits them,
and `api.list` remains deterministic if retained.

## Implementation Strategy Decisions

- Deletion is preferred over a diagnostic shim.
- `api.list` may remain as read-only local cache discovery; it must not imply
  an answer-producing api namespace.
- Future `api.*` documentation/corpus/hierarchical-memory tooling is out of
  scope and tracked by `260616-epic-api-namespace-documentation-memory-tooling`.
- Historical changelog entries can remain historical; active shipped guidance,
  specs, mental models, runtime contracts, and tests must be updated.

## Rejected Alternatives

- Rebuilding `api.ask` as a corpus-routed native-subagent playbook inside M4:
  rejected because it mixes spawn/runtime deletion with a new documentation
  product design.
- Keeping `api.ask` as a compatibility shim by default: rejected because the
  stale public name invites continued use.
- Moving documentation reasoning into a new MCP-owned model router: rejected;
  reasoning belongs in native subagents, lead context, or explicit mercenary
  delegation.

## Approach

- Remove runtime tool registration and dispatch for the agent-backed tools.
- Delete or detach unused API docs manager/async implementation files and tests.
- Update runtime JSON contracts and wsflow tests.
- Rewrite shipped rsrc guidance and regenerate rsrc manifests/mirrors.
- Update docs and mental models during closeout to remove stale live-contract
  language and clear planned markers.

## Constraints

- Preserve root/session-key behavior for retained tools.
- Preserve wsflow product-mode gates.
- Keep docs in English.
- Do not edit `ai-docs/.deps/` cache content.

## Out of scope

- New dependency-documentation cache/index/staleness design.
- Hierarchical memory implementation.
- wsflow product-mode convergence beyond keeping this deletion compatible.
- Dashboard changes except compile/test fallout from runtime tool inventory.

## Details

Likely code surfaces:

- `agents-plugin-tool/internal/mcp/server.go`
- `agents-plugin-tool/internal/mcp/api_docs.go`
- `agents-plugin-tool/internal/mcp/api_async.go`
- `agents-plugin-tool/internal/mcp/*api*_test.go`
- `agents-plugin-tool/cmd/ws-mcp/main_test.go`
- `agents-plugin/runtime.json`
- `agents-plugin-wsflow/runtime.json`
- `agents-plugin/rsrc/`
- `agents-plugin-wsflow/rsrc/`
- `agents-plugin-wsflow/tests/`

## Verification Contract

The implementation is acceptable only when the removed tools are gone from all
live runtime and shipped guidance surfaces, retained `api.list` behavior remains
deterministic if kept, package tests pass, and closeout docs remove planned
markers for implemented removals.

## References

- [Must] `ai-docs/tickets/ready/260616-refactor-remove-agent-backed-api-tools.md` - selected scope and spec trace.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` - migration anchor and spawn-removal boundary.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP registry, runtime metadata, product-mode gates.
- [Must] `ai-docs/mental-model/api-documentation-cache.md` - current api-doc manager and async contracts to remove.
- [Must] `ai-docs/mental-model/workflow-skills.md` - workflow guidance and rsrc coupling.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - rsrc manifest and wsflow mirror rules.

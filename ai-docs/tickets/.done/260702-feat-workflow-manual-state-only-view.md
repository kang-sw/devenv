---
title: add a session-state-only MCP tool to avoid re-dumping the full manual
sage-review: completed
completed: 2026-07-02
---

# add a session-state-only MCP tool to avoid re-dumping the full manual

## Context

Found during a v0.31.1 dogfooding pass. `workflow_manual` re-dumps the entire
~150-line manual on every call. In one session it was called 3 times when only
the session key plus the Session State section (todos/agenda) was actually
needed. This is especially costly right after compaction or during a
`lead-revive`, precisely when context budget is tightest — the caller wants a
cheap "what's my key and current state" check, not a full manual reload.

## Suggestion

Do not overload `workflow_manual` with a `state_only` flag — the name
`workflow_manual` should mean the manual, full stop, not a mode-dependent
mix of manual-or-state. Instead, add a new, separate MCP tool (naming open to
bikeshedding, e.g. `session_state` or `workflow_state`) whose sole
responsibility is returning the Session State section (todos/agenda) for the
caller's session key, with no manual reference text at all.
`workflow_manual` keeps its current always-full-dump behavior unchanged.

For a session key that is invalid, expired, or otherwise not in a normal
continue state, the new tool must reuse `workflow_manual`'s existing
key-validation/error behavior rather than defining a separate error path —
this is a thin read of the same session state `workflow_manual` already
resolves, not a new state machine.

The new tool is lead-only, same as `workflow_manual` (`isLeadOnlyTool`),
not open to delegate/leaf scopes even though the underlying `todo.*`/
`agenda.*` data is itself scope-open. Rationale: this tool is a cheaper
view of the same lead-bootstrap/recovery surface `workflow_manual` serves
(compaction/revive context checks), not a general todo/agenda accessor —
keep it in the same tool family and gating as its sibling rather than
introducing a second, differently-scoped way to read session state.

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md`. Caller-visible change: new MCP tool
(name open to bikeshedding, e.g. `session_state`) returning only the Session
State (todos/agenda) for the caller's session key, without the full manual
body; `workflow_manual` itself is unchanged. Contract-first spec: no.

## Phases

### Phase 1: Add session-state-only MCP tool

- Add a new MCP tool (name TBD, e.g. `session_state`) that takes the
  caller's `session_key` and returns only the Session State portion
  (todos/agenda) for that session — no manual reference/primitives text.
- Reuse `workflow_manual`'s existing session-key resolution and
  error/validation behavior for invalid, expired, or unknown keys; do not
  invent a separate error path for this tool.
- `workflow_manual` itself stays unchanged — same always-full-dump
  behavior, same schema.
- Wire the new tool into the MCP tool registration/list alongside the
  existing `workflow_manual` tool.
- At implementation closeout, update `ai-docs/spec/mcp-tools.md` per the
  `## Spec Impact` note above (`Contract-first spec: no`).

Verification:
- For a given `session_key`, the new tool's output matches exactly the
  "Session State" section content that `workflow_manual` renders for that
  same session at the same point in time.
- The new tool's response is substantially shorter than a full
  `workflow_manual` render (no manual body/primitives reference text).
- An empty session (no todos, no agenda) returns an empty state payload,
  not an error.
- An invalid/expired/unknown `session_key` produces the same
  error/validation behavior as `workflow_manual` for that key, not a
  distinct error shape.

### Result

Implemented as a new `workflow_state` MCP tool (chosen name: mirrors
`workflow_manual`'s single-underscore naming style so the two sit together as
an obvious family in tool listings).

**Files changed:**
- `agents-plugin-tool/internal/mcp/workflow_manual.go`: added
  `handleWorkflowState`, which reuses `workflow_manual`'s exact key-validation
  and fail-loud notice text, and renders only `renderSessionState(rec)` (no
  manual body) for a resolved key. The fresh-bootstrap sentinel has no FRESH
  mode here — it falls through to the same fail-loud path as any other
  unresolvable key, since the sentinel is never a stored record.
- `agents-plugin-tool/internal/mcp/server.go`: registered `workflow_state` in
  `isLeadOnlyTool` (same gating as `workflow_manual`), added the
  `case "workflow_state":` dispatch, and added the tool schema (`session_key`
  required, no `root` parameter since there is no fresh-mode mint path).
- `agents-plugin-tool/internal/mcp/session_state_test.go`: added
  `TestWorkflowStateReturnsSessionStateOnly` (exact-match against
  `workflow_manual`'s Session State suffix, shorter-response assertion, no
  manual-body leakage), `TestWorkflowStateEmptySessionReturnsEmptyPayloadNotError`,
  `TestWorkflowStateKeylessRejected`,
  `TestWorkflowStateDelegateKeyBlockedSameAsWorkflowManual` (asserts identical
  -32601 lead-only rejection shape as `workflow_manual`),
  `TestWorkflowStateUnknownKeySameFailLoudAsWorkflowManual` (asserts byte-exact
  fail-loud notice match against `workflow_manual`'s for the same key, and no
  key file minted), `TestWorkflowStateFreshSentinelIsFailLoudNotFresh`, and
  `TestWorkflowStateToolSchema`.
- `agents-plugin/runtime.json`, `agents-plugin-wsflow/runtime.json`: added the
  `workflow_state` tool entry to the `tools` contract map (same version range
  as sibling entries at the current pending version); required because
  `cmd/ws-mcp`'s golden contract-surface tests (`TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`,
  `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`) assert the live
  tool set equals these manifests exactly. The version-bump script only
  rewrites version ranges on existing keys, so the new key was added by hand
  per its own doctring caveat.
- `ai-docs/spec/mcp-tools.md`: added `### Session-State-Only View
  {#260702-workflow-state-tool}` under the existing Session State Tools
  section, documenting the new tool's behavior, gating, and reused
  fail-loud/keyless error paths, per the ticket's Spec Impact note
  (contract-first spec: no).

**Verification evidence:**
- `go build ./...` — clean.
- `go test ./internal/mcp/... -run WorkflowState -v` — all 7 new tests pass.
- `go test ./... -count=1` from `agents-plugin-tool/` — all packages pass
  (`cmd/ws-mcp`, `internal/mcp`, and all other internal packages), including
  the golden contract-surface tests after the `runtime.json` updates.
- `mcp__plugin_ws_ws__spec_index_verify` — spec index ok after the doc edit.

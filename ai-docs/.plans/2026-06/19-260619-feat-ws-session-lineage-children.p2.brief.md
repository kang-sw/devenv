# Brief: 260619-feat-ws-session-lineage-children (Phase 2)

## Intent

Add an optional `parent_session_key` argument to `ws.ferrule` so a lead
coordinating several repository roots in one conversation (e.g. multiple git
worktrees) can record each additional control key's lineage back to its primary
control key. This is the scenario-2 ("work-in") half of session lineage,
complementing Phase 1's scenario-1 ("dispatch-into") render-path lineage. It is
metadata only and grants no new capability — `ws.ferrule` already mints a full
control key per root.

Phase 1 already widened `sessionStore.mint(root, scope, parent)` and made
`handleLeadLogin` pass `""`. Phase 2 only wires a real value into that existing
parameter from a new optional argument, plus a guard against linking to a
non-existent parent.

## Scope Boundary

In scope (Phase 2 only):
- New optional `parent_session_key` property on the `ws.ferrule` (`bootstrapToolName`)
  input schema.
- `handleLeadLogin` reads it; validates existence; threads it into the existing
  `mint(canonical, scope, parent)` call.

Out of scope: the `ws.session.children` enumeration tool (Phase 3); any change to
the render-path lineage (Phase 1, done); scope-widening or capability changes;
worktree creation/teardown.

## Caller-Visible Contract

`ws.ferrule` gains one optional argument `parent_session_key`:
- Absent or empty → the minted control key is parent-less (unchanged from today;
  the primary bootstrap key has no parent).
- Present and resolvable to an existing session key → the minted key records it
  as `parent` (pure lineage metadata; child scope is still
  `parseCapabilityScope(capability)`, unchanged).
- Present but unknown (does not resolve in the session store) → the call is
  rejected with a clear error and **no key is minted** (avoids creating a
  mislinked key). This is the spec's "invalid/unknown parent is handled without
  minting a mislinked key."

Full contract: `ai-docs/spec/mcp-tools.md` `{#260619-session-key-lineage-children}`
(the `ws.ferrule accepts an optional parent_session_key` bullet).

## Contract Instructions

All under `agents-plugin-tool/internal/mcp/`.

- `server.go`, `ws.ferrule` input schema (currently ~lines 1820-1828, the
  `bootstrapToolName` tool entry): add a `parent_session_key` property alongside
  `root`/`capability`/`format`. Keep the description terse and consistent with
  the deliberately obscure `ws.ferrule` surface (the tool description is
  "Reserved workflow primitive. See ws:workflow-manual before use."). Do not add
  it to `required`. Suggested description: `"Optional parent session key to record
  coordination lineage. See ws:workflow-manual."`
- `server.go`, `handleLeadLogin` (~lines 1047-1056): after computing `scope` and
  before `mint`, read `parent_session_key`:
  - `parentKey, _ := arguments["parent_session_key"].(string)`; `parentKey = strings.TrimSpace(parentKey)`.
  - If `parentKey != ""`, validate via `s.sessions.lookup(parentKey)`. If not
    found, `return toolTextResponse(id, "", fmt.Errorf("session bootstrap: parent_session_key %q is not a known session key", parentKey))` — do not mint.
  - Pass `parentKey` (which is `""` when absent) into
    `s.sessions.mint(canonical, scope, parentKey)`.
- Do not enforce that the parent is control/lead scope. The verification contract
  only addresses unknown parents; parent is pure metadata and never widens scope,
  so existence validation is the contract. (Recording a parent must never change
  the minted key's own scope — that already holds: scope comes from
  `parseCapabilityScope`, untouched.)

Reuse the existing `lookup`/`mint` machinery. Introduce no new store, index, or
lineage mechanism.

## Integration Test Instructions

Extend the existing MCP server tests (the file that already exercises
`handleLeadLogin` / `ws.ferrule`; if none, add to `session_auth_test.go` or
`server_test.go` consistent with where ferrule is currently tested):

- ferrule with a valid `parent_session_key` (e.g. mint a primary lead key first,
  then ferrule a second root passing the first key) records that parent on the
  second key — assert via `lookup`.
- ferrule without `parent_session_key` mints a parent-less key (empty `parent`).
- ferrule with an unknown `parent_session_key` returns an error and mints no key
  (assert no new key file / error response; the store key count does not grow).
- Empty-string `parent_session_key` behaves as absent (parent-less, no error).

If the ferrule handler is reachable in wsflow no-agent mode through the same code
path, no separate test is required — the behavior is identical because the parent
plumbing is mode-independent; note this in the result rather than adding a
redundant test.

Pass criteria: `cd agents-plugin-tool && go test ./internal/mcp/...` green;
`go build ./...` clean.

## Implementation Strategy Decisions

- Existence-validate the parent, fail loud on unknown — faithful to "without
  minting a mislinked key" and surfaces lead mistakes (stale/cleared key)
  immediately.
- No scope enforcement on the parent: parent is metadata only and cannot widen
  capability; the verification contract scopes only to unknown parents.
- Pure value-plumbing into Phase 1's already-widened `mint` parameter; no
  signature changes this phase.

## Rejected Alternatives

- Silently minting parent-less on an unknown parent: rejected — hides a lead
  error and produces a key the lead believes is linked but is not.
- Enforcing parent scope == control/lead: rejected as out-of-contract
  over-engineering for Phase 2; parent is metadata and never widens scope.

## Constraints

- Single MCP package; no new exported Go symbols.
- Behavior-preserving when `parent_session_key` is absent (identical to today).
- `ws.ferrule` remains lead-only via the existing keyed gate; this phase does not
  touch that guard.

## Out of scope

`ws.session.children` (Phase 3); mental-model rewrites beyond Phase 2 contact;
worktree lifecycle.

## Verification Contract

`cd agents-plugin-tool && go test ./internal/mcp/...` green; the four test
additions above present and passing; `go build ./...` clean.

## References
<!-- [Must] read before starting. [Maybe] consult if uncertain. -->
- `ai-docs/spec/mcp-tools.md` `{#260619-session-key-lineage-children}` - [Must] the `parent_session_key` contract bullet
- `agents-plugin-tool/internal/mcp/server.go` `handleLeadLogin` + `ws.ferrule` schema - [Must] the edit sites
- `agents-plugin-tool/internal/mcp/session_auth.go` `mint`/`lookup` - [Must] the Phase-1-widened machinery being reused
- `ai-docs/tickets/ready/260619-feat-ws-session-lineage-children.md` Phase 2 - [Must] the phase plan + verification
- `ai-docs/mental-model/mcp-runtime.md` - [Maybe] session-key resolution invariants

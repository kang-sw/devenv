# Brief: 260619-feat-ws-session-lineage-children (Phase 3)

## Intent

Add a read-only `session.children` MCP tool that lets a lead re-discover the
session keys it minted. It scans the flat `keys/` store, returns the subtree of
keys whose `parent` chain roots at the presented key (Phases 1-2 record those
`parent` edges), labels each child by capability scope, and includes the child
key string so the lead can re-thread it after context loss. This is the
caller-facing read surface that completes the session-lineage feature; once it
lands the spec's 🚧 marker comes off.

## Scope Boundary

In scope (Phase 3 only):
- A new store enumeration method on `sessionStore` that returns the descendant
  subtree of a given key (BFS over `parent` edges, depth-bounded, liveness
  computed).
- A new `session.children` tool: schema entry in `tools()` + dispatch case +
  handler. Text-default + `format:"json"` output.

Out of scope: any change to mint/lineage recording (Phases 1-2, done); worktree
creation/teardown; key eviction/pruning (the tool only *reports* liveness, it
never deletes); mental-model rewrites beyond Phase 3 contact (the ticket plans a
`mcp-runtime`/`named-agent-runtime` touch — make it a minimal, accurate update,
see Doc Touch below).

## Caller-Visible Contract

`session.children(session_key, depth?, format?, include_dead?)`:
- Returns, read-only, the subtree of keys whose `parent` chain roots at
  `session_key`. The queried key itself is NOT included — only its descendants.
- Each entry is labeled by its stored capability scope: `control` (a
  coordination key, scope `lead`), `delegate`, or `leaf`. Each entry includes the
  child key string (a credential) so the lead can re-thread it.
- A caller only ever sees the subtree under the key it presents. An unknown or
  childless key yields an empty result (not an error).
- `depth`: integer, default `1` (immediate children only). A higher value returns
  that many levels. `0` returns the full subtree (unbounded).
- `include_dead`: boolean, default `false`. Liveness = the child's bound `root`
  path still exists on disk. Dead keys are filtered by default; `include_dead:
  true` returns them flagged `live: no`.
- Output: compact labeled text by default (per
  `#260512-mcp-llm-readable-output-defaults`), carrying the re-threadable child
  key strings; `format: "json"` is the structured escape hatch.

Tool name is `session.children` (the `session.*` family, NOT `ws.session.`). This
is deliberate: `roleAllowsTool` already blocks the `session.` prefix for
delegate and leaf scopes, so registering in this family restricts the tool to
lead-scoped keys with **zero gate change**. (Sibling stubs
`session.set_default_root`/`session.get_default_root` are retired and not in
`tools()`; `session.children` is the first live `session.*` tool.)

Full contract: `ai-docs/spec/mcp-tools.md` `{#260619-session-key-lineage-children}`.

## Contract Instructions

All under `agents-plugin-tool/internal/mcp/`.

### 1. Store enumeration (`session_auth.go`)

Add a method that walks the flat keys dir and returns the descendant subtree:

```go
// sessionChild is one enumerated descendant of a queried key.
type sessionChild struct {
    key    string
    root   string
    scope  toolRole
    parent string
    depth  int  // distance from the queried key; 1 = immediate child
    live   bool // root path still exists
}

// children returns the descendants of parentKey from the flat keys store,
// ordered deterministically (depth, then key). maxDepth bounds the walk:
// maxDepth >= 1 returns that many levels; maxDepth <= 0 returns the full
// subtree. The queried key itself is not included. A cycle guard (visited set)
// prevents infinite loops on a malformed parent edge.
func (s *sessionStore) children(parentKey string, maxDepth int) ([]sessionChild, error)
```

Implementation notes:
- Read `keysDir()`; list `*.json` entries; for each, derive the key (filename
  without `.json`) and `readRecord(dir, key)`. Skip unreadable/malformed files
  (consistent with `lookup` returning not-found on bad records — do not error the
  whole call on one bad file).
- Build adjacency `map[parent][]key`. BFS from `parentKey` at depth 0 (root, not
  emitted); enqueue each child at `depth+1`; stop expanding a node once
  `maxDepth >= 1 && depth == maxDepth`.
- `live` = the record's `Root` path exists (`os.Stat`, treat any stat error as
  dead). Compute it for every emitted child; the handler decides filtering.
- Use a `visited` set keyed by session key to guard against cycles.
- Hold `s.mu` for the directory read to match the store's mutex discipline
  (other methods lock for RMW; a read-only walk taking the lock is consistent and
  cheap given the small store). Do not call `lookup`/`readRecord` variants that
  re-lock and deadlock — read records inline under the single lock, or structure
  so the lock is not held recursively. (`readRecord` itself does NOT lock, so
  calling it under `s.mu` is safe — verify before relying on it.)

### 2. Tool registration + dispatch (`server.go`)

- Add a `session.children` entry to the `tools()` schema list (near the other
  read tools). `inputSchema` properties: `session_key` (string, required),
  `depth` (integer, optional — describe default 1, 0 = full subtree),
  `include_dead` (boolean, optional), `format` (string, optional, `"json"`).
  `required: ["session_key"]`. Use the existing `stringProperty` /
  `integerProperty` / `booleanProperty` helpers (confirm a bool helper exists;
  if not, inline the `{"type":"boolean","description":...}` map as other schemas
  do).
- Add a `case "session.children":` to the `tools/call` dispatch switch, calling a
  new `handleSessionChildren(req.ID, params.Arguments)`.
- The keyed capability gate already runs before dispatch: a non-lead key calling
  `session.children` is rejected by the pre-existing `session.` prefix block in
  `roleAllowsTool` — no gate edit needed.

### 3. Handler (`server.go`)

`handleSessionChildren(id, arguments)`:
- `sessionKey, _ := arguments["session_key"].(string)`; trim; if empty →
  `toolTextResponse(id, "", fmt.Errorf("session.children: session_key is required"))`.
- Parse `depth` WITHOUT `intFromArgument` (that helper coerces `<= 0` to its
  fallback, which would break the `depth: 0 = full` contract). Default 1; read an
  explicit value when present:
  ```go
  depth := 1
  if raw, ok := arguments["depth"]; ok {
      if f, ok := raw.(float64); ok { depth = int(f) }
  }
  ```
  Pass `depth` straight to `store.children` (which treats `<= 0` as unbounded).
- `includeDead, _ := arguments["include_dead"].(bool)`.
- Call `s.sessions.children(sessionKey, depth)`; on error return it.
- Filter: drop entries with `!live` unless `includeDead`.
- Output:
  - JSON (`wantsJSON(arguments)`): `toolJSONResponse` with a stable array of
    objects `{key, scope, parent, depth, live, root}` (scope as the label string
    below). Include a small wrapper if it reads better (e.g.
    `{"session_key": ..., "depth": ..., "children": [...]}`), but keep field names
    stable.
  - Text (default): compact labeled lines, indented by depth, each carrying the
    re-threadable key, scope label, and (when `include_dead`) a live flag. A
    header line is fine. Empty result → a clear "no children" line, not blank.
- Scope label mapping: `roleLead → "control"`, `roleDelegate → "delegate"`,
  `roleLeaf → "leaf"` (any other → the raw string).

## Integration Test Instructions

Add tests (in `session_auth_test.go` for the store method, and alongside the
existing ferrule/server tests for the tool handler — match where current
`session.*`/ferrule behavior is tested):

Store-level (`children`):
- A control key with two render-minted delegate leaves and one ferruled
  coordination control child returns all three at depth 1, labeled by scope.
- Depth bounding: a 2-level tree (lead → delegate → leaf via a parent chain)
  returns only immediate children at `depth: 1`, and the full tree at `depth: 0`.
- A leaf key (no descendants) returns an empty slice.
- Isolation: a sibling subtree under a *different* parent is never returned for
  the queried key.
- Liveness: a child whose `root` points at a non-existent path is marked
  `live: false`.

Handler-level (`session.children`):
- Dead children are filtered by default and returned (flagged `live: no`) when
  `include_dead: true`.
- Text output carries the child key strings (assert a known child key substring
  is present).
- `format: "json"` returns the stable fields and parses.
- Missing `session_key` errors.

Pass criteria: `cd agents-plugin-tool && go test ./internal/mcp/...` green;
`go build ./...` clean.

## Doc Touch (minimal)

The ticket plans a `mcp-runtime` / `named-agent-runtime` mental-model touch. Keep
it minimal and accurate: add a sentence to whichever doc describes the
session-key store/lifecycle, noting that keys form a `parent` lineage and
`session.children` enumerates a key's subtree (read-only, lead-scoped). Do NOT
rewrite the docs. If a mental-model edit is needed, follow the lead's authoring
constraints — if uncertain whether a given line is in scope, leave it to the lead
and note it in your result rather than expanding the edit.

## Implementation Strategy Decisions

- `session.children` (not `ws.session.children`): reuses the pre-provisioned
  `session.` capability-gate block; zero gate-semantics change. (Lead-confirmed.)
- `depth` parsed directly, not via `intFromArgument`, because that helper maps
  `<= 0` to the fallback and would silently break `depth: 0 = full`.
- Liveness computed in the store walk (one `os.Stat` per child); filtering
  decided in the handler so the store method stays policy-free.
- Unknown/childless key → empty result, not an error: enumeration is read-only
  discovery; the lead presents its own key and an empty subtree is a valid answer.

## Rejected Alternatives

- Naming it `ws.session.children` and extending `roleAllowsTool` to match
  `ws.session.`: rejected — changes gate semantics for no benefit when the
  `session.*` family already exists and is pre-gated.
- A persisted child index: rejected — the flat store is small and a directory
  walk on demand is simpler and avoids an index to keep consistent.
- Including the queried key in the result: rejected — the contract returns
  descendants; the caller already holds the queried key.

## Constraints

- Single MCP package; no new exported Go symbols.
- Read-only: the tool never writes or deletes a key file (liveness is reported,
  not acted on).
- Deterministic output ordering (depth, then key) so tests and LLM reads are
  stable.
- Behavior-preserving for all existing tools.

## Verification Contract

`cd agents-plugin-tool && go test ./internal/mcp/...` green; the store-level and
handler-level tests above present and passing; `go build ./...` clean.

## References
<!-- [Must] read before starting. [Maybe] consult if uncertain. -->
- `ai-docs/spec/mcp-tools.md` `{#260619-session-key-lineage-children}` - [Must] the `session.children` contract bullet + depth/liveness/format rules
- `agents-plugin-tool/internal/mcp/session_auth.go` - [Must] the store (`keysDir`, `readRecord`, `mint`, mutex discipline) the walk reuses
- `agents-plugin-tool/internal/mcp/server.go` `roleAllowsTool` (~2479), the `tools/call` switch, `config.show` (text/json pattern), `tools()` schema list, `intFromArgument` (~the zero-coercion gotcha) - [Must] dispatch/gate/output patterns
- `ai-docs/tickets/ready/260619-feat-ws-session-lineage-children.md` Phase 3 - [Must] the phase plan + settled contract decisions + verification
- `ai-docs/mental-model/mcp-runtime.md`, `ai-docs/mental-model/named-agent-runtime.md` - [Maybe] the minimal doc touch target

# Plan: 260703-chore-sage-review-builtin-default-on — Phase 1: Add the builtin default

## Relevant Ticket Contract
- Add `wsconfig.ItemSageReview: "auto"` to `builtinConfigDefaults()` in
  `agents-plugin-tool/internal/mcp/server.go`. `"auto"` is the raw config value
  that resolves to the `required` ticket posture — not the posture string
  `"required"` itself.
- Update the `ItemSageReview` doc comment in
  `agents-plugin-tool/internal/wsconfig/scope.go:34-38` to state the new
  builtin default (`auto`), replacing "Builtin default: off (absent =
  disabled)".
- **Required, not optional**: swap `nil` → `builtinConfigDefaults()` at the two
  `tickets.move` (`server.go:1063`) and `tickets.create` (`server.go:1086`)
  `wsconfig.NewResolver(...)` call sites. Without this swap the new builtin
  default is structurally unreachable (`NewResolver` substitutes a `nil`
  `builtinDefaults` map with an empty map).
- Verification boundary: with no project-scope `sage_review` override, resolved
  posture must become `required` (not `skipped`) via `tickets.create` /
  `tickets.move`. Existing tests asserting `skipped`-by-default must be
  updated; a test must assert an explicit project-scope override still wins
  over the new builtin default.
- No change to value vocabulary, setter surface, or posture-mapping logic.

## Out of Scope
- `260626-bug-sage-review-config-setter-missing` (no `config.tuning`/
  `ws:lead-tune` writer for `sage_review` exists) — separate, pre-existing gap,
  not touched here.
- The `config.prompt.set`/`unset` `nil`-passing call sites at `server.go:651`
  and `702` — these only use `resolver.Set`/`Unset` for `prompt.*` keys, never
  read `ItemSageReview` via `Get`, so they are unaffected and unchanged.
- The apparent `off|auto|ask` vs. `recommended|required|skipped` vocabulary
  "mismatch" — ticket confirms these are two different vocabularies (raw
  config input vs. derived frontmatter posture) and need no fix.
- Any spec edit: `ai-docs/spec/mcp-tools.md` already documents the
  resolved-value-to-posture mapping; it does not state the shipped builtin
  default value, so this is not a contract change.

## Codebase Findings
- `agents-plugin-tool/internal/mcp/server.go#L318-L323` — `builtinConfigDefaults()`
  currently returns only `ItemWorkflowPreferSubagent: "off"` and
  `ItemWorkflowPreferMercenary: "hide"`. Add the third entry here.
- `agents-plugin-tool/internal/wsconfig/scope.go#L34-L38` — `ItemSageReview`
  constant with the doc comment to update. Line 37 is the exact line
  containing the stale "Builtin default: off (absent = disabled)" text.
- `agents-plugin-tool/internal/wsconfig/scope.go#L76` — `RegisterDefaultScope(ItemSageReview, ScopeProject)`
  confirms explicit project scope always outranks builtin; no change needed
  here, context only.
- `agents-plugin-tool/internal/mcp/server.go#L1051-L1073` — `tickets.move` case;
  `wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)` at line
  1063 is the first `nil` to replace with `builtinConfigDefaults()`.
- `agents-plugin-tool/internal/mcp/server.go#L1074-L1094` — `tickets.create`
  case; identical `nil` at line 1086, second call site to fix.
- `agents-plugin-tool/internal/mcp/server.go#L523,553,596,1184,3925` — sibling
  `NewResolver` call sites already pass `builtinConfigDefaults()` explicitly;
  confirms the correct call pattern/precedent to copy exactly (no options
  change, just swap the second argument).
- `agents-plugin-tool/internal/wsconfig/resolver.go#L64-L67` — `NewResolver`
  substitutes a `nil` `builtinDefaults` argument with an empty map, which is
  why the two call sites above make the builtin default unreachable today.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1787-L1825` —
  `TestServeStdioTicketsCreateUsesResolvedSageReviewConfig` is the only
  existing test exercising `tickets.create` through the MCP tool-call path. It
  explicitly sets `wsconfig.ItemSageReview` to `"ask"` at project scope before
  calling `tickets.create`, and asserts `sage-review: recommended`. Since this
  test sets an explicit project-scope override, it is unaffected by the
  builtin-default flip and needs no change — but it is good precedent/coverage
  to extend or copy for a new "no override → required" test and a
  "project-scope override still wins" test (ticket explicitly requires the
  latter).
- No other test in the repo calls `tickets.create`/`tickets.move` through the
  MCP layer, and no test in `internal/wsconfig/*_test.go` asserts default
  resolution for `ItemSageReview` — confirmed via grep across
  `internal/mcp/*_test.go`, `internal/wsdoc/*_test.go`, and
  `internal/wsconfig/*_test.go`. The `sage-review: skipped` occurrences in
  `internal/wsdoc/tickets_mutate_test.go` (lines 342/364/384/404/424/446/468)
  construct ticket frontmatter directly and do not go through
  `wsconfig.NewResolver`/`builtinConfigDefaults` — out of scope, no change
  needed there.

## Implementation Plan
1. In `agents-plugin-tool/internal/mcp/server.go`, add
   `wsconfig.ItemSageReview: "auto"` as a third entry in the map literal
   returned by `builtinConfigDefaults()` (around line 319-323).
2. In `agents-plugin-tool/internal/mcp/server.go:1063`, change
   `wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)` to
   `wsconfig.NewResolver(wsconfig.Options{}, builtinConfigDefaults(), adapter, adapter)`
   (matches the pattern already used at lines 523/553/596/1184).
3. In `agents-plugin-tool/internal/mcp/server.go:1086`, apply the identical
   `nil` → `builtinConfigDefaults()` swap for the `tickets.create` resolver.
4. In `agents-plugin-tool/internal/wsconfig/scope.go:34-38`, update the
   `ItemSageReview` doc comment's last line from
   `// Builtin default: off (absent = disabled).` to state the new builtin
   default is `auto` (e.g. `// Builtin default: auto (gate runs unless a
   project/session/global override disables it).`) — keep the rest of the
   comment (value semantics for auto/ask/off) unchanged.
5. In `agents-plugin-tool/internal/mcp/session_state_test.go`, add test
   coverage near `TestServeStdioTicketsCreateUsesResolvedSageReviewConfig`
   (~line 1787):
   - A new test that creates a ticket via `tickets.create` with no explicit
     `sage_review` config set, and asserts the created ticket frontmatter
     contains `sage-review: required` (builtin default now reachable).
   - Confirm/extend an analogous case for `tickets.move` if an existing
     `tickets.move`-through-MCP test exists (none found in survey — adding one
     is in scope per the ticket's verification boundary, mirroring the
     `tickets.create` test structure with `initGit`/`useLeadProfile` setup).
   - The existing `TestServeStdioTicketsCreateUsesResolvedSageReviewConfig`
     already covers "explicit project-scope override still wins" (it sets
     `sage_review=ask` and expects `recommended`, not the new `required`
     default) — verify it still passes unchanged after the swap; no edit
     needed unless it breaks.

## Verification Plan
- `cd agents-plugin-tool && go build ./...`
- `cd agents-plugin-tool && go test ./internal/mcp/... ./internal/wsconfig/... ./internal/wsdoc/...`
- Manually confirm via the new/updated test(s) that:
  - No project-scope `sage_review` override → resolved posture `required`.
  - Explicit project-scope `sage_review` override (e.g. `ask`) → still
    resolves to its mapped posture (`recommended`), unaffected by the builtin
    change.

## Escalations
- None.

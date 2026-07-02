# Plan: 260630-refactor-ws-raw-tool-prefix-removal — Phase 2: Update prose references

## Relevant Ticket Contract

- Replace all `ws.*` tool identifiers in prose strings (AI instructions, error strings, follow-up hints, rsrc markdown) with unprefixed names.
- Files in scope: `agents-plugin/rsrc/*.md`, `agents-plugin-tool/internal/mcp/session_state.go`, `agents-plugin-tool/internal/mcp/implement_resolver.go`, and any test fixtures hardcoding old names.
- `ws.path.generate` is prose-only (not a registered MCP tool); rename to `path.generate` in AI instruction prose.
- After any rsrc edits, run:
  ```
  WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -run TestRegenerateShippedManifest
  WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -run TestRegenerateWsflowRsrcMirror
  ```
- Verification: `go build ./...` and `go test ./...` pass.

## Out of Scope

- Phase 1 changes (server.go, runtime.json identifiers) — already completed in 6a73170e.
- `wsagent/agent.go` follow-up hint strings (`ws.mercenary.*`) — these are AI-facing shorthand lines returned from mercenary status/call/result handlers; they are not registered MCP tool names and are in a different package boundary. The ticket's Change Surface table does not list this file. Leave unchanged.
- `agents-plugin-tool/internal/mcp/server.go` description strings (`ws.mercenary.*`, `ws.agenda.set value blobs` comment) — Phase 1 already handled the server.go functional identifiers; remaining occurrences are prose in code comments and a tool description that name the mercenary surface by logical name, not by MCP registration key. The ticket does not list these as Phase 2 targets.
- `agents-plugin-tool/internal/mcp/playbook_tools.go` prose (`ws.mercenary.call`, `ws.mercenary.register`) — these are commentary and user-facing text in the always-on mercenary tip and guidance blocks, not registered MCP tool identifiers. Not listed in the ticket's Change Surface table.
- `agents-plugin-tool/internal/mcp/workflow_manual.go` Go comments (lines 13, 26, 28, 150, 170) referencing `ws.workflow_manual` and `ws.ferrule` — these are internal code comments, not AI instruction prose strings returned to callers. The ticket's Phase 1 Result already updated error strings in this file; remaining references are comments only.
- Test comment strings that label legacy behavior (e.g., `ws.mercenary.*` in `prefer_mercenary_phase2_test.go:127`, `session_auth_test.go:458/494`, `mercenary_surface_test.go` comment strings) — descriptive labels only, not functional assertions.
- `mercenary_surface_test.go:361` — calls `callToolOnce` with `"ws.lead.prefer_mercenary"` to assert it returns "unknown tool"; this is a regression guard for a removed tool, not a prose update.

## Codebase Findings

- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L31` — `ws.mercenary.call` in `ws:full-only` block → `mercenary.call`
- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L46` — `ws.ferrule(root:...)` in `ws:fresh-only` block → `ferrule(root:...)`
- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L56` — `ws.ferrule` → `ferrule`
- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L90-L93` — `ws.mercenary.register`, `ws.mercenary.call`, `ws.mercenary.result` → unprefixed
- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L161` — `ws.mercenary.cancel`, `ws.mercenary.call` → unprefixed
- `agents-plugin/rsrc/lead-salvage/lead-salvage.md#L57` — `ws.mercenary.result(...)` → `mercenary.result(...)`
- `agents-plugin/rsrc/lead-verify-design/lead-verify-design.md#L49-L51` — `ws.mercenary.register`, `ws.mercenary.call`, `ws.mercenary.result` → unprefixed (3 occurrences)
- `agents-plugin-tool/internal/mcp/session_state.go#L525` — AI instruction string: `ws.path.generate(kind: "plan", stems: [...])` → `path.generate(kind: "plan", stems: [...])`
- `agents-plugin-tool/internal/mcp/session_state.go#L838,862,904,952,956,960,972,1009,1035,1039,1043,1070,1072,1104,1135,1157,1175,1189,1210` — `const tool = "ws.agenda.set"` etc. used in error formatting; replace all `"ws.<tool-name>"` local `tool` const/variable strings with unprefixed equivalents
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L661` — AI instruction prose: `ws.path.generate(kind: "plan")` → `path.generate(kind: "plan")`
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L45,87,169` — test assertions check for presence/absence of `ws.path.generate` substring in instructions; must be updated to `path.generate` after source change
- `agents-plugin-tool/internal/mcp/session_state_test.go#L143,271,1619,1659,1664,1742` — test assertions hardcode `ws.path.generate` in expected instruction strings; must match updated source

## Implementation Plan

1. Edit `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`: replace all `ws.ferrule`, `ws.mercenary.call`, `ws.mercenary.register`, `ws.mercenary.result`, `ws.mercenary.cancel` with unprefixed forms (7 occurrences).
2. Edit `agents-plugin/rsrc/lead-salvage/lead-salvage.md#L57`: replace `ws.mercenary.result` → `mercenary.result`.
3. Edit `agents-plugin/rsrc/lead-verify-design/lead-verify-design.md#L49-L51`: replace `ws.mercenary.register`, `ws.mercenary.call`, `ws.mercenary.result` → unprefixed (3 lines).
4. Run regen tests to rebuild shipped manifests from updated rsrc:
   ```
   cd agents-plugin-tool && WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -run TestRegenerateShippedManifest
   cd agents-plugin-tool && WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -run TestRegenerateWsflowRsrcMirror
   ```
5. Edit `agents-plugin-tool/internal/mcp/session_state.go`: replace all `"ws.agenda.set"`, `"ws.agenda.clear"`, `"ws.enter.implement"`, `"ws.enter.proceed"`, `"ws.enter.sprint"`, `"ws.enter.salvage"`, `"ws.todo.append"`, `"ws.todo.insert_before"`, `"ws.todo.insert_after"`, `"ws.todo.check"`, `"ws.todo.erase"`, `"ws.todo.clear"`, `"ws.todo.list"`, `"ws.todo.read"`, `"ws.todo.reorder"` (all are local `tool` const/variable values used only in error formatting) with unprefixed equivalents; also replace `ws.path.generate` in the AI instruction string at L525.
6. Edit `agents-plugin-tool/internal/mcp/implement_resolver.go#L661`: replace `ws.path.generate(kind: "plan")` → `path.generate(kind: "plan")`.
7. Update test assertions in `agents-plugin-tool/internal/mcp/implement_resolver_test.go`: change `"ws.path.generate"` to `"path.generate"` at lines 45, 87, and 169.
8. Update test assertions in `agents-plugin-tool/internal/mcp/session_state_test.go`: change all `ws.path.generate` occurrences in expected instruction strings at lines 143, 1619, 1659, 1664, 1742. Line 271 checks for forbidden `"ws.path.generate"` — update to `"path.generate"`.
9. Run full build and test suite: `cd agents-plugin-tool && go build ./... && go test -count=1 ./...`

## Verification Plan

- `go build ./...` must pass with no errors.
- `go test -count=1 ./...` must pass; the test suite includes functional round-trips for session-state tools (error messages now carry unprefixed names) and implement resolver instruction checks (now expect `path.generate`).
- Spot-check: grep for remaining `ws\.` in the changed files to confirm no old-style tool names were missed.

## Escalations

- None.

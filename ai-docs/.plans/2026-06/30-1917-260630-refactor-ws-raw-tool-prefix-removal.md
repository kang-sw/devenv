# Plan: 260630-refactor-ws-raw-tool-prefix-removal — Phase 1: Rename tool identifiers in server.go and runtime.json

## Relevant Ticket Contract

- Remove `ws.` prefix from all workflow-state MCP tools; no compatibility aliases.
- Applies to both `ws` and `wsflow` product modes.
- Scope: `server.go` tool identifiers and `runtime.json` capability declarations only. Prose references (rsrc, session_state.go, implement_resolver.go) are Phase 2.
- `bootstrapToolName` const change must propagate to all callsites in the same commit (used in `isLeadOnlyTool`, switch case, and tool definition block).
- `agents-plugin/runtime.json` is regen-covered; `agents-plugin-wsflow/runtime.json` is hand-maintained — both must be updated in this slice.
- Post-edit regen required: `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -run TestRegenerateShippedManifest` and `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -run TestRegenerateWsflowRsrcMirror`.
- Root-aware session safety and session_key/root bootstrap constraints are unchanged; only string identifiers change.

## Out of Scope

- Prose references in `agents-plugin/rsrc/*.md` — Phase 2.
- `session_state.go` AI instruction prose (`ws.path.generate` etc.) — Phase 2.
- `implement_resolver.go` AI instruction prose — Phase 2.
- Test fixture strings hardcoding old names — Phase 2.
- `ws.path.generate` is prose-only (not a registered MCP tool); no tool rename needed.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/server.go#L52` — `bootstrapToolName = "ws.ferrule"` const; rename to `"ferrule"`. All callsites use the const so no secondary literal hunts needed for this name.
- `agents-plugin-tool/internal/mcp/server.go#L62` — `isLeadOnlyTool`: hardcodes `"ws.workflow_manual"` and `HasPrefix(..., "ws.lead.")` as literals (not using the const). Both must be updated to `"workflow_manual"` and `"lead."`.
- `agents-plugin-tool/internal/mcp/server.go#L393-L423` — switch cases for all agenda/enter/todo/workflow_manual tools (13 ws.* cases); rename each.
- `agents-plugin-tool/internal/mcp/server.go#L1166-L1342` — switch cases for all ws.mercenary.* tools (14 cases plus `ws.mercenary.recall` at L1314 which has no tool definition but has a live case handler — must be renamed to `mercenary.recall`).
- `agents-plugin-tool/internal/mcp/server.go#L1298` — `strings.TrimPrefix(params.Name, "ws.mercenary.debug.")` — rename prefix argument to `"mercenary.debug."`.
- `agents-plugin-tool/internal/mcp/server.go#L2658-L3610` — tool definition `"name"` fields (29 ws.* definitions); rename all.
- `agents-plugin-tool/internal/mcp/server.go#L3656-L3659` — `toolSchemaRequiresSessionKey`: 14 string literals for ws.mercenary.* tools; rename all.
- `agents-plugin-tool/internal/mcp/server.go#L3689,L3710,L3735,L3761,L3778,L3780,L3895` — `HasPrefix(name, "ws.mercenary.")` appears 7 times (in `LeadToolNames`, `filteredTools`, `publicToolDefinition`, `toolAllowed`, `roleAllowsTool` x2, `noAgentHiddenTool`); rename prefix to `"mercenary."`.
- `agents-plugin-tool/internal/mcp/server.go#L3848` — `agentCallHandleText` return string includes prose references to `ws.mercenary.*` follow-up tools. This is a prose string in Go source — Phase 2 scope per ticket, but note it is inside `server.go`. Ticket Change Surface table lists `workflow_manual.go` and `session_state.go` for prose, not `server.go`; treat this line as Phase 2 (prose only, not an identifier).
- `agents-plugin-tool/internal/mcp/workflow_manual.go#L165,L185,L200,L212,L216` — error strings reference `ws.workflow_manual` and `ws.ferrule` (5 occurrences); rename both. (Ticket explicitly calls this file out for Phase 1.)
- `agents-plugin/runtime.json#L11-L56` — 17 `ws.*` tool keys in the `"tools"` map; rename all. File is regen-covered but the ticket says regen covers it — run regen after server.go edits, do not hand-edit.
- `agents-plugin-wsflow/runtime.json#L14-L57` — 17 `ws.*` tool keys in the `"tools"` map; **hand-edit required** (not regenerated). Rename identically to ws runtime.json.
- **Risk signal**: `ws.mercenary.recall` (L1314) has a live switch case but no corresponding tool definition entry and is absent from the ticket's rename table. It still needs renaming in the case handler to `mercenary.recall` for consistency; the regen tests will not catch a missed case rename.
- **Risk signal**: `agentCallHandleText` at L3848 contains `ws.mercenary.*` prose inside `server.go`. The ticket Change Surface table does not list `server.go` for prose, so this should be handled in Phase 2; verify with lead if needed.

## Implementation Plan

1. **`server.go` — const and gating functions** (`agents-plugin-tool/internal/mcp/server.go#L52,L62`):
   - Change `bootstrapToolName = "ws.ferrule"` to `bootstrapToolName = "ferrule"`.
   - In `isLeadOnlyTool`, change literal `"ws.workflow_manual"` to `"workflow_manual"` and `HasPrefix(name, "ws.lead.")` to `HasPrefix(name, "lead.")`.

2. **`server.go` — switch case handlers** (`server.go#L393-L423` and `#L1166-L1342`):
   - Strip `ws.` prefix from every `case "ws.agenda.*"`, `"ws.enter.*"`, `"ws.todo.*"`, `"ws.workflow_manual"`, and `"ws.mercenary.*"` (including `ws.mercenary.recall` at L1314).
   - Update `strings.TrimPrefix(params.Name, "ws.mercenary.debug.")` to `strings.TrimPrefix(params.Name, "mercenary.debug.")` at L1298.

3. **`server.go` — tool definition name fields** (`server.go#L2658-L3610`):
   - Strip `ws.` prefix from all 29 `"name": "ws.*"` map entries.

4. **`server.go` — toolSchemaRequiresSessionKey** (`server.go#L3656-L3659`):
   - Strip `ws.` prefix from all 14 `ws.mercenary.*` literals in the case list.

5. **`server.go` — HasPrefix guards** (`server.go#L3689,L3710,L3735,L3761,L3778,L3780,L3895`):
   - Change all `HasPrefix(name, "ws.mercenary.")` to `HasPrefix(name, "mercenary.")`.

6. **`workflow_manual.go` — error strings** (`agents-plugin-tool/internal/mcp/workflow_manual.go#L165,L185,L200,L212,L216`):
   - Change `"ws.workflow_manual"` to `"workflow_manual"` and `"ws.ferrule"` to `"ferrule"` in the five error return strings.

7. **`agents-plugin-wsflow/runtime.json` — hand-edit** (full file, 17 entries):
   - Strip `ws.` prefix from all `ws.*` keys in the `"tools"` map: `ws.ferrule` → `ferrule`, `ws.agenda.*` → `agenda.*`, `ws.enter.*` → `enter.*`, `ws.todo.*` → `todo.*`, `ws.workflow_manual` → `workflow_manual`.

8. **Regen `agents-plugin/runtime.json`** (run from `agents-plugin-tool/`):
   ```
   WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -run TestRegenerateShippedManifest
   WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -run TestRegenerateWsflowRsrcMirror
   ```
   These regenerate `agents-plugin/runtime.json` and the wsflow rsrc mirror from the live tool list.

## Verification Plan

- `go build ./...` from `agents-plugin-tool/` — must pass with no errors.
- `go test ./...` from `agents-plugin-tool/` — all tests must pass, including `TestShippedManifestUpToDate` and `mercenary_surface_test.go`.
- Confirm `agents-plugin/runtime.json` no longer contains any `ws.ferrule`, `ws.agenda.*`, `ws.enter.*`, `ws.todo.*`, or `ws.workflow_manual` keys.
- Confirm `agents-plugin-wsflow/runtime.json` matches the same set of renamed tool keys.
- Confirm `isLeadOnlyTool` still gates `ferrule`, `workflow_manual`, and tools with `lead.` prefix correctly (no regression in session auth).

## Escalations

- None.

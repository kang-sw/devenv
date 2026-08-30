# Plan: 260814-feat-manuals-always-on-authoring-anchor — Phase 2: Retire manuals.list / manuals.find

## Relevant Ticket Contract

- Remove the `manuals.list`/`manuals.find` MCP tools (schema + dispatch) and the `ws-mcp manuals list|find` CLI mirror.
- Remove dead Go surface `ManualsFind` and `formatManuals`; **keep `ManualsList`** — `computeManuals` (the always-on ambient block landed in Phase 1) still depends on it.
- Reverse the `d2c82584` wiring: `toolSchemaRequiresSessionKey`, `runtimeCapabilityCommandNames()`, both `runtime.json` files (tools + commands sections, ws and wsflow).
- Regenerate the wsflow `rsrc/` mirror only if the removal touches mirrored surface (regen only, never hand-edit).
- Update the exact-match runtime-capability tests and any manuals-tool tests.
- Spec: remove `mcp-tools.md {#260807-manuals-discovery-tools}`; redirect the `documentation-system.md {#260807-manuals-document-system}` discovery-surface statement to name the ambient `# Manuals` block.
- Constraint: do not remove `ManualsList` or the manuals doc tier; only the two MCP tools/CLI mirror retire.
- Version bump is the lead's merge-time step — out of scope for this plan.

## Out of Scope

- Phase 1 behavior (already landed in `525064f4`) — the always-on `computeManuals` anchor, its guidance text, and its `.local.md` rendering rule are not touched.
- Adding a `manuals-conventions.md` `convention.read` doc — explicitly out of scope per the ticket's Decisions.
- Any change to the manuals doc tier schema (`summary:` field) or `ManualsList` behavior itself.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/server.go#L1232-L1252` — `case "manuals.list":` / `case "manuals.find":` dispatch blocks; both call `formatManuals` and `wsdoc.ManualsList`/`wsdoc.ManualsFind`.
- `agents-plugin-tool/internal/mcp/server.go#L4273-L4291` — the two tool schema entries inside `tools()`.
- `agents-plugin-tool/internal/mcp/server.go#L4744` — `toolSchemaRequiresSessionKey` switch list includes `"manuals.list", "manuals.find"` among many other names on one line; remove just those two tokens.
- `agents-plugin-tool/internal/mcp/server.go#L3172-L3178` — `formatManuals` function to delete.
- **`LeadToolNames()` (server.go#L4767-L4788) needs NO separate edit.** It derives tool names dynamically from `tools()` (filtered by `permanentlyHiddenTool`/no-agent/mercenary-hidden), so removing the two schema entries from `tools()` automatically removes them from `LeadToolNames()` and thus from `runtime.capabilities`. The ticket text's "drop the manuals entries from ... the LeadToolNames session-key list" describes the *effect*, not a separate edit site — confirmed no standalone hardcoded list exists for this.
- `agents-plugin-tool/internal/wsdoc/manuals.go#L59-L90` — `ManualsFind` function to delete. **Keep** `ManualsList` (L28-57) unchanged.
- `agents-plugin-tool/internal/wsdoc/manuals.go#L20-L23` — `ManualsList`'s doc comment references "`formatManuals` in package `mcp`" by name; this becomes stale once `formatManuals` is deleted and must be updated (e.g. point at `computeManuals` in `manuals_announcement.go` instead).
- `agents-plugin-tool/internal/wsdoc/manuals_test.go#L89-L115` — `TestManualsFindFiltersByQueryAcrossSummaryAndBody` and `TestManualsFindWithEmptyQueryReturnsFullList` directly exercise `ManualsFind`; both must be deleted alongside the function or the package fails to build. (Not named explicitly in the ticket text — found by tracing all `ManualsFind` callers.)
- `agents-plugin-tool/internal/mcp/format.go#L78-L80` — exported `FormatManuals` wrapper around `formatManuals`. Its only caller is the CLI mirror (`cmd/ws-mcp/main.go`); once the CLI mirror is removed this wrapper is dead too and must be deleted (also not explicitly named in the ticket text, found by tracing `FormatManuals` usage — only 2 call sites, both in the CLI mirror being removed).
- `agents-plugin-tool/internal/mcp/manuals_workflow_manual_test.go#L53-L101` — `TestManualsListAndFindMCPToolsReturnFixtureManual` (the exact test the ticket names) to delete. The file's other two tests (`TestWorkflowManualCarriesManualsBlockOnFreshAndContinue`, `TestWorkflowManualManualsBlockIsAlwaysOnWhenNoManualsExist`) test the Phase-1 ambient block and stay unchanged.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L58-L59` — `case "manuals": manualsCommand(os.Args[2:])` top-level dispatch to remove.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L74,L77` — two `usage()` strings list `...|manuals|references>`; drop `manuals|` from both.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L851-L899` — `manualsCommand`, `manualsUsage`, `manualsList`, `manualsFind` — the entire CLI mirror implementation to delete as one contiguous block.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L231-L232` — `runtimeCapabilityCommandNames()`'s hardcoded slice contains `"manuals.find"` and `"manuals.list"` entries to remove.
- `agents-plugin-tool/cmd/ws-mcp/main_test.go#L400-L401` — fixture writes (`ai-docs/manuals/deploy.md`, `ai-docs/manuals/no-summary.md`) inside `TestDocumentationCLICommandsDefaultToTextAndKeepJSONFormat`; confirmed by grep these fixtures are used ONLY by the manuals CLI subtests, safe to delete alongside them.
- `agents-plugin-tool/cmd/ws-mcp/main_test.go#L414-L415` — the `"manuals list"`/`"manuals find"` table-driven subtest entries.
- `agents-plugin-tool/cmd/ws-mcp/main_test.go#L473-L495` — the trailing no-summary verification block that shells out to `manuals list`/`manuals find` directly (covers verification requirement (b) at the CLI layer) — delete this whole block too.
- `agents-plugin-tool/cmd/ws-mcp/main_test.go#L51-L186` — **`TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`** and **`TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`** (this is the actual "wsflow counterpart" — both live in this one file, not a separate location). Both read their expected tool/command sets directly from `agents-plugin/runtime.json` / `agents-plugin-wsflow/runtime.json` via `readRuntimeContractTest`/`readRuntimeContractAtTest` (L678-694) and diff against the live binary's `runtime capabilities` output. **No test-code edit is needed for either** — they self-correct once the two `runtime.json` files and the binary (schema+dispatch+CLI) are updated consistently. This differs from the ticket text's implication that these tests need direct edits.
- `agents-plugin/runtime.json#L54-L55` (tools) and `#L121-L122` (commands) — the two `manuals.list`/`manuals.find` entries in each section.
- `agents-plugin-wsflow/runtime.json#L56-L57` (tools) and `#L105-L106` (commands) — same shape, wsflow copy.
- `ai-docs/spec/mcp-tools.md#L1573-L1582` — `## Manuals Discovery Tools {#260807-manuals-discovery-tools}` section (heading + two paragraphs), sitting between `## Mental-Model Discovery Tools` (ends L1571) and `## Reference Trace Tool` (starts L1583). Confirmed no other doc references this anchor (`grep -rn 260807-manuals-discovery-tools` hits only this spec and the ticket itself) — safe to delete outright.
- `ai-docs/spec/documentation-system.md#L231-L233` — `` `ws/manuals.list` and `ws/manuals.find` expose manual path and summary metadata... `` paragraph to redirect toward the ambient `# Manuals` block as the discovery surface (Phase 1 already updated the surrounding paragraphs at L204-222; this is the one paragraph phase 1 left alone).
- `ai-docs/spec/documentation-system.md#L223-L229` (specifically `#L226`) — adjacent paragraph says a summary-less manual doesn't vanish "from the ambient block **or discovery tools**"; after removal there is only the ambient block, so this phrase needs the same-section consistency fix (trim "or discovery tools"). Not explicitly named in the ticket's Spec Impact but directly adjacent to the required edit and left stale otherwise.
- `ai-docs/manuals/wsflow-mirroring.md` and a full-repo grep for `manuals.list`/`manuals.find` across `agents-plugin/rsrc`, `agents-plugin/skills`, `agents-plugin-wsflow/rsrc`, `agents-plugin-wsflow/skills` — **zero hits**. No shipped skill or rsrc playbook body references either tool name, and `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py` diffs the live binary against `runtime.json` dynamically (no hardcoded manuals strings). **No wsflow rsrc mirror regen is required for this phase** — confirmed by search, not assumed.
- Baseline verified green before planning: `go build ./...`, `go vet ./...` clean from `agents-plugin-tool/`; `go test ./internal/mcp/... -run 'Manuals|RuntimeCapabilities'` and `go test ./cmd/ws-mcp/... -run 'RuntimeCapabilities|Documentation'` both pass today. Pre-existing `gofmt -l` dirty files (`internal/mcp/playbook_tools.go`, `internal/wsagent/agent_test.go`, `internal/wsconfig/config.go`, `internal/wsconfig/global.go`) are unrelated — do not touch them; scope `gofmt -l` checks to files this phase actually edits.

## Implementation Plan

1. `agents-plugin-tool/internal/mcp/server.go`: delete the `manuals.list`/`manuals.find` dispatch cases (L1232-1252), the two tool schema entries (L4273-4291), the `formatManuals` function (L3172-3178), and the `"manuals.list", "manuals.find"` tokens from the `toolSchemaRequiresSessionKey` switch line (L4744). Verify `LeadToolNames()` needs no edit (derives from `tools()`).
2. `agents-plugin-tool/internal/wsdoc/manuals.go`: delete `ManualsFind` (L59-90); update `ManualsList`'s doc comment (L20-23) to stop naming the now-deleted `formatManuals`, pointing instead at `computeManuals` (`manuals_announcement.go`) as the remaining consumer.
3. `agents-plugin-tool/internal/wsdoc/manuals_test.go`: delete `TestManualsFindFiltersByQueryAcrossSummaryAndBody` and `TestManualsFindWithEmptyQueryReturnsFullList` (L89-115).
4. `agents-plugin-tool/internal/mcp/format.go`: delete the `FormatManuals` wrapper (L78-80).
5. `agents-plugin-tool/internal/mcp/manuals_workflow_manual_test.go`: delete `TestManualsListAndFindMCPToolsReturnFixtureManual` (L53-101); leave the other two tests in the file untouched.
6. `agents-plugin-tool/cmd/ws-mcp/main.go`: delete the `case "manuals":` dispatch (L58-59), drop `manuals|` from both `usage()` strings (L74, L77), delete `manualsCommand`/`manualsUsage`/`manualsList`/`manualsFind` (L851-899), and delete the `"manuals.find"`/`"manuals.list"` entries from `runtimeCapabilityCommandNames()`'s slice (L231-232).
7. `agents-plugin-tool/cmd/ws-mcp/main_test.go`: inside `TestDocumentationCLICommandsDefaultToTextAndKeepJSONFormat`, delete the two manuals fixture writes (L400-401), the two `"manuals list"`/`"manuals find"` table entries (L414-415), and the trailing summary-less verification block (L473-495).
8. `agents-plugin/runtime.json`: delete the `manuals.list`/`manuals.find` entries from the `tools` section (L54-55) and the `commands` section (L121-122).
9. `agents-plugin-wsflow/runtime.json`: same removal in its `tools` (L56-57) and `commands` (L105-106) sections.
10. `ai-docs/spec/mcp-tools.md`: delete the `## Manuals Discovery Tools {#260807-manuals-discovery-tools}` section (L1573-1582) in full.
11. `ai-docs/spec/documentation-system.md`: rewrite the `` `ws/manuals.list` and `ws/manuals.find` `` paragraph (L231-233) to state the always-on ambient `# Manuals` block (not the two retired tools) is the manuals discovery surface; also trim the stale "or discovery tools" phrase at L226 for internal consistency within the same section.
12. Run `ws/spec_index.verify` to confirm no dangling anchor references after the `mcp-tools.md` section removal (pre-verified no cross-doc references exist, but this is the authoritative check).
13. Skip wsflow rsrc mirror regen — confirmed no shipped rsrc/skill body references either retired tool name; do not run the regen commands from `ai-docs/manuals/wsflow-mirroring.md` for this phase.

## Verification Plan

- `cd agents-plugin-tool && go build ./...`
- `cd agents-plugin-tool && go vet ./...`
- `cd agents-plugin-tool && gofmt -l <touched files>` (scope to files this phase edits; ignore the four pre-existing dirty files noted above)
- `cd agents-plugin-tool && go test ./internal/mcp/... ./internal/wsdoc/...`
- `cd agents-plugin-tool && go test ./cmd/ws-mcp/...` (covers both `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` and `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`, which read `runtime.json` dynamically and should pass once step 8-9 land)
- `python3 -m unittest discover agents-plugin-wsflow/tests` from repo root (wsflow package contract + skill bundle drift check)
- `ws/spec_index.verify` (spec anchor integrity)
- **Known-unrelated pre-existing failures to ignore, do not chase**: `TestWorkflowManualCarriesNotesBlockOnFreshAndContinuePositionedAfterSessionState` and `TestWorkflowManualNotesBlockAbsentWhenNoNotesExist` — both fail from a naive `strings.Index(body, "# Notes")` colliding with the prose heading `### Notes / durable memory` (added in `fbec365f`), unrelated to this phase's manuals surface.

## Escalations

- None.

# Plan: 260903-refactor-mcp-todo-signature-merge — Phase 1: Merge the insert-trio into todo.add

## Relevant Ticket Contract

- Frozen signature: `todo.add { session_key, key, title, instruction?, position?: "end"|"before"|"after" (default "end"), ref_key? }`, `required: [session_key, key, title]`.
- `ref_key` required iff `position ∈ {before, after}`; must be **omitted** (not just empty) when `position` is `end` — rejected as an error, not silently ignored.
- Frozen error contract (verbatim, reusing existing messages where noted):
  - `position` not in enum → `"todo.add: position must be one of end, before, after"`.
  - `position ∈ {before, after}` and `ref_key` missing/empty → `"todo.add: ref_key is required when position is <before|after>"`.
  - `position = end` and `ref_key` supplied → `"todo.add: ref_key must be omitted when position is end"`.
  - bad `ref_key` format / unknown `ref_key` / duplicate `key` / bad `instruction` type → reuse `todoInsert`/`normalizeTodoKey`/`todoAppend`/`todoInstructionArg` messages verbatim (unchanged core functions, just wrapped with `%s: %w` under the `"todo.add"` tool name instead of the old tool names).
- Frozen output: single compact confirmation `"todo added: <normalizedKey>\n"` for **all** three positions — replaces both `"todo appended: %s\n"` and `"todo inserted: %s\n"`.
- Implementation constraint: this is a schema/dispatch **merge**, not new list logic — `handleTodoAdd` branches on `position` and calls the existing `todoAppend(...)` (for `end`) or `todoInsert(..., after = position=="after")` (for `before`/`after`). Do not touch `todoAppend`/`todoInsert`/`normalizeTodoKey` core logic.
- Remove `todo.append`, `todo.insert_before`, `todo.insert_after` from registration (`tools()` schema list) and dispatch (`callTool` switch). One-shot hard cut — no alias.
- Full call-site sweep required (ticket enumerates all of these explicitly): Go registration/dispatch, `runtime.json` ×2 (both drift-tested against the live tool set — see Codebase Findings), `mcp-tools.md` `{#260625-session-state-tools}` (including the "no skill-side `todo.append` loop is needed" prose sentence), the `tickets.checklist` tool description ("a single todo.append instruction"), playbook token call sites in `agents-plugin/rsrc/lead-write-ticket/`, `lead-forge-spec/`, `lead-forge-mental-model/` (wsflow mirror is regenerated, never hand-edited), and tests.
- Acceptance: `grep -r "todo\.(append|insert_before|insert_after)"` clean across code/specs/playbooks; `todo.add` reproduces each old placement with a byte-identical *mutation*; new unified confirmation string asserted by tests; every error branch fails loud; full Go suite green (`go test ./...` in `agents-plugin-tool/`, run with `-count=1` since two regen steps below use env-gated test bodies that the cache can mask).
- Spec impact: edit `mcp-tools.md` `{#260625-session-state-tools}` anchor only — no new stem, no heading change.

## Out of Scope

- `todo.check`, `todo.erase`, `todo.clear`, `todo.reorder`, `todo.list`, `todo.read` — untouched; `read`↔`list` fold explicitly left for later.
- `note.mute`/`note.unmute` — candidate dropped by the ticket's background section; not touched.
- Layers ① (enter affordance rename), ③ (read-surface collapse), ④ (verb-vocabulary rename) of the parent epic — separate tickets/phases.
- The epic-level prose mentioning `todo.append`/`insert_before`/`insert_after` at `ai-docs/tickets/todo/260903-epic-mcp-tool-surface-affordance-reduction.md#L57` — descriptive epic background, not an actionable phase file; leave as historical narrative.
- `CHANGELOG.md` and any `.done`/`.plans` historical files that mention the old tool names — archival, not live call sites.
- `agents-plugin/skills/**/SKILL.md` — grep confirmed no SKILL.md body references `todo.append`/`insert_before`/`insert_after` directly (only the `rsrc/` playbook bodies do), so the skills-manifest regen (`WSRSRC_REGEN_SKILLS=1 ... TestGenerateRealSkillsManifest`) is not required for this phase.
- `ai-docs/mental-model/mcp-runtime.md` — grep confirmed no todo-tool-specific mentions; no edit needed.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/session_state.go#L122-134` — `todoAppend(list, key, title, status, instruction)`: pure core, duplicate-key check via `indexOfTodo`, appends to end. Reuse unchanged.
- `agents-plugin-tool/internal/mcp/session_state.go#L136-163` — `todoInsert(list, refKey, key, title, status, instruction, after bool)`: pure core shared by both former insert tools, differing only by the `after` bool. Reuse unchanged.
- `agents-plugin-tool/internal/mcp/session_state.go#L1180-1205` — `handleTodoAppend`: parses `session_key`/`key`/`title`/`instruction` via `sessionStateKey`/`rawStringArg`/`todoInstructionArg`, normalizes key, calls `todoAppend` under `s.sessions.mutateTodos`, returns `"todo appended: %s\n"`. This whole function is replaced by `handleTodoAdd`.
- `agents-plugin-tool/internal/mcp/session_state.go#L1207-1239` — `handleTodoInsert(id, args, after bool)`: same arg-parsing shape plus `ref_key` (via `rawStringArg`, required today), calls `todoInsert`, returns `"todo inserted: %s\n"`. Replaced by the same `handleTodoAdd`.
- `agents-plugin-tool/internal/mcp/session_state.go#L881-887` — `rawStringArg(toolName, name, args)`: returns `"%s: %s is required"` when the value is missing or empty. Reusable as-is for `key`/`title`, but **not** for the conditional `ref_key` — that needs bespoke presence/enum logic (see Implementation Plan step 1) because the requirement is conditional on `position`, and the "must be omitted for end" branch needs to distinguish "present" (even empty string) from "absent", which `rawStringArg`'s empty-string-means-missing collapse cannot express.
- `agents-plugin-tool/internal/mcp/server.go#L901-914` (`handleAgendaSet`) — precedent for map-presence detection via `value, ok := args["value"]` (checks the comma-ok idiom, not just an empty-string/zero-value check). Use the same idiom for detecting whether `ref_key` was supplied at all, since "supplied but empty" must still trip the `end`-must-omit error per the ticket's fail-loud stance ("consistent with reorder's exactly-one posture").
- `agents-plugin-tool/internal/mcp/server.go#L1362-1374` (`handleTodoReorder`) — precedent for `position` as a **nested** `{before|after: ref_key}` object; the ticket explicitly reserves that shape for `reorder` and wants `todo.add`'s `position` as a flat enum string instead — do not copy this shape.
- `agents-plugin-tool/internal/mcp/server.go#L4970-4976` (`enumStringProperty`) and `#L3615` (`"format": enumStringProperty(...)` used as an **optional**, non-required schema field) — exact precedent for `todo.add`'s optional `position` enum: use `enumStringProperty` and simply omit `position` from the `required` array (JSON Schema optional-enum, not `nullableEnumStringProperty` — the field is omittable, not nullable).
- `agents-plugin-tool/internal/mcp/server.go#L556-561` — dispatch cases to remove: `"todo.append"` → `handleTodoAppend`, `"todo.insert_before"`/`"todo.insert_after"` → `handleTodoInsert(..., false/true)`. Replace with one `case "todo.add": return s.handleTodoAdd(req.ID, params.Arguments)`, placed in the same slot (immediately before the existing `case "todo.check":` at `#L562`).
- `agents-plugin-tool/internal/mcp/server.go#L3620-3663` — the three schema entries to delete (`todo.append` `#L3620-3633`, `todo.insert_before` `#L3634-3648`, `todo.insert_after` `#L3649-3663`), replaced by one `todo.add` entry in the same slot, immediately before the `todo.check` entry (`#L3664` today).
- `agents-plugin-tool/internal/mcp/server.go#L4174` — `tickets.checklist` schema `"description"` field: `"...for installing into a single todo.append instruction..."` → `"...for installing into a single todo.add instruction..."`.
- No `toolSchemaRequiresSessionKey`-style allowlist or other static tool-name list references `todo.append`/`insert_before`/`insert_after` beyond the dispatch switch and schema list above (grep-confirmed) — session-state tools are reachable by any role per the comment at `#L856-859` (`roleAllowsTool` does not gate these prefixes), so no role-gating list needs an update.
- `agents-plugin/runtime.json#L19-21` and `agents-plugin-wsflow/runtime.json#L22-24` — `"todo.append"`, `"todo.insert_before"`, `"todo.insert_after"` keys under `"tools"`, each `">=0.44.4-dev <0.45.0"`. Both files are **exact-match drift-tested**, not just documentation: `agents-plugin-tool/cmd/ws-mcp/main_test.go#L152-159` (`TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`) reads `agents-plugin-wsflow/runtime.json` and compares its tool set against the live `runtime capabilities` output, and `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L89-116` (`test_runtime_contract_matches_agentless_capabilities`, `self.assertEqual(set(contract["tools"]), set(payload["tools"]))`) does the same from the Python side. Both runtime.json files must have the three old keys removed and one `"todo.add": ">=0.44.4-dev <0.45.0"` key added, or these tests fail.
- `ai-docs/spec/mcp-tools.md#L275-276` — "Derivation logic lives in Go, so no skill-side `todo.append` loop is needed for a covered mode:" → rename `todo.append` to `todo.add` in this sentence (the point being made — no manual loop needed — is unaffected by the merge).
- `ai-docs/spec/mcp-tools.md#L408-425` (the `**Todo.**` paragraph under `{#260625-session-state-tools}`) — `#L414-415`: "Creation mutations (`todo.append`, `todo.insert_before`, and `todo.insert_after`) accept optional nullable `instruction` and reject non-string non-null values." must become a `todo.add`-only sentence documenting the `position` enum (default `end`), the conditionally required `ref_key`, and the unified `instruction` contract. This is the ticket's one designated spec-impact anchor.
- `agents-plugin/rsrc/lead-forge-mental-model/lead-forge-mental-model.md#L96,100` — `{{.McpNamespace}}/todo.append` (bare mention) and `{{.McpNamespace}}/todo.append(session_key: ..., key: "forge-mental-model-<domain>", title: "...")` (a plain append call, no ref_key) → rename tool token to `todo.add` only; no other args needed since `position` defaults to `end`.
- `agents-plugin/rsrc/lead-forge-spec/lead-forge-spec.md#L106,110,298` — same pattern: one bare mention (`#L106`) and two plain-append call examples (`#L110`, `#L298`) → rename to `todo.add` only.
- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L52,61` — two prose mentions ("install one todo via `todo.append` carrying the returned ... checklist") → rename to `todo.add`; no call-shape change (still a plain append, no ref_key needed).
- `agents-plugin-wsflow/rsrc/{lead-forge-mental-model,lead-forge-spec,lead-write-ticket}/*.md` — byte-identical generated mirrors of the three files above (confirmed via grep, same line content). Per `ai-docs/manuals/wsflow-mirroring.md#L244-271`, do **not** hand-edit; regenerate via the after-edit checklist in Implementation Plan step 7.
- `agents-plugin-tool/internal/mcp/session_state_test.go` — 13 call sites and 8 confirmation-string assertions need updating (all confirmed via read, no other files match):
  - `#L1415` — `callToolWithKey(..., "todo.append", {key: "stale", title: "stale"})`, no assertion on the return value; rename tool only.
  - `#L2927-2954` (`TestServeStdioTodoKeyNormalization`) — `#L2927` and `#L2932` and `#L2945` and `#L2950` all use `"todo.append"`; `#L2929` asserts `"todo appended: review.step_1"` → `"todo added: review.step_1"`. The other three (`#L2932,2945,2950`) assert on `"already exists"`/`"invalid character"`/`"leading or trailing whitespace"` — unaffected by the confirmation-string rename, just the tool-name string.
  - `#L2957-3030` (`TestServeStdioTodoInstructionReadSurface`) — `#L2966` `"todo.append"` asserting `"todo appended: a"` (`#L2970`) → `"todo added: a"`; `#L2973` `"todo.insert_before"` (`ref_key: "a"`) asserting `"todo inserted: b"` (`#L2978`) → becomes `"todo.add"` with `position: "before", ref_key: "a"`, asserting `"todo added: b"`; `#L2981` `"todo.insert_after"` asserting `"todo inserted: c"` (`#L2986`) → `"todo.add"` with `position: "after", ref_key: "a"`, asserting `"todo added: c"`; `#L3022` `"todo.append"` with a bad `instruction` type, asserting `"instruction must be a string or null"` — tool-name rename only.
  - `#L3044-3050` (`TestServeStdioTodoListInstructionRendering`) — `"todo.append"` asserting `"todo appended: render"` → `"todo added: render"`.
  - `#L3075-3088` (`TestServeStdioTodoCheckCheckpointRendering`) — loop body `#L3083` `"todo.append"` asserting `"todo appended: "+item.key` → `"todo added: "+item.key`.
  - `#L3159-3165` (`TestServeStdioTodoReorderHandler`) — loop body `#L3160` `"todo.append"` asserting `"todo appended: "+k` → `"todo added: "+k`.
  - `#L3645-3651` (`TestWorkflowManualTodoInstructionPreview`) — `"todo.append"` asserting `"todo appended: restore"` → `"todo added: restore"`.
  - No other file under `agents-plugin-tool/internal/mcp/` references these three tool-name strings by grep — `TestTodoInsertAndCheck`/`TestTodoInstructionPreservedThroughStatusAndOrderMutations`/`TestTodoKeyUniquenessAndReuse` at `session_state_test.go#L510-576` call the pure `todoAppend`/`todoInsert` **functions** directly (not through tool dispatch), so those stay untouched.
- `agents-plugin-tool/internal/mcp/panic_recovery_test.go#L15,25,27,35` — `TestServeStdioRecoversPanicAndPersistsCrashTrace` uses the literal string `"todo.append"` purely as a stand-in write-handler name to force a panic via `testPanicHook`. Confirmed at `server.go#L521-522`: `testPanicHook(params.Name)` fires in the dispatch switch **before** the handler-specific case, keyed only by the raw tool-name string passed in the JSON-RPC request — so this is a safe, mechanical rename to `"todo.add"` in all four spots (the comment `#L15`, the `panicMessage` string `#L25`, the hook body's name check `#L27`, and the JSON-RPC request line `#L35`); the empty `"arguments":{}` payload is untouched (the panic fires before argument parsing).
- `ai-docs/manuals/wsflow-mirroring.md#L263-271` — exact regen order for `agents-plugin/rsrc/**` edits: (1) `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`, then (2) `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`. Both from `agents-plugin-tool/`. Both `-count=1` flags are mandatory — the regen entrypoints are env-gated test bodies with no changing input, so Go's test cache can return a stale green `ok` without running the write side effect.

## Implementation Plan

1. In `agents-plugin-tool/internal/mcp/session_state.go`, replace `handleTodoAppend` (`#L1180-1205`) and `handleTodoInsert` (`#L1207-1239`) with one `handleTodoAdd(id json.RawMessage, args map[string]any) response`:
   - `const tool = "todo.add"`.
   - Parse `session_key` (`sessionStateKey`), `key` (`rawStringArg`) → `normalizeTodoKey`, `title` (`args["title"].(string)`), `instruction` (`todoInstructionArg`) — identical to today's parsing.
   - Resolve `position`: default `"end"`; if `args["position"]` is present and non-nil, read it as a string (a non-string value simply won't match any enum case below). If the resolved value is not one of `end`/`before`/`after`, return `fmt.Errorf("%s: position must be one of end, before, after", tool)`.
   - Detect `ref_key` presence with the comma-ok idiom (mirroring `handleAgendaSet`'s `value, ok := args["value"]`, not `rawStringArg`'s empty-collapse): `refKeyRaw, refKeyPresent := args["ref_key"]`; treat `nil` as not-present; `refKey, _ := refKeyRaw.(string)`.
   - Branch on `position`:
     - `"end"`: if `refKeyPresent` (regardless of whether the string is empty), return `fmt.Errorf("%s: ref_key must be omitted when position is end", tool)`. Otherwise call `s.sessions.mutateTodos(sessionKey, func(list []todoItem) ([]todoItem, error) { return todoAppend(list, normalizedKey, title, todoPending, instruction) })`.
     - `"before"`/`"after"`: if `refKey == ""` (covers both "missing" and "present but empty" — matches the ticket's "missing/empty" wording and mirrors `rawStringArg`'s existing empty-collapse semantics for required fields), return `fmt.Errorf("%s: ref_key is required when position is %s", tool, position)`. Otherwise call `s.sessions.mutateTodos(sessionKey, func(list []todoItem) ([]todoItem, error) { return todoInsert(list, refKey, normalizedKey, title, todoPending, instruction, position == "after") })`.
   - On any `mutateTodos` error, return `fmt.Errorf("%s: %w", tool, err)` (unchanged wrapping pattern — this is what makes the bad-`ref_key`/duplicate-`key` messages verbatim-reused, since the wrapped error text comes straight from `todoAppend`/`todoInsert`/`normalizeTodoKey`).
   - On success, return `toolTextResponse(id, fmt.Sprintf("todo added: %s\n", normalizedKey), nil)` — the single unified confirmation for all three positions.
2. In `agents-plugin-tool/internal/mcp/server.go#L556-561`, replace the three dispatch cases with one: `case "todo.add": return s.handleTodoAdd(req.ID, params.Arguments)`, kept in the same position immediately before `case "todo.check":`.
3. In `agents-plugin-tool/internal/mcp/server.go#L3620-3663`, replace the three schema entries with one, in the same slot before the `todo.check` entry:
   ```go
   {
       "name":        "todo.add",
       "description": "Add a new pending todo item with a caller-provided key (unique within the active list) and title. position defaults to \"end\" (append); \"before\"/\"after\" insert relative to ref_key, which is required for those positions and rejected for \"end\". Erased keys are reusable.",
       "inputSchema": map[string]any{
           "type": "object",
           "properties": map[string]any{
               "session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
               "key":         stringProperty("Caller-provided item key. Normalized to lowercase; accepts letters, digits, '.', '_', and '-'; leading or trailing whitespace is rejected; unique within the active list after normalization."),
               "title":       stringProperty("Human-facing item title."),
               "instruction": nullableStringProperty("Optional full instruction prose for this item. Null or omit to leave unset."),
               "position":    enumStringProperty(`Where to add the item. Defaults to "end" (append). "before"/"after" require ref_key; "end" must not carry one.`, []string{"end", "before", "after"}),
               "ref_key":     stringProperty("Existing item key to insert before/after. Required when position is before or after; must be omitted when position is end."),
           },
           "required": []string{"session_key", "key", "title"},
       },
   },
   ```
   Note `position` and `ref_key` are deliberately absent from `required` — the enum default and the before/after-conditional requirement are runtime-validated in `handleTodoAdd`, not schema-encoded (matches how `format` is handled at `#L3615`).
4. In `agents-plugin-tool/internal/mcp/server.go#L4174`, change the `tickets.checklist` description's `"...into a single todo.append instruction..."` to `"...into a single todo.add instruction..."`.
5. Edit `agents-plugin/runtime.json#L19-21`: delete the three `"todo.append"`/`"todo.insert_before"`/`"todo.insert_after"` lines, add one `"todo.add": ">=0.44.4-dev <0.45.0",` in their place (keep alphabetic/positional ordering consistent with the surrounding list — same slot).
6. Edit `agents-plugin-wsflow/runtime.json#L22-24`: identical change.
7. Edit `agents-plugin/rsrc/lead-forge-mental-model/lead-forge-mental-model.md#L96,100`, `agents-plugin/rsrc/lead-forge-spec/lead-forge-spec.md#L106,110,298`, and `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L52,61`: rename the `todo.append` token to `todo.add` in each (call-shape unchanged — all are plain appends with no `ref_key`). Then run, from `agents-plugin-tool/`:
   ```bash
   WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
   WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
   ```
   to regenerate `agents-plugin/rsrc/manifest.json` and sync `agents-plugin-wsflow/rsrc/**` byte-for-byte. Do not hand-edit the wsflow copies.
8. Edit `ai-docs/spec/mcp-tools.md#L275-276`: rename `todo.append` to `todo.add` in the "no skill-side ... loop is needed" sentence.
9. Edit `ai-docs/spec/mcp-tools.md#L414-415`: replace "Creation mutations (`todo.append`, `todo.insert_before`, and `todo.insert_after`) accept optional nullable `instruction` and reject non-string non-null values." with a `todo.add`-only sentence documenting: single creation mutation `todo.add(position: end|before|after = end, ref_key?)`; `ref_key` required iff `position` is `before`/`after` and rejected for `end`; accepts optional nullable `instruction`, rejecting non-string non-null values (unchanged instruction contract); returns the unified `todo added: <key>` confirmation.
10. Update `agents-plugin-tool/internal/mcp/session_state_test.go` per the per-line mapping in Codebase Findings: rename every `"todo.append"`/`"todo.insert_before"`/`"todo.insert_after"` tool-name string literal to `"todo.add"`, adding `"position": "before"`/`"position": "after"` (with the existing `"ref_key"` value carried over unchanged) for the two former insert call sites (`#L2973`, `#L2981`), and update every `"todo appended: ..."`/`"todo inserted: ..."` assertion string to `"todo added: ..."`.
11. Update `agents-plugin-tool/internal/mcp/panic_recovery_test.go#L15,25,27,35`: rename the four `"todo.append"` occurrences (comment, `panicMessage` string, hook name check, JSON-RPC request line) to `"todo.add"`. No other line in that file changes.
12. Add the error-branch test cases enumerated below (new test function(s) in `session_state_test.go`, colocated near the other `TestServeStdioTodo*` handler tests).
13. Run `cd agents-plugin-tool && go build ./... && go test ./... -count=1`.
14. Run the wsflow package tests: `python3 -m unittest discover agents-plugin-wsflow/tests` — this exercises `test_runtime_contract_matches_agentless_capabilities`, which will fail if `agents-plugin-wsflow/runtime.json`'s `"tools"` set doesn't exactly match the live registered tool set (i.e., catches a missed `todo.add`/stale `todo.append` entry).
15. Run the grep sweep: `grep -rn "todo\.append\|todo\.insert_before\|todo\.insert_after" agents-plugin-tool/internal/mcp agents-plugin/runtime.json agents-plugin-wsflow/runtime.json ai-docs/spec/mcp-tools.md agents-plugin/rsrc agents-plugin-wsflow/rsrc` — expect no matches (historical `.done`/`.plans`/`CHANGELOG.md`/the epic ticket file are intentionally excluded, see Out of Scope).

### Required error-branch test cases

Add these as new test(s) exercising `handleTodoAdd` through the tool-dispatch surface (`callToolWithKey(..., "todo.add", ...)`), each asserting the exact frozen error string:

1. **`position` not in enum** — call `todo.add` with `position: "middle"` (or any non-`end`/`before`/`after` string) and valid `key`/`title`; assert the response contains `"todo.add: position must be one of end, before, after"`.
2. **`ref_key` missing for `position: "before"`** — call with `position: "before"` and no `ref_key` key at all; assert `"todo.add: ref_key is required when position is before"`. Repeat for `position: "after"` asserting `"...is required when position is after"`.
3. **`ref_key` present but empty string for `position: "before"`/`"after"`** — call with `ref_key: ""`; assert the same "required" message as case 2 (empty counts as missing, per the ticket's "missing/empty" wording).
4. **`ref_key` supplied for `position: "end"`** (including the implicit default, i.e. `position` omitted entirely) — call with a non-empty `ref_key` and either `position: "end"` or `position` omitted; assert `"todo.add: ref_key must be omitted when position is end"`.
5. **Byte-identical mutation for all three placements** — extend or add to `TestServeStdioTodoInstructionReadSurface`-style coverage: build a list via `todo.add(position: end)` (equivalent to old `todo.append`), `todo.add(position: before, ref_key: X)` (equivalent to old `todo.insert_before`), and `todo.add(position: after, ref_key: X)` (equivalent to old `todo.insert_after`); assert the resulting list order/keys via `readState(key).Todos` match exactly what the pre-merge `todoAppend`/`todoInsert` core-function tests already prove (`TestTodoInsertAndCheck#L546-576`) — i.e. the same before/after insertion positions relative to `ref_key`, not just "no error". This is the direct backstop for the ticket's "byte-identical mutation" acceptance line, since the mutation cores (`todoAppend`/`todoInsert`) are reused unchanged and only the dispatch wiring is new.
6. **Unified confirmation string** — for each of the three positions above, assert the response text is exactly `"todo added: <key>\n"` (not `"todo appended:"`/`"todo inserted:"`), confirming the old per-position strings are gone from the new dispatch path.
7. **Reused verbatim error messages still fire through the new tool** — duplicate `key` (`"todo key %q already exists"`), invalid `key` format (`"invalid character"` / `"leading or trailing whitespace"`), unknown `ref_key` (`"ref_key %q not found"`), and bad `instruction` type (`"instruction must be a string or null"`) — one assertion each via `todo.add`, confirming `handleTodoAdd`'s error wrapping (`fmt.Errorf("%s: %w", tool, err)`) still surfaces the core functions' unchanged messages under the new tool name.

## Verification Plan

- `cd agents-plugin-tool && go build ./... && go test ./... -count=1` — must be green. `-count=1` is required because the wsflow rsrc/manifest regen steps in Implementation Plan step 7 are env-gated test bodies that Go's test cache can otherwise report as a stale `ok` without re-running.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — must pass, in particular `test_runtime_contract_matches_agentless_capabilities` (exact tool-set match against `agents-plugin-wsflow/runtime.json`).
- Grep sweep (Implementation Plan step 15) clean.
- Manual/test confirmation that `todo.add` reproduces each old tool's mutation byte-for-byte (error-branch test case 5 above is the concrete backstop for this).
- Re-read the three edited `agents-plugin/rsrc/**` files after edit and diff against their `agents-plugin-wsflow/rsrc/` counterparts post-regen to confirm byte-identical mirroring.

## Escalations

- None.

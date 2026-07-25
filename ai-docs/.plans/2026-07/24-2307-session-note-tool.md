# Plan: lead-goal-fan-out-step — Phase 1: session.note MCP tool

## Relevant Ticket Contract

- `session.note(session_key, child_session_key, text)` — the lead annotates a
  child key with a free-form one-line note; `session_key` authorizes (must
  resolve to a lead-scope key), `child_session_key` is the target.
- Note text is an **additive field on the child's own per-session record**
  (`<cache-root>/keys/<child_session_key>.json`), keyed by `child_session_key`
  — not the lead's record — so `session.children` reads each child's note
  directly. No new schema/store; durable across restart.
- Surfaced in `session.children` output alongside `key/scope/parent/depth/live/root`.
- Lead-only gating is **free**: `session.*` is already rejected for
  `delegate`/`leaf` at the keyed-call handler (`roleAllowsTool`); no new gate
  logic needed.
- The field is convention-agnostic free-form text; the `<stem>: <state>` board
  convention is a Phase 2 skill-body concern, not a tool schema field.
- Tests required (from ticket): lead writes/updates a child note; note appears
  in `session.children`; delegate/leaf key rejected; note survives a re-read
  (restart/compaction persistence).
- Spec impact for this phase: `ai-docs/spec/mcp-tools.md` add a `session.note`
  entry beside `#260619-session-key-lineage-children`; `ai-docs/spec/plugin-runtime.md`
  note the new tool on the advertised capability surface.

## Out of Scope

- Phase 2 (`lead-goal-fan-out-step` skill, transclusion branch, `<stem>: <state>`
  board convention) and Phase 3 (wsflow probe of `ferrule`/`playbook.render`
  mint/transclusion-under-wsflow) are not touched here.
- No child-self-note path (deferred per ticket).
- No verification that `child_session_key` is actually a descendant of
  `session_key` (lineage ownership check) — the ticket sketch does not request
  it and `session.children` itself has no equivalent check; `session.note`
  mirrors that trust model (holding a valid session_key is the authorization).
- No new capability tier, no new `session.*` sub-namespace beyond the tool
  itself.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/session_auth.go#L44-L60` — `sessionRecord`
  is the on-disk JSON shape (`<cache-root>/keys/<key>.json`); `Overrides`,
  `Agenda`, `Todos` are all additive `omitempty` fields on this same struct.
  Add `Note string \`json:"note,omitempty"\`` here, after `Todos`.
- `agents-plugin-tool/internal/mcp/session_auth.go#L26-L34` — `sessionChild`
  (in-memory enumerated descendant) needs a `note string` field.
- `agents-plugin-tool/internal/mcp/session_auth.go#L187-L265` — `children()`
  builds each `sessionChild` from its `sessionRecord`; add `note: record.Note`
  in the child-construction block (~L242-L249).
- `agents-plugin-tool/internal/mcp/session_state.go#L718-L738` — `mutateRecord`
  is the shared atomic read-modify-write primitive (temp-write+rename) already
  used by `setAgenda`/`clearAgenda`/`enterMode`. Reuse it for the new setter
  instead of hand-rolling another RMW path (mirrors `setAgenda` at L753-L761).
- `agents-plugin-tool/internal/mcp/session_state.go#L834-L860` — `sessionStateKey`
  and `stringArg` are the existing arg-parsing helpers; reuse for the new
  handler's `session_key`/`child_session_key` extraction. `text` needs custom
  parsing (below) because an **empty string is a valid, meaningful value**
  (ticket's "empty-omit discipline" — omitempty in JSON is what makes an
  empty-text write disappear from the record, not a separate "clear" verb).
- `agents-plugin-tool/internal/mcp/server.go#L1782-L1837` — `sessionChildOutput`
  struct + `handleSessionChildren`: add `Note string \`json:"note,omitempty"\``
  to the struct (after `Root`, L1788) and `Note: child.note` in the output
  loop (~L1819-L1826).
- `agents-plugin-tool/internal/mcp/server.go#L1852-L1873` — `formatSessionChildren`:
  add a conditional `note:` line under each child when non-empty (mirror the
  existing conditional `live:` field print at L1863-L1869), so the compact
  text form also surfaces it.
- `agents-plugin-tool/internal/mcp/server.go#L541-L546` — dispatch switch;
  `session.children` sits at L545-546. Add `case "session.note": return
  s.handleSessionNote(req.ID, params.Arguments)` immediately after.
- `agents-plugin-tool/internal/mcp/server.go#L3200-L3213` — `session.children`
  tool schema in `tools()`. Insert the `session.note` schema entry immediately
  after (properties: `session_key`, `child_session_key`, `text`, all
  `stringProperty`; all three `required`; no `format` — this is a write/confirm
  tool like `agenda.set`, which also has no `format` property).
- `agents-plugin-tool/internal/mcp/server.go#L4438-L4452` — `roleAllowsTool`:
  `session.` prefix already blocked for `roleDelegate` (L4443-4445) and
  `roleLeaf` (L4448). **This is the entire lead-only gate; no code change
  needed here** — confirms the ticket's "gating is correct and free" claim.
- `agents-plugin-tool/internal/mcp/server.go#L4559-L4577` — `noAgentHiddenTool`:
  `session.note` is **not** added to any hidden case here, so it is NOT hidden
  under `WS_MCP_NO_AGENT=1` (wsflow product mode) — it ships to wsflow
  automatically, matching the ticket's "Ship to both ws and wsflow (option B)"
  decision.
- **Risk signal (must fix in this phase, not deferred to Phase 3):**
  `agents-plugin-tool/cmd/ws-mcp/main_test.go#L48-L101`
  (`TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`) and
  `#L149-L183` (`TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`)
  both assert an **exact** match between the live served tool set and the
  `tools` map keys of `agents-plugin/runtime.json` / `agents-plugin-wsflow/runtime.json`
  respectively (`agents-plugin-wsflow/runtime.json` even declares
  `"runtime_capabilities": {"match": "exact"}` at L8-10). Since `session.note`
  is not `noAgentHiddenTool`-gated, adding it to `tools()` makes it appear in
  **both** live surfaces immediately. If only `agents-plugin/runtime.json` gets
  the new entry, `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`
  breaks on this phase's own commit — this is not something Phase 3 can safely
  absorb later, because CI in this phase would already be red. Both manifest
  files must get a `"session.note": ">=0.36.1-dev <0.37.0"` line (same
  literal range string as every adjacent entry in each file) in this phase.
  This mechanical manifest addition is distinct from Phase 3's actual scope
  (probing whether `ferrule`/`playbook.render` mint/transclusion *behave*
  correctly under the wsflow reduced runtime) — no behavioral wsflow work is
  pulled forward, only the two manifest lines needed to keep the exact-match
  tests green.
- `agents-plugin-tool/internal/mcp/session_auth_test.go#L776-L802` —
  `writeSessionRecordForTest` / `childrenByKey` test helpers; `#L446-L555`
  (`TestCapabilityScopedKeyGatesTools`) is the reference pattern for
  lead/delegate/leaf gate assertions (`callToolOnce` + `assertGateError`
  checking JSON-RPC `-32601`); `#L956-L995`
  (`TestSessionChildrenJSONOutputStableFields`) is the reference pattern for
  asserting new `session.children` JSON fields land correctly;
  `#L609-L644` (`TestSessionKeySurvivesFreshServerInstance`) is the reference
  pattern for restart/persistence (fresh `sessionStore`/`NewServer` over the
  same `WS_CACHE_HOME`).
- `ai-docs/spec/mcp-tools.md#L173-L207` (`#260619-session-key-lineage-children`)
  — existing `session.children` spec prose to extend with the note field;
  `#L208-L243` (`#260625-session-state-tools`) — sibling pattern for how
  additive per-session-record fields are documented (agenda/todos section
  shape to mirror for the new note paragraph).

## Implementation Plan

1. `agents-plugin-tool/internal/mcp/session_auth.go`: add `Note string
   \`json:"note,omitempty"\`` to `sessionRecord` (after `Todos`, ~L59); add
   `note string` to `sessionChild` (~L34); populate `note: record.Note` in the
   `children()` construction block (~L242-249); add a `setNote` method (near
   `mint`/`children`, e.g. after `children()` at L265) that calls
   `s.mutateRecord(targetKey, func(r *sessionRecord) error { r.Note = text;
   return nil })` — `targetKey` is the **child** key, not the caller's own key.
2. `agents-plugin-tool/internal/mcp/server.go`: add `Note string
   \`json:"note,omitempty"\`` to `sessionChildOutput` (~L1788) and `Note:
   child.note` in `handleSessionChildren`'s output loop (~L1819-1826); add a
   conditional `note:` line to `formatSessionChildren` (~L1868-1870, guarded
   on `child.Note != ""`).
3. `agents-plugin-tool/internal/mcp/server.go`: add `handleSessionNote` near
   `handleSessionChildren` (after ~L1837). Parse `session_key` via
   `sessionStateKey("session.note", args)` (required, used only as the
   already-gated authorization identity — no further lookup needed); parse
   `child_session_key` via `stringArg("session.note", "child_session_key",
   args)` (required, non-empty); parse `text` manually — `args["text"]` must
   be present and a string, but **may be empty** (empty text is a legitimate
   "clear the note" write, made possible purely by the `omitempty` JSON tag,
   not a separate clear verb, per ticket's explicit sketch). Call
   `s.sessions.setNote(childKey, text)`; on error (child key not found) return
   `toolTextResponse(id, "", err)`. Return `"note cleared: <child>\n"` when
   `text == ""`, else `"note set: <child>\n"`.
4. `agents-plugin-tool/internal/mcp/server.go`: add `case "session.note":
   return s.handleSessionNote(req.ID, params.Arguments)` after the
   `"session.children"` case (~L546).
5. `agents-plugin-tool/internal/mcp/server.go`: add the `session.note` schema
   to `tools()` immediately after the `session.children` schema (~L3213):
   `session_key`, `child_session_key`, `text` — all `stringProperty`, all
   three `required`, no `format` property (write/confirm tool, matches
   `agenda.set`'s shape).
6. `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`: add
   `"session.note": ">=0.36.1-dev <0.37.0"` to each file's `tools` map
   (copy the literal range string from the adjacent `"session.children"`
   line in that same file) — required to keep
   `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` and
   `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface` passing (see
   Codebase Findings risk signal).
7. Tests in `agents-plugin-tool/internal/mcp/session_auth_test.go` (mirror the
   cited reference patterns):
   - `TestSessionNoteSetAndUpdateAppearsInChildren` — mint a lead key and a
     delegate child (`writeSessionRecordForTest` or `mint`), call
     `session.note` twice (write then update) with the lead's `session_key`
     targeting the child's key, then call `session.children` and assert the
     returned/JSON child carries the latest `note` text.
   - `TestSessionNoteRejectedForDelegateAndLeaf` — mirror
     `TestCapabilityScopedKeyGatesTools`'s `assertGateError` pattern: mint
     delegate and leaf keys, call `session.note` with each as `session_key`,
     assert JSON-RPC `-32601` (blocked by the existing `session.` prefix
     gate in `roleAllowsTool`).
   - `TestSessionNoteEmptyTextClearsNote` — set a note, then call again with
     `text: ""`, assert the child's `note` is absent from
     `session.children` JSON output (omitempty) or empty in text output.
   - `TestSessionNoteSurvivesFreshServerInstance` — mirror
     `TestSessionKeySurvivesFreshServerInstance`: write a note, construct a
     **new** `sessionStore`/`Server` over the same `WS_CACHE_HOME`, confirm
     `session.children` (or a direct `readState`/`children()` call) still
     reports the note — proves file-backed persistence, not just in-process
     state.
8. `ai-docs/spec/mcp-tools.md`: extend the `#260619-session-key-lineage-children`
   section (~L190-207) with a `session.note` paragraph: signature, that it
   writes onto the child's own record (not the caller's), the lead-only gate
   reuse, and that it is surfaced in `session.children` output.
9. `ai-docs/spec/plugin-runtime.md`: note the new tool on the advertised
   capability surface per `#260506-runtime-capabilities-single-probe` and its
   record-file persistence per `#260626-post-compaction-session-restoration`
   (both referenced directly in the ticket's Spec Impact section).

## Verification Plan

- `cd agents-plugin-tool && go build ./...`
- `cd agents-plugin-tool && go test ./internal/mcp/...` (new
  `TestSessionNote*` cases plus existing `session.children`/gate tests stay
  green)
- `cd agents-plugin-tool && go test ./cmd/ws-mcp/...` — specifically confirms
  `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` and
  `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface` stay green
  after the two `runtime.json` edits.
- Manual smoke (optional): `ws-mcp serve --stdio` round-trip of `ferrule` →
  `session.note` → `session.children` to visually confirm the note surfaces
  in the compact text formatter, not just JSON.

## Escalations

- None.

# Implementer Brief — `ws.workflow_manual` tool + restore rendering (Phase 3a)

> You (the implementer) do NOT have access to the originating ticket and must
> not read it. Every binding decision is restated below. If something here
> conflicts with code you read, that is a deviation — escalate per the Deviation
> Protocol; do not reinterpret the intent.

## Intent

Add a new MCP tool `ws.workflow_manual(session_key?: string)` to the ws-mcp
server. It renders the existing `lead-workflow-manual` rsrc playbook (via the
existing variable-substitution pipeline) and then, in the tool handler ONLY,
branches on whether a `session_key` was supplied and resolves to a stored
session record:

- **fresh** (no key): manual + always-shown per-root ferrule rule + a gated
  self-bootstrap line.
- **continue** (key resolves): manual + per-root rule, self-bootstrap line
  stripped, plus an appended "Session State" section rendered server-side from
  the session record (agenda blobs as remind + todo list in summary mode).
- **fail-loud** (key given but no record): manual + an explicit
  "no restorable state for this key" notice; never mint a key.

Also: make the `git.commit` MCP formatter re-inject the current todo summary
after a commit, register the new tool in both `runtime.json` files, and add a
dedicated mode-gating region marker around the self-bootstrap line in the rsrc
(regenerating the manifest + wsflow mirror).

The rsrc remains the single source of truth for all prompt text. The handler
owns ONLY mode branching and the server-side Session State render. The handler
must never carry manual prose.

## Scope Boundary

In scope (code + data only):

1. New tool dispatch case + `tools()` schema entry in
   `agents-plugin-tool/internal/mcp/server.go`.
2. New handler function (recommended file:
   `agents-plugin-tool/internal/mcp/workflow_manual.go`, or add to
   `session_state.go` — your call; keep it in `internal/mcp`).
3. A dedicated mode-gating region marker in
   `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md` wrapping
   ONLY the self-bootstrap line; handler-side strip logic for continue mode.
4. `git.commit` formatter change in `internal/mcp` (`formatGitCommit`) to
   re-inject the todo summary after commit.
5. Register `ws.workflow_manual` in `agents-plugin/runtime.json` AND
   `agents-plugin-wsflow/runtime.json` (current-line fence
   `>=0.30.8-dev <0.31.0`).
6. Regenerate `manifest.json` + the wsflow rsrc mirror.
7. Tests (see Integration Test Instructions).

Out of scope: see "Out of scope" section. Notably, do NOT touch spec/doc prose
or any skill files.

## Caller-Visible Contract

New tool: `ws.workflow_manual(session_key?: string)`.

- `session_key` is OPTIONAL (not in the schema's `required` list). This is the
  one ws tool whose root resolution does NOT require a key — an absent key is
  the fresh-bootstrap signal, not an error. Do NOT route this tool through
  `resolveToolRoot` and do NOT emit `mandatory_session_key` guidance. (The
  manual is host-neutral text + session-record state; it needs no repository
  root.)
- Returns MCP text (use `toolTextResponse`). Three response shapes:

  **Fresh** (`session_key` empty/omitted):
  - The full rendered manual body, including the always-shown per-root ferrule
    rule AND the self-bootstrap line (the gated region is KEPT).
  - No "Session State" section.

  **Continue** (`session_key` present AND `s.sessions.readState(key)` returns
  ok=true):
  - The rendered manual body with the per-root ferrule rule kept but the
    gated self-bootstrap region STRIPPED.
  - Appended, after the manual body, a `## Session State` section containing:
    - agenda blobs (remind) — each stored agenda blob keyed by name, value
      pretty-printed as JSON.
    - todo list in SUMMARY mode via `renderTodos(record.Todos, false)`.

  **Fail-loud** (`session_key` present but `readState` ok=false):
  - The rendered manual body (gated region treatment: render it as fresh —
    keep the self-bootstrap line, since the caller has no usable key and may
    need to mint one). Then append an explicit notice line, e.g.
    `## Session State\n(no restorable state for session key "<key>"; this key
    resolves to no stored session record — do not assume prior agenda/todo.)`.
  - NEVER call `s.sessions.mint(...)` or any key-minting path. Fail loud =
    informative text, not an error response and not a new key.

- Backward compatibility: the existing
  `playbook.print(name: "lead-workflow-manual")` path MUST keep working and
  return the ungated fresh form (self-bootstrap line present). The new gating
  marker must therefore be invisible to / preserved-as-content by the normal
  print path. See Constraints for how the marker must behave under
  `playbook.print`.

## Contract Instructions

- Render the manual through the SAME pipeline `playbook.print` uses. Concretely,
  call the existing `printPlaybook(s, rsrcRoot, "lead-workflow-manual",
  callerContext, configOpts, workflowLang, overrideLookup)` (signature in
  `playbook_tools.go`) — reuse `resolveRsrcRoot("")`, `buildOverrideLookup`, and
  the workflow.lang resolver exactly as the `playbook.print` dispatch case does
  (server.go ~979-999). Do not re-implement substitution.
- After you obtain the rendered body string, perform mode branching on the
  body STRING in the handler:
  - The gating marker (defined in the Survey Plan) wraps the self-bootstrap
    line. In FRESH and FAIL-LOUD modes, strip only the marker comment lines and
    keep the inner content. In CONTINUE mode, strip the marker comment lines AND
    the inner content between them.
  - Provide one small pure helper (e.g.
    `stripModeGatedRegion(body string, keepContent bool) string`) so the strip
    logic is table-testable without a Server.
- The Session State render reuses the Phase 1 `renderTodos` (summary mode) and
  reads agenda from `record.Agenda` (a `map[string]json.RawMessage` on
  `sessionRecord`). Sort agenda keys for deterministic output.
- `git.commit` re-injection: modify `formatGitCommit` so that after the existing
  commit summary it appends the current session's todo summary. `formatGitCommit`
  currently takes only `wsgit.CommitResult` and has no session access — you must
  thread the todo summary in. The cleanest path: have the `git.commit` dispatch
  case (server.go ~687-707) resolve the caller's session record (it already has
  `params.Arguments["session_key"]` available via the resolved root path; read
  the key directly from `params.Arguments`), render `renderTodos(rec.Todos,
  false)` when a record exists with a non-empty todo list, and pass that string
  into `formatGitCommit` (add a parameter) OR append it at the dispatch site
  after `formatGitCommit` returns. Keep ALL of this in `internal/mcp`; do NOT
  push todo logic into `internal/wsgit`. When the session has no todos (or no
  resolvable key), append nothing — the commit output is unchanged.

## Integration Test Instructions

Test file to EXTEND: `agents-plugin-tool/internal/mcp/session_state_test.go`
(Phase 1 integration tests live here; reuse its helpers `useLeadProfile`,
`callLogin` + `parseLoginResponse`, `callToolWithKey`, `initGit`, and the
`WS_CACHE_HOME` tempdir pattern). Pure-logic strip-helper tests may go in the
same file or a sibling `_test.go`.

Run command (from `agents-plugin-tool/`):
```
go test ./internal/mcp/ ./internal/wsrsrc/
```
For the full Phase 3a gate also run the regen drift guards (see Verification
Contract). Read full output; claim pass only after reading it.

Pass criteria (each a distinct test or sub-assertion):

1. **Fresh mode**: call `ws.workflow_manual` with NO `session_key`. Response
   contains the self-bootstrap sentence (assert on a stable fragment of the
   self-bootstrap line, e.g. `"mint your"` / the chosen wording — match what you
   put in the rsrc) AND the per-root rule fragment (e.g.
   `"once per working root"`). Response does NOT contain `"Session State"`.
   Note: `callToolWithKey` always injects a key; for the no-key case issue the
   tools/call directly without `session_key` (adapt the `callToolWithKey`
   payload inline, or add a `callToolNoKey` helper).
2. **Continue mode**: login → `ws.enter.implement(need_review:true,
   need_doc:false)` to populate agenda+todos → call `ws.workflow_manual` with
   the key. Assert: self-bootstrap fragment ABSENT; per-root rule fragment
   PRESENT; `"Session State"` present; todo summary content present (e.g.
   `"Route"`, `"Prep"`); agenda content present (the `implement` blob).
3. **Unknown key**: call `ws.workflow_manual` with a syntactically-valid but
   never-minted key (e.g. `"no-such-key-here"`). Assert: response contains the
   no-restorable-state notice; assert NO new key file was created (the keys dir
   under `WS_CACHE_HOME` has no file for that key) — i.e. minting did not occur.
4. **git.commit re-injection**: login → `ws.enter.implement` → stage a file and
   `git.commit` with the key → assert the commit response text contains a todo
   summary fragment (e.g. `"Route"` or a `- [` marker). Also assert that a
   commit on a session with NO todos appends nothing extra (commit output
   unchanged shape).
5. **Drift guards green** (run-command level, see Verification Contract):
   `TestShippedManifestUpToDate` and `TestWsflowRsrcMirrorUpToDate` pass after
   regen.

## Implementation Strategy Decisions

- **The handler owns mode branching only; rsrc owns all prose.** The handler
  string-manipulates the rendered body to strip/keep the gated region and to
  append Session State; it never embeds manual sentences. (Binding decision 1.)
- **Dedicated mode-gating marker, NOT the override marker.** The override marker
  (`<!-- ws:override:... -->`) carries config.prompt user-customization lookup
  semantics and is orthogonal to fresh-vs-continue visibility. Use a NEW,
  distinct HTML-comment marker pair (token defined in the Survey Plan) so the
  two concerns never collide. The per-root ferrule rule stays OUTSIDE the marker
  (always shown). (Binding decision 3.)
- **No template-conditional engine.** Do not add a general show/hide conditional
  feature to the rsrc render engine or to `wsrsrc`. Mode logic is a string strip
  in the tool handler. (Explicitly rejected alternative — see below.)
- **`git.commit` re-injection lives in `internal/mcp`.** Formatting stays in
  `internal/mcp` per the mcp-runtime mental model; `internal/wsgit` is not
  touched. (Binding decision 4.)
- **Both runtime.json files, current-line fence.** `ws.workflow_manual` is a
  lead session tool (not mercenary/exec), so it belongs in the wsflow contract
  too. The wsflow runtime.json declares
  `"runtime_capabilities": {"match": "exact"}` — omitting the tool there will
  make wsflow startup reject the binary. Use the fence `>=0.30.8-dev <0.31.0`
  matching Phase 1. There is no separate fast-path subset; `runtime.capabilities`
  derives lead tool names from `tools()`. (Binding decisions 5 + plugin-runtime
  wsflow-surface-boundary rule.)
- **Optional session_key, no root resolution.** This tool intentionally does not
  require a key and does not resolve a repository root; an absent key is the
  fresh signal. (Binding decision 2 + Caller-Visible Contract.)

## Rejected Alternatives

- **General template conditional in the render engine** (rejected by ticket
  design): adds a conditional surface across ALL playbooks for a single tool's
  need. Mode logic lives in the dedicated handler instead.
- **Reusing the `<!-- ws:override:... -->` marker for gating** (rejected):
  override markers mean "user-customizable text via config.prompt", not
  "conditionally hidden". Conflating them would make the self-bootstrap line
  user-overridable and tangle two engines. Use a fresh marker token.
- **Putting todo re-injection in `internal/wsgit`** (rejected): violates the
  formatting-stays-in-`internal/mcp` invariant.
- **Minting a key in fail-loud mode** (rejected): silently orphans prior
  agenda/todo and defeats the whole restoration purpose. Fail loud = text notice
  only.

## Approach

1. Add the gating marker around the self-bootstrap line in the rsrc; confirm the
   per-root rule is outside it. (Survey Plan §marker.)
2. Write the pure strip helper + its table tests first (pure-logic → TDD per the
   impl-playbook test strategy).
3. Add the handler (render via `printPlaybook`, branch, strip, append Session
   State). Add the dispatch case + `tools()` schema entry.
4. Wire `git.commit` todo re-injection in the dispatch case / formatter.
5. Register the tool in both runtime.json files.
6. Regenerate manifest + wsflow mirror.
7. Write the integration tests; run the full gate.

## Constraints

- The gating marker MUST be benign under `playbook.print`: the existing
  `playbook.print(name: "lead-workflow-manual")` path renders the manual WITHOUT
  knowing the marker. Choose a marker that is an HTML comment (so it is inert
  Markdown) and that the `selectProductModeBlocks` / `applyOverrideMarkers`
  passes do NOT consume or choke on. Verify by reading
  `renderProductModePlaybookBody` / `applyOverrideMarkers` (in
  `playbook_tools.go`): both only react to their own specific marker tokens
  (`ws:full-only`, `ws:wsflow-only`, `ws:mercenary-on`, `ws:override:`,
  `ws:/override:`). A new distinct token (e.g. `ws:fresh-only:start` /
  `ws:fresh-only:end`) passes through both untouched and surfaces verbatim in
  `playbook.print` output. That verbatim-comment behavior is acceptable for the
  backward-compat path (HTML comments are invisible when the Markdown renders)
  AND the new tool strips the marker lines in all three modes.
  — If, on reading, you find `selectProductModeBlocks` would swallow your chosen
  token, pick a different token; do NOT add the new token to
  `selectProductModeBlocks`'s switch (that would make product-mode gating, not
  fresh/continue gating).
- The handler must keep ALL manual prose in the rsrc. Only the Session State
  heading/format strings and the no-restorable-state notice are handler-owned
  (these are state-render scaffolding, not manual procedure prose).
- Do not change `ws.ferrule` / `bootstrapToolName` obscurity guarantees.
  `ws.workflow_manual` is a normal, descriptively-named tool; it does not mint
  keys and is not lead-only-gated (it must be callable with no key, and a
  no-key call cannot be gated by the keyed capability gate). Confirm
  `isLeadOnlyTool` does NOT match `ws.workflow_manual` (it matches only
  `ws.ferrule` and `ws.lead.*`) — leave it unchanged.
- Honor the impl-playbook Verify discipline: read full test/build output before
  claiming pass; diagnose blame before fixing; resolve new warnings.

## Out of scope

- **Phase 2** (`ws.enter.*` integration into skills) — not this phase.
- **Phase 3b** (skill restructure: add `lead-revive`, remove
  `lead-load-workflow-manual`, repoint the 6 manual-self-load skills) — depends
  on 3a; NOT this phase. Do not edit any `SKILL.md`.
- **Spec/doc prose** — `ai-docs/spec/mcp-tools.md`,
  `ai-docs/spec/plugin-runtime.md`, `ai-docs/ref/ws-mcp.md` are handled by the
  lead's Doc pre-pass AFTER your Edit, NOT by you. Do not edit them.
- The plugin patch version bump is a dev-merge step, not a Phase 3a deliverable.
  Do NOT run `bump-ws-version.sh` and do NOT edit version edition points.

## Details

- `sessionRecord` (in `session_auth.go`): `Agenda map[string]json.RawMessage
  \`json:"agenda,omitempty"\`` and `Todos []todoItem \`json:"todos,omitempty"\``.
- Read state with `s.sessions.readState(sessionKey) (sessionRecord, bool)`
  (in `session_state.go`) — holds the store mutex, safe for concurrent reads.
- Todo summary render: `renderTodos(record.Todos, false)` (false = summary).
- Render pipeline entry: `printPlaybook(s, rsrcRoot, name, callerContext,
  configOpts, workflowLang, overrideLookup)` returns `(body, recommendedTier,
  err)`; ignore `recommendedTier` (manual is `kind: print`, declares no tier).
- Harness/namespace come from inside the pipeline; you pass
  `callerContext = nil` (the manual declares only `WorkflowLang`, which the
  pipeline injects from the workflow.lang resolver) — mirror the
  `playbook.print` dispatch case for `buildOverrideLookup` + workflow.lang.
- Marker-aware render passes to read before choosing the marker token:
  `selectProductModeBlocks` and `applyOverrideMarkers` in `playbook_tools.go`.
- Test helpers: `useLeadProfile(t)` (server_test.go:574), `callLogin` +
  `parseLoginResponse` (session_auth_test.go), `callToolWithKey` /
  `responseLinesByID` / `toolText` / `initGit` (session_state_test.go &
  server_test.go).

## Verification Contract

Per the impl-playbook (`§Verify`, `§Test Strategy`, `§Test Failure Diagnosis`):

1. `cd agents-plugin-tool && go build ./...` — must compile clean.
2. `go test ./internal/mcp/` — all new + existing MCP tests pass. Read full
   output.
3. After the rsrc marker edit, regenerate then verify drift guards:
   - `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run
     TestRegenerateShippedManifest`
   - `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run
     TestRegenerateWsflowRsrcMirror`
   - then `go test ./internal/wsrsrc` — `TestShippedManifestUpToDate` and
     `TestWsflowRsrcMirrorUpToDate` must be green. (Both rsrc trees must stay
     byte-identical.)
4. Runtime contract: a runtime.capabilities / package contract test may assert
   the lead tool surface against `runtime.json`. After registering the tool in
   both runtime.json files, run `go test ./internal/mcp/ ./internal/wsrsrc/` and
   any package contract test under `agents-plugin*/tests` if your change touches
   the contract surface; the wsflow `match: exact` contract will fail if the
   tool is missing from `agents-plugin-wsflow/runtime.json`.
5. Pure-logic test strategy: write the strip-helper edge-case tests FIRST
   (TDD); integration/IO handler tests after (implement-first). Diagnose blame
   before any fix; do not patch a test to mask a real handler bug.

Claim "pass" only after reading the full output of each command.

## References

- `[Must]` `agents-plugin-tool/internal/mcp/playbook_tools.go` — render pipeline:
  `printPlaybook`, `renderPlaybookBody`, `substitutePlaybookVars`,
  `applyOverrideMarkers`, `selectProductModeBlocks`. Read before choosing the
  marker token and before wiring the render call.
- `[Must]` `agents-plugin-tool/internal/mcp/server.go` — dispatch (`callTool`,
  the `case` block ~360-394 + the `playbook.print` case ~979-999 + `git.commit`
  case ~687-707), `tools()` schema (~2259+, playbook entries ~2947-2972),
  `formatGitCommit` (~1652), `isLeadOnlyTool` (~59), `LeadToolNames` (~3173).
- `[Must]` `agents-plugin-tool/internal/mcp/session_state.go` — `renderTodos`,
  `readState`, `sessionRecord` usage, handler patterns.
- `[Must]` `agents-plugin-tool/internal/mcp/session_auth.go` — `sessionRecord`
  struct (`Agenda`, `Todos` fields).
- `[Must]` `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md` —
  the "### Session setup" block (~lines 50-64) you will split with the marker.
- `[Must]` `agents-plugin/runtime.json` + `agents-plugin-wsflow/runtime.json` —
  registration sites (after `path.generate` / before `playbook.render`, or
  anywhere in the `tools` object; alphabetical not required).
- `[Must]` `agents-plugin-tool/internal/mcp/session_state_test.go` — extend for
  integration tests; reuse helpers.
- `[Must]` impl-playbook (via `ws/infra.read(name: "impl-playbook")`) — Verify
  discipline, test-strategy split, deviation protocol.
- `[Must]` migration anchor `260605-research-ws-native-subagent-pivot` (idea
  ticket) — Phase 3a touches plugin architecture + adapter boundaries. Binding
  constraints copied here: rsrc is the single prompt source of truth (handler
  carries no prose); MCP `tools/list` filtering is advisory, capability
  enforcement is the server-side keyed gate (this tool is NOT lead-only and must
  be callable keyless); host-neutral text first.
- `[Maybe]` `ai-docs/mental-model/mcp-runtime.md` — "Add an MCP tool" recipe,
  formatting-stays-in-`internal/mcp`, LeadToolNames/runtime.capabilities notes.
- `[Maybe]` `ai-docs/mental-model/plugin-runtime.md` — wsflow surface boundary
  (new tools go in both contracts unless mercenary/exec), rsrc regen recipe.

# Survey Plan — Phase 3a `ws.workflow_manual` + restore rendering

Exact source insertion points, verified against the tree on branch
`implement/260625-ws-session-state-machine-p1`. Line numbers are approximate
(read the named symbol; do not trust the line alone). Where a strategy could be
unviable, `[escalate-to-lead]` is noted.

## 1. New tool dispatch case — `server.go` `callTool`

- File: `agents-plugin-tool/internal/mcp/server.go`.
- Insertion: in the big `switch params.Name` inside `callTool`. The session-state
  cases are at ~367-394 (`ws.agenda.*`, `ws.enter.*`, `ws.todo.*`). Add
  `case "ws.workflow_manual":` adjacent to them (e.g. right after the
  `ws.todo.reorder` case at ~394, before `case bootstrapToolName:` at ~395):
  ```go
  case "ws.workflow_manual":
      return s.handleWorkflowManual(req.ID, params.Arguments)
  ```
- Note: this case must NOT call `s.resolveToolRoot` (no root required; absent key
  is the fresh signal). Compare to the session-state cases which also skip
  `resolveToolRoot`.

## 2. New tool schema — `server.go` `tools()`

- `tools()` begins ~2259. Add a new schema map for `ws.workflow_manual` near the
  `playbook.print` / `playbook.render` entries (~2947-2972) for locality.
- Schema shape (session_key OPTIONAL — not in `required`):
  ```go
  {
      "name":        "ws.workflow_manual",
      "description": namespaceText("Render the ws workflow primitives manual. With no session_key: fresh mode (includes the session-bootstrap line). With a session_key that resolves: continue mode (omits the bootstrap line, appends restored agenda + todo Session State). With a session_key that does not resolve: fail-loud notice, no key minted."),
      "inputSchema": map[string]any{
          "type": "object",
          "properties": map[string]any{
              "session_key": stringProperty("Optional ws session key. Omit for fresh bootstrap; provide a known key to restore agenda/todo Session State."),
          },
          // no "required"
      },
  },
  ```
- `LeadToolNames()` (~3173) derives from `tools()` automatically; no separate
  edit. Confirm `permanentlyHiddenTool` / `noAgentHiddenTool` do NOT match
  `ws.workflow_manual` (they match mercenary/exec/config-prefixed names) so it
  surfaces in both full and wsflow capability lists.

## 3. Handler function

- New file (recommended): `agents-plugin-tool/internal/mcp/workflow_manual.go`,
  package `mcp`. (Or append to `session_state.go`.)
- Handler `func (s *Server) handleWorkflowManual(id json.RawMessage, args map[string]any) response`:
  1. `key, _ := args["session_key"].(string); key = strings.TrimSpace(key)`.
  2. Render body via the SAME path as the `playbook.print` dispatch case
     (server.go ~979-999):
     - `rsrcRoot, err := resolveRsrcRoot("")`
     - `overrideLookup := buildOverrideLookup(s, key)` (nil when key empty — fine)
     - workflow.lang: build `sessionConfigAdapter{s: s.sessions}` + resolver,
       `resolver.Get(key, wsconfig.ItemWorkflowLang)`, take `.Value`.
     - `body, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, wsconfig.Options{}, workflowLang, overrideLookup)`.
  3. Branch:
     - key == "" → FRESH: `body = stripModeGatedRegion(body, true)` (keep
       content, drop marker lines). Return body.
     - key != "" and `rec, ok := s.sessions.readState(key); ok` → CONTINUE:
       `body = stripModeGatedRegion(body, false)` (drop content + markers),
       then `body += "\n\n" + renderSessionState(rec)`. Return body.
     - key != "" and !ok → FAIL-LOUD: `body = stripModeGatedRegion(body, true)`
       (keep bootstrap line — caller may need to mint), then
       `body += "\n\n## Session State\n(no restorable state for session key \"" + key + "\"; ...)"`.
       Return body. NEVER call `s.sessions.mint`.
  4. `return toolTextResponse(id, body+"\n", nil)`.
- `renderSessionState(rec sessionRecord) string`: build a `## Session State`
  section. Agenda: sort `rec.Agenda` keys, print each `### agenda: <key>` +
  pretty JSON of the blob (remind). Todo: `### Todos\n` + `renderTodos(rec.Todos,
  false)`. (Handler-owned scaffolding strings only; no manual prose.)

## 4. Mode-gating region marker — rsrc

- File: `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`.
- Current "### Session setup" block (~lines 50-64) mixes the fresh-only
  self-bootstrap sentence with the always-shown per-root rule in ONE paragraph.
  Split into two: keep the per-root rule OUTSIDE the marker (always shown), wrap
  ONLY the self-bootstrap sentence inside the marker.
- Marker token (NEW, distinct from override + product-mode markers):
  `<!-- ws:fresh-only:start -->` … `<!-- ws:fresh-only:end -->`.
  - Verified non-colliding: `selectProductModeBlocks` switches only on
    `ws:full-only` / `ws:wsflow-only` / `ws:mercenary-on` start/end tokens
    (playbook_tools.go ~514-521, 528-562) — `ws:fresh-only:*` is not matched, so
    those lines pass through verbatim. `applyOverrideMarkers` matches only the
    `<!-- ws:override:` / `<!-- ws:/override:` prefixes — `ws:fresh-only:` is not
    matched, passes through. So under `playbook.print` the marker comment lines
    surface verbatim as inert HTML comments (acceptable; backward compat holds).
  - Do NOT add `ws:fresh-only:*` to `selectProductModeBlocks` (that would make it
    product-mode gating, not fresh/continue gating). The ONLY consumer is the new
    handler's `stripModeGatedRegion`.
- Proposed rsrc edit (split the existing paragraph):
  ```
  At the start of any lead workflow session, ... mint your session key.  [per-root + bootstrap currently one block]
  ```
  becomes:
  ```
  <!-- ws:fresh-only:start -->
  You have no session key yet: call `ws.ferrule(root: "<absolute-working-directory>")`
  for this root to mint your lead key. The name is deliberately non-descriptive
  and is taught only here.
  <!-- ws:fresh-only:end -->

  Each key binds to one canonical repository root — the git top-level of the path
  you pass — and a git worktree resolves to its own top-level, so it counts as a
  distinct root. Call `ws.ferrule` once per working root, and thread the matching
  `session_key` through every subsequent root-aware {{.McpNamespace}} tool call
  that targets that root.
  ```
  (Keep the `ws.ferrule` literal token + the "deliberately non-descriptive /
  taught only here" obscurity note; preserve the `{{.McpNamespace}}` var. Exact
  prose is the implementer's to tune so test fragments match — pick stable
  fragments: e.g. `"mint your lead key"` for the gated line and `"once per
  working root"` for the always-shown rule, and reference them in the tests.)

## 5. `stripModeGatedRegion` pure helper

- Co-locate with the handler. Signature:
  `func stripModeGatedRegion(body string, keepContent bool) string`.
- Logic: split lines; drop any line whose trimmed form == `ws:fresh-only:start`
  marker or `ws:fresh-only:end` marker; for lines BETWEEN a start and the next
  end, keep them iff `keepContent`. Outside markers, always keep. Tolerate an
  unclosed marker (treat trailing region per `keepContent`) — mirror the
  defensive posture in `applyOverrideMarkers`.
- Pure → TDD: table tests (keep vs strip, no-marker passthrough, multiple lines
  inside). Put in `session_state_test.go` or a new `workflow_manual_test.go`.

## 6. `git.commit` todo re-injection — `internal/mcp`

- Dispatch case `git.commit` at server.go ~687-707; formatter `formatGitCommit`
  at ~1652 (takes only `wsgit.CommitResult`, returns string).
- Plan: in the `git.commit` dispatch case, AFTER computing `result` and BEFORE
  `return toolTextResponse(req.ID, formatGitCommit(result), err)`:
  - read `key, _ := params.Arguments["session_key"].(string)` (the case already
    resolves root via `resolveToolRoot` which needs the key, so it is present).
  - `summary := ""; if k := strings.TrimSpace(key); k != "" { if rec, ok :=
    s.sessions.readState(k); ok && len(rec.Todos) > 0 { summary =
    renderTodos(rec.Todos, false) } }`.
  - Build the response text as `formatGitCommit(result)` + (when summary != "")
    a `\n## Todo (post-commit)\n` + summary block. Either append at the dispatch
    site, or add a `todoSummary string` param to `formatGitCommit` — appending at
    the dispatch site is simpler and keeps `formatGitCommit` signature stable for
    its other callers (check callers first: `grep -n formatGitCommit`). The JSON
    path (`wantsJSON`) is unchanged — re-injection is text-mode only.
- All in `internal/mcp`; `internal/wsgit` untouched. (mcp-runtime invariant.)
- `[escalate-to-lead]` ONLY if `formatGitCommit` turns out to have callers that
  would break from an appended-at-callsite approach AND a param add is also
  awkward — unlikely; the appended-at-callsite path avoids signature churn.

## 7. runtime.json registration (BOTH files)

- `agents-plugin/runtime.json` `tools` object (~8-76): add
  `"ws.workflow_manual": ">=0.30.8-dev <0.31.0",` (e.g. after
  `"path.generate"` line 52 / before `"playbook.print"` line 53). Order is not
  enforced.
- `agents-plugin-wsflow/runtime.json` `tools` object (~11-62): add the same
  line. MANDATORY here — this file declares `"runtime_capabilities": {"match":
  "exact"}` (lines 8-10), so the launcher rejects the binary if `tools()`
  surfaces a tool absent from this contract (or vice versa). The tool is a lead
  session tool, not mercenary/exec, so it belongs in the wsflow contract per the
  plugin-runtime wsflow-surface-boundary rule.

## 8. Manifest + wsflow mirror regen

- After the rsrc edit (step 4), from `agents-plugin-tool/`:
  - `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
  - `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
  - verify: `go test ./internal/wsrsrc` → `TestShippedManifestUpToDate` +
    `TestWsflowRsrcMirrorUpToDate` green.
- Confirmed regen test names by reading
  `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go` and
  `wsflow_mirror_test.go`.

## 9. Tests

- Extend `agents-plugin-tool/internal/mcp/session_state_test.go` (integration) +
  pure-logic strip tests (here or a sibling file).
- Helpers reused: `useLeadProfile`, `callLogin`+`parseLoginResponse`,
  `callToolWithKey`, `responseLinesByID`, `toolText`, `initGit`,
  `WS_CACHE_HOME` tempdir pattern. For the no-key fresh test, add a small
  `callToolNoKey` variant (copy `callToolWithKey` minus the
  `args["session_key"] = key` line) since `callToolWithKey` always injects a key.
- Cases: fresh (bootstrap present, no Session State), continue (bootstrap absent,
  per-root rule present, agenda+todo summary present), unknown-key (notice + no
  key file minted under WS_CACHE_HOME/keys), git.commit re-injection (todo
  summary appended; empty-todo session unchanged), drift guards green.

## Run command

From `agents-plugin-tool/`:
```
go build ./... && go test ./internal/mcp/ ./internal/wsrsrc/
```
(Plus the two regen commands in step 8 before the wsrsrc verify run.)

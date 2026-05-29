# Brief: 260529-refactor-wsflow-implement-convergence (code portion)

## Intent

Add a new wsflow-only MCP tool `prompt.render(stem, context) -> { prompt_path }`
plus a symmetric "wsflow-only tool" visibility gate, so wsflow skills can hand
bundled delegate prompts to native subagents. This brief covers ONLY the Go code,
the wsflow runtime contract, and the tests. The skill rewrites and docs are
already done by the lead on this branch (commit `accdc685`); do not touch them.

## Scope Boundary

In scope:
- `agents-plugin-tool/internal/mcp/server.go`: new `wsflowOnlyTool` predicate, the
  mirror gate at the existing gate points, the `prompt.render` tool schema, the
  dispatch case, and a `renderPrompt` implementation.
- `agents-plugin-tool/internal/wsstate/generated_paths.go`: add a `"prompt"` path
  kind.
- `agents-plugin-wsflow/runtime.json`: add `prompt.render` to `tools`.
- Go tests in `agents-plugin-tool/internal/mcp/server_test.go` (and `wsstate`/
  `wsprompt` if you add helpers).
- Python tests in `agents-plugin-wsflow/tests/`.

Out of scope (already done or deliberately excluded):
- Any `agents-plugin-wsflow/skills/**` file — the lead authored these. Do not edit.
- Any `ai-docs/**` doc/spec/ticket/mental-model file — lead-owned.
- `agents-plugin/runtime.json` (full ws) — MUST NOT list `prompt.render`. Only
  confirm it stays absent if a full-ws runtime contract test exists.
- No CLI mirror for `prompt.render` (no `commands` entry).

## Caller-Visible Contract

- `prompt.render(stem: string, context?: object<string,string>) -> { prompt_path }`.
  The MCP text response is the rendered tmp file's absolute path (one path, no
  JSON envelope required — follow the `path.generate` text-response style).
- Behavior: load the bundled prompt named `stem`; apply render-time namespace
  substitution; append the caller-supplied `context` as an injected block; write
  the result to a worktree-scoped tmp file; return that path.
- Namespace substitution: replace `ws/` with `<RuntimeNamespace()>/` and `ws:`
  with `<RuntimeNamespace()>:` in the prompt body. Since the tool only runs in
  wsflow mode, this yields `wsflow/` and `wsflow:`. Drive it from
  `RuntimeNamespace()`, not a hardcoded literal.
- It MUST NOT mint, require, or return an `expected_output_path`. The only
  returned artifact is `prompt_path`.
- Visibility (the mirror gate): `prompt.render` is advertised in `tools/list`,
  passes `toolAllowed`, and is callable ONLY when NOT in full-ws mode (i.e. in
  wsflow/agentless mode). In full ws it is hidden from `tools/list` and an
  explicit call returns a clear JSON-RPC error. This is the exact mirror of the
  existing `NoAgentMode() && noAgentHiddenTool(name)` gate.

## Contract Instructions

Files / symbols (line anchors from the survey at
`.claude/survey-prompt-render-implementation.md` — verify before editing, code
may have drifted):

1. Gate predicate — `agents-plugin-tool/internal/mcp/server.go`:
   - Add `func wsflowOnlyTool(name string) bool` next to `noAgentHiddenTool`
     (~line 3087). Return `true` only for `"prompt.render"`. Use a `switch`/set
     shape so more wsflow-only tools can be added later.
   - Gate point 1 — `callTool` (~line 341, right after the existing
     `NoAgentMode() && noAgentHiddenTool` check): add
     `if !NoAgentMode() && wsflowOnlyTool(params.Name) { return errorResponse(req.ID, -32601, fmt.Sprintf("%s: tool not available in full ws mode: %s", RuntimeNamespace(), params.Name)) }`.
   - Gate point 3 — `toolAllowed` (~line 2983, right after the existing
     `NoAgentMode() && noAgentHiddenTool` check): add
     `if !NoAgentMode() && wsflowOnlyTool(name) { return false }`.
   - Gate point 2 — `tools/list`/`filteredTools` (~line 2926): no new code needed;
     it already filters through `toolAllowed`. Confirm this by test.

2. Tool schema — `tools()` (~line 2290): add a `prompt.render` entry. `stem` is a
   required string property; `context` is an optional object property of string
   values (e.g. `map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "..."}`). Description:
   "Render a bundled delegate prompt by stem with namespace substitution and
   injected context; returns a tmp prompt file path (wsflow only)."

3. Dispatch case — `callTool` switch (near the `path.generate` case ~line 834):
   ```go
   case "prompt.render":
       root, err := s.resolveToolRoot(params.Arguments, params.Meta)
       if err != nil {
           return toolTextResponse(req.ID, "", err)
       }
       stem, _ := params.Arguments["stem"].(string)
       promptPath, err := renderPrompt(root, stem, stringMap(params.Arguments["context"]))
       return toolTextResponse(req.ID, promptPath+"\n", err)
   ```
   Add a `stringMap(any) map[string]string` helper if one does not already exist
   (mirror the existing `stringList` helper used by `path.generate`).

4. `renderPrompt(root, stem string, context map[string]string) (string, error)`:
   - Load the prompt body by stem. Reuse `wsprompt` loading; if `resolveOne` is
     unexported, add a small exported helper in `wsprompt` (e.g.
     `func RenderSource(stem string) (string, error)`) that returns the prompt
     body for a bare stem (reject paths/`..`, same validation as existing
     loaders). Prefer reusing existing logic over duplicating it.
   - Apply namespace substitution driven by `RuntimeNamespace()` (see contract).
   - Append an injected context block when `context` is non-empty, e.g.:
     ```
     <substituted body>

     ## Render Context
     - <key>: <value>
     ```
     Deterministic key order (sort keys) so output is stable for tests.
   - Allocate the output path via
     `wsstate.NewManager(wsstate.Options{}).GeneratePaths(root, "prompt", []string{stem})`
     and write the rendered text to `generated[0].Path`. (GeneratePaths creates
     the file with O_EXCL; write the body into that file.)
   - Return the path.

5. `generated_paths.go`: extend `generatedPathTarget` to support `kind == "prompt"`
   → a worktree-scoped prompt tmp dir (add a `PromptDir` to `Layout` in
   `layout.go`, mirroring `ReviewDir`), extension `.md`. Follow the existing
   `review` kind exactly.

6. `agents-plugin-wsflow/runtime.json`: add `"prompt.render": ">=0.29.4-dev <0.30.0"`
   to the `tools` map (match the version range of the sibling entries). No
   `commands` entry.

Reuse, do not reinvent: `RuntimeNamespace()`, `NoAgentMode()`, the existing
prompt loader in `wsprompt`, `wsstate.GeneratePaths`, `toolTextResponse`,
`s.resolveToolRoot`, `stringList`/`stringMap`. Forbidden: any temporary/mock/
fallback prompt source, a second hardcoded prompt copy, or a per-namespace prompt
bundle (the bundle stays shared; only render output is substituted).

## Integration Test Instructions

Boundary types: MCP tool gating + MCP tool behavior + runtime contract.

Go (`agents-plugin-tool/internal/mcp/server_test.go`, extend existing patterns;
model on `TestServeStdioNoAgentModeHidesAgentBackedTools`):
- `tools/list` in wsflow mode (`WS_MCP_NO_AGENT=1`, `WS_MCP_NAMESPACE=wsflow`)
  INCLUDES `prompt.render`; in full ws mode (no env) EXCLUDES it.
- `tools/call prompt.render` in full ws mode returns a JSON-RPC error and does not
  write a file.
- `tools/call prompt.render` in wsflow mode with `stem: "code-reviewer"` returns a
  path; the file at that path exists, contains `wsflow/` and contains no `ws/`
  substring, and (with a `context`) contains the injected `## Render Context`
  block with the supplied key/value.
- A wsprompt/wsstate unit test for the new `"prompt"` path kind if you add helpers.

Python (`agents-plugin-wsflow/tests/`):
- `test_wsflow_runtime_contract.py`: assert `prompt.render` appears in the wsflow
  agentless `runtime.capabilities`/`tools` surface and matches `runtime.json`; if
  there is a full-ws contract path, assert it is absent there.
- `test_wsflow_skill_bundle.py`: update `EXPECTED_WSFLOW_ONLY_SKILLS` (line ~74)
  to `[]` (no wsflow-only skills now; `lead-edit` removed), drop the `lead-edit`
  entries (lines ~26, ~44), and remove the `mental-model-updater` entry from
  `FORBIDDEN_PATTERNS` (the lead relaxed that token; render-time substitution
  sanitizes it). Keep `\bws/` and `\bws:` forbidden.

Run commands (read FULL output; claim pass only after reading it):
- `cd agents-plugin-tool && go build ./... && go test ./internal/mcp/... ./internal/wsprompt/... ./internal/wsstate/...`
- `python3 -m unittest discover agents-plugin-wsflow/tests`

## Implementation Strategy Decisions (do not reopen)

- Single shared prompt bundle; render substitutes at runtime. `content_sha256`
  stays identical across ws and wsflow (you add a tool, not a prompt file).
- Mirror gate is symmetric and lives at the same points as the agentless gate.
- Context injection is an appended labeled block, NOT placeholder editing of the
  shipped prompt files. Do not add `{{...}}` placeholders to prompt bodies.
- No `expected_output_path`. File-writing prompts get their output path from the
  caller via `context`; that path is injected into the body for the subagent to
  use, but `prompt.render` itself never creates or returns it.

## Rejected Alternatives

- Per-package prompt bundles or baked `{{ns}}` placeholders — breaks the shared
  `content_sha256` contract.
- Minting `expected_output_path` — native subagents may lack write permission;
  free-text response is always available.

## Constraints

- wsflow-only: never advertise or allow `prompt.render` in full ws.
- Do not add it to `agents-plugin/runtime.json`.
- Deterministic rendered output (sorted context keys) for stable tests.
- Follow existing handler/test style; no new external deps.

## Out of scope

- Skills, docs, specs, tickets, mental models (lead-owned, already committed).
- CLI mirror for `prompt.render`.

## Verification Contract

- `go build ./...` clean; `go test` for `internal/mcp`, `internal/wsprompt`,
  `internal/wsstate` pass.
- `python3 -m unittest discover agents-plugin-wsflow/tests` passes with the
  `lead-edit` divergence exception removed and `prompt.render` contract coverage
  added.
- New Go tests prove both gate directions (wsflow advertises+serves; full ws
  hides+rejects), namespace substitution (`wsflow/` present, `ws/` absent), and
  context injection.

## References

<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `.claude/survey-prompt-render-implementation.md` - [Must] exact line anchors,
  function signatures, and gate/handler/path/prompt-loader map.
- `ai-docs/spec/mcp-tools.md` (`#260529-prompt-render-tool`,
  `#260529-wsflow-only-tool-surface`) - [Must] the caller-visible contract.
- `ai-docs/mental-model/mcp-runtime.md` - [Must] tool gating architecture and the
  three gate points.
- `ai-docs/mental-model/prompt-bundle.md` - [Maybe] prompt discovery/resolution
  and the rule that prompt-file changes (not tool additions) move the bundle hash.
- `agents-plugin-tool/internal/mcp/server.go` - [Must] gate + handler edits.
- `agents-plugin-tool/internal/wsstate/generated_paths.go`,
  `agents-plugin-tool/internal/wsstate/layout.go` - [Must] new `prompt` path kind.
- `agents-plugin-tool/internal/wsprompt/prompts.go` - [Must] prompt loader to reuse.

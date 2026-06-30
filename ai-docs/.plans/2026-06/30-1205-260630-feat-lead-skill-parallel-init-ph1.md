# Plan: 260630-feat-lead-skill-parallel-init — Phase 1: workflow_manual absorbs ferrule for fresh-start

## Relevant Ticket Contract

- **Change:** Add optional `root` parameter to `ws.workflow_manual`. In FRESH mode (sentinel key `"obsidian-latch"`), when `root` is supplied, the handler mints a session key internally (via the same `canonicalSetupRoot` + `sessions.mint` path as `handleLeadLogin`) and returns the minted key inline with the rendered manual.
- **Response contract (new branch):** `workflow_manual("obsidian-latch", root: "...")` → strip fresh-only block (`stripModeGatedRegion(body, false)`), then append `## Session Key\n<minted-key>` and `## Session State` (empty for new session).
- **Response contract (existing branches unchanged):** `workflow_manual("obsidian-latch")` with no root → FRESH unchanged (gated block kept). CONTINUE mode → unchanged. FAIL-LOUD mode → unchanged.
- **Constraint:** `root` must be an absolute filesystem path; validation via `canonicalSetupRoot` (same as ferrule). Non-sentinel key with `root` argument must still hit FAIL-LOUD before any minting — mint path is gated exclusively on the sentinel branch.
- **Spec update required:** `ai-docs/spec/mcp-tools.md` anchor `260626-workflow-manual-restoration-entry` must document the new `root` param and FRESH-mode key return.
- **Verification:** After change, `workflow_manual("obsidian-latch", root: "...")` returns a session key in the response body; no second `workflow_manual` or separate `ferrule` call is needed before `project_tree` or `git.status`.

## Out of Scope

- Phase 2 (SKILL.md parallel entry declaration for lead-discuss and lead-sprint, On: invoke simplification) — not touched in this phase.
- lead-discuss, lead-sprint, lead-proceed, lead-implement playbook changes.
- `agents-plugin-wsflow` rsrc mirror updates — ticket Phase 2 constraint names them; Phase 1 only adds the handler and tool-schema change.
- Any change to the ferrule tool itself (`ws.ferrule` / `bootstrapToolName` handler).

## Codebase Findings

- `/home/swkang/devenv/agents-plugin-tool/internal/mcp/workflow_manual.go#L118-L173` — `handleWorkflowManual`: the handler to modify. Sentinel branch (lines 163–166) currently calls `stripModeGatedRegion(body, true)` and returns. The new mint-and-strip path must be inserted here, gated on `root != ""`.
- `/home/swkang/devenv/agents-plugin-tool/internal/mcp/workflow_manual.go#L43-L66` — `stripModeGatedRegion(body, false)`: used in CONTINUE mode to strip the fresh-only block; must be reused in the new root-supplied sentinel branch (not `true`). Ticket explicitly calls out this selection.
- `/home/swkang/devenv/agents-plugin-tool/internal/mcp/server.go#L1371-L1410` — `canonicalSetupRoot` + `handleLeadLogin`: the reference mint path. `canonicalSetupRoot` validates and canonicalizes root; `sessions.mint(canonical, roleLead, "")` creates the key. Both functions are in scope to call directly from `handleWorkflowManual` (same package).
- `/home/swkang/devenv/agents-plugin-tool/internal/mcp/server.go#L3439-L3447` — `ws.workflow_manual` tool schema: `inputSchema.properties` currently has only `session_key`. Must add `root` as an optional `stringProperty` describing the absolute Git worktree root for fresh-start minting.
- `/home/swkang/devenv/agents-plugin-tool/internal/mcp/session_state_test.go#L2262-L2287` — `TestWorkflowManualFreshMode`: existing test for no-root fresh mode. A new test `TestWorkflowManualFreshModeWithRoot` is needed alongside it, verifying: (a) `## Session Key` section present, (b) fresh-only block absent (no "mint your lead key"), (c) `## Session State` present, (d) returned key resolves to a lead session record.
- `/home/swkang/devenv/agents-plugin-tool/internal/mcp/server.go#L366-L371` — Lead-only keyed gate: passes sentinel key through (lookup miss → `found=false` → gate skips), so the new `root` argument poses no gate interaction concern. No gate change needed.
- `/home/swkang/devenv/agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md#L55-L62` — Fresh-only gated region (markers at lines 55, 63): the content between `ws:fresh-only:start` and `ws:fresh-only:end` contains the "mint your lead key" instruction. In the root-supplied branch this block must be stripped via `stripModeGatedRegion(body, false)` per the ticket contract. No change to the rsrc file itself is needed.
- `/home/swkang/devenv/ai-docs/spec/mcp-tools.md#L314-L353` — Spec anchor `260626-workflow-manual-restoration-entry`: describes current FRESH/CONTINUE/FAIL-LOUD/keyless branches. Must be updated to add the new `root`-supplied FRESH-with-minting branch and document `## Session Key` response section.

## Implementation Plan

1. **Add `root` extraction to `handleWorkflowManual`** (`workflow_manual.go#L119`): after extracting `key`, extract `root` with `root, _ := args["root"].(string); root = strings.TrimSpace(root)`.

2. **Add mint-and-strip path in the sentinel branch** (`workflow_manual.go#L163-L166`): split the existing `if key == freshBootstrapKey` block into two sub-cases:
   - `root != ""` (new): call `canonicalSetupRoot(root)` — fail-loud on error. Call `s.sessions.mint(canonical, roleLead, "")` — fail-loud on error. Call `stripModeGatedRegion(body, false)` (strips fresh-only block). Append `"\n\n## Session Key\n" + mintedKey`. Append `"\n\n" + renderSessionState(sessionRecord{})` (empty state). Return.
   - `root == ""` (existing, unchanged): `stripModeGatedRegion(body, true)` and return as before.

3. **Update the tool schema** (`server.go#L3439-L3447`): add `"root": stringProperty("Optional absolute Git worktree root. When provided alongside the fresh-bootstrap sentinel, the handler mints a lead session key and returns it inline, eliminating the separate ws.ferrule call.")` to `inputSchema.properties`. Keep `required: ["session_key"]` unchanged.

4. **Add test `TestWorkflowManualFreshModeWithRoot`** (`session_state_test.go`, after `TestWorkflowManualFreshMode` at line 2287): set up `WS_CACHE_HOME`, create a temp git dir, call `ws.workflow_manual` with `session_key: freshBootstrapKey` and `root: tempGitDir`. Assert: response contains `"## Session Key"`, response does not contain `"mint your lead key"`, response contains `"## Session State"`, the extracted key resolves to a lead-scoped session record via `server.sessions.readState(key)`.

5. **Update spec** (`ai-docs/spec/mcp-tools.md#L326-L331`): in the `fresh` bullet of the `260626-workflow-manual-restoration-entry` section, add a new sub-case for `root`-supplied behavior: key return, fresh-only block stripped, `## Session Key` and empty `## Session State` appended, ferrule call eliminated.

## Verification Plan

- Run `go test ./internal/mcp/... -run TestWorkflowManual` in `agents-plugin-tool/` — all existing tests must pass; new `TestWorkflowManualFreshModeWithRoot` must pass.
- Manual check: confirm `TestWorkflowManualFreshMode` (no-root path) still returns "mint your lead key" and no `## Session Key` section.
- Confirm `TestWorkflowManualUnknownKey` (FAIL-LOUD) still returns only the no-restore notice and no manual body, even if a `root` argument were hypothetically passed (FAIL-LOUD guard runs at line 136, before the sentinel branch — no change needed).

## Escalations

- None.

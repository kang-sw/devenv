# Plan: 260814-feat-note-project-local-untracked-layer — Phase 1

## Relevant Ticket Contract

- Add a fourth note layer `clone`: project-scoped, worktree-agnostic,
  untracked, substrate `proj/<projectKey>/` — one JSON file per layer
  (parallel to `machine`/`worktree`), NOT one file per key like `repo`.
- Extend the `layer` enum on every `note.*` tool (`write`/`search`/`erase`/
  `mute`/`unmute`), the `workflow_manual` ambient `# Notes` injection, and the
  `mcp-tools.md` note-layer spec (`## Note Tools {#260810-note-tools}` and
  `### Note Injection {#260810-note-injection}`).
- Distinct, separately-verifiable sub-step: re-point the `_index.local.md`
  migration guidance in bootstrap/project-memory docs at `clone` instead of
  `machine`.
- Verify: a `clone` note is visible from a sibling worktree of the same
  project, absent from a different project on the same machine, and never
  staged by git; a `clone` note surfaces in the ambient `# Notes` injection
  alongside the other layers; the re-pointed `_index.local.md` guidance names
  `clone`, not `machine`.
- BUNDLED (`260814-bug-wsflow-runtime-contract-missing-note-mute-unmute`): add
  `note.mute`/`note.unmute` to the runtime tool contract(s) so
  `test_runtime_contract_matches_agentless_capabilities` passes. Decide
  wsflow-only vs. both packages against the intended agentless capability
  surface (note.* is not agent-gated — reachable by any scope, no
  `noAgentHiddenTool` entry — so both packages should carry the same set).
  Apply any version-edition change only through
  `agents-plugin-tool/scripts/bump-ws-version.sh`, never by hand-editing
  edition points.

## Out of Scope

- `260814-feat-note-project-local-untracked-layer` Phase 2+ (none currently
  declared beyond Phase 1) and any future note-layer work.
- `260807-feat-note-memory-layers` / `260810` repo-layer mechanics — reused
  read-only as precedent, not modified.
- Redesigning `_index.local.md`/bootstrap doc structure beyond the literal
  `machine` → `clone` re-point (mirroring the existing `repo`-layer doc
  pattern already present in the same files).
- Version bump execution (`bump-ws-version.sh`) — that rides the AGENTS.md
  "dev-merge" convention, not this phase; this phase only adds the two new
  tool keys to `runtime.json` at the currently-committed version range.
- Any change to `note.write`/`erase`/`mute`/`unmute`/`search` semantics beyond
  adding the fourth enum value and its storage/injection wiring.

## Codebase Findings

- `agents-plugin-tool/internal/wsstate/paths.go#L176-L199` — `layoutFor`
  already computes `Layout.ProjectDir = filepath.Join(cacheRoot, "proj",
  projectKey)`, worktree-agnostic (keyed on `projectKey`, not `worktreeKey`).
  This is exactly the `clone` substrate the ticket specifies — no new
  wsstate mechanism needed, just a new `wsnote` accessor that reads
  `Layout.ProjectDir` instead of `Layout.WorktreeDir`.
- `agents-plugin-tool/internal/wsnote/store.go#L17-L56` — `Layer` type and
  `LayerMachine`/`LayerWorktree`/`LayerRepo` consts (L20-24); `MachinePath`
  (L30-39, sibling of global config); `WorktreePath` (L41-49, uses
  `layout.WorktreeDir`); `RepoDir` (L51-56). Add `LayerClone Layer = "clone"`
  and a `ClonePath(root string) (string, error)` mirroring `WorktreePath` but
  reading `layout.ProjectDir` and joining `"notes.json"`. `Write`/`Erase`/
  `SetVisible`/`Load`/`rmw` (L61-205) are layer-agnostic file-path operations
  — no changes needed there; `ClonePath` reuses them exactly like
  `WorktreePath` does.
- `agents-plugin-tool/internal/wsnote/record.go#L1-L10` — package doc
  comment says "three note-memory layers" / "All three layers share the same
  record shape" — update to four/all four.
- `agents-plugin-tool/internal/wsnote/inject.go#L30-L57` — `Compute` builds
  `all []layeredRecord` by loading machine (L33-38), then worktree
  (L41-49, gated on `root != ""`), then repo (L51-57, same gate). Add a
  parallel `clone` block gated the same way as worktree/repo (`root != ""`,
  `ClonePath` degrades silently like `WorktreePath`/`RepoLoad` on error) that
  appends `layeredRecord{Record: rec, Layer: LayerClone}`. Comment at L20-29
  ("across the machine, worktree, and repo layers") needs updating to
  mention `clone`.
- `agents-plugin-tool/internal/mcp/note_tools.go#L54-L120` — this is the
  single dispatch chokepoint: `resolveNoteStore` (L65-105) switches on
  `wsnote.Layer` to build a `fileNoteStore{path: ...}` for machine (L71-85,
  session-key-only, no root) and worktree (L86-95, `resolveToolRoot` +
  `WorktreePath`), and a `repoNoteStore{dir: ...}` for repo (L96-101). Add a
  `case wsnote.LayerClone` between worktree and repo: `resolveToolRoot` (same
  as worktree) + `wsnote.ClonePath(root)`, wrapped in `fileNoteStore{...}`
  (clone shares the file-per-layer storage shape with machine/worktree, not
  the file-per-key shape of repo — matches the struct doc comment at
  L14-20 which will also need the "machine/worktree" enumeration extended).
  Default-branch error text at L103 (`want "machine", "worktree", or
  "repo"`) must list `clone` too. `noteLayerArg` (L107-120) needs a
  `case wsnote.LayerClone: return wsnote.LayerClone, nil` arm and its error
  string at L118 updated the same way.
- `agents-plugin-tool/internal/mcp/note_announcement.go#L15-L21` — doc
  comment "merging the machine and worktree layers" is already stale (it
  omits `repo`, which `Compute` already covers) — update while touching this
  file to enumerate all four, or leave as pre-existing minor drift if out of
  the ticket's literal ask. `computeNotes` itself needs no code change — it
  is a thin pass-through to `wsnote.Compute`.
- `agents-plugin-tool/internal/mcp/server.go#L4293-L4357` — five tool
  schemas, each with a `"layer"` field built via `enumStringProperty(desc,
  []string{"machine", "worktree", "repo"})` and a description string
  enumerating the three layers in prose: `note.write` (L4293-4302,
  enum at L4299), `note.erase` (L4306-4315, enum at L4312), `note.mute`
  (L4319-4328, enum at L4325), `note.unmute` (L4332-4341, enum at L4338),
  `note.search` (L4345-4357, enum at L4351). Each needs `"clone"` appended to
  the enum slice and to the human-readable enumeration in both the tool
  `"description"` and the field's own enum description string.
- `agents-plugin-tool/internal/mcp/note_tools_test.go#L65,#L356` — the
  `TestNoteWriteSearchEraseRoundTripPerLayer` loop
  (`for _, layer := range []string{"machine", "worktree"}`, L65) and
  `TestNoteWriteRejectsInvalidLayer`'s exact-string assertion
  (``must be "machine", "worktree", or "repo"``, L356) will both go stale
  the moment the enum/error text changes — both need mechanical updates
  alongside the source change, or the go test suite goes red on this
  ticket's own edit.
- `agents-plugin-tool/internal/mcp/note_tools_test.go#L26-L54` (`mintRootKey`)
  and `#L34-L54` (`twoWorktreesOfOneRepo`) are the exact fixtures to reuse
  for the new clone-isolation test: `twoWorktreesOfOneRepo` for "visible from
  a sibling worktree of the same project" (mirrors
  `TestNoteWorktreeLayerIsolatedAcrossWorktrees`, L234-272, which already
  demonstrates the machine-shared / worktree-isolated pattern side by side —
  a `clone`-shared assertion slots in next to the existing `machine`-shared
  one), and `mintRootKey` a second time (fresh temp dir/new git repo = new
  `projectKey`) for "absent from a different project."
- `agents-plugin-tool/internal/wsnote/store_test.go#L126-L137`
  (`TestMachinePathIsSiblingOfGlobalConfig`) is the pattern to parallel for a
  new `TestClonePathIsProjectScopedAndWorktreeAgnostic`-style unit test
  asserting `ClonePath` resolves under `Layout.ProjectDir` and is identical
  for two worktrees of the same repo but different across two repos.
- `agents-plugin-tool/internal/wsnote/inject_test.go` — `TestComputeRendersMachineAndWorktreeLayers`
  (L19-54) and `TestComputeSortsAndCapsAcrossAllThreeLayers` (L72-109) are the
  patterns to extend/parallel with a clone-layer case; no test currently
  hardcodes "three layers" as a count assertion (confirmed by scanning for
  `len(`/"three" — none found), so no incidental breakage expected in
  `inject_test.go` beyond adding new coverage.
- `agents-plugin-tool/internal/mcp/note_workflow_manual_test.go` — no
  hardcoded layer-count assertions found (scanned for `len(`/"three"); the
  ambient-injection ordering/elision/mute tests are layer-parametric via
  string literals only, so no existing test needs correction for a fourth
  layer, only new coverage for it.
- `ai-docs/spec/mcp-tools.md#L416-L521` (`## Note Tools {#260810-note-tools}`)
  — full prose describing three layers, storage split (file-per-layer for
  machine/worktree vs. file-per-key for repo, L425-439), the `layer` enum
  literal at L446-447, and the RMW storage description at L504-517. All need
  a fourth-layer edit: add `clone` to the enum list and the file-per-layer
  storage description (clone shares the `machine`/`worktree` shape, not
  repo's).
- `ai-docs/spec/mcp-tools.md#L713-L750` (`### Note Injection
  {#260810-note-injection}`) — L717-718 enumerates
  "the `machine`, `worktree`, and `repo` layers" for the injected block; add
  `clone`.
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md#L8` (Project Memory
  "Local" bullet: `` `ai-docs/_index.local.md` if present; it is .gitignored
  machine context ``) and `#L96` (migration-block ai-docs/ layout tree:
  `_index.local.md    - local memory, .gitignored`, sitting right above the
  already-correct `ws-notes/` / `repo`-layer bullet at `#L101`) — re-point at
  `clone`. `#L149` is a historical changelog entry ("v0014: ... add
  `ai-docs/_index.local.md` to `.gitignore`") — leave untouched, it documents
  a past template revision, not current guidance.
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md#L29` (`` `_index.local.md`
  is machine-local memory and should be ignored by Git. ``, in the `##
  ai-docs/ Layout` section right above the correct `repo`-layer bullet at
  `#L32-34`) — same re-point, same pattern as the `ws-notes/`/`repo` bullet
  immediately below it.
- `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md#L8,#L97` and
  `agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md#L29` — byte-for-byte
  same text as the full-ws counterparts (confirmed by diff-grep). Per
  `ai-docs/manuals/wsflow-mirroring.md` ("Bootstrap Template Rules": "Treat
  `lead-bootstrap` as a mirrored skill: behavior changes require checking
  both packages... update both templates in one logical change"), these are
  **hand-edited in parallel**, not regenerated — `lead-bootstrap` is outside
  the rsrc byte-identical-mirror and the substitution-mirrored-skill
  mechanisms (it is a plain curated mirrored skill).
- `/home/swkang/devenv/AGENTS.md` (this repo's own dogfooded root doc,
  `## Project Memory` → item 2 "Local") — currently reads `` read
  `ai-docs/_index.local.md` if present; it is .gitignored machine context ``,
  the same drifted phrasing as the template. Since this repo dogfoods its own
  bootstrap output and "Update drifted docs on contact" is a standing repo
  rule, re-point this line too in the same change.
- `agents-plugin/runtime.json#L28-L30` and
  `agents-plugin-wsflow/runtime.json#L31-L33` — both list only
  `"note.write"`, `"note.erase"`, `"note.search"` at
  `">=0.40.3-dev <0.41.0"`. Neither lists `note.mute`/`note.unmute` even
  though `internal/mcp/server.go` schemas (L4319-4341) and dispatch
  (`server.go#L1257-L1260`, `case "note.mute": ... case "note.unmute":`)
  already serve them live — this is the bug ticket's exact gap. This phase's
  `clone` enum work also needs `note.write`/`erase`/`mute`/`unmute`/`search`
  entries updated for nothing version-wise (adding an enum value doesn't
  change the tool-name inventory), so the two edits are independent line
  additions in the same two files.
- `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L90-L116`
  (`test_runtime_contract_matches_agentless_capabilities`) — asserts
  `set(contract["tools"]) == set(payload["tools"])` where `payload` comes
  from a live `go run ./cmd/ws-mcp runtime capabilities` invocation (env from
  `.mcp.json`'s wsflow server block). This is the red test the bug ticket
  names; it will go green once `note.mute`/`note.unmute` are added to
  `agents-plugin-wsflow/runtime.json`'s `"tools"` object with the same
  `">=0.40.3-dev <0.41.0"` value as their siblings.
- `agents-plugin/tests/` — confirmed (grep across `*.py`) there is no
  equivalent live-binary-comparison test for `agents-plugin/runtime.json`;
  its tests exercise `runtime_capabilities_compatible` against
  hand-built fixture contracts only, matching the bug ticket's own
  observation ("the ws package test suite does not flag it"). Adding
  `note.mute`/`note.unmute` to `agents-plugin/runtime.json` is therefore not
  required to turn any test green, but mental-model rule
  `ai-docs/mental-model/mcp-runtime.md` Common Mistakes ("Adding MCP tools
  without updating `agents-plugin/runtime.json`; launcher compatibility
  checks compare the required MCP tool surface against runtime metadata")
  makes the same drift a latent bug in the full-ws package too — both
  `note.mute`/`note.unmute` are unconditionally live (no `noAgentHiddenTool`
  gate on `note.*`), so both packages' contracts should carry them for
  consistency with the intended full surface, not only the package with a
  red test.
- `agents-plugin-tool/scripts/bump-ws-version.sh#L60-L68` (`update_runtime`)
  — the bump script rewrites the version RANGE on every EXISTING key in
  `"tools"`/`"commands"`; it does not add or remove keys. Adding
  `"note.mute"`/`"note.unmute"` as brand-new keys is therefore a manual edit
  in this phase, using the currently-committed range string
  `">=0.40.3-dev <0.41.0"` (matching every other current entry) — no script
  invocation needed for this phase.
- `ai-docs/manuals/wsflow-mirroring.md#L244-L275` ("Rsrc Tree Provisioning")
  — confirms the rsrc byte-identical-mirror regen only applies to
  `agents-plugin/rsrc/` → `agents-plugin-wsflow/rsrc/`. Neither `runtime.json`
  nor the `skills/lead-bootstrap/` templates nor `mcp-tools.md` live under
  `rsrc/`, so the two-step rsrc regen (`TestGenerateRealManifest` +
  `TestRegenerateWsflowRsrcMirror`) is only needed IF this phase also touches
  an `agents-plugin/rsrc/**` file. Scanned `agents-plugin/rsrc/` for note-tool
  layer enumerations (`executor-wrapup.md`, `lead-workflow-manual.md`) —
  both only mention the `repo` layer generically (`note.write(layer:
  "repo", ...)`) with no three/four-layer enumeration, so no rsrc edit is
  required by this phase. Still run the rsrc drift-guard tests in
  Verification as a safety net in case an edit is later found necessary.

## Implementation Plan

1. **`agents-plugin-tool/internal/wsnote/store.go`**: add
   `LayerClone Layer = "clone"` next to the existing consts (L20-24); add
   `ClonePath(root string) (string, error)` mirroring `WorktreePath`
   (L41-49) but resolving `layout.ProjectDir` instead of `layout.WorktreeDir`,
   joined with `"notes.json"`.
2. **`agents-plugin-tool/internal/wsnote/record.go`**: update the package doc
   comment (L1-10) from "three note-memory layers" / "All three layers" to
   four, adding a one-clause description of `clone` parallel to the existing
   `machine`/`worktree` clauses.
3. **`agents-plugin-tool/internal/wsnote/inject.go`**: add a `clone` block in
   `Compute` (after the worktree block, L41-49, before the repo block) that
   loads `wsnote.ClonePath(root)` the same way worktree does (gated on
   `root != ""`, silent degrade on error) and appends
   `layeredRecord{Record: rec, Layer: LayerClone}`. Update the doc comment
   (L20-29) to name all four layers.
4. **`agents-plugin-tool/internal/mcp/note_tools.go`**: add a
   `case wsnote.LayerClone` arm in `resolveNoteStore` (between the worktree
   arm at L86-95 and the repo arm at L96-101) using `resolveToolRoot` +
   `wsnote.ClonePath(root)` wrapped in `fileNoteStore{...}`; add the matching
   `case wsnote.LayerClone: return wsnote.LayerClone, nil` arm in
   `noteLayerArg` (L107-120); update both error strings (L103, L118) and the
   `noteStore` interface doc comment (L14-20) to mention `clone`.
5. **`agents-plugin-tool/internal/mcp/server.go`**: for each of the five
   `note.*` schemas (`note.write` L4293-4302, `note.erase` L4306-4315,
   `note.mute` L4319-4328, `note.unmute` L4332-4341, `note.search`
   L4345-4357), append `"clone"` to the `enumStringProperty` values slice and
   to the layer enumeration in both the tool `"description"` and the field's
   own description string.
6. **`agents-plugin-tool/internal/mcp/note_announcement.go`**: optionally
   correct the `computeNotes` doc comment (L15-18, already stale re: `repo`)
   to enumerate all four layers while touching this area for consistency.
7. **Runtime contract (bundled bug ticket)**: add
   `"note.mute": ">=0.40.3-dev <0.41.0"` and
   `"note.unmute": ">=0.40.3-dev <0.41.0"` to the `"tools"` object in BOTH
   `agents-plugin/runtime.json` (near L28-30) and
   `agents-plugin-wsflow/runtime.json` (near L31-33), matching the existing
   `note.write`/`note.erase`/`note.search` entries' format and value exactly.
8. **Docs re-point** (mechanical `machine` → `clone` word substitution,
   mirroring the existing `repo`-layer bullet already present in each file):
   - `agents-plugin/skills/lead-bootstrap/AGENTS.template.md#L8` and `#L96`
   - `agents-plugin/skills/lead-bootstrap/WORKFLOW.md#L29`
   - `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md#L8` and
     `#L97`
   - `agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md#L29`
   - `/home/swkang/devenv/AGENTS.md` (`## Project Memory` item 2, "Local")
   Do not touch the historical changelog line
   (`AGENTS.template.md#L149`/wsflow equivalent).
9. **Spec (`ai-docs/spec/mcp-tools.md`)**: extend
   `## Note Tools {#260810-note-tools}` (L416-521) — layer enum at L446-447,
   storage-mechanism description at L425-439 and L504-517 (clone joins the
   file-per-layer group with machine/worktree, not repo's file-per-key
   group) — and `### Note Injection {#260810-note-injection}` (L713-750,
   layer enumeration at L717-718) to name `clone` as the fourth layer.
10. **Test updates** (existing tests that go stale on the enum/error-text
    change):
    - `agents-plugin-tool/internal/mcp/note_tools_test.go#L65`: add
      `"clone"` to the `TestNoteWriteSearchEraseRoundTripPerLayer` layer
      loop.
    - `agents-plugin-tool/internal/mcp/note_tools_test.go#L356`: update the
      expected error substring in `TestNoteWriteRejectsInvalidLayer` to
      include `"clone"`.
11. **New test coverage**:
    - `agents-plugin-tool/internal/wsnote/store_test.go`: a
      `ClonePath`-parallel test to `TestMachinePathIsSiblingOfGlobalConfig`
      (L126-137) asserting `ClonePath` resolves under the project's
      `ProjectDir` and is identical across two worktrees of one repo.
    - `agents-plugin-tool/internal/mcp/note_tools_test.go`: a new test
      parallel to `TestNoteWorktreeLayerIsolatedAcrossWorktrees`
      (L234-272), using `twoWorktreesOfOneRepo` (L34-54) for the
      shared-across-worktrees assertion and a second `mintRootKey` call
      (fresh repo → different `projectKey`) for the absent-from-a-different-
      project assertion, plus a `git status --porcelain` check on the
      worktree root confirming the clone store path (outside the working
      tree, like machine/worktree) is never staged.
    - `agents-plugin-tool/internal/wsnote/inject_test.go`: extend/parallel
      `TestComputeRendersMachineAndWorktreeLayers` (L19-54) and
      `TestComputeSortsAndCapsAcrossAllThreeLayers` (L72-109) with a
      `clone`-layer case so `Compute`'s ambient-injection coverage includes
      all four layers.
    - `agents-plugin-tool/internal/mcp/note_workflow_manual_test.go`: one
      end-to-end case (parallel to the existing worktree/repo cases) proving
      a `clone`-layer note surfaces in the `workflow_manual` `# Notes` block
      tagged `[clone]`.

## Verification Plan

- `cd agents-plugin-tool && go build ./... && go test ./...` — full Go suite,
  including the new/updated `wsnote` and `mcp` package tests above.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — must include
  `test_runtime_contract_matches_agentless_capabilities` going green (the
  bundled bug ticket's exact target) and `test_wsflow_skill_bundle` staying
  green after the `agents-plugin-wsflow/skills/lead-bootstrap/` edits.
- `python3 -m unittest discover agents-plugin/tests` — confirm no regression
  from the `runtime.json`/`skills/lead-bootstrap/` edits on the full-ws side.
- Manifest + wsflow mirror regen safety net (only actually changes files if
  survey step 11's "no rsrc edit required" conclusion turns out wrong):
  ```
  WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
  WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
  ```
  then `git status` on `agents-plugin/rsrc/manifest.json` and
  `agents-plugin-wsflow/rsrc/` — expect no diff; a diff means an rsrc file
  was missed above and must be reconciled before commit.
- Scope-isolation manual/integration check (covered by the new automated
  test in step 11, restate as an explicit runnable check): via two linked
  git worktrees of one temp repo, `note.write(layer: "clone", ...)` from one
  worktree's session key is visible via `note.search(layer: "clone")` from
  the other worktree's session key (worktree-agnostic); a `note.write` under
  a second, unrelated repo's session key does not see it (project-scoped);
  `git status --porcelain` in the worktree never lists the clone store path
  (untracked, stored outside the working tree under
  `<cache-root>/proj/<projectKey>/notes.json`).
- `workflow_manual` ambient injection check: write a `clone`-layer note,
  call `workflow_manual` on a fresh-with-root or continue session, confirm
  the `# Notes` block contains a `- [clone] ...` line.
- `ai-docs/spec/mcp-tools.md` anchors: after editing, re-run
  `ws/spec_index.verify` (or `spec_index.verify` MCP tool) to confirm no
  duplicate-anchor regression was introduced.

## Escalations

- None.

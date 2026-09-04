# Plan: 260903-refactor-mcp-verb-vocabulary-unification — Phase 1: Freeze the map and apply the scripted rename

## Relevant Ticket Contract

- Frozen name map (exact, verified against `agents-plugin-tool/internal/mcp/server.go`):
  `tickets.find` → `tickets.query`, `specs.find` → `specs.query`,
  `mental_models.find` → `mental_models.query`, `note.search` → `note.query`,
  `playbook.print` → `playbook.read`, `runtime.info` → `runtime.read`.
- `mental_models.list`/`mental_models.status` are an explicit noted exception —
  **not** renamed or collapsed in this phase.
- Method is deterministic script, not hand-edit-by-name: sweep Go tool
  registration + dispatch switch + name constants + tool descriptions,
  `runtime.json`, `mcp-tools.md` + `workflow-skills.md`, playbook tokens in
  `agents-plugin/rsrc/` + the wsflow mirror (regenerated, not hand-edited), and
  tests.
- Behavior is byte-unchanged: rename only, no handler logic touched.
- Acceptance: no old name remains in-package (`references.trace`/grep sweep
  clean), `go test ./...` in `agents-plugin-tool/` green, `tools/list` shows
  canonical verbs.
- Deprecation posture: one-shot hard cut, no alias/transition window — every
  old-name reference is removed in the same pass, none left as compat alias.
- Ordering: this ④-script pass runs before ①'s `enter.*` authoring pass on the
  same skill files, so `lead-proceed`/`lead-implement` text is authored once.

## Out of Scope

- `mental_models.list`/`.status` verb names (explicit ticket exception).
- `exec.*`/`mercenary.*` families (epic-level exclusion, unstable/gated
  surface, not this ticket's landing gate).
- **CLI subcommand surface** (`agents-plugin-tool/cmd/ws-mcp/main.go`
  `ticketsCommand`/`specsCommand`/`mentalModelsCommand` `case "find":`
  dispatch, and `runtime.json`'s `"commands"` section, and
  `runtimeCapabilityCommandNames()`'s hardcoded list). The ticket's frozen map
  and Method describe the **MCP JSON-RPC tool surface** (`tools()` +
  `switch params.Name` dispatch) only; the CLI verb surface is a materially
  different, separately-versioned surface not mentioned anywhere in Decisions
  or Method. Leaving it untouched is self-consistent: `runtime.json`
  `"commands"` values and `runtimeCapabilityCommandNames()` in `main.go` are
  each other's only mutual test dependency
  (`TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` in
  `agents-plugin-tool/cmd/ws-mcp/main_test.go`), independent of the `"tools"`
  section — so leaving both alone breaks nothing. See Escalations-adjacent
  note below; flagged for lead awareness, not blocking.
- `ai-docs/mental-model/*.md` (`mcp-runtime.md`, `workflow-skills.md`,
  `prompt-bundle.md`, `documentation-system.md`, `plugin-runtime.md`,
  `api-documentation-cache.md`) and `ai-docs/manuals/*.md`
  (`ws-mcp.md`, `ws-agent-runtime.md`, `windows-dogfood.md`) prose mentions of
  the six old names. Phase 1's Method enumerates only `mcp-tools.md` +
  `workflow-skills.md` (the specs) for doc sweep; mental models/manuals are not
  listed. Left as literal residue for a follow-up doc-drift pass (or the
  executor may opportunistically sweep them — same low-risk token swap — but
  it is not required for this phase's acceptance criteria).
- `ai-docs/.plans/**` and `ai-docs/tickets/.done/**` / `.dropped/**` — historical
  point-in-time records. **Do not retroactively rename inside these**; they
  describe what was true when written. (These dominate a naive repo-wide grep
  for the old names — exclude them explicitly from the sweep scope.)
- `agents-plugin/tests/test_ws_mcp_launcher_capabilities.py` lines 51-147 use
  `"runtime.info"` as synthetic fixture data with fabricated version strings
  (`0.18.1-dev`), not real contract values — renaming is optional/cosmetic,
  not required for correctness.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/server.go` — 21 literal occurrences of the
  six old names across: dispatch `switch params.Name` cases (`526`, `1045`,
  `1085`, `1127`, `1158`, `1454`), two user-facing error strings that name the
  tool (`1047` `specs.find uses ticket_stem...`, `1087` `mental_models.find
  uses spec_stem...`), tool registration `"name"` fields (`3408`, `3998`,
  `4031`, `4108`, `4141`, `4329`), one cross-tool description mentioning
  `note.search` by name inside `note.mute`'s description (`4083`), the
  `toolSchemaRequiresSessionKey` name list (`4531-4533`, three of the six
  appear here: `specs.find`, `mental_models.find`, `tickets.find`), and
  comments (`505`, `1506`, `2490`, `5067`, safe but should still be renamed for
  consistency/no-stale-comment).
- `agents-plugin-tool/internal/mcp/note_tools.go:432` — `const tool =
  "note.search"` is used to prefix real error messages (`fmt.Errorf("%s: %w",
  tool, err)`) and passed into `noteSearchLayersArg(tool, args)` for its own
  error text — a behavior-visible site, not just a comment. Also comments at
  `145`, `162`, `420`, `478`, `551`.
- `agents-plugin-tool/internal/mcp/proceed_resolver.go:353,355` — **behavior-
  visible** formatted strings returned by `enter.proceed` that literally tell
  the caller to `Call %s/playbook.print(name: "lead-implement")` /
  `"lead-write-ticket"`. Must become `%s/playbook.read(...)`.
- `agents-plugin-tool/internal/mcp/workflow_manual.go:20,234` — comments only.
- `agents-plugin-tool/internal/wsconfig/scope.go:56`,
  `agents-plugin-tool/internal/wsdoc/tickets_scope.go:410`,
  `agents-plugin-tool/internal/wsdoc/legacy_marker.go:177`,
  `agents-plugin-tool/internal/wsnote/inject.go:112,115`,
  `agents-plugin-tool/internal/mcp/note_announcement.go:10`,
  `agents-plugin-tool/internal/mcp/playbook_tools.go:309,515` — comments only,
  safe mechanical rename.
- `agents-plugin-tool/internal/mcp/note_tools_test.go` (56 hits),
  `server_test.go` (36), `playbook_tools_test.go` (25),
  `note_workflow_manual_test.go` (9), `main_test.go` (6),
  `tickets_scope_test.go` (6), `prompt_override_test.go` (5),
  `main.go:136,145` (smoke-test JSON-RPC fixtures),
  `prefer_mercenary_phase2_test.go` (4), `session_state_test.go` (3),
  `legacy_marker_render_test.go`, `panic_recovery_test.go`,
  `wsdoc/legacy_marker_test.go`, `wsdoc/spec_discovery_test.go`,
  `wsrsrc/skills_mirror_test.go` (3), `wsrsrc/wsrsrc_test.go` (1) — literal
  tool-name-string test fixtures/assertions, mechanical rename, no logic
  change expected.
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go` — structural
  drift-guard test; contains **no** literal old-name strings itself (verified),
  no edit needed, but its regenerated outputs (see below) must be re-run.
- `agents-plugin/runtime.json` — `"tools"` section keys at lines `9` (
  `runtime.info`), `32` (`note.search`), `41` (`specs.find`), `44`
  (`mental_models.find`), `48` (`tickets.find`), `63` (`playbook.print`) —
  **required** rename: `LeadToolNames()` (`server.go:4557`) derives the tool
  list dynamically from `tools()`'s `"name"` field via `advertisedToolName`
  (no-op passthrough, `server.go:4666`), so once registration is renamed, this
  file's `"tools"` section must match exactly or
  `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`
  (`main_test.go:51-104`) fails. The `"commands"` section (lines `73`, `102`,
  `109`) is **out of scope** — see Out of Scope.
- `agents-plugin-wsflow/runtime.json` — mirrors the same `"tools"`-section
  keys; also curated (not generated), needs the same direct edit. Verified by
  `TestRuntimeCapabilitiesCommandReportsNoAgentSurface` (`main_test.go:106+`)
  and `main_test.go:148-170` (reads this file directly as
  `readRuntimeContractAtTest(..., "agents-plugin-wsflow", "runtime.json")`).
- `ai-docs/spec/mcp-tools.md` — 28 occurrences; six target anchors:
  `{#260505-runtime-debug-metadata-tools}` (line 67),
  `{#260810-note-tools}` (line 448, plus cross-refs at 838, 876, 2029),
  `{#260505-spec-discovery-tools}` (line 1177),
  `{#260505-ticket-discovery-tools}` (line 1227),
  `{#260505-mental-model-discovery-tools}` (line 1738),
  `{#260609-playbook-tools}` (line 2065, plus cross-ref at 2379). No
  `{#slug}` heading text changes — only in-prose tool-name token swaps.
- `ai-docs/spec/workflow-skills.md` — 6 occurrences (locations found via
  `references.trace`/grep at implementation time, per ticket's own note that
  the sweep enumerates sites).
- `agents-plugin/rsrc/**/*.md` — 30 occurrences across 15 canonical playbook
  files (`code-reviewer.md`, `doc-gap-discovery/doc-gap-discovery.md`,
  `executor-wrapup.md`, `impl-playbook.md`, `lead-backfill-docs/*.md`,
  `lead-discuss/*.md`, `lead-forge-spec/*.md`, `lead-goal-fan-out-step/*.md`,
  `lead-implement/*.md`, `lead-workflow-manual/*.md`, `lead-write-ticket/*.md`,
  `plan-populator-research/*.md`, `plan-populator-survey/*.md`,
  `reference-discovery/*.md`, `ticket-reviewer-design/*.md`). Edit these
  canonical sources directly; **never** hand-edit
  `agents-plugin-wsflow/rsrc/` — it is a generated byte-identical mirror
  (procedure below).
- `agents-plugin/skills/*/SKILL.md` (curated shim files, not generated) call
  `ws/playbook.print(name: "<lead-name>")` — e.g.
  `agents-plugin/skills/lead-proceed/SKILL.md` (verified content: `Call
  ws/playbook.print(name: "lead-proceed")...`). Rename the call site to
  `ws/playbook.read(...)` in each shim. Same pattern applies to
  `lead-add-rule`, `lead-backfill-docs`, `lead-bootstrap`, `lead-discuss`,
  `lead-forge-mental-model`, `lead-forge-spec`, `lead-goal-fan-out-step`,
  `lead-review`, `lead-scope-worktree`, `lead-ship`, `lead-tune`,
  `lead-write-ticket`.
- `agents-plugin-wsflow/skills/*/SKILL.md` — same shim pattern with
  `wsflow/playbook.print(...)`, curated independently **except** for the four
  substitution-mirrored skills (`lead-prefer-subagent`,
  `lead-verify-discussion`, `lead-drain-ready-queue`, `mcp-server-repair`),
  which are generated from the matching `agents-plugin/skills/<name>/SKILL.md`
  source — edit only the `ws` source for those four, then regenerate (do not
  hand-edit the wsflow copy).
- `agents-plugin/tests/test_skill_dispatch_contracts.py:14` — **required**:
  `self.assertIn('ws/playbook.print(name: "lead-proceed")', shim)` must become
  `'ws/playbook.read(name: "lead-proceed")'` to match the renamed shim, or the
  test fails against real content.
- `agents-plugin/tests/test_skill_dispatch_contracts.py:63,73` — `assertNotIn`
  checks (currently checking for absence of `'ws/playbook.print(name:
  "lead-verify-discussion")'` / `"lead-drain-ready-queue")'`) should also be
  updated to the new-name string to keep testing the intended invariant (no
  playbook-read indirection for these inline skills); not required to pass,
  but required to keep testing the right thing.
- `ai-docs/manuals/wsflow-mirroring.md` — governs regen order/commands (read,
  not edited by this phase): `WSRSRC_REGEN=1 go test ./internal/wsrsrc/...
  -count=1 -run TestGenerateRealManifest` (regens
  `agents-plugin/rsrc/manifest.json` after any canonical rsrc edit), then
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run
  TestRegenerateWsflowRsrcMirror` (syncs `agents-plugin-wsflow/rsrc/`
  byte-for-byte). Separately, `WSRSRC_REGEN_SKILLS=1 go test
  ./internal/wsrsrc/... -count=1 -run TestGenerateRealSkillsManifest` after any
  `agents-plugin/skills/` edit. For the four substitution-mirrored skills:
  compose first (`WS_REGEN_COMPOSED_SKILLS=1 go test ./internal/wsrsrc
  -count=1 -run TestRegenerateComposedSkills`), then mirror
  (`WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run
  TestRegenerateWsflowSkillsMirror`). All `-count=1` flags are mandatory (test
  cache otherwise skips the write side effect).

## Implementation Plan

1. Freeze the six literal token pairs (old → new, all safe dotted-name forms
   with no substring collision risk observed): `tickets.find`→`tickets.query`,
   `specs.find`→`specs.query`, `mental_models.find`→`mental_models.query`,
   `note.search`→`note.query`, `playbook.print`→`playbook.read`,
   `runtime.info`→`runtime.read`.
2. Script a literal-string find-and-replace of the six pairs across:
   `agents-plugin-tool/**/*.go` (registration, dispatch, error strings, the
   `const tool = "note.search"` local constant, comments — all 21+ sites in
   `server.go` plus the files listed in Codebase Findings), excluding no Go
   file (mechanical rename is uniform; comments included for no-stale-text).
3. Update `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`
   `"tools"` sections only (six keys each); leave `"commands"` sections
   untouched (Out of Scope).
4. Update `ai-docs/spec/mcp-tools.md` and `ai-docs/spec/workflow-skills.md`:
   rename literal tool-name tokens in prose at the six anchors and their
   cross-references; do not touch `{#slug}` heading text.
5. Update the 15 canonical `agents-plugin/rsrc/**/*.md` playbook files (30
   occurrences) with the same token swap. Then run, in order:
   `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run
   TestGenerateRealManifest` and `WS_REGEN_WSFLOW_RSRC=1 go test
   ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`. Do not
   hand-edit `agents-plugin-wsflow/rsrc/`.
6. Update `agents-plugin/skills/*/SKILL.md` shim call sites
   (`ws/playbook.print(...)` → `ws/playbook.read(...)`) for the 13 curated
   shims listed in Codebase Findings, plus the corresponding curated
   `agents-plugin-wsflow/skills/*/SKILL.md` shims
   (`wsflow/playbook.print(...)` → `wsflow/playbook.read(...)`) — **except**
   the four substitution-mirrored skills, where only the `agents-plugin`
   source is edited. Then run
   `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -count=1 -run
   TestGenerateRealSkillsManifest`, then (for the four mirrored skills, in
   order) `WS_REGEN_COMPOSED_SKILLS=1 go test ./internal/wsrsrc -count=1 -run
   TestRegenerateComposedSkills` and `WS_REGEN_WSFLOW_SKILLS=1 go test
   ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`.
7. Update all `_test.go` files under `agents-plugin-tool/` containing the old
   literal names (fixtures/assertions listed in Codebase Findings) with the
   same token swap.
8. Update `agents-plugin/tests/test_skill_dispatch_contracts.py:14,63,73` to
   match the renamed shim call sites.
9. Thin human prose-cleanup pass: reword the affected tools' own
   `"description"` strings that use "Find"/"Search"/"Print"/"Info" as their
   leading verb (e.g. `specs.find`'s "Find spec files by query..." →
   "Query spec files by..."), and any surrounding sentence fragments the
   mechanical token swap would otherwise leave awkward. Scope this to the six
   renamed tools' own descriptions and direct cross-references found in step
   4/5/6 — do not touch unrelated prose.
10. Run a `references.trace`/grep sweep for the six old literal tokens scoped
    to `agents-plugin-tool/`, `agents-plugin/`, `agents-plugin-wsflow/`
    (excluding `ai-docs/.plans/**`, `ai-docs/tickets/.done/**`,
    `ai-docs/tickets/.dropped/**`, and — per Out of Scope —
    `runtime.json`'s `"commands"` sections and the CLI dispatch
    `case "find":` sites in `main.go`) and confirm it is clean.

## Verification Plan

- `go test ./...` run inside `agents-plugin-tool/` — must be green (covers
  Go unit/integration tests including `main_test.go`'s runtime-contract
  checks and `wsrsrc`'s mirror/manifest drift guards).
- `python3 -m unittest discover agents-plugin-wsflow/tests` — covers the
  wsflow runtime-contract and distributed-skill-bundle checks
  (`ai-docs/manuals/wsflow-mirroring.md` "Static Verification").
- `python3 -m unittest discover agents-plugin/tests` (or the specific
  `test_skill_dispatch_contracts.py` / `test_ws_mcp_launcher_capabilities.py`
  modules) — covers the shim-content and launcher-capability contract checks.
- Manual/tool-level: start `ws-mcp` and confirm `tools/list` shows the six
  canonical verbs (`tickets.query`, `specs.query`, `mental_models.query`,
  `note.query`, `playbook.read`, `runtime.read`) and no old names.
- `references.trace`/grep sweep (step 10 above) clean within the scoped
  in-package surfaces.

## Escalations

- None. Confidence is high enough to proceed directly to implementation; the
  two judgment calls made above (CLI-command-surface exclusion, and mental-
  model/manual docs left to a follow-up pass) are both low-risk, internally
  self-consistent, and flagged in Out of Scope for the executor/lead to
  override if they disagree — neither blocks or risks the Phase 1 acceptance
  criteria as written.

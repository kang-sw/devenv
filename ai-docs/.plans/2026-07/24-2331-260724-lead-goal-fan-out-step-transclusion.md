# Plan: 260724-feat-lead-fan-out-worktree — Phase 2: lead-goal-fan-out-step entry skill + goal-step transclusion

## Relevant Ticket Contract

- Deliverable A (transclusion branch): generalize `printPlaybook`'s existing
  `lead-workflow-manual`/`prefer_subagent` `LoadSkillBody` +
  `wrapRenderedPlaybookForConcatenation` precedent
  (`agents-plugin-tool/internal/mcp/playbook_tools.go:889-914`) so that when
  serving `lead-goal-fan-out-step`, after normal render+substitute, it
  `LoadSkillBody(skillsRoot, "lead-goal-step")` and appends via
  `wrapRenderedPlaybookForConcatenation("lead-goal-step", "Goal Step", body)`,
  producing a visible `<playbook name="lead-goal-step" title="Goal Step">…</playbook>`
  boundary.
- Append is **post-substitution** (after `substitutePlaybookVars`), matching the
  precedent, because the transcluded goal-step body is static prose with no
  `{{.` placeholders and must not trip the undeclared-var guard.
- Unlike the `lead-prefer-subagent` precedent, this new branch is **unconditional
  on the skill name** — no config-flag gate. Confirmed from ticket text ("Body
  doctrine... require recursive native dispatch" is *runtime* behavior described in
  the skill body, not a serve-time config toggle) and from the Decisions section:
  there is no mention of a config knob controlling whether the transclusion
  happens; it happens whenever `lead-goal-fan-out-step` is served, period.
- `lead-goal-step` lives **only** in the skills tree
  (`agents-plugin/skills/lead-goal-step/SKILL.md`), not in `rsrc/` — use
  `wsrsrc.ResolveSkillsRoot()` + `wsrsrc.LoadSkillBody`, exactly like the existing
  branch does for `lead-prefer-subagent`. Accept it is NOT rsrc-manifest
  hash-verified (documented trade-off already lived with).
- Deliverable B (entry skill): thin `SKILL.md` shim at
  `agents-plugin/skills/lead-goal-fan-out-step/SKILL.md` (workflow_manual +
  `playbook.print` guidance, same shape as `lead-goal-step`/`lead-discuss`
  SKILL.md). The rsrc overlay body already exists and is finalized at
  `agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md`
  (frontmatter `kind: print`, `delegates: true`). Do not rewrite its doctrine.
- Add a hash entry for `lead-goal-fan-out-step/lead-goal-fan-out-step.md` to
  `agents-plugin/rsrc/manifest.json`, and a hash entry for the new
  `lead-goal-fan-out-step/SKILL.md` to `agents-plugin/skills/manifest.json` —
  both via the Go regen tests (see Codebase Findings), not hand-computed hashes.
- Register `lead-goal-fan-out-step` as an entry skill: add to the namespace list
  fenced block in `ai-docs/spec/workflow-skills.md`
  `#260505-lead-skill-namespace-surface`, and bump the directly-invocable count
  (currently "14 entry skills") at `#260610-entry-skill-surface-reduction`.
- Tests required: serving `lead-goal-fan-out-step` yields the overlay followed by
  the wrapped goal-step body; the `<playbook name="lead-goal-step">` boundary tag
  is present; a change to `lead-goal-step` SKILL.md is reflected in served output
  (lockstep) — follow the `TestPlaybookPrintWsflowProductModeFiltersHiddenGuidance`
  / `prefer_mercenary_phase2_test.go` transclusion test patterns.
- Spec updates in scope for this phase (doc pre-pass): `ai-docs/spec/plugin-runtime.md`
  (new note on the serve-time skill-body transclusion mechanism, generalizing the
  `lead-prefer-subagent` precedent — currently **absent** from this file entirely)
  and `ai-docs/spec/workflow-skills.md` (new `lead-goal-fan-out-step` entry near the
  `lead-goal-step` cluster, namespace-list registration, count bump).
- `session.note` (Phase 1) is already shipped — no work needed here; do not touch
  `ai-docs/spec/mcp-tools.md`'s Phase-1 content.

## Out of Scope

- Phase 1 (`session.note` MCP tool) — done, see ticket "Result (c522438c)".
- Phase 3 (wsflow exposure verification + mirror, `agents-plugin-wsflow` runtime
  capability probing, `python3 -m unittest discover agents-plugin-wsflow/tests`) —
  explicitly deferred; do not edit `agents-plugin-wsflow/` in this phase.
- The batch-parallel fan-out *runtime behavior itself* (worktree creation,
  `ferrule` minting, mini-lead dispatch, merge subagents) — this is all already
  authored prose in the finalized
  `agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md` body; this
  phase only wires it into the serving/registration surface. Do not edit that
  file's doctrine.
- `substitutionMirroredSkills` list in
  `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go:15-19` — that list is
  for *inline-body* skills with no rsrc playbook (`lead-goal-step`,
  `lead-prefer-subagent`, `lead-verify-discussion`). `lead-goal-fan-out-step` has
  an rsrc playbook (`kind: print`) and a normal thin shim, so it follows the
  ordinary rsrc-mirror + ws/wsflow substitution path, not this list. Do not add it
  there.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` `EXPECTED_SKILLS` and any
  other wsflow-side inventory — Phase 3 territory per the ticket's own Phase 3
  section ("wsflow-mirrored `lead-goal-step` SKILL.md... verified in Phase 3").
- The mental-model count in `ai-docs/mental-model/workflow-skills.md:17` ("16
  directly user-invocable... entry skills") uses a different counting convention
  (includes `lead-prefer-subagent`/`lead-revive`) than the spec's "14 entry
  skills" list. The ticket's Spec Impact section only names
  `ai-docs/spec/workflow-skills.md`, not the mental-model file. Bump only the spec
  count/list; leave the mental-model file alone unless a later drift pass touches
  it (flagged as a minor consistency note, not required by this phase's contract).

## Codebase Findings

- `agents-plugin-tool/internal/mcp/playbook_tools.go:452-455` — constants for the
  existing precedent: `workflowManualPlaybookName = "lead-workflow-manual"`,
  `preferSubagentPlaybookName = "lead-prefer-subagent"`,
  `preferSubagentPlaybookTitle = "Prefer Subagent"`,
  `preferSubagentEnabledValue = "on"`. Add parallel constants for the new branch,
  e.g. `goalStepPlaybookName = "lead-goal-step"`,
  `goalStepPlaybookTitle = "Goal Step"`, and a name constant for the new serving
  skill, e.g. `goalFanOutStepPlaybookName = "lead-goal-fan-out-step"` (match
  existing naming style; exact identifiers are the implementer's call).
- `agents-plugin-tool/internal/mcp/playbook_tools.go:889-914` — the exact
  `printPlaybook` function to generalize:
  ```go
  func printPlaybook(s *Server, rsrcRoot, name string, callerContext map[string]string, configOpts wsconfig.Options, workflowLang string, overrideLookup overrideLookupFn) (string, string, error) {
      body, recommendedTier, err := renderPlaybookBody(s, rsrcRoot, name, callerContext, configOpts, "", "", false, workflowLang, overrideLookup)
      if err != nil {
          return "", "", err
      }
      if name != workflowManualPlaybookName {
          return body, recommendedTier, nil
      }
      enabled, err := workflowPreferSubagentEnabled(configOpts)
      ...
      skillsRoot, err := wsrsrc.ResolveSkillsRoot()
      ...
      appendBody, err := wsrsrc.LoadSkillBody(skillsRoot, preferSubagentPlaybookName)
      ...
      body += "\n\n" + wrapRenderedPlaybookForConcatenation(preferSubagentPlaybookName, preferSubagentPlaybookTitle, appendBody)
      return body, recommendedTier, nil
  }
  ```
  Insertion point for the new branch: immediately after the existing
  `if name != workflowManualPlaybookName { return body, recommendedTier, nil }`
  early-return / prefer-subagent block, add a second, independent branch keyed on
  `name == "lead-goal-fan-out-step"` that is unconditional (no config-flag check)
  — mirror the skills-root-resolve + `LoadSkillBody` + append shape but skip the
  `workflowPreferSubagentEnabled` gate entirely. Both branches can coexist as two
  sequential `if` blocks over the same `body` variable since the gates are
  mutually exclusive on `name`.
- `agents-plugin-tool/internal/mcp/playbook_tools.go:857-867` —
  `wrapRenderedPlaybookForConcatenation(name, title, body string) string` signature
  (trims trailing newline from body, HTML-escapes name/title, emits
  `<playbook name="%s" title="%s">\n%s\n</playbook>`). Reuse as-is.
- `agents-plugin-tool/internal/wsrsrc/loader.go:49` — `func ResolveSkillsRoot() (string, error)`
  (honors `WS_SKILLS_ROOT` env, else `filepath.Dir(os.Executable())/../skills`).
  `agents-plugin-tool/internal/wsrsrc/loader.go:64` — `func LoadSkillBody(root, name string) (string, error)`
  (reads `<root>/<name>/SKILL.md`, strips frontmatter, returns body; explicitly
  not manifest-hash-verified — same accepted trade-off).
- `agents-plugin/skills/lead-goal-step/SKILL.md:1-4` — template shape for the new
  thin shim frontmatter:
  ```
  ---
  name: lead-goal-step
  description: Advance a goal-pursuit run by one step, picking the next `ready/` ticket and handing it to lead-proceed; `ready/` is the sole progress gate. Stop if ready/ is empty.
  ---
  ```
  followed by a `# Goal Step` heading and full inline body (this skill happens to
  carry its whole procedure inline, no `playbook.print` call, because it's on the
  `substitutionMirroredSkills` list). The **structurally closer** template is
  `agents-plugin/skills/lead-discuss/SKILL.md` (full text): frontmatter
  `name:`/`description:`, then a body that calls `ws/playbook.print(name: "lead-discuss", ...)`
  and `ws/workflow_manual(...)` in parallel and says "execute the procedure
  returned by playbook.print" — this is the thin-shim shape the ticket wants for
  `lead-goal-fan-out-step` (rsrc `kind: print` body already exists, so the SKILL.md
  must route to it via `playbook.print`, not inline the procedure).
- `agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md` — already
  exists, finalized, frontmatter `kind: print`, `delegates: true`. Full
  batch-parallel overlay body (degenerate-to-serial, batch selection, dispatch,
  board convention, merge). Do not edit; only wire it in via manifest + shim +
  transclusion branch.
- `agents-plugin/rsrc/manifest.json` — currently has **no entry** for
  `lead-goal-fan-out-step/lead-goal-fan-out-step.md` (confirmed via grep), so
  `TestShippedManifestUpToDate` (below) is presently red on this file alone (the
  rsrc body was pre-drafted ahead of manifest registration). Alphabetical
  insertion point is between the `lead-forge-spec/...` (line 21) and
  `lead-implement/...` (line 22) entries.
- `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go` — the rsrc-tree
  drift/regen mechanism:
  - `TestShippedManifestUpToDate` (line 24) — drift gate; `reflect.DeepEqual`s a
    freshly generated manifest against the committed `agents-plugin/rsrc/manifest.json`.
  - `TestRegenerateShippedManifest` (line 93) — the regen mechanism, **no-op
    unless `WS_REGEN_MANIFEST=1`**. Exact command (from `agents-plugin-tool/`):
    `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`.
    This is how the new rsrc hash entry must be produced — do not hand-compute a
    sha256.
- `agents-plugin-tool/internal/wsrsrc/skills_manifest_test.go` — the **separate**,
  independent `agents-plugin/skills/manifest.json` drift/regen mechanism (hashes
  SKILL.md files, no rsrc semantics):
  - `TestSkillsManifestDriftIsVisible` (line 29) — drift gate.
  - `TestGenerateRealSkillsManifest` (line 55) — regen, gated by
    `WSRSRC_REGEN_SKILLS=1` (distinct env var from the rsrc one, deliberately, per
    the file's own comment). Exact command:
    `cd agents-plugin-tool && WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v`.
  Both regen tests must be run after creating the new SKILL.md and confirming the
  rsrc file's presence, so both manifests pick up the new file hashes in one pass.
- `agents-plugin/skills/manifest.json:12` — existing entry format example:
  `"lead-goal-step/SKILL.md": "226ea400e920a46c5ceb9a9e5ae78e6c775fa5d75d30e4c43f287e18b9a7f7e1"`
  — confirms this manifest is SKILL.md-only (no per-skill subdirectory files like
  `agents/openai.yaml` unless present, as seen for `lead-discuss` at line 9).
- Entry-skill registration surfaces (spec doc, code-side, no Go test asserts the
  literal count number):
  - `ai-docs/spec/workflow-skills.md:23-44` — the fenced namespace list; `lead-goal-step`
    is at line 29 (alphabetically between `lead-forge-spec` and `lead-implement`).
    Insert `lead-goal-fan-out-step` alphabetically adjacent (it sorts before
    `lead-goal-step` lexicographically: `lead-goal-fan-out-step` < `lead-goal-step`
    since `f` < `s`) — i.e. immediately before line 29.
  - `ai-docs/spec/workflow-skills.md:52-56` — the "14 entry skills" prose sentence
    naming all 14 by backtick-name, ending `..., and `lead-goal-step`.`. Add
    `lead-goal-fan-out-step` to this list and change "14" to "15". Anchor
    `#260610-entry-skill-surface-reduction` sits at line 70 (end of that
    paragraph block spanning lines 57-70) — the whole paragraph is the bump
    target, not just the number.
  - `ai-docs/spec/workflow-skills.md` `lead-goal-step`'s full documented entry
    lives under `## Planning Workflow Skills` starting around line 435 (prose
    block through ~line 545, with sub-anchors like
    `#260723-lead-goal-step-rename-reposition`,
    `#260723-goal-step-ticket-curation-authority`,
    `#260723-goal-step-blocked-progress-conclusion`). The ticket asks for a new
    `lead-goal-fan-out-step` entry "adjacent to the lead-goal-step cluster" —
    insert a new prose block immediately after this cluster (before whatever
    follows at ~line 547), describing it as a batch-parallel goal-step variant
    that transcludes goal-step's contract via the `printPlaybook` mechanism.
  - No Go test enforces the literal "14"/"15" count string — confirmed via grep;
    the count lives only in spec prose. No code change required for the count
    itself beyond the spec edit.
  - `ai-docs/mental-model/workflow-skills.md:17` uses a **different** counting
    convention ("16 directly user-invocable... entry skills", includes
    `lead-prefer-subagent`/`lead-revive`) tagged with the same anchor id. This is
    an existing cross-doc inconsistency, not something this phase's ticket
    contract asks to reconcile (Spec Impact only names
    `ai-docs/spec/workflow-skills.md`) — noted as a pre-existing wrong-assumption
    risk (see report), not an in-scope fix.
- Transclusion test precedents to follow:
  - `agents-plugin-tool/internal/mcp/playbook_tools_test.go:768-837`
    (`TestPlaybookPrintWsflowProductModeFiltersHiddenGuidance`) — calls
    `printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, configOpts, "", buildOverrideLookup(s, ""))`
    directly (unit-level), asserts the boundary tag
    `` `<playbook name="lead-prefer-subagent" title="Prefer Subagent">` `` is
    **absent** when the preference is off (line 816-818) and **present** plus
    containing `"Maximum-delegation posture for this session"` when on (lines
    829-836), toggling via `resolver.Set(wsconfig.ItemWorkflowPreferSubagent, "on", wsconfig.SetOptions{})`
    (line 821). For the new unconditional branch, the analogous test simply calls
    `printPlaybook(s, rsrcRoot, "lead-goal-fan-out-step", ...)` once (no
    toggle needed) and asserts the boundary tag
    `<playbook name="lead-goal-step" title="Goal Step">` is present, plus a
    known-unique substring from `lead-goal-step`'s current body (e.g.
    `"Goal-pursuit step; \`ready/\` is the sole progress gate."` or
    `"One finished ticket is not a finished goal."`) to prove the actual file
    content flowed through, not a stub.
  - `agents-plugin-tool/internal/mcp/prefer_mercenary_phase2_test.go:281-350` —
    end-to-end MCP-tool-level precedent
    (`TestWorkflowPreferSubagentWorkflowManualPrintProductionPath`,
    `TestWorkflowPreferSubagentWorkflowManualClaudeGetsStaticSkillBody`): spins up
    `NewServer`, sets `WS_RSRC_ROOT`/`WS_SKILLS_ROOT` to the real
    `agents-plugin/rsrc`/`agents-plugin/skills` trees, calls the `playbook.print`
    tool and asserts on the returned text. Useful as the "lockstep" test pattern:
    the simplest reliable way to prove "a change to lead-goal-step SKILL.md is
    reflected in served output" without a temp-file mutation fixture is to
    separately call `wsrsrc.LoadSkillBody(skillsRoot, "lead-goal-step")` in the
    test itself and assert the served `lead-goal-fan-out-step` output's appended
    block equals (or contains) exactly that freshly-read body — since both sides
    read the same live file, any future edit to it changes both assertions
    identically, which is what "lockstep" means here.
  - A second direct-load precedent exists at `playbook_tools_test.go` (~line
    1813-1823) for `lead-verify-discussion` using
    `wsrsrc.LoadSkillBody(skillsRoot, "lead-verify-discussion")` directly — same
    pattern, useful if a standalone `LoadSkillBody` unit-check (independent of
    `printPlaybook`) is wanted for the new branch too.
- Build/test commands (from `ai-docs/_index.md` / ticket's own prior-phase
  verification): from `agents-plugin-tool/` — `go build ./...`,
  `go test ./internal/mcp/...`, `go test ./cmd/ws-mcp/...`, `go vet ./...`.
  wsflow python tests are explicitly Phase 3 / out of scope, but must not be
  broken incidentally — this phase does not touch `agents-plugin-wsflow/` at all,
  so no incidental breakage is expected.

## Implementation Plan

1. In `agents-plugin-tool/internal/mcp/playbook_tools.go`, add constants near
   the existing ones at line 452-455 for the new transclusion target (e.g.
   `goalFanOutStepPlaybookName = "lead-goal-fan-out-step"`,
   `goalStepPlaybookName = "lead-goal-step"`, `goalStepPlaybookTitle = "Goal Step"`).
2. In `printPlaybook` (lines 889-914), after the existing
   `lead-workflow-manual`/`prefer_subagent` block returns unchanged for all other
   names, add a second unconditional branch: `if name == goalFanOutStepPlaybookName { ... }`
   that resolves `wsrsrc.ResolveSkillsRoot()`, `wsrsrc.LoadSkillBody(skillsRoot, goalStepPlaybookName)`,
   and appends `"\n\n" + wrapRenderedPlaybookForConcatenation(goalStepPlaybookName, goalStepPlaybookTitle, appendBody)`
   to `body` before returning — mirroring the existing branch's error-wrapping
   style (`fmt.Errorf("resolve skills root for appended %s: %w", ...)` /
   `fmt.Errorf("load appended %s: %w", ...)`). Keep this branch independent of
   `workflowPreferSubagentEnabled` — no config check.
3. Create `agents-plugin/skills/lead-goal-fan-out-step/SKILL.md` following the
   `lead-discuss/SKILL.md` thin-shim shape: frontmatter `name: lead-goal-fan-out-step`
   and a `description:` (host-neutral, e.g. reusing/adapting the ticket's own
   framing: "Advance a goal-pursuit run by one step with batch-parallel worktree
   fan-out when two or more ready tickets are mutually independent and recursive
   subagent dispatch is available; falls back to lead-goal-step's serial single-
   ticket step otherwise."), then a body calling
   `ws/playbook.print(name: "lead-goal-fan-out-step", session_key: <your key, omit if fresh>)`
   and `ws/workflow_manual(...)` in parallel, executing the returned procedure —
   same two lines as `lead-discuss/SKILL.md`.
4. Regenerate both manifests from `agents-plugin-tool/`:
   - `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
     (picks up the already-existing `rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md`).
   - `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v`
     (picks up the new `skills/lead-goal-fan-out-step/SKILL.md`).
   Confirm both `agents-plugin/rsrc/manifest.json` and `agents-plugin/skills/manifest.json`
   gained exactly one new alphabetically-placed entry each, with no unrelated diff.
5. Update `ai-docs/spec/workflow-skills.md`:
   - Insert `lead-goal-fan-out-step` into the fenced namespace list (lines 23-44),
     immediately before `lead-goal-step` (line 29).
   - Update the "14 entry skills" sentence (lines 52-56) to "15 entry skills" and
     add `` `lead-goal-fan-out-step` `` to the named list (place it adjacent to
     `lead-goal-step` in the enumeration for readability).
   - Add a new prose entry describing `lead-goal-fan-out-step` immediately after
     the `lead-goal-step` cluster (~after line 545), stating: it is a
     batch-parallel `lead-goal-step` variant; its full contract (goal-run
     posture, terminal states, staging, continuation) is transcluded verbatim at
     serve time via the `printPlaybook` mechanism (cross-reference the
     `lead-prefer-subagent` precedent and the new branch); the overlay adds only
     batch selection + worktree/mini-lead dispatch + serial merge; give it its
     own anchor id following the file's `{#YYMMDD-slug}` convention (implementer
     picks the exact date prefix/slug per the file's existing convention).
6. Update `ai-docs/spec/plugin-runtime.md`: add a short new paragraph/subsection
   documenting the serve-time skill-body transclusion mechanism in `printPlaybook`
   — describe it generally (a code-side append of a `LoadSkillBody`-read skills-
   tree body, wrapped in a visible `<playbook name=... title=...>` boundary,
   applied for two cases: `lead-workflow-manual`+`prefer_subagent` gated, and
   `lead-goal-fan-out-step` unconditional) so the mechanism itself — not just the
   two call sites — has one documented home. Follow the prose style of the
   existing `#260506-runtime-capabilities-single-probe` /
   `#260626-post-compaction-session-restoration` paragraphs (mechanism +
   cross-reference anchors), and give the new paragraph its own anchor id.
7. Add Go tests in `agents-plugin-tool/internal/mcp/playbook_tools_test.go`
   (or a new adjacent test function near
   `TestPlaybookPrintWsflowProductModeFiltersHiddenGuidance`), following that
   test's direct-`printPlaybook`-call pattern:
   - Call `printPlaybook(s, rsrcRoot, "lead-goal-fan-out-step", nil, configOpts, "", buildOverrideLookup(s, ""))`.
   - Assert the returned body contains `<playbook name="lead-goal-step" title="Goal Step">`.
   - Assert it also contains a known-stable substring copied from
     `lead-goal-step/SKILL.md`'s current body (e.g.
     `"Goal-pursuit step; \`ready/\` is the sole progress gate."`) to prove real
     content, not a stub.
   - Add a "lockstep" check: separately call
     `wsrsrc.LoadSkillBody(skillsRoot, "lead-goal-step")` in the test and assert
     the served output's appended block equals (or contains) exactly that
     freshly-read body, so any future edit to `lead-goal-step/SKILL.md` is
     automatically reflected without needing to update this test's fixture text.
8. Run verification (see Verification Plan) and fix any drift before considering
   the phase done. Do not touch Phase 3 / wsflow files.

## Verification Plan

- `cd agents-plugin-tool && go build ./...` — clean.
- `cd agents-plugin-tool && go vet ./...` — clean.
- `cd agents-plugin-tool && go test ./internal/wsrsrc/... -count=1` — both
  `TestShippedManifestUpToDate` and `TestSkillsManifestDriftIsVisible` green
  after the two regen commands in Implementation step 4.
- `cd agents-plugin-tool && go test ./internal/mcp/... -count=1` — new
  transclusion tests green, plus no regression in the existing
  `TestPlaybookPrintWsflowProductModeFiltersHiddenGuidance` /
  `TestWorkflowPreferSubagentWorkflowManualPrintProductionPath` /
  `TestWorkflowPreferSubagentWorkflowManualClaudeGetsStaticSkillBody`.
- `cd agents-plugin-tool && go test ./cmd/ws-mcp/... -count=1` — exact-match
  runtime-surface tests still green (confirms the new SKILL.md/manifest entries
  didn't break the ws-side runtime capability surface; wsflow's own
  `runtime.json` is untouched in this phase, so its exact-match test is
  unaffected here — that reconciliation is Phase 3).
- Manual spot-check: `ws/playbook.print(name: "lead-goal-fan-out-step")` (or the
  equivalent direct `printPlaybook` call in a scratch test) renders with correct
  namespace substitution and the appended `<playbook name="lead-goal-step">`
  block — this is the ticket's own stated Phase 2 verification boundary,
  satisfied by the automated tests above rather than requiring a live harness
  round-trip.
- Confirm `git status` / `git diff --stat` after implementation touches only:
  `playbook_tools.go`, the new `playbook_tools_test.go` additions,
  `agents-plugin/skills/lead-goal-fan-out-step/SKILL.md` (new),
  `agents-plugin/rsrc/manifest.json`, `agents-plugin/skills/manifest.json`,
  `ai-docs/spec/workflow-skills.md`, `ai-docs/spec/plugin-runtime.md` — no
  incidental edits to `agents-plugin/rsrc/lead-goal-fan-out-step/lead-goal-fan-out-step.md`
  (doctrine frozen) or any `agents-plugin-wsflow/` file (Phase 3).
- The ticket's own Phase 2 "Verification" paragraph also describes a **batch≥2
  end-to-end dry run** (two real worktrees, two mini-leads, session.note/children
  round-trip, serial merges) — this is explicitly framed as "prototype-validate"
  runtime-behavior validation of the *already-drafted* rsrc body, not a
  build-verification step for this wiring phase's file changes. It is out of
  scope for this survey-level plan's automated verification; flag it to the lead
  as a separate manual/live-run activity if desired after this phase's code
  lands.

## Escalations

- None.

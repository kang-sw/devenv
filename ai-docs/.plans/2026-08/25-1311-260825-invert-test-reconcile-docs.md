# Plan: 260825-refactor-ws-wsflow-bootstrap-artifact-convergence — Phase 4: Invert the guard test and reconcile specs/docs

## Relevant Ticket Contract

- Rewrite `test_bootstrap_template_uses_wsflow_local_version_lineage` to assert
  **convergence** for both scaffolded pairs (`AGENTS.template.md` and
  `WORKFLOW.md`): identical package-neutral emitted output modulo the version
  tag, and shared counter head matches (ticket L463-467).
- Comparison runs on the **emitted (fresh-mode) body**: apply the same strip
  `lead-bootstrap.md`'s fresh-mode step performs — strip only the
  `<!-- MIGRATION: ... delete this block -->` block and the
  `<!-- MIGRATION CHECKLIST ... -->` block; the Inclusion-test comment is
  **not** stripped (ticket L468-476).
- Add coverage for the fail-loud guard's code-level detection; the
  skill-instruction refuse itself is documentation-asserted, not unit-tested
  (ticket L477-479).
- Update spec anchors `workflow-skills.md {#260513-wsflow-agentless-skill-surface}`
  and `mcp-tools.md {#260703-bootstrap-staleness-warning}` per `## Spec Impact`
  (ticket L207-222, L480).
- `plugin-runtime.md {#260513-wsflow-agentless-plugin-package}` and
  `claude-compatibility.md {#260513-wsflow-claude-compatible-package}` need no
  change (ticket L219-222 — confirmed by reading both anchors, neither carries
  template-version-lineage text).
- Record the override of `260728-research-parallel-workflow-guide-divergence`'s
  Non-Scope for **both** pairs (ticket Constraints L185-193, Phase 4 body L480-481).
- Update `wsflow-mirroring.md` Bootstrap Template Rules L291-292 without
  contradicting the Phase-1-added Artifact neutrality invariant / Enforceable
  corollary (L296-304), and add the skills-manifest-regen documentation item
  (ticket Phase 4 body, deliverable D).
- Invert mental-model `workflow-skills.md` L85, L110, L119 (ticket Phase 4 body,
  deliverable E).
- Two version axes stay decoupled: the migration ordinal is never sourced from
  the plugin `X.Y.Z` edition (ticket Constraints L194-201) — no code touches
  this in Phase 4, only confirm docs don't imply coupling.
- Verification boundary: inverted test fails on any re-introduced
  fingerprint/drift in either pair and on a counter split; specs no longer
  assert package-local lineage; `python3 -m unittest discover
  agents-plugin-wsflow/tests` green (ticket L483-485).
- Phase 4 depends on Phases 1-3 (all complete on `impl/develop/bootstrap-artifact-converge`,
  confirmed via ticket Results at L273-459). This is the ticket's final phase;
  the branch merges as one unit after this phase (ticket L458-459, L373-374).

## Out of Scope

- Any code change to `agents-plugin-tool/internal/mcp/bootstrap_alarm.go` or
  `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md` (Phase 3 done).
- Any `AGENTS.template.md`/`WORKFLOW.md` body or `v0047` counter change
  (Phase 1/2 done) — Phase 4 touches only the test file and docs/specs.
- Re-adding guard Go tests — `TestBootstrapStalenessWarningFiresOnAboveHeadTag`
  and `TestBootstrapStalenessWarningFiresOnUnparseableTag` already exist and
  are verified sufficient (see Codebase Findings).
- Moving `260728-research-parallel-workflow-guide-divergence`'s ticket status
  (`idea/` → `.done/`/`.dropped/`) — the ticket only requires a cross-reference
  note recording the override, not a status decision; leave status as-is.
- This repo's own `ai-docs/WORKFLOW.md` (the third copy `260728` names) is
  downstream generated output of the now-converged template pair, not a
  shipped template itself; Phase 4 does not add new drift tooling for it
  (matches Phase 1's Decision that `ai-docs/WORKFLOW.md` "is not hand-edited to
  diverge" — no separate action needed here).

## Codebase Findings

- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L231-239` — the
  current Phase-2-trimmed stub only asserts the tag marker string is present;
  it is the target of the rewrite. Method name
  `test_bootstrap_template_uses_wsflow_local_version_lineage` should be
  RENAMED (the old name asserts the opposite of the new behavior); confirmed
  via `grep -rn` that no other code (only historical ticket/plan prose in
  `ai-docs/tickets/.done/` and `ai-docs/.plans/`, which are frozen records) references
  this method name, so renaming is safe.
- `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md#L37-41` — `## On: fresh`
  step 1: "Copy template to `AGENTS.md`, stripping template-internal migration
  blocks." Two blocks qualify as scaffold-only per their own markers:
  `agents-plugin/skills/lead-bootstrap/AGENTS.template.md#L92-119`
  (`<!-- MIGRATION: Set up ai-docs/ for this project, then delete this
  block. ... -->`) and `#L127-239` (`<!-- MIGRATION CHECKLIST ... -->`, closes
  with header text "NEVER copy into a project AGENTS.md"). The Inclusion-test
  comment at `#L121-125` (`<!-- Inclusion test: ... -->`) carries no non-copy
  marker and is migration-`v0010`-permanent — confirmed it is NOT one of the
  two stripped blocks.
- Verified by direct `diff`: `agents-plugin/skills/lead-bootstrap/AGENTS.template.md`
  lines 1-91 and lines 120-126 (Inclusion-test block) are byte-identical to
  the wsflow copy; the two files diverge ONLY inside the MIGRATION and
  MIGRATION CHECKLIST comment blocks (wsflow substitutes `ws:`→`wsflow:`,
  `ws/`→`wsflow/` tokens and carries one extra equivalence-note paragraph);
  both tag lines read `<!-- Template Version: v0047 -->`.
- `diff agents-plugin/skills/lead-bootstrap/WORKFLOW.md
  agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md` — empty (fully
  byte-identical, confirmed live). This file has no scaffold-only strip
  blocks (Phase 1 Result, ticket L286-292), so the test compares it RAW, no
  transform needed.
- Verified via a local Python mutation script (regex
  `r'<!-- MIGRATION:.*?-->\n*'` and `r'<!-- MIGRATION CHECKLIST.*?-->\n*'`,
  both `re.DOTALL`, applied to both `AGENTS.template.md` raw texts): emitted
  bodies are byte-identical (4554 chars each) including the `v0047` tag; the
  Inclusion-test comment and tag survive the strip; a simulated counter split
  (`v0047`→`v0046` in one copy) and a simulated fingerprint reintroduction
  (adding `ws/note.write` text outside the stripped blocks) both make the
  post-strip comparison unequal — confirms the transform is neither too
  broad (over-stripping real content) nor too narrow (missing a scaffold
  block) and that the equality assertion is sensitive to both failure modes
  the ticket names. The regex is safe against the deliberately-escaped
  `--\>` inside checklist entry v0011's own prose (not a real close-comment).
- `agents-plugin-tool/internal/mcp/bootstrap_alarm_test.go#L165-198` —
  `TestBootstrapStalenessWarningFiresOnAboveHeadTag` asserts the above-head
  fire direction (message text, both version numbers, `config.tune` pointer,
  "code-level detector only" honest-enforcement text).
- `agents-plugin-tool/internal/mcp/bootstrap_alarm_test.go#L200-232` —
  `TestBootstrapStalenessWarningFiresOnUnparseableTag` asserts the
  unparseable-tag fire direction with the same coverage shape, distinct from
  the marker-absent silent case. Together these two tests fully cover
  deliverable B (fail-loud guard's code-level detection); ALREADY-SATISFIED
  in Phase 3 — no new Go test needed.
- `ai-docs/spec/workflow-skills.md#L253-257` — anchor
  `{#260513-wsflow-agentless-skill-surface}`, closing paragraph: "wsflow
  bootstrap uses package-local template version history. Its downstream
  `AGENTS.template.md` starts at `v0001` for the wsflow baseline and does not
  replay the full bootstrap migration backlog. Bootstrap behavior changes
  remain mirroring-sensitive: maintainers check both packages and bump each
  package's template version only when that package receives the behavior
  change." — needs full inversion to the shared-counter + package-neutral
  contract.
- `ai-docs/spec/mcp-tools.md#L672-703` — anchor
  `{#260703-bootstrap-staleness-warning}`. Current text (L680-682) says "The
  comparison is package-local: whichever package's MCP binary is running
  resolves its own shipped template... so there is no cross-package (ws vs
  wsflow) comparison." This sentence is still technically true (each binary
  still resolves its own shipped template file) but needs a new paragraph
  documenting the fail-loud above-head/unknown-tag direction added in Phase 3
  (`bootstrap_alarm.go`'s five-way branch), naming both new fire messages
  ("ahead of this package's own template head" / "Bootstrap template tag is
  unrecognized") and the honest-enforcement limit (detector-only, the refuse
  is skill-instruction-level per `lead-bootstrap.md`'s `## On: refuse`, not a
  mechanical block).
- `ai-docs/spec/plugin-runtime.md#L85-100` and
  `ai-docs/spec/claude-compatibility.md#L38-53` — read in full; confirmed
  neither anchor (`{#260513-wsflow-agentless-plugin-package}`,
  `{#260513-wsflow-claude-compatible-package}`) mentions template version
  lineage; both are manifest/MCP-key/marketplace scope only. No edit needed —
  matches ticket's `## Spec Impact` "Checked and needing no change" note.
- `ai-docs/manuals/wsflow-mirroring.md#L286-304` — Bootstrap Template Rules.
  L291-292: "Keep bootstrap template version histories package-local;
  matching behavior changes may use different version numbers in each
  package." L293: "Do not copy the full bootstrap migration backlog into the
  wsflow template." Both need inversion — L293 is not named in the ticket's
  Phase 4 body but is now directly false (Phase 2 Result confirms wsflow's
  template DOES carry the full ws `v0001..v0047` checklist verbatim,
  token-substituted); leaving it unedited would contradict the very next
  bullet after the L291-292 rewrite. L296-304 (Artifact neutrality invariant +
  Enforceable corollary) already describe the emitted-artifact-neutral state
  correctly and should NOT be touched — the rewritten L291-293 must describe
  the counter/backlog reality without restating those bullets.
- `ai-docs/manuals/wsflow-mirroring.md#L244-243` (Rsrc/Skills provisioning
  section) — currently documents only the rsrc-tree regen entrypoints
  (`TestGenerateRealManifest`, `TestRegenerateWsflowRsrcMirror`) at L253-267.
  No existing bullet documents `WSRSRC_REGEN_SKILLS=1 go test
  ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest` for edits under
  `agents-plugin/skills/` (confirmed via `grep -n
  "WSRSRC_REGEN_SKILLS|TestGenerateRealSkillsManifest"` — the string appears
  only in `ai-docs/mental-model/workflow-skills.md#L114`'s "Add a Codex
  workflow skill" recipe, not in `wsflow-mirroring.md`). Phase 3's Result
  (ticket L440-448) hit exactly this gap. Add a short item near the existing
  Rsrc Tree Provisioning "After-edit checklist" (L263-271) noting the skills
  manifest regen is a separate gate from the rsrc manifest regen.
- `ai-docs/mental-model/workflow-skills.md#L85` — "`lead-bootstrap` is
  mirrored between ws and wsflow, but downstream template version histories
  are package-local; wsflow starts its bootstrap baseline at `v0001` and does
  not replay the full ws migration backlog. {#260513-wsflow-agentless-skill-surface}"
  — invert; keep the spec-stem anchor reference.
- `ai-docs/mental-model/workflow-skills.md#L110` (Coupling section) —
  "Bootstrap template changes must check both ws and wsflow packages; matching
  behavior may use different template version numbers because each package
  owns its own downstream lineage." — invert to shared-counter coupling
  language.
- `ai-docs/mental-model/workflow-skills.md#L119` (Extension Points & Change
  Recipes) — "**Change bootstrap baseline behavior**: update both
  `lead-bootstrap` packages when applicable, but bump each package's
  `AGENTS.template.md` version only inside that package's own lineage." —
  invert: one shared bump applies to both packages' emitted tag.
- `ai-docs/tickets/idea/260728-research-parallel-workflow-guide-divergence.md#L84-88`
  — `## Non-Scope`: "Converging the two `AGENTS.template.md` lineages, which
  an existing test forbids." Ticket is still in `idea/` (unmoved). The
  broader "three near-identical workflow guides" framing (L11-17) names a
  THIRD copy — this repo's own `ai-docs/WORKFLOW.md` — which `260825` does
  NOT converge (it is downstream output of the now-unified template, per
  260825's own Decisions, not itself a shipped template); the note added here
  must be precise about covering 2 of 3 copies, not all 3.
- Confirmed via `grep -rn` across `*.md/*.py/*.go`: no live (non-ticket,
  non-plan-history) reference to
  `test_bootstrap_template_uses_wsflow_local_version_lineage` outside the
  test file itself; safe to rename without a dangling-reference cleanup step
  elsewhere. `.claude/worktrees/*` hits are stale worktree copies outside the
  active tree.

## Implementation Plan

1. **Rewrite the test** in
   `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`, replacing the
   `test_bootstrap_template_uses_wsflow_local_version_lineage` method
   (L231-239) — RENAME to
   `test_bootstrap_scaffolds_emit_converged_output_across_packages` (reflects
   what it now asserts; update the class if any other test references the old
   name — confirmed none do).
   - Add module-level constants/helpers near the existing `FORBIDDEN_PATTERNS`
     block: a compiled strip transform mirroring `lead-bootstrap.md`'s
     `## On: fresh` step 1 —
     ```python
     _MIGRATION_SETUP_BLOCK = re.compile(r"<!-- MIGRATION:.*?-->\n*", re.DOTALL)
     _MIGRATION_CHECKLIST_BLOCK = re.compile(r"<!-- MIGRATION CHECKLIST.*?-->\n*", re.DOTALL)

     def _emit_fresh_body(raw_template_text: str) -> str:
         text = _MIGRATION_SETUP_BLOCK.sub("", raw_template_text)
         text = _MIGRATION_CHECKLIST_BLOCK.sub("", text)
         return text
     ```
   - Test body:
     ```python
     def test_bootstrap_scaffolds_emit_converged_output_across_packages(self):
         ws_agents_raw = (FULL_PLUGIN_SKILLS_DIR / "lead-bootstrap" / "AGENTS.template.md").read_text(encoding="utf-8")
         wsflow_agents_raw = (SKILLS_DIR / "lead-bootstrap" / "AGENTS.template.md").read_text(encoding="utf-8")
         ws_emitted = _emit_fresh_body(ws_agents_raw)
         wsflow_emitted = _emit_fresh_body(wsflow_agents_raw)
         self.assertEqual(ws_emitted, wsflow_emitted)

         tag_pattern = re.compile(r"<!-- Template Version: (v\d+) -->")
         ws_tag = tag_pattern.search(ws_emitted)
         wsflow_tag = tag_pattern.search(wsflow_emitted)
         self.assertIsNotNone(ws_tag)
         self.assertIsNotNone(wsflow_tag)
         self.assertEqual(ws_tag.group(1), wsflow_tag.group(1))

         ws_workflow = (FULL_PLUGIN_SKILLS_DIR / "lead-bootstrap" / "WORKFLOW.md").read_text(encoding="utf-8")
         wsflow_workflow = (SKILLS_DIR / "lead-bootstrap" / "WORKFLOW.md").read_text(encoding="utf-8")
         self.assertEqual(ws_workflow, wsflow_workflow)
     ```
   - Keep a short comment above the method citing ticket `260825` Phase 4 and
     stating the strip mirrors `lead-bootstrap.md`'s fresh-mode step (do not
     duplicate the full ticket prose — one or two lines).
   - Do not touch any other test method in this file.

2. **Guard-detection coverage (deliverable B)**: no code change. Confirmed
   `TestBootstrapStalenessWarningFiresOnAboveHeadTag` and
   `TestBootstrapStalenessWarningFiresOnUnparseableTag` in
   `agents-plugin-tool/internal/mcp/bootstrap_alarm_test.go` already cover
   both fail-loud directions end-to-end (fire condition, both version
   numbers, config.tune pointer, honest-enforcement text). No further action.

3. **Update `ai-docs/spec/workflow-skills.md#L253-257`**
   (`{#260513-wsflow-agentless-skill-surface}` closing paragraph): replace the
   package-local-lineage paragraph with the shared-counter + package-neutral
   contract, e.g.: "wsflow bootstrap emits a package-neutral downstream
   artifact converged with the full ws package: `AGENTS.template.md` and
   `WORKFLOW.md` produce identical emitted output across both packages modulo
   the shared `<!-- Template Version: vNNNN -->` tag, and both packages share
   one migration-ordinal lineage (wsflow no longer runs a separate
   `v0001..v0008` counter). Bootstrap behavior changes remain
   mirroring-sensitive: maintainers check both packages and bump the shared
   template version once for a change either package receives." Keep the
   anchor `{#260513-wsflow-agentless-skill-surface}` unchanged (no rename).
   Call `ws/spec_index.verify` after the edit.

4. **Update `ai-docs/spec/mcp-tools.md#L672-703`**
   (`{#260703-bootstrap-staleness-warning}`): keep the existing package-local
   *detection mechanism* sentence (still accurate — each binary still
   resolves its own shipped template file via `wsrsrc.ResolveSkillsRoot()`),
   but add a new paragraph after the existing "silent by design" paragraph
   (after L701) documenting the fail-loud direction: the warning now also
   fires when the downstream tag is strictly ahead of the running package's
   own template head, or does not parse as `vNNNN` — both fire directions
   name the version number(s), point at `config.tune(key: "bootstrap_alarm",
   value: "off")`, and state the honest-enforcement limit: this is a
   code-level detector only, backed by a skill-level refuse instruction in
   `lead-bootstrap`, not a mechanical hard-block on reconcile/restamp. Keep
   the anchor unchanged. Call `ws/spec_index.verify` after the edit.

5. **No edit** to `ai-docs/spec/plugin-runtime.md` or
   `ai-docs/spec/claude-compatibility.md` — confirmed clean.

6. **Update `ai-docs/manuals/wsflow-mirroring.md#L291-293`**: replace both
   bullets —
   - "Keep bootstrap template version histories package-local; matching
     behavior changes may use different version numbers in each package." →
     "Bootstrap template version history is a single shared migration-ordinal
     lineage across both packages; a behavior change bumps the shared
     `<!-- Template Version: vNNNN -->` counter once and both packages emit
     the same head."
   - "Do not copy the full bootstrap migration backlog into the wsflow
     template." → "The wsflow template's `MIGRATION CHECKLIST` carries the
     full ws migration backlog, token-substituted (`ws:`→`wsflow:`,
     `ws/`→`wsflow/`), plus one equivalence-note paragraph recording which ws
     version wsflow's former consolidated baseline was equivalent through."
   Leave L288-290 (mirrored-skill bullet) and L294-304 (baseline-update
   bullet, Artifact neutrality invariant, Enforceable corollary) untouched —
   verify the rewritten bullets do not restate or contradict L296-304.

7. **Add the skills-manifest-regen documentation item** to
   `ai-docs/manuals/wsflow-mirroring.md`'s Rsrc Tree Provisioning section
   (near the existing "After-edit checklist" at L263-271): a new bullet
   stating that editing any file under `agents-plugin/skills/` (including
   `lead-bootstrap` templates) requires a separate regen gate —
   `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run
   TestGenerateRealSkillsManifest` (mandatory `-count=1` per this doc's
   existing convention for env-gated regen entrypoints) — guarded by
   `TestSkillsManifestDriftIsVisible`, distinct from the rsrc-manifest regen
   (`WSRSRC_REGEN`/`WS_REGEN_MANIFEST`) documented above it. Reference Phase
   3's regression (`260825` ticket L440-448) as the motivating incident in
   one clause, not a full retelling.

8. **Update `ai-docs/mental-model/workflow-skills.md`** (three edits, keep
   spec-stem anchors in prose per mental-model conventions):
   - L85: "`lead-bootstrap` is mirrored between ws and wsflow; both packages
     emit a package-neutral downstream artifact converged on one shared
     migration-ordinal lineage — wsflow no longer runs an independent
     baseline. {#260513-wsflow-agentless-skill-surface}"
   - L110: "Bootstrap template changes must check both ws and wsflow
     packages; both packages share one migration-ordinal counter, so a
     behavior change bumps the tag once and both templates emit the same
     head."
   - L119: "**Change bootstrap baseline behavior**: update both
     `lead-bootstrap` packages in one logical change; both packages emit the
     same `AGENTS.template.md` version tag from the shared migration-ordinal
     lineage."
   Add `(mental-model-updated)` to the commit message per mental-model
   conventions.

9. **Add a cross-reference note to
   `ai-docs/tickets/idea/260728-research-parallel-workflow-guide-divergence.md`**:
   append a short paragraph after the `## Non-Scope` section (new heading,
   e.g. `## Resolution Note`), recording precisely: ticket `260825` overrides
   this ticket's Non-Scope exclusion and converges the two
   `AGENTS.template.md` lineages onto one package-neutral artifact and shared
   migration-ordinal counter, and separately converges the two shipped
   `WORKFLOW.md` template copies to byte-identical content — answering this
   ticket's "structural equivalence, declared-substitution" open question
   (L66-67) for those two pairs. Note explicitly that the THIRD copy this
   ticket names, this repo's own `ai-docs/WORKFLOW.md`, is downstream
   generated output of the now-unified template and is not itself converged
   by `260825` — that residual stays open if still relevant. Do not move this
   ticket's status; leave the status decision to a future triage pass.

10. **Verify no dangling references**: re-run the `grep -rn
    "test_bootstrap_template_uses_wsflow_local_version_lineage"` search
    (already run — only historical `ai-docs/tickets/.done/`,
    `ai-docs/.plans/`, and this ticket's own frozen Result text reference the
    old name; those are frozen records per ticket conventions and must NOT be
    edited).

## Verification Plan

- `python3 -m unittest discover agents-plugin-wsflow/tests` — must be green
  (currently 10/10; rename replaces one test 1-for-1, count stays the same).
- Prove the inverted test fails on drift/counter-split: run the same mutation
  experiment already validated during survey (temporarily flip one copy's
  `<!-- Template Version: vNNNN -->` tag, or reintroduce a `ws/`-prefixed
  token outside the two stripped comment blocks, rerun the single test, confirm
  failure, then revert — do not leave the mutation committed).
- `grep -rn "test_bootstrap_template_uses_wsflow_local_version_lineage"
  agents-plugin-wsflow/` — must return no hits after the rename.
- `ws/spec_index.verify` after the two spec edits (`workflow-skills.md`,
  `mcp-tools.md`) — must report no new health issues.
- Manual read-back: confirm `ai-docs/spec/workflow-skills.md
  {#260513-wsflow-agentless-skill-surface}` and `ai-docs/spec/mcp-tools.md
  {#260703-bootstrap-staleness-warning}` no longer state or imply
  package-local version lineage.
- Manual read-back: confirm `ai-docs/manuals/wsflow-mirroring.md`'s Bootstrap
  Template Rules (L286-304 region) is internally consistent — the rewritten
  L291-293 must not contradict the Artifact neutrality invariant / Enforceable
  corollary immediately below it.
- Manual read-back: confirm `ai-docs/mental-model/workflow-skills.md` L85,
  L110, L119 no longer state package-local lineage, and each retains its
  spec-stem anchor reference.
- If the wsflow test suite has a Go-side counterpart to re-run for the
  skills-manifest doc addition, no code changed here, so
  `go test ./internal/wsrsrc/...` is not required by this phase, but running
  it once is a cheap sanity check given Phase 3's regression history; not a
  hard verification gate for Phase 4 since no source file under
  `agents-plugin/skills/` changes in this phase.
- Final: confirm ticket `260825`'s Phase 4 `### Result` section (written at
  implementation time, not by this plan) records the rename decision, spec
  edits, and the `260728` cross-reference note, matching ticket conventions
  for Result/Edition content.

## Escalations

- None.

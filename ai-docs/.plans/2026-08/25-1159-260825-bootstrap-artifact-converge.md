# Plan: 260825-refactor-ws-wsflow-bootstrap-artifact-convergence — Phase 1: Converge the scaffolded artifacts to one package-neutral form

## Relevant Ticket Contract

- **Governing principle (Decisions):** the bootstrapped artifact is
  package-neutral; only the runtime workflow (agents vs. no agents) differs.
  Any current setup difference between the two bootstraps is drift, not
  intended divergence.
- **Enforceable corollary (Decisions):** capability-specific content must be
  relocated to runtime skills, never left as artifact drift. Default ruling
  for any divergence is "drift → converge."
- **No `ws`/`wsflow` skill or tool name survives in either emitted artifact**
  (Decisions), except **one deliberate capability-detection gate**: the
  `AGENTS.md` pointer to `ai-docs/WORKFLOW.md` keeps a conditional naming the
  `ws`/`wsflow` `workflow-manual` MCP tool, in one fixed string identical
  across both templates.
- Note-layer refs → on-disk location (`repo` layer is `ai-docs/ws-notes/`);
  skill routing → the manual activity, keeping any host-neutral test it
  carried (e.g. the Inclusion-test rule-placement heuristic) without the
  skill name; a mechanism with no downstream on-disk meaning (worktree/clone
  note layer) → drop it from the artifact.
- **Byte-identity target (Phase 1 verification boundary):** emitted
  (fresh-mode) body of `agents-plugin` vs. `agents-plugin-wsflow` must be
  identical modulo the `<!-- Template Version: vNNNN -->` tag. Fresh-mode
  emit strips only the `<!-- MIGRATION: ... delete this block -->` block and
  the `<!-- MIGRATION CHECKLIST ... -->` block (both carry an explicit
  non-copy instruction). The Inclusion-test comment is **not** stripped
  (migration `v0010` keeps it permanently downstream) — it survives with
  only its skill-name token removed.
- Also trim the wsflow `git log -10` Project-Memory step to match ws, and
  establish the artifact-neutrality invariant + enforceable corollary in
  `ai-docs/manuals/wsflow-mirroring.md` "Bootstrap Template Rules".
- **Hard test constraint (verified against source):**
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py::test_skill_files_do_not_reference_full_ws_agent_surface`
  scans every file under `agents-plugin-wsflow/skills/` (raw text, not just
  emitted body) for `FORBIDDEN_PATTERNS` including `\bws/`, `\bws:`, `\bws\.`
  (plus `\bagents\.`, `\bsubquery\b`, and several retired-skill names). None
  of the four target files, nor the wsflow-mirroring.md addition, may
  introduce these patterns anywhere — including inside comment/scaffold
  blocks. Bare `ws` (word-bounded, not followed by `/`, `:`, `.`) and
  `wsflow` are both safe; `ai-docs/ws-notes/` (the directory name, which
  stays per the ticket) is also safe since `ws` there is followed by `-`.

## Out of Scope

- Unifying the migration-ordinal counter (Phase 2): do not touch the
  `<!-- MIGRATION CHECKLIST ... -->` block contents, the `v0001..v0047` (ws)
  or `v0001..v0008` (wsflow) lineages, or either `<!-- Template Version:
  vNNNN -->` tag value.
- The fail-loud above-head/unknown-tag guard (Phase 3).
- Inverting/rewriting `test_bootstrap_template_uses_wsflow_local_version_lineage`
  or adding the Phase-4 convergence test (Phase 4). This test's current
  assertions (`v0008` tag present, "This template has package-local version
  history" phrase present, no full-plugin bullet leaked into wsflow) must
  keep passing after Phase 1 edits, since none of them touch the checklist
  or tag.
- Spec anchor updates (`workflow-skills.md`, `mcp-tools.md`) and recording
  the 260728 Non-Scope override — both are Phase 4.
- Regenerating this repo's own `ai-docs/WORKFLOW.md` (downstream output; not
  hand-edited to diverge from source, and not regenerated until a bootstrap
  upgrade runs).
- `SKILL.md` files, other skills, or any file outside the four scaffold
  files + `wsflow-mirroring.md`.

## Codebase Findings

- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md:7` — Preamble
  bullet contains `` ws/note.search(layer: "repo") ``; emitted (not inside a
  stripped block).
- `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md:7` — same
  bullet with `` wsflow/note.search(layer: "repo") ``.
- `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md:9` — extra
  `3. **Recent history** - run \`git log -10\` for \`## AI Context\`
  rationale.` step that ws does not have (ws trimmed this at v0042/v0043);
  pure drift, not a token issue.
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md:85` /
  `agents-plugin-wsflow/...:86` — Project Knowledge bullet: `...does not
  override ws runtime or MCP parser behavior.` vs `...does not override
  wsflow runtime or wsflow MCP parser behavior.` — the ticket's one
  deliberate exception (capability-detection gate) applies here.
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md:121-125` /
  `agents-plugin-wsflow/...:122-126` — Inclusion-test HTML comment: `` via
  `ws:lead-add-rule` `` vs `` via `wsflow:lead-add-rule` ``; comment is
  emitted downstream (v0010 keeps it permanently) so this is one of the two
  ticket-enumerated survivors.
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md:92-119` /
  `agents-plugin-wsflow/...:93-120` (`<!-- MIGRATION: Set up ai-docs/ ...
  -->`) and `:127-238` / `:128-212` (`<!-- MIGRATION CHECKLIST ... -->`) —
  both scaffold-only, stripped at fresh-mode emit; contain per-package tool
  refs (`ws/note.write`, `wsflow/note.write`, `ws:lead-forge-spec`, etc.) and
  the differing version lineages. Confirmed **out of scope** for Phase 1 by
  direct `diff` of the two raw files and by the ticket's own audit — leave
  untouched.
- Full raw-file diff (`diff agents-plugin/skills/lead-bootstrap/AGENTS.template.md
  agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md`) confirms
  the only differences outside the two stripped blocks are the three items
  above (L7, L9-insert, L85/86) plus the Inclusion-test line — no other
  emitted-body divergence exists.
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md:1` vs
  `agents-plugin-wsflow/...:1` — title `# ws Workflow Guide` vs `# wsflow
  Workflow Guide`.
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md:3-11` vs
  `agents-plugin-wsflow/...:3-11` — intro paragraphs diverge beyond tokens
  (`"when ws skills or MCP tools are not available"` vs `"during manual
  workflow maintenance"`; `"installed ws tooling"` vs `"installed wsflow
  tooling"`).
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md:15` vs
  `agents-plugin-wsflow/...:15` — Authority Files first bullet: `"...root
  workflow context for agents."` vs `"...for AI agents and automation."`
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md:33-39` vs
  `agents-plugin-wsflow/...:33-40` — worktree/clone note-layer bullet
  (`ws/note.write(layer: "worktree" | "clone", ...)` /
  `wsflow/note.write(...)`) — plugin-local runtime state with no downstream
  on-disk path; **drop the whole bullet** per the corollary. Following
  `ws-notes/` bullet references `ws/note.write(layer: "repo", ...)` /
  `wsflow/note.write(layer: "repo", ...)` — rewrite to the on-disk
  description only (drop the tool-call clause; the path is already named in
  the same sentence).
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md:78` vs
  `agents-plugin-wsflow/...:79` — `"verify with ws tooling..."` vs `"verify
  with wsflow tooling..."`.
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md:141-144` vs
  `agents-plugin-wsflow/...:142-145` — Index Health step 7 routes to
  `ws:lead-forge-spec`/`ws:lead-forge-mental-model`/`ws:lead-discuss` (ws) vs
  `wsflow:lead-forge-spec`/`wsflow:lead-write-spec`/`wsflow:lead-forge-mental-model`/
  `wsflow:lead-write-ticket`/`wsflow:lead-discuss` (wsflow) — genuine
  divergence, not just tokens; convert to on-disk/manual-activity routing.
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md:156-166` vs
  `agents-plugin-wsflow/...:157-167` — final section heading and intro
  differ entirely: `## Manual Fallback` / `"When ws skills, MCP tools, or
  Claude compatibility commands are unavailable:"` vs `## Manual Maintenance`
  / `"When maintaining workflow docs manually:"`; step 5 differs: `"...then
  re-run ws verification tools when they become available."` vs `"...then
  run wsflow verification tools when the workflow change reaches normal
  tooling."`
- `ai-docs/manuals/wsflow-mirroring.md:286-296` — current "## Bootstrap
  Template Rules" bullets state "Keep bootstrap template version histories
  package-local" (still true post-Phase-1, do not touch — Phase 2 unifies
  it) and have no artifact-neutrality statement yet; insert new bullets
  documenting the invariant + corollary without touching the existing four.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py:77-91` —
  `FORBIDDEN_PATTERNS` dict confirms exactly `\bws/`, `\bws:`, `\bws\.`,
  `\bagents\.`, `\bsubquery\b`, and five retired-skill-name patterns; none of
  the planned replacement text matches any of them (verified by constructing
  the target strings and checking against the patterns).
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py:231-264` —
  `test_bootstrap_template_uses_wsflow_local_version_lineage` asserts the
  `v0008` tag, the "This template has package-local version history" phrase,
  and that no full-plugin `v\d{4}:` bullet leaks into wsflow's checklist;
  none of these are touched by Phase 1 edits (all live inside the MIGRATION
  CHECKLIST block, out of scope), so the test keeps passing unmodified.
- `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md:35-37` and
  `agents-plugin-wsflow/rsrc/lead-bootstrap/lead-bootstrap.md:35-37` — fresh
  mode: "Copy template to `AGENTS.md`, stripping template-internal migration
  blocks" then "Copy `WORKFLOW.md` to `ai-docs/WORKFLOW.md`" (no stripping
  for `WORKFLOW.md` — it is copied wholesale, confirming every line of
  `WORKFLOW.md` is emitted downstream and must converge).

## Implementation Plan

1. **`agents-plugin/skills/lead-bootstrap/AGENTS.template.md`**
   - L7: replace `` (`ws/note.search(layer: "repo")`) `` with the on-disk
     phrasing: `` read the `repo` note layer at `ai-docs/ws-notes/` (one file
     per key) `` in place of `read repo-tracked notes (...)`.
   - L85: replace `...does not override ws runtime or MCP parser behavior.`
     with the capability-gated form: `...read it only if the `ws` or
     `wsflow` `workflow-manual` MCP tool is not in your toolbox. It is
     explanatory and does not override plugin runtime or MCP parser
     behavior.`
   - L121-125: drop `` via `ws:lead-add-rule` `` from the Inclusion-test
     comment, keeping the placement heuristic: `...## Domain Rules` instead.`
   - Leave the `<!-- MIGRATION: ... -->` block, `<!-- MIGRATION CHECKLIST
     ... -->` block, and `<!-- Template Version: v0047 -->` tag untouched.

2. **`agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md`**
   - L7: replace `` (`wsflow/note.search(layer: "repo")`) `` with the same
     on-disk phrasing used in step 1, so both files read byte-identically.
   - L9: delete the `3. **Recent history** - run \`git log -10\`...` line
     entirely (no renumbering needed — it is the last Project Memory item).
   - L86: replace `...does not override wsflow runtime or wsflow MCP parser
     behavior.` with the same capability-gated sentence used in step 1.
   - L122-126: drop `` via `wsflow:lead-add-rule` `` the same way as step 1,
     producing byte-identical Inclusion-test text.
   - Leave the `<!-- MIGRATION: ... -->` block, `<!-- MIGRATION CHECKLIST
     ... -->` block, and `<!-- Template Version: v0008 -->` tag untouched.

3. **`agents-plugin/skills/lead-bootstrap/WORKFLOW.md`**
   - L1: `# ws Workflow Guide` → `# Workflow Guide`.
   - L3-11: converge the two intro paragraphs to a neutral version, e.g.:
     `"...preserve the project shape when plugin skills or MCP tools are not
     available..."` and `"When this guide and installed plugin tooling
     disagree, treat the installed plugin, runtime, and bundled conventions
     as canonical..."` — must exactly match what is written into the wsflow
     copy in step 4.
   - L15: `...canonical root workflow context for agents.` →
     `...canonical root workflow context for AI agents and automation.`
     (adopt wsflow's wording).
   - L33-36: delete the worktree/clone note-layer bullet entirely.
   - L37-39: rewrite the `ws-notes/` bullet to drop the tool-call clause:
     `` - `ws-notes/` is the git-tracked `repo` note layer, one file per key.
     It holds volatile or tracked session context; prune stale entries
     qualitatively as the project advances. ``
   - L78: `...verify with ws tooling when it becomes available.` →
     `...verify with plugin tooling when it becomes available.`
   - L141-144: rewrite step 7 to route by on-disk destination instead of
     skill name: `"Route deeper semantic work through the owning workflow:
     behavior into `ai-docs/spec/`, modification knowledge into
     `ai-docs/mental-model/`, ticket readiness/status wording into the
     ticket body, and ambiguous direction to a discussion pass."`
   - L156-166: rename `## Manual Fallback` intro to `"When workflow skills,
     MCP tools, or Claude compatibility commands are unavailable:"` (keep
     the heading `## Manual Fallback`) and reword step 5 to `"Verify with
     plain Git and shell commands, then re-run plugin verification tools
     when they become available."`

4. **`agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md`**
   - Apply the identical target text from step 3 to every corresponding
     line/section (L1 title; L3-11 intro; L15 Authority Files bullet;
     L33-40 worktree/clone bullet deletion + `ws-notes/` bullet rewrite;
     L79 tooling wording; L142-145 routing step 7; L157-167 section rename
     from `## Manual Maintenance` to `## Manual Fallback` with the unified
     intro/step-5 wording).
   - Result: `agents-plugin/skills/lead-bootstrap/WORKFLOW.md` and
     `agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md` become
     byte-identical (this file has no scaffold-only blocks to strip, so
     full-file identity is the target, not just emitted-body identity).

5. **`ai-docs/manuals/wsflow-mirroring.md`** — in `## Bootstrap Template
   Rules` (around line 286-296), append two new bullets after the existing
   four (do not edit the existing "Keep bootstrap template version histories
   package-local" bullet — still accurate until Phase 2):
   - An **Artifact neutrality invariant** bullet: the downstream artifact
     (`AGENTS.md` + `ai-docs/WORKFLOW.md`) bootstrap scaffolds is
     package-neutral — `ws` and `wsflow` emit identical content modulo the
     `<!-- Template Version: vNNNN -->` tag; only the runtime workflow
     differs.
   - An **Enforceable corollary** bullet: capability-specific content must
     be relocated to runtime skills, never left as artifact drift; default
     ruling for divergence in `AGENTS.template.md`/`WORKFLOW.md` is "drift →
     converge," with the one deliberate exception being the single fixed
     capability-detection gate string naming both `ws` and `wsflow` for the
     `ai-docs/WORKFLOW.md` pointer.

6. **Grep verification pass** (see Verification Plan) across all four
   edited files plus the `wsflow-mirroring.md` addition for `\bws/`,
   `\bws:`, `\bws\.` before considering the phase done.

## Verification Plan

- `grep -noE '\bws/|\bws:|\bws\.' agents-plugin/skills/lead-bootstrap/AGENTS.template.md agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md` — expect matches only inside the untouched `<!-- MIGRATION CHECKLIST ... -->` block line ranges (unchanged from before this phase); zero matches in any edited line.
- `grep -noE '\bws/|\bws:|\bws\.' agents-plugin/skills/lead-bootstrap/WORKFLOW.md agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md` — expect zero matches in both files.
- `diff agents-plugin/skills/lead-bootstrap/WORKFLOW.md agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md` — expect empty (full-file identity).
- Emitted-body diff for `AGENTS.template.md`: strip the `<!-- MIGRATION:
  ... -->` and `<!-- MIGRATION CHECKLIST ... -->` comment blocks from each
  raw template (e.g. with a small `awk`/`sed` range-delete on the `<!--
  MIGRATION` ... first following `-->` boundaries), then `diff` the two
  results — expect the only remaining difference to be the `<!--
  Template Version: vNNNN -->` line (`v0047` vs `v0008`).
- `python3 -m unittest discover agents-plugin-wsflow/tests` — full wsflow
  package test suite must pass, in particular
  `test_skill_files_do_not_reference_full_ws_agent_surface` (no forbidden
  pattern introduced) and
  `test_bootstrap_template_uses_wsflow_local_version_lineage` (still passes
  unmodified: `v0008` tag, package-local-history phrase, and no leaked
  full-plugin bullet, all untouched by this phase).
- Manual read-through of both `AGENTS.template.md` files' MIGRATION
  CHECKLIST and MIGRATION blocks to confirm they are byte-for-byte unchanged
  from before the phase (out-of-scope guard).

## Escalations

- None.

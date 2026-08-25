# Plan: 260825-refactor-ws-wsflow-bootstrap-artifact-convergence — Phase 2: Unify the migration version counter (shared lineage)

## Relevant Ticket Contract

- Unify wsflow's migration-ordinal lineage onto ws's shared counter: "content
  already aligned — relabel, not replay." wsflow drops its parallel
  `v0001..v0008` ordinal; both templates emit ws's single `vNNNN` head
  (currently `v0047`).
- The unified template **retains ws's full `v0001..v0047` checklist** (real
  deployed low-tag ws downstream projects must still upgrade) and folds
  wsflow's old consolidated baseline in as an **equivalence note** (which ws
  version it is equivalent through), **not** a replacement changelog.
- **Two version axes stay decoupled.** The `<!-- Template Version: vNNNN -->`
  migration ordinal (numeric, walked by the upgrade handler) is a separate
  axis from the plugin `X.Y.Z` edition owned by
  `agents-plugin-tool/scripts/bump-ws-version.sh`; unification must not source
  the ordinal from `X.Y.Z` or vice versa.
- Depends on Phase 1 (already landed on this branch, commit `52fbf0bc` +
  `103014f7`): both `AGENTS.template.md` emit identical bodies modulo the
  version tag; both `WORKFLOW.md` are byte-identical and carry no version
  lineage.
- Verification boundary (ticket text): "fresh wsflow and fresh ws bootstraps
  emit the same head ordinal; opening a fresh wsflow project with ws produces
  no artifact change and re-stamps to head."
- Test coupling: `test_bootstrap_template_uses_wsflow_local_version_lineage`
  currently enforces the package-local divergence Phase 2 removes; its
  now-false assertions must be dropped so the suite stays green, but the
  positive convergence rewrite belongs to Phase 4 (do not add it here).

## Out of Scope

- Phase 1 (already complete on this branch) — no further edits to the
  converged emitted-body content (note-layer refs, capability gate, Inclusion
  test comment, `WORKFLOW.md` prose).
- Phase 3 — the fail-loud above-head/unknown-tag guard (`bootstrap_alarm.go`
  detection + skill-level refuse instruction).
- Phase 4 — rewriting `test_bootstrap_template_uses_wsflow_local_version_lineage`
  into the full convergence assertion (identical fresh-mode emitted body +
  shared head + guard-detection coverage); updating
  `ai-docs/spec/workflow-skills.md {#260513-wsflow-agentless-skill-surface}`
  (L253-257, "wsflow bootstrap uses package-local template version history");
  updating `ai-docs/spec/mcp-tools.md {#260703-bootstrap-staleness-warning}`;
  inverting `ai-docs/manuals/wsflow-mirroring.md` L291-292 ("Keep bootstrap
  template version histories package-local") and recording the `260728`
  Non-Scope override. These are explicitly Phase 4's per the ticket's own
  Phase 4 body ("Update the spec anchors ... and the wsflow-mirroring.md
  Bootstrap Template Rules").
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md` (ws's own copy) —
  already at the shared head (`v0047`); no change needed.
- `agents-plugin-tool/scripts/bump-ws-version.sh` — verified it never touches
  `AGENTS.template.md` or the `Template Version` tag (see Codebase Findings);
  no edit needed, and none should be introduced.
- `agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md` — confirmed
  byte-identical to its ws counterpart and free of any version-tag text; not
  touched by this phase.

## Codebase Findings

- `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md#L127-L214` —
  current wsflow `MIGRATION CHECKLIST` block: header line 131 reads "This
  template has package-local version history; apply only entries listed
  here."; entries `v0001..v0008` are wsflow's own independent numbering (not
  ws's); tag at L214 is `<!-- Template Version: v0008 -->`. This whole block
  is the Phase 2 edit target.
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md#L127-L241` — ws's
  full `MIGRATION CHECKLIST`: entries `v0001..v0047`, tag `v0047` at L241.
  This is the content to fold into wsflow's copy (token-substituted), per the
  ticket's "retains ws's full checklist" decision.
- `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md#L20-L33` — confirms the
  checklist is scaffold-only: `## On: fresh` step 1 is "Copy template to
  `AGENTS.md`, stripping template-internal migration blocks." `## On: upgrade`
  walks "migration checklist items where version > current, in order" and
  "Apply each item only when its condition is met" — i.e. the walk is
  numeric-ordinal-keyed against whatever checklist the running package ships,
  confirming Phase 2's relabel-not-replay claim is mechanically sound: a
  project tagged below head only ever gets entries `> tag` walked, and Phase 1
  already proved (ticket Background, "Structural misfire audit = zero") that
  no ws entry `v0001..v0047` applies to an already-converged wsflow-shaped
  project.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L231-L264` — the
  coupled test. Four assertions become false once the counter unifies:
  - L233 `self.assertIn("<!-- Template Version: v0008 -->", text)` — wsflow
    will stamp `v0047`.
  - L234 `self.assertIn("This template has package-local version history", text)`
    — this string is removed from the template in this phase.
  - L241-L249 (`full_tag_match` / `assertNotIn`) — asserts wsflow's tag is NOT
    the full-plugin's current tag; post-unification they are equal by design.
  - L251-L264 (`leaked_bullets`) — asserts no full-plugin `- vNNNN:` bullet
    line appears verbatim in wsflow's text; post-unification, entries with no
    `ws:`/`ws/` token (e.g. the future-copied `v0001` "If
    `ai-docs/_memory.md` exists...") are copied byte-for-byte and are supposed
    to "leak" — this check is fundamentally incompatible with unification.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L1-L13,77-91` —
  `SKILLS_DIR = PLUGIN_DIR / "skills"` (`PLUGIN_DIR` = `agents-plugin-wsflow`)
  and `FORBIDDEN_PATTERNS` includes `\bws/`, `\bws:`, `\bws\.` (word-boundary
  anchored) scanned via `SKILLS_DIR.rglob("*")`, which includes
  `lead-bootstrap/AGENTS.template.md`. Confirmed by grep: the current wsflow
  template already has **zero** matches for `\bws/|\bws:|\bws\.` (all its own
  tokens are `wsflow:`/`wsflow/`), while ws's own copy uses `ws:`/`ws/` at
  L99, L115, L155, L159-162, L168, L213, L230. Because `\b` requires a word
  boundary immediately before `ws`, `wsflow:`/`wsflow/` substrings never
  false-match — confirms copying ws's checklist entries with every `ws:`→
  `wsflow:` and `ws/`→`wsflow/` token substituted keeps
  `test_skill_files_do_not_reference_full_ws_agent_surface` green.
  - One residual, non-blocking nuance: ws checklist entry `v0036` (L169) has
    bare prose "does not override **ws** runtime or MCP parser behavior" with
    no `:`/`/` suffix — `FORBIDDEN_PATTERNS` does not match bare `ws`, so this
    passes either way; prefer rewording to `ws`/`wsflow` runtime for accuracy
    when copying, but it is not test-enforced and is scaffold-only (stripped
    at emit), so it is a low-risk polish item, not a blocker.
- `agents-plugin-tool/scripts/bump-ws-version.sh` (full file) — touches
  `plugin.json`/`.codex-plugin/plugin.json` `version`, `runtime.json`
  `plugin_version`, `main.go` `var version`, a `version=X.Y.Z-dev` line, and
  prose mentions of `ws@X.Y.Z`/`wsflow@X.Y.Z` in `AGENTS.md` and a `ws-mcp`
  ref doc. **No reference to `AGENTS.template.md`, `Template Version`, or any
  `vNNNN` ordinal anywhere in this script** — confirms the two axes are
  already decoupled today; Phase 2 must not introduce any such coupling.
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md` vs
  `agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md` — `diff` returns
  empty (byte-identical) and neither contains `Template Version`/`vNNNN`/`v00`
  text — confirms this file is untouched by Phase 2, as the ticket states it
  "has no version lineage."
- `ai-docs/manuals/wsflow-mirroring.md#L291-292` — "Keep bootstrap template
  version histories package-local; matching behavior changes may use
  different version numbers in each package." Still present and now
  inaccurate after this phase; confirmed Phase 4-owned per the ticket's own
  Phase 4 text, so this phase leaves it as a known, intentionally deferred
  doc-drift window.
- `ai-docs/spec/workflow-skills.md#L253-257` — "wsflow bootstrap uses
  package-local template version history..." — same deferred-to-Phase-4
  status, confirmed via the ticket's `## Spec Impact` section.

## Implementation Plan

1. **`agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md`** — replace
   the `MIGRATION CHECKLIST` block (`L127-L212`, i.e. from `<!-- MIGRATION
   CHECKLIST` through the closing `-->` just before the `Template Version`
   tag) with:
   - The same header wording ws uses (`agents-plugin/.../AGENTS.template.md#L127-L133`),
     minus the "package-local version history" line, plus "Skip obsoleted
     items." (matching ws's header).
   - A new **equivalence-note** paragraph stating wsflow's former
     `v0001..v0008` consolidated baseline (the lineage reset from commit
     `599fb453`, per the ticket's Background) is equivalent through ws
     `v0047` — i.e. the Phase 1 audit found no ws checklist entry through
     `v0047` applies beyond what that baseline already established — and that
     this checklist now shares ws's single lineage instead of numbering
     independently. This is a note, not a replacement changelog: do not
     delete or renumber wsflow's history, just annotate it once above the
     shared entries.
   - ws's full `v0001..v0047` entries, copied verbatim from
     `agents-plugin/skills/lead-bootstrap/AGENTS.template.md#L134-L238`, with
     every `ws:` token replaced by `wsflow:` and every `ws/` token replaced by
     `wsflow/` (matches the existing wsflow convention already used
     throughout this file, e.g. current L99/L115/L184/L203). Leave content
     wording otherwise untouched (this is the "content already aligned"
     relabel, not a rewrite).
   Then update the tag line (current `L214`) from
   `<!-- Template Version: v0008 -->` to `<!-- Template Version: v0047 -->`.

2. **Verify token hygiene** — after the edit, run:
   `grep -nE '\bws/|\bws:|\bws\.' agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md`
   and confirm it returns nothing (or only false-positive-free `wsflow:`/`wsflow/`
   substrings, which the `\b` boundary already excludes).

3. **`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L231-L264`** —
   minimally edit `test_bootstrap_template_uses_wsflow_local_version_lineage`
   to drop the four now-false assertions (L233, L234, L241-L249, L251-L264)
   without deleting the method and without adding positive convergence
   assertions (that is Phase 4's). Leave one weak structural check so the
   method is not a bare no-op, e.g.:

   ```python
   def test_bootstrap_template_uses_wsflow_local_version_lineage(self):
       # Ticket 260825 Phase 2 unified wsflow's migration ordinal onto ws's
       # shared v0001..v0047 lineage, so the package-local-divergence
       # assertions this test used to make are no longer true by design.
       # Phase 4 (ticket 260825) rewrites this into a positive convergence
       # assertion (identical fresh-mode emitted body + shared head across
       # both packages) plus guard-detection coverage; left minimal until then.
       text = (SKILLS_DIR / "lead-bootstrap" / "AGENTS.template.md").read_text(encoding="utf-8")
       self.assertIn("<!-- Template Version:", text)
   ```

   Do not rename the test method in this phase (Phase 4 "inverts the guard
   test," which covers the rename/rewrite).

## Verification Plan

- `python3 -m unittest discover agents-plugin-wsflow/tests` — expect the same
  10/10 pass count as Phase 1's recorded result, with the trimmed test still
  green.
- Ordinal decoupling check: `grep -n "Template Version\|vNNNN" agents-plugin-tool/scripts/bump-ws-version.sh` returns nothing (confirms no coupling was introduced).
- Emitted-body identity (manual, ad hoc — no committed test in this phase;
  Phase 4 owns that): strip the two scaffold-only comment blocks (`<!--
  MIGRATION: ... -->` and `<!-- MIGRATION CHECKLIST ... -->`) from both
  `AGENTS.template.md` files the same way `lead-bootstrap.md`'s fresh-mode
  step 1 does, then diff. Expect **zero** difference (not even the version
  tag, since both now stamp `v0047`) — this is a stronger result than Phase
  1's "differs only on the version-tag line."
- Cross-open behavior (manual reasoning / agent-executed, no Go code exists
  for bootstrap reconcile per the ticket's own note — nothing to unit-test
  here):
  - A fresh `wsflow` bootstrap now stamps `v0047`; a `ws` session opening that
    project reads its own head, so the upgrade walk is empty (no entries with
    `version > 47`) — matches ticket verification text.
  - A legacy `v0008`-tagged wsflow project, on wsflow re-bootstrap or ws-open,
    walks checklist entries `v0009..v0047` against the shared checklist; each
    is judged individually and, per the ticket's own audit ("Structural
    misfire audit = zero"), should Skip since the content already matches the
    post-`v0047` end state — landing at `v0047` with no artifact changes.
- Confirm no unintended files changed: `git status` / `git diff --stat` should
  show only `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md`
  and `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`.

## Escalations

- None.

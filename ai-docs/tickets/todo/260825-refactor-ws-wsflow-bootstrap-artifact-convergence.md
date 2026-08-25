---
title: Converge ws/wsflow bootstrap into one package-neutral artifact with a shared migration version counter
related:
  260728-research-parallel-workflow-guide-divergence: supersedes its Non-Scope exclusion of converging the two AGENTS.template.md lineages (and the test that forbade it)
  260703-chore-bootstrap-staleness-alarm: prior package-local tag comparison this refactor generalizes; reuses its `<!-- Template Version: vNNNN -->` tag format
  260622-bug-wsflow-launcher-coldload-divergence: adjacent ws/wsflow divergence class (launcher), same "divergence is drift, not intended" posture
spec:
  - 260513-wsflow-agentless-skill-surface
sage-review-design: completed
---

# Converge ws/wsflow bootstrap into one package-neutral artifact with a shared migration version counter

## Background

`ws` (full, agentful) and `wsflow` (agentless derivative) each ship their own
`lead-bootstrap` skill with a separate downstream `AGENTS.template.md` and a
**separate migration version lineage**: ws runs `v0001..v0047`
(`agents-plugin/skills/lead-bootstrap/AGENTS.template.md`), wsflow runs its own
`v0001..v0008` (`agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md`,
whose `v0001` is a consolidated baseline rolling up ws's early backlog). The
split was deliberate (commit `599fb453`, "reset bootstrap template lineage") so
wsflow adopters would not replay ws's inherited backlog, and it is currently
enforced by a test (`test_bootstrap_template_uses_wsflow_local_version_lineage`)
and stated in specs (`workflow-skills.md {#260513-wsflow-agentless-skill-surface}`
and `mcp-tools.md {#260703-bootstrap-staleness-warning}`) and the
`wsflow-mirroring.md` "Bootstrap Template Rules".

The `<!-- Template Version: vNNNN -->` tag has **no lineage namespace**, so the
same number means different states in each package. Users who move a project
between `ws` and `wsflow` hit real problems. Concrete observed symptom: a
wsflow-bootstrapped project (tag `v0008`, which is wsflow's *head*) opened by
`ws`, which reads `v0008` as its own near-oldest (obsolete) version and tries to
"upstream" the project by walking `v0009..v0047`.

### What the investigation established (evidence, not assumption)

- **ws upgrade mechanics.** `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md`
  upgrade mode walks each checklist entry where `version > current`, applying
  each **only when its condition is met** (`judge: migration-condition`:
  Skip/Apply-subset/Apply-all). Global invariant: "Every migration item is
  idempotent; re-running on an already-migrated project produces no changes."
  `adopt` mode instead audits `v0001..latest` against actual state
  and stamps latest. `fresh` mode skips the walk and stamps latest directly.
- **Structural misfire audit = zero.** Auditing every ws entry `v0001..v0047`
  against a fresh wsflow project: **no doc-structure entry Applies** — wsflow's
  baseline already equals ws's post-`v0046/v0047` end state (no
  `_index.md`/`_index.local.md`/`_memory.md`/`plans/`/`wip/`/`deps/`, archives
  dot-prefixed, `ready/` present, `## Ticket Updates`/`## Spec` in Commit Rules,
  ticket-completion rule present, gitignore globs correct).
- **The residual is fingerprint/drift, not capability-bound content — measured on
  the emitted body, not the raw file.** The `AGENTS.template.md` raw diff is
  dominated by an entirely different `MIGRATION CHECKLIST` (ws `v0001..v0047` vs
  wsflow's consolidated `v0001..v0008`), but that block lives inside a
  scaffold-only `<!-- MIGRATION CHECKLIST ... -->` comment never written
  downstream; convergence is measured on the emitted (fresh-mode) output.
  - **`AGENTS.template.md` emitted body — exactly two plugin references survive
    downstream.** Most `ws/`/`ws:` tokens live in blocks stripped at emit time:
    the `ai-docs/` set-up block (`ws/note.write` at the file's L99/L115) is inside
    a `<!-- MIGRATION: ... delete this block -->` comment, and the remaining `ws:`
    skill entries are inside the scaffold-only MIGRATION CHECKLIST. What actually
    reaches a downstream `AGENTS.md` is (a) one tool ref —
    `ws/note.search(layer: "repo")` in the Preamble (L7) — and (b) one *skill*
    name — `ws:lead-add-rule` — inside the Inclusion-test comment (L121-125),
    which migration entry `v0010` explicitly keeps **permanently** downstream, so
    it is emitted, not scaffold. The wsflow copy substitutes both tokens
    (`wsflow/note.search`, `wsflow:lead-add-rule`) and additionally kept a stale
    `git log -10` Project-Memory step that ws trimmed at `v0042`/`v0043`.
  - **`WORKFLOW.md` emitted prose — both tool and skill references, plus genuine
    divergence.** It is copied downstream wholesale and references both MCP tools
    (`ws/note.write`, L34/L38) and skills (`ws:lead-forge-spec`,
    `ws:lead-forge-mental-model`, `ws:lead-discuss`, L141/L142/L144), and the two
    copies diverge in prose/framing beyond tokens. Critically, `WORKFLOW.md` is
    by its own charter (its L3-7 preamble) the fallback guide *for a maintainer
    working when ws skills or MCP tools are not available* — so every
    plugin-namespaced reference in it (skills and tools alike) is drift, not
    neutralizable content (see Decisions).
  - No omitted-section divergence exists (earlier notes naming Branch Policy /
    Documentation-System / Ticket-System referred to this repo's own root
    `AGENTS.md`, not the scaffold templates). **No genuinely agentless-vs-agentful
    artifact content was found.**
- **Unspecified tag handling.** Neither `lead-bootstrap.md` nor the templates
  specify behavior for a tag above the running package's head, or a tag number
  absent from its list. This is the mechanism behind the reverse direction
  (wsflow, head `v0008`, opening a `v0047` ws project): the migration *walk* is
  empty, but step-4 template-managed reconcile + step-8 restamp could silently
  downgrade the artifact to agentless shape and stamp `v0008` — an undefined,
  potentially destructive path currently masked only by the executing agent's
  ad-hoc judgment.

## Decisions

- **Scope covers every scaffolded artifact, not just `AGENTS.template.md`.** The
  same principle applies to the bootstrap-scaffolded `WORKFLOW.md` (bootstrap
  writes `ai-docs/WORKFLOW.md` from the skill-dir source): the two scaffold
  copies `agents-plugin/skills/lead-bootstrap/WORKFLOW.md` and
  `agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md` must converge to one
  package-neutral artifact. `ai-docs/WORKFLOW.md` is downstream output of that
  source and is not hand-edited to diverge.
- **Governing principle (user-set): the bootstrapped artifact is
  package-neutral; only the runtime workflow differs.** `ws` and `wsflow` differ
  in how the workflow *runs* (agents vs no agents), never in the project files
  they scaffold. Any current setup difference between the two bootstraps is
  ruled **drift (a bug)**, not intended divergence.
- **Enforceable corollary.** The principle holds only if capability-specific
  content is kept OUT of the artifact and IN the runtime skills. During
  convergence, any artifact content that would need to differ by agent
  capability is **relocated to the runtime skills**, never left as artifact
  drift. Default ruling for any divergence is "drift → converge".
- **No `ws`/`wsflow` skill or tool name survives in either emitted artifact.**
  The earlier split (neutralize tool refs in `AGENTS.md`, remove them only in
  `WORKFLOW.md`) is dropped: a bare primitive like `note.search` is still dead
  for a non-plugin reader (the tool is not installed) and redundant for a plugin
  reader (the runtime `workflow_manual` already loads it), so keeping the name
  buys nothing. In **both** `AGENTS.md` and `WORKFLOW.md`, every
  plugin-namespaced reference is **removed** and rewritten to what a reader
  without the plugin can act on:
  - note-layer refs → the on-disk location (the `repo` layer is
    `ai-docs/ws-notes/`, one file per key);
  - skill routing (`ws:lead-forge-spec`, `ws:lead-add-rule`, …) → the manual
    activity it stands for, keeping any host-neutral test it carried (e.g. the
    Inclusion-test rule-placement heuristic) without the skill name;
  - a mechanism with no downstream on-disk meaning at all (e.g. the
    worktree/clone note layer, which is plugin-local runtime state) is
    capability-bound content: **drop it from the artifact** per the corollary
    rather than invent a fake path.
  Result: both emitted bodies are byte-identical across packages modulo the
  version tag, and carry no package fingerprint of any kind.
- **Shared migration counter follows by construction.** One package-neutral
  artifact ⇒ one converged template ⇒ one shared version lineage. This is a
  **relabel, not a replay**: the audit proved the content is already aligned, so
  unifying wsflow onto ws's counter carries no structural-migration risk.
- **Cross-open guard is ordinal-skew based, not capability-based.** The
  motivating case is a wsflow-stamped project opened by ws (and the reverse), but
  once artifacts and counters are unified, same-release ws and wsflow ship an
  identical template at an identical head — there is no capability-based
  downgrade left. The only residual hazard is a tag **above the running binary's
  own template head** (or absent from its known lineage): an older binary opening
  a project a newer one stamped. The guard keys on that ordinal position, not on
  which package is opening. A below-head tag is a clean upgrade/re-stamp to head
  (allowed); an above-head/unknown-ordinal tag is fail-loud refuse (Phase 3). A
  package must never silently reconcile or restamp a project whose tag is above
  its own template head.
- **Inject-vs-preserve is dissolved.** The question "when ws opens a wsflow
  project, inject agent sections vs preserve agentless" is closed by the
  principle: there are no agent-specific artifact sections; anything that looks
  like one is drift (converge) or misplaced runtime content (relocate). No
  inject/preserve switch is added.
- **Existing `v0008` projects.** Not remapped by machinery. A wsflow
  re-bootstrap lifts `v0008 → head`; a ws-open re-stamps to head. Both land the
  same neutral artifact at the shared head. Truly-legacy pre-fix projects (a
  small, known dogfood population) are a one-time convergence, not a permanent
  code path — the legacy low-tag band shrinks to empty once all projects emit the
  unified number.

### Rejected alternatives

- **Elevate wsflow's number only (one-time), keep separate lineages.** Rejected:
  not durable (hand-maintained templates re-diverge on the next bump) and does
  not address the artifact fingerprint, which the reconcile step re-asserts even
  when the migration walk is empty.
- **Lineage marker on the tag (`v0008-wsflow`) + ws-side lineage detector.**
  Rejected as the primary fix: it preserves the divergence the principle
  declares a bug, re-introduces the cross-package coupling the split avoided, and
  cannot disambiguate already-shipped legacy tags that carry no marker.
- **Make ws entries token-tolerant (`ws|wsflow`).** Rejected in favor of
  removing the fingerprint at the source: tolerance would preserve the drift the
  principle forbids.

## Constraints

- This refactor **overrides** `260728-research-parallel-workflow-guide-divergence`'s
  Non-Scope ("Converging the two `AGENTS.template.md` lineages, which an existing
  test forbids") and **inverts** the guard test
  `test_bootstrap_template_uses_wsflow_local_version_lineage` from
  divergence-enforcing to convergence-enforcing.
- 260728 frames the `AGENTS.template.md` pair and the three-copy `WORKFLOW.md`
  guide as the **same** drift class and asks any answer to cover both. Both are in
  scope here (see Decisions), so this ticket supersedes that Non-Scope for **both**
  pairs, closing 260728's convergence question rather than half of it.
- Two version axes stay distinct. The plugin `X.Y.Z` edition (the
  plugin-cache-invalidation key) stays owned by
  `agents-plugin-tool/scripts/bump-ws-version.sh`; the migration-template ordinal
  (`<!-- Template Version: vNNNN -->`, compared numerically by the upgrade walk)
  is a separate axis bumped when a migration entry is appended. The counter
  unification operates on the ordinal axis only — wsflow drops its parallel
  `v0001..v0008` ordinal and shares ws's single `v0001..v0047` head — and must
  not source the ordinal from the plugin `X.Y.Z` version or vice versa.
- wsflow's reason to exist (agentless runtime — skills that do not spawn agents)
  is untouched; only the scaffolded artifact converges.
- Claude compatibility (`CLAUDE.md` = `@AGENTS.md`) is host-specific and
  orthogonal; do not fold it into the package-neutrality change.

## Spec Impact

Ready promotion will need these addressed (recorded now for recovery; this
ticket lands in `todo/` first):

- `workflow-skills.md {#260513-wsflow-agentless-skill-surface}` — invert
  "wsflow bootstrap uses package-local template version history" to the shared
  counter + package-neutral artifact contract.
- `mcp-tools.md` (`#260703-bootstrap-staleness-warning` area) — the version tag
  is now a shared, package-neutral coordinate; add the fail-loud above-head/
  unknown-tag behavior to the bootstrap/staleness contract.

Checked and needing no change: `plugin-runtime.md
{#260513-wsflow-agentless-plugin-package}` and `claude-compatibility.md
{#260513-wsflow-claude-compatible-package}` carry no template-version-lineage
clause (manifest / MCP-key / marketplace scope only).

## Phases

### Phase 1: Converge the scaffolded artifacts to one package-neutral form

Make the ws and wsflow scaffold sources produce **identical downstream artifacts
modulo the version tag**, across both scaffolded files:
`{agents-plugin,agents-plugin-wsflow}/skills/lead-bootstrap/AGENTS.template.md`
and `{agents-plugin,agents-plugin-wsflow}/skills/lead-bootstrap/WORKFLOW.md`. The
known `AGENTS.template.md` divergence is small (a few tool-namespace token
locations + the `git log -10` drift; see Background); diff the `WORKFLOW.md` pair
to enumerate its divergence. Apply the unified removal rule (see Decisions) to both artifacts: **remove every
`ws`/`wsflow` skill and tool name** and rewrite each reference to what a
plugin-less reader can act on — note-layer refs to the on-disk path
(`ai-docs/ws-notes/` for the `repo` layer), skill routing to the manual activity
(keeping any host-neutral test, e.g. the Inclusion-test rule-placement
heuristic), and any plugin-only mechanism with no downstream on-disk meaning (the
worktree/clone note layer) dropped per the corollary. Concretely, the emitted
references to eliminate are: in `AGENTS.template.md`, the Preamble's
`ws/note.search` (L7) and the Inclusion-test comment's `ws:lead-add-rule` (L123,
emitted permanently via migration `v0010`); in `WORKFLOW.md`, `ws/note.write`
(L34/L38) and the `ws:lead-forge-spec`/`ws:lead-forge-mental-model`/`ws:lead-discuss`
routing (L141-144). Also trim the wsflow `git log -10` drift to match ws, and
reconcile the residual `WORKFLOW.md` prose divergence. Apply the default ruling to any
further difference surfaced during the convergence diff — **drift → converge**,
unless the content is capability-bound, in which case **relocate it to the
runtime skills** (record any relocation). Establish and document the artifact-neutrality invariant +
enforceable corollary in `wsflow-mirroring.md` Bootstrap Template Rules.
Verification: a diff of each pair's fresh-mode outputs is empty except the
version tag; enumerate and justify any residual difference.

### Phase 2: Unify the migration version counter (shared lineage)

Relabel wsflow's migration-ordinal lineage onto ws's shared counter (content
already aligned — relabel, not replay). The unified template **retains ws's full
`v0001..v0047` migration checklist** — ws has real deployed low-tag downstream
projects that must still upgrade — and folds wsflow's consolidated baseline in as
an equivalence note (the ws version it is equivalent through), not as a
replacement changelog. wsflow drops its parallel `v0001..v0008` ordinal and both
converged templates emit ws's single `vNNNN` head. **Keep the two version axes
decoupled:** the `<!-- Template Version: vNNNN -->` migration ordinal (compared
numerically by the upgrade walk against checklist entry keys) stays the
template's own head — bumped when a migration entry is appended — and is **not**
sourced from the plugin `X.Y.Z` edition that `bump-ws-version.sh` owns; unifying
the ordinal must not couple it to the `X.Y.Z` bump (that coupling would break the
numeric walk). Confirm the
cross-open paths: a fresh wsflow project stamps the shared head so ws reads it as
current (empty walk); a legacy `v0008` project converges to head via
re-bootstrap (wsflow) or re-stamp (ws-open). Depends on Phase 1 (converged
content is the precondition for a single lineage). Verification: fresh wsflow and
fresh ws bootstraps emit the same head ordinal; opening a fresh wsflow project
with ws produces no artifact change and re-stamps to head.

### Phase 3: Fail-loud version-skew guard (above-head / unknown tag)

Guard a tag **above the running package's own template head**, or absent from its
known lineage, across two surfaces: (1) **detect/warn** — extend the Go staleness
banner (`bootstrap_alarm.go`, surfaced via `ferrule`/`workflow_manual`; spec
`mcp-tools.md #260703`, today warning only on the stale-behind direction) to also
fire on the above-head/unknown-tag direction; (2) **refuse** — the agent-run
`lead-bootstrap` skill instruction to stop rather than reconcile/restamp.
**Honest enforcement contract:** bootstrap reconcile is agent-executed (no Go
code performs it), so the refuse is a skill-level instruction backed by the
code-level warning, **not a mechanical hard-block** — state this limit rather
than imply a code-enforced block. This closes the `ws → wsflow`
downgrade-corruption path and the currently-unspecified out-of-range-tag gap;
keep `wsflow → ws` (below-head upgrade) allowed. Depends on Phase 2 ("own
template head" is well-defined against the shared counter). Verification: the
extended staleness detection is unit-tested on an above-head tag; wsflow opening
a shared-head ws project surfaces the warning and the skill leaves artifact + tag
unchanged; ws opening a below-head project proceeds to a clean re-stamp.

### Phase 4: Invert the guard test and reconcile specs/docs

Rewrite `test_bootstrap_template_uses_wsflow_local_version_lineage` to assert
**convergence** for both scaffolded pairs (`AGENTS.template.md` and
`WORKFLOW.md`): each emits identical package-neutral output modulo the version
tag, and the shared counter head matches. The comparison runs on the **emitted
(fresh-mode) body**, not the raw template: the test must apply the same
fresh-mode emit transform `lead-bootstrap.md` uses — strip the scaffold-only HTML
comments (the `<!-- MIGRATION CHECKLIST -->` block Phase 2 keeps in the raw file,
plus the Inclusion-test note per Phase 1's fresh-mode confirmation) — to each raw
template before diffing, so the retained ws `v0001..v0047` checklist does not
false-fail the convergence assertion. Add coverage for the fail-loud guard's
code-level detection (the skill-instruction refuse is asserted by documentation,
not a unit test — see Phase 3's enforcement contract).
Update the spec anchors named in `## Spec Impact` and the `wsflow-mirroring.md`
Bootstrap Template Rules; record the override of `260728`'s Non-Scope for both
the template and the WORKFLOW.md guide. Depends on Phases 1-3 (the enforced state
must exist before the test pins it). Verification: the inverted test fails on any
re-introduced fingerprint/drift in either pair and on a counter split; specs no
longer assert package-local version lineage.

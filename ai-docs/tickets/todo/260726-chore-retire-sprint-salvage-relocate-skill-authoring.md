---
title: "Retire lead-sprint and lead-salvage; relocate lead-skill-authoring out of the distribution surface"
sage-review-design: completed
related:
  260630-epic-skill-playbook-diet: supersedes this epic's lead-sprint diet target and lead-salvage out-of-scope entry
  260605-research-ws-native-subagent-pivot: migration anchor; entry-skill surface shrinks
  260708-feat-lead-revive-hook-replacement: references both retired skills
  260703-chore-review-delegates-true-classification: references both retired skills
  260626-research-playbook-print-lead-surface-leak: references both retired skills
  260702-research-destructive-dedup-methodology: references lead-skill-authoring as method source
spec-remove:
  - 260505-sprint-session-container
  - 260523-sprint-episode-workflow-shell
  - 260513-wsflow-sprint-skill
  - 260510-salvage-recovery-workflow-skill
related-mental-model:
  - workflow-skills
---

# Retire lead-sprint and lead-salvage; relocate lead-skill-authoring out of the distribution surface

## Background

Three `lead-*` skills carry no inbound routing from any other live skill or
playbook body, but each occupies the shipped distribution surface and the
user-invocable `/ws:<name>` trigger list:

- `lead-sprint` (155-line body) — an episode-oriented session shell. Its
  distinguishing feature, the `sprint/` branch container, was already removed by
  `260523-refactor-lead-sprint-episode-shell`; what remains routes real work back
  out to `lead-proceed`/`lead-implement`.
- `lead-salvage` (212-line body) — premise-collapse recovery. No recorded
  invocation; its ticket-writing half already delegates to `lead-write-ticket`.
- `lead-skill-authoring` (146-line body) — this is a different case. It is not
  unused: it is the authoring and audit manual for *this* repository's own
  skills and playbooks, and `AGENTS.md` binds it as a mandatory pre-edit read.
  It is upstream maintenance documentation that is currently shipped downstream
  as an invocable skill, which is a packaging error rather than a dead skill.

Grep across `agents-plugin/{rsrc,skills}` and `agents-plugin-wsflow/{rsrc,skills}`
finds **zero** references to any of the three hyphenated skill names from any
other skill or playbook body. This is the same evidence bar the diet epic used to
authorize the `lead-verify-design` deletion on 2026-07-20.

That grep is scoped to the skill name, and there is exactly one **semantic**
caller it does not catch: `agents-plugin/rsrc/lead-implement/lead-implement.md:84`
(mirrored at `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md:84`)
reads "Stop for the user's choice: merge, new slice, sprint, or stop." After
`lead-sprint` is gone that offers a destination that does not exist, so it is an
edit surface, not an incidental mention.

## Decisions

- **`lead-sprint`: delete.** Remove the skill shim, the rsrc playbook body, the
  wsflow shim, the wsflow rsrc body, and both manifest entries. Its sprint-edit
  micro-edit episode gets no replacement and is not reintroduced elsewhere. No
  successor route was decided; `lead-implement` direct-edit mode is the nearest
  existing path, but nothing in this ticket redirects callers to it.
- **`lead-salvage`: delete.** Same surfaces. It has no wsflow skill shim (only a
  mirrored rsrc body), so its wsflow footprint is one file plus a manifest entry.
- **`lead-skill-authoring`: relocate, do not delete.** The body content survives
  verbatim; only its *distribution* is removed. The rsrc playbook body — not the
  9-line SKILL.md shim — is the substance and becomes the relocated document
  under `ai-docs/ref/`. `_index.md` references it under an auditing-oriented
  heading (e.g. "On auditing skill/playbook content") so a session looking for
  authoring rules still finds it without a plugin round-trip.
- **`enter.sprint` and `enter.salvage`: delete the MCP tools too.** Confirmed by
  the user 2026-07-26, satisfying the `AGENTS.md` always-ask requirement for
  deleting functionality and changing protocol semantics. Each is a static mode
  switch with no resolver, no facts, and no verdict, and each has exactly one
  caller, which Phase 1 removes; leaving them would keep two tools in the public
  MCP schema that no procedure can reach.
  The rejected alternative was keeping the handlers as caller-less CLI-testable
  primitives — the same argument that preserved `format: "json"` in
  `260726-feat-enter-verdict-scenario-output`. It was rejected because the
  deletion decision and its rationale are recoverable from Git history, so
  retaining dead surface buys nothing that the history does not already provide.
  Phase 4 carries this work and is no longer gated; it is ordered last only
  because its verification surface is the served tool set rather than the skill
  set.
- **Phase boundaries follow blast radius, not skill count.** The two deletions
  share identical mechanics and land together; the relocation is separate because
  it must repoint a binding `AGENTS.md` invariant in the same commit that moves
  the file.

## Constraints

- **`AGENTS.md` binds `lead-skill-authoring` as a hard invariant in three
  places** — Code Standards rule 5 (`AGENTS.md:79`), the Documentation System
  index (`:160`), and the pre-edit read list (`:169`). All three name the path
  `agents-plugin/skills/lead-skill-authoring/SKILL.md`. These must be repointed to
  the new `ai-docs/ref/` path in the same commit that removes the file; a phase
  that deletes the skill and defers the `AGENTS.md` edit leaves the repository's
  own root rules pointing at a missing file.
- **`skills_mirror.go` carries `lead-salvage` and `lead-skill-authoring` in its
  `disqualifyingTokens` denylist**
  (`agents-plugin-tool/internal/wsrsrc/skills_mirror.go:36-37`).
  That list gates *substitution-mirrored generation by content*, not distribution,
  so the entries do not block deletion. **Decided: leave both entries in place** as
  inert defensive guards, and record why in the phase that touches the file.
  Removing them would newly permit mirroring of any future skill body mentioning
  the retired names — inert only while no such body exists, which is not a
  property worth depending on. Phase 1's and Phase 3's grep allowlists assume this
  decision; reversing it invalidates both.
- **Mental-model bullets carry stable `{#slug}` anchors.** Follow the
  `lead-verify-design` precedent: mark removed rather than silently deleting an
  anchored bullet, so anchor references elsewhere resolve to an explicit tombstone.
- **`mental-model/workflow-skills.md:73` states "The four core lead skills call
  one `enter.*` tool each"** and enumerates all four. After Phase 1 this is two.
  This bullet is load-bearing for `enter.*` call-site placement and must be
  rewritten, not just trimmed.
- **`runtime.json` declares the served tool set and is asserted exactly.** Both
  `agents-plugin/runtime.json:19-20` and `agents-plugin-wsflow/runtime.json:22-23`
  carry `enter.sprint` / `enter.salvage` capability ranges, and
  `agents-plugin-tool/cmd/ws-mcp/main_test.go` compares the served tool set to
  each contract with `slices.Equal` (full ws at `:91`, wsflow at `:178`). The
  wsflow launcher matches the same manifest with
  `runtime_capabilities.match "exact"`, so a partial edit breaks wsflow at
  runtime, not just in tests. Any change to the served tool set must edit both
  files in the same commit. Note the interaction with the version rule below:
  the tool-name keys in `runtime.json` are hand-edited here; the version string
  in the same file is not — that remains the bump script's alone.
- **`CHANGELOG.md` is append-only history.** It mentions all three names; do not
  rewrite historical entries.
- **`.done/`, `.dropped/`, and `ai-docs/.plans/` are historical artifacts.** They
  are the bulk of the raw grep hit count (62/42/78 total hits) and are explicitly
  out of the cascade scope. `.dropped/260619-research-claude-teammate-mode-subagent-collection-doc-gap.md`
  is the single `.dropped/` hit.
- **Version bump on dev-merge** runs through
  `agents-plugin-tool/scripts/bump-ws-version.sh`; never hand-edit edition points.

## Prior Art

- `lead-verify-design` deletion, executed 2026-07-20 under
  `260630-epic-skill-playbook-diet`, plan
  `ai-docs/.plans/2026-07/20-1520-verify-design-diet.md`. It establishes the full
  removal sweep this ticket repeats: rsrc bodies, wsflow shim, manifests, Go
  golden test, wsflow inventory test, and spec/mental-model/`wsflow-mirroring`/
  `_index.md` reference updates.
- `260523-refactor-lead-sprint-episode-shell` already removed sprint's branch
  container, which is why the remaining body is a thin router.

## Spec Impact

Target areas: `ai-docs/spec/workflow-skills.md` and `ai-docs/spec/mcp-tools.md`.

Removed stems (declared in `spec-remove:`), all in `workflow-skills.md`:

- `260505-sprint-session-container` and `260523-sprint-episode-workflow-shell` —
  the `## Sprint Session Shell` section spans `:936-955` and carries **two**
  anchors: the section heading at `:936` and a second at `:955` closing the
  sprint-edit episode paragraph. Both must be declared, or deleting the section
  silently removes an undeclared stem. Neither is referenced outside `.done/`.
- `260513-wsflow-sprint-skill` — the wsflow sprint mirror paragraph, `:213-221`
  (anchor at `:221`).
- `260510-salvage-recovery-workflow-skill` — the two salvage paragraphs,
  `:401-416`.

Edited, not removed, in `workflow-skills.md`:

- The skill-name list fence closes at `:45`; the three entries to drop are `:35`,
  `:37`, and `:38`.
- The invocable-surface sentence loses `lead-sprint`, `lead-salvage`, and
  `lead-skill-authoring` from the `/ws:<name>` enumeration (`:54-58`). The
  "15 entry skills" count that becomes 12 is on `:53`, one line above.
- `260610-entry-skill-surface-reduction` (`:62-72`) — `:68` reads "Context-heavy
  entry skills (lead-discuss and lead-sprint) are an exception: their SKILL.md
  carries a parallel init declaration". The exception survives with `lead-discuss`
  alone. This is a live stem edit, not an incidental mention.
- `260513-wsflow-agentless-skill-surface` (`:201-211`) loses `lead-sprint` from
  the shipped list.
- The wsflow exclusion sentence is `:223-225`, **not** `:227-229` — `:225` holds
  the end of the exclusion sentence and the start of the unrelated thin-shim
  sentence. Do not delete it as part of the `260513-wsflow-sprint-skill` removal
  above; it is a separate sentence that reduces to `lead-write-skeleton` alone
  once the other two names are gone.
- The documentation-closure sentence (`:782`) loses its `lead-sprint` clause.
- `260514-skill-authoring-carried-context` (`:199`) must state where the
  authoring manual now lives.

Edited in `mcp-tools.md`, in Phase 4, since these describe the tools rather than
the skills:

- `:261` enumerates `enter.implement`, `enter.proceed`, `enter.sprint`, and
  `enter.salvage` as the typed mode switches.
- `:366-368` carry per-mode todo-derivation bullets for both retiring tools (the
  salvage bullet wraps onto `:368`).
- Both sit inside `## Session State Tools {#260625-session-state-tools}`, an
  anchor that survives — hence its correct absence from `spec-remove:`.

Caller-visible change: three `/ws:<name>` entry points disappear from the public
surface in Phases 1-2, and two MCP tools disappear in Phase 4.
Contract-first spec: no — this removes existing documented behavior rather than
introducing new contract.

## Phases

### Phase 1: Retire the lead-sprint and lead-salvage skills

Delete both skills from the distribution surface, leaving the tree green. The
`enter.sprint` / `enter.salvage` tools are **not** touched here — see Phase 4.

Surfaces to remove:

- `agents-plugin/skills/lead-sprint/SKILL.md`,
  `agents-plugin/skills/lead-salvage/SKILL.md`
- `agents-plugin/rsrc/lead-sprint/`, `agents-plugin/rsrc/lead-salvage/`
- `agents-plugin-wsflow/skills/lead-sprint/`,
  `agents-plugin-wsflow/rsrc/lead-sprint/`, `agents-plugin-wsflow/rsrc/lead-salvage/`
- Manifest entries in `agents-plugin/skills/manifest.json`,
  `agents-plugin/rsrc/manifest.json`, `agents-plugin-wsflow/rsrc/manifest.json`
  (regenerate; do not hand-edit hashes)
- Assertions in `agents-plugin-tool/internal/mcp/playbook_tools_test.go` and
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`

Edits in this phase, on contact:

- `agents-plugin/rsrc/lead-implement/lead-implement.md:84` and its wsflow mirror
  — drop `sprint` from the closeout choice list. No successor is substituted.
- The `workflow-skills.md` removals and edits listed under Spec Impact, except
  the `lead-skill-authoring` clauses, which belong to Phase 2.
- The wsflow exclusion sentence (`:223-225`) loses `lead-salvage` now, so the
  spec does not spend a commit documenting an exclusion policy for a deleted
  skill. Phase 2 removes `lead-skill-authoring` from the same sentence.
- `mental-model/workflow-skills.md` — six bullets, not two. The full sprint /
  salvage surface in that file is:
  - `:73` ({#260625-ws-session-state-machine}) — "four core lead skills call one
    `enter.*` tool each" becomes two. Required in Phase 1 even though the tools
    survive until Phase 4, because the bullet is about which *skills* call them.
  - `:72` ({#260510-salvage-recovery-workflow-skill}) — anchored; mark-remove per
    the `lead-verify-design` precedent rather than deleting.
  - `:101` — `lead-salvage`-only and **unanchored**, so the mark-remove precedent
    does not apply; this is a plain deletion.
  - `:18` ({#260609-rsrc-playbook-distribution},
    {#260610-entry-skill-surface-reduction}), `:19`
    ({#260513-wsflow-agentless-skill-surface}), and `:78`
    ({#260505-workflow-primitive-reference}) each cite
    "(`lead-discuss`, `lead-sprint`)" as the parallel-init exception. The
    exception itself survives with `lead-discuss` alone.
  - `:80` ({#260505-implementation-workflow-skills}) — "`lead-sprint` routes
    larger work back through normal implementation gates" goes.
  `:35` ({#260514-skill-authoring-carried-context}) also names a retired skill but
  belongs to Phase 2; see there.

Verification: `go test ./...` in `agents-plugin-tool`, the wsflow bundle test,
`ws/spec_index_verify`, and a repo grep for either skill name whose only
surviving hits are the four allowlisted classes below. The allowlist is not a
loophole — each class is scheduled elsewhere or deliberately retained:

1. History: `CHANGELOG.md`, `.done/`, `.dropped/`, `.plans/`.
2. The ticket graph — Phase 3.
3. The Phase 4 Go surface: `session_state.go:685`/`:697` doc comments,
   `:687`/`:699` derive functions, `:1129-1135` handlers, `server.go`,
   `runtime.json` ×2, `session_state_test.go`, `mcp-tools.md`.
4. The retained `skills_mirror.go:36-37` guards and the `wsflow-mirroring.md`
   text documenting them — see Phase 3 for the per-site split.

### Phase 2: Relocate lead-skill-authoring to ai-docs/ref/

Move the content out of the distribution surface without losing it.

- Move `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md` to
  `ai-docs/ref/skill-authoring.md`. The destination basename is fixed here on
  purpose: five separate sites must be repointed at the exact literal path in the
  same commit, so it cannot be left to implementer choice. Strip the
  `kind: print` frontmatter, which is playbook-serving metadata with no meaning
  outside rsrc. Preserve the body otherwise verbatim; this phase is a relocation,
  not a rewrite.
- Delete `agents-plugin/skills/lead-skill-authoring/` (SKILL.md and
  `agents/openai.yaml`), `agents-plugin/rsrc/lead-skill-authoring/`,
  `agents-plugin-wsflow/rsrc/lead-skill-authoring/`, the two `defaultPrompt`
  strings in `agents-plugin/.codex-plugin/plugin.json` (`:28-29` — these are
  Codex prompt suggestions, not a skill registration entry), and all three
  manifest entries.
- Repoint all three `AGENTS.md` sites (`:79`, `:160`, `:169`) at the new path in
  this same commit.
- Repoint `ai-docs/_index.md` — it carries the same class of binding pointer as
  `AGENTS.md` and the same same-commit rule applies. Four sites: `:66`, the
  "Read Before Editing" table row naming the old SKILL.md path; `:79`, the prose
  "Before editing skill, agent, prompt, or convention text, read
  `agents-plugin/skills/lead-skill-authoring/SKILL.md`"; `:41`, which claims the
  Codex UI install has verified `ws:lead-skill-authoring`, a skill that will no
  longer be installable; and `:157`, which describes `workflow-skills.md` as
  covering "sprint work".
- The `_index.md:64-77` "Read Before Editing" table is the correct home for the
  relocated document — it is the existing register for exactly this class of
  mandatory pre-read reference. Retitle the row's description toward auditing
  ("On auditing skill/playbook content") rather than adding a separate heading;
  the document is now found by reading, not by invoking a skill, so the pointer
  must say when to read it.
- Update `{#260514-skill-authoring-carried-context}` in **both** files: the spec
  copy at `workflow-skills.md:199` and the mental-model copy at
  `mental-model/workflow-skills.md:35`, which states the audit covers
  `agents-plugin/skills/*/SKILL.md` and `agents-plugin/rsrc/lead-*/lead-*.md`.
  Both must point at the relocated document instead.
- Remove `lead-skill-authoring` from the `260513-wsflow-agentless-skill-surface`
  exclusion sentence, which then reduces to `lead-write-skeleton` alone.

Verification: same test set as Phase 1, plus a check that no live file references
the old plugin path.

### Phase 3: Ticket-graph cascade and closing sweep

Update every live ticket that names a retired skill, then verify globally.

Live tickets to reconcile:

- `todo/260630-epic-skill-playbook-diet` — remove `lead-sprint` from the Phase 3/4
  curated target list and `lead-salvage` from Out of Scope; record that this
  ticket superseded both entries rather than deleting them silently.
- `todo/260708-feat-lead-revive-hook-replacement`
- `todo/260703-chore-review-delegates-true-classification`
- `todo/260702-research-destructive-dedup-methodology`
- `todo/260722-feat-goal-run-autonomy-posture`
- `todo/260716-feat-mental-model-comment-placement-rule`
- `todo/260620-bug-ws-delegate-playbook-output-language-unbound`
- `idea/260605-research-ws-native-subagent-pivot`
- `idea/260626-research-playbook-print-lead-surface-leak`

Also sweep `ai-docs/ref/codex-integration.md`, and `ai-docs/ref/wsflow-mirroring.md`
**per site** — its five hits split three ways and a generic sweep gets them wrong:

- `:44` — `lead-sprint` in the shipped wsflow skill list. Remove.
- `:59-60` — `lead-salvage` and `lead-skill-authoring` under "Excluded:". These
  must track the spec's exclusion sentence, which Phases 1-2 reduce to
  `lead-write-skeleton` alone. Remove both.
- `:156` and `:259-260` — documentation of the `skills_mirror.go` denylist, which
  the Constraints retain as inert guards. These must stay and stay accurate.

Left deliberately unchanged: `ai-docs/_index.md:244` ("the additive `lead-sprint`
closeout are all recorded") is historical phase narrative, not a live pointer.
Stated here so the closing grep does not trip on it.

Closing verification: `ws/references_trace`, `ws/spec_index_verify`, full Go and
wsflow test runs, and a final grep using the same four-class allowlist Phase 1
defines — minus class 2, since this phase clears the ticket graph, and minus
class 3 if Phase 4 has already run.

### Phase 4: Retire the enter.sprint and enter.salvage MCP tools

The served tool set is asserted for exact equality against two hand-maintained
contracts, so every surface below must land in one commit or the build fails and
the wsflow launcher breaks at runtime:

- `agents-plugin/runtime.json:19-20` and `agents-plugin-wsflow/runtime.json:22-23`
  — remove both capability entries. Hand-edit the tool-name keys only; the
  version string in these files stays owned by `bump-ws-version.sh`.
- `agents-plugin-tool/internal/mcp/server.go` — dispatch cases `:559-562`,
  schema blocks at `:3488` and `:3502`.
- `agents-plugin-tool/internal/mcp/session_state.go` — the handlers
  `handleEnterSprint` / `handleEnterSalvage` at `:1129-1135`, and separately the
  now-unreferenced `deriveSprintTodos` (`:687`) and `deriveSalvageTodos` (`:699`)
  together with their doc comments at `:685` and `:697`, which are the last live
  mentions of either skill name in Go.
- `agents-plugin-tool/internal/mcp/session_state_test.go:406-410` — asserts the
  todo key sets of both derive functions by direct call. Deleting the functions
  without this edit is a **compile** error that stops the whole `internal/mcp`
  test package from building, not a failing assertion. The other `"sprint"`
  occurrences in that file (`:819`, `:2809-2822`) are unrelated agenda-key
  strings; leave them.
- `ai-docs/spec/mcp-tools.md:261` and `:366-367` — see Spec Impact.

Verification: `go test ./...` must pass, with specific attention to the exact
tool-set comparisons in `agents-plugin-tool/cmd/ws-mcp/main_test.go` — the
`slices.Equal` assertions at `:91` (full ws) and `:178` (wsflow), which are what
catch a half-finished edit. Do not mistake `:159` and `:653` for the tripwire;
those are the contract-*read* helper call sites, not the comparisons.

## Out of Scope

- The remaining `260630-epic-skill-playbook-diet` targets (`lead-write-spec`,
  `lead-add-rule`, `lead-workflow-manual`). This ticket removes two of that
  epic's entries; it does not execute the rest of the diet.
- `260726-feat-enter-verdict-scenario-output`. That ticket already scopes
  `enter.sprint`/`enter.salvage` as out of scope for verdict-scenario work; this
  ticket's Phase 4 removes them outright, so the two are compatible and must not
  be sequenced against each other.
- Rewriting the relocated `lead-skill-authoring` body. Content revision is a
  separate concern from packaging.
- Any replacement for sprint-edit or salvage workflows.

## Blocked (2026-07-26)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | enter.sprint / enter.salvage disposition is explicitly undecided and gates Phase 1 | critical | missing |
| 2 | runtime.json tool contracts omitted; Phase 1 verification is guaranteed to fail | critical | autonomous |
| 3 | ai-docs/spec/mcp-tools.md is not in the Spec Impact scope but documents both tools | important | autonomous |
| 4 | session_state_test.go omitted; deleting the derive funcs breaks compilation | important | autonomous |
| 5 | spec-remove list is incomplete — the Sprint Session Shell section holds a second anchor | important | autonomous |
| 6 | _index.md carries the same binding lead-skill-authoring path guarded only in AGENTS.md | important | autonomous |
| 7 | lead-implement offers "sprint" as a live user choice with no successor after deletion | important | autonomous |
| 8 | Spec Impact line references for the wsflow exclusion sentence are wrong | minor | autonomous |
| 9 | Phase 1 leaves the spec asserting wsflow excludes a skill that no longer exists | minor | autonomous |
| 10 | Phase 2 does not name the relocation target filename | minor | autonomous |

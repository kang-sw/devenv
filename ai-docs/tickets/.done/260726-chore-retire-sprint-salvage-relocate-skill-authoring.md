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
sage-review-completeness: completed
completed: 2026-07-28
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
  the `## Sprint Session Shell` section spans `:974-993` and carries **two**
  anchors: the section heading at `:974` and a second at `:993` closing the
  sprint-edit episode paragraph. Both must be declared, or deleting the section
  silently removes an undeclared stem. Neither is referenced outside `.done/`.
- `260513-wsflow-sprint-skill` — the wsflow sprint mirror paragraph, `:213-221`
  (anchor at `:221`).
- `260510-salvage-recovery-workflow-skill` — the two salvage paragraphs,
  `:410-425` (anchor closing the second paragraph at `:425`).

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
- The documentation-closure sentence (`:820`) loses its `lead-sprint` clause.
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
  - `:76` ({#260625-ws-session-state-machine}) — "four core lead skills call one
    `enter.*` tool each" becomes two. Required in Phase 1 even though the tools
    survive until Phase 4, because the bullet is about which *skills* call them.
  - `:75` ({#260510-salvage-recovery-workflow-skill}) — anchored; mark-remove per
    the `lead-verify-design` precedent rather than deleting.
  - `:104` — `lead-salvage`-only and **unanchored**, so the mark-remove precedent
    does not apply; this is a plain deletion.
  - `:18` ({#260609-rsrc-playbook-distribution},
    {#260610-entry-skill-surface-reduction}), `:19`
    ({#260513-wsflow-agentless-skill-surface}), and `:81`
    ({#260505-workflow-primitive-reference}) each cite
    "(`lead-discuss`, `lead-sprint`)" as the parallel-init exception. The
    exception itself survives with `lead-discuss` alone.
  - `:83` ({#260505-implementation-workflow-skills}) — "`lead-sprint` routes
    larger work back through normal implementation gates" goes.
  `:35` ({#260514-skill-authoring-carried-context}) also names a retired skill but
  belongs to Phase 2; see there.

Verification: `go test ./...` in `agents-plugin-tool`, the wsflow bundle test,
`ws/spec_index_verify`, and a repo grep for either skill name whose only
surviving hits are the four allowlisted classes below. The allowlist is not a
loophole — each class is scheduled elsewhere or deliberately retained:

1. History: `CHANGELOG.md`, `.done/`, `.dropped/`, `.plans/`.
2. The ticket graph — Phase 3.
3. The Phase 4 Go surface: `session_state.go:717`/`:729` doc comments,
   `:719`/`:731` derive functions, `:1161-1166` handlers, `server.go`,
   `runtime.json` ×2, `session_state_test.go`, `mcp-tools.md`.
4. The retained `skills_mirror.go:36-37` guards and the `wsflow-mirroring.md`
   text documenting them — see Phase 3 for the per-site split.

### Result (1a96ba1e) - 2026-07-28

Phase 1 complete. Both skills removed from every distribution surface: the two
`agents-plugin/skills/` directories, the three `rsrc/` bodies across both
lineages, the wsflow `skills/lead-sprint/`, and all three manifests via the
env-gated regen tests (no hand-edited hashes). `playbook_tools_test.go` lost
`TestPlaybookPrintGoldenLeadSprint` and `TestPlaybookPrintGoldenLeadSalvage`
plus their table cases, and `repointed` reduced to `lead-proceed`/`lead-discuss`.
`test_wsflow_skill_bundle.py` lost both `EXPECTED_SKILLS` entries, the
`lead-sprint` title and pointer-tail entries, and `EXPECTED_PARALLEL_INIT_SKILLS`
reduced to `lead-discuss`/`lead-goal-fan-out-step`; two new forbidden-reference
patterns guard against reintroduction, since these skills were retired outright
rather than merely excluded from wsflow.

Spec `workflow-skills.md`: four anchored stems removed, entry-skill count
15 -> 13, the parallel-init exception reduced to `lead-discuss`, the wsflow
shipped list and exclusion sentence reduced, and the doc-closure sentence
stripped of its `lead-sprint` clause. Mental model: all six scheduled bullets
handled with the split the plan called for -- `:75` mark-removed as a tombstone
per the `lead-verify-design` precedent, `:104` plainly deleted as unanchored,
and `:17` rewritten to defer to the spec enumeration rather than restate a count
that drifts on every skill change.

Deviation: `:76` was worded as "`enter.sprint` and `enter.salvage` are
caller-less until they are removed" rather than asserting the tools were gone.
The first draft claimed retirement that had not happened yet at this commit;
Phase 4 settles the sentence.

Verification: full Go suite (12 packages, `-count=1`), 10 wsflow tests, three
regens, `spec_index_verify: ok`, and the closing grep within the four-class
allowlist.

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

### Result (726cfde4) - 2026-07-28

Phase 2 complete. `git mv` moved the manual to `ai-docs/ref/skill-authoring.md`
with the `kind: print` frontmatter stripped and the body otherwise verbatim;
the `agents-plugin/skills/lead-skill-authoring/` and both `rsrc/` directories
are gone, along with the two `defaultPrompt` strings in
`.codex-plugin/plugin.json` (replaced with `lead-discuss` / `lead-proceed`
suggestions) and all three manifest entries. All three `AGENTS.md` sites and all
four `_index.md` sites repointed in the same commit, with the "Read Before
Editing" row retitled toward auditing so the pointer states when to read rather
than what to invoke. Both copies of `{#260514-skill-authoring-carried-context}`
now name the relocated document; the spec copy additionally records that these
rules are an upstream reference read directly, not a shipped invocable skill.
Entry-skill count 13 -> 12 and the wsflow exclusion sentence reduced to
`lead-write-skeleton`.

`TestPlaybookPrintGoldenLeadSkillAuthoring` was replaced rather than deleted:
`TestSkillAuthoringRelocatedOutOfRsrc` asserts the name no longer resolves as an
rsrc playbook, that the relocated file exists and still carries its doctrine
text, and that it no longer starts with playbook frontmatter. Vacuity-checked by
moving the file aside -- the test failed with "relocated authoring manual
missing" -- then restored by manual edit.

Deviation: `session_state.go:574`'s doc comment referenced the
`lead-skill-authoring` reader model and was repointed on contact. The ticket did
not schedule this site.

Verification: same set as Phase 1, plus a grep confirming no live file
references the old plugin path.

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
- `:60-61` — `lead-salvage` and `lead-skill-authoring` under "Excluded:". These
  must track the spec's exclusion sentence, which Phases 1-2 reduce to
  `lead-write-skeleton` alone. Remove both.
- `:157` and `:260-261` — documentation of the `skills_mirror.go` denylist, which
  the Constraints retain as inert guards. These must stay and stay accurate.

Left deliberately unchanged: `ai-docs/_index.md:244` ("the additive `lead-sprint`
closeout are all recorded") is historical phase narrative, not a live pointer.
Stated here so the closing grep does not trip on it.

Closing verification: `ws/references_trace`, `ws/spec_index_verify`, full Go and
wsflow test runs, and a final grep using the same four-class allowlist Phase 1
defines — minus class 2, since this phase clears the ticket graph, and minus
class 3 if Phase 4 has already run.

### Result (cd84927d) - 2026-07-28

Phase 3 complete. Each cascade site was adjudicated as forward-looking plan
versus historical narrative rather than swept: plan text was corrected, and
narrative recording what happened at the time was left alone. The
`wsflow-mirroring.md` per-site split was followed as written -- `:44` and the
two `Excluded:` entries removed, the `skills_mirror.go` denylist paragraph kept
and relabeled with a note that the entries are deliberately retained as inert
guards, and `lead-sprint` added to the forbidden-distributed-reference list.
`codex-integration.md:34`'s probe record moved to `$ws:lead-write-ticket` /
`$ws:lead-discuss` with a dated relocation note.

Two findings worth carrying forward:

- `ready/260726-bug-inline-playbook-invocation-commit-ownership` has its
  decisive blocker dissolved by this retirement. Its `## Blocked` argues a
  callee-side rule cannot work because "the same callee, `lead-update-spec`,
  needs opposite behavior from two different callers" -- and `lead-sprint.md:97`
  was the only Category C caller. One caller survives, so the stated reason no
  longer holds. Recorded on that ticket.
- `todo/260630-epic-skill-playbook-diet.md:99` carried a malformed phase heading
  that predates this work (confirmed via `git show HEAD:`) and would have blocked
  any commit touching the file. Fixed on contact by splitting into Phase 3 plus a
  `[dropped]` Phase 4, honoring the stable-numbering rule rather than renumbering.

`idea/260605-research-ws-native-subagent-pivot` gained a
`#### Superseded in part` note reversing its "lead-skill-authoring stays entry"
decision and dropping the entry-shim count 11 -> 8.

Verification: `references_trace`, `spec_index_verify: ok`, `tickets.verify: PASS`,
full Go and wsflow runs, and the closing grep with `--exclude-dir` for
`.claude/worktrees/` and `.worktree/`, which are other checkouts of this same
repo and polluted the first sweep.

### Phase 4: Retire the enter.sprint and enter.salvage MCP tools

The served tool set is asserted for exact equality against two hand-maintained
contracts, so every surface below must land in one commit or the build fails and
the wsflow launcher breaks at runtime:

- `agents-plugin/runtime.json:19-20` and `agents-plugin-wsflow/runtime.json:22-23`
  — remove both capability entries. Hand-edit the tool-name keys only; the
  version string in these files stays owned by `bump-ws-version.sh`.
- `agents-plugin-tool/internal/mcp/server.go` — dispatch cases `:559-562`,
  schema blocks at `:3600` and `:3614`.
- `agents-plugin-tool/internal/mcp/session_state.go` — the handlers
  `handleEnterSprint` / `handleEnterSalvage` at `:1161-1166`, and separately the
  now-unreferenced `deriveSprintTodos` (`:719`) and `deriveSalvageTodos` (`:731`)
  together with their doc comments at `:717` and `:729`, which are the last live
  mentions of either skill name in Go.
- `agents-plugin-tool/internal/mcp/session_state_test.go:519-523` — asserts the
  todo key sets of both derive functions by direct call. Deleting the functions
  without this edit is a **compile** error that stops the whole `internal/mcp`
  test package from building, not a failing assertion. The other `"sprint"`
  occurrences in that file (`:932`, `:3103-3112`) are unrelated agenda-key
  strings; leave them.
- `ai-docs/spec/mcp-tools.md:261` and `:366-367` — see Spec Impact.

Verification: `go test ./...` must pass, with specific attention to the exact
tool-set comparisons in `agents-plugin-tool/cmd/ws-mcp/main_test.go` — the
`slices.Equal` assertions at `:91` (full ws) and `:178` (wsflow), which are what
catch a half-finished edit. Do not mistake `:159` and `:653` for the tripwire;
those are the contract-*read* helper call sites, not the comparisons.

### Result (3dc87a56) - 2026-07-28

Phase 4 complete, in one commit as the phase required. Removed: both dispatch
cases and both schema blocks in `server.go`; `handleEnterSprint` /
`handleEnterSalvage` and the now-unreferenced `deriveSprintTodos` /
`deriveSalvageTodos` with their doc comments in `session_state.go`; the derive
assertions in `session_state_test.go` (the unrelated agenda-key hits left
alone); the capability entries in both `runtime.json` files, tool-name keys only;
and the spec enumeration in `mcp-tools.md`, which now names `enter.implement` and
`enter.proceed` and drops the two mode bullets. The mental-model sentence Phase 1
left provisional is settled to record the tools as retired with their callers.

The `main_test.go` tripwire was mutation-checked rather than trusted: re-adding a
lone `enter.sprint` entry to `agents-plugin/runtime.json` made
`TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` fail with the
exact served-versus-contract diff, confirming the assertion is load-bearing.
Restored by manual edit.

Verification: `go test ./... -count=1` green across all 12 packages, three
regens clean with no manifest drift, 10 wsflow tests OK, `spec_index_verify: ok`.

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

## Coordinate Refresh (f9161287) - 2026-07-28

This ticket was authored at `4527e651` and sat on an unmerged branch while the
goal branch landed 120 commits over the same files. Every `file:line` reference
below was re-measured after the merge; the cited content all survived unchanged,
but roughly a third of the coordinates had moved. Refreshed:

- `session_state.go` — doc comments `685`/`697` -> `717`/`729`, derive functions
  `687`/`699` -> `719`/`731`, handlers `1129-1135` -> `1161-1166`.
- `server.go` — schema blocks `3488`/`3502` -> `3600`/`3614`. Dispatch cases
  `559-562` unchanged.
- `session_state_test.go` — derive assertions `406-410` -> `519-523`; the
  unrelated agenda-key hits `819`/`2809-2822` -> `932`/`3103-3112`.
- `spec/workflow-skills.md` — Sprint Session Shell `936-955` -> `974-993` (both
  anchors), salvage paragraphs `401-416` -> `410-425`, documentation-closure
  sentence `782` -> `820`.
- `mental-model/workflow-skills.md` — `72`/`73` -> `75`/`76`, `78` -> `81`,
  `80` -> `83`, `101` -> `104`.
- `ref/wsflow-mirroring.md` — `59-60` -> `60-61`, `156` -> `157`,
  `259-260` -> `260-261`.

Verified unchanged: `AGENTS.md:79/160/169`, `_index.md:41/66/79/157/244`,
`lead-implement.md:84` (both lineages), `skills_mirror.go:36-37`,
`runtime.json` `19-20`/`22-23`, `main_test.go:91/178` and the `159`/`653`
decoys, `mcp-tools.md:261` and `366-368`, and the `workflow-skills.md` skill-name
list, invocable-surface sentence, wsflow mirror paragraph, and exclusion sentence.

Ticket conventions say to point at code by the search that finds it rather than
by surveyed coordinates. This ticket predates that discipline and is unusually
coordinate-dense; the refresh above makes it accurate today, but an implementer
should still grep for the cited content rather than trust a line number.

## Design Review Record (2026-07-26)

Historical. The design posture is `completed` in frontmatter; finding 1 below was
resolved by the user's 2026-07-26 confirmation, recorded under **Decisions**.

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | enter.sprint / enter.salvage disposition is explicitly undecided and gates Phase 1 | critical | resolved 2026-07-26 — user confirmed deletion; Phase 4 ungated |
| 2 | runtime.json tool contracts omitted; Phase 1 verification is guaranteed to fail | critical | autonomous |
| 3 | ai-docs/spec/mcp-tools.md is not in the Spec Impact scope but documents both tools | important | autonomous |
| 4 | session_state_test.go omitted; deleting the derive funcs breaks compilation | important | autonomous |
| 5 | spec-remove list is incomplete — the Sprint Session Shell section holds a second anchor | important | autonomous |
| 6 | _index.md carries the same binding lead-skill-authoring path guarded only in AGENTS.md | important | autonomous |
| 7 | lead-implement offers "sprint" as a live user choice with no successor after deletion | important | autonomous |
| 8 | Spec Impact line references for the wsflow exclusion sentence are wrong | minor | autonomous |
| 9 | Phase 1 leaves the spec asserting wsflow excludes a skill that no longer exists | minor | autonomous |
| 10 | Phase 2 does not name the relocation target filename | minor | autonomous |


## Resolution (2026-07-28)

All four phases landed on `impl/retire-sprint-salvage`: 1a96ba1e (skill retirement), 726cfde4 (skill-authoring relocation to `ai-docs/ref/skill-authoring.md`), cd84927d (ticket-graph cascade), 3dc87a56 (MCP tool retirement).

`lead-sprint` and `lead-salvage` no longer exist in either lineage, and neither do the `enter.sprint` / `enter.salvage` tools that served them. The skill-authoring manual is now an upstream reference document read directly rather than a shipped invocable skill; the entry-skill surface went 15 -> 12.

Two follow-ups this work created rather than closed:

- `ready/260726-bug-inline-playbook-invocation-commit-ownership` lost the blocker it called decisive. Its Category C had exactly one entry (`lead-sprint.md:97`), and the stated reason a callee-side rule could not work -- two callers needing opposite behavior from `lead-update-spec` -- no longer holds with one caller left.
- No successor entry point was named for ad-hoc documentation reconciliation. `lead-sprint`'s wrap-episode was one of only two doors to `lead-update-spec` and `mental-model-updater`, and it refused without a `Sprint-Edit:` marker -- which is precisely the ad-hoc case. `lead-implement`'s `{doc-pre-pass}` is now the only door, and it requires having gone through implement. This ticket deliberately declines to name a successor; the gap is real and unaddressed.

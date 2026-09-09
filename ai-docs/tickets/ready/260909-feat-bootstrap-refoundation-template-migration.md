---
title: "Ship the reduced layout downstream: bootstrap template migration for the refoundation"
sage-review-design: completed
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260909-refactor-retire-spec-mental-model-layers: prerequisite; the layers this migration stops scaffolding must already be gone from the tooling, or the template would ship a layout the runtime still expects
  260909-refactor-lead-surface-collapse-worker-stop-protocol: prerequisite; the collapsed lead surface is what the rewritten workflow guide describes
  260909-refactor-drain-ready-queue-worker-spawner: prerequisite; the worker/stop model the guide describes must exist before it is documented downstream
  260909-refactor-route-resolve-implement-reads-ticket-facts: prerequisite; the ticket-as-plan routing the guide describes must be live first
  260909-chore-retire-mercenary-surface: prerequisite; a downstream guide written while the mercenary surface still exists would document a path being removed
  260825-refactor-ws-wsflow-bootstrap-artifact-convergence: converged the two bootstrap lineages onto one shared version counter; this migration item must be authored once and mirrored, never forked
  260807-refactor-dissolve-project-index: precedent; its disposable `acmewidgets` fixture dogfood is the verification shape reused in Phase 2
sage-review-completeness: completed
sage-review-design-reviewed: 2dd87bcf59174a5b
sage-review-completeness-reviewed: 2dd87bcf59174a5b
---

# Ship the reduced layout downstream: bootstrap template migration for the refoundation

## Background

Every other child of this epic changes how the workflow behaves inside this
repository. None of them reaches a downstream project: a project holds only what
bootstrap installed, and it keeps that shape until a versioned migration item
moves it. The epic's scope line names "a bootstrap template migration that ships
the new shape downstream", and its completion criteria require that "a fresh
bootstrap and an upgraded project converge on the reduced layout". This ticket is
that item.

Concretely, downstream projects today are scaffolded with `ai-docs/spec/`,
`ai-docs/mental-model.md`, and `ai-docs/mental-model/`; their `AGENTS.md` routes
domain-scoped rules into `ai-docs/mental-model/<domain>.md ## Domain Rules`,
carries a `## Spec` commit trailer and a `renamed-spec:` rule, and points
`## Project Memory` at "generated ticket/spec inventories"; their
`ai-docs/WORKFLOW.md` has whole `## Specs` and `## Mental Models` sections; and
bootstrap's fresh handler ends by suggesting the two forge skills. Epic decision
1 retires those layers as hand-maintained derived documents, epic decision 2
names the memory tiers that remain (tickets, commit `## AI Context`, notes, a few
manuals), and epic decision 3 replaces the spec's role: tests are the contract
and the ticket is the plan.

This ticket lands **last** among the children. Its subject is a description of
the other children's end state, so authoring it earlier would ship a guide to
behavior that does not exist yet, and the migration item's version number would
have to be re-cut every time an earlier child changed the shape.

## Decisions

1. **Archive downstream spec and mental-model content; never delete it.** On
   upgrade the item `git mv`s an existing `ai-docs/spec/`,
   `ai-docs/mental-model/`, and `ai-docs/mental-model.md` under `ai-docs/.old/`.
   Rationale: this is user-authored content bootstrap did not write, and epic
   decision 1's verdict is that a *live* drifting intermediary has negative
   value, not that the prose is worthless. `ai-docs/.old/` already exists for
   exactly this — the tracked project archive hidden from default listings —
   and the template's own earlier item already established `git mv`-ing legacy
   spec archives into it.
   Rejected: deleting the directories (destroys content bootstrap did not
   author, and no migration item in the checklist's history deletes user prose).
   Rejected: leaving them in place and only changing the scaffold for fresh
   projects (upgraded and fresh projects would then hold different layouts,
   which fails the epic's convergence criterion, and the drifting layer that
   decision 1 removes would survive in every existing project).
2. **One migration item, not several.** The template version tag is a single
   counter and the upgrade handler walks items forward-only; splitting the
   change into several numbered items makes a partially-migrated project
   reachable, where a project could carry the new commit rules and the old
   scaffold. Precedent: the `_index.md` dissolution shipped as one multi-part
   item with an explicit "one-time migration judgment call" caveat, and its
   fixture dogfood confirmed the upgraded and fresh states converged.
   Rejected: one item per touched section.
3. **Authored once, mirrored to wsflow in the same change.** The two
   `AGENTS.template.md` files were converged onto one shared version lineage and
   their emitted fresh-mode bodies are byte-identical apart from the MCP
   namespace token; a package test enforces that. The new item gets the same
   number in both files.
   Rejected: shipping to the full package first and mirroring later — that
   re-forks the lineage the convergence ticket closed.
4. **The workflow guide states tests-as-contract as an assumption, and
   bootstrap does not check it.** Epic Non-Scope excludes downstream test
   coverage: whether a project's tests can carry the behavioral contract is that
   project's property. The guide says what the workflow now assumes and what
   follows from it; it adds no doctor check, no alarm, and no gate.
   Rejected: a coverage check or warning at bootstrap time.
5. **Add the `### Binding Anchor` scaffold rather than only updating guidance.**
   Survey finding: shipped playbooks already read a project's declared
   `### Binding Anchor` from `AGENTS.md` through the generic hook, but the
   shipped `AGENTS.template.md` has no such section and no binding-anchor read
   step in `## Project Memory`. The hook has consumers and no scaffold, so a
   downstream project has no way to learn the declaration exists. The item adds
   the optional section with the two required keys and the corresponding
   `## Project Memory` step.
6. **Dogfood on a disposable fixture before release.** Two runs, mirroring the
   `_index.md` precedent: an idempotency run on an already-migrated project and
   a full migration run on a fixture built at the previous template tag with
   live spec and mental-model content. Rejected: releasing on unit tests alone —
   both prior bootstrap-touching tickets found compiled-code dependencies only
   during execution, which the evidence audit records as the expected pattern.
7. **This repository's own `### Binding Anchor` replacement belongs to the
   epic's board-reconciliation step, not to this ticket.** The epic lists it as
   a Planned reminder next to board reconciliation, and its completion criteria
   already track "the binding anchor declaration points at the evidence-audit
   ticket" at epic level. Two further reasons: the edit is gated on the anchor
   ticket being populated, which is independent of every phase here; and folding
   a repository-local `AGENTS.md` edit into the one ticket whose whole subject is
   shipped downstream text puts local and shipped edits in the same commit range,
   which is precisely the confusion Architecture Rule 4 warns about. If the user
   prefers it bundled, this ticket's last phase is the fallback slot.

- **This ticket owns every sentence in the template, the shipped guide, and
  the bootstrap playbook; the spec-retirement sibling owns tools and embedded
  conventions.** The guide's `## Tickets` sentence that calls `ready/` "the
  spec-addressed implementation-ready status" and any template line that
  describes promotion under the spec-address gate are rewritten here, in
  Phase 1, to the sage-reviewed gate epic Cross-Child Decision 8 keeps. No
  two children edit the same downstream sentence. *Rejected: inherit the
  wording from the sibling* — it has disclaimed the template.
- **Superseded checklist items are marked, not left live.** The template's
  `[obsoleted by vNNNN]` mechanism applies to every earlier item the new
  item makes wrong — the items that call the spec-authoring, forge, and
  mental-model-updater skills, the item that adds the `## Spec` trailer, and
  the item that rewrites `ai-docs/spec/` content — because `adopt` walks
  every item from the first, not only the head. *Rejected: rely on the
  forward-only walk* — adopt has no such protection.
- **Landing the item is the deliverable; the release is not.** The package
  release that makes the new head tag visible to installed projects is
  `lead-ship`'s, user-approved (epic Cross-Child Decision 13). Phase 2 ends
  with the three runs recorded; nothing in this ticket tags or publishes.
- **Convergence is compared over template-managed sections only.** The
  bootstrap invariant leaves project-specific sections alone, so the
  comparison in Phase 2 run (b) covers the sections the upgrade handler
  rewrites, plus `ai-docs/WORKFLOW.md` whole; project-specific content is
  excluded by construction. *Rejected: byte equality of `AGENTS.md`* —
  unreachable for any fixture with content.
- **No README under `ai-docs/.old/`.** The commit message and the archive
  step in the item are the record (epic Cross-Child Decision 2's memory
  tiers); a generated README is a fifth copy of the same sentence.

## Constraints

- **Shipped-surface rule (`AGENTS.md` Architecture Rule 4, epic decision 11).**
  Everything this ticket touches ships: the template body, the migration item,
  the workflow guide, and the bootstrap playbook all run in projects that hold
  only what bootstrap installed. No refoundation vocabulary, no ticket stems, no
  epic names, no package directory names, no commit hashes. The migration item
  must read as a plain conditional instruction ("if `ai-docs/spec/` exists,
  `git mv` it under `ai-docs/.old/spec`"), never as a citation. Any
  project-specific input the new guidance needs is read through a generic hook —
  a declared `AGENTS.md` section, a convention read, a config key — with this
  repository declaring its own value behind that hook.
- **wsflow mirroring (`ai-docs/manuals/wsflow-mirroring.md`).** `lead-bootstrap`
  is in the shipped wsflow skill set and, unlike the generated `rsrc/` tree, its
  `AGENTS.template.md` and `WORKFLOW.md` are curated per-package copies. The
  package test compares emitted fresh-mode bodies byte-for-byte after stripping
  the scaffold-only migration blocks and normalizing the version tag, so the two
  files must change together and the MCP namespace token must remain the only
  difference between them.
- **Bootstrap's own invariants.** Every migration item is idempotent —
  re-running on an already-migrated project produces no changes; items apply only
  when their condition is met; project-specific sections (Architecture Rules,
  custom Code Standards and Project Knowledge entries) are never overwritten;
  unresolved conflicts are flagged inline rather than resolved by overwriting.
- **Forward-only upgrades with a second entry path.** The upgrade handler walks
  only items with version above the project's tag and never re-walks; the adopt
  handler instead audits the whole checklist against actual state and stamps the
  head. Both must reach the same end state, and the fresh handler must reach it
  without walking anything.
- **Bumping the head arms the staleness alarm everywhere.** The alarm reads the
  head from `lead-bootstrap/AGENTS.template.md` and compares it against a
  project's installed tag, so raising the head makes every already-bootstrapped
  project — this repository included — report as stale until it upgrades.
- **Do not delete user content.** Restates Decision 1 as a hard boundary for the
  implementation.

## Prior Art

Surveyed by reading the bootstrap skill and playbook, both packages' template
and guide copies, and grepping for the template version tag and its consumers.

**`agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md`** (`kind: print`) —
`## Invariants` (idempotency, never overwrite project-specific sections, surgical
merge with `<!-- CONFLICT: ... -->` markers); the mode detection in `## On:
invoke` and the `fresh` / `upgrade` / `refuse` / `adopt` / `claude-migrate`
handlers; `## On: fresh` step 3 ("Create `ai-docs/` structure per the template
setup block"), step 5 (the `.gitignore` entries this ticket keeps), and step 10
(suggests the two forge skills — must go); the `## On: index health check` route
table, three of whose rows route to a forge skill and must go; and
`judge: migration-condition` (Skip / Apply subset / Apply all), which is the
judgment the new item is evaluated under.

**`agents-plugin/skills/lead-bootstrap/AGENTS.template.md`** (241 lines,
head `<!-- Template Version: v0047 -->`) — the `<!-- MIGRATION CHECKLIST ... -->`
scaffold-only comment carrying items v0001 through v0047 with the "NEVER copy
into a project AGENTS.md" caveat; the `<!-- MIGRATION: Set up ai-docs/ ... -->`
scaffold block whose layout tree lists `mental-model.md`, `mental-model/`, and
`spec/`; the `<!-- Inclusion test: ... -->` comment that routes domain-scoped
rules to `ai-docs/mental-model/<domain>.md ## Domain Rules`; `## Commit Rules`
with its `## Spec` trailer block and the `renamed-spec: <old-stem> -> <new-stem>`
line; `## Project Memory` step 1, which names "generated ticket/spec
inventories"; and the `## Project Knowledge` bullets pointing at
`ai-docs/WORKFLOW.md` and the ticket conventions. Note what is **absent**: the
template has no `### Binding Anchor` section and no binding-anchor read step,
though shipped playbooks read that declaration (Decision 5).

**`agents-plugin/skills/lead-bootstrap/WORKFLOW.md`** (161 lines) — `## Authority
Files`, `## ai-docs/ Layout` (whose bullets name `spec/`, `mental-model.md`, and
`mental-model/`), `## Tickets`, `## Specs`, `## Mental Models`, `## Index
Health`, `## Commit Traceability`, `## Manual Fallback`.

**`agents-plugin-wsflow/skills/lead-bootstrap/`** — curated `AGENTS.template.md`,
`WORKFLOW.md`, and `SKILL.md`. The raw template diff against the full package is
38 lines and consists entirely of the MCP namespace substitution; the guides
differ only in the same way.

**`agents-plugin-tool/internal/mcp/bootstrap_alarm.go`** — `templateVersionTag`
and `templateVersionMarker` regexes, `parseTemplateVersionTag`,
`readTemplateVersion`, `readInstalledVersionState` (which distinguishes "no
marker" from "unparseable marker"), and `latestKnownTemplateVersion`, which
resolves the head by reading `<skillsRoot>/lead-bootstrap/AGENTS.template.md`.
Covered by `bootstrap_alarm_test.go`.

**`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`** — `_MIGRATION_SETUP_BLOCK`,
`_emit_fresh_body` (mirrors the fresh handler's strip step), and
`test_bootstrap_scaffolds_emit_converged_output_across_packages`, which compares
both packages' emitted template bodies and both `WORKFLOW.md` copies. This is the
test that will fail first if the two packages are edited unevenly.
`test_wsflow_runtime_contract.py` covers the reduced runtime contract.

**Corrections to assumed touchpoints, established by reading the files:**

- `agents-plugin-tool/scripts/bump-ws-version.sh` rewrites the plugin manifests,
  both `runtime.json` files, `cmd/ws-mcp/main.go`, the release-asset script, and
  the package-version lines in this repository's root `AGENTS.md`. It does **not**
  touch `AGENTS.template.md` or the template version tag — the template counter is
  bumped by hand and is independent of the package version.
- `agents-plugin-tool/internal/wsdoc/doctor.go` contains no spec or mental-model
  logic at all. The doc-coverage machinery lives in
  `internal/mcp/doc_coverage_alarm.go`, `internal/wsreview/checkpoint.go`, and the
  `doc_coverage_alarm` item in `internal/wsconfig/scope.go`. That alarm is retired
  by the sibling layer-retirement ticket; if any part of it survives, it will fire
  against the archived directories, so this ticket must confirm it is gone before
  the fixture run (the earlier fixture dogfood raised exactly this alarm on spec
  stubs).
- `internal/wsrsrc` tests pin marker handling and skill mirroring, not template
  content. The template-content pins are `bootstrap_alarm_test.go` and the wsflow
  bundle test.

**Verification shape** — the `_index.md` dissolution's Phase 2: a throwaway
`acmewidgets` git fixture built at the previous template tag with live legacy
content, one idempotency no-op run against the already-migrated repository, one
full migration run against the fixture, the staleness alarm observed firing
before and clearing after, and the convergence invariant checked by comparing the
upgraded fixture against a fresh bootstrap.

## Phases

### Phase 1: Author the migration item and the reduced template, guide, and playbook

Goal: cut one new versioned item at the head of the checklist in both packages
and bring the template body, the workflow guide, and the bootstrap playbook to
the state that item produces, so that fresh, upgrade, and adopt all land on the
same layout.

The item's conditional steps:

- If `ai-docs/spec/` exists, `git mv` it to `ai-docs/.old/spec` (merging into an
  existing archive rather than clobbering it). Same for `ai-docs/mental-model/`
  to `ai-docs/.old/mental-model` and `ai-docs/mental-model.md` to
  `ai-docs/.old/mental-model.md`. Create `ai-docs/.old/` first if absent.
- Remove the two directories and the index file from the scaffold layout block
  so fresh projects never create them.
- Rewrite the inclusion-test comment so domain-scoped rules have a home that
  still exists: short universal rules inline, longer or path-scoped rules in
  `ai-docs/manuals/` declared through the conventions section below (epic
  Cross-Child Decision 18).
- Add the optional path-scoped conventions section under `## Workflow`
  (for example `### Implementation Conventions` with `paths` -> manual rows),
  the generic hook the worker playbook reads; a project that declares none
  has no convention read. Scaffold `ai-docs/manuals/` in the layout block if
  it is not already there.
- Instruct a triage of the archived mental-model and spec content before the
  archive step, as a one-time judgment call for the migrating project:
  derivable from code (archive only), prescriptive convention (to a manual
  or inline `AGENTS.md`), site-specific trap (to a code comment),
  non-derivable external fact (to `ai-docs/ref/`). The item states the four
  classes and the destinations; it does not automate the classification.
- Update `## Project Memory` step 1 so it no longer points at a spec inventory,
  and add the binding-anchor read step.
- Add the optional `### Binding Anchor` section under `## Workflow` with its two
  required keys and a note that a project declaring neither has no gate.
- Reconcile `## Commit Rules`: the `## Spec` trailer and the `renamed-spec:`
  line have no target once the layer is gone. Before editing, confirm
  against the spec-retirement sibling's landed Result that no runtime parser
  still consumes the trailer; if one does, record it and treat the section
  as a coordination point.
- Rewrite the template's `## Ticket System` guidance and the guide's
  `## Tickets` section so `ready/` is described as the sage-reviewed
  implementation-ready status with no spec-address wording (Decisions).
- Mark every earlier item the new item supersedes `[obsoleted by v0048]`
  (Decisions): the items that invoke `lead-write-spec`, `lead-forge-spec`,
  `lead-forge-mental-model`, or `mental-model-updater`, the item that adds
  the `## Spec` trailer, and the item that rewrites `ai-docs/spec/` content.
  Find them by grepping the checklist for those names and paths.
- Update any `## Project Knowledge` or `## Architecture Rules` guidance bullet
  that routes deep detail to specs or mental models.
- Keep the `.gitignore` entries the earlier items established; this item adds
  and removes none of them.
- Carry the "one-time migration judgment call, not an automated reconciliation;
  do not build staleness-detection tooling for it" caveat, matching the earlier
  dissolution items.

Alongside the item: rewrite `WORKFLOW.md` — drop `## Specs` and `## Mental
Models` entirely, drop their bullets from `## ai-docs/ Layout`, and add a section
describing the behavioral contract as tests and a section describing the
worker-and-stop execution model, with the tests-as-contract assumption stated
plainly and no enforcement attached (Decision 4). Update
`lead-bootstrap.md`: drop the forge-skill suggestion from the fresh handler,
drop the three index-health route rows that route to a forge skill (behavior
coverage, modification knowledge, project reading map), and check the
handler steps that reference the removed layout. The two new guide sections
also land in this repository's `ai-docs/WORKFLOW.md` (the drafts README
assigns them here). Place the committed drafts
`ai-docs/ref/refound-drafts/bootstrap-template.md` and
`workflow-guide-sections.md`: move the text, delete each draft and its
README row, fresh-reader audit once on each placed file.

Verification expectations:

- Both packages' `AGENTS.template.md` carry the same new head tag, and the
  wsflow bundle test's converged-emission assertion passes unchanged — the only
  difference between the emitted bodies is still the namespace token.
- `bootstrap_alarm_test.go` passes and `latestKnownTemplateVersion` resolves to
  the new head from both skill roots.
- Reading `playbook.read(name: "lead-bootstrap")` in both product modes yields
  no reference to a spec or mental-model layer and no forge-skill suggestion.
- Read every changed sentence as a lead in a project that has never heard of
  this repository: no ticket stems, no epic names, no package paths, no
  refoundation vocabulary.
- Full Go suite and both wsflow test modules green.

Touchpoints: `agents-plugin/skills/lead-bootstrap/AGENTS.template.md`,
`agents-plugin/skills/lead-bootstrap/WORKFLOW.md`,
`agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md`,
`agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md`,
`agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md`,
`agents-plugin-wsflow/skills/lead-bootstrap/SKILL.md` (only if its description
names a removed layer), `agents-plugin-wsflow/rsrc/lead-bootstrap/lead-bootstrap.md`
(generated mirror; regenerate with the mirroring manual's env var),
`ai-docs/WORKFLOW.md`, `ai-docs/ref/refound-drafts/README.md`,
`agents-plugin-tool/internal/mcp/bootstrap_alarm_test.go`,
`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`.

### Phase 2: Dogfood the upgrade on a disposable fixture

Sequentially dependent on Phase 1: this phase runs the item Phase 1 authored,
against a live build, and can only be done once the item exists downstream-shaped.

Goal: prove the three entry paths converge and that no compiled-code dependency
was missed, before the item reaches any real project.

- **Run (a) — idempotency.** Run bootstrap upgrade against this repository. By
  the time this ticket lands, the sibling layer-retirement ticket has already
  removed this repository's own spec and mental-model directories, so run (a) is
  the *no-op-plus-tag-bump* case: the archive steps must all evaluate to Skip and
  the only change is the template-managed section updates and the new version
  tag. A second immediate run must produce no changes at all.
- **Run (b) — full migration.** Build a throwaway `acmewidgets` git fixture
  under scratch at the previous template tag, carrying a populated
  `ai-docs/spec/` tree, an `ai-docs/mental-model.md`, an
  `ai-docs/mental-model/` tree with at least one nested domain, and no
  `ai-docs/.old/`. Confirm the staleness alarm fires before the run. Run the
  upgrade. Confirm every file is present under `ai-docs/.old/` with history
  preserved by `git mv`, that nothing was deleted, that the alarm clears, and
  that the resulting `AGENTS.md`'s template-managed sections and the whole
  `ai-docs/WORKFLOW.md` match what a fresh bootstrap in an empty repository
  produces — the convergence invariant, scoped per Decisions.
- **Run (c) — adopt.** Strip the version tag from a copy of the fixture and run
  adopt; it must reach the same end state as run (b) without re-walking the
  archive move destructively.
- Confirm the doc-coverage alarm no longer exists, or does not fire against the
  archived directories.
- Run (a)'s output — this repository's `AGENTS.md` at the new template
  version — is committed as part of this phase; without it this repository
  alarms stale in every session.
- No release here (Decisions): the tag bump reaches installed projects only
  through `lead-ship`, after the user approves, and a broken item would then
  be visible immediately and everywhere — which is why all three runs must be
  clean and recorded first.

Verification expectations: the three runs above, recorded with the observed
alarm transitions and the fresh-versus-upgraded comparison, plus the full Go
suite and both wsflow test modules against the release build.

Touchpoints: a scratch fixture repository (not committed), the built plugin
under test, this repository's `AGENTS.md` (run (a)'s version tag), and — only
if the runs surface a defect — the Phase 1 files.

## Open Questions

None. The trailer-parser check, the guide-wording ownership, and the `.old/`
README question are settled under `## Decisions` and carried into Phase 1.

## Sage Review Round 1 (2026-09-09)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Three open questions are lead decisions, not design-review questions (trailer parser, guide wording ownership, .old README) | critical | All three settled under a new Decisions section: bootstrap owns template and guide wording; trailer check against sibling Result; no README under .old; Open Questions emptied. |
| 2 | Phase 2 titled as a release step though release is lead-ship's (epic Decision 13) | major | Phase 2 retitled and its release bullet replaced; decision recorded. |

### Completeness Reviewer — concern

| # | Title | Severity |
|---|-------|----------|
| 1 | Superseded checklist items not marked obsoleted despite adopt auditing every item | major |
| 2 | Convergence invariant stated as whole-file equality, unreachable for any fixture with content | major |
| 3 | Touchpoints miss the wsflow generated mirror, ai-docs/WORKFLOW.md, the drafts and README, and this repo's AGENTS.md at the new version | minor |
| 4 | Index-health route table has three forge rows, not two | minor |

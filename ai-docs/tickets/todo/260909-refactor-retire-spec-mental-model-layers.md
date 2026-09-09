---
title: "Retire the spec and mental-model layers: tools, gates, write-time doc passes, and conventions"
sage-review-design: required
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260909-chore-ws-refoundation-git-history-measurement-manual: prerequisite; the epic orders the measurement manual first and requires a before-baseline recorded on this repository before any layer is removed
  260909-research-ws-refoundation-evidence-audit: evidence for verdicts A1, A2, A4 and the rejected alternatives restated below
  260909-refactor-route-resolve-implement-reads-ticket-facts: sibling; that ticket addresses no spec stem because this one retires the anchors describing the resolver
  260909-feat-bootstrap-refoundation-template-migration: downstream; the bootstrap template still teaches the anchor system and the doc directories after this ticket lands
  260723-feat-ready-spec-address-hard-gate: drop candidate; this ticket removes the gate that ticket would harden
  260716-feat-mental-model-comment-placement-rule: drop candidate; built on a layer this ticket retires
  260716-feat-mental-model-openup-injection: drop candidate; built on a layer this ticket retires
  260716-feat-sage-related-mental-model-curation: drop candidate; built on a layer this ticket retires
---

# Retire the spec and mental-model layers: tools, gates, write-time doc passes, and conventions

## Background

Epic `260909-epic-ws-worker-interpreter-refoundation` applies a truth criterion
(Cross-Child Decision 1): a layer survives only if reconstructing its value on
demand with a current model costs more than maintaining it. Decision 2 fixes the
memory tiers that remain — tickets, commit `## AI Context`, notes, a few
manuals, and non-derivable runtime traps as code comments at the site that
bites. Decision 3 makes tests the behavioral contract and the sage-stamped
ticket the plan. The spec and mental-model layers fail the criterion under all
three, so this ticket removes them as maintained layers here and from the
shipped workflow.

The evidence is in `260909-research-ws-refoundation-evidence-audit`:

- **A1 (false)** — mental-model holds three kinds of knowledge and each has a
  better home. Derivable structure: one Explore-class dispatch re-derives it.
  Runtime traps: they belong next to the code that bites, where a
  premise-changing edit must touch the same file. Rationale: already in
  `## AI Context` and tickets. The layer is a copy, and its "not derivable in
  30 seconds" inclusion test was written for a 200K-context era.
- **A2 (false)** — about 4.4% of commits are `docs(spec)` and 3.3% carry
  `(mental-model-updated)`, yet the spec corpus was archived wholesale and
  rebuilt once (`b20890e9`), a sample of 8 items showed 2 contradicting code,
  and mental-model prose is explicitly exempt from rename discipline
  (`260904-refactor-mental-model-doc-drift-epic-renames`). The maintenance mode
  that actually worked was regeneration (`lead-forge-*`), not increments.
- **A4 (true premise, wrong placement)** — spec is a real external contract,
  but it was placed as a `ready/` precondition where it blocks promotion, and
  nobody checks it against code afterwards. The "spec drift" review checklist
  item never produced a recorded finding, and only one consumption point
  (the design-review spec-conflict check) changes an agent's action.

Current footprint on this repository, measured at authoring time:
`ai-docs/spec/` is 8 files and 5,425 lines carrying 228 unique anchors;
`ai-docs/mental-model/` is 11 files and 1,060 lines plus the
`ai-docs/mental-model.md` overview; 302 of 5,193 commits carry an updated-specs
trailer section and 194 carry `(mental-model-updated)`. The shipped doc-layer
playbooks are 996 lines across seven `rsrc/` stems, and the two retiring
conventions are 313 lines. A repository-specific aggravation applies: this
repository's specs describe prose (playbooks), so they drift faster here than
in a downstream code project.

Humans do not read `ai-docs/`; every human-facing answer is an AI digest. A
drifted document is therefore a stale intermediary between code and the human,
with negative value — which is why the layers are retired rather than relocated.

## Decisions

- **Delete the layer rather than reshape it.** The evidence audit's bootstrap
  section found the cost is internal Go coupling — ticket status directories,
  the `{#YYMMDD-slug}` anchor regex as the cross-reference key, and the
  mental-model directory — and that two precedents (`260807` index dissolution,
  `260825` template convergence) each discovered compiled-code dependencies only
  during execution. Deleting is cheaper: every hidden dependency surfaces as a
  compile or test failure instead of as silent half-migrated behavior.
  - Rejected: a destructive `ai-docs/` layout restructure as the primary lever.
    The layout is not the cost center, and reshaping the anchor key has the
    highest Go fan-out while deleting the layer has the lowest.
  - Rejected: keeping spec and moving it out of the `ready/` precondition. With
    tests as the contract and no human readers, a relocated spec is still a
    hand-maintained derived document, which Decision 1 forbids any child from
    reintroducing.
- **Four replacements, no new maintained document.**
  1. Tests are the behavioral contract (Decision 3).
  2. A behavior change with no accompanying test change is a review finding,
     replacing the retired spec-drift checklist item.
  3. Non-derivable runtime traps become code comments at the site that bites
     (Decision 2), so a premise-changing edit must touch the same file.
  4. An optional on-demand digest generator whose output is an untracked cache
     stamped with the commit hash it was generated from. Design only in this
     ticket; see `## Open Questions`.
  - Rejected: Haiku-class workers reading pre-digested docs as their context.
    The epic sets workers at current-mainstream or previous-flagship class,
    deriving context from source pointers; digests are an optional cache, never
    an input contract.
- **The sage gate stays; only the spec-address half is removed.** Cross-Child
  Decision 8 keeps fact population and design review as the `ready/` gate. In
  `lead-write-ticket` the two gates are adjacent but distinct, and in Go
  `sageReviewStageRequirement` currently reuses the same category-exemption
  mechanism as the spec-address warning; the separation must leave the sage
  path behaving identically.
- **This repository's own corpus is archived, not destroyed.** `ai-docs/.old/`
  is the tracked project archive and already holds `spec/260421/` and
  `spec/260505/` snapshots from the earlier wholesale archive. The current
  `ai-docs/spec/` and `ai-docs/mental-model/` move under the same scheme so
  that the 228 anchors cited across `.done/` tickets stay resolvable by grep.
  - Rejected: outright deletion relying on git history. Anchor citations in
    closed tickets are absolute references; a `git log` archaeology step per
    citation is a worse cost than one hidden tracked directory.
- **The `(mental-model-updated)` marker retires as a convention, not as code.**
  No Go code emits or parses it; it is a playbook and convention contract only.
  The commit-message surface that does exist in Go is the `git.commit` option
  set and its rendered sections, which are removed with the layer.

- **Closed inventory is untouched.** Per epic Cross-Child Decision 14, nothing
  under `.done/` or `.dropped/` is edited or moved: the roughly 218 closed
  tickets carrying `related-mental-model:` keep it, and the spec anchors that
  closed tickets cite stay as citations. Any reader that resolves those
  references (`references.trace`, the ticket graph) must tolerate an
  unresolved target once the corpus lives under `ai-docs/.old/`.
  *Rejected: strip the key from closed tickets* — a mechanical rewrite of
  history with no consumer.

## Constraints

- **Shipped-surface rule (AGENTS.md Architecture Rule 4).** No text under
  `agents-plugin/`, `agents-plugin-wsflow/`, the embedded conventions, or any
  string the MCP tooling emits to an agent may name this repository's tickets,
  epics, commit hashes, layout, tooling, or migration vocabulary. Removal text
  is especially prone to this: a doctor check, advisory, or todo instruction
  explaining *why* the doc pass is gone must be written for a project that has
  never heard of this one, or must not exist at all. Verify with
  `python3 -m unittest discover agents-plugin/tests`, and remember that a test
  pinning a devenv-only string is itself the bug.
- **wsflow mirroring (`ai-docs/manuals/wsflow-mirroring.md`).** The
  `agents-plugin-wsflow/rsrc/` tree is generated byte-identical to
  `agents-plugin/rsrc/`, and the wsflow shipped-skill list currently includes
  `lead-backfill-docs`, `lead-write-spec`, `lead-update-spec`, `lead-forge-spec`,
  and `lead-forge-mental-model`. Removing them changes the shipped set, the
  manifests, the inclusion list in that manual, and the package tests. Regenerate
  with the manual's own env vars (`WS_REGEN_WSFLOW_RSRC`, `WS_REGEN_WSFLOW_SKILLS`,
  each with `-count=1`); do not hand-edit the mirrored tree. Distributed wsflow
  text stays non-ws-aware.
- **Skill authoring.** Every changed playbook, skill, or convention goes through
  the invariant checklist in `ai-docs/manuals/skill-authoring.md` before commit.
- **Ordering.** Block-depends on
  `260909-chore-ws-refoundation-git-history-measurement-manual`: the epic makes
  the measurement manual a prerequisite for every removal, and the before-pass
  must be recorded on this repository first, since the removal changes the very
  commit trailers the manual reads.
- **No `spec:` or `spec-remove:` frontmatter on this ticket.** Listing 228
  retiring anchors is not addressing; Phase 1 removes the gate that would ask
  for it. This is deliberate, not an omission.
- **The bootstrap template is out of scope.**
  `agents-plugin/skills/lead-bootstrap/AGENTS.template.md` and the shipped
  `WORKFLOW.md` still teach the anchor system and the doc directories; the
  template version bump and downstream migration item belong to
  `260909-feat-bootstrap-refoundation-template-migration`. This ticket must not
  leave the template describing tools that no longer exist without that ticket
  landing, so the cross-ticket seam is stated in each phase's verification.
- **Exclude the duplicate plugin trees under `.claude/worktrees/` from every
  sweep**; they double-hit each grep and are not the edit target.
- Independent review, the ticket system, the status directories, the review
  ledger, and the Go route resolver are explicitly out of scope (epic
  `## Non-Scope`).

## Prior Art

Found by search; rerun the greps rather than trusting these coordinates.

**Go — spec layer.** `agents-plugin-tool/internal/wsdoc/spec_discovery.go`
(`SpecsList`, `SpecsFind`, `SpecsStatus`), `spec_tools.go` (`GenerateSpecStem`,
`VerifySpecIndex`, `SpecAnchors`, the `{#YYMMDD-slug}` regex),
`internal/mcp/format.go` (`FormatSpecs`, `FormatSpecFind`). Grep:
`SpecsFind\|SpecsList\|SpecsStatus\|GenerateSpecStem\|VerifySpecIndex\|SpecAnchors`.

**Go — mental-model layer.** `internal/wsdoc/mental_model_discovery.go`
(`MentalModelsFind`, `MentalModelsStatus`), `mental_models.go`
(`MentalModelsList`). Grep: `MentalModels\|mental_models\.`.

**Go — shared and adjacent.** `internal/wsdoc/query_match.go` is the scoring
shared by both query tools; `references.go` (`ReferencesTrace`) resolves
spec/mental-model cross-references; `legacy_marker.go` (about 17 KB, no exported
symbols) is the legacy spec-marker advisory and `## Spec Impact` collector;
`doc_coverage.go` is only two predicates (`SpecAreaHasFrontmatterFile`,
`MentalModelAreaHasFrontmatterFile`); `project_tree.go` renders the spec and
mental-model areas; `doctor.go` is reached from `cmd/ws-mcp/main.go`.

**Go — MCP registration.** `internal/mcp/server.go` is a single dispatch switch
plus a schema table plus a tool-name allowlist, so each retiring tool has three
sites: `specs.query`, `spec_stem.generate`, `spec_index.verify`,
`mental_models.list`, `mental_models.query`, `mental_models.status`,
`references.trace`. Argument-shape guards (`hasTicketStemArgument`,
`hasSpecStemArgument`, the `mentions_ticket_stem` rejection) sit beside the
dispatch entries.

**Go — doc coverage alarm.** `internal/mcp/doc_coverage_alarm.go`
(`docCoverageWarning`, `injectDocCoverageWarning`), injected from the lead-login
path in `server.go` and from `internal/mcp/workflow_manual.go`; the config knob
is registered in `server.go` with the key constant in
`internal/wsconfig/scope.go`. `internal/mcp/review_track_alarm.go` and
`internal/wsreview/checkpoint.go` model themselves on the same warning shape and
must keep working after the doc alarm is gone.

**Go — spec-address gate.** `internal/wsdoc/tickets_mutate.go`
(`readyGateWarning`, `exemptReadyGateCategories`, `ticketCategoryRE`) and
`internal/wsdoc/tickets_verify.go` (`TicketVerify` adds a `spec-address`
warning for `ready`). Both are soft warnings today, not hard blocks; the hard
half lives in the playbook judgment. `sageReviewStageRequirement` in the same
mutate file reuses `ticketCategoryRE` — that is the coupling to unpick.
`internal/wsdoc/tickets.go` exposes `Specs`/`SpecRemoves` from frontmatter;
`internal/wsdoc/tickets_graph.go` builds a `specAnchors` set and resolves
`related:` against ticket stems union spec anchors;
`internal/wsdoc/tickets_template.go` carries `spec:` and
`related-mental-model:` in the skeleton.

**Go — commit trailers.** `internal/wsgit/git.go`: option fields
`MentalModelNotes`, `UpdatedSpecs`, `UpdatedMentalModels` (JSON
`mental_model_notes`, `updated_specs`, `updated_mental_models`) and the rendered
sections `### Mental Model Notes`, `## Updated Specs`, `## Updated Mental
Models`. Note the section names differ from the `## Spec` heading in this
repository's own `AGENTS.md` commit template, which is a separate,
repository-local convention line to update.

**Go — implement pipeline doc todos.** `internal/mcp/session_state.go` installs
`doc-pre-pass`, `doc-commit-gate`, and `doc-closeout` and builds their
instruction text in `implementDocPrePassInstruction`,
`implementDocCommitGateInstruction`, `implementDocCloseoutInstruction`; the
pre-pass text names `mental-model-updater` conditionally.

**Shipped playbooks (both packages, byte-identical `rsrc/` trees).**
`lead-update-spec` (84 lines), `lead-write-spec` (110), `lead-forge-spec` (307),
`lead-forge-mental-model` (222), `lead-backfill-docs` (126),
`mental-model-updater` (67), `doc-gap-discovery` (80). Skill directories exist
for `lead-backfill-docs`, `lead-forge-spec`, `lead-forge-mental-model` in the
full package and additionally for `lead-write-spec` and `lead-update-spec` in
wsflow. `lead-implement` dispatches the doc todos to `lead-update-spec`;
`lead-backfill-docs` renders `doc-gap-discovery`; `ticket-reviewer-design`
consumes `related-mental-model:`; `lead-write-ticket` carries the
`spec-address-gate` and `missing-spec-address` judgments and couples them to the
sage gate in the cascade-edit and bulk-promotion sections. Manifests
(`rsrc/manifest.json`, `skills/manifest.json`) carry per-stem hashes in both
packages.

**Conventions.** `agents-plugin-tool/internal/wsdoc/conventions/spec-conventions.md`
(99 lines) and `mental-model-conventions.md` (214 lines), embedded through
`//go:embed conventions/*.md` in `conventions.go` with canonical names and
aliases, served as `convention.read`. Nine playbook call sites read them.

**Tests.** Concentrated in `internal/wsdoc/{spec_discovery,mental_model_discovery,references,doc_coverage,legacy_marker,project_tree,tickets_verify,tickets_mutate}_test.go`,
`internal/mcp/{server,doc_coverage_alarm,legacy_marker_render,session_state,tickets_verify,tickets_template,playbook_tools}_test.go`,
`internal/wsgit/git_test.go`, `internal/wsrsrc/{forge_spec_ambiguity,skills_mirror,manifest_shipped,skills_manifest,workflow_guide}_test.go`,
and the Python bundles under `agents-plugin/tests/` and
`agents-plugin-wsflow/tests/`. The `legacy_marker` suites are the largest single
spec-coupled block.

**Archive precedent.** `b20890e9` ("docs(spec): archive stale spec set") moved
the whole corpus with `git mv` and zero content change; `c461550a` established
`ai-docs/.old/` as the hidden tracked archive. `260807` (index dissolution) and
`260825` (template convergence) are the precedents for compiled-code
dependencies surfacing only during execution.

## Open Questions

Not settled by the epic; resolve at design review or defer.

- **Digest generator shape.** The epic names "on-demand digests as untracked
  caches stamped with a commit hash" but settles neither the trigger (an MCP
  tool, a worker-invoked playbook, or nothing shipped at all), the cache
  location (a note layer, an untracked `ai-docs/` sibling, or the existing
  API-documentation-cache mechanism), nor the staleness rule beyond the stamp.
  Options: (a) ship nothing and let the worker re-derive per run, which is the
  cheapest and matches the truth criterion most literally; (b) ship a generator
  as a deferred phase of this ticket; (c) ship it as a separate child. This
  ticket defaults to (a) and treats (b)/(c) as a follow-up, but the choice is
  not the author's to make.
- **`references.trace` scope.** The tool resolves spec and mental-model
  cross-references, which retire — but `tickets_graph.go` also resolves
  `related:` against spec anchors, and closed tickets cite anchors. Remove the
  tool wholesale, or keep a ticket-to-ticket trace and remove only the doc half?
  The parent brief lists it under removal; the ticket-graph coupling was found
  during survey and may argue for the narrower cut.
- **`legacy_marker.go` disposition.** It is the largest spec-coupled Go file
  with no exported symbols and its own advisory surface. Whether it dies with
  the layer or has a residual non-spec purpose was not determined; read it
  before Phase 2.
- **This repository's `AGENTS.md`.** Its `## Documentation System`,
  `## Code Standards` (item 5), and commit-template `## Spec` trailer describe
  the retiring layers. Updating it is required for coherence but is a
  repository-local edit that overlaps the epic's planned binding-anchor
  replacement; whether it lands here or with that planned item is open.

## Phases

Sequentially dependent: Phase 1 removes the consumers so Phase 2's deletions do
not break a live caller, and Phase 2 removes the code so Phase 3's archive does
not orphan a reader.

### Phase 1: Remove the gates, the alarm, and the write-time doc passes

Goal: nothing in the workflow demands, checks, or schedules a spec or
mental-model update any more, while the documents themselves and the read tools
still exist. This phase is behavior-visible on its own and is the one the
measurement manual's after-pass will attribute most of the change to.

Scope:

- Remove the spec-address warning from `tickets.move` and `tickets.verify`
  (`readyGateWarning` and its `spec-address` warning), and remove the
  `spec-address-gate` / `missing-spec-address` judgments, the
  `On: Spec-address Check` procedure, and the outcome-routing rows from
  `lead-write-ticket`. Unpick `sageReviewStageRequirement` from the shared
  category-exemption mechanism so the sage gate keeps its current behavior
  exactly, including the category exemptions.
- Remove `doc-pre-pass`, `doc-commit-gate`, and `doc-closeout` from the
  implement todo installation and delete their instruction builders; remove the
  matching dispatch lines from `lead-implement` and the doc-closeout history
  note from `lead-workflow-manual`.
- Remove the doc coverage alarm: the warning builder, both injection sites, the
  config knob and its key constant, its default, and its `config.tune`
  description. Leave the review-track alarm and the review checkpoint working.
- Add the replacement review finding — a behavior change with no test change —
  to the review checklist that carries the retiring spec-drift item, written in
  project-neutral terms.
- Update this repository's `AGENTS.md` documentation-system and commit-template
  lines only if the Open Question resolves to landing them here.

Verification: `go test ./...` and `go vet ./...` under `agents-plugin-tool/`;
`python3 -m unittest discover agents-plugin/tests` and
`agents-plugin-wsflow/tests`. Expect churn in `internal/mcp/session_state_test.go`,
`internal/mcp/doc_coverage_alarm_test.go`, `internal/wsdoc/tickets_verify_test.go`,
`internal/wsdoc/tickets_mutate_test.go`, and `internal/mcp/tickets_verify_test.go`;
`doc_coverage_alarm_test.go` and `internal/wsdoc/doc_coverage_test.go` are
deleted with their subject. Add a test asserting that a `ready/` move of a
non-exempt ticket with no spec addressing produces no warning, and that the sage
gate still refuses an unstamped ticket. Re-run the shipped-surface guard after
every playbook edit.

Touchpoints: `internal/wsdoc/tickets_mutate.go`, `internal/wsdoc/tickets_verify.go`,
`internal/mcp/tickets_verify.go` if a mirror exists, `internal/mcp/session_state.go`,
`internal/mcp/doc_coverage_alarm.go`, `internal/mcp/workflow_manual.go`,
`internal/mcp/server.go`, `internal/wsconfig/scope.go`,
`internal/wsreview/checkpoint.go` (verify only),
`agents-plugin/rsrc/{lead-write-ticket,lead-implement,lead-workflow-manual,code-reviewer}/`
and the wsflow mirrors, both `manifest.json` pairs.

### Phase 2: Remove the tools, the playbooks, and the conventions

Goal: the spec and mental-model layers have no code, no tool surface, no
playbook, and no convention document. Depends on Phase 1: while the gates and
doc passes still exist, these tools have live callers.

Scope:

- Delete the MCP tools `specs.query`, `spec_stem.generate`, `spec_index.verify`,
  `mental_models.list`, `mental_models.query`, `mental_models.status`, and
  `references.trace` — each at its dispatch entry, its schema declaration, its
  allowlist entry, and its argument-shape guard — together with their formatters
  and the `wsdoc` implementations behind them.
- Delete the playbooks `lead-update-spec`, `lead-write-spec`, `lead-forge-spec`,
  `lead-forge-mental-model`, `lead-backfill-docs`, `mental-model-updater`, and
  `doc-gap-discovery`, their skill directories in both packages, their manifest
  entries, and their render-eligibility entries. Remove `mental-model-updater`
  from the render-eligible stem list. Update the shipped-skill inclusion list
  and the render-dispatch section of `ai-docs/manuals/wsflow-mirroring.md`
  in the same change.
- Remove the `git.commit` options `mental_model_notes`, `updated_specs`, and
  `updated_mental_models` and their rendered sections.
- Remove `spec-conventions.md` and `mental-model-conventions.md` from the
  embedded conventions, along with their canonical names and aliases; leave
  `ticket-conventions.md` and `convention.read` itself intact.
- Remove `spec:`, `spec-remove:`, and `related-mental-model:` from the ticket
  skeleton, the `Specs`/`SpecRemoves` ticket fields if nothing else reads them,
  the spec-anchor half of the ticket graph's `related:` resolution, and the
  spec and mental-model areas from `project_tree` and `doctor`. Resolve
  `legacy_marker.go` and `references.trace` per `## Open Questions` before
  cutting.
- Remove `related-mental-model:` consumption from `ticket-reviewer-design`.

Verification: the compile is the survey — `go build ./...` first, and treat
every resulting error as a discovered dependency to record. Then `go test ./...`,
`go vet ./...`, both Python bundles, and the wsflow mirror regeneration with the
manual's env vars and `-count=1`. Delete rather than adapt the suites whose
subject is gone; keep and adjust `internal/wsdoc/tickets_graph_test.go`,
`project_tree_test.go`, and the `wsrsrc` manifest and mirror suites. Confirm
`tools/list` no longer advertises the seven tools and that `convention.read`
still serves `ticket-conventions`.

Touchpoints: `internal/wsdoc/{spec_discovery,spec_tools,mental_model_discovery,mental_models,references,query_match,legacy_marker,doc_coverage,conventions,project_tree,doctor,tickets,tickets_graph,tickets_template}.go`,
`internal/wsdoc/conventions/`, `internal/mcp/{server,format}.go`,
`internal/wsgit/git.go`, `cmd/ws-mcp/main.go`, the seven `rsrc/` stems and their
skill directories in both packages, both manifest pairs,
`ai-docs/manuals/wsflow-mirroring.md`.

### Phase 3: Archive this repository's spec and mental-model corpus

Goal: `ai-docs/spec/` and `ai-docs/mental-model/` no longer exist as maintained
directories here, and their content survives as a dated tracked archive. Depends
on Phase 2: while `project_tree` and `doctor` still render the areas, moving the
directories produces spurious findings.

Scope:

- Triage before archiving (epic Cross-Child Decision 18): a cheap-tier pass
  classifies every mental-model entry and every spec section as one of:
  derivable from code (archive only); prescriptive convention (move the
  rule to `ai-docs/manuals/` or, if short and universal, inline into
  `AGENTS.md`, and declare it in the path-scoped conventions section);
  site-specific trap (becomes a code comment at the site that bites);
  non-derivable external fact (moves to `ai-docs/ref/`). The triage result
  is recorded in this phase's `### Result` as counts per class plus the
  moved items, so the after-pass of the measurement manual can see what
  survived. Nothing is rewritten in place; the archive keeps the original
  text.
- `git mv` `ai-docs/spec/` and `ai-docs/mental-model/` (and the
  `ai-docs/mental-model.md` overview) under `ai-docs/.old/` following the
  existing dated-snapshot scheme, content unchanged, so the 228 anchors cited
  by closed tickets stay greppable.
- Update `ai-docs/WORKFLOW.md` and this repository's `AGENTS.md`
  `## Documentation System` list to describe the reduced layout.
- Record the after-pass input for the measurement manual: the removal changes
  the commit trailers the manual reads, so state the cut-over commit.

Verification: `git status` shows moves only, no content diffs
(`git diff -M --stat` reports pure renames); `go test ./...` and both Python
bundles still pass; `project_tree` output no longer names the removed areas; a
grep for one previously cited anchor still resolves inside `ai-docs/.old/`.
Confirm no shipped file under either plugin package references the moved paths.

Touchpoints: `ai-docs/spec/`, `ai-docs/mental-model/`, `ai-docs/mental-model.md`,
`ai-docs/.old/`, `ai-docs/WORKFLOW.md`, `AGENTS.md`, `CLAUDE.md` only if its
shim body changes.

---
title: "Retire the spec and mental-model layers: tools, gates, write-time doc passes, and conventions"
sage-review-design: completed
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260909-chore-ws-refoundation-git-history-measurement-manual: prerequisite; the epic orders the measurement manual first and requires a before-baseline recorded on this repository before any layer is removed
  260909-research-ws-refoundation-evidence-audit: evidence for verdicts A1, A2, A4 and the rejected alternatives restated below
  260909-refactor-route-resolve-implement-reads-ticket-facts: sibling; that ticket addresses no spec stem because this one retires the anchors describing the resolver
  260909-feat-bootstrap-refoundation-template-migration: downstream; owns the bootstrap template and shipped guide, which still teach the anchor system and the doc directories after this ticket lands
  260909-refactor-lead-surface-collapse-worker-stop-protocol: sibling; both edit lead-write-ticket, lead-implement, and lead-workflow-manual, and both touch ticket-conventions; whichever lands second reconciles the shared lines
  260723-feat-ready-spec-address-hard-gate: drop candidate; this ticket removes the gate that ticket would harden
  260716-feat-mental-model-comment-placement-rule: drop candidate; built on a layer this ticket retires
  260716-feat-mental-model-openup-injection: drop candidate; built on a layer this ticket retires
  260716-feat-sage-related-mental-model-curation: drop candidate; built on a layer this ticket retires
sage-review-completeness: completed
sage-review-design-reviewed: 8d66854141604cb7
sage-review-completeness-reviewed: 8d66854141604cb7
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

- **The ticket conventions and the ticket skeleton lose their spec lines; the
  rest stays.** `ticket-conventions.md` is 73 lines of deterministic
  invariants (path rule, status directories, frozen Result, Edition
  append) that pass the authoring standard as written, so it is not
  rewritten. But five of its Status Flow bullets state the ready spec-address
  gate this ticket removes (spec addressing on `ready/` entry, the
  `lead-write-spec` non-invocation, `## Spec Impact`, the linked-spec drop
  route), and the skeleton carries the optional `## Spec Impact` section and
  the legacy `plans:` / `skeletons:` keys that no shipped path reads. Leaving
  them would make the convention contradict the tool that serves it. Phase 2
  strips exactly those lines and keys; every other convention line and
  template section is untouched.
  *Rejected: re-baseline the ticket conventions under the authoring
  standard* — its lines already carry a citable failure each; a rewrite
  would change wording without removing a rule.

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
`doc_coverage.go` no longer exists — Phase 1 deleted it whole, predicates and
walker, with the doc coverage alarm (`### Result (e2810a33)`); `project_tree.go`
renders only a dedicated spec area (`renderSpecs`, project_tree.go#L28-L37,128-136)
— mental-model has no dedicated area there, only the generic `ai-docs/`
directory listing (project_tree.go has no "mental" occurrence); `doctor.go`'s
`Doctor` no longer checks spec/mental-model presence (that check went with
`doc_coverage.go` in Phase 1) and now only verifies repo root, `ai-docs/`, and
`AGENTS.md` (doctor.go#L14-L47), reached from `cmd/ws-mcp/main.go`.

**Go — MCP registration.** `internal/mcp/server.go` is a single dispatch switch
plus a schema table plus a tool-name allowlist, so each retiring tool has three
sites: `specs.query`, `spec_stem.generate`, `spec_index.verify`,
`mental_models.list`, `mental_models.query`, `mental_models.status`,
`references.trace`. Argument-shape guards (`hasTicketStemArgument`,
`hasSpecStemArgument`, the `mentions_ticket_stem` rejection) sit beside the
dispatch entries.

**Go — doc coverage alarm (resolved in Phase 1).** `internal/mcp/doc_coverage_alarm.go`,
`docCoverageWarning`, `injectDocCoverageWarning`, and the `internal/wsconfig/scope.go`
key constant no longer exist — Phase 1 deleted this paragraph's whole subject
(`### Result (e2810a33)`). `internal/mcp/review_track_alarm.go` and
`internal/wsreview/checkpoint.go` model themselves on the same warning shape and
are confirmed (by that Result) to still work after the doc alarm's removal.

**Go — spec-address gate (resolved in Phase 1).** `readyGateWarning` and
`exemptReadyGateCategories` in `internal/wsdoc/tickets_mutate.go`, and the
`spec-address` warning `internal/wsdoc/tickets_verify.go`'s `TicketVerify` added
for `ready`, no longer exist — Phase 1 removed them (`### Result (e2810a33)`).
`ticketCategoryRE` survives (still read by `sageReviewStageRequirement`, whose
doc comment that Result says now states its own rule in full rather than
deriving it from the removed gate); the exemption map survives too, renamed
`nonImplementationCategories` around its surviving reader (`missingRouteFacts`)
rather than deleted. `internal/wsdoc/tickets.go` exposes `Specs`/`SpecRemoves`
from frontmatter (still current — confirmed read by `internal/mcp/server.go`,
`internal/wsdoc/legacy_marker.go`, and `internal/wsdoc/references.go` besides
`tickets.go` itself);
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

**Go — implement pipeline doc todos (resolved in Phase 1).** `internal/mcp/session_state.go`
no longer installs `doc-pre-pass`, `doc-commit-gate`, or `doc-closeout`, and
`implementDocPrePassInstruction`, `implementDocCommitGateInstruction`, and
`implementDocCloseoutInstruction` no longer exist — Phase 1 removed them
(`### Result (e2810a33)`). `mental-model-updater` is no longer named from that
pipeline; it is still named by `internal/mcp/server.go` (render-eligible stem
list), `lead-backfill-docs`, `lead-forge-mental-model`, and
`skills/lead-bootstrap/AGENTS.template.md`, which Phase 2's own scope (the
render-eligible-stem-list bullet) and the out-of-scope bootstrap template
already account for separately.

**Shipped playbooks (both packages, byte-identical `rsrc/` trees).**
`lead-update-spec` (84 lines), `lead-write-spec` (110), `lead-forge-spec` (307),
`lead-forge-mental-model` (222), `lead-backfill-docs` (126),
`mental-model-updater` (67), `doc-gap-discovery` (80). Skill directories exist
for `lead-backfill-docs`, `lead-forge-spec`, `lead-forge-mental-model` in the
full package and additionally for `lead-write-spec` and `lead-update-spec` in
wsflow. `lead-implement` no longer exists — the sibling
`260909-refactor-lead-surface-collapse-worker-stop-protocol` (done) retired it
in favor of `ticket-worker`, which has no `lead-update-spec` reference; the
only remaining caller of `lead-update-spec` by name is `lead-backfill-docs`.
`lead-backfill-docs` renders `doc-gap-discovery`; `ticket-reviewer-design`
consumes `related-mental-model:`; `lead-write-ticket` is now `lead-ticket`
(same sibling) and, per this ticket's own `### Result (e2810a33)`, no longer
carries a `spec-address-gate` or `missing-spec-address` judgment or an
`On: Spec-address Check` procedure — that half of Phase 1's scope already
landed. Manifests
(`rsrc/manifest.json`, `skills/manifest.json`) carry per-stem hashes in both
packages.

**Conventions.** `agents-plugin-tool/internal/wsdoc/conventions/spec-conventions.md`
(99 lines) and `mental-model-conventions.md` (214 lines), embedded through
`//go:embed conventions/*.md` in `conventions.go` with canonical names and
aliases, served as `convention.read`. Nine playbook call sites read them.

**Tests.** Concentrated in `internal/wsdoc/{spec_discovery,mental_model_discovery,references,legacy_marker,project_tree,tickets_verify,tickets_mutate}_test.go`
(`doc_coverage_test.go` was already deleted with its subject in Phase 1),
`internal/mcp/{server,legacy_marker_render,session_state,tickets_verify,tickets_template,playbook_tools}_test.go`
(`doc_coverage_alarm_test.go` likewise already deleted in Phase 1),
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
- **`legacy_marker.go` disposition.** It is the largest spec-coupled Go file
  with no exported symbols and its own advisory surface. Whether it dies with
  the layer or has a residual non-spec purpose was not determined; read it
  before Phase 2.
- (Settled) This repository's `AGENTS.md` edits land in Phase 3 in one
  change; the binding-anchor section stays with the epic's planned item.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdoc/{spec_discovery,spec_tools,mental_model_discovery,mental_models,references,query_match,legacy_marker,conventions,project_tree,tickets,tickets_graph,tickets_template}.go, internal/mcp/{server,format}.go, internal/wsgit/git.go, cmd/ws-mcp/main.go, seven rsrc stems and skill dirs in both agents-plugin and agents-plugin-wsflow, both manifest.json pairs, ai-docs/manuals/wsflow-mirroring.md, ai-docs/spec/, ai-docs/mental-model/, ai-docs/WORKFLOW.md, AGENTS.md |
| scope.surface | public-interface | deletes seven shipped MCP tools confirmed live in internal/mcp/server.go (dispatch at L1043-1140, schema at L3844-3980, allowlist at L4368-4369): specs.query, spec_stem.generate, spec_index.verify, mental_models.list, mental_models.query, mental_models.status, references.trace |
| scope.new_public_symbol | no | none — Phase 2 and Phase 3 only delete tools, symbols, playbooks, conventions, and directories |
| scope.new_type_contract | no | none — no new type or function signature is introduced |
| scope.test_surface | existing | go test ./..., go vet ./..., python3 -m unittest discover (agents-plugin/tests and agents-plugin-wsflow/tests), and the wsflow mirror-regeneration tests all already exist and passed at Phase 1's e2810a33; Phase 2 deletes the matching *_test.go files (doc_coverage_test.go and doc_coverage_alarm_test.go already gone in Phase 1) rather than adding new ones |
| complexity.reuse_points | confirmed | Phase 3 reuses the existing ai-docs/.old/ dated-snapshot archive scheme (ai-docs/.old/spec/260421 and ai-docs/.old/spec/260505 already exist there); Phase 2's sage-gate note reuses Phase 1's already-landed `nonImplementationCategories` unpicking |
| complexity.side_effect_risk | moderate | the sweep spans two shipped plugin packages (agents-plugin, agents-plugin-wsflow) plus a shipped MCP tool contract, and several Prior Art claims were found stale by already-landed sibling work in this pass (lead-write-ticket to lead-ticket, lead-implement to ticket-worker, doc_coverage.go/doctor.go already deleted) — a worker following the uncorrected Prior Art literally would chase dead targets |
| risk.correctness | moderate | large multi-file, multi-package deletion sweep with several call sites only estimated ("Known at authoring") rather than pinned; the compile-as-survey verification step is the safety net the phase itself relies on |
| risk.fit | low | verified against the parent epic: Cross-Child Decisions 1, 2, 3, 8, 14, and 18 all exist in ai-docs/tickets/todo/260909-epic-ws-worker-interpreter-refoundation.md and match this ticket's paraphrase of each |
| risk.test | low | both phases name concrete verification commands (go build/vet/test, python3 -m unittest discover x2, wsflow mirror regen with -count=1) already exercised successfully once through Phase 1's landing |
| risk.security_or_contract | moderate | removes a shipped MCP tool contract (seven tools) and shipped git.commit trailer options (mental_model_notes, updated_specs, updated_mental_models) that agents-plugin/agents-plugin-wsflow consumers may already call; mitigated by the AGENTS.md Architecture Rule 4 shipped-surface test but is a real external contract removal |

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
- In `agents-plugin/rsrc/code-review-correctness/code-review-correctness.md`,
  remove the spec-drift checklist item and add in its place the replacement
  finding — a behavior change with no test change — written in
  project-neutral terms.

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
`agents-plugin/rsrc/{lead-write-ticket,lead-implement,lead-workflow-manual,code-review-correctness}/`
and the wsflow mirrors, both `manifest.json` pairs.

### Result (e2810a33) - 2026-09-10

Landed as `91621687..e2810a33` on `impl/epic/refound/swipe-panda-food`,
branched from `epic/refound` at `fb2d7602`.

**What landed.** `91621687` retired the ready spec-address gate: deleted
`readyGateWarning`, the `spec-address` warning `TicketVerify` added for
`ready`, and the `ready -> todo/idea` demote tip telling a caller to clear
`spec:`/`spec-remove:`/`## Spec Impact`. `07facc32` removed `doc-pre-pass`,
`doc-commit-gate`, and `doc-closeout` from `deriveImplementTodosFromVerdict`
and deleted their three instruction builders. `1ba8c89e` removed the
doc-coverage alarm whole: `internal/mcp/doc_coverage_alarm.go`, its two
`workflow_manual.go` injection sites, the `ferrule` result entry in
`server.go`, `internal/wsdoc/doc_coverage.go`'s two predicates and their
`dirHasFrontmatterFile` walker, the `doc_coverage_alarm` config-registry
entry, `wsconfig.ItemDocCoverageAlarm` with its `RegisterGlobalOnly` call and
its `builtinConfigDefaults` entry, the tuning-catalog knob, and both
hand-enumerated `config.tune` schema strings. `9997d584` replaced the
correctness reviewer's spec-drift checklist item and dropped `doc-closeout`
from the workflow manual's branch note. `fd50a281` and `e2810a33` are the
review fix commits (below).

**Unpicking the sage gate.** The coupling the phase named turned out to be
narrower than the ticket's Prior Art suggested: `sageReviewStageRequirement`
never read `exemptReadyGateCategories`, only the shared `ticketCategoryRE`
stem parser, so its behavior needed no change at all. What did couple them was
justification — the function's doc comment derived its per-stage rule from the
spec-address gate's exemptions. It now states the rule in full and owns it.
The map itself has a surviving reader (`missingRouteFacts`), so it is renamed
`nonImplementationCategories` and re-documented around what it actually means,
rather than deleted or duplicated. `tickets_sage.go`'s route-facts exemption
comment is repointed the same way.

**Replacement finding.** `code-review-correctness` item 6 is now: *"Unrecorded
behavior change: observable behavior changes but no test changes with it —
tests are the behavioral contract, so the diff leaves no executable record of
the contract it just changed. Assertion quality and coverage depth stay with
the Test partition."* The second sentence is load-bearing: without it the item
collides with the `## Out of scope` line directly below that routes test
coverage to the Test partition.

**Cross-ticket reconciliation (structural deviation, adapted).** The sibling
`260909-refactor-lead-surface-collapse-worker-stop-protocol` landed on
`epic/refound` first (`ce5f30ef`), so two of this phase's named targets no
longer exist: `lead-write-ticket` is now `lead-ticket` and carries no
`spec-address-gate` / `missing-spec-address` judgment and no `On: Spec-address
Check` procedure, and `lead-implement` is retired entirely in favor of
`ticket-worker`, which already states "Tests are the behavioral contract;
there is no separate behavior document to keep in sync." Both phase items were
verified satisfied against the tree, not assumed. This is the reconciliation
the ticket's `related:` entry anticipated for whichever sibling landed second.

**Deliberately not done in this phase.** `lead-workflow-manual`'s
`### Spec addressing` concept section and the two Ticket System Concepts lines
that name the ready spec-address gate are left standing: they are the prose
twin of the `ticket-conventions.md` Status Flow bullets that Phase 2 owns by
name, and splitting them across phases would leave the convention and its
explanation disagreeing. **Phase 2 must remove them with the convention
bullets.** The doc-mode plumbing (`doc_mode`, `doc_reason`, `need_doc` in the
resolver verdict and agenda JSON, and their rendered lines) is retained: the
todos behind it are gone, but changing a published output shape is an
always-ask decision under `AGENTS.md` `### Approval Protocol` and no phase of
this ticket scopes it. A worker therefore still reads `Doc Mode: standard` in
a verdict whose `Next:` line and todo skeleton name no documentation step — a
cosmetic inconsistency, recorded here so a later API-shape decision can clear
it.

**Verification** (all output read in full, all green, re-run at `e2810a33`):
`go build ./...`; `go vet ./...`; `go test ./...` (14 packages ok);
`python3 -m unittest discover agents-plugin/tests` (55 tests, OK);
`python3 -m unittest discover agents-plugin-wsflow/tests` (10 tests, OK);
`WSRSRC_REGEN=1 ... TestGenerateRealManifest` and
`WS_REGEN_WSFLOW_RSRC=1 ... TestRegenerateWsflowRsrcMirror`, both with
`-count=1`, after the playbook edits; `diff -r agents-plugin/rsrc
agents-plugin-wsflow/rsrc` empty. `gofmt -l internal/` reports only the three
files already dirty at `fb2d7602`.

**Tests.** Both assertions the phase required exist:
`TestTicketsMoveToReadyNoSpecAddressingWarning` (a non-exempt `ready/` move
with no spec addressing produces no spec warning and still succeeds) and
`TestTicketVerifyReadySageGateStillRefusesUnstampedTicket` (the sage gate is
unchanged). `TestTicketVerifyReadyEmitsNoSpecAddressWarning` and
`TestTicketsVerifyNoSpecAddressingIsClean` cover the same removal from
`tickets.verify` and end-to-end through `git.commit`. The six spec-addressing
move tests collapse into one negative test rather than being deleted, so a
reintroduced gate fails; `TestDeriveImplementTodos` now pins the full todo key
list for the same reason. Deleted with their subject:
`internal/mcp/doc_coverage_alarm_test.go` and
`internal/wsdoc/doc_coverage_test.go`.

**Review.** One full-scope reviewer, the allocation the route set. Round 1:
3 Important, 6 Minor, no Critical — all three Important were the same defect
in three places, an agent-facing string that outlived its subject
(`route.resolve_implement`'s `Next:` line promising "standard documentation
gates"; the final action gate telling a worker to verify a "standard
documentation closeout"; `tickets.verify`'s tool description advertising
"spec-address is reported as a warning only"). All [fixed] in `fd50a281`,
along with a gofmt regression and four stale comments naming the deleted
`readyGateWarning`. The final gate's documentation clause was removed from
*both* doc modes, not only the standard one, which left `DocMode`/`DocReason`
without a reader on `implementTodoVerdict` and took them off that struct.
Round 2 (verification-only): clean with 1 Minor — the round-1 comment fix had
substituted a new wrong claim (`sage-review-freshness` is one of two surviving
warning producers, not the only one); [fixed] in `e2810a33` by stating the
invariant without naming a producer. [won't fix: predates this range, is not
produced by this change, and a `.gitignore` edit is outside the phase's
touchpoints] — the untracked `agents-plugin-tool/ws-mcp` build artifact,
reported to the lead instead.

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
  embedded conventions, along with their canonical names and aliases; keep
  `convention.read` itself. In `ticket-conventions.md`, remove only the
  Status Flow bullets that state the spec-address gate (spec addressing on
  `ready/` entry, the epic and workset exemptions from it, the
  `lead-write-spec` non-invocation, and the linked-spec drop route) and the
  `idea/` / `todo/` `spec:` allowances; every other line stays (`## Decisions`).
- Remove `spec:`, `spec-remove:`, and `related-mental-model:` from the ticket
  skeleton, together with the legacy `plans:` / `skeletons:` keys, their
  explanatory paragraph, and the optional `## Spec Impact` section;
  the `Specs`/`SpecRemoves` ticket fields if nothing else reads them (three
  other readers exist today: `internal/mcp/server.go`,
  `internal/wsdoc/legacy_marker.go`, and `internal/wsdoc/references.go` —
  check each after this phase's own removals land),
  the spec-anchor half of the ticket graph's `related:` resolution, and the
  spec area from `project_tree` (its only dedicated renderer, `renderSpecs`;
  mental-model has no dedicated area there to remove — it already renders as
  a plain `ai-docs/` subdirectory) — `doctor` has nothing left to remove here,
  since its spec/mental-model presence check was already deleted with
  `doc_coverage.go` in Phase 1.
  `references.trace` is deleted wholesale: the ticket-graph half resolves
  `related:` against spec anchors that no longer exist, and the collateral
  (closed tickets' spec-anchor entries stop resolving) is accepted by epic
  Cross-Child Decision 14. Read `legacy_marker.go` before cutting it and
  record any non-spec residue in the Result.
- Remove `related-mental-model:` consumption from `ticket-reviewer-design`.
- Sweep the survivors: grep the seven tool names and the two directory paths
  (`ai-docs/spec`, `ai-docs/mental-model`) across `agents-plugin/rsrc`,
  `agents-plugin/skills`, `agents-plugin-wsflow/`, and `agents-plugin-tool/`
  (excluding `.claude/worktrees/`), and record a per-file disposition in the
  Result. Known at authoring: `lead-workflow-manual` (seven call sites),
  `reference-discovery` (three), `lead-discuss` (zero today — no "spec" or
  "mental" text found in `agents-plugin/rsrc/lead-discuss/lead-discuss.md`;
  the "two" no longer holds), `lead-add-rule`
  (two, plus a routing table into `ai-docs/mental-model/<domain>.md` — its
  rule-persisting destination becomes `ai-docs/manuals/` declared through
  the path-scoped conventions section, and that reroute is this ticket's),
  `lead-ticket` (renamed from `lead-write-ticket` by
  `260909-refactor-lead-surface-collapse-worker-stop-protocol`, done; no
  spec/mental-model text found there beyond the generic word "spec" in
  `task-list.md`), `impl-playbook.md`, `executor-wrapup.md`,
  `code-reviewer.md`, `plan-populator-survey` and `plan-populator-research`
  (deleted by the route-facts sibling; if still present, edit), and the
  `note_tools.go` advisory string that names spec and mental-model
  destinations. A playbook that survives with a dead call is a defect this
  phase owns.

Verification: the compile is the survey — `go build ./...` first, and treat
every resulting error as a discovered dependency to record. Then `go test ./...`,
`go vet ./...`, both Python bundles, and the wsflow mirror regeneration with the
manual's env vars and `-count=1`. Delete rather than adapt the suites whose
subject is gone; keep and adjust `internal/wsdoc/tickets_graph_test.go`,
`project_tree_test.go`, and the `wsrsrc` manifest and mirror suites. Confirm
`tools/list` no longer advertises the seven tools and that `convention.read`
still serves `ticket-conventions`.

Touchpoints: `internal/wsdoc/{spec_discovery,spec_tools,mental_model_discovery,mental_models,references,query_match,legacy_marker,conventions,project_tree,tickets,tickets_graph,tickets_template}.go`
(`doc_coverage.go` was already deleted in Phase 1; `doctor.go` needs no change
— its spec/mental-model check went with it),
`internal/wsdoc/conventions/`, `internal/mcp/{server,format}.go`,
`internal/wsgit/git.go`, `cmd/ws-mcp/main.go`, the seven `rsrc/` stems and their
skill directories in both packages, both manifest pairs,
`ai-docs/manuals/wsflow-mirroring.md`.

### Result (0fd8fb33) - 2026-09-10

The spec and mental-model layers have no code, no tool surface, no playbook, and
no convention document. Landed in three commits on
`impl/epic/refound/swipe-panda-food`: `17bb97db` (tool surface), `835c9d85`
(playbook surface), `0fd8fb33` (review fixes).

**Tools.** All seven removed at each of the four sites the phase names, plus
formatters and the `wsdoc` implementations: `spec_discovery.go`, `spec_tools.go`,
`mental_model_discovery.go`, `mental_models.go`, `references.go`,
`query_match.go`, `legacy_marker.go` and their tests, the two convention
documents with their canonical names and aliases, and the matching CLI
subcommands and `runtime.json` capability entries in both packages.
`legacy_marker.go` was read before cutting: it dies whole, with no non-spec
residue — its generic markdown helpers (`fenceTracker`, `htmlCommentTracker`,
`splitMarkdownIndent`, `yamlFrontmatterEnd`, `markdownFence`, `collectSpecImpact`)
had no caller outside the file, and its only entry points were `spec_discovery.go`
and `project_tree.go`'s spec area. `query_match.go` died with its two callers.

**Open question resolved.** `Specs`/`SpecRemoves` are removed from `TicketInfo`:
all three named readers are gone (`server.go`'s flag, `legacy_marker.go`,
`references.go`). `Plans`/`Skeletons` stay — a separate legacy artifact this
ticket removes only from the skeleton.

**project_tree.** `renderAIDocs` stops skipping `ai-docs/spec`, so the directory
now renders as a plain `ai-docs/` subdirectory exactly as mental-model already
did. Pinned by fixture in `project_tree_test.go`: the directory lists, and
neither a `spec:` header nor an anchor entry appears.

**Conventions.** `ticket-conventions.md` lost exactly the spec-gate Status Flow
bullets and the `idea/`/`todo/` `spec:` allowances. Three further lines were
trimmed rather than deleted because only a clause referenced the retired layer:
the workset ready-exemption, the workset "spec-ready behavior" phrase, and the
Result-text "or linked spec" clause. Every other line is intact.

**Per-file disposition of the survivor sweep.** Rewritten because a deleted call
carried the step's meaning: `ticket-reviewer-design` (spec-territory conflict
check replaced by a declared-constraint check over `parent:`/`related:`, the two
edges that still exist), `reference-discovery` (reads `ai-docs/manuals/` and
`ai-docs/ref/` through `project_tree` instead of the deleted discovery tools, and
shortlists before reading now that no call is brief-scoped), `lead-add-rule` (see
below), `lead-bootstrap` (two `_index.md` drift routes replaced so the two
Candidate/Signal rows that fed them still resolve),
`ticket-reviewer-completeness` (the `ready/` sage-gate field check asked for a
retired `spec:` key on every promoted ticket). Trimmed: `code-reviewer`,
`impl-playbook` (its `### Mental Model Notes` commit subsection went with the
`git.commit` option, so the invariant now names `## AI Context` itself),
`executor-wrapup` (lost the Ancestor Loading section whole),
`lead-review`, `lead-workflow-manual`, `delegate-orientation`, `implementer`,
`fresh-reader-audit`, `lead-check-blockers`, `lead-ticket/task-list`.
Deliberately untouched: `skills/lead-bootstrap/AGENTS.template.md` and
`WORKFLOW.md` in both packages, whose versioned migration entries are a replay
log owned by the bootstrap-template sibling.
Deliberately kept: `hasSpecStemArgument` — the tickets tools have no generic
unknown-argument rejection, so removing it would turn a stale caller's
`spec_stem` into a silently unfiltered ticket list instead of an error.

**lead-add-rule reroute.** Domain-scoped rules now land in a manual under
`ai-docs/manuals/` declared through `AGENTS.md` `## Workflow` ->
`### Implementation Conventions`, the same generic hook the worker playbook
reads. Two consequences the review surfaced and this phase fixed: the skill now
targets `AGENTS.md` rather than `CLAUDE.md` (which is the `@AGENTS.md` shim in a
bootstrapped project, so a rule appended there is invisible to every playbook
that reads `AGENTS.md`), and the no-conventions-section case — the default
downstream state — routes to the no-matching-manual row, which proposes the
project's first manual and the section together, rather than to an absolute
"route everything to the root file" sentence that made the new destination
unreachable.

**Tests.** Suites whose subject is gone are deleted; every other deletion is
converted into an assertion that fails on reintroduction — `tools/list`
advertises none of the seven tools nor the three commit trailers,
`printPlaybook` resolves none of the seven stems, `ReadConvention` misses on the
retired names, the CLI rejects the retired flags and subcommands,
`TicketTemplate` carries none of the retired keys, the ticket graph resolves
ticket stems only, `project_tree` renders no spec area, and
`RETIRED_SKILL_NAMES` now catches a retired stem named in prose, not just a
restored directory.

**Verification** (full output read): `go build ./...` clean; `go vet ./...`
clean; `go test ./... -count=1` 15 packages ok; `agents-plugin/tests` 55 OK;
`agents-plugin-wsflow/tests` 10 OK; all five regen generators re-run and then
idempotent with no env var set; `diff -r agents-plugin/rsrc
agents-plugin-wsflow/rsrc` empty. A live stdio `tools/list` advertises none of
the seven tools and none of the three `git.commit` options, and
`convention.read(name: "ticket-conventions")` still returns the document.
Two review rounds: round 1 raised 5 Important + 5 Minor, all fixed in `0fd8fb33`
except `hasSpecStemArgument` (kept, rationale above); round 2 returned clean.

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
- Update `ai-docs/WORKFLOW.md` and this repository's `AGENTS.md` in one
  edit: the `## Documentation System` list, `## Code Standards` item 5
  (currently "Skill/agent authoring", naming `ai-docs/manuals/skill-authoring.md`
  only — AGENTS.md#L79-82 has no spec/mental-model content, so this bullet's
  third target does not exist as stated; the only other `## Code Standards`
  items are simplicity, surgical changes, responsibility check, and
  testability, none of them spec/mental-model either), and
  the `## Commit Rules` `## Spec` trailer and `renamed-spec:` line, to
  describe the reduced layout. The `### Binding Anchor` section is the
  epic's planned item and is not touched here.
- Record the after-pass input for the measurement manual: the removal changes
  the commit trailers the manual reads, so state the cut-over commit.

Verification: `git status` shows moves only, no content diffs
(`git diff -M --stat` reports pure renames); `go test ./...` and both Python
bundles still pass; `project_tree` output no longer names the removed areas; a
grep for one previously cited anchor still resolves inside `ai-docs/.old/`.
Confirm no shipped file under either plugin package references the moved
paths, excluding `skills/lead-bootstrap/AGENTS.template.md` and
`skills/lead-bootstrap/WORKFLOW.md` in both packages: those are owned by
`260909-feat-bootstrap-refoundation-template-migration`, which is the
cross-ticket seam, and they keep naming the directories until it lands.

Touchpoints: `ai-docs/spec/`, `ai-docs/mental-model/`, `ai-docs/mental-model.md`,
`ai-docs/.old/`, `ai-docs/WORKFLOW.md`, `AGENTS.md`, `CLAUDE.md` only if its
shim body changes.

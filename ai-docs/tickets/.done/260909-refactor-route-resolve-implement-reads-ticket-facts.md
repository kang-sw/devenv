---
title: "route.resolve_implement reads route facts from the sage-stamped ticket and drops the in-run survey and fast paths"
sage-review-design: completed
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260909-chore-ws-refoundation-git-history-measurement-manual: prerequisite; the epic makes the before-baseline a precondition for every removal
  260909-refactor-drain-ready-queue-worker-spawner: prerequisite; establishes the worker that becomes this resolver's caller and holds the lead-capability child key
  260909-refactor-lead-surface-collapse-worker-stop-protocol: prerequisite; defines the stop-and-report protocol the reduced `prep` guardrail and the ad-hoc path defer to
  260909-refactor-retire-spec-mental-model-layers: prerequisite for the doc todos and the spec anchors; retires the anchors that describe this resolver, which is why this ticket addresses no spec stem
  260909-research-ws-refoundation-evidence-audit: evidence for the cost table and the rejected alternatives restated below
  260908-feat-implement-skip-survey-for-localized-ticket-target: prior art; introduced the four-fact `plan_depth: none` exception this ticket generalizes into the default
  260908-feat-survey-plan-is-route-not-contract: prior art; already reduced the survey plan to a route rather than a contract
sage-review-completeness: completed
sage-review-design-reviewed: b39ecf90d7ce1fe4
sage-review-completeness-reviewed: b39ecf90d7ce1fe4
completed: 2026-09-09
---

# route.resolve_implement reads route facts from the sage-stamped ticket and drops the in-run survey and fast paths

## Background

Under epic `260909-epic-ws-worker-interpreter-refoundation`, Cross-Child
Decision 3 makes the sage-stamped ticket the plan and the place where key
decisions are encoded, and Decision 4 makes a native-harness worker the
interpreter that executes a whole ticket while the lead never edits source.
`route.resolve_implement` is the last stage that still assumes the opposite:
it consumes facts the *lead* gathered from conversation, and it schedules an
in-run survey to recover navigation the ticket already contains.

Three consequences follow from those decisions.

**The facts have the wrong source.** The resolver takes about thirty leaf
fields across `facts.scope`, `facts.complexity`, `facts.risk`, and `policy`,
all gathered by the lead before the call. Decision 3 puts that judgment at
ticket authoring — fact population by a cheap tier, design review by a heavy
tier — so re-deriving the same values per run is the authoring pipeline's
output being thrown away and re-made by the expensive model.

**The in-run survey is a second planning pass over an already-planned
ticket.** `260908-feat-implement-skip-survey-for-localized-ticket-target`
already found that for tickets which freeze the file sweep and call sites, the
survey re-lists what the ticket said and the delegate hop bought nothing; it
carved out a four-fact exception. With the ticket as the plan, the exception is
the rule, and `plan_depth` has nothing left to select.

**The two fast paths have no holder.** `automaticDirectEditEligible` exists so
the lead can edit inline instead of delegating; `automaticLeadOnlyReviewEligible`
exists so the lead can skip independent review on work it did itself. The lead
no longer edits source, so the first path has no subject, and the second would
have the worker review its own edits — which epic `## Non-Scope` forbids, since
independent review is the one gate whose value this repository's history
evidences (relaxing it in `260828` raised abort rates and was partially restored
in `260831`).

The `prep` guardrail is the invisible half of the cost. It is a Go template
string, not a document, so it never appeared in a docs-only audit: every
implement run injects "run mental-model lookup, read returned docs ancestors
first, `<binding anchor clause>`, and read `infra.read("impl-playbook")`" before
any work, unconditionally. The evidence audit's cost table counts the
impl-playbook alone at 75 lines injected on every route, plus about 150 lines of
mental-model doc, against a total mandated read of about 1,577 lines before any
source is read.

## Decisions

- **Route facts come from the sage-stamped ticket, not from the caller.**
  Decision 3 places the judgment at authoring; this ticket makes the resolver
  consume it. The survey found that the fact populator writes nothing today
  and the sage stamp records only review posture, so no existing artifact
  carries these values; epic Cross-Child Decision 15 settles the transport:
  the fact populator gains edit rights on the ticket it populates and writes
  a route-facts body section (a table under a stable heading), and the
  resolver reads that section through the `wsdoc` ticket projection. Body
  placement is what makes the sage stamp cover the facts: the stamp digests
  the body, so facts edited after the stamp invalidate it.
  - Rejected: frontmatter keys — would need a separate freshness check
    because the stamp digest covers the body only, and grows a ~30-field
    block. Rejected: extending `SageRecord` — the stamp is review posture, and
    the facts should be written by the tier that populated them, not the tier
    that reviewed them. Rejected: run-time derivation — puts derivation back
    at execution, which is the cost this ticket removes.
  - Rejected: keeping the caller-supplied facts and merely defaulting them from
    the ticket. That leaves two sources of truth for the same judgment and
    preserves the lead turn spent restating it, which is the cost being removed.
- **The in-run survey and research stages are removed, along with `plan_depth`.**
  `plan_depth` only ever produced `survey` or `none`; `research` is reachable
  only as a runtime escalation from a survey return. With the survey gone, the
  planner stems `plan-populator-survey` and `plan-populator-research` have no
  dispatcher on this path, and the plan-path allocation and stub-writing
  choreography go with them. Decision 7 keeps the information-loss mitigation
  that mattered: the worker reads the ticket source, not another agent's
  summary.
  - Rejected: keeping `plan_depth: survey` as an opt-in for weak tickets. A
    ticket too weak to execute is a ticket that failed the sage gate; the
    correct response is the closed stop list's "ticket decision contradicted by
    code reality" (Decision 5), not a second planning pass at run time.
  - Rejected: worker-side survey as a mandatory child dispatch. Decision 4
    recommends the worker spawn Explore-class children for survey and review at
    its own discretion; making it a resolver-scheduled stage reintroduces the
    stage under a new name.
- **The `direct-edit` and `lead-only review` fast paths are removed.** With the
  lead out of the editing seat the delegation axis collapses: the worker
  executes. Independent review stays, risk-keyed and severity-graded, dispatched
  by the worker (epic `## Non-Scope`), so `review_alloc` keeps its `single` and
  partitioned outcomes and loses only `lead-only`.
  - Rejected: retaining `lead-only` for trivial changes. That is precisely the
    "three reviewers on a three-line change" over-escalation inverted; the
    audit's A7 verdict says the over-escalation was the allocation, not the
    existence of review, and the partition logic already scales it down.
- **The `prep` guardrail is reduced to what a worker needs.** The mental-model
  lookup clause dies with the layer that backs it
  (`260909-refactor-retire-spec-mental-model-layers`). What survives is a
  question for the stop-protocol sibling, not this ticket's invention: the
  binding-anchor clause is read through a generic hook and stays only if the
  worker still needs it before edits, and `infra.read("impl-playbook")` stays
  only if its content is not already the worker playbook. The reduction is
  stated as a target here and its final content is agreed with
  `260909-refactor-lead-surface-collapse-worker-stop-protocol`.
- **The low-ceremony current-branch path is removed with the other fast
  paths.** `currentBranchImplementEligible` is built on the two predicates
  Phase 1 deletes and `ExplicitDelegationRequest`, so it cannot survive Phase
  1 unchanged. It goes: the caller is now always a worker on a goal staging
  branch, one branch per unit of work is the uniform rule, and an ad-hoc
  target gets a branch exactly as a ticket target does. `low_ceremony_if_safe`
  is therefore orphaned and removed in Phase 2 with the other orphaned
  fields; the two tests that pin the path
  (`TestResolveImplementCurrentBranchPreferenceGate`,
  `…CurrentBranchCompletionNearMisses`) are deleted in Phase 1, not kept.
  - Rejected: inline the deleted predicates to keep the path — preserves a
    third fast path for the one caller class the epic removed from the
    editing seat.
- **Missing route facts are a lead-side condition, never a worker stop**
  (epic Cross-Child Decision 23). Three defined behaviors: the `ready/`
  promotion gate checks that `## Route Facts` is present (presence only; the
  sage reviewer judges content); the resolver reports an absent or unreadable
  block as a named outcome in its verdict text rather than a conservative
  default; and `lead-run`, before spawning, renders the fact populator once
  when the section is absent, so tickets promoted before this lands — this
  repository's own remaining children included — and tickets from older
  plugins never reach a worker without facts. No downstream migration item.
  - Rejected: stop (c) on missing facts — a spawn, a stop, and a lead turn
    per legacy ticket for a fact one cheap-tier call supplies.
- **Runtime policy inputs stay caller-supplied.** Only the facts a ticket can
  carry move to the ticket: scope, surface, test surface, and the risk axes.
  `policy.branch.*` (including `merge_confirm`), `review.override`, and
  `docs.*` describe the run, not the ticket, and the worker keeps supplying
  them; the branch-plan machinery they feed is out of scope here.
- **Ad hoc `implement <description>` proceeds without a ticket.** The worker
  takes the description as its contract, proceeds, and stops only on the closed
  stop list (Decision 5): a low-reversibility merge, an `[escalate-to-lead]`
  entry or Open Decision Queue, a decision contradicted by code reality, an
  always-ask irreversible action, or a Critical finding still open after the
  fix round. Everything else it decides, records in `## AI Context`, and carries
  into the merge-stop report for veto.
  - Rejected: forcing an ad-hoc description through ticket creation first. The
    epic keeps inventory stage moves as user-and-lead batch actions (Decision 8);
    minting a ticket per ad-hoc request puts promotion work back on the lead,
    which is the A6 failure the epic is fixing.

## Constraints

- **Shipped-surface rule (AGENTS.md Architecture Rule 4).** Everything this
  ticket touches is shipped: the resolver's emitted todo instructions, the
  `prep` guardrail string, the verdict reason lines, and the `lead-implement`
  playbook all run in projects that hold only what bootstrap installs. No text
  may name this repository's tickets, epics, commit hashes, layout, tooling, or
  migration vocabulary; project-specific input goes through a generic hook. The
  binding-anchor clause is the existing example of that pattern and must stay
  one. Guard: `python3 -m unittest discover agents-plugin/tests`.
- **wsflow mirroring (`ai-docs/manuals/wsflow-mirroring.md`).** `lead-implement`
  is a shared `rsrc/` playbook mirrored byte-identically into
  `agents-plugin-wsflow/`, and `plan-populator-survey` / `plan-populator-research`
  are on the wsflow render-eligible stem list. Removing them changes that list,
  the manifests, and the wsflow package tests; regenerate with the manual's env
  vars and `-count=1` rather than hand-editing the mirror.
- **The published MCP schema stays opaque.** `params` is declared
  `{"type":"object"}` with only `session_key` required, pinned by
  `TestRouteResolveImplementSchemaIsOpaque`. Whatever the fact source becomes,
  the tool must not grow a typed public surface — that is the standing policy
  from `260903-epic-mcp-tool-surface-affordance-reduction`, which the epic
  subsumes rather than reverses.
- **No `spec:` field, deliberately.** The anchors that describe this resolver
  today are `{#260625-session-state-tools}`, `{#260827-ticket-stem-word-key-branch}`,
  `{#260830-review-watermark-checkpoint-nudge}`, `{#260620-ticket-close-tool}`
  and `{#260505-tool-profile-gating}` in `ai-docs/spec/mcp-tools.md`, and
  `{#260505-implementation-workflow-skills}`,
  `{#260612-reviewer-allocation-tier-default}`,
  `{#260619-stateless-implement-review-continuity}`,
  `{#260523-implement-doc-closeout-compaction}`,
  `{#260707-implement-branch-cleanup-naming-gate}`,
  `{#260505-proceed-routing-pipeline}`,
  `{#260519-proceed-implementation-dispatch-precheck}`,
  `{#260529-wsflow-converged-implement-spine}` and
  `{#260505-workflow-delegate-prompt-boundaries}` in
  `ai-docs/spec/workflow-skills.md`. They are listed as reading material for the
  implementer, not as addressing targets: the spec layer retires under
  `260909-refactor-retire-spec-mental-model-layers`, so this ticket addresses no
  spec stem and adds no `## Spec Impact`.
- **Behavior is pinned by a large test corpus, and that is the contract**
  (Decision 3). `implement_resolver_test.go` is 1,231 lines and 32 top-level
  tests; the implement slice of `session_state_test.go` is about 20 more. Tests
  whose subject is removed are deleted with it; tests whose subject moves are
  rewritten against the new source, not deleted.
- **Out of scope:** the branch-plan and merge-root machinery, the review
  watermark checkpoint nudge, the `tickets.close` ahead-of-merge-root
  observation, and independent review itself. The epic's `## Non-Scope` keeps
  the Go route resolver as a mechanical tool; this ticket changes what it reads
  and what stages it schedules, not that it exists.
- Exclude the duplicate plugin trees under `.claude/worktrees/` from sweeps.

## Prior Art

Found by search; rerun the greps rather than trusting coordinates.

**The resolver.** `agents-plugin-tool/internal/mcp/implement_resolver.go`
(about 1,086 lines). Input types `implementInput`, `implementTargetInput`,
`implementFactsInput{Scope, Complexity, Risk}`, `implementPolicyInput`, every
leaf a `factString` with `parseEnumFact` / `parseObjectString` / `factOr` shared
from `proceed_resolver.go`. Pipeline in `resolveImplement`:
`normalizeImplementFacts` → `deriveImplementDelegation` →
`deriveImplementPlanDepth` → `deriveImplementReviewAlloc` →
`deriveImplementDocMode` → `deriveResolvedImplementBranchPlan`, then
`implementConditions`, `implementReason`, `implementNextInstruction`,
`renderImplementRaw`.

Fact families to relocate: `facts.scope` (`span`, `surface`,
`new_public_symbol`, `new_type_contract`, `test_surface`,
`explicit_delegation_request`, `explicit_direct_edit_request`),
`facts.complexity` (`change_points`, `reuse_points`, `strategy_shape`,
`side_effect_risk`, `cold_context`), `facts.risk` (`correctness`, `fit`,
`test`, `security_or_contract`), and `policy` (`low_ceremony_if_safe`,
`branch.*`, `review.override`, `docs.*`). Survey finding worth acting on:
`cold_context` is parsed and normalized but read by no derivation, condition,
or reason string — a dead field the relocation should drop rather than carry.

Removal targets: `automaticDirectEditEligible` (single-file, internal, no new
public symbol, no new type contract, `test_surface != new-files`),
`automaticLeadOnlyReviewEligible` (direct-edit plus four `low` risk facts),
`deriveImplementDelegation` and its `explicit_*_request` overrides,
`deriveImplementPlanDepth` and the four-fact ticket exception it carries,
and the `lead-only` arm of `deriveImplementReviewAlloc`. Keep
`implementReviewPartitions` and `partitionedReviewAlloc`.

**Todo installation.** `agents-plugin-tool/internal/mcp/session_state.go`:
`deriveImplementTodosFromVerdict` builds the list, `(*sessionStore).enterMode`
performs the single atomic write, `(*Server).handleEnterImplement` dispatches
it. Todo ids in order: `route`, `prep`, `edit`, `review` (when `NeedReview` or
lead-only), then `doc-pre-pass`, `doc-commit-gate`, `doc-closeout` when
`NeedDoc`, then either `complete` or `final-action-gate` + `merge`. Titles from
`implementPrepTitle` ("Prep" / "Prep (survey plan)" / "Prep (research plan)").
`handleEnterImplement` also fills `BindingAnchorClause` from
`wsreview.ReadAgentsBindingAnchor(root).PrepClause()` and retains a legacy
top-level argument path (`parseImplementDelegation`,
`parseLegacyImplementPlanDepth`, `parseImplementReviewAlloc`,
`deriveImplementTodos`) that must be reconciled or removed with the new shape.

**The `prep` guardrail.** `implementPrepInstruction` in the same file. Its
unconditional preamble is `"Before edits or dispatch, run mental-model lookup,
read returned docs ancestors first, " + verdict.BindingAnchorClause + "and read
infra.read(\"impl-playbook\"). "`, followed by a `plan_depth` switch whose
`none`+delegated arm carries the entire six-section stub-plan choreography and
whose `survey` arm carries the planner dispatch and the `[escalate-to-research]`
/ `[escalate-to-lead]` handling. `plannerAuthorityInputs` switches the render
variable list on `inline` versus `ticket`.

**Where the fact schema is declared — three divergent places.** The published
JSON schema in `internal/mcp/server.go` is deliberately opaque; the real schema
is Go struct tags plus hand-rolled `parseImplement*` validators; the
human-authoritative copy is the `## Fact Contract` tables in
`agents-plugin/rsrc/lead-implement/lead-implement.md` (mirrored in wsflow),
whose text is pinned by `internal/mcp/playbook_tools_test.go`. Any relocation
must land in all three or delete two of them.

**The authoring-side artifacts the facts would come from.**
`agents-plugin/rsrc/ticket-fact-populator/ticket-fact-populator.md`
(`kind: render`, `role: delegate`, `tier: medium`) writes nothing — it returns a
plain-text verdict with `checked:` / `corrections:` / `decision_gaps:` /
`unverified:` / `relations:` blocks and states "You never edit the ticket; the
caller applies what you return". `tickets.sage_stamp` and `tickets.sage_gate`
live in `internal/wsdoc/tickets_sage.go` and `tickets_sage_freshness.go`;
`SageRecord` writes only the frontmatter fields `sage-review-design`,
`sage-review-completeness` and their `-reviewed` sha256 body digests via
`writeFrontmatterField`, plus a `## Blocked (<date>)` body section. Neither
artifact carries a route fact today.

**Ticket frontmatter round trip.** `internal/wsdoc/frontmatter.go` is a
hand-rolled parser (`frontmatter`, `frontmatterFromText`, `cleanScalar`)
supporting scalars, `- ` lists, and one nesting level; `writeFrontmatterField`
in `tickets_mutate.go` does a line-level replace-or-insert that byte-preserves
the rest of the file. Arbitrary keys survive a round trip and `tickets.verify`
enforces no key whitelist — but `readTicketFromBytes` projects only `title`,
`parent`, `related`, `spec`, `spec-remove`, `plans`, `skeletons`, `completed`
into `TicketInfo`, so a new facts key would need a new reader.

**The ad-hoc path.** No `ticket_target` symbol exists; the ad-hoc case is
`target.kind == "inline"` (or the `"unknown"` default). `deriveImplementPlanDepth`
gates its skip on `targetKind == "ticket"`, so an inline delegated target always
gets `survey` however strong its facts; `currentBranchImplementEligible` — the
eleven-condition low-ceremony fast path that yields `Branch Action: current` and
a `complete` tail instead of `final-action-gate` + `merge` — is inline-only, so
it is the only way an ad-hoc target skips branch creation and merge todos.
`target.kind: "unknown"` is the degenerate case: it fails both checks and also
falls through to the ticket-shaped `plannerAuthorityInputs` variable list,
naming `ticket_path` and `selected_phase` for a target that has neither.
Upstream routing is `proceed_resolver.go`'s `implementation-dispatch.inline-direct`.

**Tests.** `internal/mcp/implement_resolver_test.go` (1,231 lines, 32 top-level
tests) pins each direct-edit predicate individually
(`TestResolveImplementStrategyRules`,
`TestResolveImplementExplicitDirectEditOverridesMultiFileScope`,
`TestResolveImplementCurrentBranchPreferenceGate`,
`TestResolveImplementCurrentBranchCompletionNearMisses`), lead-only review
(`TestAutomaticLeadOnlyReviewEligibleRequiresGenuineLow`), and `plan_depth`
(`TestResolveImplementDelegatedDefaultsToSurveyPlan`,
`TestResolveImplementDelegatedLocalizedTicketSkipsSurveyPlan`,
`TestResolveImplementSurveyEscalatesResearchFromSurveySignal`,
`TestResolveImplementInlineDelegatedNextDefersPlannerAuthorityToPrep`).
`internal/mcp/session_state_test.go` adds about 20 pipeline tests including
`TestDeriveImplementTodosPrepGuardrails`, `…DelegatedNoneStub` (pins the six
stub sections), `…InstructionsDirectEditLeadOnly`, and
`TestRouteResolveImplementSchemaIsOpaque`. Also touching it:
`review_watermark_checkpoint_test.go`, `playbook_tools_test.go`,
`mercenary_surface_test.go`, `panic_recovery_test.go`.

## Open Questions

The heading and table format are the placed draft's; the current-branch
path, the missing-facts behavior, and the caller inputs are settled under
`## Decisions`. Remaining:

- **Which facts survive the collapse.** Removing the delegation axis, the plan
  stages, and the lead-only arm leaves `facts.risk` and the review-partition
  inputs clearly needed and the `explicit_*_request` and `low_ceremony_if_safe`
  policy fields clearly orphaned; `facts.scope` and `facts.complexity` are used
  by review partitioning as well as by the removed paths, so the surviving set
  must be derived from the remaining consumers rather than assumed. `cold_context`
  is dead today and should not be carried forward.
- **Final content of the reduced `prep` guardrail.** Agreed with
  `260909-refactor-lead-surface-collapse-worker-stop-protocol`; this ticket
  states the target (drop the mental-model clause, keep only what a worker
  cannot derive) but does not fix the wording alone.

## Phases

Sequentially dependent: Phase 1 removes the consumers, which determines how
small the fact set in Phase 2 has to be. Doing Phase 2 first would relocate
fields that Phase 1 then deletes.

### Phase 1: Collapse the resolver — remove the survey stages and the two fast paths

Goal: `route.resolve_implement` schedules no in-run planning stage and offers no
path in which the caller edits source or reviews its own work. The facts still
arrive from the caller; only the consumers change.

Scope:

- Delete `deriveImplementPlanDepth`, the `plan_depth` output field, the
  `survey` and `research` arms of `implementPrepInstruction`, the stub-plan
  choreography in its `none`+delegated arm, `plannerAuthorityInputs`, and the
  `Prep (survey plan)` / `Prep (research plan)` titles. Remove the
  `plan-populator-survey` and `plan-populator-research` dispatch from
  `lead-implement` and from the render-eligible stem list in both packages;
  update `ai-docs/manuals/wsflow-mirroring.md`'s render-dispatch section in the
  same change.
- Delete `automaticDirectEditEligible`, `automaticLeadOnlyReviewEligible`,
  `deriveImplementDelegation`, `currentBranchImplementEligible` and the
  low-ceremony `Branch Action: current` path it gates, the
  `explicit_delegation_request` and `explicit_direct_edit_request` fields,
  and the `lead-only` arm of `deriveImplementReviewAlloc`. `review_alloc` keeps `single` and its
  partitioned outcomes; `implementReviewPartitions` and `partitionedReviewAlloc`
  are unchanged.
- Reduce `implementPrepInstruction` to the agreed worker preamble: drop the
  mental-model lookup clause, keep the binding-anchor clause only if the
  stop-protocol sibling still needs it before edits, and re-evaluate
  `infra.read("impl-playbook")` against the worker playbook's content.
- Update the `## Fact Contract` tables to match, in both packages, in
  whichever playbook carries them when this phase runs: `lead-implement` if
  `260909-refactor-lead-surface-collapse-worker-stop-protocol` Phase 2 has not
  landed, otherwise `ticket-worker`.

Verification: `go build ./...` first and record every compile error as a
discovered consumer; then `go test ./...` and `go vet ./...` under
`agents-plugin-tool/`, `python3 -m unittest discover agents-plugin/tests`, and
the wsflow bundle with a mirror regeneration (`-count=1`, the manual's env
vars). Delete the tests whose subject is gone
(`…DelegatedDefaultsToSurveyPlan`, `…DelegatedLocalizedTicketSkipsSurveyPlan`,
`…SurveyEscalatesResearchFromSurveySignal`,
`…AutomaticLeadOnlyReviewEligibleRequiresGenuineLow`,
`…InstructionsDirectEditLeadOnly`, `…DelegatedNoneStub`,
`…CurrentBranchPreferenceGate`, `…CurrentBranchCompletionNearMisses`); keep
and adjust the partitioned-review, branch-plan, and merge-root suites, which
must be behaviourally unchanged apart from the removed current-branch
shortcut. Add a test asserting the emitted todo list for a
ticket target contains `route`, `prep`, `edit`, `review` and no planning stage,
and one asserting a low-risk change still allocates an independent reviewer.
Re-run the shipped-surface guard after each instruction-text edit.

Touchpoints: `agents-plugin-tool/internal/mcp/implement_resolver.go`,
`session_state.go`, `server.go` (dispatch and the opaque schema, unchanged in
shape), `internal/mcp/{implement_resolver,session_state,playbook_tools}_test.go`,
`agents-plugin/rsrc/lead-implement/` and its wsflow mirror,
`agents-plugin/rsrc/plan-populator-survey/`,
`agents-plugin/rsrc/plan-populator-research/`, both `manifest.json` pairs,
`ai-docs/manuals/wsflow-mirroring.md`.

### Result (407b12a8) - 2026-09-09

Landed as `a894d3db..407b12a8` on `impl/epic/refound/setup-sweep-crave`.

**What landed.** `a894d3db` collapsed the resolver: deleted
`deriveImplementPlanDepth` and the `plan_depth` output field,
`plannerAuthorityInputs`, `deriveImplementDelegation`,
`automaticDirectEditEligible`, `automaticLeadOnlyReviewEligible`,
`currentBranchImplementEligible` with the `Branch Action: current` path and its
`validObservedStartCommit` / `deriveResolvedImplementBranchPlan` helpers, the
`explicit_delegation_request` / `explicit_direct_edit_request` facts, the
`lead-only` review arm, `implementPrepTitle`, `implementCompletionInstruction`,
`isCurrentBranchCompletion`, `isLeadOnlyReview`, and
`parseLegacyImplementPlanDepth`. `implementReviewPartitions` and
`partitionedReviewAlloc` are kept. Delegation is now the constant
`implementDelegationMode = "delegated"`; `NeedReview` is unconditionally true.
`6ea093ed` deleted the two `plan-populator-*` playbooks with their wsflow
mirrors, dropped them from `wsflowRenderEligibleStems`, rewrote the
`reference-discovery` line that named `plan-populator-survey`, and updated the
render-dispatch section of `ai-docs/manuals/wsflow-mirroring.md`. `2f96d79b`
and `407b12a8` are the review round-1 fix commits (below).

**Verification** (all output read in full, all green):
`go build ./...`; `go vet ./...`; `go test ./...` (14 packages ok);
`python3 -m unittest discover agents-plugin/tests` (55 tests, OK);
`python3 -m unittest discover agents-plugin-wsflow/tests` (10 tests, OK);
`go test ./internal/wsrsrc/... -count=1`; the five env-gated regen entrypoints
re-run with `-count=1` produced no drift; `diff -r agents-plugin/rsrc
agents-plugin-wsflow/rsrc` is empty.

**Review.** Round 1 (one full-scope reviewer, the allocation the route set):
3 Important, 8 Minor, no Critical. Round 2 (verification-only): clean — all
three Important and all eight in-scope Minor [fixed], no regressions.
Dispositions:

- Three files left not gofmt-clean by the collapse — [fixed] (`2f96d79b`).
- `implementer-relay` / `implementer-elevated` still declared `PlanPath` and
  told the agent to read a plan whose only producer this phase deleted, so the
  render substituted the empty string (`wsrsrc` rejects only *undeclared*
  keys, so nothing surfaced it) — [fixed] (`407b12a8`): variable renamed to
  `TargetPath`, plan-referring prose repointed at the target, and the
  `## Relevant Ticket Contract` clause — a section of the deleted plan file —
  replaced by a target-as-contract clause. Mirror and both manifests
  regenerated.
- The retained `delegated` label contradicted its own instruction (the same
  todo now tells that caller to edit the source itself) and no shipped
  playbook read it — [fixed] (`2f96d79b`): the Edit todo is titled `Edit`,
  `implementEditTitle` is gone, and `implementDelegationMode` documents that
  the value names how the caller was reached, not what it does.
- Eight Minor (Prep wording collision, undocumented legacy-axis asymmetry, an
  over-broad test comment, a lost helper doc comment, blank-line drift, a
  stale line-number citation, a re-wrap, docs drift) — seven [fixed]
  (`2f96d79b`, `407b12a8`); the spec/mental-model drift is
  [deferred: the ticket's Constraints assign those anchors to
  `260909-refactor-retire-spec-mental-model-layers`].

**Decisions taken.**

- The second Open Question (final prep-guardrail wording) is resolved here,
  since the sibling lead-surface ticket never settled it: the Prep preamble
  keeps only what the executing caller cannot derive from the target — the
  project-declared binding anchor, read through the generic
  `ReadAgentsBindingAnchor` hook so a project that declares none renders
  nothing, plus `infra.read("impl-playbook")`, whose test-strategy table,
  mechanical-edit criteria and unchanged-input verification rule the later
  gates cite by name and which the worker playbook does not carry. The
  mental-model lookup clause is dropped with the layer that backed it.
- The `## Fact Contract` tables this phase asks to update do not exist:
  `lead-implement` was deleted by the already-landed
  `260909-refactor-lead-surface-collapse-worker-stop-protocol`, and the
  `ticket-worker` playbook carries no such section. Recorded as a cosmetic
  adaptation rather than a stop.
- `lead-only` was still reachable through the `policy.review.override` enum
  and `parseImplementReviewAlloc`; the phase text names neither, but leaving
  either would have kept the deleted allocation reachable, so both were
  closed.
- The Edit todo instruction was rewritten because `PlanPath` no longer exists
  to hand off; the new text converges on what the shipped `ticket-worker`
  playbook already says in its own edit step.
- The three retired legacy top-level axes are handled asymmetrically on
  purpose: `delegation` and `review_alloc` still select behavior, so a retired
  value errors rather than silently downgrading what the caller asked for,
  while `plan_depth` selected a stage that no longer exists at all and is
  dropped. Documented in one comment on each site.
- No spec or mental-model edit, per this ticket's Constraints.
- Scope for the relay repoint was held to the two playbooks the route
  dispatches. `implementer` and `review-adjudicator` also still declare
  `PlanPath` but no shipped text dispatches either; forward note recorded on
  `407b12a8`.
- Commit-hygiene slip: the `git rm -r` of the two `plan-populator-*` trees was
  already staged when `a894d3db` was written, so those four file deletions
  landed in the resolver commit even though `6ea093ed`'s message describes
  them. Not amended — the branch is shared, and a correction is a new commit,
  not a rewrite.


### Phase 2: Read the surviving facts from the sage-stamped ticket

Goal: for a ticket target the resolver derives its route facts from the
ticket, and the caller supplies the target, the session key, and the runtime
policy inputs only (`policy.branch.*`, `review.override`, `docs.*`) — no
route fact. For an ad-hoc
description the worker proceeds from the description under the closed stop list.
Depends on Phase 1's landed Result, which fixes the surviving fact set.

Scope:

- Implement the transport by placing the committed draft
  `ai-docs/ref/refound-drafts/ticket-fact-populator.md` over
  `agents-plugin/rsrc/ticket-fact-populator/ticket-fact-populator.md` (move
  the text; delete the draft and its README row; fresh-reader audit once).
  The draft already carries the single-file edit scope, the `### Result`
  immutability rule, the `## Route Facts` table (heading, columns, placement
  before `## Phases`), and the `## Constraints` convention citations (epic
  Cross-Child Decision 18); resolve its `[design-review: ...]` marker to the
  fact set Phase 1's Result fixed and delete the marker. The resolver reads
  that section through the `wsdoc` ticket projection — extend the projection
  rather than adding a second parser.
- Make a missing or unreadable fact block a defined outcome at three points
  (Decisions): the `ready/` landing in `tickets_sage.go` refuses a
  non-exempt ticket with no `## Route Facts` section; the resolver names the
  absence in its verdict text instead of falling through to a conservative
  verdict — the audit's A8 verdict names "when in doubt fall through to full
  routing" as exactly the bias capable models obey; and the `lead-run`
  body gains one pre-spawn line: when the section is absent, render the
  fact populator on the ticket first.
- Define the ad-hoc path explicitly for `target.kind` `inline` and `unknown`:
  the description is the contract, the worker proceeds, and the verdict text
  points at the closed stop list rather than at a planning stage.
- Remove the orphaned caller-supplied fields (`explicit_*_request`,
  `low_ceremony_if_safe`) and the legacy top-level argument path
  (`delegation` / `plan_depth` / `review_alloc` / `need_review` / `need_doc`
  as top-level arguments), unless a grep of both packages finds a shipped
  caller, in which case record it and keep the shim. Keep the published
  schema opaque.
- Update the `## Fact Contract` section: if the lead no longer supplies facts,
  the section describes what the ticket must carry and where, or it is deleted.

Verification: `go test ./...`, `go vet ./...`, both Python bundles, and the
wsflow regeneration. New tests: a sage-stamped ticket fixture resolves to a
verdict with no caller facts; the same ticket with the fact block removed
produces the defined missing-facts outcome rather than a default verdict, and
a `ready/` landing of such a ticket is refused by the gate; an
ad-hoc inline target resolves without a planning stage and without a ticket
read; `TestRouteResolveImplementSchemaIsOpaque` still passes unchanged. End to
end, confirm `enter.implement` installs the expected todo ids for both a ticket
target and an ad-hoc description.

Touchpoints: `agents-plugin-tool/internal/mcp/implement_resolver.go`,
`session_state.go`, `internal/wsdoc/{frontmatter,tickets,tickets_mutate,tickets_sage}.go`,
`agents-plugin/rsrc/{ticket-fact-populator,lead-run}/` and the playbook that
carries the Fact Contract at that time (`lead-implement` or `ticket-worker`),
their wsflow mirrors, the resolver and session-state test files, both
`manifest.json` pairs, `ai-docs/ref/refound-drafts/README.md`.

### Result (2a2809de) - 2026-09-09

`route.resolve_implement` now derives every routing fact from the ticket's
`## Route Facts` section. The caller supplies the target, the session key, and
the runtime policy (`policy.branch.*`, `review.override`, `docs.*`) and nothing
else; a `facts` argument is refused outright rather than merged, since a fact
defaulted from the ticket but overridable by the caller leaves the same
judgment in two places, which is the cost this phase exists to remove.

Landed:

- `wsdoc` projection extended rather than duplicated: `TicketInfo` gains
  `RouteFactsPresent` and `RouteFacts`, parsed by `ticketRouteFacts` (header
  and alignment rows skipped by shape, section ended by any heading, only the
  first two columns read — the evidence column is the reviewer's). `TicketAt`
  is the new by-path reader; it confines a caller-supplied path to the board
  and rebases an absolute path under the root first, so the path's spelling
  does not decide what counts as a ticket.
- The resolver's `loadImplementRouteFacts` resolves the target to one of four
  named statuses — `ticket`, `absent`, `unreadable`, `ad-hoc` — reported in the
  verdict text, the conditions, and the warnings. It resolves the ticket by
  path, by absolute path, or by stem alone. An unrecognized fact key, a value
  outside its enum, and a table missing any accepted fact all make the block
  unreadable: the strictness runs in both directions because an omitted row
  parses to the same `unknown` an author can write deliberately, and an
  all-unknown risk set allocates the *smallest* review.
- The eleven surviving facts are those that feed `implementReviewPartitions`
  or the verdict's scope statement. `change_points`, `strategy_shape`, and
  `cold_context` are gone with the derivations Phase 1 deleted, as are
  `low_ceremony_if_safe` and the orphaned `explicit_*_request` fields, and the
  legacy top-level argument path is removed with the tests whose subject was
  that path. `TestRouteResolveImplementSchemaIsOpaque` still passes unchanged.
- Missing facts are a defined outcome at three points: `SageGate` refuses a
  `ready/` landing of a non-exempt ticket with no section
  (`stop_missing_route_facts`, with its own `next_instruction`); `tickets.move`
  tips the same gap advisorily; and the resolver names the absence instead of
  falling through to a conservative verdict.
- An ad-hoc target (`inline`, `unknown`) reads no ticket at all: the verdict
  says so and points at the worker's closed stop list.
- The committed populator draft is placed over the shipped playbook with its
  `[design-review: ...]` marker resolved, its table matching the resolver's
  keys and enums exactly; `lead-run` populates before spawning when the
  section is absent or a worker reports it incomplete, and `lead-ticket`
  always populates for a `ready/` landing.

Verification (all read in full, green): `go build ./...`, `go vet ./...`,
`go test ./... -count=1` (14 packages), `python3 -m unittest discover
agents-plugin/tests` (55), `agents-plugin-wsflow/tests` (10),
`diff -r agents-plugin/rsrc agents-plugin-wsflow/rsrc` empty, all five
env-gated regeneration entrypoints re-run with `-count=1` producing no drift,
and a leak scan over every changed shipped surface.

Review: partitioned across correctness, fit, and test. Round 1 raised two
Important (an incomplete table routing as a deliberate all-unknown verdict and
so buying the *least* review; an absolute `ticket_path` to a real ticket
hard-stopping the run with a remediation that could not fix it), one Important
in the test partition (the confinement test passed by coincidence through an
unrelated read error), and seven Minor. All Important and four Minor were
fixed in `8f0093a0` and `7991d938`; round 2 returned clean on all three
partitions, the test reviewer confirming by mutation that the confinement test
now genuinely pins its guard. Left as reported: the third read of the same
ticket in the `ready/` move branch (it matches the shape already there), a
sparse-checkout-hidden ticket reading as `absent`, `missingRouteFacts` failing
open on an I/O error, and a second `## Route Facts` section being ignored.

A fresh-reader audit of the placed populator found the fact names and enums
correct against the resolver but the block itself hazardous to its own reader:
escaped alternations with no filled example, and no rule that the cells are
plain text, either of which silently makes a ticket unroutable. Both are closed
by construction in `92389a0f`.

Open Question 1 ("which facts survive the collapse") is answered by the
eleven-fact set above, derived from the decision consumers rather than chosen.


## Sage Review Round 1 (2026-09-09)

### Design Reviewer — pass

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Three-round Critical wording predates epic Decision 20 | minor | Reworded to Critical still open after the fix round. |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | currentBranchImplementEligible cannot survive Phase 1 unchanged yet is left undecided | critical |
| 2 | Missing route facts behavior undefined for tickets promoted before the change (including this epic's own children) | major |
| 3 | Route-facts section heading and table format left to design review though the committed populator draft fixes them | major |
| 4 | Caller-supplied runtime policy inputs ambiguous under 'caller supplies only the target' | major |
| 5 | Fact Contract table location depends on collapse Phase 2 landing | minor |
| 6 | Legacy top-level argument path disposition open | minor |

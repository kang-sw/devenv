---
title: "Delegated implement targets skip the survey plan when the ticket already localizes the change"
sage-review-design: completed
sage-review-completeness: completed
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - 260625-session-state-tools
  - 260505-implementation-workflow-skills
  - 260519-proceed-implementation-dispatch-precheck
related:
  260908-feat-survey-plan-is-route-not-contract: prerequisite, both phases; Phase 1 makes the implementer read the ticket so a stub plan is enough, Phase 2 removes the plan from reviewer input and makes wrap-up deviations ticket-diffed so the stub is never judged as a contract
  260903-epic-mcp-tool-surface-affordance-reduction: policy source for "no new input field"; the skip condition reuses the existing complexity facts
  260908-bug-shipped-prep-guardrail-hardcodes-devenv-migration-anchor: why the stub's Codebase Findings rule names the Prep reads rather than any one project's anchor
related-mental-model:
  - workflow-skills
sage-review-completeness-reviewed: a02e844dc8406cab
sage-review-design-reviewed: 149405a7396b2fa6
---

# Delegated implement targets skip the survey plan when the ticket already localizes the change

## Background

`route.resolve_implement` derives `plan_depth` in
`agents-plugin-tool/internal/mcp/implement_resolver.go`
(`deriveImplementPlanDepth`): every `delegated` verdict returns `survey`,
and only `direct-edit` returns `none`. Automatic direct edit requires
`span: single-file`, `surface: internal`, no new public symbol, no new type
contract, and no new test files at once (`automaticDirectEditEligible`), so
almost every ticket-driven target is delegated and therefore surveyed. A
dead branch below the delegated short-circuit
(`ChangePoints == "clear" && SideEffectRisk == "low"` returning `none`,
unreachable because both remaining paths return `none`) shows the depth
decision was once meant to depend on change-point clarity.

Audit of the eight September plans under `ai-docs/.plans/2026-09/`
(2026-09-08). For `260903-refactor-mcp-verb-vocabulary-unification`,
`260903-refactor-mcp-read-surface-collapse`, and
`260903-refactor-mcp-todo-signature-merge` the ticket had already frozen the
file sweep, call sites, and acceptance greps; each survey's
`## Codebase Findings` re-listed them and the route was the ticket's own
edit list. The delegate hop and its context spend bought nothing. The two
plans whose tickets left connection points to discovery
(`260901-feat-note-oversize-layer-aware-clone-path`,
`260907-feat-ws-project-tree-parent-nested-ticket-render`) earned their
survey.

`260908-feat-survey-plan-is-route-not-contract` reduces the survey to a
route: the plan carries no contract text, and the implementer reads the
ticket. What the survey still delivers is navigation, which the lead's
complexity facts already describe: `change_points`
(`clear|partially-known|unknown`), `reuse_points`
(`confirmed|unconfirmed|not-applicable|unknown`), `strategy_shape`
(`single-obvious|multiple-viable|unknown`), and `side_effect_risk`
(`low|moderate|high|unknown`), all declared in the `lead-implement` fact
table and parsed by `normalizeImplementFacts`. When the lead can already
state all four at their strongest value, a survey has nothing left to find.

## Decisions

- **The skip condition reuses the four complexity facts.** A delegated
  ticket target resolves to `plan_depth: none` when `change_points: clear`,
  `reuse_points: confirmed` or `not-applicable`,
  `strategy_shape: single-obvious`, and `side_effect_risk: low` hold
  together; any other value on any of the four keeps `survey`. The dead
  branch is removed rather than revived, since its two-fact condition
  ignores reuse and strategy, the other two things a survey delivers. The
  facts are the lead's judgment, so the lead forces a survey by reporting
  the weaker value it actually holds; no override field is added.
  - Rejected: a new fact such as `ticket_localizes_change`. It is schema
    growth on a tool under `260903-epic-mcp-tool-surface-affordance-reduction`,
    and the four existing facts already encode the same judgment.
  - Rejected: reviving the dead two-fact branch as-is. Skipping while reuse
    or strategy is unknown drops navigation the implementer then re-derives
    in its own context.
- **A skipped survey still yields a plan file, written by the lead.** For
  `delegated` with `plan_depth: none` the Prep step allocates the plan path
  with `path.generate(kind: "plan", ...)` as today and the lead writes a
  stub: `## Relevant Ticket Contract` holds the ticket path and the
  selected phase heading (the same content `260908` Decision 2 leaves
  there); `## Out of Scope`, `## Implementation Plan`, and
  `## Verification Plan` each carry the line "Skipped: the ticket
  localizes the change (facts: change_points=clear, reuse_points=<value>,
  strategy_shape=single-obvious, side_effect_risk=low)", a non-empty
  section, so the survey playbook's "empty Implementation Plan or
  Verification Plan requires escalation" rule does not apply;
  `## Escalations` holds `None.` per `260908` Decision 6.
  `## Codebase Findings` carries the binding constraints the Prep
  guardrails already have the lead read before dispatch, when any apply:
  the mental-model lookup documents, `infra.read("impl-playbook")`, and
  any anchor document the project's `AGENTS.md` declares for the target's
  topics. Each is one line naming the source and the constraint, so the
  implementer inherits what the survey would have copied from the same
  reads; when none applies the section holds "Skipped: no binding
  constraint from Prep reads". The rule is stated in terms of the Prep
  reads, not of any one project's anchor, because the plugin ships to
  projects that declare different anchors or none
  (`260908-bug-shipped-prep-guardrail-hardcodes-devenv-migration-anchor`).
  The stub is lead-written, so the planner playbooks' output rules
  describe planner output and are not amended. The implementer is then rendered with
  `PlanPath` exactly as after a survey; with `260908` Phase 2 landed,
  reviewer dispatch and wrap-up do not change.
  - Rejected: a plan-less dispatch that passes the ticket path and phase
    as new `implementer` variables. It adds render variables and a second
    dispatch shape that reviewers and `executor-wrapup` would have to
    recognize. The file is not the cost being removed; the planner delegate
    hop is.
- **Ticket targets only.** A delegated inline target keeps `survey`: an
  inline contract has no file for the stub to point at, and `260908`
  Decision 3 places its verbatim text in the plan, which today needs a
  planner to write it. A lead-written inline stub is a possible follow-up
  once `260908` has landed; it is not decided here.

## Constraints

- No new `route.resolve_implement` input field, enum value, or output
  field; `plan_depth: none` for a delegated verdict is a new value
  combination on existing fields only.
- Block-depends on `260908-feat-survey-plan-is-route-not-contract`, both
  phases: without a ticket-reading implementer (Phase 1) the stub gives
  the delegate nothing to implement from, and until reviewers stop
  receiving the plan and wrap-up diffs against the ticket (Phase 2) a
  stub would make the reviewer's plan checks vacuous.
- Go touch points: `deriveImplementPlanDepth` and its reason string;
  `implementPrepInstruction` (its `none` case is worded for direct edit and
  needs a delegated variant that names the stub) and
  `implementEditInstruction` (the delegated `default` branch already
  dispatches without a plan-readiness clause; it must name `PlanPath`);
  `parseLegacyImplementPlanDepth` (its `delegated` case rejects `none`
  with "want survey"; the legacy compatibility entry accepts `none` for
  delegated too, so the error text does not state a contract the resolver
  no longer holds, and the message for `research` is unchanged);
  pinned tests `TestResolveImplementDelegatedDefaultsToSurveyPlan`,
  `TestDeriveImplementTodoInstructionsDelegatedSurvey`, and
  `TestDeriveImplementTodoInstructionsPrepGuardrails` in
  `agents-plugin-tool/internal/mcp/`.
- No playbook changes. `lead-implement` Prep describes no plan depth;
  depth handling is the Go-owned `implementPrepInstruction`, and Execute
  Verdict rule 4 has the playbook subsections add only what the
  instruction omits, so the stub step lives in the Go instruction alone.
  `deriveImplementPlanDepth` gains a target-kind parameter (available at
  its call site as `input.Target.Kind`) so Decision 3 is enforced in the
  resolver.
- Spec addressing: `ai-docs/spec/mcp-tools.md`
  `{#260625-session-state-tools}` (the sentence "`plan_depth` is `none` for
  direct edit and `survey` for reachable delegated preparation");
  `ai-docs/spec/workflow-skills.md` `{#260505-implementation-workflow-skills}`
  (the sentence "Plan population defaults to the survey planner for
  delegated mode", and the delegated-mode sentence "generates a plan
  path, dispatches a planner to write or refine that single implementation
  plan, spawns an implementer agent with the plan", which gains the
  lead-written stub as the no-planner case) and
  `{#260519-proceed-implementation-dispatch-precheck}`
  (the binding-constraints sentence "binding implementation constraints
  from the anchor are copied into the plan and the anchor is listed as a
  `[Must]` reference before plan population or implementer dispatch",
  which gains the lead-written stub's `## Codebase Findings` as the plan
  section the copy lands in; if
  `260908-bug-shipped-prep-guardrail-hardcodes-devenv-migration-anchor`
  rewords that sentence first, the stub clause follows the reworded
  text). Both
  workflow-skills anchors are rewritten by `260908` Phase 1; this ticket
  edits the post-`260908` text. `{#260505-proceed-routing-pipeline}`'s
  "decide delegated plan depth" clause states what `lead-proceed` does not
  do and stays true. Existing anchors only, no new stem, no heading change.
- Mental-model addressing: `ai-docs/mental-model/workflow-skills.md`, the
  line "`lead-implement` defaults delegated preparation to
  `plan-populator-survey`" gains the four-fact skip as the exception; no
  other mental-model line is falsified.
- Review allocation, branch plan, and doc mode derivation are untouched.

## Phases

### Phase 1: Four-fact skip condition, delegated stub plan, and spec

Depends on both phases of `260908` having landed. Change
`deriveImplementPlanDepth` (adding the target-kind parameter) so a
delegated ticket target returns `none` under the four-fact condition and
`survey` otherwise, and delete the dead branch.
Add the delegated `none` wording to `implementPrepInstruction` (allocate
the plan path, write the stub with the sections Decision 2 lists including
the `## Codebase Findings` carry-over of binding constraints from the Prep
reads, no planner dispatch) and make the delegated
`default` branch of `implementEditInstruction` render `implementer` with
`PlanPath`. Update the four spec sentences named in Constraints. Add a drift pin: a
Go test asserting the stub's section headings equal the heading set of the
`plan-populator-survey` template variants (read through the rsrc loader),
so a later planner-template rename fails here rather than shipping a
stale stub. Tests: a resolver test that the four strongest
values yield `plan_depth: none` for a delegated ticket target and that
weakening any one of them (including `unknown`) yields `survey`; a todo
derivation test for the delegated `none` Prep and Edit instructions; the
existing survey-default test keeps passing with facts left `unknown`.
Verification: `go test ./...` in `agents-plugin-tool/` with `-count=1`,
and one dogfood run on a ticket whose phase already enumerates its edit
list, confirming the verdict reads `Plan Depth: none`, the stub plan is
written, the implementer is dispatched without a planner hop, and the
implementer proceeds from the ticket on the "Skipped:" route sections
rather than bouncing the plan back to the lead as defective (the stub's
own sentence is the only text telling it so; if the dogfood run bounces,
the implementer playbook gains one sentence and the no-playbook-change
constraint is amended by Edition).

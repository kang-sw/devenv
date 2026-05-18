---
title: Workflow phase-unit proceed and contract skeleton cleanup
related:
  260517-bug-lead-proceed-overbroad-slice: dogfood evidence that autonomous broad slice selection can hide distinct implementation blast radii
  260517-bug-ws-agent-empty-result-after-tool-use: adjacent runtime lifecycle bug; do not solve with workflow timeout policy in this ticket
spec:
  - 260505-planning-workflow-skills
  - 260505-proceed-routing-pipeline
  - 260510-skeleton-contract-populator-flow
  - 260505-implementation-workflow-skills
related-mental-model:
  - workflow-skills
  - documentation-system
  - named-agent-runtime
---

# Workflow phase-unit proceed and contract skeleton cleanup

## Background

Recent dashboard dogfood exposed repeated workflow drift: `lead-proceed`
selected an overbroad implementation slice, `lead-write-ticket` currently
encourages phase granularity that would become too small under single-phase
proceed routing, `lead-write-skeleton` currently encourages compile-clean
population before real implementation, and discussion around named-agent hangs
risked turning runtime bugs into workflow policy.

This ticket captures the settled terminology and implementation direction:

- use `non-working contract skeleton`, not "brain sketch";
- use `single-phase proceed policy` and
  `1 proceed = 1 ticket phase implementation unit`;
- define ticket phases as complete implementation units, not granular task
  checklist items;
- strengthen the existing plan-populator prompts as the pre-implementation
  context and planning pass; do not rename prompts unless there is a separate
  migration reason;
- prefer `ad hoc implementation shortcut risk`, `mock-data wiring`, and
  `fallback or temporary implementation logic` over informal "duct tape"
  wording in durable docs;
- call the agent runtime issue `named-agent hang and empty-result lifecycle`.

## Decisions

- Do not rename `lead-write-skeleton`; redefine its contract.
- Do not use `multi-phase` or `phase-wise implementation` terminology for the
  `lead-proceed` change.
- Author actionable child-ticket phases from a fresh-session view: each phase
  should be the next complete behavior a future `lead-proceed` run can finish.
- Do not make documentation checkpoint cadence changes in this ticket. The
  current doc pipeline is noisy, but it also prevents drift. Keep cadence as a
  separate policy discussion.
- Do not add strong workflow timeout or takeover rules for named-agent hangs in
  this ticket. Treat stale or empty agent results as runtime lifecycle bugs
  first.

## Phases

### Phase 1: Ticket phase units and single-phase proceed policy

Change `lead-write-ticket` and `lead-proceed` so ticket phases are complete
implementation units, and one proceed invocation selects exactly one ticket
phase as the implementation unit when the target has phases.

Requirements:

- Define a ticket phase as one complete implementation unit for one
  `lead-proceed` run: after the phase, the targeted caller-visible behavior is
  working, reviewable, and verifiable without temporary behavior.
- Define `1 proceed = 1 ticket phase implementation unit`.
- Update `lead-write-ticket` phase authoring so it asks what complete behavior
  the next fresh session should finish, not how to serialize internal tasks.
- In `lead-write-ticket`, use multiple phases only for sequential complete
  increments with distinct success criteria; split unrelated complete
  increments into child tickets.
- In `lead-write-ticket`, treat setup, API, UI, tests, skeletons, and
  investigation as phase ingredients by default; elevate one to a phase only
  when it leaves a reviewable deliverable that a fresh session can complete and
  hand off cleanly.
- Require each non-epic actionable phase to state its completion boundary: what
  behavior is done after the phase, what remains deferred, and what
  verification proves the phase complete.
- Honor an explicit user-named phase exactly.
- Without an explicit phase, select the first unfinished phase by default.
- Do not autonomously group adjacent unfinished phases as the broadest cohesive
  slice.
- If a phase is too large or crosses unrelated implementation surfaces, stop for
  conservative phase or ticket slicing instead of splitting inside `proceed`.
- Preserve whole-target handling only for targets without phase sections and
  for narrow inline work that does not need durable phase tracking.
- Update ws and wsflow skill surfaces consistently where applicable.
- Update ticket conventions, spec, and mental-model text that currently
  describe "broadest cohesive unfinished slice" selection or granular phase
  preference.

### Result (a0df5510) - 2026-05-18

Implemented phase-unit ticket authoring and proceed routing for ws and wsflow.
`lead-write-ticket` now frames non-epic actionable phases as complete
fresh-session implementation units with completion, deferred-scope, and
verification boundaries. Runtime ticket conventions use the same phase-unit
definition.

`lead-proceed` now selects one explicitly named phase or, by default, the first
unfinished phase. It stops when one proceed request names multiple phases or
when the selected phase is too broad or crosses unrelated implementation
surfaces, instead of grouping adjacent unfinished phases.

The `workflow-skills` spec now records the implemented phase-unit behavior and
removes the Phase 1 planned callouts. The workflow-skills and
documentation-system mental models now describe one selected phase as the hard
downstream scope.

Verification:

- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `go test ./...` from `agents-plugin-tool/`
- `ws/spec_index.verify`

#### Edition (09399724) - 2026-05-18

Collapsed `lead-proceed` scope selection from a separate
`judge: implementation-slice` section into a mechanical ready-ticket scope
resolution step. Ready tickets now continue to implementation with resolved
scope only: whole target for tickets without phases, one explicitly named
phase, or the first unfinished phase by default. Multiple explicit phases and
plainly too-broad phase text stop for phase or ticket slicing.

The workflow spec and workflow-skills mental model now describe scope
resolution instead of slice selection. Verification repeated:

- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `go test ./...` from `agents-plugin-tool/`
- `ws/spec_index.verify`

#### Edition (18450413) - 2026-05-18

Restructured `lead-write-ticket` and its wsflow mirror after the phase-unit
policy landed. The edit preserves ticket creation, edit, ready-gate, commit,
and handoff semantics while splitting the long invoke flow into short handlers:
create ticket, edit ticket, apply ticket content, intent review, spec-stem
check, and output handoff.

Verification repeated:

- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `git diff --check -- agents-plugin/skills/lead-write-ticket/SKILL.md agents-plugin-wsflow/skills/lead-write-ticket/SKILL.md`

#### Edition (bbe8a802) - 2026-05-18

Tightened the `lead-write-ticket` judgment section and wsflow mirror after the
handler restructure. Prose-shaped judgments now use labeled decision lines for
ready spec gates, initial status, cascade edits, ticket size, phase need, and
missing spec coverage, while preserving the same triggers and stop conditions.

Verification repeated:

- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `git diff --check -- agents-plugin/skills/lead-write-ticket/SKILL.md agents-plugin-wsflow/skills/lead-write-ticket/SKILL.md`

### Phase 2: Contract brief instead of skeleton artifacts

Remove skeleton artifact generation from the normal implementation workflow.
Keep `lead-write-skeleton` itself untouched for backward compatibility, but stop
routing new implementation work through it. Absorb the useful skeleton role into
`lead-write-code` brief authoring: the brief must state concrete contract and
integration-test instructions before implementers write code.

Requirements:

- Do not edit `agents-plugin/skills/lead-write-skeleton/SKILL.md` in this
  phase.
- Remove normal-route references from other workflow skills that invoke or
  require `lead-write-skeleton`.
- Keep `skeletons:` ticket frontmatter backward-compatible as a legacy artifact
  map; absence must not imply that a missing skeleton should be created.
- Change `lead-implement` so public interface, cross-module boundary, and new
  type contract work routes to `lead-write-code` with contract-brief
  expectations, not to `lead-write-skeleton`.
- Keep `lead-edit` narrow: direct lead edits honor caller scope and local
  verification only; remove skeleton or plan artifact interpretation from
  `lead-edit` and its wsflow mirror.
- Extend `lead-write-code` brief authoring so it absorbs the useful skeleton
  role with concrete `Contract Instructions` and `Integration Test
  Instructions`.
- `Contract Instructions` must name expected files/modules, public
  types/functions/handlers/tools, visibility, call shape, input/output shape,
  lifecycle boundaries, existing mechanisms to reuse, and forbidden temporary,
  fallback, or mock-data wiring.
- `Integration Test Instructions` must name the required test boundary type
  such as parser, CLI, MCP tool, doc convention, skill routing, runtime
  lifecycle, or agent relay; state whether to extend existing tests or create
  new integration tests.
- Implementer prompts must treat brief contract and integration-test
  instructions as acceptance criteria.
- Fit and test reviewers must compare the implementation against the brief's
  contract and integration-test instructions.
- Update specs and mental models so `lead-write-skeleton` is deprecated from
  normal workflow routing without deleting the skill or bundled legacy prompt.

### Result (8306418a) - 2026-05-18

Deprecated skeleton artifact generation from the normal implementation workflow
without editing `lead-write-skeleton` itself. `lead-implement` no longer checks
for skeleton need or invokes `lead-write-skeleton`; public interface,
cross-module boundary, and new type contract work routes to `lead-write-code`
so the implementation brief can carry contract and integration-test detail.

`lead-write-code` briefs now include concrete `Contract Instructions` and
`Integration Test Instructions`. Implementers treat those sections as
acceptance criteria, fit reviewers compare implementation against contract
instructions, and test reviewers check that required integration tests exist
and prove the specified boundary.

`lead-edit` and its wsflow mirror no longer interpret skeleton artifacts or
plan-style briefs; they remain narrow direct edit primitives bound by caller
scope and local verification. Ticket `skeletons:` frontmatter is now documented
as a legacy artifact map whose absence does not trigger skeleton creation.

Verification:

- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `go test ./...` from `agents-plugin-tool/`
- `ws/spec_index.verify`
- `git diff --check`

#### Edition (4bf38f10) - 2026-05-18

Streamlined `lead-write-code` within the existing single skill file after
contract and integration-test instructions moved into the brief. Review
orchestration now uses short handlers for allocation, reviewer spawning, and
relay control. Reviewer calls share one prompt frame plus a partition table,
and plan/review prompts moved into templates. `judge: plan-depth` and
`judge: partition-allocation` now use labeled decision lines.

Verification repeated:

- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `git diff --check -- agents-plugin/skills/lead-write-code/SKILL.md`

#### Edition (854111c2) - 2026-05-18

Applied the skill-authoring Markdown hierarchy guidance to `lead-write-code`.
The invariant list now uses Branch, Context, Brief, Review, Agents, and Output
groups, and templates are grouped under Brief, Plan, Review, and Report
sections. Behavior remains unchanged from the prior structure cleanup.

Verification repeated:

- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `git diff --check -- agents-plugin/skills/lead-write-code/SKILL.md`

### Phase 3: Plan-populator guardrails

Strengthen implementation routing so the existing plan-populator prompts catch
contract, reuse, and shortcut risks before code is written. This is not a new
workflow route or prompt rename; it is a guardrail pass inside the existing
`survey` and `research` plan-depth paths.

Requirements:

- Keep existing prompt names unless there is a separate migration reason.
- Treat `plan-populator-survey` and `plan-populator-research` as the two
  implementation surfaces for this phase.
- Describe their behavior as a pre-implementation context and planning pass, not
  as a new route.
- Ensure both prompts look for public contract violations, missed reuse of
  existing project mechanisms, ad hoc implementation shortcut risk, mock-data
  wiring, and fallback or temporary implementation logic.
- Treat duplicated glue code, test-passing code that bypasses existing project
  mechanisms, and fallback-based behavior as examples of ad hoc implementation
  shortcut risk.
- Ensure selected-scope binding decisions remain visible to implementers and fit
  reviewers.
- For `plan-populator-survey`, surface the risks as reference-map entries,
  constraints, and opinion notes that help implementers avoid exploratory
  misses without turning the survey into an implementation plan.
- For `plan-populator-research`, reflect the risks in the self-contained plan:
  context, ordered steps, testing, and success criteria must preserve contract
  and integration-test instructions from the brief.
- Remove or reframe skeleton-era plan-populator wording around the brief's
  `Contract Instructions` and `Integration Test Instructions`; keep
  backward-compatible artifact interpretation only when an older brief actually
  provides such artifacts.

### Phase 4: Documentation cadence and runtime lifecycle boundaries

Record boundaries without changing the behavior yet.

Requirements:

- Leave documentation checkpoint cadence as discussion-needed policy work.
- Preserve current Result/Edition and post-implementation doc pipeline behavior
  unless a later ticket explicitly changes it.
- Keep `260517-bug-ws-agent-empty-result-after-tool-use` focused on the
  named-agent hang and empty-result lifecycle.
- Do not mask named-agent runtime bugs with broad workflow takeover rules before
  the runtime lifecycle is understood.

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
- use `pre-implementation survey pass` as the descriptive term while keeping
  existing prompt names such as `project-survey` and `plan-populator-survey`;
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

### Phase 2: Non-working contract skeleton

Redefine `lead-write-skeleton` as a non-working contract skeleton step instead
of a populated, compile-clean stub flow.

Requirements:

- Keep the skill name `lead-write-skeleton`.
- Allow non-compiling source when it clearly records contracts and boundaries.
- Allow public contracts, module/file boundaries, type or function signatures,
  stubs, and intent comments.
- Forbid behavior implementation, mock-data wiring, fallback or temporary
  implementation logic, visual polish or presentational completion, and
  temporarily working feature code.
- Remove or replace the `skeleton-populator` population requirement from this
  skill's normal flow.
- Remove compile-clean/build-valid stub requirements from the skeleton contract.
- Clarify that implementers use the optional plan plus skeleton diff as design
  input, then replace or complete the skeleton with real working behavior.
- Revisit ticket `skeletons:` frontmatter semantics if the recorded artifact is
  no longer a final populated skeleton hash.

### Phase 3: Pre-implementation survey pass guardrails

Strengthen implementation routing so the survey step catches contract and reuse
risks before code is written.

Requirements:

- Keep existing prompt names unless there is a separate migration reason.
- Describe the behavior as a `pre-implementation survey pass`.
- Ensure the pass looks for public contract violations, missed reuse of existing
  project mechanisms, ad hoc implementation shortcut risk, mock-data wiring, and
  fallback or temporary implementation logic.
- Treat duplicated glue code, test-passing code that bypasses existing project
  mechanisms, and fallback-based behavior as examples of ad hoc implementation
  shortcut risk.
- Ensure selected-slice binding decisions remain visible to implementers and fit
  reviewers.
- Prefer reusing existing `project-survey` and `plan-populator-survey`
  mechanisms before adding a new prompt.

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

---
title: Lead implement single-plan delegation
sage-review: recommended
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260627-feat-enter-implement-deterministic-verdict-engine: predecessor that moved deterministic implement verdict and todo guidance into MCP
  260627-feat-lead-implement-rendered-delegate-prompts: predecessor that made implementer and review relay prompts file-first
related-mental-model:
  - mcp-runtime
  - prompt-bundle
  - workflow-skills
---

# Lead implement single-plan delegation

## Background

The delegated `lead-implement` path still carries two artifact concepts:
a lead-authored brief, then an optional survey or research plan populated from
that brief. Dogfooding showed that this keeps too much responsibility on the
lead. In delegated work, the lead can drift into source exploration while trying
to write a useful brief, which pollutes lead context and duplicates the
planner's job.

The settled design is to keep the existing delegation route shape and review/fix
loop, but collapse the artifact contract into one generated implementation plan.
The ticket and selected phase remain the authoritative contract. A planner reads
that ticket/phase, clips the relevant contract, explores the codebase, and writes
or refines the plan. The executor then reads only that plan. Reviewers review the
ticket, plan, and diff.

This is a convention-centered refactor of an already working route, not a new
delegation architecture. The route still supports a medium/light survey planner
and escalation to a large research planner; the difference is that both write to
the same plan file instead of chaining brief plus survey/research artifacts.

## Decisions

- Delegated `lead-implement` should remove the lead-authored brief artifact.
- The lead may read the ticket and selected phase in delegated mode, but should
  not directly explore source, specs, mental models, or implementation files
  before planner dispatch unless another workflow gate explicitly requires it.
- A generated plan path is created before planner dispatch. The intended MCP
  surface is `ws.path.generate(kind: "plan", stems: ["<ticket-stem-or-task>"])`.
- Plan files live under `ai-docs/.plans/YYYY-MM/DD-hhmm-<stem>.md`.
- The survey planner is the delegated default. It runs at the existing medium
  tier, reads the ticket and selected phase, clips relevant contract text,
  explores the codebase, writes a light implementation plan, and reports
  confidence.
- Survey escalation is preserved. When confidence is low or strategy requires
  deeper judgment, the survey planner returns an escalation signal and reason.
- The research planner runs at the existing large tier and refines or replaces
  the same plan file. Do not introduce xlarge as a default in this ticket; leave
  xlarge tier policy as a future tuning decision if needed.
- The executor reads the plan path only. It should not read the ticket directly
  unless the plan explicitly authorizes that as an escalation path.
- Reviewers review against the ticket, the plan, and the diff. The existing
  partitioned review and fix loop stay in place.
- Final merge ceremony should avoid a mandatory lead diff reread after clean
  reviewer passes. The lead still aggregates review disposition, verification,
  ticket/doc status, and user approval before merge.

## Plan Contract

The plan file is the single delegated implementation contract. It must be
self-contained enough for a fresh executor to act without re-researching, while
making clear that the ticket remains the source of truth.

Required plan sections:

- `Relevant Ticket Contract` - clipped ticket and phase requirements that govern
  the implementation.
- `Out of Scope` - ticket content or nearby concerns intentionally excluded from
  this phase.
- `Codebase Findings` - concrete files, APIs, tests, conventions, and reusable
  mechanisms discovered by planner exploration.
- `Implementation Plan` - ordered steps with expected files or components.
- `Verification Plan` - commands, tests, and manual checks the executor should
  run.
- `Escalations` - unresolved blockers, low-confidence areas, or explicit
  permissions needed before execution.

The planner may summarize or quote ticket contract only to preserve scope. It
must not create new policy decisions that are absent from the ticket. When the
ticket is insufficient, contradictory, or underspecified for implementation, the
planner should escalate instead of silently filling in missing contract.

## Spec Impact

Target spec areas:

- `mcp-tools`: update `ws.enter.implement` todo/verdict contract to remove
  `plan_depth=brief` as a reachable delegated artifact, describe single-plan
  delegated routing, and add `path.generate(kind: "plan")`.
- `workflow-skills`: update `lead-implement` workflow contract so delegated work
  is planner-to-executor over one plan file, while direct edit remains
  lead-owned implementation from the ticket.

Expected caller-visible change:

- Delegated implementation no longer asks the lead to write a separate brief.
  The visible workflow creates a plan path, dispatches a planner, optionally
  escalates to research on the same path, dispatches the executor with that plan,
  then runs the existing review/fix loop.

Contract-first spec: yes.

## Phases

### Phase 1: Generate plan artifact paths

Add `ws.path.generate(kind: "plan")` support. Plan paths must be repo-local under
`ai-docs/.plans/YYYY-MM/DD-hhmm-<stem>.md`, with the existing path generation
semantics for collision avoidance and safe stem normalization. Keep existing
`review` and `prompt` path behavior unchanged.

Verification boundary:

- Unit tests cover `kind="plan"` path shape, stem normalization, uniqueness, and
  unchanged behavior for existing kinds.
- The MCP schema and docs mention `plan` as an accepted path kind.

### Phase 2: Rework planner playbooks around ticket-to-plan

Update `plan-populator-survey` and `plan-populator-research` so their render
context is ticket path, selected phase, and plan path. Remove the brief-path
dependency from the planner contract.

Survey should write the light plan format and return `[ok]` or
`[escalate-to-research]` with confidence and escalation rationale. Research
should read any existing survey output at the same plan path, then refine or
replace it with a deeper implementation plan.

Verification boundary:

- Playbook render tests cover the new context variables and absence of required
  `brief_path`.
- Prompt text preserves existing tier frontmatter: survey remains medium,
  research remains large.
- Tests or golden checks cover the required plan sections and escalation output.

### Phase 3: Route lead-implement delegated prep through single-plan flow

Update `lead-implement` and related rendered implementer/review relay prompts so
delegated work follows:

1. lead reads ticket and selected phase;
2. lead generates a plan path;
3. planner writes or refines the plan;
4. executor reads the plan only;
5. reviewers review ticket, plan, and diff;
6. the existing review/fix loop continues.

Direct edit mode remains lead-owned: the lead reads the ticket and implements
without generating a planner artifact unless the implementation route escalates
to delegated planning.

Verification boundary:

- Lead implement golden tests no longer require a lead-authored brief for
  delegated mode.
- Implementer prompt tests assert plan-only execution unless an explicit
  escalation authorizes reading the ticket.
- Reviewer prompt tests assert review against ticket, plan, and diff.

### Phase 4: Update enter.implement resolver and todo instructions

Update `ws.enter.implement` resolver labels and derived todo instructions so
delegated mode no longer exposes `brief` as a plan depth. The default delegated
prep should be the survey planner writing a light plan, with research escalation
when the survey planner reports low confidence or strategic uncertainty.

The raw verdict and todo instructions should tell the lead the next concrete MCP
or playbook action without requiring the LLM to re-solve the old brief/survey
decision tree.

Verification boundary:

- Resolver tests cover delegated default, survey escalation language, direct edit
  behavior, and branch-stop behavior without unreachable planner instructions.
- Todo rendering tests show plan-generation and planner/executor steps in the
  reachable delegated path.
- Existing review/fix loop tests continue to pass.

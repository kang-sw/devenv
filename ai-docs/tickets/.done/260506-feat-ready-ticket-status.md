---
title: Ready ticket status and backlog split
related-mental-model:
  - documentation-system
  - workflow-skills
  - git-workflow-tools
spec:
  - 260505-ticket-document-system
  - 260505-documentation-authoring-workflows
  - 260505-ticket-discovery-tools
  - 260505-planning-workflow-skills
  - 260505-proceed-routing-pipeline
completed: 2026-05-06
---

# Ready Ticket Status And Backlog Split

## Background

The current ticket lifecycle uses `idea/`, `todo/`, `.done/`, and `.dropped/`.
`todo/` carries two meanings at once:

- accepted backlog work that is worth preserving as a ticket;
- spec-gated work that can enter the implementation pipeline.

This makes `idea/` absorb too much backlog. Promoting `idea/` to `todo/` is
premature whenever the work is clearly worth doing but not yet ready for spec
authoring, so genuine rough ideas are buried among accepted-but-not-ready work.

Introduce `ready/` as the implementation queue and move the spec gate from
`idea/` -> `todo/` to `todo/` -> `ready/`.

## Decisions

- `idea/` means rough capture before triage.
- `todo/` means accepted backlog with ticket-level intent, not implementation
  readiness.
- `ready/` means spec-gated implementation queue.
- `.done/` and `.dropped/` remain archives.
- `ready/` only requires spec gate completion. Plans and skeletons remain
  downstream `lead-proceed` decisions.
- `todo/` tickets may carry `spec:` frontmatter when known in advance. The link
  is a recoverability hint and ready-promotion candidate, not proof of
  implementation readiness.
- `ready/` tickets without spec linkage are invalid for non-`epic`,
  non-`research` work.
- `## Ticket Queue` should list `ready/` work only. `todo/` is backlog, not the
  immediate queue.
- `tickets.list`, `tickets.find`, `tickets.status`, and `project_tree` should
  expose `idea/`, `todo/`, and `ready/` as active discovery statuses.
- `lead-proceed` given a `todo/` ticket should route through ready promotion
  rather than implementing immediately.
- Avoid phrasing such as `todo-or-higher`; name the exact status semantics
  instead.

## Phases

### Phase 1: Lifecycle Spec And Conventions

Update the documentation contract for ticket lifecycle semantics.

Required outcomes:

- Ticket conventions describe `idea/` -> `todo/` -> `ready/` -> `.done/` or
  `.dropped/`.
- Spec conventions require a `ready/` ticket for `🚧` entries, except where the
  existing `epic` and `research` exemptions apply.
- Documentation-system and workflow-skills specs describe the split between
  accepted backlog and implementation queue.
- Mental models capture the new modification hazards for ticket status parsing,
  queue rendering, and workflow gates.
- Existing ambiguous phrases such as `todo-or-higher` are replaced with explicit
  status-specific wording.

### Result (6e20de3) - 2026-05-06

Implemented the lifecycle contract across bundled and compatibility convention
docs. Specs now describe `todo/` as accepted backlog and `ready/` as the
spec-gated implementation queue, and mental models capture status parsing,
queue rendering, and workflow gate hazards.

### Phase 2: Bootstrap Migration Rule

Update the managed `AGENTS.template.md` migration guide so downstream projects
can migrate incrementally after the existing `wip/` removal rule.

Required outcomes:

- Add a new template version after v0034.
- Update the initial directory sketch to include
  `idea/ todo/ ready/ .done/ .dropped/`.
- Add an idempotent migration rule that:
  - creates `ai-docs/tickets/ready/` if absent;
  - moves existing non-`epic`, non-`research` implementation-ready tickets from
    `todo/` to `ready/` with `git mv`;
  - keeps `epic`, `research`, missing-spec, and uncertain tickets in `todo/`;
  - creates an empty `todo/` directory when all old todo tickets move;
  - treats `ready/` as the implementation queue;
  - instructs users to promote scoped `idea/` tickets to `todo/` through
    `ws:lead-discuss`.

### Result (6e20de3) - 2026-05-06

Added bootstrap migration v0035 to the Codex AGENTS template and Claude
compatibility template. The migration creates `ready/`, moves spec-linked
implementation-ready tickets from `todo/`, keeps uncertain backlog in `todo/`,
and treats `ready/` as the `## Ticket Queue` source.

### Phase 3: Discovery And Git Plumbing

Teach the runtime documentation and Git helpers about the new active status.

Required outcomes:

- Ticket status normalization and ranking include `ready/`.
- Default active ticket scans include `idea/`, `todo/`, and `ready/`.
- `project_tree` renders `ready/` tickets distinctly enough that the
  implementation queue is visible.
- `tickets.list`, `tickets.find`, and `tickets.status` accept and report
  `ready/`.
- `git.commit` ticket move expansion recognizes `ready/` paths.
- Tests cover discovery, status filters, project-tree rendering, and ticket move
  expansion.

### Result (6e20de3) - 2026-05-06

Added `ready` to ticket discovery normalization, default active scans, MCP/CLI
schema text, project-tree rendering, and Git ticket move expansion. Tests now
cover explicit `ready` filters, project-tree ordering, and `todo/` -> `ready/`
move staging.

### Phase 4: Workflow Skill Routing

Update workflow skills and prompts so promotion into implementation readiness is
part of the normal flow.

Required outcomes:

- `lead-write-ticket` creates accepted actionable backlog as `todo/` without
  requiring immediate spec linkage.
- `lead-write-ticket` applies the spec gate when creating or moving a
  non-`epic`, non-`research` ticket into `ready/`.
- `lead-discuss` distinguishes `idea/` -> `todo/` triage from `todo/` ->
  `ready/` spec-gated promotion.
- `lead-proceed` routes `todo/` tickets through ready promotion before
  implementation and only treats `ready/` tickets as direct implementation
  targets.
- `lead-write-spec`, reconstruction flows, survey prompts, and compatibility
  guidance stop treating `todo/` as the implementation-bearing status.

### Result (6e20de3) - 2026-05-06

Updated Codex and Claude workflow skills/prompts so `todo/` is backlog and
`ready/` is the implementation target. A review fix tightened `lead-proceed`
and Claude `/proceed` so inline-created or existing `todo/` tickets route
through ready promotion before skeleton or implementation stages.

### Phase 5: Repository Self-Migration

Migrate this repository after the new rules and tooling exist.

Required outcomes:

- Add `ai-docs/tickets/ready/`.
- Move only clearly implementation-ready current `todo/` tickets to `ready/`.
- Keep `epic`, `research`, and ambiguous backlog tickets in `todo/`.
- Refresh `_index.md` so `## Ticket Queue` lists `ready/` work only while active
  ticket discovery still mentions `idea/`, `todo/`, and `ready/`.
- Leave completion and drop flows flexible; do not add a hard prohibition on
  direct `todo/` -> `.done/` when a future exceptional workflow justifies it.

### Result (6e20de3) - 2026-05-06

Created the live `ready/` directory and moved current spec-linked non-epic,
non-research implementation work there. Kept the workflow stability epic in
`todo/`, preserved rough research/default-root items in `idea/`, and refreshed
`_index.md` so the active table and queue follow the new lifecycle.

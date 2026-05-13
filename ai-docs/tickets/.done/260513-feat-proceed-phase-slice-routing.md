---
title: Proceed phase-slice routing
spec:
  - 260505-proceed-routing-pipeline
  - 260505-implementation-workflow-skills
  - 260505-ticket-document-system
related-mental-model:
  - workflow-skills
  - documentation-system
completed: 2026-05-13
---

# Proceed phase-slice routing

## Background

`lead-proceed` currently routes an existing non-epic `ready/` ticket directly to
implementation. In practice this can make the ticket feel like the implementation
unit even when the ticket contains multiple unfinished phases. The workflow
should keep `write-ticket` responsible for ticket structure while making
`proceed` responsible for selecting the current implementation slice.

Users may also pass an existing `todo/` ticket to `proceed`. That should be
treated as implementation intent: promotion to `ready/` should run
autonomously through the normal ticket gate, and discussion should be used only
when promotion exposes unresolved design choices that cannot be settled from the
ticket and specs.

## Decisions

- `proceed` does not rejudge ticket quality, demand ticket splitting, or mutate
  ticket structure.
- A non-epic `ready/` ticket remains an implementation target, but the default
  implementation slice is the first unfinished phase, not the whole ticket.
- Multiple phases may be included only when the user explicitly asks for them or
  the phases cannot be verified separately.
- A `todo/` ticket path passed to `proceed` is implementation intent. Promotion
  to `ready/` should run autonomously before slice selection.
- Escalation to `lead-discuss` is narrow: unresolved design decisions, unclear
  completion criteria, failed spec coverage creation, user trade-offs, and
  non-implementation targets such as epic, research, or exploratory work.

## Constraints

- Preserve `write-ticket` as the owner of ticket decomposition and phase
  authoring rules.
- Preserve `lead-proceed` as route-only: it reads workflow artifacts, chooses
  readiness stages and the implementation slice, then routes to
  `lead-implement`.
- The selected slice must be visible in the proceed announcement and passed to
  downstream implementation as a hard scope boundary.
- Results should be recorded only for the implemented phase or phases; unfinished
  phases remain open.

## Phases

### Phase 1: Add phase-slice proceed routing

Update the workflow skill contracts so `lead-proceed` autonomously promotes
`todo/` tickets to `ready/` when possible, selects the current implementation
slice only after a non-epic `ready/` target is available, and passes that slice
to `lead-implement` as a hard scope boundary.

Acceptance criteria:

- Existing `todo/` ticket input can flow `todo -> ready -> implement` without a
  discussion stop when the ticket has no unresolved design decision.
- Existing `ready/` ticket input defaults to the first unfinished phase.
- The proceed announcement reports the selected slice.
- `lead-implement` and downstream edit/write-code guidance treat the selected
  slice as hard scope.
- Documentation and mental-model entries distinguish ticket decomposition from
  proceed's execution-slice selection.

### Result (50971df) - 2026-05-13

Implemented in workflow skill contracts and docs. `lead-proceed` now treats
actionable `todo/` ticket paths as implementation intent, promotes them through
`lead-write-ticket` when only ready-gate normalization is needed, escalates only
for unresolved design blockers, selects an implementation slice after a non-epic
`ready/` target exists, and passes that slice to `lead-implement` as hard scope.
`lead-implement`, `lead-edit`, and `lead-write-code` now preserve
caller-provided scope boundaries. Specs and mental models distinguish ticket
decomposition from proceed's execution-slice selection.

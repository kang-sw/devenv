---
title: lead-sprint episode workflow shell
spec:
  - 260523-sprint-episode-workflow-shell
related:
  260521-refactor-wsflow-lead-implement-mirroring-gap: adjacent wsflow divergence discovered during the same lead skill cascade
related-mental-model:
  - workflow-skills
completed: 2026-05-23
---

# lead-sprint episode workflow shell

## Background

The current full ws `lead-sprint` still carries the old `sprint/` branch and
deferred wrap-up model, but recent workflow changes removed full ws
`lead-edit` and `lead-write-code` and made `lead-implement` the unified
implementation spine. Routing sprint tasks into `lead-implement` conflicts with
both sides: `lead-implement` stops or redirects on `sprint/` branches, while
`lead-sprint` is supposed to suppress per-task documentation updates.

The settled direction is to redefine `lead-sprint` from scratch as an
episode-oriented workflow shell. Sprint should coordinate a conversation session
with lightweight edit episodes and normal workflow handoff; it should no longer
be a feature-branch container with a final wrap-up pass.

## Decisions

- `lead-sprint` owns episode routing and episode closure, not general
  implementation.
- `lead-sprint` must not create or require `sprint/` branches.
- `lead-sprint` must not run one final wrap-up over an accumulated branch range.
- `sprint-edit` is a narrow lead-owned interactive edit lane for small changes
  in one current edit context.
- Larger implementation work routes through `ws:lead-proceed` when traceability
  or workflow readiness is needed, or through `ws:lead-implement` only when a
  ticketless inline target is already narrow, routine, and fully scoped.
- After each `sprint-edit` commit, the skill asks in the user's active language:
  `[sprint] Should we keep refining <current edit context>, wrap it up here, or shift direction?`
- The question must be interpreted as: keep refining keeps the episode open;
  wrap it up finishes the episode and runs the documentation pass for its
  marked commits; shift direction decides whether to finish the current episode
  before starting a different one.

## Phases

### Phase 1: Redefine sprint around edit episodes

Replace full ws `lead-sprint` behavior with an episode-oriented workflow shell.

Required behavior:
- On invoke, load workflow primitives, project map, and git status; do not create
  a branch automatically.
- Recover or continue any open sprint-edit episode from active conversation or
  commit markers when possible.
- Route questions, design discussion, and exploration without forcing spec or
  ticket authoring.
- Add `judge: sprint-edit` with strict boundaries: small interactive edits only,
  one current edit context, lead-owned direct editing, no public contract,
  routing semantics, protocol, ticket phase completion, cross-module new
  pattern, plan, review allocation, or branch decision.
- Route work outside `sprint-edit` to the normal workflow instead of imitating
  `lead-implement` inside sprint.
- Commit sprint-edit source changes with a recoverable marker in `## AI Context`
  such as `Sprint-Edit: <episode-slug>` and `Sprint-Edit-Context: <one-line context>`.
- After each sprint-edit commit, ask the active-language wrap-up question from
  `## Decisions`.
- On wrap-up for an episode, collect that episode's marked commits, run the
  spec and mental-model documentation pass for that range, commit docs, and
  return to the sprint session loop.
- On direction shift, decide whether the current episode wraps up before opening
  a new edit context.
- Replace stale branch/wrap-up and `edit/write-code` wording in specs, mental
  models, index canonical flow text, and wsflow surfaces affected by this
  behavioral change.

Deferred scope:
- Do not make `lead-implement` sprint-aware.
- Do not create a new full implementation subsystem inside `lead-sprint`.
- Do not redesign `lead-proceed` or `lead-implement` beyond references needed
  for the sprint route.
- Do not resolve the broader wsflow `lead-implement` mirroring gap beyond keeping
  wsflow `lead-sprint` behavior explicitly non-stale.

Verification:
- Skill authoring invariants remain satisfied for every changed skill
  invariant or constraint.
- No stale full ws `lead-sprint` references to mandatory `sprint/` branches,
  final wrap-up, `lead-edit`, or `lead-write-code` remain outside intentional
  historical/reference contexts.
- The workflow-skills spec and mental model describe episode-oriented sprint
  behavior and the updated handoff boundaries.
- wsflow mirroring guidance and tests still pass or any intentional divergence is
  documented.
- Relevant skill-bundle tests pass.

### Result (8e7f40a) - 2026-05-23

Implemented the episode-oriented sprint shell for full ws and mirrored the
session shell in wsflow. `lead-sprint` now stays on the current branch, routes
discussion and exploration inline, gates small direct changes through
`sprint-edit`, marks sprint-edit commits, recovers open episodes from active
conversation or commit markers, asks whether to keep refining, wrap up, or shift
direction, and runs episode-scoped documentation closure when an episode wraps.

The implementation removed stale full ws sprint-branch handoff text from
`lead-discuss` and `lead-implement`, updated the workflow-skills spec and mental
model, and refreshed the canonical flow index. wsflow preserves its documented
`lead-edit` execution surface only for lead-owned direct sprint-edit changes;
larger wsflow work routes through normal workflow gates.

Verification:
- `ws/spec_index.verify()` passed.
- `python3 -m unittest discover agents-plugin/tests` passed, 9 tests.
- `python3 -m unittest discover agents-plugin-wsflow/tests` passed, 9 tests.
- Fit review passed after adding open sprint-edit episode recovery.
- Fresh-reader audit reported no material execution blockers after fixing
  episode documentation commit ordering.

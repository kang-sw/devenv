---
title: Proceed implementation dispatch precheck
parent: 260513-epic-workflow-question-loop-hygiene
spec:
  - 260519-proceed-implementation-dispatch-precheck
plans:
  phase-1: 2026-05/19-260519-feat-proceed-implementation-dispatch.brief
related-mental-model:
  - workflow-skills
---

# Proceed implementation dispatch precheck

## Background

Dogfooding a ready-ticket implementation showed that `lead-proceed` reliably
chooses the high-level pipeline and slice, but does not settle the internal
implementation dispatch before handing off to `lead-implement`. That leaves room
for the implementation stage to shortcut a caller-visible or cross-module
change into direct editing even when the correct route is delegated
`lead-write-code`.

The desired behavior in full ws is for `lead-proceed` to decide the
implementation dispatch from the same conversation and artifact evidence it
already uses for routing, then carry that dispatch as a hard downstream
constraint. `lead-implement` may escalate a direct-edit dispatch to write-code
if later evidence requires it, but must not downgrade a proceed-selected
write-code dispatch to direct edit.

The wsflow package has a different implementation surface: `lead-write-code` is
excluded and `lead-implement` routes through `lead-edit`. wsflow should therefore
mirror the routing clarity goal, not the full ws dispatch enum.

This work also absorbs the stale standalone skeleton-routing capture. Normal
implementation routing no longer creates generated skeleton artifacts;
contract-heavy work is handled through `lead-write-code` briefs.

## Constraints

- Do not make `lead-proceed` inspect source code, source stubs, tests, broad
  documentation, or implementation plans to choose dispatch.
- Do not make `lead-proceed` invoke `lead-edit` or `lead-write-code` directly;
  it still hands implementation work to `lead-implement`.
- Do not reintroduce a skeleton branch into normal routing. Dispatch is direct
  edit versus write-code; contract-brief depth belongs inside write-code.
- Direct edit is allowed only when all direct-edit predicates are known true
  from ticket or conversation artifacts.
- For wsflow, do not introduce `write-code` wording or a full ws dispatch enum;
  express the precheck as execution path, complexity/risk flag, and branch mode.

## Phases

### Phase 1: Remove stale skeleton routing language

Audit active ws and wsflow skill text for skeleton references that describe
normal implementation routing. Remove or mark legacy-only wording so current
skills align with the spec and mental model: `lead-write-skeleton` remains a
compatibility artifact, but normal proceed/implement/write-ticket routing should
not describe skeleton decisions as live routing branches.

Acceptance criteria:

- `lead-proceed`, `lead-implement`, and `lead-write-ticket` no longer present
  skeleton decisions as part of normal implementation routing.
- wsflow mirrors stay aligned where the affected skill text exists.
- Specs and mental models continue to state that contract checkpoints live in
  `lead-write-code` briefs, not generated skeleton artifacts.

### Result (418d2fe) - 2026-05-19

Removed stale normal-routing skeleton wording from `lead-proceed` and
`lead-write-ticket`. `lead-proceed` now describes itself as route-only without
claiming skeleton authoring or skeleton decisions, and its artifact check no
longer names skeletons as a normal proceed input. `lead-write-ticket` now says
`lead-implement` resolves plan depth and execution mode, rather than skeleton
needs.

`lead-implement` in the source tree already omitted skeleton routing language,
and wsflow proceed/implement/write-ticket skills had no matching stale skeleton
references. The spec and mental model still preserve legacy/deprecated skeleton
artifact behavior and the `lead-write-code` brief-based contract checkpoint.

### Phase 2: Add proceed dispatch precheck

Teach `lead-proceed` to select a conservative implementation dispatch before
the `lead-implement` handoff in full ws.

The full ws proceed announcement should include:

- `Implementation Dispatch`: `direct-edit` or `write-code`.
- `Dispatch Reason`: the predicate that selected the dispatch.
- `Branch Mode`: direct current branch, create `implement/<scope>`, continue
  `implement/*`, or sprint blocked.

Dispatch rules:

- Select direct edit only when every predicate is known true from artifacts:
  single-file scope, internal-only, no caller-visible behavior change, no public
  contract change, no new public symbols, no new tests expected, and no explicit
  delegation request.
- Select write-code when any direct-edit predicate is false or unknown.
- Treat ready tickets, spec-linked changes, MCP/CLI/user-visible output changes,
  cross-skill routing changes, and multi-file or test-bearing work as write-code
  unless the artifacts make the direct-edit predicates unambiguously true.
- Carry the selected dispatch to `lead-implement` as a hard lower bound:
  `lead-implement` may escalate direct-edit to write-code, but must not
  downgrade write-code to direct-edit.

The wsflow proceed announcement should not mention `write-code`. It should
instead include:

- `Execution Path`: `wsflow:lead-implement -> wsflow:lead-edit`.
- `Complexity Flag`: narrow, broad, caller-visible, or cross-module based on
  ticket and conversation artifacts.
- `Branch Mode`: continue current branch, create branch when explicitly
  requested or repository rules require it, or sprint blocked.

Verification should cover:

- full ws: at least one ready-ticket caller-visible workflow change where
  proceed selects write-code even before source inspection;
- wsflow: the same kind of task receives a caller-visible or broad complexity
  flag without introducing unavailable `lead-write-code` routing.

---
title: skeleton contract draft and populator split
related:
  260429-research-host-neutral-ws-plugin: host-neutral skill semantics must stay portable across Codex and Claude surfaces
  260503-epic-ws-agent-workflow-stability: skeleton orchestration depends on named-agent and subquery reliability
related-mental-model:
  - workflow-skills
  - named-agent-runtime
  - prompt-bundle
---

# skeleton contract draft and populator split

## Background

`lead-write-skeleton` currently delegates almost the entire skeleton design to
`skeleton-writer`: the delegate explores the codebase, chooses type shapes and
test structure, writes compile-clean stubs, and reports back for lead review.

The desired flow gives the lead ownership of the public contract draft. The lead
should first ask a deep subquery for candidate insertion points for the desired
classes, structs, traits, functions, signatures, and test locations. The lead then
writes an intentionally non-compiling API skeleton and comment-only integration
test targets. A narrower delegate, tentatively `skeleton-populator`, turns that
contract draft into compile-clean stubs and build-valid test scaffolding.

This keeps contract decisions visible in the lead session while still delegating
mechanical population, imports, syntax repair, and build-clean normalization.

## Decisions

- Replace "delegate owns design" with "lead owns contract draft; delegate
  populates it".
- Use `ws/subquery(deep_research: true)` before source edits to identify insertion
  points and adjacent conventions for the desired public contracts.
- Allow the lead's first skeleton edit to be intentionally non-compiling when it is
  limited to public API shapes and comment-only integration test targets.
- Delegate only the conversion from draft contract to compile-clean stubs to a
  `skeleton-populator` prompt or equivalent renamed role.
- Preserve the final acceptance boundary: the lead reviews the diff, runs build
  or syntax checks, and commits the skeleton artifacts.

## Phases

### Phase 1: Specify the new skeleton contract flow

Update `lead-write-skeleton` to define the new sequence:

1. identify the ticket and contract directives;
2. call `ws/subquery(deep_research: true)` for insertion-point research;
3. lead writes non-compiling public contract draft and comment-only test targets;
4. delegate population converts the draft to compile-clean stubs;
5. lead reviews, verifies build, commits, and records skeleton metadata.

State clear boundaries for what may appear in the non-compiling draft: public
types, public fields, trait/function/method signatures, module placement, and
comment-only integration target descriptions. Implementation logic remains out of
scope.

Success criteria:
- The skill no longer says the delegate owns skeleton design.
- The lead-owned non-compiling draft is permitted only before populator handoff.
- The final committed skeleton remains build-clean.

### Phase 2: Add or reshape the populator delegate prompt

Introduce `skeleton-populator` or reshape the existing `skeleton-writer` prompt so
the delegate reads the lead-authored draft and performs only population work:
syntax repair, imports, placeholder bodies, compile-clean test scaffolding, and
build verification.

The prompt must not reinterpret the ticket into a different public contract. If it
finds a contract conflict, it reports the conflict for lead judgment instead of
silently changing signatures.

Success criteria:
- Prompt text names the draft as authoritative for contract shape.
- The delegate reports any necessary contract amendments instead of owning them.
- Prompt bundle metadata and runtime references are updated when a new prompt stem
  is added.

### Phase 3: Align workflow docs and compatibility references

Update workflow-skills spec, workflow-skills mental model, prompt-bundle mental
model, and any Claude compatibility skill text needed to keep the two surfaces
intentionally aligned.

Success criteria:
- `lead-proceed`, `lead-implement`, `lead-edit`, and `lead-write-code` still
  describe skeleton artifacts consistently.
- Existing skeleton artifacts remain read-only for downstream implementation unless
  an approved skeleton amendment exists.
- The updated docs make the lead/delegate responsibility split recoverable in a
  fresh session.

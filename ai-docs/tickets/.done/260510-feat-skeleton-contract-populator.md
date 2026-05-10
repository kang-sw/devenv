---
title: skeleton contract draft and populator split
related:
  260429-research-host-neutral-ws-plugin: host-neutral skill semantics must stay portable across Codex and Claude surfaces
  260503-epic-ws-agent-workflow-stability: skeleton orchestration depends on named-agent and subquery reliability
spec:
  - 260510-skeleton-contract-populator-flow
related-mental-model:
  - workflow-skills
  - named-agent-runtime
  - prompt-bundle
completed: 2026-05-10
---

# skeleton contract draft and populator split

## Background

`lead-write-skeleton` currently delegates almost the entire skeleton design to
`skeleton-writer`: the delegate explores the codebase, chooses type shapes and
test structure, writes compile-clean stubs, and reports back for lead review.

The desired flow gives the lead ownership of the public contract draft. The lead
should first ask a deep subquery for candidate insertion points for the desired
classes, structs, traits, functions, signatures, and test locations. The lead
then writes intentionally non-compiling low-resolution source and comment-only
integration test targets. A narrower delegate, `skeleton-populator`, turns that
draft into compile-clean stubs and build-valid test scaffolding.

This keeps contract decisions visible in the lead session while still delegating
mechanical population, imports, syntax repair, and build-clean normalization.

## Decisions

- Replace "delegate owns design" with "lead owns contract draft; delegate
  populates it".
- Use `ws/subquery(deep_research: true)` before source edits to identify insertion
  points and adjacent conventions for the desired public contracts.
- Allow the lead's first skeleton edit to be intentionally non-compiling,
  low-resolution source that minimizes lead effort while preserving visible
  contract intent.
- Classify draft comments with language-neutral `CONTRACT:`, `HINT:`, and
  `HOLE:` markers so the populator can distinguish binding public shape from
  research cues and intentionally unknown references.
- Delegate only the conversion from draft contract to compile-clean stubs to a
  `skeleton-populator` prompt or equivalent renamed role.
- Preserve the final acceptance boundary: the lead reviews the diff, runs build
  or syntax checks, and commits the skeleton artifacts.

## Phases

### Phase 1: Specify the new skeleton contract flow

Update `lead-write-skeleton` to define the new sequence:

1. identify the ticket and contract directives;
2. call `ws/subquery(deep_research: true)` for insertion-point research;
3. lead writes low-resolution source draft and comment-only test targets;
4. delegate population converts the draft to compile-clean stubs;
5. lead reviews, verifies build, commits, and records skeleton metadata.

State clear boundaries for what may appear in the non-compiling draft: public
types, public fields, trait/function/method signatures, module placement, and
comment-only integration target descriptions. The draft may use invalid
placeholder identifiers, guessed adjacent references, and explicit open holes
when concrete project types require source discovery. Implementation logic
remains out of scope.

Draft markers:
- `CONTRACT:` marks binding public names, visibility, module placement, call
  shape, and behavior targets.
- `HINT:` marks approximate type references, adjacent APIs, dependency
  direction, or "something like X" source-discovery cues.
- `HOLE:` marks intentionally unknown concrete types, imports, fixtures, helpers,
  or test harness locations.

The populator may normalize `HINT:` references and fill `HOLE:` entries when a
clear project-local choice exists. It must elevate missing or conflicting
`CONTRACT:` elements when preserving the contract would require a public shape
change.

Success criteria:
- The skill no longer says the delegate owns skeleton design.
- The lead-owned non-compiling draft is permitted only before populator handoff.
- The draft marker vocabulary is language-neutral and not tied to one source
  syntax.
- The final committed skeleton remains build-clean.

### Result - 2026-05-10

`lead-write-skeleton` now performs deep insertion-point research, requires the
lead to write a low-resolution source draft with language-neutral `CONTRACT:`,
`HINT:`, and `HOLE:` comment markers, and delegates only source discovery plus
compile-clean normalization to `skeleton-populator`. The final committed
skeleton remains build-clean after the populator handoff.

### Phase 2: Add or reshape the populator delegate prompt

Introduce `skeleton-populator` or reshape the existing `skeleton-writer` prompt so
the delegate reads the lead-authored draft and performs only population work:
source discovery, syntax repair, imports, placeholder bodies, compile-clean test
scaffolding, and build verification.

The prompt must not reinterpret the ticket into a different public contract. If it
finds a contract conflict, it reports the conflict for lead judgment instead of
silently changing signatures.

Success criteria:
- Prompt text names the draft as authoritative for contract shape.
- The delegate reports any necessary contract amendments instead of owning them.
- Prompt bundle metadata and runtime references are updated when a new prompt stem
  is added.

### Result - 2026-05-10

Added the embedded `skeleton-populator` prompt and narrowed `skeleton-writer` to
a compatibility prompt with the same contract-preserving populator boundary.
Prompt tests cover the new stem and marker guidance; runtime prompt bundle
metadata includes the new prompt stem and hash.

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

### Result - 2026-05-10

Updated workflow-skills spec and mental models, prompt-bundle mental model,
runtime references, ws MCP prompt inventory, and project memory. Claude fallback
skill text remains untouched because `claude-plugin/` is frozen legacy fallback
and no explicit Claude compatibility change was required for this Codex-first
workflow update.

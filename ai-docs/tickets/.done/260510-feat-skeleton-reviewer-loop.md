---
title: skeleton reviewer loop for contract draft population
related:
  260510-feat-skeleton-contract-populator: follows the lead-owned draft and skeleton-populator split
spec:
  - 260510-skeleton-contract-populator-flow
related-mental-model:
  - workflow-skills
  - prompt-bundle
completed: 2026-05-10
---

# skeleton reviewer loop for contract draft population

## Background

After adding lead-owned low-resolution skeleton drafts and the
`skeleton-populator` delegate, the remaining review step still made the lead read
and judge the populated diff directly. That contradicted the goal of keeping the
skeleton path lightweight: the lead should own contract judgment, not re-spend
context on detailed source discovery output.

The improved flow adds a read-only `skeleton-reviewer` delegate. It verifies that
the populated skeleton preserved `CONTRACT:` semantics, resolved `HINT:` and
`HOLE:` markers plausibly, stayed within stub-only scope, and provided build or
syntax evidence.

## Decisions

- Keep skeleton review lighter than `write-code`: one reviewer, one amendment
  round, then stop and report if still non-clean.
- Keep `skeleton-reviewer` read-only so correction remains either lead judgment
  or a bounded `skeleton-populator` amendment.
- Scope skeleton review to contract preservation, marker resolution, stub-only
  boundaries, and build or syntax evidence; implementation completeness remains
  out of scope.

## Phases

### Phase 1: Add skeleton-reviewer prompt

Add an embedded `skeleton-reviewer` prompt that reads the ticket, lead draft
files, populator report, and current diff. The reviewer returns `[clean]` or
`[non-clean]` findings for contract drift, hint or hole resolution problems,
implementation leakage, and missing build or syntax evidence.

### Result - 2026-05-10

Added the read-only `skeleton-reviewer` prompt with marker-specific review
heuristics and prompt tests. Runtime prompt bundle metadata now includes the new
stem.

### Phase 2: Route write-skeleton through a lightweight review loop

Update `lead-write-skeleton` so it reads the populator result, invokes
`skeleton-reviewer`, runs build after a clean review, and allows only one
amendment round before stopping and reporting.

### Result - 2026-05-10

`lead-write-skeleton` now registers both `skeleton-populator` and
`skeleton-reviewer`. Non-clean skeleton review findings route to lead contract
amendment, populator amendment, or implementation-leak cleanup, with one
re-review maximum.

### Phase 3: Align workflow docs

Update workflow specs, mental models, runtime references, MCP prompt inventory,
and project memory so fresh sessions recover the lightweight skeleton loop.

### Result - 2026-05-10

Updated workflow-skills spec and mental model, prompt-bundle mental model,
runtime agent requirements, ws MCP prompt inventory, and project memory.

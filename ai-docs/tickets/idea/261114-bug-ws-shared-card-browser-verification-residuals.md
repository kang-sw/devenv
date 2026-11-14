---
title: "Shared cards: preserve unclassified browser assertion failures after visual acceptance"
related:
  261114-feat-ws-dashboard-shared-card-actions: "source-integrated feature; owner accepted disclosed browser residuals"
---

# Shared-card browser verification residuals

## Background

The owner inspected the actual isolated candidate, requested two header polish
fixes, and explicitly approved source integration followed by bug discovery
during use. The shared-card implementation landed in develop at
`f227e6d9b1187ad299e5eea61272495630709e1e` with a clean independent lite review
and passing final focused controls and affected-package checks. This record
keeps the disclosed residual evidence from becoming ownerless after closure.
It is not a finding that the product is broken, nor authority to start another
repair or broad verification round.

## Observed Residuals

- The earlier complete 420px adapter scenario failed
  `actual pointer drag independently reorders an opened preview without pinning`
  at `scripts/board-headless.mjs:1187` on candidate `34592e8b3`.
  `/tmp/shared-card-671-adapter-420.log` preserves the output. The assertion
  combines changed placement time, pin state and drag-marker cleanup without
  separate captured values; the failing condition and product-versus-fixture
  cause remain unclassified.
- A historical repeated-topic-update action-state assertion failed twice
  before later diagnostics and the 1800px scenario passed. Its cause remains
  unclassified; no timing explanation was established. The original
  postcondition remains intact. `/tmp/shared-card-attempt-evidence.md`
  preserves timestamps, excerpts and transcript pointers; the first standalone
  log was overwritten by the second attempt.
- The remaining broad narrow/board-only browser matrix was not rerun under
  the owner's visual-acceptance disposition. Final focused `shared-controls`
  passed at 1800px and 420px, but is not relabeled as a broad `shared-actions`
  pass. Evidence and detailed provenance remain in the related completed
  ticket's Result and historical operational sections.

## Decisions

- Preserve and disclose these residuals after the owner-approved source
  integration rather than repeatedly blocking that integration on the same
  automatic matrix. Rejected: claiming the failed assertions passed or
  silently weakening their expected behavior.
- This idea records follow-up only. Diagnosis, repair scope and execution
  timing are not settled by this record.

## Constraints

- Do not infer product correctness or a fixture/timing cause from a later
  passing checkpoint alone.
- Keep source verification separate from shared/live runtime delivery; the
  source merge did not activate the shared daemon.
- Frozen P4/649/history/ResizeObserver boundaries are unchanged.

## Phases

### Phase 1: Scope future diagnosis from retained evidence or observed use

Pending owner direction. Establish which recorded condition fails and whether
it corresponds to actual user-visible behavior before selecting a repair and
verification plan. Preserve the original assertions and historical evidence.

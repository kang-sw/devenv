---
title: "Dashboard: record future decomposition of large frontend files"
---

# Dashboard: record future decomposition of large frontend files

## Background

The rail integration reported advisory sizes of 2,093 lines for
`agents-plugin-tool/internal/wsdashboard/dashboard.css` and 9,089 lines for
`agents-plugin-tool/internal/wsdashboard/dashboard.js`. These are the merge
advisory's counts, not a new measurement. The advisory did not block the merge.

The owner selected whiteboard (668)=a on 2026-11-14: record a separate idea
without interrupting the approved feature implementations or integrations.

## Decisions

- Capture future file decomposition as an idea only. Rejected: making file
  splitting the next implementation priority or doing it inside active work.
- Decomposition design and implementation timing remain undecided. This idea
  does not authorize a source change, a size-warning exception or a new
  prerequisite for current feature work.

## Constraints

- Preserve the approved work already in flight; this ticket is not a blocker.
- Before promotion, settle the decomposition scope and verification plan.
  No module boundaries, extraction mechanism or implementation plan are
  approved by this record.

## Phases

### Phase 1: Future decomposition, pending scope settlement

Record only. Do not execute this phase from the idea inventory. Define and
settle the implementation and verification contract if the owner resumes it.

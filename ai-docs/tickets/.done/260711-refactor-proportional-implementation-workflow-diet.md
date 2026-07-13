---
title: "Make routine implementation workflow costs proportional"
sage-review-design: skipped
sage-review-completeness: skipped
related:
  260630-epic-skill-playbook-diet: applies the epic's MCP-ification boundary to execution cost rather than removing delegation
  260711-feat-current-branch-low-ceremony: preserves the existing safe low-ceremony current-branch path
spec:
  - 260505-proceed-routing-pipeline
  - 260505-implementation-workflow-skills
  - 260625-session-state-tools
  - 260505-ticket-document-system
related-mental-model:
  - workflow-skills
  - mcp-runtime
  - documentation-system
plans:
  phase-1: 2026-07/11-1541-260711-refactor-proportional-implementation-workflow-diet
completed: 2026-07-11
---

# Make routine implementation workflow costs proportional

## Background

The workflow's delegated implementation boundary is valuable: it keeps source
exploration and implementation detail out of the lead context at a small
latency cost. The remaining bloat is macro-level duplication around otherwise
bounded work: ticket creation from direct inline requests, review partition
fanout driven by surface labels rather than independent risks, repeated full
test runs after documentation-only changes, and Result or mental-model text
that repeats an already authoritative ticket or spec.

This refactor preserves the normal discussion, ticket, delegation, survey-plan,
Sage, branch, merge, and push boundaries while making routine direct-entry
implementation costs proportional to the actual coordination and risk facts.

## Decisions

- Preserve the normal `lead-discuss -> lead-write-ticket -> ticket ->
  lead-proceed` path. Existing ticket targets retain their current readiness and
  implementation routing.
- For a ticketless direct inline request, require a ticket only when the work
  spans multiple independently reviewable phases after required discussion or
  needs cross-session context that linked spec/commit history cannot recover. A
  settled bounded change may remain inline regardless of file count or public
  surface when it is one reviewable slice and commit/spec history is sufficient.
- Preserve delegated implementation, the survey planner, and the implementer's
  file-first `PlanPath` contract. Do not add a ticket-direct implementer route.
- Preserve the existing Sage design and completeness lifecycle unchanged.
- Derive review partitions from independent risk signals rather than surface
  labels alone. Correctness covers material correctness/security risk and new
  contracts or public symbols; fit covers material fit risk, cross-module work,
  or reuse uncertainty; test covers material test risk or new test files.
- Resolve zero or one review partition to `single`; use `partitioned` only for
  two or more independent partitions. Existing test coverage alone must not add
  a test partition, and public surface alone must not add a fit partition.
- Reuse passing full-suite evidence only while its covered code, tests,
  dependencies, build configuration, and generated inputs remain unchanged.
  Documentation-only commits run only their affected checks.
- Dispatch the mental-model updater only for a new non-obvious invariant,
  reusable domain rule, or modification guideline not already covered by the
  authoritative spec.
- Ticket Result and Edition text records deviations, verification evidence,
  unresolved findings, and deferred follow-up findings without restating the
  phase plan or linked spec.
- Add no new workflow mode, routing fact, public schema field, or user-facing
  vocabulary.

## Constraints

- Keep raw safety classification, `low_ceremony_if_safe`, branch isolation,
  merge confirmation, and no-push boundaries unchanged.
- Keep review mandatory where the existing verdict requires it; this change
  reduces fanout, not review authority.
- Canonical playbook and infra sources remain authoritative; regenerate
  manifests and the byte-identical wsflow mirror rather than hand-editing
  generated resources.
- Use exactly one implementation subagent for this slice. The lead owns ticket
  capture, diff review, verification, documentation closeout, and final
  reporting; do not spawn Sage, planner, or reviewer agents for this run.
- Treat the current uncommitted canonical wording draft as the starting patch,
  preserving unrelated branch history and user changes.

## Phases

### Phase 1: Apply proportional inline, review, verification, and Result rules

- Finalize the current canonical wording draft for direct-inline ticket
  judgment, test evidence reuse, and delta-only Result/Edition content.
- Adjust `enter.implement` review allocation so zero or one independent risk
  partition produces `single`, while two or more remain partitioned; remove
  existing-test-only and public-surface-only fanout.
- Update generated todo instructions so unchanged-source full-suite evidence is
  reusable and mental-model dispatch is conditional on new reusable guidance.
- Extend focused resolver, session-state, convention, playbook, manifest, and
  wsflow mirror tests. Regenerate canonical and product-mode resources.
- Verify the full Go and wsflow suites, inspect the complete diff in the lead,
  update the linked specs and affected mental models only where behavior or
  modification guidance changed, record the minimal ticket Result, and stop
  before merge or push.

### Result (fbc4e65b) - 2026-07-11

Behavioral delta: bounded ticketless inline work now depends on coordination
and recovery needs rather than file count or public surface; automatic review
uses independent risk partitions and selects a single reviewer for zero or one
partition. Verification and documentation guidance now reuse unchanged-input
full-suite evidence, condition mental-model work, and keep Result/Edition prose
delta-only.

Verification: focused MCP/wsdoc/wsrsrc tests, the full Go suite, nine wsflow
Python tests, generated manifest/mirror checks, byte-identity checks, and
`git diff --check` passed. Lead-owned review fixed three wording/contract gaps
after the implementation commit.

Deviation: the lead authored the minimal PlanPath to honor the one-implementer
request, but the implementer then spawned one nested fresh-reader audit because
the skill-authoring playbook required it. No further subagents were used. No
unresolved or deferred implementation findings remain.

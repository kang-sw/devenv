---
title: lead-implement delegated pre-edit guard
related:
  260525-bug-implement-review-fix-owner: adjacent implementation-owner clarification for delegated lead-implement runs
spec:
  - 260505-implementation-workflow-skills
related-mental-model:
  - workflow-skills
---

# lead-implement delegated pre-edit guard

## Background

During the `260525-feat-ws-dashboard-sqlite-agent-activity-source`
implementation, `lead-implement` directly edited source files even though the
ticket involved backend cross-module behavior, API projection semantics, a new
dependency, route tests, and persistence-backed activity projection behavior.
That scope should have routed through delegated write-code.

The current skill has direct/delegated criteria, but the criteria are not strong
enough as a pre-edit guard. In practice, the lead can inspect source for
routing context and then continue into source mutation without explicitly
proving every direct-edit predicate or spawning the implementer after a
delegated verdict.

## Expected guard

Ticket-driven ready implementation should default to delegated write-code.
Direct edit should require a short explicit verdict before any source mutation,
and that verdict should prove every direct-edit predicate:

- Single file.
- Internal-only.
- No callers affected.
- No new public symbols.
- No new test files.
- No explicit delegation request.

If any predicate is unknown, or if the scope involves cross-module behavior,
public/API semantics, dependencies, route behavior, tests, or persistence
behavior, the run should be delegated.

On a delegated verdict, the lead may inspect source only for routing, brief, and
plan quality. Source mutation should be owned by the implementer agent.

## Recovery note

When this guard is missed and a lead-authored partial implementation already
exists, do not discard the work by default. Treat the changes as a lead draft,
not authoritative implementation output, and hand them to the implementer for
review, correction, verification, and source/test/dependency commits. The lead
then owns review orchestration, documentation, ticket closeout, and final gate.

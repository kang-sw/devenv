---
title: "Implementation routing creates a nested impl branch despite explicit non-impl continuation"
related:
  260921-feat-ws-impl-branch-identity-resolver: "landed identity_vouch contract"
  260918-feat-route-implement-shared-branch-continuation-override: "closed continuation predecessor"
  261115-feat-ws-whiteboard-panel-first-stream: "ticket whose completed Phase 1 history accompanied the reported route"
---

# Implementation routing creates a nested impl branch despite explicit non-impl continuation

## Background

Dogfood finding #8, 2026-11-16. A ticket worker explicitly assigned to a
non-impl branch such as `exec4/panel-first-literal-code` called
`route.resolve_implement` with an exact `policy.branch.identity_vouch` and
`allow_rename=no`. The ticket was already ready and carried Phase 1 Result and
Edition history. The resolver still returned `action=create` with a nested
`impl/<non-impl-branch>/<stem>` target instead of continuing or explicitly
refusing that assignment. The full request/response was not supplied; retain it
before concluding which normalization/branch-plan path caused the outcome.

Source search anchors: `implementMergeRootFor`, `resolveImplementBranchPlan`
and `identity_vouch` in
`agents-plugin-tool/internal/mcp/implement_resolver.go`. A named non-impl branch
being its own merge root must not be confused with authority to depart from an
explicit continuation assignment. `allow_rename=no` alone is not assumed to
mean that creating another branch is forbidden.

## Reproduction

Construct a disposable repository on `exec4/panel-first-literal-code` and a
ready ticket with Phase 1 Result/Edition history. Invoke the route using the
actual incident facts, an exact branch-and-ticket identity vouch, and
`allow_rename=no`. Record normalized facts and the branch plan. Compare a normal
impl continuation, absent vouch, mismatched vouch and an explicit new-work case.
Do not execute the returned branch action during reproduction.

## Candidate fix directions

- Support explicitly vouched non-impl continuation with a clear identity and
  ownership contract.
- If unsupported, return an explicit refusal describing the incompatibility,
  rather than a create action contradictory to the assigned route.
- Distinguish rename permission, branch identity, continuation intent and
  create permission instead of overloading any one signal.

## Decisions

Compatibility target: **T0** for a correction preserving existing route inputs
and supported callers. This is provisional, not approval of an input/schema or
branch-policy change. Ground old/new request writers and branch-plan consumers
before settling the tier; removing supported old calls would be T3, not fixed
by refreshing the tool list.

## Open questions

- Is identity_vouch meant to cover non-impl branches, or is a separate explicit
  continuation signal required?
- Should the supported outcome be reuse, or an explicit stop pending lead action?
- How should existing Result/Edition history influence continuation without
  granting branch ownership by itself?

## Phases

### Phase 1: Make explicit non-impl continuation yield a coherent branch verdict

Recover the exact route facts, settle the supported continuation/refusal
contract, and implement it with no implicit rename or branch creation. Future
fixtures cover the incident, vouch mismatches, foreign unmerged work, existing
impl continuation, target collisions, and text/JSON/action/checklist agreement.
No branch mutation, resolver call or test is performed by this capture.

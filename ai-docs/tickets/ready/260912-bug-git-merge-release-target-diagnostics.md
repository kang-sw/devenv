---
title: "Make git.merge release-target refusals actionable and safely acknowledgeable"
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260912-research-git-merge-release-target-policy: source research; its Verified Findings and Confirmed Decisions define this contract
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c853fa3be24f85a0
sage-review-completeness-reviewed: c853fa3be24f85a0
---

# Make git.merge release-target refusals actionable and safely acknowledgeable

## Background

`route.resolve_implement` can produce `impl/main/<stem>` and
`impl/master/<stem>` in trunk-only projects, but `git.merge` rejects those
encoded roots before running its ordinary safety checks. The refusal exposes
neither the release-policy premise nor a supported resolution, leaving a valid
worker handoff with no integration path.

This child replaces that topology-specific dead end with a diagnostic,
default-deny acknowledgement flow. It derives only from the source research's
Verified Findings and Confirmed Decisions; automatic authorization from
inferred topology, rejected in that research, is not part of this phase.

## Decisions

- For an `impl/main/<stem>` or `impl/master/<stem>` candidate without an
  acknowledgement, return a structured `policy_blocked` result rather than an
  opaque MCP error. Include every refusal reason and an actionable resolution
  for each.
- Apply that default-deny first call regardless of inferred repository
  topology. Review posture may enrich the diagnostics but does not authorize
  the target automatically.
- Classify every diagnostic as `must_resolve` or `overrideable`.
  `release_target_override` acknowledges only the latter. Ref validity,
  encoded-target identity, worktree cleanliness, an existing merge,
  containment, and changed inspected tips remain `must_resolve`.
- Extend the request with this literal acknowledgement shape:

  ```text
  release_target_override?: boolean  # default false
  expected_source_oid?: string       # required when override is true
  expected_target_oid?: string       # required when override is true
  ```

- Return the inspected source and target OIDs with the refusal. An override
  rechecks both immediately before mutation and returns a new diagnostic when
  either differs.
- Include the available review frontier and uncovered candidate range as
  evidence. Do not infer review quality or unresolved findings from the
  frontier alone.
- When a Git result cannot be parsed reliably, attach the command and bounded
  raw output to that diagnostic entry.
- Tell the caller to retry the same `git.merge` call with
  `release_target_override: true` and the returned OIDs after explicit
  acknowledgement. Do not present native Git as an impl-integration fallback.
- Keep the existing successful behavior for non-release targets and the
  existing non-waivable Git safety and conflict behavior.

## Constraints

- Keep the acknowledgement release-target-specific; do not introduce a generic
  override for current or future refusal classes.
- Do not add an opaque acknowledgement-token lifecycle.
- Preserve lead-only authorization and `impl/<root>/<stem>` target derivation.
- Keep repository-specific branch topology out of the MCP contract.
- Preserve the raw-Git exception for goal-to-parent promotion only; it does not
  apply to an `impl/*` release-target refusal.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/git_merge.go, agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/mcp/git_merge_test.go, agents-plugin/rsrc/lead-run/lead-run.md, and agents-plugin-wsflow/rsrc/lead-run/lead-run.md |
| scope.surface | public-interface | git.merge is an MCP tool with a published input schema in agents-plugin-tool/internal/mcp/server.go#L3389-L3406; the ticket changes its request and result contract. |
| scope.new_public_symbol | no | none; the ticket extends the existing git.merge request and result rather than naming a new tool or exported symbol. |
| scope.new_type_contract | yes | release_target_override, expected_source_oid, and expected_target_oid are named request fields in Decisions. |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/git_merge_test.go#L63-L123 covers current main/master, dirty-worktree, conflict, and checkout-safety behavior. |
| complexity.reuse_points | confirmed | Existing git.merge implementation and schema supply the extension points (agents-plugin-tool/internal/mcp/git_merge.go#L24-L121; agents-plugin-tool/internal/mcp/server.go#L3389-L3406). |
| complexity.side_effect_risk | high | The acknowledgement gates a mutating merge into main or master. |
| risk.correctness | high | Incorrect OID binding or diagnostic classification could merge a changed or unsafe candidate. |
| risk.fit | moderate | The confirmed source research fixes a parent-epic integration dead end while retaining current non-release behavior. |
| risk.test | high | The phase requires coverage for diagnostic combinations, retry OID changes, parsing fallback, safety checks, conflicts, and both package surfaces. |
| risk.security_or_contract | high | The public lead-only MCP merge contract gains an acknowledgement that must remain release-target-specific and default-deny. |

## Phases

### Phase 1: Diagnose and acknowledge release-target merges

Extend the `git.merge` request, result, and merge implementation with the
confirmed acknowledgement and diagnostic contract. Collect all checkable
policy and safety findings before mutation, distinguish must-resolve findings
from the release-target acknowledgement, and keep the candidate snapshot bound
to its source and target OIDs.

Update the lead-run handoff prose to relay the structured diagnostics and ask
for explicit acknowledgement only when the tool reports an overrideable
release-target policy finding. After approval it re-calls `git.merge` with the
override and inspected OIDs; it never substitutes a native Git merge. Verify
main and master refusals, mixed must-resolve and overrideable diagnostics, OID
changes between refusal and retry, raw-output fallback, non-release target
compatibility, non-waivable safety checks, conflicts, and the wsflow mirror and
package contracts.

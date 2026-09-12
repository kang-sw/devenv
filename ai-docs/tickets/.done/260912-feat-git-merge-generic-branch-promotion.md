---
title: "Generalize ws/git.merge across local branch promotion paths"
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260912-research-git-merge-epic-develop-boundary: source research; its Verified Findings and Confirmed Decisions define this contract
  260911-feat-ws-git-merge-lead-owned-merge-authority: prior impl-only merge authority
  260912-bug-git-merge-release-target-diagnostics: preserve its release-target safety contract
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f5078d532341cc78
sage-review-completeness-reviewed: f5078d532341cc78
completed: 2026-09-12
---

# Generalize ws/git.merge across local branch promotion paths

## Background

`ws/git.merge` currently rejects any source that is not named
`impl/<root>/<stem>`. A dogfood attempt to promote `epic/refound` to `develop`
therefore spent a tool call only to learn that the lead had to use native Git.
The generic tool name should expose one safe merge primitive for local branch
promotion while retaining the stronger lifecycle rules available for recognized
workflow branches.

## Decisions

- Accept any exact local source branch and exact local target branch through
  `ws/git.merge`. Preserve the existing optional current-branch source; require
  an explicit target when it cannot be derived from an `impl/*` source.
- For `impl/*`, require the explicit target assertion to match the encoded root
  when provided, and preserve target derivation when it is omitted.
- Keep the tool lead-only and always merge with `--no-ff` using the existing
  structured merge record.
- Keep the current exact-ref, clean-state, merge-in-progress, containment, tip
  recheck, and conflict-handoff behavior for every source kind.
- Keep the default-deny `main` and `master` diagnostics and their OID-bound
  `release_target_override`. An override never waives a `must_resolve` finding.
- Delete a source branch only after a successful merge when it is `impl/*` or
  `goal/*`. Preserve `epic/*` and other source branches.
- Route goal-to-parent, epic-to-review-track, and other lead-owned branch
  integrations through `ws/git.merge`; do not instruct the lead to fall back to
  native Git based on branch type.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Preserve existing impl callers and text/JSON output compatibility while
  extending accepted branch shapes.
- Do not add a second merge primitive or a branch-type dispatch that forces a
  caller to choose between `ws/git.merge` and native Git.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/git_merge.go, agents-plugin-tool/internal/mcp/server.go, agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md |
| scope.surface | public-interface | git.merge MCP schema and description at agents-plugin-tool/internal/mcp/server.go#L3392-L3409 |
| scope.new_public_symbol | no | existing git.merge tool is extended; no new tool is requested |
| scope.new_type_contract | no | existing branch and target inputs are extended in place at agents-plugin-tool/internal/mcp/server.go#L3396-L3408 |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/git_merge_test.go and agents-plugin-tool/internal/mcp/playbook_tools_test.go |
| complexity.reuse_points | confirmed | existing mergeImplBranch at agents-plugin-tool/internal/mcp/git_merge.go#L75-L301 is the merge and safety implementation to generalize |
| complexity.side_effect_risk | high | the tool switches branches, creates merge commits, and conditionally deletes source branches |
| risk.correctness | high | branch validation, target selection, lifecycle cleanup, and release acknowledgement must remain coherent |
| risk.fit | moderate | lead playbooks currently direct goal promotion to native Git at agents-plugin/rsrc/lead-run/lead-run.md#L143-L152 |
| risk.test | moderate | existing tests cover impl-only mechanics and shipped prose must gain generic-promotion coverage |
| risk.security_or_contract | high | public lead-only merge contract retains exact-ref and release-target safety guarantees |

## Phases

### Phase 1: Generalize merge mechanics and lifecycle cleanup

Extend the public tool contract and implementation to inspect and merge any
exact local source and target while applying the confirmed lifecycle-aware
validation and cleanup rules. Update lead workflow prose so supported branch
promotions consistently call `ws/git.merge`.

Verification covers generic and recognized branch sources, source preservation
and deletion, impl encoded-target compatibility, release-target refusal and
acknowledged retry, hard safety failures, conflict retention, structured output,
and mirrored shipped playbook behavior.

### Result (48d033c1) - 2026-09-12

Extended `git.merge` to accept exact local non-impl sources with an explicit
target, retaining impl encoded-root derivation and assertion checks. Successful
merges delete impl and goal sources and preserve epic and ordinary sources.
The existing exact-ref, containment, worktree, tip-recheck, conflict, and
OID-bound release-acknowledgement pipeline serves every source kind.

Updated the public schema and lead promotion guidance, regenerated canonical
resource manifests and the wsflow mirror, and added real-Git promotion,
refusal, conflict-retention, and MCP release-retry coverage. Existing text and
JSON fields remain compatible. Internal `implMerge` names were retained to
avoid unrelated renaming; there were no structural deviations.

Verification: `go test ./...` and `scripts/smoke-ws-mcp.sh ..` passed from
`agents-plugin-tool`; `python3 -m unittest discover agents-plugin-wsflow/tests`
passed all 11 tests. Focused merge and rendered lead-policy checks also passed.
Independent correctness/security, fit, and test reviews were clean; fit review
included a fresh-reader audit and ws/wsflow rendering and mirror checks.

Unresolved findings: none. Deferred scope: none. Installed-cache verification
was not required because launcher and plugin-managed startup configuration
were unchanged.

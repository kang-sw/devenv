---
title: "Allow subagents to acquire and release pooled worktree leases"
related:
  261116-feat-ws-worktree-release-guard: existing lease-holder and dirty-unleased release guard
  260915-bug-ticket-worker-serial-route-acquires-extra-worktree: preserve the user-approved serial versus parallel worktree contract
sage-review-design-reviewed: 2868ef47b07e9be7
sage-review-completeness-reviewed: 2868ef47b07e9be7
sage-review-design: completed
sage-review-completeness: completed
---

# Allow subagents to acquire and release pooled worktree leases

## Background

The owner proposes letting native and Pi subagents manage pooled leases and
adding a merge-time reminder about checked-out source branches. In the triggering
incident, delegates could not acquire from the pool, so the lead directed them
to create raw worktrees under `D:/Temp`; the dashboard detected them outside the
pool. Its pending-worktree admission view excludes pooled paths; pooled
worktrees have a separate card (`agents-plugin-tool/internal/wsdashboard/rail_worktree_admission.go:292-314`,
`agents-plugin-tool/internal/mcp/dashboard_worktrees.go:94-135`).

Today acquire and release are lead-only both at dispatch and in their handlers;
Pi worker profiles (`PiWorkerWSTools` and `PiMiniLeadWSTools`, from which
`AgentWSTools` also derives) omit them
(`agents-plugin-tool/internal/mcp/server_inventory.go#L29-L31,L143-L148,L220-L240`,
`agents-plugin-tool/internal/mcp/server_dispatch.go#L138-L150,L851-L856,L900-L904`,
`agents-plugin-tool/internal/mcp/server_sessions.go#L322-L336`,
`agents-plugin-tool/internal/mcp/server_schemas.go#L499-L524`,
`agents-plugin-tool/internal/wsagent/builtin.go#L89-L132`). Acquire records the
caller as lease parent and mints a lead-scoped holder key at the acquired root
(`agents-plugin-tool/internal/mcp/server_dispatch.go#L883-L886`,
`agents-plugin-tool/internal/mcp/worktree_tools.go#L358-L377`). The release guard
already admits the holder's own key or lease parent, but also accepts a supplied
holder `key` independent of caller identity
(`agents-plugin-tool/internal/mcp/worktree_tools.go#L778-L800`,
`agents-plugin-tool/internal/mcp/server_dispatch.go#L929`).
The supplied-worker-key release test is at
`agents-plugin-tool/internal/mcp/worktree_release_guard_test.go#L87-L98`, and the
holder's-own-key test is at
`agents-plugin-tool/internal/mcp/worktree_release_guard_test.go#L146-L158`.

`git.merge` checks whether the target is held elsewhere; a checked-out source
can instead leave a successful merge with branch deletion refused and a cleanup
advisory (`agents-plugin-tool/internal/mcp/git_merge.go:232-270,398-421`).
`worktree.list` already gathers pooled branch, path and lease facts
(`agents-plugin-tool/internal/mcp/worktree_list.go#L16-L61,L127-L205`).

## Decisions

- P1: native subagents AND Pi profiles (pi-worker, pi-mini-lead) may call worktree.acquire and worktree.release. (1189)=a: open Pi profiles now too (Pi uses the acquired path as cwd for nested Pi).
- (1188)=b: force release is allowed ONLY for the root lead (the user-facing lead session), not the whole lead scope (native lead-run workers excluded). Rationale: force deletes uncommitted work; this morning's incident was another session's work being wiped.
- (1190) ok: the worktree key acquire mints never exceeds the caller's scope (a delegate's acquire yields a delegate-scope key) - blocks privilege escalation.
- (1191) ok: a non-lead caller may release only by identity (the holder itself or its lease parent); knowing the `key` argument alone is not enough - blocks sibling release. Root lead behavior and existing tests unchanged.
- (1192) ok: worktree.remove and worktree.list stay lead-only.
- (1193) ok: P2 nudge - after a successful git.merge, an advisory says the source branch is checked out in pooled worktree <path> (holder <key>); release it if done. No auto-release. Out-of-pool worktrees shown as fact too. Uses the existing `advisory` field -> T0.
- Lead-settled: `AgentWSTools` inherits `worktree.acquire` and `worktree.release` from `PiMiniLeadWSTools`, and both tools are explicitly refused in `readOnlyRefusedTool` (`agents-plugin-tool/internal/mcp/permission_gate.go#L19-L35`) and for `roleLeaf` (`agents-plugin-tool/internal/mcp/server_inventory.go#L232-L237`, including unkeyed `call` discovery at `#L121-L125`). Rationale: the ticket's purpose is subagent worktree leasing across harnesses; explicit refusals close the read-only fail-open, consistent with `261115-feat-ws-permission-profile` Decision 1 and `261119-feat-ws-codex-hosted-worker` (172). Rejected: filtering `worktree.acquire`/`worktree.release` out of `AgentWSTools`.
- Lead-settled: gate Decision `(1191)`'s `key`-argument release bypass on `entry.isTopLevelLead()` (`agents-plugin-tool/internal/mcp/session_auth.go#L32-L34`), so non-root callers — including parented `roleLead` workers (native lead-run workers, sub-leads, and workers holding a lead-minted worktree key) as well as delegates — may release only by identity (the holder itself or its lease parent). Rationale: direct consequence of `(1188)=b`; keeps root-lead behavior and the two existing release tests (`TestWorktreeReleaseByTheLeasesWorkerKey`, `TestWorktreeReleaseByTheHoldersOwnSessionKey`) unchanged. Rejected: gating on `entry.scope == roleLead`.
- Compatibility: P1 is intended as T0 permission widening and P2 as T0
  advisory text, preserving existing calls and store shapes. Older servers or
  profiles may still refuse or omit the newly available calls; use capability
  detection or a compatible fallback rather than assuming mixed-version support.
  Confirm the directional tier against the implementation; compatible additions
  are T0 and the highest affected path controls (`ai-docs/manuals/release-compatibility.md#L54-L57,L61,L73-L78`).

## Constraints

- Go tests run on remote w5090 only (`D:/sys_lock.sh remote w5090 run ...`); never run local `go test` or ws test executables on the office Windows PC.
- Write temporary files under `D:/Temp/<purpose>/` only.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)

## Prior Decisions

- Prior Decisions: rationale index not consulted (tool unavailable to agent.spawn workers)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server_inventory.go, server_dispatch.go, server_schemas.go, permission_gate.go, session_auth.go, worktree_tools.go, git_merge.go, agents-plugin-tool/internal/wsagent/builtin.go, agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md, agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md |
| scope.surface | public-interface | MCP tools worktree.acquire, worktree.release, and git.merge advisory field, plus PiWorkerWSTools, PiMiniLeadWSTools, AgentWSTools, and shipped guidance |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/worktree_tools_test.go, worktree_release_guard_test.go, git_merge_test.go, worker_worktree_tools_test.go, agents-plugin-tool/internal/wsagent/builtin_test.go |
| complexity.reuse_points | confirmed | releaseHolderCheck, acquireWorktree, listWorktrees, ownedPoolRoots, readWorktreeLease, isLeadOnlyTool, roleAllowsTool, readOnlyRefusedTool, isTopLevelLead, PiWorkerWSTools, PiMiniLeadWSTools, AgentWSTools, and implMergeResult.Advisory read at HEAD |
| complexity.side_effect_risk | high | widens destructive worktree reset/clean and pooled worktree provisioning to subagents and updates live role gates and merge advisories |
| risk.correctness | high | minted key scope ceiling, root-lead-gated key-argument bypass, root-lead-only force, read-only/roleLeaf refusal, and pooled vs out-of-pool merge source detection |
| risk.fit | high | must reconcile server_inventory/dispatch split, AgentWSTools inheritance with readOnlyRefusedTool and roleLeaf exclusions, and existing cleanup_failed diagnostic vs advisory tests |
| risk.test | high | existing tests pin non-lead acquire refusal, cross-lead worker-key release, and empty Advisory on cleanup_failed |
| risk.security_or_contract | high | must prevent privilege escalation to roleLead, read-only/roleLeaf access, sibling key-argument release across non-root callers, non-root force deletion, and unrequested serial-worker worktrees |

## Phases

### Phase 1: Enable scoped subagent leases

Allow `worktree.acquire` and `worktree.release` through the role gates, the
two Pi profile allowlists (`PiWorkerWSTools`, `PiMiniLeadWSTools`), and
`AgentWSTools` (inherited from `PiMiniLeadWSTools`), while explicitly refusing
both tools in `readOnlyRefusedTool`
(`agents-plugin-tool/internal/mcp/permission_gate.go#L19-L35`) and for
`roleLeaf` (`agents-plugin-tool/internal/mcp/server_inventory.go#L232-L237`,
including unkeyed `call` discovery at `#L121-L125`). Enforce all decisions
above: cap minted worktree keys to the caller's scope (`(1190)`), gate both
`force: true` (`(1188)=b`) and the `key`-argument release bypass (`(1191)`) on
`entry.isTopLevelLead()` (`agents-plugin-tool/internal/mcp/session_auth.go#L32-L34`)
so every non-root caller (including parented `roleLead` workers, sub-leads,
worktree-key holders, and delegates) may release only by identity (the holder
itself or its lease parent). Preserve the existing acquire and release
serialization and guard, the lead-only `worktree.remove`/`worktree.list`
boundary, and the user-approved serial/parallel provisioning contract. Update
shipped tool descriptions for `worktree.acquire` and `worktree.release`
(`agents-plugin-tool/internal/mcp/server_schemas.go#L499-L524`, removing the
`Lead-only.` prefix and documenting root-lead-only `force` and identity-only
non-root release rules) and mirrored workflow guidance in
`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md` and
`agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md`. Pi
nested work uses the acquired path as `cwd`: bind the minted worker key to that
acquired root without exceeding the caller's scope or profile tool ceiling.

Acceptance: tests cover native delegate, Pi profile, and `AgentWSTools`
(`agent.spawn`) access; read-only `agent.spawn` child refusal via
`readOnlyRefusedTool`; `roleLeaf` and unkeyed `call` discovery refusal; scoped
minted keys; nested-Pi `cwd` acquisition with key binding and profile ceiling
intact; holder and parent self-release; sibling `key`-argument refusal for
both delegates and parented `roleLead` workers; root-lead force and non-root
force refusal; unchanged root-lead release behavior; updated
`worktree.acquire`/`worktree.release` tool descriptions; and ws/wsflow guidance
mirroring checks. Existing release-guard tests pass; serial workers do not
acquire an unrequested extra tree. `worktree.remove` and `worktree.list` remain
unavailable to delegates.

### Phase 2: Advise on checked-out merge sources

After any successful `git.merge`, run the checked-out-source inspection before
the non-`impl/*`/`goal/*` early return (`agents-plugin-tool/internal/mcp/git_merge.go#L397`)
and report a checked-out pooled source's path and holder in the existing
`advisory` field, with conditional release guidance. Report an out-of-pool
source checkout as fact without suggesting pool release, and handle unleased
pooled or prunable worktree entries cleanly. Leave `advisory` empty when no
worktree checks out the source branch (preserving
`TestImplMergeCleanupFailureSurfacesDiagnostic` at
`agents-plugin-tool/internal/mcp/git_merge_test.go#L454`). Do not release
automatically; retain successful merge and existing cleanup semantics.

Acceptance: tests cover pooled held sources (including unleased pooled and
prunable entries), out-of-pool sources, non-`impl/*`/`goal/*` merged sources,
no checked-out source (including `cleanup_failed` keeping `advisory` empty),
and both text and JSON advisory output, without changing merge success or
releasing any checkout. Phase 2 is independent of Phase 1's implementation.

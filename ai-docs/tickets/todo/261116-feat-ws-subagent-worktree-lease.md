---
title: "Allow subagents to acquire and release pooled worktree leases"
related:
  261116-feat-ws-worktree-release-guard: existing lease-holder and dirty-unleased release guard
  260915-bug-ticket-worker-serial-route-acquires-extra-worktree: preserve the user-approved serial versus parallel worktree contract
---

# Allow subagents to acquire and release pooled worktree leases

## Context

The owner proposes letting native and Pi subagents manage pooled leases and
adding a merge-time reminder about checked-out source branches. In the triggering
incident, delegates could not acquire from the pool, so the lead directed them
to create raw worktrees under `D:/Temp`; the dashboard detected them outside the
pool. Its pending-worktree admission view excludes pooled paths; pooled
worktrees have a separate card (`agents-plugin-tool/internal/wsdashboard/rail_worktree_admission.go:292-314`,
`agents-plugin-tool/internal/mcp/dashboard_worktrees.go:94-135`).

Today acquire and release are lead-only both at dispatch and in their handlers;
Pi worker profiles also omit them (`agents-plugin-tool/internal/mcp/server.go:126-148,729-767,1442-1447,1492-1496`,
`agents-plugin-tool/internal/wsagent/builtin.go:83-112`). Acquire records the
caller as lease parent and mints a lead-scoped holder key at the acquired root
(`agents-plugin-tool/internal/mcp/server.go:1475-1479`,
`agents-plugin-tool/internal/mcp/worktree_tools.go:350-369`). The release guard
already admits the holder's own key or lease parent, but also accepts a supplied
holder `key` independent of caller identity (`agents-plugin-tool/internal/mcp/worktree_tools.go:769-792`).
The holder-key test is at `agents-plugin-tool/internal/mcp/worktree_release_guard_test.go:144-151`.

`git.merge` checks whether the target is held elsewhere; a checked-out source
can instead leave a successful merge with branch deletion refused and a cleanup
advisory (`agents-plugin-tool/internal/mcp/git_merge.go:232-270,398-421`).
`worktree.list` already gathers pooled branch, path and lease facts
(`agents-plugin-tool/internal/mcp/worktree_list.go:15-67,128-159`).

## Decisions

- P1: native subagents AND Pi profiles (pi-worker, pi-mini-lead) may call worktree.acquire and worktree.release. (1189)=a: open Pi profiles now too (Pi uses the acquired path as cwd for nested Pi).
- (1188)=b: force release is allowed ONLY for the root lead (the user-facing lead session), not the whole lead scope (native lead-run workers excluded). Rationale: force deletes uncommitted work; this morning's incident was another session's work being wiped.
- (1190) ok: the worktree key acquire mints never exceeds the caller's scope (a delegate's acquire yields a delegate-scope key) - blocks privilege escalation.
- (1191) ok: a non-lead caller may release only by identity (the holder itself or its lease parent); knowing the `key` argument alone is not enough - blocks sibling release. Root lead behavior and existing tests unchanged.
- (1192) ok: worktree.remove and worktree.list stay lead-only.
- (1193) ok: P2 nudge - after a successful git.merge, an advisory says the source branch is checked out in pooled worktree <path> (holder <key>); release it if done. No auto-release. Out-of-pool worktrees shown as fact too. Uses the existing `advisory` field -> T0.
- Compatibility: P1 is intended as T0 permission widening and P2 as T0
  advisory text, preserving existing calls and store shapes. Older servers or
  profiles may still refuse or omit the newly available calls; use capability
  detection or a compatible fallback rather than assuming mixed-version support.
  Confirm the directional tier against the implementation; compatible additions
  are T0 and the highest affected path controls (`ai-docs/manuals/release-compatibility.md:42-47,49-55,63-68`).

## Phases

### Phase 1: Enable scoped subagent leases

Allow acquire and release through the role gates and the two Pi profile
allowlists, while enforcing the decisions above. Preserve the existing acquire
and release serialization and guard, the lead-only remove/list boundary, and
the user-approved serial/parallel provisioning contract. Update shipped tool
descriptions and applicable workflow/playbook guidance. Pi nested work uses
the acquired path as cwd; account for its key binding and profile ceiling.

Acceptance: tests cover native delegate and Pi profile access, scoped minted
keys, holder and parent self-release, sibling/key-argument refusal, root-lead
force and non-root force refusal, and unchanged root-lead release behavior.
Existing release-guard tests pass; serial workers do not acquire an unrequested
extra tree. Worktree remove/list remain unavailable to delegates.

### Phase 2: Advise on checked-out merge sources

After successful `git.merge`, report a checked-out pooled source's path and
holder in the existing advisory, with conditional release guidance. Report an
out-of-pool source checkout as fact without suggesting pool release. Do not
release automatically; retain successful merge and existing cleanup semantics.

Acceptance: tests cover pooled held sources, out-of-pool sources, no checked-out
source, and both text and JSON advisory output, without changing merge success
or releasing any checkout. Phase 2 is independent of Phase 1's implementation.

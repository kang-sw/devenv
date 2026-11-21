---
title: "ws 도그푸딩 도구 문제: 티켓 이동 커밋·워크트리 재사용·보고서 경로"
related:
  261121-bug-ws-dogfood-tune-agy-release-route-gaps: "completed coverage for bound-child release and agy tune; its Result records pooled reuse without a lease check"
  261119-bug-ws-agent-v2-dogfood-surprises: "adjacent agent-v2 dogfood; no duplicate fix in this ticket"
  261120-bug-ws-agent-dogfood-delivery-context: "adjacent delivery context work; no duplicate fix in this ticket"
---

# Dogfood tool surprises: ticket move commit, pooled worktree reuse, and report path

## Background

The develop lead reported five tool surprises on 2026-11-21. This ticket retains only the three still-uncovered fixes. The observations were not reproduced while drafting; `VERIFIED` denotes a current source or completed-ticket fact, and `INFERRED` denotes a reported runtime outcome or cause that source inspection alone cannot establish. Source citations are repository-relative; the source tree was inspected read-only in the main checkout.

1. **Untracked ticket move then commit.** **INFERRED (lead observation):** after `tickets.create_empty` created an uncommitted ticket, `tickets.move` followed by `git.commit` failed when the caller named either the old or new path. **VERIFIED (source):** `TicketsMove` calls `scopedGitMove` and returns its error (`agents-plugin-tool/internal/wsdoc/tickets_mutate.go:282-294`); `atomicGitMove` stages the old file, then calls `git mv` (`agents-plugin-tool/internal/wsdoc/tickets_mutate.go:945-960`). `git.commit` expands ticket paths from status, stages its explicit paths, and filters the rename's deleted side before verification (`agents-plugin-tool/internal/wsgit/git_commit.go:97-105,128-149`; `agents-plugin-tool/internal/wsgit/git_commit_staging.go:131-188`). Existing staging tests model tracked old paths, not a ticket created and moved before its first commit (`agents-plugin-tool/internal/wsgit/git_commit_staging_test.go:14-29,76-109`). **INFERRED:** the failing boundary and root cause remain unknown. Preserve the actual tool errors in the regression before choosing a repair.
2. **Pooled worktree reused while still bound.** **INFERRED (lead observation):** acquisition reused a clean pooled worktree while its lease or live child still referred to it. **VERIFIED (source):** the selector accepts a detached, non-releasing, profile-matched, non-quarantined worktree with empty `git status --porcelain` without reading its lease or querying children (`agents-plugin-tool/internal/mcp/worktree_tools.go:454-490`); `acquireWorktree` then mints a fresh key and writes a lease (`agents-plugin-tool/internal/mcp/worktree_tools.go:340-378`). `worktree.list` already reads the lease (`agents-plugin-tool/internal/mcp/worktree_list.go:186-203`), and spawn binds a child to a worktree root (`agents-plugin-tool/internal/wsagent/agent_tools.go:669-708`). **VERIFIED (prior evidence):** the completed `261121-bug-ws-dogfood-tune-agy-release-route-gaps` Result explicitly records the missing lease check (`ai-docs/tickets/.done/261121-bug-ws-dogfood-tune-agy-release-route-gaps.md:79`). The completed release guard protects `worktree.release`; it does not establish acquire safety (`agents-plugin-tool/internal/mcp/worktree_release_lock.go:63-80`).
3. **No report path kind.** **INFERRED (lead observation):** callers used `review` or `plan` as a substitute when allocating a report path. **VERIFIED (source):** the `path.generate` schema lists only `review`, `prompt`, `plan`, and `clone` (`agents-plugin-tool/internal/mcp/server_schemas.go:1120-1128`); `GeneratePaths` handles `plan` and `clone` specially and the remaining target switch rejects an unknown kind, including `report` (`agents-plugin-tool/internal/wsstate/generated_paths.go:21-42,127-137`).
4. **Release with a bound child.** **VERIFIED (overlap):** `261121-bug-ws-dogfood-tune-agy-release-route-gaps` implemented the daemon-wide bound-child check in commit `732bdbe`; current release admission calls it before hygiene (`ai-docs/tickets/.done/261121-bug-ws-dogfood-tune-agy-release-route-gaps.md:75-80`; `agents-plugin-tool/internal/mcp/worktree_release_lock.go:63-80`). No phase here duplicates that fix.
5. **`agy.*` tune refusal.** **VERIFIED (overlap):** the same completed ticket corrected missing agy manifest exposure in hosted Pi staging and Electron startup in commit `732bdbe`, with tests; it records a separate hosted Pi source follow-up (`ai-docs/tickets/.done/261121-bug-ws-dogfood-tune-agy-release-route-gaps.md:18,76-80`). Current `config.tune` rejects undeclared keys, and the catalog depends on loaded manifests (`agents-plugin-tool/internal/mcp/server_dispatch.go:388-417`; `agents-plugin-tool/internal/mcp/config_manifest.go:106-129`). The reported blanket claim that shipped agy manifests do not exist is incorrect; no duplicate phase is opened here.

6. **`route.resolve_implement` returns a stale merged branch.** **INFERRED (worker observation, 2026-11-21):** for `261115-feat-ws-permission-profile`, `git.resolve_impl_branch` and `route.resolve_implement` returned `impl/develop/foyer-cause-shore`, an earlier branch for the same ticket that is fully merged into `develop` (no commits ahead), while the lead had provisioned the worktree on `impl/develop/permission-p3-codex`; `tickets.acquire` then recorded the provisioned branch correctly. A worktree acquired on the resolved name would check out an old merged tip instead of current `develop`. Source location not yet inspected.

## Decisions

1. **Ticket move/commit, provisional T0:** preserve the existing ticket status paths and `tickets.move`/`git.commit` arguments and result shapes. Make the first-commit sequence work after identifying its actual failing step. The old and new handlers read ordinary Git index/status entries and Markdown ticket files; the proposed correction writes the same index and ticket forms (`agents-plugin-tool/internal/wsdoc/tickets_mutate.go:945-960`; `agents-plugin-tool/internal/wsgit/git_commit.go:97-149`). An old binary can read those forms, and reopening with the new binary requires no migration. Re-grade if diagnosis requires a new shared shape or old-call refusal.
2. **Acquisition safety, provisional T0:** treat an extant lease and a daemon-visible non-closed child bound to a candidate as reasons not to reuse it; select another eligible checkout or create one. Keep `worktree.acquire` arguments/results and the existing lease representation (`agents-plugin-tool/internal/mcp/worktree_tools.go:340-378,454-490`; `agents-plugin-tool/internal/mcp/worktree_list.go:186-203`). An old binary reads the same lease and worktree state, though it may still make the unsafe reuse decision; the new binary needs no recovery on reopen. Re-grade if the chosen stale-lease policy changes the stored lease format.
3. **Report kind, provisional T0 with capability gating:** preserve existing `path.generate` kinds and response shape, and add `report` only after the report's storage and write authority are settled. Old calls still use their existing paths (`agents-plugin-tool/internal/wsstate/generated_paths.go:21-69,127-137`). A new caller must discover support or use a compatible fallback before sending `report` to an old server, because the old target switch refuses it before writing; without that gate the new-to-old direction is T3 (`agents-plugin-tool/internal/wsstate/generated_paths.go:127-137`; `ai-docs/manuals/release-compatibility.md`, Tools and Protocol Mapping). No on-disk migration is expected if the kind only allocates an artifact file; re-grade if implementation chooses a new shared store or mandatory new call.

## Constraints

- `agents-plugin-tool/` edits read `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/performance.md` first.
- `agents-plugin-tool/internal/` edits read `ai-docs/manuals/release-compatibility.md` first.
- `agents-plugin-tool/internal/mcp/` edits also read `ai-docs/manuals/ws-mcp.md` first.
- Keep the completed release guard and agy tune work in `261121-bug-ws-dogfood-tune-agy-release-route-gaps`; verify the remaining hosted Pi follow-up there rather than re-opening it here.

## Open Decision Queue

- [technical] For an existing lease with no live bound child, should acquisition always skip the checkout or support an explicit stale-lease takeover path?
- [product] Where should `path.generate(kind: "report")` allocate files, and which callers may write them? Settle this before changing the schema or dispatcher.

## Phases

### Phase 1: Commit a newly created ticket after moving it

Capture the exact errors and index/status state for `tickets.create_empty` -> `tickets.move` -> `git.commit`. In separate fresh fixtures, name the old path and the new path in the commit call. Fix the responsible move or staging boundary while preserving the existing tool contracts. Acceptance: both flows produce one commit containing the ticket at the destination status, no stale source path, no unrelated staged path, and `tickets.verify` passes on the destination. Include the first-commit case in regression coverage; do not assume the tracked-old-path tests cover it.

### Phase 2: Keep leased or child-bound worktrees out of reuse

Under the pool acquisition lock, check candidate lease and daemon-wide bound-child state before selecting a clean detached checkout. Settle the stale-lease policy in the Open Decision Queue before implementation. Acceptance: acquisition does not reuse a clean detached candidate while another lease or non-closed child is bound to it, including a nested child; it selects another eligible candidate or creates one, preserves the original lease/checkout, and the same candidate becomes reusable only after the binding is cleared. Cover acquisition racing release or spawn at the existing admission boundary.

### Phase 3: Allocate reports through an explicit path kind

Settle report storage and write authority in the Open Decision Queue, then expose the chosen `report` kind consistently in schema and allocator while preserving the four existing kinds. Acceptance: `path.generate(kind: "report")` returns a valid writable report artifact at the settled location; existing `review`, `prompt`, `plan`, and `clone` behavior remains intact; mixed-version callers gate the new kind or fall back before calling an old server.

### Phase 4: Resolve a fresh implementation branch when the derived one is already merged

Diagnose how `git.resolve_impl_branch` / `route.resolve_implement` derive the branch name for a ticket that already has a fully merged implementation branch. Acceptance: when the derived branch exists and has no commits ahead of the base, the resolver either returns a name that a fresh acquire creates on the current base, or reports the stale branch explicitly so the caller does not check out an old tip; when the derived branch carries unmerged work, existing behavior is unchanged. Regression fixtures cover both cases. Provisional tier T0 (result field values only; no argument or shape change); re-grade if a new result field is needed.

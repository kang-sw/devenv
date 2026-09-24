---
title: worktree.list - inspect pooled worktrees and their lease holders
related:
  260910-feat-lead-run-worktree-parallel-route: origin of worktree.acquire/release; this ticket addresses part of its deferred item (h) (GC) by inspection only, not reclamation
  260923-bug-worktree-acquire-concurrent-pool-claim: same pool subsystem (concurrent acquire race); independent
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 7f91f67c65e7d0f5
sage-review-completeness-reviewed: 7f91f67c65e7d0f5
completed: 2026-09-24
---

# worktree.list - inspect pooled worktrees and their lease holders

## Background

`worktree.acquire` / `worktree.release` have no reclamation path. A pooled
worktree that is acquired and never released (the lead session is cut off
mid-work, compaction loses the key, a worker is abandoned) stays
branch-checked-out indefinitely:

- The next `acquire` skips it and creates a new worktree, so the pool grows
  without bound (no TTL, no liveness check, no sweep at acquire or server
  start, no `git worktree prune`, no pool cap).
- A leaked worktree that checked out a parent branch directly (the sparse
  housekeeping checkout, `worktree.acquire(base: <parent>, sparse_paths:
  [...])` with no `target_branch`) blocks every `git.merge` into that branch
  with `target_held_elsewhere`. A leaked `impl/*` worktree only costs disk and
  pool capacity; `git.merge` refuses only when the merge *target* branch is
  held elsewhere.
- Nothing records which session holds a pooled worktree. The minted
  `worker_key` record points worktree-ward (`Root`), but there is no reverse
  pointer, and `release` does not retire the key, so after reuse several keys
  resolve to the same path and none is identifiable as the current holder.

`260910-feat-lead-run-worktree-parallel-route` deferred "a pool cap and GC"
(item (h)). This ticket deliberately does not add GC: it gives the lead and
user the facts to decide, and a path to act on through the existing
`worktree.release(path:)`.

## Decisions

- **Inspection, not reclamation.** Add a read-only `worktree.list` MCP tool
  that enumerates facts per pooled worktree and leaves the judgment to the
  reading lead and user. Rejected: automatic GC gated on conditions such as
  "branch merged + no commits for N days + clean tree" - it needs a
  lead-session liveness judgment the runtime cannot make reliably (session key
  mtime is only refreshed by ws-routed calls, not native edits or shell
  commits), and a wrong reclamation hands a live session's worktree to another
  acquirer.
- **Scope: pool worktrees only.** List exactly the worktrees `release`
  treats as owned: those under the resolved `worktree_pool` root and, when
  the config is the default, also those under the legacy in-tree fallback
  pool (`$(GitRoot)/.ws-worktrees`), reusing `release`'s ownership check so
  no worktree is releasable but unlisted. The primary root and foreign
  worktrees are excluded; `release` refuses them anyway, and `git.merge`'s
  refusal already names such a holder's path.
- **Reported facts per entry** (facts only, no stale/alive verdict):
  - path;
  - branch, or detached;
  - dirty or clean, counting untracked files;
  - last commit time of `HEAD`;
  - newest file mtime, computed only over the paths `git status --porcelain`
    reports; empty for a clean tree, which relies on the last commit time.
    Rejected: walking the whole tree - slow and noisy on ignored content such
    as `node_modules`;
  - lease holder from the lease record (below): `worker_key`, `parent_key`,
    `acquired_at`, and each key record's mtime; or "no lease record" when the
    worktree has none (acquired before this change, or released).
- **No separate merge-blocking flag.** The branch field already says which
  branch the worktree holds, which is exactly what `git.merge` checks.
  Rejected: a derived "blocks merge into X" field - a restatement of the
  branch field.
- **Lease record (worktree -> key).** `worktree.acquire` writes a lease file
  recording `worker_key`, `parent_key`, and `acquired_at` in the acquired
  worktree's Git admin directory (`.git/worktrees/<id>/`, resolved via
  `git rev-parse --git-dir` in the worktree); `worktree.release` removes it.
  `worktree.list` reads holders from these files. Rationale: Git removes the
  admin directory on `git worktree remove`/`prune`, so the record's lifetime
  tracks the worktree's; it sits outside the working tree, so `release`'s
  `clean -ffdx` does not touch it; and it identifies the *current* holder,
  which a key scan cannot (records carry no creation time and their mtime is
  refreshed on use). Rejected: scanning the session `keys/` directory for
  records whose `Root` matches - cost grows with every key minted
  machine-wide (4,848 records / 19 MB observed on the maintainer's machine,
  inflated by integration-test runs), and it cannot distinguish the current
  holder from stale keys of earlier acquisitions.
- **Lease write ordering and failure.** `acquire` writes the lease file only
  after the worker key mint succeeds, so the existing failed-mint detach path
  never leaves a lease behind. If the lease write fails, `acquire` detaches
  the worktree (as on a failed mint) and fails; the already-minted key is
  left unused and ages out through normal key pruning. Rejected: succeeding
  with a warning - the worktree would then list as "no lease record",
  indistinguishable from a pre-change or released one.
- **Exposure.** `worktree.list` is visible wherever `worktree.acquire` /
  `worktree.release` are, following 95d18ff6: agent and no-agent (wsflow)
  modes, the `server.go` tool gate lists, and the tool maps of all three
  `runtime.json` files (`agents-plugin`, `agents-plugin-wsflow`,
  `agents-plugin-pi`).
- **No-touch key reads.** `worktree.list` reads key records without the
  mtime refresh that `sessionStore.lookup` performs (`touch`), so listing does
  not fabricate activity on the keys it reports.
- **Release nudge, clean entries only.** Each clean entry carries a nudge that
  it is releasable via `worktree.release(path: <path>)`, together with a
  warning that `release` discards uncommitted changes (`reset --hard` +
  `clean -ffdx`). Dirty entries carry no release nudge. Rejected: making
  `release` refuse a dirty worktree unless an explicit discard argument is
  passed - it changes existing `release` semantics that lead-run cleanup may
  rely on; the listed dirty fact plus the warning is sufficient.
- **`git.merge` pointer.** When `target_held_elsewhere` names a ws pool
  worktree as the holder, its resolution text additionally points at
  `worktree.list` for inspecting that holder.
- **Authorization.** `worktree.list` requires a lead session key, like
  `worktree.acquire` / `worktree.release`.

## Constraints

- `release` behavior is otherwise unchanged: the only addition is removing
  the lease file. It still does not retire the worker key.
- Out of scope: automatic GC or sweep, lead-session liveness judgment, pool
  cap, retiring session keys on release, and a `workflow_manual` banner for
  leaked pool worktrees.
- Every string the tool emits (nudge, warning, `git.merge` resolution text)
  ships to downstream projects: follow
  `ai-docs/manuals/shipped-surface-boundary.md`.
- MCP server changes follow `ai-docs/manuals/ws-mcp.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 260910-feat-lead-run-worktree-parallel-route (2026-09-15, Result): "Deferred item (d) concurrency cap: resolved as the user-approved batch count itself ... no `pool cap`/GC (item h) is added while the project holds its single-maintainer-serial posture." — bearing: constrains
- 95d18ff6 (2026-09-15, commit): "Reuse eligibility is the conservative golden rule from the ticket: under the owned pool prefix, at detached HEAD, and clean. A branch-checked-out or dirty worktree is skipped so no in-progress work is stomped" — bearing: supports
- 95d18ff6 (2026-09-15, commit): "Key minting reuses the existing sessionStore.mint seam with roleLead ..., parent = the lead key. The pure git lifecycle (provisionWorktree/releaseWorktree) stays Server-free and Runner-injected for table tests" — bearing: constrains
- 95d18ff6 (2026-09-15, commit): "The tool pair is visible in both agent and no-agent (wsflow) modes, matching git.merge/route.resolve_implement, so all three runtime.json tool maps get the same two entries" — bearing: constrains
- 6bc205d2 (2026-09-15, commit): "The release guard is the key safety fix: worktree.release takes a caller path/key, and without the guard a lead passing the main-repo path ... would hard-reset the primary working tree." — bearing: constrains
- e02d2eb0 (2026-09-16, commit): "releaseWorktree now checks both the recomputed default pool and the legacy in-tree pool when the config is the default, because acquire's own fallback decision at provision time is not visible to a later" — bearing: constrains
- 260918-feat-ws-sparse-housekeeping-worktree-occupied-root (2026-09-18, Decisions): "`git.merge` refuses a target held by another worktree, as a merge stop ... The release-before-merge rule ... is advisory; its violation surfaces in the other lead's `git.merge`" — bearing: supports
- 186a87f7 (2026-09-19, commit): "Fixed all four sites that embed a worktree path (TestImplMergeRefuses... pool-holder/plain-holder, TestImplMergeDispatchClassifiesHeldTargetByPool pool-holder/plain-holder) rather than only the three named" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/worktree_tools.go, agents-plugin-tool/internal/mcp/server.go dispatch L1104-L1170 and schemas L3836-L3860 and tool gates L93 L4263, agents-plugin-tool/internal/mcp/git_merge.go L228-L237, agents-plugin-tool/internal/mcp/session_auth.go, agents-plugin/runtime.json, agents-plugin-wsflow/runtime.json, agents-plugin-pi/runtime.json |
| scope.surface | public-interface | new MCP tool worktree.list; changed worktree.acquire and worktree.release side effects; changed git.merge target_held_elsewhere resolution text |
| scope.new_public_symbol | yes | worktree.list MCP tool |
| scope.new_type_contract | yes | lease file record worker_key parent_key acquired_at in the worktree git-dir, and the worktree.list text and json entry shape |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/worktree_tools_test.go TestWorktreeAcquireReleaseDispatch and TestReleaseWorktreeRefusesNonPoolTargets, agents-plugin-tool/internal/mcp/git_merge_test.go TestImplMergeRefusesTargetHeldElsewhere pool-holder |
| complexity.reuse_points | confirmed | listWorktrees, resolvePoolRoot, pathUnder, isDefaultPoolConfig in worktree_tools.go; requireLeadSessionKey and wantsJSON in server.go; sessionStore.readRecord without touch in session_auth.go L212-L226 |
| complexity.side_effect_risk | moderate | acquire and release gain file writes and removals in the git admin dir next to the destructive reset --hard and clean -ffdx path; list must avoid the lookup touch |
| risk.correctness | moderate | lease lifecycle across pool reuse, failed-mint detach at server.go L1128-L1135, the legacy in-tree fallback pool, porcelain path mtime parsing, and path canonicalization |
| risk.fit | moderate | new tool must be registered consistently in server.go gate lists and all three runtime.json tool maps per 95d18ff6 |
| risk.test | moderate | mtime no-touch assertion interacts with touchGuardWindow throttling, and held-path assertions have a Windows canonicalization history per 186a87f7 |
| risk.security_or_contract | moderate | new shipped MCP tool contract and lease file format; nudge text recommends a destructive release on the shipped surface |

## Phases

### Phase 1: Lease record and worktree.list

Implement the lease record in `worktree.acquire` / `worktree.release`, the
`worktree.list` tool (text default, `format: "json"` structured output, as the
sibling worktree tools do), and the `git.merge` resolution pointer, per
`## Decisions`.

Implementation details left to the worker, within these bounds:

- **Lease file contract.** JSON with a schema version, holding
  `worker_key`, `parent_key`, and `acquired_at` (RFC 3339 UTC). Name it
  with a `ws-` prefix (for example `ws-worktree-lease.json`) so it cannot
  collide with Git's own admin files. Resolve the admin directory with
  `git rev-parse --absolute-git-dir`; plain `--git-dir` can return a
  relative path.
- **Lease lifecycle edges.** `acquire` overwrites any existing lease, so a
  worktree detached outside ws and later reacquired gets a fresh record.
  `release` tolerates a missing lease file. `list` reports branch/detached
  and the lease independently, so a detached worktree that still carries a
  lease (detached by hand) shows both facts rather than hiding either.
- **Pruned key records.** A lease can name a `worker_key` or `parent_key`
  whose record was deleted by key retention pruning (30 days idle). Report
  that key as "record missing" instead of an mtime.
- **Dirty-path mtime.** Parse the new path of a rename (`old -> new`), skip
  paths that no longer exist (deletions), and take a collapsed untracked
  directory's (`dir/`) own mtime; do not switch to `-uall`, which reopens
  the rejected full-tree walk.
- **Enumeration.** Skip prunable entries (directory gone); `git.merge`
  already routes those to `git worktree prune`. Order entries by path.
- **JSON shape.** Pick field names and empty encodings consistent with the
  sibling worktree tools' JSON output (for example a null branch when
  detached, an absent newest mtime on a clean tree, a null lease when there
  is none).
- **Terminology.** Shipped strings and JSON fields say "worktree lease",
  never bare "lease", to stay distinct from the ticket leases of
  260924-feat-origin-ticket-ownership-index.
- **Keys shown in full.** Entries carry full `worker_key` / `parent_key`
  values. The tool is lead-only and the key records are already readable on
  disk; no truncation.
- **`git.merge` legacy pool.** Extend the `target_held_elsewhere` pool-holder
  classification to the legacy in-tree fallback pool under the default
  config, reusing the same ownership check as `release` (e02d2eb0), so every
  holder `worktree.list` shows gets the `worktree.list` pointer.

Verification expectations:

- A test that `acquire` writes the lease file with the minted `worker_key`,
  the caller's key as `parent_key`, and `acquired_at`, and that `release`
  removes it.
- `worktree.list` tests covering: a held clean worktree (lease present, nudge
  and warning present); a held dirty worktree (dirty fact, dirty-path mtime,
  no nudge); a detached released worktree (no lease record); a worktree with
  no lease file from before this change; exclusion of the primary root.
- A test that listing does not change the reported key records' mtimes; the
  records' mtimes must be aged past `touchGuardWindow` first, or the
  assertion passes trivially under touch throttling. Call `list` with a
  second lead key: the auth lookup legitimately touches the caller's own
  key, and the auth path's touch is not bypassed.
- A test that a lease naming a pruned key record lists that key as record
  missing.
- A test that `release` succeeds on a worktree with no lease file.
- A `git.merge` refusal test with a legacy in-tree pool holder asserts the
  pool-holder classification and the `worktree.list` pointer.
- Registration: `worktree.list` appears in the `server.go` tool gate lists
  and all three `runtime.json` tool maps, asserted by the existing
  registration/drift tests or a new check where none covers a map.
- A test that a legacy in-tree pool worktree is listed under the default
  config.
- A test that a failed lease write fails `acquire` and leaves the worktree
  detached.
- The `git.merge` pool-holder refusal test asserts the `worktree.list`
  pointer in the resolution text.

### Result (fdee1187) - 2026-09-24

Landed in ade203ac (feature) and fdee1187 (review fixes).

- **Worktree lease.** `worktree.acquire` writes `ws-worktree-lease.json`
  (`schema_version`, `worker_key`, `parent_key`, `acquired_at` RFC 3339 UTC)
  into the worktree's Git admin dir (`rev-parse --absolute-git-dir`) after a
  successful mint, atomically (temp + rename, so an existing lease is
  overwritten); a failed write detaches the worktree and fails the call.
  `worktree.release` removes the lease after the detach succeeds and tolerates
  a missing file; it still does not retire the worker key. Lease I/O lives in
  `agents-plugin-tool/internal/mcp/worktree_lease.go`.
- **`worktree.list`** (`worktree_list.go`): lead-only, text default,
  `format: "json"` opt-in. Enumerates owned pool worktrees (skipping the
  primary root, foreign worktrees, and prunable records), ordered by path,
  with: path; `branch` (null when detached); `dirty` (counts untracked; null
  and `state: unknown` when status cannot be read); `head_commit_time`;
  `newest_dirty_mtime` over `git --no-optional-locks status --porcelain -z`
  paths (rename new path, collapsed `dir/` own mtime, deletions skipped;
  omitted when clean); `worktree_lease` (null when absent) with each key's
  `*_record_mtime` (null = record missing), read via a new no-touch
  `sessionStore.recordMtime`; a `release` nudge plus discard warning on
  known-clean entries only; per-entry `warnings` for unreadable facts.
- **Shared ownership rule.** `ownedPoolRoots` / `underAnyPool` (configured
  pool plus the legacy in-tree pool under the default config) now back
  `release`, `list`, and `git.merge`'s held-target classification;
  `mergeImplBranch` takes `poolRoots []string`.
- **`git.merge` pointer.** A pool holder (including the legacy in-tree pool)
  gets a `worktree.list` pointer in the resolution text.
- **Registration.** `isLeadOnlyTool`, `toolSchemaRequiresSessionKey`, the
  tool schema, all three `runtime.json` tool maps, and the Pi bridge tool
  contract (61 -> 62).

Decisions:

- The `worktree.list` pointer also rides in the `target_held_elsewhere`
  reason, not only the resolution: a non-release `git.merge` refusal surfaces
  only the reason as its error text, so a resolution-only pointer would never
  reach the lead through dispatch.
- Clean entries all carry the release nudge per the Decisions, including
  already-released idle worktrees; the nudge text conditions release on the
  holder being done.
- `dirty` is nullable so a failed status is never reported as clean.
- Deferred (review minor): `wsgit.ExecRunner` combines stderr into stdout, so
  a git warning could contaminate the `-z` stream; the same exposure exists
  in the other status callers and needs a stdout-only runner.

Verification:

- `go test ./...` in `agents-plugin-tool` with an isolated empty `HOME`: all
  packages ok. Under the real `HOME`, two pre-existing config tests
  (`TestResolveAgentTierForHarnessFallsBackToDefault`,
  `TestServeStdioConfigResolveAgentFallsBackToDefault`) fail from the user's
  global config, independent of this change.
- `scripts/smoke-ws-mcp.sh ..`: ok.
- `python3 -m unittest discover -s tests` in `agents-plugin` (73) and
  `agents-plugin-wsflow` (13): OK.
- `node --test test/bridge.test.ts` in `agents-plugin-pi`: 75 pass.
- Review: partitioned correctness/fit/test round 1 (1 Important terminology,
  1 Important schema-gate test, minors), all fixed in fdee1187 and confirmed by
  a round-2 verifier.

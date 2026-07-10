---
title: "Combine GitDiscovery::discover's three rev-parse spawns into one"
sage-review: required
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
related:
  260710-bug-dashboard-root-picker-blocking-git-spawn: sibling perf fix in the same call-site cluster; that ticket fixed blocking-on-async-runtime, this one fixes redundant process-spawn count
---

# Combine GitDiscovery::discover's three rev-parse spawns into one

## Background

Live dogfooding on the native-Windows daemon (2026-07-10, `--no-auth`,
127.0.0.1), after the sibling `spawn_blocking` fix, measured `git.exe`
process-spawn cost directly at ~90-130ms per invocation on this machine (an
OS/AV-layer process-creation floor, not something the app controls).
`GET /api/dashboard/work-roots/open` still took ~2s wall-clock with 3
already-opened work roots registered, because it re-discovers every opened
work root on every open call (4 git spawns each) plus the newly-opened
candidate (4 more) -- roughly 16 git spawns x ~100ms.

`GitDiscovery::discover` (`ws-dashboard/crates/daemon/src/discovery.rs:429-447`)
spawns three separate `git rev-parse` processes per work root via the shared
`git_path` helper (discovery.rs:461-476):

```rust
let worktree_dir = git_path(path, &["rev-parse", "--show-toplevel"])?;
let common_dir = git_path(
    path,
    &["rev-parse", "--path-format=absolute", "--git-common-dir"],
)?;
let git_dir = git_path(path, &["rev-parse", "--path-format=absolute", "--git-dir"])?;
```

`git rev-parse` accepts multiple query flags in a single invocation and
prints one line per flag in the order given. Verified directly on Windows
against a real repo:

```
> git rev-parse --show-toplevel --path-format=absolute --git-common-dir --git-dir
D:/Workspace/Repos/InspectTGV_AIDriven
D:/Workspace/Repos/InspectTGV_AIDriven/.git
D:/Workspace/Repos/InspectTGV_AIDriven/.git
```

Three lines, in the same order as the three query flags. `--path-format=absolute`
is a modifier, not a query flag, and does not add its own output line. This
means `GitDiscovery::discover` can issue exactly one `git` process spawn
instead of three, halving the per-work-root spawn count from 4 to 2 (the
remaining spawn is the separate `git worktree list --porcelain` call in
`git_worktree_paths`, discovery.rs:478-495, a different subcommand that
cannot be merged into `rev-parse`).

This is a pure process-count reduction with no behavior change: the three
values obtained (toplevel, common-dir, git-dir) and the logic that derives
`kind` from `common_dir == git_dir` are unchanged; only the number of `git`
process spawns needed to obtain them changes from 3 to 1.

## Constraints

- Do not change `git_worktree_paths`/`git worktree list --porcelain` -- out
  of scope, different subcommand.
- Do not change the O(N) "re-discover every opened work root on every open
  call" design -- that is a separate, deliberately deferred design decision
  (see sibling ticket `260710-bug-dashboard-root-picker-blocking-git-spawn`),
  not addressed here.
- Preserve exact existing semantics: if the combined command fails (e.g.
  path is not a git repository), `GitDiscovery::discover` returns `None`,
  matching current behavior where the first failing `git_path` call already
  short-circuits to `None`.
- Preserve exact existing path values and `kind` derivation
  (`common_dir == git_dir` -> `GitPrimaryRoot`, else `GitLinkedWorktree`).

## Phases

### Phase 1: Merge the three rev-parse calls into one process spawn

1. In `discovery.rs`, replace `GitDiscovery::discover`'s three `git_path`
   calls with a single `git -C <path> rev-parse --show-toplevel
   --path-format=absolute --git-common-dir --git-dir` invocation; parse the
   three output lines in order (toplevel, common-dir, git-dir), applying the
   same trimming/normalization (`normalize_candidate_path`) `git_path`
   already applied per value.
2. Remove `git_path` if it becomes unused dead code after the refactor (per
   investigation: confirm no other call site depends on it before removing).
3. Add or update a unit/integration test asserting `GitDiscovery::discover`
   (or its effect through `discover_work_root`) still returns correct
   `kind`/paths for both a primary git root and a linked worktree, and that
   a non-git directory still discovers as `PlainDirectory` (i.e. combined
   command failure still falls through exactly as before).
4. Verify no regression: existing discovery-related tests in
   `tests/routes.rs` and any `discovery.rs` unit tests continue to pass
   unmodified.
5. Native-Windows live re-verification (before/after wall-clock timing of
   `open_work_root`/`GET /api/dashboard/resources` against the same
   already-opened-3-work-roots state) is deferred to manual dogfooding --
   record as an Edition note if performed.

### Result (b198a466)

Implemented as a direct-edit hotfix (lead-only review, no plan artifact per
verdict, stacked on the sibling `spawn_blocking` fix branch, renamed to
`implement/git-discovery-combined-rev-parse-hotfix`).

- `GitDiscovery::discover` now issues one `git -C <path> rev-parse
  --show-toplevel --path-format=absolute --git-common-dir --git-dir` spawn
  and parses its three output lines in order, instead of three separate
  `git_path` calls.
- `git_path` had exactly three call sites, all inside `discover`, and no
  direct unit test (confirmed via repo-wide grep before removal); replaced
  with a smaller `non_empty_path(line: &str)` helper carrying the same
  trim/empty-check/normalize logic per output line. No dead-code warning on
  build.
- Item 3 (dedicated new test): not added as a separate test, since existing
  `discovery.rs` unit tests already exercise this exact code path
  end-to-end and unmodified -- `local_provider_distinguishes_git_primary_roots_and_linked_worktrees`
  and `local_provider_discovers_linked_worktrees_from_primary_root` cover
  primary-vs-linked-worktree `kind` derivation, and non-git-directory
  fallback is covered by existing plain-directory discovery tests. All
  passed unmodified post-refactor, satisfying the same regression-check
  intent item 3 asked for.
- Verification: `cargo build -p ws-dashboard-daemon` clean (no dead-code
  warning); `cargo test -p ws-dashboard-daemon` 204/204 passed (0 failed)
  across all test binaries, unmodified.
- Item 5 (native-Windows live re-verification) deferred to manual dogfooding
  against the running native daemon; record as an Edition note if/when
  performed.

## Spec Impact

None. Internal process-spawn-count reduction; no observable HTTP contract,
response shape, or discovered `kind`/path values change.

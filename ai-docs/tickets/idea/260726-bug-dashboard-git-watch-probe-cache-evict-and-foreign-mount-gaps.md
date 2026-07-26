---
title: "Three narrow, accepted-deferred gaps in the git FS-watch invalidation work: a
  pre-existing GitProbeCache::evict key mismatch, and two Phase 4 foreign-mount /
  late-created-directory edge cases"
related:
  260726-refactor-ws-dashboard-git-fs-watch-invalidation: origin
---

## Background

Three small, independently-scoped gaps surfaced across
`260726-refactor-ws-dashboard-git-fs-watch-invalidation`'s later phases. Each
was deliberately deferred (bounded impact, narrow trigger condition, or
already-tracked elsewhere) rather than fixed inline, but none has its own
tracking artifact yet. Bundled here since they're all small and in the same
area (git work-root discovery/watch state), not because they share a root
cause.

## Gap 1: `GitProbeCache::evict`'s key-derivation bug (pre-existing, `18037cc3`)

`evict(&discovered.path)` (`discovery.rs:743-747`) recomputes
`GitProbeKey::for_path` at exactly the moment `canonicalize` is failing (the
root just went non-`Available`), so the recomputed key can never match the
warm entry's canonical key that was stored while the root was still
reachable. The eviction call is therefore a silent no-op on the one path
that most needs it.

Carried forward through Phase 3 (D1) and Phase 4 (D1) as non-load-bearing:
`reconcile`'s disarm path is keyed off an uncached `WorkRootAvailability`
check and is unaffected. The only bounded consequence is on the *arm* path —
`WatchTargets` freshness on reappearance is bounded by the existing 30s
`GitProbeCache` TTL, specifically in the scenario where a root's `kind`
changes during the outage (e.g. git repo → plain directory → git repo
again), not on an ordinary unchanged-repo reappear. Both Phase 3 and Phase 4
added integration tests that specifically exercise the kind-change-across-
outage shape and pass regardless of whether this bug is present or fixed —
i.e. the tests pin the *bound*, not the fix.

Fix direction (not attempted): `evict` needs the *pre-outage* canonical key,
not a freshly recomputed one — either cache the canonical key alongside the
warm entry and evict by identity, or key eviction off the same
non-canonicalizing fallback path a failing `canonicalize` would produce.

## Gap 2: foreign-mount allowlist gate checks only `targets.worktree`

`mount_allows_watching` (`work_root_watch.rs`, Phase 4) is applied only to
a repo's `worktree` target before arming. A linked worktree whose `worktree`
directory sits on a local filesystem but whose `common_dir` (the primary
repo's `.git`) lives on a foreign mount (WSL2 `/mnt/*`, NFS, CIFS,
SSHFS/FUSE) arms successfully and reports `Armed`, but never observes
`common_dir` filesystem events — so ref changes made through that shared
`common_dir` (switch, fetch, branch create/delete from the linked worktree
or any sibling) are silently invisible to this repo's watcher. The
Constraints wording ("resolve the target's mount filesystem type") arguably
already covers all three `WatchTargets` fields, not just `worktree`.

Fix direction (not attempted): apply `mount_allows_watching` to
`git_dir`/`common_dir` as well as `worktree`, degrading the whole repo if
any of the three fails the check.

## Gap 3: `common_dir/info` created after arming is never watched on Linux

`common_dir/info` is registered only if it already exists at arm time
(Phase 4). `register_incremental_directory` explicitly refuses anything
under `common_dir`, so an `info/` directory created after a repo is already
armed (e.g. the first time something runs `git config --local` or otherwise
triggers git to lazily create `info/exclude`'s parent) is never picked up on
Linux — `info/exclude` edits made from that point on are missed until the
next full re-arm cycle.

Fix direction (not attempted): either register `common_dir/info` eagerly
regardless of whether it exists yet (watching for its own creation via the
parent `common_dir` registration, which Linux does register), or special-
case `common_dir/info`'s creation event to trigger an immediate incremental
registration despite the general "nothing under `common_dir` is
incrementally registered" rule.

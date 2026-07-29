---
title: "WorkRootId hash flips when a workRoot path fails to canonicalize (dir removed/recreated)"
---

# WorkRootId hash flips when a workRoot path fails to canonicalize (dir removed/recreated)

## Background

Surfaced as a dogfood surprise while running `cargo test -p ws-dashboard-daemon`
natively on macOS for the first time (260725-bug-dashboard-terminal-platform-macos-unsupported
Phase 1 — a narrowly-scoped process-identity/kill-mechanism port; this bug is
unrelated to that ticket's terminal-platform surface and was deliberately left
out of its Phase 1 fix so as not to widen a surgical port into an unreviewed
discovery-subsystem change).

`ws-dashboard/crates/daemon/src/discovery.rs::canonical_or_normalized` derives
`WorkRootId` via:

```rust
fn canonical_or_normalized(path: &Path) -> PathBuf {
    path.canonicalize()
        .unwrap_or_else(|_| normalize_candidate_path(path))
}
```

`local_work_root_id_for_path` hashes this result (`stable_path_hash`). When the
directory exists, `canonicalize()` succeeds and resolves any symlinked path
segment. When the directory does not exist (removed, or race-checked before
creation), `canonicalize()` fails and the fallback hashes the *unresolved*
path instead. On macOS, `std::env::temp_dir()` (and `/tmp`, `/etc`) resolve
through a symlink (`/var/folders/...` -> `/private/var/folders/...`), so a
workRoot's `WorkRootId` differs depending on whether the directory currently
exists on disk. Two IDs computed for the *same* logical root at different
existence-states therefore do not match.

Observed failure mode: `LocalDashboardResourcesProvider::dashboard_resources_with_registry_sync`
computes a workRoot's `pruned_work_root_ids` entry using the ID derived while
the directory is *missing* (unresolved form), then calls
`opened.unregister(&that_id)` — which silently no-ops because the actually
registered entry is keyed by the ID derived while the directory *existed*
(resolved form). The stale entry survives in `OpenedWorkRoots`, so:

- a pruned/removed workRoot can silently un-prune itself once its directory
  reappears on disk, without any explicit re-open call, and
- routes that should 404 "unknown workRoot" for a genuinely-missing root can
  instead hit a stale-but-still-registered entry and answer a different
  status code than callers expect.

## Evidence

Two `ws-dashboard-daemon` integration tests fail deterministically and in
isolation (`cargo test -p ws-dashboard-daemon --test routes <name> -- --test-threads=1`)
on macOS (aarch64-apple-darwin), both tracing to this one root cause:

- `dashboard_resources_refresh_prunes_workspace_without_available_work_roots`
  (`tests/routes.rs:1027`): after remove+recreate of the workRoot directory,
  the pruned workspace reappears without an explicit open.
- `online_missing_work_root_returns_bounded_unavailable_without_path_leak`
  (`tests/routes.rs:1344`): a route that should 404 "unknown workRoot" for a
  removed root instead returns 409.

Neither test touches terminal/process code; both go through
`discovery.rs`/`work_root_files.rs` only. Not reproduced identically on Linux
in this session (no readily-available symlinked `$TMPDIR`), but the
instability is structural, not macOS-exclusive — any deployment where a
workRoot (or an ancestor directory, e.g. a symlinked home or mount point)
lives under a symlinked path segment can hit it.

Also worth noting: `discovery.rs::paths_equivalent` already carries a comment
explicitly declining to reuse `canonical_or_normalized` for equality checks
because of this exact canonical/non-canonical asymmetry — i.e. the
instability class was already known at that call site, just not closed at
the ID-derivation call site.

## Phases

### Phase 1: Stabilize WorkRootId derivation independent of path existence

Make `canonical_or_normalized` (or its caller) produce the same hash input
for a given logical path regardless of whether the leaf directory currently
exists — e.g. canonicalize the deepest existing ancestor and append the
remaining (nonexistent) components verbatim, rather than falling back to a
fully-unresolved path the moment `canonicalize()` fails on the leaf.

Needs a design decision, not a drive-by patch: confirm the fallback strategy
doesn't regress `paths_equivalent`'s already-deliberate non-reuse of this
helper, check performance (repeated `exists()`/`canonicalize()` walk cost for
deeply-nested missing paths, called on every resource-view recompute), and
decide whether the same instability affects any other `WorkRootId`-keyed
identity path (terminal registry lookups, git-worktree linked-root discovery)
beyond the two tests found so far.

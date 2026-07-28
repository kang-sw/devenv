---
title: "dashboard fs watch: macOS backend selection - RESOLVED as macos_fsevent; kqueue's per-entry fd cost recorded as the reason"
related:
  260727-chore-merge-ws-dashboard-dev-into-goal-branch: surfaced-by
  260726-refactor-ws-dashboard-git-fs-watch-invalidation: subject
completed: 2026-07-28
---

## Stem note

The stem `260728-bug-dashboard-macos-kqueue-fd-ceiling-unguarded` was
minted while `macos_kqueue` was still the shipping choice, so it now reads
narrower than the ticket's actual subject. Stems are immutable absolute
references (history is queried by `git log --grep=<stem>`), so it stays.
Read it as "the macOS fs-watch backend question", not as "kqueue ships and
needs a guard".

## Status: settled

`ws-dashboard/crates/daemon` builds macOS fs-watching on `notify`'s
`macos_fsevent`, and `tests/git_watch.rs` is **11 passed, 0 failed** on
macOS. The backend question this ticket was opened to defer is **decided in
fsevent's favour**, on its own evidence and with no trade-off left to weigh:
fsevent passes the whole suite AND carries no per-fd cost, while kqueue's
equal pass rate is a fixture-scale artifact over a real-repo fd hazard (see
Evidence). Nothing here is awaiting a decision.

The ticket is retained as the durable record of why the selection is what it
is - the kqueue finding below is the load-bearing reason, and it is not
written down anywhere else at this length. It carries no open work.

## Correction: the original "five reds" diagnosis was wrong

This ticket originally recorded fsevent as 6/11, and attributed the five reds
to `work_root_watch.rs` being written against a per-directory event model
while "FSEvents delivers a coalesced recursive stream". **Both halves of that
were false**, and it is recorded here rather than deleted because the false
version briefly drove a proposed fix direction (rewriting the invalidation
logic against a stream shape that does not exist).

- **FSEvents is not coalescing here.** notify creates the stream with
  `kFSEventStreamCreateFlagFileEvents | kFSEventStreamCreateFlagNoDefer` at
  latency `0.0` (`notify-8.2.0/src/fsevent.rs:300-301`) - per-FILE event
  paths, delivered without deferral. That is precisely the shape
  `work_root_watch.rs`'s invalidation keys on.
- **The five reds were a defect in the test fixture, not in the backend.**
  `tests/git_watch.rs`'s `armed_fixture_with_config` built its
  `WatchTargets { worktree, git_dir, common_dir }` straight from
  `std::env::temp_dir()`, skipping the `canonical_or_normalized` pass that
  every production caller goes through (`discovery.rs`'s
  `watch_reconcile_entry_for` routes all three fields through it, and
  `watch_key` shares the same chain). On macOS `std::env::temp_dir()` is
  `/var/folders/<...>/T`, a symlink to `/private/var/folders/<...>/T`.
  notify canonicalizes a watched root internally, so every delivered event
  path read `/private/var/...` while the registry held `/var/...` as the
  owning root; `owners_for_path`'s `starts_with` prefix scan matched no
  owner, no epoch bumped, and each positive-bump assertion ran out its 5s
  deadline.
- **Canonicalizing the fixture root took the suite to 11/11**, with **no
  production change** - the invalidation logic was never at fault.

Corroboration that the fixture, not the backend, was the variable:

- All five reds were `armed_fixture` cases asserting a POSITIVE bump. The
  `armed_fixture` cases that "passed" asserted negatives or spawn counts,
  both of which hold vacuously under a total owner miss.
- The one positive-bump case that passed throughout
  (`a_route_driven_branch_create_on_one_worktree_bumps_a_sibling_linked_worktrees_refs`)
  is the only one built through `full_router_app_state()` and the real,
  canonicalizing discovery path.
- kqueue's earlier 11/11 under the same broken fixture is consistent with
  this: the mismatch only bites a backend that canonicalizes the watched
  root before reporting event paths.
- The same `/var` -> `/private/var` symlink was independently traced as the
  cause of the separate, pre-existing `discovery.rs:1213` failure.

Fixed in `tests/git_watch.rs` by canonicalizing the fixture root at creation
(`init_git_repo_at_canonical_path`), applied to `armed_fixture_with_config`
and to the three inline fixtures that had copied the same pattern. The
gitignored-write case's two negative assertions were vacuous before this and
now hold for real.

## Evidence: why fsevent and not kqueue

Both measured on the same worktree with everything else identical.

### `macos_fsevent` - full green, no fd cost

- `cargo test -p ws-dashboard-daemon --test git_watch`: **11 passed, 0
  failed** (2026-07-28, with the fixture repair above).
- One FSEvents stream per watched root. No per-entry, and no per-directory,
  file-descriptor cost.
- Per-file event paths, no coalescing (`fsevent.rs:300-301` flags above).

### `macos_kqueue` - equal pass rate, per-ENTRY fd cost

- `--test git_watch` 11/11 as well, measured under the older
  un-canonicalized fixture and not re-measured since (nothing turns on it
  now). Either way the green was always a fixture-SCALE artifact:
  `git_watch.rs` builds tiny throwaway repos, and a real work root is four
  orders of magnitude larger.
- notify's kqueue backend has **no recursive kernel primitive**. It emulates
  one: `add_watch` with `is_recursive` WalkDirs the entire subtree and calls
  `add_single_watch` on **every entry, files included, with no ignore
  filter** (`notify-8.2.0/src/kqueue.rs:295-316`).
  `kqueue::Watcher::add_filename` documents that it opens the file and holds
  the descriptor (`kqueue-1.2.0/src/watcher.rs:93`). The cost is **one fd per
  filesystem entry**, not the "one fd per watched directory" the original
  Cargo.toml comment claimed.
- Scale on this worktree: `find . | wc -l` = **154,423** entries (1,781 with
  `node_modules` and `target` pruned). `launchctl limit maxfiles` on this
  machine reports a **256** soft limit.
- The mitigations that exist are Linux-only. `max_dirs` is enforced only in
  the Linux plan/apply and Linux incremental paths, and `do_arm` *computes*
  an `IgnoreSet` that it then never passes to `do_arm_recursive` - so the
  `node_modules`/`target` exclusion machinery would be inert for macOS
  registration. (This is not a live defect under fsevent, which registers one
  recursive stream and needs no registration-time pruning; the computed
  `IgnoreSet` is still applied on the event-classification path, which is
  what the now-non-vacuous gitignored-write case proves.)
- Failure would not be clean. `do_arm_recursive`'s unwind unwatches only the
  previously *successful* roots, so a root that failed mid-WalkDir would keep
  its partially-added watches and their fds until the whole watcher drops. fd
  exhaustion is process-global: it starves HTTP accept, helper Unix sockets,
  and `git_exec` spawns, not just the watcher.
- Production reaches this path by default (`WatchConfig::default()` is
  `WatchMode::Auto`; `tests/routes.rs` pins `WatchMode::Off`, which is why
  the route suite would not have seen it).

Choosing kqueue would have required threading the `IgnoreSet` into
`do_arm_recursive`, adding a `max_dirs`-equivalent macOS cap, and fixing the
unwind leak - all to reach a still-unbounded per-entry cost. fsevent needs
none of it.

## Deliberately not in scope

- Linux and Windows. inotify and ReadDirectoryChangesW are unaffected; the
  feature selection is `cfg`-inert on both.
- `discovery.rs:1213`. It shares the `/var` -> `/private/var` root cause but
  is a production-side bug on the dev line, not a fixture defect, and is not
  owned here.


## Resolution (2026-07-28)

Opened during Phase 2 of `260727-chore-merge-ws-dashboard-dev-into-goal-branch` to defer a macOS fs-watch backend decision, and closed in the same phase because the decision turned out not to need deferring. `macos_fsevent` is selected, `tests/git_watch.rs` is 11/11 on macOS, and no production code changed to get there.

The ticket is closed rather than dropped because its content is a settled decision with its evidence, not abandoned scope. Two things in it are worth not re-deriving: kqueue's cost is one file descriptor per filesystem ENTRY (notify emulates recursion with an unfiltered WalkDir), which is why it is not the backend even though it also passed the suite; and the five reds that originally looked like a backend limitation were a test-fixture defect - `armed_fixture_with_config` skipped the `canonical_or_normalized` pass every production caller makes, so on macOS the registry held `/var/...` while notify reported `/private/var/...` and `owners_for_path` matched nothing.

The false diagnosis this ticket briefly carried ("FSEvents delivers a coalesced recursive stream") is retained in the body as a correction rather than deleted, because it had already driven a proposed fix direction that would have rewritten invalidation logic against a stream shape that does not exist.

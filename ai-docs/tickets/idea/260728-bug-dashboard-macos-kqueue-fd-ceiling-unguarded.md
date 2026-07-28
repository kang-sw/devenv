---
title: "dashboard fs watch: the macOS backend selection is unresolved - kqueue costs one fd per filesystem entry, fsevent loses five git_watch tests"
related:
  260727-chore-merge-ws-dashboard-dev-into-goal-branch: surfaced-by
  260726-refactor-ws-dashboard-git-fs-watch-invalidation: subject
---

## Stem note

The stem `260728-bug-dashboard-macos-kqueue-fd-ceiling-unguarded` was
minted while `macos_kqueue` was still the shipping choice, so it now reads
narrower than the ticket's actual subject. Stems are immutable absolute
references (history is queried by `git log --grep=<stem>`), so it stays.
Read it as "the macOS fs-watch backend question", not as "kqueue ships and
needs a guard".

## Symptom

`ws-dashboard/crates/daemon` currently builds macOS fs-watching on
`notify`'s `macos_fsevent`, and five of `tests/git_watch.rs`'s eleven tests
are red on macOS as a result. That is a deliberate, visible state, not a
settled one: **neither macOS backend notify offers is actually good enough
for what `work_root_watch.rs` needs**, and this ticket exists so the choice
gets made on its own evidence rather than inside an unrelated merge.

Surfaced while resolving Phase 2 of
`260727-chore-merge-ws-dashboard-dev-into-goal-branch`. The dev side's
`notify = { version = "8", default-features = false }` does not build on
macOS at all - notify gates its fsevent module on
`not(feature = "macos_kqueue")` rather than on `macos_fsevent`, so dropping
default features removes the `fsevent-sys` crate while still compiling the
module that imports it. Some feature must be named. The merge first named
`macos_kqueue` on test-pass-rate evidence; review found that evidence was
measured at a scale that hides the backend's real cost, and the merge was
corrected to `macos_fsevent` (notify's own default, i.e. the minimum change
that makes the `default-features = false` intent build). The backend
decision itself is deferred here.

## Evidence

Both measurements, taken on the same worktree with everything else
identical. Carry both forward; neither number alone decides anything.

### `macos_kqueue` - green suite, per-ENTRY fd cost

- `cargo test -p ws-dashboard-daemon --test git_watch`: **11 passed, 0
  failed**.
- That green is a fixture-scale artifact. `git_watch.rs` builds tiny
  throwaway repos; a real work root is four orders of magnitude larger.
- notify's kqueue backend has **no recursive kernel primitive**. It
  emulates one: `add_watch` with `is_recursive` WalkDirs the entire subtree
  and calls `add_single_watch` on **every entry, files included, with no
  ignore filter** (search `add_single_watch` in `notify`'s `src/kqueue.rs`).
  `kqueue::Watcher::add_filename` documents that it opens the file and
  holds the descriptor. So the cost is **one fd per filesystem entry**, not
  the "one fd per watched directory" the original Cargo.toml comment
  claimed.
- Scale on this worktree: `find . | wc -l` = **154,423** entries (1,781
  with `node_modules` and `target` pruned). `launchctl limit maxfiles` on
  this machine reports a **256** soft limit.
- The mitigations that exist are Linux-only. `max_dirs` is enforced only in
  the Linux plan/apply and Linux incremental paths, and `do_arm` *computes*
  an `IgnoreSet` that it then never passes to `do_arm_recursive` - so the
  `node_modules`/`target` exclusion machinery is entirely inert for macOS
  registration.
- Failure is not clean. `do_arm_recursive`'s unwind unwatches only the
  previously *successful* roots, so the root that failed mid-WalkDir keeps
  its partially-added watches and their fds until the whole watcher drops.
  fd exhaustion is process-global: it starves HTTP accept, helper Unix
  sockets, and `git_exec` spawns, not just the watcher.
- Production reaches this path by default (`WatchConfig::default()` is
  `WatchMode::Auto`; `tests/routes.rs` pins `WatchMode::Off`, which is why
  the route suite cannot see it).

### `macos_fsevent` - no fd cost, five real reds

- `cargo test -p ws-dashboard-daemon --test git_watch`: **6 passed, 5
  failed**. One FSEvents stream per root; no per-entry fd cost at all.
- The five reds, with their assertion text (measured 2026-07-28):
  - `writing_an_untracked_file_bumps_worktree_epoch_only` - "an untracked
    file write must bump the worktree epoch within 5s"
  - `git_switch_dash_c_bumps_refs_epoch` - "git switch -c must bump refs
    within 5s"
  - `git_worktree_add_bumps_refs_epoch_on_the_primary_root` - "git worktree
    add must bump the primary root's refs within 5s"
  - `a_directory_created_after_arming_gets_registered_and_a_write_inside_it_bumps`
    - "a write inside a directory created after arming must bump worktree
    within 5s"
  - `mkdir_and_write_in_one_step_still_bumps_via_the_parent_directory_event`
    - "mkdir+write in one step must still bump worktree within 5s"
- These are **not flaky timeouts to be re-run away**. They are one
  substantive finding: `work_root_watch.rs` is written against a
  per-directory event model - it registers per-directory watches and
  expects a distinct event per directory, including for directories that
  appear after arming. FSEvents delivers a **coalesced recursive stream**
  instead, so the per-directory events the invalidation logic keys on never
  arrive, and no epoch bumps. Read the last two test names above: they name
  exactly the post-arming-directory case that coalescing erases.

## Fix direction (not decided)

The real question is which of these the project wants; each has a different
owner and cost.

1. **Adapt `work_root_watch.rs` to FSEvents' event model on macOS** -
   stop keying invalidation on per-directory events and derive epoch bumps
   from the coalesced stream's paths. Keeps zero fd cost. Costs a real
   change to the invalidation logic, which is the subject of
   `260726-refactor-ws-dashboard-git-fs-watch-invalidation`.
2. **Ship kqueue and make `do_arm_recursive` macOS-aware in the same
   change** - thread the already-computed `IgnoreSet` through, apply a
   `max_dirs`-equivalent cap on macOS, correct `do_arm_recursive`'s "no
   walk, no cap" doc comment (true under FSEvents, false under kqueue), and
   unwatch the failing root on the unwind path. Note that even fully
   ignore-filtered, this is still per-entry, and the pruned figure above
   (1,781) is one worktree's, not a bound.
3. **Neither backend; watch a narrow path set on macOS** - e.g. only
   `.git`/refs plus a shallow tracked-file frontier, accepting reduced
   coverage in exchange for a bounded fd count and no reliance on
   per-directory delivery.

Before choosing, measure two unknowns: the real fd cost of arming a
realistic work root under kqueue *with* the ignore set applied, and where
the daemon's soft `maxfiles` limit actually sits after launchd/shell
inheritance. Option 2 is only viable if that pair leaves headroom.

## Deliberately not in scope

- Changing the current `macos_fsevent` selection without doing one of the
  three above. The five reds are the honest visible state and must not be
  turned green by swapping backends again.
- Linux and Windows. inotify and ReadDirectoryChangesW are unaffected; the
  feature selection is `cfg`-inert on both.

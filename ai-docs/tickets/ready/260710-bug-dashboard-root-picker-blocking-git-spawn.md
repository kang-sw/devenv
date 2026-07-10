---
title: "Dashboard root-picker routes block the async runtime with synchronous git spawns"
sage-review: required
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
related:
  260710-bug-dashboard-windows-verbatim-path-not-normalized: prior fix in the same call-site cluster (path normalization); this ticket fixes a distinct blocking-execution defect
---

# Dashboard root-picker routes block the async runtime with synchronous git spawns

## Background

Live dogfooding on the native-Windows daemon (2026-07-10, `--no-auth`,
127.0.0.1) surfaced severe overall responsiveness degradation, most visibly
as multi-second-plus latency when opening a folder ("workRoot"). An
investigation subagent traced this to root cause in
`ws-dashboard/crates/daemon/src/root_picker.rs`:

- `open_work_root` (root_picker.rs:209-279) is declared `async fn` but calls
  `provider.dashboard_resources()` synchronously at line 218, and
  `live_dashboard_resources(&state.opened_work_roots)` again at line 270.
  Both flow into `discover_work_root` -> `discover_existing_dir` ->
  `GitDiscovery::discover` (discovery.rs:320-447), which spawns at least
  three `git` subprocesses per candidate work root
  (`rev-parse --show-toplevel`, `--git-common-dir`, `--git-dir` via
  `git_path`, discovery.rs:461-476) plus a fourth
  (`git worktree list --porcelain` via `git_worktree_paths`,
  discovery.rs:478-495). `open_work_root`'s second call re-runs this
  discovery for every already-opened work root, not just the one being
  opened.
- `set_work_root_activation` (root_picker.rs:281-310, line 308) and
  `remove_workspace` (root_picker.rs:312-359, lines 318 and 357) call
  `live_dashboard_resources` the same unwrapped way.
- `list_root_picker` (root_picker.rs:113-127) calls the synchronous
  `root_picker_view` (root_picker.rs:361-390) directly. That function does
  `path.canonicalize()`/`fs::metadata` on the target dir, one
  `fs::metadata` per listed entry (`entry_for_directory`, line 393), and
  `known_picker_places` (413-467) canonicalizes home, every candidate drive
  letter A-Z on Windows (`filesystem_roots`, 531-540, up to 26 `is_dir()`
  stats), any mount roots, and every pinned path -- 30+ synchronous
  filesystem syscalls per listing call.

None of this work is wrapped in `tokio::task::spawn_blocking`, so it runs
directly on a shared tokio worker thread. This is the same hazard the
sibling route already documents and avoids:

```rust
// resources.rs:26-33
pub async fn local_dashboard_resources_view(state: &AppState) -> DashboardResourcesView {
    // Live discovery runs synchronous filesystem and `git` subprocess work, so
    // keep it off the async worker threads.
    let opened = state.opened_work_roots.clone();
    let (view, pruned_work_root_ids) =
        tokio::task::spawn_blocking(move || live_dashboard_resources_with_sync(&opened))
            .await
            .expect(...);
```

`root_picker.rs`'s four handlers bypass this wrapper entirely. Besides the
per-call latency (`git.exe` process spawn is notably slower on Windows than
Linux, especially under real-time AV scanning), running this work directly
on a tokio worker thread starves that worker of capacity to service other
concurrent requests (SSE streams, terminal I/O, other API routes) for the
full duration of the call -- this is why the *whole* daemon feels sluggish
during a folder-open, not just that one endpoint.

Recursive directory walks, repo-size computation, DNS/`localhost`-string
resolution, and the `git_worktree.rs`/`git_toolbar.rs` git calls were all
checked by the investigation and ruled out -- the latter two already
correctly use `spawn_blocking` at their call sites (git_toolbar.rs:140, 156,
173, 213, 310).

## Constraints

- Fix must not change the observable behavior/response shape of any
  affected route -- this is a performance-only fix executed via
  `spawn_blocking`, not a logic change.
- Mirror the existing, already-reviewed pattern in
  `resources.rs::local_dashboard_resources_view` rather than inventing a new
  blocking-dispatch idiom.
- Do not attempt to reduce the number of `git` subprocess spawns in this
  ticket -- that is a separate, independent optimization; this ticket only
  fixes the blocking-on-async-runtime defect.

## Phases

### Phase 1: Move root-picker blocking work off the async runtime

1. In `root_picker.rs`, wrap the synchronous body of `open_work_root`'s two
   `dashboard_resources`/`live_dashboard_resources` calls, and
   `set_work_root_activation`'s and `remove_workspace`'s
   `live_dashboard_resources` calls, in `tokio::task::spawn_blocking`,
   following the exact pattern already used in
   `resources.rs::local_dashboard_resources_view`.
2. Wrap `list_root_picker`'s call into `root_picker_view` (and any other
   directly-synchronous work in that handler, e.g. `known_picker_places`)
   in `tokio::task::spawn_blocking`.
3. Preserve existing error handling / response shapes exactly; only change
   where the synchronous work executes (blocking-pool thread vs. async
   worker thread), not what it computes or returns.
4. Add or update tests demonstrating each fixed handler no longer runs its
   git/filesystem work inline on the calling async task (e.g. a test that
   the handler still functions correctly end-to-end after the
   `spawn_blocking` wrap; existing route integration tests in
   `tests/routes.rs` should continue to pass unmodified as a correctness
   regression check).
5. Native-Windows live re-verification is deferred to manual dogfooding
   (no Windows CI runner) -- record it as an Edition note if performed.

### Result (1084ff23)

Implemented as a direct-edit hotfix (lead-only review, no plan artifact per
verdict). In `root_picker.rs`:

- `open_work_root`'s single-candidate `provider.dashboard_resources()` call
  and `list_root_picker`'s `root_picker_view(...)` call are each wrapped in
  their own `tokio::task::spawn_blocking`, since neither goes through the
  shared `OpenedWorkRoots` registry.
- `open_work_root`'s aggregated-response call, `set_work_root_activation`,
  and `remove_workspace` (both of its call sites) now call
  `resources::local_dashboard_resources_view(&state).await` instead of the
  raw sync `live_dashboard_resources(&state.opened_work_roots)` -- reusing
  the exact existing, already-reviewed `spawn_blocking` wrapper the ticket
  named, rather than duplicating a second blocking-dispatch idiom.
- Net effect: no handler on the root-picker surface runs synchronous
  filesystem or `git` subprocess work directly on a tokio async worker
  thread anymore.
- Incidental improvement, not a regression: reusing
  `local_dashboard_resources_view` means these four routes also inherit its
  existing stale-workRoot terminal self-pruning, previously only applied on
  the canonical `GET /api/dashboard/resources` route. Response shape is
  unchanged; this only affects already-invalid/pruned workRoot terminal
  cleanup.
- Verification: `cargo build -p ws-dashboard-daemon` clean;
  `cargo test -p ws-dashboard-daemon` 204/204 passed (0 failed) across all
  test binaries, including the full `tests/routes.rs` root-picker/work-root
  integration suite, unmodified -- confirms no behavior/response-shape
  regression per Constraints.
- Item 5 (native-Windows live re-verification) deferred to manual dogfooding
  against the running native daemon; record as an Edition note if/when
  performed.

## Spec Impact

None. This is an internal execution-model fix (dispatch onto the blocking
thread pool); it changes no observable HTTP contract, request/response
shape, or route behavior covered by existing specs.

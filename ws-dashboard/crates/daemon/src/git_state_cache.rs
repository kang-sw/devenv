//! Result cache for `/git/status` and `/git/branches` (Phase 3 of the
//! git-state-cache ticket; epoch source stubbed, watcher lands in Phase 4).
//!
//! Two independently revalidated slots per `WatchKey` (`worktree`, `refs`),
//! each gated by a compound `(epoch, ttl)` check rather than TTL alone: a
//! slot is valid only when its stored epoch equals the epoch sampled for
//! this read AND its age is under the TTL. Mirrors `discovery::ProbeSlots`'s
//! locking discipline (map lock released before the per-key lock is
//! acquired, so the map is never held across a `git` spawn) but is
//! deliberately its own type rather than a `ProbeSlots<GitCacheSlot>`
//! instantiation - see `GitStateCache`'s doc comment for why.

use std::collections::HashMap;
use std::env;
use std::sync::{Arc, Mutex, OnceLock};
use std::time::{Duration, Instant};

use crate::discovery::WatchKey;
use crate::git_toolbar::{GitBranchEntry, GitChangeSummary, GitSyncSummary};

const DEFAULT_GIT_CACHE_TTL_MS: u64 = 2000;

static GIT_CACHE_TTL: OnceLock<Duration> = OnceLock::new();

/// TTL for a warm `GitStateCache` slot under a stable epoch, read from
/// `WS_DASHBOARD_GIT_CACHE_TTL_MS` (default 2000ms) once per process and
/// cached in a `OnceLock`, matching `git_exec::git_timeout_from_env`'s
/// pattern exactly (R4, Phase 3 review adjudication - the prior per-request
/// `env::var` read diverged from that precedent while claiming to follow it).
/// That in turn mirrors `discovery::git_probe_ttl_from_env`, which is read
/// once into `GitProbeCache::default`. At the frontend's 5s poll interval
/// every steady-state tick still misses this TTL by design (Phase 3 D6): with
/// no watcher yet, a short ceiling is what keeps stale git state from being
/// served. This phase's spawn reduction comes from intra-tick de-duplication
/// (D1) and single-flight burst coalescing (D2), not from TTL hits; the
/// TTL-driven win arrives only with Phase 4's armed, much longer ceiling.
///
/// The TTL itself stays a per-call `Duration` parameter on
/// `GitStateCache::worktree`/`refs` (not a field on `GitStateCache`): that is
/// the injection surface Phase 4's two TTLs (120s armed / 2s degraded) will
/// select at the call site. This function only changes how the *default*
/// value is read, not how it is threaded through.
pub(crate) fn git_cache_ttl_from_env() -> Duration {
    *GIT_CACHE_TTL.get_or_init(|| {
        let millis = env::var("WS_DASHBOARD_GIT_CACHE_TTL_MS")
            .ok()
            .and_then(|raw| raw.trim().parse::<u64>().ok())
            .unwrap_or(DEFAULT_GIT_CACHE_TTL_MS);
        Duration::from_millis(millis)
    })
}

/// Pluggable per-key mutation-epoch source for `GitStateCache`'s two
/// independently invalidated axes (`worktree`, `refs`).
///
/// Same idiom as `CodexWorkRootResolver`/`ClaudeWorkRootResolver`: a stable
/// `Arc<dyn EpochSource>` field on `AppState`, injected once at construction
/// (`server.rs` for production, a stub for tests). Phase 4 adds the `notify`
/// FS-event pipeline as a second writer to the same concrete store; it does
/// not change this trait or any `git_toolbar.rs` call site.
pub trait EpochSource: Send + Sync {
    /// `(worktree_epoch, refs_epoch)` for `key`. Callers MUST sample this
    /// once, before either `GitStateCache` fill closure runs, and stamp the
    /// filled slot with that pre-fill sample - not a value read after the
    /// `git` spawn returns. This is what lets a concurrent bump that lands
    /// mid-spawn be caught as a miss on the *next* read instead of being
    /// silently blessed into the slot the in-flight fill is about to write.
    fn epochs(&self, key: &WatchKey) -> (u64, u64);

    /// Bumped by the switch/create-branch and fetch/push/pull-ff-only
    /// mutating routes so a user action is never TTL-delayed.
    fn bump_worktree(&self, key: &WatchKey);

    /// See `bump_worktree`. `git_pull_ff_only` bumps both axes; the other
    /// mutating routes bump only this one.
    fn bump_refs(&self, key: &WatchKey);
}

/// The ticket's Phase 3 stub names this source `StaticZero`, which reads as
/// "epochs are hardcoded to the constant 0 forever" - that literal reading is
/// wrong and would make the ticket's own Phase 3 acceptance test
/// unimplementable ("`POST /git/switch-branch` and assert the next
/// `/git/status` reflects the new branch immediately, epoch bump beats TTL").
/// This type holds real per-key mutable counters; "static" in the ticket's
/// name describes the absence of a background/watcher-driven writer in this
/// phase, not a literal constant. Named `MutationEpochSource` here instead,
/// to describe what it actually does: the only writers in this phase are the
/// explicit mutating-route `bump_*` calls. Phase 4 adds the FS-event pipeline
/// as a second writer to this same store, without touching any
/// `git_toolbar.rs` call site (Phase 3 Lead Disposition D3).
#[derive(Debug, Default)]
pub struct MutationEpochSource {
    epochs: Mutex<HashMap<WatchKey, (u64, u64)>>,
}

impl EpochSource for MutationEpochSource {
    fn epochs(&self, key: &WatchKey) -> (u64, u64) {
        self.epochs
            .lock()
            .expect("mutation epoch source lock poisoned")
            .get(key)
            .copied()
            .unwrap_or((0, 0))
    }

    fn bump_worktree(&self, key: &WatchKey) {
        let mut epochs = self
            .epochs
            .lock()
            .expect("mutation epoch source lock poisoned");
        epochs.entry(key.clone()).or_insert((0, 0)).0 += 1;
    }

    fn bump_refs(&self, key: &WatchKey) {
        let mut epochs = self
            .epochs
            .lock()
            .expect("mutation epoch source lock poisoned");
        epochs.entry(key.clone()).or_insert((0, 0)).1 += 1;
    }
}

/// The union of what `/git/status` and `/git/branches` each independently
/// computed before this phase - the refs-slot fill value, shared by both
/// routes (Phase 3 Lead Disposition D1). Today each 5s poll tick pays for
/// both handlers computing their own refs state independently
/// (`branch --show-current`, the detached `rev-parse --short HEAD`, and the
/// current-branch `rev-list --left-right --count` each run twice); one
/// shared refs fill collapses those duplicates on every tick, cold or warm.
///
/// Deliberately NOT narrowed to whichever route asks first: a narrower slot
/// would silently break the "second call adds zero spawns" guarantee for
/// whichever route did not fill it.
#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub(crate) struct RefState {
    pub(crate) branch_name: Option<String>,
    pub(crate) detached_oid: Option<String>,
    pub(crate) upstream: Option<String>,
    pub(crate) sync: GitSyncSummary,
    pub(crate) branch_list: Vec<GitBranchEntry>,
    // R13 (Phase 3 review adjudication): a `checked_out: BTreeSet<String>`
    // field was here, populated in `compute_ref_state` but never read off a
    // `RefState`/`refs` value anywhere (`checked_out_branches(root, stats)`'s
    // return is consumed locally, before being folded into each
    // `GitBranchEntry.checked_out`). Deleted: dead cached state is stale-data
    // surface no test can observe.
}

#[derive(Default)]
struct GitCacheSlot {
    worktree: Option<(u64, Instant, GitChangeSummary)>,
    refs: Option<(u64, Instant, RefState)>,
}

/// Result cache for `/git/status`/`/git/branches`, keyed by `WatchKey`.
///
/// Cheap to clone (`Arc`-backed map); lives in `AppState` so every route
/// shares one cache, same convention as `GitProbeCache`.
///
/// NOT built as `discovery::ProbeSlots<GitCacheSlot>`, for two reasons:
/// (1) `ProbeSlots::get_or_probe`'s validity check is TTL-only
/// (`probed_at.elapsed() < ttl`) with no parameter for an externally-supplied
/// epoch comparator, but this cache's check is compound
/// (`stored_epoch == epoch && age < ttl`); (2) `GitCacheSlot` has two
/// *independently* revalidated parts (`worktree`, `refs`) with different
/// epoch kinds and different value types - a single generic `T` would force
/// `T = GitCacheSlot` and lose the ability to revalidate/fill one part while
/// reusing the other, which is the entire point of the ticket's two epochs.
///
/// It DOES mirror `ProbeSlots`'s locking discipline exactly: the map lock in
/// `slot_for` is released before the per-key lock is acquired, so the map is
/// never held across a `git` spawn. Under Phase 3 D2 (the two routes'
/// concurrent per-tick `Promise.all` miss a cold slot at the same moment;
/// what collapses them into one fill is the per-key lock serializing the
/// second request behind the first) this discipline is a correctness
/// requirement of this phase, not a stylistic echo of `ProbeSlots`.
#[derive(Clone, Default)]
pub struct GitStateCache {
    slots: Arc<Mutex<HashMap<WatchKey, Arc<Mutex<GitCacheSlot>>>>>,
}

impl GitStateCache {
    fn slot_for(&self, key: &WatchKey) -> Arc<Mutex<GitCacheSlot>> {
        let mut slots = self
            .slots
            .lock()
            .expect("git state cache slot map lock poisoned");
        slots.entry(key.clone()).or_default().clone()
    }

    /// `epoch` MUST be sampled by the caller (via `EpochSource::epochs`)
    /// before `probe` runs; the returned value is stamped with that
    /// pre-sampled epoch, never one read after `probe` returns.
    pub(crate) fn worktree(
        &self,
        key: &WatchKey,
        epoch: u64,
        ttl: Duration,
        probe: impl FnOnce() -> GitChangeSummary,
    ) -> GitChangeSummary {
        let slot = self.slot_for(key);
        let mut guard = slot.lock().expect("git state cache slot lock poisoned");
        if let Some((cached_epoch, probed_at, value)) = guard.worktree.as_ref() {
            if *cached_epoch == epoch && probed_at.elapsed() < ttl {
                return value.clone();
            }
        }
        let value = probe();
        guard.worktree = Some((epoch, Instant::now(), value.clone()));
        value
    }

    /// See `worktree`'s epoch-sampling contract; identical shape for the
    /// refs slot.
    pub(crate) fn refs(
        &self,
        key: &WatchKey,
        epoch: u64,
        ttl: Duration,
        probe: impl FnOnce() -> RefState,
    ) -> RefState {
        let slot = self.slot_for(key);
        let mut guard = slot.lock().expect("git state cache slot lock poisoned");
        if let Some((cached_epoch, probed_at, value)) = guard.refs.as_ref() {
            if *cached_epoch == epoch && probed_at.elapsed() < ttl {
                return value.clone();
            }
        }
        let value = probe();
        guard.refs = Some((epoch, Instant::now(), value.clone()));
        value
    }

    /// Drop every cached slot for every root (R3, Phase 3 review
    /// adjudication). Called from `git_worktree.rs`'s `git worktree add`/
    /// `remove` handlers, alongside the pre-existing
    /// `GitProbeCache::clear()` call there, for the same reason: this daemon
    /// just changed the worktree/branch set outside the mutating
    /// `git_toolbar.rs` routes (which invalidate via `EpochSource` bumps),
    /// so the cached `worktree list --porcelain` (`checked_out`/
    /// `disabledReason`) and `refs/heads` (`branch_list`) answers are stale.
    ///
    /// Repo-wide, not per-key: the epoch/slot is keyed per worktree path
    /// (`WatchKey`), but `refs/heads` and `worktree list --porcelain` are
    /// repository-wide, so a per-key clear would leave sibling worktrees'
    /// cached `branch_list`/`checked_out` stale (carried forward to Phase 4,
    /// which is where the refs axis should be keyed by common dir instead).
    /// Also closes the slot map's unbounded growth (one entry per `WatchKey`
    /// for the daemon's lifetime, with no prior eviction path), matching
    /// `GitProbeCache`'s `evict`/`clear` pair.
    pub(crate) fn clear(&self) {
        self.slots
            .lock()
            .expect("git state cache slot map lock poisoned")
            .clear();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::Path;
    use std::sync::atomic::{AtomicU64, Ordering};

    fn key(path: &str) -> WatchKey {
        crate::discovery::watch_key(Path::new(path))
    }

    /// R12 (Phase 3 review adjudication): split from a single combined
    /// hit+expiry test into this hit-only leg plus
    /// `worktree_slot_is_a_miss_once_a_short_ttl_elapses` below. A long TTL
    /// removes the former 150ms hit-vs-expiry margin as a flakiness source -
    /// this leg cannot spuriously miss on a loaded box the way a
    /// short-margin hit could.
    #[test]
    fn worktree_slot_is_a_hit_under_a_fixed_epoch_within_a_long_ttl() {
        let cache = GitStateCache::default();
        let k = key("/tmp/ws-dashboard-git-state-cache-ttl-hit-fixture");
        let calls = AtomicU64::new(0);
        let ttl = Duration::from_secs(60);

        let first = cache.worktree(&k, 0, ttl, || {
            calls.fetch_add(1, Ordering::SeqCst);
            GitChangeSummary {
                added_lines: 1,
                ..Default::default()
            }
        });
        assert_eq!(first.added_lines, 1);
        assert_eq!(calls.load(Ordering::SeqCst), 1);

        // Same epoch, still within TTL: hit, probe not called again.
        let second = cache.worktree(&k, 0, ttl, || {
            calls.fetch_add(1, Ordering::SeqCst);
            GitChangeSummary {
                added_lines: 99,
                ..Default::default()
            }
        });
        assert_eq!(
            second.added_lines, 1,
            "must return the cached value, not re-probe"
        );
        assert_eq!(calls.load(Ordering::SeqCst), 1);
    }

    /// See `worktree_slot_is_a_hit_under_a_fixed_epoch_within_a_long_ttl`'s
    /// doc comment (R12). A short TTL plus a short sleep - a fraction of the
    /// former combined test's 200ms - pins only the expiry boundary.
    #[test]
    fn worktree_slot_is_a_miss_once_a_short_ttl_elapses() {
        let cache = GitStateCache::default();
        let k = key("/tmp/ws-dashboard-git-state-cache-ttl-expiry-fixture");
        let calls = AtomicU64::new(0);
        let ttl = Duration::from_millis(20);

        let first = cache.worktree(&k, 0, ttl, || {
            calls.fetch_add(1, Ordering::SeqCst);
            GitChangeSummary {
                added_lines: 1,
                ..Default::default()
            }
        });
        assert_eq!(first.added_lines, 1);
        assert_eq!(calls.load(Ordering::SeqCst), 1);

        std::thread::sleep(Duration::from_millis(40));
        let second = cache.worktree(&k, 0, ttl, || {
            calls.fetch_add(1, Ordering::SeqCst);
            GitChangeSummary {
                added_lines: 2,
                ..Default::default()
            }
        });
        assert_eq!(
            second.added_lines, 2,
            "TTL elapsed under the same epoch must re-probe"
        );
        assert_eq!(calls.load(Ordering::SeqCst), 2);
    }

    /// R5 (Phase 3 review adjudication), mirroring
    /// `discovery::git_probe_memo_single_flights_concurrent_misses_for_one_key`:
    /// two threads race into `GitStateCache::refs` for the same key at the
    /// same epoch, the first probe sleeping to force an overlap window, and
    /// only ONE probe call must be observed. This is the layer at which D2's
    /// single-flight guarantee actually lives (the map lock in `slot_for` is
    /// released before the per-key lock is taken, and the per-key lock is
    /// held across `probe()`) - the integration test in `tests/routes.rs`
    /// (`git_toolbar_status_and_branches_concurrent_cold_miss_share_one_refs_fill`)
    /// pins D1 (the two routes share one refs fill) but is equally satisfied
    /// by a purely serial execution, so it does not by itself discriminate
    /// single-flight from an ordinary sequential TTL hit.
    #[test]
    fn refs_slot_single_flights_concurrent_misses_for_one_key() {
        let cache = GitStateCache::default();
        let k = key("/tmp/ws-dashboard-git-state-cache-single-flight-fixture");
        let probes = Arc::new(AtomicU64::new(0));
        let ttl = Duration::from_secs(60);

        let handles: Vec<_> = (0..2)
            .map(|_| {
                let cache = cache.clone();
                let k = k.clone();
                let probes = Arc::clone(&probes);
                std::thread::spawn(move || {
                    cache.refs(&k, 0, ttl, || {
                        probes.fetch_add(1, Ordering::SeqCst);
                        std::thread::sleep(Duration::from_millis(100));
                        RefState {
                            branch_name: Some("single-flight".to_owned()),
                            ..Default::default()
                        }
                    })
                })
            })
            .collect();

        for handle in handles {
            let value = handle.join().expect("refs probe thread");
            assert_eq!(value.branch_name.as_deref(), Some("single-flight"));
        }

        assert_eq!(
            probes.load(Ordering::SeqCst),
            1,
            "concurrent misses for one key at one epoch must collapse into a single probe"
        );
    }

    /// Pins the cache layer's stamp contract only: a probe closure that
    /// bumps the epoch mid-execution (simulating a concurrent mutating route
    /// landing while this fill's `git` spawn is in flight) must NOT poison
    /// the slot with the post-bump epoch. The slot is stamped with the
    /// epoch sampled BEFORE the probe ran, so the very next read - which
    /// samples the now-bumped epoch - is a miss and re-probes.
    ///
    /// This does NOT pin D7's real requirement (`epoch_source.epochs(&key)`
    /// must run once, before either fill closure, at the CALLER) - this test
    /// samples the epoch itself and passes a plain `u64` in, and
    /// `GitStateCache::refs` holds no `EpochSource`, so it is structurally
    /// incapable of catching a caller that re-samples between the worktree
    /// and refs fills. That caller-level property is pinned by
    /// `status_for_path_samples_the_epoch_exactly_once`/
    /// `branches_for_path_samples_the_epoch_exactly_once` in
    /// `git_toolbar.rs` (R6, Phase 3 review adjudication).
    #[test]
    fn refs_fill_samples_the_epoch_before_the_probe_so_a_mid_fill_bump_forces_the_next_read_to_miss(
    ) {
        let cache = GitStateCache::default();
        let source = MutationEpochSource::default();
        let k = key("/tmp/ws-dashboard-git-state-cache-race-fixture");
        let ttl = Duration::from_secs(60);

        // Sample the epoch BEFORE the probe runs, per the contract every
        // real caller (status_for_path/branches_for_path) must follow.
        let (_, refs_epoch) = source.epochs(&k);
        assert_eq!(refs_epoch, 0);

        let first = cache.refs(&k, refs_epoch, ttl, || {
            // A concurrent mutating route bumps the epoch WHILE this fill's
            // git spawn is (conceptually) in flight.
            source.bump_refs(&k);
            RefState::default()
        });
        assert_eq!(first, RefState::default());

        let (_, refs_epoch_after) = source.epochs(&k);
        assert_eq!(
            refs_epoch_after, 1,
            "the bump inside probe() must already be visible"
        );

        let calls = AtomicU64::new(0);
        let second = cache.refs(&k, refs_epoch_after, ttl, || {
            calls.fetch_add(1, Ordering::SeqCst);
            RefState {
                branch_name: Some("recomputed".to_owned()),
                ..Default::default()
            }
        });
        assert_eq!(
            calls.load(Ordering::SeqCst),
            1,
            "the slot must have been stamped with the PRE-probe epoch (0), so \
             a read sampling the POST-bump epoch (1) is a miss and re-probes"
        );
        assert_eq!(second.branch_name.as_deref(), Some("recomputed"));
    }

    #[test]
    fn bump_worktree_and_bump_refs_invalidate_independently() {
        let cache = GitStateCache::default();
        let source = MutationEpochSource::default();
        let k = key("/tmp/ws-dashboard-git-state-cache-independent-fixture");
        let ttl = Duration::from_secs(60);

        let (wt0, refs0) = source.epochs(&k);
        cache.worktree(&k, wt0, ttl, || GitChangeSummary {
            added_lines: 1,
            ..Default::default()
        });
        cache.refs(&k, refs0, ttl, || RefState {
            branch_name: Some("main".to_owned()),
            ..Default::default()
        });

        source.bump_worktree(&k);
        let (wt1, refs1) = source.epochs(&k);
        assert_eq!(refs1, refs0, "bumping worktree must not move the refs epoch");

        let refs_calls = AtomicU64::new(0);
        let refs_after_wt_bump = cache.refs(&k, refs1, ttl, || {
            refs_calls.fetch_add(1, Ordering::SeqCst);
            RefState::default()
        });
        assert_eq!(
            refs_calls.load(Ordering::SeqCst),
            0,
            "refs slot must still be a hit after only the worktree epoch bumped"
        );
        assert_eq!(refs_after_wt_bump.branch_name.as_deref(), Some("main"));

        let wt_calls = AtomicU64::new(0);
        let wt_after_bump = cache.worktree(&k, wt1, ttl, || {
            wt_calls.fetch_add(1, Ordering::SeqCst);
            GitChangeSummary {
                added_lines: 2,
                ..Default::default()
            }
        });
        assert_eq!(
            wt_calls.load(Ordering::SeqCst),
            1,
            "worktree epoch bump must force a re-probe"
        );
        assert_eq!(wt_after_bump.added_lines, 2);
    }
}

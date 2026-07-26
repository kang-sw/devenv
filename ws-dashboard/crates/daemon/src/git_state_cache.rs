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

use std::collections::{BTreeSet, HashMap};
use std::env;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use crate::discovery::WatchKey;
use crate::git_toolbar::{GitBranchEntry, GitChangeSummary, GitSyncSummary};

const DEFAULT_GIT_CACHE_TTL_MS: u64 = 2000;

/// TTL for a warm `GitStateCache` slot under a stable epoch, read from
/// `WS_DASHBOARD_GIT_CACHE_TTL_MS` (default 2000ms, modeled on
/// `discovery::git_probe_ttl_from_env`). At the frontend's 5s poll interval
/// every steady-state tick still misses this TTL by design (Phase 3 D6): with
/// no watcher yet, a short ceiling is what keeps stale git state from being
/// served. This phase's spawn reduction comes from intra-tick de-duplication
/// (D1) and single-flight burst coalescing (D2), not from TTL hits; the
/// TTL-driven win arrives only with Phase 4's armed, much longer ceiling.
pub(crate) fn git_cache_ttl_from_env() -> Duration {
    let millis = env::var("WS_DASHBOARD_GIT_CACHE_TTL_MS")
        .ok()
        .and_then(|raw| raw.trim().parse::<u64>().ok())
        .unwrap_or(DEFAULT_GIT_CACHE_TTL_MS);
    Duration::from_millis(millis)
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
    pub(crate) checked_out: BTreeSet<String>,
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
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::Path;
    use std::sync::atomic::{AtomicU64, Ordering};

    fn key(path: &str) -> WatchKey {
        crate::discovery::watch_key(Path::new(path))
    }

    #[test]
    fn worktree_slot_is_a_ttl_hit_under_a_fixed_epoch_and_a_miss_once_ttl_elapses() {
        let cache = GitStateCache::default();
        let k = key("/tmp/ws-dashboard-git-state-cache-ttl-fixture");
        let calls = AtomicU64::new(0);
        let ttl = Duration::from_millis(150);

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

        std::thread::sleep(Duration::from_millis(200));
        let third = cache.worktree(&k, 0, ttl, || {
            calls.fetch_add(1, Ordering::SeqCst);
            GitChangeSummary {
                added_lines: 2,
                ..Default::default()
            }
        });
        assert_eq!(
            third.added_lines, 2,
            "TTL elapsed under the same epoch must re-probe"
        );
        assert_eq!(calls.load(Ordering::SeqCst), 2);
    }

    /// Pins the epoch-sample-before-fill correctness requirement: a probe
    /// closure that bumps the epoch mid-execution (simulating a concurrent
    /// mutating route landing while this fill's `git` spawn is in flight)
    /// must NOT poison the slot with the post-bump epoch. The slot is
    /// stamped with the epoch sampled BEFORE the probe ran, so the very next
    /// read - which samples the now-bumped epoch - is a miss and re-probes.
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

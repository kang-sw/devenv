---
title: Replace the dashboard daemon's interval-driven git polling with FS-watch-driven
  epoch invalidation behind cheap cached endpoints, keeping polling as a hard TTL
  ceiling
sage-review-design: required
related:
  260711-idea-dashboard-git-status-polling-index-lock-contention: source of the
    accepted long-term direction; its Non-Goals explicitly defer the watch-based
    rearchitecture to a follow-up implementation-ready ticket, which is this one
  260714-research-dashboard-workroot-watch-push-channel: the push-channel research
    ticket; this ticket answers its "Required First Step" prior-art question and
    deliberately does NOT build the push channel it explores
  260725-perf-dashboard-daemon-workroot-fanout-concurrency: proposes parallelizing
    the serial per-root fan-out; Phases 2 and the already-landed probe memo largely
    obviate that premise by deleting most of the fan-out instead of widening it
  260724-idea-dashboard-daemon-side-git-poll-response-timeout: closed by Phase 1's
    bounded-wait/kill-on-timeout git exec seam
  260722-bug-dashboard-daemon-git-ops-block-api: records the measured latency this
    work removes at the source
  260710-idea-dashboard-open-work-root-full-registry-redundant-rediscovery: the same
    redundant-rediscovery axis, scoped to the open-work-root path
  260726-idea-dashboard-moved-workroot-red-with-no-recovery-affordance: shares the
    availability-probe surface Phase 4's reconcile hook keys off; that ticket owns
    the presentation side, this one must not regress availability freshness
plans:
  phase-1: 2026-07/26-1315-git-fs-watch-invalidation
  phase-2: 2026-07/26-1315-git-fs-watch-invalidation
  phase-3: 2026-07/26-1315-git-fs-watch-invalidation
  phase-4: 2026-07/26-1315-git-fs-watch-invalidation
related-mental-model:
  - ws-web-dashboard
sage-review-completeness: required
---

# Replace interval-driven git polling with FS-watch-driven epoch invalidation

## Background

Measured on the Windows dogfood daemon (2026-07-26, `Win32_Process` cumulative
counters — process sampling undercounts because `git.exe` lives ~10-30 ms):

| | Before | After the probe-memo hotfix (`18037cc3`) |
|---|---|---|
| `git` spawns | 9.6/s (~830k/day) | 2.1/s (~180k/day) |
| non-read file ops | 2,844/s | 1,271/s |
| read ops | 563/s | 95/s |
| daemon CPU | 12.0% of one core | 5.7% |

The owner's stated budget is **one `git` invocation per 5 s per work root**
(0.2/s/root). With 9 registered roots the aggregate budget is ~1.8/s, so the
post-hotfix 2.1/s is at the edge of it — but only because a 30 s TTL is hiding
the fan-out, not because the fan-out is gone. Every TTL expiry still pays the
full `2N+W` burst, and the per-tick recompute for the selected root is still
unconditional work.

Composition of the load, established by tracing rather than guessing:

- Three independent frontend schedulers poll at 5 s / 5 s / 3 s
  (`gitToolbar.ts` git toolbar, `resourceRefresh.ts` resources, `App.tsx`
  activity), and `refreshGit` has no request-side in-flight guard — only a
  stale-response discard.
- `resolve_git_context` (`git_toolbar.rs:328-356`) answers a question about
  **one** work root by running `resources::live_dashboard_resources` over
  **all** of them: `2N+W` spawns per call, from routes that fire every 5 s.
- The Activity Console SSE (`work_root_activity.rs`) is not a push channel at
  all: it is a 200 ms server-side poll loop, and each iteration re-resolves
  `git_identity` (2 spawns) per root — ~20 spawns/s for a single subscriber.
  Measured at +84.6 pp of one core and +46,612 file-ops/s when opened. This is
  pane-gated (`activityPaneOpenForSelected`), so it is latent, not always-on.
- `git_text` / `run_git` (`git_toolbar.rs:566-587`) use bare `.output()`: no
  timeout, no kill, and **stderr is discarded silently**. A failing or hanging
  git invocation is invisible, which is why the owner's perception of an
  "infinite retry against a stale lock" was indistinguishable from the real
  behavior even though no retry loop exists.

`260711` recorded the target architecture on 2026-07-11 (owner decision):
replace fixed-interval polling with change-triggered refresh watching
`.git/index`, `.git/HEAD`, `.git/refs/**`. Its Non-Goals explicitly leave
scheduling that work to a follow-up implementation-ready ticket. This is that
ticket.

## Already Landed

The full plan is recovered verbatim at
`ai-docs/.plans/2026-07/26-1315-git-fs-watch-invalidation.md`; its **Phase 2**
(the discovery probe memo) shipped ahead of this ticket as the hotfix
`18037cc3 perf(dashboard): memoize git discovery probes to stop the spawn storm`:

- `GitProbeCache` in `discovery.rs` — two-level lock (map lock released before
  the per-key `Mutex` is acquired) giving memoization **and** single-flight, so
  three concurrent routes missing at the same instant produce one spawn.
- Availability (`fs::metadata` / `fs::read_dir`) deliberately left **uncached**,
  with an explicit `CONTRACT:` comment and eviction on any non-`Available`
  result, so `moved`/`missing`/`inaccessible` detection stays instant.
- `WS_DASHBOARD_GIT_PROBE_TTL_MS` (default 30 000; `0` disables).
- Verified: 128 lib tests, 166 `tests/routes.rs` tests, both named pins
  executed.

**Not** landed from that plan phase: widening `DiscoveredWorkRoot`
(`discovery.rs:324-331`) with `git_dir` / `common_dir`. `GitDiscovery::probe`
already computes both and throws them away. Phase 4 needs them to arm the
watcher without extra spawns, so that widening is folded into Phase 4.

The phase numbering in this ticket is therefore **not** the plan file's
numbering. Mapping: plan Phase 0 → Phase 1, plan Phase 1 → Phase 2, plan
Phase 2 → already landed, plan Phase 3 → Phase 3, plan Phase 4 → Phase 4, plan
Phase 5 → Non-Goals.

## Decisions

- **Watch-driven invalidation with cached endpoints, NOT an SSE push channel.**
  The watcher bumps a per-repo epoch; route handlers serve from a cache valid
  while `cached_epoch == current_epoch && age < ttl`. The frontend's existing
  5 s scheduler is untouched, but a tick where nothing changed costs **zero**
  git spawns. Rejected: pushing to the browser. The only thing push buys is
  latency (5 s → ~150 ms), and nothing in the measured problem is a latency
  complaint — a cached endpoint captures essentially the whole CPU win.
- **This answers `260714`'s prior-art question.** The existing "Activity Console
  Watch Stream" cannot be generalized to git/resource refresh and should not be
  imitated: it is a server-side 200 ms poll loop, and its lossy-hint resync
  machinery (`ActivitySnapshotInvalidationReason::{Overflow, WatchReset,
  Fallback}`, `ActivityUpdateMode::PollFallback`) cost hundreds of lines. A
  TTL-bounded cache is self-healing for free — a missed event costs at most one
  TTL of staleness and needs no protocol.
- **`DocumentEventHub` is a poor template** even though it is genuine push: its
  events are authoritative and complete (the daemon performed the write). FS
  events are lossy hints — overflow, coalescing, and git's own lock-rename noise
  all mean an event may be missing or spurious.
- **Two epochs per repo, not one** (`worktree` and `refs`). `branches_for_path`
  costs `3 + B` spawns and its inputs change only on branch/fetch operations;
  typing in a source file must not re-run `B` `rev-list` calls.
- **Ignore rules derived from git, not hardcoded.** Hardcoding `target/` /
  `node_modules/` is unsafe because `--untracked-files=all` is the point: in a
  repo where `target/` is not gitignored, those untracked files must be
  reported. Instead run one `git status --porcelain=v1 -uno --ignored=matching
  -z` per repo at arm time and collect the `!!` entries. On failure: empty
  ignore set → watch everything (correct, just noisier).
- **Polling is never deleted.** Phase 3's cache miss falls through to the same
  `changes_for_path` / `branches_for_path` calls that run today. The watcher
  only changes the TTL: 15 000 ms armed, 2 000 ms degraded/unarmed. Even with
  the watcher completely broken the daemon is strictly better than today,
  because the Phase 2 fan-out removal is unconditional.
- **`WorkRootId` derivation stays frozen.** Caches and the watcher get a
  separate `WatchKey` (canonicalize → normalize → `\` ⇒ `/` → lowercase on
  Windows). `opened-workroots.json` stores mixed separators
  (`"D:/Workspace/.../.git\\ws-worktree\\jpeg"`) and
  `normalize_registered_root` only strips the `\\?\` verbatim prefix, so two
  spellings of one missing root hash to different ids today. Fixing that in
  `local_work_root_id_for_path` would churn ids the frontend keys nav selection,
  workbench panes, and browser-local `workNavOrder.ts` persistence on. A
  separate `WatchKey` costs ~15 lines and zero blast radius; unifying the
  persisted spellings is a separate cleanup.
- **ahead/behind needs no slow timer.** `rev_counts` compares two **local**
  refs (`refs/heads/*` vs `refs/remotes/*`); today's 5 s poll does not contact
  the network either. Those refs change only when a local
  `fetch`/`pull`/`push` writes `refs/remotes/**`, `packed-refs`, or
  `FETCH_HEAD`, all of which are in the watch set. FS-watch invalidation is
  therefore exact parity with today. Automatic `git fetch` would be a new
  feature (network I/O, credential prompts) and is a Non-Goal.
- **`notify` v8 with `default-features = false`** (avoids the
  `crossbeam-channel` pull-in); forward the callback into a
  `tokio::sync::mpsc`. Rejected `notify-debouncer-full`: it maintains a file-ID
  cache sized to the watched tree and we need no rename correlation, only
  "something under X changed."
- **gitoxide is out of scope and demoted.** Once invalidation is event-driven
  the number of git invocations collapses, so replacing the remaining few with
  an embedded implementation has little left to win.

## Constraints

- **Linux/WSL inotify is the hard constraint.** `ReadDirectoryChangesW` with
  `bWatchSubtree=TRUE` costs **one** kernel handle per tree; inotify costs one
  watch descriptor **per directory**. The measured requirement across the 4
  dogfood repos is ~9,800 directories against a default `max_user_watches` of
  8,192. The git-derived ignore set accounts for ~7,379 of those
  (264/1565, 384/803, 6600/7164, 131/264), so pruning takes it to ~2,400 —
  comfortably under the limit. This pruning is load-bearing, not an
  optimization.
- Over-budget repos must be **left entirely unarmed** (`Degraded`), never
  partially armed. Enforce a process budget of
  `min(max_user_watches * 60 / 100, WS_DASHBOARD_GIT_WATCH_MAX_DIRS)` (default
  cap 6000). Exhausting inotify descriptors would degrade unrelated
  applications on the host (editors, other watchers).
- The daemon must **never fail to boot** because a watcher could not start:
  `watcher: Option<...>`, init failure ⇒ all repos `Unarmed`, warn once, 2 s
  TTL.
- Poll-path git must stay lock-free per the `ws-web-dashboard` Domain Rules:
  `--no-optional-locks` on `status`, `diff-index` plumbing for change lines.
  A watch-triggered recompute runs the same commands, so the rule still holds.
  Note the accepted cost: git cannot persist its refreshed index stat-cache
  under that flag, so every recompute pays full `lstat` — which is exactly why
  cutting the *number* of recomputes matters more than making each one cheaper.
- The registry side effects in `resources.rs` (pruning, discovered-root
  registration) must keep running unconditionally on every call. Cache hits may
  never skip them. The already-landed hotfix preserves this; Phase 4's reconcile
  hook must not change it.
- Windows: the watcher holds directory handles. `notify` opens with
  `FILE_SHARE_DELETE` so deletes are permitted, but `git worktree remove` must
  still disarm the repo **before** running, then re-arm via the next reconcile.

## Prior Art

- `terminal.rs:452` `output_signal: watch::Sender<u64>` — a monotonic-counter
  wakeup signal. This is precisely the epoch primitive, already an in-repo
  pattern.
- `DocumentEventHub` (`work_root_files.rs:44-64`, `broadcast` cap 64) + SSE
  `document_events` — the reference shape for a future Phase 5, and the reason
  Phase 5 is cheap once the epoch exists.
- `GitProbeCache` (`discovery.rs`, landed `18037cc3`) — the two-level-lock
  memo+single-flight pattern Phase 3's `GitStateCache` reuses verbatim.
- Test fixtures: `git_toolbar.rs` `init_fixture_repo`, `tests/routes.rs`
  `run_git` / `init_git_repo` / `skip_without_git` / `git_toolbar_get_json` /
  `app_state()`.

## Spec Impact

Target spec area: `ai-docs/spec/ws-web-dashboard/index.md`, section
`## Git-Aware WorkRoot Toolbar {#260524-ws-dashboard-git-aware-workroot-toolbar}`
(the "Status refresh stays host-light … then polls conservatively only for the
selected visible WorkRoot" paragraph).

Expected caller-visible change:

- Git status/branch freshness becomes **change-triggered with a TTL ceiling**
  rather than "refreshed on every poll tick". The contract to document is the
  ceiling (15 s armed / 2 s degraded), not the mechanism, plus the guarantee
  that user-initiated mutations (branch switch/create, fetch/push/pull) are
  never TTL-delayed because they bump the epoch directly.
- A new owner-authed diagnostics route `GET /api/dashboard/diag/git` reporting
  spawn counters and per-repo watch health.
- One HTTP status delta: an *unavailable* git work root returns `409 workRoot
  unavailable` instead of today's `404 unknown workRoot`, because per-root
  resolution no longer lets workspace-level pruning mask the answer.
- The existing lock-free-read requirement is unchanged and must be restated as
  still applying to watch-triggered recomputes.

Also update the `ws-web-dashboard` mental model's poll-path git rule, which
currently says "poll-path git invocation (≤5s cadence)".

Contract-first spec: no. The behavior is a load/freshness change to an
already-specified surface; closeout documentation after implementation is
sufficient.

## Phases

Phases 1 and 2 are independently shippable and independently valuable. Phase 3
depends on Phase 1 (its spawn counter is what makes "the second call added zero
spawns" assertable at all). Phase 4 depends on Phase 3 (it swaps the stubbed
`EpochSource` for the real one) and on the `DiscoveredWorkRoot` widening
described under Already Landed.

### Phase 1: Git exec seam — bounded wait, kill on timeout, stderr logging, spawn counters

New `crates/daemon/src/git_exec.rs` owning the single git-spawn seam:

```rust
pub struct GitOutcome { pub status: ExitStatus, pub stdout: String, pub stderr: String, pub elapsed: Duration }
pub enum GitFailure { Spawn(io::Error), Timeout, Status(i32) }
pub fn capture(root: &Path, args: &[&str], budget: Duration) -> Result<GitOutcome, GitFailure>
pub struct GitSpawnStats { total: AtomicU64, timeouts: AtomicU64, failures: AtomicU64, by_subcommand: Mutex<BTreeMap<&'static str, u64>> }
```

- `capture` = `Command::spawn` + bounded wait + `child.kill()` on expiry
  (default 10 s, `WS_DASHBOARD_GIT_TIMEOUT_MS`). This closes
  `260724-idea-dashboard-daemon-side-git-poll-response-timeout` for the poll
  path.
- Non-zero exit or timeout ⇒ `tracing::warn!(subcommand, code, stderr =
  %truncate(stderr, 512), elapsed_ms)`. Closes the silent-failure hole that made
  the owner's "infinite retry" perception unfalsifiable.
- Rewrite `git_text` / `run_git` (`git_toolbar.rs:566-587`), `GitDiscovery::probe`,
  `probe_git_worktree_paths` (`discovery.rs`), and `git_output`
  (`work_root_activity.rs`) as thin wrappers over `capture`, so every existing
  call site and the four in-file `git_toolbar.rs` tests compile unchanged.
- New owner-authed `GET /api/dashboard/diag/git` next to `dashboard_build_info`
  (`router.rs:108`) returning `{ totalSpawns, timeouts, failures, bySubcommand,
  uptimeMs }`.

Rationale for shipping this first: every later phase's claim is a spawn-rate
claim, and today there is no way to assert one from inside a test. This phase
converts "we measured it by hand on the live host" into a route any test or
dogfood check can read.

**Verification boundary.** Unit test: `capture` kills a child that outlives its
budget (inject the binary name so the test can spawn a deliberately long-running
process rather than relying on a git trick). Live: two `/api/dashboard/diag/git`
reads 60 s apart on the Windows dogfood daemon derive spawns/s — this number is
the acceptance gate for Phases 2 and 4. Not covered: nothing in this phase
changes observable git behavior, so no route-contract test changes.

Estimated diff ~+240/−70 across 7 files.

### Phase 2: Resolve git routes against one work root, not all of them

- Rewrite `resolve_git_context` (`git_toolbar.rs:328-356`) to resolve a single
  root: `state.opened_work_roots.get(id)` → `None` ⇒ `Unknown` (404);
  `activation != Online` ⇒ `Offline` (409);
  `discovery::discover_work_root` (make `pub(crate)`) → `availability !=
  Available` ⇒ `Unavailable` (409); `kind` not `GitPrimaryRoot |
  GitLinkedWorktree` ⇒ `NonGit` (400); else `GitContext { root_path }`. Drops
  the `use crate::resources::live_dashboard_resources` import. **`2N+W` → 1
  spawn** for `/git/status` and for `/git/branches`.
- Memoize `git_identity` (`work_root_activity.rs`) behind a per-root
  `OnceCell` keyed by `WatchKey`, invalidated only when `.git` goes missing. Its
  inputs (worktree root, common root) are structural and effectively immutable
  for a registered root. This kills both the 3 s activity poll's 2 spawns/tick
  **and** the Activity SSE's ~20/s.
- Add `discovery::watch_key`.

**Behavior deltas to accept explicitly and pin:**

- The git routes no longer trigger the discovered-worktree registry sync. New
  linked worktrees still appear via the 5 s `/api/dashboard/resources` poll and
  immediately after `git_worktree_add_submit`.
- Workspace-level pruning no longer masks a per-root answer, so an unavailable
  git root returns 409 instead of 404. `tests/routes.rs`
  `git_toolbar_status_gates_and_reports_counts_without_paths` pins exactly four
  cases (200 available / 400 plain / 409 offline / 404 unknown-id); all four are
  preserved by the logic above. Add a fifth case for the moved-root 409.

**Verification boundary.** `/api/dashboard/diag/git` delta drops by
~2×(2N+W) per 5 s. Full `cargo test -p ws-dashboard-daemon` green, with the
named `routes.rs` git-toolbar and resources pins executed and reported by name,
not just by count. Not covered: whether removing the registry-sync side effect
from the git routes is perceptible in the UI — that needs a dogfood pass, stated
here rather than claimed as tested.

Estimated diff ~+130/−50 production, ~+90 test, 4 files.

### Phase 3: Result cache for `/git/status` and `/git/branches`, epoch stubbed

```rust
struct RefState { branch_name, detached_oid, upstream, sync, branch_list, checked_out }
struct GitCacheSlot { worktree: Option<(u64, Instant, GitChangeSummary)>, refs: Option<(u64, Instant, RefState)> }
pub struct GitStateCache { slots: Arc<Mutex<HashMap<WatchKey, Arc<Mutex<GitCacheSlot>>>>> }
```

- `status_for_path` and `branches_for_path` (`git_toolbar.rs:358-438`) take
  `(&GitStateCache, &EpochSource, &Path)` and read/fill the two slot parts.
  `changes_for_path` stays a pure function so its four in-file tests keep
  working verbatim.
- `EpochSource` is a trait with a `StaticZero` impl in this phase and the
  watcher impl in Phase 4 — so this phase is TTL-only and independently
  testable, and Phase 4 becomes purely "make the epoch real".
- Mutating routes bump epochs directly so a user action is never TTL-delayed:
  `git_switch_branch`, `git_create_branch`, and the fetch/push/pull
  `mutate_no_body` paths bump `refs`; `git_pull_ff_only` also bumps `worktree`.
  The existing manual `git_fetch` route stays the only fetch trigger.
- TTL from `WS_DASHBOARD_GIT_CACHE_TTL_MS`, default 2000 while `StaticZero`.

**Verification boundary.** Integration: hit `/git/status` twice inside the TTL
and assert via Phase 1's counter that the second call adds **zero** spawns; then
`POST /git/switch-branch` and assert the next `/git/status` reflects the new
branch immediately (epoch bump beats TTL). `branches_switch_and_create_revalidate_state`
must pass unmodified. Not covered: cross-`serverRoute` behavior — local
delegation already routes through `git_status`, so no `servers.rs` logic change
is expected, but that expectation is asserted by the existing forwarding tests
rather than a new one.

Estimated diff ~+270/−95 production, ~+160 test, 4 files.

### Phase 4: The `notify` watcher — real epochs, per-platform arming, budget-aware degradation

New `crates/daemon/src/work_root_watch.rs`. **This is the large phase**: ~550
lines in the new module alone, roughly half of it the Linux `PerDirectory` path
plus budget/degrade handling that the Windows path does not need.

Implement in this order, because the first step is the correctness core and is
fully testable without any I/O:

1. **`classify(path, &ArmedRepo) -> Option<EpochKind>`** — pure function. Under
   `common_dir/objects|lfs|modules` ⇒ ignore; `*.lock` under any git dir ⇒
   ignore (git's create-then-rename lock dance is pure noise, and `index.lock`
   churn is exactly what `260711` was filed about);
   `common_dir/{HEAD,packed-refs,FETCH_HEAD,ORIG_HEAD}` or `refs/**` or
   `worktrees/**` ⇒ `Refs`; `git_dir/index` ⇒ `Worktree`; under an `IgnoreSet`
   dir ⇒ ignore; `.gitignore` / `.git/info/exclude` ⇒ `Worktree` + mark the
   ignore set stale; else under `worktree` ⇒ `Worktree`.
2. **`IgnoreSet::derive(worktree)`** — one `git_exec::capture`, parse `!!`
   entries from `-z` output.
3. **Arming.** `WatchStrategy::RecursiveSubtree` on Windows/macOS (one
   registration per target, filter event paths against the `IgnoreSet`);
   `PerDirectory` on Linux/WSL (walk with the ignore set applied, register
   `NonRecursive` per surviving directory, count against `DirBudget`,
   over-budget ⇒ `Degraded` and register nothing). A new directory created
   inside a watched tree triggers registering it and re-checking the budget;
   the known inotify race where a directory is created and populated before
   registration is exactly what the TTL fallback covers.
4. **Event pipeline.** `notify` callback (own thread) → `mpsc::unbounded_send` →
   one long-lived tokio task: coalesce 100 ms trailing / 500 ms max, map each
   path through `classify`, bump the union of `EpochKind`s per repo once.
   `event.need_rescan()` (inotify `IN_Q_OVERFLOW`, `ReadDirectoryChangesW`
   buffer overflow) ⇒ bump both epochs for every repo on that watcher and set
   `Degraded{"rescan required"}` for one TTL window.
5. **Reconcile through exactly one hook.**
   `registry.reconcile(&[(WatchKey, Option<WatchTargets>, WorkRootAvailability)])`
   called from `resources::live_dashboard_resources_with_sync` after
   `sync_discovered_roots`, using the widened `DiscoveredWorkRoot` fields — **no
   extra git spawns**, because that call site already computes the authoritative
   root set and availability every 5 s. Semantics: present + `Available` + not
   armed ⇒ arm; present + not `Available` ⇒ disarm + bump both (so the next poll
   recomputes and reports the degraded state); absent ⇒ disarm + drop epochs.
   One code path covers register/unregister, `moved`/`missing`/`inaccessible`
   transitions, `remove_workspace`, and `git_worktree_remove_submit` instead of
   six separate hooks.
6. **Wire the real `EpochSource`** into `GitStateCache`; select TTL from
   `WatchHealth` (15 000 ms armed, 2 000 ms degraded/unarmed).
7. **Config:** `WS_DASHBOARD_GIT_WATCH=off|auto|force` (default `auto`; `off` ⇒
   every repo `Unarmed` on the 2 s TTL — the rollback switch),
   `WS_DASHBOARD_GIT_WATCH_MAX_DIRS`, `WS_DASHBOARD_GIT_WATCH_DEBOUNCE_MS`.
   Extend `/api/dashboard/diag/git` with `{ repos: [{ health, registeredDirs,
   worktreeEpoch, refsEpoch, lastEventMs }] }`.

Also widen `DiscoveredWorkRoot` (`discovery.rs:324-331`) with `git_dir` /
`common_dir`, which `GitDiscovery::probe` already computes and discards.

**Verification boundary, in three honest tiers.**

- *Unit:* `classify` table test (~25 cases) covering `objects/` exclusion,
  `HEAD`/`refs/` inclusion, `index.lock` suppression, ignore-set membership, and
  linked-worktree `git_dir`. `DirBudget` arithmetic and the over-budget degrade
  decision. Debounce coalescing with an injected clock. `watch_key`
  normalization against the real mixed-separator string from
  `opened-workroots.json` and its all-forward-slash twin. `IgnoreSet` parsing of
  a fixed `-z` byte string.
- *Integration:* new `crates/daemon/tests/git_watch.rs` against a real temp
  repo — arm, `fs::write` an untracked file, poll the `worktree` epoch until
  bumped or 5 s deadline, assert `refs` did **not** bump; `git switch -c` ⇒
  `refs` bumped; `git worktree add` ⇒ `refs` bumped; a file under a gitignored
  `target/` ⇒ **no** bump. Availability lifecycle: rename the root away ⇒
  reconcile disarms and bumps ⇒ status reports unavailable ⇒ rename back ⇒
  re-arms. `#[cfg(unix)]` with `WS_DASHBOARD_GIT_WATCH_MAX_DIRS=1` asserting
  `Degraded` + short TTL rather than partial arming. Windows: worktree-remove
  while armed does not fail with a sharing violation (existing worktree-remove
  pins are the reference). All deadline-polling, never fixed sleeps.
- *Live-only — state as not covered by tests, do not pretend otherwise:*
  sustained spawns/s and CPU% on the Windows dogfood daemon over ≥10 min with
  the browser open, Activity pane both closed and open; real inotify descriptor
  consumption on WSL (`find /proc/*/fd -lname anon_inode:inotify | wc -l`);
  buffer-overflow/rescan handling under a real `cargo build` storm; and
  end-to-end perceived freshness, which per the `ws-web-dashboard` Domain Rules
  needs a browser-level assertion in `frontend/e2e/` for the toolbar chip
  updating after an external edit.

Estimated diff ~+720/−70 production, ~+280 test, 9 files.

**Rollback ladder, each rung independent:** `WS_DASHBOARD_GIT_WATCH=off`
disables Phase 4 only; `WS_DASHBOARD_GIT_CACHE_TTL_MS=0` disables Phase 3;
`WS_DASHBOARD_GIT_PROBE_TTL_MS=0` disables the landed probe memo; reverting
Phase 2 is a self-contained rewrite of one function.

## Non-Goals

- **SSE push for git state** (the plan's Phase 5). Deferred and unscheduled;
  revisit only if latency becomes an actual complaint, and not until Phase 4 has
  run in dogfood for a week. Shape is recorded in the plan file.
- **Automatic `git fetch` on a slow timer.** New feature, not
  regression-avoidance: it spawns network I/O and can trigger credential
  prompts.
- **Changing `WorkRootId` derivation** or unifying the persisted path spellings
  in `opened-workroots.json` — separate cleanup, and changing it would churn ids
  the frontend persists against.
- **gitoxide / embedding a git implementation.** Demoted by this ticket's own
  premise.
- **Retiring the Activity Console.** Separate RnR from the agent-GUI
  suspension; recorded as a review item in
  `260725-research-ws-dashboard-pty-agent-pivot`. Phase 2 makes the pane cheap
  either way.
- **Frontend scheduler changes.** The three timers stay exactly as they are;
  this ticket makes their ticks cheap rather than removing them.

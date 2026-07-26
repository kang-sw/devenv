---
title: Replace the dashboard daemon's interval-driven git polling with FS-watch-driven
  epoch invalidation behind cheap cached endpoints, keeping polling as a hard TTL
  ceiling
sage-review-design: completed
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
  phase-5: 2026-07/26-1315-git-fs-watch-invalidation
related-mental-model:
  - ws-web-dashboard
sage-review-completeness: completed
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
watcher without extra spawns, so that widening is folded into Phase 4 — those
two fields plus the worktree path are exactly what Phase 4's
`WatchTargets { worktree, git_dir, common_dir }` carries.

The phase numbering in this ticket is therefore **not** the plan file's
numbering. Mapping: plan Phase 0 → Phase 1, plan Phase 1 → Phase 2, plan
Phase 2 → already landed, plan Phase 3 → Phase 3, plan Phase 4 → **split into
Phase 4 (recursive-subtree platforms) and Phase 5 (Linux `PerDirectory`)**, plan
Phase 5 (SSE push) → Non-Goals. Where the plan and this ticket disagree, the
ticket wins: several plan claims were corrected by the design review and by a
reading of this host's actual `max_user_watches` (see Constraints).

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

- **Linux/WSL inotify is the platform asymmetry.** `ReadDirectoryChangesW` with
  `bWatchSubtree=TRUE` costs **one** kernel handle per tree; inotify costs one
  watch descriptor **per directory**. The measured requirement across the 4
  dogfood repos is ~9,800 directories, of which the git-derived ignore set
  accounts for ~7,379 (264/1565, 384/803, 6600/7164, 131/264), so pruning takes
  it to ~2,400.
  **How binding this is depends entirely on the host's actual limit, which must
  be read at runtime and never assumed.** Mainline's default
  `max_user_watches` is 8,192, against which ~9,800 would fail and ~2,400 fits.
  But this project's own WSL2 dev host reports **524,288** (checked 2026-07-26),
  where even the unpruned figure is irrelevant. So on the measured hosts pruning
  is an efficiency and memory measure, not a correctness requirement — treat the
  earlier "load-bearing" framing as an overstatement derived from the mainline
  default rather than from a reading. The budget mechanism still matters, because
  a distro at 8,192 or a monorepo an order of magnitude larger is entirely
  plausible; it just must not be justified with a number nobody measured.
- **WSL2 `/mnt/*` (DrvFs / 9P) does not deliver inotify events for changes made
  from the Windows side.** This is exactly this project's dogfood topology, so a
  WSL-side daemon watching a work root under `/mnt/d` would arm successfully and
  then silently never fire. The TTL ceiling bounds the damage to one window, but
  silent-and-armed is the worst reporting state: the diag route would claim
  `Armed` while behaving `Unarmed`. Detect the filesystem before arming (a
  `/mnt/`-prefixed or `9p`/`drvfs` mount for the target ⇒
  `Degraded{"filesystem does not deliver events"}`) rather than trusting the
  arm to have worked. Note the live daemon runs natively on Windows and is
  unaffected; this hazard belongs to WSL-side developer daemons.
- Over-budget repos must be **left entirely unarmed** (`Degraded`), never
  partially armed. Enforce a process budget of
  `min(max_user_watches * 60 / 100, WS_DASHBOARD_GIT_WATCH_MAX_DIRS)` (default
  cap 6000). Exhausting inotify descriptors would degrade unrelated
  applications on the host (editors, other watchers).
- The budget is **process-wide and shared across repos**, so arming order
  decides which repo gets it when the set does not all fit. Arm in the
  reconcile pass's own iteration order over the authoritative root set, and
  make that order deterministic (sort by `WatchKey`) so the same registry
  produces the same armed/degraded split across restarts. Do **not** reorder by
  size, recency, or selection: a heuristic that silently re-arms a different
  repo each boot makes a degraded repo look intermittent. Already-armed repos
  keep their registrations across a reconcile; the budget is only consulted for
  repos being newly armed.
- **The budget must count every descriptor the process actually consumes**, or
  the "never degrade unrelated applications" rule is unenforceable. Two sources
  escape a naive count. (a) `git status --ignored=matching` never reports `.git`,
  so the walk must **exclude `git_dir` / `common_dir` from registration**, not
  merely ignore their events in `classify` — those are different operations, and
  registering them burns descriptors on the 256-way `objects/` fanout,
  `objects/pack`, and `.git/modules`. (b) On Linux,
  `RecursiveMode::Recursive` makes `notify` walk and register per-directory
  *internally*, so any recursive target's descriptors are invisible to
  `DirBudget` and to the `registeredDirs` figure in the diag route. Therefore on
  Linux use `NonRecursive` for **every** target including `refs/**` and
  `worktrees/**`, walking them ourselves so each descriptor is counted.
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
  `document_events` — the reference shape for the deferred SSE push in Non-Goals,
  and the reason that push is cheap once the epoch exists.
- `resolve_online_available_work_root` (`work_root_files.rs:798`) — the existing
  zero-git-spawn availability gate Phase 2 reuses instead of writing a second one.
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
described under Already Landed. Phase 5 depends on Phase 4 and only affects
Linux/WSL hosts.

### Phase 1: Git exec seam — bounded wait, kill on timeout, stderr logging, spawn counters

New `crates/daemon/src/git_exec.rs` owning the single git-spawn seam:

```rust
pub struct GitOutcome { pub status: ExitStatus, pub stdout: String, pub stderr: String, pub elapsed: Duration }
pub enum GitFailure { Spawn(io::Error), Timeout, Status(i32) }
pub enum GitFailureExpectation { Unexpected, ExpectedNonZero }
pub struct GitSpawnStats { total: AtomicU64, timeouts: AtomicU64, failures: AtomicU64, by_subcommand: Mutex<BTreeMap<GitSubcommand, u64>> }
pub fn capture(stats: &GitSpawnStats, root: &Path, args: &[&str],
               expect: GitFailureExpectation, budget: Duration) -> Result<GitOutcome, GitFailure>
```

- **`GitSpawnStats` is owned explicitly, never a process-global static.** A
  global would be polluted across the many concurrently-running git-spawning
  tests in `tests/routes.rs` (no `serial_test` gating there), which would make
  Phase 3's central "the second call added zero spawns" assertion flaky in
  exactly the direction that hides a regression — and it violates Code Standards
  #4. Hold `Arc<GitSpawnStats>` in `AppState` and thread `&GitSpawnStats` into
  `capture`. The `discovery.rs` free functions already take `&GitProbeCache`
  after `18037cc3`, so they take a second borrowed handle the same way; that
  established threading pattern is why this is a mechanical change rather than a
  design problem.
- **`capture` must drain `stdout`/`stderr` concurrently with the bounded wait.**
  A `spawn` + `try_wait`-loop that does not drain deadlocks the moment the child
  fills the pipe buffer (~64 KB), so the child blocks forever and then gets
  killed at the budget. `status --porcelain=v1 --untracked-files=all` on a repo
  with a large un-gitignored build directory exceeds 64 KB easily — precisely the
  repos this ticket targets. Since Phase 1 rewrites **every** git call site, an
  undrained implementation converts working-but-slow into hard timeout failure
  across the whole daemon. Specified shape: spawn one reader thread per pipe
  reading to EOF, wait with a deadline, `child.kill()` on expiry, then join the
  readers. Today's `.output()` gets this right for free; the replacement must
  not regress it. **A test must pin it:** capture a child emitting >1 MB on
  stdout and assert success rather than `Timeout`.
- Bounded wait + `child.kill()` on expiry (default 10 s,
  `WS_DASHBOARD_GIT_TIMEOUT_MS`). This closes
  `260724-idea-dashboard-daemon-side-git-poll-response-timeout` for the poll
  path.
- **Warn only on unexpected failure.** `tracing::warn!(subcommand, code, stderr =
  %truncate(stderr, 512), elapsed_ms)` fires on `Spawn` / `Timeout` always, and
  on non-zero exit only when the call site passed
  `GitFailureExpectation::Unexpected`. Several poll-path probes fail routinely by
  design — `rev-parse --abbrev-ref --symbolic-full-name @{upstream}` exits
  non-zero for every branch with no upstream, and `rev-parse --short HEAD` fails
  on an unborn HEAD. Warning on those would produce a continuous stream at
  5 s × 9 roots and bury the actual silent-failure signal this phase exists to
  expose, inverting its own goal. Expected non-zero exits still increment the
  counters; they just do not log.
- `by_subcommand` is keyed by an interned `GitSubcommand` enum, not
  `&'static str` — the key cannot be derived from a borrowed `args: &[&str]`.
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

**Verification boundary.** Unit tests: `capture` kills a child that outlives its
budget (inject the binary name so the test can spawn a deliberately long-running
process rather than relying on a git trick); `capture` survives a child emitting
>1 MB on stdout; an `ExpectedNonZero` call increments `failures` without
logging. Live: two `/api/dashboard/diag/git` reads 60 s apart on the Windows
dogfood daemon derive spawns/s — this number is the acceptance gate for Phases 2
and 4. Not covered: nothing in this phase changes observable git behavior, so no
route-contract test changes.

Estimated diff ~+260/−70 across 7 files.

### Phase 2: Resolve git routes against one work root, not all of them

- Rewrite `resolve_git_context` (`git_toolbar.rs:328-356`) to resolve a single
  root: `state.opened_work_roots.get(id)` → `None` ⇒ `Unknown` (404);
  `activation != Online` ⇒ `Offline` (409);
  `discovery::discover_work_root` (make `pub(crate)`) → `availability !=
  Available` ⇒ `Unavailable` (409); `kind` not `GitPrimaryRoot |
  GitLinkedWorktree` ⇒ `NonGit` (400); else `GitContext { root_path }`. Drops
  the `use crate::resources::live_dashboard_resources` import. **`2N+W` → 1
  spawn** per call for `/git/status` and for `/git/branches`.
  **Reuse `resolve_online_available_work_root` (`work_root_files.rs:798`)
  rather than writing a second gate.** It already does
  get → activation → `is_dir`/`read_dir` with the identical
  404-unknown / 409-offline / 409-unavailable mapping and the same message
  strings, with zero git spawns. Two divergent availability gates in one daemon
  is the failure mode to avoid here; the only thing to add on top is the
  git-`kind` check.
- **Memoize `git_identity`** (`work_root_activity.rs:2357`) per root keyed by
  `WatchKey`. **Not a `OnceCell`:** it must be evictable, and it returns
  `Option`, returning `None` for non-git and bare-repo roots. A `OnceCell` would
  make a plain directory the user later `git init`s — or a repo moved by
  `git worktree move` — permanently stuck on the memoized answer until daemon
  restart, which is exactly the class of live-registration change the sibling
  `260726-idea-dashboard-moved-workroot-red-with-no-recovery-affordance` is
  about. Eviction owner, stated because Phase 2 has no availability signal of its
  own (the reconcile hook that carries availability arrives in Phase 4): in
  Phase 2, **cache only `Some` results and always re-probe on `None`**, so the
  not-yet-a-repo case is self-correcting and only the stable case is memoized.
  Phase 4's reconcile then evicts on any non-`Available` transition. This kills
  both the 3 s activity poll's 2 spawns/tick **and** the Activity SSE's ~20/s.
- Add `discovery::watch_key`.

**Behavior deltas to accept explicitly and pin:**

- The git routes no longer trigger the discovered-worktree registry sync. New
  linked worktrees still appear via the 5 s `/api/dashboard/resources` poll and
  immediately after `git_worktree_add_submit`.
- Workspace-level pruning no longer masks a per-root answer, so an unavailable
  git root returns 409 instead of 404. This is **convergence onto the invariant
  `resolve_online_available_work_root` already enforces elsewhere in the daemon,
  not a new contract** — the 404 is the anomaly. `tests/routes.rs`
  `git_toolbar_status_gates_and_reports_counts_without_paths` pins exactly four
  cases (200 available / 400 plain / 409 offline / 404 unknown-id); all four are
  preserved by the logic above. Add a fifth case for the moved-root 409.

**Correct the expected win — the pre-hotfix arithmetic no longer applies.**
`GitProbeCache` (default TTL 30 000 ms) is shared through `AppState` by
`/git/status`, `/git/branches` **and** `/api/dashboard/resources`, and the
resources poll keeps refilling it every 5 s regardless of what this phase does to
the git routes. So the fan-out is already amortized to roughly once per 30 s, and
the steady-state spawn reduction from the `resolve_git_context` rewrite alone is
**near zero**. What this phase actually buys:

1. The `git_identity` memo — 0.67/s from the 3 s activity poll, plus ~20/s
   whenever the Activity pane is open. This is the measurable win.
2. Removal of the latency spike: a `/git/status` call that misses the probe TTL
   currently pays the whole `2N+W` fan-out inline before answering.
3. The 409 correctness convergence above.
4. Structural prerequisite: the git routes stop depending on
   `live_dashboard_resources` at all, which is what lets Phase 3 cache per-root
   without reasoning about the whole-registry call.

An implementer measuring against the old "~2×(2N+W) per 5 s" gate would see a far
smaller delta and be unable to distinguish success from regression.

**Verification boundary.** `/api/dashboard/diag/git` delta with the Activity pane
**open** drops by ~20/s, and with it closed by ~0.67/s; the fan-out figure is
explicitly *not* the gate. Full `cargo test -p ws-dashboard-daemon` green, with
the named `routes.rs` git-toolbar and resources pins executed and reported by
name, not just by count. Because the daemon's `error` string is surfaced verbatim
by `refreshGit`, the 404 → 409 change alters visible toolbar text, so per the
`ws-web-dashboard` Domain Rules this phase needs its own browser-level assertion
in `frontend/e2e/` for the moved-root message — do not defer that to Phase 4,
which allocates e2e only for freshness. Not covered: whether removing the
registry-sync side effect from the git routes is perceptible in the UI — that
needs a dogfood pass, stated here rather than claimed as tested.

Estimated diff ~+130/−50 production, ~+120 test, 5 files.

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

### Phase 4: The `notify` watcher — real epochs on the recursive-subtree platforms

New `crates/daemon/src/work_root_watch.rs`, **Windows/macOS `RecursiveSubtree`
only**. Linux `PerDirectory` arming, `DirBudget`, and budget-degradation are
Phase 5.

Rationale for the split: the original single phase bundled ~550 lines in which
roughly half was the Linux path plus budget/degrade handling the Windows path
does not need — and that same half is where a bug both silently breaks
invalidation *and* can degrade unrelated applications on the host. Phases 1-3 are
deliberately structured for independent shippability, and collapsing that
exactly where the risk concentrates is the wrong place to stop. Split this way,
Phase 4 is dogfoodable on the live Windows daemon immediately and Phase 5 has a
real revert boundary rather than only a kill switch. On Linux, Phase 4 alone
leaves every repo `Unarmed` on the 2 s TTL — i.e. Phase 3's behavior, which is
already better than today.

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
3. **Arming — `RecursiveSubtree` only in this phase.** One
   `RecursiveMode::Recursive` registration per target, event paths filtered
   against the `IgnoreSet`. Cheap: one kernel handle per tree on Windows, one
   FSEvents stream on macOS. Bump both epochs **on arm**, not only on disarm: a
   slot filled while `Unarmed` carries epoch 0, and without a bump it stays valid
   for the whole 15 s TTL even though it was computed during a window in which no
   events were being observed — as are any changes landing during the arming walk
   itself. `WatchStrategy::PerDirectory` is declared in the enum but returns
   `Unarmed` until Phase 5, so Linux is explicitly on the polling path here.
4. **Event pipeline.** `notify` callback (own thread) → `mpsc::unbounded_send` →
   one long-lived tokio task: coalesce 100 ms trailing / 500 ms max
   (`WS_DASHBOARD_GIT_WATCH_DEBOUNCE_MS` default 100, max window fixed at 5×
   the trailing value), map each path through `classify`, bump the union of
   `EpochKind`s per repo once.
   `event.need_rescan()` (inotify `IN_Q_OVERFLOW`, `ReadDirectoryChangesW`
   buffer overflow) ⇒ bump both epochs for every repo on that watcher and set
   `Degraded{"rescan required"}` for one TTL window.
5. **Reconcile through exactly one hook.**
   `registry.reconcile(&[(WatchKey, Option<WatchTargets>, WorkRootAvailability)])`
   — where `WatchTargets { worktree: PathBuf, git_dir: PathBuf, common_dir:
   PathBuf }` is populated straight from the widened `DiscoveredWorkRoot`, and
   `None` means the root is not a git root — called from
   `resources::live_dashboard_resources_with_sync` after
   `sync_discovered_roots`, using the widened `DiscoveredWorkRoot` fields. Reading
   those fields costs **no extra git spawns** because that call site already
   computes the authoritative root set and availability every 5 s — but note the
   *arm path it triggers* does cost a spawn (`IgnoreSet::derive`) plus a walk, so
   do not restate "no extra git spawns" about reconcile as a whole.
   Semantics: present + `Available` + `Unarmed` ⇒ arm;
   present + not `Available` ⇒ disarm + bump both (so the next poll recomputes and
   reports the degraded state); absent ⇒ disarm + drop epochs. One code path
   covers register/unregister, `moved`/`missing`/`inaccessible` transitions,
   `remove_workspace`, and `git_worktree_remove_submit` instead of six separate
   hooks.
   **Two rules this hook must not get wrong:**
   - **`Degraded` is sticky, and re-arm is backed off.** The arm condition is
     `Unarmed`, never "not armed" — `Degraded` must not re-enter it, or a repo
     that failed to arm gets re-armed every reconcile, i.e. every 5 s, each
     attempt costing one `IgnoreSet::derive` spawn plus a walk. That is a spawn
     storm of exactly the shape this ticket exists to remove. Retry a `Degraded`
     repo on an exponential backoff (start 60 s, cap 15 min) or on an explicit
     availability transition, never on the reconcile cadence.
   - **Arming never runs inline on the resources route.** `reconcile` computes the
     desired delta and hands arm/disarm work to the watcher task over the same
     channel the event pipeline uses. Arming inline would put an
     `IgnoreSet::derive` spawn and a multi-thousand-`read_dir` walk on the 5 s
     poll handler — reintroducing the latency this ticket removes.
6. **Wire the real `EpochSource`** into `GitStateCache`; select TTL from
   `WatchHealth` (15 000 ms armed, 2 000 ms degraded/unarmed).
7. **Config:** `WS_DASHBOARD_GIT_WATCH=off|auto|force` and
   `WS_DASHBOARD_GIT_WATCH_DEBOUNCE_MS`. Semantics, stated because the plan left
   `force` undefined: `off` ⇒ never arm anything, every repo `Unarmed` on the 2 s
   TTL (the rollback switch); `auto` (default) ⇒ arm where the platform strategy
   and the pre-arm filesystem check allow, degrade silently otherwise;
   `force` ⇒ attempt to arm even where `auto` would pre-emptively degrade
   (the WSL `/mnt` filesystem check, and in Phase 5 the descriptor budget),
   logging the override once per repo. `force` exists to make a suspected-bad
   heuristic testable on a real host, not as a supported production mode.
   Extend `/api/dashboard/diag/git` with `{ repos: [{ health, worktreeEpoch,
   refsEpoch, lastEventMs }] }`.

Also widen `DiscoveredWorkRoot` (`discovery.rs:324-331`) with `git_dir` /
`common_dir`, which `GitDiscovery::probe` already computes and discards.

**Verification boundary, in three honest tiers.**

- *Unit:* `classify` table test (~25 cases) covering `objects/` exclusion,
  `HEAD`/`refs/` inclusion, `index.lock` suppression, ignore-set membership, and
  linked-worktree `git_dir`. Debounce coalescing with an injected clock.
  `watch_key` normalization against the real mixed-separator string from
  `opened-workroots.json` and its all-forward-slash twin. `IgnoreSet` parsing of
  a fixed `-z` byte string. Reconcile decision table: `Degraded` does not re-arm
  on the reconcile cadence, and arming bumps both epochs.
- *Integration:* new `crates/daemon/tests/git_watch.rs` against a real temp
  repo — arm, `fs::write` an untracked file, poll the `worktree` epoch until
  bumped or 5 s deadline, assert `refs` did **not** bump; `git switch -c` ⇒
  `refs` bumped; `git worktree add` ⇒ `refs` bumped; a file under a gitignored
  `target/` ⇒ **no** bump. Availability lifecycle: rename the root away ⇒
  reconcile disarms and bumps ⇒ status reports unavailable ⇒ rename back ⇒
  re-arms. Windows: worktree-remove while armed does not fail with a sharing
  violation (existing worktree-remove pins are the reference). All
  deadline-polling, never fixed sleeps. **These tests only run armed on
  Windows/macOS** — gate them so a Linux CI run asserts the `Unarmed` fallback
  instead of silently passing a no-op.
- *Live-only — state as not covered by tests, do not pretend otherwise:*
  sustained spawns/s and CPU% on the Windows dogfood daemon over ≥10 min with the
  browser open, Activity pane both closed and open; buffer-overflow/rescan
  handling under a real `cargo build` storm; and end-to-end perceived freshness,
  which per the `ws-web-dashboard` Domain Rules needs a browser-level assertion in
  `frontend/e2e/` for the toolbar chip updating after an external edit.

Estimated diff ~+430/−70 production, ~+200 test, 9 files.

### Phase 5: Linux `PerDirectory` arming with a counted descriptor budget

Adds `WatchStrategy::PerDirectory`, `DirBudget`, and budget-driven degradation so
Linux/WSL daemons can arm instead of falling back to the 2 s TTL. Depends on
Phase 4 (it fills in the strategy Phase 4 declared and stubbed).

- Walk the worktree ourselves with the `IgnoreSet` applied and register
  `RecursiveMode::NonRecursive` per surviving directory, so every descriptor is
  counted — including `refs/**` and `worktrees/**`, which must **not** use
  `Recursive` here (see Constraints: notify registers those internally and they
  would be invisible to both `DirBudget` and the diag figure).
- Exclude `git_dir` / `common_dir` from **registration**, not merely from
  `classify`. `git status --ignored=matching` never reports `.git`, so nothing
  else prunes the 256-way `objects/` fanout, `objects/pack`, or `.git/modules`.
  The non-recursive `common_dir` top level, `refs/**`, and `worktrees/**` targets
  are registered explicitly and separately.
- Read the host's real `/proc/sys/fs/inotify/max_user_watches` at startup and
  enforce `min(limit * 60 / 100, WS_DASHBOARD_GIT_WATCH_MAX_DIRS)` (default cap
  6000). Over-budget ⇒ `Degraded{"watch budget exceeded"}` with **nothing
  registered**; runtime `notify::ErrorKind::MaxFilesWatch` ⇒ the same degrade,
  logged once per repo.
- A directory created inside a watched tree registers that directory and
  re-checks the budget. The known inotify race — a directory created and
  populated before registration — is exactly what the TTL fallback covers.
- Pre-arm filesystem check from Constraints: a target on a WSL `/mnt` DrvFs/9P
  mount degrades rather than arming, because it would arm successfully and never
  fire.
- Expose `registeredDirs` per repo in `/api/dashboard/diag/git`.

**Verification boundary.** Unit: `DirBudget` arithmetic and the over-budget
degrade decision; the registration-exclusion set (a `git_dir` path is never a
registration candidate even though `classify` would also ignore its events).
Integration `#[cfg(unix)]`: `WS_DASHBOARD_GIT_WATCH_MAX_DIRS=1` ⇒ `Degraded` +
short TTL and **zero** registrations, not partial arming; the armed happy path
reuses Phase 4's `git_watch.rs` assertions with the strategy forced to
`PerDirectory`. Live-only: real descriptor consumption on WSL
(`find /proc/*/fd -lname anon_inode:inotify | wc -l`) against the actual repos,
and confirmation that a `/mnt`-hosted root degrades rather than silently
reporting `Armed`.

Estimated diff ~+290/−10 production, ~+120 test, 3 files.

**Rollback ladder, each rung independent:** `WS_DASHBOARD_GIT_WATCH=off`
disables Phases 4-5; reverting Phase 5 alone leaves Linux `Unarmed` and Windows
armed; `WS_DASHBOARD_GIT_CACHE_TTL_MS=0` disables Phase 3;
`WS_DASHBOARD_GIT_PROBE_TTL_MS=0` disables the landed probe memo; reverting
Phase 2 is a self-contained rewrite of one function.

## Non-Goals

- **SSE push for git state** (the plan's own Phase 5, unrelated to this ticket's
  Phase 5). Deferred and unscheduled; revisit only if latency becomes an actual
  complaint, and not until Phase 4 has run in dogfood for a week. Shape is
  recorded in the plan file.
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

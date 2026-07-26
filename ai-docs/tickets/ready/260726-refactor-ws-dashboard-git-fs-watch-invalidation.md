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
Phase 2 → already landed, plan Phase 3 → Phase 3, plan Phase 4 → **Phase 4, which
is the entire watcher** (it was briefly split into Phase 4 + Phase 5 along a
platform seam; that split is gone — see the Phase 5 heading for the history), plan
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
  reported. Instead run one `git status --porcelain=v1 -unormal
  --ignored=matching -z` per repo at arm time and collect the `!!` entries.
  On failure: empty ignore set → watch everything (correct, just noisier).

  **Use `-unormal`, never `-uno` — verified on git 2.43.0, 2026-07-26.** An
  earlier revision of this ticket specified `-uno`, which **suppresses the `!!`
  output entirely**: on this repo the same command yields 0 ignored entries with
  `-uno` and 10 with `-unormal`. Implementing `-uno` would silently produce an
  empty ignore set on every repo — the one failure mode that looks like success,
  since watching everything is still functionally correct, just 15× more
  expensive. Do not use `-uall` either: it enumerates every untracked file
  individually, which is expensive on a dirty tree, and `-unormal` already
  reports ignored directories collapsed (`ws-dashboard/target/`,
  `.../node_modules/`), which is exactly the prefix form the walk needs.
  A unit test must pin that the constructed argv contains `-unormal`.
- **Polling is never deleted.** Phase 3's cache miss falls through to the same
  `changes_for_path` / `branches_for_path` calls that run today. The watcher
  only changes the TTL: 120 000 ms armed, 2 000 ms degraded/unarmed. Even with
  the watcher completely broken the daemon is strictly better than today,
  because the Phase 2 fan-out removal is unconditional.
- **The armed TTL is 120 s, not the 15 s the plan proposed** (owner, 2026-07-26).
  This value is *only* the missed-event safety net: every real change still lands
  within one frontend tick via the epoch bump, so the ceiling governs how stale
  state may get when the watcher silently fails, not normal freshness. The owner
  judged 1-2 minutes acceptable for that case, and stretching it is close to pure
  win — at the frontend's 5 s cadence a 15 s ceiling leaves 2 of every 3 ticks
  free, while 120 s leaves **23 of 24**, which is the difference between roughly
  a 3× and a 24× reduction in recomputes for the selected root. The degraded /
  unarmed TTL stays at 2 s precisely because that is the real polling path and
  must not inherit the safety-net number.
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
- **One registration strategy on all platforms: gitignore-aware walk +
  per-directory `NonRecursive` + counted cap** (owner, 2026-07-26). Recursive
  registration is removed from the design entirely, not kept as a
  platform-specific fast path.

  The initial objection was that Windows gives subtree watching for **one** kernel
  handle (`bWatchSubtree=TRUE`) while per-directory registration would cost
  hundreds, so recursive-on-Windows looked like a free win worth a `cfg` seam.
  The measurement above retires that objection: after pruning it is ~200
  directories per repo, ~1,800 across 9 roots — a real but acceptable cost, not
  the multi-thousand-handle blowup that was feared.

  What the uniform path buys, none of which the split path could:
  - **The count is unconditionally known**, so the cap is enforceable everywhere
    rather than being a Linux-only mechanism guarding a Linux-only risk.
  - **One code path, no `cfg` gate on arming.** The registration logic that runs
    in production on Windows is the same logic the Linux/WSL dev host executes
    under test — the split design could never test the Windows path anywhere.
  - The walk is needed on Linux regardless, so the incremental cost of using it
    everywhere is close to zero.

  **The create-before-register race is harmless here, which is why this trade is
  acceptable.** Per-directory watching classically misses files written into a
  directory created moments earlier. Under epoch semantics that does not matter:
  the `mkdir` itself is an event in a watched parent, it bumps the epoch, the
  frontend recomputes, and `git status` observes the new file. Only systems
  needing per-file event fidelity are hurt, and this one needs exactly
  "something under X changed."

  Two Windows costs must be **measured** in Phase 4 rather than assumed — `notify`
  is not yet a dependency, so neither was verifiable while writing this:
  - **Per-watch buffer footprint.** `ReadDirectoryChangesW` needs an overlapped
    buffer per watch. If that is ~16 KB, 1,800 watches is ~29 MB and acceptable;
    if it is materially larger, revisit. Report actual RSS delta on arm.
  - **Sharing-violation exposure on worktree removal** rises from 1 open
    directory handle per repo to ~200. The existing
    `git_worktree_remove_submit` reconcile hook must **disarm before** the
    removal runs, and the existing worktree-remove test pins are the reference.

  Also verify at implementation time how `notify`'s macOS backend handles
  `NonRecursive`: FSEvents is a subtree-stream mechanism, so N non-recursive
  watches may become N streams. macOS is not a deployment target here, so treat a
  finding there as a documented inefficiency rather than a blocker.
- **`notify` v8 with `default-features = false`** (avoids the
  `crossbeam-channel` pull-in); forward the callback into a
  `tokio::sync::mpsc`. Rejected `notify-debouncer-full`: it maintains a file-ID
  cache sized to the watched tree and we need no rename correlation, only
  "something under X changed."
- **gitoxide is out of scope and demoted.** Once invalidation is event-driven
  the number of git invocations collapses, so replacing the remaining few with
  an embedded implementation has little left to win.

## Constraints

- **Never register a watch we did not count.** This is the load-bearing
  invariant of the watcher (owner, 2026-07-26), and it is what removes recursive
  registration from the design. `RecursiveMode::Recursive` violates it on Linux:
  there is no kernel subtree primitive, so `notify` walks the tree and registers
  per-directory *internally*, meaning the process consumes an unbounded number of
  inotify descriptors that we cannot count, cap, or report. Exhausting them
  degrades unrelated applications on the host (editors, other watchers) and we
  would not even detect it. Therefore: **walk the tree ourselves with the ignore
  set applied and register `RecursiveMode::NonRecursive` per surviving directory,
  on every platform**, so the count is always known before a single watch is
  registered. See Decisions for why this is uniform rather than platform-split.
- **Measured directory counts (2026-07-26, this WSL2 host).** Gitignore pruning
  is a 15-19× reduction and lands at ~200 directories per repo:

  | repo | ignored entries | pruned dirs | unpruned |
  |---|---|---|---|
  | `devenv` | 27 | **200** | 3,812 |
  | `devenv/.worktree/ws-dashboard-dev` | 10 | **198** | 2,971 |
  | `devenv-dashboard-agent-client-docs` | 3 | **182** | 1,802 |
  | `devenv-dashboard-review` | 0 | **183** | 183 |

  This supersedes the earlier "~9,800 → ~2,400" figure, which came from the
  Windows-side repos and does not describe these. At ~200/repo, 9 work roots cost
  ~1,800 descriptors — comfortably inside even mainline's 8,192 default
  `max_user_watches` (this host reports **524,288**, checked 2026-07-26). The cap
  is therefore a safety valve against an unmeasured monorepo, not a limit the
  normal case approaches. **Still read the host's real limit at runtime and never
  assume it.**
- **Foreign-mount filesystems arm successfully and then never fire, so detect
  them before arming and fall back to polling.** The concrete case is WSL2
  `/mnt/*` (DrvFs / 9P), which does not deliver inotify events for changes made
  from the Windows side — exactly this project's dogfood topology — but the same
  hazard applies to network and FUSE mounts generally (NFS, CIFS/SMB, SSHFS):
  inotify is a local-VFS mechanism and cannot see writes that never pass through
  this kernel. The TTL ceiling bounds the damage to one window, but
  silent-and-armed is the worst reporting state, because the diag route would
  claim `Armed` while behaving `Unarmed`.

  Rule: **resolve the target's mount filesystem type before arming and degrade on
  anything not known-good**, i.e. allowlist local filesystems rather than
  blocklisting known-bad ones — a blocklist silently mis-arms the next
  filesystem nobody thought of, and the failure is invisible.
  `Degraded{"filesystem does not deliver events"}` puts the repo on the 2 s
  polling TTL, which is the correct answer for these mounts. Owner note
  (2026-07-26): opening a dashboard work root on a WSL `/mnt/` path is an
  anti-pattern in the first place, so this check is a guard against a
  misconfiguration, not support for it. The live daemon runs natively on Windows
  and is unaffected; this belongs to WSL-side developer daemons.
- **The cap is checked before registering anything, and an over-cap repo is left
  entirely `Degraded` — never partially armed.** Partial arming is the worst
  outcome available: it reports `Armed`, consumes descriptors, and still misses
  changes in the unregistered part of the tree. Two limits apply:
  - **Per repo:** `WS_DASHBOARD_GIT_WATCH_MAX_DIRS`, default **1024**. Chosen
    against the measured ~200 (5× headroom) and deliberately well under
    mainline's 8,192 so a single pathological repo cannot consume the host's
    budget. Over cap ⇒ `Degraded{"watch set too large: N dirs"}` on the 2 s TTL,
    which is exactly today's behavior — the fallback is never worse than the
    status quo, so a tight cap is safe by construction.
  - **Process-wide:** on Linux additionally cap the sum at
    `min(max_user_watches * 60 / 100, 8192)`, read from
    `/proc/sys/fs/inotify/max_user_watches` at startup. Windows has no
    equivalent global limit; the per-repo cap plus the measured RSS figure is the
    control there.
- **Arm in a deterministic order (sort by `WatchKey`)** so the same registry
  produces the same armed/degraded split across restarts when the process-wide
  budget cannot cover every repo. Never order by size, recency, or selection: a
  heuristic that re-arms a different repo each boot makes a degraded repo look
  intermittent, which is far harder to diagnose than a consistently degraded one.
  Already-armed repos keep their registrations across a reconcile; the budget is
  consulted only for repos being newly armed.
- **Exclude `git_dir` / `common_dir` from *registration*, not merely from
  `classify`.** These are different operations and only the first saves
  descriptors. `git status --ignored=matching` never reports `.git`, so nothing
  else prunes the 256-way `objects/` fanout, `objects/pack`, or `.git/modules`.
  The watched git-internal paths (`common_dir` top level, `refs/**`,
  `worktrees/**`) are registered explicitly and separately, and they too are
  `NonRecursive` and counted.
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
  ceiling (120 s armed / 2 s degraded), not the mechanism, plus the guarantee
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
described under Already Landed. **The shippable ticket is Phases 1-4**; the Phase
5 heading is retained only as a record of a split that was made and then undone.

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

### Phase 4: The `notify` watcher — real epochs, one strategy on every platform

New `crates/daemon/src/work_root_watch.rs`. **Single registration strategy:
gitignore-aware walk → per-directory `NonRecursive` → counted against a cap →
arm all or degrade wholly.** No recursive registration, no `WatchStrategy` enum,
no `cfg` gate on arming. This absorbs what an earlier revision split off as
Phase 5; see Decisions for the measurement that made the uniform path the cheaper
choice and Constraints for the counting invariant it rests on.

Every target arms under `auto`, including Linux, because the walk makes the
descriptor count known before anything is registered — which was the sole reason
Linux had been held back.

**What is and is not `cfg`-gated.** Nothing about arming is. The only
platform-conditional code is the process-wide inotify budget
(`/proc/sys/fs/inotify/max_user_watches`, `#[cfg(target_os = "linux")]`, absent
elsewhere) and the mount-type resolution behind the filesystem allowlist. Keep
`classify`, `IgnoreSet` derivation/parsing, `RepoEpochs`, debounce/coalescing, the
walk-and-count itself, and the `reconcile` decision table `cfg`-free and pure:
that is where the correctness risk lives, and it must stay unit-testable on the
Linux/WSL dev host where development actually happens.

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
2. **`IgnoreSet::derive(worktree)`** — one `git_exec::capture` with **`-unormal`**
   (see Constraints: `-uno` returns nothing), parse `!!` entries from `-z` output.
   Entries arrive as collapsed directory prefixes, which is the form the walk
   consumes directly.
3. **`plan_watch_set(worktree, git_dir, common_dir, &IgnoreSet) ->
   Result<Vec<PathBuf>, TooLarge>`** — the walk, and a pure-enough function to
   test against a fixture tree. Descend from `worktree`, prune any directory in
   the ignore set and prune `git_dir` / `common_dir` from **registration** (not
   merely from `classify` — see Constraints), then append the git-internal targets
   explicitly (`common_dir` top level, `refs/**`, `worktrees/**`). Count as it
   goes and bail with `TooLarge { found }` the moment the per-repo cap is
   exceeded, so a pathological monorepo costs a partial walk rather than a full
   one.
4. **Arming — all-or-nothing, per-directory `NonRecursive`.** Register every
   planned directory; on any registration error unregister what was already added
   and report `Degraded`, because a half-armed repo reports `Armed` while missing
   changes. Two pre-arm gates before touching `notify`:
   - **Cap check** from step 3 ⇒ `Degraded{"watch set too large: N dirs"}`.
   - **Filesystem allowlist** (from Constraints): resolve the target's mount type
     and degrade on anything outside the known-local set, because foreign mounts
     (WSL2 `/mnt` DrvFs/9P, NFS, CIFS, SSHFS/FUSE) arm successfully and then never
     fire. This is the guard that keeps the diag route from reporting `Armed` for
     a watcher that structurally cannot work.

   Bump both epochs **on arm**, not only on disarm: a slot filled while `Unarmed`
   carries epoch 0, and without a bump it stays valid for the whole 120 s TTL even
   though it was computed during a window in which no events were observed — as
   are any changes landing during the walk itself.
5. **Event pipeline.** `notify` callback (own thread) → `mpsc::unbounded_send` →
   one long-lived tokio task: coalesce 100 ms trailing / 500 ms max
   (`WS_DASHBOARD_GIT_WATCH_DEBOUNCE_MS` default 100, max window fixed at 5×
   the trailing value), map each path through `classify`, bump the union of
   `EpochKind`s per repo once.
   `event.need_rescan()` (inotify `IN_Q_OVERFLOW`, `ReadDirectoryChangesW`
   buffer overflow) ⇒ bump both epochs for every repo on that watcher and set
   `Degraded{"rescan required"}` for one TTL window.
6. **New directories register incrementally, re-checking the cap.** A `Create`
   event whose path is a directory, is not ignored, and is not under
   `git_dir`/`common_dir` ⇒ register it and increment the count; if that would
   exceed the per-repo cap, disarm the repo wholly and report
   `Degraded{"watch set outgrew cap"}` rather than continuing half-covered.
   The classic per-directory race — a directory created and populated before its
   registration lands — needs no mitigation here: the `mkdir` itself is an event
   in an already-watched parent, so the epoch bumps and the next recompute sees
   the contents. Do not add a rescan for it (see Decisions).
7. **Reconcile through exactly one hook.**
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
     `IgnoreSet::derive` spawn and a several-hundred-`read_dir` walk on the 5 s
     poll handler — reintroducing the latency this ticket removes.
8. **Wire the real `EpochSource`** into `GitStateCache`; select TTL from
   `WatchHealth` (120 000 ms armed, 2 000 ms degraded/unarmed;
   `WS_DASHBOARD_GIT_CACHE_TTL_MS` overrides the degraded value).
9. **Config:** `WS_DASHBOARD_GIT_WATCH=off|auto|force`,
   `WS_DASHBOARD_GIT_WATCH_DEBOUNCE_MS`, `WS_DASHBOARD_GIT_WATCH_MAX_DIRS`
   (default 1024). Semantics, stated because the plan left `force` undefined:
   - `off` ⇒ never arm anything, every repo `Unarmed` on the 2 s TTL. The
     rollback switch.
   - `auto` (**default**, every platform) ⇒ walk, count, and arm when the repo is
     under cap and on an allowlisted filesystem; degrade otherwise. Confirmed as
     the shipping default (owner, 2026-07-26) rather than a dark rollout: the
     rollback switch already exists, and shipping dark on Windows would produce no
     events to observe, so it buys no information.
   - `force` ⇒ arm even where `auto` would pre-emptively degrade, i.e. **on a
     filesystem outside the allowlist**. Logs the override once per repo. It
     exists to diagnose a suspected-wrong allowlist on a real machine, not as a
     supported production mode. It does **not** override the cap: the cap protects
     other processes on the host, and no env var should let this daemon exhaust
     them.

   Extend `/api/dashboard/diag/git` with `{ repos: [{ health, worktreeEpoch,
   refsEpoch, lastEventMs, registeredDirs }] }`. `Degraded { reason }` must
   distinguish over-cap from foreign-filesystem from arm-error, because
   reporting `Armed` — or an undifferentiated `Degraded` — for a watcher that
   structurally cannot fire is the failure mode this route exists to catch.
   `registeredDirs` is the number the cap is enforced against and the figure to
   compare with the measurements in Constraints.

Also widen `DiscoveredWorkRoot` (`discovery.rs:324-331`) with `git_dir` /
`common_dir`, which `GitDiscovery::probe` already computes and discards.

**Verification boundary, in three honest tiers.**

- *Unit:* `classify` table test (~25 cases) covering `objects/` exclusion,
  `HEAD`/`refs/` inclusion, `index.lock` suppression, ignore-set membership, and
  linked-worktree `git_dir`. Debounce coalescing with an injected clock.
  `watch_key` normalization against the real mixed-separator string from
  `opened-workroots.json` and its all-forward-slash twin. `IgnoreSet` parsing of
  a fixed `-z` byte string, **plus an argv assertion that the constructed command
  contains `-unormal` and not `-uno`** — the measured failure in Constraints is
  silent, so it needs a pin rather than a comment. Reconcile decision table:
  `Degraded` does not re-arm on the reconcile cadence, and arming bumps both
  epochs. `plan_watch_set` against a fixture tree: ignored dirs pruned,
  `git_dir`/`common_dir` absent from the returned list while the explicit
  git-internal targets are present, and `TooLarge` returned as soon as the cap is
  crossed rather than after a full walk.
- *Integration:* new `crates/daemon/tests/git_watch.rs` against a real temp
  repo — arm, `fs::write` an untracked file, poll the `worktree` epoch until
  bumped or 5 s deadline, assert `refs` did **not** bump; `git switch -c` ⇒
  `refs` bumped; `git worktree add` ⇒ `refs` bumped; a file under a gitignored
  `target/` ⇒ **no** bump. Availability lifecycle: rename the root away ⇒
  reconcile disarms and bumps ⇒ status reports unavailable ⇒ rename back ⇒
  re-arms. Windows: worktree-remove while armed does not fail with a sharing
  violation (existing worktree-remove pins are the reference). All
  deadline-polling, never fixed sleeps. **These run armed on every platform,
  including the Linux/WSL dev host, with no `force` needed** — that is the direct
  payoff of the uniform strategy, and it means the registration code executing in
  production on Windows is exercised by every local `cargo test`. Use a local path
  (not `/mnt`) so the allowlist does not degrade the fixture. Add:
  `WS_DASHBOARD_GIT_WATCH_MAX_DIRS=1` ⇒ `Degraded`, **zero** registrations, and
  the 2 s TTL still serving correct status; a directory created after arming gets
  registered and a write inside it bumps; and `mkdir sub && write sub/f` in one
  step still bumps (the race is covered by the parent's event, per Decisions).
- *Live-only — state as not covered by tests, do not pretend otherwise:*
  sustained spawns/s and CPU% on the Windows dogfood daemon over ≥10 min with the
  browser open, Activity pane both closed and open; **daemon RSS delta on arm
  across all 9 roots, which is the unverified `ReadDirectoryChangesW` per-watch
  buffer cost from Decisions and the one number that could still invalidate the
  uniform strategy on Windows**; real descriptor consumption on WSL
  (`find /proc/*/fd -lname anon_inode:inotify | wc -l`) against
  `registeredDirs`; buffer-overflow/rescan handling under a real `cargo build`
  storm; and end-to-end perceived freshness, which per the `ws-web-dashboard`
  Domain Rules needs a browser-level assertion in `frontend/e2e/` for the toolbar
  chip updating after an external edit.

Estimated diff ~+520/−70 production, ~+260 test, 9 files. Larger than the earlier
split-strategy estimate for Phase 4 alone because the walk, cap, and incremental
registration formerly scoped as Phase 5 are now in scope here — but smaller than
the old Phase 4 + Phase 5 sum, since there is only one registration path to write
and test.

### Phase 5: Linux `PerDirectory` arming with a counted descriptor budget [absorbed into Phase 4]

**Not a separate phase. Superseded on 2026-07-26** — retained unrenumbered per
ticket conventions so the history of the decision is legible. There is nothing to
implement here; Phase 4 is the whole watcher.

History, because this heading changed meaning twice in one day and the commit log
alone will not make that clear:

1. Originally a separate phase: Linux would walk and register per-directory under
   a `DirBudget` while Windows/macOS used one recursive registration, split off
   from Phase 4 to give the risky half its own revert boundary.
2. Then **dropped** by the owner, on the grounds that `git status` is already fast
   on Linux and per-directory state invites handle-accounting bugs — leaving Linux
   permanently on the 2 s polling TTL.
3. Then **absorbed** by the owner, who inverted the conclusion: rather than drop
   the walk, make it mandatory on *every* platform and delete recursive
   registration instead, since the walk is what makes the descriptor count
   knowable and the cap enforceable. That removed the platform split the drop was
   working around, so the phase has no distinct content left.

The measurement that justified step 3 is in Constraints (~200 pruned directories
per repo, not the ~2,400 previously assumed), and the reasoning is in Decisions.

**Rollback ladder, each rung independent:** `WS_DASHBOARD_GIT_WATCH=off`
disables Phase 4; `WS_DASHBOARD_GIT_CACHE_TTL_MS=0` disables Phase 3;
`WS_DASHBOARD_GIT_PROBE_TTL_MS=0` disables the landed probe memo; reverting
Phase 2 is a self-contained rewrite of one function.

## Non-Goals

- **Recursive watch registration, on any platform** (owner, 2026-07-26). Not
  merely unused — deliberately absent, including as a per-platform fast path and
  including behind an env var. It cannot satisfy the counting invariant in
  Constraints on Linux, and keeping it only for Windows would leave the
  production registration path untestable on the dev host. The
  `260726-refactor-ws-dashboard-long-uptime-leak-hardening` concern about
  per-directory state is real and is answered by the cap plus the all-or-nothing
  arm rule, not by avoiding the walk.
- **SSE push for git state** (the plan's own Phase 5, unrelated to this ticket's
  Phase 5 heading). Deferred and unscheduled; revisit only if latency becomes an
  actual complaint, and not until Phase 4 has run in dogfood for a week. Shape is
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

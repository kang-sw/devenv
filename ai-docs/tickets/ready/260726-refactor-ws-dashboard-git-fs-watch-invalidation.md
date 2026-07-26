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
- **Registration is platform-split, and the split is the cheap answer rather than
  a compromise.** `RecursiveMode::Recursive` on Windows/macOS; gitignore-aware
  walk + per-directory `NonRecursive` + counted cap on Linux. Both satisfy the
  counting invariant in Constraints — Windows because the count is 1 by
  construction, Linux because we perform the walk ourselves.

  This was briefly decided the other way (uniform per-directory everywhere,
  recursive deleted) and reversed the same day, so the reasoning is written down to
  keep it from oscillating again.

  The case for unifying was testability: with one path, the registration code
  running in production on Windows would be exercised by every `cargo test` on the
  Linux dev host. **That argument does not survive inspection.** The only
  Windows-exclusive code under the split is three `watcher.watch(target,
  RecursiveMode::Recursive)` calls. Everything with real failure modes —
  `classify`, `IgnoreSet` derivation, debounce/coalescing, epoch bookkeeping, the
  `reconcile` decision table — is shared, `cfg`-free, and tested on Linux either
  way; and `plan_watch_set` is Linux-only code running on the Linux dev host, so it
  is covered too. The split forfeits coverage of three trivial lines.

  What unifying would have cost on the only production host:
  - ~200 open directory handles per repo instead of **1** (~1,800 across 9 roots).
  - A several-hundred-`read_dir` walk on every arm, where recursive needs none.
  - An **incremental-registration state machine** for directories created after
    arming, which the kernel handles for free under `bWatchSubtree=TRUE`. This is
    exactly the long-lived per-directory bookkeeping that can leak handles over
    days of uptime — the concern behind
    `260726-refactor-ws-dashboard-long-uptime-leak-hardening` — imported into
    Windows to buy nothing.
  - Cap-enforcement logic live in the production path instead of dormant.
  - Sharing-violation exposure on worktree removal across ~200 handles, not 1.

  Trading that for three lines of coverage is a bad trade, and the gitignore event
  noise recursive registration accepts (see Constraints) is not a reason to pay it.

  **The create-before-register race is harmless under epoch semantics**, which is
  what makes the Linux per-directory path acceptable. Per-directory watching
  classically misses files written into a directory created moments earlier. Here
  the `mkdir` is itself an event in an already-watched parent, so the epoch bumps,
  the frontend recomputes, and `git status` observes the new file. Only
  per-file-fidelity systems are hurt, and this one needs exactly "something under
  X changed."
- **`notify` v8 with `default-features = false`**; forward the callback into a
  `tokio::sync::mpsc`. Do **not** justify this as "avoids the `crossbeam-channel`
  pull-in" — that reason is void: `crossbeam-channel` is already in `Cargo.lock`
  via `tracing-appender`, a direct daemon dependency (`crates/daemon/Cargo.toml:27`,
  used in `logging.rs`). The decision stands on keeping one channel type on the
  event path rather than on dependency count. Rejected `notify-debouncer-full`: it
  maintains a file-ID cache sized to the watched tree and we need no rename
  correlation, only "something under X changed."
- **gitoxide is out of scope and demoted.** Once invalidation is event-driven
  the number of git invocations collapses, so replacing the remaining few with
  an embedded implementation has little left to win.

## Constraints

- **Never register a watch we did not count.** This is the load-bearing invariant
  of the watcher (owner, 2026-07-26). It is a constraint on *counting*, not a
  mandate for per-directory registration — and the two platforms satisfy it
  differently:
  - **Windows/macOS: satisfied structurally.** `ReadDirectoryChangesW` with
    `bWatchSubtree=TRUE` is one kernel handle for the whole tree; FSEvents is one
    stream. The count is 1 per target by construction, so there is nothing to walk
    and nothing to cap.
  - **Linux: violated by `RecursiveMode::Recursive`.** There is no kernel subtree
    primitive, so `notify` walks the tree and registers per-directory
    *internally* — the process consumes inotify descriptors we cannot count, cap,
    or report, and exhausting them degrades unrelated applications on the host
    (editors, other watchers) without us detecting it. So on Linux we walk the
    tree ourselves with the ignore set applied and register
    `RecursiveMode::NonRecursive` per surviving directory, making the count known
    before a single watch is registered.

  See Decisions for why this stays platform-split rather than being unified onto
  the per-directory path.

  **Read both inotify limits, not just the watch count.**
  `/proc/sys/fs/inotify/max_user_instances` is **128** on this host and is consumed
  one per `notify` watcher instance; `max_user_watches` bounds descriptors within an
  instance. Harmless at 9 roots with a single shared watcher, but the ticket's own
  "never assume the host's limit" rule has to cover both, and it constrains any
  future move to one watcher per repo.
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
  ~1,800 descriptors on Linux — comfortably inside even mainline's 8,192 default
  `max_user_watches` (this host reports **524,288**, checked 2026-07-26). The cap
  is therefore a safety valve against an unmeasured monorepo, not a limit the
  normal case approaches. **Still read the host's real limit at runtime and never
  assume it.** These counts apply to Linux only; Windows/macOS register one watch
  per target regardless of tree size.
- **The ignore set is needed on both platforms, but for different jobs.** On Linux
  it decides *what to register*; on Windows/macOS it only *filters events*, since
  the kernel already delivers the whole subtree. So recursive registration does
  deliver events for `target/` and `node_modules/`, discarded in `classify`.
  Sizing that cost, because it is the main objection to recursive registration:
  - **The kernel side does not touch the filesystem.** `ReadDirectoryChangesW`'s
    filter is evaluated in the kernel notify path at the moment of the write, when
    the path string is already in hand — no `stat`, no directory read. It appends a
    `FILE_NOTIFY_INFORMATION` record (action code + UTF-16 relative path) to a
    pinned buffer, and that cost is borne by the *writer*, i.e. the compiler, as a
    small addition to a write it just performed.
  - **The cost we pay is userspace, per record:** a UTF-16→UTF-8 decode, a
    `PathBuf` allocation, a channel send, then `classify`. Measured scale in this
    workspace: 16,531 files under `target/`, 17,817 under
    `frontend/node_modules/`, average relative path 105 chars (≈223 bytes per
    record). A clean build is order 10⁴-10⁵ events; at roughly a microsecond each
    that is tens of milliseconds against a build spending tens of CPU-seconds —
    well under 1%. **The per-event figure is an estimate, not a measurement**, and
    is listed under live-only verification for that reason.
  - **The filter cannot exclude paths.** `ReadDirectoryChangesW` filters by change
    *type*, not by subtree, so there is no way to suppress `target/` at the source.
    This is a genuine place where the Linux path is cheaper — ignored directories
    are never registered, so they generate zero events. It is not enough to
    outweigh the handle-count difference, but it is why `classify`'s branch order
    (see Phase 4) is treated as load-bearing rather than cosmetic.
  - **The real risk is buffer overflow, and it degrades correctly.** If the buffer
    fills before we drain it, the kernel signals `ERROR_NOTIFY_ENUM_DIR` and
    `notify` surfaces `need_rescan()`. At ~223 bytes per record a 16 KB buffer holds
    ~73 records, so a large build produces many completions rather than one
    overflow; a burst that writes thousands of files at once (`git checkout`) can
    still overflow. That is not lost data under this design: overflow ⇒ bump both
    epochs + `Degraded` for one window, i.e. "recompute once", which during a build
    is the correct answer anyway since files genuinely are changing.
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

  **Allowlist membership, per platform — enumerate it here rather than leaving it
  to be invented**, since this gate runs on every armed platform and an
  over-conservative guess degrades every repo:
  - **Linux:** resolve the mount via `/proc/self/mountinfo` and allow
    `ext2|ext3|ext4|btrfs|xfs|f2fs|zfs|overlay|tmpfs`. `overlay` and `tmpfs` are
    in deliberately — container roots and `TMPDIR`-backed test fixtures are local
    VFS and do deliver events, and excluding them would fail Phase 4's own
    integration tier on any host whose temp dir is not on a disk filesystem. Reject
    everything else, notably `9p`/`drvfs` (WSL2 `/mnt/*`, verified `9p` on this
    host), `nfs*`, `cifs`/`smb*`, `fuse*`, `sshfs`.
  - **Windows:** this is the production host, so it needs the same gate, not an
    exemption — a mapped network drive or a UNC work root has the identical
    silent-and-armed failure. Use `GetDriveType` plus a `\\`-prefix check on the
    resolved path: allow `DRIVE_FIXED` and `DRIVE_RAMDISK`; reject
    `DRIVE_REMOTE`, `DRIVE_REMOVABLE`, `DRIVE_CDROM`, `DRIVE_UNKNOWN`, and any UNC
    path. (`ReadDirectoryChangesW` can work over some SMB mounts but not
    dependably, and "sometimes armed" is the state this rule exists to forbid.)
  - **macOS:** `statfs.f_fstypename` in `{apfs, hfs}`; reject `nfs`, `smbfs`,
    `webdav`, `osxfuse`/`macfuse`.
  - Resolution failure ⇒ degrade, never assume local.
  `Degraded{"filesystem does not deliver events"}` puts the repo on the 2 s
  polling TTL, which is the correct answer for these mounts. Owner note
  (2026-07-26): opening a dashboard work root on a WSL `/mnt/` path is an
  anti-pattern in the first place, so this check is a guard against a
  misconfiguration, not support for it. The live daemon runs natively on Windows
  and is unaffected; this belongs to WSL-side developer daemons.
- **Linux only — the cap is checked before registering anything, and an over-cap
  repo is left entirely `Degraded`, never partially armed.** Partial arming is the
  worst outcome available: it reports `Armed`, consumes descriptors, and still
  misses changes in the unregistered part of the tree. Two limits apply:
  - **Per repo:** `WS_DASHBOARD_GIT_WATCH_MAX_DIRS`, default **1024**. Chosen
    against the measured ~200 (5× headroom) and deliberately well under
    mainline's 8,192 so one pathological repo cannot consume the host's budget.
    Over cap ⇒ `Degraded{"watch set too large: N dirs"}` on the 2 s TTL, which is
    exactly today's behavior — the fallback is never worse than the status quo, so
    a tight cap is safe by construction.
  - **Process-wide:** cap the sum at `min(max_user_watches * 60 / 100, 8192)`,
    read from `/proc/sys/fs/inotify/max_user_watches` at startup.

  Neither limit exists on Windows/macOS, where each target costs one watch and no
  walk runs.
- **Linux only — arm in a deterministic order (sort by `WatchKey`)** so the same
  registry produces the same armed/degraded split across restarts when the
  process-wide budget cannot cover every repo. Never order by size, recency, or
  selection: a heuristic that re-arms a different repo each boot makes a degraded
  repo look intermittent, which is far harder to diagnose than a consistently
  degraded one. Already-armed repos keep their registrations across a reconcile;
  the budget is consulted only for repos being newly armed.
- **Linux only — exclude `git_dir` / `common_dir` from *registration*, not merely
  from `classify`.** These are different operations and only the first saves
  descriptors. `git status --ignored=matching` never reports `.git`, so nothing
  else prunes the 256-way `objects/` fanout, `objects/pack`, or `.git/modules`.
  The watched git-internal paths (`common_dir` top level, `refs/**`,
  `worktrees/**`) are registered explicitly and separately, `NonRecursive` and
  counted. On Windows/macOS the recursive worktree registration already covers an
  in-tree `.git`, and those events are dropped in `classify` instead — which is
  why `classify` must keep its `objects|lfs|modules` exclusions regardless of
  platform.
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

### Result (0c48065a) - 2026-07-26

Landed as `272a2912` (seam) plus three review-fix commits `4aa86b10`,
`5c6b4f2b`, `0c48065a`. Actual size ~+1,500/−200 across 11 files, roughly six
times the estimate — the estimate counted the seam and forgot that threading an
explicit `&GitSpawnStats` touches every intermediate signature between a route
handler and a git call.

Behavioral delta beyond the plan:

- **`GitOutcome` gained `output_truncated`, and stdout access is now
  accessor-only for parsing callers.** `stdout_strict()` returns `None` on
  truncation or invalid UTF-8; `stdout_text()` returns `None` on truncation. A git
  command that exits while a descendant still holds the inherited pipes reports
  its **real exit status** — a zero-exit `push` succeeds, a non-zero `fetch` still
  returns `Status(code)` — while parsing callers refuse the short read. Reached
  after a wrong first decision, recorded below.
- **`WS_DASHBOARD_GIT_TIMEOUT_MS=0` means unbounded**, matching the
  `0`-disables convention of the sibling `WS_DASHBOARD_GIT_PROBE_TTL_MS` rather
  than the "kill everything immediately" the first implementation gave it.
- **`GitDiscovery::probe` is `ExpectedNonZero`**, not `Unexpected` as the plan
  said: its `rev-parse` batch exits non-zero by design for a plain-directory
  root, so `Unexpected` would emit one warning per root per probe-TTL expiry —
  the noise this phase exists to remove. `changes_for_path`'s `diff-index … HEAD`
  moved for the same reason (exit 128 on an unborn HEAD, on the 5 s poll path).
  `probe_git_worktree_paths` stayed `Unexpected`; it runs only after the root is
  known to be a repo.
- **`open_work_root` was an uncounted path** the plan's call-site enumeration
  missed: `root_picker.rs` built `LocalDashboardResourcesProvider::new` without
  the stats handle, so a live production route's discovery probes landed in a
  discarded counter. Threaded. The post-fix sweep confirms the only remaining
  discarded counter outside `#[cfg(test)]` is `resolve_work_root_agents_dir`'s,
  which has zero `src/` callers and carries a comment saying so.
- **Deadline is bounded on every exit path, not just expiry.** Output collection
  uses `mpsc::recv_timeout` against one shared absolute deadline plus a 50 ms
  post-exit grace, so the `try_wait → Ok(Some(status))` path cannot block either.
- Poll-quantum backoff (250 µs → ×2 → 5 ms cap) instead of a fixed 10 ms sleep,
  which measured 10.7 ms median per spawn against a 1.2-1.9 ms `.output()`
  baseline.

Verification: 142 lib tests (14 in `git_exec`), 167 `tests/routes.rs` tests —
the route count is unchanged from baseline, which is this phase's own tripwire for
"no observable git behavior changed". Clippy warning set byte-identical to
baseline. All three ticket-named unit pins exist and were checked for vacuity by
review: the >1 MB pin uses 2 MB so it genuinely fails against an undrained
implementation, and the no-logging pin uses a real `tracing::Subscriber` event
counter with a positive-control sibling.

**Deviations from the plan, and one lead error.** The plan's recommendation to
give `resolve_work_root_agents_dir` a throwaway counter was kept but its stated
rationale replaced: it holds because that function has zero production callers,
not because "nothing user-facing reads its counters" — a reason that would also
license hiding the hot Activity path. More seriously, the lead's cycle-2
disposition told the implementer to report `Timeout` when a child exited but its
output could not be collected, on the reasoning that truncated-but-successful
output is more dangerous. That was wrong in a way that mattered: the callers that
provoke descendants (`run_git` → switch/fetch/push/pull) discard stdout, while the
callers that parse stdout do not spawn descendants, so the rule fired only where
it was useless and turned a **successful push into `400 "push failed"`** under a
common `ssh ControlPersist` configuration. Reversed in cycle 3 by splitting the
concern instead of choosing a side.

**Unresolved and deferred.**

- Live Windows dogfood verification — two `/api/dashboard/diag/git` reads 60 s
  apart — was **not** performed; the sandbox is Linux/WSL2. The acceptance number
  for Phases 2 and 4 therefore does not exist yet, and one of them will need it.
- The `#[cfg(windows)]` counterparts for the kill-on-timeout and >1 MB pins were
  written but never executed. Windows is the production host, so first run there
  may find them wrong.
- Each timeout deliberately detaches two reader threads and two pipe handles,
  permanent if the descendant is immortal. Forwarded to
  `260726-refactor-ws-dashboard-long-uptime-leak-hardening` Phase 2, which also
  now owns the observation that `kill()`/`wait()` are unbounded against a child
  wedged in uninterruptible I/O — so "bounded on every path" means "bounded except
  an unkillable child".
- `git_worktree.rs`'s 8 direct `Command::new("git")` sites stay outside the seam
  and the counters by design. Filed as
  `260726-refactor-dashboard-worktree-git-spawns-through-exec-seam`; the budget,
  not the counting, is the open design question there.
- `WorkRootActivityProjector::project` is dead public surface that now carries a
  `git_stats` parameter. Left alone deliberately — deleting public surface is its
  own decision.
- The review loop ran to its 3-cycle cap. Each cycle produced a genuinely new,
  genuinely valid Critical/Important finding in the same function, which is the
  divergence case the cap exists to convert into an owner decision. Nothing in the
  live playbook surfaced the cap; filed as
  `260726-bug-lead-implement-lost-review-relay-cycle-cap`.

Closes `260724-idea-dashboard-daemon-side-git-poll-response-timeout` for the poll
path only; the worktree flows named above remain unbounded.

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
  own (the reconcile hook that carries availability arrives in Phase 4): cache
  `Some` under the normal TTL, and cache `None` under a **short negative TTL**
  (`WS_DASHBOARD_GIT_IDENTITY_NEGATIVE_TTL_MS`, default 3000) so the
  not-yet-a-repo case still self-corrects within one user-visible beat.
  Phase 4's reconcile then evicts on any non-`Available` transition.

  **A short negative TTL, not "always re-probe on `None`"** — an earlier revision
  said the latter and it left the headline storm intact. `git_identity` returns
  `None` for non-git roots, bare repos, and any repo whose common dir is not named
  `.git`, so under always-re-probe a **plain-directory work root with the Activity
  pane open sustains ~10-20 spawns/s indefinitely** through the 200 ms SSE loop
  (`work_root_activity.rs:378` → `watch_snapshot_blocking` →
  `resolve_work_root_state_dir` at `:486` → `git_identity`, 2 spawns) — the exact
  number this bullet exists to remove, in one of the configurations it lists as
  motivation. The `git init` argument above rules out a *permanent* negative
  cache, not a 3 s one: three seconds is below the 3 s activity poll and far below
  any human reaction time, so the self-correcting property survives intact while
  the storm does not. This kills
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
   whenever the Activity pane is open. This is the measurable win, **and it only
   materializes if `None` is cached under the short negative TTL**: the pane's
   200 ms loop hits `git_identity` for every root, and `None` is what a
   plain-directory or bare-repo root returns, so an always-re-probe policy would
   leave the headline number untouched for exactly those roots. Verify with a
   plain-directory root selected, not only a git one.
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
- **Sample the epoch BEFORE invoking git, and stamp the slot with that sample.**
  Reading it after `git status` returns absorbs any write that landed
  mid-invocation into a slot stamped with the newer epoch, which then reads as
  valid for the entire TTL. This is structurally the same hole the leading-only
  debounce discussion in Phase 4 rejects, on the read side instead of the write
  side, and the 120 s armed TTL makes it 60× more damaging than on the 2 s path.
  A unit test must pin it: bump the epoch between the sample and the fill, then
  assert the next read is a miss.
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

### Phase 4: The `notify` watcher — real epochs on every platform

New `crates/daemon/src/work_root_watch.rs`. **Every platform arms under `auto`**,
which is what this phase absorbed from the former Phase 5 — Linux is no longer
held back on the polling TTL. Registration is platform-split per Decisions:

- **Windows/macOS:** one `RecursiveMode::Recursive` registration per target. One
  kernel handle / one FSEvents stream, no walk, no cap. The ignore set is used only
  to filter incoming events.
- **Linux:** gitignore-aware walk → per-directory `NonRecursive` → counted against
  the cap → arm all or degrade wholly. Required because there is no kernel subtree
  primitive and `notify`'s emulation would register descriptors we cannot count.

**What is `cfg`-gated — keep this list short.** Only the registration backend
(recursive vs. walk-and-register), the process-wide inotify budget
(`#[cfg(target_os = "linux")]`), and mount-type resolution for the filesystem
allowlist. Keep `classify`, `IgnoreSet` derivation/parsing, `RepoEpochs`,
debounce/coalescing, and the `reconcile` decision table `cfg`-free and pure: that
is where the correctness risk lives and it must stay unit-testable on the
Linux/WSL dev host. `plan_watch_set` is Linux-only but needs no `cfg` on its logic
— it is a pure function over a path and an ignore set, and it is exercised on the
dev host as a matter of course.

Implement in this order, because the first step is the correctness core and is
fully testable without any I/O:

1. **`classify(path, &ArmedRepo) -> Option<EpochKind>`** — pure function, in this
   order:
   1. **Ignore-set match ⇒ ignore.** First, because it is the hot path: recursive
      registration delivers every gitignored-tree event on Windows (see
      Constraints), so a `cargo build` sends order 10⁴-10⁵ paths through this
      function and they all exit here, on one test. This is safe to put first
      because `git status --ignored` never reports paths under `.git`, so no
      git-internal path can match the ignore set.
   2. **Explicit ignore-rule files ⇒ `Worktree`** + signal the ignore set stale:
      any `.gitignore`, plus `common_dir/info/exclude`. **`info/exclude` must be
      matched here, before the git-dir branch below**, or it is unreachable — it
      lives at `$GIT_COMMON_DIR/info/exclude` by definition, so a git-dir-first
      order routes it into the git chain where it matches nothing and is silently
      dropped. On Linux this also means `common_dir/info/` must be added to the
      registered git-internal targets in step 3; the previously specified set
      (`common_dir` top level, `refs/**`, `worktrees/**`) never watches it.
   3. **Under `git_dir` / `common_dir`:** `objects|lfs|modules` ⇒ ignore; `*.lock`
      ⇒ ignore (git's create-then-rename lock dance is pure noise, and
      `index.lock` churn is exactly what `260711` was filed about);
      `{HEAD,packed-refs,FETCH_HEAD,ORIG_HEAD}` or `refs/**` or `worktrees/**` ⇒
      `Refs`; `index` ⇒ `Worktree`; **anything else under a git dir ⇒ ignore** —
      an explicit fallthrough, so `config`, `COMMIT_EDITMSG`, `hooks/`, and
      `logs/` are decided rather than left to the reader.
   4. **Otherwise ⇒ `Worktree`.**

   **Ignore-set entries are path prefixes, and they are not all directories.**
   Measured on this repo: of the 10 `!!` entries, **5 are files** —
   `.claude/scheduled_tasks.lock`, `ai-docs/_index.local.md`,
   `ai-docs/_install.local.sh`, `ai-docs/tickets/ready/.gitkeep-local`. Match
   directory entries (trailing `/`) as prefixes and file entries as exact paths.
   Treating the set as directories-only — which earlier wording did — leaves an
   ignored file inside a tracked directory neither pruned nor filtered, so every
   write to it bumps `Worktree` and buys a recompute. Two of those five files are
   written by this workflow, so the daemon would recompute git status on its own
   routine writes in its own repo.

   `classify` returns only a classification — it never spawns git and never
   mutates the ignore set. The stale signal is returned to the caller and handled
   by the pipeline in step 5.
2. **`IgnoreSet::derive(worktree)`** — one `git_exec::capture` with **`-unormal`**
   (see Constraints: `-uno` returns nothing), parse `!!` entries from `-z` output.
   Keep the trailing-`/` distinction from the raw output: directory entries become
   prefixes, file entries become exact matches (see step 1). Verified property the
   walk depends on: `-unormal --ignored=matching` **does** report ignored
   directories nested inside untracked directories (`!! untracked_dir/build/`), so
   pruning will not miss those.
3. **`plan_watch_set(worktree, git_dir, common_dir, &IgnoreSet) ->
   Result<Vec<PathBuf>, TooLarge>`** — the Linux walk, written as a pure function
   over a fixture tree. Descend from `worktree`, prune any directory matching the
   ignore set and prune `git_dir` / `common_dir` from **registration** (not merely
   from `classify` — see Constraints), then append the git-internal targets
   explicitly: `common_dir` top level, **`common_dir/info/`** (required by step 1's
   `info/exclude` rule), `refs/**`, `worktrees/**`. Count as it goes and bail with
   `TooLarge { found }` the moment the per-repo cap is crossed, so a pathological
   monorepo costs a partial walk rather than a full one.
4. **Arming.** Both paths share the target-dedup rule, one pre-arm gate, and one
   post-arm rule.
   - **Dedup targets by `WatchKey`, then fan events out to every owning repo.**
     A primary root and its linked worktrees **share one `common_dir`** — verified
     in the measured topology itself: `/home/swkang/devenv` and
     `/home/swkang/devenv/.worktree/ws-dashboard-dev`, the two repos in the
     Constraints table, both report `/home/swkang/devenv/.git`. Registering it once
     per root would deliver every shared-`.git` write N times on Windows and
     double-count it against the Linux cap. Maintain one registration per distinct
     target with a reverse index target → owning repos; an event bumps the
     classified epoch for each owner.
   - **Windows/macOS:** `watch(target, RecursiveMode::Recursive)` for the worktree
     plus, for a linked worktree, `git_dir` and `common_dir` when they sit outside
     it. No walk, no cap. **Note the volume this admits:** a recursive `common_dir`
     registration covers the 256-way `objects/` fanout and packfiles, which the
     event-cost sizing in Constraints does not include (it sizes only worktree-side
     `target/` and `node_modules/`). Those events are dropped by `classify` step 3,
     but they are the reason `objects|lfs|modules` exclusion stays first in the
     git-dir arm, and the live-only measurement must cover a `git gc` / fetch as
     well as a `cargo build`.
   - **Linux:** register every path from step 3 `NonRecursive`, all-or-nothing —
     on any registration error, unregister what was already added and report
     `Degraded`, because a half-armed repo reports `Armed` while missing changes.
     Cap breach from step 3 ⇒ `Degraded{"watch set too large: N dirs"}`.
   - **Shared pre-arm gate — filesystem allowlist** (from Constraints): resolve the
     target's mount type and degrade on anything outside the known-local set,
     because foreign mounts (WSL2 `/mnt` DrvFs/9P, NFS, CIFS, SSHFS/FUSE) arm
     successfully and then never fire. This is the guard that keeps the diag route
     from reporting `Armed` for a watcher that structurally cannot work.
   - **Shared post-arm rule:** bump both epochs **on arm**, not only on disarm. A
     slot filled while `Unarmed` carries epoch 0, and without a bump it stays valid
     for the whole 120 s TTL even though it was computed during a window in which
     no events were observed — as are any changes landing during arming itself.
5. **Event pipeline.** `notify` callback (own thread) → `mpsc::unbounded_send` →
   one long-lived tokio task that maps each path through `classify` and bumps the
   resulting `EpochKind`s.

   **At most one git spawn per 30 s per repo originates from this path**, and none
   at all from an ordinary file event. A bump is an `AtomicU64` increment marking a
   cache slot for recomputation; git runs when a route is served, i.e. on the
   frontend's existing poll. So a burst of ordinary writes of any size costs N
   atomic increments and **zero** spawns, and the effective minimum interval
   between git invocations is the poll cadence (5 s per root) with no additional
   throttle needed. The floor is the status quo: a file changing continuously means
   every poll recomputes, which is exactly today's behavior. The single exception is
   the rate-limited `IgnoreSet` re-derivation below — stated as an exception rather
   than as "no git ever runs here", which an earlier revision claimed in bold six
   paragraphs above its own counterexample.

   **Coalescing is leading + trailing, at most two bumps per kind per window** —
   `WS_DASHBOARD_GIT_WATCH_DEBOUNCE_MS` default 100, window closing 100 ms after
   the last event and capped at 5× that from the first:
   - **Leading:** the first event of a kind bumps immediately, so a single save is
     visible to a poll landing 10 ms later. Latency ~0.
   - **Trailing:** if any further event of that kind arrived after the leading
     bump, bump once more when the window closes.

   Both halves are required and the reasoning belongs here because either alone is
   wrong. Trailing-only delays the bump past a poll that lands during the window,
   serving stale state for a further TTL. Leading-only is worse — it is a
   **correctness hole**: the leading bump is computed and stored by a poll at
   t+10 ms, subsequent writes in the window are suppressed, and the slot then reads
   as valid for the whole TTL despite being out of date. Leading+trailing costs one
   extra atomic increment and closes that.

   `event.need_rescan()` (inotify `IN_Q_OVERFLOW`, `ReadDirectoryChangesW`
   buffer overflow) ⇒ bump both epochs for every repo on that watcher and set
   `Degraded{"rescan required"}` for one TTL window.
   **`.gitignore` / `.git/info/exclude` changes must not re-derive the ignore set
   inline.** `IgnoreSet::derive` is a git spawn, and on Linux a changed ignore set
   also changes the registration set, so re-derivation there means a re-walk plus a
   register/unregister delta. Deriving per event would reintroduce exactly the
   spawn storm this ticket exists to remove — a tool that rewrites a `.gitignore`
   in a loop would drive one spawn per write. Instead: bump `Worktree` immediately
   (cheap, and the correct signal), mark the set stale, and schedule re-derivation
   on the watcher task **at most once per 30 s per repo**, coalescing all staleness
   marks in that interval into one. A stale ignore set costs efficiency, not
   correctness: until it refreshes, events from a newly-ignored directory still
   produce spurious `Worktree` bumps and therefore extra recomputes, while
   `git status` itself reads the ignore rules fresh on every invocation and stays
   accurate throughout. That asymmetry is what makes a long re-derive interval
   safe.

   **On Linux a new ignore set also changes the registration set, so specify that
   path rather than implying it.** Re-derivation there is: compute the new set →
   `plan_watch_set` again → if it returns `TooLarge`, disarm wholly and report
   `Degraded{"watch set outgrew cap"}` → otherwise apply the diff (unregister paths
   no longer planned, register newly planned ones) → bump both epochs, because the
   filter changed and previously-suppressed paths may now be significant. A diff,
   not a disarm-and-re-arm: re-arming would re-enter the 30 s arm-interval guard and
   re-spawn `IgnoreSet::derive` for the set just computed. On Windows/macOS there is
   no registration change — only the filter is swapped — so the same 30 s limit
   bounds one spawn and nothing else.
6. **Linux only — new directories register incrementally, re-checking the cap.**
   A `Create` event whose path is a directory, is not ignored, and is not under
   `git_dir`/`common_dir` ⇒ register it and increment the count; if that would
   exceed the per-repo cap, disarm the repo wholly and report
   `Degraded{"watch set outgrew cap"}` rather than continuing half-covered.
   Windows/macOS need none of this — the kernel covers new subdirectories under a
   recursive registration, which is a large part of why recursive is kept there.
   The classic per-directory race — a directory created and populated before its
   registration lands — needs no mitigation: the `mkdir` itself is an event in an
   already-watched parent, so the epoch bumps and the next recompute sees the
   contents. Do not add a rescan for it (see Decisions).
7. **Reconcile through exactly one hook.**
   `registry.reconcile(&[(WatchKey, Option<WatchTargets>, WorkRootAvailability)])`
   — where `WatchTargets { worktree: PathBuf, git_dir: PathBuf, common_dir:
   PathBuf }` is populated straight from the widened `DiscoveredWorkRoot`, and
   `None` means the root is not a git root — called from
   `resources::live_dashboard_resources_with_sync` after
   `sync_discovered_roots`, using the widened `DiscoveredWorkRoot` fields. Reading
   those fields costs **no extra git spawns** because that call site already
   computes the authoritative root set and availability every 5 s — but note the
   *arm path it triggers* does cost a spawn (`IgnoreSet::derive`) plus, on Linux, a
   walk, so do not restate "no extra git spawns" about reconcile as a whole.
   Semantics: present + `Available` + `Unarmed` ⇒ arm;
   present + not `Available` ⇒ disarm + bump both (so the next poll recomputes and
   reports the degraded state); absent ⇒ disarm + drop epochs. One code path
   covers register/unregister, `moved`/`missing`/`inaccessible` transitions,
   `remove_workspace`, and `git_worktree_remove_submit` instead of six separate
   hooks.
   **Two rules this hook must not get wrong:**
   - **Arm attempts are rate-limited per repo regardless of why the repo is
     unarmed.** Two separate guards, and the second is not optional:
     - `Degraded` is sticky and retried on exponential backoff (start 60 s, cap
       15 min). The arm condition is `Unarmed`, never "not armed" — `Degraded` must
       not re-enter it.
     - **A minimum 30 s interval between arm attempts per repo**, checked before
       any arm work runs. This exists because the `Degraded` backoff alone does not
       cover the availability path: `not Available ⇒ disarm` yields `Unarmed`,
       which *is* the arm-eligible state, so a flapping root re-arms on every
       reconcile tick with the backoff never consulted. Availability is recomputed
       from deliberately-uncached `fs::metadata`/`read_dir` every 5 s
       (`discovery.rs:336` CONTRACT), so a transient failure — or the rename cycle
       Phase 4's own integration tier exercises — drives arm work at the reconcile
       cadence. Each attempt costs one `IgnoreSet::derive` spawn (a
       `git status --ignored=matching` over the whole tree, the same command Phase 1
       warns can exceed the 64 KB pipe buffer) plus, on Linux, a
       several-hundred-`read_dir` walk. That is the unbounded-rate anti-pattern this
       ticket exists to remove, reachable through the one hook everything is routed
       through — so the interval guard is load-bearing, not defensive.
   - **Arming never runs inline on the resources route.** `reconcile` computes the
     desired delta and hands arm/disarm work to the watcher task over the same
     channel the event pipeline uses. Arming inline would put an
     `IgnoreSet::derive` spawn and, on Linux, a several-hundred-`read_dir` walk on
     the 5 s poll handler — reintroducing the latency this ticket removes.
8. **Wire the real `EpochSource`** into `GitStateCache`; select TTL from
   `WatchHealth` — 120 000 ms armed, 2 000 ms degraded/unarmed, with
   `WS_DASHBOARD_GIT_CACHE_TTL_MS` overriding the degraded value and
   **`WS_DASHBOARD_GIT_ARMED_TTL_MS` the armed one**. The armed ceiling is the
   least-verified number in this ticket and it must not require
   `WS_DASHBOARD_GIT_WATCH=off` — i.e. giving up watching entirely — to walk back.
9. **Config:** `WS_DASHBOARD_GIT_WATCH=off|auto|force`,
   `WS_DASHBOARD_GIT_WATCH_DEBOUNCE_MS`, and `WS_DASHBOARD_GIT_WATCH_MAX_DIRS`
   (default 1024, Linux-only effect). Semantics, stated because the plan left
   `force` undefined:
   - `off` ⇒ never arm anything, every repo `Unarmed` on the 2 s TTL. The
     rollback switch.
   - `auto` (**default**, every platform) ⇒ arm when the pre-arm gates allow;
     degrade otherwise. Confirmed as the shipping default (owner, 2026-07-26)
     rather than a dark rollout: the rollback switch already exists, and shipping
     dark on Windows would produce no events to observe, so it buys no information.
   - `force` ⇒ arm even where `auto` would pre-emptively degrade, i.e. **on a
     filesystem outside the allowlist**. Logs the override once per repo. It
     exists to diagnose a suspected-wrong allowlist on a real machine, not as a
     supported production mode. It does **not** override the cap: the cap protects
     other processes on the host, and no env var should let this daemon exhaust
     them.

   Extend `/api/dashboard/diag/git` with `{ repos: [{ health, worktreeEpoch,
   refsEpoch, lastEventMs, registeredWatches }] }`. `Degraded { reason }` must
   distinguish over-cap from foreign-filesystem from arm-error, because
   reporting `Armed` — or an undifferentiated `Degraded` — for a watcher that
   structurally cannot fire is the failure mode this route exists to catch.
   `registeredWatches` is the count the Linux cap is enforced against (compare
   with the measurements in Constraints) and is the number of recursive
   registrations, normally 1-3, on Windows/macOS — so a large value there would
   itself signal that the wrong path ran.

Also widen `DiscoveredWorkRoot` (`discovery.rs:324-331`) with `git_dir` /
`common_dir`, which `GitDiscovery::probe` already computes and discards.

**Verification boundary, in three honest tiers.**

- *Unit:* `classify` table test (~25 cases) covering `objects/` exclusion,
  `HEAD`/`refs/` inclusion, `index.lock` suppression, ignore-set membership, and
  linked-worktree `git_dir`. Cases that pin the defects found in re-review, since
  each was reachable from plausible-looking wording: `common_dir/info/exclude` ⇒
  `Worktree` + stale signal (**not** dropped into the git-dir arm); an ignored
  **file** inside a tracked directory ⇒ ignore (the set is not directories-only);
  `common_dir/config` and `COMMIT_EDITMSG` ⇒ ignore via the explicit git-dir
  fallthrough. Do **not** attempt to assert branch *order* — a pure function cannot
  expose which branches ran, so order is pinned by outcomes above, not by
  introspection. Debounce coalescing
  with an injected clock, asserting the leading+trailing shape specifically: one
  event ⇒ exactly **one** bump (leading only — no trailing bump when nothing
  followed it); two events inside one window ⇒ exactly **two**; a thousand events
  inside one window ⇒ still exactly two. Plus the `.gitignore` rule: N staleness
  marks inside the 30 s interval schedule exactly **one** re-derivation while
  `Worktree` bumps on each of them, and re-derivation never runs on the event
  thread.
  `watch_key` normalization against the real mixed-separator string from
  `opened-workroots.json` and its all-forward-slash twin. `IgnoreSet` parsing of
  a fixed `-z` byte string, **plus an argv assertion that the constructed command
  contains `-unormal` and not `-uno`** — the measured failure in Constraints is
  silent, so it needs a pin rather than a comment. Reconcile decision table:
  `Degraded` does not re-arm on the reconcile cadence, and arming bumps both
  epochs. `plan_watch_set` against a fixture tree — Linux-only production code but
  a pure function, so it unit-tests anywhere: ignored dirs pruned,
  `git_dir`/`common_dir` absent from the returned list while the explicit
  git-internal targets are present, and `TooLarge` returned as soon as the cap is
  crossed rather than after a full walk.
- *Integration:* new `crates/daemon/tests/git_watch.rs` against a real temp
  repo — arm, `fs::write` an untracked file, poll the `worktree` epoch until
  bumped or 5 s deadline, assert `refs` did **not** bump; `git switch -c` ⇒
  `refs` bumped; `git worktree add` ⇒ `refs` bumped; a file under a gitignored
  `target/` ⇒ **no** bump. **Burst containment, asserted against Phase 1's spawn
  counter:** write 1,000 tracked files without serving a request, then assert the
  spawn total is **unchanged** and one subsequent `/git/status` costs the normal
  fixed number — the direct pin that an ordinary event burst never drives git from
  the event path. Then the exception case, which the tracked-file burst does not
  reach: rewrite `.gitignore` 50 times in a row and assert **at most one**
  additional spawn, so the 30 s re-derive limit is pinned rather than described.
  Availability flap: toggle a root unavailable/available 10 times across reconcile
  ticks and assert arm attempts are bounded by the 30 s interval, not one per tick.
  Shared `common_dir` dedup: register a primary root and a linked worktree, then
  assert one `refs` write bumps **both** repos while the target is registered
  **once**. Availability lifecycle: rename the root away ⇒
  reconcile disarms and bumps ⇒ status reports unavailable ⇒ rename back ⇒
  re-arms. Windows: worktree-remove while armed does not fail with a sharing
  violation (existing worktree-remove pins are the reference). All
  deadline-polling, never fixed sleeps. **These run armed on every platform,
  including the Linux/WSL dev host** — every shared behavior above is asserted
  identically on both registration paths, so the split costs no coverage of
  anything but the three `watch(.., Recursive)` calls themselves. Use a local path
  (not `/mnt`) so the allowlist does not degrade the fixture. Linux-only additions,
  `#[cfg(target_os = "linux")]`: `WS_DASHBOARD_GIT_WATCH_MAX_DIRS=1` ⇒ `Degraded`,
  **zero** registrations, and the 2 s TTL still serving correct status; a directory
  created after arming gets registered and a write inside it bumps; and
  `mkdir sub && write sub/f` in one step still bumps (the race is covered by the
  parent's event, per Decisions). On Windows the last two must hold *without* any
  incremental-registration code, so assert them there too — that is the property
  recursive mode is being kept for.
- *Live-only — state as not covered by tests, do not pretend otherwise:*
  sustained spawns/s and CPU% on the Windows dogfood daemon over ≥10 min with the
  browser open, Activity pane both closed and open; daemon RSS and handle-count
  delta on arm across all 9 roots (expected to be small now that Windows registers
  ~3 watches per repo rather than ~200 — a large delta would mean the Linux path
  ran); real descriptor consumption on WSL
  (`find /proc/*/fd -lname anon_inode:inotify | wc -l`) against
  `registeredWatches`; **watcher-task CPU during a clean `cargo build` on Windows,
  which is the one estimate in Decisions that was never measured** — the claim is
  that 10⁴-10⁵ discarded `target/` events cost well under 1% against the build, so
  sample the watcher thread's CPU time across a build and record whether
  `need_rescan()` fired and how often — **and measure a `git gc` and a `git fetch`
  too, not only a `cargo build`**, because the recursive `common_dir` registration
  admits the whole `objects/` fanout and packfile churn, which the Constraints
  sizing does not cover; and end-to-end perceived freshness, which
  per the `ws-web-dashboard` Domain Rules needs a browser-level assertion in
  `frontend/e2e/` for the toolbar chip updating after an external edit.

Estimated diff ~+480/−70 production, ~+240 test, 9 files — the walk, cap, and
incremental registration formerly scoped as Phase 5 are in scope here, but only on
the Linux path.

### Phase 5: Linux `PerDirectory` arming with a counted descriptor budget [absorbed into Phase 4]

**Not a separate phase. Superseded on 2026-07-26** — retained unrenumbered per
ticket conventions so the history of the decision is legible. There is nothing to
implement here; Phase 4 is the whole watcher.

History, because this heading changed meaning three times in one day and the commit
log alone will not make that clear:

1. Originally a separate phase: Linux would walk and register per-directory under a
   `DirBudget` while Windows/macOS used one recursive registration, split off from
   Phase 4 to give the risky half its own revert boundary.
2. Then **dropped**, on the grounds that `git status` is already fast on Linux and
   per-directory state invites handle-accounting bugs — leaving Linux permanently on
   the 2 s polling TTL.
3. Then **inverted**: rather than drop the walk, make it mandatory on *every*
   platform and delete recursive registration, since the walk is what makes the
   descriptor count knowable and the cap enforceable.
4. Then **corrected to the current design**, because step 3 over-generalized. The
   counting invariant is a constraint on counting, and Windows already satisfies it
   with a count of 1; forcing the walk there bought coverage of three trivial lines
   in exchange for ~200 handles per repo, a per-arm walk, and an
   incremental-registration state machine the kernel provides for free. So:
   recursive on Windows/macOS, walk-and-count on Linux — and Linux **does** arm
   under `auto`, which is the durable win from steps 2-3. The phase has no distinct
   content left because its Linux mechanism now sits inside Phase 4.

The measurements behind steps 3-4 are in Constraints (~200 pruned directories per
repo, not the ~2,400 previously assumed), and the reasoning is in Decisions.

**Rollback ladder, each rung independent:** `WS_DASHBOARD_GIT_WATCH=off`
disables Phase 4; `WS_DASHBOARD_GIT_CACHE_TTL_MS=0` disables Phase 3;
`WS_DASHBOARD_GIT_PROBE_TTL_MS=0` disables the landed probe memo; reverting
Phase 2 is a self-contained rewrite of one function.

## Non-Goals

- **Recursive registration on Linux**, including behind an env var. It cannot
  satisfy the counting invariant in Constraints there: `notify` emulates recursion
  with per-directory watches we cannot count, cap, or report, so exhausting the
  host's descriptors would be both possible and undetectable. Recursive
  registration remains the *chosen* strategy on Windows/macOS, where the count is 1
  — see Decisions for why that asymmetry is the cheap answer rather than a
  compromise. The `260726-refactor-ws-dashboard-long-uptime-leak-hardening` concern
  about per-directory bookkeeping is real, and it is confined to the Linux path plus
  answered there by the cap and the all-or-nothing arm rule.
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

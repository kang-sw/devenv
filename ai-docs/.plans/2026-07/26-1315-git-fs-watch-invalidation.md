<!-- Recovered verbatim from the Plan-agent output of 2026-07-26 (session
     78fe486b). Numbering below is the ORIGINAL plan's; the ticket
     260726-perf-dashboard-git-fs-watch-invalidation renumbers phases and
     records that this plan's Phase 2 already shipped as 18037cc3. -->

# Plan: FS-watch-driven git invalidation for the ws-dashboard daemon (`notify`), polling retained as fallback

## 1. Current architecture — how one poll tick actually flows

### 1.1 Frontend timers (three independent schedulers, all 5s or faster)

| Timer | Site | Cadence | Routes hit |
|---|---|---|---|
| Git toolbar | `frontend/src/gitToolbar.ts:234-256` (`startGitRefreshScheduler`, default `intervalMs = 5000` at `:237`), wired at `frontend/src/App.tsx:6718-6733` | 5s + `visibilitychange` + `window focus` | `/git/status` **and** `/git/branches` fired together in one `Promise.all` — `App.tsx:6661-6712` (`refreshGit`) |
| Resources | `App.tsx:857-868`, interval const `frontend/src/resourceRefresh.ts:6` (`resourceAvailabilityPollIntervalMs = 5_000`) | 5s | `/api/dashboard/resources` + `/api/dashboard/servers` |
| Activity feed | `App.tsx:4833-4836`, const `App.tsx:444` (`workRootActivityRefreshIntervalMs = 3_000`) | 3s | `/api/dashboard/work-roots/{id}/activity` |

`refreshGit` (`App.tsx:6661`) has no request-side in-flight guard — only a stale-response discard via `requestSeq` (`App.tsx:6640`, checked at `:6675`). All fetches go through `fetchWithTimeout` (`frontend/src/fetchWithTimeout.ts:22`, `DEFAULT_FETCH_TIMEOUT_MS = 8_000` at `:13`) — client aborts at 8s, **daemon keeps working** (see §1.4).

### 1.2 The discovery fan-out — the 85%

`git_toolbar.rs:328-356` `resolve_git_context` answers a question about **one** work root by running full discovery over **all** of them:

```
resolve_git_context (git_toolbar.rs:332)
  -> resources::live_dashboard_resources          (resources.rs:53-55)
  -> live_dashboard_resources_with_sync           (resources.rs:57-74)
  -> LocalDashboardResourcesProvider::dashboard_resources_with_registry_sync
                                                   (discovery.rs:64-125)
       for each owner candidate root (discovery.rs:69):
         discover_work_root(path)                  (discovery.rs:70 -> :320-358 -> :360-404)
           -> GitDiscovery::discover               (discovery.rs:435-472)
              -> 1 spawn: git rev-parse --show-toplevel --path-format=absolute
                          --git-common-dir --git-dir            (discovery.rs:439-450)
         git_worktree_paths(path)                  (discovery.rs:74 -> :491-508)
              -> 1 spawn: git worktree list --porcelain         (discovery.rs:492-497)
         for each linked worktree path (discovery.rs:82):
           discover_work_root(linked_path)         (discovery.rs:89)
              -> 1 more rev-parse spawn
  -> then side effects: opened.unregister(...) for pruned (resources.rs:69-71)
                        opened.sync_discovered_roots(...)       (resources.rs:72)
```

Cost = **`2N + W` spawns** per call (N owner roots, W linked worktrees). At N=8, W=4 → 20 spawns. Three routes call it per 5s tick — `/git/status` (`git_toolbar.rs:141`), `/git/branches` (`:157`), `/api/dashboard/resources` (`resources.rs:23` → `:26-47`) — so **60 spawns / 5s = 12/s of pure discovery**, which is why ~85% of the measured 9.6/s is discovery. `dashboard_servers` (`servers.rs:220-229`) does *not* run discovery; it is not part of the fan-out.

Note that `live_dashboard_resources_with_sync` is **not pure** — it unregisters pruned roots and registers newly-discovered linked worktrees. Any memoization must respect that (§4, Phase 2).

### 1.3 The actual git work behind each route

`status_for_path` (`git_toolbar.rs:358-392`), ~5 spawns:
`branch --show-current` (`:359`), `rev-parse --short HEAD` if detached (`:361`), `rev-parse --abbrev-ref --symbolic-full-name @{upstream}` (`:365-373`), `rev-list --left-right --count` via `rev_counts` (`:524-531`, spawn at `:526`), plus `changes_for_path` (`:440-504`) = `--no-optional-locks diff-index -M --numstat HEAD --` (`:454-464`) + `--no-optional-locks status --porcelain=v1 --untracked-files=all` (`:477-485`).

`branches_for_path` (`git_toolbar.rs:394-438`), **`3 + B` spawns**: `branch --show-current` (`:395`), `worktree list --porcelain` via `checked_out_branches` (`:554-560`), `for-each-ref` (`:402-409`), then **one `rev-list` per branch with an upstream** (`sync_for_branch` `:518-522` → `rev_counts` `:526`). On a repo with 20 upstream-tracking branches this is the single most expensive route in the daemon, and it is polled every 5 seconds.

Activity route: `work_root_activity` (`work_root_activity.rs:187-221`) → `project_with_recent_limit` (`:108`) → `project_blocking` (`:507`) → `resolve_work_root_state_dir` (`:516-526`) → `git_identity` (`:2357-2377`) = **2 spawns** (`:2358` and `:2359-2362`) every 3s.

**Landmine found while reading (not in the established facts):** the Activity Console SSE `work_root_activity_events` (`work_root_activity.rs:274-298`) is not a push channel at all — its stream body sleeps **200ms** and re-runs `watch_snapshot` (`work_root_activity.rs:378-383`). `watch_snapshot_blocking` (`:546-566`) calls `project_blocking` **and** `activity_item_versions` (`:568`), each of which independently calls `resolve_work_root_state_dir` → `git_identity` → 2 spawns. That is **~4 git spawns every 200 ms = ~20/s** for the selected root, gated on `activityPaneOpenForSelected` (`App.tsx:4564`). The measured 9.6/s was with that pane closed. Opening the Activity Console roughly triples the daemon's git spawn rate. This is a two-line fix (memoize `git_identity` per root — it is derived from paths that essentially never change) and it belongs in the cheap prerequisite phase.

### 1.4 Structural gaps confirmed in code

- Zero timeout/kill: `git_text` (`git_toolbar.rs:566-577`), `run_git` (`:579-587`), `GitDiscovery::discover` (`:439-450`), `git_worktree_paths` (`:492-497`), `git_output` (`work_root_activity.rs:2379-2392`) — all bare `.output()`. Client abort at 8s does not cancel the child.
- Silent failure: `git_text` returns `None` on non-zero exit and discards `stderr` (`:573-576`); `run_git` maps everything to `Err(())` (`:585-586`).
- Four duplicated spawn helpers, no shared seam to instrument.
- No `notify` anywhere: `ws-dashboard/Cargo.toml` workspace deps and `crates/daemon/Cargo.toml` have no watch crate, and `Cargo.lock` (2330 lines) has no `notify`/`inotify`/`kqueue` entries.

### 1.5 Existing push machinery (relevant to the target-architecture decision)

- **Real broadcast:** `DocumentEventHub` (`work_root_files.rs:44-64`, `tokio::sync::broadcast` cap 64) + SSE endpoint `document_events` (`:461-489`) with `KeepAlive::default()`. Published from `write_work_root_file` (`:436-446`). This is genuine in-process write-triggered push. Frontend `EventSource` at `App.tsx:4867`.
- **Fake push:** the Activity SSE above (server-side 200 ms poll loop). Do not copy this shape.
- **Watch channel:** `terminal.rs:452` `output_signal: watch::Sender<u64>` (created `:924`, `:1581`) — a monotonic-counter wakeup signal. This is precisely the right primitive for the git-invalidation epoch and is already an in-repo pattern.
- **SSE forwarding across linked servers exists:** `forward_server_scoped_document_events` (`servers.rs:1046`), used by `server_scoped_document_events` (`:1038-1047`). So a future git SSE is *possible* across `serverRoute`, but needs its own forwarding path; the discrete git routes use `forward_server_scoped_operation` (`servers.rs:1367`).

---

## 2. Target architecture

### 2.1 Recommendation: **watch-driven invalidation + cheap cached endpoints. Keep frontend polling. Do NOT build a git SSE now.**

**What changes:** the FS watcher does not push to the browser. It bumps a per-repo **epoch counter**; route handlers serve from a cache whose validity is `cached_epoch == current_epoch && age < ttl`. The frontend keeps its existing 5s scheduler untouched, but a tick where nothing changed costs **zero git spawns**.

**Justification against the existing machinery:**

1. The one thing an SSE would buy is latency (5s → ~150ms). Nothing in the measured problem is a latency complaint; the problem is 830k spawns/day and 16.8% of a core. A cached endpoint captures ~100% of the CPU win.
2. `DocumentEventHub` is a poor template. Its events are *authoritative and complete* (the daemon itself performed the write, so the payload is exact). FS events are *lossy hints* — buffer overflow, coalescing, and Windows lock-rename noise all mean "an event may be missing or spurious." An SSE built on lossy hints needs backfill/resync semantics; the Activity SSE already has that machinery (`ActivitySnapshotInvalidationReason::{Overflow, WatchReset, Fallback}`, `ActivityUpdateMode::PollFallback`, `work_root_activity.rs:330-370`) and it cost hundreds of lines. A TTL-bounded cache gets self-healing for free: a missed event costs at most one TTL of staleness, no protocol needed.
3. Real push additionally requires: `serverRoute` SSE forwarding for git (`servers.rs`), per-connection subscription lifecycle, `EventSource` reconnect + cursor backfill in `App.tsx`, and a new core event type in `crates/core`. That is Phase-5-sized work whose only benefit is latency.
4. The epoch is the reusable asset. If push is wanted later, Phase 5 is "expose the epoch over SSE" — the hard part (correct, cross-platform, budget-aware invalidation) is already done and independently verified.

**Where things live:**

- `crates/daemon/src/git_exec.rs` (new) — the single git-spawn seam: timeout, kill-on-timeout, stderr logging, spawn counter.
- `crates/daemon/src/work_root_watch.rs` (new) — owns the `notify` watcher(s), per-repo watch registration, epochs, ignore sets, health/degradation. Held in `AppState` (`router.rs:78-98`), constructed in `server.rs:105-122`.
- `crates/daemon/src/git_toolbar.rs` — gains `GitStateCache` (two independently-invalidated parts, §2.3).
- `crates/daemon/src/discovery.rs` — gains `ProbeCache` for `GitDiscovery::discover` + `git_worktree_paths`, and widens `DiscoveredWorkRoot` (`:311-318`) to carry `git_dir`/`common_dir` so the watcher can arm without extra spawns.
- Reconcile hook: **one** call site, `resources::live_dashboard_resources_with_sync` (`resources.rs:57-74`), which already computes the authoritative root set + availability every 5s.

### 2.2 What the watcher watches (and why `.git` cannot be blanket-ignored)

Per **repo** (deduped by `common_dir`, so N linked worktrees of one repo share one arming pass):

| Target | Mode | Why |
|---|---|---|
| `<worktree_dir>/**` minus git-derived ignore set | recursive (Win/macOS) / per-dir (Linux) | untracked + modified detection for `changes_for_path` |
| `<common_dir>` top level, non-recursive | non-recursive everywhere | `HEAD`, `index`, `FETCH_HEAD`, `MERGE_HEAD`, `packed-refs`, `ORIG_HEAD` |
| `<common_dir>/refs/**` | recursive | branch create/delete/move; nested names (`refs/heads/feat/x`) need recursion |
| `<common_dir>/worktrees/**` | recursive | per-linked-worktree `HEAD`/`index`; also detects `git worktree add/remove` → invalidates the cached `worktree list` |
| `<git_dir>` (linked worktrees only), non-recursive | non-recursive | that worktree's own `HEAD`/`index` |

**Never watched:** `<common_dir>/objects/**`, `<common_dir>/lfs/**`, `<common_dir>/modules/**`. **Watched but filtered out of epoch bumps:** `*.lock` inside the git dir (git's create-then-rename lock dance is pure noise, and `index.lock` churn is exactly what `260711` was about).

This satisfies envelope item **#3** directly: `.git` is watched selectively, only `objects/` is excluded.

### 2.3 Two epochs, not one

```rust
struct RepoEpochs { worktree: AtomicU64, refs: AtomicU64 }
```

- `worktree` bumped by worktree-subtree events **and** `<git_dir>/index` events → invalidates `changes_for_path` (`git_toolbar.rs:440-504`) only.
- `refs` bumped by `<common_dir>/{HEAD,packed-refs,FETCH_HEAD,ORIG_HEAD}`, `refs/**`, `worktrees/**` events → invalidates the ref-derived part: `branch --show-current`, `@{upstream}`, `rev_counts`, `for-each-ref`, `checked_out_branches`.

This split matters because `branches_for_path` costs `3 + B` spawns and its inputs change only on branch/fetch operations. Typing in a source file must not re-run `B` `rev-list` calls.

### 2.4 Ignore rules derived from git, not hardcoded (envelope #2)

Hardcoding `target/`, `node_modules/` is unsafe: `--untracked-files=all` is the whole point, and in a repo where `target/` is *not* gitignored those untracked files must be reported.

Instead, at arm time run **one** spawn per repo:

```
git -C <worktree> --no-optional-locks status --porcelain=v1 -uno --ignored=matching -z
```

> **CORRECTION (2026-07-26, verified on git 2.43.0) — do not implement the command
> above.** `-uno` suppresses the `!!` output entirely: this repo yields 0 ignored
> entries with `-uno` and 10 with `-unormal`. Use `-unormal`. The ticket carries the
> corrected form and a unit test pinning the argv.

and collect the `!! path/` entries into a per-repo `IgnoreSet`. That is git's own answer, collapsed to directories. Re-derive when a `.gitignore` / `.git/info/exclude` / `core.excludesFile` event is seen. On failure: empty ignore set → watch everything (correct, just noisier).

Load-bearing consequence for Linux: the measured noisy-dir counts (264/1565, 384/803, 6600/7164, 131/264 = ~7,379 of ~9,796) are exactly this set. Pruning it takes the watch-descriptor requirement from ~9,800 to **~2,400**, comfortably under the default `max_user_watches` of 8192.

### 2.5 Platform strategy (envelope #6)

```rust
enum WatchStrategy { RecursiveSubtree, PerDirectory }   // chosen at runtime, injectable for tests
enum WatchHealth   { Armed, Degraded { reason: &'static str }, Unarmed }
```

- **Windows / macOS** → `RecursiveSubtree`: one `RecursiveMode::Recursive` registration per target, event-path filtering against the `IgnoreSet`. Cheap (one handle / one FSEvents stream).
- **Linux / WSL** → `PerDirectory`: walk the worktree ourselves, skipping the `IgnoreSet`, and register `RecursiveMode::NonRecursive` per surviving directory so ignored subtrees never consume a descriptor. Before arming, read `/proc/sys/fs/inotify/max_user_watches` and enforce a process budget (default `min(limit * 60 / 100, WS_DASHBOARD_GIT_WATCH_MAX_DIRS)`, default cap 6000). A repo that would blow the budget is **not partially armed** — it goes to `WatchHealth::Degraded{"watch budget exceeded"}` and keeps the short-TTL polling path. Runtime `notify::ErrorKind::MaxFilesWatch` → same degrade, logged once per repo.
- New directory created inside a watched tree → in `PerDirectory` mode the create event triggers registering that directory (and re-checking the budget). This is the known inotify race; a directory created and populated before we register it is exactly what the TTL fallback covers.

### 2.6 Overflow / rescan (envelope #1)

`notify` surfaces this as `Event::need_rescan()` (inotify `IN_Q_OVERFLOW`, `ReadDirectoryChangesW` buffer overflow). Handling: bump **both** epochs for every repo owned by that watcher, and set `WatchHealth::Degraded{"rescan required"}` for one TTL window so the short TTL applies. Polling is never deleted — it is the same code path, only its TTL changes:

| State | TTL | Effect at the frontend's 5s cadence |
|---|---|---|
| `Armed`, epoch unchanged | 15 000 ms | 2 of every 3 ticks are free |
| `Armed`, epoch bumped | — (immediate recompute) | fresh within one tick of the change |
| `Degraded` / `Unarmed` | 2 000 ms | every tick recomputes → today's behavior, minus the fan-out |

Even with the watcher completely broken the daemon is strictly better than today (the Phase 1/2 fan-out removal is unconditional). That is the rollback story.

### 2.7 ahead/behind — correcting envelope #5

`rev_counts` (`git_toolbar.rs:524-531`) runs `rev-list --left-right --count <upstream>...<HEAD>`. Both sides are **local refs**: `refs/heads/*` and `refs/remotes/*`. Today's 5s poll does not contact the network either — it only re-reads local refs. Those refs change only when a local `git fetch`/`pull`/`push` writes `refs/remotes/**`, `packed-refs`, or `FETCH_HEAD`, **all of which are in the watch set (§2.2)**. So FS-watch invalidation gives *exact parity* with today's polling for ahead/behind; no slow timer is needed for correctness.

A slow timer would only be needed to add **automatic `git fetch`**, which is a new feature, not a regression-avoidance measure — it spawns network I/O and can trigger credential prompts. Explicitly out of scope; the existing manual `git_fetch` route (`git_toolbar.rs:265-276`) stays the only fetch trigger, and its `run_git` completion should bump the `refs` epoch directly (cheap, one line).

### 2.8 Path normalization (why any cache needs it first)

`opened-workroots.json` stores mixed separators (e.g. `"D:/Workspace/Repos/InspectTGV_AIDriven/.git\\ws-worktree\\jpeg"`). `normalize_registered_root` (`work_root_files.rs:108-111`) only strips the Windows `\\?\` verbatim prefix — it does **not** unify separators or case. `canonical_or_normalized` (`discovery.rs:520-523`) canonicalizes when possible but falls back to `normalize_candidate_path` (`:510-518`), which returns the raw string for absolute paths. So two spellings of the same *missing/moved* root hash to different `WorkRootId`s (`discovery.rs:539-544`).

Introduce a **separate** key type used only by caches and the watcher:

```rust
// discovery.rs
pub(crate) struct WatchKey(String);
pub(crate) fn watch_key(path: &Path) -> WatchKey  // canonicalize -> normalize_display_path
                                                  // -> '\\' => '/' -> lowercase on Windows
```

**Do not** change `local_work_root_id_for_path`/`canonical_or_normalized`. Changing them would churn `WorkRootId` values for degraded roots, and the frontend keys nav selection, workbench panes, and `workNavOrder.ts`'s browser-local persisted order on those ids. Keep id derivation frozen; a separate `WatchKey` costs ~15 lines and zero blast radius. (Unifying the persisted spellings is a worthwhile separate cleanup with its own ticket.)

---

## 3. Cheap prerequisite vs. watcher work — the explicit split

| | Phases | Claim | Watcher needed? |
|---|---|---|---|
| **CHEAP PREREQUISITE** | 0, 1, 2 | removes the `2N+W` fan-out from all three routes and the hidden Activity-SSE 20/s; ~85–90% of spawns | **no** |
| **WATCHER** | 4 | removes the remaining per-tick recompute (the ~15% that is real work) | yes |
| **Bridge** | 3 | installs the cache with epoch stubbed to 0 (TTL-only), so Phase 4 is purely "make the epoch real" | no |

Phases 0–2 are independently shippable and independently valuable. If Phase 4 is never done, the daemon still drops from ~9.6 spawns/s to roughly 1.5/s.

---

## 4. Phased breakdown

### Phase 0 — Instrumentation seam (ships first; every later phase is measured against it)

**Files:** new `crates/daemon/src/git_exec.rs`; edit `git_toolbar.rs`, `discovery.rs`, `work_root_activity.rs`, `git_worktree.rs`, `lib.rs`, `router.rs`.

**New types:**
```rust
pub struct GitOutcome { pub status: ExitStatus, pub stdout: String, pub stderr: String, pub elapsed: Duration }
pub enum GitFailure { Spawn(io::Error), Timeout, Status(i32) }
pub fn capture(root: &Path, args: &[&str], budget: Duration) -> Result<GitOutcome, GitFailure>
pub struct GitSpawnStats { total: AtomicU64, timeouts: AtomicU64, failures: AtomicU64, by_subcommand: Mutex<BTreeMap<&'static str, u64>> }
```

**Changes:**
1. `capture` = `Command::spawn` + bounded wait + `child.kill()` on expiry (default 10s, `WS_DASHBOARD_GIT_TIMEOUT_MS`). Directly closes `260724-idea-dashboard-daemon-side-git-poll-response-timeout` for the poll path.
2. Non-zero exit / timeout → `tracing::warn!(subcommand, code, stderr = %truncate(stderr, 512), elapsed_ms)`. Fixes the silent-failure hole at `git_toolbar.rs:573-576` and `:585-586`.
3. Rewrite `git_text` (`git_toolbar.rs:566-577`) and `run_git` (`:579-587`) as thin wrappers over `capture` — keeps all existing call sites and the four in-file tests (`git_toolbar.rs:639`, `:681`, `:707`, `:759`) compiling unchanged. Same for `GitDiscovery::discover` (`discovery.rs:439-450`), `git_worktree_paths` (`:492-497`), `git_output` (`work_root_activity.rs:2380-2385`).
4. New owner-authed route `GET /api/dashboard/diag/git` in `router.rs` (next to `dashboard_build_info`, `router.rs:108`) returning `{ totalSpawns, timeouts, failures, bySubcommand, uptimeMs }`.

**Verification boundary:** curl `/api/dashboard/diag/git` twice 60s apart on the live Windows dogfood daemon → derive spawns/s. This number is the acceptance gate for Phases 1, 2, 4. Unit test: `capture` kills a child that outlives its budget (spawn `git` with a bogus long-running `--exec-path` trick, or better, inject the binary name so the test can spawn `sleep`/`timeout`).

**Diff:** ~+240 / −70, 7 files.

---

### Phase 1 — Remove the discovery fan-out from the git routes (biggest single win)

**Files:** `git_toolbar.rs`, `discovery.rs`, `work_root_activity.rs`.

**Changes:**
1. Rewrite `resolve_git_context` (`git_toolbar.rs:328-356`) to resolve **one** root:
   - `state.opened_work_roots.get(id)` → `None` ⇒ `GitContextError::Unknown` (404)
   - `root.activation != Online` ⇒ `Offline` (409)
   - `discovery::discover_work_root(&root.path)` (make `pub(crate)`) → `availability != Available` ⇒ `Unavailable` (409); `kind` not `GitPrimaryRoot|GitLinkedWorktree` ⇒ `NonGit` (400)
   - else `GitContext { root_path: root.path }`
   Drops the `use crate::resources::live_dashboard_resources` import at `git_toolbar.rs:13`. **`2N+W` → 1 spawn** for `/git/status`, same for `/git/branches`.
2. Memoize `git_identity` (`work_root_activity.rs:2357-2377`) behind a `OnceCell`-per-root map keyed by `WatchKey`, invalidated only when `.git` is missing. Its inputs (worktree root, common root) are structural and effectively immutable for a registered root. This kills both the 3s activity poll's 2 spawns/tick **and** the Activity-SSE's ~20/s (§1.3). ~35 lines.
3. Add `discovery::watch_key` (§2.8).

**Behavior deltas to accept explicitly (and pin):**
- The git routes no longer trigger the discovered-worktree registry sync. New linked worktrees still appear via the 5s `/api/dashboard/resources` poll (`resources.rs:72`) and immediately after `git_worktree_add_submit`. Acceptable; note in the ticket.
- Workspace-level pruning (`discovery.rs:108-115`) no longer masks a per-root answer, so an *unavailable* git root now returns 409 `workRoot unavailable` instead of 404 `unknown workRoot`. Verify against `routes.rs:7666` `git_toolbar_status_gates_and_reports_counts_without_paths`, which pins exactly four cases (200 available / 400 plain / 409 offline / 404 unknown-id) — all four are preserved by the logic above. Add a fifth case for the moved-root 409.

**Verification boundary:** `/api/dashboard/diag/git` delta drops ~2×(2N+W)/5s. `cargo test -p ws-dashboard-daemon` green, especially `routes.rs:7617`, `:7666`, `:7736`, `:7840`, `:5724`, `:5954`.

**Diff:** ~+130 / −50 production, ~+90 test, 4 files.

---

### Phase 2 — TTL + single-flight memo for the remaining discovery fan-out

**Files:** `discovery.rs`, `resources.rs`.

**New types (in `discovery.rs`):**
```rust
struct ProbeSlot { git: Option<(Instant, Option<GitDiscovery>)>, worktrees: Option<(Instant, Vec<PathBuf>)> }
pub(crate) struct ProbeCache { slots: Arc<Mutex<HashMap<WatchKey, Arc<Mutex<ProbeSlot>>>>> }
```
Two-level locking (map lock released before the per-key lock) gives memoization **and** single-flight, so three concurrent routes missing at the same instant produce one spawn, not three.

**Changes:**
1. Cache **only** the git probes — `GitDiscovery::discover` (`discovery.rs:435-472`) and `git_worktree_paths` (`:491-508`). TTL default 30 000 ms (`WS_DASHBOARD_GIT_PROBE_TTL_MS`), plus epoch invalidation once Phase 4 lands.
2. Leave `fs::metadata` / `fs::read_dir` in `discover_work_root` (`:320-358`) and `discover_existing_dir` (`:361-372`) **uncached** — they are cheap and they are what detects `moved`/`missing`/`inaccessible` (`:331-357`). Availability stays instant; envelope **#4** preserved.
3. On `availability != Available`, evict that key's `ProbeSlot` so a reappearing root re-probes immediately.
4. Widen `DiscoveredWorkRoot` (`discovery.rs:311-318`) with `git_dir: Option<PathBuf>`, `common_dir: Option<PathBuf>`. `GitDiscovery::discover` already computes both (`:459-460`) and currently throws `git_dir` away after the `kind` comparison at `:461-465`. Phase 4 needs these to arm without extra spawns.
5. `ProbeCache` lives in `AppState`; `LocalDashboardResourcesProvider` (`discovery.rs:40-45`) gains a `probes: ProbeCache` field, threaded from `live_dashboard_resources_with_sync` (`resources.rs:60`).

Important: the registry side effects (`resources.rs:69-72`) keep running **unconditionally** on every call. Only the probes are cached. Cache hits therefore cannot skip pruning or discovered-root registration.

**Verification boundary:** unit test in `discovery.rs` — a `ProbeCache` with a counting fake probe returns the same value for two calls inside the TTL and re-probes after; a barrier-synchronized two-thread test proves single-flight (probe called once). Integration: `routes.rs:709` `dashboard_resources_discovers_linked_git_worktrees_from_opened_primary` must still pass, and a new test must show that removing a linked worktree is reflected within TTL.

**Diff:** ~+210 / −55 production, ~+130 test, 3 files.

---

### Phase 3 — Result cache for `/git/status` and `/git/branches` (epoch stubbed)

**Files:** `git_toolbar.rs`, `router.rs`, `server.rs`, `servers.rs` (no logic change — local delegation at `servers.rs:1365` already routes through `git_status`).

**New types (`git_toolbar.rs`):**
```rust
struct RefState { branch_name: Option<String>, detached_oid: Option<String>, upstream: Option<String>,
                  sync: GitSyncSummary, branch_list: GitBranchList, checked_out: BTreeSet<String> }
struct GitCacheSlot { worktree: Option<(u64 /*epoch*/, Instant, GitChangeSummary)>,
                      refs:     Option<(u64,            Instant, RefState)> }
pub struct GitStateCache { slots: Arc<Mutex<HashMap<WatchKey, Arc<Mutex<GitCacheSlot>>>>> }
```

**Changes:**
1. `status_for_path` (`:358-392`) and `branches_for_path` (`:394-438`) become `(&GitStateCache, &EpochSource, &Path)` and read/fill the two slot parts. `changes_for_path` (`:440-504`) stays a pure function (its four in-file tests at `:639`–`:796` keep working verbatim).
2. `EpochSource` is a trait with a `StaticZero` impl in this phase and the watcher impl in Phase 4 — so Phase 3 is TTL-only and independently testable.
3. Mutating routes bump epochs directly, so a user action is never TTL-delayed: `git_switch_branch` (`:166`), `git_create_branch` (`:206`), `mutate_no_body` for fetch/push/pull (`:304-326`) → bump `refs`; `git_pull_ff_only` also bumps `worktree`.
4. TTL from `WS_DASHBOARD_GIT_CACHE_TTL_MS` (default 2000 with `StaticZero`; §2.6 table once armed).

**Verification boundary:** integration test — hit `/git/status` twice inside the TTL with a `--no-optional-locks status` counter from Phase 0's stats; assert the second call adds zero spawns. Then `POST /git/switch-branch` and assert the next `/git/status` reflects the new branch immediately (epoch bump beats TTL). `routes.rs:7736` (`branches_switch_and_create_revalidate_state`) is the existing pin for this; it must pass unmodified.

**Diff:** ~+270 / −95 production, ~+160 test, 4 files.

---

### Phase 4 — The watcher (the large phase; be honest about it)

**Files:** new `crates/daemon/src/work_root_watch.rs`; edit `Cargo.toml` (×2), `lib.rs`, `router.rs`, `server.rs`, `resources.rs`, `git_toolbar.rs`, `discovery.rs`.

**Dependency:** `notify = { version = "8", default-features = false, features = ["macos_fsevent"] }` — `default-features = false` avoids the `crossbeam-channel` pull-in; use the callback constructor and forward into a `tokio::sync::mpsc`. Do **not** take `notify-debouncer-full`: it maintains a file-ID cache sized to the watched tree, and we do not need rename correlation — we only need "something under X changed."

**New types:**
```rust
pub struct WorkRootWatchRegistry { inner: Arc<Mutex<WatchInner>>, epochs: Arc<RwLock<HashMap<WatchKey, Arc<RepoEpochs>>>> }
struct WatchInner { watcher: Option<RecommendedWatcher>, repos: HashMap<WatchKey, ArmedRepo>, dir_budget: DirBudget }
struct ArmedRepo { targets: WatchTargets, ignore: IgnoreSet, registered_dirs: BTreeSet<PathBuf>, health: WatchHealth, armed_at: Instant }
struct WatchTargets { worktree: PathBuf, git_dir: PathBuf, common_dir: PathBuf }
pub struct RepoEpochs { worktree: AtomicU64, refs: AtomicU64 }
enum EpochKind { Worktree, Refs, Both }
enum WatchStrategy { RecursiveSubtree, PerDirectory }  // SUPERSEDED: no strategy enum; see ticket
pub enum WatchHealth { Armed, Degraded { reason: &'static str }, Unarmed }
struct IgnoreSet { dirs: BTreeSet<PathBuf> }   // CORRECTED: `--ignored=matching -unormal -z` (`-uno` returns nothing)
struct DirBudget { limit: usize, used: usize }
pub enum WatchMode { Off, Auto, Force }        // WS_DASHBOARD_GIT_WATCH
```

**Implementation steps, in order:**

1. **`classify(path) -> Option<EpochKind>`** — pure function, the correctness core. Given an event path and an `ArmedRepo`, decide `Worktree` / `Refs` / ignore. Rules: under `common_dir/objects|lfs|modules` ⇒ ignore; `*.lock` under any git dir ⇒ ignore; `common_dir/{HEAD,packed-refs,FETCH_HEAD,ORIG_HEAD}` or `refs/**` or `worktrees/**` ⇒ `Refs`; `git_dir/index` ⇒ `Worktree`; under an `IgnoreSet` dir ⇒ ignore; else under `worktree` ⇒ `Worktree`; `.gitignore`/`.git/info/exclude` ⇒ `Worktree` + mark ignore-set stale. **~100% unit-testable with plain `PathBuf`s, no filesystem.** Write this first and test it exhaustively.
2. **`IgnoreSet::derive(worktree)`** — one `git_exec::capture` call, parse `!! ` entries from `-z` output.
3. **Arming.** `arm(targets, strategy, budget)`; `PerDirectory` walks with the ignore set applied and counts against `DirBudget`; over-budget ⇒ `Degraded`, register nothing.
4. **Event pipeline.** `notify` callback (its own thread) → `mpsc::unbounded_send` → one long-lived tokio task: coalesce for 100 ms (trailing, 500 ms max), map each path via `classify`, bump the union of `EpochKind`s per repo once. `event.need_rescan()` ⇒ bump `Both` for all repos + `Degraded{"rescan required"}` for one TTL window.
5. **Reconcile (envelope #4).** `registry.reconcile(&[(WatchKey, Option<WatchTargets>, WorkRootAvailability)])` called from `live_dashboard_resources_with_sync` (`resources.rs:57-74`) after `sync_discovered_roots` (`:72`), using the widened `DiscoveredWorkRoot` fields from Phase 2 — **no extra git spawns**. Semantics: present+`Available`+not armed ⇒ arm; present+not-`Available` ⇒ disarm + bump `Both` (so the next poll recomputes and reports the degraded state); absent from the set ⇒ disarm + drop epochs. Covers register/unregister (`work_root_files.rs:148-164`), `moved`/`missing`/`inaccessible` transitions (`discovery.rs:331-357`), `remove_workspace` (`root_picker.rs`), and `git_worktree_remove_submit`, through a single code path instead of six hooks.
6. **Wire the real `EpochSource`** into `GitStateCache`; select TTL from `WatchHealth` per §2.6.
7. **Config:** `WS_DASHBOARD_GIT_WATCH=off|auto|force` (default `auto`; `off` ⇒ every repo `Unarmed`, 2s TTL — the rollback switch), `WS_DASHBOARD_GIT_WATCH_MAX_DIRS`, `WS_DASHBOARD_GIT_CACHE_TTL_MS`, `WS_DASHBOARD_GIT_WATCH_DEBOUNCE_MS`. Surface health in `/api/dashboard/diag/git` (`{ repos: [{ health, registeredDirs, worktreeEpoch, refsEpoch, lastEventMs }] }`).

**Verification boundary (three tiers):**
- Unit (`work_root_watch.rs`): `classify` table test (~25 cases) covering `objects/` exclusion, `HEAD`/`refs/` inclusion, `index.lock` suppression, ignore-set membership, linked-worktree `git_dir`. Budget arithmetic. Debounce coalescing with an injected clock.
- Integration (`crates/daemon/tests/`): new `git_watch.rs` with a real temp repo — arm, `fs::write` an untracked file, poll `worktree` epoch until bumped or 5s timeout; assert `refs` epoch did **not** bump. Then `git switch -c`, assert `refs` bumped. Then `git worktree add`, assert `refs` bumped. Then create a file under a gitignored `target/`, assert **no** bump. `#[cfg(unix)]` variant with `WS_DASHBOARD_GIT_WATCH_MAX_DIRS=1` asserting `Degraded` + short TTL rather than partial arming. These need generous timeouts and must be polling-with-deadline, never fixed sleeps.
- Live-only (cannot be tested): sustained spawns/s and CPU% on the Windows dogfood daemon via `/api/dashboard/diag/git` over ≥10 minutes with the browser open, both with the Activity pane closed and open; inotify descriptor count on WSL (`find /proc/*/fd -lname anon_inode:inotify | wc -l`); and behavior across a real `moved` → restored work-root transition.

**Diff:** ~+720 / −70 production, ~+280 test, 9 files. **This is genuinely large** — `work_root_watch.rs` alone is ~550 lines, and roughly half of that is the Linux `PerDirectory` path plus budget/degrade handling that the Windows path does not need.

---

### Phase 5 — SSE push (explicitly deferred, not scheduled)

Only if latency becomes a complaint. Shape: broadcast the epoch (mirroring `DocumentEventHub`, `work_root_files.rs:44-64`), new SSE `/git/events` next to `document_events` (`:461-489`), forwarded via a new `forward_server_scoped_git_events` modeled on `servers.rs:1046`, frontend `EventSource` replacing the `setInterval` in `startGitRefreshScheduler` (`gitToolbar.ts:247`) with the timer kept as fallback. Estimated ~+300 daemon / ~+180 frontend / ~+120 test. Do not start this until Phase 4 has run in dogfood for a week.

---

## 5. Risks and rollback

| Risk | Mitigation |
|---|---|
| **Missed FS event ⇒ stale git status** (the headline risk) | TTL ceiling is absolute: 15s armed, 2s degraded. Never "wait forever for an event." Polling code path is *never deleted* — Phase 3's cache lookup falls through to the same `changes_for_path`/`branches_for_path` calls. |
| Linux inotify exhaustion breaking *other* apps (editors, watchers) | Hard process budget at 60% of `max_user_watches`; over-budget repos are `Degraded`, not partially armed. Git-derived ignore pruning takes ~9,800 dirs → ~2,400. `WS_DASHBOARD_GIT_WATCH=off` kills all watching. |
| Ignore rules hiding a genuinely untracked file | Ignore set comes from `git status --ignored=matching`, i.e. git's own answer — by construction it cannot hide anything `--untracked-files=all` would report. Re-derived on `.gitignore` change. Integration test pins both directions. |
| Phase 1's per-root resolve changes an HTTP status code | Four existing cases pinned by `routes.rs:7666`; add the moved-root case. Deltas enumerated in §Phase 1. |
| Phase 2 caching hides a removed/added linked worktree | Only git probes are cached; availability probes are not. Worst case 30s (TTL) or one `.git/worktrees/` event (armed). |
| Watcher thread panic / `notify` init failure | `watcher: Option<...>`; init failure ⇒ all repos `Unarmed`, warn once, 2s TTL. The daemon must never fail to boot because a watcher could not start. |
| Watcher holding directory handles blocks `git worktree remove` / dir rename on Windows | `ReadDirectoryChangesW` opens with `FILE_SHARE_DELETE` (notify does this), so deletes are permitted. Still: disarm the repo *before* `git_worktree_remove_submit` runs `git worktree remove`, and re-arm via the next reconcile. Explicit acceptance test needed (`routes.rs:7199`, `:7248`, `:7298` are the existing worktree-remove pins). |
| Memory growth from `registered_dirs` on huge monorepos | Budget cap doubles as a memory cap. `/api/dashboard/diag/git` exposes `registeredDirs` per repo for dogfood monitoring. |

**Rollback ladder (each rung independent):**
1. `WS_DASHBOARD_GIT_WATCH=off` — disables Phase 4 only; Phases 0–3 keep the ~85% win.
2. `WS_DASHBOARD_GIT_CACHE_TTL_MS=0` — disables Phase 3's cache; Phases 0–2 keep the fan-out win.
3. `WS_DASHBOARD_GIT_PROBE_TTL_MS=0` — disables Phase 2's memo; Phase 1 keeps the git-route fan-out win.
4. Revert Phase 1 — a self-contained rewrite of one function (`git_toolbar.rs:328-356`).

---

## 6. Test strategy, by what is actually provable

**Pure unit tests (no filesystem, no git) — the correctness core:**
- `work_root_watch::classify` — the exhaustive path-classification table (§Phase 4 step 1). This is where a bug would silently break invalidation, and it is fully testable without any I/O. Highest test-value-per-line in the whole plan.
- Debounce coalescing with an injected clock.
- `DirBudget` arithmetic and the over-budget degrade decision.
- `discovery::watch_key` normalization: the real mixed-separator string from `opened-workroots.json` and its all-forward-slash twin must map to one key; Windows case-insensitivity; `\\?\` prefix.
- `ProbeCache` / `GitStateCache` TTL + epoch validity with a counting fake probe; two-thread barrier test for single-flight.
- `IgnoreSet` parsing of a fixed `git status --ignored -z` byte string.

**Integration with a real temp git repo** (pattern already established — `git_toolbar.rs:622-637` `init_fixture_repo`, `tests/routes.rs:8944-8967` `run_git`/`init_git_repo`/`skip_without_git`, `:12276` `git_toolbar_get_json`, `app_state()`):
- New `crates/daemon/tests/git_watch.rs`: arm → mutate → epoch assertions (per Phase 4). Deadline-polling, never fixed sleeps.
- Cache-hit spawn accounting through `/api/dashboard/diag/git` (Phase 0 is the prerequisite that makes this assertable at all).
- Availability lifecycle: rename the work-root directory away → reconcile disarms + epoch bumps → status reports unavailable → rename back → re-arms.
- Windows-only: worktree-remove-while-armed does not fail with a sharing violation.
- `#[cfg(unix)]`: `WS_DASHBOARD_GIT_WATCH_MAX_DIRS=1` ⇒ `Degraded`, and the route still returns correct data on the short TTL.

**Only provable by live measurement (state this in the ticket; do not pretend a test covers it):**
- Sustained spawns/s and CPU% on the Windows dogfood daemon (`/api/dashboard/diag/git` delta over ≥10 min, browser open, Activity pane both closed and open).
- Real inotify descriptor consumption on WSL against the actual 4 repos (~9,800 dirs).
- Buffer-overflow / rescan handling under a real `cargo build` storm — synthesizable but not reliably reproducible in CI.
- End-to-end perceived freshness. Per the `ws-web-dashboard` Domain Rules (`ai-docs/mental-model/ws-web-dashboard/index.md:22`), any visible-UI change needs browser-level verification; the git toolbar chip updating after an external edit is a Playwright assertion in `frontend/e2e/`.

**Docs to update on landing:** the poll-path git rule at `ai-docs/mental-model/ws-web-dashboard/index.md:28` and `:79` currently says "poll-path git invocation (≤5s cadence)" — after this work the cadence is event-driven with a TTL ceiling, and the lock-free-form requirement still holds (a watch-triggered recompute runs the same commands). Close/reference `260711-idea-dashboard-git-status-polling-index-lock-contention` (this is its stated long-term plan), `260724-idea-dashboard-daemon-side-git-poll-response-timeout` (Phase 0 closes it), `260725-perf-dashboard-daemon-workroot-fanout-concurrency` (Phases 1–2 largely obviate its "parallelize the serial fan-out" premise by deleting most of the fan-out instead), and `260714-research-dashboard-workroot-watch-push-channel` (§2.1 answers its "Required First Step" prior-art question: the Activity watch-stream is a server-side 200ms poll, so it cannot be generalized — and should not be imitated).

---

## 5-line summary

1. **5 phases** — 0 instrumentation/timeout seam, 1 per-root git-route resolve, 2 discovery probe memo, 3 result cache (epoch stubbed), 4 the `notify` watcher; Phase 5 (SSE push) explicitly deferred and unscheduled.
2. **Total scheduled diff ≈ +1,570 / −340 production lines plus ≈ +660 test lines across 15 distinct files**; Phase 4 alone is ~+720/−70 and the new `work_root_watch.rs` is ~550 lines.
3. **Cheap prerequisite (Phases 0–2, ~+580/−175) is fully independent of the watcher** and captures ~85–90% of the spawn reduction on its own — including a previously-unrecorded ~20 spawns/s from the Activity Console SSE's 200 ms server-side poll loop (`work_root_activity.rs:378` → `watch_snapshot_blocking:546` → 4× `git_identity` spawns).
4. **Riskiest part: Phase 4's Linux `PerDirectory` arming** — per-directory inotify registration under a hard `max_user_watches` budget, with correctness depending entirely on the git-derived ignore set pruning ~9,800 dirs to ~2,400, plus the new-directory registration race; it is the only place where a bug both breaks invalidation silently and can degrade unrelated applications on the host.
5. **Recommendation is a cached endpoint, not push**: the watcher bumps two per-repo epochs (`worktree`/`refs`), the frontend's existing 5s scheduler is untouched, and polling is retained as a hard TTL ceiling (15s armed / 2s degraded) so a missed or overflowed event self-heals without any resync protocol; `WS_DASHBOARD_GIT_WATCH=off` is the single-flag rollback.

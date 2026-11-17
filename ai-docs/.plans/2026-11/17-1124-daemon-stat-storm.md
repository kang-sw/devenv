# Daemon CPU: stop the per-read file-stat storm and wake re-render fan-out

## Context

Owner, 2026-11-17 11:06-11:24: the shared dashboard daemon was unusable.
- Messages arrived ~10 s late, the login page took ~30 s, and MCP whiteboard
  calls timed out at 75 s.
- Even when healthy, the daemon keeps 1-2 cores fully loaded. Owner priority:
  top.

pprof of the current daemon (develop 892d59c23; PID 6148; Windows; ~230
hosted conversations; 7 boards / 466 json files in this repo). Artifacts are
in `D:/Temp/daemon-profile-1107/`: `cpu.pb.gz`, `goroutine1.txt`,
`streams1.json`, `streams2.json`. The symbol binary is
`C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\bin\ws-mcp-dashboard-5d316e265f4fc3c5.exe`.

- CPU: 53.9 s of samples in 30 s (1.8 cores).
  - 83% is Windows file syscalls (`runtime.cgocall` via `syscall.SyscallN`).
  - `os.Stat`: 29%. `os.Open`: 18%.
- Stat callers:
  - `wswhiteboard.folderCachedRead.unchanged`
    (`internal/wswhiteboard/folders_cache.go:56`): 6.3 s. It runs `os.Stat`
    on every json/jsonl file of a board on every cache-hit read, via
    `FolderCache.OpenRevision` (:98).
  - `wsdashboard.statStamp` via `StampWatched`
    (`internal/wsdashboard/stream_cache.go:269`, `:287`): 3.6 s.
  - `wslive.(*ObservedCache).Load`: 3.4 s.
  - `FileCache.load`: 1.2 s.
- Cumulative paths that land in that work:
  - websocket `stream` → `pageCache.get`: 18.6 s;
  - `handleSessionEvents`: 9.8 s;
  - `handleRailEvents` → `railView`: 8.8 s;
  - `wsagent.(*conv).loop` → `mcp.loadRecordWithBoard` →
    `openWhiteboardCached`: 8.6 s;
  - MCP `callTool` → `DashboardSessionSource.lookupRecord` / `openWhiteboard`:
    8.3 s;
  - `wsdashboard.(*Index).drain` / `readLane` / `apply`: 8.5 s.
- `/debug/pprof/streams` counters over ~4 min:
  - `wake_agent` 378→1231, `wake_session` 565→926, `signals` 34→818;
  - `snapshot_cache_hits` 631→13824, misses 692→1386;
  - `rail_renders` 11→220.
  - That is a wake-driven re-render storm, with many sessions woken per
    signal.
- No lock stall: goroutines are mostly idle conv loops, and the mutex profile
  is small.
- A fresh daemon also took >75 s to answer `/health` after `daemon-start
  --replace` while all clients reconnected. The same load at warm-up is the
  likely cause.

## Outcome

1. A cache-hit whiteboard read no longer stats every board file.
   - Validate a cached folder board with O(1) file checks per read. The
     folder store already commits through root `committed.json`
     (`schema_version: 2`, `generation`) plus an optional `pending.json`, so
     the generation stamp, or a single root stat plus a pending check, should
     be enough.
   - Confirm against the folder-store commit protocol that every writer,
     including older daemons and an external edit path if one is supported,
     changes what you check.
   - If some write path does not touch the root, say so and pick the
     cheapest correct check.
2. `StampWatched`/`statStamp`, `ObservedCache.Load` and `FileCache.load`
   must not re-stat unchanged inputs on every render.
   - Rate-limit stat-based validation per path, for example at most once per
     ~1 s per path, and share the result across concurrent renders.
   - Or use an existing change signal where one exists.
   - Pick the simplest option that keeps freshness within ~1 s.
3. Wake fan-out:
   - one signal must not trigger repeated full re-renders of the rail or
     session views per subscriber;
   - coalesce renders per view, at most one in flight plus one pending, and
     dedupe wakes for the same session within a short window.
   - Find where `signals` turn into `wake_agent` / `wake_session` /
     `rail_renders` and cut the multiplication.
4. Measure:
   - Before/after numbers from a reproducible Go benchmark or a test-level
     counter: the stat count per cached board read, and renders per signal
     with N subscribers.
   - The target is O(1) stats per cached read and at most 1 render per view
     per signal burst.
5. Behaviour unchanged:
   - the board still reflects writes within about a second;
   - SSE/WS clients still get updates;
   - MCP whiteboard calls see their own writes immediately (read-your-writes
     within the daemon).

## Constraints

- Before editing `agents-plugin-tool/`, read:
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/release-compatibility.md`.
- No on-disk format change; this is in-memory caching only (T0). State the
  tier with source lines if you touch anything persisted.
- Other workers are active:
  - wb:close retirement in `wswhiteboard` (branch
    impl/develop/wb-close-no-result);
  - visible-only topic rendering in dashboard JS/CSS
    (impl/develop/board-visible-only);
  - release-gate fixes in desktop-electron (impl/develop/gate-sweep-fixes-1117).
  Keep this change to the caching and wake paths.
- NO local `go test`, Go test executables, ws/ws-mcp executables or the
  launcher on this PC (owner AV rule).
  - Do not start the daemon or Electron.
  - Go tests and benchmarks run on remote w5090 only:
    `D:/sys_lock.sh remote w5090 run stat-storm <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wswhiteboard ./internal/wsdashboard ./internal/wslive ./internal/mcp ./internal/wsagent'`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation (Node).
  - Analysing the profile files with `go tool pprof` is allowed: it reads
    files and runs no tests. If Go is not local, do it on w5090.
- Temp files only under `D:/Temp/daemon-stat-storm/`, deleted afterwards.
- Commits carry no Co-Authored-By trailer. Do not push.

## Verification

- Go tests pass on w5090, with the accepted reds only.
- New tests cover:
  - a cached read does O(1) stats (counted through an injectable stat
    function or counter);
  - a write via the store, and via an external committed.json change if that
    is supported, is seen on the next read;
  - one signal with N subscribers gives at most one render per view.
- R1 review; a fixes-only R2 for substantive findings.
- Report:
  - commits and files;
  - before/after counts;
  - the tier;
  - any write path that bypasses the root stamp.

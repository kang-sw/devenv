# Read-cache bundle A: per-tool-call fixed cost, Ensure write-skip, launcher hash cache

## Context

On the owner's Windows PC, AhnLab V3 scans every file touch and every process
creation, so each syscall costs 10-100x. The release is gated on daemon
file-system load (clone note `release-gate-daemon-file-io-20261117`). The owner
approved (whiteboard (1297) = a, 2026-11-17 14:44) a zero-state-risk bundle:
read caches and skip-if-unchanged writes only. This plan is half A; a parallel
worker does half B (ticket inventory, whiteboard re-render skip, session-switch
reads) on `impl/develop/read-cache-b`. Stay out of B's areas.

Evidence: researcher B's report `D:/Temp/file-touch-map-mcp/report.md` (ranked
table, per-call overhead, top 5 candidates) and the research ticket
`ai-docs/tickets/todo/261117-research-ws-daemon-file-touch-map.md` (Outcome
Ledger). Re-locate every line; the code moved since.

## Outcome

1. **Per-tool-call fixed cost.** Every keyed MCP tool call does about 25-30
   uncached syscalls: session `key.json` read about 3 times, `EvalSymlinks`
   about 3 times, uncached config reads, endpoint and rsrc resolution. Make a
   keyed call resolve these at most once per call, and cache across calls
   where the source is immutable or can be revalidated cheaply.
   - A cache must never serve stale data a caller relies on. Prefer: memoize
     within one call; across calls, cache immutable facts (resolved root
     paths, symlink resolutions of fixed paths, rsrc bundle files) and
     revalidate mutable files by one stat (mtime+size) instead of a reread.
     Do not drop the stat when a cross-process writer can change the file
     (config edited by hand, `key.json` written by another process).
2. **`wsstate.Ensure`.** It spawns 2 git processes and rewrites `project.json`
   and `worktree.json` on every call, because `last_seen_at` changes every
   second. Skip the rewrite when nothing but `last_seen_at` would change and
   the stored value is recent (for example under a few minutes old). Cache
   the git facts per root, revalidated cheaply, for example by HEAD file
   mtime. Check every reader of `last_seen_at` first, so a coarser value is
   harmless. Name them in the commit.
3. **Launcher start.** The launcher hashes the 49 MB executable 4-5 times per
   start and does about 770 stats in dev mode. Hash once per start. Across
   starts, cache the hash keyed by path+size+mtime if a safe place already
   exists. Keep every compatibility gate's semantics.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md` and `ai-docs/manuals/ws-mcp.md`
  before editing.
- No on-disk shape change, beyond an optional new cache file that older
  binaries ignore (T0). No write-timing change except skipping identical or
  `last_seen_at`-only rewrites. State the tier per item with source lines in
  the commit.
- Simplest complete change; no config knobs. Add a per-site syscall counter
  only where the existing r2a counter seam (`file_io_*` at
  `/debug/pprof/streams`) does not already cover the site.
- NO local `go test`, Go test executables, ws/ws-mcp executables, the
  launcher, the daemon or Electron on this PC (owner antivirus rule).
  - Go builds and tests run on w5090 only:
    `D:/sys_lock.sh remote w5090 run read-cache-a 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/guiding-dragonish-consumer' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && go test ./...'`.
    Narrow the package list if the full run is too slow, but cover every
    package you touched plus `./internal/mcp`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- Temp files only under `D:/Temp/read-cache-a/`.
- Commit on `impl/develop/read-cache-a` with your own model's Co-Authored-By
  trailer. Do not push.

## Verification

- Tests show that:
  - a keyed tool call reads `key.json` and config at most once, using an
    instrumented fs seam or the counter seam;
  - a changed config or `key.json` is picked up on the next call;
  - Ensure does not rewrite when only `last_seen_at` would change within the
    window, and does rewrite on a real change;
  - the launcher hashes once.
- One passing w5090 run.
- Report:
  - the commit;
  - the files;
  - the syscall count per keyed call before and after, estimated from code
    or measured in a test;
  - the test results;
  - the tier per item.

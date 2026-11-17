# Perf: cut the hottest read-side stat/read sites seen during session switching

## Context

The owner's PC runs AhnLab V3, so every file touch costs 10-100x. Session
switches take 6-17 s. The daemon was measured on develop `34e03ae24` with the
read-cache A/B and fs-find fixes, pid 39808, at 16:38-16:44. Artifacts are in
`D:/Temp/daemon-swap-1117d/{after,switch,switch2}/`: CPU profiles, traces,
file-I/O counters and goroutine dumps. `go tool pprof` and `go tool trace` may
be run on those files.

File-I/O sites in a 120 s switching window, 43k ops in total (about 723/s):

| Site | Ops | Rate |
|---|---|---|
| `wslive/observed_cache.go:242` stat | 6,960 | ~58/s |
| `mcp/whiteboard_nudges.go:122` stat | 6,675 | |
| `mcp/whiteboard_nudges.go:125` stat | 4,303 | |
| `mcp/session_layout.go:225` read_file | 5,697 | ~47/s |
| `wswhiteboard/folders_memo.go:60` stat | 594 per 60 s | quiet period |

- `whiteboard_nudges.go:122` and `:125` together run at about 90/s.
- `session_layout.go:225` is a read even after read-cache-a introduced
  `wsstate.ReadFileCached`. Check why this path is uncached.

Live-writer writes (`wslive` `AppendObserved` fsync, open and temp per event)
are out of scope here. They are a write-strategy change that is being designed
separately and needs the owner's approval.

`registry.go:240` (heartbeat ownership fence reads) is out of scope too. It is
a separate design item.

## Outcome

For each read-side site above, find who calls it and how often per event,
request or tick, then remove the redundant work.

The approved techniques (owner (1297)=a) are:
- in-memory caches validated by a cheap signal;
- skip-if-unchanged;
- not re-statting what was just statted in the same pass;
- hoisting per-item stats out of loops;
- deriving from data already in memory.

Specifics:
1. **`observed_cache.go:242`.** Why does each lookup stat? If the daemon
   process is itself the writer of that file (the live writer), it knows when
   the file changed, so use an in-process generation counter. Fall back to
   stat only for files written by other processes, or at a bounded rate.
2. **`whiteboard_nudges.go:122/125`.** These run nudge checks at about 90
   stats per second. Find the loop, whether per board, per topic or per tick.
   Make it event- or generation-driven, or bound it to the daemon's existing
   30 s reconciliation. Nudge timing semantics must stay the same: overdue
   30-minute debounce, hourly list, readiness on board update plus the 30 s
   reconcile.
3. **`session_layout.go:225`.** Route this read through
   `wsstate.ReadFileCached` (stat-validated) or remove the repeat read.
   Read-modify-write paths under locks stay uncached.
4. **`folders_memo.go:60`.** If the memo's stat runs per request for many
   topics, consider a board-level generation check: one stat of the board's
   `committed.json` or `meta.json` instead of one stat per topic. Do this only
   if it is safe given the store's commit protocol. Otherwise leave it and
   explain why.

## Constraints

- **T0 only.** No on-disk shape change, no write-strategy change. If a fix
  needs one, stop and report it instead.
- **Correctness.** Hand edits and other processes' writes must still be seen.
  Every cache is validated, or bounded so it is never stale for longer than
  the existing reconcile interval.
- **No local runs on this PC.** No `go test`, Go test executables, ws/ws-mcp
  exes, launcher or daemon on this PC (antivirus rule).
- **Go runs on w5090 only:**
  `D:/sys_lock.sh remote w5090 run hot-stat 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && GOOS=windows go vet ./internal/... && go test -count=1 ./internal/mcp ./internal/wslive ./internal/wswhiteboard ./internal/wsstate ./internal/wsdashboard'`
  - Redirect the output to a file under `D:/Temp/hot-stat/`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Tests.** Add a test per site showing the reduced count, using the existing
  `file_io_site` counters or a seam.
- **Scope.** Simplest complete change; R1 review, then a fixes-only R2.
- **Commit.** Commit on `impl/develop/hot-stat-sites` with your own model's
  Co-Authored-By trailer and state the tier with source lines. Do not push.
- **Temp files** go only under `D:/Temp/hot-stat/`.
- **Urgency.** Keep momentum: the owner cannot use session switching.

## Report

- per site: the cause, the fix, and the ops/s before → after, estimated or
  counted in a test;
- the commit and files;
- the w5090 result;
- the tier;
- anything you left alone and why.

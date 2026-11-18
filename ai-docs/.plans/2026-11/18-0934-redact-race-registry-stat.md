# Perf/correctness: tracker Redact race and repeated registry stats

## Context

The owner chose (1344)=a on 2026-11-18 at 09:33. That bundle is two
follow-ups from the 2026-11-17 performance work, which was found by profiling
the shared daemon on the owner's Windows PC. The PC runs AhnLab V3, so every
file touch costs 10-100x. Design rule from the owner: minimize file touches;
live state stays in memory. Read caches and skip-if-unchanged writes are
approved under (1297). Any other write-strategy change needs the owner's
approval, so stop and report instead.

1. **Tracker `v.Redact()` race (pre-existing correctness bug).**
   - In the tracker host code, a `StoreView` returned by `ViewWithin` (or its
     cache) is shared between callers.
   - `v.Redact()` mutates that shared view in place. Concurrent callers can
     race, or see another caller's redaction.
   - Find it with:
     `grep -rn "Redact()" agents-plugin-tool/internal`
     `grep -rn "func .*ViewWithin" agents-plugin-tool/internal`
2. **Repeated registry stats.** `registry.go` around line 240 (find the
   registry with a hot stat path; likely under `agents-plugin-tool/internal/`)
   was measured at about 7.75 `os.Stat` calls per second on the idle daemon.
   An earlier note called the full fix an "L2/L3 design"; do the simple part
   here.

## Outcome

1. **Redact no longer mutates shared state.**
   - Redaction works on a copy owned by the caller, or the cached view is
     stored already redacted. Choose the one that is simplest and correct.
   - Copy only what Redact mutates (a shallow copy plus the mutated
     slices/maps), not a deep copy of everything, if that is clearly safe.
   - Add a test that fails under `-race` (or shows the shared mutation)
     before the fix and passes after.
2. **Fewer registry stats.**
   - Identify who calls the line-240 path and how often. Cut the repeated
     stats with an in-memory cache:
     - our own writes update the cache directly;
     - an external change is picked up at a bounded, coarser interval, or by
       one stat per bounded window rather than per call.
   - This is a read cache, approved under (1297). Do not change what is
     written or when.
   - Report the estimated stats/s before → after from the code.
   - If no safe simple cut exists (for example, correctness needs the
     per-call stat because another process writes the file), stop at
     analysis. Report the callers, the frequency and a proposed design
     instead of implementing.

## Constraints

- **Coordination.** A Mac Pi worker is implementing
  `261117-feat-ws-tracker-view-summary-delta` Phase 1 (server-side
  incremental tracker view fold, summary hash diff, cursor) on
  `impl/develop/ember-bunny-cacti`. It will touch the tracker view and
  encoding code.
  - Keep your tracker change minimal and local to the Redact/ViewWithin
    sharing, to keep merge conflicts small.
  - Do not refactor the view or its encoding.
  - Another worker, on `impl/develop/cross-minor-replace`, touches
    `internal/wsdashboard/ensure.go`, `cmd/ws-mcp/proxy.go` and
    desktop-electron. Do not touch those.
- **Read first:** `ai-docs/manuals/release-compatibility.md`. The expected
  tier is T0, with no wire or on-disk change. State the tier in the commit
  and cite source lines.
- **No local runs on this PC** (antivirus rule): no `go test`, Go test exes,
  gofmt or other Go tools, ws/ws-mcp exes, launcher or daemon. Go runs only on
  w5090:
  `D:/sys_lock.sh remote w5090 run redact-registry 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && gofmt -l ./internal && go vet ./... && GOOS=windows go vet ./internal/... && go test -race -count=1 <the touched packages>'`
  - Redirect the output to a file under `D:/Temp/redact-registry/`.
  - Run it with run_in_background and wait for it to finish. Do not end your
    turn while it is pending.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
  - The w5090 queue has 2 slots shared with another worker. Be patient.
- **Temp files** go only under `D:/Temp/redact-registry/`.
- **Process:** Simplest complete change. R1 review, then a fixes-only R2 only
  for substantive findings.
- **Commit:** Commit on `impl/develop/redact-race-registry-stat` with your own
  model's Co-Authored-By trailer. Do not push.

## Report

- the Redact fix (file:line), and the test that proves it;
- the registry stat callers, and stats/s before → after;
- the w5090 result;
- the commit;
- anything left alone.

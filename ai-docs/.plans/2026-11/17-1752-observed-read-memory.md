# Perf: serve hosted sessions' observed evidence from memory (C3), and restore evidence-dir trust

## Context

The owner's PC runs AhnLab V3, so every file touch costs 10-100x. The owner
rule: keep live state in memory, and never reread unchanged data.

Read the design pass at `D:/Temp/live-writer-design/design.md` first,
section `### C3`. The owner approved C3 at (1304)=b on 2026-11-17 at 17:36.
C3 is reader-side only and T0. C2 (the append log) is NOT approved and is
not part of this change.

Already merged on develop:
- **C1** (`5266d117b`): writes only fold-changing evidence.
- **hot-stat** (`e2c4b88bf`): `observed_cache.go` takes regular-entry
  metadata from the directory listing (`observedEntryInfo`).

The design's measured cost:

| Path | Cost |
|---|---|
| Dashboard cold load, hosted session with N records | 1 + N stats + N opens (6,960 stats / 6,910 opens in 120 s across cold loads) |

## Outcome

1. **Hosted sessions: zero evidence I/O on the read path.**
   - When the snapshot comes from a live slot (`c.live`),
     `SnapshotCache.snapshotResult` (`snapshot_cache.go:201-212`, near there
     on develop) skips `ObservedCache.Load`.
   - The live writer's state already holds the full fold
     (`wslive/state.go`, around lines 1018, 1049 and 1063-1070 in the
     design's citation; re-locate them on develop). Use that fold.
   - Verify that the result is identical: the same Current and History as
     the cache path would give for a writer that seeded from the directory.
     A test must prove this.
   - Any other reader of hosted evidence that has the live state at hand
     takes the same route. Name each one you change.
2. **Restore `observedRead.trusted()`.** The hot-stat worker found that it
   can never return true since `fcf912cd6`, because
   `wsstate.SameFileMetadata` rejects directories. So every validation after
   the 1 s interval lists the evidence directory: one directory open per
   loaded session per second.
   - Fix it with a directory-aware comparison for `dirInfo`, without
     changing `SameFileMetadata`'s contract for files.
   - The existing "trusted" assertions cannot tell trust apart from a
     listing. Make a test that fails on today's code and passes after the
     fix, using a counter seam such as `readDir` or the `file_io_site`
     counters.
   - Keep today's bound for noticing another process's change.
3. **File-only sessions** (no live slot) keep today's cache path. The
   design's log-offset reading and name-hash depend on C2. Do not do them.

## Constraints

- **Tier T0.** Reader-side only. No change to file format, names or writes.
  Cite source lines in the commit.
- **Correctness.** A hosted session whose writer has not yet seeded, or a
  slot that is closing, must fall back to the cache path rather than show an
  empty fold. Another process writing evidence for a hosted session (for
  example a takeover) must still be noticed within today's bound. If that is
  not possible from memory, keep the cache path for that case and report it.
- **Tests.** Add Go tests for:
  - a hosted snapshot does zero evidence I/O and folds the same as the cache
    path;
  - the fallback when the slot is not live;
  - trust within the interval does no directory listing;
  - a changed directory is still noticed.
- **Coordination: do not touch these.**
  - `wslive/observed.go` write predicate (C1, just merged; read it only);
  - whiteboard, tracker, usage and image code (other workers);
  - dashboard JS/CSS.
- **No local runs on this PC.** No `go test`, Go test exes, ws/ws-mcp exes,
  launcher or daemon (antivirus rule).
- **Go runs only on w5090:**
  `D:/sys_lock.sh remote w5090 run observed-read 40 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && GOOS=windows go vet ./internal/... && go test -count=1 ./internal/wslive ./internal/wsagent ./internal/mcp ./internal/wsdashboard'`
  - Redirect the output to a file under `D:/Temp/observed-read/`.
  - The remote queue may be long. Run the command with run_in_background
    and wait for it to complete. Do not end your turn while it is pending.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Process.** Simplest complete change. R1 review, then a fixes-only R2.
- **Commit.** Commit on `impl/develop/observed-read-memory` with your own
  model's Co-Authored-By trailer, and state the tier. Do not push.
- **Temp files** go only under `D:/Temp/observed-read/`.

## Report

- each reader changed (file:line), and the fold-equivalence argument;
- the `trusted()` fix;
- evidence I/O per hosted cold load and per second per session, before →
  after;
- the tests;
- the commit and files;
- the w5090 result;
- the tier;
- anything left alone.

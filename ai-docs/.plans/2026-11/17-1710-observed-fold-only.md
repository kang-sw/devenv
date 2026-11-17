# Perf: write only fold-changing observed-evidence records (break the prune/replay rewrite loop)

## Context

The owner's PC runs AhnLab V3, so every file touch costs 10-100x. A read-only
design pass is saved at `D:/Temp/live-writer-design/design.md`, with counts in
`evidence_counts.txt` next to it. Read it first. In summary:

**Per-record cost today.** `wslive.AppendObserved` (`observed.go:257-311`)
costs 10 syscalls per record, including one fsync. These are measured
numbers: 12.7 ms per record, and 10.55 s of fsync per 30 s of trace.

**Root cause: a prune/replay loop, confirmed on disk.**
- Prune deletes repeated records.
- On daemon restart, conversation state is seeded from the pruned set
  (`record.go:368`).
- The holder's ring then replays the same responses
  (`runner_events.go:16` → `conv.apply` → `record.go:407-411 applyQuiet` →
  `writeObserved` → `AppendObserved`). They look new and are re-created with
  fsync.
- The next prune deletes them again.
- `seedSpawnRows` (`spawn_live.go:162-168`) does the same for child evidence.

**Evidence from the capture.**
- 285 of 307 evidence files with an mtime in the capture window held `at`
  values 1.3-3.9 h older than the file.
- While a conversation replays, it refuses to start a turn
  (`runner_conv.go:798`). So after every daemon swap, the first message waits
  for these rewrites.

**What restoration needs.** Only the per-agent fold (Current plus History,
`observed.go:193-216`). `observed_prune.go:12-20` proves that the first
record, change points and latest give the same fold.

## Outcome (design option C1 only)

Write an observed record to disk only when it changes the persisted fold:
- the agent's first record;
- a model change point;
- or a record newer than the current latest, per the fold's own rule.

Records that do not change the fold, including ring replays of already-folded
responses and spawn re-seeds, update in-memory state only and touch no file.

- The restored fold after a crash or restart stays identical to today's,
  because prune already reduces the on-disk set to exactly what the fold
  needs.
- No file format change, and no change to the read side (`ReadObserved`, the
  `ObservedCache`).
- Keep the parent's path for Pi-child evidence working: child evidence that
  changes the fold must still be written.
- Do NOT do C2 (append log) or C3 (reader changes). They are separate and
  await the owner's approval.

## Constraints

- **Tier T0.** Same files, same names, same content for every record that is
  written; fewer records written. Cite source lines for which version writes
  and reads what. If you find C1 needs a format or read-side change, stop and
  report.
- **Owner approval basis.** (1297) approves skip-if-unchanged writes. C1 is
  that, at fold level. Any further write-strategy change needs a new approval.
  Stop and report rather than expand.
- **Tests.**
  - Replaying an already-folded ring touches zero evidence files: use the
    `file_io_site` counters or a seam.
  - A model change still writes.
  - A restart after prune followed by a replay yields the same fold and no new
    files.
  - Child evidence path still works.
- **Coordination.** Another worker (branch `impl/develop/hot-stat-sites`) is
  editing `wslive/observed_cache.go` read-side stats. Do not edit
  `observed_cache.go`.
- **No local runs on this PC.** No `go test`, Go test exes, ws/ws-mcp exes,
  launcher or daemon (antivirus rule).
- **Go runs only on w5090:**
  `D:/sys_lock.sh remote w5090 run observed-fold 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/porthole-diabetic-banjo' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && GOOS=windows go vet ./internal/... && go test -count=1 ./internal/wslive ./internal/wsagent ./internal/mcp ./internal/wsdashboard'`
  - Redirect the output to a file under `D:/Temp/observed-fold/`.
  - Wait for the run to finish before ending your turn.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Process.** Simplest complete change, then an R1 review and a fixes-only
  R2.
- **Commit.** Commit on `impl/develop/observed-fold-only` with your own
  model's Co-Authored-By trailer and state the tier. Do not push.
- **Temp files** go only under `D:/Temp/observed-fold/`.
- **Urgency.** Every daemon swap currently stalls the owner's first message.

## Report

- the exact write predicate and where it lives (file:line);
- syscalls per replayed and per live record, before → after;
- the tests;
- the commit and files;
- the w5090 result;
- the tier with source lines;
- anything left alone.

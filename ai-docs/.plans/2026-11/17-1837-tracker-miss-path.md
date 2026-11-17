# Perf: the tracker view cache miss must not spawn git or reload an unchanged ledger

## Context

The owner's PC runs AhnLab V3, so every file touch or process spawn costs
10-100x. The tracker view refetch fix merged at `41a1fa839`
(`wsdashboard/tracker_answers.go`, `cmd/ws-mcp/tracker_host.go`). It caches
the view by data revision and answers repeats with 304.

The owner measured it on 2026-11-17 at 18:37:
- a cache hit takes 7 ms;
- a cache miss (`/tracker/stores/<store>/view` after any store change) still
  takes 500-1000 ms.

That worker's Forward named the miss-path cost: the view build calls
`ledger.Reload` and `countCommits` (git subprocesses) inside
`wstrackerhost.view`, even when the store's own files did not change.

## Outcome

1. **A miss whose ledger and git state did not change spawns no git and
   reloads nothing.**
   - Memoize the ledger load and the commit counts, validated by a cheap
     stamp: the store files' stat identity, and for git something like the
     HEAD ref file's stat or the packed-refs/ref mtime. Find the cheapest
     correct stamp in the code.
   - Re-run git only when its stamp changed.
2. **Measure and name the remaining miss cost.**
   - After (1), find what else the miss path does per request (file reads,
     JSON encode of the whole view, other subprocesses).
   - Fix whatever is a read cache or skip-if-unchanged (approved under
     (1297)).
   - Report the rest with numbers. Do not start the summary/delta redesign:
     that is a separate ticket (owner (1307)=a).
3. Keep the 304/ETag behaviour from `41a1fa839` unchanged.

## Constraints

- **Tier.** No on-disk or response shape change; expected T0. Cite source
  lines.
- **Correctness.**
  - A commit, a ledger write by another process, or a store change must
    still show on the next view request.
  - State the staleness bound if a stamp can miss a change.
- **Tests.** Add Go tests for:
  - an unchanged miss runs no git (use a seam or counter);
  - a changed HEAD or ledger re-runs or reloads.
- **Coordination: do not touch these.**
  - `wswhiteboard`, `wslive`, `mcp/whiteboard_*` (other workers);
  - image code, usage code;
  - dashboard JS/CSS, unless strictly required.
- **No local runs on this PC.** No `go test`, Go test exes, ws/ws-mcp exes,
  launcher or daemon (antivirus rule).
- **Go runs only on w5090:**
  `D:/sys_lock.sh remote w5090 run tracker-miss 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/guiding-dragonish-consumer' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && GOOS=windows go vet ./internal/... ./cmd/... && go test -count=1 ./cmd/ws-mcp ./internal/wsdashboard <the tracker host package>'`
  - Adjust the package list to what you touch, and keep `go vet ./...`.
  - Redirect the output to a file under `D:/Temp/tracker-miss/`.
  - The queue is long. Run the command with run_in_background and wait for
    it to finish. Do not end your turn while it is pending.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Process.** Simplest complete change. R1 review, then a fixes-only R2.
- **Commit.** Commit on `impl/develop/tracker-miss-path` with your own
  model's Co-Authored-By trailer, and state the tier. Do not push.
- **Temp files** go only under `D:/Temp/tracker-miss/`.

## Report

- what the miss did before, and what it does now (subprocesses and file
  touches per miss);
- the stamps and the staleness bound;
- the remaining miss cost, with numbers;
- the tests;
- the commit and files;
- the w5090 result;
- the tier;
- anything left alone.

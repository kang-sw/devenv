# Perf: stop refetching the whole tracker store view on every dashboard action

## Context

The owner's DevTools on the session dashboard (2026-11-17 17:05):
- After one steer send, the frontend fetched `/tracker/stores` (387 B), then
  `/tracker/stores/swdev/view` (291 kB, 669 ms).
- Then it fetched the same pair again right away (view 291 kB, 402 ms).
- An earlier trace showed the same view at 291 kB / 532 ms during a session
  switch.

The owner (17:06): "this fetches all of the store data every time — what is
this?" The full store view is downloaded, and rebuilt server-side, on every
action, even when the store did not change.

The owner's PC runs AhnLab V3, so every file touch costs 10-100x. The owner
rule: minimise file touches; never re-read or re-send unchanged data.

## Outcome

1. **Find every frontend trigger** that fetches `/tracker/stores` and
   `/tracker/stores/<x>/view` (dashboard assets `dashboard-*.js`; the
   initiator was `dashboard-e9f5cc7….js:27`, the bundled fetch helper).
   - Explain why one action fetches it twice.
   - Remove the redundant trigger, or coalesce triggers fired within one
     action into one fetch.
2. **Make an unchanged store cost nearly nothing.**
   - The server answers a conditional request with 304 and no body when the
     store has not changed. Use an ETag or revision derived from data the
     daemon already has in memory, such as the tracker store's revision or
     generation counter. Do not hash 291 kB per request if a cheaper signal
     exists.
   - The frontend sends the validator and keeps its last body.
   - Server-side, an unchanged store does no file reads to answer. Use an
     in-memory view cache keyed by that revision, if the view build reads
     files.
3. **Check whether the frontend needs the full 291 kB view** at the moment it
   fetches it, for example at session switch and on steer. If a cheaper
   existing endpoint or a lazy fetch on opening the tracker UI would do, use
   it. Keep this within the existing API shape: do not add a new delta
   protocol in this change.

## Constraints

- **Tier.** T0 only. No on-disk shape change and no change to existing
  response bodies for clients that do not send validators. ETag/304 is
  additive. If something needs more, stop and report.
- **Correctness.** A real tracker change (a new event or update) must still
  show up as promptly as it does today. Changes made by other processes must
  be picked up: validate any cache by a cheap signal, or bound it by the
  daemon's existing reconcile.
- **Don't touch other workers' files.**
  - CV CSS `dashboard-board.css:93-96`.
  - `dashboard-session-view.js` / `sessionBoardAnchors` (switch-click worker).
  - Composer bubble code (composer-dup worker).
  - Refresh stamps / `foreign_schedule.go` (just merged; read it, don't
    rework it).
- **Tests.** Add Go tests:
  - 304 on an unchanged store;
  - 200 with a new body after a change;
  - zero file reads on an unchanged repeat, if there is a counter seam such as
    `file_io_site` counters.
- **No local runs on this PC.** No `go test`, Go test exes, ws/ws-mcp exes,
  launcher or daemon (antivirus rule).
- **Go runs only on w5090:**
  `D:/sys_lock.sh remote w5090 run tracker-view 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/guiding-dragonish-consumer' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && go test -count=1 ./internal/wsdashboard ./internal/mcp ./internal/wstracker'`
  - Adjust the package list to what you touch, and keep `go vet ./...`.
  - Redirect the output to a file under `D:/Temp/tracker-view/`, and wait for
    the run to finish before ending your turn.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Process.** Simplest complete change; R1 review, then a fixes-only R2.
- **Commit.** Commit on `impl/develop/tracker-view-refetch` with your own
  model's Co-Authored-By trailer, and state the tier with source lines. Do not
  push.
- **Temp files** go only under `D:/Temp/tracker-view/`.
- **Urgency.** The owner finds every action slow; keep momentum.

## Report

- Triggers found, and why the view was fetched twice.
- The fix: frontend and server.
- Bytes and ms per action, before → after, estimated or tested.
- The commit and files.
- The w5090 result.
- Anything left alone.

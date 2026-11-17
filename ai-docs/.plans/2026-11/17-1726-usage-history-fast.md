# Perf: usage?history must not block on the Codex probe or re-parse 633 kB per request

## Context

The owner's PC runs AhnLab V3, so every file touch costs 10-100x. The owner
measured `usage?history=1` in DevTools on 2026-11-17: 633 kB in 2.48 s. A
read-only diagnosis (`D:/Temp/daemon-swap-1117f/`) found three causes on
develop:

1. **The probe blocks the request.**
   - The handler calls `d.planUsage.query` synchronously
     (`plan_usage.go:257` → `plan_usage_probe.go:21-45`).
   - That makes an HTTPS call to the Codex usage API (10 s timeout) whenever
     the last query is more than 5 min old. This is always the case on the
     first request after a daemon restart.
   - `queryMu` serializes other viewers behind it.
   - Measured: 0.06-0.09 s without a probe; 0.68 s and 1.31 s with
     refresh=1.
2. **Every request re-reads the history file.**
   - `history()` (`plan_usage_history.go:186-200`) reads and fully parses the
     633 kB file on every request.
   - That is two `json.Unmarshal` calls per sample across 5172 samples, then
     a sort.
3. **Every request can also rewrite the file.**
   - Each request calls `pushClaude` for every claude session
     (`plan_usage.go:249-254`).
   - When a sample is new, `appendHistoryLocked` (:127-183) re-reads,
     re-parses and rewrites 633 kB.
   - Five-hour samples arrive about every 3.6 s (median).

**Why 633 kB.** The file holds:
- 5000 claude five_hour samples, at the `planUsageHistoryCap` limit;
- 133 codex samples;
- 39 claude seven_day samples.

They cover 32 h. The frontend `trend()` (`assets/usage.js:26-60`,
`visible()`) draws only samples whose `resetsAt` equals the current window's.
Right now 142 of 5172 samples are drawn: about 19 kB of 695 kB.

## Outcome

1. **Answer from cache and probe in the background.**
   - Answer from the cached entries immediately, and run the stale probe in
     the background, one at a time.
   - Block only when there is no value at all: no cached entry, and nothing
     persisted to show.
   - Viewers never queue behind `queryMu` while a cached value exists.
2. **Keep the parsed history in memory.**
   - Memoize the parsed history, validated by the file's stat (size and
     mtime), so an unchanged file costs one stat.
   - `appendHistoryLocked` works from the in-memory copy instead of
     re-reading and re-parsing the file. A changed stat, such as another
     process writing, triggers a reload.
3. **Send only what the chart draws.**
   - Per harness and kind, project only current-window samples plus the one
     preceding sample that the adjacency check in `usage.js` uses.
   - Keep the response schema; only the element count drops.
   - Verify against `usage.js` that the drawn chart is unchanged.
   - Memoize the projected JSON by the history revision.
4. **Skip unchanged samples (approved under (1297)).**
   - If a new sample's values equal the previous sample of the same
     harness/kind/window, do not write it, but only if the drawn chart stays
     unchanged. Check whether the chart extends to the latest sample time.
   - If skipping changes what is drawn, do not skip; report it instead.
   - Do NOT switch the file to append-only, throttle writes, or change the
     cap. Those are write-strategy changes that need the owner's approval.
     Report the remaining write rate.

## Constraints

- **Tier.** No on-disk shape change, and the response shape is unchanged
  (fewer elements). State the tier with source lines. Expected T0. Check
  whether any other consumer of `usage?history` needs the full set, such as
  MCP tools, the Electron shell or tests. If one does, keep it full for that
  consumer and report.
- **Tests.** Add Go tests for:
  - a stale cache answers immediately and starts one background probe;
  - with no value, the request blocks;
  - an unchanged history file means one stat and no read;
  - a write after a changed stat reloads;
  - the projection keeps current-window samples plus the preceding sample;
  - an equal sample is skipped, if implemented.
- **Coordination: do not touch these.**
  - dashboard CSS, `dashboard-session-view.js`, composer code;
  - tracker view code, image code;
  - `wswhiteboard`, `wslive`.
  - Touch `usage.js` only if needed. Prefer server-side projection.
- **No local runs on this PC.** No `go test`, Go test exes, ws/ws-mcp exes,
  launcher or daemon (antivirus rule). Node JS tests locally are OK.
- **Go runs only on w5090:**
  `D:/sys_lock.sh remote w5090 run usage-hist 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/degrease-unadvised-nutcase' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && GOOS=windows go vet ./internal/... && go test -count=1 ./internal/wsdashboard ./internal/mcp'`
  - Adjust the package list to what you touch, and keep `go vet ./...`.
  - Redirect the output to a file under `D:/Temp/usage-hist/`.
  - Wait for the run to finish before ending your turn.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Process.** Simplest complete change. R1 review, then a fixes-only R2.
- **Commit.** Commit on `impl/develop/usage-history-fast` with your own
  model's Co-Authored-By trailer, and state the tier. Do not push.
- **Temp files** go only under `D:/Temp/usage-hist/`.

## Report

- the probe change;
- the memo and its validation;
- the projection rule and its chart-equivalence check;
- the sample skip (done or reported);
- bytes and ms per request, before → after;
- the remaining write rate;
- the tests;
- the commit and files;
- the w5090 result;
- the tier;
- anything left alone.

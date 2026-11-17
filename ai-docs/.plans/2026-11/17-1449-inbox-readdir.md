# Inbox directory listing hot path: stop relisting unchanged inboxes

## Context

The shared dashboard daemon on the owner's Windows PC (AhnLab V3 scans every
file touch) was profiled at 14:47, after the conversation cleanup left 38 open
conversations. The artifacts are in `D:/Temp/hot-read-1117/`: cpu30.pprof,
goroutine.txt, and streamsA/B.json taken 60.4 s apart.

- The top per-site file-I/O rate is `wsinbox/inbox.go:1237` read_dir at
  23.5/s.
- In the CPU profile, `wsstate.ReadDir` takes 7.5%. Of that,
  `wsinbox.entryNames` takes 4.4%, reached via `wsinbox.Names` and `readAll`.
- With only 38 open conversations, 23.5 listings per second means some path
  relists inboxes that have not changed.

Owner approval: whiteboard (1297) = a, 2026-11-17 14:44. It covers read
reductions with zero state risk: read caches only, no write-strategy change.

## Outcome

1. **Find the callers.** Identify the callers that drive `wsinbox` directory
   listings.
   - Use the profile: `go tool pprof -peek 'wsinbox\.(entryNames|Names|readAll)' D:/Temp/hot-read-1117/cpu30.pprof`.
   - Confirm in source.
   - For each caller, establish what it is for and what triggers it: a
     heartbeat, render, stream tick, held count, drain, nudge, or a
     permission or header read.
2. **Stop relisting unchanged inboxes.** Make each caller skip the listing
   when the inbox has not changed.
   - Prefer the daemon's own change knowledge. For current-minor clients,
     inbox writes happen inside the daemon process. Examples are an in-memory
     per-inbox revision bumped by the daemon's own writes, or the runner's
     existing inbox signal.
   - Where a writer in another process is possible, check the directory's
     mtime before a full listing.
   - Where the listing's result is already held in memory, remove the
     listing.
3. **Keep the inbox semantics exactly.** That covers held, drained and
   answer-pending states, ordering, the durable write before acknowledging,
   and crash recovery.
   - An external writer must still be picked up. A standalone
     `ws-mcp mailbox wait` or an older same-minor client can write an inbox.
   - Where that applies, revalidate with a stat. Do not trust memory alone.

## Constraints

- **Manuals.** Read these before editing:
  - `ai-docs/manuals/shipped-surface-boundary.md`
  - `ai-docs/manuals/release-compatibility.md`
  - `ai-docs/manuals/ws-mcp.md`
- **Tier.** No change to on-disk shape or to write timing. The expected tier
  is T0. State it in the commit with source lines.
- **Parallel branches.** Avoid these other branches' areas where possible, so
  the merges stay clean:
  - `impl/develop/read-cache-b`: session page and stream rendering, the
    header runner-state cache, whiteboard and tickets.
  - `impl/develop/read-cache-a`: key and config caches, and Ensure.
  - `impl/develop/conv-auto-close`: the wsagent close and idle paths.

  If the hot caller is the hosted-lead header re-fetch of inbox or permission
  state on every header update, that item belongs to read-cache-b. Report it,
  and fix only the other callers.
- **Scope.** The simplest complete change. No config knobs.
- **Local execution.** On this PC, do not run local `go test`, Go test
  executables, ws or ws-mcp executables, the launcher, the daemon or Electron.
  This is the owner's antivirus rule.
  - `go tool pprof` on the existing profile files is allowed.
  - Go builds and tests run on w5090 only:
    `D:/sys_lock.sh remote w5090 run inbox-readdir 25 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/porthole-diabetic-banjo' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./internal/wsinbox ./internal/wsagent ./internal/wsdashboard ./internal/mcp && go test ./internal/wsinbox ./internal/wsagent ./internal/wsdashboard ./internal/mcp'`.
    Add any other package you touch.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Temp files** only under `D:/Temp/inbox-readdir/`.
- **Commit** on `impl/develop/inbox-readdir`, ending with your own model's
  Co-Authored-By trailer. Do not push.

## Verification

- **New tests** show that:
  - an unchanged inbox is not relisted when the hot path's trigger fires;
  - a daemon write is seen at once;
  - an external write is seen on the next check.
- **Existing tests.** The inbox tests pass unchanged.
- **Remote run.** One passing w5090 run.
- **Report:**
  - the caller chains, with file:line;
  - listings per second before and after, estimated from trigger rates;
  - the commit;
  - the files;
  - the test results;
  - the tier.

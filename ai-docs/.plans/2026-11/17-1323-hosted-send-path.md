# Hosted-session send path: no conversation-loop wait on the request, no file I/O fence per write

## Context

Owner, 2026-11-17 13:09-13:15: in daemon-hosted sessions (Pi and hosted
Claude leads, wsagent workers), a message sent from the dashboard takes a long
time to appear even in the "not yet sent" state, and longer to reach the agent.
The owner's Windows PC amplifies every file syscall 10-100x (AhnLab V3: stat
0.4-0.7 ms, ReadDir 1.5-3 ms, atomic replace ~24 ms). Release gate (owner
12:54): no release until interactive paths stop waiting on file I/O.

A read-only trace of develop (28f523038; re-locate lines) found:

- **PageInput on the request path.** `POST /s/<id>/message` →
  `wsdashboard/inbox_routes.go` `handleInboxWrite` (~:225). After the inbox
  write commits, a hosted session calls `PageInput` (~:264-270 →
  `wsagent/visibility.go:61-76` → `runner_conv.go:193-202` `c.do`). It blocks
  the HTTP handler with no timeout until the single-threaded conversation loop
  picks up `c.reqs` (`runner_conv.go:337-344`). That is 30-300 ms while the
  agent streams and seconds while the loop is in `startProcess` or a 2 s
  replace retry. The post-write wakes (`:271` → `inboxChanged`, which signals
  the runner) run only after it returns, so a slow PageInput delays delivery
  too. If PageInput fails (the conversation ended), the route answers 500 after
  the inbox write already committed. The composer keeps the text, and a resend
  duplicates the message.
- **File I/O fence per live write.** Every process event goes `apply` →
  `liveWriter.apply` (`record.go:388-404`) → `write` (~:445), under the owner
  fence. For a hosted lead the fence is `lockSummaryWriter`
  (`runner_summary.go:13-30`):
  - `lockRow` (`runner_conv.go:590-605`) takes the `LockRegistration` flock and
    reads the registration in `rowForeign`;
  - it then reads it again with `ReadRegistration`.

  Each snapshot flush (`record.go:491-525`) takes that fence twice
  (`publishSnapshot` at ~:530-541 and `withOwner`) and does `writeFileAtomic`
  (~24 ms plus a 2 s retry tail in `replace_file_windows.go:20-33`), all on the
  conversation loop.

A parallel worker (`impl/develop/daemon-io-r2a`) owns these. Do not touch them:
- the M8 answer-replay lock in `inbox_routes.go` `scheduleAnswerReplay`;
- M1, in-memory held counts / `Runner.List` / `seedSpawnRows`;
- M2, the `wslive` observed evidence cache;
- nudge discovery, whiteboard cache cap, syscall counter seam.

Another worker does the dashboard composer's instant bubble (JS).

## Outcome

1. **F2: PageInput off the request path (commit this FIRST, alone; report its
   hash at once).**
   - Before writing anything for a hosted session, check whether the
     conversation is closed or ended, from in-memory runner state under its
     mutex. If it is, refuse without any inbox write; keep the route's current
     error status and message shape.
   - For a conversation that is not hidden, do not call `c.do` at all.
   - For a hidden one, unhide asynchronously. That closure triggers
     `inboxChanged` after unhiding, so delivery still happens.
   - The route returns as soon as the inbox write commits, then fires the
     post-write wakes immediately.
   - Keep these rules: a message is delivered exactly once; it is still
     delivered to a hidden conversation; an ended conversation refuses with no
     inbox entry left behind.
2. **F4b: cache the registration-row fence in memory.**
   - Do not take the flock and read the registration twice on every live write.
   - Keep the verified ownership in memory. Re-check it on the existing
     heartbeat and `checkRow` paths, and at real ownership transitions
     (terminal-login handoff, stop, takeover), under the lock as today.
   - A foreign owner must still stop our writes within one heartbeat.
3. **F4c: snapshot flush off the conversation loop.**
   - Give each conversation a background writer for `snapshot.json`. Newest
     write wins; intermediate states may be skipped. The RAM slot is already
     published first, so readers see the new state immediately.
   - Flush on shutdown or stop, so a clean stop leaves the final snapshot on
     disk.
   - The on-disk format is unchanged.
   - Leave the ring append alone in this change. Report whether moving it too is
     worth a follow-up.

## Constraints

- Read before editing:
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/release-compatibility.md`.
- T0 only: in-memory and ordering changes, with the same on-disk shapes. If
  anything persisted or cross-version changes, stop and report it with the
  tier and source lines.
- Simplest complete change. No new config knobs or frameworks.
- NO local `go test`, Go test executables, ws/ws-mcp executables, or the
  launcher on this PC (owner AV rule). Do not start the daemon or Electron. Go
  tests run on w5090 only:
  `D:/sys_lock.sh remote w5090 run hosted-send <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/guiding-dragonish-consumer' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsagent ./internal/wsdashboard ./internal/wsinbox ./internal/mcp'`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- Temp files only under `D:/Temp/hosted-send-path/`, deleted afterwards.
- Commits carry no Co-Authored-By trailer. Do not push.
- Reviews: R1, then a fixes-only R2 for substantive findings; minor findings go
  to Forward.

## Verification

- **F2:** a test where the conversation loop is blocked (for example, holding
  the loop busy) shows the message POST returns without waiting for it, and the
  message is still delivered once the loop frees. An ended conversation
  refuses with no inbox entry. A hidden conversation is unhidden and delivered.
- **F4b:** a counting test shows N live writes do not take the registration
  flock or read the registration N times. A foreign owner detected on the
  heartbeat stops writes.
- **F4c:** a slow (stubbed) snapshot write does not block the loop; the final
  state is on disk after stop.
- One w5090 run of the package list above.
- Report:
  - commits, F2 first, with its hash as soon as it lands;
  - files;
  - the tier;
  - anything skipped and why.

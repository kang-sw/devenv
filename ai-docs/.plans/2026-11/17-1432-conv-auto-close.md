# Auto-close finished Pi child conversations (stop the conversation leak)

## Context

The shared dashboard daemon had 228 open conversations, 191 of them finished
Pi children. Each open record is restored at daemon start with a permanent
`conv.loop` and a 30 s heartbeat that rewrites `snapshot.json` and touches the
registration row. On the owner's PC (AhnLab V3 scans every file touch) this was
the top CPU path: `heartbeatSnapshot` is 32.9% of the post-r2a profile. The
191 were closed by hand offline today; this change stops them from piling up
again.

Read-only findings (cite-checked, `agents-plugin-tool/internal/`):
- Only `CloseConversation` / `closeOne` close a conversation
  (`wsagent/runner.go` ~1103-1140). The idle timeout only stops the process
  (`wsagent/runner_conv.go:382-386`, `idleNow` in `runner_events.go:605-635`).
- Closing a lead does not close its Pi children: the cascade follows
  conversation parents only (`wsagent/children.go:796-823`, condition `:803`).
- A closed Pi child reopens on `Runner.Send` (pi.send and the dashboard POST)
  through `resumePiChild` (`children.go:348-388`) when its parent conversation
  is open and its Pi session file exists. A child whose parent is closed or
  gone cannot be reopened.
- `pi.wait` returns `TurnClosed` with no message for an already-closed child
  (`children.go:841-843`). So a child must not close before its result report
  is delivered (`PendingReports` empty), or the parent loses the result.
- `close()` drops held inbox messages (`runner_events.go:748-760`).
- The 7-day prune (`wsdashboard/registry.go:56` `RegistryRetention`,
  `PruneRegistrations` :355-394) skips closed Pi children because `retired()`
  (:426-428) is true only for leads.

## Owner decisions (2026-11-17)

- (1299): a Pi child conversation closes as soon as its activity stops, not
  after a 24 h idle window. Reopening by message is the recovery path.
- (1300): closed Pi child records are deleted after the same 7-day retention
  as retired leads.

## Outcome

1. **Close on activity stop.** A Pi child conversation (non-lead, Pi backend,
   spawned by a lead or a parent conversation) closes through the existing
   close path as soon as all of these hold:
   - `idleNow()` is true (no turn, no replay, no background tasks, no active
     child conversations);
   - its result report has been delivered: `PendingReports` is empty;
   - no held inbox mail and no `rec.Queued`;
   - no `HeldTurn`.

   Check it at the points where these can become true: turn end, report
   delivery to the parent, and the idle timeout (keep the idle timeout as the
   fallback). Pick the simplest hook that covers them. Calling the close path
   from inside the conversation's own loop may deadlock (`closeOne` stops the
   loop, `CloseConversation` takes `resumeMu`); dispatch it so it cannot.
2. **Lead close cascades to its Pi children.** When a lead conversation
   closes, also close the Pi children whose lead parent (`ParentLead` or the
   equivalent field) is that lead, through the same close path, after the
   existing conversation-parent cascade.
3. **Restore does not revive finished children.** At `Runner.Start`, a Pi
   child record that is open on disk but meets the Outcome 1 conditions as far
   as the record can tell (turn not open, no queued or held mail, no pending
   reports, no held turn), or whose parent conversation is closed or missing,
   is marked closed instead of getting a loop. Others restore as today.
4. **Retention.** `retired()` also covers closed non-lead Pi children, so
   `PruneRegistrations` removes them after `RegistryRetention`. Verify which
   timestamp the age is measured from. It must count from the close (or last
   activity), not from creation, so a child closed today survives 7 days.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing.
- Simplest complete change. Reuse `closed_at` and the existing close and
  prune paths. No new config knobs, no new on-disk fields. Expected tier: T0.
  An older binary reads these records as ordinary closed records. State the
  tier with source lines in the commit.
- Hosted Claude leads and the root conversation are unaffected; only Pi
  children close automatically.
- Do not change heartbeat or snapshot write timing. That is separate design
  work.
- NO local `go test`, Go test executables, ws/ws-mcp executables, the
  launcher, the daemon, or Electron on this PC (owner antivirus rule).
  - Go builds and tests run on w5090 only:
    `D:/sys_lock.sh remote w5090 run conv-auto-close 25 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./internal/wsagent ./internal/wsdashboard ./internal/mcp && go test ./internal/wsagent ./internal/wsdashboard ./internal/mcp'`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- Temp files only under `D:/Temp/conv-auto-close/`.
- Commit on `impl/develop/conv-auto-close` with NO Co-Authored-By trailer.
  Do not push.

## Verification

- Tests:
  - a finished Pi child with its report delivered closes;
  - a child with an undelivered report, held mail, or a held turn stays open
    until those clear, then closes;
  - `pi.wait` started before the child finishes still receives the result;
  - `Send` to an auto-closed child reopens it;
  - closing a lead closes its Pi children;
  - `Start` marks finished and orphaned children closed and starts no loop
    for them;
  - prune removes a closed Pi child older than the retention and keeps a
    recent one.
- One passing w5090 run of the command above.
- Report:
  - the commit;
  - the files;
  - the test results;
  - the tier with source lines;
  - any guard you could not implement.

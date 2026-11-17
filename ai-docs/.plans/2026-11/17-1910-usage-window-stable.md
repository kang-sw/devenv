# Perf: usage history must stop rewriting itself as expired windows flap

## Context

The owner's PC runs AhnLab V3, so every file write costs 10-100x. The
usage-history-fast change (merged at `b2e52d6a1`, files
`agents-plugin-tool/internal/wsdashboard/plan_usage_probe.go` and
`plan_usage_history.go`) made the history response small (633 kB → about
18 kB). Its worker found the root cause of the write load. Each session
refresh goes through `pushClaude`, which takes the session's reported
5-hour window as-is. As a result:
- an expired window that a stale session still reports is re-inserted;
- a report with no window erases the live window.

So the window disappears and comes back about every 6 s, and each flap
rewrites the whole history file (about 633 kB).

The owner approved the fix as (1308)=a on 2026-11-17 at 19:10, including the
popover follow-up.

## Outcome

1. **`pushClaude` stops flapping.**
   - A reported window whose end time has already passed is ignored. It
     neither replaces nor re-inserts anything.
   - A report with no window keeps the existing live window rather than
     clearing it.
   - A newer valid window still replaces the old one at the window boundary.
2. **No rewrite when nothing changed.** If the history content is unchanged
   after a push, the file is not written. Skip-if-unchanged is approved under
   (1297). Compare in memory; do not reread the file to compare.
3. **The popover refetches after a forced refresh.** After the user forces a
   usage refresh, the usage popover fetches `usage?history` again, so it shows
   the new data without a reload. This was the prior worker's Forward.
4. **Optional, only if it is small.** `entries()`/`detect()` read three
   credential files per request. Memoize them, validated by stat (a read
   cache, approved under (1297)). Never log or print credential contents. Skip
   this if it grows beyond a small change, and report it as a Forward.

## Constraints

- **Tier.** No on-disk shape or wire change; expected T0. Cite source lines.
  Read `ai-docs/manuals/release-compatibility.md` before editing
  `agents-plugin-tool/internal/`.
- **Charts unchanged.** The drawn usage charts must not change.
- **Tests.** Add Go tests for:
  - an expired reported window is ignored;
  - a report without a window keeps the live window;
  - a valid newer window replaces the old one;
  - an unchanged push performs no write (use a seam or counter).
  - Use the existing node harness for the popover if one covers it.
    Otherwise state how you verified it.
- **Coordination: do not touch these.**
  - `access.go`, `localhost_auth.go` (another worker);
  - tracker host code (another worker);
  - `wswhiteboard`, `wslive`, `mcp/whiteboard_*`;
  - wide drawer, explorer, cards.js (a peer session's UI track).
- **No local runs on this PC.** No `go test`, Go test exes, `gofmt` or other
  Go tools, ws/ws-mcp exes, launcher or daemon (antivirus rule). Node JS tests
  may run locally.
- **Go runs only on w5090:**
  `D:/sys_lock.sh remote w5090 run usage-window 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && gofmt -l ./internal/wsdashboard && go vet ./... && GOOS=windows go vet ./internal/... && go test -count=1 ./internal/wsdashboard'`
  - Redirect the output to a file under `D:/Temp/usage-window/`.
  - Run it with run_in_background and wait for it to finish. Do not end your
    turn while it is pending.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Time.** The owner leaves at about 20:05 KST. Aim to commit by about
  19:55. If you cannot finish, commit what passes and report what remains.
- **Process.** Simplest complete change. R1 review, then a fixes-only R2 only
  for substantive findings.
- **Commit.** Commit on `impl/develop/usage-window-stable` with your own
  model's Co-Authored-By trailer, and state the tier. Do not push.
- **Temp files** go only under `D:/Temp/usage-window/`.

## Report

- the window rules before → after;
- writes per minute, before → after (estimated from code);
- the popover change;
- whether the credential memo was done;
- the tests;
- the commit and files;
- the w5090 result;
- anything left alone.

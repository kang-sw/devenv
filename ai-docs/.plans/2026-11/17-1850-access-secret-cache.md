# Perf: the dashboard access guard must not read its secret files on every request

## Context

The owner's PC runs AhnLab V3, so every file touch costs 10-100x. The
image-no-base64 worker (merged at `875de30bd`) found that the dashboard
access guard (`wsdashboard/access.go`) does an `os.ReadFile` of
`localhost-secret` on every local request, and of `lan-token` on every LAN
request. It holds the guard's `tokenMu` while it reads. That adds one file
open to every page, topic, image and event request, and serializes
concurrent requests behind the read.

The owner asked whether the check is a real problem. The answer, on the
board: the check stays, because it stops arbitrary web pages and other local
users from driving the daemon. Only the per-request disk read goes.

## Outcome

1. **The secret values are cached in memory.**
   - Serve them from memory, validated by a cheap stat identity (size, mtime,
     file identity), or by a short TTL if a stat per request is still too
     much. Pick the cheaper option that keeps the behaviour below, and say
     which one you chose and why.
   - `wsstate.ReadFileCached` or the existing stat-validated read helpers may
     fit; reuse one rather than add a new cache type.
   - `tokenMu` must not be held across a disk read on the hit path.
2. **Keep today's behaviour.**
   - Rotation: when the file is deleted or rewritten, as by rotation on
     delete or a login-link reissue, the next request after the change sees
     the new value or the absence. State the bound if you use a TTL.
   - The comparison stays constant-time where it is today.
   - Token creation and first-run behaviour are unchanged.
3. **Fix the two stale "inline" image comments** in
   `mcp/dashboard_page.go` (around lines 274 and 284). The inliner was
   removed in `875de30bd`. Comments only.

## Constraints

- **Tier.** No on-disk or wire change; expected T0. Cite source lines.
- **Security.** This is a security boundary. Do not weaken it:
  - a stale cache must never accept a token that was rotated away beyond the
    stated bound;
  - a missing or unreadable file must fail closed as today.
  - R1 review must focus on this.
- **Tests.** Add Go tests for:
  - repeated requests do not re-read the file (use a seam or counter);
  - rotation (rewrite and delete) is seen;
  - an unreadable file still refuses.
- **Coordination: do not touch these.**
  - `wswhiteboard`, `wslive`, `mcp/whiteboard_*`;
  - tracker host code (another worker);
  - dashboard JS/CSS.
- **No local runs on this PC.** No `go test`, Go test exes, ws/ws-mcp exes,
  launcher or daemon (antivirus rule).
- **Go runs only on w5090:**
  `D:/sys_lock.sh remote w5090 run access-secret 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/manifesto-charm-obtrusive' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && GOOS=windows go vet ./internal/... && go test -count=1 ./internal/wsdashboard ./internal/mcp'`
  - Redirect the output to a file under `D:/Temp/access-secret/`.
  - The queue is long. Run the command with run_in_background and wait for
    it to finish. Do not end your turn while it is pending.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Never print or log the secret values** in tests, logs, temp files or the
  report.
- **Process.** Simplest complete change. R1 review, then a fixes-only R2.
- **Commit.** Commit on `impl/develop/access-secret-cache` with your own
  model's Co-Authored-By trailer, and state the tier. Do not push.
- **Temp files** go only under `D:/Temp/access-secret/`.

## Report

- the cache, its validation and the rotation bound;
- file touches per request, before → after;
- the tests;
- the commit and files;
- the w5090 result;
- the tier;
- anything left alone.

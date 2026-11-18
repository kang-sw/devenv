# Fix: Werkschaft recovers when daemon-start reports "unavailable" but a daemon is up

## Context

Release 0.50.6 is super urgent (owner, 2026-11-18 09:44). Two of its changes
make the Windows app replace older daemons automatically: 6b91aff4e (an older
patch) and 302f38aac (an older minor).

Incident on the owner's Windows PC (heavy antivirus, AhnLab V3), 09:45 KST:
- The lead swapped the shared daemon with `dev.py rerun daemon`, i.e.
  `daemon-start --replace`.
- The Werkschaft app, installed v0.50.5, showed "Cannot start dashboard
  daemon: daemon-start exited with status 1: daemon spawn mode=breakaway
  {"outcome":"unavailable","pid":0,"port":0,"version":""}".
- The app did not recover by itself, although the new daemon (pid 32532) had
  been alive and serving HTTP since 09:45:48.

A read-only diagnosis found (paths relative to `agents-plugin-tool/`):
- `daemon-start` returns `unavailable` when `waitForSuccessor`
  (`internal/wsdashboard/ensure.go` about lines 221-249) times out. It needs
  `endpoint.json` with a new token AND `Health` with a pid match
  (`internal/wsdashboard/client.go` about lines 22-47). The report is empty
  because `ReadEndpoint` found no file (`cmd/ws-mcp/proxy.go` about lines
  443-446).
- On this PC every swap prints this false failure. Here, for about 60 s after
  the new daemon started, neither Go (10 s wait) nor Python (60 s) could
  read a valid endpoint, while HTTP was served. The `endpoint.json` file's
  write time is 09:49:06, but its content is from 09:45:48.
- Root cause unknown. Candidates:
  - (a) an antivirus/sharing hold makes the file unreadable for a while;
  - (b) something deletes or recreates the file;
  - (c) a daemon-side self-repair rewrites a missing endpoint. Check whether
    such code exists: grep the endpoint writers and any periodic endpoint
    check.
- In the app, `desktop-electron/daemon-controller.js`:
  - The no-daemon branch (about lines 156-174) launches `daemon-start
    --require-independent-lifetime --start-wait 13s`. A nonzero exit throws
    "Cannot start dashboard daemon" with no re-probe.
  - The incumbent branch (about lines 127-134) re-probes `find` once after a
    launch error, then throws "availability is uncertain".
  - `main.js` (about lines 365 and 425) shows the error, and per the review
    a blocking modal titled "Daemon replacement unavailable" (about lines
    366-371).
- Risk for 0.50.6: on PCs like this one, a successful automatic upgrade could
  still show an error and leave the app unconnected.

## Outcome

1. **App: recover instead of failing** (primary; closes the gap whatever the
   root cause).
   - In `connectDaemon`, when a `daemon-start` launch fails (no-daemon branch
     and incumbent branch), poll `find(root)` for a bounded window of about
     15 s, reusing `waitForDaemon`-style polling.
   - Attach if a daemon answers whose core is acceptable, under the same
     rules as `attachWinner`: never attach an older or unidentified winner
     when an upgrade was intended. If none answers, fail with today's error.
   - When the found daemon is the old incumbent (the upgrade did not happen),
     attach it with a non-blocking notice, not a modal. Only explicit menu
     replace keeps today's dialog.
   - Also fold the correctness-review Minor: `attachWinner` throws "An older
     or unidentified daemon won the handoff" for a third daemon that is newer
     than the incumbent but older than the bundle. Attach it when it is at
     least the incumbent's core (with a notice) instead of failing the
     connection. The bundle being newer can wait for the next start.
2. **App: no blocking modal for automatic upgrades.** Failures of an
   automatic (not user-requested) upgrade must not raise the blocking
   "Daemon replacement unavailable" modal on every startup or recovery
   connect. Use a non-blocking notice or the shell log. Keep the modal for
   the explicit menu replace.
3. **Go: diagnostics only, small.** So the root cause can be caught next
   time:
   - in `waitForSuccessor`, on timeout, log the last `ReadEndpoint` error
     and the last `Health` error (one line each);
   - in `StartDaemon`'s endpoint write (`daemon.go` about line 522), log a
     failure or retry;
   - if `dashboard.log` lines have no timestamps and adding one is a
     one-line logger flag change, add it.
   Do not change wait budgets or Ensure semantics. If you find a clear,
   small root-cause fix (for example a self-repair path or a racy delete),
   stop and report it rather than implementing it.
4. **Tests.**
   - Node: in `desktop-electron/test/daemon-controller.test.js`, add cases
     where a launch exits nonzero and then `find` returns:
     - a newer/equal acceptable daemon → attach;
     - the old incumbent → attach with notice;
     - nothing → error;
     - the third-daemon case → attach.
     Add a main-level test if a test exists for the modal path.
   - Go: keep the existing tests green; add a test only where logging changed
     behaviour (it should not).

## Constraints

- **Read first:**
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/release-compatibility.md`. The expected tier is T0. State
    it in the commit.
- **Scope:** `desktop-electron/daemon-controller.js`, `main.js` and their
  tests; Go logging in `internal/wsdashboard` (`ensure.go`, `daemon.go`, the
  log setup). Another worker is editing the plugin launcher
  (`ws-mcp-launcher.py` copies), `AGENTS.md`, `release-compatibility.md` and
  ticket 261115. Do not touch those.
- **No local runs on this PC** (antivirus rule): no Electron app, `go test`,
  Go test exes, gofmt or other Go tools, ws/ws-mcp exes, launcher or daemon.
  - Node tests for desktop-electron may run locally (`npm test` in
    `agents-plugin-tool/desktop-electron`).
  - Go runs only on w5090:
    `D:/sys_lock.sh remote w5090 run daemon-recover 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/manifesto-charm-obtrusive' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && gofmt -l ./internal/wsdashboard ./cmd/ws-mcp && go vet ./... && GOOS=windows go vet ./... && go test -count=1 ./internal/wsdashboard ./cmd/ws-mcp'`
  - Redirect the output to `D:/Temp/daemon-recover/`, run it with
    run_in_background, and wait for it to finish.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
  - The w5090 queue is shared. Be patient.
- **Never read or print credentials:** endpoint.json tokens, cookies, login
  tokens. Read only pid, port, version and started_at fields if you must
  inspect live files.
- **Urgency:** the owner is waiting to release. Aim to commit within about
  45 minutes. Stop and report if the scope grows.
- **Temp files** go only under `D:/Temp/daemon-recover/`.
- **Process:** Simplest complete change. R1 review, then a fixes-only R2 only
  for substantive findings.
- **Commit:** Commit on `impl/develop/daemon-start-recover` with your own
  model's Co-Authored-By trailer. Do not push.

## Report

- what changed (file:line);
- the app behaviour before → after for each failure shape;
- the Go diagnostics added;
- any root-cause evidence you found;
- the tests and the w5090 result;
- the commit;
- anything left alone.

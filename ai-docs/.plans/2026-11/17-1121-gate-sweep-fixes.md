# Release-gate sweep fixes (I-1..I-5) before v0.50.3

## Context

The owner's (1287)=a: run the release-gate sweep, then ship a provisional
v0.50.3 so the Electron self-update path can be tested.

The sweep of `7c69cc3fe..892d59c23` returned NEEDS FIX with five Important
findings and no Critical. The findings file has the full detail and file:line
citations; read it first:
`C:\Users\user\.cache\ws@kang-sw-devenv\proj\de04b4ac\review-paths\50674a4a-01-findings.md`.

## Outcome

Fix I-1 to I-5 exactly as the findings file describes. The Minor items are
out of scope; leave them.

1. **I-1, update.js relaunch loop.**
   - A stale successful `applyAttempt` must defer the launch at most once, and
     only for the install it targeted. Record the target install in
     `applyAttempt`, and/or clear or stamp it before `deferLaunch`.
   - Add a test: a different or older install, sharing the same userData and
     holding a leftover success result, does not defer.
   - The existing same-folder race still defers once.
2. **I-2, macOS log growth.**
   - Long-running children spawned by `daemon-controller.js launch()` (the
     macOS `dashboard` daemon and the preflight) must not keep an unbounded
     log fd.
   - Keep the log only for the short-lived starter, or the simplest
     equivalent.
   - Add a test.
3. **I-3, manual citations.**
   - Update the stale citations in `ai-docs/manuals/ws-mcp.md` (:20, :30,
     :967, :992) and `ai-docs/manuals/release-compatibility.md` (:81, :94) to
     the current files and lines.
   - Verify each new line number with grep.
4. **I-4, unwired tests.**
   - Add Go wrappers so `go test` runs `shell-post-layout.test.cjs` and
     `pill-layout.test.cjs` when Node is present, skipping when Node is absent,
     like `note_typing_test.go:14`.
   - Wrap `pill-grow.browser.cjs` behind the `WS_PLAYWRIGHT_MODULE` skip.
5. **I-5, Windows helper coverage.**
   - Add a CI-safe variant of the marker-only dummy-folder helper test:
     - it uses a temp dir the test creates;
     - it has no owner-path guard;
     - it runs on Windows only;
     - it never launches an app or daemon.
   - It must run in the default `npm test` on windows-latest.
   - If it cannot be made safe, stop and report why instead of skipping
     silently.

## Constraints

- Before editing `agents-plugin-tool/`, read:
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/release-compatibility.md`.
- Simplest complete fix; no refactors beyond the findings.
- Do not touch `agents-plugin/rsrc/whiteboard/whiteboard.md`.
- NO local `go test`, Go test executables, ws/ws-mcp executables, or the
  launcher on this PC (owner AV rule). Do not start the daemon or Electron.
  - Go tests run only on remote w5090:
    `D:/sys_lock.sh remote w5090 run gate-fix <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/manifesto-charm-obtrusive' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard ./internal/wswhiteboard ./internal/mcp'`.
  - w5090 has no Node, so the new wrappers skip there.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- Node tests run locally:
  - the desktop-electron `node --test test/*.test.js`;
  - the two `.cjs` scripts, directly with node.
  - The I-5 test may run its PowerShell helper against a test-created dummy
    folder under `D:/Temp/gate-sweep-fixes/`; that does not launch any app.
- Temp files only under `D:/Temp/gate-sweep-fixes/`, deleted afterwards.
- Commits carry no Co-Authored-By trailer. Do not push.

## Verification

- desktop-electron Node suite: all pass, and the new I-5 test runs, not
  skipped, on this Windows PC with default env.
- One w5090 Go run as above.
- R1 review; a fixes-only R2 only for substantive findings.
- Report:
  - the commits;
  - the files changed;
  - the test results;
  - per-finding status (I-1..I-5).

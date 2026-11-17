# Electron Windows build: start the daemon automatically

## Context

Owner 2026-11-17 09:15 KST: the packaged Windows Electron app
(`agents-plugin-tool/desktop-electron/`, the win32 desktop build the release
workflow produces) must start the ws daemon by itself. Without this the app
cannot be distributed. Earlier work repeatedly missed this requirement, so it
is the single outcome of this task. Recent related commits: 9db551d0b
(bundled daemon input assembled from release binaries), 3d0940f0f (daemon tree
unpacked into app.asar.unpacked), ba0926ee7 (unpackaged entry fix), 621131667
(startup failure handler), the Python-missing guide page.

## Outcome

1. Launching the packaged Windows app with no daemon running starts the daemon
   automatically (from the bundled daemon the package ships, not from a
   developer checkout or a globally installed plugin), waits until it is
   reachable, and opens the dashboard. No manual step; the "daemon not
   running" page is no longer the normal outcome on a clean Windows machine.
2. When a compatible daemon is already running, the app attaches to it and
   starts nothing.
3. When the automatic start fails, the existing daemon-missing page shows the
   failure reason (and the log path), with a retry that re-attempts the start.
4. The daemon process outlives or ends with the app according to whatever the
   existing daemon lifecycle contract already is; do not invent a new one.

## Approach

- First establish the root cause with evidence: trace the exact code path a
  packaged win32 launch takes today (paths resolved inside app.asar /
  app.asar.unpacked, the executable or interpreter it spawns, spawn options,
  readiness wait) and state why it does not start the daemon on Windows.
  Likely suspects to check, not conclusions: the start path still requires
  Python or a launcher script that the packaged app does not ship; a path
  resolved inside app.asar instead of app.asar.unpacked; a `.exe` suffix or
  spawn `shell`/`windowsHide` issue; the start being gated behind a dev-only
  condition; a readiness timeout too short for a cold first start.
- Fix the simplest complete way inside `desktop-electron/`. If the fix needs a
  change to the daemon, the launcher contract, or the release packaging
  outside `desktop-electron/`, it is still in scope when small; state its
  compatibility tier in the commit `## AI Context`. Stop and report before a
  T3 change.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `agents-plugin-tool/`, and `ai-docs/manuals/release-compatibility.md` before
  editing `agents-plugin-tool/internal/`.
- NO local `go test`, no Go test exes, no ws/ws-mcp executables, no launcher,
  and do not launch the Electron app against a real daemon on this PC (owner
  AV rule). Node tests run locally. Go, if touched, only on remote w5090:
  `D:/sys_lock.sh remote w5090 run <who> <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ...'`.
- Temp files only under `D:/Temp/electron-daemon-autostart/`, deleted after.
- Commits carry no Co-Authored-By trailer.
- Do not push develop or main; no release, no tags.

## Verification

- Node unit tests: packaged win32 path resolution points at the unpacked
  bundled daemon; no daemon -> start is spawned with the right command and
  options and the dashboard loads after readiness; daemon present -> no spawn;
  start failure -> daemon-missing page with the reason; retry re-attempts.
- Windows packaged evidence: if the repository's existing CI has a desktop
  win32 build or Windows smoke job runnable on a branch (workflow_dispatch),
  you may push this impl branch and run it, extending the smoke to assert that
  the packaged app started the daemon and reached the dashboard if that is
  small. Report the run URL and result. If no such job is runnable, say so and
  give the exact manual check for the owner.
- Report: root cause with file:line evidence, the fix, test results, and what
  remains unverified on a real Windows machine.

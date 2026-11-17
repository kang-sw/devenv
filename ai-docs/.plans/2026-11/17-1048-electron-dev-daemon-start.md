# Electron dev run: "Start daemon" button starts the repo checkout's daemon

## Context

Owner 2026-11-17 10:47: the start-daemon button merged in 9d079b660 (plan
`17-1035-electron-start-daemon-button.md`) shows disabled in the unpackaged
dev run (`npx electron .` from `agents-plugin-tool/desktop-electron`), which
is the build the owner actually tests with. The page reads:

- "Daemon start is unavailable in this development run."
- "Cannot use bundled daemon at
  `D:\Workspace\Repos\devenv\agents-plugin-tool\desktop-electron\daemon`:
  ENOENT ... stat '...\desktop-electron\daemon\.claude-plugin\plugin.json'".

The previous worker chose "dev run without a bundled daemon: disabled with a
reason". The owner needs the button to work in the dev run too.

## Outcome

1. In an unpackaged dev run where `desktop-electron/daemon/` is absent, the
   daemon source falls back to the repo checkout's plugin package. The
   packaged build stages that same package into `daemon/`. It is most likely
   `<repo>/agents-plugin/` (it has `.claude-plugin/plugin.json` and the
   launcher). Confirm against the staging script that builds `daemon/` and
   pick what it copies.
2. Both the button and the launch-time autostart use this fallback. They
   share the same start function (staging, Python gate, breakaway fallback).
   - Make the fallback a change in where the daemon root is resolved, not a
     second start path.
3. If the repo plugin root cannot start a daemon as-is, keep the button
   disabled and make the reason name the exact missing piece and the command
   the owner can run instead. One example of a missing piece is a launcher
   that needs a built `ws-mcp` binary that is not present.
   - Report this case; do not build binaries.
4. Packaged builds are unchanged: they still use only the bundled `daemon/`.
   No fallback outside an unpackaged run.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `agents-plugin-tool/`.
- Keep the change to daemon-root resolution in `desktop-electron` and its
  Node tests. Do not touch `update.js` or the no-daemon page layout beyond
  the reason text.
- NO local `go test`, Go test exes, ws/ws-mcp executables, or the launcher on
  this PC (owner AV rule).
  - Do not start the daemon or Electron.
  - Node unit tests run locally; mock the filesystem and spawn.
- Temp files only under `D:/Temp/electron-dev-daemon-start/`, deleted
  afterwards.
- Commits carry no Co-Authored-By trailer. Do not push.

## Verification

- Node tests:
  - an unpackaged run with no `daemon/` and a present repo plugin root
    resolves to the repo root, and the button is enabled;
  - a packaged run never falls back;
  - both missing gives a disabled button with a reason.
- The existing desktop-electron Node suite passes.
- One review round (R1); a fixes-only R2 for substantive findings.
- Report:
  - files changed and the commit;
  - test results;
  - the resolved dev-run daemon root;
  - whether the repo root can start a daemon without a prior build, and if
    not, what is missing.

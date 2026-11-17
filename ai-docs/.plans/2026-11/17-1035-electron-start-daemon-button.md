# Electron: "start daemon" button on the no-daemon page

## Context

Owner, 2026-11-17 10:34: when the daemon is down and the Electron app shows
its "no daemon" page, the owner wants a button that starts the daemon by
hand.

Electron already starts the bundled daemon automatically at launch. That
work merged in 74cf094bc and touched:

- `desktop-electron/main.js` (Python gate around line 261);
- `daemon-stage.js` (cross-drive staging);
- the Windows CREATE_BREAKAWAY_FROM_JOB fallback.

It also attaches to a daemon that is already running. The no-daemon page
also covers the Python-missing guide. A daemon can still be absent while the
app is open, for example when it was stopped or replaced, or when it crashed
after launch. In that case there is currently no way to start one from the
app.

## Outcome

1. The no-daemon page gets a primary button "데몬 시작" (i18n'd like the
   rest of the page; English "Start daemon").
   - It runs the same start path the launch-time autostart uses: the same
     staging, breakaway handling and Python gate.
   - Do not create a second start implementation; reuse or extract the
     existing function.
2. While starting, the button is disabled with a progress label
   ("시작하는 중…").
   - On success the app attaches and shows the dashboard, exactly as after
     autostart.
   - On failure the page shows the reason (for example Python missing, the
     bundled daemon is absent, or a timeout) and re-enables the button.
3. If a daemon has meanwhile come up on its own (another launcher, or a
   `--replace` swap), the button just attaches; it never starts a second
   daemon.
4. Packaged builds and the unpackaged dev run (`npx electron .`) behave as
   autostart does in each. If autostart is not available in a mode, for
   example the dev run without a bundled daemon, show the button disabled
   with a one-line reason, or hide it. Report which you chose.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `agents-plugin-tool/`.
- Keep the change to the no-daemon page, its IPC/preload bridge, and a
  shared start function.
- Do not change autostart behaviour or the self-update code (`update.js`,
  merged 92a94e16e).
- NO local `go test`, Go test exes, ws/ws-mcp executables, or launcher on this
  PC (owner AV rule).
  - Do not start the ws daemon, the bundled daemon, or the Electron app
    against a real daemon.
  - Node unit tests run locally: mock the start function and the IPC.
- Temp files only under `D:/Temp/electron-start-daemon-button/`, deleted
  afterwards.
- Commits carry no Co-Authored-By trailer. Do not push develop or main.

## Verification

- Node tests:
  - the button invokes the shared start path;
  - a second click while starting is ignored;
  - a failure re-enables the button with a reason;
  - an already-running daemon makes the button attach without spawning;
  - the dev-run mode follows the chosen behaviour.
- The existing desktop-electron Node suite passes.
- R1 review; a fixes-only R2 for substantive findings.
- Report:
  - files changed;
  - commit;
  - test results;
  - the dev-run behaviour choice;
  - the exact owner check: stop the daemon, see the page, click the button,
    and the dashboard appears.

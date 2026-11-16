# Electron shell: Python install guide page when Python is missing

## Context

Owner request 2026-11-16 17:46 KST: when Python is not available, the Electron
shell should show a Python install guide as its first page, the same way it
already shows a page when the daemon is not running.

## Outcome

1. At startup, before or instead of trying to start/reach the daemon through a
   path that needs Python, the Electron shell detects that no usable Python is
   available (whatever interpreter lookup the shell's daemon start path already
   relies on; reuse its resolution, do not invent a second one).
2. In that case it loads a local guide page styled like the existing
   "daemon not running" page: says Python is required, how to install it on
   Windows (python.org installer or `winget install Python.Python.3.13`, check
   "Add python.exe to PATH"), and offers a retry action that re-runs detection
   and proceeds normally once Python is found.
3. When Python is present, nothing changes.
4. Human-facing strings follow the existing page's language/i18n style.

## Constraints

- Simplest complete change inside `agents-plugin-tool/desktop-electron/`;
  follow the existing daemon-missing page's structure and style.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `agents-plugin-tool/`.
- NO local `go test`, no test exes, no ws/ws-mcp exes, no launcher, and do not
  launch the Electron app against a real daemon on this PC. Node tests
  (`main.test.js` style) run locally. If any Go file is touched, Go runs only
  on remote w5090 (`D:/sys_lock.sh remote w5090 run ...`).
- Temp files only under `D:/Temp/electron-python-guide/`, deleted after.

## Verification

- Node unit test: Python missing -> guide page chosen; Python present ->
  normal path; retry re-runs detection.
- LOOK: render the guide page HTML (static, e.g. Playwright/Chromium against
  the file) at ~1200px, dark theme if the existing page is dark; save the PNG
  under D:/Temp/electron-python-guide/ and report its path.
- R1 review; fixes-only R2 for substantive findings.

# Electron: F12 / Ctrl+Shift+I do not open DevTools

## Context

Owner 2026-11-17 09:17 KST, on the Windows Electron app
(`agents-plugin-tool/desktop-electron/`): pressing F12 or Ctrl+Shift+I does
not open the developer tools window. DevTools support landed in bcd90e2a5
(Exec3 fcb1e9702: View > Toggle Developer Tools, F12 and Ctrl+Shift+I
targeting the dashboard webContents) and was only unit-tested, never checked
on a real Windows app. Later desktop commits may interact: ba0926ee7 (entry
start condition), 688470a44 (localhost cookie injection before navigation),
3d0940f0f (asar.unpacked daemon tree), 9db551d0b, 621131667. It is unknown
whether the owner ran the packaged v0.50.2 win32 build or an unpackaged dev
run; the fix must work in both.

A parallel worker is changing daemon auto-start in the same package on branch
impl/develop/electron-daemon-autostart; keep this change to the DevTools path
so the two merge cleanly.

## Outcome

1. In the Windows app, packaged and unpackaged, F12 and Ctrl+Shift+I open
   (toggle) DevTools for the dashboard page, also when the page has focus and
   when the dashboard's own keydown handlers run, and also on the
   daemon-missing / Python guide / login pages.
2. The View menu item (if the menu is shown) keeps working.

## Approach

- Root cause first, with file:line evidence. Suspects to check, not
  conclusions: shortcuts bound only as menu accelerators while the
  application menu is null/hidden or not set on Windows; `devTools: false` in
  webPreferences for packaged builds; accelerators registered on a window that
  is later replaced, or on the wrong webContents (e.g. a BrowserView /
  WebContentsView whose input does not reach the window menu); the dashboard
  page swallowing the key event; ba0926ee7 changing startup order so the
  registration never runs.
- Simplest robust fix: e.g. a `before-input-event` handler on every
  webContents the app shows that toggles DevTools for F12 and
  Ctrl+Shift+I (Cmd+Alt+I on macOS), plus the menu item.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `agents-plugin-tool/`.
- NO local `go test`, no Go test exes, no ws/ws-mcp executables, no launcher,
  and do not launch the Electron app against a real daemon on this PC (owner
  AV rule). Node tests run locally.
- Temp files only under `D:/Temp/electron-devtools-keys/`, deleted after.
- Commits carry no Co-Authored-By trailer. Do not push develop or main.

## Verification

- Node unit test: the key handler toggles DevTools for F12 and
  Ctrl+Shift+I on each shown webContents, ignores other keys, and is
  registered in both packaged and unpackaged startup.
- Report: root cause with file:line, the fix commit, Node test results, and
  the exact manual check for the owner.

# Electron: DevTools frontend blocked (ERR_BLOCKED_BY_CLIENT)

## Context

Owner 2026-11-17 09:39, on a fresh dev run (`npx electron . --enable-logging`
in `agents-plugin-tool/desktop-electron`, develop 58da8580c): F12 still opens
no developer tools. Two earlier fixes (f646e388b before-input-event handler,
feba39f94 detached DevTools for the dashboard WebContentsView) are in and the
handler fires; stdout shows
`DevTools shortcut F12: source webContents=1, dashboard webContents=1`.
The real failure is in stderr:

```
electron: Failed to load URL: devtools://devtools/bundled/devtools_app.html?remoteBase=https://chrome-devtools-frontend.appspot.com/serve_file/@78eb7a08.../... with error: ERR_BLOCKED_BY_CLIENT
```

`ERR_BLOCKED_BY_CLIENT` means the app itself cancelled the request: most
likely a `session.webRequest.onBeforeRequest` (or similar request/navigation
guard, permission handler, `will-navigate`/`will-attach-webview`/
`setWindowOpenHandler`, protocol interception, or the localhost cookie/token
injection from 688470a44) that only allows the daemon origin and cancels
everything else, including the `devtools://` scheme the DevTools frontend
loads from. Find the exact guard first (file:line), do not assume.

Parallel workers also edit `desktop-electron/` (daemon auto-start in
`main.js`, self-update in `update.js`); keep this change to the guard so the
merges stay clean.

## Outcome

1. F12, Ctrl+Shift+I and the View menu open DevTools for the dashboard in the
   Windows app, packaged and unpackaged: the request guard lets the DevTools
   frontend load (`devtools:` scheme and whatever DevTools itself requests,
   e.g. `chrome-devtools:`/`devtools://` resources) while still blocking what
   it was meant to block for page content.
2. Root cause stated with file:line evidence.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `agents-plugin-tool/`.
- NO local `go test`, Go test exes, ws/ws-mcp executables, or launcher on this
  PC. Node tests run locally. Do not kill or launch Electron instances: the
  owner has a dev run open; the lead relaunches after merge.
- Temp only under `D:/Temp/electron-devtools-blocked/`, deleted after.
- Commits carry no Co-Authored-By trailer. Do not push develop or main.

## Verification

- Node unit test: the guard allows `devtools://` (and DevTools frontend)
  URLs and still blocks a disallowed page URL it blocked before.
- Existing desktop-electron Node tests pass (a pre-existing
  `host.test.js:93` packaging-fixture failure may be excluded if proven at
  base).
- Report: root cause file:line, fix commit, test results.

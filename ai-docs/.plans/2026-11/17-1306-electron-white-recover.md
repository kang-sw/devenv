# Electron shell: recover from white window (failed main-frame load, renderer death, hang)

## Context

Owner, 2026-11-17 12:53: the Electron window sometimes turns fully white and comes
back only after a manual refresh.

A read-only investigation found that
`agents-plugin-tool/desktop-electron/main.js` registers no handler for any crash
or load-failure event. Specifically:

- **Failed loads.** Only the initial `loadURL`/`loadFile` rejection is handled
  (`loadDocument`, main.js:264-279, which falls back to `connection.html` via
  `navigationFailed`, :252-258). A page-initiated top-level navigation that
  fails commits a Chromium error page. That page renders blank white with no
  fallback. The likely trigger is the dashboard's `assets`-event
  `location.reload()` (`dashboard-stream.js:516-528`) firing while the daemon is
  slow or restarting.
- **Renderer death.** No `render-process-gone` handler, so a dead renderer stays
  white.
- **Hangs.** No `unresponsive` handler.
- **Manual retry.** Connection > Retry (Ctrl+R) calls `connect()`, which does a
  fresh `loadURL`. That is why a manual refresh recovers.
- **Evidence.** The dashboard renderer process was replaced at 12:52:30, just
  before the report. No GPU or network-service restart occurred.

## Outcome

In `main.js`, on the dashboard `WebContents`:

1. **`did-fail-load`.**
   - Act only when `isMainFrame` is true and the error is not ERR_ABORTED (-3).
     Ignore subframes.
   - Retry automatically with a short backoff (for example 1 s, 2 s, 4 s, capped
     around 10 s), reusing the existing `connect()` / pinned-URL path.
   - Keep showing `connection.html` with the error in the meantime, if that is
     what `connect()` already does on failure.
   - Do not loop forever without the user seeing the state: after the cap, keep
     retrying at the cap interval while showing the connection page.
   - Reset the backoff after a successful load.
2. **`render-process-gone`.** When `reason !== 'clean-exit'`, log the reason and
   reconnect via `connect()`. Use the same backoff or cap so a renderer that
   crashes on load does not spin.
3. **`unresponsive` / `responsive`.** Log them. If the renderer stays
   unresponsive for about 10 s, reload through `connect()`.
4. **Logging.** Log each event (reason, error code, URL with any query string
   stripped, so no `login=` token is logged) to the existing shell log mechanism,
   if there is one. Otherwise use console output, which goes to the
   `--enable-logging` terminal. Next time, the cause will then be visible.
5. **Request guard.** Make sure the `webRequest.onBeforeRequest` guard
   (main.js:204-213) does not cancel the retry because `pin` is null after a
   failure. The retry must target the daemon origin it was pinned to.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` first.
- Simplest complete change in `desktop-electron/`. No new config knobs.
- Do not change the daemon, dashboard JS or Go.
- Do NOT launch Electron, the daemon, ws/ws-mcp executables, the launcher, or
  `go test` on this PC (owner AV rule).
- Node unit tests for desktop-electron run locally, if they exist. Add tests for
  the handler logic where the module structure allows it. For example, factor the
  backoff and decision logic into a small pure function, as `policy.js` is
  tested.
- Temp files only under `D:/Temp/electron-white-recover/`, deleted afterwards.
- Commit with no Co-Authored-By trailer. Do not push.

## Verification

- Unit tests for the decisions:
  - a main-frame non-abort failure retries with backoff;
  - a subframe or ERR_ABORTED is ignored;
  - `render-process-gone` with a non-clean exit reconnects;
  - the backoff is capped and resets on success.
- Report:
  - the commit;
  - the files;
  - the test results;
  - how the owner picks it up (dev run: restart `npx electron .`).

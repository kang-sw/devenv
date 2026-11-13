# Dashboard: installable as an app (PWA)

## Problem

Owner request 2026-11-13: the dashboard page (served by the daemon at
`http://127.0.0.1:29417`, e.g. `/` and the whiteboard page `/s/<replyid>`)
should be installable as an app (Edge/Chrome "Install app", own window, own
taskbar icon) instead of living in a browser tab.

## Outcome

- The daemon serves a web app manifest (`name`/`short_name` e.g. "ws
  dashboard", `start_url` the dashboard root, `scope` "/" so session and
  whiteboard pages open inside the app window, `display: standalone`,
  dark `theme_color`/`background_color` matching the dashboard theme) and
  icons (at least 192 and 512 px PNG, plus a maskable one if cheap; an
  original simple glyph, no third-party artwork). Every dashboard HTML page
  links the manifest and sets `theme-color`.
- Installability criteria current Edge/Chrome need on `http://127.0.0.1`
  (a secure context): establish whether a service worker is still required
  for the install prompt; if required (or needed for the install menu item
  in Edge), add a minimal one that passes requests straight to the network
  (no offline cache: the dashboard is live data, a stale cache would show
  wrong state). State the finding in the commit's `## AI Context`.
- Nothing about the existing page behaviour changes: same routes, same
  fetches, same live updates. Opening a session link from inside the app
  stays in the app window.
- Note: the daemon port is fixed (29417), so the installed app's origin is
  stable. If the port can be configured, say what an installed app does when
  it changes (no fix needed, just report).
- Assets/served routes only (`agents-plugin-tool/internal/wsdashboard/`),
  T0. No i18n strings unless a visible string is added.

## Out of scope

- Offline mode, push notifications, badges.
- Restarting the daemon or sessions (the lead does the live check).

## Verification

- `go test ./internal/wsdashboard/... -count=1` on remote w5090 (clone note
  `remote-build-w5090`), with a test that the manifest and icons are served
  with the right content types and that the pages link the manifest.
- If cheap: a headless Edge check against a locally started test server
  (not the live daemon) that the manifest parses (e.g. DevTools protocol
  `Page.getAppManifest` reports no errors / `Page.getInstallabilityErrors`
  is empty). State what was not checked.

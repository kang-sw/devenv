---
title: "Migrate the Werkschaft desktop shell from Tauri to Electron"
related:
  261113-feat-werkschaft-thin-tauri-shell: "current Tauri shell (Phase 1 Result e7121971); parity source; its binding decisions carry over except the webview technology"
  261112-research-ws-ade-desktop-shell-and-rename: "Werkschaft concept; Electron was the recorded fallback if the webview matrix proved painful"
---

# Migrate the Werkschaft desktop shell from Tauri to Electron

## Background

Owner decision (843)=b, relayed by MAIN on 2026-11-15 at 00:59:09Z: migrate
the Werkschaft desktop shell from Tauri to Electron.

Owner-approved rationale: Tauri on macOS renders through WKWebView, so every
browser default the dashboard relies on has to be re-implemented natively:
zoom keys, `beforeunload` close confirmation, and link/file opening. The
dashboard is built and Playwright-tested on Chromium. Electron embeds
Chromium, which removes the "works in the browser, not in the app" class of
defects. The Tauri ticket recorded Electron as the fallback if the webview
matrix proved painful (`261113-feat-werkschaft-thin-tauri-shell` Decision 1).

The current shell lives in `agents-plugin-tool/desktop/` (Rust:
`src/main.rs`, `src/zoom.rs`, `src/navigation.rs`, `src/endpoint.rs`;
`tauri.conf.json`; `scripts/build-slot.py`). A read-only parity inventory
was taken on 2026-11-15 at develop `1612e099c`; the table below is its
summary, with evidence paths relative to `agents-plugin-tool/`.

### Parity inventory of the Tauri shell

| # | Current behavior | Evidence | Electron requirement |
|---|---|---|---|
| P1 | macOS-only; one window navigated to the daemon-served dashboard; bundled HTML is only the connection page | `desktop/src/main.rs:3-4,158-162`, `desktop/ui/index.html` | Same: one `BrowserWindow`, no second frontend; a bundled connection page only |
| P2 | Endpoint discovery: `WS_CACHE_HOME` override, else `$HOME/.cache/ws@kang-sw-devenv`; reads `dashboard/endpoint.json` | `desktop/src/endpoint.rs:28-49,103-110` | Same resolution order and file |
| P3 | Attach validation: 64 KiB cap; only `port`/`pid` read, both nonzero; GET `http://127.0.0.1:<port>/health`, no proxy, no redirects, 2 s timeout; `ws-dashboard` service and matching PID; reread endpoint after health and reject a change | `desktop/src/endpoint.rs:12-25,47-100` | Same checks in the main process |
| P4 | Credential-free attach: endpoint token never read into the shell or page; no daemon start/lifecycle call; endpoint file never written | `desktop/src/endpoint.rs:14-19,47-100`, `desktop/README.md:123-130` | Same |
| P5 | Titles: `Werkschaft — Connecting…`, `Werkschaft` on attach, connection page with the error and `⌘R to retry` on failure, `Navigation failed` on immediate navigation error | `desktop/src/main.rs:48-85,158-159` | Same strings and states |
| P6 | Connection → Retry (`CmdOrCtrl+R`) reruns discovery/health/attach, not a page reload; overlapping attempts ignored; no periodic reconnect | `desktop/src/main.rs:48-61,87-93,102-104,115-118` | Same menu item and semantics |
| P7 | Window 1280×860, minimum 760×520; no geometry, URL or zoom persistence; zoom resets to 100% per process | `desktop/src/main.rs:96-101,158-162` | Same (no persistence) |
| P8 | Default macOS menus (App, File, Edit, View Fullscreen, Window, Help) plus Connection and View additions | `desktop/src/main.rs:115-153` | Equivalent standard roles plus the same additions |
| P9 | View zoom: Actual Size `CmdOrCtrl+0`, Zoom In `CmdOrCtrl+=` and `CmdOrCtrl+Shift+=`, Zoom Out `CmdOrCtrl+-`; ladder 50, 67, 75, 80, 90, 100, 110, 125, 150, 175, 200, 250, 300 %, clamped; off-ladder values snap to the adjacent step | `desktop/src/zoom.rs:1-48`, `desktop/src/main.rs:104-151` | Same ladder, shortcuts and clamping |
| P10 | Only the exact discovered origin `http://127.0.0.1:<port>` stays in the window; `localhost`, IPv6, HTTPS or another port count as external; rediscovery replaces the pin | `desktop/src/navigation.rs:16-38,46-76` | Same exact-origin pin |
| P11 | External HTTP(S) navigation (including other local ports and remote boards) is blocked in-window and opened in the system browser | `desktop/src/main.rs:38-45,163-175` | Same, through the OS opener for http/https only |
| P12 | Userinfo URLs and file/javascript/data/mailto/ws/custom schemes are blocked and never sent to the OS opener | `desktop/src/navigation.rs:5,16-38,80-108` | Same |
| P13 | `about:blank` / `about:srcdoc` allowed for sandboxed frames, with no native authority | `desktop/src/navigation.rs:20-25` | Same; Electron may restrict them to subframes |
| P14 | Popups never create a native window: same-origin → navigate main window; external → system browser; others ignored | `desktop/src/main.rs:177-198` | Same policy |
| P15 | User agent ends in `Werkschaft/<version>`; the dashboard and board frames use it for desktop file-link behavior (click → link menu, Cmd/Ctrl-click → preview) | `desktop/src/main.rs:22-30,160`, `internal/wsdashboard/assets/dashboard.js:3127-3154,3285-3304,4227-4243`, `internal/wswhiteboard/assets/doc-frame.js:135-152,186-191` | Keep the same UA token so the dashboard needs no change |
| P16 | "Open with system" is daemon HTTP (`open`, `fs/act` open/reveal), not shell IPC | `internal/wsdashboard/assets/dashboard.js:3206-3246`, `internal/wsdashboard/fs_act_route.go` | Works unchanged; verify from the Electron window |
| P17 | X-close destroys the window without consulting `beforeunload`; closing never stops the daemon | `desktop/src/main.rs:96-202` | Closing still never stops the daemon; X-close now honors `beforeunload` (see Decisions) |
| P18 | No single-instance handling (Tauri Phase 2 scope) | `desktop/src/main.rs:100-101` | Phase 1 adds it ((858)=a) |
| P19 | Identity: product `Werkschaft`, version `0.1.0`, bundle id `dev.werkschaft.desktop`, `.app` target, temporary `icons/icon.png` | `desktop/tauri.conf.json:3-5,23-27` | Same identity and icon |
| P20 | Pinned, frozen build: `scripts/build-slot.py --allocated` builds an unsigned debug `.app` offline with owned-process cleanup | `desktop/scripts/build-slot.py` | An offline, pinned, unsigned `.app` build with the same owned-process discipline |
| P21 | Security: no invoke handler, plugins, preload or global API; empty capability; page gets zero native command authority; connection-page CSP `default-src 'none'` | `desktop/src/main.rs:100-101,217-233`, `desktop/tauri.conf.json:9-20` | Same zero-authority posture (see Decisions) |

## Decisions

- **(843)=b, owner: Electron replaces Tauri as the shell technology.**
  Rationale as in Background. Rejected: keep Tauri and re-implement each
  browser default natively in Rust (the cost the owner wants to stop
  paying).
- **Binding Tauri decisions carry over unchanged** except the webview
  technology (Decision 1's choice of Tauri). From
  `261113-feat-werkschaft-thin-tauri-shell`: Decision 1 (thin shell loading
  exactly `http://127.0.0.1:<port>/`, external links to the system browser,
  closing leaves the daemon running, no self-update); Decision 2 (the
  launcher's SHA256-pinned daemon stays the authority; the shell never
  replaces or stops a running daemon); Decision 3 (attach only: read the
  endpoint, respect the cache override, check health, never invoke
  `ws-mcp daemon-start`, never hand the endpoint token to page JavaScript);
  Decision 4 (unsigned local dogfood `.app`; signing and notarization are
  separate work); Decision 7 (no new UI credential or daemon API merely to
  render); Decision 8 (a scoped grant only for a later, justified authority
  need, under that ticket's constraints).
- **Remote-dashboard posture: no added native privilege.** The renderer runs
  with `contextIsolation: true`, `nodeIntegration: false`, `sandbox: true`,
  and no preload script, matching P21 and Decision 7. A remote or
  non-pinned-origin board never loads in the window (P10, P11). Rejected: a
  preload bridge for zoom, close or open-with-system, which the main process
  and the daemon's HTTP routes already cover.
- **X-close shows the dashboard's close confirmation exactly as in Chrome**
  (acceptance criterion, not a separate Tauri fix). Window close runs the
  page's `beforeunload`; when the page prevents unload, the main process
  handles `webContents` `will-prevent-unload` with a native leave/stay dialog
  and closes only on "leave". Today the dashboard prevents unload only while
  a pasted image lacks an attachment reference
  (`internal/wsdashboard/assets/dashboard.js:8812-8824,8925-8930`); the shell
  follows whatever the page decides and adds no rule of its own. Rejected:
  re-implementing the page's guard in the main process.
- **Keep the Tauri crate in place** until the Electron shell reaches verified
  parity. Removing `agents-plugin-tool/desktop/` Tauri sources is a separate,
  owner-approved step; this ticket deletes no functionality.
- **Packaging: macOS `.app` first.** The current shell is macOS-only (P1).
  Windows delivery is the later Phase 3 (re-homed by (859)); Linux is out of
  scope.
- **(857)=a, owner: the Electron download is a one-time, MAIN-run
  prerequisite with an exact pinned version.** MAIN (allowed by the owner:
  "MAIN에서 하는 거 허용입니다.") runs the networked install described in
  Constraints → Prerequisite; the lockfile records the exact versions and
  integrity hashes; afterwards every install and build is offline. Workers
  never fetch. Rejected: a manual release-zip download into a custom
  `ELECTRON_CACHE` (more manual steps, no lockfile integrity); vendoring the
  binary into git (about 100 MB per version).
- **(858)=a, owner: single-instance launch/focus lands in Phase 1.** A second
  launch focuses the existing window (`app.requestSingleInstanceLock` and the
  `second-instance` event) instead of opening another window. Rejected:
  strict parity first and single-instance later.
- **(859)=a, owner: the Tauri ticket's unimplemented Phases 2-4 move here.**
  They become Phases 2-4 below, rewritten for Electron (single-instance
  already moved into Phase 1). After Phase 1 verifies parity, the lead closes
  `261113-feat-werkschaft-thin-tauri-shell` with a note that its Phases 2-4
  were re-homed here; its Phase 1 Result stays as history. Rejected: leaving
  them on the Tauri ticket; dropping them.
- **Electron sources live in a new sibling directory
  `agents-plugin-tool/desktop-electron/`.** The Tauri crate stays in place
  until parity; separate directories keep two independent package manifests,
  lockfiles and build outputs apart, and the later Tauri removal is a clean
  delete. Rejected: sharing `agents-plugin-tool/desktop/`.
- **Packaging uses plain `@electron/packager`.** An unsigned `.app` needs
  only packaging; the packager has the smallest dependency tree to fetch once
  and keep offline. Rejected: `electron-builder` and Electron Forge, which
  add installers, publishers and auto-update machinery this ticket excludes.
- **The main process is plain JavaScript (CommonJS).** The shell is a few
  hundred lines; a TypeScript compile step adds an offline toolchain
  dependency for no runtime benefit. Rejected: a TypeScript build step.
- **Compatibility: not a cross-version surface.** The desktop shell adds no
  MCP tool or argument, `/mcp` hello change, shared store shape, tracker
  event or config/rsrc manifest field; it consumes the existing endpoint file
  and HTTP routes read-only. No tier applies (T0 if one must be named).

## Constraints

- **Offline build environment.** Builds and tests run with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` and npm offline
  (`npx --offline`, `npm ci --offline`). Nothing in this ticket fetches from
  the network silently; the one-time Electron download is the MAIN-run
  prerequisite below.
- **Prerequisite (MAIN-run, before Phase 1 dispatch; (857)=a).** In a pool
  worktree on the branch Phase 1 will start from, MAIN runs:

  ```sh
  mkdir -p agents-plugin-tool/desktop-electron
  cd agents-plugin-tool/desktop-electron
  printf '%s\n' '{"name":"werkschaft-desktop-electron","version":"0.1.0","private":true,"main":"main.js"}' > package.json
  npm install --save-dev --save-exact electron@44.5.1 @electron/packager@latest
  rm -rf node_modules && npm ci --offline   # proves the offline reinstall
  ```

  `electron@44.5.1` is the newest stable release visible offline on
  2026-11-15 (`electron-to-chromium` 1.5.444 in the local npx cache lists
  44.5.1 as the latest stable and 45.0.0 only as alpha). If
  `npm view electron@44 version` shows a newer 44.x patch at install time,
  MAIN pins that patch instead. `@electron/packager` is saved at the exact
  version `latest` resolves to. MAIN commits `package.json` and
  `package-lock.json` (never `node_modules/`) and records both resolved
  versions in the commit's `## AI Context`. The Electron zip lands in
  Electron's default cache (`~/Library/Caches/electron`) and the npm
  tarballs in the npm cache, which keeps later `npm ci --offline` and
  `@electron/packager` runs offline. The Phase 1 worker stops and reports
  when the lockfile or the cached Electron zip is missing; it never fetches.
- Never touch the shared daemon or port 29417. Launch tests against an
  isolated daemon fixture or a stub endpoint and health server on a free port
  with its own `WS_CACHE_HOME`.
- Never run the whole `./internal/wsdashboard` Go test package; use
  `go test -run`. End every test or app run with a scan for processes the run
  started and stop only those.
- No dashboard or daemon change is expected: the UA token (P15) and the
  daemon HTTP routes (P16) carry the existing coupling. A needed dashboard
  change returns to the lead first.
- Architecture Rule 2: the Electron sources stay inside `agents-plugin-tool/`
  (`agents-plugin-tool/desktop-electron/`, see Decisions).
- Before editing under `agents-plugin-tool/`, read
  `ai-docs/manuals/shipped-surface-boundary.md`.

## Phases

### Phase 1: Electron macOS shell at verified Tauri parity, with X-close confirmation

Build the Electron shell in `agents-plugin-tool/desktop-electron/`,
packaged with `@electron/packager`, meeting every row P1-P21 of the parity
inventory and the Decisions above. Keep the user agent token
`Werkschaft/<version>` so the dashboard's desktop file-link behavior works
unchanged. Package an unsigned macOS `.app` with the same identity and icon
(P19) through an offline, pinned build. Add single-instance launch/focus
((858)=a). Leave the Tauri crate untouched.

Verification, on isolated fixtures only (never port 29417, never the shared
daemon):

- A Playwright `_electron` launch smoke: the window attaches to an isolated
  daemon fixture (or a stub endpoint/health server with its own
  `WS_CACHE_HOME`), loads the dashboard, and shows the `Werkschaft` title;
  a missing or mismatched endpoint shows the connection page with the error;
  Connection → Retry reattaches after the fixture appears.
- X-close confirmation: with a dashboard state that prevents unload (an
  unreferenced pasted image), X-close shows the leave/stay dialog; "stay"
  keeps the window and its draft; "leave" closes it; without that state the
  window closes immediately. The daemon fixture keeps running after close.
- Zoom: `Cmd+0`, `Cmd+=`, `Cmd+Shift+=`, `Cmd+-` walk the P9 ladder and clamp
  at 50 % and 300 %.
- Open with system: the link menu's open action reaches the fixture's
  `fs/act` / `open` route from the Electron window (assert the request, not
  an OS application launch).
- Navigation policy: same-origin navigation stays in-window; an external
  HTTPS link, another local port, `localhost`, and a remote board go to the
  OS opener (stubbed in tests); blocked schemes and userinfo URLs go nowhere;
  `window.open` never creates a second window.
- Security posture: assert `contextIsolation`, `sandbox`, no
  `nodeIntegration`, no preload, and that `window.require`/`process` are
  undefined in the page.
- Single instance: a second launch of the built app focuses the existing
  window and exits without opening another window.
- An offline `.app` build succeeds from the pinned lockfile with no network
  access, and the built app launches for the smoke above.
- Record commands, fixture setup, process scans and any parity row that
  could not be verified.

### Phase 2: Daily-use reliability

Re-homed from `261113-feat-werkschaft-thin-tauri-shell` Phase 2 ((859)=a).
Verify rediscovery and reconnect after a daemon restart, and broader
acceptance in the Electron window: dashboard writes, terminals and live
transports, clipboard, isolated content frames, internal and external
links, and notification fallback. Add safe missing-daemon start only after
establishing a race-safe non-replacement path under the carried Decision 2;
verify it cannot replace or stop an existing daemon and does not bind daemon
lifetime to shell exit. The scoped-grant decision stays independent of this
phase.

### Phase 3: Windows packaged delivery

Re-homed from the Tauri ticket's Phase 3 ((859)=a). Add a Windows build of
the Electron shell and zip delivery with the daemon exe. Verify attach, safe
missing-daemon start, second-launch focus, external links, writes, close
survival and the X-close confirmation on Windows, and report platform
differences explicitly. Signing, installers, an updater, the rename and a
built-in agent remain separate later work; carried Decision 4's signing
boundary holds.

### Phase 4: Scoped grant only when new shell-only HTTP authority is needed

Re-homed unchanged in substance from the Tauri ticket's Phase 4
((859)=a). Independent of platform delivery and gated on an actual new
authority need; no ceremonial bootstrap key for today's browser surface.
Apply the carried Decision 8 and that ticket's grant constraints before any
daemon registration or bootstrap contract, and keep the endpoint credential
native and separate from the grant. Verification when activated: daemon
recognition of only the scoped authority, authenticated bootstrap and
rediscovery after restart, bounded expiry and exit/crash revocation, an
exact trusted-window ACL, rejection of external and preview/opaque-frame
access, and absence from URLs, persistent storage, logs and general
`postMessage`; `/mcp`, lifecycle, master credentials and generic native
commands/filesystem stay excluded, and existing browser guards, sanitizer
and capability roots still hold. Document reload/re-bootstrap and any
fetch/stream transport implications; do not assume browser WebSocket or
EventSource accept bearer headers.

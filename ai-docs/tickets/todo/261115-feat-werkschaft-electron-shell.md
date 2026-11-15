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
| P18 | No single-instance handling (Tauri Phase 2 scope) | `desktop/src/main.rs:100-101` | See (848) |
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
  Windows and Linux are out of scope for this ticket.
- **Compatibility: not a cross-version surface.** The desktop shell adds no
  MCP tool or argument, `/mcp` hello change, shared store shape, tracker
  event or config/rsrc manifest field; it consumes the existing endpoint file
  and HTTP routes read-only. No tier applies (T0 if one must be named).

## Constraints

- **Offline build environment.** Builds and tests run with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` and npm offline
  (`npx --offline`, `npm ci --offline`). Nothing in this ticket fetches from
  the network silently; the one-time Electron download is settled by (844).
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
  (placement in (845)).
- Before editing under `agents-plugin-tool/`, read
  `ai-docs/manuals/shipped-surface-boundary.md`.

## Open Decision Queue

Last ID used: (849)

(844) [open] [policy] How the one-time Electron download is fetched, pinned and cached.
- Context: no Electron binary or npm package exists on this machine (the npx
  cache holds only `@electron` helpers and `electron-to-chromium`). Electron
  and the packager need one networked install; afterwards builds are offline.
- Alternative a: pin an exact current stable `electron` and
  `@electron/packager` version in `package.json` / `package-lock.json`; the
  owner (or MAIN on the owner's go) runs one networked `npm ci` in the
  Electron directory; the binary lands in Electron's default cache
  (`~/Library/Caches/electron`) and the npm cache, so later
  `npm ci --offline` and builds need no network.
- Alternative b: the owner downloads the release zip and its SHASUMS manually
  and places them in a repo-external `ELECTRON_CACHE` directory; npm installs
  with `ELECTRON_SKIP_BINARY_DOWNLOAD` and the packager reads the cache.
- Alternative c: vendor the binary into the repository. Rejected in advance:
  about 100 MB of binary history per version.
> Recommend a: the fewest manual steps, the lockfile records the exact
> version and integrity hash, and Electron's own cache keeps later builds
> offline. The worker does not fetch; it stops if the binary is absent.

(845) [technical] [open] A new sibling directory `agents-plugin-tool/desktop-electron/` over sharing `agents-plugin-tool/desktop/` - the Tauri crate stays in place until parity, and separate directories keep two independent `package.json` / lockfiles and build outputs apart; removal of the Tauri directory later is a clean delete.

(846) [technical] [open] Plain `@electron/packager` over `electron-builder` or Electron Forge - an unsigned `.app` needs only packaging; packager has the smallest dependency tree to fetch once and keep offline, while builder/Forge add installers, publishers and auto-update machinery this ticket excludes.

(847) [technical] [open] Main-process TypeScript-free plain JavaScript (CommonJS) over a TypeScript build step - the shell is a few hundred lines; a compile step adds an offline toolchain dependency for no runtime benefit.

(848) [open] [policy] Whether single-instance launch/focus lands in this ticket.
- Context: the Tauri shell has none (P18); it is approved scope deferred to
  that ticket's Phase 2. Electron offers it as one call
  (`app.requestSingleInstanceLock` plus focusing the existing window).
- Alternative a: include it in Phase 1 (a second launch focuses the
  existing window).
- Alternative b: strict parity now; leave it to the reliability phase in
  (849).
> Recommend a: it is already approved scope, costs a few lines, and avoids
> two Werkschaft windows racing the same endpoint.

(849) [open] [policy] Where the Tauri ticket's unimplemented Phases 2-4 go.
- Context: `261113-feat-werkschaft-thin-tauri-shell` still lists Phase 2
  (daily-use reliability, single instance, reconnect after restart, safe
  missing-daemon start), Phase 3 (Windows/WebView2 and zip delivery) and
  Phase 4 (scoped grant), all phrased for Tauri/WKWebView/WebView2.
- Alternative a: re-home them as later phases of this ticket, rewritten for
  Electron, and close the Tauri ticket once Electron parity is verified (its
  Phase 1 Result stays as history).
- Alternative b: leave them on the Tauri ticket untouched; decide later.
- Alternative c: drop them; capture fresh tickets when needed.
> Recommend a: the requirements (reconnect, safe start, grant limits)
> survive the technology change, and one ticket keeps the shell's contract
> in one place.

## Phases

### Phase 1: Electron macOS shell at verified Tauri parity, with X-close confirmation

Build the Electron shell in the directory settled by (845) with the
packaging settled by (846), meeting every row P1-P21 of the parity
inventory and the Decisions above. Keep the user agent token
`Werkschaft/<version>` so the dashboard's desktop file-link behavior works
unchanged. Package an unsigned macOS `.app` with the same identity and icon
(P19) through an offline, pinned build. Leave the Tauri crate untouched.

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
- An offline `.app` build succeeds from the pinned lockfile with no network
  access, and the built app launches for the smoke above.
- Record commands, fixture setup, process scans and any parity row that
  could not be verified.

---
title: "Migrate the Werkschaft desktop shell from Tauri to Electron"
related:
  261113-feat-werkschaft-thin-tauri-shell: "current Tauri shell (Phase 1 Result e7121971); parity source; its binding decisions carry over except the webview technology"
  261112-research-ws-ade-desktop-shell-and-rename: "Werkschaft concept; Electron was the recorded fallback if the webview matrix proved painful"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1c15439c2a1a2b7d
sage-review-completeness-reviewed: 1c15439c2a1a2b7d
---

# Migrate the Werkschaft desktop shell from Tauri to Electron

## Background

Owner decision (843)=b, relayed by MAIN on 2026-11-15 at 00:59:09Z: migrate
the Werkschaft desktop shell from Tauri to Electron.

Owner-approved rationale: Tauri on macOS renders through WKWebView, so every
browser default the dashboard relies on has to be re-implemented natively:
this shell already re-implements zoom keys and external link opening, and it
has no close-request handler, so the dashboard's `beforeunload` guard never
runs on X-close (`agents-plugin-tool/desktop/src/main.rs:38-45,96-202`). The
dashboard is built and browser-tested on Chromium (Playwright harnesses such
as `agents-plugin-tool/internal/wsdashboard/testdata/files-explorer.browser.cjs`).
Electron embeds Chromium, which is expected to remove the "works in the
browser, not in the app" class of defects; Phase 1 verification, not this
expectation, establishes parity. The Tauri ticket recorded Electron as the
fallback if the webview matrix proved painful
(`261113-feat-werkschaft-thin-tauri-shell` Decision 1).

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
| P4 | Credential-free attach: endpoint bytes are read into native memory, but the token is discarded during deserialization and never exposed to the page; no daemon start/lifecycle call; endpoint file never written (`agents-plugin-tool/desktop/src/endpoint.rs#L47-L60`) | `desktop/src/endpoint.rs:14-19,47-100`, `desktop/README.md:123-130` | Same |
| P5 | Titles: `Werkschaft — Connecting…`, `Werkschaft` on attach; on discovery failure the static connection page is shown and the window title is `Werkschaft — <message> — ⌘R to retry`; immediate navigation error sets `Werkschaft — Navigation failed — ⌘R to retry` (`agents-plugin-tool/desktop/src/main.rs#L48-L85`, `agents-plugin-tool/desktop/ui/index.html`) | `desktop/src/main.rs:48-85,158-159` | Same strings and states |
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
  exactly `http://127.0.0.1:<port>/`, a single instance, external links to
  the system browser, closing leaves the daemon running, no self-update);
  Decision 2 (daemon ownership: the shell attaches to whatever daemon is
  running, starts its beside-exe daemon only when none is, and never
  replaces or stops a running daemon); Decision 3 (attach contract: read
  `<cache root>/dashboard/endpoint.json` respecting the cache-root override,
  check `GET /health`, never hand the endpoint token to page JavaScript; the
  first slice only attaches and never invokes `ws-mcp daemon-start`, because
  that command runs `Ensure` and can replace a daemon, and a missing or
  unhealthy daemon shows the retry view); Decision 4 (unsigned local dogfood
  `.app`; signing and notarization are separate work); Decision 7 (no new UI
  credential or daemon API merely to render); Decision 8 (a scoped grant
  only for a later, justified authority need, under that ticket's
  constraints). Here Phase 1 is that attach-only slice; Phase 2's safe
  missing-daemon start is Decision 2's "starts only when none is" step and
  still never uses `ws-mcp daemon-start` or any path that can replace or
  stop a running daemon.
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
  event or config/rsrc manifest field; it reads the existing endpoint file
  and reuses existing HTTP routes, including the state-changing POST `open`
  and `fs/act` routes (`agents-plugin-tool/internal/wsdashboard/sysopen_route.go#L12-L23`,
  `agents-plugin-tool/internal/wsdashboard/fs_act_route.go#L17-L34`).
  No tier applies to Phases 1-3 (T0 if one must be named). Phase 4 would add
  a scoped-grant contract; its compatibility tier is settled when that phase
  activates, before implementation.

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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261113-feat-werkschaft-thin-tauri-shell (2026-11-15, commit 78bbc60e): "Both need a native new-window/subframe policy decision." — bearing: constrains
- 44edb6f0 (2026-11-15, commit): "The remote dashboard deliberately holds no native permissions (a test pins this)" — bearing: constrains
- 1612e099 (2026-11-15, commit): "Owner request: generalize 'open in system browser' to the OS opener after a .app path failed (old item opened the file-viewer URL, which 404s for dirs)." — bearing: supports
- 0f16157b (2026-11-15, commit): "The runner now probes immediately after each foreground step, permanently retires confirmed-absent groups, and cleans up only active unretired groups" — bearing: constrains
- f88b60bf (2026-11-15, commit): "Existing wsstate CacheRoot and wsdashboard Health are the contract: respect WS_CACHE_HOME, read only endpoint port/PID, validate ws-dashboard plus matching PID" — bearing: constrains
- 261112-research-ws-ade-desktop-shell-and-rename (2026-11-12, Confirmed Decisions): "(496) announced, owner ok 2026-11-13: the product name is Werkschaft, superseding `ws-ade`." — bearing: constrains
- 261110-research-ws-dashboard-agent-host-foundations (2026-11-10, Confirmed Decisions): "Superseded by (493)=b and MAIN's 2026-11-14 start-now direction" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | New agents-plugin-tool/desktop-electron/ package, package.json, package-lock.json, main.js and packaging/smoke inputs; existing agents-plugin-tool/desktop/ retained. |
| scope.surface | public-interface | User-facing desktop window, menus, close confirmation and packaged macOS/Windows delivery; Phase 4 conditionally adds shell-only HTTP authority. |
| scope.new_public_symbol | unknown | Phase 1 names no exported daemon/tool symbol; gated Phase 4 has no concrete registration/bootstrap API or symbol names yet. |
| scope.new_type_contract | yes | Phase 4 explicitly requires a new scoped-grant registration/bootstrap contract when activated; Phase 1 reuses endpoint port/PID and health service/PID (agents-plugin-tool/desktop/src/endpoint.rs#L14-L25). |
| scope.test_surface | new-files | Phase 1 requires a new Playwright _electron smoke; agents-plugin-tool/desktop-electron/ is absent. Existing UA coupling tests: agents-plugin-tool/internal/wsdashboard/desktop_link_test.go. |
| complexity.reuse_points | confirmed | Read agents-plugin-tool/desktop/src/endpoint.rs, src/navigation.rs, src/zoom.rs and src/main.rs; dashboard.js desktopShell/openLinkMenu, doc-frame.js, and fs_act_route.go implement the carried attach, navigation, zoom and HTTP opening behavior. |
| complexity.side_effect_risk | high | Phase 1 opens native windows/external URLs; Phase 2 adds daemon startup and Phase 4 conditionally issues native-scoped authority, with non-replacement and revocation obligations. |
| risk.correctness | high | Close cancellation must preserve image drafts; attach must reject stale endpoints; later startup must not replace a live daemon (Phases 1-2). |
| risk.fit | moderate | Independent Electron package must preserve the existing daemon-served frontend, UA coupling and zero-native-authority Phase 1 while retaining Tauri sources (Decisions, P15-P21). |
| risk.test | high | No Electron package, lockfile or smoke exists yet; offline reinstall/build and actual macOS/Windows window behavior remain to be demonstrated (Prerequisite, Phases 1 and 3). |
| risk.security_or_contract | high | Untrusted frame/navigation isolation, OS opening, credential exclusion and the later scoped grant are explicit boundaries (P10-P14, P21, Phase 4; agents-plugin-tool/desktop/src/navigation.rs#L16-L38). |

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

#### Incomplete implementation checkpoint - 2026-11-15

Phase 1 is **not complete** and has no Result. Sources, behavioral unit tests,
an offline packaging script and a built-app smoke now live in
`agents-plugin-tool/desktop-electron/`; Tauri and dashboard sources are unchanged.
The source-built test daemon is `test/daemon/main.go`, using the real dashboard
assets and daemon implementation, an OS-assigned port and its own disposable
`WS_CACHE_HOME`. It never invokes Ensure or a shared daemon.

Verification recorded on the implementation branch:

- `npm ci --offline --no-audit --no-fund`, then
  `node node_modules/electron/install.js`: succeeded using the prepared cache;
  `electron --version`: `v44.11.3`.
- `npm test`: 14 tests passed, including endpoint bounds/identity/deadline,
  native navigation and redirect hooks, initial-frame request filtering,
  renderer preferences, leave/stay semantics, retry overlap and single instance.
- `npm run build`: succeeded from `electronZipDir` (download code bypassed),
  producing `dist/Werkschaft-darwin-arm64/Werkschaft.app`. Identity is
  `dev.werkschaft.desktop`; the copied temporary icon is packaged as `electron.icns`.
  Packager warns only that the optional newer `.icon` format is absent.
- `WS_PLAYWRIGHT=<cached-playwright> npm run smoke`: incomplete. The built app
  verified missing/mismatched endpoint states, successful retry to the isolated
  source-built daemon, title, dimensions, UA, renderer isolation, credential
  exclusion, menu callbacks and zoom clamps. It stops at `test/smoke.cjs`'s
  remote-frame assertion (`true !== false`): native frame URL metadata is not
  yet a demonstrated assertion of committed remote content. No security-policy
  relaxation was made to get past the assertion. Earlier opaque-frame probing
  established that Playwright reports an empty URL for a real native
  `about:srcdoc` frame. The subsequent diagnostic probe itself failed because
  `require` is unavailable inside Playwright's serialized main-process evaluate;
  no further repair was attempted under the non-convergence stop rule (f).
- Earlier smoke-only fixes: wait for `did-finish-load` before the next Retry
  (overlapping requests are intentionally ignored); pass the popup URL as an
  explicit serialized evaluate argument, not a closure variable.
- Physical Cmd zoom shortcut delivery is unverified. CDP keyboard events did
  not reach Electron's native before-input/menu dispatch; `sendInputEvent`
  reached before-input but not AppKit menus. Tests assert actual accelerator
  registrations and menu callbacks instead, without adding a keyboard polyfill.
- Offline `go test ./internal/wsi18n` and selected dashboard tests
  `-run 'TestDesktopPathClickOpensMenuAndModifiedClickPreviews|TestDocFrameDesktopModifiedClickGoesToHost|TestOutsidePreview'`
  reproduced only the lead-declared base failures: the two fullscreen-width
  keys missing in nine language tables and
  `TestOutsidePreviewCardRemintsOnEveryLoad`'s `removeAttribute` TypeError.
  They were left unchanged.
- Each app/daemon probe stopped only its own children. Smoke evidence and final
  empty owned-process scans are retained under ignored `dist/evidence/`.

Remaining acceptance: resolve the remote-frame assertion against actual
committed content/request cancellation, then complete the real dashboard
open-with-system, pasted-image X-close stay/leave, built second-launch focus,
normal close and daemon-survival smoke sections. Those assertions are authored
but have not yet been reached. Native physical keyboard delivery also remains
unverified. Independent review was explicitly deferred to the lead because
child-agent tools are disabled for this run. Phases 2-4 are untouched.

### Result (5beffc47a) - 2026-11-15

Phase 1's attach-only macOS shell and built-app acceptance landed. The shell
implementation is checkpoint `4592f07d1`; `5beffc47a` completes its smoke
contracts without changing the runtime policy. Phases 2-4 remain unimplemented;
the ticket stays in `ready/`. Tauri and dashboard/daemon sources are unchanged.
Independent review is deferred to the lead under this run's explicit
no-child-agent adaptation.

Verification (Go commands use `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`):

- In `agents-plugin-tool/desktop-electron/`, `npm ci --offline --no-audit
  --no-fund && node node_modules/electron/install.js` succeeded from cache;
  `node_modules/.bin/electron --version` reports `v44.11.3`. `npm test`
  passed all 14 tests. `npm run build` produced the unsigned local
  `dist/Werkschaft-darwin-arm64/Werkschaft.app` using the pinned cached zip,
  with no download path. The existing optional `.icon` format warning remains;
  the packaged `electron.icns` exists. Plist checks confirm `Werkschaft`,
  `dev.werkschaft.desktop`, `0.1.0`; the source PNG matches the retained
  Tauri icon byte-for-byte (P19-P20).
- In `agents-plugin-tool/`, `go build -o desktop-electron/dist/smoke-daemon
  ./desktop-electron/test/daemon` succeeded. The fixture runs real dashboard
  code/assets on an OS-assigned port with a disposable `WS_CACHE_HOME`,
  without Ensure or shared-daemon access.
- `WS_PLAYWRIGHT=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright
  npm run smoke` passed against the newly built app. It verifies missing and
  mismatched endpoint errors, Retry attachment, title, dimensions, UA and
  renderer credential/authority exclusion; actual dashboard file-link menu
  `fs/act` opening (request intercepted before OS launch); pasted-image
  X-close Stay preserving the draft, Leave closing, and normal close without
  a draft; second built launch exiting and restoring/focusing the sole
  window; unchanged endpoint bytes, daemon survival and fresh-process zoom
  reset. Unit tests additionally cover discovery bounds, total deadline,
  endpoint reread, navigation failure, retry overlap and permission denial
  (P1-P8, P15-P18, P21).
- Navigation smoke verifies same-origin popup navigation, denied native
  popups, external HTTPS/other-port/localhost/remote-board handoff, blocked
  schemes and userinfo, real opaque srcdoc content, and remote-frame denial
  both under dashboard CSP and on a test-only same-origin page without CSP
  (P10-P14). A separate listening local target received zero requests.
  Evidence: `dist/evidence/frame-documents.json`.
- Zoom accelerators and actual native menu callbacks, ladder traversal and
  50%/300% clamps pass (P9). **Physical Cmd shortcut delivery remains
  unverified:** `osascript -l JavaScript -e 'ObjC.import("ApplicationServices");
  $.AXIsProcessTrusted()'` returns `false`. No accessibility permission was
  requested; no renderer keyboard polyfill was introduced.
- Whole `go test ./internal/wsi18n` reproduces only the declared base failure:
  `js.card.width.extend` and `js.card.width.normal` missing in de, es, fr, it,
  ja, pt-BR, ru, zh-Hans and zh-Hant. Selected dashboard acceptance via
  `go test ./internal/wsdashboard -run
  'TestDesktopPathClickOpensMenuAndModifiedClickPreviews|TestDocFrameDesktopModifiedClickGoesToHost|TestOutsidePreview|TestSessionPageFramesOnlyItsOwnOrigin|TestSessionWithoutAdapterHasNoComposerAndItsMessageRouteRefuses'
  -count=1` reproduces only the declared base
  `TestOutsidePreviewCardRemintsOnEveryLoad` `removeAttribute` TypeError.
  Neither base defect was changed or worked around.
- Logs, fixture metadata, draft capture and process scans are in ignored
  `agents-plugin-tool/desktop-electron/dist/evidence/` (`acceptance.log`,
  `summary.json`, `wsi18n.log`, `dashboard-neighbours.log`,
  `electron-dashboard-draft.png`, `process-scans.jsonl`). Successful smoke
  cleanup ends with zero live owned processes. One earlier Playwright CDP
  dialog race crashed the runner; its sole remaining isolated daemon was
  identified by recorded PID/start time, stopped, and scanned absent.

Decisions from verification:

- Assert committed document and target-request outcomes, not
  `WebFrameMain.url`: CSP denial leaves attempted URL metadata but commits
  `chrome-error://chromewebdata/`; native navigation prevention retains
  `about:blank` without a `did-fail-load` event. No security rule was relaxed.
- Exercise the real session iframe because a top-level `?frame=1` page
  deliberately redirects to the dashboard shell. Use the normal English
  preference cookie to make menu locators independent of the machine locale.
- Let Electron's main process own beforeunload. A Playwright dialog listener
  suppresses its default CDP auto-accept, which otherwise races the already
  resolved native Stay/Leave decision. App-exit assertions are bounded.

#### Edition (c0bf0ef53) - 2026-11-15

Owner directive (relayed by MAIN, 2026-11-15): every Electron run and visual
check runs on the Windows ThinkPad host, never on the owner's Mac (Electron
windows steal focus there). Headless steps that open no window (unit tests,
the macOS package build) may stay on the Mac. MAIN technical call, announced
to the owner: Phase 1's smoke/visual acceptance reruns on the Windows host,
which pulls forward only a win32 package step and a platform switch in the
smoke harness; Phase 3 keeps zip delivery, missing-daemon start, external
links and the platform report.

- `scripts/build.cjs` gains a win32-x64 path (`@electron/packager`
  `platform: 'win32'`; an `.ico` derived from the retained PNG, or no
  custom icon with that recorded); the darwin path is unchanged.
- `test/smoke.cjs` switches the built executable and the fixture daemon
  name (`.exe`) by platform and keeps every assertion; a Windows-only delta
  (Ctrl accelerators, the window menu bar, dialog shape, second-instance
  focus) is reported, not papered over.
- Host run (approved by MAIN): git-sync the branch into
  `C:\Users\ki608\devenv`; one networked `npm ci` in that clone (may
  download `electron-win32-x64` 44.11.3), no global installs; the smoke runs
  in the logged-in console session via `schtasks /create ... /it /ru ki608`
  then `/run` (a GUI started straight from SSH lands in session 0 and shows
  nothing), and screenshots come from that session; a process scan on the
  host at the end, killing only what the run launched. Playwright comes from
  a copied module directory (no networked install).
- P9 physical zoom keys stay recorded as unverified unless a cheap Windows
  check delivers them.
- Never launch Electron, the smoke, or the built app on the Mac.

##### Windows rerun checkpoint (01eabf885) - 2026-11-15

The platform changes landed, but Windows acceptance has **not run**. No
Electron executable, built app or smoke was launched on the Mac. No later
phase, shell runtime policy, dashboard/daemon source, dependency pin or
protocol/API was changed. The ticket remains in `ready/`.

- `scripts/build.cjs` supports offline win32-x64 packaging from the normal
  local Electron cache. Windows uses Electron's default icon (the Edition's
  allowed no-custom-icon option); darwin's PNG-to-icns path is unchanged.
- The smoke selects the Windows app/fixture `.exe` paths and retains every
  existing assertion. A test-only host helper inventories Windows processes
  with CIM PID, parent and creation identity so cleanup retains the original
  owned-process safety. Additional evidence covers native menu metadata,
  close-dialog options and an optional Windows OS-injected Ctrl-key probe.
- Mac headless `npm test`: **18 passed** (the original 14 plus four platform,
  process-identity and offline-packaging contracts). `npm run build`:
  **passed**, producing `dist/Werkschaft-darwin-arm64/Werkschaft.app`
  offline. The pre-existing optional `.icon` format warning remains.
- A git bundle based on the host's `08991c9b9` was copied and fetched into
  the host clone successfully. `git switch
  impl/exec4/electron-phase1/cake-ember-ramp` then refused to overwrite the
  pre-existing `agents-plugin-tool/internal/wsdoc/tickets_scope_test.go`
  modification (+52/-4). The host stays on its original `develop` checkout;
  that work was not stashed, discarded, committed or edited. Stop (b): the
  lead must authorize preserving/restoring it before this checkout proceeds.
- Another prerequisite needs a narrow authorization correction: the host
  has no Windows Electron cache, and the pinned Electron 44.11.3 package has
  **no postinstall script**. Its explicit `node
  node_modules/electron/install.js` downloads the executable; the approved
  single `npm ci --no-audit --no-fund` alone will not do so. No npm install
  or network download was attempted. The lead was asked to authorize that
  one pinned win32-x64 artifact download; no permission was inferred.
- Windows-versus-Mac empirical differences (Ctrl delivery, native menu bar,
  dialog rendering, focus, dashboard screenshot) remain **unverified**.
  The configured Windows default icon differs from the custom macOS icon;
  the unchanged runtime's retry title still says `⌘R` on either platform.
  No platform assertion was removed to hide a difference.
- Final host `tasklist`/CIM scans show no electron, Werkschaft, node or
  smoke-daemon processes. This run launched none, signalled none, and
  created no scheduled task (`exec4-smoke` is absent). The Mac process scan
  shows no owned app/fixture processes. Ignored evidence is under
  `agents-plugin-tool/desktop-electron/dist/evidence/windows/`
  (`summary.json`, `preflight-process-scan.log`); there are no Windows
  screenshots because acceptance has not started. The allowed npm ci
  remains unused, and Playwright copying/daemon build remain pending.
- Independent review is deferred to the lead under the explicit
  no-child-agent adaptation. Resume with the host-file preservation and
  explicit Electron download authorizations, then complete the original
  Windows interactive-session smoke and evidence cleanup.

#### Edition (7525029be) - 2026-11-15

Fixed the independent lite review's Critical cancelled-navigation finding.
Retry and same-origin popup loads now distinguish Electron `ERR_ABORTED`
(`errno: -3`) from a genuine load failure. Choosing Stay retains the current
page and restores the previous pin/title rather than loading
`connection.html`. Retry captures that state before its provisional title
and rediscovered pin, including when discovery moves to another port. Real
load failures retain the existing failure title, cleared pin and connection
page. No renderer authority, protocol/API, dependency or later phase changed.

Verification for this review-fix round only:

- The focused Retry regression failed against the previous implementation:
  Stay incorrectly loaded `connection.html`. After the fix, headless
  `npm test` passed all **21 tests**. New regressions cover Retry Stay,
  same-origin popup Stay, retained page/internal title and origin pin, no
  connection page or external handoff for later same-origin frames, released
  retry lock, and genuine Retry/popup failures after a previous cancellation.
- Offline `npm run build` produced the unsigned darwin-arm64 app. Only the
  pre-existing optional `.icon` format warning remains. `git diff --check`
  passed.
- The final local process scan found no remaining test/build/app/fixture
  processes in this worktree; no processes were signalled. No Electron or
  GUI ran on the Mac, and this round made **no Windows-host access**.
- Windows smoke for this fix rides the next owner-authorized host run. The
  preceding host blockers remain with the owner; this edition does not claim
  live Electron acceptance. This is the requested lite-review fix, not a new
  implementation phase or an additional independent review pass.

#### Edition (eff3ced5b) - 2026-11-15

Fixed the two Critical findings from the final lead delta-review round:

- Navigation cancellation now requires the **observed Stay outcome of the
  still-current load**, not merely `ERR_ABORTED` / `errno: -3`. Retry and
  same-origin popup attempts carry distinct identities; a superseded
  rejection cannot restore a stale origin/title or load the connection page.
  Non-Stay aborts retain the genuine-failure path. An unrelated X-close Stay
  cannot mark a pending navigation as Stay. The preceding Stay preservation
  regressions remain in force.
- Smoke cleanup anchors ownership to the identity captured from each launched
  child (PID, parent, creation time and launch boundary), not a later PID-only
  discovery. Children/grandchildren must be created no earlier than their
  parent and the recorded root. A reused root PID invalidates new adoption.
  Windows creation identities use UTC tick strings to preserve CIM's full
  100 ns precision; macOS retains its existing second-precision `ps`
  identities. Both daemon and remaining-process signals revalidate PID and
  creation identity immediately before each signal. No image-name kill is
  used.

Verification: headless `npm test` passed all **29 tests**, including non-Stay
and Leave aborts, late popup failures after Retry pins origin B, retained
Stay behavior, unrelated close vetoes, stale-parent/PID reuse, creation-order
checks, legitimate grandchildren and identity mismatch between cleanup
signals. Offline `npm run build` produced the unsigned darwin-arm64 app;
only the pre-existing optional `.icon` warning remains. `git diff --check`
passed. The local process scan found no remaining test/build/app/fixture
processes in this worktree; no processes were signalled.

This round did not access Windows or launch Electron/any GUI on the Mac.
Live smoke verification remains deferred to the next owner-authorized Windows
run. No protocol/API, dependency, dashboard/daemon source, frozen phase plan
or later phase changed. This records the final requested review fixes, not a
new phase or an additional independent review pass.

#### Edition (0c9961c6f) - 2026-11-15

Fixed the third review's narrow Critical rollback-state finding. Attempts no
longer snapshot another attempt's provisional pin/title. The retained rollback
state advances only on a matching main-document `did-navigate` commit for the
current load target. Stay restores that committed state, including the last
committed connection-page error title; a late promise settlement cannot
advance it. Runtime changes are limited to rollback-state handling. Cleanup
code and the smoke harness are unchanged.

The required Retry → popup → Stay regression was added **before** editing
`main.js` and confirmed **RED on `0bb29c533`** with only the test file changed:
A's retained frame was cancelled (`{ cancel: true }` instead of `{}`). After
the fix, headless `npm test` passed all **33 tests**, including that regression,
commit-before-load-completion, late superseded success without a commit, and
connection-page title restoration. Unit fixtures now explicitly emit document
commit events rather than treating a pending/resolved load as proof of commit.

Offline `npm run build` passed for darwin-arm64 with only the pre-existing
optional `.icon` warning; `git diff --check` passed. The local process scan
found no remaining test/build/app/fixture processes in this worktree, and no
processes were signalled. No Windows host was accessed and no Electron/GUI ran
on the Mac. Live smoke remains deferred to the owner-authorized Windows run;
no new phase, policy, dependency, API or independent review pass was added.

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
native and separate from the grant. The Electron equivalents of the Tauri
bootstrap command and trusted-window ACL are designed when the phase
activates and recorded as an edition of this phase before implementation.
Verification when activated: daemon
recognition of only the scoped authority, authenticated bootstrap and
rediscovery after restart, bounded expiry and exit/crash revocation, an
exact trusted-window ACL, rejection of external and preview/opaque-frame
access, and absence from URLs, persistent storage, logs and general
`postMessage`; `/mcp`, lifecycle, master credentials and generic native
commands/filesystem stay excluded, and existing browser guards, sanitizer
and capability roots still hold. Document reload/re-bootstrap and any
fetch/stream transport implications; do not assume browser WebSocket or
EventSource accept bearer headers.

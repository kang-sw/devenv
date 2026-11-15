---
title: "Electron shell on Windows: app-drawn VS Code-style title bar"
related:
  261115-feat-werkschaft-electron-shell: prerequisite (Phase 1 shell only per (943)=a; Windows build already supported in agents-plugin-tool/desktop-electron/scripts/build.cjs#L10-L13)
  261115-feat-ws-daemon-release-auto-update: precedent (shell-owned overlay view for the update toast)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6d137ba89e3979fe
sage-review-completeness-reviewed: 6d137ba89e3979fe
---

# Electron shell on Windows: app-drawn VS Code-style title bar

## Background

Owner requirement (relayed by MAIN, 2026-11-15), verbatim: "electron쪽은
windows 쪽에서는 os native 윈도우 타이틀 바 대신 vscode 스타일로 우리 쪽에서
만들었으면 싶긴 하네요." (On Windows, the Electron shell should replace the
OS-native window title bar with a VS Code-style bar we draw ourselves.)

The Electron shell (`261115-feat-werkschaft-electron-shell`) is one
`BrowserWindow` navigated to the daemon-served dashboard, with a bundled
connection page as its only shell-owned HTML (parity row P1) and the
standard menus plus Connection and View additions (P8). Windows delivery is
that ticket's Phase 3. On Windows a hidden title bar also hides the native
menu bar, so the P8 menus need a home in the new bar.

Useful existing pieces:
- Electron `titleBarStyle: 'hidden'` with `titleBarOverlay` keeps the
  Windows-drawn minimize/maximize/close controls (including the Windows 11
  snap-layout flyout on maximize hover) and lets the app draw the rest of
  the bar; the overlay's `color`/`symbolColor` can be updated at runtime
  (`win.setTitleBarOverlay`).
- The dashboard already keeps `meta[name="theme-color"]` in step with its
  own theme setting for installed-app title bars
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L463-L468`);
  Electron reports it through `webContents` `did-change-theme-color`.
- The auto-update ticket already settled a shell-owned overlay view with a
  narrow preload for its toast (Decisions → Toast surface; implementation is
  Phase 2 in `ai-docs/tickets/todo/261115-feat-ws-daemon-release-auto-update.md`),
  the precedent for shell-drawn UI beside the remote dashboard
  (pending 261115-feat-ws-daemon-release-auto-update).

## Decisions

- **(883)=a, owner: the bar shows the app icon, a compact menu and a
  centered title, with no search box.** The compact menu is the P8 menus
  opened from a hamburger button and by Alt (VS Code compact mode); the
  title is the page's `document.title` (session/board name). Rejected:
  full inline menu labels (b); a centered search box opening the dashboard
  palette (c), which duplicates existing palette entry points and needs a
  shell-to-page command channel.
- **Separate follow-on ticket after the Electron ticket's Phase 3**
  (announced, accepted by MAIN): the Phase 1 worker was running on that
  ticket, and the bar needs a Windows build. Rejected: a new phase inside
  `261115-feat-werkschaft-electron-shell`.
- **macOS and Linux keep the native title bar** (announced, accepted): the
  owner's words scope the change to Windows.
- **Window controls stay Windows-drawn** through `titleBarStyle: 'hidden'`
  + `titleBarOverlay` (announced, accepted), keeping the Windows 11
  snap-layout flyout on maximize hover, as VS Code does.
- **The bar is shell-owned** (technical, accepted): a bundled bar page in
  its own view above the dashboard view, with a narrow preload (title
  updates in, menu commands out). Rejected: the dashboard drawing it through
  Window Controls Overlay CSS (needs a dashboard change and puts shell UI
  inside remote content). Follows the auto-update toast precedent.
- **Colors follow the dashboard's `theme-color`** (technical, accepted) via
  `did-change-theme-color`, also applied to `setTitleBarOverlay`, falling
  back to the OS scheme (`nativeTheme`) until the page reports one.
  Rejected: OS scheme only (mismatches the dashboard's own light/dark
  setting).
- **(943)=a, owner (relayed by MAIN, 2026-11-15): start the bar now**,
  queued after the whiteboard deps UI. A Windows build of the shell already
  exists (the owner-check copy on the Windows host, built from develop), so
  Phase 1 no longer waits on the Electron ticket's Phase 3 landing.
  Rejected: holding until Phase 3 closes.
- **macOS and Linux are verified by unit tests only** (announced: a direct
  consequence of the owner directive that Electron never launches on the
  owner's Mac). The platform-switch unit test proves those platforms keep
  the native title bar. Rejected: a macOS GUI smoke run.
- **Dashboard view ownership on Windows** (technical, EXEC4): the
  dashboard loads in its own sibling view below the bar view, and every
  existing per-webContents behavior binds to the dashboard's webContents
  through one accessor, unit-tested on both layouts: title handling, the
  beforeunload/close veto, the navigation guard, zoom, and theme-color. The
  dashboard keeps its isolation unchanged (contextIsolation, sandbox,
  no preload); the narrow preload belongs to the local bar view only, and
  the bar view loads only the bundled bar page. Rejected: overlaying the bar
  on the window's own dashboard webContents (it covers dashboard content
  and would need a dashboard inset change, which the Constraints bar).

## Constraints

- Windows only. No change to the macOS or Linux window chrome.
- No dashboard asset change is planned; if the implementation finds one
  necessary, that is a stop for a ticket edition, not a silent addition.
- The drag region covers the empty bar area only; menu, title and buttons
  are not draggable. The window stays resizable from its edges, and the
  maximize/restore double-click on the bar keeps working.
- Every Electron run and visual check runs on the Windows host
  (`ssh ki608@192.168.33.6`, recorded as a clone note), never on the
  owner's Mac (owner directive, 2026-11-15: Electron windows steal focus
  there). Launch GUI runs in the logged-in console session through
  `schtasks ... /it /ru ki608` (a GUI started straight from SSH lands in
  session 0 and shows nothing) and take screenshots from that session.
  Sync source by git into the host clone; no edits outside that tree, no
  global installs or system settings changes, and clean up only what the
  run placed.
- Owner run rules apply (offline env, isolated daemons with their own
  `WS_CACHE_HOME` on free ports, never the shared dashboard daemon or port
  29417, owned process scan at the end).
- Not a cross-version surface (shell chrome only).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261115-feat-werkschaft-electron-shell (2026-11-15, Decisions): "The renderer runs with `contextIsolation: true`, `nodeIntegration: false`, `sandbox: true`, and no preload script" — bearing: constrains
- 261115-feat-ws-daemon-release-auto-update (2026-11-15, Decisions): "The toast is a small shell-owned overlay view (bottom-right of the window, local packaged content only, never remote content)" — bearing: supports
- 0bc7fcf2 (2026-11-15, commit): "Reuse theme messages; language reload now also checks cached frames for drafts. Preferences are available before selecting a session." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/desktop-electron/: main.js, bundled bar HTML and narrow preload, tests and packaging inputs; scripts/build.cjs#L25-L27 currently copies only four runtime files. |
| scope.surface | public-interface | Windows window chrome, native controls and P8 menu/keyboard access are user-facing; dashboard assets stay unchanged. |
| scope.new_public_symbol | no | No new daemon, MCP or dashboard export is planned; the new preload is confined to the bundled shell-owned bar. |
| scope.new_type_contract | yes | New local preload contract carries title updates into the bar and menu commands out; exact channel and symbol names are not yet specified (Decisions). |
| scope.test_surface | existing | agents-plugin-tool/desktop-electron/test/main.test.js already mocks BrowserWindow, menus, renderer isolation and close veto; test/smoke.cjs exercises the packaged shell. |
| complexity.reuse_points | confirmed | Read main.js#L81-L95 and #L148-L166 for close/menu/zoom behavior, connection.html and test/main.test.js; dashboard.js#L455-L468 supplies theme-color updates. The toast overlay is only a design precedent, not landed code. |
| complexity.side_effect_risk | high | Adding a dashboard view and privileged local bar changes content ownership, keyboard routing and window-close handling around the existing draft-preservation guard. |
| risk.correctness | high | main.js#L29-L40 binds title handling to window.webContents, #L81-L95 handles unload there, and #L56-L63 allows only connection.html as a local main-frame document; new views must preserve these behaviors deliberately. |
| risk.fit | moderate | The local bar/preload must fit the one-window shell without changing dashboard assets or macOS/Linux chrome; no reusable overlay implementation exists in desktop-electron/. |
| risk.test | high | Existing unit fixtures mock Electron; Windows snap hover, drag/double-click, Alt/menu focus, theme repaint and close behavior need the specified Windows-host acceptance, not a Mac GUI run. |
| risk.security_or_contract | high | A new local preload/menu authority boundary must remain inaccessible to the daemon-served dashboard; current main.js#L23-L28 and test/main.test.js#L54-L62 explicitly enforce its no-preload isolation. |

## Phases

### Phase 1: Windows app-drawn title bar

Implement the settled decisions in `agents-plugin-tool/desktop-electron/`.

Verification:
- Unit tests for the platform switch (Windows gets the hidden title bar with
  the overlay; macOS/Linux construct the window as before) and for the
  theme-color mapping, including the fallback before the page reports a
  color.
- On the Windows smoke host: the native title bar is gone and the app bar
  shows the decided content; dragging moves the window; double-click
  maximizes and restores; the Windows-drawn controls work and the snap
  layout flyout appears on maximize hover (Windows 11); every P8 menu
  action is reachable from the bar and by keyboard; switching the
  dashboard theme recolors the bar and the overlay controls; the X-close
  confirmation from the Electron ticket still runs. Screenshots are kept as
  evidence outside the repository.

### Result (c8609cb1b) - 2026-11-15

Implementation landed; **Windows interactive acceptance remains incomplete**.
This is an implementation checkpoint, not a claim that Phase 1 passed its
live verification. The ticket remains in `ready/` at the caller's direction.

Delta:
- `919520109` adds Windows-only hidden chrome and Windows-drawn overlay
  controls, a bundled bar view with icon/hamburger/centered document title,
  and a sibling dashboard view below it. Native macOS/Linux window options
  remain unchanged. Empty bar space is draggable; icon/menu/title are not.
- Dashboard navigation, title, zoom, theme and unload handling bind through
  `dashboardContents`. Closing the Windows window explicitly runs the
  dashboard view's beforeunload path before completing native window close.
- The bar alone receives a narrow state/menu preload, in a separate
  in-memory session restricted to bundled resources. Menu requests verify
  sender/main-frame identity. The dashboard retains isolation, sandbox,
  disabled Node/webview and no preload. No dashboard assets changed.
- Dashboard theme-color maps to bar/overlay colors with contrast selection
  and nativeTheme fallback. Hamburger and bare Alt share the native P8 menu.
  Packaging now includes every bar runtime file and its icon.
- `c8609cb1b` is the single authorized fix round: explicitly initialize the
  inert BrowserWindow owner document with about:blank, deny its navigation
  and popups, and keep its title separate from the dashboard. It also fixes
  acceptance input targeting to use Electron's PID rather than its cmd
  wrapper. The owner-document diagnosis is not yet confirmed by live success.

Verification:
- Mac: `npm test` passed 39/39; offline `npm run build` passed for
  darwin-arm64, retaining only the existing optional `.icon` warning.
  `node --check test/titlebar-smoke.cjs` and `git diff --check` passed.
  No Electron, built app, smoke script or GUI ran on the Mac.
- Windows: one isolated clone, one `npm ci --no-audit --no-fund`, and one
  explicit `node node_modules/electron/install.js`; caches stayed under the
  unique verification folder. On both heads, `npm test` passed 39/39 and
  offline win32-x64 packaging passed. The source-built fixture used its own
  cache and an OS-assigned port, never the shared daemon or port 29417.
- First console-session run on `919520109`: Playwright timed out at 60
  seconds after attaching Node and Chrome debuggers; no UI assertion
  completed. Its source waits for every attached page to initialize, while
  the new owner webContents had no committed document; the fix above
  addresses that lifecycle gap.
- The one authorized rerun on `c8609cb1b` did not reach the runner:
  Task Scheduler returned `0xC0000142` (3221225794) starting the command.
  No second smoke log, Electron process or screenshot was produced.
  No further attempt was made.
- Evidence remains ignored under
  `agents-plugin-tool/desktop-electron/dist/evidence/windows/tb/` (summary,
  initial launch/build logs, scheduler result and owned-process scans).
  The exact scheduled task and unique Windows verification folder were
  deleted after an empty scoped CIM scan; the Mac bundle was deleted.
  The final Mac owned executable scan was also empty.

Unverified: visible chrome and centering, live title propagation,
menu/Alt/keyboard reachability and individual P8 actions, drag/double-click,
Windows controls and snap hover, live theme repaint, physical X-close
confirmation and real pasted-image draft preservation. No interactive
screenshots exist. The owner-copy rebuild/shortcut update was skipped because
its prerequisite core checks did not pass; that folder was not accessed.

Decisions: retain the caller's explicit `exec4/titlebar-impl` branch rather
than the route's nested branch suggestion; skip child delegation and independent
review under the caller's override (the lead owns one lite review). Do not
retry beyond the single authorized fix round. A fresh Windows acceptance run
requires renewed lead authorization; this checkpoint does not close the phase.

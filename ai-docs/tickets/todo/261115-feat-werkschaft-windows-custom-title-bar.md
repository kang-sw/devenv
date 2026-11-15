---
title: "Electron shell on Windows: app-drawn VS Code-style title bar"
related:
  261115-feat-werkschaft-electron-shell: prerequisite (Phase 1 macOS shell; Phase 3 Windows packaged delivery)
  261115-feat-ws-daemon-release-auto-update: precedent (shell-owned overlay view for the update toast)
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
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:455-461`);
  Electron reports it through `webContents` `did-change-theme-color`.
- The auto-update ticket already settled a shell-owned overlay view with a
  narrow preload for its toast (T3), the precedent for shell-drawn UI beside
  the remote dashboard.

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

## Phases

### Phase 1: Windows app-drawn title bar

Blocked until the Electron shell ticket's Phase 3 lands a Windows build.
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
  evidence.
- macOS smoke: the window still has the native title bar and its menus.

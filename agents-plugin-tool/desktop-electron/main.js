'use strict';
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { connectDaemon } = require('./daemon-controller');
const { nextZoom, destination } = require('./policy');
const { HEIGHT, colors, createBar, trackDashboardTheme } = require('./titlebar');
const { createUpdater } = require('./update');
const UPDATE = Object.freeze({ check: 'werkschaft:update-check', download: 'werkschaft:update-download',
  skip: 'werkschaft:update-skip', restart: 'werkschaft:update-restart', subscribe: 'werkschaft:update-subscribe', status: 'werkschaft:update-status' });

function start({ app, BaseWindow, BrowserWindow, WebContentsView, ipcMain, nativeTheme, Menu, shell, dialog, session }, platform = process.platform) {
  app.setName('Werkschaft');
  if (!app.requestSingleInstanceLock()) { app.quit(); return; }
  let window, pin = null, connecting = false, title = 'Werkschaft — Connecting…';
  let zoom = 1;
  const focus = () => {
    if (!window || window.isDestroyed()) return;
    if (window.isMinimized()) window.restore();
    window.show();
    window.focus();
  };
  app.on('second-instance', focus);
  app.on('activate', focus);
  app.on('window-all-closed', () => app.quit());

  app.whenReady().then(() => {
    // The update status waits here until the attached dashboard subscribes.
    let updatePending = null, updateTarget = null;
    const sendUpdate = status => {
      updatePending = status;
      if (updateTarget && !updateTarget.isDestroyed()) { updateTarget.send(UPDATE.status, status); updatePending = null; }
    };
    const updater = createUpdater({ version: app.getVersion(), packaged: app.isPackaged,
      userData: app.getPath('userData'), platform, send: sendUpdate });
    // A stage left by a reboot or unclean exit: swap it in and relaunch.
    try { if (updater.applyOnLaunch()) { app.quit(); return; } }
    catch (error) { console.error(`Update apply at launch failed: ${error.message}`); }
    // "Restart now" quits through the normal path (the page may still veto)
    // and relaunches; a plain full quit applies without relaunching. Neither
    // path shuts down the independently started or attached daemon.
    let relaunch = false;
    app.on('will-quit', () => {
      try { updater.applyOnQuit(relaunch); } catch (error) { console.error(`Update apply at quit failed: ${error.message}`); }
    });
    // The dashboard's only preload is the update bridge (dashboard-preload.js);
    // its IPC is accepted from the pinned dashboard origin only. No permissions.
    session.defaultSession.setPermissionRequestHandler((_wc, _permission, callback) => callback(false));
    session.defaultSession.setPermissionCheckHandler(() => false);
    const Window = platform === 'win32' ? BaseWindow : BrowserWindow;
    const dashboardPreload = path.join(__dirname, 'dashboard-preload.js');
    window = new Window({
      title, width: 1280, height: 860, minWidth: 760, minHeight: 520,
      ...(platform === 'win32'
        ? { icon: path.join(__dirname, 'icons/icon.ico'), titleBarStyle: 'hidden',
          titleBarOverlay: { ...colors(null, nativeTheme.shouldUseDarkColors), height: HEIGHT } }
        : { webPreferences: { contextIsolation: true, nodeIntegration: false, sandbox: true, webviewTag: false, preload: dashboardPreload } }),
    });
    const dashboardView = platform === 'win32' ? new WebContentsView({ webPreferences: {
      contextIsolation: true, nodeIntegration: false, sandbox: true, webviewTag: false, preload: dashboardPreload,
    } }) : null;
    // All page behavior uses this accessor, regardless of who owns the content.
    const dashboardContents = () => dashboardView ? dashboardView.webContents : window.webContents;
    const wc = dashboardContents();
    let bar, applicationMenu, menuOpen = false, dialogOpen = false;
    function focusDashboard(afterInteraction = false) {
      if (!dashboardView || window.isDestroyed() || wc.isDestroyed() || !window.isFocused()) return;
      if (menuOpen || dialogOpen || (!afterInteraction && bar?.view.webContents.isFocused())) return;
      wc.focus();
    }
    function openMenu() {
      if (menuOpen) return;
      menuOpen = true;
      // Native edit roles must target the page, not the hamburger document.
      wc.focus();
      applicationMenu.popup({ window, x: 32, y: HEIGHT, callback: () => {
        menuOpen = false;
        focusDashboard(true);
      } });
    }
    if (dashboardView) {
      bar = createBar({ window, WebContentsView, ipcMain, nativeTheme, openMenu });
      window.contentView.addChildView(dashboardView);
      window.contentView.addChildView(bar.view);
      const layout = () => {
        const [width, height] = window.getContentSize();
        bar.view.setBounds({ x: 0, y: 0, width, height: HEIGHT });
        dashboardView.setBounds({ x: 0, y: HEIGHT, width, height: Math.max(0, height - HEIGHT) });
      };
      layout();
      window.on('resize', layout);
      window.on('focus', () => focusDashboard());
      wc.once('did-finish-load', () => focusDashboard());
      window.on('closed', () => { if (!wc.isDestroyed()) wc.close(); });
      window.setMenuBarVisibility(false);
      // Bare Alt opens the same menu as the hamburger, from either view.
      for (const contents of [wc, bar.view.webContents]) {
        let altOnly = false;
        contents.on('before-input-event', (event, input) => {
          if (input.type === 'keyDown') {
            altOnly = input.key === 'Alt' && !input.control && !input.shift && !input.meta;
            if (altOnly) event.preventDefault();
          }
          if (input.key === 'Alt' && input.type === 'keyUp' && altOnly) {
            event.preventDefault(); altOnly = false; openMenu();
          }
        });
      }
      trackDashboardTheme(wc, color => bar.setTheme(color));
    }
    let navigation, closing = false, retained = { pin, title };
    window.on('close', event => {
      closing = true;
      if (dashboardView && !wc.isDestroyed()) {
        // BaseWindow.close does not run its views' beforeunload.
        event.preventDefault();
        wc.close({ waitForBeforeUnload: true });
      }
    });
    if (dashboardView) wc.on('destroyed', () => { if (!window.isDestroyed()) window.close(); });
    function beginNavigation() {
      const attempt = { loading: false, stayed: false };
      navigation = attempt;
      return attempt;
    }
    const isCurrent = attempt => navigation === attempt && !window.isDestroyed();
    wc.setUserAgent(`${wc.getUserAgent()} Werkschaft/${app.getVersion()}`);
    wc.on('page-title-updated', (event, value) => {
      event.preventDefault();
      if (pin && typeof value === 'string') {
        setTitle(value);
        retained.title = value;
      } else window.setTitle(title);
    });
    wc.on('will-attach-webview', event => event.preventDefault());
    const openExternal = url => {
      // Classification is repeated at the authority boundary; never open a scheme
      // merely because a caller thinks it was already validated.
      if (destination(url, pin) !== 'external') return;
      shell.openExternal(url).catch(() => console.error('Could not open the external link in the system browser'));
    };
    // Initial iframe src loads do not reliably emit will-frame-navigate.
    // Enforce the pin at the network boundary too, including redirect hops.
    const connectionURL = pathToFileURL(path.join(__dirname, 'connection.html')).href;
    wc.on('did-navigate', (_event, url) => {
      const target = navigation?.commitTarget;
      // Rollback belongs to the committed document, never another attempt's
      // provisional pin/title. A late promise settlement cannot advance it.
      if (target && (target.pin ? destination(url, target.pin) === 'dashboard' : url === connectionURL)) retained = target;
    });
    session.defaultSession.webRequest.onBeforeRequest((details, callback) => {
      if (details.resourceType !== 'mainFrame' && details.resourceType !== 'subFrame') return callback({});
      if (details.resourceType === 'mainFrame' && details.url === connectionURL) return callback({});
      const action = destination(details.url, pin, details.resourceType === 'mainFrame');
      if (action === 'dashboard' || action === 'frame') return callback({});
      callback({ cancel: true });
      if (action === 'external') openExternal(details.url);
    });
    const guard = (event, url, isMainFrame) => {
      const action = destination(url, pin, isMainFrame);
      if (action === 'dashboard' || action === 'frame') return;
      event.preventDefault();
      if (action === 'external') openExternal(url);
    };
    // Frame navigation and redirects need their own hooks: will-navigate covers
    // only the main frame, and checking just the initial URL misses redirects.
    wc.on('will-frame-navigate', event => guard(event, event.url, event.isMainFrame));
    wc.on('will-redirect', event => guard(event, event.url, event.isMainFrame));
    wc.setWindowOpenHandler(({ url }) => {
      const action = destination(url, pin);
      if (action === 'dashboard') navigate(url);
      else if (action === 'external') openExternal(url);
      return { action: 'deny' };
    });
    wc.on('will-prevent-unload', event => {
      // Capture the load before the synchronous native dialog: a newer attempt
      // must never inherit an older load's Stay (or an unrelated X-close veto).
      const attempt = !closing && navigation?.loading ? navigation : null;
      let leave;
      dialogOpen = true;
      try {
        leave = dialog.showMessageBoxSync(window, {
          type: 'question', buttons: ['Stay', 'Leave'], defaultId: 0, cancelId: 0,
          title: 'Leave Werkschaft?', message: 'Leave this page?',
          detail: 'Changes you made may not be saved.', noLink: true,
        });
      } finally { dialogOpen = false; focusDashboard(true); }
      closing = false;
      // Electron's preventDefault here means ignore the page's veto, not stay.
      if (leave === 1) event.preventDefault();
      else if (attempt && isCurrent(attempt)) attempt.stayed = true;
    });
    function setTitle(value) {
      title = value;
      if (!window.isDestroyed()) { window.setTitle(title); bar?.setTitle(title); }
    }
    function navigationFailed(attempt) {
      pin = null;
      const failureTitle = 'Werkschaft — Navigation failed — ⌘R to retry';
      setTitle(failureTitle);
      attempt.commitTarget = { pin: null, title: failureTitle };
      return wc.loadFile(path.join(__dirname, 'connection.html')).catch(() => {});
    }
    async function navigate(url, attempt = beginNavigation()) {
      if (!isCurrent(attempt)) return;
      attempt.commitTarget = { pin: `${new URL(url).origin}/`, title: 'Werkschaft' };
      attempt.loading = true;
      try { await wc.loadURL(url); }
      catch (error) {
        if (!isCurrent(attempt)) return;
        // ERR_ABORTED alone also means supersession or policy cancellation. Only
        // this still-current load's observed Stay may restore the retained page.
        if (attempt.stayed && (error.code === 'ERR_ABORTED' || error.errno === -3)) {
          pin = retained.pin;
          setTitle(retained.title);
        } else await navigationFailed(attempt);
      } finally {
        attempt.loading = false;
        if (navigation === attempt) navigation = null;
      }
    }
    async function connect() {
      if (connecting || window.isDestroyed()) return;
      const attempt = beginNavigation();
      connecting = true;
      setTitle('Werkschaft — Connecting…');
      try {
        let url;
        try { url = await connectDaemon({ appRoot: app.getAppPath(), userData: app.getPath('userData'), platform }); }
        catch (error) {
          if (!isCurrent(attempt)) return;
          pin = null;
          const failureTitle = `Werkschaft — ${error.message} — ⌘R to retry`;
          setTitle(failureTitle);
          attempt.commitTarget = { pin: null, title: failureTitle };
          await wc.loadFile(path.join(__dirname, 'connection.html'));
          return;
        }
        if (!isCurrent(attempt)) return;
        pin = url;
        setTitle('Werkschaft');
        await navigate(url, attempt);
      } catch { if (isCurrent(attempt)) await navigationFailed(attempt); }
      finally {
        connecting = false;
        if (navigation === attempt) navigation = null;
      }
    }
    const fromDashboard = event => event.sender === wc && event.senderFrame === wc.mainFrame &&
      !!pin && destination(event.senderFrame.url, pin) === 'dashboard';
    wc.on('did-start-navigation', (_event, _url, inPlace, isMainFrame) => {
      if (isMainFrame && !inPlace) updateTarget = null;
    });
    ipcMain.on(UPDATE.subscribe, event => {
      if (!fromDashboard(event)) return;
      updateTarget = wc;
      if (updatePending) sendUpdate(updatePending);
      else wc.send(UPDATE.status, { state: 'phase', phase: updater.phase() });
    });
    ipcMain.on(UPDATE.check, event => { if (fromDashboard(event)) updater.check(true); });
    ipcMain.on(UPDATE.download, event => { if (fromDashboard(event)) updater.download(); });
    ipcMain.on(UPDATE.skip, (event, version) => { if (fromDashboard(event)) updater.skip(version); });
    ipcMain.on(UPDATE.restart, event => {
      if (!fromDashboard(event) || !updater.canRestart()) return;
      relaunch = true;
      // A vetoed close leaves the app running; a later plain quit must not relaunch.
      setTimeout(() => { relaunch = false; }, 15000).unref?.();
      app.quit();
    });
    updater.schedule();
    function zoomBy(step) {
      zoom = nextZoom(zoom, step);
      wc.setZoomFactor(zoom);
    }
    const menu = [
      { role: 'appMenu' },
      { label: 'File', submenu: [{ role: 'close' }] },
      { role: 'editMenu' },
      { label: 'View', submenu: [
        { role: 'togglefullscreen' }, { type: 'separator' },
        { id: 'zoom_reset', label: 'Actual Size', accelerator: 'CmdOrCtrl+0', click: () => zoomBy('reset') },
        { id: 'zoom_in', label: 'Zoom In', accelerator: 'CmdOrCtrl+=', click: () => zoomBy('in') },
        { id: 'zoom_in_shifted', label: 'Zoom In', accelerator: 'CmdOrCtrl+Shift+=', click: () => zoomBy('in') },
        { id: 'zoom_out', label: 'Zoom Out', accelerator: 'CmdOrCtrl+-', click: () => zoomBy('out') },
      ] },
      { role: 'windowMenu' }, { role: 'help', submenu: [] },
      { label: 'Connection', submenu: [{ id: 'retry', label: 'Retry', accelerator: 'CmdOrCtrl+R', click: connect }] },
    ];
    applicationMenu = Menu.buildFromTemplate(menu);
    Menu.setApplicationMenu(applicationMenu);
    connect();
  });
}
if (require.main === module) start(require('electron'));
module.exports = { start };

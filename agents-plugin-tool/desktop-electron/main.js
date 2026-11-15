'use strict';
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { discover } = require('./endpoint');
const { nextZoom, destination } = require('./policy');

function start({ app, BrowserWindow, Menu, shell, dialog, session }) {
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
    // No preload, IPC handlers, webview, Node, or permissions for any document.
    session.defaultSession.setPermissionRequestHandler((_wc, _permission, callback) => callback(false));
    session.defaultSession.setPermissionCheckHandler(() => false);
    window = new BrowserWindow({
      title, width: 1280, height: 860, minWidth: 760, minHeight: 520,
      webPreferences: { contextIsolation: true, nodeIntegration: false, sandbox: true, webviewTag: false },
    });
    const wc = window.webContents;
    wc.setUserAgent(`${wc.getUserAgent()} Werkschaft/${app.getVersion()}`);
    wc.on('page-title-updated', event => { event.preventDefault(); window.setTitle(title); });
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
      const leave = dialog.showMessageBoxSync(window, {
        type: 'question', buttons: ['Stay', 'Leave'], defaultId: 0, cancelId: 0,
        title: 'Leave Werkschaft?', message: 'Leave this page?',
        detail: 'Changes you made may not be saved.', noLink: true,
      });
      // Electron's preventDefault here means ignore the page's veto, not stay.
      if (leave === 1) event.preventDefault();
    });
    function setTitle(value) { title = value; if (!window.isDestroyed()) window.setTitle(title); }
    function navigationFailed() {
      pin = null;
      setTitle('Werkschaft — Navigation failed — ⌘R to retry');
      return wc.loadFile(path.join(__dirname, 'connection.html')).catch(() => {});
    }
    async function navigate(url, previous = { pin, title }) {
      try { await wc.loadURL(url); }
      catch (error) {
        if (window.isDestroyed()) return;
        // Stay rejects loadURL with ERR_ABORTED, not a failed attachment. Undo
        // Retry's provisional pin/title without replacing the retained draft page.
        if (error.code === 'ERR_ABORTED' || error.errno === -3) {
          pin = previous.pin;
          setTitle(previous.title);
        } else await navigationFailed();
      }
    }
    async function connect() {
      if (connecting || window.isDestroyed()) return;
      const previous = { pin, title };
      connecting = true;
      setTitle('Werkschaft — Connecting…');
      try {
        let url;
        try { url = await discover(); }
        catch (error) {
          pin = null;
          setTitle(`Werkschaft — ${error.message} — ⌘R to retry`);
          await wc.loadFile(path.join(__dirname, 'connection.html'));
          return;
        }
        if (window.isDestroyed()) return;
        pin = url;
        setTitle('Werkschaft');
        await navigate(url, previous);
      } catch { if (!window.isDestroyed()) await navigationFailed(); }
      finally { connecting = false; }
    }
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
    Menu.setApplicationMenu(Menu.buildFromTemplate(menu));
    connect();
  });
}
if (require.main === module) start(require('electron'));
module.exports = { start };

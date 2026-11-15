'use strict';
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const HEIGHT = 36;
const STATE = 'werkschaft:bar-state', MENU = 'werkschaft:bar-menu';

function colors(color, dark) {
  if (!/^#[\da-f]{6}$/i.test(color || '')) color = dark ? '#202020' : '#f3f3f3';
  const rgb = color.slice(1).match(/../g).map(x => parseInt(x, 16) / 255)
    .map(x => x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4);
  const light = rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722 > 0.179;
  return { color, symbolColor: light ? '#000000' : '#ffffff' };
}

function createBar({ window, WebContentsView, ipcMain, nativeTheme, openMenu }) {
  const page = path.join(__dirname, 'titlebar.html');
  const pageURL = pathToFileURL(page).href;
  const bar = new WebContentsView({ webPreferences: {
    contextIsolation: true, nodeIntegration: false, sandbox: true, webviewTag: false,
    preload: path.join(__dirname, 'titlebar-preload.js'), partition: 'werkschaft-titlebar',
  } });
  const wc = bar.webContents;
  let title = window.getTitle(), themeColor = null;
  function update() {
    const theme = colors(themeColor, nativeTheme.shouldUseDarkColors);
    window.setTitleBarOverlay({ ...theme, height: HEIGHT });
    wc.send(STATE, { title, ...theme });
  }
  const allowed = new Set([pageURL, pathToFileURL(path.join(__dirname, 'titlebar-renderer.js')).href,
    pathToFileURL(path.join(__dirname, 'icons/icon.png')).href]);
  wc.session.setPermissionRequestHandler((_wc, _permission, callback) => callback(false));
  wc.session.setPermissionCheckHandler(() => false);
  wc.session.webRequest.onBeforeRequest((details, callback) => callback({ cancel: !allowed.has(details.url) }));
  wc.on('will-navigate', event => event.preventDefault());
  wc.on('will-redirect', event => event.preventDefault());
  wc.setWindowOpenHandler(() => ({ action: 'deny' }));
  wc.on('will-attach-webview', event => event.preventDefault());
  wc.on('did-finish-load', update);
  const menu = event => {
    if (event.sender === wc && event.senderFrame === wc.mainFrame && event.senderFrame.url === pageURL) openMenu();
  };
  ipcMain.on(MENU, menu);
  nativeTheme.on('updated', update);
  window.on('closed', () => {
    ipcMain.removeListener(MENU, menu);
    nativeTheme.removeListener('updated', update);
    if (!wc.isDestroyed()) wc.close();
  });
  wc.loadFile(page);
  return { view: bar, setTitle(value) { title = value; update(); },
    setTheme(value) { themeColor = value; update(); }, update };
}
module.exports = { HEIGHT, STATE, MENU, colors, createBar };

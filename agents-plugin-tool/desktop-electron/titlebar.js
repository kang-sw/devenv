'use strict';
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { randomUUID } = require('node:crypto');
const HEIGHT = 30;
const THEME_MARKER = '__WERKSCHAFT_BACKGROUND_V1__:';
const STATE = 'werkschaft:bar-state', MENU = 'werkschaft:bar-menu';

// Only opaque colors can describe the painted strip without compositing.
function parseColor(value) {
  if (typeof value !== 'string') return null;
  if (/^#[\da-f]{6}$/i.test(value)) return value.toLowerCase();
  const match = /^(rgb|rgba)\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})(?:\s*,\s*(0|1|0?\.\d+|1\.0+))?\s*\)$/.exec(value);
  if (!match || (match[1] === 'rgba') !== (match[5] !== undefined)) return null;
  if (match[5] !== undefined && Number(match[5]) !== 1) return null;
  const rgb = match.slice(2, 5).map(Number);
  if (rgb.some(x => x > 255)) return null;
  return '#' + rgb.map(x => x.toString(16).padStart(2, '0')).join('');
}

function colors(color, dark) {
  color = parseColor(color) || (dark ? '#202020' : '#f3f3f3');
  const rgb = color.slice(1).match(/../g).map(x => parseInt(x, 16) / 255)
    .map(x => x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4);
  const light = rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722 > 0.179;
  return { color, symbolColor: light ? '#000000' : '#ffffff' };
}

function parseThemeMessage(message, token) {
  const prefix = THEME_MARKER + token + ':';
  if (typeof message !== 'string' || message.length > 200 || !message.startsWith(prefix)) return undefined;
  try {
    const value = JSON.parse(message.slice(prefix.length));
    return value === null ? null : parseColor(value) || undefined;
  } catch { return undefined; }
}

// Runs in the existing isolated page, with no preload or privileged bridge.
function observeBackground(token, marker, parse) {
  window.__werkschaftBackgroundObserver?.();
  const read = () => parse(document.body && getComputedStyle(document.body).backgroundColor)
    || parse(getComputedStyle(document.documentElement).backgroundColor);
  let pending = false, previous = read();
  const report = () => {
    if (pending) return;
    pending = true;
    requestAnimationFrame(() => {
      pending = false;
      const color = read();
      if (color !== previous) {
        previous = color;
        console.info(marker + token + ':' + JSON.stringify(color));
      }
    });
  };
  const observer = new MutationObserver(report);
  for (const node of [document.documentElement, document.body]) {
    if (node) observer.observe(node, { attributes: true });
  }
  const media = matchMedia('(prefers-color-scheme: dark)');
  media.addEventListener('change', report);
  window.__werkschaftBackgroundObserver = () => {
    observer.disconnect();
    media.removeEventListener('change', report);
  };
  return previous;
}

function trackDashboardTheme(wc, setTheme) {
  let token = randomUUID(), background = null, themeColor = null;
  const update = () => setTheme(background || themeColor);
  wc.on('did-change-theme-color', (_event, color) => { themeColor = parseColor(color); update(); });
  wc.on('did-start-navigation', (_event, _url, inPlace, isMainFrame) => {
    if (!isMainFrame || inPlace) return;
    token = randomUUID();
    background = themeColor = null;
    update();
  });
  wc.on('console-message', event => {
    if (event.frame !== wc.mainFrame) return;
    const value = parseThemeMessage(event.message, token);
    if (value !== undefined) { background = value; update(); }
  });
  wc.on('did-finish-load', () => {
    const loadedToken = token;
    const script = `(${observeBackground})(${JSON.stringify(token)}, ${JSON.stringify(THEME_MARKER)}, ${parseColor})`;
    wc.executeJavaScript(script).then(value => {
      if (token !== loadedToken || wc.isDestroyed()) return;
      background = parseColor(value);
      update();
    }).catch(() => {
      if (token === loadedToken && !wc.isDestroyed()) { background = null; update(); }
    });
  });
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
module.exports = { HEIGHT, STATE, MENU, THEME_MARKER, parseColor, colors, parseThemeMessage,
  observeBackground, trackDashboardTheme, createBar };

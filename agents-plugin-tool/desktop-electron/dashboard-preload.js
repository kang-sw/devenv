'use strict';
// Bounded native bridge: update controls and a payload-free notification
// after an explicit native menu copy. No login URL, cookie or bearer is exposed.
const { contextBridge, ipcRenderer } = require('electron');
contextBridge.exposeInMainWorld('werkschaftDesktop', Object.freeze({
  daemon: Object.freeze({
    availability() { return ipcRenderer.invoke('werkschaft:daemon-availability'); },
    start() { return ipcRenderer.invoke('werkschaft:daemon-start'); },
  }),
  login: Object.freeze({
    onCopied(callback) {
      if (typeof callback !== 'function') return;
      ipcRenderer.on('werkschaft:login-copied', () => callback());
    },
  }),
  update: Object.freeze({
    check() { ipcRenderer.send('werkschaft:update-check'); },
    download() { ipcRenderer.send('werkschaft:update-download'); },
    skip(version) { ipcRenderer.send('werkschaft:update-skip', String(version)); },
    restart() { ipcRenderer.send('werkschaft:update-restart'); },
    // Manual channel actions carry no payload: release selection, URLs and
    // the canary/stable decision stay in the main process.
    checkCanary() { ipcRenderer.send('werkschaft:update-check-canary'); },
    backToStable() { ipcRenderer.send('werkschaft:update-back-to-stable'); },
    // { canary, version } of the running app; canary only for a packaged canary.
    channel() { return ipcRenderer.invoke('werkschaft:update-channel'); },
    onStatus(callback) {
      if (typeof callback !== 'function') return;
      ipcRenderer.on('werkschaft:update-status', (_event, status) => callback(status));
      ipcRenderer.send('werkschaft:update-subscribe');
    },
  }),
}));

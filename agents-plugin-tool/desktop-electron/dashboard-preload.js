'use strict';
// The dashboard's only native bridge: the app-update toasts and the settings
// row (none / available / ready, with Restart now). Every call is a
// payload-light IPC message the main process accepts only from the attached
// dashboard origin's main frame.
const { contextBridge, ipcRenderer } = require('electron');
contextBridge.exposeInMainWorld('werkschaftDesktop', Object.freeze({
  update: Object.freeze({
    check() { ipcRenderer.send('werkschaft:update-check'); },
    download() { ipcRenderer.send('werkschaft:update-download'); },
    skip(version) { ipcRenderer.send('werkschaft:update-skip', String(version)); },
    restart() { ipcRenderer.send('werkschaft:update-restart'); },
    onStatus(callback) {
      if (typeof callback !== 'function') return;
      ipcRenderer.on('werkschaft:update-status', (_event, status) => callback(status));
      ipcRenderer.send('werkschaft:update-subscribe');
    },
  }),
}));

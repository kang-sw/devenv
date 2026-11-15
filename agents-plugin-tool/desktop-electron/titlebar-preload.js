'use strict';
const { contextBridge, ipcRenderer } = require('electron');
contextBridge.exposeInMainWorld('werkschaftBar', Object.freeze({
  onState(callback) {
    const listener = (_event, state) => callback(state);
    ipcRenderer.on('werkschaft:bar-state', listener);
    return () => ipcRenderer.removeListener('werkschaft:bar-state', listener);
  },
  openMenu() { ipcRenderer.send('werkschaft:bar-menu'); },
}));

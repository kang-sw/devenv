'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { EventEmitter } = require('node:events');

test('login bridge is subscription-only and discards all native event payloads', () => {
  const ipcRenderer = new EventEmitter(), sends = [];
  ipcRenderer.send = (...args) => sends.push(args);
  let bridge;
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../dashboard-preload.js'), 'utf8'), {
    require: name => {
      assert.equal(name, 'electron');
      return { ipcRenderer, contextBridge: { exposeInMainWorld: (name, value) => {
        assert.equal(name, 'werkschaftDesktop'); bridge = value;
      } } };
    },
  });
  assert.deepEqual(Object.keys(bridge.login), ['onCopied']);
  assert(Object.isFrozen(bridge.login));
  assert.deepEqual(sends, [], 'preload startup requests no clipboard operation');
  bridge.login.onCopied(null);
  let received;
  bridge.login.onCopied((...args) => { received = args; });
  assert.deepEqual(sends, [], 'subscribing requests no clipboard operation');
  ipcRenderer.emit('werkschaft:login-copied', { sender: 'native-event' }, 'never-expose-credentials');
  assert.deepEqual(received, []);
});

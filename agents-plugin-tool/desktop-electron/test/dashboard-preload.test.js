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

test('update bridge adds payload-free canary and stable-return actions beside the stable controls', async () => {
  const ipcRenderer = new EventEmitter(), sends = [], invokes = [];
  ipcRenderer.send = (...args) => sends.push(args);
  ipcRenderer.invoke = (...args) => { invokes.push(args); return Promise.resolve({ canary: true, version: '1.2.3-canary.4' }); };
  let bridge;
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../dashboard-preload.js'), 'utf8'), {
    require: () => ({ ipcRenderer, contextBridge: { exposeInMainWorld: (_name, value) => { bridge = value; } } }),
  });
  assert(Object.isFrozen(bridge.update));
  // The original members stay, so an older dashboard keeps working unchanged.
  assert.deepEqual(Object.keys(bridge.update),
    ['check', 'download', 'skip', 'restart', 'checkCanary', 'backToStable', 'channel', 'onStatus']);
  bridge.update.check();
  // Arguments a page passes never cross the bridge.
  bridge.update.checkCanary('https://evil.test/x.zip', { allowOlder: true });
  bridge.update.backToStable('0.0.1');
  assert.deepEqual(sends, [['werkschaft:update-check'], ['werkschaft:update-check-canary'], ['werkschaft:update-back-to-stable']]);
  assert.deepEqual(await bridge.update.channel('ignored'), { canary: true, version: '1.2.3-canary.4' });
  assert.deepEqual(invokes, [['werkschaft:update-channel']]);
});

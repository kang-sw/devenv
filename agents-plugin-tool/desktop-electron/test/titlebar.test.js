'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { colors } = require('../titlebar');

test('theme color mapping selects contrast and rejects invalid input in favor of OS scheme', () => {
  assert.deepEqual(colors('#ffffff', true), { color: '#ffffff', symbolColor: '#000000' });
  assert.deepEqual(colors('#000000', false), { color: '#000000', symbolColor: '#ffffff' });
  for (const value of [null, '', 'red', 'transparent', '#123', '#12345600', 'url(x)']) {
    assert.deepEqual(colors(value, false), { color: '#f3f3f3', symbolColor: '#000000' });
    assert.deepEqual(colors(value, true), { color: '#202020', symbolColor: '#ffffff' });
  }
});
test('bar preload exposes only state subscription and a payload-free menu command', () => {
  let api, handler, removed, sent;
  vm.runInNewContext(fs.readFileSync(require.resolve('../titlebar-preload'), 'utf8'), {
    require(name) {
      assert.equal(name, 'electron');
      return { contextBridge: { exposeInMainWorld(name, value) { assert.equal(name, 'werkschaftBar'); api = value; } },
        ipcRenderer: { on(channel, fn) { assert.equal(channel, 'werkschaft:bar-state'); handler = fn; },
          removeListener(channel, fn) { removed = [channel, fn]; }, send(...args) { sent = args; } } };
    },
  });
  assert.deepEqual(Object.keys(api), ['onState', 'openMenu']);
  assert(Object.isFrozen(api));
  let received;
  const dispose = api.onState(state => { received = state; });
  const state = { title: '<script>not markup</script>', color: '#112233' };
  handler({ sender: 'must not escape' }, state);
  assert.equal(received, state);
  dispose(); assert.deepEqual(removed, ['werkschaft:bar-state', handler]);
  api.openMenu('ignored'); assert.deepEqual(sent, ['werkschaft:bar-menu']);
});

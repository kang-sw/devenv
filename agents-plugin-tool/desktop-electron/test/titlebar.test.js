'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { HEIGHT, parseColor, colors } = require('../titlebar');

test('strip CSS and view height agree at 30px', () => {
  assert.equal(HEIGHT, 30);
  assert.match(fs.readFileSync(require.resolve('../titlebar.html'), 'utf8'), /header \{ height: 30px;/);
});
test('theme color mapping selects contrast and rejects invalid input in favor of OS scheme', () => {
  assert.deepEqual(colors('#ffffff', true), { color: '#ffffff', symbolColor: '#000000' });
  assert.equal(parseColor('#F3F3F0'), '#f3f3f0');
  assert.equal(parseColor('rgb(243, 243, 240)'), '#f3f3f0');
  assert.equal(parseColor('rgba(20, 21, 24, 1)'), '#141518');
  assert.equal(parseColor('rgba(20,21,24,1.0)'), '#141518');
  assert.deepEqual(colors('rgb(243, 243, 240)', true), { color: '#f3f3f0', symbolColor: '#000000' });
  assert.deepEqual(colors('rgba(20, 21, 24, 1)', false), { color: '#141518', symbolColor: '#ffffff' });
  assert.equal(colors('rgb(117,117,117)', false).symbolColor, '#ffffff');
  assert.equal(colors('rgb(118,118,118)', false).symbolColor, '#000000');
  assert.deepEqual(colors('#000000', false), { color: '#000000', symbolColor: '#ffffff' });
  for (const value of [null, '', 'red', 'transparent', '#123', '#12345600', 'url(x)',
    'rgba(1, 2, 3, 0)', 'rgba(1,2,3,.5)', 'rgb(256,0,0)', 'rgb(-1,0,0)',
    'rgb(1,2,3,1)', 'rgba(1,2,3)', 'rgb(1,2,3); color:red']) {
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

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(__dirname + '/assets/dashboard.js', 'utf8');

test('scroll restoration coalesces read phase, keeps owners and pinned conversation, and yields to input', () => {
  const frames = [], events = {}, order = [];
  const pane = (name, value) => ({ isConnected: true, get scrollTop() { order.push('read:' + name); return value; }, set scrollTop(v) { order.push('write:' + name); value = v; } });
  const a = pane('a', 30), b = pane('b', 40), owned = pane('owned', 50), convo = pane('convo', 60);
  owned.wbScrollOwner = true;
  const context = { Map, convo, stick: true, requestAnimationFrame(fn) { frames.push(fn); return frames.length; }, window: { addEventListener(type, fn) { events[type] = fn; } } };
  vm.createContext(context);
  vm.runInContext(source.slice(source.indexOf('  var scrollRestore ='), source.indexOf('  // The conversation stays')) + '\nthis.restore = restoreScroll;', context);
  context.restore([[a, 10], [b, 20], [owned, 0], [convo, 0]]);
  context.restore([[a, 99]]);
  assert.equal(frames.length, 1);
  assert.deepEqual(order, []);
  frames.shift()();
  assert.deepEqual(order, ['read:a', 'read:b', 'write:a', 'write:b']);
  assert.equal(a.scrollTop, 10);
  order.length = 0;
  context.restore([[a, 100]]); events.wheel(); frames.shift()();
  assert.deepEqual(order, []);
});

test('conversation pagination supersedes an older queued WS scroll restoration', () => {
  const frames = [], convo = { isConnected: true, scrollTop: 80, scrollHeight: 1000 };
  const context = { Map, convo, stick: false, requestAnimationFrame(fn) { frames.push(fn); return 1; },
    window: { addEventListener() {} }, epoch: 1, histEpoch: 1, histBusy: true, box: {},
    frag: html => html, mergeEntries() { convo.scrollHeight += 1500; return []; },
    sweepOutImgs() {}, mermaid() {}, nearTop() {}, res: { before: 'e1' }
  };
  vm.createContext(context);
  vm.runInContext(source.slice(source.indexOf('  var scrollRestore ='), source.indexOf('  // The conversation stays')) + '\nrestoreScroll([[convo, 80]]);', context);
  const conversation = fs.readFileSync(__dirname + '/assets/dashboard-conversation.js', 'utf8');
  const pagination = conversation.split('}).then(function (res) {')[1].split('    }, function () {')[0];
  vm.runInContext('(function(){' + pagination + '})()', context);
  assert.equal(convo.scrollTop, 1580);
  frames.shift()();
  assert.equal(convo.scrollTop, 1580);
});

'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const vm = require('node:vm');
const { THEME_MARKER, parseThemeMessage, trackDashboardTheme, colors } = require('../titlebar');
const tick = () => new Promise(setImmediate);

test('console channel strictly accepts only bounded, current-token color or null messages', () => {
  const prefix = THEME_MARKER + 'current:';
  assert.equal(parseThemeMessage(prefix + '"rgb(20, 21, 24)"', 'current'), '#141518');
  assert.equal(parseThemeMessage(prefix + 'null', 'current'), null);
  for (const message of [null, {}, '', 'log ' + prefix + '"#ffffff"',
    THEME_MARKER + 'stale:"#ffffff"', prefix + '"#ffffff" trailing',
    prefix + '{"color":"#ffffff"}', prefix + '["#ffffff"]', prefix + 'true',
    prefix + '"transparent"', prefix + '"rgba(1,2,3,0.5)"', prefix + '"url(x)"',
    prefix + '"' + ' '.repeat(200) + '#ffffff"']) {
    assert.equal(parseThemeMessage(message, 'current'), undefined);
  }
});

function page() {
  const root = { backgroundColor: 'rgb(240, 240, 240)' }, body = { backgroundColor: 'rgb(20, 21, 24)' };
  let observer, mediaListener, frame;
  const observed = [], logged = [];
  const context = {
    window: {}, document: { documentElement: root, body },
    getComputedStyle: node => node,
    MutationObserver: class {
      constructor(callback) { observer = this; this.callback = callback; }
      observe(node, options) { observed.push([node, options]); }
      disconnect() { this.disconnected = true; }
    },
    matchMedia: query => {
      assert.equal(query, '(prefers-color-scheme: dark)');
      return { addEventListener: (_event, callback) => { mediaListener = callback; },
        removeEventListener: (_event, callback) => { assert.equal(callback, mediaListener); } };
    },
    requestAnimationFrame: callback => { frame = callback; },
    console: { info: message => logged.push(message) },
  };
  return { context, root, body, observed, logged, observer: () => observer,
    mutate() { observer.callback(); frame(); }, mediaChange() { mediaListener(); frame(); } };
}
function contents(execute) {
  return Object.assign(new EventEmitter(), { mainFrame: {}, isDestroyed: () => false, executeJavaScript: execute });
}

test('load reads actual body then root background, observes theme attributes/media and keeps isolation', async () => {
  const p = page(), themes = [];
  const wc = contents(async script => vm.runInNewContext(script, p.context));
  trackDashboardTheme(wc, value => themes.push(value));
  wc.emit('did-finish-load');
  await tick();
  assert.equal(themes.at(-1), '#141518');
  assert.deepEqual(p.observed.map(([node]) => node), [p.root, p.body]);
  const deliver = () => wc.emit('console-message', { frame: wc.mainFrame, message: p.logged.at(-1) });
  p.body.backgroundColor = 'rgb(243, 243, 240)';
  p.mutate(); deliver();
  assert.equal(themes.at(-1), '#f3f3f0');
  wc.emit('did-change-theme-color', {}, '#000000');
  assert.equal(themes.at(-1), '#f3f3f0', 'actual page background wins over a stale meta color');
  p.body.backgroundColor = 'rgba(0, 0, 0, 0)';
  p.root.backgroundColor = 'rgb(30, 30, 30)';
  p.mediaChange(); deliver();
  assert.equal(themes.at(-1), '#1e1e1e', 'transparent body uses the root');
  const count = p.logged.length;
  p.mutate();
  assert.equal(p.logged.length, count, 'unchanged colors do not spam the console');
  p.root.backgroundColor = 'transparent';
  p.mutate(); deliver();
  assert.equal(themes.at(-1), '#000000', 'unreadable background keeps a usable theme-color');
  wc.emit('did-change-theme-color', {}, null);
  assert.equal(themes.at(-1), null);
  assert.equal(colors(themes.at(-1), true).color, '#202020');
  assert.equal(colors(themes.at(-1), false).color, '#f3f3f3');
  p.context.window.__werkschaftBackgroundObserver();
  assert.equal(p.observer().disconnected, true);
});

test('theme channel ignores child frames, stale documents and failed or late JavaScript reads', async () => {
  const p = page(), themes = [], pending = [];
  const wc = contents(script => new Promise((resolve, reject) => {
    pending.push({ resolve, reject, value: vm.runInNewContext(script, p.context) });
  }));
  trackDashboardTheme(wc, value => themes.push(value));
  wc.emit('did-finish-load');
  pending[0].resolve(pending[0].value);
  await tick();
  p.body.backgroundColor = '#ffffff';
  p.mutate();
  const message = p.logged.at(-1);
  wc.emit('console-message', { frame: {}, message });
  assert.equal(themes.at(-1), '#141518');
  wc.emit('did-start-navigation', {}, '/#fragment', true, true);
  wc.emit('console-message', { frame: wc.mainFrame, message });
  assert.equal(themes.at(-1), '#ffffff', 'same-document navigation retains the observer');
  wc.emit('did-finish-load');
  wc.emit('did-start-navigation', {}, '/next', false, true);
  assert.equal(themes.at(-1), null);
  pending[1].resolve('#ffffff');
  wc.emit('console-message', { frame: wc.mainFrame, message });
  await tick();
  assert.equal(themes.at(-1), null, 'old promise and old marker cannot repaint a new document');
  wc.emit('did-finish-load');
  pending[2].reject(new Error('navigation interrupted'));
  await tick();
  assert.equal(themes.at(-1), null, 'JavaScript read failure falls back without an unhandled rejection');
});

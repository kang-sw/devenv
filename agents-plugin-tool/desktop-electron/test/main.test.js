'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const tick = () => new Promise(setImmediate);

async function fixture(discover = async () => 'http://127.0.0.1:39418/') {
  // Replace only the main-process discovery dependency; never a renderer bridge.
  const endpoint = require('../endpoint');
  const original = endpoint.discover;
  endpoint.discover = discover;
  delete require.cache[require.resolve('../main')];
  const { start } = require('../main');
  endpoint.discover = original;
  const app = Object.assign(new EventEmitter(), { setName() {}, requestSingleInstanceLock: () => true, quit() { this.quitCalled = true; }, whenReady: () => Promise.resolve(), getVersion: () => '0.1.0' });
  class Window extends EventEmitter {
    constructor(options) {
      super(); this.options = options; this.title = options.title; this.loads = [];
      this.webContents = Object.assign(new EventEmitter(), {
        setUserAgent: agent => { this.agent = agent; }, getUserAgent: () => 'Chromium',
        setWindowOpenHandler: handler => { this.popup = handler; },
        loadURL: async url => { this.loads.push(url); }, loadFile: async file => { this.loads.push(file); },
        setZoomFactor: zoom => { this.zoom = zoom; },
      });
      Window.current = this;
    }
    isDestroyed() { return false; }
    setTitle(title) { this.title = title; }
    isMinimized() { return true; }
    restore() { this.restored = true; }
    show() { this.shown = true; }
    focus() { this.focused = true; }
  }
  const session = { defaultSession: {
    setPermissionRequestHandler(handler) { this.request = handler; },
    setPermissionCheckHandler(handler) { this.check = handler; },
    webRequest: { onBeforeRequest(handler) { this.handler = handler; } },
  } };
  let template, response = 0;
  const opened = [];
  start({ app, BrowserWindow: Window, session,
    Menu: { buildFromTemplate: t => t, setApplicationMenu: t => { template = t; } },
    shell: { openExternal: async url => { opened.push(url); } },
    dialog: { showMessageBoxSync: () => response },
  });
  await tick();
  const window = Window.current;
  const menus = Object.fromEntries(template.flatMap(item => item.submenu || []).filter(item => item.id).map(item => [item.id, item]));
  return { app, window, menus, session: session.defaultSession, opened, choose: n => { response = n; } };
}
test('native window, menus, authority, close veto and single-instance focus', async () => {
  const f = await fixture(), w = f.window;
  assert.equal(w.title, 'Werkschaft');
  assert.deepEqual(w.loads, ['http://127.0.0.1:39418/']);
  assert.equal(w.agent, 'Chromium Werkschaft/0.1.0');
  assert.deepEqual(w.options.webPreferences, { contextIsolation: true, nodeIntegration: false, sandbox: true, webviewTag: false });
  assert.equal('preload' in w.options.webPreferences, false);
  assert.equal(f.session.check(), false);
  f.session.request(null, 'clipboard-read', grant => assert.equal(grant, false));
  assert.equal(f.menus.retry.accelerator, 'CmdOrCtrl+R');
  f.menus.zoom_in.click(); assert.equal(w.zoom, 1.1);
  f.menus.zoom_in_shifted.click(); assert.equal(w.zoom, 1.25);
  f.menus.zoom_out.click(); assert.equal(w.zoom, 1.1);
  f.menus.zoom_reset.click(); assert.equal(w.zoom, 1);
  let override = false;
  w.webContents.emit('will-prevent-unload', { preventDefault() { override = true; } });
  assert.equal(override, false);
  f.choose(1);
  w.webContents.emit('will-prevent-unload', { preventDefault() { override = true; } });
  assert.equal(override, true);
  f.app.emit('second-instance');
  assert(w.restored && w.shown && w.focused);
});
test('native navigation, redirect, initial subframe requests and popup policies', async () => {
  const f = await fixture(), w = f.window;
  const classifyEvent = (name, url, isMainFrame) => {
    let prevented = false;
    w.webContents.emit(name, { url, isMainFrame, preventDefault() { prevented = true; } });
    return prevented;
  };
  for (const name of ['will-frame-navigate', 'will-redirect']) {
    assert.equal(classifyEvent(name, 'http://127.0.0.1:39418/s/a', true), false);
    assert.equal(classifyEvent(name, 'about:srcdoc', false), false);
    assert.equal(classifyEvent(name, 'about:srcdoc', true), true);
    assert.equal(classifyEvent(name, 'https://example.com/', false), true);
    assert.equal(classifyEvent(name, 'https://user:secret@example.com/', true), true);
  }
  assert.deepEqual(f.opened, ['https://example.com/', 'https://example.com/']);
  function request(url, resourceType) {
    let decision;
    f.session.webRequest.handler({ url, resourceType }, value => { decision = value; });
    return decision;
  }
  assert.deepEqual(request('https://example.com/frame', 'subFrame'), { cancel: true });
  assert.deepEqual(request('file:///etc/passwd', 'subFrame'), { cancel: true });
  assert.deepEqual(request('http://127.0.0.1:39418/s/a', 'subFrame'), {});
  assert.deepEqual(request(pathToFileURL(path.resolve(__dirname, '../connection.html')).href, 'mainFrame'), {});
  assert.deepEqual(request('https://example.com/image.png', 'image'), {});
  assert.deepEqual(w.popup({ url: 'http://127.0.0.1:39418/s/b' }), { action: 'deny' });
  assert.equal(w.loads.at(-1), 'http://127.0.0.1:39418/s/b');
  assert.deepEqual(w.popup({ url: 'data:text/html,hi' }), { action: 'deny' });
  assert.deepEqual(w.popup({ url: 'https://example.com/popup' }), { action: 'deny' });
  assert.equal(f.opened.at(-1), 'https://example.com/popup');
});
test('overlapping retries are ignored and rediscovery replaces the pin', async () => {
  let finish, calls = 0;
  const f = await fixture(() => { calls++; return new Promise(resolve => { finish = resolve; }); });
  await f.menus.retry.click();
  assert.equal(calls, 1);
  finish('http://127.0.0.1:39418/');
  await tick();
  const retry = f.menus.retry.click();
  assert.equal(calls, 2);
  finish('http://127.0.0.1:39419/');
  await retry;
  let canceled;
  f.session.webRequest.handler({ url: 'http://127.0.0.1:39418/', resourceType: 'mainFrame' }, value => { canceled = value.cancel; });
  assert.equal(canceled, true);
  assert.equal(f.window.loads.at(-1), 'http://127.0.0.1:39419/');
});
test('discovery and immediate navigation failure show the bundled connection page', async () => {
  const f = await fixture(async () => { throw new Error('No readable dashboard endpoint'); });
  assert.match(f.window.title, /No readable dashboard endpoint — ⌘R to retry/);
  assert.equal(path.basename(f.window.loads.at(-1)), 'connection.html');
  const healthy = await fixture();
  healthy.window.webContents.loadURL = async () => { throw new Error('renderer network failure'); };
  await healthy.menus.retry.click();
  assert.equal(healthy.window.title, 'Werkschaft — Navigation failed — ⌘R to retry');
  assert.equal(path.basename(healthy.window.loads.at(-1)), 'connection.html');
});
test('losing single-instance lock quits before creating a window', () => {
  const { start } = require('../main');
  let quit = false;
  start({ app: { setName() {}, requestSingleInstanceLock: () => false, quit() { quit = true; } } });
  assert(quit);
});

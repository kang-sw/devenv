'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const tick = () => new Promise(setImmediate);

async function fixture(discover = async () => 'http://127.0.0.1:39418/', platform = 'darwin') {
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
        loadURL: async url => { this.loads.push(url); this.currentURL = url; this.webContents.emit('did-navigate', {}, url); },
        loadFile: async file => { this.loads.push(file); this.currentURL = file; this.webContents.emit('did-navigate', {}, pathToFileURL(file).href); },
        setZoomFactor: zoom => { this.zoom = zoom; },
      });
      this.children = [];
      this.contentView = { addChildView: view => this.children.push(view) };
      this.webContents.send = (channel, value) => { this.state = { channel, value }; };
      this.webContents.focus = () => { this.contentFocused = true; };
      this.webContents.isDestroyed = () => !!this.destroyed;
      this.webContents.close = options => { this.closeOptions = options; };
      this.webContents.session = { setPermissionRequestHandler() {}, setPermissionCheckHandler() {}, webRequest: { onBeforeRequest: handler => { this.request = handler; } } };
      this.webContents.mainFrame = {};
      Window.current = this;
    }
    getTitle() { return this.title; }
    getContentSize() { return [1280, 860]; }
    setTitleBarOverlay(value) { this.overlay = value; }
    setMenuBarVisibility(value) { this.menuVisible = value; }
    close() { this.emit('close', { preventDefault() {} }); }
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
  const views = [];
  class View extends Window {
    constructor(options) { const current = Window.current; super(options); Window.current = current; views.push(this); }
    setBounds(bounds) { this.bounds = bounds; }
  }
  const ipcMain = new EventEmitter(), nativeTheme = Object.assign(new EventEmitter(), { shouldUseDarkColors: false });
  let template, response = 0, popups = 0;
  const opened = [];
  start({ app, BrowserWindow: Window, WebContentsView: View, ipcMain, nativeTheme, session,
    Menu: { buildFromTemplate: t => Object.assign(t, { popup() { popups++; } }), setApplicationMenu: t => { template = t; } },
    shell: { openExternal: async url => { opened.push(url); } },
    dialog: { showMessageBoxSync: () => response },
  }, platform);
  await tick();
  const window = Window.current;
  const menus = Object.fromEntries(template.flatMap(item => item.submenu || []).filter(item => item.id).map(item => [item.id, item]));
  return { app, window, views, ipcMain, nativeTheme, popups: () => popups, menus, session: session.defaultSession, opened, choose: n => { response = n; } };
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
function stayOnNavigation(f, error) {
  const w = f.window;
  w.webContents.loadURL = async url => {
    w.loads.push(url);
    let override = false;
    // Stand in for the dashboard's armed pasted-image beforeunload guard.
    w.webContents.emit('will-prevent-unload', { preventDefault() { override = true; } });
    assert.equal(override, false, 'Stay must retain the renderer unload veto');
    throw error;
  };
}
function assertRetainedPage(f, url, title) {
  const w = f.window;
  assert.equal(w.currentURL, url);
  assert.equal(w.title, title);
  // Check the internal title too: renderer title updates must not undo rollback.
  w.webContents.emit('page-title-updated', { preventDefault() {} });
  assert.equal(w.title, title);
  assert(!w.loads.some(load => path.basename(load) === 'connection.html'));
  let decision;
  f.session.webRequest.handler({ url: url + 's/later-frame', resourceType: 'subFrame' }, value => { decision = value; });
  assert.deepEqual(decision, {}, 'the retained origin must remain pinned');
  let prevented = false;
  w.webContents.emit('will-frame-navigate', { url: url + 's/later-frame', isMainFrame: false, preventDefault() { prevented = true; } });
  assert.equal(prevented, false);
  assert.deepEqual(f.opened, []);
}
test('Retry Stay preserves the current page, title and old pin after rediscovery', async () => {
  let discovered = 'http://127.0.0.1:39418/';
  const f = await fixture(async () => discovered), w = f.window;
  const originalURL = w.currentURL, originalTitle = w.title;
  discovered = 'http://127.0.0.1:39419/';
  stayOnNavigation(f, Object.assign(new Error('ERR_ABORTED'), { code: 'ERR_ABORTED', errno: -3 }));
  await f.menus.retry.click();
  assert.equal(w.loads.at(-1), discovered);
  assertRetainedPage(f, originalURL, originalTitle);
  // Cancellation must release the retry lock, not leave the shell stuck.
  w.webContents.loadURL = async url => { w.loads.push(url); w.currentURL = url; w.webContents.emit('did-navigate', {}, url); };
  await f.menus.retry.click();
  assert.equal(w.currentURL, discovered);
});
test('same-origin popup Stay preserves the current page, title and pin', async () => {
  const f = await fixture(), w = f.window;
  const originalURL = w.currentURL, originalTitle = w.title;
  stayOnNavigation(f, Object.assign(new Error('ERR_ABORTED'), { errno: -3 }));
  assert.deepEqual(w.popup({ url: originalURL + 's/another-session' }), { action: 'deny' });
  await tick();
  assert.equal(w.loads.at(-1), originalURL + 's/another-session');
  assertRetainedPage(f, originalURL, originalTitle);
});
test('genuine Retry and popup load failures still clear the pin and show the connection page', async () => {
  for (const trigger of ['retry', 'popup']) {
    const f = await fixture(), w = f.window;
    const originalURL = w.currentURL;
    // A previous Stay must not suppress a later, genuine network failure.
    stayOnNavigation(f, Object.assign(new Error('ERR_ABORTED'), { code: 'ERR_ABORTED' }));
    await f.menus.retry.click();
    w.webContents.loadURL = async () => { throw Object.assign(new Error('ERR_CONNECTION_REFUSED'), { code: 'ERR_CONNECTION_REFUSED', errno: -102 }); };
    if (trigger === 'retry') await f.menus.retry.click();
    else { w.popup({ url: originalURL + 's/another-session' }); await tick(); }
    assert.equal(w.title, 'Werkschaft — Navigation failed — ⌘R to retry');
    assert.equal(path.basename(w.currentURL), 'connection.html');
    let decision;
    f.session.webRequest.handler({ url: originalURL + 's/frame', resourceType: 'subFrame' }, value => { decision = value; });
    assert.deepEqual(decision, { cancel: true });
  }
});
test('ERR_ABORTED without this navigation choosing Stay takes the failure path', async () => {
  for (const trigger of ['retry', 'popup']) {
    for (const choice of ['no dialog', 'Leave']) {
      for (const abort of [{ code: 'ERR_ABORTED' }, { errno: -3 }]) {
        const f = await fixture(), w = f.window, originalURL = w.currentURL;
        f.choose(1);
        w.webContents.loadURL = async () => {
          if (choice === 'Leave') {
            let override = false;
            w.webContents.emit('will-prevent-unload', { preventDefault() { override = true; } });
            assert.equal(override, true);
          }
          throw Object.assign(new Error('ERR_ABORTED'), abort);
        };
        if (trigger === 'retry') await f.menus.retry.click();
        else { w.popup({ url: originalURL + 's/redirect-cancelled' }); await tick(); }
        assert.equal(w.title, 'Werkschaft — Navigation failed — ⌘R to retry');
        assert.equal(path.basename(w.currentURL), 'connection.html');
        let decision;
        f.session.webRequest.handler({ url: originalURL + 's/frame', resourceType: 'subFrame' }, value => { decision = value; });
        assert.deepEqual(decision, { cancel: true });
      }
    }
  }
});
test('a superseded popup cannot roll back Retry after discovery pins a new origin', async () => {
  for (const stayed of [false, true]) {
    for (const lateError of ['ERR_ABORTED', 'ERR_CONNECTION_REFUSED']) {
      let discovered = 'http://127.0.0.1:39418/';
      const f = await fixture(async () => discovered), w = f.window;
      const originalURL = w.currentURL, pending = [];
      w.webContents.loadURL = url => {
        w.loads.push(url);
        return new Promise((resolve, reject) => {
          pending.push({ resolve: () => { w.currentURL = url; w.webContents.emit('did-navigate', {}, url); resolve(); }, reject });
        });
      };
      w.popup({ url: originalURL + 's/slow-popup' });
      if (stayed) w.webContents.emit('will-prevent-unload', { preventDefault() { assert.fail('Stay must keep the veto'); } });
      discovered = 'http://127.0.0.1:39419/';
      const retry = f.menus.retry.click();
      await tick();
      assert.equal(pending.length, 2);
      assert.equal(w.loads.at(-1), discovered);
      pending[0].reject(Object.assign(new Error(lateError), { code: lateError, errno: lateError === 'ERR_ABORTED' ? -3 : -102 }));
      await tick();
      assert.equal(w.title, 'Werkschaft');
      assert(!w.loads.some(load => path.basename(load) === 'connection.html'));
      let decision;
      f.session.webRequest.handler({ url: discovered + 's/frame', resourceType: 'subFrame' }, value => { decision = value; });
      assert.deepEqual(decision, {}, 'the superseded load must not restore origin A over B');
      let prevented = false;
      w.webContents.emit('will-frame-navigate', { url: discovered + 's/frame', isMainFrame: false, preventDefault() { prevented = true; } });
      assert.equal(prevented, false);
      assert.deepEqual(f.opened, []);
      pending[1].resolve();
      await retry;
      assertRetainedPage(f, discovered, 'Werkschaft');
    }
  }
});
test('Retry then popup Stay restores the committed page instead of the provisional Retry pin', async () => {
  let discovered = 'http://127.0.0.1:39418/';
  const f = await fixture(async () => discovered), w = f.window;
  const originalURL = w.currentURL, originalTitle = w.title, pending = [];
  w.webContents.loadURL = url => {
    w.loads.push(url);
    return new Promise((resolve, reject) => { pending.push({ resolve, reject }); });
  };
  discovered = 'http://127.0.0.1:39419/';
  const retry = f.menus.retry.click();
  await tick();
  assert.equal(w.currentURL, originalURL, 'Retry has not committed origin B');
  assert.equal(pending.length, 1);
  assert.deepEqual(w.popup({ url: discovered + 's/popup' }), { action: 'deny' });
  assert.equal(pending.length, 2);
  w.webContents.emit('will-prevent-unload', { preventDefault() { assert.fail('Stay must retain the popup unload veto'); } });
  pending[1].reject(Object.assign(new Error('ERR_ABORTED'), { code: 'ERR_ABORTED', errno: -3 }));
  await tick();
  pending[0].reject(Object.assign(new Error('superseded Retry'), { code: 'ERR_ABORTED', errno: -3 }));
  await retry;
  assertRetainedPage(f, originalURL, originalTitle);
});
test('rollback advances on a document commit before its load promise settles', async () => {
  let discovered = 'http://127.0.0.1:39418/';
  const f = await fixture(async () => discovered), w = f.window, pending = [];
  w.webContents.loadURL = url => {
    w.loads.push(url);
    return new Promise((resolve, reject) => { pending.push({ resolve, reject }); });
  };
  discovered = 'http://127.0.0.1:39419/';
  const retry = f.menus.retry.click();
  await tick();
  // Main-document commit precedes did-finish-load / loadURL resolution.
  w.currentURL = discovered;
  w.webContents.emit('did-navigate', {}, discovered);
  w.popup({ url: discovered + 's/popup' });
  w.webContents.emit('will-prevent-unload', { preventDefault() { assert.fail('Stay must retain the veto'); } });
  pending[1].reject(Object.assign(new Error('ERR_ABORTED'), { code: 'ERR_ABORTED' }));
  await tick();
  pending[0].reject(Object.assign(new Error('superseded Retry'), { code: 'ERR_ABORTED' }));
  await retry;
  assertRetainedPage(f, discovered, 'Werkschaft');
});
test('a superseded Retry promise cannot advance rollback without a document commit', async () => {
  let discovered = 'http://127.0.0.1:39418/';
  const f = await fixture(async () => discovered), w = f.window, pending = [];
  const originalURL = w.currentURL, originalTitle = w.title;
  w.webContents.loadURL = url => {
    w.loads.push(url);
    return new Promise((resolve, reject) => { pending.push({ resolve, reject }); });
  };
  discovered = 'http://127.0.0.1:39419/';
  const retry = f.menus.retry.click();
  await tick();
  w.popup({ url: discovered + 's/popup' });
  pending[0].resolve(); // Late successful settlement is not a new commit event.
  await retry;
  w.webContents.emit('will-prevent-unload', { preventDefault() { assert.fail('Stay must retain the veto'); } });
  pending[1].reject(Object.assign(new Error('ERR_ABORTED'), { code: 'ERR_ABORTED' }));
  await tick();
  assertRetainedPage(f, originalURL, originalTitle);
});
test('Stay restores a committed connection-page title, not an in-flight Retry title', async () => {
  let discovered;
  const f = await fixture(async () => {
    if (!discovered) throw new Error('No readable dashboard endpoint');
    return discovered;
  }), w = f.window;
  const originalURL = w.currentURL, originalTitle = w.title, pending = [];
  w.webContents.loadURL = url => {
    w.loads.push(url);
    return new Promise((resolve, reject) => { pending.push({ resolve, reject }); });
  };
  discovered = 'http://127.0.0.1:39419/';
  const retry = f.menus.retry.click();
  await tick();
  w.popup({ url: discovered + 's/popup' });
  w.webContents.emit('will-prevent-unload', { preventDefault() { assert.fail('Stay must retain the veto'); } });
  pending[1].reject(Object.assign(new Error('ERR_ABORTED'), { code: 'ERR_ABORTED' }));
  await tick();
  pending[0].reject(Object.assign(new Error('superseded Retry'), { code: 'ERR_ABORTED' }));
  await retry;
  assert.equal(w.currentURL, originalURL);
  assert.equal(w.title, originalTitle);
  assert.equal(w.loads.filter(load => path.basename(load) === 'connection.html').length, 1);
  let decision;
  f.session.webRequest.handler({ url: discovered + 's/frame', resourceType: 'subFrame' }, value => { decision = value; });
  assert.deepEqual(decision, { cancel: true }, 'a connection page has no retained dashboard origin');
});
test('an unrelated close Stay does not turn a later navigation abort into Stay', async () => {
  const f = await fixture(), w = f.window;
  w.webContents.emit('will-prevent-unload', { preventDefault() { assert.fail('Stay must keep the close veto'); } });
  w.webContents.loadURL = async () => { throw Object.assign(new Error('ERR_ABORTED'), { code: 'ERR_ABORTED' }); };
  await f.menus.retry.click();
  assert.equal(path.basename(w.currentURL), 'connection.html');
});
test('X-close Stay during a pending load is not that navigation choosing Stay', async () => {
  const f = await fixture(), w = f.window;
  let reject;
  w.webContents.loadURL = () => new Promise((_resolve, fail) => { reject = fail; });
  const retry = f.menus.retry.click();
  await tick();
  w.emit('close');
  w.webContents.emit('will-prevent-unload', { preventDefault() { assert.fail('Stay must keep the close veto'); } });
  reject(Object.assign(new Error('ERR_ABORTED'), { code: 'ERR_ABORTED' }));
  await retry;
  assert.equal(path.basename(w.currentURL), 'connection.html');
});
test('losing single-instance lock quits before creating a window', () => {
  const { start } = require('../main');
  let quit = false;
  start({ app: { setName() {}, requestSingleInstanceLock: () => false, quit() { quit = true; } } });
  assert(quit);
});

test('platform switch keeps native construction on macOS/Linux and isolates Windows sibling views', async () => {
  for (const platform of ['darwin', 'linux', 'win32']) {
    const f = await fixture(undefined, platform), w = f.window;
    const isolation = { contextIsolation: true, nodeIntegration: false, sandbox: true, webviewTag: false };
    assert.deepEqual(w.options.webPreferences, isolation);
    if (platform !== 'win32') {
      assert.deepEqual(w.options, { title: 'Werkschaft — Connecting…', width: 1280, height: 860, minWidth: 760, minHeight: 520, webPreferences: isolation });
      assert.equal(f.views.length, 0);
      continue;
    }
    assert.equal(w.options.titleBarStyle, 'hidden');
    assert.deepEqual(w.options.titleBarOverlay, { color: '#f3f3f3', symbolColor: '#000000', height: 36 });
    const [dashboard, bar] = f.views, wc = dashboard.webContents;
    assert.deepEqual(dashboard.options.webPreferences, isolation);
    assert.equal(w.menuVisible, false);
    assert.deepEqual(w.children, [dashboard, bar]);
    assert.deepEqual(dashboard.bounds, { x: 0, y: 36, width: 1280, height: 824 });
    assert.deepEqual(bar.bounds, { x: 0, y: 0, width: 1280, height: 36 });
    assert.deepEqual(w.loads, ['about:blank'], 'the inert owner must have a committed document for CDP initialization');
    assert.deepEqual(w.popup({ url: 'https://example.com' }), { action: 'deny' });
    assert.deepEqual(dashboard.loads, ['http://127.0.0.1:39418/']);
    assert.equal(dashboard.agent, 'Chromium Werkschaft/0.1.0');
    assert.equal(path.basename(bar.options.webPreferences.preload), 'titlebar-preload.js');
    wc.emit('page-title-updated', { preventDefault() {} }, 'Board — Test');
    assert.equal(w.title, 'Board — Test');
    assert.equal(bar.state.value.title, 'Board — Test');
    f.menus.zoom_in.click(); assert.equal(dashboard.zoom, 1.1);
    let blocked = false;
    wc.emit('will-frame-navigate', { url: 'file:///etc/passwd', isMainFrame: true, preventDefault() { blocked = true; } });
    assert(blocked);
    w.emit('close', { preventDefault() { blocked = 'close'; } });
    assert.equal(blocked, 'close');
    assert.deepEqual(dashboard.closeOptions, { waitForBeforeUnload: true });
    let override = false;
    wc.emit('will-prevent-unload', { preventDefault() { override = true; } });
    assert.equal(override, false);
    f.choose(1);
    wc.emit('will-prevent-unload', { preventDefault() { override = true; } });
    assert.equal(override, true);
    // A real view destruction completes the window close, not a Stay answer.
    dashboard.destroyed = true;
    let completed = false;
    w.close = () => { completed = true; };
    wc.emit('destroyed');
    assert(completed);
  }
});
test('title changes and connection failure titles remain correct on both content layouts', async () => {
  for (const platform of ['darwin', 'linux', 'win32']) {
    let unavailable = false;
    const f = await fixture(async () => {
      if (unavailable) throw new Error('Endpoint unavailable');
      return 'http://127.0.0.1:39418/';
    }, platform);
    const wc = f.views[0]?.webContents || f.window.webContents;
    wc.emit('page-title-updated', { preventDefault() {} }, 'My document');
    assert.equal(f.window.title, 'My document');
    unavailable = true;
    await f.menus.retry.click();
    wc.emit('page-title-updated', { preventDefault() {} }, 'Bundled connection page');
    assert.match(f.window.title, /Endpoint unavailable/);
    if (platform === 'win32') assert.equal(f.views[1].state.value.title, f.window.title);
  }
});
test('bar authority accepts only its bundled main frame; Alt and hamburger share the native menu', async () => {
  const f = await fixture(undefined, 'win32'), [dashboard, bar] = f.views;
  const { MENU } = require('../titlebar');
  const frame = bar.webContents.mainFrame;
  frame.url = pathToFileURL(path.resolve(__dirname, '../titlebar.html')).href;
  f.ipcMain.emit(MENU, { sender: dashboard.webContents, senderFrame: frame });
  f.ipcMain.emit(MENU, { sender: bar.webContents, senderFrame: { url: frame.url } });
  assert.equal(f.popups(), 0);
  f.ipcMain.emit(MENU, { sender: bar.webContents, senderFrame: frame });
  assert.equal(f.popups(), 1);
  assert(dashboard.contentFocused);
  for (const view of [dashboard, bar]) {
    view.webContents.emit('before-input-event', { preventDefault() {} }, { key: 'Alt', type: 'keyDown' });
    view.webContents.emit('before-input-event', { preventDefault() {} }, { key: 'Alt', type: 'keyUp' });
  }
  assert.equal(f.popups(), 3);
  for (const url of ['https://example.com/', 'file:///etc/passwd', frame.url + '?x']) {
    bar.request({ url }, value => assert.deepEqual(value, { cancel: true }));
  }
  bar.request({ url: frame.url }, value => assert.deepEqual(value, { cancel: false }));
  assert.deepEqual(bar.popup({ url: 'https://example.com' }), { action: 'deny' });
  f.window.emit('closed');
  assert.equal(f.ipcMain.listenerCount(MENU), 0);
  assert.equal(f.nativeTheme.listenerCount('updated'), 0);
});
test('dashboard theme maps to both bar and overlay, with native fallback and reset on navigation', async () => {
  const f = await fixture(undefined, 'win32'), [dashboard, bar] = f.views;
  dashboard.webContents.emit('did-change-theme-color', {}, '#112233');
  assert.deepEqual(f.window.overlay, { color: '#112233', symbolColor: '#ffffff', height: 36 });
  assert.equal(bar.state.value.color, '#112233');
  f.nativeTheme.shouldUseDarkColors = true;
  f.nativeTheme.emit('updated');
  assert.equal(f.window.overlay.color, '#112233');
  dashboard.webContents.emit('did-change-theme-color', {}, null);
  assert.equal(f.window.overlay.color, '#202020');
  dashboard.webContents.emit('did-change-theme-color', {}, '#ffffff');
  assert.equal(bar.state.value.symbolColor, '#000000');
  dashboard.webContents.emit('did-start-navigation', {}, 'http://127.0.0.1:39418/', false, true);
  assert.equal(f.window.overlay.color, '#202020');
});

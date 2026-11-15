'use strict';
// Windows interactive-session acceptance only. Never run this on the Mac.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { spawn, execFileSync } = require('node:child_process');
const { processes, launchedRoot, descendants, signalIfCurrent } = require('./host.cjs');
if (process.platform !== 'win32') throw new Error('Windows acceptance only');
process.env.DEBUG = [process.env.DEBUG, 'pw:browser*'].filter(Boolean).join(',');
const { _electron } = require(process.env.WS_PLAYWRIGHT);
const root = process.env.TBV_ROOT;
if (!root) throw new Error('TBV_ROOT is required');
const evidence = path.join(root, 'evidence');
fs.mkdirSync(evidence, { recursive: true });
const fixture = path.join(root, 'fixture');
const cache = path.join(fixture, 'cache');
const checks = [], unverified = [], failures = [], owned = new Map();
let application, daemon, failure;
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(fn, label, timeout = 15000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) { if (await fn()) return; await wait(100); }
  throw new Error('Timed out: ' + label);
}
function remember(child, time) {
  const all = processes(), identity = launchedRoot(child, all, time);
  assert(identity, 'launch identity must be recorded');
  for (const p of descendants(identity, all)) owned.set(p.pid, p);
  fs.appendFileSync(path.join(evidence, 'pids.jsonl'), JSON.stringify({ pid: child.pid, identity }) + '\n');
  return identity;
}
function family(identity) { for (const p of descendants(identity, processes())) owned.set(p.pid, p); }
async function checkpoint(label, run) {
  try { await run(); }
  catch (error) { failures.push(label + ': ' + error.message); console.error(label, error.stack); }
}
async function physical(action, args = []) {
  if (args[0] === '-X') {
    const point = await main(({ screen }, point) => screen.dipToScreenPoint(point), { x: args[1], y: args[3] });
    args = ['-X', point.x, '-Y', point.y];
  }
  const output = execFileSync('powershell.exe', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', path.join(root, 'physical.ps1'),
    '-Action', action, '-OwnedPid', String(await main(() => process.pid)), ...args.map(String)], { encoding: 'utf8' });
  fs.appendFileSync(path.join(evidence, 'physical.jsonl'), JSON.stringify({ action, args, output }) + '\n');
  return output;
}
const main = (fn, value) => application.evaluate(fn, value);
const state = () => main(({ BaseWindow }) => {
  const w = BaseWindow.getAllWindows()[0];
  return { title: w.getTitle(), bounds: w.getBounds(), maximized: w.isMaximized() };
});
async function capture(name) {
  await physical('capture', ['-Output', path.join(evidence, name + '.png')]);
}
(async () => {
  let appIdentity, daemonIdentity;
  try {
    const log = fs.openSync(path.join(evidence, 'daemon.log'), 'w');
    let time = Date.now();
    daemon = spawn(path.join(root, 'smoke-daemon.exe'), [], { env: { ...process.env, WS_CACHE_HOME: cache,
      WS_CONFIG_HOME: path.join(fixture, 'config'), GOPROXY: 'off', GOSUMDB: 'off', GOTOOLCHAIN: 'local' }, stdio: ['ignore', log, log] });
    daemonIdentity = remember(daemon, time);
    const endpoint = path.join(cache, 'dashboard/endpoint.json');
    await until(() => fs.existsSync(endpoint), 'isolated endpoint');
    const ep = JSON.parse(fs.readFileSync(endpoint));
    assert.notEqual(ep.port, 29417); assert.equal(ep.pid, daemon.pid);
    const origin = `http://127.0.0.1:${ep.port}`;
    assert.equal((await (await fetch(origin + '/health')).json()).pid, daemon.pid);
    const executablePath = path.resolve(__dirname, '../dist/Werkschaft-win32-x64/Werkschaft.exe');
    const env = { ...process.env, WS_CACHE_HOME: cache }; delete env.ELECTRON_RUN_AS_NODE;
    time = Date.now();
    application = await _electron.launch({ executablePath, args: [`--user-data-dir=${path.join(fixture, 'profile')}`], env, timeout: 60000 });
    appIdentity = remember(application.process(), time);
    const appLog = fs.createWriteStream(path.join(evidence, 'electron.log'));
    application.process().stdout?.pipe(appLog); application.process().stderr?.pipe(appLog);
    await until(() => application.context().pages().some(p => p.url() === origin + '/'), 'dashboard view');
    const dashboard = application.context().pages().find(p => p.url() === origin + '/');
    const bar = application.context().pages().find(p => p.url().endsWith('/titlebar.html'));
    assert(bar, 'bundled bar view');
    dashboard.on('dialog', () => {});
    await bar.locator('#menu').waitFor();
    assert(await bar.locator('img').evaluate(img => img.complete && img.naturalWidth > 0));
    assert.equal(await bar.locator('#title').textContent(), await dashboard.title());
    const layout = await main(({ BaseWindow }) => {
      const w = BaseWindow.getAllWindows()[0];
      const original = w.setTitleBarOverlay.bind(w);
      w.setTitleBarOverlay = value => { global.tbOverlay = value; original(value); };
      return { visible: w.isMenuBarVisible(),
        views: w.contentView.children.map(v => ({ bounds: v.getBounds(), prefs: v.webContents.getLastWebPreferences(), url: v.webContents.getURL() })) };
    });
    fs.writeFileSync(path.join(evidence, 'layout.json'), JSON.stringify(layout, null, 2));
    assert.equal(layout.visible, false); assert.equal(layout.views[0].bounds.y, 36);
    assert.equal(layout.views[0].prefs.preload || '', '');
    assert.equal(layout.views[0].prefs.contextIsolation, true); assert.equal(layout.views[0].prefs.sandbox, true);
    assert.equal(layout.views[0].prefs.nodeIntegration, false);
    checks.push('bar visible with icon, hamburger and centered document title; native menu hidden; isolated dashboard sibling below bar');
    // Launch, then Alt without clicking: do this before the first input action.
    await main(({ Menu, BaseWindow, webContents }) => {
      const w = BaseWindow.getAllWindows()[0];
      if (webContents.getFocusedWebContents() !== w.contentView.children[0].webContents) throw new Error('Dashboard must have initial focus');
      const menu = Menu.getApplicationMenu(), original = menu.popup.bind(menu);
      global.tbMenuOpens = 0;
      global.tbInputs = [];
      for (const [view, child] of w.contentView.children.entries()) child.webContents.on('before-input-event', (_event, input) => {
        global.tbInputs.push({ view, ...input });
      });
      menu.popup = options => { global.tbMenuOpens++; original(options); };
    });
    await capture('initial');
    await checkpoint('startup Alt', async () => {
      await physical('keys', ['-Keys', '%']);
      await until(() => main(() => global.tbMenuOpens === 1), 'launch, then Alt without clicking');
      await capture('startup-alt-menu'); await physical('keys', ['-Keys', '{ESC}']);
      await until(() => main(({ BaseWindow, webContents }) => webContents.getFocusedWebContents() ===
        BaseWindow.getAllWindows()[0].contentView.children[0].webContents), 'dashboard focus after startup Alt menu');
      checks.push('launch, then physical Alt opens the menu without clicking; dismissal returns focus to dashboard');
    });
    await dashboard.evaluate(() => { document.title = 'Titlebar acceptance — Board'; });
    await until(async () => (await state()).title === 'Titlebar acceptance — Board', 'native title');
    await until(async () => await bar.locator('#title').textContent() === 'Titlebar acceptance — Board', 'bar title');
    checks.push('document.title updates native window and centered bar title');

    // Native menu reachability and OS-injected Alt. The popup is Windows-owned,
    // so capture it on screen, not through a renderer screenshot.
    await checkpoint('hamburger menu', async () => {
      const before = await main(() => global.tbMenuOpens);
      await bar.locator('#menu').click();
      await until(async () => await main(() => global.tbMenuOpens) > before, 'hamburger popup');
      await capture('hamburger-menu'); await physical('keys', ['-Keys', '{ESC}']);
      checks.push('hamburger opens the native P8 menu');
    });
    // Startup already exercised bare Alt once; do not repeat a failed gesture.
    if (!failures.some(value => value.startsWith('startup Alt:'))) await checkpoint('Alt menu', async () => {
      const before = await main(() => global.tbMenuOpens);
      await physical('keys', ['-Keys', '%']);
      await until(async () => await main(() => global.tbMenuOpens) > before, 'Alt popup');
      await capture('alt-menu'); await physical('keys', ['-Keys', '{ESC}']);
      checks.push('physical Alt reopens the same P8 menu');
    });
    const menu = await main(({ Menu }) => {
      const items = menu => menu.items.map(i => ({ label: i.label, role: i.role, id: i.id, accelerator: i.accelerator, enabled: i.enabled,
        submenu: i.submenu ? items(i.submenu) : undefined }));
      return items(Menu.getApplicationMenu());
    });
    fs.writeFileSync(path.join(evidence, 'menu.json'), JSON.stringify(menu, null, 2));
    for (const label of ['File', 'Edit', 'View', 'Window', 'Help', 'Connection']) assert(menu.some(i => i.label.replaceAll('&', '') === label));
    const zoom = () => main(({ BaseWindow }) => BaseWindow.getAllWindows()[0].contentView.children[0].webContents.getZoomFactor());
    await checkpoint('zoom accelerators', async () => {
      await physical('keys', ['-Keys', '^=']);
      await until(async () => Math.abs(await zoom() - 1.1) < 0.001, 'physical zoom accelerator');
      await physical('keys', ['-Keys', '^0']);
      await until(async () => Math.abs(await zoom() - 1) < 0.001, 'physical zoom reset');
      checks.push('Ctrl+= and Ctrl+0 reach dashboard zoom');
    });
    unverified.push('Individual destructive/edit/fullscreen menu actions were not all invoked; full menu tree recorded.');

    let before, after;
    try {
    before = await state();
    const dragX = before.bounds.x + 180, dragY = before.bounds.y + 18;
    await physical('drag', ['-X', dragX, '-Y', dragY]);
    await wait(300);
    after = await state();
    assert(before.bounds.x !== after.bounds.x || before.bounds.y !== after.bounds.y, 'empty bar drag moves window');
    checks.push('OS-injected empty-bar drag moves the window');
    await physical('double', ['-X', after.bounds.x + 180, '-Y', after.bounds.y + 18]);
    await until(async () => (await state()).maximized, 'double click maximize');
    after = await state();
    await physical('double', ['-X', after.bounds.x + 180, '-Y', after.bounds.y + 18]);
    await until(async () => !(await state()).maximized, 'double click restore');
    checks.push('OS-injected empty-bar double click maximizes and restores');
    before = await state();
    await physical('click', ['-X', before.bounds.x + before.bounds.width - 69, '-Y', before.bounds.y + 18]);
    await until(async () => (await state()).maximized, 'Windows maximize control');
    after = await state();
    await physical('click', ['-X', after.bounds.x + after.bounds.width - 69, '-Y', after.bounds.y + 18]);
    await until(async () => !(await state()).maximized, 'Windows restore control');
    before = await state();
    await physical('click', ['-X', before.bounds.x + before.bounds.width - 115, '-Y', before.bounds.y + 18]);
    await until(() => main(({ BaseWindow }) => BaseWindow.getAllWindows()[0].isMinimized()), 'Windows minimize control');
    await main(({ BaseWindow }) => { const w = BaseWindow.getAllWindows()[0]; w.restore(); w.focus(); });
    checks.push('Windows-drawn maximize, restore and minimize controls work');
    } catch (error) {
      unverified.push('Physical gestures/control automation incomplete (not retried): ' + error.message);
      await main(({ BaseWindow }) => { const w = BaseWindow.getAllWindows()[0]; w.restore(); w.unmaximize(); w.focus(); });
    }
    unverified.push('Windows 11 snap-layout hover flyout not asserted.');

    // Exercise the real dashboard theme setting, without editing assets.
    await checkpoint('theme propagation', async () => {
    const themes = [];
    for (const theme of ['light', 'dark']) {
      await dashboard.locator(`[data-theme-set="${theme}"]`).first().evaluate(button => button.click());
      await until(async () => await dashboard.locator('meta[name="theme-color"]').first().getAttribute('content') === await bar.evaluate(() => {
        const c = document.body.style.backgroundColor.match(/\d+/g); return c ? '#' + c.map(x => Number(x).toString(16).padStart(2, '0')).join('') : '';
      }), 'dashboard theme propagation ' + theme);
      themes.push(await main(() => global.tbOverlay));
      await capture('theme-' + theme);
    }
    assert.notEqual(themes[0].color, themes[1].color);
    fs.writeFileSync(path.join(evidence, 'themes.json'), JSON.stringify(themes, null, 2));
    checks.push('dashboard light/dark theme-color recolors bar and Windows overlay');
    });

    await checkpoint('X-close confirmation', async () => {
    await main(({ dialog }) => { global.tbDialogs = []; dialog.showMessageBoxSync = (_w, options) => { global.tbDialogs.push(options); return 0; }; });
    // Use a dashboard-owned beforeunload guard and physical X, not window.destroy.
    await dashboard.evaluate(() => { window.addEventListener('beforeunload', event => { event.preventDefault(); event.returnValue = ''; }); });
    await dashboard.locator('body').click({ position: { x: 200, y: 200 } });
    before = await state();
    await physical('click', ['-X', before.bounds.x + before.bounds.width - 23, '-Y', before.bounds.y + 18]);
    await until(() => main(() => global.tbDialogs.length === 1), 'X close confirmation');
    assert.equal(await main(({ BaseWindow }) => BaseWindow.getAllWindows().length), 1);
    await capture('close-stay');
    checks.push('physical Windows X reaches dashboard beforeunload confirmation; Stay retains window');
    unverified.push('Close guard is a test-injected dashboard listener; real pasted-image guard was not armed in this focused run.');
    });
    if (failures.length) failure = failures.join('\n');
  } catch (error) { failure = error.stack; console.error(failure); }
  finally {
    if (appIdentity) family(appIdentity);
    if (daemonIdentity) family(daemonIdentity);
    if (application) {
      const inputs = await main(() => global.tbInputs || []).catch(() => []);
      fs.writeFileSync(path.join(evidence, 'inputs.json'), JSON.stringify(inputs, null, 2));
      await application.evaluate(({ app }) => app.exit()).catch(() => {}); await application.close().catch(() => {});
    }
    if (daemonIdentity) signalIfCurrent(daemonIdentity);
    for (const identity of owned.values()) signalIfCurrent(identity);
    await until(() => !processes().some(p => owned.get(p.pid)?.start === p.start), 'owned processes exited');
    const remaining = processes().filter(p => p.command?.toLowerCase().includes(root.toLowerCase()) && p.pid !== process.pid);
    fs.writeFileSync(path.join(evidence, 'summary.json'), JSON.stringify({ checks, unverified, failure, owned: [...owned.values()], remaining }, null, 2));
    console.log(JSON.stringify({ checks, unverified, failure, remaining }, null, 2));
    if (failure) process.exitCode = 1;
  }
})();

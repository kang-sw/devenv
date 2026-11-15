'use strict';
// Explicitly supplied cached Playwright; no npx, downloads or shared daemon.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawn, execFileSync } = require('node:child_process');
const { once } = require('node:events');
const { _electron } = require(process.env.WS_PLAYWRIGHT);
const root = path.resolve(__dirname, '..');
const executablePath = path.join(root, `dist/Werkschaft-darwin-${process.arch}/Werkschaft.app/Contents/MacOS/Werkschaft`);
const evidence = process.env.WS_ELECTRON_EVIDENCE || path.join(root, 'dist/evidence');
fs.mkdirSync(evidence, { recursive: true });
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-smoke-'));
const daemonRoot = path.join(temp, 'daemon'), shellRoot = path.join(temp, 'shell');
fs.mkdirSync(path.join(shellRoot, 'dashboard'), { recursive: true });
const endpoint = path.join(shellRoot, 'dashboard/endpoint.json');
const owned = new Map(), checks = [];
let application, daemon;
function processes() {
  return execFileSync('ps', ['-axo', 'pid=,ppid=,lstart=,command='], { encoding: 'utf8' }).split('\n').map(line => {
    const m = line.match(/^\s*(\d+)\s+(\d+)\s+(.{24})\s+(.*)$/);
    return m && { pid: +m[1], ppid: +m[2], start: m[3], command: m[4] };
  }).filter(Boolean);
}
function remember(pid) {
  const all = processes();
  const family = new Set([pid]);
  let changed;
  do {
    changed = false;
    for (const p of all) if (family.has(p.ppid) && !family.has(p.pid)) { family.add(p.pid); changed = true; }
  } while (changed);
  for (const p of all) if (family.has(p.pid)) owned.set(p.pid, p);
}
function scan(label) {
  const live = processes().filter(p => owned.get(p.pid)?.start === p.start);
  fs.appendFileSync(path.join(evidence, 'process-scans.jsonl'), JSON.stringify({ label, live }) + '\n');
  return live;
}
async function waitFor(check, label, timeout = 5000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    if (await check()) return;
    await new Promise(resolve => setTimeout(resolve, 25));
  }
  throw new Error('Timed out: ' + label);
}
async function menu(id) {
  return application.evaluate(async ({ Menu, BrowserWindow }, id) => {
    const wc = BrowserWindow.getAllWindows()[0].webContents;
    const loaded = id === 'retry' ? new Promise(resolve => wc.once('did-finish-load', resolve)) : null;
    Menu.getApplicationMenu().getMenuItemById(id).click();
    if (loaded) { await loaded; await new Promise(setImmediate); }
  }, id);
}
async function titleIncludes(value) {
  await waitFor(() => application.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].getTitle()).then(title => title.includes(value)), 'title ' + value);
}
async function main(fn, arg) { return application.evaluate(fn, arg); }
async function launch() {
  application = await _electron.launch({ executablePath, args: [`--user-data-dir=${path.join(temp, 'profile')}`],
    env: { ...process.env, WS_CACHE_HOME: shellRoot, WS_CONFIG_HOME: path.join(temp, 'config'), ELECTRON_RUN_AS_NODE: '' }, timeout: 8000 });
  remember(application.process().pid);
  return application.firstWindow();
}
async function run() {
  const log = fs.openSync(path.join(evidence, 'daemon.log'), 'w');
  daemon = spawn(path.join(root, 'dist/smoke-daemon'), [], {
    env: { ...process.env, WS_CACHE_HOME: daemonRoot, WS_CONFIG_HOME: path.join(temp, 'config'), GOPROXY: 'off', GOSUMDB: 'off', GOTOOLCHAIN: 'local' },
    stdio: ['ignore', log, log],
  });
  remember(daemon.pid);
  await waitFor(() => fs.existsSync(path.join(daemonRoot, 'dashboard/endpoint.json')), 'source daemon endpoint');
  const ep = JSON.parse(fs.readFileSync(path.join(daemonRoot, 'dashboard/endpoint.json')));
  assert.notEqual(ep.port, 29417);
  assert.equal(ep.pid, daemon.pid);
  const origin = `http://127.0.0.1:${ep.port}`;
  const initialHealth = await (await fetch(origin + '/health')).json();
  assert.equal(initialHealth.service, 'ws-dashboard');
  fs.writeFileSync(path.join(evidence, 'fixture.json'), JSON.stringify({ pid: ep.pid, port: ep.port, cache: daemonRoot, executablePath }, null, 2));
  let page = await launch();
  page.setDefaultTimeout(4000);
  const unexpectedNetwork = [];
  await application.context().route('**/*', route => {
    const url = route.request().url();
    if (url.startsWith(origin + '/') || url.startsWith('file:')) return route.continue();
    unexpectedNetwork.push(url);
    return route.abort();
  });
  await titleIncludes('No readable dashboard endpoint');
  await page.locator('h1').filter({ hasText: 'Connect to your dashboard' }).waitFor();
  assert.match(await page.locator('meta[http-equiv="Content-Security-Policy"]').getAttribute('content'), /default-src 'none'/);
  fs.writeFileSync(endpoint, JSON.stringify({ port: ep.port, pid: ep.pid + 1, token: 'SHELL-TOKEN-MUST-NOT-LEAK' }));
  await menu('retry');
  await titleIncludes('not from the recorded dashboard daemon');
  fs.writeFileSync(endpoint, JSON.stringify({ port: ep.port, pid: ep.pid, token: 'SHELL-TOKEN-MUST-NOT-LEAK' }));
  const endpointBefore = fs.readFileSync(endpoint);
  await menu('retry');
  await page.waitForURL(origin + '/');
  await titleIncludes('Werkschaft');
  assert.equal(await main(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].getTitle()), 'Werkschaft');
  checks.push('built app: missing endpoint, mismatched PID, retry, real dashboard attach and title');
  const security = await main(({ BrowserWindow }) => {
    const w = BrowserWindow.getAllWindows()[0], p = w.webContents.getLastWebPreferences();
    return { isolation: p.contextIsolation, sandbox: p.sandbox, node: p.nodeIntegration, preload: p.preload || '', webview: p.webviewTag, min: w.getMinimumSize(), size: w.getSize() };
  });
  assert.deepEqual(security, { isolation: true, sandbox: true, node: false, preload: '', webview: false, min: [760, 520], size: [1280, 860] });
  assert.deepEqual(await page.evaluate(() => [typeof window.require, typeof window.process]), ['undefined', 'undefined']);
  assert.match(await page.evaluate(() => navigator.userAgent), / Werkschaft\/0\.1\.0$/);
  assert(!JSON.stringify(await page.evaluate(() => ({ url: location.href, local: { ...localStorage }, session: { ...sessionStorage }, html: document.documentElement.outerHTML }))).includes('SHELL-TOKEN'));
  checks.push('zero renderer authority, no preload, UA, dimensions and credential exclusion');
  await main(({ shell, dialog, BrowserWindow }) => {
    global.smokeExternal = []; global.smokeDialogs = []; global.smokeChoice = 0; global.smokeFocus = 0;
    shell.openExternal = async url => { global.smokeExternal.push(url); };
    dialog.showMessageBoxSync = (_window, options) => { global.smokeDialogs.push(options); return global.smokeChoice; };
    BrowserWindow.getAllWindows()[0].on('focus', () => global.smokeFocus++);
  });
  const accelerators = await main(({ Menu }) => ['zoom_reset', 'zoom_in', 'zoom_in_shifted', 'zoom_out', 'retry'].map(id => Menu.getApplicationMenu().getMenuItemById(id).accelerator));
  assert.deepEqual(accelerators, ['CmdOrCtrl+0', 'CmdOrCtrl+=', 'CmdOrCtrl+Shift+=', 'CmdOrCtrl+-', 'CmdOrCtrl+R']);
  const factor = () => main(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].webContents.getZoomFactor());
  // CDP keyboard events bypass native AppKit menu dispatch (also true of
  // sendInputEvent). Assert the registered accelerators above and exercise
  // the actual menu callbacks here; physical key delivery is a manual check.
  await menu('zoom_in');
  assert(Math.abs(await factor() - 1.1) < 0.001);
  await menu('zoom_in_shifted');
  assert(Math.abs(await factor() - 1.25) < 0.001);
  await menu('zoom_out');
  assert(Math.abs(await factor() - 1.1) < 0.001);
  await menu('zoom_reset');
  assert(Math.abs(await factor() - 1) < 0.001);
  for (let i = 0; i < 16; i++) await menu('zoom_in');
  assert(Math.abs(await factor() - 3) < 0.001);
  for (let i = 0; i < 20; i++) await menu('zoom_out');
  assert(Math.abs(await factor() - 0.5) < 0.001);
  await menu('zoom_reset');
  checks.push('registered Cmd+0, Cmd+=, Cmd+Shift+=, Cmd+- menu callbacks; zoom clamps 50% and 300% (physical keys unverified)');

  // All popup routes use the same native policy, with the OS opener stubbed.
  const external = ['https://example.com/', `http://127.0.0.1:${ep.port === 39419 ? 39420 : 39419}/`, `http://localhost:${ep.port}/`, 'https://example.com/remote-board'];
  for (const url of external) {
    await page.evaluate(url => window.open(url), url);
    await waitFor(() => main((_electron, url) => global.smokeExternal.includes(url), url), 'external popup ' + url);
  }
  for (const url of ['file:///etc/passwd', 'javascript:1', 'data:text/html,blocked', 'mailto:user@example.com', 'ws://localhost/', 'custom:foo', 'https://user:password@example.com/', origin.replace('127.', 'user@127.') + '/']) {
    await page.evaluate(url => window.open(url), url);
  }
  assert.deepEqual(await main(() => global.smokeExternal), external);
  assert.equal(await main(({ BrowserWindow }) => BrowserWindow.getAllWindows().length), 1);
  // An ordinary main-frame external navigation and a subframe navigation are
  // denied too, not just target=_blank links.
  await page.evaluate(() => { const a = document.createElement('a'); a.href = 'https://example.com/main'; document.body.append(a); a.click(); });
  await waitFor(() => main(() => global.smokeExternal.includes('https://example.com/main')), 'external main navigation');
  await page.evaluate(() => { const f = document.createElement('iframe'); f.sandbox = 'allow-scripts'; f.srcdoc = '<p>opaque fixture</p>'; document.body.append(f); });
  await waitFor(() => main(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].webContents.mainFrame.frames.some(f => f.url === 'about:srcdoc')), 'opaque sandbox frame');
  await page.evaluate(() => { const f = document.createElement('iframe'); f.src = 'https://example.com/frame'; document.body.append(f); });
  // The dashboard CSP rejects this initial remote frame before a navigation
  // event. Native frame state is authoritative: Playwright's OOPIF URLs can
  // be empty even when Electron reports about:srcdoc.
  assert.equal(await main(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].webContents.mainFrame.frames.some(f => f.url.startsWith('https://example.com/'))), false);
  assert.equal(page.url(), origin + '/');
  assert.deepEqual(unexpectedNetwork, []);
  checks.push('main/popup origin policy and browser handoff; opaque frame allowed, remote frame blocked by dashboard CSP; blocked schemes/userinfo; no second window');

  await page.evaluate(origin => window.open(origin + '/s/electronfixture?frame=1'), origin);
  await page.waitForURL(origin + '/s/electronfixture?frame=1');
  await page.locator('#ctext').waitFor();
  const requests = [];
  // Assert the real dashboard link-menu's HTTP request, not an OS app launch.
  await page.route('**/s/electronfixture/fs/act', async route => { requests.push({ url: route.request().url(), body: route.request().postDataJSON() }); await route.fulfill({ status: 204 }); });
  await page.evaluate(project => {
    const a = document.createElement('a'); a.id = 'smoke-file'; a.textContent = 'fixture.md';
    a.href = '/s/electronfixture/file?path=fixture.md'; a.dataset.wl = 'file'; a.dataset.rel = 'fixture.md'; a.dataset.abs = project + '/fixture.md'; document.body.append(a);
  }, path.join(daemonRoot, 'project'));
  await page.locator('#smoke-file').click();
  await page.locator('.lmenu button').filter({ hasText: /Open with system/i }).click();
  await waitFor(() => Promise.resolve(requests.length === 1), 'open with system fs/act request');
  assert.equal(requests[0].body.action, 'open');
  checks.push('real dashboard desktop file-link menu sends fs/act open (request intercepted before OS launch)');
  await page.locator('#ctext').fill('preserve this draft');
  await page.locator('#ctext').click();
  await page.evaluate(() => {
    const png = Uint8Array.from(atob('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jZ9kAAAAASUVORK5CYII='), c => c.charCodeAt(0));
    const transfer = new DataTransfer(); transfer.items.add(new File([png], 'draft.png', { type: 'image/png' }));
    document.getElementById('ctext').dispatchEvent(new ClipboardEvent('paste', { clipboardData: transfer, bubbles: true, cancelable: true }));
  });
  await waitFor(() => page.locator('#ctext').inputValue().then(value => value.includes('[Image #1]')), 'unreferenced pasted image marker');
  await main(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].close());
  await waitFor(() => main(() => global.smokeDialogs.length === 1), 'X-close leave/stay dialog');
  assert.equal(await main(({ BrowserWindow }) => BrowserWindow.getAllWindows().length), 1);
  assert.match(await page.locator('#ctext').inputValue(), /preserve this draft.*\[Image #1\]/s);
  await page.screenshot({ path: path.join(evidence, 'electron-dashboard-draft.png') });
  checks.push('real dashboard image guard: X-close Stay keeps window and draft');

  // Second launch shares only this test's profile, focuses the first, and exits.
  await main(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].minimize());
  const secondLog = fs.openSync(path.join(evidence, 'second-launch.log'), 'w');
  const second = spawn(executablePath, [`--user-data-dir=${path.join(temp, 'profile')}`], { env: { ...process.env, WS_CACHE_HOME: shellRoot, ELECTRON_RUN_AS_NODE: '' }, stdio: ['ignore', secondLog, secondLog] });
  remember(second.pid);
  const [code] = await once(second, 'exit');
  assert.equal(code, 0);
  await waitFor(() => main(({ BrowserWindow }) => !BrowserWindow.getAllWindows()[0].isMinimized() && global.smokeFocus > 0), 'second-instance focus');
  assert.equal(await main(({ BrowserWindow }) => BrowserWindow.getAllWindows().length), 1);
  scan('second launch exited');
  checks.push('built second launch exits and restores/focuses the existing sole window');
  remember(application.process().pid);
  const appExit = once(application.process(), 'exit');
  await main(({ BrowserWindow }) => { global.smokeChoice = 1; BrowserWindow.getAllWindows()[0].close(); }).catch(() => {});
  await appExit;
  application = null;
  assert.equal((await (await fetch(origin + '/health')).json()).pid, ep.pid);
  assert.deepEqual(fs.readFileSync(endpoint), endpointBefore);
  checks.push('Leave closes and daemon survives; endpoint unchanged');
  scan('guarded app exited');

  page = await launch();
  await page.waitForURL(origin + '/');
  assert(Math.abs(await factor() - 1) < 0.001);
  remember(application.process().pid);
  const cleanExit = once(application.process(), 'exit');
  await main(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].close()).catch(() => {});
  await cleanExit;
  application = null;
  assert.equal((await (await fetch(origin + '/health')).json()).pid, ep.pid);
  checks.push('no image draft: immediate X-close; fresh process zoom resets; daemon survives');
}
(async () => {
  let failure;
  try { await run(); }
  catch (error) { failure = error; console.error(error.stack); }
  finally {
    if (application) {
      remember(application.process().pid);
      await application.evaluate(({ app }) => app.exit()).catch(() => {});
      await application.close().catch(() => {});
    }
    if (daemon && daemon.exitCode === null) {
      const exited = once(daemon, 'exit'); daemon.kill('SIGTERM'); await exited;
    }
    // Signal only processes this invocation recorded, and only if their start
    // identity still matches. Never retain a stale numerical PID as authority.
    for (const p of scan('before cleanup')) {
      try { process.kill(p.pid, 'SIGTERM'); } catch (error) { if (error.code !== 'ESRCH') throw error; }
    }
    await waitFor(() => Promise.resolve(scan('final').length === 0), 'owned process exit');
    fs.writeFileSync(path.join(evidence, 'summary.json'), JSON.stringify({ checks, failure: failure?.message || null, owned: [...owned.values()], temp }, null, 2));
    fs.rmSync(temp, { recursive: true, force: true });
  }
  console.log(JSON.stringify({ checks, failure: failure?.message || null, evidence }, null, 2));
  if (failure) process.exitCode = 1;
})();

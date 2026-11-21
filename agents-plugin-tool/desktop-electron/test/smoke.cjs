'use strict';
// Explicitly supplied cached Playwright; no npx, downloads or shared daemon.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const http = require('node:http');
const path = require('node:path');
const { spawn, execFileSync } = require('node:child_process');
const { once } = require('node:events');
const root = path.resolve(__dirname, '..');
const { builtPaths, processes, launchedRoot, descendants, signalIfCurrent } = require('./host.cjs');
const { draftFrame, attachImageDraft, assertImageDraft, openLegacyGuard } = require('./draft-close.cjs');
const { executablePath, daemonPath } = builtPaths(root);
const evidence = process.env.WS_ELECTRON_EVIDENCE || path.join(root, 'dist/evidence');
fs.mkdirSync(evidence, { recursive: true });
const electronAppLog = path.join(evidence, 'electron-app.log');
const playwrightDebugLog = path.join(evidence, 'playwright-debug.log');
fs.writeFileSync(electronAppLog, '');
fs.writeFileSync(playwrightDebugLog, '');
// Enable before loading Playwright: a rejected launch exposes no app streams.
process.env.DEBUG = [process.env.DEBUG, 'pw:browser*'].filter(Boolean).join(',');
const stderrWrite = process.stderr.write;
process.stderr.write = function (chunk, ...args) {
  fs.appendFileSync(playwrightDebugLog, chunk);
  return stderrWrite.call(this, chunk, ...args);
};
const { _electron } = require(process.env.WS_PLAYWRIGHT);
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-smoke-'));
const daemonRoot = path.join(temp, 'daemon'), shellRoot = path.join(temp, 'shell');
fs.mkdirSync(path.join(shellRoot, 'dashboard'), { recursive: true });
const endpoint = path.join(shellRoot, 'dashboard/endpoint.json');
const isolatedHome = path.join(temp, 'home');
fs.mkdirSync(isolatedHome);
const fixtureEnv = { ...process.env, HOME: isolatedHome, WS_CONFIG_HOME: path.join(temp, 'config') };
for (const name of ['CLAUDE_CONFIG_DIR', 'CODEX_HOME', 'PI_CODING_AGENT_DIR']) delete fixtureEnv[name];
const owned = new Map(), checks = [];
let application, daemon, applicationIdentity, daemonIdentity, dimensions;
const targetRequests = [];
const target = http.createServer((request, response) => {
  targetRequests.push(request.url);
  response.end('<p id="remote-marker">REMOTE CONTENT MUST NOT COMMIT</p>');
});
function remember(identity, all = processes()) {
  for (const p of descendants(identity, all)) owned.set(p.pid, p);
}
function recordLaunch(child, launchedAt) {
  const all = processes();
  const identity = launchedRoot(child, all, launchedAt);
  fs.appendFileSync(path.join(evidence, 'process-scans.jsonl'), JSON.stringify({ label: 'launch identity', pid: child.pid, identity }) + '\n');
  remember(identity, all);
  return identity;
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
  const launchedAt = Date.now();
  fs.appendFileSync(electronAppLog, `\nLaunch at ${new Date(launchedAt).toISOString()}\n`);
  const env = { ...fixtureEnv, WS_CACHE_HOME: shellRoot,
    ELECTRON_ENABLE_LOGGING: '1', ELECTRON_ENABLE_STACK_DUMPING: '1' };
  delete env.ELECTRON_RUN_AS_NODE;
  try {
    application = await _electron.launch({ executablePath, args: [`--user-data-dir=${path.join(temp, 'profile')}`],
      env, timeout: 60000 });
  } catch (error) {
    fs.appendFileSync(electronAppLog, `Launch failed: ${error.message}\nPlaywright-captured app output (also in playwright-debug.log):\n`);
    fs.appendFileSync(electronAppLog, fs.readFileSync(playwrightDebugLog));
    throw error;
  }
  const child = application.process();
  for (const [name, stream] of [['stdout', child.stdout], ['stderr', child.stderr]]) {
    stream?.on('data', chunk => fs.appendFileSync(electronAppLog, `[${name}] ${chunk}`));
  }
  applicationIdentity = recordLaunch(child, launchedAt);
  const page = await application.firstWindow();
  // The main process owns Electron's beforeunload decision. With no listener,
  // Playwright auto-accepts it via CDP after the native handler has already
  // answered, racing the close and producing "No dialog is showing".
  page.on('dialog', () => {});
  return page;
}
async function run() {
  await new Promise(resolve => target.listen(0, '127.0.0.1', resolve));
  assert.notEqual(target.address().port, 29417);
  const targetOrigin = `http://127.0.0.1:${target.address().port}`;
  const log = fs.openSync(path.join(evidence, 'daemon.log'), 'w');
  const daemonLaunchedAt = Date.now();
  daemon = spawn(daemonPath, [], {
    env: { ...fixtureEnv, WS_CACHE_HOME: daemonRoot, GOPROXY: 'off', GOSUMDB: 'off', GOTOOLCHAIN: 'local' },
    stdio: ['ignore', log, log],
  });
  daemonIdentity = recordLaunch(daemon, daemonLaunchedAt);
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
  await application.context().addCookies([{ name: 'ws_dashboard_lang', value: 'en', url: origin }]);
  const unexpectedNetwork = [];
  await application.context().route('**/*', route => {
    const url = route.request().url();
    if (url.startsWith(origin + '/') || url.startsWith(targetOrigin + '/') || url.startsWith('file:')) return route.continue();
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
  const { size, workArea, preload, ...security } = await main(({ BrowserWindow, screen }) => {
    const w = BrowserWindow.getAllWindows()[0], p = w.webContents.getLastWebPreferences();
    return { isolation: p.contextIsolation, sandbox: p.sandbox, node: p.nodeIntegration, preload: p.preload || '', webview: p.webviewTag, min: w.getMinimumSize(), size: w.getSize(), workArea: screen.getDisplayMatching(w.getBounds()).workArea };
  });
  dimensions = { size, workArea };
  console.log(JSON.stringify({ dimensions }));
  assert.equal(path.basename(preload), 'dashboard-preload.js');
  assert.deepEqual(security, { isolation: true, sandbox: true, node: false, webview: false, min: [760, 520] });
  assert.equal(size[0], 1280);
  assert.equal(size[1], Math.min(860, workArea.height));
  assert.deepEqual(await page.evaluate(() => [typeof window.require, typeof window.process]), ['undefined', 'undefined']);
  const appVersion = await main(({ app }) => app.getVersion());
  assert((await page.evaluate(() => navigator.userAgent)).endsWith(` Werkschaft/${appVersion}`));
  assert(!JSON.stringify(await page.evaluate(() => ({ url: location.href, local: { ...localStorage }, session: { ...sessionStorage }, html: document.documentElement.outerHTML }))).includes('SHELL-TOKEN'));
  checks.push('isolated dashboard preload, UA, dimensions and credential exclusion');
  await main(({ shell, dialog, BrowserWindow }) => {
    global.smokeExternal = []; global.smokeDialogs = []; global.smokeChoice = 0; global.smokeFocus = 0;
    global.smokeFailures = [];
    BrowserWindow.getAllWindows()[0].webContents.on('did-fail-load', (_event, code, description, url, isMainFrame) => {
      global.smokeFailures.push({ code, description, url, isMainFrame });
    });
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
  const modifier = process.platform === 'win32' ? 'Ctrl' : 'Cmd';
  checks.push(`registered ${modifier}+0, ${modifier}+=, ${modifier}+Shift+=, ${modifier}+- menu callbacks; zoom clamps 50% and 300% (physical keys checked separately)`);
  fs.writeFileSync(path.join(evidence, 'platform.json'), JSON.stringify(await main(({ BrowserWindow, Menu }) => {
    const window = BrowserWindow.getAllWindows()[0];
    return { platform: process.platform, arch: process.arch, versions: process.versions,
      menuBarVisible: window.isMenuBarVisible(), menus: Menu.getApplicationMenu().items.map(item => ({ label: item.label, role: item.role })),
      retryTitle: window.getTitle() };
  }), null, 2));
  if (process.platform === 'win32' && process.env.WS_ELECTRON_PHYSICAL_KEYS === '1') {
    const observations = [];
    for (const [keys, expected] of [['^0', 1], ['^=', 1.1], ['^+=', 1.25], ['^-', 1.1], ['^0', 1]]) {
      const pid = application.process().pid;
      const script = `$shell = New-Object -ComObject WScript.Shell; if (-not $shell.AppActivate(${pid})) { throw 'Could not activate owned Electron window' }; Start-Sleep -Milliseconds 150; $shell.SendKeys('${keys}'); Start-Sleep -Milliseconds 150`;
      let error;
      try { execFileSync('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script], { stdio: 'pipe' }); }
      catch (e) { error = e.message; }
      observations.push({ keys, expected, actual: await factor(), error });
    }
    fs.writeFileSync(path.join(evidence, 'physical-keys.json'), JSON.stringify(observations, null, 2));
    checks.push(observations.every(o => !o.error && Math.abs(o.actual - o.expected) < 0.001)
      ? 'Windows OS-injected physical Ctrl+0, Ctrl+=, Ctrl+Shift+=, Ctrl+- delivered to the native menu'
      : 'Windows physical Ctrl zoom delivery incomplete; see physical-keys.json (not a replacement for menu assertions)');
    await menu('zoom_reset');
  }

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
  await page.evaluate(() => { const f = document.createElement('iframe'); f.name = 'smoke-opaque'; f.sandbox = 'allow-scripts'; f.srcdoc = '<p>opaque fixture</p>'; document.body.append(f); });
  await waitFor(() => main(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].webContents.mainFrame.frames.some(f => f.url === 'about:srcdoc')), 'opaque sandbox frame');
  const documents = [];
  async function frameDocument(name) {
    const result = await main(async ({ BrowserWindow }, name) => {
      const frame = BrowserWindow.getAllWindows()[0].webContents.mainFrame.frames.find(f => f.name === name);
      return { requestedURL: frame.url, document: await frame.executeJavaScript('({url:location.href,text:document.body.innerText})') };
    }, name);
    documents.push({ name, ...result });
    return result.document;
  }
  assert.equal((await frameDocument('smoke-opaque')).text, 'opaque fixture');
  async function deniedFrame(name, csp = false) {
    const url = targetOrigin + '/' + name;
    await page.evaluate(({ name, url }) => { const f = document.createElement('iframe'); f.name = name; f.src = url; document.body.append(f); }, { name, url });
    if (csp) {
      await waitFor(() => main((_electron, url) => global.smokeFailures.some(f => f.url === url && f.code === -30 && !f.isMainFrame), url), name + ' CSP failure');
    } else {
      await waitFor(() => main((_electron, url) => global.smokeExternal.includes(url), url), name + ' native browser handoff');
    }
    // CSP commits an error document; preventing will-frame-navigate instead
    // retains about:blank and emits no did-fail-load. Neither commits the target.
    assert.equal((await frameDocument(name)).url, csp ? 'chrome-error://chromewebdata/' : 'about:blank');
    assert.deepEqual(targetRequests, []);
  }
  await deniedFrame('smoke-csp-denied', true);
  assert.equal(page.url(), origin + '/');
  checks.push('dashboard CSP rejects remote frame: error document, did-fail-load and zero target requests (attempted URL metadata is not committed content)');

  // Exercise the native boundary independently of the real dashboard's CSP.
  // This same-origin fixture is test-only and changes no shipped dashboard asset.
  await page.route(origin + '/electron-policy-fixture', route => route.fulfill({ contentType: 'text/html', body: '<h1>Native navigation policy fixture</h1>' }));
  await page.goto(origin + '/electron-policy-fixture');
  await deniedFrame('smoke-native-denied');
  await waitFor(() => main((_electron, url) => global.smokeExternal.includes(url), targetOrigin + '/smoke-native-denied'), 'native subframe browser handoff');
  assert.equal(page.url(), origin + '/electron-policy-fixture');
  assert.deepEqual(unexpectedNetwork, []);
  fs.writeFileSync(path.join(evidence, 'frame-documents.json'), JSON.stringify({ documents, failures: await main(() => global.smokeFailures), targetRequests }, null, 2));
  checks.push('native initial-frame policy rejects remote content without CSP; main/popup browser handoff; opaque frame allowed; blocked schemes/userinfo; no second window');

  await page.evaluate(origin => window.open(origin + '/s/electronfixture'), origin);
  await page.waitForURL(origin + '/s/electronfixture');
  const dashboardURL = origin + '/s/electronfixture?frame=1';
  await waitFor(() => Promise.resolve(page.frames().some(frame => frame.url() === dashboardURL)), 'real session frame');
  const dashboard = page.frame({ url: dashboardURL });
  await dashboard.locator('#grid:not([inert]) #ctext').waitFor({ state: 'attached' });
  const requests = [];
  // Assert the real dashboard link-menu's HTTP request, not an OS app launch.
  await page.route('**/s/electronfixture/fs/act', async route => { requests.push({ url: route.request().url(), body: route.request().postDataJSON() }); await route.fulfill({ status: 204 }); });
  await dashboard.evaluate(project => {
    const a = document.createElement('a'); a.id = 'smoke-file'; a.textContent = 'fixture.md';
    a.href = '/s/electronfixture/file?path=fixture.md'; a.dataset.wl = 'file'; a.dataset.rel = 'fixture.md'; a.dataset.abs = project + '/fixture.md'; document.body.append(a);
  }, path.join(daemonRoot, 'project'));
  await dashboard.locator('#smoke-file').click();
  await dashboard.locator('.lmenu button').filter({ hasText: /Open with system/i }).click();
  await waitFor(() => Promise.resolve(requests.length === 1), 'open with system fs/act request');
  assert.deepEqual(requests[0].body, { action: 'open', root: path.join(daemonRoot, 'project'), path: 'fixture.md' });
  checks.push('real dashboard desktop file-link menu sends fs/act open (request intercepted before OS launch)');
  await attachImageDraft(page.frameLocator('iframe.sframe.on'));
  await page.screenshot({ path: path.join(evidence, 'electron-dashboard-draft.png') });

  // Second launch shares only this test's profile, focuses the first, and exits.
  await main(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].minimize());
  const secondLog = fs.openSync(path.join(evidence, 'second-launch.log'), 'w');
  const secondLaunchedAt = Date.now();
  const secondEnv = { ...fixtureEnv, WS_CACHE_HOME: shellRoot };
  delete secondEnv.ELECTRON_RUN_AS_NODE;
  const second = spawn(executablePath, [`--user-data-dir=${path.join(temp, 'profile')}`], { env: secondEnv, stdio: ['ignore', secondLog, secondLog] });
  recordLaunch(second, secondLaunchedAt);
  await waitFor(() => Promise.resolve(second.exitCode !== null || second.signalCode !== null), 'second launch exit');
  assert.equal(second.exitCode, 0);
  await waitFor(() => main(({ BrowserWindow }) => !BrowserWindow.getAllWindows()[0].isMinimized() && global.smokeFocus > 0), 'second-instance focus');
  assert.equal(await main(({ BrowserWindow }) => BrowserWindow.getAllWindows().length), 1);
  scan('second launch exited');
  checks.push('built second launch exits and restores/focuses the existing sole window');
  remember(applicationIdentity);
  const firstProcess = application.process();
  // The stub would choose Stay if the durable renderer still raised a veto.
  await main(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].close()).catch(() => {});
  await waitFor(() => Promise.resolve(firstProcess.exitCode !== null || firstProcess.signalCode !== null), 'recovery-capable X-close without a prompt');
  application = null;
  assert.equal((await (await fetch(origin + '/health')).json()).pid, ep.pid);
  assert.deepEqual(fs.readFileSync(endpoint), endpointBefore);
  checks.push('committed image draft closes immediately and leaves the daemon running');
  scan('recovery-capable app exited');

  page = await launch();
  await page.waitForURL(origin + '/');
  assert(Math.abs(await factor() - 1) < 0.001);
  await assertImageDraft(await draftFrame(page, origin));
  await page.screenshot({ path: path.join(evidence, 'electron-dashboard-restored.png') });
  checks.push('same profile restores draft text and the original image bytes');

  await openLegacyGuard(page, origin);
  await main(({ dialog }) => {
    global.smokeDialogs = []; global.smokeChoice = 0;
    dialog.showMessageBoxSync = (_window, options) => { global.smokeDialogs.push(options); return global.smokeChoice; };
  });
  await main(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].close());
  await waitFor(() => main(() => global.smokeDialogs.length === 1), 'legacy X-close Stay/Leave dialog');
  assert.equal(await main(({ BrowserWindow }) => BrowserWindow.getAllWindows().length), 1);
  fs.writeFileSync(path.join(evidence, 'close-dialog.json'), JSON.stringify(await main(() => global.smokeDialogs[0]), null, 2));
  assert.match(await page.locator('textarea').inputValue(), /unpersisted image/);
  remember(applicationIdentity);
  const freshProcess = application.process();
  await main(({ BrowserWindow }) => { global.smokeChoice = 1; BrowserWindow.getAllWindows()[0].close(); }).catch(() => {});
  await waitFor(() => Promise.resolve(freshProcess.exitCode !== null || freshProcess.signalCode !== null), 'legacy Leave closes the app');
  application = null;
  assert.equal((await (await fetch(origin + '/health')).json()).pid, ep.pid);
  checks.push('legacy renderer keeps the native Stay/Leave fallback for a held image');

}
(async () => {
  let failure;
  try { await run(); }
  catch (error) { failure = error; console.error(error.stack); }
  finally {
    if (application) {
      remember(applicationIdentity);
      await application.evaluate(({ app }) => app.exit()).catch(() => {});
      await application.close().catch(() => {});
    }
    if (daemon && daemon.exitCode === null && signalIfCurrent(daemonIdentity)) {
      await once(daemon, 'exit');
    }
    target.closeAllConnections();
    if (target.listening) await new Promise(resolve => target.close(resolve));
    // Every candidate was reached from a recorded launch identity in creation
    // order. Re-query each PID immediately before its signal, not once per batch.
    for (const p of scan('before cleanup')) signalIfCurrent(p);
    await waitFor(() => Promise.resolve(scan('final').length === 0), 'owned process exit');
    fs.writeFileSync(path.join(evidence, 'summary.json'), JSON.stringify({ platform: process.platform, checks, dimensions, failure: failure?.message || null, owned: [...owned.values()], temp }, null, 2));
    fs.rmSync(temp, { recursive: true, force: true });
  }
  console.log(JSON.stringify({ checks, dimensions, failure: failure?.message || null, evidence }, null, 2));
  if (failure) process.exitCode = 1;
})();

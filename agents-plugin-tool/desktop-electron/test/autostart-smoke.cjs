'use strict';
// CI-only: never run the packaged app or a real daemon on the owner's PC.
if (process.env.GITHUB_ACTIONS !== 'true' || process.platform !== 'win32') {
  throw new Error('Packaged autostart smoke is Windows CI-only');
}
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawn, execFileSync } = require('node:child_process');
const { inspect, shutdown } = require('../endpoint');
const { builtPaths } = require('./host.cjs');
const root = path.resolve(__dirname, '..');
const { executablePath } = builtPaths(root);
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-autostart-'));
const cache = path.join(temp, 'cache'), profile = path.join(temp, 'profile');
const evidence = path.join(root, 'dist/autostart-evidence');
fs.mkdirSync(evidence, { recursive: true });
const log = fs.openSync(path.join(evidence, 'app.log'), 'a');
let app, observed;
const checks = [];
async function waitFor(check, label, timeout = 60000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const value = await check();
    if (value) return value;
    await new Promise(resolve => setTimeout(resolve, 200));
  }
  throw new Error(`Timed out: ${label}`);
}
function browserAddress() {
  // Electron's app userData and Chromium's --user-data-dir can differ.
  // The browser endpoint printed by this owned process is authoritative.
  const matches = fs.readFileSync(path.join(evidence, 'app.log'), 'utf8').matchAll(/DevTools listening on (ws:\/\/127\.0\.0\.1:\d+\/devtools\/browser\/[^\s]+)/g);
  return [...matches].at(-1)?.[1];
}
async function targets() {
  try {
    const address = browserAddress();
    if (!address) return [];
    return await (await fetch(`http://127.0.0.1:${new URL(address).port}/json/list`)).json();
  } catch { return []; }
}
async function launch() {
  fs.rmSync(path.join(profile, 'DevToolsActivePort'), { force: true });
  const env = { ...process.env, WS_CACHE_HOME: cache, WS_CONFIG_HOME: path.join(temp, 'config'),
    WS_MCP_HOLDER_PYTHON: path.join(temp, 'python-does-not-exist.exe') };
  delete env.ELECTRON_RUN_AS_NODE;
  app = spawn(executablePath, [`--user-data-dir=${profile}`, '--remote-debugging-port=0'], {
    env, stdio: ['ignore', log, log], windowsHide: true,
  });
  app.on('error', error => { console.error(error); });
  observed = await waitFor(async () => {
    assert.equal(app.exitCode, null, 'packaged app exited before readiness');
    try { return await inspect(cache); } catch { return null; }
  }, 'app automatically starts a healthy daemon');
  await waitFor(async () => (await targets()).some(target => target.type === 'page' &&
    target.url.startsWith(observed.url)), 'dashboard document reaches the daemon origin');
}
async function closeApp() {
  if (!app || app.exitCode !== null) return;
  const socket = new WebSocket(browserAddress());
  try {
    await new Promise((resolve, reject) => {
      socket.addEventListener('open', resolve, { once: true });
      socket.addEventListener('error', reject, { once: true });
    });
    socket.send(JSON.stringify({ id: 1, method: 'Browser.close' }));
    await waitFor(() => app.exitCode !== null, 'app closes', 10000);
  } finally { socket.close(); }
}
(async () => {
  let failure;
  try {
    assert(!fs.existsSync(path.join(cache, 'dashboard/endpoint.json')));
    await launch();
    const first = observed;
    const daemonExe = execFileSync('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command',
      `(Get-CimInstance Win32_Process -Filter "ProcessId = ${first.endpoint.pid}").ExecutablePath`], { encoding: 'utf8' }).trim();
    const hash = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
    const bundled = path.join(path.dirname(executablePath), 'resources/app.asar.unpacked/daemon/ws-mcp.exe');
    assert.equal(hash(daemonExe), hash(bundled), 'running daemon must be the bundled binary, not a global plugin');
    checks.push('empty isolated cache + missing Python: app started bundled daemon and reached dashboard');
    const startLog = fs.readFileSync(path.join(profile, 'daemon-start.log'), 'utf8');
    assert.match(startLog, /daemon spawn mode=breakaway/);
    if (startLog.includes('WS_DAEMON_BREAKAWAY_DENIED=5')) {
      assert.match(startLog, /daemon spawn mode=job-lifetime/);
      checks.push('numeric breakaway denial retried successfully with explicit job lifetime');
    }
    await closeApp();
    assert.equal((await inspect(cache)).endpoint.pid, first.endpoint.pid);
    checks.push('daemon survives app exit; job-lifetime mode may still end when the containing job closes');
    await launch();
    assert.equal(observed.endpoint.pid, first.endpoint.pid);
    assert.equal(observed.health.started_at, first.health.started_at);
    checks.push('second packaged launch attaches to same compatible daemon');
    await closeApp();
  } catch (error) { failure = error; console.error(error.stack); }
  finally {
    const pages = await targets();
    fs.writeFileSync(path.join(evidence, 'targets.json'), JSON.stringify(pages, null, 2));
    for (const page of pages) {
      if (!page.url.startsWith('file:')) continue;
      const diagnostic = new URL(page.url).searchParams;
      const diagnosticLog = diagnostic.get('log');
      if (diagnosticLog?.startsWith('Error log: ')) {
        const source = diagnosticLog.slice('Error log: '.length);
        if (path.basename(source) === 'daemon-start.log' && fs.existsSync(source)) {
          fs.copyFileSync(source, path.join(evidence, 'daemon-start.log'));
        }
      }
    }
    // Only this test's isolated endpoint is eligible for authenticated cleanup.
    try { await closeApp(); } catch { if (app?.exitCode === null) app.kill(); }
    try { if (observed) await shutdown(cache, observed); } catch (error) { console.error('Cleanup:', error.message); }
    for (const file of ['daemon-start.log', 'startup-error.log']) {
      const source = path.join(profile, file);
      if (fs.existsSync(source)) fs.copyFileSync(source, path.join(evidence, file));
    }
    fs.writeFileSync(path.join(evidence, 'summary.json'), JSON.stringify({ checks, endpoint: observed?.endpoint,
      health: observed?.health, failure: failure?.message || null }, null, 2));
    fs.closeSync(log);
    // Windows may still hold the staged binary briefly during authenticated drain.
    await waitFor(() => {
      try { fs.rmSync(temp, { recursive: true, force: true }); return true; } catch { return false; }
    }, 'isolated smoke cleanup', 10000);
  }
  console.log(JSON.stringify({ checks, failure: failure?.message || null }));
  if (failure) process.exitCode = 1;
})().catch(error => { console.error(error); process.exitCode = 1; });

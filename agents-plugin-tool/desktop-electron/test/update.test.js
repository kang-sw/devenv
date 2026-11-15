'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');
const crypto = require('node:crypto');
const { execFileSync, spawn, spawnSync } = require('node:child_process');
const u = require('../update');

const tmp = prefix => fs.mkdtempSync(path.join(os.tmpdir(), `werkschaft-update-${prefix}-`));
// Async, so the event loop reaps a waited-on child (a zombie still answers kill -0).
function runAsync(command, args) {
  return new Promise(resolve => {
    const child = spawn(command, args);
    let stdout = '', stderr = '';
    child.stdout.on('data', d => { stdout += d; });
    child.stderr.on('data', d => { stderr += d; });
    child.on('close', status => resolve({ status, stdout, stderr }));
  });
}
const sha = data => crypto.createHash('sha256').update(data).digest('hex');

test('version compare is numeric x.y.z and ignores prereleases', () => {
  assert.deepEqual(u.parseVersion('v0.50.0'), [0, 50, 0]);
  assert.equal(u.parseVersion('v0.51.0-rc.1'), null);
  assert.equal(u.isNewer('v0.50.0', '0.49.9'), true);
  assert.equal(u.isNewer('v0.50.10', '0.50.9'), true);
  assert.equal(u.isNewer('v1.0.0', '0.99.99'), true);
  assert.equal(u.isNewer('v0.50.0', '0.50.0'), false);
  assert.equal(u.isNewer('v0.49.10', '0.50.0'), false);
  assert.equal(u.isNewer('v0.51.0-rc.1', '0.50.0'), false);
  assert.equal(u.isNewer('garbage', '0.50.0'), false);
});

test('asset and SUMS selection per platform; anything missing means no update', () => {
  const assets = names => names.map(name => ({ name, browser_download_url: `https://example.test/${name}` }));
  const all = assets(['Werkschaft-darwin-arm64.zip', 'Werkschaft-win32-x64.zip', u.SUMS]);
  const mac = u.selectRelease({ tag_name: 'v0.51.0', assets: all }, { platform: 'darwin', arch: 'arm64', current: '0.50.0' });
  assert.deepEqual(mac, { version: '0.51.0', asset: 'Werkschaft-darwin-arm64.zip',
    zipURL: 'https://example.test/Werkschaft-darwin-arm64.zip', sumsURL: `https://example.test/${u.SUMS}` });
  const win = u.selectRelease({ tag_name: 'v0.51.0', assets: all }, { platform: 'win32', arch: 'x64', current: '0.50.0' });
  assert.equal(win.asset, 'Werkschaft-win32-x64.zip');
  const reason = (release, platform = 'darwin', arch = 'arm64') => u.selectRelease(release, { platform, arch, current: '0.50.0' }).reason;
  assert.match(reason({ tag_name: 'v0.51.0', assets: assets([u.SUMS]) }), /no Werkschaft-darwin-arm64.zip/);
  assert.match(reason({ tag_name: 'v0.51.0', assets: assets(['Werkschaft-darwin-arm64.zip']) }), /no Werkschaft-desktop-SHA256SUMS/);
  assert.match(reason({ tag_name: 'v0.51.0', assets: all }, 'darwin', 'x64'), /no Werkschaft-darwin-x64.zip/);
  assert.match(reason({ tag_name: 'v0.50.0', assets: all }), /not newer/);
  assert.match(reason({ tag_name: 'v0.52.0', prerelease: true, assets: all }), /prerelease/);
  assert.match(reason({ tag_name: 'v0.52.0', draft: true, assets: all }), /draft/);
  const sums = u.parseSums(`${'a'.repeat(64)}  Werkschaft-darwin-arm64.zip\r\n${'B'.repeat(64)} *Werkschaft-win32-x64.zip\nnoise\n`);
  assert.equal(sums.get('Werkschaft-darwin-arm64.zip'), 'a'.repeat(64));
  assert.equal(sums.get('Werkschaft-win32-x64.zip'), 'b'.repeat(64));
  assert.equal(sums.size, 2);
});

test('only https, or http to loopback, is fetched', () => {
  assert.ok(u.allowedURL('https://api.github.com/x'));
  assert.ok(u.allowedURL('http://127.0.0.1:9/x'));
  assert.ok(u.allowedURL('http://localhost:9/x'));
  assert.equal(u.allowedURL('http://example.com/x'), null);
  assert.equal(u.allowedURL('https://user:pw@example.com/x'), null);
  assert.equal(u.allowedURL('file:///etc/passwd'), null);
});

// A local release feed: /repos/<repo>/releases/latest, the assets, and a
// redirect hop like GitHub's asset download.
async function feed(files, release) {
  const hits = [];
  const server = http.createServer((req, res) => {
    hits.push({ url: req.url, agent: req.headers['user-agent'] });
    if (req.url === `/repos/${u.REPO}/releases/latest`) {
      if (!release()) { res.writeHead(500); return res.end(); }
      res.writeHead(200, { 'content-type': 'application/json' });
      return res.end(JSON.stringify(release()));
    }
    if (req.url.startsWith('/download/')) {
      res.writeHead(302, { location: `/blob/${req.url.slice('/download/'.length)}` });
      return res.end();
    }
    const name = decodeURIComponent(req.url.slice('/blob/'.length));
    if (!req.url.startsWith('/blob/') || !(name in files)) { res.writeHead(404); return res.end(); }
    res.writeHead(200);
    res.end(files[name]);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const base = `http://127.0.0.1:${server.address().port}`;
  const asset = name => ({ name, browser_download_url: `${base}/download/${encodeURIComponent(name)}` });
  return { base, asset, hits, close: () => new Promise(resolve => server.close(resolve)) };
}

// A fake installed app: <root>/Applications/Werkschaft.app/Contents/MacOS/Werkschaft.
function fakeApp(root, marker) {
  const app = path.join(root, 'Werkschaft.app');
  fs.mkdirSync(path.join(app, 'Contents/MacOS'), { recursive: true });
  fs.writeFileSync(path.join(app, 'Contents/MacOS/Werkschaft'), marker);
  return app;
}

test('feed override, skip persistence, manual check and up-to-date / failure statuses', async () => {
  const userData = tmp('state');
  let release = null;
  const zip = Buffer.from('zip bytes');
  const files = { 'Werkschaft-darwin-arm64.zip': zip };
  const server = await feed(files, () => release);
  try {
    const sent = [];
    const strip = list => list.map(({ phase, ...rest }) => rest);
    const make = () => u.createUpdater({ version: '0.50.0', userData, packaged: false, platform: 'darwin', arch: 'arm64',
      feed: server.base + '/', send: s => sent.push(s), log: () => {} });
    let updater = make();
    assert.deepEqual(updater.phase(), { phase: 'unchecked', version: null, skipped: false });
    // Feed down: automatic checks stay silent, a manual check says so.
    assert.equal(await updater.check(false), 'error');
    assert.deepEqual(sent, []);
    assert.equal(await updater.check(true), 'error');
    assert.deepEqual(sent.pop(), { state: 'error', manual: true, phase: { phase: 'unchecked', version: null, skipped: false } });
    release = { tag_name: 'v0.50.0', assets: [] };
    assert.equal(await updater.check(false), 'current');
    assert.deepEqual(sent, []);
    assert.equal(await updater.check(true), 'current');
    assert.deepEqual(sent.pop(), { state: 'current', version: '0.50.0', manual: true, phase: { phase: 'none', version: null, skipped: false } });
    // A newer tag whose SUMS lacks this platform's line is no update.
    files[u.SUMS] = `${sha(zip)}  Werkschaft-win32-x64.zip\n`;
    release = { tag_name: 'v0.51.0', assets: [server.asset('Werkschaft-darwin-arm64.zip'), server.asset(u.SUMS)] };
    assert.equal(await updater.check(true), 'current');
    sent.length = 0;
    files[u.SUMS] = `${sha(zip)}  Werkschaft-darwin-arm64.zip\n`;
    assert.equal(await updater.check(false), 'available');
    assert.deepEqual(sent.pop(), { state: 'available', version: '0.51.0', current: '0.50.0', skipped: false, manual: false,
      phase: { phase: 'available', version: '0.51.0', skipped: false } });
    assert.ok(server.hits.every(hit => hit.agent === 'werkschaft-desktop'));
    // Skip persists across restarts and silences that version's automatic toast only.
    assert.equal(updater.skip('0.50.9'), false);
    assert.equal(updater.skip('0.51.0'), true);
    assert.deepEqual(JSON.parse(fs.readFileSync(path.join(userData, 'update-state.json'))), { skipped: '0.51.0' });
    assert.deepEqual(updater.phase(), { phase: 'available', version: '0.51.0', skipped: true });
    updater = make();
    assert.equal(await updater.check(false), 'skipped');
    assert.deepEqual(sent, []);
    assert.equal(await updater.check(true), 'available');
    assert.deepEqual(strip([sent.pop()]), [{ state: 'available', version: '0.51.0', current: '0.50.0', skipped: true, manual: true }]);
    // A newer version than the skipped one shows again.
    release = { ...release, tag_name: 'v0.52.0' };
    assert.equal(await updater.check(false), 'available');
    assert.equal(sent.pop().skipped, false);
    // A dev (unpackaged) run reports instead of downloading.
    assert.equal(await updater.download(), 'download-error');
    assert.match(sent.pop().message, /not a packaged app/);
  } finally { await server.close(); fs.rmSync(userData, { recursive: true, force: true }); }
});

test('download verifies the checksum, stages the extracted app, and the quit path swaps it in', { skip: process.platform !== 'darwin' }, async () => {
  const root = tmp('flow');
  const apps = path.join(root, 'Applications'), userData = path.join(root, 'userData'), build = path.join(root, 'build');
  fs.mkdirSync(apps); fs.mkdirSync(build);
  const installed = fakeApp(apps, 'old');
  fakeApp(build, 'new');
  const zipFile = path.join(root, 'Werkschaft-darwin-arm64.zip');
  execFileSync('/usr/bin/ditto', ['-c', '-k', '--keepParent', path.join(build, 'Werkschaft.app'), zipFile]);
  const zip = fs.readFileSync(zipFile);
  const files = { 'Werkschaft-darwin-arm64.zip': zip, [u.SUMS]: `${'0'.repeat(64)}  Werkschaft-darwin-arm64.zip\n` };
  const release = { tag_name: 'v0.51.0', assets: [] };
  const server = await feed(files, () => release);
  release.assets = [server.asset('Werkschaft-darwin-arm64.zip'), server.asset(u.SUMS)];
  try {
    const sent = [], detached = [];
    const updater = u.createUpdater({ version: '0.50.0', userData, packaged: true, platform: 'darwin', arch: 'arm64',
      execPath: path.join(installed, 'Contents/MacOS/Werkschaft'), pid: 999999, feed: server.base,
      send: s => sent.push(s), log: () => {}, detach: (command, args, log) => detached.push({ command, args, log }) });
    // Checksum mismatch: rejected, nothing staged, nothing to apply.
    assert.equal(await updater.check(false), 'available');
    assert.equal(await updater.download(), 'download-error');
    assert.equal(updater.canRestart(), false);
    assert.match(sent.at(-1).message, /checksum mismatch/);
    assert.equal(fs.existsSync(path.join(userData, 'update')), false);
    assert.equal(updater.applyOnQuit(), false);
    // Matching checksum: downloaded, verified, extracted and staged.
    files[u.SUMS] = `${sha(zip)}  Werkschaft-darwin-arm64.zip\n`;
    assert.equal(await updater.check(false), 'available');
    assert.equal(await updater.download(), 'ready');
    assert.deepEqual(sent.slice(-2), [{ state: 'downloading', version: '0.51.0', phase: { phase: 'available', version: '0.51.0', skipped: false } },
      { state: 'ready', version: '0.51.0', phase: { phase: 'ready', version: '0.51.0', skipped: false } }]);
    assert.equal(updater.canRestart(), true);
    const staged = JSON.parse(fs.readFileSync(path.join(userData, 'update/staged.json')));
    assert.equal(staged.version, '0.51.0');
    assert.equal(fs.readFileSync(path.join(staged.app, 'Contents/MacOS/Werkschaft'), 'utf8'), 'new');
    assert.equal(fs.existsSync(path.join(userData, 'update/Werkschaft-darwin-arm64.zip')), false);
    // A later check of the same version reports it ready, not available.
    assert.equal(await updater.check(false), 'ready');
    // Restart now asks for a relaunch; a plain quit does not.
    assert.equal(updater.applyOnQuit(true), true);
    assert.equal(detached.pop().args.at(-1), '1');
    // Quit: one detached /bin/sh helper, no relaunch.
    assert.equal(updater.applyOnQuit(), true);
    const [{ command, args, log }] = detached;
    assert.equal(command, '/bin/sh');
    assert.deepEqual(args.slice(1), ['999999', installed, staged.app, `${installed}-update-backup`, path.join(userData, 'update'), '0']);
    assert.equal(log, path.join(userData, 'update-apply.log'));
    const result = spawnSync(command, args, { encoding: 'utf8' });
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.equal(fs.readFileSync(path.join(installed, 'Contents/MacOS/Werkschaft'), 'utf8'), 'new');
    assert.equal(fs.existsSync(`${installed}-update-backup`), false);
    assert.equal(fs.existsSync(path.join(userData, 'update')), false);
    assert.deepEqual(fs.readdirSync(apps), ['Werkschaft.app']);
  } finally { await server.close(); fs.rmSync(root, { recursive: true, force: true }); }
});

// POSIX-only: it runs the /bin/sh helper and waits on /bin/sleep.
test('the sh helper waits for the app to exit, and restores the old app when the swap fails', { skip: process.platform === 'win32' }, async () => {
  const root = tmp('helper');
  try {
    const script = path.join(root, 'apply.sh');
    fs.writeFileSync(script, u.helperScript('darwin'));
    const installed = fakeApp(path.join(root, 'apps'), 'old');
    const stagedApp = fakeApp(path.join(root, 'staging', 'extract'), 'new');
    const app = spawn('/bin/sleep', ['0.6']);
    const started = Date.now();
    const [command, args] = u.helperCommand('darwin', script, { pid: app.pid, target: installed, staged: stagedApp,
      backup: `${installed}-update-backup`, staging: path.join(root, 'staging'), relaunch: false });
    const helper = await runAsync(command, args);
    assert.equal(helper.status, 0, helper.stdout + helper.stderr);
    assert.ok(Date.now() - started >= 500, 'swapped only after the app exited');
    assert.equal(fs.readFileSync(path.join(installed, 'Contents/MacOS/Werkschaft'), 'utf8'), 'new');
    // Failure: the staged app is gone, so the old one must come back intact.
    const [, failArgs] = u.helperCommand('darwin', script, { pid: 999999, target: installed, staged: path.join(root, 'missing.app'),
      backup: `${installed}-update-backup`, staging: path.join(root, 'staging'), relaunch: false });
    const failed = spawnSync('/bin/sh', failArgs, { encoding: 'utf8' });
    assert.equal(failed.status, 1);
    assert.match(failed.stdout, /restored the previous app/);
    assert.equal(fs.readFileSync(path.join(installed, 'Contents/MacOS/Werkschaft'), 'utf8'), 'new');
    assert.equal(fs.existsSync(`${installed}-update-backup`), false);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

test('helper and extraction commands for both platforms', () => {
  const opts = { pid: 42, target: 'C:\\Apps\\Werkschaft-win32-x64', staged: 'C:\\Users\\a\\AppData\\Roaming\\Werkschaft\\update\\extract\\Werkschaft-win32-x64',
    backup: 'C:\\Apps\\Werkschaft-win32-x64-update-backup', staging: 'C:\\Users\\a\\AppData\\Roaming\\Werkschaft\\update', relaunch: true };
  const [ps, psArgs] = u.helperCommand('win32', 'C:\\s\\apply.ps1', opts, { SystemRoot: 'C:\\Windows' });
  assert.equal(ps, path.join('C:\\Windows', 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe'));
  assert.deepEqual(psArgs, ['-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-WindowStyle', 'Hidden', '-File', 'C:\\s\\apply.ps1',
    '-AppPid', '42', '-Target', opts.target, '-Staged', opts.staged, '-Backup', opts.backup, '-Staging', opts.staging, '-Relaunch', '1']);
  const ps1 = u.helperScript('win32');
  assert.match(ps1, /^param\(\[int\]\$AppPid, \[string\]\$Target, \[string\]\$Staged, \[string\]\$Backup, \[string\]\$Staging, \[int\]\$Relaunch\)/);
  for (const needle of ['Wait-Process -Id $AppPid', 'Move-Item -LiteralPath $Target -Destination $Backup',
    'Move-Item -LiteralPath $Staged -Destination $Target', 'Move-Item -LiteralPath $Backup -Destination $Target',
    "Join-Path $Target 'Werkschaft.exe'"]) assert.ok(ps1.includes(needle), needle);
  const sh = u.helperScript('darwin');
  assert.match(sh, /^#!\/bin\/sh\n/);
  for (const needle of ['kill -0 "$pid"', 'mv "$target" "$backup"', 'mv "$staged" "$target"', 'mv "$backup" "$target"', '/usr/bin/open "$target"']) {
    assert.ok(sh.includes(needle), needle);
  }
  assert.deepEqual(u.helperCommand('darwin', '/s/apply.sh', { ...opts, relaunch: false })[1].at(-1), '0');
  assert.deepEqual(u.extractCommand('darwin', '/z.zip', '/d'), ['/usr/bin/ditto', ['-x', '-k', '/z.zip', '/d']]);
  assert.deepEqual(u.extractCommand('win32', 'C:\\z.zip', 'C:\\d', { SystemRoot: 'C:\\Windows' }),
    [path.join('C:\\Windows', 'System32', 'tar.exe'), ['-x', '-f', 'C:\\z.zip', '-C', 'C:\\d']]);
  // The darwin branch resolves with the host's path module, POSIX on a real Mac;
  // a Windows host would resolve the POSIX fixture to a drive path.
  if (process.platform !== 'win32') {
    assert.equal(u.installLocation('darwin', '/Applications/Werkschaft.app/Contents/MacOS/Werkschaft', true), '/Applications/Werkschaft.app');
  }
  assert.equal(u.installLocation('darwin', '/x/node_modules/electron/dist/Electron.app/Contents/MacOS/Electron', false), null);
  assert.equal(u.installLocation('win32', path.join('C:', 'Apps', 'W', 'Werkschaft.exe'), true), path.join('C:', 'Apps', 'W'));
  assert.match(u.applyProblem('darwin', '/private/var/folders/x/AppTranslocation/y/d/Werkschaft.app', '/u'), /Applications/);
  assert.match(u.applyProblem('darwin', null, '/u'), /not a packaged app/);
});

test('stagedApp finds the app inside an extracted archive on both layouts', () => {
  const root = tmp('stagedapp');
  try {
    assert.equal(u.stagedApp('darwin', root), null);
    const app = fakeApp(root, 'x');
    assert.equal(u.stagedApp('darwin', root), app);
    const win = path.join(root, 'Werkschaft-win32-x64');
    fs.mkdirSync(win);
    assert.equal(u.stagedApp('win32', root), null);
    fs.writeFileSync(path.join(win, 'Werkschaft.exe'), '');
    assert.equal(u.stagedApp('win32', root), win);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

test('an unwritable install keeps the stage and reports; a launch apply runs once per version', { skip: process.platform === 'win32' || process.getuid?.() === 0 }, () => {
  const root = tmp('launch');
  try {
    const apps = path.join(root, 'apps'), userData = path.join(root, 'userData');
    const installed = fakeApp(apps, 'old');
    const stage = fakeApp(path.join(userData, 'update', 'extract'), 'new');
    fs.writeFileSync(path.join(userData, 'update', 'staged.json'), JSON.stringify({ version: '0.51.0', app: stage }));
    const sent = [], detached = [];
    const make = () => u.createUpdater({ version: '0.50.0', userData, packaged: true, platform: 'darwin', arch: 'arm64',
      execPath: path.join(installed, 'Contents/MacOS/Werkschaft'), send: s => sent.push(s), log: () => {},
      detach: (command, args) => detached.push(args) });
    fs.chmodSync(apps, 0o555);
    try {
      assert.match(u.applyProblem('darwin', installed, path.join(userData, 'update')), /not writable/);
      assert.equal(make().applyOnQuit(), false);
      assert.ok(fs.existsSync(stage), 'the verified stage is kept');
      assert.equal(make().applyOnLaunch(), false);
      assert.equal(sent.at(-1).state, 'apply-error');
      assert.match(sent.at(-1).message, /not writable/);
      assert.ok(fs.existsSync(stage), 'an unwritable folder at launch keeps the stage and spends no attempt');
      assert.equal(detached.length, 0);
    } finally { fs.chmodSync(apps, 0o755); }
    assert.equal(make().applyOnLaunch(), true);
    assert.equal(detached.at(-1).at(-1), '1', 'a launch apply relaunches');
    // The relaunched app is still the old one: the swap failed, so give up on it.
    assert.equal(make().applyOnLaunch(), false);
    assert.equal(fs.existsSync(path.join(userData, 'update')), false);
    assert.equal(sent.at(-1).state, 'apply-error');
    assert.equal(detached.length, 1);
    assert.equal(make().applyOnLaunch(), false);
    assert.equal(JSON.parse(fs.readFileSync(path.join(userData, 'update-state.json'))).launchAttempt, null);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

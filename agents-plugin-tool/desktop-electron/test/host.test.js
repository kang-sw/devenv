'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { build, archive, SUMS } = require('../scripts/build.cjs');
const { builtPaths, processes, launchedRoot, descendants, signalIfCurrent } = require('./host.cjs');
const pkg = require('../package.json');
const { fixture } = require('./daemon-fixture.cjs');

test('built smoke paths select Windows exe/daemon and retain the macOS app', () => {
  assert.deepEqual(builtPaths('/fixture', 'win32', 'arm64'), {
    executablePath: path.join('/fixture', 'dist/Werkschaft-win32-x64/Werkschaft.exe'),
    daemonPath: path.join('/fixture', 'dist/smoke-daemon.exe'),
  });
  assert.deepEqual(builtPaths('/fixture', 'darwin', 'arm64'), {
    executablePath: path.join('/fixture', 'dist/Werkschaft-darwin-arm64/Werkschaft.app/Contents/MacOS/Werkschaft'),
    daemonPath: path.join('/fixture', 'dist/smoke-daemon'),
  });
  assert.throws(() => builtPaths('/fixture', 'linux'), /darwin and win32 only/);
});

test('Windows process inventory preserves PID, parent and creation identity without signalling', () => {
  const expected = [{ pid: 20, ppid: 10, start: '639000704960000000', command: 'Werkschaft.exe' }];
  const result = processes('win32', (command, args) => {
    assert.equal(command, 'powershell.exe');
    assert(args.includes('-NonInteractive'));
    assert.match(args.at(-1), /Get-CimInstance Win32_Process/);
    assert.match(args.at(-1), /CreationDate.ToUniversalTime\(\).Ticks.ToString/);
    assert(!/Stop-Process|taskkill/.test(args.at(-1)));
    return JSON.stringify(expected);
  });
  assert.deepEqual(result, expected);
  assert.deepEqual(processes('win32', () => ''), []);
});

test('macOS process inventory retains the original ps creation identity', () => {
  assert.deepEqual(processes('darwin', command => {
    assert.equal(command, 'ps');
    return '  20  10 Sun Nov 15 12:34:56 2026 /fixture/Werkschaft\n';
  }), [{ pid: 20, ppid: 10, start: 'Sun Nov 15 12:34:56 2026', command: '/fixture/Werkschaft' }]);
});

const launchTime = Date.UTC(2026, 10, 15, 12, 0, 0);
function processRecord(pid, ppid, offset = 0) {
  return { pid, ppid, start: String(621355968000000000n + BigInt(launchTime + offset) * 10000n), command: 'test-only process' };
}
test('a launched root is recorded once from a live child, parent and launch-time boundary', () => {
  const child = { pid: 100, exitCode: null, signalCode: null };
  const root = processRecord(100, 99);
  const identity = launchedRoot(child, [root], launchTime, 99);
  assert.deepEqual(identity, root);
  assert(Object.isFrozen(identity));
  assert.equal(launchedRoot(child, [processRecord(100, 99, -1)], launchTime, 99), null);
  assert.equal(launchedRoot(child, [root], launchTime, 98), null);
  assert.equal(launchedRoot({ ...child, exitCode: 0 }, [root], launchTime, 99), null);
  assert.deepEqual(descendants(identity, [processRecord(100, 99, 10), processRecord(200, 100, 20)]), [], 'a reused root PID must not be recaptured');
});
test('stale Windows parent IDs cannot adopt older unrelated processes or their children', () => {
  const root = processRecord(100, 99);
  const stale = processRecord(200, 100, -1);
  const staleChild = processRecord(201, 200, 1);
  assert.deepEqual(descendants(root, [staleChild, stale, root]), [root]);
  // Preserve ordering even inside the same millisecond (CIM has 100 ns ticks).
  const justOlder = { ...stale, start: String(BigInt(root.start) - 1n) };
  assert.deepEqual(descendants(root, [root, justOlder]), [root]);
});
test('a legitimate grandchild chain is owned only in parent/root creation order', () => {
  const root = processRecord(100, 99);
  const child = processRecord(200, 100, 10), grandchild = processRecord(300, 200, 20);
  const olderThanParent = processRecord(400, 200, 5);
  const unknownBirth = { ...processRecord(500, 100), start: '' };
  assert.deepEqual(descendants(root, [grandchild, olderThanParent, unknownBirth, child, root]), [root, child, grandchild]);
  const macRoot = { ...root, start: 'Sun Nov 15 12:00:00 2026' };
  const macChild = { ...child, start: 'Sun Nov 15 12:00:01 2026' };
  assert.deepEqual(descendants(macRoot, [macChild, macRoot]), [macRoot, macChild]);
});
test('cleanup revalidates each PID/creation identity immediately before its signal', () => {
  const root = processRecord(100, 99), child = processRecord(200, 100, 10);
  let snapshot = [root, child];
  const signals = [];
  const signal = (pid, value) => {
    signals.push([pid, value]);
    // The next candidate's PID was reused after the previous process signalled.
    snapshot = [processRecord(200, 99, 30)];
  };
  assert.equal(signalIfCurrent(root, () => snapshot, signal), true);
  assert.equal(signalIfCurrent(child, () => snapshot, signal), false);
  assert.equal(signalIfCurrent(child, () => [], signal), false);
  assert.deepEqual(signals, [[100, 'SIGTERM']]);
  assert.equal(signalIfCurrent(child, () => [child], () => { throw Object.assign(new Error('gone'), { code: 'ESRCH' }); }), false);
});
test('Windows packaging consumes only the pinned cached x64 zip and ships the approved brand icons', async t => {
  const { bundled } = fixture(t);
  bundled.metadata.version = bundled.contract.plugin_version;
  bundled.save(bundled.binary + '.metadata.json', bundled.metadata);
  const daemonInput = bundled.root;
  const cache = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-cache-test-'));
  let staged;
  const out = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-out-test-'));
  const runs = [];
  const run = (command, args) => { runs.push([command, args]); fs.writeFileSync(args.find(a => a.endsWith('.zip')), 'zip bytes'); };
  try {
    const nested = path.join(cache, 'checksum');
    fs.mkdirSync(nested);
    fs.writeFileSync(path.join(nested, `electron-v${pkg.devDependencies.electron}-win32-x64.zip`), 'test double');
    await build({ platform: 'win32', arch: 'arm64', daemonInput, cache, out, run, packageApp: async options => {
      assert.equal(options.platform, 'win32');
      assert.equal(options.arch, 'x64');
      assert.equal(options.electronVersion, pkg.devDependencies.electron);
      assert.equal(options.electronZipDir, nested);
      assert.deepEqual(options.asar, { unpackDir: 'daemon' }, 'the complete daemon input must be outside app.asar');
      assert.equal(options.icon, path.join(options.dir, 'icons/icon.ico'));
      assert.equal(options.appBundleId, 'dev.werkschaft.desktop');
      staged = options.dir;
      assert.deepEqual(fs.readdirSync(staged).sort(), ['connection.html', 'connection.js', 'daemon', 'daemon-controller.js', 'daemon-selection.js', 'daemon-stage.js', 'daemon-version.js', 'dashboard-preload.js', 'endpoint.js', 'icons', 'main.js', 'package.json', 'policy.js', 'python-missing.html', 'python.js', 'recovery.js', 'shell-log.js', 'titlebar-preload.js', 'titlebar-renderer.js', 'titlebar.html', 'titlebar.js', 'update.js']);
      assert.deepEqual(fs.readdirSync(path.join(staged, 'icons')).sort(), ['icon.ico', 'icon.png']);
      for (const file of ['icon.png', 'icon.ico']) {
        assert.deepEqual(fs.readFileSync(path.join(staged, 'icons', file)), fs.readFileSync(path.resolve(__dirname, '../icons', file)));
      }
      assert.match(fs.readFileSync(path.join(staged, 'titlebar.html'), 'utf8'), /src="icons\/icon.png"/);
      assert.match(fs.readFileSync(path.join(staged, 'main.js'), 'utf8'), /icon: path.join\(__dirname, 'icons\/icon.ico'\)/);
      const stagedPackage = JSON.parse(fs.readFileSync(path.join(staged, 'package.json')));
      assert.equal(stagedPackage.version, bundled.contract.plugin_version);
      assert.equal(options.appVersion, bundled.contract.plugin_version);
      assert.equal(options.buildVersion, bundled.contract.plugin_version);
      assert.equal(stagedPackage.author, 'kang-sw');
      return ['test-only packager output'];
    } });
    assert.equal(fs.existsSync(staged), false);
    const zip = path.join(out, 'Werkschaft-win32-x64.zip');
    assert.equal(runs.length, 1);
    assert.deepEqual(runs[0][1].slice(0, 4), process.platform === 'win32' ? ['-a', '-c', '-f', zip] : ['-c', '-k', '--keepParent', path.join(out, 'Werkschaft-win32-x64')]);
    assert.equal(fs.readFileSync(path.join(out, SUMS), 'utf8'),
      `${require('node:crypto').createHash('sha256').update('zip bytes').digest('hex')}  Werkschaft-win32-x64.zip\n`);
    await assert.rejects(build({ platform: 'win32', daemonInput, cache: path.join(cache, 'absent'), packageApp: () => assert.fail('must never package/download') }), /Offline prerequisite missing.*win32-x64.zip; no download attempted/);
    await assert.rejects(build({ platform: 'linux', cache }), /darwin and win32 only/);
  } finally { fs.rmSync(cache, { recursive: true, force: true }); fs.rmSync(out, { recursive: true, force: true }); }
});
test('Windows packaging retains a validated canary suffix and refuses a mismatched contract core', async t => {
  const { bundled } = fixture(t);
  bundled.metadata.version = '0.50.1-canary.12';
  bundled.save(bundled.binary + '.metadata.json', bundled.metadata);
  const cache = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-cache-test-'));
  const out = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-out-test-'));
  t.after(() => { fs.rmSync(cache, { recursive: true, force: true }); fs.rmSync(out, { recursive: true, force: true }); });
  fs.writeFileSync(path.join(cache, `electron-v${pkg.devDependencies.electron}-win32-x64.zip`), 'test double');
  const run = (_command, args) => fs.writeFileSync(args.find(arg => arg.endsWith('.zip')), 'zip bytes');
  let packaged = 0;
  const packageApp = async options => {
    packaged++;
    assert.equal(options.appVersion, '0.50.1-canary.12');
    assert.equal(options.buildVersion, '0.50.1-canary.12');
    assert.equal(JSON.parse(fs.readFileSync(path.join(options.dir, 'package.json'))).version, '0.50.1-canary.12');
    assert.equal(JSON.parse(fs.readFileSync(path.join(options.dir, 'daemon', 'runtime.json'))).plugin_version, '0.50.1');
    return ['test-only packager output'];
  };
  await build({ platform: 'win32', daemonInput: bundled.root, cache, out, run, packageApp });
  assert.equal(packaged, 1);
  bundled.contract.plugin_version = '0.50.2';
  bundled.save(bundled.contractFile, bundled.contract);
  await assert.rejects(build({ platform: 'win32', daemonInput: bundled.root, cache, out, run, packageApp }), /daemon version\/contract mismatch/);
  assert.equal(packaged, 1, 'a core mismatch must fail before packaging');
});
test('release archive: macOS ditto keeps the .app parent, Windows bsdtar, and SUMS replaces only its own line', () => {
  const out = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-archive-test-'));
  try {
    const runs = [];
    const run = (command, args) => { runs.push([command, args]); fs.writeFileSync(args.find(a => a.endsWith('.zip')), command); };
    fs.writeFileSync(path.join(out, SUMS), `${'f'.repeat(64)}  Werkschaft-win32-x64.zip\n${'e'.repeat(64)}  Werkschaft-darwin-arm64.zip\n`);
    const mac = archive({ out, platform: 'darwin', arch: 'arm64', host: 'darwin', run });
    assert.deepEqual(runs[0], ['/usr/bin/ditto', ['-c', '-k', '--keepParent', path.join(out, 'Werkschaft-darwin-arm64', 'Werkschaft.app'), mac.zip]]);
    const win = archive({ out, platform: 'win32', arch: 'x64', host: 'win32', run, env: { SystemRoot: 'C:\\Windows' } });
    assert.deepEqual(runs[1], [path.join('C:\\Windows', 'System32', 'tar.exe'), ['-a', '-c', '-f', win.zip, '-C', out, 'Werkschaft-win32-x64']]);
    assert.equal(fs.readFileSync(path.join(out, SUMS), 'utf8'),
      `${mac.hash}  Werkschaft-darwin-arm64.zip\n${win.hash}  Werkschaft-win32-x64.zip\n`);
  } finally { fs.rmSync(out, { recursive: true, force: true }); }
});

'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { build } = require('../scripts/build.cjs');
const { builtPaths, processes } = require('./host.cjs');
const pkg = require('../package.json');

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
  const expected = [{ pid: 20, ppid: 10, start: '2026-11-15T12:34:56.0000000+09:00', command: 'Werkschaft.exe' }];
  const result = processes('win32', (command, args) => {
    assert.equal(command, 'powershell.exe');
    assert(args.includes('-NonInteractive'));
    assert.match(args.at(-1), /Get-CimInstance Win32_Process/);
    assert.match(args.at(-1), /CreationDate.ToString/);
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

test('Windows packaging consumes only the pinned cached x64 zip and runtime allowlist, without a custom icon', async () => {
  const cache = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-cache-test-'));
  let staged;
  try {
    const nested = path.join(cache, 'checksum');
    fs.mkdirSync(nested);
    fs.writeFileSync(path.join(nested, `electron-v${pkg.devDependencies.electron}-win32-x64.zip`), 'test double');
    await build({ platform: 'win32', arch: 'arm64', cache, packageApp: async options => {
      assert.equal(options.platform, 'win32');
      assert.equal(options.arch, 'x64');
      assert.equal(options.electronVersion, pkg.devDependencies.electron);
      assert.equal(options.electronZipDir, nested);
      assert.equal(options.icon, undefined);
      assert.equal(options.appBundleId, 'dev.werkschaft.desktop');
      staged = options.dir;
      assert.deepEqual(fs.readdirSync(staged).sort(), ['connection.html', 'endpoint.js', 'main.js', 'package.json', 'policy.js']);
      assert.equal(JSON.parse(fs.readFileSync(path.join(staged, 'package.json'))).version, pkg.version);
      return ['test-only packager output'];
    } });
    assert.equal(fs.existsSync(staged), false);
    await assert.rejects(build({ platform: 'win32', cache: path.join(cache, 'absent'), packageApp: () => assert.fail('must never package/download') }), /Offline prerequisite missing.*win32-x64.zip; no download attempted/);
    await assert.rejects(build({ platform: 'linux', cache }), /darwin and win32 only/);
  } finally { fs.rmSync(cache, { recursive: true, force: true }); }
});

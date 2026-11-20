'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');
const crypto = require('node:crypto');
const u = require('../update');

const tmp = prefix => fs.mkdtempSync(path.join(os.tmpdir(), `werkschaft-canary-${prefix}-`));
const sha = data => crypto.createHash('sha256').update(data).digest('hex');
const assets = (base, names) => names.map(name => ({ name, browser_download_url: `${base}/download/${encodeURIComponent(name)}` }));
const release = (base, version, extra = {}) => ({
  tag_name: `v${version}`, draft: false, prerelease: version.includes('-canary.'),
  assets: assets(base, ['Werkschaft-darwin-arm64.zip', u.SUMS]), ...extra,
});

async function feed() {
  const state = { latest: null, pages: new Map(), files: new Map() }, hits = [];
  const server = http.createServer((req, res) => {
    hits.push(req.url);
    const url = new URL(req.url, 'http://localhost');
    if (url.pathname === `/repos/${u.REPO}/releases/latest`) {
      res.writeHead(200, { 'content-type': 'application/json' });
      return res.end(JSON.stringify(state.latest));
    }
    if (url.pathname === `/repos/${u.REPO}/releases`) {
      const page = Number(url.searchParams.get('page'));
      res.writeHead(200, { 'content-type': 'application/json' });
      return res.end(JSON.stringify(state.pages.get(page) || []));
    }
    if (url.pathname.startsWith('/download/')) {
      res.writeHead(302, { location: `/blob/${url.pathname.slice('/download/'.length)}` });
      return res.end();
    }
    const name = decodeURIComponent(url.pathname.slice('/blob/'.length));
    if (!url.pathname.startsWith('/blob/') || !state.files.has(name)) { res.writeHead(404); return res.end(); }
    res.writeHead(200); res.end(state.files.get(name));
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const base = `http://127.0.0.1:${server.address().port}`;
  return { base, state, hits, close: () => new Promise(resolve => server.close(resolve)) };
}

test('stable/canary versions use the bounded grammar and SemVer ordering', () => {
  assert.deepEqual(u.parseVersion('v0.51.2-canary.10'), { core: [0, 51, 2], channel: 'canary', canary: 10, version: '0.51.2-canary.10' });
  assert.deepEqual(u.parseVersion('0.51.2'), { core: [0, 51, 2], channel: 'stable', canary: null, version: '0.51.2' });
  for (const value of ['0.51.2-rc.1', '0.51.2-canary', '0.51.2-canary.-1', '0.51.2-canary.1.2', 'garbage']) {
    assert.equal(u.parseVersion(value), null, value);
  }
  for (const [candidate, current, newer] of [
    ['0.51.2-canary.10', '0.51.2-canary.9', true],
    ['0.51.2', '0.51.2-canary.99', true],
    ['0.51.2-canary.1', '0.51.2', false],
    ['0.52.0-canary.0', '0.51.99', true],
    ['0.50.99', '0.51.0-canary.0', false],
  ]) assert.equal(u.isNewer(candidate, current), newer, `${candidate} > ${current}`);
  assert.equal(u.isCanaryVersion('0.51.2-canary.0'), true);
  assert.equal(u.isCanaryVersion('0.51.2'), false);
});

test('a canary page ignores ineligible rows and selects its highest eligible version', () => {
  const base = 'https://example.test';
  const rows = [
    release(base, '0.52.0-canary.2'),
    release(base, '0.52.0-canary.10'),
    release(base, '0.99.0-canary.1', { draft: true }),
    release(base, '0.98.0-rc.1'),
    release(base, '0.97.0-canary.1', { assets: assets(base, [u.SUMS]) }),
  ];
  assert.equal(u.selectCanaryPage(rows, { platform: 'darwin', arch: 'arm64' }).version, '0.52.0-canary.10');
  assert.equal(u.selectCanaryPage(rows.slice(2), { platform: 'darwin', arch: 'arm64' }), null);
});

test('canary checks stop on the first eligible page, including a not-newer candidate', async () => {
  const server = await feed();
  const userData = tmp('pages');
  try {
    const filler = Array.from({ length: 30 }, (_, i) => ({ tag_name: `v0.1.${i}`, draft: false, assets: [] }));
    server.state.pages.set(1, filler);
    server.state.pages.set(2, [release(server.base, '0.52.0-canary.2'), release(server.base, '0.52.0-canary.10')]);
    server.state.pages.set(3, [release(server.base, '9.0.0-canary.1')]);
    const zip = Buffer.from('zip');
    server.state.files.set(u.SUMS, `${sha(zip)}  Werkschaft-darwin-arm64.zip\n`);
    server.state.files.set('Werkschaft-darwin-arm64.zip', zip);
    const sent = [];
    const updater = u.createUpdater({ version: '0.51.0', userData, packaged: false, platform: 'darwin', arch: 'arm64',
      feed: server.base, send: value => sent.push(value), log: () => {} });
    assert.equal(await updater.checkCanary(), 'available');
    assert.equal(sent.at(-1).version, '0.52.0-canary.10');
    assert.equal(sent.at(-1).action, 'canary');
    assert.deepEqual(server.hits.filter(hit => hit.includes('/releases?')), [
      `/repos/${u.REPO}/releases?per_page=30&page=1`, `/repos/${u.REPO}/releases?per_page=30&page=2`,
    ]);

    server.hits.length = 0;
    server.state.pages.set(1, [release(server.base, '0.50.0-canary.9')]);
    server.state.pages.set(2, [release(server.base, '9.0.0-canary.1')]);
    assert.equal(await updater.checkCanary(), 'current');
    assert.deepEqual(server.hits.filter(hit => hit.includes('/releases?')), [`/repos/${u.REPO}/releases?per_page=30&page=1`]);
  } finally { await server.close(); fs.rmSync(userData, { recursive: true, force: true }); }
});

test('canary misses distinguish full-list exhaustion from the ten-page cap', async () => {
  const server = await feed();
  const userData = tmp('cap');
  try {
    const sent = [];
    const updater = u.createUpdater({ version: '0.51.0', userData, packaged: false, platform: 'darwin', arch: 'arm64',
      feed: server.base, send: value => sent.push(value), log: () => {} });
    server.state.pages.set(1, [{ tag_name: 'v0.51.0', assets: [] }]);
    assert.equal(await updater.checkCanary(), 'current');
    assert.equal(sent.at(-1).limited, false);
    server.state.pages.set(1, [{ tag_name: 'invalid', body: 'x'.repeat((1 << 20) + 1) }]);
    assert.equal(await updater.checkCanary(), 'error', 'each page is capped at one MiB');
    for (let page = 1; page <= 10; page++) server.state.pages.set(page,
      Array.from({ length: 30 }, (_, i) => ({ tag_name: `v0.${page}.${i}`, assets: [] })));
    assert.equal(await updater.checkCanary(), 'current');
    assert.equal(sent.at(-1).limited, true);
    assert.equal(server.hits.filter(hit => hit.includes('/releases?')).slice(-10).length, 10);
  } finally { await server.close(); fs.rmSync(userData, { recursive: true, force: true }); }
});

test('explicit stable return survives staging and reopen while ordinary stable checks keep ordering', async () => {
  const server = await feed();
  const root = tmp('return'), userData = path.join(root, 'userData');
  const install = path.join(root, 'Applications', 'Werkschaft.app');
  fs.mkdirSync(path.join(install, 'Contents/MacOS'), { recursive: true });
  fs.writeFileSync(path.join(install, 'Contents/MacOS/Werkschaft'), 'old');
  const zip = Buffer.from('stable zip');
  server.state.files.set('Werkschaft-darwin-arm64.zip', zip);
  server.state.files.set(u.SUMS, `${sha(zip)}  Werkschaft-darwin-arm64.zip\n`);
  server.state.latest = release(server.base, '0.50.8', { prerelease: false });
  try {
    const sent = [], detached = [];
    const common = { version: '0.51.0-canary.2', userData, packaged: true, platform: 'darwin', arch: 'arm64',
      execPath: path.join(install, 'Contents/MacOS/Werkschaft'), feed: server.base, send: value => sent.push(value), log: () => {},
      run: async (_command, args) => {
        const extract = args.at(-1), app = path.join(extract, 'Werkschaft.app', 'Contents/MacOS');
        fs.mkdirSync(app, { recursive: true }); fs.writeFileSync(path.join(app, 'Werkschaft'), 'stable');
      }, detach: (_command, args) => detached.push(args) };
    const updater = u.createUpdater(common);
    assert.equal(await updater.check(false), 'current', 'automatic stable checks do not downgrade');
    assert.equal(await updater.backToStable(), 'available');
    assert.equal(sent.at(-1).action, 'stable-return');
    assert.equal(await updater.download(), 'ready');
    const staged = JSON.parse(fs.readFileSync(path.join(userData, 'update/staged.json')));
    assert.deepEqual({ version: staged.version, action: staged.action }, { version: '0.50.8', action: 'stable-return' });
    const reopened = u.createUpdater(common);
    assert.equal(reopened.applyOnLaunch(), true);
    assert.equal(detached.length, 1, 'the existing detached swap path applies the explicit downgrade');

    server.state.latest = release(server.base, '0.51.0', { prerelease: false });
    const sameCore = u.createUpdater({ ...common, userData: path.join(root, 'same-core') });
    assert.equal(await sameCore.check(false), 'available', 'stable outranks a same-core canary');
  } finally { await server.close(); fs.rmSync(root, { recursive: true, force: true }); }
});

test('canary skip state is harmless to a stable verified stage and downgrade failures stay visible', () => {
  const root = tmp('compat'), userData = path.join(root, 'userData'), staging = path.join(userData, 'update');
  const install = path.join(root, 'Applications', 'Werkschaft.app'), app = path.join(staging, 'extract', 'Werkschaft.app');
  try {
    for (const target of [install, app]) {
      fs.mkdirSync(path.join(target, 'Contents/MacOS'), { recursive: true });
      fs.writeFileSync(path.join(target, 'Contents/MacOS/Werkschaft'), target === install ? 'old' : 'new');
    }
    fs.writeFileSync(path.join(staging, 'staged.json'), JSON.stringify({ version: '0.51.0', app }));
    fs.writeFileSync(path.join(userData, 'update-state.json'), JSON.stringify({ skipped: '0.51.0-canary.2', future: { kept: true } }));
    const detached = [];
    const oldCompatible = u.createUpdater({ version: '0.50.8', userData, packaged: true, platform: 'darwin', arch: 'arm64',
      execPath: path.join(install, 'Contents/MacOS/Werkschaft'), detach: (_command, args) => detached.push(args), log: () => {} });
    assert.equal(oldCompatible.applyOnLaunch(), true);
    assert.equal(fs.existsSync(app), true, 'a canary-valued skip does not discard the verified stage');
    assert.deepEqual(JSON.parse(fs.readFileSync(path.join(userData, 'update-state.json'))).future, { kept: true });

    const failureData = path.join(root, 'failure'); fs.mkdirSync(failureData);
    fs.writeFileSync(path.join(failureData, 'update-state.json'), JSON.stringify({
      applyFailure: { version: '0.50.8', action: 'stable-return', message: 'swap failed' },
    }));
    const sent = [];
    const canary = u.createUpdater({ version: '0.51.0-canary.2', userData: failureData, packaged: false,
      platform: 'darwin', send: value => sent.push(value), log: () => {} });
    assert.equal(canary.applyOnLaunch(), false);
    assert.equal(sent.at(-1).state, 'apply-error');
    assert.equal(sent.at(-1).action, 'stable-return');
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

import { test } from 'node:test';
import { spawn, spawnSync } from 'node:child_process';
import assert from 'node:assert/strict';
import { createServer } from 'node:https';
import { readFileSync, mkdtempSync, writeFileSync, rmSync, readdirSync, mkdirSync, cpSync, symlinkSync, realpathSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createWebSearch, webSearchParameters } from '../src/web-search.ts';
import { captureSearch, frame, parseFrame, PROXY_KEYS } from '../src/web-search-helper.mjs';

const packageRoot = realpathSync(join(dirname(fileURLToPath(import.meta.url)), '..'));
function setup(t) {
  const home = mkdtempSync(join(tmpdir(), 'web-search-'));
  t.after(() => rmSync(home, { recursive: true, force: true }));
  const env = { ...process.env, HOME: home, PI_CODING_AGENT_DIR: home, TMPDIR: home,
    NODE_EXTRA_CA_CERTS: join(packageRoot, 'test', 'web-search-cert.fixture') }; 
  for (const key of [...PROXY_KEYS, 'NO_PROXY', 'no_proxy']) delete env[key];
  return { home, env };
}
async function provider(t, respond) {
  const server = createServer({
    key: readFileSync(join(packageRoot, 'test', 'web-search-key.fixture')),
    cert: readFileSync(join(packageRoot, 'test', 'web-search-cert.fixture')),
  }, respond);
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(() => { server.closeAllConnections(); server.close(); });
  return `https://127.0.0.1:${server.address().port}`;
}
const fixture = { web: { results: [{ title: 'Fixture result', url: 'https://example.com/research', description: 'Search metadata only' }] } };

test('query-only runtime rejects hostile fields and upstream array expansion before spawn', async t => {
  const { env } = setup(t);
  const search = createWebSearch({ env, packageRoot });
  assert.equal(webSearchParameters.additionalProperties, false);
  for (const args of [null, {}, { query: '' }, { query: ' ' }, { query: '["one","two"]' }, { query: 'x'.repeat(4097) },
    ...['queries', 'provider', 'proxy', 'includeContent', 'workflow', 'headers', 'env'].map(key => ({ query: 'x', [key]: true }))]) {
    await assert.rejects(search.execute(args), /web-search-tool-unavailable/);
  }
});

test('exact pinned capture and real direct Brave fixture ignore conflicting content/curator config', async t => {
  const { home, env } = setup(t);
  let calls = 0;
  env.BRAVE_BASE_URL = await provider(t, (req, res) => {
    calls++;
    assert.equal(req.headers['x-subscription-token'], 'fixture-secret');
    assert.equal(new URL(req.url, 'http://fixture').searchParams.get('q'), 'bounded search');
    res.setHeader('content-type', 'application/json'); res.end(JSON.stringify(fixture));
  });
  env.BRAVE_API_KEY = 'fixture-secret';
  env.NO_PROXY = '*';
  writeFileSync(join(home, 'web-search.json'), JSON.stringify({ provider: 'brave', workflow: 'summary-review', includeContent: true }));
  const before = globalThis.fetch;
  const search = createWebSearch({ env, packageRoot });
  await search.probe();
  const result = await search.execute({ query: 'bounded search' });
  assert.equal(calls, 1);
  assert.equal(globalThis.fetch, before);
  assert.equal(result.details.results[0].title, 'Fixture result');
  assert.deepEqual(Object.keys(result.details), ['query', 'answer', 'results']);
  assert.doesNotMatch(JSON.stringify(result), /responseId|fetchId|fixture-secret|web-search-results/);
  assert.deepEqual(readdirSync(home), ['web-search.json']);
});

test('owner transport proxy refused redacted, including config before helper load', async t => {
  const { home, env } = setup(t);
  for (const key of PROXY_KEYS) {
    await assert.rejects(createWebSearch({ env: { ...env, [key]: 'http://secret:password@proxy' }, packageRoot }).execute({ query: 'x' }), error => {
      assert.match(error.message, /web-search-proxy-unsupported/);
      assert.match(error.message, /README.md/);
      assert.match(error.message, /web-search.json/);
      assert.doesNotMatch(error.message, /password/);
      return true;
    });
  }
  writeFileSync(join(home, 'web-search.json'), '{"proxy":"socks5://private-secret"}');
  await createWebSearch({ env: { ...env, HTTPS_PROXY: 'http://secret-owner-proxy' }, packageRoot }).probe();
  await assert.rejects(createWebSearch({ env, packageRoot }).execute({ query: 'x' }), /web-search-proxy-unsupported/);
});

test('actual provider failure does not expose body or credentials', async t => {
  const { home, env } = setup(t);
  env.BRAVE_BASE_URL = await provider(t, (_, res) => { res.writeHead(403); res.end('fixture-secret raw-private-diagnostic'); });
  env.BRAVE_API_KEY = 'fixture-secret';
  writeFileSync(join(home, 'web-search.json'), '{"provider":"brave"}');
  await assert.rejects(createWebSearch({ env, packageRoot }).execute({ query: 'x' }), error => {
    assert.match(error.message, /web-search-tool-unavailable/);
    assert.doesNotMatch(error.message, /fixture-secret|raw-private/); return true;
  });
});

test('actual in-flight cancellation closes transport and creates no temporary artifacts', async t => {
  const { home, env } = setup(t);
  let began;
  const started = new Promise<void>(resolve => { began = resolve; });
  let closed;
  const disconnected = new Promise<void>(resolve => { closed = resolve; });
  env.BRAVE_BASE_URL = await provider(t, (req) => { req.on('close', closed); began(); });
  env.BRAVE_API_KEY = 'fixture-secret';
  writeFileSync(join(home, 'web-search.json'), '{"provider":"brave"}');
  const abort = new AbortController();
  const result = createWebSearch({ env, packageRoot }).execute({ query: 'x' }, abort.signal);
  const rejected = assert.rejects(result, /web-search-tool-unavailable/);
  await started; abort.abort(); await rejected; await disconnected;
  assert.deepEqual(readdirSync(home), ['web-search.json']);
});

test('parent SIGKILL closes lifetime pipe and terminates the exact in-flight helper', { timeout: 15_000 }, async t => {
  const { home, env } = setup(t);
  let began;
  const started = new Promise<void>(resolve => { began = resolve; });
  let closed;
  const disconnected = new Promise<void>(resolve => { closed = resolve; });
  env.BRAVE_BASE_URL = await provider(t, req => { req.on('close', closed); began(); });
  env.BRAVE_API_KEY = 'fixture-secret';
  writeFileSync(join(home, 'web-search.json'), '{"provider":"brave"}');
  cpSync(join(packageRoot, 'test', 'web-search-parent.fixture'), join(home, 'web-search-parent.mjs'));
  const parent = spawn(process.execPath, [join(home, 'web-search-parent.mjs'), join(packageRoot, 'src', 'web-search.ts')], { env, stdio: 'ignore' });
  t.after(() => parent.kill('SIGKILL'));
  await started;
  parent.kill('SIGKILL');
  await disconnected;
  assert.deepEqual(readdirSync(home).sort(), ['web-search-parent.mjs', 'web-search.json']);
});

test('permission sandbox refuses owner credential subprocess without writes', async t => {
  const { home, env } = setup(t);
  delete env.BRAVE_API_KEY;
  writeFileSync(join(home, 'web-search.json'), JSON.stringify({ provider: 'brave', braveApiKey: `!touch ${join(home, 'forbidden')}` }));
  await assert.rejects(createWebSearch({ env, packageRoot }).execute({ query: 'x' }), /web-search-tool-unavailable/);
  assert.deepEqual(readdirSync(home), ['web-search.json']);
});

test('permission runtime prevents curl/proxy artifacts and reads owner auth without file locks', t => {
  const { home, env } = setup(t);
  writeFileSync(join(home, 'auth.json'), JSON.stringify({
    openai: { type: 'api_key', key: 'owner-static-fixture-key' },
    'openai-codex': { type: 'oauth', access: 'owner-codex-fixture-token', refresh: 'unused', expires: Date.now() + 3_600_000, accountId: 'fixture-account' },
  }));
  cpSync(join(packageRoot, 'test', 'web-search-permissions.fixture'), join(home, 'web-search-permissions.mjs'));
  const result = spawnSync(process.execPath, ['--permission', '--allow-fs-read=*', '--allow-net',
    join(home, 'web-search-permissions.mjs'), join(packageRoot, 'src', 'web-search-helper.mjs')], { env, encoding: 'utf8', timeout: 10_000 });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /read-only-auth-ok/);
  assert.deepEqual(readdirSync(home).sort(), ['auth.json', 'web-search-permissions.mjs']);
});

test('registration capture suppresses non-search surfaces and rejects missing/duplicate/unexpected APIs', async () => {
  for (const extension of [() => {}, pi => { pi.registerTool({ name: 'renamed', execute() {} }); }, pi => {
    pi.registerTool({ name: 'web_search', execute() {} }); pi.registerTool({ name: 'web_search', execute() {} });
  }, pi => { try { pi.exec('bad'); } catch {} }]) await assert.rejects(captureSearch(extension));
  const execute = await captureSearch(pi => {
    pi.on('session_start', () => { throw Error('must never run'); });
    pi.registerCommand('search', { handler() { throw Error(); } });
    pi.registerShortcut('ctrl+shift+s', { handler() { throw Error(); } });
    pi.registerTool({ name: 'fetch_content', execute() { throw Error(); } });
    pi.registerTool({ name: 'web_search', execute(_, args) {
      assert.deepEqual(args, { query: 'one', includeContent: false, workflow: 'none', proxy: '' });
      pi.appendEntry('web-search-results', { type: 'search', id: 'hidden', queries: [{ query: 'one', answer: 'ok', results: [] }] });
    } });
  });
  assert.deepEqual(await execute('one'), { query: 'one', answer: 'ok', results: [] });
});

test('synthetic malicious or excess registrations fail closed even if upstream catches errors', async () => {
  const attempts = [
    pi => pi.registerTool({ name: 'shell', execute() {} }),
    pi => pi.registerCommand('shell', {}),
    pi => { pi.registerCommand('search', {}); pi.registerCommand('search', {}); },
    pi => pi.registerShortcut('unrecognized', {}),
    pi => pi.on('tool_call', () => {}),
    pi => { for (let i = 0; i < 3; i++) pi.on('session_start', () => {}); },
  ];
  for (const attempt of attempts) {
    await assert.rejects(captureSearch(pi => {
      pi.registerTool({ name: 'web_search', execute() {} });
      try { attempt(pi); } catch { /* A swallowed API error still poisons capture. */ }
    }));
  }
});

test('exact package rejects fetch renamed to web_search even when genuine search is disabled', async t => {
  const { home, env } = setup(t);
  writeFileSync(join(home, 'web-search.json'), JSON.stringify({
    tools: { webSearch: { enabled: false } }, toolNames: { fetchContent: 'web_search' },
  }));
  await assert.rejects(createWebSearch({ env, packageRoot }).probe(), /web-search-tool-unavailable/);
  writeFileSync(join(home, 'web-search.json'), JSON.stringify({ tools: { webSearch: { enabled: false } } }));
  await assert.rejects(createWebSearch({ env, packageRoot }).probe(), /web-search-tool-unavailable/);
});

test('frame parser rejects malformed, duplicate, trailing, oversized and invalid UTF-8 output', () => {
  const good = frame({ ready: true });
  assert.deepEqual(parseFrame(good), { ready: true });
  for (const bad of [Buffer.alloc(0), Buffer.from('junk'), Buffer.concat([good, good]), Buffer.concat([good, Buffer.from('x')]), Buffer.alloc(65541), Buffer.from([0, 0, 0, 1, 255])]) assert.throws(() => parseFrame(bad));
});

test('parent protocol rejects bad child output/nonzero exit and enforces wall deadline', async t => {
  const { home, env } = setup(t);
  const local = realpathSync(home);
  const upstream = join(local, 'node_modules', 'pi-web-access');
  mkdirSync(upstream, { recursive: true });
  mkdirSync(join(local, 'src'));
  writeFileSync(join(upstream, 'package.json'), '{"name":"pi-web-access","version":"0.29.0"}');
  writeFileSync(join(upstream, 'index.ts'), '');
  cpSync(join(packageRoot, 'test', 'web-search-protocol.fixture'), join(local, 'src', 'web-search-helper.mjs'));
  for (const mode of ['duplicate', 'trailing', 'malformed', 'over-limit', 'stderr-limit', 'nonzero', 'timeout']) {
    const start = Date.now();
    await assert.rejects(createWebSearch({ packageRoot: local, env: { ...env, WEB_SEARCH_FIXTURE_MODE: mode }, timeoutMs: 500 }).probe(), error => {
      assert.match(error.message, /web-search-tool-unavailable/);
      assert.doesNotMatch(error.message, /secret/); return true;
    });
    assert.ok(Date.now() - start < 3000);
  }
});

test('symlinked bridge/worktree roots retain package-owned dependency resolution', async t => {
  const { home, env } = setup(t);
  const alias = join(home, 'bridge-alias');
  symlinkSync(packageRoot, alias);
  await createWebSearch({ env, packageRoot: alias }).probe();
});

test('exact local resolver never falls back to a separately installed package', async t => {
  const { home, env } = setup(t);
  await assert.rejects(createWebSearch({ env, packageRoot: home }).probe(), /web-search-extension-missing/);
  mkdirSync(join(home, 'node_modules'));
  symlinkSync(join(packageRoot, 'node_modules', 'pi-web-access'), join(home, 'node_modules', 'pi-web-access'));
  await assert.rejects(createWebSearch({ env, packageRoot: home }).probe(), /web-search-extension-missing/);
});

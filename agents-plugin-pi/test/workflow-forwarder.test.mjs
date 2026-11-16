// Pure Node forwarding seam: no launcher, daemon, real SDK, or npm imports.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statfsSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, isAbsolute, join } from 'node:path';
import test from 'node:test';
import { pathToFileURL } from 'node:url';

const tempRoot = process.env.WS_TEST_TMP_ROOT || tmpdir();
const bootstrap = readFileSync(new URL('../bin/ws-pi-forward.mjs', import.meta.url));
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
// Deliberately explicit and unsorted; raw CRLF and UTF-8 bytes are significant.
// Do not ask the helper or builder to compute its own expected record.
const inputs = {
  'src/nested/helper.mjs': Buffer.from('export const word = "ü";\r\n'),
  'src/index.ts': Buffer.from('export default async function () {}\r\n'),
  'src/eff_large_wordlist.txt': Buffer.from('11111\tabacus\r\n'),
  'scripts/bundle.mjs': Buffer.from('// inert build input\n'),
  'package-lock.json': Buffer.from('{"lockfileVersion":3}\r\n'),
};
const inputHash = sha(Object.keys(inputs).sort().map(name => `${name}\n${sha(inputs[name])}\n`).join(''));
const goodHeader = `// ws-pi-bundle-inputs: sha256-${inputHash}`;
function put(file, bytes) {
  mkdirSync(dirname(file), { recursive: true });
  writeFileSync(file, bytes);
}
function fixture(t, owner = 'ws', prepared = false) {
  assert.ok(isAbsolute(tempRoot), 'scratch must be absolute');
  const minimum = Number(process.env.WS_TEST_MIN_FREE_GIB || 0);
  assert.ok(Number.isSafeInteger(minimum) && minimum >= 0);
  if (minimum) {
    const disk = statfsSync(tempRoot, { bigint: true });
    assert.ok(disk.bavail * disk.bsize >= BigInt(minimum) * 1024n ** 3n, `STOP: scratch free space below ${minimum} GiB`);
  }
  const root = mkdtempSync(join(tempRoot, 'workflow-forwarder-'));
  const key = `ws-forwarder:${root}`;
  t.after(() => { delete globalThis[key]; rmSync(root, { recursive: true, force: true }); });
  const original = prepared
    ? join(root, `original ${owner} # percent % ü`, 'native-pi/first-party')
    : join(root, 'original Pi # percent % ü', 'agents-plugin-pi');
  put(join(original, 'package.json'), '{"type":"module"}\n');
  put(join(original, 'bin/ws-pi-forward.mjs'), bootstrap);
  if (prepared) {
    put(join(original, 'bundle-inputs.json'), JSON.stringify({ schema_version: 1, sha256: inputHash }));
  } else {
    for (const [name, bytes] of Object.entries(inputs)) put(join(original, name), bytes);
    // A source checkout must not accidentally trust a prepared record instead.
    put(join(original, 'bundle-inputs.json'), JSON.stringify({ schema_version: 1, sha256: 'f'.repeat(64) }));
  }
  const dist = join(original, 'dist/index.js');
  const uri = pathToFileURL(dist).href;
  const body = `
globalThis[${JSON.stringify(key)}] = (globalThis[${JSON.stringify(key)}] || 0) + 1;
export class HostToken {}
export const token = new HostToken();
export const identity = { url: import.meta.url, owner: ${JSON.stringify(owner)}, token };
export default async function (...args) {
  const api = args[0];
  api?.registerTool?.({ name: 'ws-execute' });
  api?.registerTool?.({ name: 'ws__playbook_read' });
  api?.events?.emit('ws:lead-initialized', undefined);
  await Promise.resolve();
  return { identity, HostToken, receiver: this, args };
}
`;
  put(dist, goodHeader + '\n' + body);
  // Model the selected workflow root and resolve exactly its Pi sibling entry.
  // Python independently checks that the launcher actually emits these bytes.
  const selected = join(original, '.runtime/workflow', sha(root), owner);
  put(join(selected, 'runtime.json'), JSON.stringify({ plugin: owner }));
  const sibling = join(dirname(selected), 'agents-plugin-pi');
  put(join(sibling, 'package.json'), '{"type":"module","pi":{"extensions":["./index.mjs"]}}\n');
  const shim = `import { createPiForwarder } from ${JSON.stringify(pathToFileURL(join(original, 'bin/ws-pi-forward.mjs')).href)};\nexport default createPiForwarder(${JSON.stringify(uri)});\n`;
  put(join(sibling, 'index.mjs'), shim);
  return {
    original, dist, uri, key, body,
    async entry() {
      const pi = join(dirname(selected), 'agents-plugin-pi');
      const manifest = JSON.parse(readFileSync(join(pi, 'package.json'), 'utf8'));
      return import(pathToFileURL(join(pi, manifest.pi.extensions[0])).href);
    },
  };
}
async function rejectsBeforeEffects(f, pattern = /bundle is stale/) {
  const forwarded = await f.entry();
  assert.equal(globalThis[f.key], undefined, 'importing the sibling must not statically import original dist');
  const tools = [], emitted = [];
  const api = { registerTool: tool => tools.push(tool), events: { emit: (...args) => emitted.push(args) } };
  await assert.rejects(forwarded.default(api), error => {
    assert.match(error.message, /\[ws Pi\] EXTENSION NOT LOADED/);
    assert.match(error.message, pattern);
    assert.match(error.message, /rebuild\/reinstall/);
    return true;
  });
  assert.equal(globalThis[f.key], undefined, 'refusal precedes original module side effects');
  assert.deepEqual(tools, [], 'stale dist cannot register misleading sentinel tools');
  assert.deepEqual(emitted, [], 'stale dist cannot claim successful initialization');
}

for (const [owner, prepared] of [['ws', false], ['wsflow', false], ['ws', true], ['wsflow', true]]) {
  const label = `${owner} ${prepared ? 'prepared-native' : 'source'}`;
  test(`${label}: selected sibling awaits factory and preserves original module graph, URL, this and args`, async t => {
    const f = fixture(t, owner, prepared);
    if (prepared) {
      for (const name of ['src', 'package-lock.json', 'scripts']) assert.equal(existsSync(join(f.original, name)), false, name);
    }
    const forwarded = await f.entry();
    assert.equal(globalThis[f.key], undefined);
    const receiver = {}, argument = {}, trailing = { token: 'argument identity' };
    const result = await Reflect.apply(forwarded.default, receiver, [argument, trailing]);
    const direct = await import(f.uri);
    assert.notStrictEqual(forwarded.default, direct.default, 'wrapper function intentionally differs');
    assert.strictEqual(result.identity, direct.identity);
    assert.strictEqual(result.identity.token, direct.token);
    assert.strictEqual(result.HostToken, direct.HostToken);
    assert.ok(result.identity.token instanceof direct.HostToken, 'SDK-like class identity is not copied');
    assert.equal(result.identity.url, f.uri);
    assert.equal(result.identity.owner, owner);
    for (const escape of ['%20', '%23', '%25', '%C3%BC']) assert.ok(f.uri.includes(escape), escape);
    assert.strictEqual(result.receiver, receiver);
    assert.strictEqual(result.args[0], argument);
    assert.strictEqual(result.args[1], trailing);
    assert.equal(globalThis[f.key], 1, 'only the original module was evaluated');
  });
}

// npm omits the root lock from package.files; the producer retains its raw
// bytes privately, but the canonical hashed row remains package-lock.json.
for (const drift of [null, 'bundle-inputs/lock.json', 'src/index.ts']) {
  test(`partial-source package: ${drift ? `stale ${drift} refuses` : 'retained raw lock loads fresh factory'}`, async t => {
    const f = fixture(t);
    rmSync(join(f.original, 'package-lock.json'));
    put(join(f.original, 'bundle-inputs/lock.json'), inputs['package-lock.json']);
    assert.equal(existsSync(join(f.original, 'package-lock.json')), false);
    assert.equal(existsSync(join(f.original, 'src/index.ts')), true);
    if (drift) {
      put(join(f.original, drift), Buffer.from('// drifted retained input\r\n'));
      await rejectsBeforeEffects(f);
    } else {
      const forwarded = await f.entry();
      assert.equal(globalThis[f.key], undefined);
      const result = await forwarded.default();
      const direct = await import(f.uri);
      assert.strictEqual(result.identity, direct.identity);
      assert.strictEqual(result.HostToken, direct.HostToken);
      assert.equal(result.identity.url, f.uri);
      assert.equal(globalThis[f.key], 1);
    }
  });
}

for (const name of Object.keys(inputs)) {
  test(`source drift in ${name} rejects before original evaluation`, async t => {
    const f = fixture(t);
    // A stale root lock may not be hidden by a current retained fallback.
    if (name === 'package-lock.json') put(join(f.original, 'bundle-inputs/lock.json'), inputs[name]);
    put(join(f.original, name), Buffer.concat([inputs[name], Buffer.from('\n// drift\n')]));
    await rejectsBeforeEffects(f);
  });
}
for (const [label, header] of [
  ['missing', '// no bundle header'],
  ['invalid', '// ws-pi-bundle-inputs: sha256-not-a-digest'],
  ['stale', `// ws-pi-bundle-inputs: sha256-${'0'.repeat(64)}`],
]) {
  test(`source ${label} dist header rejects before original evaluation`, async t => {
    const f = fixture(t);
    put(f.dist, header + '\n' + f.body);
    await rejectsBeforeEffects(f, label === 'stale' ? /bundle is stale/ : /header is missing or invalid/);
  });
}
for (const owner of ['ws', 'wsflow']) {
  for (const [label, record] of [
    ['wrong schema', { schema_version: 2, sha256: inputHash }],
    ['invalid hash', { schema_version: 1, sha256: 'not-a-digest' }],
    ['stale hash', { schema_version: 1, sha256: '0'.repeat(64) }],
    ['malformed JSON', '{'],
  ]) {
    test(`${owner} prepared ${label} record refuses locally before original effects`, async t => {
      const f = fixture(t, owner, true);
      put(join(f.original, 'bundle-inputs.json'), typeof record === 'string' ? record : JSON.stringify(record));
      await rejectsBeforeEffects(f, label === 'wrong schema' ? /invalid prepared bundle input record/ : label === 'malformed JSON' ? /JSON|property name/i : /bundle is stale/);
    });
  }
  test(`${owner} prepared stale header refuses despite current input record`, async t => {
    const f = fixture(t, owner, true);
    put(f.dist, `// ws-pi-bundle-inputs: sha256-${'0'.repeat(64)}\n` + f.body);
    await rejectsBeforeEffects(f);
  });
}

test('failed selected factory stays loud in the existing watchdog without taking over input/mail', async t => {
  const f = fixture(t);
  put(join(f.original, 'src/index.ts'), '// stale source\n');
  const source = readFileSync(new URL('../../agents-plugin-tool/internal/wsagent/pi_lead_bridge.mjs', import.meta.url), 'utf8');
  const prelude = 'const servers = null; const sessionKey = "existing-lead-key"; const launchWarning = "";\n';
  const { default: register } = await import('data:text/javascript;base64,' + Buffer.from(prelude + source).toString('base64'));
  // Metadata only, not a host/SDK import. Keep host skew out of this assertion.
  const cli = join(f.original, 'fake-host/cli.js');
  put(cli, '// never executed\n');
  put(join(dirname(cli), 'package.json'), '{"name":"@earendil-works/pi-coding-agent","version":"0.84.4"}');
  const previous = process.argv[1];
  process.argv[1] = cli;
  t.after(() => { process.argv[1] = previous; });
  const handlers = {}, signals = {}, notices = [], tools = [], registered = [];
  const deliveries = [];
  const deliver = value => { deliveries.push(value); return value; };
  const api = {
    events: { on: (name, handler) => { signals[name] = handler; }, emit: (name, value) => signals[name]?.(value) },
    on: (name, handler) => { handlers[name] = handler; },
    getAllTools: () => tools.map(name => ({ name })),
    registerTool: tool => tools.push(tool.name),
    registerMcpServer: (...args) => registered.push(args),
    sendUserMessage: deliver,
  };
  register(api);
  const { default: factory } = await f.entry();
  await assert.rejects(factory(api), /EXTENSION NOT LOADED.*stale/);
  assert.equal(globalThis[f.key], undefined);
  const ctx = { ui: { notify: (message, type) => notices.push({ message, type }) } };
  handlers.session_start({}, ctx);
  assert.equal(notices[0].type, 'error');
  assert.match(notices[0].message, /EXTENSION NOT LOADED OR INCOMPLETE.*missing native lead tools ws-execute, ws__playbook_read/);
  assert.match(notices[0].message, /Dashboard input\/mail delivery remains reachable/);
  assert.doesNotMatch(notices[0].message, /UNSUPPORTED HOST/);
  const prompt = handlers.before_agent_start({ systemPrompt: 'preserved input' }, ctx).systemPrompt;
  assert.ok(prompt.startsWith('preserved input\n\n'));
  assert.match(prompt, /do not assume its tools exist/);
  tools.push('ws-execute', 'ws__playbook_read');
  assert.match(handlers.before_agent_start({ systemPrompt: 'preserved' }, ctx).systemPrompt, /initialization did not complete/);
  assert.deepEqual(Object.keys(handlers).sort(), ['agent_start', 'before_agent_start', 'session_shutdown', 'session_start']);
  assert.deepEqual(registered, [], 'a local factory failure adds no global MCP admission/fallback');
  assert.strictEqual(api.sendUserMessage, deliver);
  assert.equal(api.sendUserMessage('dashboard input'), 'dashboard input');
  assert.equal(api.sendUserMessage('mail'), 'mail');
  assert.deepEqual(deliveries, ['dashboard input', 'mail']);
});

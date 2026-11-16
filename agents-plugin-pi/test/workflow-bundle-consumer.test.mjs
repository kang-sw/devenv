import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, statfsSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { tmpdir } from 'node:os';
import { dirname, isAbsolute, join, resolve } from 'node:path';
import test, { after, before } from 'node:test';
import { fileURLToPath, pathToFileURL } from 'node:url';

// Consumer/ESM semantics proof only: no launcher, real SDK, or daemon is loaded.
const repo = fileURLToPath(new URL('../../', import.meta.url));
const tempRoot = process.env.WS_TEST_TMP_ROOT || tmpdir();
const minFreeGiB = Number(process.env.WS_TEST_MIN_FREE_GIB || 0);
const peer = '3d791f606a633141ef496cf09c821fbb3f7ae12f';
const peerFiles = {
  'daemon-selection.js': 'aa9240e32fa4b079c9991c2885bf60f3b135616bcb4c73c88e9a73fb030111fc',
  'daemon-stage.js': '628107ccdf2485ab78b23e4b436e3ab63a450b8dd4e2d956b747b7183544f01c',
};
const products = { ws: 'agents-plugin', wsflow: 'agents-plugin-wsflow' };
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const json = file => JSON.parse(readFileSync(file, 'utf8'));
let root, producer, bundleManifest, validateDaemonRoot, copyPackage;
const packages = new Map();

function freeSpace() {
  assert.ok(Number.isSafeInteger(minFreeGiB) && minFreeGiB >= 0, 'WS_TEST_MIN_FREE_GIB must be a nonnegative integer');
  if (minFreeGiB === 0) return;
  const disk = statfsSync(tempRoot, { bigint: true });
  assert.ok(disk.bavail * disk.bsize >= BigInt(minFreeGiB) * 1024n ** 3n, `STOP: ${tempRoot} free space below ${minFreeGiB} GiB`);
}
function git(...args) {
  return execFileSync('git', args, { cwd: repo, env: process.env, maxBuffer: 32 * 1024 * 1024 });
}
function committed(path) {
  return git('show', `${producer}:${path}`);
}
function files(dir, prefix = '') {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const name = prefix + entry.name;
    assert.ok(entry.isFile() || entry.isDirectory(), `not a regular package entry: ${name}`);
    return entry.isDirectory() ? files(join(dir, entry.name), name + '/') : [name];
  }).sort();
}
function assertSkills(packageRoot) {
  const skills = join(packageRoot, 'skills');
  const manifest = json(join(skills, 'manifest.json'));
  assert.equal(manifest.schema_version, 1);
  const actual = files(skills).filter(name => name !== 'manifest.json');
  assert.deepEqual(Object.keys(manifest.files), actual, 'complete, sorted slash-relative skills inventory');
  for (const name of actual) {
    assert.ok(!name.includes('\\') && !name.startsWith('/') && !name.split('/').includes('..'));
    assert.match(manifest.files[name], /^[a-f0-9]{64}$/);
    // Match the peer consumer: UTF-8 decoding, then CRLF -> LF only.
    const normalized = readFileSync(join(skills, name), 'utf8').replace(/\r\n/g, '\n');
    assert.equal(manifest.files[name], sha(normalized), name);
  }
  return actual;
}
function assertSelection(selected, expectedRoot, owner) {
  assert.equal(selected.root, expectedRoot);
  assert.equal(selected.source, 'bundled');
  assert.equal(selected.binary, join(expectedRoot, 'ws-mcp.exe'));
  assert.equal(selected.contract.plugin, owner);
  assert.equal(json(join(expectedRoot, '.claude-plugin/plugin.json')).name, owner);
  assert.deepEqual(selected.env, {
    WS_RSRC_ROOT: join(expectedRoot, 'rsrc'), WS_SKILLS_ROOT: join(expectedRoot, 'skills'),
  });
  for (const name of ['runtime.json', 'config-manifest.json', 'dashboard-config-manifest.json']) {
    assert.deepEqual(readFileSync(join(expectedRoot, name)), committed(`${products[owner]}/${name}`), `${owner}/${name} bytes/defaults`);
  }
  assertSkills(expectedRoot);
}
function refuse(packageRoot, destination, pattern) {
  assert.throws(() => validateDaemonRoot(packageRoot, 'bundled', 'win32'), pattern);
  assert.throws(() => copyPackage(packageRoot, destination, 'bundled', 'win32'), pattern);
  assert.equal(existsSync(destination), false, 'refusal must precede staging');
}

before(() => {
  assert.ok(isAbsolute(tempRoot), 'WS_TEST_TMP_ROOT must be an absolute scratch path');
  for (const name of ['TMP', 'TEMP', 'TMPDIR']) process.env[name] = tempRoot;
  freeSpace();
  root = mkdtempSync(join(tempRoot, 'workflow-consumer-'));
  producer = git('rev-parse', 'HEAD').toString().trim();
  bundleManifest = JSON.parse(committed('agents-plugin-pi/workflow-bundles/manifest.json'));
  const consumer = join(root, 'exec3');
  mkdirSync(consumer);
  // The exact peer CommonJS sources need their own package boundary, not Pi's ESM scope.
  writeFileSync(join(consumer, 'package.json'), '{"type":"commonjs"}\n');
  for (const [name, expected] of Object.entries(peerFiles)) {
    const bytes = git('show', `${peer}:agents-plugin-tool/desktop-electron/${name}`);
    assert.equal(sha(bytes), expected, `immutable peer ${name}`);
    writeFileSync(join(consumer, name), bytes);
  }
  const require = createRequire(pathToFileURL(join(consumer, 'proof.cjs')));
  ({ validateDaemonRoot } = require('./daemon-selection.js'));
  ({ copyPackage } = require('./daemon-stage.js'));
  for (const owner of Object.keys(products)) {
    freeSpace();
    const bytes = committed(`agents-plugin-pi/workflow-bundles/${owner}.zip`);
    assert.equal(sha(bytes), bundleManifest[owner].archive_sha256);
    const archive = join(root, `${owner}.zip`), destination = join(root, owner);
    writeFileSync(archive, bytes);
    // Extraction is Python stdlib zipfile only, never the producer/launcher helper.
    execFileSync('python', ['-I', '-c', [
      'import pathlib, sys, zipfile',
      'with zipfile.ZipFile(sys.argv[1]) as archive:',
      '    names = archive.namelist()',
      '    assert len(names) == len(set(names))',
      '    for name in names:',
      '        p = pathlib.PurePosixPath(name)',
      '        assert not p.is_absolute() and ".." not in p.parts and "\\\\" not in name',
      '        assert (archive.getinfo(name).external_attr >> 16) & 0o170000 == 0o100000',
      '    archive.extractall(sys.argv[2])',
    ].join('\n'), archive, destination], { cwd: root, env: process.env, stdio: 'pipe' });
    const inventory = Object.fromEntries(files(destination).map(name => [name, sha(readFileSync(join(destination, name)))]));
    assert.deepEqual(inventory, bundleManifest[owner].files, 'actual committed ZIP inventory');
    const contract = json(join(destination, 'runtime.json'));
    const inert = Buffer.from('INERT consumer fixture, not an executable\n');
    writeFileSync(join(destination, 'ws-mcp.exe'), inert);
    writeFileSync(join(destination, 'ws-mcp.exe.metadata.json'), JSON.stringify({
      schema_version: 1, version: contract.plugin_version, binary_sha256: sha(inert),
      mcp_protocol: String(contract.mcp_protocol || ''),
      tools: Object.keys(contract.tools || {}).sort(), commands: Object.keys(contract.commands || {}).sort(),
    }));
    packages.set(owner, destination);
  }
});
after(() => { if (root) rmSync(root, { recursive: true, force: true }); });

test('consumer provenance is pinned to immutable Exec3 sources', t => {
  t.diagnostic(`producer=${producer}; peer=${peer}; node=${process.version}`);
  t.diagnostic(`scratch=${tempRoot}; minimum_free_gib=${minFreeGiB}`);
  for (const [name, hash] of Object.entries(peerFiles)) t.diagnostic(`${name} sha256=${hash}`);
  for (const owner of Object.keys(products)) {
    t.diagnostic(`${owner}.zip sha256=${bundleManifest[owner].archive_sha256}; version=${json(join(packages.get(owner), 'runtime.json')).plugin_version}`);
  }
});

for (const owner of Object.keys(products)) {
  test(`${owner}: committed producer ZIP validates and stages with the exact peer consumer`, t => {
    freeSpace();
    const source = packages.get(owner), destination = join(root, `${owner}-staged`);
    assertSelection(validateDaemonRoot(source, 'bundled', 'win32'), source, owner);
    assertSelection(copyPackage(source, destination, 'bundled', 'win32'), destination, owner);
    assert.deepEqual(files(destination), files(source));
    for (const name of files(source)) assert.deepEqual(readFileSync(join(destination, name)), readFileSync(join(source, name)), name);
    t.diagnostic(`${owner}: ${files(source).length} staged files; ${assertSkills(destination).length} skill entries`);
  });

  test(`${owner}: missing referenced Claude agent refuses validation and staging`, () => {
    const source = packages.get(owner);
    const ref = json(join(source, '.claude-plugin/plugin.json')).agents[0];
    const relative = ref.replace(/^\$\{CLAUDE_PLUGIN_ROOT\}\//, '').replace(/^\.\//, '');
    const file = join(source, relative), original = readFileSync(file);
    rmSync(file);
    try { refuse(source, join(root, `${owner}-missing-reference`), /Cannot use bundled daemon.*ENOENT/s); }
    finally { writeFileSync(file, original); }
  });

  test(`${owner}: incomplete skills inventory refuses validation and staging`, () => {
    const source = packages.get(owner), file = join(source, 'skills/manifest.json');
    const original = readFileSync(file), manifest = JSON.parse(original);
    delete manifest.files[Object.keys(manifest.files)[0]];
    writeFileSync(file, JSON.stringify(manifest));
    try { refuse(source, join(root, `${owner}-bad-skills`), /Unlisted skills file/); }
    finally { writeFileSync(file, original); }
  });

  test(`${owner}: incomplete daemon tool inventory refuses validation and staging`, () => {
    const source = packages.get(owner), file = join(source, 'ws-mcp.exe.metadata.json');
    const original = readFileSync(file), metadata = JSON.parse(original);
    assert.ok(metadata.tools.length > 0);
    metadata.tools.pop();
    writeFileSync(file, JSON.stringify(metadata));
    try { refuse(source, join(root, `${owner}-bad-tools`), /Daemon missing required tools/); }
    finally { writeFileSync(file, original); }
  });
}

test('wsflow manifest is generated in the ZIP, not patched into canonical curated skills', () => {
  const source = packages.get('wsflow');
  const canonical = git('ls-tree', '-r', '--name-only', producer, '--', 'agents-plugin-wsflow/skills/').toString().trim().split('\n');
  assert.ok(!canonical.includes('agents-plugin-wsflow/skills/manifest.json'), 'raw canonical wsflow has no input manifest');
  assert.deepEqual(assertSkills(source), canonical.map(name => name.slice('agents-plugin-wsflow/skills/'.length)).sort());
  for (const name of canonical) {
    assert.deepEqual(readFileSync(join(source, 'skills', name.slice('agents-plugin-wsflow/skills/'.length))), committed(name), name);
  }
  const file = join(source, 'skills/manifest.json'), original = readFileSync(file);
  rmSync(file);
  try { refuse(source, join(root, 'raw-wsflow-without-generated-manifest'), /skills[\\/]manifest\.json.*ENOENT|ENOENT.*skills[\\/]manifest\.json/s); }
  finally { writeFileSync(file, original); }
});

test('default-export forwarding keeps original ESM URL, owner marker and object identity', async () => {
  freeSpace();
  const original = join(root, 'original owner # percent % ü space', 'dist', 'index.js');
  mkdirSync(dirname(original), { recursive: true });
  writeFileSync(join(dirname(dirname(original)), 'package.json'), '{"type":"module"}\n');
  writeFileSync(original, 'export default { url: import.meta.url, owner: "wsflow-stub", token: {} };\n');
  const uri = pathToFileURL(original).href;
  assert.match(uri, /%20/);
  assert.match(uri, /%23/);
  assert.match(uri, /%25/);
  assert.match(uri, /%C3%BC/);
  const shim = resolve(root, 'prepared cache with spaces', 'dist', 'index.js');
  mkdirSync(dirname(shim), { recursive: true });
  writeFileSync(join(dirname(dirname(shim)), 'package.json'), '{"type":"module"}\n');
  // Same forwarding form as the materializer's original.as_uri() shim.
  writeFileSync(shim, `export { default } from ${JSON.stringify(uri)};\n`);
  const direct = await import(uri), forwarded = await import(pathToFileURL(shim).href);
  assert.strictEqual(forwarded.default, direct.default);
  assert.strictEqual(forwarded.default.token, direct.default.token);
  assert.equal(forwarded.default.url, uri);
  assert.equal(forwarded.default.owner, 'wsflow-stub');
  assert.notEqual(forwarded.default.url, pathToFileURL(shim).href);
});

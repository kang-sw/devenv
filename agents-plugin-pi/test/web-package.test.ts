import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdtempSync, mkdirSync, readFileSync, realpathSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createWebSearch } from '../src/web-search.ts';
import { prepareSkillsDir } from '../src/skills-dir.ts';

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const peers = ['pi-ai', 'pi-coding-agent', 'pi-tui'];

function fixtureEnv(root: string): NodeJS.ProcessEnv {
  const env = { ...process.env };
  for (const [key, folder] of Object.entries({ HOME: 'home', USERPROFILE: 'home', WS_CACHE_HOME: 'cache', WS_CONFIG_HOME: 'config', npm_config_cache: 'npm-cache', PI_CODING_AGENT_DIR: 'pi-agent' })) {
    mkdirSync(join(root, folder), { recursive: true }); env[key] = join(root, folder);
  }
  env.npm_config_userconfig = join(root, 'npmrc');
  writeFileSync(env.npm_config_userconfig, '');
  env.PI_OFFLINE = '1';
  env.PI_TELEMETRY = '0';
  return env;
}

function run(command: string, args: string[], cwd: string, env: NodeJS.ProcessEnv): string {
  const result = spawnSync(command, args, { env, cwd, encoding: 'utf8', timeout: 90_000, maxBuffer: 8 * 1024 * 1024 });
  assert.equal(result.status, 0, `${command}: ${result.error ?? ''}\n${result.stdout}\n${result.stderr}`);
  return result.stdout;
}

function testCli(): string {
  if (process.env.WS_PI_TEST_CLI) return realpathSync(process.env.WS_PI_TEST_CLI);
  // Like the original packaging test, use the installed development host peers,
  // never a personal Pi config or a second host core inside the payload.
  for (const root of [packageRoot, dirname(packageRoot)]) {
    const host = join(root, 'node_modules/@earendil-works/pi-coding-agent');
    if (!existsSync(join(host, 'package.json'))) continue;
    const manifest = JSON.parse(readFileSync(join(host, 'package.json'), 'utf8'));
    const cli = join(host, typeof manifest.bin === 'string' ? manifest.bin : manifest.bin.pi);
    if (existsSync(cli)) return realpathSync(cli);
  }
  assert.fail('Install the package development dependencies or set WS_PI_TEST_CLI to a supported test host');
}

// The ticket requires offline tests: dry-run packing reads installed dependencies
// only, with scripts disabled and its own empty cache; real installs are opt-in.
test('offline npm pack inventory includes the helper and bundled search license, not host cores', t => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'web-pack-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const packed = JSON.parse(run('npm', ['pack', '--dry-run', '--json', '--ignore-scripts', '--offline'], packageRoot, fixtureEnv(root)))[0];
  const entries = packed.files.map((file: { path: string }) => file.path);
  for (const path of ['src/web-search-helper.mjs', 'src/helper-host.mjs', 'node_modules/pi-web-access/README.md', 'node_modules/pi-web-access/LICENSE']) assert.ok(entries.includes(path), path);
  assert.equal(entries.some((path: string) => /node_modules\/@earendil-works\/pi-(ai|coding-agent|tui)\//.test(path)), false, 'physical pack inventory, not lockfile annotations');
  const manifest = JSON.parse(readFileSync(join(packageRoot, 'package.json'), 'utf8'));
  assert.ok(manifest.bundledDependencies.includes('pi-web-access'));
  for (const name of peers) assert.ok(manifest.peerDependencies[`@earendil-works/${name}`]);
});

// Consume committed producer output without dependency installation. Probe the
// helper by default against an explicit host CLI with isolated config/state.
test('prepared package-only install/update carries the external helper closure offline', { timeout: 180_000 }, async t => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'web-package-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const env = fixtureEnv(root);
  const previous = process.argv[1];
  process.argv[1] = testCli();
  t.after(() => { process.argv[1] = previous; });
  for (const product of ['agents-plugin', 'agents-plugin-wsflow']) {
    const owner = join(root, product);
    for (const name of ['native-pi', 'bin', 'rsrc', 'skills', 'runtime.json', 'dashboard-config-manifest.json']) {
      mkdirSync(dirname(join(owner, name)), { recursive: true });
      cpSync(join(packageRoot, '..', product, name), join(owner, name), { recursive: true });
    }
    let installed = '';
    for (let pass = 0; pass < 2; pass++) {
      const result = spawnSync(process.env.WS_MCP_HOLDER_PYTHON || 'python3', ['-I', '-B', join(owner, 'bin/ws-pi-payload.py'), '--plugin', owner, '--materialize', join(root, 'native-cache')], { env, cwd: root, encoding: 'utf8', timeout: 150_000 });
      assert.equal(result.status, 0, `${result.error ?? ''}\n${result.stderr}`);
      const next = realpathSync(result.stdout.trim());
      if (pass) assert.equal(next, installed, 'second offline consumption reuses verified closure');
      installed = next;
    }
    assert.equal(prepareSkillsDir(installed, dirname(installed)), join(installed, 'skills'), 'native skills resolve entirely from the materialized owner overlay');
    const manifest = JSON.parse(readFileSync(join(installed, 'package.json'), 'utf8'));
    assert.ok(manifest.files.includes('scripts/'), 'plain npm/Pi packages retain bundle freshness inputs');
    const upstream = join(installed, 'node_modules/pi-web-access');
    assert.equal(JSON.parse(readFileSync(join(upstream, 'package.json'), 'utf8')).version, '0.29.0');
    assert.match(readFileSync(join(upstream, 'LICENSE'), 'utf8'), /MIT License/);
    for (const name of peers) {
      assert.ok(manifest.peerDependencies[`@earendil-works/${name}`]);
      assert.equal(existsSync(join(installed, 'node_modules/@earendil-works', name)), false);
    }
    assert.equal(existsSync(join(root, 'node_modules')), false, 'no ancestor checkout dependencies');
    await createWebSearch({ packageRoot: installed, env, cwd: root }).probe();

    // Plain Pi Git-install shape: prepared production dependencies land at the
    // clone root rather than beside the extension. Use the same physical tree,
    // not npm, producer links, or a user/global dependency fallback.
    const clone = join(root, product + '-git-clone');
    const ext = join(clone, 'agents-plugin-pi');
    mkdirSync(join(ext, 'src'), { recursive: true });
    cpSync(join(installed, 'node_modules'), join(clone, 'node_modules'), { recursive: true });
    for (const name of ['web-search-helper.mjs', 'helper-host.mjs']) cpSync(join(installed, 'src', name), join(ext, 'src', name));
    await createWebSearch({ packageRoot: ext, env, cwd: root }).probe();
    const webManifest = join(clone, 'node_modules/pi-web-access/package.json');
    const original = readFileSync(webManifest);
    writeFileSync(webManifest, JSON.stringify({ name: 'pi-web-access', version: '0.0.0' }));
    await assert.rejects(createWebSearch({ packageRoot: ext, env }).probe(), /web-search-extension-missing/);
    writeFileSync(webManifest, original);
    rmSync(join(clone, 'node_modules/pi-web-access'), { recursive: true, force: true });
    await assert.rejects(createWebSearch({ packageRoot: ext, env }).probe(), /web-search-extension-missing/);
  }
});

// The ticket's offline-test rule keeps real npm installation/network opt-in;
// retain the tarball update and tracked-lock Git-install checks rather than
// deleting these lanes merely because the default suite cannot download deps.
test('opt-in real tarball install/update and Git install honor package and tracked lock contracts', {
  skip: process.env.WS_PI_TEST_NETWORK !== '1', timeout: 240_000,
}, async t => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'web-install-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const env = fixtureEnv(root);
  const previous = process.argv[1];
  process.argv[1] = testCli();
  t.after(() => { process.argv[1] = previous; });
  const packed = JSON.parse(run('npm', ['pack', '--ignore-scripts', '--json', '--pack-destination', root], packageRoot, env))[0];
  const tarball = join(root, packed.filename);
  const entries = run('tar', ['-tzf', tarball], root, env).trim().split('\n');
  for (const path of ['package/src/web-search-helper.mjs', 'package/node_modules/pi-web-access/README.md', 'package/node_modules/pi-web-access/LICENSE']) assert.ok(entries.includes(path), path);
  assert.equal(entries.some(path => /node_modules\/@earendil-works\/pi-(ai|coding-agent|tui)\//.test(path)), false);
  const install = join(root, 'host');
  mkdirSync(install);
  writeFileSync(join(install, 'package.json'), '{"private":true,"type":"module"}');
  const installed = join(install, 'node_modules/ws-pi-bridge');
  for (let pass = 0; pass < 2; pass++) {
    if (pass) rmSync(installed, { recursive: true, force: true });
    run('npm', ['install', '--ignore-scripts', '--legacy-peer-deps', '--no-audit', '--no-fund', '--package-lock=false', tarball], install, env);
    assert.ok(existsSync(join(installed, 'src/web-search-helper.mjs')));
  }
  const manifest = JSON.parse(readFileSync(join(installed, 'package.json'), 'utf8'));
  const upstream = join(installed, 'node_modules/pi-web-access');
  assert.equal(JSON.parse(readFileSync(join(upstream, 'package.json'), 'utf8')).version, '0.29.0');
  assert.match(readFileSync(join(upstream, 'LICENSE'), 'utf8'), /MIT License/);
  for (const name of peers) {
    assert.ok(manifest.peerDependencies[`@earendil-works/${name}`]);
    assert.equal(existsSync(join(installed, 'node_modules/@earendil-works', name)), false);
    assert.equal(existsSync(join(install, 'node_modules/@earendil-works', name)), false, 'npm did not install another host core');
  }
  await createWebSearch({ packageRoot: installed, env }).probe();

  const clone = join(root, 'clone');
  mkdirSync(clone);
  const rootDependencies = JSON.parse(readFileSync(join(packageRoot, '../package.json'), 'utf8')).dependencies;
  assert.deepEqual(rootDependencies, manifest.dependencies);
  writeFileSync(join(clone, 'package.json'), JSON.stringify({ private: true, dependencies: rootDependencies }));
  const lock = join(packageRoot, 'package-lock.json');
  cpSync(lock, join(clone, 'package-lock.json'));
  run('npm', ['install', '--ignore-scripts', '--legacy-peer-deps', '--no-audit', '--no-fund'], clone, env);
  assert.equal(JSON.parse(readFileSync(join(clone, 'node_modules/undici/package.json'), 'utf8')).version, JSON.parse(readFileSync(lock, 'utf8')).packages['node_modules/undici'].version, 'clone install honors the tracked lockfile');
  const ext = join(clone, 'agents-plugin-pi');
  mkdirSync(join(ext, 'src'), { recursive: true });
  for (const name of ['web-search-helper.mjs', 'helper-host.mjs']) cpSync(join(packageRoot, 'src', name), join(ext, 'src', name));
  // The old fixture supplied external host peers at the clone root. Preserve
  // that shape too; these links never enter either prepared product archive.
  const scope = join(clone, 'node_modules/@earendil-works');
  mkdirSync(scope, { recursive: true });
  for (const name of peers) {
    assert.equal(existsSync(join(scope, name)), false);
    symlinkSync(realpathSync(join(packageRoot, 'node_modules/@earendil-works', name)), join(scope, name));
  }
  await createWebSearch({ packageRoot: ext, env }).probe();
  rmSync(join(clone, 'node_modules/pi-web-access'), { recursive: true, force: true });
  await assert.rejects(createWebSearch({ packageRoot: ext, env }).probe(), /web-search-extension-missing/);
});

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdtempSync, mkdirSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createWebSearch } from '../src/web-search.ts';

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));

// Consume committed producer output, never run npm or use an ambient npm cache.
// The opt-in CLI is an actual supported test host; no personal Pi config loads.
test('prepared package-only install/update carries the external helper closure offline', { timeout: 90_000 }, async t => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'web-package-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const env = { ...process.env };
  for (const [key, folder] of Object.entries({ HOME: 'home', USERPROFILE: 'home', WS_CACHE_HOME: 'cache', WS_CONFIG_HOME: 'config', npm_config_cache: 'npm-cache', PI_CODING_AGENT_DIR: 'pi-agent' })) {
    mkdirSync(join(root, folder), { recursive: true }); env[key] = join(root, folder);
  }
  const cli = process.env.WS_PI_TEST_CLI;
  const previous = process.argv[1];
  if (cli) process.argv[1] = realpathSync(cli);
  t.after(() => { process.argv[1] = previous; });
  for (const product of ['agents-plugin', 'agents-plugin-wsflow']) {
    const owner = join(root, product);
    for (const name of ['native-pi', 'bin', 'rsrc', 'skills', 'runtime.json', 'dashboard-config-manifest.json']) {
      mkdirSync(dirname(join(owner, name)), { recursive: true });
      cpSync(join(packageRoot, '..', product, name), join(owner, name), { recursive: true });
    }
    let installed = '';
    for (let pass = 0; pass < 2; pass++) {
      const result = spawnSync(process.env.WS_MCP_HOLDER_PYTHON || 'python3', ['-I', '-B', join(owner, 'bin/ws-pi-payload.py'), '--plugin', owner, '--materialize', join(root, 'native-cache')], { env, cwd: root, encoding: 'utf8', timeout: 30_000 });
      assert.equal(result.status, 0, `${result.error ?? ''}\n${result.stderr}`);
      const next = realpathSync(result.stdout.trim());
      if (pass) assert.equal(next, installed, 'second offline consumption reuses verified closure');
      installed = next;
    }
    const manifest = JSON.parse(readFileSync(join(installed, 'package.json'), 'utf8'));
    assert.ok(manifest.files.includes('scripts/'), 'plain npm/Pi packages retain bundle freshness inputs');
    const upstream = join(installed, 'node_modules/pi-web-access');
    assert.equal(JSON.parse(readFileSync(join(upstream, 'package.json'), 'utf8')).version, '0.29.0');
    assert.match(readFileSync(join(upstream, 'LICENSE'), 'utf8'), /MIT License/);
    assert.equal(existsSync(join(installed, 'node_modules/@earendil-works/pi-coding-agent')), false);
    assert.equal(existsSync(join(root, 'node_modules')), false, 'no ancestor checkout dependencies');
    if (cli) await createWebSearch({ packageRoot: installed, env, cwd: root }).probe();
    else t.diagnostic('helper execution requires WS_PI_TEST_CLI; archive/closure assertions still ran');

    // Plain Pi Git-install shape: prepared production dependencies land at the
    // clone root rather than beside the extension. Use the same physical tree,
    // not npm, producer links, or a user/global dependency fallback.
    const clone = join(root, product + '-git-clone');
    const ext = join(clone, 'agents-plugin-pi');
    mkdirSync(join(ext, 'src'), { recursive: true });
    cpSync(join(installed, 'node_modules'), join(clone, 'node_modules'), { recursive: true });
    for (const name of ['web-search-helper.mjs', 'helper-host.mjs']) cpSync(join(installed, 'src', name), join(ext, 'src', name));
    if (cli) await createWebSearch({ packageRoot: ext, env, cwd: root }).probe();
    const webManifest = join(clone, 'node_modules/pi-web-access/package.json');
    const original = readFileSync(webManifest);
    writeFileSync(webManifest, JSON.stringify({ name: 'pi-web-access', version: '0.0.0' }));
    await assert.rejects(createWebSearch({ packageRoot: ext, env }).probe(), /web-search-extension-missing/);
    writeFileSync(webManifest, original);
    rmSync(join(clone, 'node_modules/pi-web-access'), { recursive: true, force: true });
    await assert.rejects(createWebSearch({ packageRoot: ext, env }).probe(), /web-search-extension-missing/);
  }
});

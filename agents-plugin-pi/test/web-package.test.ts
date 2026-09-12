import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, mkdirSync, readFileSync, realpathSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createWebSearch } from '../src/web-search.ts';

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
function run(command: string, args: string[], cwd: string): string {
  const result = spawnSync(command, args, { cwd, encoding: 'utf8', timeout: 90_000, maxBuffer: 8 * 1024 * 1024 });
  assert.equal(result.status, 0, `${command}: ${result.error ?? ''}\n${result.stdout}\n${result.stderr}`);
  return result.stdout;
}

test('packed install/update carries the pinned search license and resolves host peers without bundled core copies', { timeout: 180_000 }, async t => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'web-package-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const packed = JSON.parse(run('npm', ['pack', '--ignore-scripts', '--json', '--pack-destination', root], packageRoot))[0];
  const tarball = join(root, packed.filename);
  const entries = run('tar', ['-tzf', tarball], root).trim().split('\n');
  for (const path of ['package/src/web-search-helper.mjs', 'package/node_modules/pi-web-access/README.md', 'package/node_modules/pi-web-access/LICENSE']) assert.ok(entries.includes(path), path);
  assert.equal(entries.some(path => /node_modules\/@earendil-works\/pi-(ai|coding-agent|tui)\//.test(path)), false, 'physical artifact, not lockfile inBundle annotations');
  const install = join(root, 'host');
  mkdirSync(install);
  writeFileSync(join(install, 'package.json'), '{"private":true,"type":"module"}');
  // Existing local npm cache supplies ordinary dependencies. No registry/network
  // and no lifecycle scripts or automatic peer installations are permitted.
  const installed = join(install, 'node_modules', 'ws-pi-bridge');
  for (let pass = 0; pass < 2; pass++) {
    if (pass) rmSync(installed, { recursive: true, force: true });
    run('npm', ['install', '--offline', '--ignore-scripts', '--legacy-peer-deps', '--no-audit', '--no-fund', '--package-lock=false', tarball], install);
    assert.ok(existsSync(join(installed, 'src', 'web-search-helper.mjs')), 'install/update restores the shipped helper');
  }
  const manifest = JSON.parse(readFileSync(join(installed, 'package.json'), 'utf8'));
  const upstream = join(installed, 'node_modules', 'pi-web-access');
  assert.equal(JSON.parse(readFileSync(join(upstream, 'package.json'), 'utf8')).version, '0.29.0');
  assert.match(readFileSync(join(upstream, 'LICENSE'), 'utf8'), /MIT License/);
  for (const name of ['pi-ai', 'pi-coding-agent', 'pi-tui']) {
    assert.ok(manifest.peerDependencies[`@earendil-works/${name}`]);
    assert.equal(existsSync(join(installed, 'node_modules', '@earendil-works', name)), false);
    const peerScope = join(install, 'node_modules', '@earendil-works');
    mkdirSync(peerScope, { recursive: true });
    assert.equal(existsSync(join(peerScope, name)), false, 'npm did not auto-install a second host core');
    symlinkSync(realpathSync(join(packageRoot, 'node_modules', '@earendil-works', name)), join(peerScope, name));
  }
  const env = { ...process.env, PI_CODING_AGENT_DIR: root };
  await createWebSearch({ packageRoot: installed, env }).probe();
});

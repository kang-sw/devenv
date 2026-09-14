import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { cpSync, existsSync, mkdtempSync, mkdirSync, readFileSync, realpathSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
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

  // Git-install shape: Pi's `pi install git:...` clones the repo and runs
  // `npm install` only against the repo-root manifest (260903's git-root-install
  // model), which the mirror this ticket's Phase 1 added makes carry the same
  // six runtime dependencies as agents-plugin-pi/package.json. That lands
  // `pi-web-access` at the clone root, one level above the extension subdir —
  // not inside the subdir's own `node_modules`. Reproduce that shape with a
  // real install against the actual mirrored root manifest and confirm
  // resolution still succeeds through the clone-root candidate.
  const cloneRoot = join(root, 'clone');
  mkdirSync(cloneRoot);
  const rootDependencies = JSON.parse(readFileSync(join(packageRoot, '..', 'package.json'), 'utf8')).dependencies;
  writeFileSync(join(cloneRoot, 'package.json'), JSON.stringify({ private: true, dependencies: rootDependencies }));
  // Same offline invariant as the packed install above: the local npm cache
  // already has these exact pinned versions from agents-plugin-pi's own install.
  run('npm', ['install', '--offline', '--ignore-scripts', '--legacy-peer-deps', '--no-audit', '--no-fund', '--package-lock=false'], cloneRoot);
  const extDir = join(cloneRoot, 'agents-plugin-pi');
  mkdirSync(join(extDir, 'src'), { recursive: true });
  cpSync(join(packageRoot, 'src', 'web-search-helper.mjs'), join(extDir, 'src', 'web-search-helper.mjs'));
  const clonePeerScope = join(cloneRoot, 'node_modules', '@earendil-works');
  mkdirSync(clonePeerScope, { recursive: true });
  for (const name of ['pi-ai', 'pi-coding-agent', 'pi-tui']) {
    symlinkSync(realpathSync(join(packageRoot, 'node_modules', '@earendil-works', name)), join(clonePeerScope, name));
  }
  await createWebSearch({ packageRoot: extDir, env }).probe();

  // Absent from both known locations fails closed rather than resolving
  // anywhere else (e.g. a global/user copy via require.resolve's free walk).
  rmSync(join(cloneRoot, 'node_modules', 'pi-web-access'), { recursive: true, force: true });
  await assert.rejects(createWebSearch({ packageRoot: extDir, env }).probe(), /web-search-extension-missing/);
});

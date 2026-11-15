import assert from 'node:assert/strict';
import { cpSync, mkdtempSync, mkdirSync, realpathSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { helperHost } from '../src/helper-host.mjs';

test('plain Node helpers resolve Pi peers from the real CLI, not the payload or cwd', t => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'ws-helper-host-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const file = (name, body) => { const p = join(root, name); mkdirSync(dirname(p), { recursive: true }); writeFileSync(p, body); return p; };
  const cli = file('host/dist/bundle/cli.js', '');
  file('host/dist/bundle/index.js', 'export class ModelRegistry {}');
  file('host/dist/index.js', 'throw Error("wrong graph")');
  file('host/package.json', '{"name":"@earendil-works/pi-coding-agent"}');
  file('host/node_modules/@earendil-works/pi-tui/package.json', '{"name":"@earendil-works/pi-tui","main":"index.js"}');
  const tui = file('host/node_modules/@earendil-works/pi-tui/index.js', 'export class Text {}');
  file('host/node_modules/@earendil-works/pi-ai/package.json', '{"name":"@earendil-works/pi-ai","exports":{".":"./index.js","./compat":"./compat.js"}}');
  file('host/node_modules/@earendil-works/pi-ai/index.js', '');
  file('host/node_modules/@earendil-works/pi-ai/compat.js', '');
  const resolved = helperHost(cli);
  assert.equal(resolved.sdk, join(dirname(cli), 'index.js'));
  assert.equal(resolved.alias['@earendil-works/pi-tui'], tui);
  assert.equal(resolved.alias['@earendil-works/pi-coding-agent'], resolved.sdk);
  rmSync(resolved.sdk);
  assert.throws(() => helperHost(cli), /SDK unavailable/);
  assert.equal(helperHost(file('ordinary-test.mjs', '')), undefined);
});

for (const requireBranch of [false, true]) {
  test(`host peers honor import conditions, not cwd/aliases or require (${requireBranch})`, t => {
    const root = realpathSync(mkdtempSync(join(tmpdir(), 'ws-helper-esm-host-')));
    const previousCwd = process.cwd();
    const previousAlias = process.env.JITI_ALIAS;
    t.after(() => {
      process.chdir(previousCwd);
      if (previousAlias === undefined) delete process.env.JITI_ALIAS;
      else process.env.JITI_ALIAS = previousAlias;
      rmSync(root, { recursive: true, force: true });
    });
    const file = (name, body) => { const p = join(root, name); mkdirSync(dirname(p), { recursive: true }); writeFileSync(p, body); return p; };
    const conditional = target => requireBranch ? { require: './wrong.cjs', import: target } : { types: './unused.d.ts', import: target };
    const cli = file('host/dist/bundle/cli.js', 'throw Error("CLI must not execute")');
    const sdk = file('host/dist/bundle/index.js', 'throw Error("SDK must not import")');
    file('host/dist/index.js', 'throw Error("ordinary SDK is the wrong graph")');
    file('host/package.json', JSON.stringify({ name: '@earendil-works/pi-coding-agent', type: 'module' }));
    const expected = { '@earendil-works/pi-coding-agent': sdk };
    for (const name of ['pi-tui', 'pi-ai']) {
      const prefix = `host/node_modules/@earendil-works/${name}/`;
      const exports = { '.': conditional('./index.js'), ...(name === 'pi-ai' ? { './compat': conditional('./compat.js') } : {}) };
      file(prefix + 'package.json', JSON.stringify({ name: `@earendil-works/${name}`, type: 'module', exports }));
      expected[`@earendil-works/${name}`] = file(prefix + 'index.js', 'throw Error("peer must not import")');
      if (name === 'pi-ai') expected['@earendil-works/pi-ai/compat'] = file(prefix + 'compat.js', 'throw Error("compat must not import")');
      file(prefix + 'wrong.cjs', 'throw Error("require condition is wrong")');
      file(`node_modules/@earendil-works/${name}/package.json`, JSON.stringify({ name: `@earendil-works/${name}`, exports: { '.': './index.js', './compat': './compat.js' } }));
      file(`node_modules/@earendil-works/${name}/index.js`, 'throw Error("outer peer is wrong")');
      file(`node_modules/@earendil-works/${name}/compat.js`, 'throw Error("outer compat is wrong")');
    }
    const linked = join(root, 'pi-link');
    symlinkSync(cli, linked);
    const producer = join(root, 'producer');
    mkdirSync(producer);
    process.chdir(producer);
    process.env.JITI_ALIAS = JSON.stringify({ '@earendil-works/pi-ai': join(root, 'node_modules/@earendil-works/pi-ai/index.js') });
    const resolved = helperHost(linked);
    assert.equal(resolved.sdk, sdk);
    assert.deepEqual(resolved.alias, expected);
    // A loaded package's exports are cached independently of moduleCache.
    // Make the negative host cold; keep the outer compat decoy unchanged.
    cpSync(join(root, 'host'), join(root, 'cold-host'), { recursive: true });
    file('cold-host/node_modules/@earendil-works/pi-ai/package.json', JSON.stringify({ name: '@earendil-works/pi-ai', type: 'module', exports: { '.': conditional('./index.js') } }));
    assert.throws(() => helperHost(join(root, 'cold-host/dist/bundle/cli.js')), /export|resolve|module|package/i, 'missing host compat must not fall back to the outer copy');
  });
}

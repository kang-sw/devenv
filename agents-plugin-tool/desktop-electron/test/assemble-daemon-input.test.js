'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { assemble } = require('../scripts/assemble-daemon-input.cjs');
const { selectDaemon } = require('../daemon-selection');
const { copyPackage } = require('../daemon-stage');
const repo = path.resolve(__dirname, '../../..');

for (const product of ['wsflow', 'ws']) for (const platform of ['darwin', 'win32']) {
  test(`assemble real ${product} package for ${platform} from non-executable bytes`, t => {
    const base = fs.mkdtempSync(path.join(os.tmpdir(), 'desktop assembly test '));
    t.after(() => fs.rmSync(base, { recursive: true, force: true }));
    const source = path.join(repo, product === 'ws' ? 'agents-plugin' : 'agents-plugin-wsflow');
    const contract = JSON.parse(fs.readFileSync(path.join(source, 'runtime.json'), 'utf8'));
    const appRoot = path.join(base, 'app'), userData = path.join(base, 'user data');
    fs.mkdirSync(appRoot); fs.mkdirSync(userData);
    const binary = path.join(base, 'release-asset');
    const metadata = product === 'ws' ? binary + '.metadata.json' : path.join(base, 'identity.json');
    // Not an executable, including when testing Windows inputs on macOS.
    const bytes = Buffer.from('fake ws-mcp bytes; never execute');
    fs.writeFileSync(binary, bytes);
    const identity = { schema_version: 1, version: contract.plugin_version,
      binary_sha256: crypto.createHash('sha256').update(bytes).digest('hex'),
      mcp_protocol: contract.mcp_protocol };
    const save = () => fs.writeFileSync(metadata, JSON.stringify(identity));
    save();
    const out = path.join(appRoot, 'daemon');
    const options = { binary, out, platform, ...(product === 'ws' ? { package: product } : { metadata }) };
    const assembled = assemble(options);
    const select = () => selectDaemon({ appRoot, userData, platform });
    const selected = select();
    assert.equal(selected.source, 'bundled');
    assert.equal(selected.contract.plugin, product);
    assert.equal(selected.contract.plugin_version, contract.plugin_version);
    assert.equal(selected.metadata.binary_sha256, identity.binary_sha256);
    assert.equal(assembled.binary, selected.binary);
    assert.equal(path.basename(selected.binary), platform === 'win32' ? 'ws-mcp.exe' : 'ws-mcp');
    assert.deepEqual(fs.readFileSync(selected.binary), bytes);
    if (platform === 'darwin' && process.platform !== 'win32') assert(fs.statSync(selected.binary).mode & 0o111);
    const plugin = JSON.parse(fs.readFileSync(path.join(out, '.claude-plugin', 'plugin.json'), 'utf8'));
    assert.equal(plugin.version, contract.plugin_version);
    for (const name of ['bin/ws-mcp-launcher.py', 'config-manifest.json', 'dashboard-config-manifest.json',
      'claude-agents/effort-low.md', 'rsrc/manifest.json', 'skills/manifest.json']) {
      assert(fs.statSync(path.join(out, name)).isFile(), name);
    }
    // Exercise build.cjs's copy gate too: no links anywhere, including hooks/bin.
    const copy = copyPackage(out, path.join(base, 'copy'), 'bundled', platform);
    assert.equal(copy.contract.plugin, product);
    assert.throws(() => assemble(options), /EEXIST/);
    assert.deepEqual(fs.readFileSync(selected.binary), bytes, 'existing output was not removed');

    // Both the assembly gate and the app's own selector refuse a wrong core.
    identity.version = '99.0.0'; save();
    const bad = path.join(base, 'bad');
    assert.throws(() => assemble({ ...options, out: bad }), /version\/contract mismatch/);
    assert.equal(fs.existsSync(bad), false, 'failed owned output was cleaned');
    fs.copyFileSync(metadata, selected.binary + '.metadata.json');
    assert.throws(select, /version\/contract mismatch/);
  });
}

test('assembly rejects unsupported packages and targets before writing output', () => {
  assert.throws(() => assemble({ package: 'pi' }), /Package must be wsflow or ws/);
  assert.throws(() => assemble({ platform: 'linux' }), /Target platform/);
  assert.throws(() => assemble({ platform: 'win32' }), /--binary and --out/);
});

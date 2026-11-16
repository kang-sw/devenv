'use strict';
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const { selectDaemon, SETTING } = require('../daemon-selection');
const sha = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
function fixture(t, platform = 'win32') {
  const base = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-daemon-input-'));
  t.after(() => fs.rmSync(base, { recursive: true, force: true }));
  const appRoot = path.join(base, 'app'), userData = path.join(base, 'userData');
  fs.mkdirSync(appRoot); fs.mkdirSync(userData);
  const binaryName = platform === 'win32' ? 'ws-mcp.exe' : 'ws-mcp';
  function make(root, version = '0.50.1-dev') {
    fs.mkdirSync(root, { recursive: true });
    const binary = path.join(root, binaryName), contractFile = path.join(root, 'runtime.json');
    // These bytes cannot execute. Tests never invoke a binary or child process.
    fs.writeFileSync(binary, 'fixture bytes, not an executable');
    const contract = { plugin: 'ws', plugin_version: '0.50.1', mcp_protocol: '2025-03-26',
      tools: { 'runtime.read': '>=0.50.1' }, commands: { 'runtime.info': '>=0.50.1' } };
    const metadata = { schema_version: 1, version, binary_sha256: sha(fs.readFileSync(binary)),
      mcp_protocol: '2025-03-26', tools: ['runtime.read'], commands: ['runtime.info'] };
    const save = (file, value) => fs.writeFileSync(file, JSON.stringify(value));
    save(contractFile, contract); save(binary + '.metadata.json', metadata);
    const pluginFile = path.join(root, '.claude-plugin', 'plugin.json');
    fs.mkdirSync(path.dirname(pluginFile));
    for (const rel of ['bin/ws-mcp-launcher.py', 'claude-agents/effort-low.md']) {
      fs.mkdirSync(path.dirname(path.join(root, rel)), { recursive: true });
      fs.writeFileSync(path.join(root, rel), 'text-only fixture');
    }
    for (const rel of ['config-manifest.json', 'dashboard-config-manifest.json']) save(path.join(root, rel), { schema_version: 1 });
    const plugin = { name: 'ws', agents: ['./claude-agents/effort-low.md'], mcpServers: { ws: {
      args: ['${CLAUDE_PLUGIN_ROOT}/bin/ws-mcp-launcher.py', 'serve', '--stdio'],
      env: { WS_MCP_CONFIG_MANIFESTS: '${CLAUDE_PLUGIN_ROOT}/config-manifest.json;${CLAUDE_PLUGIN_ROOT}/dashboard-config-manifest.json' },
    } } };
    save(pluginFile, plugin);
    for (const [tree, rel] of [['rsrc', 'hello/hello.md'], ['skills', 'hello/SKILL.md']]) {
      const dir = path.join(root, tree); fs.mkdirSync(path.join(dir, 'hello'), { recursive: true });
      fs.writeFileSync(path.join(dir, rel), 'hello\r\nworld\r\n');
      save(path.join(dir, 'manifest.json'), { schema_version: 1, files: { [rel]: sha('hello\nworld\n') } });
    }
    return { root, binary, contract, metadata, contractFile, pluginFile, plugin, save };
  }
  const bundled = make(path.join(appRoot, 'daemon'));
  const dev = make(path.join(base, 'selected'), '0.50.3-dev');
  const choose = value => fs.writeFileSync(path.join(userData, SETTING), JSON.stringify(value));
  const select = () => selectDaemon({ appRoot, userData, platform });
  return { base, appRoot, userData, bundled, dev, choose, select };
}
module.exports = { fixture, sha };

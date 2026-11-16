import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { applyWorkflowOwner } from '../src/workflow-owner.ts';

test('prepared native packages select their owning product; plain Pi remains unchanged', t => {
  const root = mkdtempSync(join(tmpdir(), 'ws-native-owner-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const env = { WS_RSRC_ROOT: 'ambient', WS_MCP_NO_AGENT: 'ambient' };
  applyWorkflowOwner(root, env);
  assert.deepEqual(env, { WS_RSRC_ROOT: 'ambient', WS_MCP_NO_AGENT: 'ambient' });
  for (const plugin of ['ws', 'wsflow']) {
    writeFileSync(join(root, 'workflow-owner.json'), JSON.stringify({ plugin }));
    applyWorkflowOwner(root, env);
    assert.equal(env.WS_RSRC_ROOT, join(root, 'rsrc'));
    assert.equal(env.WS_SKILLS_ROOT, join(root, 'skills'));
    assert.equal(env.WS_MCP_NAMESPACE, plugin);
    const config = JSON.parse(readFileSync(fileURLToPath(new URL(`../../${plugin === 'ws' ? 'agents-plugin' : 'agents-plugin-wsflow'}/.mcp.json`, import.meta.url)), 'utf8'));
    assert.equal(env.WS_MCP_SETUP_TOOL, config.mcpServers[plugin].env.WS_MCP_SETUP_TOOL ?? 'ws.setup');
    assert.equal(env.WS_MCP_NO_AGENT, plugin === 'wsflow' ? '1' : '');
  }
  writeFileSync(join(root, 'workflow-owner.json'), '{"plugin":"other"}');
  assert.throws(() => applyWorkflowOwner(root, env), /Invalid native workflow/);
});

import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { applyWorkflowOwner, consumeWorkflowOwner, workflowConfigManifests } from '../src/workflow-owner.ts';

test('hosted source and prepared Pi use the selected whole workflow package', t => {
  const base = mkdtempSync(join(tmpdir(), 'ws-hosted-owner-'));
  t.after(() => rmSync(base, { recursive: true, force: true }));
  const pi = join(base, 'pi');
  mkdirSync(pi);
  for (const owner of ['ws', 'wsflow']) {
    const root = join(base, owner);
    mkdirSync(join(root, '.claude-plugin'), { recursive: true });
    writeFileSync(join(root, '.claude-plugin', 'plugin.json'), JSON.stringify({ name: owner }));
    for (const prepared of [false, true]) {
      if (prepared) writeFileSync(join(pi, 'workflow-owner.json'), JSON.stringify({ plugin: owner === 'ws' ? 'wsflow' : 'ws' }));
      const env = { WS_PI_WORKFLOW_ROOT: root, WS_MCP_NAMESPACE: 'starter', WS_MCP_NO_AGENT: 'starter' };
      applyWorkflowOwner(pi, env);
      assert.equal(env.WS_MCP_NAMESPACE, owner);
      assert.equal(env.WS_MCP_NO_AGENT, owner === 'wsflow' ? '1' : '');
      assert.equal(env.WS_MCP_SETUP_TOOL, owner === 'wsflow' ? 'setup' : 'ws.setup');
      assert.equal(env.WS_MCP_CLI_NAME, owner === 'wsflow' ? 'wsflow-cli' : 'ws-cli');
      assert.equal(env.WS_RSRC_ROOT, join(root, 'rsrc'));
      assert.equal(env.WS_SKILLS_ROOT, join(root, 'skills'));
      assert.deepEqual(workflowConfigManifests(pi, env), [
        join(pi, 'config-manifest.json'),
        join(root, 'config-manifest.json'),
        join(root, 'dashboard-config-manifest.json'),
      ]);
      rmSync(join(pi, 'workflow-owner.json'), { force: true });
    }
  }
  assert.deepEqual(workflowConfigManifests(pi, {}), [join(pi, 'config-manifest.json'), join(pi, 'dashboard-config-manifest.json')]);
  assert.throws(() => applyWorkflowOwner(pi, { WS_PI_WORKFLOW_ROOT: 'relative' }), /must be absolute/);
  const invalid = join(base, 'other');
  mkdirSync(join(invalid, '.claude-plugin'), { recursive: true });
  writeFileSync(join(invalid, '.claude-plugin', 'plugin.json'), '{"name":"other"}');
  assert.throws(() => workflowConfigManifests(pi, { WS_PI_WORKFLOW_ROOT: invalid }), /Invalid hosted workflow/);
});

test('only the actual hosted Pi consumes selection; nested Pi keeps its own owner', async t => {
  const base = mkdtempSync(join(tmpdir(), 'ws-hosted-identity-'));
  t.after(() => rmSync(base, { recursive: true, force: true }));
  const root = join(base, 'ws');
  mkdirSync(join(root, '.claude-plugin'), { recursive: true });
  writeFileSync(join(root, '.claude-plugin', 'plugin.json'), '{"name":"ws"}');
  const pi = join(base, 'pi');
  mkdirSync(pi);
  writeFileSync(join(pi, 'workflow-owner.json'), '{"plugin":"wsflow"}');
  for (const hosted of [true, false]) {
    const env = { WS_HOSTED_SESSION: '1', WS_HOSTED_SESSION_KEY: 'key', WS_HOSTED_SESSION_ROW: 'row', WS_PI_WORKFLOW_ROOT: root };
    await consumeWorkflowOwner(pi, env, { pid: 42, readRow: async () => JSON.stringify({ host_pid: hosted ? 42 : 43 }), sleep: async () => {} });
    assert.equal(env.WS_PI_WORKFLOW_ROOT, undefined, 'launch-only root must not leak to children');
    assert.equal(env.WS_MCP_NAMESPACE, hosted ? 'ws' : 'wsflow');
    assert.equal(env.WS_RSRC_ROOT, join(hosted ? root : pi, 'rsrc'));
    assert.deepEqual(workflowConfigManifests(pi), hosted
      ? [join(pi, 'config-manifest.json'), join(root, 'config-manifest.json'), join(root, 'dashboard-config-manifest.json')]
      : [join(pi, 'config-manifest.json'), join(pi, 'dashboard-config-manifest.json')]);
  }
});

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, readdirSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { RpcClient } from '@earendil-works/pi-coding-agent';
import { createAgentStorageContext } from '../src/agent-storage.ts';

// No LLM request is sent. RpcClient.start/stop/getState, extension loading,
// the control-channel hello, readiness publication and production
// spawn/resume remain real.
test('real Explore extension startup and dormant restart prove fresh facade readiness without paid inference', { timeout: 60_000 }, async t => {
  const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
  const cli = join(dirname(fileURLToPath(import.meta.resolve('@earendil-works/pi-coding-agent'))), 'cli.js');
  const oldArgv = process.argv[1];
  process.argv[1] = cli;
  const { spawnAgent, sendToAgent } = await import('../src/spawner.ts');
  process.argv[1] = oldArgv;
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'web-real-startup-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const prompts: string[] = [];
  t.mock.method(RpcClient.prototype, 'prompt', async (message: string) => { prompts.push(message); });
  const registry: any = new Map();
  const context: any = {
    cwd: packageRoot, storage: createAgentStorageContext('startup-lead', root), wsToolNames: [], inheritModel: 'openrouter/openai/gpt-4o',
    extensionPath: join(packageRoot, 'src', 'index.ts'), toolGroup: 'read-only-explore', spawnRole: 'explore', exploreMode: 'code-search',
  };
  try {
    const result = await spawnAgent(registry, context, {
      systemPromptPath: join(packageRoot, 'explore-guide.md'), prompt: 'not dispatched to a model',
    });
    const record = registry.get(result.agent_id);
    const first = record.channel;
    assert.ok(first, 'the launch owns a live control channel');
    assert.deepEqual(await first.readiness('web'), { tools: ['web_search', 'ws_web_fetch'] });
    assert.equal(first.generation, 1);
    assert.equal(existsSync(join(record.ownership.home, 'web-tools-ready.json')), false, 'readiness no longer travels through a file');
    assert.ok(!readdirSync(record.ownership.home).some(name => name.endsWith('ready.json')));
    record.ownershipObserverStop?.();
    await record.client.stop();
    first.close();
    record.client = undefined;
    record.channel = undefined;
    record.running = false;
    await sendToAgent(registry, context, record.agentId, 'resume without inference');
    const second = record.channel;
    assert.notEqual(second, first, 'a restarted launch binds its own channel');
    assert.equal(second.generation, 2);
    assert.notEqual(second.credential, first.credential, 'stale readiness cannot satisfy a restarted launch');
    assert.notDeepEqual(second.endpoint, first.endpoint);
    assert.deepEqual(await second.readiness('web'), { tools: ['web_search', 'ws_web_fetch'] });
    assert.deepEqual(prompts, ['not dispatched to a model', 'resume without inference']);
  } finally {
    for (const record of registry.values()) { record.ownershipObserverStop?.(); record.channel?.close(); await record.client?.stop(); }
  }
});

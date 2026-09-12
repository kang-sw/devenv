import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, realpathSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { RpcClient } from '@earendil-works/pi-coding-agent';
import { createAgentStorageContext } from '../src/agent-storage.ts';

// No LLM request is sent. RpcClient.start/stop/getState, extension loading,
// upstream capture, nonce publication and production spawn/resume remain real.
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
    extensionPath: join(packageRoot, 'src', 'index.ts'), toolGroup: 'read-only', spawnRole: 'explore', exploreMode: 'simple',
  };
  try {
    const result = await spawnAgent(registry, context, {
      systemPromptPath: join(packageRoot, 'explore-guide.md'), prompt: 'not dispatched to a model',
    });
    const record = registry.get(result.agent_id);
    const readyPath = join(record.ownership.home, 'web-tools-ready.json');
    const first = JSON.parse(readFileSync(readyPath, 'utf8'));
    assert.deepEqual(first.tools, ['web_search', 'ws_web_fetch']);
    assert.equal(first.nonce, record.subtreeChannel.nonce);
    record.ownershipObserverStop?.();
    await record.client.stop();
    record.client = undefined;
    record.running = false;
    await sendToAgent(registry, context, record.agentId, 'resume without inference');
    const second = JSON.parse(readFileSync(readyPath, 'utf8'));
    assert.notEqual(second.nonce, first.nonce, 'stale readiness cannot satisfy a restarted launch');
    assert.equal(second.nonce, record.subtreeChannel.nonce);
    assert.deepEqual(second.tools, first.tools);
    assert.deepEqual(prompts, ['not dispatched to a model', 'resume without inference']);
  } finally {
    for (const record of registry.values()) { record.ownershipObserverStop?.(); await record.client?.stop(); }
  }
});

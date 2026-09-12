import assert from 'node:assert/strict';
import { test } from 'node:test';
import { existsSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Readable } from 'node:stream';
import { allocateAgentHome, createAgentStorageContext, readOwnership, writeOwnership } from '../src/agent-storage.ts';
import { evictForCapacity, type RpcAgentRecord } from '../src/spawner.ts';
import { applySessionStartAgentRetention } from '../src/index.ts';
import { createBoundedWebFetcher } from '../src/web-fetch.ts';

const fetch = createBoundedWebFetcher({
  lookup: async () => [{ address: '8.8.8.8', family: 4 }],
  request: async () => Object.assign(Readable.from(['<article>' + 'x'.repeat(9000) + '</article>']), {
    statusCode: 200, headers: { 'content-type': 'text/html' },
  }),
});

test('real capacity eviction removes fetched spills from a confirmed-stopped Explore home', async t => {
  const root = mkdtempSync(join(tmpdir(), 'web-cap-retention-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const ownership = allocateAgentHome(createAgentStorageContext('lead', root), 'explore', 'explore', 'web-search');
  const spill = await fetch({ url: 'https://example.org/page', cacheHome: ownership.home });
  assert.ok(existsSync(spill.path!));
  const metadata = readOwnership(ownership.home)!;
  writeOwnership({ ...metadata, liveness: { lifecycle: 'stopped', running: false } });
  const record = { agentId: ownership.agentId, ownership, sessionPath: ownership.sessionPath,
    systemPromptPath: '/unused', wsToolNames: [], toolGroup: 'read-only', spawnRole: 'explore',
    streaming: false, running: false, reportLog: [],
  } as RpcAgentRecord;
  const registry = new Map([[record.agentId, record]]);
  assert.deepEqual(evictForCapacity(registry, 1), { ok: true, evictedLabel: 'explore' });
  assert.equal(registry.size, 0);
  assert.equal(existsSync(spill.path!), false, 'deletion precedes fixture teardown');
  assert.equal(existsSync(ownership.home), false);
});

test('controller stale pruning owns abandoned spills while retaining interrupted/uncertain sessions', async t => {
  const root = mkdtempSync(join(tmpdir(), 'web-stale-retention-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const config = join(root, 'goal-loop-config.json');
  writeFileSync(config, JSON.stringify({ child_retention_ttl_days: 1 }));
  const homes = ['abandoned', 'interrupted'].map(id => allocateAgentHome(createAgentStorageContext('previous-lead', root), id, 'explore', 'web-search'));
  const spills = [];
  for (const ownership of homes) {
    spills.push(await fetch({ url: 'https://example.org/page', cacheHome: ownership.home }));
    const metadata = readOwnership(ownership.home)!;
    writeOwnership({ ...metadata, lastActivityAt: Date.now() - 3 * 86_400_000,
      liveness: ownership.agentId === 'abandoned' ? { lifecycle: 'stopped', running: false } : { lifecycle: 'unknown' },
    });
  }
  applySessionStartAgentRetention(undefined, root, config, []);
  assert.equal(existsSync(spills[0].path!), false, 'stale abandoned content is removed by controller maintenance');
  assert.equal(existsSync(spills[1].path!), true, 'an interrupted child with uncertain liveness is not inferred dead');
  const interrupted = readOwnership(homes[1].home)!;
  writeOwnership({ ...interrupted, liveness: { lifecycle: 'stopped', running: false } });
  applySessionStartAgentRetention(undefined, root, config, []);
  assert.equal(existsSync(spills[1].path!), false, 'once termination is confirmed the shared stale lifecycle removes its spill');
});

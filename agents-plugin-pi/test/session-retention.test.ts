import assert from "node:assert/strict";
import { existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, test } from "node:test";
import { allocateAgentHome, createAgentStorageContext, readOwnership, writeOwnership } from "../src/agent-storage.ts";
import type { PersistedOrphan } from "../src/agent-sidecar.ts";
import { applySessionStartAgentRetention } from "../src/index.ts";
import { aggregateDescendantCosts } from "../src/agent-footer.ts";

function orphan(agentId: string, sessionPath: string, ownership?: PersistedOrphan["ownership"]): PersistedOrphan {
  return { agentId, sessionPath, systemPromptPath: "/tmp/prompt.md", wsToolNames: [], toolGroup: "full-worker", ...(ownership ? { ownership } : {}) };
}

describe("controller session-start child retention", () => {
  test("reads the adapter config, prunes another lead's stale home, and filters only that owned sidecar entry", (t) => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-session-retention-"));
    const config = join(root, "goal-loop-config.json");
    const now = Date.parse("2026-10-10T00:00:00.000Z");
    t.mock.method(Date, "now", () => now);
    try {
      writeFileSync(config, JSON.stringify({ child_retention_ttl_days: 0.5 }));
      const stale = allocateAgentHome(createAgentStorageContext("other-lead", root), "stale-child", "worker");
      const recent = allocateAgentHome(createAgentStorageContext("other-lead", root), "recent-child", "worker");
      for (const [owned, lastActivityAt, cost] of [[stale, now - 86_400_000, .4], [recent, now - 1_000, .2]] as const) {
        const metadata = readOwnership(owned.home)!;
        writeOwnership({ ...metadata, lastActivityAt, telemetry: { version: 1, origin: { sessionId: `${owned.agentId}-session`, sessionPath: owned.sessionPath!, emptyPrefix: true }, estimatedUsd: cost }, liveness: { ...metadata.liveness, lifecycle: "stopped", running: false } });
      }
      const legacy = orphan("legacy", join(root, "legacy-session.jsonl"));
      const recovered = [orphan(stale.agentId, stale.sessionPath!, stale), orphan(recent.agentId, recent.sessionPath!, recent), legacy];

      const retained = applySessionStartAgentRetention(undefined, root, config, recovered);
      assert.equal(existsSync(stale.home), false);
      assert.equal(existsSync(recent.home), true);
      assert.deepEqual(retained.map(entry => entry.agentId), [recent.agentId, "legacy"]);
      const aggregate = aggregateDescendantCosts(createAgentStorageContext("other-lead", root), new Map());
      assert.equal(aggregate.knownUsd.toFixed(2), "0.60");
      assert.deepEqual({ ...aggregate, knownUsd: 0 }, { knownUsd: 0, knownContributors: 2, unknownContributors: 0, descendants: 2 }, "retention roll-up preserves the deleted child beside the retained child");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("literal false disables pruning and worker/explore session starts never run global maintenance", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-session-retention-"));
    const config = join(root, "goal-loop-config.json");
    try {
      writeFileSync(config, JSON.stringify({ child_retention_ttl_days: false }));
      const recovered = [orphan("legacy", join(root, "legacy-session.jsonl"))];
      let calls = 0;
      let resolvedTtl: number | false | undefined;
      const prune = (_root: string, ttl: number | false) => { calls += 1; resolvedTtl = ttl; return { scanned: 0, retained: 0, failed: 0, deletedHomes: [] }; };
      assert.strictEqual(applySessionStartAgentRetention("worker", root, config, recovered, prune), recovered);
      assert.strictEqual(applySessionStartAgentRetention("explore", root, config, recovered, prune), recovered);
      assert.equal(calls, 0);
      applySessionStartAgentRetention(undefined, root, config, recovered, prune);
      assert.equal(calls, 1, "a controller session invokes the seam even when the resolved TTL disables its scanner");
      assert.equal(resolvedTtl, false);
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("a retention failure is diagnostic-only and preserves every recovered sidecar record", (t) => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-session-retention-"));
    const config = join(root, "missing-config.json");
    const diagnostics = t.mock.method(console, "error", () => {});
    try {
      const recovered = [orphan("legacy", join(root, "legacy-session.jsonl"))];
      const retained = applySessionStartAgentRetention("fork", root, config, recovered, () => { throw new Error("permission denied"); });
      assert.strictEqual(retained, recovered);
      assert.equal(diagnostics.mock.callCount(), 1);
      assert.match(String(diagnostics.mock.calls[0].arguments[0]), /permission denied/);
    } finally { rmSync(root, { recursive: true, force: true }); }
  });
});

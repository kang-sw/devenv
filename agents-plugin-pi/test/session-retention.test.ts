import assert from "node:assert/strict";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, test } from "node:test";
import { allocateAgentHome, createAgentStorageContext, readOwnership, writeOwnership } from "../src/agent-storage.ts";
import type { PersistedOrphan } from "../src/agent-sidecar.ts";
import { applySessionShutdownOwnershipDiagnostics, applySessionStartAgentRetention, applySessionStartOwnershipDiagnostics } from "../src/index.ts";
import { ownerNotifyRef } from "../src/spawner.ts";

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
      const saved = JSON.parse(readFileSync(join(root, "ws-agents", "other-lead", ".cost-estimate", "checkpoint.json"), "utf8"));
      assert.deepEqual(saved.evictedBaseline, { knownUsd: .4, knownContributors: 1, unknownContributors: 0, descendants: 1 }, "retention folds only the deleted direct record into the scalar baseline");
      assert.deepEqual(saved.agents, [], "retention does not discover or retain identities for sibling homes");
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

  test("the production ownership reporter is TUI-owner-only and resets at the session boundary", (t) => {
    const diagnostics = t.mock.method(console, "error", () => {});
    const notices: string[] = [];
    const ctx = { mode: "tui", ui: { notify: (message: string) => notices.push(message) } } as never;
    const fail = () => { throw new Error("raw /private/path"); };
    try {
      applySessionStartOwnershipDiagnostics(undefined, ctx);
      applySessionStartAgentRetention(undefined, "/unused", "/unused", [], fail);
      applySessionStartAgentRetention(undefined, "/unused", "/unused", [], fail);
      assert.equal(notices.length, 1, "one adapter session reports a stable authoritative fingerprint once");

      applySessionShutdownOwnershipDiagnostics();
      applySessionStartOwnershipDiagnostics(undefined, ctx);
      applySessionStartAgentRetention(undefined, "/unused", "/unused", [], fail);
      assert.equal(notices.length, 2, "a new adapter session gets a fresh bounded reporter");

      applySessionStartOwnershipDiagnostics("fork", ctx);
      applySessionStartAgentRetention("fork", "/unused", "/unused", [], fail);
      assert.equal(notices.length, 2, "a spawned role never owns the Pi-native owner notification surface");
      assert.equal(diagnostics.mock.callCount(), 0);
    } finally { applySessionShutdownOwnershipDiagnostics(); }
  });

  test("a retention failure reports once outside terminal streams and preserves every recovered sidecar record", (t) => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-session-retention-"));
    const config = join(root, "missing-config.json");
    const diagnostics = t.mock.method(console, "error", () => {});
    const notices: string[] = [];
    ownerNotifyRef.current = message => notices.push(message);
    try {
      const recovered = [orphan("legacy", join(root, "legacy-session.jsonl"))];
      const retained = applySessionStartAgentRetention("fork", root, config, recovered, () => { throw new Error("permission denied at /private/home"); });
      assert.strictEqual(retained, recovered);
      assert.equal(diagnostics.mock.callCount(), 0);
      assert.deepEqual(notices, ["ws: owned-agent retention could not start; no uncertain home was removed."]);
      assert.doesNotMatch(notices[0]!, /permission|private/);
      assert.doesNotMatch(String(applySessionStartAgentRetention), /console\.|process\.(?:stdout|stderr)/);
    } finally { ownerNotifyRef.current = undefined; rmSync(root, { recursive: true, force: true }); }
  });
});

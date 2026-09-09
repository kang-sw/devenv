import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, test } from "node:test";
import { allocateAgentHome, createAgentStorageContext, readOwnership, touchOwnership } from "../src/agent-storage.ts";

describe("agent storage", () => {
  test("allocates a persistent child beneath the configured Pi root and persists ownership", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const owned = allocateAgentHome(createAgentStorageContext("lead_1", root), "agent-1", "worker");
      assert.equal(owned.home, join(root, "ws-agents", "lead_1", "agent-1"));
      assert.equal(owned.sessionPath, join(owned.home, "session.jsonl"));
      const before = readOwnership(owned.home)!;
      touchOwnership(owned.home);
      assert.ok(readOwnership(owned.home)!.lastActivityAt >= before.lastActivityAt);
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("keeps terminal no-session leaves out of ordinary session files and rejects traversal", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
    try {
      const leaf = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-2", "explore", undefined, true);
      assert.equal(leaf.sessionPath, undefined);
      assert.throws(() => createAgentStorageContext("../escape", root), /unsafe Pi session id/);
    } finally { rmSync(root, { recursive: true, force: true }); }
  });
});

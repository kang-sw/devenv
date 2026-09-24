/**
 * Per-child eviction records (260924-bug-pi-retention-cross-owner-checkpoint-fold):
 * a removal writes `ws-agents/<owner>/.cost-estimate/evicted/<agentId>.json`
 * under the removal lock, readers sum the records at read time, and a record
 * supersedes every live count of its agentId. Cross-process cases spawn real
 * node processes synchronized by release files; fault seams go through
 * `removeOwnedAgentHome`'s `remove`/`beforeDetach`, never timing.
 */
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import fs from "node:fs";
import { existsSync, lstatSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, renameSync, rmSync, symlinkSync, utimesSync, writeFileSync } from "node:fs";
import { syncBuiltinESMExports } from "node:module";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { afterEach, describe, test } from "node:test";
import {
  allocateAgentHome,
  createAgentStorageContext,
  hasEvictionRecord,
  ownedHomeRemovalState,
  persistOwnershipTelemetry,
  pruneStaleAgentHomes,
  readEvictionRecord,
  readEvictionRecords,
  readOwnership,
  removedAgentMessage,
  removeOwnedAgentHome,
  updateOwnership,
  writeEvictionRecord,
  writeOwnership,
} from "../src/agent-storage.ts";
import type { CumulativeCost } from "../src/agent-telemetry.ts";
import { createAgentFooterController, descendantUsageValue, persistAgentCostCheckpoint, registerAgentCostOwner, retentionEvictionCost } from "../src/agent-footer.ts";
import { evictForCapacity, sendToAgent, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";
import { captureOrphans, parseOrphans, readAndClearSidecarAt, reviveOrphans, serializeOrphans, writeSidecarAt } from "../src/agent-sidecar.ts";
import { createThreadRegistryHandle, ensureRespondent, type ThreadRecord } from "../src/ask.ts";
import { truncateToWidth, visibleWidth } from "../src/pi-tui.ts";
import { symlinkSkip, symlinksAvailable } from "./fixtures/symlink-probe.ts";

const roots = new Set<string>();
afterEach(() => { for (const root of roots) rmSync(root, { recursive: true, force: true }); roots.clear(); });
function root(): string { const value = mkdtempSync(join(tmpdir(), "ws-pi-eviction-")); roots.add(value); return value; }
type Storage = ReturnType<typeof createAgentStorageContext>;

// Dyadic costs keep every sum exact regardless of summation order.
const usd = (knownUsd: number, contributors = 1): CumulativeCost => ({ knownUsd, knownContributors: contributors, unknownContributors: 0, descendants: contributors });
function record(agentId: string, storage: Storage, ownUsd: number): RpcAgentRecord {
  const ownership = allocateAgentHome(storage, agentId, "worker");
  const value = {
    agentId, sessionPath: ownership.sessionPath!, ownership, systemPromptPath: join(storage.root, "prompt.md"),
    telemetry: { version: 1 as const, origin: { sessionId: `${agentId}-session`, sessionPath: ownership.sessionPath!, emptyPrefix: true as const }, estimatedUsd: ownUsd },
    wsToolNames: [], toolGroup: "full-worker", streaming: false, running: false, reportLog: [],
  } as RpcAgentRecord;
  persistOwnershipTelemetry(ownership.home, value.telemetry);
  return value;
}
/** Stopped and older than any retention TTL. */
function stale(child: RpcAgentRecord): void {
  const metadata = readOwnership(child.ownership!.home)!;
  writeOwnership({ ...metadata, lastActivityAt: 1, liveness: { ...metadata.liveness, lifecycle: "stopped", running: false } });
}
function owner(storage: Storage, ...children: RpcAgentRecord[]): RpcAgentRegistry {
  const registry: RpcAgentRegistry = new Map(children.map(child => [child.agentId, child]));
  registerAgentCostOwner(registry, storage);
  return registry;
}
const retentionOptions = { evictionCost: retentionEvictionCost };
const evictedDir = (storage: Storage) => join(storage.root, "ws-agents", storage.ownerSessionId, ".cost-estimate", "evicted");
const recordFiles = (storage: Storage) => existsSync(evictedDir(storage)) ? readdirSync(evictedDir(storage)).sort() : [];
function checkpoint(storage: Storage): any {
  return JSON.parse(readFileSync(join(storage.root, "ws-agents", storage.ownerSessionId, ".cost-estimate", "checkpoint.json"), "utf8"));
}
function silenceDiagnostics(t: { mock: { method: (...args: any[]) => unknown } }): void { t.mock.method(console, "error", () => {}); }

const STORAGE_URL = JSON.stringify(new URL("../src/agent-storage.ts", import.meta.url).href);
const FOOTER_URL = JSON.stringify(new URL("../src/agent-footer.ts", import.meta.url).href);
// A fixture process announces `label` and then blocks until the release file
// exists: a barrier, not a timing guess.
const BARRIER = `
  import { existsSync, writeSync } from "node:fs";
  function barrier(release, label) {
    writeSync(1, label + "\\n");
    const deadline = Date.now() + 10_000;
    const sleeper = new Int32Array(new SharedArrayBuffer(4));
    while (!existsSync(release)) {
      if (Date.now() > deadline) throw new Error("fixture release timed out");
      Atomics.wait(sleeper, 0, 0, 1);
    }
  }
`;
interface NodeFixture { stdout(): string; closed: Promise<number | null>; waitFor(line: string): Promise<void>; kill(): void }
function nodeProcess(script: string): NodeFixture {
  const child = spawn(process.execPath, ["--input-type=module", "--eval", BARRIER + script], { stdio: ["ignore", "pipe", "pipe"] });
  let stdout = "", stderr = "", code: number | null | undefined;
  child.stdout.setEncoding("utf8").on("data", chunk => { stdout += chunk; });
  child.stderr.setEncoding("utf8").on("data", chunk => { stderr += chunk; });
  const closed = new Promise<number | null>(resolve => child.on("close", exit => { code = exit; resolve(exit); }));
  return {
    stdout: () => stdout,
    closed: closed.then(exit => { if (exit !== 0 && exit !== 7) throw new Error(`fixture exited ${exit}: ${stderr}`); return exit; }),
    async waitFor(line) {
      const deadline = Date.now() + 10_000;
      while (!stdout.split("\n").includes(line)) {
        if (code !== undefined) throw new Error(`fixture exited ${code} before "${line}": ${stderr}`);
        if (Date.now() > deadline) throw new Error(`fixture never printed "${line}": ${stderr}`);
        await new Promise(resolve => setTimeout(resolve, 5));
      }
    },
    kill() { if (code === undefined) child.kill(); },
  };
}
function resultOf(fixture: NodeFixture): any {
  const line = fixture.stdout().split("\n").find(entry => entry.startsWith("result "));
  assert.ok(line, `fixture printed no result: ${fixture.stdout()}`);
  return JSON.parse(line.slice("result ".length));
}
/** Another process holds `child`'s ownership claim (inside its final eligibility check) until released; its removal then retains the home. */
async function holdRemovalClaim(home: string, release: string): Promise<NodeFixture> {
  const fixture = nodeProcess(`
    import { readOwnership, removeOwnedAgentHome } from ${STORAGE_URL};
    const result = removeOwnedAgentHome(readOwnership(${JSON.stringify(home)}), undefined, () => { barrier(${JSON.stringify(release)}, "held"); return false; });
    writeSync(1, "result " + JSON.stringify(result) + "\\n");
  `);
  try { await fixture.waitFor("held"); } catch (error) { fixture.kill(); throw error; }
  return fixture;
}

describe("a removal that ends with the home in place leaves no record", () => {
  test("retained at the final eligibility check: no record, cost unchanged, home in place", () => {
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5), kept = record("kept", storage, .25);
    const registry = owner(storage, child, kept);
    stale(child);
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2));
    let costed = 0;
    const result = removeOwnedAgentHome(readOwnership(child.ownership!.home)!, undefined, () => false, { evictionCost: metadata => { costed += 1; return retentionEvictionCost(metadata); } });
    assert.equal(result.status, "retained");
    assert.equal(costed, 0, "no record value is computed for a home that failed the final check");
    assert.deepEqual(recordFiles(storage), []);
    assert.equal(existsSync(child.ownership!.home), true);
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2));
  });

  test("a busy ownership lock held by another live process writes no record and repairs none", { timeout: 20_000 }, async () => {
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5);
    const registry = owner(storage, child);
    stale(child);
    for (const priorRecord of [false, true]) {
      if (priorRecord) assert.equal(writeEvictionRecord(storage, "child", usd(.5)), true, "precondition: a removal-pending record");
      const release = join(storage.root, `release-${priorRecord}`);
      const holder = await holdRemovalClaim(child.ownership!.home, release);
      try {
        const result = removeOwnedAgentHome(readOwnership(child.ownership!.home)!, undefined, undefined, retentionOptions);
        assert.deepEqual(result, { status: "retained", reason: "owned home is unavailable or busy" });
        assert.equal(existsSync(child.ownership!.home), true);
        if (priorRecord) {
          assert.deepEqual(readEvictionRecord(storage, "child"), usd(.5), "a busy lock attempts no repair");
          assert.deepEqual(descendantUsageValue(registry), usd(.5), "the removal-pending child counts once, through its record");
          assert.equal(registry.has("child"), true, "a home-present recorded child stays registered");
        } else {
          assert.deepEqual(recordFiles(storage), [], "a busy lock writes no record");
          assert.deepEqual(descendantUsageValue(registry), usd(.5));
        }
      } finally {
        writeFileSync(release, "release");
        await holder.closed.finally(() => holder.kill());
      }
      assert.deepEqual(resultOf(holder), { status: "retained", reason: "owned-home eligibility changed before deletion" });
      // The holder's own call held the lock and ended with the home in place:
      // that is the one repair path, so a prior record is gone afterwards.
      assert.deepEqual(recordFiles(storage), []);
      assert.deepEqual(descendantUsageValue(registry), usd(.5), "counted once again, live");
    }
  });

  test("a detach failure rolls back the record it wrote", t => {
    silenceDiagnostics(t);
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5), kept = record("kept", storage, .25);
    const registry = owner(storage, child, kept);
    stale(child);
    let during: CumulativeCost | undefined, recordedDuring: CumulativeCost | undefined;
    const result = removeOwnedAgentHome(readOwnership(child.ownership!.home)!, undefined, undefined, {
      ...retentionOptions,
      beforeDetach: () => {
        recordedDuring = readEvictionRecord(storage, "child");
        during = descendantUsageValue(registry);
        throw new Error("injected detach failure");
      },
    });
    assert.equal(result.status, "failed");
    assert.deepEqual(recordedDuring, usd(.5), "precondition: the record was written before the detach");
    assert.deepEqual(during, usd(.75, 2), "while the record exists the child counts once, through it");
    assert.deepEqual(recordFiles(storage), [], "the record was rolled back");
    assert.equal(existsSync(child.ownership!.home), true);
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2));
    assert.equal(registry.has("child"), true);
  });
});

describe("cross-process retention and the live owner", () => {
  test("two root-lead prunes racing on one owner's stale children record each child exactly once", { timeout: 30_000 }, async () => {
    const storage = createAgentStorageContext("lead", root());
    const children = Array.from({ length: 12 }, (_, i) => record(`child-${i}`, storage, (i + 1) / 64));
    for (const child of children) stale(child);
    const expected = children.reduce((sum, _, i) => sum + (i + 1) / 64, 0);
    const release = join(storage.root, "release-prunes");
    const prunes = [0, 1].map(i => nodeProcess(`
      import { pruneStaleAgentHomes } from ${STORAGE_URL};
      import { retentionEvictionCost } from ${FOOTER_URL};
      barrier(${JSON.stringify(release)}, "ready");
      const result = pruneStaleAgentHomes(${JSON.stringify(storage.root)}, 1, { evictionCost: retentionEvictionCost });
      writeSync(1, "result " + JSON.stringify(result) + "\\n");
    `));
    try {
      await Promise.all(prunes.map(prune => prune.waitFor("ready")));
      writeFileSync(release, "release");
      await Promise.all(prunes.map(prune => prune.closed));
    } finally { for (const prune of prunes) prune.kill(); }
    const deleted = prunes.flatMap(prune => resultOf(prune).deletedHomes as string[]);
    assert.deepEqual([...deleted].sort(), children.map(child => child.ownership!.home).sort(), "every stale home is removed by exactly one prune");
    assert.deepEqual(recordFiles(storage), children.map(child => `${child.agentId}.json`).sort(), "one record file per child");
    for (const [i, child] of children.entries()) assert.deepEqual(readEvictionRecord(storage, child.agentId), usd((i + 1) / 64), child.agentId);
    const restarted = owner(storage);
    assert.deepEqual(descendantUsageValue(restarted), usd(expected, 12), "the owner counts each child exactly once");
    assert.equal(existsSync(join(storage.root, "ws-agents", "lead", ".cost-estimate", "checkpoint.json")), false, "retention never writes the checkpoint");
  });

  test("a prune that meets another prune's held removal claim retains; the child is recorded once", { timeout: 20_000 }, async () => {
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5);
    stale(child);
    const release = join(storage.root, "release-held");
    const holder = nodeProcess(`
      import { pruneStaleAgentHomes } from ${STORAGE_URL};
      import { retentionEvictionCost } from ${FOOTER_URL};
      const result = pruneStaleAgentHomes(${JSON.stringify(storage.root)}, 1, { evictionCost: metadata => {
        const cost = retentionEvictionCost(metadata);
        barrier(${JSON.stringify(release)}, "held");
        return cost;
      } });
      writeSync(1, "result " + JSON.stringify(result) + "\\n");
    `);
    try {
      await holder.waitFor("held");
      const second = pruneStaleAgentHomes(storage.root, 1, retentionOptions);
      assert.deepEqual(second.deletedHomes, [], "the second root lead meets the held claim");
      assert.deepEqual(recordFiles(storage), []);
      writeFileSync(release, "release");
      await holder.closed;
    } finally { holder.kill(); }
    assert.deepEqual(resultOf(holder).deletedHomes, [child.ownership!.home]);
    assert.deepEqual(pruneStaleAgentHomes(storage.root, 1, retentionOptions).deletedHomes, [], "a later prune finds nothing left");
    assert.deepEqual(recordFiles(storage), ["child.json"]);
    assert.deepEqual(descendantUsageValue(owner(storage)), usd(.5));
  });

  test("a live owner that holds the removed child counts it once through the record, drops it, and persists a checkpoint without it", { timeout: 20_000 }, async () => {
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5), kept = record("kept", storage, .25);
    const registry = owner(storage, child, kept);
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2));
    assert.equal(persistAgentCostCheckpoint(registry), true);
    assert.deepEqual(checkpoint(storage).agents.map((entry: any) => entry.agentId), ["child", "kept"], "precondition: the owner's checkpoint tracks the child");
    const before = readFileSync(join(storage.root, "ws-agents", "lead", ".cost-estimate", "checkpoint.json"), "utf8");
    stale(child);

    const retention = nodeProcess(`
      import { pruneStaleAgentHomes } from ${STORAGE_URL};
      import { retentionEvictionCost } from ${FOOTER_URL};
      writeSync(1, "result " + JSON.stringify(pruneStaleAgentHomes(${JSON.stringify(storage.root)}, 1, { evictionCost: retentionEvictionCost })) + "\\n");
    `);
    await retention.closed.finally(() => retention.kill());
    assert.deepEqual(resultOf(retention).deletedHomes, [child.ownership!.home]);
    assert.equal(readFileSync(join(storage.root, "ws-agents", "lead", ".cost-estimate", "checkpoint.json"), "utf8"), before, "retention never writes the owner's checkpoint");
    assert.deepEqual(readEvictionRecord(storage, "child"), usd(.5));
    assert.equal(registry.has("child"), true, "precondition: the owner's registry still holds the removed child");

    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "counted once, through the record");
    assert.equal(registry.has("child"), false, "reconcile drops a recorded entry whose home is gone");
    assert.equal(persistAgentCostCheckpoint(registry), true);
    assert.deepEqual(checkpoint(storage).agents.map((entry: any) => entry.agentId), ["kept"], "the persisted agents[] excludes the removed child");
    assert.deepEqual(checkpoint(storage).evictedBaseline, usd(0, 0));
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "still once after the child left the registry");
    assert.deepEqual(descendantUsageValue(owner(storage, kept)), usd(.75, 2), "and after a restart from the persisted checkpoint");
  });

  test("the owner's persist loop racing another process's retention loses no cost", { timeout: 30_000 }, async () => {
    const storage = createAgentStorageContext("lead", root());
    const removed = Array.from({ length: 8 }, (_, i) => record(`gone-${i}`, storage, (i + 1) / 32));
    const kept = record("kept", storage, .25);
    kept.client = {} as never;
    const registry = owner(storage, ...removed, kept);
    for (const child of removed) stale(child);
    const total = usd(removed.reduce((sum, _, i) => sum + (i + 1) / 32, .25), 9);
    assert.deepEqual(descendantUsageValue(registry), total);
    const release = join(storage.root, "release-retention");
    const retention = nodeProcess(`
      import { pruneStaleAgentHomes } from ${STORAGE_URL};
      import { retentionEvictionCost } from ${FOOTER_URL};
      barrier(${JSON.stringify(release)}, "ready");
      writeSync(1, "result " + JSON.stringify(pruneStaleAgentHomes(${JSON.stringify(storage.root)}, 1, { evictionCost: retentionEvictionCost })) + "\\n");
    `);
    let done = false, iterations = 0;
    try {
      await retention.waitFor("ready");
      void retention.closed.finally(() => { done = true; }).catch(() => {});
      writeFileSync(release, "release");
      while (!done) {
        assert.equal(persistAgentCostCheckpoint(registry), true);
        assert.deepEqual(descendantUsageValue(registry), total, `every child counts exactly once at iteration ${iterations}`);
        iterations += 1;
        await new Promise(resolve => setImmediate(resolve));
      }
      await retention.closed;
    } finally { retention.kill(); }
    assert.equal(resultOf(retention).deletedHomes.length, 8);
    assert.equal(persistAgentCostCheckpoint(registry), true);
    assert.deepEqual(descendantUsageValue(registry), total, "final total: live plus recorded");
    assert.deepEqual([...readEvictionRecords(storage)].sort(([a], [b]) => a.localeCompare(b)), removed.map((child, i) => [child.agentId, usd((i + 1) / 32)]), "every record intact");
    assert.deepEqual(checkpoint(storage).agents.map((entry: any) => entry.agentId), ["kept"]);
    assert.deepEqual(descendantUsageValue(owner(storage, kept)), total, "a restart from the checkpoint and the records agrees");
  });
});

describe("crash window", () => {
  test("a crash between the record write and the detach counts the child once; a later prune removes it without a second count", { timeout: 20_000 }, async () => {
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5), kept = record("kept", storage, .25);
    const registry = owner(storage, child, kept);
    stale(child);
    const crash = nodeProcess(`
      import { readOwnership, removeOwnedAgentHome } from ${STORAGE_URL};
      import { retentionEvictionCost } from ${FOOTER_URL};
      removeOwnedAgentHome(readOwnership(${JSON.stringify(child.ownership!.home)}), undefined, undefined, { evictionCost: retentionEvictionCost, beforeDetach: () => process.exit(7) });
      writeSync(1, "not reached\\n");
    `);
    assert.equal(await crash.closed.finally(() => crash.kill()), 7);
    assert.deepEqual(readEvictionRecord(storage, "child"), usd(.5), "the record survived the crash");
    assert.equal(existsSync(child.ownership!.home), true, "the home was never detached");
    const lock = join(storage.root, "ws-agents", "lead", ".child.ownership-lock");
    assert.equal(existsSync(lock), true, "the dead process left its claim behind");

    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "counted once, through the record");
    assert.equal(registry.has("child"), true, "a home-present recorded child stays registered (removal-pending)");
    assert.equal(persistAgentCostCheckpoint(registry), true);
    assert.deepEqual(checkpoint(storage).agents.map((entry: any) => entry.agentId), ["kept"], "the recorded child is excluded from the persisted agents[]");

    const pruned = pruneStaleAgentHomes(storage.root, 1, retentionOptions);
    assert.deepEqual(pruned.deletedHomes, [child.ownership!.home], "the stale claim of a dead process does not strand the home");
    assert.deepEqual(readEvictionRecord(storage, "child"), usd(.5), "the second removal leaves the record value unchanged");
    assert.deepEqual(recordFiles(storage), ["child.json"]);
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "no second count");
    assert.equal(registry.has("child"), false);
  });
});

describe("record value", () => {
  test("a rewrite merges with the existing record: a lower observation keeps the floor, a higher one replaces it", () => {
    const storage = createAgentStorageContext("lead", root());
    assert.equal(writeEvictionRecord(storage, "child", usd(.5)), true);
    assert.equal(writeEvictionRecord(storage, "child", usd(.25)), true);
    assert.deepEqual(readEvictionRecord(storage, "child"), { knownUsd: .5, knownContributors: 1, unknownContributors: 1, descendants: 1 }, "the floor survives a regressed observation");
    assert.equal(writeEvictionRecord(storage, "child", usd(.75)), true);
    assert.deepEqual(readEvictionRecord(storage, "child"), usd(.75));
  });

  test("a second removal whose telemetry regressed keeps the first record's floor", () => {
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5);
    owner(storage, child);
    stale(child);
    assert.equal(writeEvictionRecord(storage, "child", usd(.5)), true, "a first removal recorded .5 and crashed before the detach");
    const home = child.ownership!.home;
    const metadata = readOwnership(home)!;
    writeOwnership({ ...metadata, telemetry: { ...metadata.telemetry!, estimatedUsd: .25 } });
    assert.deepEqual(retentionEvictionCost(readOwnership(home)!), usd(.25), "precondition: the second removal observes less");
    assert.deepEqual(pruneStaleAgentHomes(storage.root, 1, retentionOptions).deletedHomes, [home]);
    assert.equal(readEvictionRecord(storage, "child")!.knownUsd, .5, "the rewrite did not overwrite the floor");
  });

  test("retention's cost merges the owner checkpoint's tracked value with the ownership telemetry", () => {
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5);
    const registry = owner(storage, child);
    // The owner tracked more than the ownership telemetry on disk shows.
    child.telemetry = { ...child.telemetry!, estimatedUsd: .75 };
    assert.deepEqual(descendantUsageValue(registry), usd(.75), "the owner's cost state is live");
    assert.equal(persistAgentCostCheckpoint(registry), true);
    assert.deepEqual(checkpoint(storage).agents, [{ agentId: "child", cost: usd(.75) }]);
    assert.equal(readOwnership(child.ownership!.home)!.telemetry!.estimatedUsd, .5, "precondition: disk telemetry is lower");
    assert.deepEqual(retentionEvictionCost(readOwnership(child.ownership!.home)!), { knownUsd: .75, knownContributors: 1, unknownContributors: 1, descendants: 1 }, "the tracked floor wins over the regressed telemetry");
  });
});

describe("capacity eviction and the legacy baseline", () => {
  test("a legacy nonzero evictedBaseline still counts beside retention and capacity records, and is never changed", () => {
    const storage = createAgentStorageContext("lead", root());
    const bucket = join(storage.root, "ws-agents", "lead", ".cost-estimate");
    mkdirSync(bucket, { recursive: true });
    const legacy = usd(.5, 3);
    writeFileSync(join(bucket, "checkpoint.json"), JSON.stringify({
      version: 1, lead: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cost: usd(0, 0) }, evictedBaseline: legacy, agents: [],
    }));
    const retained = record("retained", storage, .125), evicted = record("evicted", storage, .25), kept = record("kept", storage, .0625);
    const registry = owner(storage, retained, evicted, kept);
    const total = usd(.5 + .125 + .25 + .0625, 6);
    assert.deepEqual(descendantUsageValue(registry), total, "the legacy baseline plus the live children");

    stale(retained);
    assert.equal(removeOwnedAgentHome(readOwnership(retained.ownership!.home)!, undefined, undefined, retentionOptions).status, "deleted");
    assert.deepEqual(descendantUsageValue(registry), total, "retention moves a child into its record");
    updateOwnership(evicted.ownership!.home, { liveness: { lifecycle: "stopped", running: false } });
    assert.deepEqual(evictForCapacity(registry, registry.size), { ok: true, evictedLabel: "evicted" });
    assert.deepEqual(readEvictionRecord(storage, "evicted"), usd(.25));
    assert.deepEqual(descendantUsageValue(registry), total, "capacity eviction moves a child into its record");
    assert.equal(persistAgentCostCheckpoint(registry), true);
    assert.deepEqual(checkpoint(storage).evictedBaseline, legacy, "the legacy baseline is read-only");
    assert.deepEqual(checkpoint(storage).agents.map((entry: any) => entry.agentId), ["kept"]);

    const restarted = owner(storage, { ...kept });
    assert.deepEqual(descendantUsageValue(restarted), total, "a restart sums the legacy baseline, both records, and the live child");
  });
});

describe("a recorded agentId never comes back", () => {
  test("sidecar revival skips a recorded agentId even while its home is present; an unrecorded one revives", () => {
    const storage = createAgentStorageContext("lead", root());
    const pending = record("pending", storage, .5), kept = record("kept", storage, .25);
    const registry = owner(storage, pending, kept);
    const orphans = captureOrphans(registry);
    const sidecar = join(storage.root, "lead.sidecar.json");
    writeSidecarAt(sidecar, orphans);
    assert.equal(writeEvictionRecord(storage, "pending", usd(.5)), true);
    assert.equal(existsSync(pending.ownership!.home), true, "precondition: removal-pending, home present");

    const parsed = readAndClearSidecarAt(sidecar);
    assert.deepEqual(parsed.map(entry => entry.agentId), ["kept"], "parseOrphans skips the recorded agentId");
    const restarted = owner(storage);
    const revived = reviveOrphans(restarted, [...orphans]);
    try {
      assert.deepEqual(revived.map(entry => entry.agentId), ["kept"], "reviveOrphans skips it too; positive control: the unrecorded one revives");
      assert.equal(restarted.has("pending"), false);
      assert.deepEqual(descendantUsageValue(restarted), usd(.75, 2), "the pending child counts once, through its record");
    } finally { for (const entry of revived) entry.ownershipObserverStop?.(); }
  });

  test("a dormant send or relaunch of a recorded agentId is refused before any launch, home present or gone", async t => {
    silenceDiagnostics(t);
    for (const variant of ["home present", "home gone"] as const) {
      const storage = createAgentStorageContext("lead", root());
      const child = record("child", storage, .5), kept = record("kept", storage, .25);
      const registry = owner(storage, child, kept);
      assert.deepEqual(descendantUsageValue(registry), usd(.75, 2));
      stale(child);
      if (variant === "home present") assert.equal(writeEvictionRecord(storage, "child", usd(.5)), true);
      else assert.equal(removeOwnedAgentHome(readOwnership(child.ownership!.home)!, undefined, undefined, retentionOptions).status, "deleted");
      const activity = readOwnership(child.ownership!.home)?.lastActivityAt;
      await assert.rejects(sendToAgent(registry, { cwd: storage.root }, "child", "resume"), { message: removedAgentMessage("child") }, variant);
      assert.equal(child.client, undefined, `${variant}: no launch`);
      assert.equal(child.launching, undefined, `${variant}: no launch`);
      assert.equal(readOwnership(child.ownership!.home)?.lastActivityAt, activity, `${variant}: no activity claim was taken`);
      assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), `${variant}: cost neither lost nor double counted`);
      assert.equal(registry.has("child"), variant === "home present", `${variant}: only a gone home is dropped`);
    }
  });

  test("a send to an agentId that reconcile already dropped names the removal, not an unknown id", async () => {
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5);
    const registry = owner(storage, child);
    stale(child);
    assert.equal(removeOwnedAgentHome(readOwnership(child.ownership!.home)!, undefined, undefined, retentionOptions).status, "deleted");
    assert.deepEqual(descendantUsageValue(registry), usd(.5));
    assert.equal(registry.has("child"), false, "precondition: reconcile dropped the entry");
    await assert.rejects(sendToAgent(registry, { cwd: storage.root }, "child", "resume"), { message: removedAgentMessage("child") });
    await assert.rejects(sendToAgent(registry, { cwd: storage.root }, "never-existed", "hi"), /unknown agentId "never-existed"/, "positive control");
    assert.deepEqual(descendantUsageValue(registry), usd(.5));
  });

  test("a fork rehydrate of a recorded respondent notifies the removal and does not repopulate the registry", async () => {
    const bridge = { wsToolNames: [], defaultSessionKeyRef: { current: undefined } } as never;
    for (const variant of ["home present", "home gone", "unrecorded"] as const) {
      const storage = createAgentStorageContext("lead", root());
      const fork = record("fork", storage, .5), kept = record("kept", storage, .25);
      const lead = owner(storage, fork, kept);
      assert.deepEqual(descendantUsageValue(lead), usd(.75, 2));
      stale(fork);
      if (variant === "home present") assert.equal(writeEvictionRecord(storage, "fork", usd(.5)), true);
      if (variant === "home gone") assert.equal(removeOwnedAgentHome(readOwnership(fork.ownership!.home)!, undefined, undefined, retentionOptions).status, "deleted");
      // Lead restart: the respondent is not in the registry, only in the thread.
      const restarted = owner(storage, { ...kept });
      const handle = createThreadRegistryHandle();
      const thread: ThreadRecord = {
        threadId: "q1", title: "reopen", question: "reopen?", status: "open", origin: "fork-raised", respondentAgentId: "fork", createdAt: "now", touchedAt: "now",
        forkResume: { sessionPath: fork.sessionPath, systemPromptPath: fork.systemPromptPath, wsToolNames: [], toolGroup: "full-worker", ownership: fork.ownership },
      } as ThreadRecord;
      handle.threads.set(thread.threadId, thread);
      const notices: Array<{ message: string; type?: string }> = [];
      const ctx = { mode: "tui", ui: { notify: (message: string, type?: string) => notices.push({ message, type }) } } as never;
      const agentId = await ensureRespondent({} as never, ctx, bridge, restarted, handle, thread, { cwd: storage.root });
      if (variant === "unrecorded") {
        assert.equal(agentId, "fork", "positive control: an unrecorded fork rehydrates");
        assert.equal(restarted.has("fork"), true);
        assert.deepEqual(notices, []);
        continue;
      }
      assert.equal(agentId, undefined, variant);
      assert.equal(restarted.has("fork"), false, `${variant}: the registry is not repopulated`);
      assert.equal(notices.length, 1, variant);
      assert.equal(notices[0]!.type, "error");
      assert.ok(notices[0]!.message.includes(removedAgentMessage("fork")), notices[0]!.message);
      assert.deepEqual(descendantUsageValue(restarted), usd(.75, 2), `${variant}: the fork counts once, through its record`);
    }
  });
});

describe("stale-record repair", () => {
  function pendingFixture() {
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5), kept = record("kept", storage, .25);
    const registry = owner(storage, child, kept);
    stale(child);
    return { storage, child, registry, home: child.ownership!.home };
  }

  test("a record left while the home is present is deleted by the next removal that ends with the home in place", () => {
    const { storage, registry, home } = pendingFixture();
    assert.equal(writeEvictionRecord(storage, "child", retentionEvictionCost(readOwnership(home)!)!), true);
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "before: once, through the record");
    // Not opted in, like the stale sidecar-duplicate discard: repair still runs.
    assert.equal(removeOwnedAgentHome(readOwnership(home)!, undefined, () => false).status, "retained");
    assert.deepEqual(recordFiles(storage), [], "the record was repaired");
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "after: once, live");
    assert.equal(registry.has("child"), true);
  });

  test("a failed remove with a successful rename-back deletes the record; once before, during and after", t => {
    silenceDiagnostics(t);
    const { storage, registry, home } = pendingFixture();
    assert.equal(writeEvictionRecord(storage, "child", usd(.5)), true);
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "before");
    let during: CumulativeCost | undefined;
    const result = removeOwnedAgentHome(readOwnership(home)!, () => { throw new Error("injected remove failure"); }, undefined, {
      ...retentionOptions,
      beforeDetach: () => { during = descendantUsageValue(registry); },
    });
    assert.equal(result.status, "failed");
    assert.deepEqual(during, usd(.75, 2), "during: once, through the record");
    assert.equal(existsSync(home), true, "the rename-back restored the home");
    assert.deepEqual(recordFiles(storage), []);
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "after: once, live");
  });

  test("a reconcile while the home is detached under a held claim keeps the entry; the rename-back then counts it live", t => {
    silenceDiagnostics(t);
    const { storage, registry, home } = pendingFixture();
    let during: CumulativeCost | undefined, registeredDuring: boolean | undefined;
    const result = removeOwnedAgentHome(readOwnership(home)!, () => {
      during = descendantUsageValue(registry);
      registeredDuring = registry.has("child");
      throw new Error("injected remove failure after the detach");
    }, undefined, retentionOptions);
    assert.equal(result.status, "failed");
    assert.deepEqual(during, usd(.75, 2), "during: once, through the record");
    assert.equal(registeredDuring, true, "a detached home under a held claim is not final: the entry stays registered");
    assert.equal(existsSync(home), true, "the rename-back restored the home");
    assert.deepEqual(recordFiles(storage), [], "the repair deleted the record");
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "after: once, live");
    assert.equal(registry.has("child"), true);
  });

  test("a failed rename-back that leaves the home off its path keeps the record", t => {
    silenceDiagnostics(t);
    const { storage, registry, home } = pendingFixture();
    const elsewhere = join(storage.root, "moved-away");
    const result = removeOwnedAgentHome(readOwnership(home)!, staged => {
      renameSync(staged, elsewhere);
      throw new Error("injected remove failure after the staged directory moved");
    }, undefined, retentionOptions);
    assert.equal(result.status, "failed");
    assert.equal(existsSync(home), false, "the home is off its path");
    assert.equal(existsSync(elsewhere), true);
    assert.deepEqual(readEvictionRecord(storage, "child"), usd(.5), "the record is kept");
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "once, through the record");
    assert.equal(registry.has("child"), false, "a recorded entry whose home is gone is dropped");
  });

  test("a recorded entry with a live client or a launch in flight is excluded but never dropped", () => {
    for (const variant of ["client", "launching"] as const) {
      const { storage, child, registry, home } = pendingFixture();
      if (variant === "client") child.client = {} as never;
      else child.launching = Promise.resolve();
      assert.equal(writeEvictionRecord(storage, "child", usd(.5)), true);
      rmSync(home, { recursive: true });
      assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), `${variant}: once, through the record`);
      assert.equal(registry.has("child"), true, `${variant}: the entry stays registered`);
    }
  });

  test("a failed record write removes no home", t => {
    silenceDiagnostics(t);
    for (const variant of ["no cost", "symlinked evicted directory"] as const) {
      if (variant === "symlinked evicted directory" && !symlinksAvailable()) {
        t.diagnostic(`${variant}: skipped; ${symlinkSkip()}`);
        continue;
      }
      const { storage, registry, home } = pendingFixture();
      const outside = join(root(), "outside");
      mkdirSync(outside);
      if (variant === "symlinked evicted directory") {
        mkdirSync(join(storage.root, "ws-agents", "lead", ".cost-estimate"));
        symlinkSync(outside, evictedDir(storage));
      }
      const result = removeOwnedAgentHome(readOwnership(home)!, undefined, undefined, { evictionCost: variant === "no cost" ? () => undefined : retentionEvictionCost });
      assert.equal(result.status, "failed", variant);
      assert.equal(existsSync(home), true, `${variant}: no home is removed without a record`);
      assert.equal(readEvictionRecord(storage, "child"), undefined, variant);
      assert.deepEqual(readdirSync(outside), [], `${variant}: nothing was written through a symlink`);
      assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), variant);
    }
  });
});

describe("the sidecar's removal gate under a held claim", () => {
  function sidecarFixture() {
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5), kept = record("kept", storage, .25);
    stale(child);
    const serialized = serializeOrphans(captureOrphans(owner(storage, child, kept)));
    return { storage, home: child.ownership!.home, serialized };
  }
  const claimOf = (home: string) => join(dirname(home), `.${basename(home)}.ownership-lock`);
  function revive(storage: Storage, serialized: string): { ids: string[]; owned: boolean; registry: RpcAgentRegistry } {
    const registry = owner(storage);
    const revived = reviveOrphans(registry, parseOrphans(serialized));
    for (const entry of revived) entry.ownershipObserverStop?.();
    return { ids: revived.map(entry => entry.agentId).sort(), owned: !!registry.get("child")?.ownership, registry };
  }

  test("an absent home under a held claim survives a parse and a revival; once the claim is released with the home gone, the next parse drops it", t => {
    silenceDiagnostics(t);
    const { storage, home, serialized } = sidecarFixture();
    let during: ReturnType<typeof parseOrphans> | undefined, revivedDuring: ReturnType<typeof revive> | undefined;
    const result = removeOwnedAgentHome(readOwnership(home)!, staged => {
      assert.equal(existsSync(home), false, "precondition: the home is detached");
      assert.equal(existsSync(claimOf(home)), true, "precondition: the claim is held");
      during = parseOrphans(serialized);
      revivedDuring = revive(storage, serialized);
      rmSync(staged, { recursive: true, force: true });
    }, undefined, retentionOptions);
    assert.equal(result.status, "deleted");
    assert.deepEqual(during!.map(entry => entry.agentId), ["child", "kept"], "the claimed entry survives the parse");
    assert.equal(during!.find(entry => entry.agentId === "child")!.ownership?.home, home, "its descriptor is kept for a rename-back");
    assert.deepEqual(revivedDuring!.ids, ["child", "kept"], "and the revival");
    assert.equal(revivedDuring!.owned, true);
    assert.deepEqual(descendantUsageValue(revivedDuring!.registry), usd(.75, 2), "the completed removal counts once: reconcile drops the revived entry");
    assert.equal(revivedDuring!.registry.has("child"), false);
    assert.deepEqual(parseOrphans(serialized).map(entry => entry.agentId), ["kept"], "the next parse drops it");
    assert.deepEqual(revive(storage, serialized).ids, ["kept"]);
  });

  test("an eviction record with the home present under a held claim survives a parse; after the removal rolls back the entry is still revivable", t => {
    silenceDiagnostics(t);
    const { storage, home, serialized } = sidecarFixture();
    let during: ReturnType<typeof parseOrphans> | undefined;
    const result = removeOwnedAgentHome(readOwnership(home)!, undefined, undefined, {
      ...retentionOptions,
      beforeDetach: () => {
        assert.ok(readEvictionRecord(storage, "child"), "precondition: the record is written under the claim");
        assert.equal(existsSync(home), true, "precondition: the home is present");
        during = parseOrphans(serialized);
        throw new Error("injected failure before the detach");
      },
    });
    assert.equal(result.status, "failed");
    assert.deepEqual(during!.map(entry => entry.agentId), ["child", "kept"], "the claimed entry survives the parse");
    assert.equal(readEvictionRecord(storage, "child"), undefined, "precondition: the rollback deleted the record");
    assert.equal(existsSync(claimOf(home)), false, "precondition: the claim is released");
    const after = revive(storage, serialized);
    assert.deepEqual(after.ids, ["child", "kept"], "still revivable");
    assert.equal(after.owned, true);
  });

  test("with no claim, a legacy absent home without a record is still dropped", () => {
    const { storage, home, serialized } = sidecarFixture();
    rmSync(home, { recursive: true, force: true });
    assert.equal(readEvictionRecord(storage, "child"), undefined, "precondition: no record");
    assert.deepEqual(parseOrphans(serialized).map(entry => entry.agentId), ["kept"]);
    assert.deepEqual(revive(storage, serialized).ids, ["kept"]);
  });

  test("a record read while a remover takes the claim is not proof of removal", t => {
    const { storage, home } = sidecarFixture();
    const ownership = readOwnership(home)!;
    assert.equal(writeEvictionRecord(storage, "child", usd(.5)), true);
    // The remover takes the claim while the gate reads the record (it wrote
    // the record under that claim).
    const original = fs.readFileSync;
    t.mock.method(fs, "readFileSync", function (this: unknown, path: fs.PathOrFileDescriptor, ...rest: unknown[]) {
      if (String(path).includes(`${join(".cost-estimate", "evicted")}`) && !existsSync(claimOf(home))) mkdirSync(claimOf(home));
      return (original as any).call(this, path, ...rest);
    });
    syncBuiltinESMExports();
    try { assert.equal(ownedHomeRemovalState(ownership), "claimed"); }
    finally { t.mock.restoreAll(); syncBuiltinESMExports(); }
    rmSync(claimOf(home), { recursive: true, force: true });
    assert.equal(ownedHomeRemovalState(ownership), "removed", "positive control: the same record with no claim is removal");
  });

  test("an absent home renamed back as the claim is released reads as present", t => {
    const { home } = sidecarFixture();
    const ownership = readOwnership(home)!;
    const staged = `${home}.detached`;
    renameSync(home, staged);
    mkdirSync(claimOf(home));
    // The remover renames the home back and releases its claim between the
    // gate's first home read and its claim read.
    const original = fs.existsSync;
    t.mock.method(fs, "existsSync", function (this: unknown, path: fs.PathLike) {
      if (String(path) === claimOf(home) && existsSync(staged)) { renameSync(staged, home); rmSync(claimOf(home), { recursive: true, force: true }); }
      return original.call(this, path);
    });
    syncBuiltinESMExports();
    try { assert.equal(ownedHomeRemovalState(ownership), "present"); }
    finally { t.mock.restoreAll(); syncBuiltinESMExports(); }
  });
});

describe("freshness", () => {
  function context() {
    let factory: any;
    const ctx = {
      mode: "tui", cwd: "/work/project",
      model: { provider: "provider", id: "model", reasoning: false, contextWindow: 200_000 },
      sessionManager: { getEntries: () => [], getSessionName: () => undefined, getCwd: () => "/work/project" },
      getContextUsage: () => ({ tokens: 1, percent: 0, contextWindow: 200_000 }),
      ui: { setFooter(next: any) { factory = next; } },
    };
    const mount = () => factory(
      { requestRender() {} },
      { fg: (_color: string, text: string) => text },
      { getGitBranch: () => undefined, getExtensionStatuses: () => new Map(), getAvailableProviderCount: () => 1, onBranchChange: () => () => {} },
    );
    return { ctx: ctx as never, mount };
  }

  test("a record written by another process is reflected at the owner's next reconcile", { timeout: 20_000 }, async () => {
    const storage = createAgentStorageContext("lead", root());
    const kept = record("kept", storage, .25);
    kept.client = {} as never;
    const registry = owner(storage, kept);
    assert.equal(writeEvictionRecord(storage, "earlier", usd(.125)), true);
    const past = new Date(Date.now() - 60_000);
    utimesSync(evictedDir(storage), past, past);
    const ui = context();
    const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth }, async () => undefined as never);
    try {
      const component = ui.mount();
      assert.match(component.render(120)[1], /D ~\$0\.38/, "precondition: the settled record is counted");
      const writer = nodeProcess(`
        import { writeEvictionRecord } from ${STORAGE_URL};
        writeSync(1, "result " + JSON.stringify(writeEvictionRecord({ root: ${JSON.stringify(storage.root)}, ownerSessionId: "lead" }, "elsewhere", { knownUsd: 0.5, knownContributors: 1, unknownContributors: 0, descendants: 1 })) + "\\n");
      `);
      await writer.closed.finally(() => writer.kill());
      assert.equal(resultOf(writer), true);
      controller.refreshAgents();
      assert.match(component.render(120)[1], /D ~\$0\.88/, "the footer reflects the new record at the next refresh");
      assert.deepEqual(descendantUsageValue(registry), usd(.875, 3));
    } finally { controller.stop(); }
  });

  test("a record landing within the settle window of the last read is still seen, even with an unchanged directory signal", () => {
    const storage = createAgentStorageContext("lead", root());
    const kept = record("kept", storage, .25);
    const registry = owner(storage, kept);
    assert.equal(writeEvictionRecord(storage, "first", usd(.5)), true);
    // Pin one recent mtime so the second write can reproduce the same signal.
    const tick = new Date(Date.now());
    utimesSync(evictedDir(storage), tick, tick);
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2));
    assert.equal(writeEvictionRecord(storage, "second", usd(.125)), true);
    utimesSync(evictedDir(storage), tick, tick);
    assert.deepEqual(descendantUsageValue(registry), usd(.875, 3), "a signal younger than the settle window was not cached");
  });

  test("a failed record read keeps the last good records and is retried, not cached", t => {
    const storage = createAgentStorageContext("lead", root());
    const kept = record("kept", storage, .25);
    const registry = owner(storage, kept);
    assert.equal(writeEvictionRecord(storage, "first", usd(.5)), true);
    const past = new Date(Date.now() - 60_000);
    utimesSync(evictedDir(storage), past, past);
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "warm");
    assert.equal(writeEvictionRecord(storage, "second", usd(.125)), true);
    const older = new Date(Date.now() - 30_000);
    utimesSync(evictedDir(storage), older, older);
    const unreadable = join(evictedDir(storage), "second.json");
    t.mock.method(fs, "readFileSync", ((original: typeof fs.readFileSync) => function (this: unknown, path: fs.PathOrFileDescriptor, ...rest: unknown[]) {
      if (String(path) === unreadable) throw new Error("injected read failure");
      return (original as any).call(this, path, ...rest);
    })(fs.readFileSync));
    syncBuiltinESMExports();
    try {
      assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "the failed read hides no record already counted");
    } finally { t.mock.restoreAll(); syncBuiltinESMExports(); }
    assert.deepEqual(descendantUsageValue(registry), usd(.875, 3), "the settled signal was not cached by the failed read");
  });

  test("repeated reconciles and renders with no change do not rescan evicted/", t => {
    const storage = createAgentStorageContext("lead", root());
    const kept = record("kept", storage, .25);
    kept.client = {} as never;
    const registry = owner(storage, kept);
    assert.equal(writeEvictionRecord(storage, "gone", usd(.5)), true);
    // A signal younger than the settle window is deliberately never cached.
    const past = new Date(Date.now() - 60_000);
    utimesSync(evictedDir(storage), past, past);
    const ui = context();
    const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth }, async () => undefined as never);
    const evictedPath = evictedDir(storage);
    let scans = 0;
    t.mock.method(fs, "readdirSync", ((original: typeof fs.readdirSync) => function (this: unknown, path: fs.PathLike, ...rest: unknown[]) {
      if (String(path) === evictedPath) scans += 1;
      return (original as any).call(this, path, ...rest);
    })(fs.readdirSync));
    syncBuiltinESMExports();
    try {
      const component = ui.mount();
      assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "warm");
      scans = 0;
      for (let i = 0; i < 5; i++) {
        assert.deepEqual(descendantUsageValue(registry), usd(.75, 2));
        controller.refreshAgents();
        assert.match(component.render(120)[1], /D ~\$0\.75/);
      }
      assert.equal(scans, 0, "an unchanged signal never lists the directory");
      assert.equal(writeEvictionRecord(storage, "another", usd(.125)), true);
      assert.deepEqual(descendantUsageValue(registry), usd(.875, 3));
      assert.ok(scans >= 1, "positive control: a changed directory is rescanned through the guarded fs");
    } finally { t.mock.restoreAll(); syncBuiltinESMExports(); controller.stop(); }
  });
});

describe("containment", () => {
  test("a symlinked evicted/ directory is refused: not read, counted, written through, or deleted through", { skip: symlinkSkip() }, t => {
    silenceDiagnostics(t);
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5);
    const registry = owner(storage, child);
    stale(child);
    const outside = join(root(), "outside");
    mkdirSync(outside);
    const planted = JSON.stringify({ version: 1, agentId: "child", cost: usd(.5) });
    writeFileSync(join(outside, "child.json"), planted);
    writeFileSync(join(outside, "other.json"), JSON.stringify({ version: 1, agentId: "other", cost: usd(4) }));
    mkdirSync(join(storage.root, "ws-agents", "lead", ".cost-estimate"));
    symlinkSync(outside, evictedDir(storage));

    assert.equal(readEvictionRecord(storage, "child"), undefined);
    assert.equal(readEvictionRecords(storage).size, 0);
    assert.equal(hasEvictionRecord(child.ownership!), false);
    assert.deepEqual(descendantUsageValue(registry), usd(.5), "only the live child; no planted record");
    assert.equal(registry.has("child"), true);
    assert.equal(writeEvictionRecord(storage, "fresh", usd(1)), false);
    assert.equal(removeOwnedAgentHome(readOwnership(child.ownership!.home)!, undefined, () => false).status, "retained");
    assert.equal(removeOwnedAgentHome(readOwnership(child.ownership!.home)!, undefined, undefined, retentionOptions).status, "failed");
    assert.equal(existsSync(child.ownership!.home), true);
    assert.deepEqual(readdirSync(outside).sort(), ["child.json", "other.json"], "nothing written or deleted through the symlink");
    assert.equal(readFileSync(join(outside, "child.json"), "utf8"), planted);
  });

  test("a symlinked record file is refused: not read, counted, written through, or deleted through", { skip: symlinkSkip() }, t => {
    silenceDiagnostics(t);
    const storage = createAgentStorageContext("lead", root());
    const child = record("child", storage, .5);
    const registry = owner(storage, child);
    stale(child);
    assert.equal(writeEvictionRecord(storage, "sibling", usd(.25)), true);
    const outside = join(root(), "planted.json");
    const planted = JSON.stringify({ version: 1, agentId: "child", cost: usd(8) });
    writeFileSync(outside, planted);
    const link = join(evictedDir(storage), "child.json");
    symlinkSync(outside, link);

    assert.equal(readEvictionRecord(storage, "child"), undefined);
    assert.deepEqual([...readEvictionRecords(storage).keys()], ["sibling"]);
    assert.equal(hasEvictionRecord(child.ownership!), false);
    assert.deepEqual(descendantUsageValue(registry), usd(.75, 2), "the child counts live; the planted record is ignored");
    assert.equal(registry.has("child"), true);
    // A removal that ends with the home in place attempts the repair: it must not delete through the link.
    assert.equal(removeOwnedAgentHome(readOwnership(child.ownership!.home)!, undefined, () => false).status, "retained");
    assert.equal(lstatSync(link).isSymbolicLink(), true, "the repair refused the symlinked record");
    assert.equal(readFileSync(outside, "utf8"), planted);
    // A record write replaces the link atomically instead of writing through it.
    writeEvictionRecord(storage, "child", usd(.5));
    assert.equal(readFileSync(outside, "utf8"), planted, "nothing written through the symlink");
  });
});

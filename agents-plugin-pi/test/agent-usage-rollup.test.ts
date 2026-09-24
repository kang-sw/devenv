/**
 * In-process contract for the descendant-usage roll-up
 * (260924-feat-pi-agent-channel-usage-rollup): parent-side ordering and
 * durable storage, the child-side reporter over a real channel pair, the own
 * reduction's preservation of the reported value, and the per-hop fold rule.
 * The multi-process tree and restart cases live in
 * `agent-usage-rollup.integration.test.ts`.
 */
import assert from "node:assert/strict";
import fs from "node:fs";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { syncBuiltinESMExports } from "node:module";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, test } from "node:test";
import { ChildChannel, ParentChannel } from "../src/agent-channel.ts";
import { allocateAgentHome, createAgentStorageContext, persistOwnershipTelemetry, readOwnership, updateOwnership } from "../src/agent-storage.ts";
import { parseTelemetry, type CumulativeCost } from "../src/agent-telemetry.ts";
import {
  DESCENDANT_USAGE_MESSAGE,
  DESCENDANT_USAGE_RESUME_KEY,
  acceptDescendantUsage,
  attachDescendantUsage,
  createDescendantUsageReporter,
  descendantUsageOf,
} from "../src/agent-usage-rollup.ts";
import { descendantUsageValue, persistAgentCostCheckpoint, persistEvictedAgentCost, persistOwnedTelemetryRollup, registerAgentCostOwner } from "../src/agent-footer.ts";
import { evictForCapacity, refreshAgentTelemetry, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";
import { captureOrphans, readAndClearSidecarAt, reviveOrphans, writeSidecarAt } from "../src/agent-sidecar.ts";

const roots = new Set<string>();
afterEach(() => { for (const root of roots) rmSync(root, { recursive: true, force: true }); roots.clear(); });
function root(): string { const value = mkdtempSync(join(tmpdir(), "ws-pi-rollup-")); roots.add(value); return value; }
type Storage = ReturnType<typeof createAgentStorageContext>;

const usd = (knownUsd: number, descendants = 1): CumulativeCost => ({ knownUsd, knownContributors: 1, unknownContributors: 0, descendants });
function record(agentId: string, storage: Storage, ownUsd: number | undefined): RpcAgentRecord {
  const ownership = allocateAgentHome(storage, agentId, "worker");
  const value: RpcAgentRecord = {
    agentId, sessionPath: ownership.sessionPath!, ownership,
    ...(ownUsd === undefined ? {} : { telemetry: { version: 1 as const, origin: { sessionId: `${agentId}-session`, sessionPath: ownership.sessionPath!, emptyPrefix: true as const }, estimatedUsd: ownUsd } }),
    wsToolNames: [], toolGroup: "full-worker", streaming: false, running: false, reportLog: [],
  } as RpcAgentRecord;
  if (value.telemetry) persistOwnershipTelemetry(ownership.home, value.telemetry);
  return value;
}
function stopped(child: RpcAgentRecord): void { updateOwnership(child.ownership!.home, { liveness: { lifecycle: "stopped", running: false } }); }
function checkpoint(storage: Storage): any {
  return JSON.parse(readFileSync(join(storage.root, "ws-agents", storage.ownerSessionId, ".cost-estimate", "checkpoint.json"), "utf8"));
}
function assistant(id: string, cost: number, tokens = 100) {
  return { type: "message", id, message: { role: "assistant", usage: { input: tokens, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: tokens, cost: { total: cost } } } };
}
function writeSession(path: string, sessionId: string, entries: unknown[], parentSession?: string): void {
  writeFileSync(path, [{ type: "session", version: 3, id: sessionId, ...(parentSession ? { parentSession } : {}) }, ...entries].map(entry => JSON.stringify(entry)).join("\n") + "\n");
}

describe("parent-side ordering", () => {
  test("keeps the highest sequence per launch: duplicates and late stale values neither inflate nor lower", () => {
    const child = record("child", createAgentStorageContext("lead", root()), .1);
    assert.equal(acceptDescendantUsage(child, 1, { seq: 1, usage: usd(1) }), true);
    assert.equal(acceptDescendantUsage(child, 1, { seq: 3, usage: usd(3) }), true);
    assert.equal(acceptDescendantUsage(child, 1, { seq: 3, usage: usd(9) }), false, "a duplicate sequence is ignored");
    assert.equal(acceptDescendantUsage(child, 1, { seq: 2, usage: usd(2) }), false, "a late lower sequence cannot lower the total");
    assert.equal(acceptDescendantUsage(child, 1, { seq: 1, usage: usd(9) }), false, "a late replay cannot inflate the total");
    assert.deepEqual(descendantUsageOf(child), usd(3));
  });

  test("a newer launch's first value replaces the stored one; the previous launch's late value is then ignored", () => {
    const child = record("child", createAgentStorageContext("lead", root()), .1);
    acceptDescendantUsage(child, 1, { seq: 5, usage: usd(5) });
    assert.equal(acceptDescendantUsage(child, 2, { seq: 1, usage: usd(4) }), true, "generation 2 replaces even a lower value at seq 1");
    assert.deepEqual(descendantUsageOf(child), usd(4));
    assert.equal(acceptDescendantUsage(child, 1, { seq: 6, usage: usd(7) }), false, "generation 1 after generation 2 is ignored");
    assert.deepEqual(descendantUsageOf(child), usd(4));
  });

  test("malformed reports are ignored and consume no sequence", () => {
    const child = record("child", createAgentStorageContext("lead", root()), .1);
    for (const bad of [undefined, null, {}, { seq: 0, usage: usd(1) }, { seq: 1.5, usage: usd(1) }, { seq: 1, usage: { knownUsd: -1, knownContributors: 1, unknownContributors: 0, descendants: 1 } }]) {
      assert.equal(acceptDescendantUsage(child, 1, bad), false);
    }
    assert.equal(child.descendantUsageOrder, undefined);
    assert.equal(acceptDescendantUsage(child, 1, { seq: 1, usage: usd(1) }), true);
  });

  test("an accepted value is stored in ownership telemetry beside the own-usage fields; unchanged values write nothing", t => {
    const child = record("child", createAgentStorageContext("lead", root()), .1);
    acceptDescendantUsage(child, 1, { seq: 1, usage: usd(2) });
    const persisted = readOwnership(child.ownership!.home)!;
    assert.equal(persisted.telemetry?.estimatedUsd, .1);
    assert.deepEqual(persisted.telemetry?.descendantUsage, usd(2));
    const writes = t.mock.method(fs, "renameSync");
    syncBuiltinESMExports();
    try {
      persistOwnershipTelemetry(child.ownership!.home, child.telemetry);
      assert.equal(writes.mock.callCount(), 0, "the write-on-change comparison covers the descendant field: no change, no write");
      acceptDescendantUsage(child, 1, { seq: 2, usage: usd(3) });
      assert.equal(writes.mock.callCount(), 1, "a changed descendant field alone is a change");
    } finally { t.mock.restoreAll(); syncBuiltinESMExports(); }
    assert.deepEqual(readOwnership(child.ownership!.home)!.telemetry?.descendantUsage, usd(3));
  });

  test("a child without own-usage telemetry keeps the value in memory and never clears its durable record", () => {
    const child = record("child", createAgentStorageContext("lead", root()), undefined);
    assert.equal(acceptDescendantUsage(child, 1, { seq: 1, usage: usd(2) }), true);
    assert.deepEqual(descendantUsageOf(child), usd(2));
    assert.ok(readOwnership(child.ownership!.home), "the ownership record is intact");
  });

  test("parseTelemetry keeps a valid descendant value and drops a malformed one", () => {
    const base = { version: 1, origin: { sessionId: "s", sessionPath: "/p", emptyPrefix: true }, estimatedUsd: 1 };
    assert.deepEqual(parseTelemetry({ ...base, descendantUsage: usd(2) })?.descendantUsage, usd(2));
    assert.equal(parseTelemetry({ ...base, descendantUsage: { knownUsd: "2" } })?.descendantUsage, undefined);
  });
});

describe("reporter over a real channel", () => {
  async function pair(generation = 3) {
    const parent = await ParentChannel.bind(generation, { force: "tcp" });
    const child = await ChildChannel.connect({ endpoint: parent.endpoint, credential: parent.credential, generation }, { backoffCapMs: 50 });
    return { parent, child };
  }

  test("sends only on change, never for an unchanged or zero value; the parent stores each change", async () => {
    const { parent, child } = await pair();
    try {
      const lead = record("hop", createAgentStorageContext("lead", root()), .5);
      lead.launchGeneration = parent.generation;
      const messages: Record<string, unknown>[] = [];
      parent.onMessage(msg => { if (msg.t === DESCENDANT_USAGE_MESSAGE) messages.push(msg); });
      let changed!: () => void;
      let changes = 0;
      attachDescendantUsage(lead, parent, () => { changes += 1; changed?.(); });
      let value: CumulativeCost = { knownUsd: 0, knownContributors: 0, unknownContributors: 0, descendants: 0 };
      const reporter = createDescendantUsageReporter(child);
      reporter.evaluate();
      reporter.setSource(() => value);
      reporter.evaluate(); reporter.evaluate();
      value = usd(1.25, 2);
      const arrived = new Promise<void>(resolve => { changed = resolve; });
      reporter.evaluate(); reporter.evaluate(); reporter.evaluate();
      await arrived;
      await new Promise(resolve => setTimeout(resolve, 50));
      assert.equal(messages.length, 1, "a leaf's zero value and repeated evaluations of an unchanged value send nothing");
      assert.deepEqual(messages[0], { t: DESCENDANT_USAGE_MESSAGE, seq: 1, usage: usd(1.25, 2), gen: parent.generation });
      assert.equal(changes, 1);
      assert.deepEqual(descendantUsageOf(lead), usd(1.25, 2));
      reporter.setSource(undefined);
      value = usd(9);
      reporter.evaluate();
      await new Promise(resolve => setTimeout(resolve, 50));
      assert.equal(messages.length, 1, "a paused source sends nothing");
    } finally { child.close(); parent.close(); }
  });

  test("a change while disconnected rides the reconnect hello's resume section and is accepted once", async () => {
    const { parent, child } = await pair();
    try {
      const lead = record("hop", createAgentStorageContext("lead", root()), .5);
      lead.launchGeneration = parent.generation;
      let value = usd(1);
      const reporter = createDescendantUsageReporter(child);
      reporter.setSource(() => value);
      const changes: CumulativeCost[] = [];
      let notify: (() => void) | undefined;
      attachDescendantUsage(lead, parent, () => { changes.push(descendantUsageOf(lead)!); notify?.(); });
      const reports: Record<string, unknown>[] = [];
      let resent: (() => void) | undefined;
      // Registered after attachDescendantUsage, so each report was already applied when this runs.
      parent.onMessage(msg => { if (msg.t === DESCENDANT_USAGE_MESSAGE) { reports.push(msg); if (msg.seq === 2) resent?.(); } });
      let first = new Promise<void>(resolve => { notify = resolve; });
      reporter.evaluate();
      await first;
      const resumed = new Promise<Record<string, unknown>>(resolve => parent.onConnection((_conn, hello) => { if (hello.reconnect) resolve(hello.resume); }));
      const dropped = new Promise<void>(resolve => child.onDisconnect(() => {
        // Evaluated while the child has no connection: the send is skipped.
        value = usd(2);
        reporter.evaluate();
        resolve();
      }));
      first = new Promise<void>(resolve => { notify = resolve; });
      const resend = new Promise<void>((resolve, reject) => {
        resent = resolve;
        setTimeout(() => reject(new Error("no report was resent after the reconnect")), 2_000).unref();
      });
      resend.catch(() => { /* awaited below; an earlier failure must not leave it unhandled */ });
      parent.live!.close();
      await dropped;
      assert.deepEqual((await resumed)[DESCENDANT_USAGE_RESUME_KEY], { seq: 2, usage: usd(2) });
      await first;
      // The reporter resends its latest report once the reconnect completes;
      // the parent's sequence check drops it as a duplicate of the hello's.
      await resend;
      assert.deepEqual(reports, [
        { t: DESCENDANT_USAGE_MESSAGE, seq: 1, usage: usd(1), gen: parent.generation },
        { t: DESCENDANT_USAGE_MESSAGE, seq: 2, usage: usd(2), gen: parent.generation },
      ]);
      assert.deepEqual(changes, [usd(1), usd(2)], "the resend is not counted as a change");
      assert.deepEqual(lead.descendantUsageOrder, { generation: parent.generation, seq: 2 });
    } finally { child.close(); parent.close(); }
  });

  test("the channel's own readiness resume key cannot be claimed by a feature", async () => {
    const { parent, child } = await pair();
    try {
      assert.throws(() => child.provideResume("readiness", () => ({})), /resume key "readiness" is reserved/);
      assert.doesNotThrow(() => child.provideResume("another-feature", () => undefined)(), "positive control: any other key is accepted");
    } finally { child.close(); parent.close(); }
  });

  test("a report for a record that has moved to another launch is ignored", async () => {
    const { parent, child } = await pair();
    try {
      const lead = record("hop", createAgentStorageContext("lead", root()), .5);
      lead.launchGeneration = parent.generation + 1;
      let changes = 0;
      attachDescendantUsage(lead, parent, () => { changes += 1; });
      const seen = new Promise<void>(resolve => parent.onMessage(() => resolve()));
      child.send({ t: DESCENDANT_USAGE_MESSAGE, seq: 1, usage: usd(1) });
      await seen;
      assert.equal(changes, 0);
      assert.equal(descendantUsageOf(lead), undefined);
    } finally { child.close(); parent.close(); }
  });
});

describe("own reduction and the reported value", () => {
  test("a fork hop's own usage still excludes its inherited prefix and keeps context tokens, and the reported value survives the refresh", () => {
    const storage = createAgentStorageContext("lead", root());
    const fork = record("fork", storage, undefined);
    writeSession(fork.sessionPath, "fork-session", [assistant("inherited", 5, 900)], "/parent/session.jsonl");
    fork.telemetry = { version: 1, origin: { sessionId: "fork-session", sessionPath: fork.sessionPath, prefixEntryId: "inherited" } };
    acceptDescendantUsage(fork, 1, { seq: 1, usage: usd(2) });
    writeSession(fork.sessionPath, "fork-session", [assistant("inherited", 5, 900), assistant("own", .25, 300)], "/parent/session.jsonl");
    refreshAgentTelemetry(fork);
    assert.equal(fork.telemetry?.estimatedUsd, .25, "the inherited parent-history prefix is not the fork's own usage");
    assert.equal(fork.telemetry?.contextTokens, 300, "the widget/audit context field is the child's own latest occupancy");
    assert.deepEqual(fork.telemetry?.descendantUsage, usd(2));
    assert.deepEqual(readOwnership(fork.ownership!.home)!.telemetry?.descendantUsage, usd(2));
  });

  test("a telemetry reset (a different session) drops own usage but not the reported descendant value", () => {
    const child = record("child", createAgentStorageContext("lead", root()), .1);
    acceptDescendantUsage(child, 1, { seq: 1, usage: usd(2) });
    writeSession(child.sessionPath, "a-different-session", [assistant("x", 1)]);
    refreshAgentTelemetry(child);
    assert.equal(child.telemetry, undefined);
    assert.deepEqual(descendantUsageOf(child), usd(2));
    // The next refresh binds telemetry to the new session; the reported value
    // must be restored into it and into the durable ownership record.
    refreshAgentTelemetry(child);
    assert.equal(child.telemetry?.origin.sessionId, "a-different-session", "precondition: telemetry was re-created for the new session");
    assert.equal(child.telemetry?.estimatedUsd, 1);
    assert.deepEqual(child.telemetry?.descendantUsage, usd(2));
    assert.deepEqual(readOwnership(child.ownership!.home)!.telemetry?.descendantUsage, usd(2));
  });
});

describe("per-hop value and the fold rule", () => {
  test("a hop's value is its direct children's subtree totals, dormant ones included, plus its evicted baseline, and reads no session file", t => {
    const storage = createAgentStorageContext("hop", root());
    const live = record("live", storage, .5), dormant = record("dormant", storage, .25);
    live.client = {} as any;
    acceptDescendantUsage(dormant, 1, { seq: 1, usage: usd(1, 3) });
    const registry: RpcAgentRegistry = new Map([[live.agentId, live], [dormant.agentId, dormant]]);
    registerAgentCostOwner(registry, storage);
    const bucket = join(storage.root, "ws-agents", storage.ownerSessionId, ".cost-estimate");
    fs.mkdirSync(bucket, { recursive: true });
    writeFileSync(join(bucket, "checkpoint.json"), JSON.stringify({
      version: 1, lead: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cost: { knownUsd: 0, knownContributors: 0, unknownContributors: 0, descendants: 0 } },
      evictedBaseline: usd(.1), agents: [],
    }));
    const reads = t.mock.method(fs, "readFileSync", ((original: typeof fs.readFileSync) => function (this: unknown, path: fs.PathOrFileDescriptor, ...rest: unknown[]) {
      assert.doesNotMatch(String(path), /\.jsonl$/, "the hop value never reads a session");
      return (original as any).call(this, path, ...rest);
    })(fs.readFileSync));
    syncBuiltinESMExports();
    let value: CumulativeCost | undefined;
    try { value = descendantUsageValue(registry); } finally { t.mock.restoreAll(); syncBuiltinESMExports(); }
    assert.ok(reads.mock.callCount() > 0, "positive control: the checkpoint read went through the guarded fs");
    assert.equal(value!.knownUsd.toFixed(2), "1.85", "live .5 + dormant .25 + its descendants 1 + evicted baseline .1");
    assert.equal(value!.unknownContributors, 0);
  });

  test("evicting a direct child folds its whole subtree into the baseline exactly once, and a restart rebuilds the same value", () => {
    const storage = createAgentStorageContext("hop", root());
    const evicted = record("evicted", storage, .5), kept = record("kept", storage, .25);
    acceptDescendantUsage(evicted, 1, { seq: 1, usage: usd(2, 2) });
    const registry: RpcAgentRegistry = new Map([[evicted.agentId, evicted], [kept.agentId, kept]]);
    registerAgentCostOwner(registry, storage);
    assert.equal(descendantUsageValue(registry)!.knownUsd, 2.75);
    stopped(evicted);
    assert.deepEqual(evictForCapacity(registry, 2), { ok: true, evictedLabel: "evicted" });
    // Whole object: folding only the own usage still keeps knownUsd at 2.5
    // through the monotonic merge with the reconciled entry; the contributor
    // counts are what show it.
    const subtree = { knownUsd: 2.5, knownContributors: 2, unknownContributors: 0, descendants: 3 };
    assert.deepEqual(checkpoint(storage).evictedBaseline, subtree, "own .5 plus stored descendant 2");
    assert.equal(persistEvictedAgentCost(registry, evicted), true, "a retry does not fold again");
    assert.deepEqual(checkpoint(storage).evictedBaseline, subtree);
    assert.equal(descendantUsageValue(registry)!.knownUsd, 2.75, "eviction moves the subtree into the baseline without changing the value");
    persistAgentCostCheckpoint(registry);

    // Restart: a fresh registry revived from the kept child's durable record.
    const revived = { ...kept, telemetry: readOwnership(kept.ownership!.home)!.telemetry } as RpcAgentRecord;
    delete revived.descendantUsage; delete revived.descendantUsageOrder;
    const restarted: RpcAgentRegistry = new Map([[revived.agentId, revived]]);
    registerAgentCostOwner(restarted, storage);
    assert.equal(descendantUsageValue(restarted)!.knownUsd, 2.75, "the checkpoint restores the fold; nothing is counted twice");
  });

  test("an identity that leaves the registry without a removal is dropped, not folded, so a later retention removal folds it once", () => {
    const storage = createAgentStorageContext("hop", root());
    const child = record("child", storage, .5);
    acceptDescendantUsage(child, 1, { seq: 1, usage: usd(1) });
    const registry: RpcAgentRegistry = new Map([[child.agentId, child]]);
    registerAgentCostOwner(registry, storage);
    assert.equal(descendantUsageValue(registry)!.knownUsd, 1.5);
    persistAgentCostCheckpoint(registry);
    assert.deepEqual(checkpoint(storage).agents.map((entry: any) => entry.agentId), ["child"]);

    // A restart without the child in the registry (no sidecar) drops it.
    const restarted: RpcAgentRegistry = new Map();
    registerAgentCostOwner(restarted, storage);
    assert.equal(descendantUsageValue(restarted)!.knownUsd, 0);
    persistAgentCostCheckpoint(restarted);
    assert.equal(checkpoint(storage).evictedBaseline.knownUsd, 0, "no fold without a removal");
    assert.deepEqual(checkpoint(storage).agents, []);

    // Retention removes the home: one fold of the subtree total.
    stopped(child);
    assert.equal(persistOwnedTelemetryRollup(readOwnership(child.ownership!.home)!), true);
    assert.equal(checkpoint(storage).evictedBaseline.knownUsd, 1.5);
  });

  test("a sidecar orphan whose home retention already removed is not revived, so its folded subtree counts once; one whose home exists still is", () => {
    const storage = createAgentStorageContext("hop", root());
    const removed = record("removed", storage, .5), kept = record("kept", storage, .25);
    // parseOrphans keeps only an entry with a system prompt or a fork context.
    removed.systemPromptPath = kept.systemPromptPath = join(storage.root, "prompt.md");
    acceptDescendantUsage(removed, 1, { seq: 1, usage: usd(1, 2) });
    const registry: RpcAgentRegistry = new Map([[removed.agentId, removed], [kept.agentId, kept]]);
    registerAgentCostOwner(registry, storage);
    assert.equal(descendantUsageValue(registry)!.knownUsd, 1.75);
    persistAgentCostCheckpoint(registry);
    const orphans = captureOrphans(registry);
    assert.deepEqual(orphans.map(orphan => [orphan.agentId, !!orphan.ownership]), [["removed", true], ["kept", true]], "precondition: both sidecar entries are owned");
    const sidecar = join(root(), "hop.sidecar.json");
    writeSidecarAt(sidecar, orphans);

    // Retention (possibly another owner's lead): fold, then remove the home.
    stopped(removed);
    assert.equal(persistOwnedTelemetryRollup(readOwnership(removed.ownership!.home)!), true);
    rmSync(removed.ownership!.home, { recursive: true, force: true });
    const folded = { knownUsd: 1.5, knownContributors: 2, unknownContributors: 0, descendants: 3 };
    assert.deepEqual(checkpoint(storage).evictedBaseline, folded, "own .5 plus stored descendant 1, folded once");

    // Restart: the hop revives from the sidecar written before the removal,
    // through the production read path (which re-validates ownership against
    // disk), and, separately, from the in-memory entries.
    for (const [path, entries] of [["sidecar file", readAndClearSidecarAt(sidecar)], ["in-memory entries", orphans]] as const) {
      const restarted: RpcAgentRegistry = new Map();
      registerAgentCostOwner(restarted, storage);
      const revived = reviveOrphans(restarted, [...entries]);
      try {
        assert.deepEqual(revived.map(entry => entry.agentId), ["kept"], `${path}: positive control: the orphan whose home exists is revived`);
        assert.equal(restarted.has("removed"), false, path);
        assert.deepEqual(descendantUsageValue(restarted), { knownUsd: 1.75, knownContributors: 3, unknownContributors: 0, descendants: 4 }, `${path}: baseline 1.5 plus kept .25; the removed subtree only through the baseline`);
      } finally { for (const entry of revived) entry.ownershipObserverStop?.(); }
    }
  });
});

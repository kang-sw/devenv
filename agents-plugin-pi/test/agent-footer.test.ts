import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, test } from "node:test";
import { allocateAgentHome, createAgentStorageContext, updateOwnership } from "../src/agent-storage.ts";
import {
  createAgentFooterController,
  createAgentFooterSessionLifecycle,
  formatCumulativeCost,
  persistEvictedAgentCost,
  registerAgentCostOwner,
  shouldArmAgentFooter,
  type AgentFooterComponent,
} from "../src/agent-footer.ts";
import { agentWidgetRefreshRef, evictForCapacity, stopAgent, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";
import { truncateToWidth, visibleWidth } from "../src/pi-tui.ts";
import { applySessionShutdownAgentFooter, applySessionStartAgentFooter } from "../src/index.ts";

const roots = new Set<string>();
afterEach(() => { for (const root of roots) rmSync(root, { recursive: true, force: true }); roots.clear(); });
function root(): string { const value = mkdtempSync(join(tmpdir(), "ws-pi-footer-")); roots.add(value); return value; }
function telemetry(sessionId: string, sessionPath: string, cost: number | "partial" | "unknown") {
  return {
    version: 1 as const,
    origin: { sessionId, sessionPath, emptyPrefix: true as const },
    ...(typeof cost === "number" ? { estimatedUsd: cost } : cost === "partial" ? { partialEstimatedUsd: 0.25 } : {}),
  };
}
function record(agentId: string, storage: ReturnType<typeof createAgentStorageContext>, cost: number | "partial" | "unknown", role: "worker" | "execute-worker" | "fork" | "explore" = "worker"): RpcAgentRecord {
  const ownership = allocateAgentHome(storage, agentId, role);
  return {
    agentId,
    sessionPath: ownership.sessionPath!,
    ownership,
    telemetry: telemetry(`${agentId}-session`, ownership.sessionPath!, cost),
    wsToolNames: [], toolGroup: "full-worker", streaming: false, running: false, reportLog: [],
  } as RpcAgentRecord;
}
function context(entries: readonly unknown[] = []) {
  let footerFactory: any;
  let component: AgentFooterComponent | undefined;
  let restores = 0;
  const ctx = {
    mode: "tui", cwd: "/work/project",
    model: { provider: "provider", id: "model", reasoning: true, contextWindow: 200_000 }, thinkingLevel: "high",
    sessionManager: { getEntries: () => entries, getSessionName: () => "named session", getCwd: () => "/work/project" },
    getContextUsage: () => ({ percent: 25, contextWindow: 200_000 }),
    ui: { setFooter(next: any) { component?.dispose?.(); component = undefined; footerFactory = next; if (!next) restores++; } },
  };
  const mount = (fg: (color: string, text: string) => string = (_color, text) => text) => {
    component = footerFactory(
      { requestRender() {} },
      { fg },
      { getGitBranch: () => "feature/footer", getExtensionStatuses: () => new Map(), getAvailableProviderCount: () => 2, onBranchChange: () => () => {} },
    );
    return component!;
  };
  return { ctx, mount, get restores() { return restores; } };
}
function checkpoint(storage: ReturnType<typeof createAgentStorageContext>): any {
  return JSON.parse(readFileSync(join(storage.root, "ws-agents", storage.ownerSessionId, ".cost-estimate", "checkpoint.json"), "utf8"));
}
const plain = (text: string) => text.replace(/\u001b\[[0-9;]*m/g, "");

describe("bounded direct-agent estimates", () => {
  test("counts running, idle, dormant, and all direct roles while ignoring nested ownership", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir);
    const direct = [
      record("running", storage, .4), record("idle", storage, 0, "execute-worker"),
      record("dormant", storage, "unknown", "fork"), record("explore", storage, "partial", "explore"),
    ];
    direct[0].running = true; direct[0].streaming = true; direct[2].client = undefined;
    const nested = record("nested", createAgentStorageContext("running-session", dir), 99);
    assert.ok(nested.ownership, "nested fixture exists but is deliberately absent from the direct registry");
    const registry: RpcAgentRegistry = new Map(direct.map(item => [item.agentId, item]));
    const ui = context();
    const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth });
    const rendered = plain(ui.mount().render(120).join("\n"));
    assert.match(rendered, /Direct agents ~\$0\.65 \+ \?/);
    assert.doesNotMatch(rendered, /99/);
    controller.stop();
  });

  test("never decreases on missing, partial, unknown, or regressing cumulative snapshots", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), child = record("child", storage, 1);
    const registry: RpcAgentRegistry = new Map([[child.agentId, child]]), ui = context();
    const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth });
    const component = ui.mount();
    assert.match(component.render(100)[1], /Direct agents ~\$1\.00/);
    child.telemetry = undefined; controller.refreshAgents();
    assert.match(component.render(100)[1], /Direct agents ~\$1\.00 \+ \?/);
    child.telemetry = telemetry("child-session", child.sessionPath, "partial"); controller.refreshAgents();
    assert.match(component.render(100)[1], /Direct agents ~\$1\.00 \+ \?/);
    child.telemetry = telemetry("child-session", child.sessionPath, .5); controller.refreshAgents();
    assert.match(component.render(100)[1], /Direct agents ~\$1\.00 \+ \?/);
    child.telemetry = telemetry("child-session", child.sessionPath, 1.5); controller.refreshAgents();
    assert.match(component.render(100)[1], /Direct agents ~\$1\.50/);
    controller.stop();
  });

  test("capacity eviction folds each identity once into one bounded checkpoint", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir);
    const old = record("old", storage, .6), kept = record("kept", storage, .2);
    const registry: RpcAgentRegistry = new Map([[old.agentId, old], [kept.agentId, kept]]);
    registerAgentCostOwner(registry, storage);
    const ui = context(), controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth });
    const component = ui.mount();
    updateOwnership(old.ownership!.home, { liveness: { lifecycle: "stopped", running: false } });
    assert.deepEqual(evictForCapacity(registry, 2), { ok: true, evictedLabel: "old" });
    controller.refreshAgents();
    assert.match(component.render(100)[1], /Direct agents ~\$0\.80/);
    assert.equal(persistEvictedAgentCost(registry, old), true, "a retry on the same evicted record is idempotent");
    controller.stop();
    const saved = checkpoint(storage);
    assert.equal(saved.evictedBaseline.knownUsd, .6);
    assert.deepEqual(saved.agents.map((entry: any) => entry.agentId), ["kept"]);
    assert.ok(saved.agents.length <= registry.size, "checkpoint identity count is bounded by the registry");
    const restoredRegistry: RpcAgentRegistry = new Map([[kept.agentId, kept]]), restoredUi = context();
    const restored = createAgentFooterController(restoredUi.ctx, restoredRegistry, storage, { truncateToWidth, visibleWidth });
    assert.match(restoredUi.mount().render(100)[1], /Direct agents ~\$0\.80/, "reload adds the evicted baseline and restored live identity exactly once");
    restored.stop();
  });

  test("many unique evictions remain one scalar baseline plus live registry identities", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), kept = record("kept", storage, .2);
    kept.client = {} as any;
    const registry: RpcAgentRegistry = new Map([[kept.agentId, kept]]), ui = context();
    const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth });
    for (let index = 0; index < 64; index++) {
      const old = record(`old-${index}`, storage, .01);
      updateOwnership(old.ownership!.home, { liveness: { lifecycle: "stopped", running: false } });
      registry.set(old.agentId, old);
      assert.deepEqual(evictForCapacity(registry, 2), { ok: true, evictedLabel: old.agentId });
    }
    controller.stop();
    const saved = checkpoint(storage);
    assert.equal(saved.evictedBaseline.knownUsd.toFixed(2), "0.64");
    assert.deepEqual(saved.agents.map((entry: any) => entry.agentId), ["kept"]);
    assert.ok(saved.agents.length <= registry.size, "historical eviction identities never accumulate in the checkpoint");
  });

  test("reload reads one checkpoint, preserves totals, and does not double count restored records", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), child = record("child", storage, .4);
    const registry: RpcAgentRegistry = new Map([[child.agentId, child]]);
    const firstUi = context(), first = createAgentFooterController(firstUi.ctx, registry, storage, { truncateToWidth, visibleWidth });
    first.acceptUsage({ role: "assistant", usage: { input: 10, output: 2, cacheRead: 3, cacheWrite: 4, cost: { total: .5 } } });
    first.stop();

    const restored = record("child", storage, .4);
    const restoredRegistry: RpcAgentRegistry = new Map([[restored.agentId, restored]]);
    const secondUi = context([{ type: "message", message: { role: "assistant" } }]);
    const second = createAgentFooterController(secondUi.ctx, restoredRegistry, storage, { truncateToWidth, visibleWidth });
    const rendered = plain(secondUi.mount().render(120).join("\n"));
    assert.match(rendered, /↑10/); assert.match(rendered, /Lead ~\$0\.50/); assert.match(rendered, /Direct agents ~\$0\.40/);
    second.stop();
  });

  test("failed eviction checkpoint retries without double folding", (t) => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), child = record("child", storage, .4);
    const registry: RpcAgentRegistry = new Map([[child.agentId, child]]), ui = context();
    const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth });
    const component = ui.mount(), bucket = join(storage.root, "ws-agents", storage.ownerSessionId, ".cost-estimate"), blocked = join(dir, "blocked-checkpoint");
    mkdirSync(blocked); symlinkSync(blocked, bucket, "dir");
    const diagnostics = t.mock.method(console, "error", () => {});
    assert.equal(persistEvictedAgentCost(registry, child), false);
    assert.equal(persistEvictedAgentCost(registry, child), false);
    assert.match(component.render(100)[1], /Direct agents ~\$0\.40/, "failed retries leave the mounted state unfurled");
    assert.ok(diagnostics.mock.callCount() >= 2, "each failed durability boundary is surfaced");
    rmSync(bucket);
    assert.equal(persistEvictedAgentCost(registry, child), true);
    assert.equal(persistEvictedAgentCost(registry, child), true, "a committed fold stays idempotent");
    registry.delete(child.agentId);
    controller.stop();
    assert.equal(checkpoint(storage).evictedBaseline.knownUsd, .4);
  });

  test("ordinary child stop persists the post-reconciliation estimate boundary", async () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), child = record("child", storage, .2);
    const registry: RpcAgentRegistry = new Map([[child.agentId, child]]), ui = context();
    const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth });
    child.client = {
      abort: async () => { child.telemetry!.estimatedUsd = .8; },
      stop: async () => {},
    } as any;
    agentWidgetRefreshRef.current = () => controller.refreshAgents();
    try {
      await stopAgent(registry, child.agentId, undefined, { silent: true });
      assert.equal(checkpoint(storage).agents[0].cost.knownUsd, .8);
    } finally {
      agentWidgetRefreshRef.current = undefined;
      controller.stop();
    }
  });

  test("alias reuse remains identity-based", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir);
    const first = record("first-id", storage, .3), second = record("second-id", storage, .4);
    first.alias = "reviewer"; second.alias = "reviewer";
    const registry: RpcAgentRegistry = new Map([[first.agentId, first], [second.agentId, second]]), ui = context();
    const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth });
    assert.match(ui.mount().render(100)[1], /Direct agents ~\$0\.70/);
    controller.stop();
  });
});

describe("incremental lead usage", () => {
  test("failed final checkpoint is surfaced and retained across same-process reload", (t) => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), registry: RpcAgentRegistry = new Map(), firstUi = context();
    const first = createAgentFooterController(firstUi.ctx, registry, storage, { truncateToWidth, visibleWidth });
    first.acceptUsage({ role: "assistant", usage: { input: 10, output: 2, cost: { total: .5 } } });
    const owner = join(storage.root, "ws-agents", storage.ownerSessionId), bucket = join(owner, ".cost-estimate"), blocked = join(dir, "blocked-checkpoint");
    mkdirSync(owner, { recursive: true }); mkdirSync(blocked); symlinkSync(blocked, bucket, "dir");
    const diagnostics = t.mock.method(console, "error", () => {});
    first.stop();
    assert.equal(diagnostics.mock.callCount(), 1, "shutdown surfaces the failed checkpoint");
    rmSync(bucket);
    const secondUi = context(), second = createAgentFooterController(secondUi.ctx, registry, storage, { truncateToWidth, visibleWidth });
    assert.match(secondUi.mount().render(100)[1], /Lead ~\$0\.50/, "reload uses the retained in-process estimate instead of stale disk state");
    second.stop();
    assert.equal(checkpoint(storage).lead.cost.knownUsd, .5, "the next writable boundary durably catches up");
  });

  test("accepts assistant, nested tool, and compaction usage once and keeps latest assistant cache rate", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), registry: RpcAgentRegistry = new Map(), ui = context();
    const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth });
    const component = ui.mount();
    const assistant = { role: "assistant", usage: { input: 10, output: 2, cacheRead: 3, cacheWrite: 4, cost: { total: .5 } } };
    const tool = { role: "toolResult", usage: { input: 5, output: 1, cost: { total: .1 } } };
    const compact = { type: "compaction", usage: { input: 7, output: 1, cost: { total: .2 } } };
    controller.acceptUsage(assistant); controller.acceptUsage(assistant); controller.acceptUsage(tool); controller.acceptUsage(compact);
    const rendered = plain(component.render(140).join("\n"));
    assert.match(rendered, /↑22/); assert.match(rendered, /↓4/); assert.match(rendered, /CH17\.6%/); assert.match(rendered, /Lead ~\$0\.80/);
    controller.stop();
  });

  test("a missing checkpoint with existing history starts unknown rather than scanning or inventing an exact lifetime value", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), ui = context([{ type: "message", message: { role: "assistant" } }]);
    const controller = createAgentFooterController(ui.ctx, new Map(), storage, { truncateToWidth, visibleWidth });
    assert.match(ui.mount().render(80)[1], /Lead —/);
    controller.stop();
  });
});

describe("custom footer render and lifecycle", () => {
  test("render uses cached telemetry in O(1), performs no history or registry traversal, preserves widths, and labels estimates", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), registry: RpcAgentRegistry = new Map();
    for (let index = 0; index < 256; index++) registry.set(`agent-${index}`, record(`agent-${index}`, storage, .01));
    const ui = context(), controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth });
    const component = ui.mount();
    ui.ctx.sessionManager.getEntries = () => { throw new Error("render walked history"); };
    const originalEntries = registry.entries.bind(registry), originalIterator = registry[Symbol.iterator].bind(registry);
    (registry as any).entries = () => { throw new Error("render walked registry"); };
    (registry as any)[Symbol.iterator] = () => { throw new Error("render walked registry"); };
    for (let index = 0; index < 10_000; index++) {
      const lines = component.render(80);
      assert.ok(lines.every(line => visibleWidth(line) <= 80));
    }
    const wide = plain(component.render(120).join("\n"));
    assert.match(wide, /Lead ~\$0\.00/); assert.match(wide, /Direct agents ~\$2\.56/);
    (registry as any).entries = originalEntries; (registry as any)[Symbol.iterator] = originalIterator;
    controller.stop();
  });

  test("styles context occupancy as primary text through 70 percent and error above it", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), registry: RpcAgentRegistry = new Map();
    const ui = context();
    let percent = 70;
    ui.ctx.getContextUsage = () => ({ percent, contextWindow: 200_000 });
    const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth });
    const component = ui.mount((color, text) => `<${color}>${text}</${color}>`);
    assert.match(component.render(120)[1], /<text>70\.0%\/200k<\/text>/);
    percent = 70.1;
    const high = component.render(120)[1];
    assert.match(high, /<error>70\.1%\/200k<\/error>/);
    assert.doesNotMatch(high, /<warning>/);
    controller.stop();
  });

  test("replacement and shutdown dispose branch callbacks and restore the default exactly once", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), registry: RpcAgentRegistry = new Map();
    let factory: any, component: AgentFooterComponent | undefined, disposed = 0, restores = 0;
    const ctx = { cwd: "/", sessionManager: { getEntries: () => [] }, ui: { setFooter(next: any) { component?.dispose?.(); component = undefined; factory = next; if (!next) restores++; } } };
    const first = createAgentFooterController(ctx, registry, storage, { truncateToWidth, visibleWidth });
    component = factory({ requestRender() {} }, { fg: (_c: string, s: string) => s }, { getGitBranch: () => null, getExtensionStatuses: () => new Map(), onBranchChange: () => () => { disposed++; } });
    const second = createAgentFooterController(ctx, registry, storage, { truncateToWidth, visibleWidth });
    assert.equal(disposed, 1);
    component = factory({ requestRender() {} }, { fg: (_c: string, s: string) => s }, { getGitBranch: () => null, getExtensionStatuses: () => new Map(), onBranchChange: () => () => { disposed++; } });
    first.stop(); second.stop();
    assert.equal(disposed, 2); assert.equal(restores, 2);
  });

  test("stale async starts cannot mount after replacement or stop", async () => {
    let release!: (value: any) => void;
    const pending = new Promise<any>(resolve => { release = resolve; });
    const calls: string[] = [];
    const lifecycle = createAgentFooterSessionLifecycle(() => pending, () => ({ refresh() {}, refreshAgents() {}, acceptUsage() {}, checkpoint: () => true, stop() { calls.push("stop-controller"); } }));
    const ctx = { mode: "tui", cwd: "/", sessionManager: { getEntries: () => [] }, ui: { setFooter() {} } };
    const storage = createAgentStorageContext("lead", root());
    const start = lifecycle.start(undefined, ctx, new Map(), storage);
    lifecycle.stop(); release({ truncateToWidth, visibleWidth }); await start;
    assert.deepEqual(calls, []);
  });
});

test("index session seams mount the re-enabled footer beside an existing widget and restore it on reload/shutdown", async () => {
  const storage = createAgentStorageContext("lead", root()), registry: RpcAgentRegistry = new Map();
  let factory: any, component: AgentFooterComponent | undefined, restores = 0;
  const widgets = new Map<string, unknown>();
  const ctx = {
    mode: "tui", cwd: "/", sessionManager: { getEntries: () => [], getSessionName: () => undefined, getCwd: () => "/" }, getContextUsage: () => ({ percent: 0, contextWindow: 1000 }),
    ui: { setFooter(next: any) { component?.dispose?.(); component = undefined; factory = next; if (!next) restores++; }, setWidget(key: string, value: unknown) { widgets.set(key, value); } },
  };
  ctx.ui.setWidget("ws-agents", ["agent card"]);
  const lifecycle = createAgentFooterSessionLifecycle(async () => ({ truncateToWidth, visibleWidth }));
  await applySessionStartAgentFooter(lifecycle, undefined, ctx, registry, storage);
  component = factory({ requestRender() {} }, { fg: (_c: string, text: string) => text }, { getGitBranch: () => null, getExtensionStatuses: () => new Map(), onBranchChange: () => () => {} });
  assert.ok(component.render(80)[1].includes("Lead ~$0.00 Direct agents ~$0.00"));
  await applySessionStartAgentFooter(lifecycle, undefined, ctx, registry, storage);
  assert.equal(restores, 1, "reload restores the prior footer before replacement");
  applySessionShutdownAgentFooter(lifecycle);
  assert.equal(restores, 2); assert.deepEqual(widgets.get("ws-agents"), ["agent card"]);
});

test("footer arming is limited to TUI lead/fork sessions", () => {
  assert.equal(shouldArmAgentFooter(undefined, "tui"), true); assert.equal(shouldArmAgentFooter("fork", "tui"), true);
  assert.equal(shouldArmAgentFooter("worker", "tui"), false); assert.equal(shouldArmAgentFooter("explore", "tui"), false); assert.equal(shouldArmAgentFooter(undefined, "rpc"), false);
});

test("cost formatter distinguishes complete, partial, unknown, and zero estimates", () => {
  assert.equal(formatCumulativeCost({ knownUsd: 1, knownContributors: 1, unknownContributors: 0, descendants: 1 }), "~$1.00");
  assert.equal(formatCumulativeCost({ knownUsd: 1, knownContributors: 1, unknownContributors: 1, descendants: 1 }), "~$1.00 + ?");
  assert.equal(formatCumulativeCost({ knownUsd: 0, knownContributors: 0, unknownContributors: 1, descendants: 1 }), "—");
  assert.equal(formatCumulativeCost({ knownUsd: 0, knownContributors: 0, unknownContributors: 0, descendants: 0 }), "~$0.00");
});

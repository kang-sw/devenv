import assert from "node:assert/strict";
import fs from "node:fs";
import fsPromises from "node:fs/promises";
import { syncBuiltinESMExports } from "node:module";
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
import { applySessionShutdownAgentFooter, applySessionStartAgentFooter, registerAgentFooterGitEvents } from "../src/index.ts";

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
  let restores = 0, renders = 0;
  const ctx = {
    mode: "tui", cwd: "/work/project",
    model: { provider: "provider", id: "model", reasoning: true, contextWindow: 200_000 }, thinkingLevel: "high",
    sessionManager: { getEntries: () => entries, getSessionName: () => "named session", getCwd: () => "/work/project" },
    getContextUsage: () => ({ tokens: 50_000, percent: 25, contextWindow: 200_000 }),
    ui: { setFooter(next: any) { component?.dispose?.(); component = undefined; footerFactory = next; if (!next) restores++; } },
  };
  const mount = (fg: (color: string, text: string) => string = (_color, text) => text) => {
    component = footerFactory(
      { requestRender() { renders++; } },
      { fg },
      { getGitBranch: () => "feature/footer", getExtensionStatuses: () => new Map(), getAvailableProviderCount: () => 2, onBranchChange: () => () => {} },
    );
    return component!;
  };
  return { ctx, mount, get restores() { return restores; }, get renders() { return renders; } };
}
function checkpoint(storage: ReturnType<typeof createAgentStorageContext>): any {
  return JSON.parse(readFileSync(join(storage.root, "ws-agents", storage.ownerSessionId, ".cost-estimate", "checkpoint.json"), "utf8"));
}
const plain = (text: string) => text.replace(/\u001b\[[0-9;]*m/g, "");

test("Git cache publishes beside branch, before session name, and drops indicators as one group", async t => {
  const ui = context(), storage = createAgentStorageContext("lead", root());
  const registry: RpcAgentRegistry = new Map();
  let queries = 0;
  const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth }, async () => {
    queries++;
    return { ahead: 1, behind: 2, added: 12, deleted: 3, changed: 2, untracked: 1, operation: "merging" };
  });
  const spans: Array<[string, string]> = [];
  const component = ui.mount((color, text) => { spans.push([color, text]); return `\u001b[36m${text}\u001b[39m`; });
  controller.turnEnd();
  for (let i = 0; i < 5; i++) await Promise.resolve();
  assert.equal(ui.renders, 1);
  const full = "/work/project (feature/footer) merging ↑1 ↓2 +12 -3 ~2 ?1 • named session";
  assert.equal(plain(component.render(full.length)[0]), full);
  for (const pair of [["warning", "merging"], ["success", "↑1"], ["warning", "↓2"], ["success", "+12"], ["error", "-3"], ["warning", "~2"], ["accent", "?1"]]) {
    assert.ok(spans.some(span => span[0] === pair[0] && span[1] === pair[1]));
  }
  assert.equal(plain(component.render(full.length - 1)[0]), "/work/project (feature/footer) • named session");
  ui.ctx.sessionManager.getEntries = () => { throw new Error("no render-time history traversal"); };
  // Arm guards only after the cache has published, so setup/teardown remain
  // allowed to persist checkpoints while every cached render is IO-free.
  for (const api of [fs, fsPromises]) {
    for (const name of Object.keys(api)) {
      if (/^(read|stat|lstat|fstat|open|access|exists|realpath|glob|watch)/.test(name) && typeof (api as any)[name] === "function") {
        t.mock.method(api as any, name, () => assert.fail(`render touched filesystem: ${name}`));
      }
    }
  }
  const traversalMethods = ["entries", "keys", "values", "forEach", Symbol.iterator] as const;
  for (const method of traversalMethods) {
    Object.defineProperty(registry, method, { configurable: true, value: () => assert.fail(`render traversed registry: ${String(method)}`) });
  }
  syncBuiltinESMExports();
  try {
    for (const width of [0, 1, 40, 80, 120]) {
      assert.ok(component.render(width).every(line => visibleWidth(line) <= width));
    }
    assert.equal(queries, 1, "rendering never queries Git");
  } finally {
    for (const method of traversalMethods) Reflect.deleteProperty(registry, method);
    t.mock.restoreAll(); syncBuiltinESMExports(); controller.stop();
  }
});

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
    assert.match(rendered, /D ~\$0\.65 \+ \?/);
    assert.doesNotMatch(rendered, /99/);
    controller.stop();
  });

  test("never decreases on missing, partial, unknown, or regressing cumulative snapshots", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), child = record("child", storage, 1);
    const registry: RpcAgentRegistry = new Map([[child.agentId, child]]), ui = context();
    const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth });
    const component = ui.mount();
    assert.match(component.render(100)[1], /D ~\$1\.00/);
    child.telemetry = undefined; controller.refreshAgents();
    assert.match(component.render(100)[1], /D ~\$1\.00 \+ \?/);
    child.telemetry = telemetry("child-session", child.sessionPath, "partial"); controller.refreshAgents();
    assert.match(component.render(100)[1], /D ~\$1\.00 \+ \?/);
    child.telemetry = telemetry("child-session", child.sessionPath, .5); controller.refreshAgents();
    assert.match(component.render(100)[1], /D ~\$1\.00 \+ \?/);
    child.telemetry = telemetry("child-session", child.sessionPath, 1.5); controller.refreshAgents();
    assert.match(component.render(100)[1], /D ~\$1\.50/);
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
    assert.match(component.render(100)[1], /D ~\$0\.80/);
    assert.equal(persistEvictedAgentCost(registry, old), true, "a retry on the same evicted record is idempotent");
    controller.stop();
    const saved = checkpoint(storage);
    assert.equal(saved.evictedBaseline.knownUsd, .6);
    assert.deepEqual(saved.agents.map((entry: any) => entry.agentId), ["kept"]);
    assert.ok(saved.agents.length <= registry.size, "checkpoint identity count is bounded by the registry");
    const restoredRegistry: RpcAgentRegistry = new Map([[kept.agentId, kept]]), restoredUi = context();
    const restored = createAgentFooterController(restoredUi.ctx, restoredRegistry, storage, { truncateToWidth, visibleWidth });
    assert.match(restoredUi.mount().render(100)[1], /D ~\$0\.80/, "reload adds the evicted baseline and restored live identity exactly once");
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
    assert.match(rendered, /↑10/); assert.match(rendered, /L ~\$0\.50/); assert.match(rendered, /D ~\$0\.40/);
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
    assert.match(component.render(100)[1], /D ~\$0\.40/, "failed retries leave the mounted state unfurled");
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
    assert.match(ui.mount().render(100)[1], /D ~\$0\.70/);
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
    assert.match(secondUi.mount().render(100)[1], /L ~\$0\.50/, "reload uses the retained in-process estimate instead of stale disk state");
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
    assert.match(rendered, /↑22/); assert.match(rendered, /↓4/); assert.match(rendered, /CH17\.6%/); assert.match(rendered, /L ~\$0\.80/);
    controller.stop();
  });

  test("a missing checkpoint with existing history starts unknown rather than scanning or inventing an exact lifetime value", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), ui = context([{ type: "message", message: { role: "assistant" } }]);
    const controller = createAgentFooterController(ui.ctx, new Map(), storage, { truncateToWidth, visibleWidth });
    assert.match(ui.mount().render(80)[1], /L —/);
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
    assert.match(wide, /L ~\$0\.00 \+ D ~\$2\.56/);
    (registry as any).entries = originalEntries; (registry as any)[Symbol.iterator] = originalIterator;
    controller.stop();
  });

  test("shows used over maximum context, emphasizing only usage and turning it red above 70 percent", () => {
    const dir = root(), storage = createAgentStorageContext("lead", dir), registry: RpcAgentRegistry = new Map();
    const ui = context();
    let tokens = 140_000, percent = 70;
    ui.ctx.getContextUsage = () => ({ tokens, percent, contextWindow: 200_000 });
    const controller = createAgentFooterController(ui.ctx, registry, storage, { truncateToWidth, visibleWidth });
    const component = ui.mount((color, text) => `<${color}>${text}</${color}>`);
    assert.match(component.render(120)[1], /<text>140k<\/text><dim>\/200k/);
    tokens = 142_000; percent = 71;
    const high = component.render(120)[1];
    assert.match(high, /<error>142k<\/error><dim>\/200k/);
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
    const lifecycle = createAgentFooterSessionLifecycle(() => pending, () => ({ turnEnd() {}, input() {}, refresh() {}, refreshAgents() {}, acceptUsage() {}, checkpoint: () => true, stop() { calls.push("stop-controller"); } }));
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
  assert.ok(component.render(80)[1].includes("L ~$0.00 + D ~$0.00"));
  await applySessionStartAgentFooter(lifecycle, undefined, ctx, registry, storage);
  assert.equal(restores, 1, "reload restores the prior footer before replacement");
  applySessionShutdownAgentFooter(lifecycle);
  assert.equal(restores, 2); assert.deepEqual(widgets.get("ws-agents"), ["agent card"]);
});

test("production turn/input hooks route each event without awaiting Git; reload and shutdown abort snapshots", async t => {
  t.mock.timers.enable({ apis: ["Date", "setTimeout"], now: 0 });
  const ui = context(), storage = createAgentStorageContext("lead", root());
  let requests = 0;
  const flights: Array<{ signal: AbortSignal; resolve: (value: undefined) => void }> = [];
  const lifecycle = createAgentFooterSessionLifecycle(async () => ({ truncateToWidth, visibleWidth }),
    (ctx, registry, storage, primitives) => createAgentFooterController(ctx, registry, storage, primitives, (_cwd, signal) => {
      requests++;
      return new Promise(resolve => flights.push({ signal, resolve }));
    }));
  const handlers = new Map<string, () => unknown>();
  registerAgentFooterGitEvents({ on(event: string, handler: () => unknown) { handlers.set(event, handler); } } as never, lifecycle);
  assert.deepEqual([...handlers.keys()], ["turn_end", "input"]);
  await lifecycle.start(undefined, ui.ctx, new Map(), storage);
  assert.equal(handlers.get("turn_end")!(), undefined); assert.equal(requests, 1);
  flights[0].resolve(undefined); for (let i = 0; i < 5; i++) await Promise.resolve();
  assert.equal(handlers.get("turn_end")!(), undefined); assert.equal(requests, 2);
  assert.equal(handlers.get("input")!(), undefined, "input never returns a Git promise");
  await lifecycle.start(undefined, ui.ctx, new Map(), storage);
  assert.equal(flights[1].signal.aborted, true, "reload aborts the old query");
  handlers.get("turn_end")!(); assert.equal(requests, 3);
  lifecycle.stop(); assert.equal(flights[2].signal.aborted, true);
  for (const flight of flights) flight.resolve(undefined);
  t.mock.timers.tick(600_000); handlers.get("turn_end")!(); assert.equal(requests, 3);
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

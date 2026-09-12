import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, test } from "node:test";
import { allocateAgentHome, createAgentStorageContext, updateOwnership } from "../src/agent-storage.ts";
import {
  aggregateDescendantCosts,
  createAgentFooterController,
  createAgentFooterSessionLifecycle,
  formatCumulativeCost,
  persistEvictedAgentCost,
  registerAgentCostOwner,
  shouldArmAgentFooter,
  summarizeLeadUsage,
  watchDescendantCosts,
  type AgentFooterComponent,
} from "../src/agent-footer.ts";
import { evictForCapacity, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";
import { truncateToWidth, visibleWidth } from "../src/pi-tui.ts";

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
function record(agentId: string, ownership: ReturnType<typeof allocateAgentHome>, cost: number | "partial" | "unknown"): RpcAgentRecord {
  return {
    agentId,
    sessionPath: ownership.sessionPath!,
    ownership,
    telemetry: telemetry(`${agentId}-session`, ownership.sessionPath!, cost),
    wsToolNames: [], toolGroup: "full-worker", streaming: false, running: false, reportLog: [],
  } as RpcAgentRecord;
}
function persist(record: RpcAgentRecord): void {
  assert.ok(record.ownership);
  updateOwnership(record.ownership.home, { telemetry: record.telemetry });
  writeFileSync(record.sessionPath, `${JSON.stringify({ type: "session", version: 3, id: record.telemetry!.origin.sessionId, timestamp: "x", cwd: "/" })}\n`);
}

describe("descendant cost aggregation", () => {
  test("counts direct roles, nested and dormant descendants once while excluding another lead", () => {
    const dir = root();
    const lead = createAgentStorageContext("lead-session", dir);
    const direct = [
      record("worker", allocateAgentHome(lead, "worker", "worker"), 0.4),
      record("execute", allocateAgentHome(lead, "execute", "execute-worker"), 0),
      record("fork", allocateAgentHome(lead, "fork", "fork"), "unknown"),
      record("explore", allocateAgentHome(lead, "explore", "explore", "simple"), 0.1),
    ];
    for (const item of direct) persist(item);
    const nestedOwner = createAgentStorageContext("worker-session", dir);
    const nested = record("nested", allocateAgentHome(nestedOwner, "nested", "worker"), "partial");
    persist(nested);
    const unrelatedOwner = createAgentStorageContext("other-lead", dir);
    const unrelated = record("unrelated", allocateAgentHome(unrelatedOwner, "unrelated", "worker"), 99);
    persist(unrelated);

    const registry: RpcAgentRegistry = new Map(direct.map(item => [item.agentId, item]));
    const cost = aggregateDescendantCosts(lead, registry);
    assert.equal(cost.knownUsd, 0.75);
    assert.equal(cost.knownContributors, 4, "known zero and the known part of a partial estimate remain known");
    assert.equal(cost.unknownContributors, 2, "one wholly unknown direct child plus one partial descendant");
    assert.equal(cost.descendants, 5, "direct worker/execute/fork/explore plus nested worker, not the unrelated lead");
    assert.equal(formatCumulativeCost(cost), "~$0.75 + ?");
  });

  test("a watcher armed before ws-agents exists observes the first descendant and later telemetry writes", async () => {
    const dir = root();
    const storage = createAgentStorageContext("lead-session", dir);
    const registry: RpcAgentRegistry = new Map();
    const changed = new Promise<void>((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error("descendant watcher did not refresh")), 1_000);
      const stop = watchDescendantCosts(storage, registry, () => { clearTimeout(timeout); stop(); resolve(); });
      const child = record("first", allocateAgentHome(storage, "first", "worker"), .2);
      persist(child);
    });
    await changed;
  });

  test("a storage-bound registry can still evict a legacy unowned record into a fresh durable namespace", () => {
    const dir = root();
    const storage = createAgentStorageContext("lead-session", dir);
    const legacy = { agentId: "legacy", sessionPath: join(dir, "legacy.jsonl"), wsToolNames: [], toolGroup: "full-worker", streaming: false, running: false, reportLog: [] } as RpcAgentRecord;
    const registry: RpcAgentRegistry = new Map([[legacy.agentId, legacy]]);
    registerAgentCostOwner(registry, storage);
    assert.deepEqual(evictForCapacity(registry, 1), { ok: true, evictedLabel: "legacy" });
    assert.equal(formatCumulativeCost(aggregateDescendantCosts(storage, registry)), "—");
  });

  test("capacity eviction rolls the whole child subtree up durably without reload duplication or decrease", () => {
    const dir = root();
    const lead = createAgentStorageContext("lead-session", dir);
    const parent = record("parent", allocateAgentHome(lead, "parent", "worker"), 0.5); persist(parent);
    const childOwner = createAgentStorageContext("parent-session", dir);
    const nested = record("nested", allocateAgentHome(childOwner, "nested", "explore", "deep"), 0.3); persist(nested);
    const registry: RpcAgentRegistry = new Map([[parent.agentId, parent]]);

    updateOwnership(parent.ownership!.home, { liveness: { lifecycle: "stopped", running: false } });
    assert.deepEqual(evictForCapacity(registry, 1), { ok: true, evictedLabel: "parent" }, "the production capacity path writes the roll-up before eviction");
    const after = aggregateDescendantCosts(lead, registry);
    assert.deepEqual(after, { knownUsd: 0.8, knownContributors: 2, unknownContributors: 0, descendants: 2 });
    assert.equal(persistEvictedAgentCost(registry, parent), true, "replaying the same eviction upserts by identity");
    assert.deepEqual(aggregateDescendantCosts(lead, registry), after, "reload/retry does not add the child twice");
    assert.match(readFileSync(join(dir, "ws-agents", "lead-session", ".cost-rollup", "parent.json"), "utf8"), /"parent"/);
  });
});

describe("lead usage and cost forms", () => {
  const assistant = (id: string, cost: unknown, input = 10) => ({ type: "message", id, message: { role: "assistant", usage: { input, output: 2, cacheRead: 3, cacheWrite: 4, cost: { total: cost } } } });
  test("formats fully known, partial, wholly unknown, and known zero distinctly", () => {
    assert.equal(formatCumulativeCost(summarizeLeadUsage([assistant("a", 1.42)]).cost), "~$1.42");
    assert.equal(formatCumulativeCost(summarizeLeadUsage([assistant("a", 1), assistant("b", undefined)]).cost), "~$1.00 + ?");
    assert.equal(formatCumulativeCost(summarizeLeadUsage([assistant("a", undefined)]).cost), "—");
    assert.equal(formatCumulativeCost(summarizeLeadUsage([assistant("a", 0)]).cost), "~$0.00");
    assert.equal(formatCumulativeCost(summarizeLeadUsage([]).cost), "~$0.00", "no calls or descendants is known zero, not unknown");
  });

  test("cache hit rate follows the latest assistant call, not later tool or compaction usage", () => {
    const assistantCall = assistant("a", .1, 10);
    const tool = { type: "message", message: { role: "toolResult", usage: { input: 100, cacheRead: 0, cacheWrite: 0, cost: { total: .1 } } } };
    const compaction = { type: "compaction", usage: { input: 100, cacheRead: 0, cacheWrite: 0, cost: { total: .1 } } };
    assert.equal(summarizeLeadUsage([assistantCall, tool, compaction]).latestCacheHitRate, 3 / 17 * 100);
  });
});

describe("custom footer controller", () => {
  test("preserves built-in information, accents only known monetary values, reacts, bounds widths, and restores default", () => {
    const dir = root();
    const storage = createAgentStorageContext("lead", dir);
    const child = record("child", allocateAgentHome(storage, "child", "worker"), "partial"); persist(child);
    const registry: RpcAgentRegistry = new Map([[child.agentId, child]]);
    const assistant = { type: "message", id: "m", message: { role: "assistant", usage: { input: 1200, output: 50, cacheRead: 20, cacheWrite: 0, cost: { total: 0.5 } } } };
    const unknown = { type: "message", id: "u", message: { role: "assistant", usage: { input: 1, output: 0, cacheRead: 0, cacheWrite: 0, cost: {} } } };
    let footerFactory: any;
    let current: AgentFooterComponent | undefined;
    let defaultRestores = 0;
    let widgetCalls = 0;
    let watched: (() => void) | undefined;
    const context = {
      cwd: "/work/project",
      model: { provider: "provider", id: "model", reasoning: true, contextWindow: 200_000 },
      thinkingLevel: "high",
      sessionManager: { getEntries: () => [assistant, unknown], getSessionName: () => "named session" },
      getContextUsage: () => ({ percent: 25, contextWindow: 200_000 }),
      ui: {
        setFooter(factory: typeof footerFactory) { footerFactory = factory; if (!factory) { current?.dispose?.(); current = undefined; defaultRestores++; } },
        setWidget() { widgetCalls++; },
      },
    };
    const controller = createAgentFooterController(context, registry, storage, { truncateToWidth, visibleWidth }, {
      watchCosts: (_storage, _registry, onChange) => { watched = onChange; return () => { watched = undefined; }; },
    });
    let renders = 0;
    let branchListener: (() => void) | undefined;
    const footerData = {
      getGitBranch: () => "feature/footer",
      getExtensionStatuses: () => new Map([["goal", "Goal loop: settling"], ["other", "Other\nstatus"]]),
      getAvailableProviderCount: () => 2,
      onBranchChange(callback: () => void) { branchListener = callback; return () => { branchListener = undefined; }; },
    };
    const semantic: Array<[string, string]> = [];
    let palette: "dark" | "light" = "dark";
    const theme = { fg(color: string, text: string) { semantic.push([color, text]); const code = palette === "dark" ? 36 : 35; return `\u001b[${code}m${text}\u001b[0m`; } };
    const plain = (text: string) => text.replace(/\u001b\[[0-9;]*m/g, "");
    current = footerFactory({ requestRender() { renders++; } }, theme, footerData);
    for (const width of [120, 80, 40]) {
      const lines = current.render(width);
      assert.ok(lines.every(line => visibleWidth(line) <= width), `ANSI-styled lines fit ${width}`);
    }
    const dark = current.render(120).join("\n");
    palette = "light";
    current.invalidate();
    const light = current.render(120).join("\n");
    assert.notEqual(dark, light, "theme invalidation rerenders through the current light/dark semantic palette");
    semantic.length = 0;
    const wide = plain(current.render(120).join("\n"));
    assert.match(wide, /\/work\/project \(feature\/footer\) • named session/);
    assert.match(wide, /↑1\.2k/); assert.match(wide, /25\.0%\/200k/); assert.match(wide, /provider.*model.*high/);
    assert.match(wide, /Lead ~\$0\.50 \+ \?/); assert.match(wide, /Subagents ~\$0\.25 \+ \?/);
    assert.match(wide, /Goal loop: settling Other status/);
    assert.deepEqual(semantic.filter(([color]) => color === "accent").map(([, text]) => text), ["~$0.50", "~$0.25"], "only known monetary substrings receive accent, not partial-estimate markers");
    assert.equal(widgetCalls, 0, "the belowEditor agent widget is not replaced or changed");
    branchListener?.(); watched?.(); controller.refresh();
    assert.equal(renders, 3, "git, descendant telemetry, and direct telemetry refreshes request rendering");
    controller.stop();
    assert.equal(defaultRestores, 1); assert.equal(branchListener, undefined); assert.equal(watched, undefined);
  });

  test("a replacement controller disposes the previous footer and uses the newly injected theme", () => {
    const dir = root(); const storage = createAgentStorageContext("lead", dir); const registry: RpcAgentRegistry = new Map();
    let component: AgentFooterComponent | undefined; let disposed = 0; let factory: any;
    const ctx = { cwd: "/", sessionManager: { getEntries: () => [] }, ui: { setFooter(next: any) { component?.dispose?.(); if (!next) { component = undefined; return; } factory = next; } } };
    const options = { watchCosts: () => () => { disposed++; } };
    const first = createAgentFooterController(ctx, registry, storage, { truncateToWidth, visibleWidth }, options);
    component = factory({ requestRender() {} }, { fg: (_c: string, s: string) => `old:${s}` }, { getGitBranch: () => null, getExtensionStatuses: () => new Map(), getAvailableProviderCount: () => 1, onBranchChange: () => () => { disposed++; } });
    const second = createAgentFooterController(ctx, registry, storage, { truncateToWidth, visibleWidth }, options);
    assert.equal(disposed, 2, "Pi last-writer replacement disposes both subscriptions owned by the old component");
    component = factory({ requestRender() {} }, { fg: (_c: string, s: string) => `new:${s}` }, { getGitBranch: () => null, getExtensionStatuses: () => new Map(), getAvailableProviderCount: () => 1, onBranchChange: () => () => {} });
    assert.ok(component.render(80).some(line => line.includes("new:")), "rendering uses the host theme injected for the replacement");
    first.stop(); second.stop();
  });
});

test("production session lifecycle replaces on reload, disarms on mode change, refreshes, and restores on shutdown", async () => {
  const storage = createAgentStorageContext("lead", root());
  const registry: RpcAgentRegistry = new Map();
  const calls: string[] = [];
  let serial = 0;
  const lifecycle = createAgentFooterSessionLifecycle(
    async () => ({ truncateToWidth, visibleWidth }),
    () => {
      const id = ++serial;
      calls.push(`start:${id}`);
      return { refresh: () => calls.push(`refresh:${id}`), stop: () => calls.push(`stop:${id}`) };
    },
  );
  const ctx = { mode: "tui", cwd: "/", sessionManager: { getEntries: () => [] }, ui: { setFooter() {} } };
  await lifecycle.start(undefined, ctx, registry, storage);
  lifecycle.refresh();
  await lifecycle.start("fork", ctx, registry, storage);
  await lifecycle.start(undefined, { ...ctx, mode: "rpc" }, registry, storage);
  lifecycle.stop();
  assert.deepEqual(calls, ["start:1", "refresh:1", "stop:1", "start:2", "stop:2"]);
});

test("footer arming is limited to TUI lead/fork sessions", () => {
  assert.equal(shouldArmAgentFooter(undefined, "tui"), true);
  assert.equal(shouldArmAgentFooter("fork", "tui"), true);
  assert.equal(shouldArmAgentFooter("worker", "tui"), false);
  assert.equal(shouldArmAgentFooter("explore", "tui"), false);
  assert.equal(shouldArmAgentFooter(undefined, "rpc"), false);
});

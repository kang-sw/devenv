import { afterEach, describe, test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, readdirSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createAgentStorageContext } from "../src/agent-storage.ts";
import { RpcClient, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import {
  registerAgentTools as registerAgentToolsBase,
  resolveTools,
  sendToAgent,
  type RpcAgentRecord,
  type RpcAgentRegistry,
} from "../src/spawner.ts";
import { WS_PI_EXPLORE_MODE_ENV, WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";
import { captureOrphans, parseOrphans, reviveOrphans, serializeOrphans } from "../src/agent-sidecar.ts";
import type { McpToolCallResult } from "../src/mcp-stdio-client.ts";

const TEST_EXTENSION_ENTRY = "/tmp/loaded ws adapter/index copy.ts";
function registerAgentTools(pi: any, bridge: any, sessionCtx: any, ...rest: any[]) {
  return registerAgentToolsBase(pi, bridge, { ...sessionCtx, extensionPath: sessionCtx.extensionPath ?? TEST_EXTENSION_ENTRY }, ...rest);
}

const storageRoots = new Set<string>();
afterEach(() => {
  for (const root of storageRoots) rmSync(root, { recursive: true, force: true });
  storageRoots.clear();
});

interface CapturedTool {
  name: string;
  description: string;
  parameters: { properties?: Record<string, unknown> };
  execute: (id: string, params: unknown, signal?: AbortSignal, update?: unknown, ctx?: unknown) => Promise<{ content: Array<{ text: string }> }>;
}

function record(overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return {
    agentId: "retained", alias: "explore-1", sessionPath: "/tmp/research.jsonl", systemPromptPath: "/tmp/research.md",
    wsToolNames: [], toolGroup: "read-only", spawnRole: "explore", exploreMode: "simple", modelBase: "pi/small", modelEffort: "medium",
    streaming: false, running: false, reportLog: [], ...overrides,
  };
}

async function withRole<T>(role: string | undefined, mode: string | undefined, fn: () => Promise<T> | T): Promise<T> {
  const oldRole = process.env[WS_PI_SPAWN_ROLE_ENV];
  const oldMode = process.env[WS_PI_EXPLORE_MODE_ENV];
  if (role === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV]; else process.env[WS_PI_SPAWN_ROLE_ENV] = role;
  if (mode === undefined) delete process.env[WS_PI_EXPLORE_MODE_ENV]; else process.env[WS_PI_EXPLORE_MODE_ENV] = mode;
  try { return await fn(); } finally {
    if (oldRole === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV]; else process.env[WS_PI_SPAWN_ROLE_ENV] = oldRole;
    if (oldMode === undefined) delete process.env[WS_PI_EXPLORE_MODE_ENV]; else process.env[WS_PI_EXPLORE_MODE_ENV] = oldMode;
  }
}

function modelResult(model = "pi/small", effort?: string, resolvedFrom = "pi"): McpToolCallResult {
  return { isError: false, content: [{ type: "text", text: JSON.stringify({ model, ...(effort === undefined ? {} : { effort }), resolved_from: resolvedFrom }) }] };
}

function installRpcHarness(state: { model: string; thinking: string; clamp?: string }) {
  const original = Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"].map(name => [name, RpcClient.prototype[name as keyof RpcClient]]));
  const calls: string[] = [];
  Object.assign(RpcClient.prototype, {
    start: async function(this: { options?: { args?: string[] } }) {
      calls.push("start");
      const args = this.options?.args ?? [];
      const sessionIndex = args.indexOf("--session");
      const sessionDirIndex = args.indexOf("--session-dir");
      const session = sessionIndex >= 0 ? args[sessionIndex + 1] : undefined;
      const sessionDir = sessionDirIndex >= 0 ? args[sessionDirIndex + 1] : undefined;
      if (session) writeFileSync(session, "mock session\n");
      else if (sessionDir) writeFileSync(join(sessionDir, "session.jsonl"), "mock session\n");
    }, stop: async () => { calls.push("stop"); }, abort: async () => { calls.push("abort"); },
    onEvent: () => () => {}, prompt: async (message: string) => { calls.push(`prompt:${message}`); },
    setThinkingLevel: async (level: string) => { calls.push(`thinking:${level}`); state.thinking = state.clamp ?? level; },
    getState: async () => {
      const [provider, id] = state.model.split("/");
      return { model: { provider, id }, thinkingLevel: state.thinking, sessionFile: "/tmp/research.jsonl" };
    },
  });
  return { calls, restore: () => Object.assign(RpcClient.prototype, original) };
}

function registerHarness(options: {
  result?: McpToolCallResult;
  auth?: boolean;
  leaf?: (ctx: unknown, opts: unknown) => Promise<unknown>;
  throwLookup?: boolean;
} = {}) {
  const tools = new Map<string, CapturedTool>();
  let lookups = 0;
  const pi = { registerTool: (tool: CapturedTool) => tools.set(tool.name, tool), sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
  const bridge = {
    client: { callTool: async (name: string): Promise<McpToolCallResult> => {
      assert.equal(name, "config.resolve_agent");
      lookups += 1;
      if (options.throwLookup) throw new Error("transport down");
      return options.result ?? modelResult("pi/small");
    } },
    wsToolNames: [], defaultSessionKeyRef: { current: "lead-key" },
  } as never;
  const leafCalls: Array<{ ctx: unknown; opts: unknown }> = [];
  const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
  storageRoots.add(root);
  const handle = registerAgentTools(pi, bridge, { cwd: "/tmp", storage: createAgentStorageContext("test-lead", root) }, undefined, async (_client, _registry, ctx, _params, opts) => {
    leafCalls.push({ ctx, opts });
    return options.leaf?.(ctx, opts) ?? { agentId: "leaf", state: "done", output: "evidence" };
  });
  return { tools, handle, root, lookups: () => lookups, leafCalls };
}

function activeDynamicTools(tools: Map<string, CapturedTool>, allowlist: string): string[] {
  const allowed = new Set(allowlist.split(","));
  return [...tools.keys()].filter(name => allowed.has(name));
}

// These execute real registered tool wrappers and real spawn/resume code. Only
// RpcClient's process transport is replaced, so no provider call is possible.
describe("persistent explore registration, dispatch, and frozen selection", () => {
  test("role/mode registration and Pi-style dynamic allowlisting permit exactly the intended explore surface", async () => {
    await withRole(undefined, undefined, () => {
      const h = registerHarness();
      const tool = h.tools.get("explore")!;
      assert.deepEqual(Object.keys(tool.parameters.properties ?? {}).sort(), ["deep_research", "query"]);
      assert.deepEqual(activeDynamicTools(h.tools, "read,grep,find,ls"), [], "simple child cannot reach dynamically registered explore");
      return h.handle.stopAll();
    });
    await withRole("fork", undefined, () => {
      const h = registerHarness();
      assert.ok(h.tools.has("explore"));
      return h.handle.stopAll();
    });
    await withRole("worker", undefined, () => {
      const h = registerHarness();
      const tool = h.tools.get("explore")!;
      assert.deepEqual(Object.keys(tool.parameters.properties ?? {}), ["query"]);
      assert.deepEqual(activeDynamicTools(h.tools, resolveTools("full-worker")), ["ws-report-to-lead", "explore"], "the actual worker allowlist admits its registered leaf alongside its established report tool");
      return h.handle.stopAll();
    });
    await withRole("explore", "deep", () => {
      const h = registerHarness();
      assert.deepEqual(activeDynamicTools(h.tools, resolveTools("read-only-explore")), ["explore"]);
      return h.handle.stopAll();
    });
    for (const mode of ["simple", undefined, "bad"]) {
      await withRole("explore", mode, () => {
        const h = registerHarness();
        assert.equal(h.tools.has("explore"), false, `explore role/${mode ?? "missing"} must not gain collection`);
        return h.handle.stopAll();
      });
    }
  });

  test("lead simple dispatch returns only id/alias and freezes Pi's actual default and clamped effort across resume", async () => {
    const rpc = installRpcHarness({ model: "pi/small", thinking: "medium", clamp: "high" });
    try {
      await withRole(undefined, undefined, async () => {
        const h = registerHarness({ result: modelResult("pi/small", "xhigh") });
        const tool = h.tools.get("explore")!;
        const toolCtx = { model: { provider: "lead", id: "large" }, thinkingLevel: "low", modelRegistry: { getAll: () => [{ provider: "pi", id: "small" }], hasConfiguredAuth: () => true } };
        const result = JSON.parse((await tool.execute("call", { query: "find the contract" }, undefined, undefined, toolCtx)).content[0]!.text);
        assert.deepEqual(Object.keys(result).sort(), ["agent_id", "alias"]);
        const researcher = h.handle.rpcRegistry.get(result.agent_id)!;
        assert.equal(researcher.ownership?.home, join(realpathSync(h.root), "ws-agents", "test-lead", result.agent_id));
        assert.equal(researcher.ownership?.sessionPath, researcher.sessionPath);
        assert.equal(researcher.toolGroup, "read-only");
        assert.equal(researcher.modelBase, "pi/small");
        assert.equal(researcher.modelEffort, "high", "Pi's actual clamp replaces requested xhigh before persistence");
        assert.equal(h.lookups(), 1);
        const restored: RpcAgentRegistry = new Map();
        reviveOrphans(restored, parseOrphans(serializeOrphans(captureOrphans(h.handle.rpcRegistry))));
        const revived = restored.get(result.agent_id)!;
        assert.deepEqual([revived.spawnRole, revived.exploreMode, revived.toolGroup, revived.modelBase, revived.modelEffort], ["explore", "simple", "read-only", "pi/small", "high"]);
        rpc.calls.length = 0;
        await sendToAgent(restored, { cwd: "/tmp", extensionPath: TEST_EXTENSION_ENTRY }, revived.agentId, "follow up");
        assert.deepEqual(rpc.calls.filter(c => c.startsWith("thinking:")), ["thinking:high"], "sidecar-restored resume uses the frozen effective effort");
        revived.client = undefined;
        const restoredTwice: RpcAgentRegistry = new Map();
        reviveOrphans(restoredTwice, parseOrphans(serializeOrphans(captureOrphans(restored))));
        rpc.calls.length = 0;
        await sendToAgent(restoredTwice, { cwd: "/tmp", extensionPath: TEST_EXTENSION_ENTRY }, revived.agentId, "second resume");
        assert.deepEqual(rpc.calls.filter(c => c.startsWith("thinking:")), ["thinking:high"], "second sidecar cycle keeps the actual effective effort");
        assert.equal(h.lookups(), 1, "resume never re-resolves small");
        await h.handle.stopAll();
      });
    } finally { rpc.restore(); }
  });

  test("simple tier with no effort captures Pi's real startup default before its first prompt", async () => {
    const rpc = installRpcHarness({ model: "pi/small", thinking: "minimal" });
    try {
      await withRole(undefined, undefined, async () => {
        const h = registerHarness({ result: modelResult("pi/small") });
        const ctx = { model: { provider: "lead", id: "large" }, thinkingLevel: "high", modelRegistry: { getAll: () => [{ provider: "pi", id: "small" }], hasConfiguredAuth: () => true } };
        const result = JSON.parse((await h.tools.get("explore")!.execute("call", { query: "default effort" }, undefined, undefined, ctx)).content[0]!.text);
        assert.equal(h.handle.rpcRegistry.get(result.agent_id)?.modelEffort, "minimal");
        assert.deepEqual(rpc.calls.filter(c => c.startsWith("thinking:")), [], "unset small effort does not invent off or a request");
        await h.handle.stopAll();
      });
    } finally { rpc.restore(); }
  });

  test("deep creation never looks up small, freezes the dispatch snapshot, and refuses an effective-effort mismatch before prompt", async () => {
    const rpc = installRpcHarness({ model: "lead/current", thinking: "medium", clamp: "medium" });
    try {
      await withRole(undefined, undefined, async () => {
        const h = registerHarness({ result: { isError: true, content: [] } });
        const tool = h.tools.get("explore")!;
        const ctx = { model: { provider: "lead", id: "current" }, thinkingLevel: "high", modelRegistry: { getAll: () => { throw new Error("deep must not read catalog"); }, hasConfiguredAuth: () => false } };
        await assert.rejects(() => tool.execute("call", { query: "deep question", deep_research: true }, undefined, undefined, ctx), /research thinking mismatch/);
        assert.equal(h.lookups(), 0, "deep parent creation does not resolve small");
        assert.deepEqual(rpc.calls.filter(c => c.startsWith("prompt:")), [], "mismatched effective deep effort never prompts");
        assert.equal(h.handle.rpcRegistry.size, 1, "ordinary persistent failure identity remains retained");
        await h.handle.stopAll();
      });
    } finally { rpc.restore(); }
  });

  test("bad small refuses before registry guard/allocation effects and deep collection refuses before its leaf", async () => {
    const before = readdirSync(tmpdir()).filter(name => name.startsWith("ws-pi-agent-")).length;
    await withRole(undefined, undefined, async () => {
      const h = registerHarness({ result: { isError: true, content: [] } });
      h.handle.rpcRegistry.set("holder", record({ agentId: "holder", alias: "explore-1", running: false }));
      const tool = h.tools.get("explore")!;
      const ctx = { model: { provider: "lead", id: "large" }, thinkingLevel: "high", modelRegistry: { getAll: () => [{ provider: "pi", id: "small" }], hasConfiguredAuth: () => true } };
      await assert.rejects(() => tool.execute("call", { query: "simple" }, undefined, undefined, ctx), /explore refused/);
      assert.equal(h.handle.rpcRegistry.size, 1);
      assert.equal(h.handle.rpcRegistry.get("holder")?.alias, "explore-1", "refusal cannot transfer an alias or evict a holder");
      assert.equal(h.lookups(), 1, "one shared resolution, not a preflight plus re-resolution");
      await h.handle.stopAll();
    });
    assert.equal(readdirSync(tmpdir()).filter(name => name.startsWith("ws-pi-agent-")).length, before, "refusal allocated no session directory");

    await withRole("explore", "deep", async () => {
      const h = registerHarness({ result: { isError: true, content: [] } });
      const tool = h.tools.get("explore")!;
      const ctx = { model: { provider: "lead", id: "large" }, thinkingLevel: "high", modelRegistry: { getAll: () => [{ provider: "pi", id: "small" }], hasConfiguredAuth: () => true } };
      await assert.rejects(() => tool.execute("call", { query: "collect" }, undefined, undefined, ctx), /explore refused/);
      assert.equal(h.leafCalls.length, 0, "collection refusal happens before render/leaf launch");
      await h.handle.stopAll();
    });
  });

  test("every simple-resolution failure rejects before records, alias transfers, process starts, or session allocation", async () => {
    const cases: Array<{ name: string; result?: McpToolCallResult; throwLookup?: boolean; models?: unknown; auth?: boolean; throwCatalog?: boolean }> = [
      { name: "transport error", throwLookup: true },
      { name: "MCP error", result: { isError: true, content: [] } },
      { name: "missing text", result: { isError: false, content: [] } },
      { name: "invalid JSON", result: { isError: false, content: [{ type: "text", text: "not-json" }] } },
      { name: "non-Pi resolution", result: modelResult("other/model", undefined, "codex") },
      { name: "unknown model", result: modelResult("pi/missing") },
      { name: "unauthenticated model", result: modelResult("pi/small"), auth: false },
      { name: "empty catalog", result: modelResult("pi/small"), models: [] },
      { name: "malformed catalog", result: modelResult("pi/small"), models: {} },
      { name: "throwing catalog", result: modelResult("pi/small"), throwCatalog: true },
    ];
    for (const failure of cases) {
      await withRole(undefined, undefined, async () => {
        const h = registerHarness({ result: failure.result, throwLookup: failure.throwLookup });
        h.handle.rpcRegistry.set("holder", record({ agentId: "holder", alias: "explore-1", running: false }));
        const ctx = {
          model: { provider: "lead", id: "large" }, thinkingLevel: "high",
          modelRegistry: { getAll: () => { if (failure.throwCatalog) throw new Error("catalog down"); return failure.models ?? [{ provider: "pi", id: "small" }]; }, hasConfiguredAuth: () => failure.auth ?? true },
        };
        await assert.rejects(() => h.tools.get("explore")!.execute("call", { query: failure.name }, undefined, undefined, ctx), /explore refused/, failure.name);
        assert.equal(h.handle.rpcRegistry.size, 1, `${failure.name}: no registry insertion/eviction`);
        assert.equal(h.handle.rpcRegistry.get("holder")?.alias, "explore-1", `${failure.name}: no alias transfer`);
        assert.equal(h.lookups(), 1, `${failure.name}: exactly one resolution`);
        await h.handle.stopAll();
      });
    }
  });

  test("worker keeps recon while a deep researcher collection is read-only and receives resolved effort", async () => {
    const context = { model: { provider: "lead", id: "large" }, thinkingLevel: "high", modelRegistry: { getAll: () => [{ provider: "pi", id: "small" }], hasConfiguredAuth: () => true } };
    await withRole("worker", undefined, async () => {
      const h = registerHarness({ result: modelResult("pi/small", "low") });
      await h.tools.get("explore")!.execute("call", { query: "worker evidence" }, undefined, undefined, context);
      assert.deepEqual(h.leafCalls[0]?.opts, { profile: "recon", effort: "low" });
      await h.handle.stopAll();
    });
    await withRole("explore", "deep", async () => {
      const h = registerHarness({ result: modelResult("pi/small", "low") });
      await h.tools.get("explore")!.execute("call", { query: "deep evidence" }, undefined, undefined, context);
      assert.deepEqual(h.leafCalls[0]?.opts, { profile: "read-only", effort: "low" });
      await h.handle.stopAll();
    });
  });
});

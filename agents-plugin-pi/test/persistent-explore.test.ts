import { afterEach, describe, test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, mkdtempSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { RpcClient, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { CHILD_MANAGEMENT_TOOLS, DELEGATION_ENV, terminalTools } from "../src/delegation-policy.ts";
import { createAgentStorageContext } from "../src/agent-storage.ts";
import { captureOrphans, parseOrphans, reviveOrphans, serializeOrphans } from "../src/agent-sidecar.ts";
import { WS_PI_EXPLORE_MODE_ENV, WS_PI_SPAWN_ROLE_ENV, type ExploreMode } from "../src/process-role.ts";
import { WEB_HOME_ENV, WEB_NONCE_ENV } from "../src/web-readiness.ts";
import { registerAgentTools, resolveTools, sendToAgent, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";
import type { McpToolCallResult } from "../src/mcp-stdio-client.ts";

const PACKAGE_ROOT = dirname(dirname(fileURLToPath(import.meta.url)));
const EXTENSION_ENTRY = join(PACKAGE_ROOT, "src", "index.ts");
const EXPLORE_GUIDE = join(PACKAGE_ROOT, "explore-guide.md");
const MODE_CONTRACT = [
  ["lookup", "small"],
  ["code-search", "small"],
  ["history-search", "small"],
  ["docs-search", "medium"],
  ["web-search", "medium"],
  ["diagnosis", "medium"],
  ["comparison", "medium"],
  ["synthesis", "large"],
] as const satisfies ReadonlyArray<readonly [ExploreMode, "small" | "medium" | "large"]>;
const MODES = MODE_CONTRACT.map(([mode]) => mode);
const TIER_BY_MODE = Object.fromEntries(MODE_CONTRACT) as Record<ExploreMode, "small" | "medium" | "large">;
const storageRoots = new Set<string>();

afterEach(() => {
  for (const root of storageRoots) rmSync(root, { recursive: true, force: true });
  storageRoots.clear();
});

interface CapturedTool {
  name: string;
  description: string;
  parameters: { properties?: Record<string, { enum?: string[]; description?: string }>; additionalProperties?: boolean };
  execute: (id: string, params: unknown, signal?: AbortSignal, update?: (partial: unknown) => void, ctx?: unknown) => Promise<{ content: Array<{ text: string }>; details?: unknown }>;
}

function withRole<T>(role: string | undefined, mode: string | undefined, policy: Record<string, unknown> | undefined, fn: () => T | Promise<T>): Promise<T> {
  const previous = { role: process.env[WS_PI_SPAWN_ROLE_ENV], mode: process.env[WS_PI_EXPLORE_MODE_ENV], policy: process.env[DELEGATION_ENV] };
  if (role === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV]; else process.env[WS_PI_SPAWN_ROLE_ENV] = role;
  if (mode === undefined) delete process.env[WS_PI_EXPLORE_MODE_ENV]; else process.env[WS_PI_EXPLORE_MODE_ENV] = mode;
  if (policy === undefined) delete process.env[DELEGATION_ENV]; else process.env[DELEGATION_ENV] = JSON.stringify(policy);
  return Promise.resolve(fn()).finally(() => {
    for (const [key, value] of [[WS_PI_SPAWN_ROLE_ENV, previous.role], [WS_PI_EXPLORE_MODE_ENV, previous.mode], [DELEGATION_ENV, previous.policy]] as const) {
      if (value === undefined) delete process.env[key]; else process.env[key] = value;
    }
  });
}

function parentPolicy(depth = 0, maxDepth = 2) {
  return {
    version: 1, depth, maxDepth, authority: "lead",
    tools: resolveTools("full-worker").split(","),
    network: { search: true, fetch: true },
  };
}

function installRpcHarness() {
  const original = Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"].map(name => [name, RpcClient.prototype[name as keyof RpcClient]]));
  const prompts: Array<{ client: unknown; message: string }> = [];
  const clients: Array<{ options?: { args?: string[]; env?: Record<string, string> } }> = [];
  const effort = new WeakMap<object, string>();
  Object.assign(RpcClient.prototype, {
    start: async function(this: { options?: { args?: string[]; env?: Record<string, string> } }) {
      clients.push(this);
      const args = this.options?.args ?? [];
      const sessionIndex = args.indexOf("--session");
      if (sessionIndex >= 0) writeFileSync(args[sessionIndex + 1]!, "mock session\n");
      const env = this.options?.env ?? {};
      if (env[WEB_HOME_ENV]) writeFileSync(join(env[WEB_HOME_ENV], "web-tools-ready.json"), JSON.stringify({ nonce: env[WEB_NONCE_ENV], tools: ["web_search", "ws_web_fetch"] }));
    },
    stop: async () => {},
    abort: async () => {},
    onEvent: () => () => {},
    prompt: async function(this: object, message: string) { prompts.push({ client: this, message }); },
    setThinkingLevel: async function(this: object, level: string) { effort.set(this, level); },
    getState: async function(this: { options?: { model?: string } }) {
      const [provider, ...id] = (this.options?.model ?? "pi/default").split("/");
      return { model: { provider, id: id.join("/") }, thinkingLevel: effort.get(this as object) ?? "medium" };
    },
  });
  return { clients, prompts, restore: () => Object.assign(RpcClient.prototype, original) };
}

function tierResult(tier: string, effort = "high"): McpToolCallResult {
  return { isError: false, content: [{ type: "text", text: JSON.stringify({ resolved_from: "pi", model: `pi/${tier}`, effort }) }] };
}

function harness(options: { auth?: boolean; result?: (tier: string) => McpToolCallResult } = {}) {
  const tools = new Map<string, CapturedTool>();
  const lookups: string[] = [];
  const pi = { registerTool: (tool: CapturedTool) => tools.set(tool.name, tool), sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
  const bridge = {
    client: { callTool: async (name: string, args: Record<string, unknown>) => {
      assert.equal(name, "config.resolve_agent");
      const tier = String(args.tier);
      lookups.push(tier);
      return options.result?.(tier) ?? tierResult(tier);
    } },
    wsToolNames: [], defaultSessionKeyRef: { current: "lead-key" },
  } as never;
  const root = mkdtempSync(join(tmpdir(), "ws-pi-explore-test-"));
  storageRoots.add(root);
  const handle = registerAgentTools(
    pi,
    bridge,
    { cwd: PACKAGE_ROOT, storage: createAgentStorageContext("owner", root), extensionPath: EXTENSION_ENTRY },
    undefined,
    EXPLORE_GUIDE,
  );
  const catalog = MODE_CONTRACT.map(([, tier]) => ({ provider: "pi", id: tier })).filter((entry, index, all) => all.findIndex(other => other.id === entry.id) === index);
  const ctx = {
    model: { provider: "pi", id: "parent" }, thinkingLevel: "xhigh",
    modelRegistry: { getAll: () => catalog, hasConfiguredAuth: () => options.auth ?? true },
  };
  return { tools, lookups, handle, root, ctx };
}

function activeDynamicTools(tools: Map<string, CapturedTool>, allowlist: readonly string[]): string[] {
  const allowed = new Set(allowlist);
  return [...tools.keys()].filter(name => allowed.has(name));
}

function legacyResearch(agentId: string, exploreMode: "simple" | "deep", toolGroup: "read-only" | "read-only-explore") {
  return {
    agentId, sessionPath: `/tmp/${agentId}.jsonl`, systemPromptPath: "/tmp/explore.md",
    wsToolNames: [], toolGroup, spawnRole: "explore", exploreMode,
    modelBase: "pi/model", modelEffort: "high", state: "idle",
  };
}

describe("persistent Explore intent modes", () => {
  test("lead, fork, worker, and researcher register one identical closed schema while terminal depth omits it from the active surface", async () => {
    const schemas: string[] = [];
    for (const [role, mode, policy] of [
      [undefined, undefined, undefined],
      ["fork", undefined, parentPolicy(0)],
      ["worker", undefined, parentPolicy(1)],
      ["explore", "diagnosis", parentPolicy(1)],
    ] as const) {
      await withRole(role, mode, policy, async () => {
        const h = harness();
        const tool = h.tools.get("explore")!;
        schemas.push(JSON.stringify({ description: tool.description, parameters: tool.parameters }));
        assert.deepEqual(Object.keys(tool.parameters.properties ?? {}), ["query", "mode"]);
        assert.deepEqual(tool.parameters.properties?.mode?.enum, MODES);
        assert.equal(tool.parameters.additionalProperties, false);
        assert.match(tool.description, /code-search.*diagnosis.*comparison.*synthesis/);
        await h.handle.stopAll();
      });
    }
    assert.equal(new Set(schemas).size, 1, "all eligible dispatcher roles expose the same Explore contract");

    await withRole("explore", "synthesis", parentPolicy(2), async () => {
      const h = harness();
      assert.ok(h.tools.has("explore"), "registration stays role-independent");
      const terminal = terminalTools(resolveTools("read-only-explore").split(","), 2, 2);
      assert.equal(activeDynamicTools(h.tools, terminal).includes("explore"), false);
      await h.handle.stopAll();
    });
  });

  test("omission and every intent mode resolve exactly the settled tier and share one persistent read/web profile", async () => {
    const rpc = installRpcHarness();
    try {
      await withRole(undefined, undefined, undefined, async () => {
        const h = harness();
        const tool = h.tools.get("explore")!;
        const cases: Array<[ExploreMode | undefined, ExploreMode]> = [[undefined, "code-search"], ...MODES.map(mode => [mode, mode] as [ExploreMode, ExploreMode])];
        for (const [requested, expectedMode] of cases) {
          const raw = await tool.execute("call", { query: `inspect ${expectedMode}`, ...(requested ? { mode: requested } : {}) }, undefined, undefined, h.ctx);
          const result = JSON.parse(raw.content[0]!.text);
          assert.deepEqual(Object.keys(result).sort(), ["agent_id", "alias"]);
          const record = h.handle.rpcRegistry.get(result.agent_id)!;
          assert.equal(record.exploreMode, expectedMode);
          assert.equal(record.modelTier, TIER_BY_MODE[expectedMode]);
          assert.equal(record.modelBase, `pi/${TIER_BY_MODE[expectedMode]}`);
          assert.equal(record.toolGroup, "read-only-explore");
          assert.deepEqual(record.delegation?.network, { search: true, fetch: true });
          assert(record.delegation?.tools.includes("web_search"));
          assert(record.delegation?.tools.includes("ws_web_fetch"));
          assert(!record.delegation?.tools.includes("bash"));
          assert(!record.delegation?.tools.includes("write"));
        }
        assert.deepEqual(h.lookups, cases.map(([, mode]) => TIER_BY_MODE[mode]));
        await h.handle.stopAll();
      });
    } finally { rpc.restore(); }
  });

  test("removed and unknown arguments reject without tier lookup or allocation", async () => {
    await withRole(undefined, undefined, undefined, async () => {
      const h = harness();
      const tool = h.tools.get("explore")!;
      await assert.rejects(() => tool.execute("old", { query: "x", deep_research: true }, undefined, undefined, h.ctx), /invalid explore arguments/);
      await assert.rejects(() => tool.execute("unknown", { query: "x", mode: "important" }, undefined, undefined, h.ctx), /unknown explore mode/);
      assert.deepEqual(h.lookups, []);
      assert.equal(h.handle.rpcRegistry.size, 0);
      assert.equal(existsSync(join(h.root, "ws-agents", "owner")), false);
      await h.handle.stopAll();
    });
  });

  test("mapped-tier authentication refusal happens before alias, registry, or storage allocation", async () => {
    await withRole(undefined, undefined, undefined, async () => {
      const h = harness({ auth: false });
      await assert.rejects(() => h.tools.get("explore")!.execute("call", { query: "diagnose", mode: "diagnosis" }, undefined, undefined, h.ctx), /explore refused: tier medium/);
      assert.deepEqual(h.lookups, ["medium"]);
      assert.equal(h.handle.rpcRegistry.size, 0);
      assert.equal(existsSync(join(h.root, "ws-agents", "owner")), false);
      await h.handle.stopAll();
    });
  });

  test("follow-up and restart recovery retain the original mode, model, effort, prompt, session, and authority", async () => {
    const rpc = installRpcHarness();
    try {
      await withRole(undefined, undefined, undefined, async () => {
        const h = harness();
        const result = JSON.parse((await h.tools.get("explore")!.execute("call", { query: "compare", mode: "comparison" }, undefined, undefined, h.ctx)).content[0]!.text);
        const original = h.handle.rpcRegistry.get(result.agent_id)!;
        const sessionPath = original.sessionPath;
        const systemPromptPath = original.systemPromptPath;
        const delegation = original.delegation;
        const firstClient = rpc.prompts[0]?.client;
        assert.ok(firstClient, "the initial query reaches the persistent RPC client");
        assert.match(rpc.prompts[0]!.message, /Intent mode: comparison[\s\S]*Question:\ncompare/);
        await sendToAgent(h.handle.rpcRegistry, { cwd: PACKAGE_ROOT, extensionPath: EXTENSION_ENTRY }, original.agentId, "follow up");
        assert.deepEqual(rpc.prompts.at(-1), { client: firstClient, message: "follow up" });
        assert.equal(original.sessionPath, sessionPath);
        assert.equal(original.systemPromptPath, systemPromptPath);
        assert.equal(original.exploreMode, "comparison");
        assert.equal(h.lookups.length, 1, "same-process continuation never re-resolves the tier");

        original.client = undefined;
        const restored: RpcAgentRegistry = new Map();
        reviveOrphans(restored, parseOrphans(serializeOrphans(captureOrphans(h.handle.rpcRegistry))));
        const revived = restored.get(original.agentId)!;
        await sendToAgent(restored, { cwd: PACKAGE_ROOT, extensionPath: EXTENSION_ENTRY }, revived.agentId, "after restart");
        const resumedPrompt = rpc.prompts.at(-1)!;
        assert.equal(resumedPrompt.message, "after restart");
        assert.notEqual(resumedPrompt.client, firstClient, "restart allocates a fresh client over the saved session");
        const resumedArgs = (resumedPrompt.client as { options?: { args?: string[] } }).options?.args ?? [];
        const sessionIndex = resumedArgs.indexOf("--session");
        assert.ok(sessionIndex >= 0, "the resumed client explicitly selects a persistent session");
        assert.equal(resumedArgs[sessionIndex + 1], sessionPath);
        assert.equal(revived.sessionPath, sessionPath);
        assert.equal(revived.systemPromptPath, systemPromptPath);
        assert.equal(revived.exploreMode, "comparison");
        assert.equal(revived.modelBase, "pi/medium");
        assert.equal(revived.modelEffort, "high");
        assert.deepEqual(revived.delegation, delegation);
        assert.equal(h.lookups.length, 1, "restart continuation uses the persisted selection");
        await h.handle.stopAll();
        await revived.client?.stop();
      });
    } finally { rpc.restore(); }
  });

  test("legacy sidecar modes normalize once at the read boundary and are never serialized again", () => {
    const raw = JSON.stringify({ version: 1, writtenAt: new Date(0).toISOString(), orphans: [
      legacyResearch("simple", "simple", "read-only"),
      legacyResearch("deep", "deep", "read-only-explore"),
    ] });
    const parsed = parseOrphans(raw);
    assert.deepEqual(parsed.map(({ exploreMode, toolGroup }) => ({ exploreMode, toolGroup })), [
      { exploreMode: "code-search", toolGroup: "read-only-explore" },
      { exploreMode: "synthesis", toolGroup: "read-only-explore" },
    ]);
    const persisted = serializeOrphans(parsed);
    assert.equal(persisted.includes('"exploreMode": "simple"'), false);
    assert.equal(persisted.includes('"exploreMode": "deep"'), false);
  });

  test("a depth-one worker dispatches the same persistent path and its terminal child cannot delegate", async () => {
    const rpc = installRpcHarness();
    try {
      await withRole("worker", undefined, parentPolicy(1), async () => {
        const h = harness();
        const result = JSON.parse((await h.tools.get("explore")!.execute("call", { query: "locate", mode: "lookup" }, undefined, undefined, h.ctx)).content[0]!.text);
        const child = h.handle.rpcRegistry.get(result.agent_id)!;
        assert.equal(child.exploreMode, "lookup");
        assert.equal(child.delegation?.depth, 2);
        for (const tool of CHILD_MANAGEMENT_TOOLS) assert.equal(child.delegation?.tools.includes(tool), false, `${tool} is stripped at terminal depth`);
        const args = (child.client as unknown as { options?: { args?: string[] } }).options?.args ?? [];
        const toolsIndex = args.indexOf("--tools");
        assert.ok(toolsIndex >= 0, "the terminal child receives an explicit --tools allowlist");
        const activeTools = (args[toolsIndex + 1] ?? "").split(",");
        for (const tool of CHILD_MANAGEMENT_TOOLS) assert.equal(activeTools.includes(tool), false, `${tool} is absent from the actual terminal --tools allowlist`);
        assert.deepEqual(child.delegation?.network, { search: true, fetch: true });
        await h.handle.stopAll();
      });
    } finally { rpc.restore(); }
  });
});

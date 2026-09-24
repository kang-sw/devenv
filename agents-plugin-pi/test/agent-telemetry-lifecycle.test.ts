/** Production-boundary telemetry lifecycle fixtures.  These use the real
 * spawner entry points with RpcClient's process transport replaced; no child
 * process, owner history, or model request is involved. */
import assert from "node:assert/strict";
import { afterEach, describe, test } from "node:test";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { RpcClient, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { agentWidgetRefreshRef, attachEventListener, refreshAgentTelemetry, registerAgentTools as registerAgentToolsBase, sendToAgent, stopAgent, type RpcAgentRecord } from "../src/spawner.ts";
import { captureOrphans, parseOrphans, rehydrateOrphanRecord, serializeOrphans } from "../src/agent-sidecar.ts";

const TEST_EXTENSION_ENTRY = "/tmp/loaded ws adapter/index copy.ts";
function registerAgentTools(pi: any, bridge: any, sessionCtx: any, ...rest: any[]) {
  writeFileSync(join(sessionCtx.cwd, "p.md"), "Offline worker prompt");
  return registerAgentToolsBase(pi, bridge, { ...sessionCtx, extensionPath: sessionCtx.extensionPath ?? TEST_EXTENSION_ENTRY }, ...rest);
}
import { captureForkResume, createThreadRegistryHandle, hydrateThreadRegistry, rehydrateForkRecord, saveThreadRegistryFile } from "../src/ask.ts";
import { persistShutdownAgentSnapshots } from "../src/index.ts";
import { allocateAgentHome, createAgentStorageContext, persistOwnershipTelemetry, readOwnership } from "../src/agent-storage.ts";

const roots = new Set<string>();
afterEach(() => { for (const root of roots) rmSync(root, { recursive: true, force: true }); roots.clear(); });
function root() { const value = mkdtempSync(join(tmpdir(), "ws-pi-telemetry-life-")); roots.add(value); return value; }
const header = (id: string, parentSession?: string) => ({ type: "session", version: 3, id, timestamp: "x", cwd: "/", ...(parentSession ? { parentSession } : {}) });
const assistant = (id: string, input: number, cost: number) => ({ type: "message", id, parentId: null, timestamp: "x", message: { role: "assistant", usage: { input, cost: { total: cost } } } });
function write(path: string, lines: unknown[]) { writeFileSync(path, `${lines.map(JSON.stringify).join("\n")}\n`); }
async function ticks() { await new Promise(resolve => setImmediate(resolve)); await new Promise(resolve => setImmediate(resolve)); }

describe("agent telemetry lifecycle at production boundaries", () => {
  test("authoritative RPC context usage wins over cached-input fallback and survives compaction null", () => {
    const session = join(root(), "child.jsonl");
    const state = { sessionId: "child", sessionFile: session, model: { provider: "p", id: "m" } };
    write(session, [header("child"), { type: "message", id: "cached", parentId: null, timestamp: "x", message: { role: "assistant", usage: { input: 342, output: 8, cacheRead: 61_440, totalTokens: 61_790, cost: { total: .2 } } } }]);
    const record = { agentId: "a", sessionPath: session } as RpcAgentRecord;
    refreshAgentTelemetry(record, state, { stats: { sessionId: "child", sessionFile: session, contextUsage: { tokens: 78_000, contextWindow: 200_000, percent: 39 } } });
    assert.equal(record.telemetry?.contextTokens, 78_000, "Pi's current-context estimate is authoritative over per-call usage");

    write(session, [header("child"), assistant("before-compaction", 3_150, .2), { type: "compaction", id: "compact", usage: { input: 999, cost: { total: .1 } } }]);
    refreshAgentTelemetry(record, state, { stats: { sessionId: "child", sessionFile: session, contextUsage: { tokens: null, contextWindow: 200_000, percent: null } } });
    assert.equal(record.telemetry?.contextTokens, 78_000, "the temporary null interval retains the last valid value for this session identity");
  });

  test("compaction_end requests authoritative stats and preserves prior occupancy through Pi's null interval", async () => {
    const dir = root(), session = join(dir, "child.jsonl");
    const state = { sessionId: "child", sessionFile: session, model: { provider: "p", id: "m" } };
    write(session, [header("child"), assistant("before", 3_150, .2)]);
    let listener: ((event: unknown) => void) | undefined, statsCalls = 0;
    const client = {
      onEvent: (fn: (event: unknown) => void) => (listener = fn, () => {}),
      getState: async () => state,
      getSessionStats: async () => { statsCalls++; return { sessionId: "child", sessionFile: session, contextUsage: { tokens: null, contextWindow: 200_000, percent: null } }; },
    } as unknown as RpcClient;
    const record = { agentId: "a", client, launchGeneration: 1, sessionPath: session, wsToolNames: [], toolGroup: "full-worker", reportLog: [], streaming: true, running: true } as RpcAgentRecord;
    refreshAgentTelemetry(record, state, { stats: { sessionId: "child", sessionFile: session, contextUsage: { tokens: 78_000, contextWindow: 200_000, percent: 39 } } });
    attachEventListener(undefined, new Map([["a", record]]), record, client);

    write(session, [header("child"), assistant("before", 3_150, .2), { type: "compaction", id: "compact", usage: { input: 999, cost: { total: .1 } } }]);
    listener!({ type: "compaction_end" }); await ticks();
    assert.equal(statsCalls, 1, "the production compaction boundary requests a fresh authoritative snapshot");
    assert.equal(record.telemetry?.contextTokens, 78_000, "the temporary RPC null retains the preceding same-session occupancy");
  });

  test("context occupancy is cleared when the RPC session identity changes", () => {
    const dir = root(), oldSession = join(dir, "old.jsonl"), nextSession = join(dir, "next.jsonl");
    write(oldSession, [header("old"), assistant("old-call", 70_000, .2)]);
    write(nextSession, [header("next")]);
    const record = { agentId: "a", sessionPath: oldSession, telemetry: { version: 1, origin: { sessionId: "old", sessionPath: oldSession, emptyPrefix: true }, contextTokens: 70_000 } } as RpcAgentRecord;
    refreshAgentTelemetry(record, { sessionId: "next", sessionFile: nextSession }, { stats: { sessionId: "next", sessionFile: nextSession, contextUsage: { tokens: null, contextWindow: 200_000, percent: null } } });
    assert.equal(record.telemetry, undefined);
    assert.equal(record.observedContextTokens, undefined);
  });

  for (const interruption of ["partial", "missing", "read-error"] as const) test(`fork origin and usage survive ${interruption} and resume complete accounting`, () => {
    const session = join(root(), "child.jsonl");
    const state = { sessionId: "s", sessionFile: session, model: { provider: "p", id: "m" }, thinkingLevel: "low" };
    const record = { agentId: "a", sessionPath: session } as RpcAgentRecord;
    const inherited = [header("s", "/parent"), assistant("parent", 900, 9)];
    write(session, inherited); refreshAgentTelemetry(record, state, { fresh: true });
    const complete = [...inherited, assistant("child", 20, .2)];
    write(session, complete); refreshAgentTelemetry(record, state);
    const origin = { ...record.telemetry!.origin };
    if (interruption === "partial") writeFileSync(session, '{"type":');
    else { rmSync(session); if (interruption === "read-error") mkdirSync(session); }
    assert.equal(refreshAgentTelemetry(record, { ...state, thinkingLevel: "high" }), true, "selection changes notify even with stale usage");
    assert.deepEqual(record.telemetry, { version: 1, origin, model: "p/m", effort: "high", contextTokens: 20, estimatedUsd: .2 });
    assert.equal(record.telemetryContextFloor, undefined);
    if (interruption === "read-error") rmSync(session, { recursive: true });
    write(session, [...complete, assistant("next", 25, .3)]);
    refreshAgentTelemetry(record, state); refreshAgentTelemetry(record, state);
    assert.deepEqual(record.telemetry?.origin, origin);
    assert.equal(record.telemetry?.contextTokens, 25); assert.equal(record.telemetry?.estimatedUsd, .5);
  });

  for (const contradiction of ["header", "path", "anchor", "duplicate", "interior", "entry-id"] as const) test(`readable ${contradiction} contradiction invalidates saved attribution`, () => {
    const session = join(root(), "child.jsonl");
    const state = { sessionId: "s", sessionFile: session };
    const record = { agentId: "a", sessionPath: session } as RpcAgentRecord;
    const entries = [header("s", "/parent"), assistant("parent", 900, 9), assistant("child", 20, .2)];
    write(session, entries.slice(0, 2)); refreshAgentTelemetry(record, state, { fresh: true });
    write(session, entries); refreshAgentTelemetry(record, state);
    if (contradiction === "header") write(session, [header("other", "/parent"), ...entries.slice(1)]);
    if (contradiction === "anchor") write(session, [entries[0], entries[2]]);
    if (contradiction === "duplicate") write(session, [...entries, assistant("child", 99, 9)]);
    if (contradiction === "entry-id") write(session, [...entries, { type: "message", message: { role: "assistant" } }]);
    if (contradiction === "interior") writeFileSync(session, JSON.stringify(entries[0]) + '\n{broken\n' + JSON.stringify(entries[2]) + '\n');
    refreshAgentTelemetry(record, contradiction === "path" ? { ...state, sessionFile: session + ".other" } : state);
    assert.equal(record.telemetry, undefined); assert.equal(record.telemetryContextFloor, undefined); assert.equal(record.observedContextTokens, undefined);
  });

  test("validated child-attributable cost is mirrored into durable ownership for ancestor reconstruction", () => {
    const dir = root();
    const ownership = allocateAgentHome(createAgentStorageContext("lead", dir), "owned", "worker");
    write(ownership.sessionPath!, [header("owned"), assistant("known", 20, .2), { type: "message", id: "unknown", message: { role: "assistant", usage: { input: 30, cost: {} } } }]);
    const record = { agentId: "owned", sessionPath: ownership.sessionPath!, ownership } as RpcAgentRecord;
    refreshAgentTelemetry(record, { sessionId: "owned", sessionFile: ownership.sessionPath! });
    assert.equal(record.telemetry?.estimatedUsd, undefined);
    assert.equal(record.telemetry?.partialEstimatedUsd, .2);
    assert.deepEqual(readOwnership(ownership.home)?.telemetry, record.telemetry);
  });

  test("durable telemetry is written only when it differs from the persisted record", () => {
    const dir = root();
    const ownership = allocateAgentHome(createAgentStorageContext("lead", dir), "owned", "worker");
    const metadataFile = join(ownership.home, "ownership.json");
    const lock = join(dirname(ownership.home), `.${ownership.agentId}.ownership-lock`);
    const claim = (pid: number) => { mkdirSync(lock); writeFileSync(join(lock, "owner.json"), JSON.stringify({ pid })); };
    const state = { sessionId: "owned", sessionFile: ownership.sessionPath! };
    const entries = [header("owned"), assistant("first", 20, .2)];
    write(ownership.sessionPath!, entries);
    const record = { agentId: "owned", sessionPath: ownership.sessionPath!, ownership } as RpcAgentRecord;
    refreshAgentTelemetry(record, state);
    assert.equal(readOwnership(ownership.home)?.telemetry?.estimatedUsd, .2);

    // A dead-pid claim is reclaimed by any lock acquisition; it survives only if no refresh tried to lock.
    claim(2_147_483_647);
    const persisted = readFileSync(metadataFile, "utf8");
    refreshAgentTelemetry(record, state); refreshAgentTelemetry(record, state);
    assert.equal(readFileSync(metadataFile, "utf8"), persisted, "unchanged telemetry is not rewritten");
    assert.equal(existsSync(join(lock, "owner.json")), true, "unchanged telemetry takes no lock");
    rmSync(lock, { recursive: true });

    claim(process.pid);
    write(ownership.sessionPath!, [...entries, assistant("second", 25, .3)]);
    refreshAgentTelemetry(record, state);
    assert.equal(record.telemetry?.estimatedUsd, .5);
    assert.equal(readOwnership(ownership.home)?.telemetry?.estimatedUsd, .2, "the busy lock rejected the write");
    rmSync(lock, { recursive: true });
    assert.equal(refreshAgentTelemetry(record, state), false, "memory did not change again");
    assert.deepEqual(readOwnership(ownership.home)?.telemetry, record.telemetry, "the comparison against disk retries the failed write");
  });

  test("telemetry change detection ignores key order and undefined keys but persists a clear", () => {
    const dir = root();
    const ownership = allocateAgentHome(createAgentStorageContext("lead", dir), "owned", "worker");
    const metadataFile = join(ownership.home, "ownership.json");
    const lock = join(dirname(ownership.home), `.${ownership.agentId}.ownership-lock`);
    const telemetry = { version: 1, origin: { sessionId: "owned", sessionPath: ownership.sessionPath!, emptyPrefix: true }, model: "p/m", estimatedUsd: .2 } as const;
    persistOwnershipTelemetry(ownership.home, telemetry);
    assert.deepEqual(readOwnership(ownership.home)?.telemetry, telemetry);

    mkdirSync(lock); writeFileSync(join(lock, "owner.json"), JSON.stringify({ pid: 2_147_483_647 }));
    const persisted = readFileSync(metadataFile, "utf8");
    persistOwnershipTelemetry(ownership.home, { estimatedUsd: .2, effort: undefined, model: "p/m", origin: { emptyPrefix: true, sessionPath: ownership.sessionPath!, sessionId: "owned" }, version: 1 });
    assert.equal(readFileSync(metadataFile, "utf8"), persisted, "reordered keys and an undefined key are not a change");
    assert.equal(existsSync(join(lock, "owner.json")), true);
    rmSync(lock, { recursive: true });

    persistOwnershipTelemetry(ownership.home, undefined);
    assert.equal(readOwnership(ownership.home)?.telemetry, undefined, "clearing in-memory telemetry is persisted");
  });

  test("collector rejection clears current selection and notifies once while preserving usage", async () => {
    const session = join(root(), "child.jsonl"); write(session, [header("s"), assistant("child", 20, .2)]);
    let listener!: (event: unknown) => void, notifications = 0;
    const client = { onEvent: (fn: typeof listener) => (listener = fn, () => {}), getState: async () => { throw new Error("state unavailable"); } } as unknown as RpcClient;
    const record = { agentId: "a", client, launchGeneration: 1, sessionPath: session, wsToolNames: [], reportLog: [] } as unknown as RpcAgentRecord;
    refreshAgentTelemetry(record, { sessionId: "s", model: { provider: "p", id: "m" }, thinkingLevel: "low" });
    const origin = { ...record.telemetry!.origin }, previous = agentWidgetRefreshRef.current;
    agentWidgetRefreshRef.current = () => { notifications++; };
    try {
      attachEventListener(undefined, undefined, record, client);
      listener({ type: "thinking_level_changed", thinkingLevel: "high" }); await ticks();
      assert.equal(record.observedModel, undefined); assert.equal(record.observedEffort, undefined);
      assert.deepEqual(record.telemetry, { version: 1, origin, contextTokens: 20, estimatedUsd: .2 });
      assert.equal(notifications, 1);
      listener({ type: "thinking_level_changed" }); await ticks(); assert.equal(notifications, 1, "unchanged unknown selection does not notify again");
    } finally { agentWidgetRefreshRef.current = previous; }
  });

  for (const replacement of ["client", "generation"] as const) test(`old collector rejection cannot clear selection after ${replacement} replacement`, async () => {
    let listener!: (event: unknown) => void, reject!: (error: Error) => void, notifications = 0;
    const pending = new Promise<never>((_, fail) => { reject = fail; });
    const client = { onEvent: (fn: typeof listener) => (listener = fn, () => {}), getState: () => pending } as unknown as RpcClient;
    const record = { agentId: "a", client, launchGeneration: 1, sessionPath: "/unused", wsToolNames: [], reportLog: [] } as unknown as RpcAgentRecord;
    const previous = agentWidgetRefreshRef.current; agentWidgetRefreshRef.current = () => { notifications++; };
    try {
      attachEventListener(undefined, undefined, record, client); listener({ type: "thinking_level_changed" });
      if (replacement === "client") record.client = {} as RpcClient; else record.launchGeneration++;
      record.observedModel = "new/model"; record.observedEffort = "high";
      record.telemetry = { version: 1, origin: { sessionId: "new", sessionPath: "/unused", emptyPrefix: true }, model: "new/model", effort: "high", contextTokens: 12, estimatedUsd: .4 };
      const expected = structuredClone(record.telemetry);
      reject(new Error("old state unavailable")); await ticks();
      assert.equal(record.observedModel, "new/model"); assert.equal(record.observedEffort, "high"); assert.deepEqual(record.telemetry, expected); assert.equal(notifications, 0);
    } finally { agentWidgetRefreshRef.current = previous; }
  });

  test("fresh task and discussion forks preserve the inherited prefix as their immutable origin", () => {
    const dir = root();
    for (const role of ["task", "discussion"]) {
      const session = join(dir, `${role}.jsonl`);
      write(session, [header(`${role}-child`, "inherited"), assistant("parent-call", 900, 9)]);
      const record = { agentId: role, sessionPath: session, wsToolNames: [], toolGroup: "full-worker", reportLog: [], spawnRole: "fork" } as RpcAgentRecord;
      refreshAgentTelemetry(record, { sessionId: `${role}-child`, sessionFile: session }, { fresh: true });
      assert.deepEqual(record.telemetry?.origin, { sessionId: `${role}-child`, sessionPath: session, prefixEntryId: "parent-call" }, `${role} fork excludes its inherited history without fabricating an empty worker origin`);
      write(session, [header(`${role}-child`, "inherited"), assistant("parent-call", 900, 9), assistant("child-call", 20, .2)]);
      refreshAgentTelemetry(record, { sessionId: `${role}-child`, sessionFile: session });
      assert.equal(record.telemetry?.contextTokens, 20); assert.equal(record.telemetry?.estimatedUsd, .2);
    }
  });

  test("a failed telemetry baseline leaves ordinary dispatch intact and never fabricates a cost origin", async () => {
    const dir = root();
    const original = Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"].map(key => [key, RpcClient.prototype[key as keyof RpcClient]]));
    const prompts: string[] = [];
    Object.assign(RpcClient.prototype, {
      start: async () => {}, stop: async () => {}, abort: async () => {}, onEvent: () => () => {}, setThinkingLevel: async () => {},
      getState: async () => { throw new Error("telemetry-state-unavailable"); }, prompt: async (message: string) => { prompts.push(message); },
    });
    try {
      const tools = new Map<string, any>();
      const pi = { registerTool: (tool: any) => tools.set(tool.name, tool), sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
      const handle = registerAgentTools(pi, { client: { callTool: async () => { throw new Error("no tier lookup"); } }, wsToolNames: [], defaultSessionKeyRef: { current: "lead" } } as never, { cwd: dir });
      const result = await tools.get("ws-agent-spawn").execute("call", { system_prompt_path: join(dir, "p.md"), prompt: "still dispatch" }, undefined, undefined, { sessionManager: { getSessionId: () => "lead" }, agentStorageRoot: dir, modelRegistry: { getAll: () => [], hasConfiguredAuth: () => true } });
      const record = handle.rpcRegistry.get(JSON.parse(result.content[0].text).agent_id)!;
      assert.deepEqual(prompts, ["still dispatch"]); assert.equal(record.telemetry, undefined);
      await handle.stopAll();
    } finally { Object.assign(RpcClient.prototype, original); }
  });

  test("a newer unknown floor-backed child call retains the prior valid context occupancy", () => {
    const dir = root(), session = join(dir, "floor.jsonl");
    const record = { agentId: "floor", sessionPath: session, telemetryContextFloor: { sessionId: "floor", sessionPath: session, prefixEntryId: "parent" }, observedContextTokens: 20, wsToolNames: [], toolGroup: "full-worker", reportLog: [], spawnRole: "fork" } as RpcAgentRecord;
    write(session, [header("floor", "parent-session"), assistant("parent", 900, 9), assistant("child-known", 20, .2), { type: "message", id: "child-unknown", parentId: null, timestamp: "x", message: { role: "assistant", usage: { cost: { total: .3 } } } }]);
    refreshAgentTelemetry(record, { sessionId: "floor", sessionFile: session });
    assert.equal(record.observedContextTokens, 20, "an unknown interval keeps the prior valid value for the same session identity");
    assert.equal(record.telemetry, undefined, "a legacy floor proves only current context, never lifetime cost");
  });

  test("spawnAgent captures authoritative context before prompting and refreshes it on dormant resume", async () => {
    const dir = root(), session = join(dir, "child.jsonl"); write(session, [header("child")]);
    const original = Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState", "getSessionStats", "setThinkingLevel"].map(key => [key, RpcClient.prototype[key as keyof RpcClient]]));
    const prompts: string[] = []; let resumed = false;
    Object.assign(RpcClient.prototype, {
      start: async () => {}, stop: async () => {}, abort: async () => {}, onEvent: () => () => {}, setThinkingLevel: async () => {},
      getState: async () => ({ sessionId: "child", sessionFile: session, model: { provider: "actual", id: resumed ? "resumed" : "clamped" }, thinkingLevel: resumed ? "medium" : "low" }),
      getSessionStats: async () => ({ sessionId: "child", sessionFile: session, contextUsage: { tokens: resumed ? 71_758 : 61_782, contextWindow: 200_000, percent: 30 } }),
      prompt: async (message: string) => { prompts.push(message); },
    });
    try {
      const tools = new Map<string, any>();
      const pi = { registerTool: (tool: any) => tools.set(tool.name, tool), sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
      const handle = registerAgentTools(pi, { client: { callTool: async () => { throw new Error("no tier lookup"); } }, wsToolNames: [], defaultSessionKeyRef: { current: "lead" } } as never, { cwd: dir });
      const result = await tools.get("ws-agent-spawn").execute("call", { system_prompt_path: join(dir, "p.md"), prompt: "first" }, undefined, undefined, { sessionManager: { getSessionId: () => "lead" }, agentStorageRoot: dir, model: { provider: "requested", id: "large" }, thinkingLevel: "high", modelRegistry: { getAll: () => [], hasConfiguredAuth: () => true } });
      const record = handle.rpcRegistry.get(JSON.parse(result.content[0].text).agent_id)!;
      assert.deepEqual(record.telemetry?.origin, { sessionId: "child", sessionPath: session, emptyPrefix: true });
      assert.equal(record.telemetry?.contextTokens, 61_782);
      assert.equal(record.observedModel, "actual/clamped"); assert.equal(record.observedEffort, "low"); assert.deepEqual(prompts, ["first"]);
      await stopAgent(handle.rpcRegistry, record.agentId, undefined, { silent: true }); resumed = true;
      await sendToAgent(handle.rpcRegistry, { cwd: dir, extensionPath: TEST_EXTENSION_ENTRY }, record.agentId, "resume");
      assert.deepEqual(prompts, ["first", "resume"], "the actual dormant send path relaunches then prompts");
      assert.equal(record.observedModel, "actual/resumed"); assert.equal(record.observedEffort, "medium"); assert.equal(record.telemetry?.contextTokens, 71_758);
      await handle.stopAll();
    } finally { Object.assign(RpcClient.prototype, original); }
  });

  test("message_end waits for state and authoritative context, coalesces bursts, and ignores an old launch", async () => {
    const dir = root(), session = join(dir, "child.jsonl"); write(session, [header("child")]);
    let listener: ((event: unknown) => void) | undefined, calls = 0, statsCalls = 0, release!: () => void;
    const barrier = new Promise<void>(resolve => { release = resolve; });
    const client = {
      onEvent: (fn: (event: unknown) => void) => (listener = fn, () => {}),
      getState: async () => { calls++; await barrier; return { sessionId: "child", sessionFile: session, model: { provider: "p", id: "new" }, thinkingLevel: "high" }; },
      getSessionStats: async () => { statsCalls++; return { sessionId: "child", sessionFile: session, contextUsage: { tokens: 61_782, contextWindow: 200_000, percent: 30.891 } }; },
    } as unknown as RpcClient;
    const record = { agentId: "a", client, launchGeneration: 2, sessionPath: session, wsToolNames: [], toolGroup: "full-worker", reportLog: [], streaming: true, running: true } as RpcAgentRecord;
    attachEventListener(undefined, new Map([["a", record]]), record, client);
    listener!({ type: "message_end", message: { role: "assistant" } }); listener!({ type: "message_update" });
    write(session, [header("child"), assistant("call", 17, .4)]); release(); await ticks();
    assert.equal(calls, 2, "a burst during the first read requests one final follow-up"); assert.equal(statsCalls, 1, "stream deltas do not repeat the authoritative RPC stats request"); assert.equal(record.telemetry?.contextTokens, 61_782); assert.equal(record.telemetry?.estimatedUsd, .4);
    const old = record.client; record.client = {} as RpcClient; listener!({ type: "message_end", message: { role: "assistant" } }); await ticks();
    assert.equal(record.client, old === record.client ? old : record.client, "late old-client work cannot revive or overwrite a replacement"); assert.equal(record.telemetry?.contextTokens, 61_782);
  });

  test("message_end falls back to cached-input JSONL usage when the stats RPC is unavailable", async () => {
    const dir = root(), session = join(dir, "child.jsonl");
    const state = { sessionId: "child", sessionFile: session, model: { provider: "p", id: "m" } };
    write(session, [header("child")]);
    let listener: ((event: unknown) => void) | undefined, statsCalls = 0;
    const client = {
      onEvent: (fn: (event: unknown) => void) => (listener = fn, () => {}),
      getState: async () => state,
      getSessionStats: async () => { statsCalls++; throw new Error("stats unavailable"); },
    } as unknown as RpcClient;
    const record = { agentId: "a", client, launchGeneration: 1, sessionPath: session, wsToolNames: [], toolGroup: "full-worker", reportLog: [], streaming: true, running: true } as RpcAgentRecord;
    refreshAgentTelemetry(record, state, { fresh: true });
    attachEventListener(undefined, new Map([["a", record]]), record, client);

    write(session, [header("child"), { type: "message", id: "cached", parentId: null, timestamp: "x", message: { role: "assistant", usage: { input: 342, output: 8, cacheRead: 61_440, totalTokens: 61_790, cost: { total: .2 } } } }]);
    listener!({ type: "message_end", message: { role: "assistant" } }); await ticks();
    assert.equal(statsCalls, 1, "the production boundary attempts the authoritative source first");
    assert.equal(record.telemetry?.contextTokens, 61_790, "a rejected stats RPC falls back to totalTokens including cached input");
    assert.equal(record.telemetry?.estimatedUsd, .2);
  });

  test("stop clears live state before final disk reconciliation", async () => {
    const dir = root(), session = join(dir, "child.jsonl"); write(session, [header("stop")]);
    let record!: RpcAgentRecord;
    const client = { abort: async () => { assert.equal(record.client, undefined); write(session, [header("stop"), assistant("last", 23, .8)]); }, stop: async () => {} } as unknown as RpcClient;
    record = { agentId: "a", client, launchGeneration: 1, sessionPath: session, telemetry: { version: 1, origin: { sessionId: "stop", sessionPath: session, emptyPrefix: true } }, wsToolNames: [], toolGroup: "full-worker", reportLog: [], streaming: true, running: true } as RpcAgentRecord;
    await stopAgent(new Map([["a", record]]), "a", undefined, { silent: true });
    assert.equal(record.client, undefined); assert.equal(record.telemetry?.contextTokens, 23); assert.equal(record.telemetry?.estimatedUsd, .8);
  });

  test("production shutdown preserves the pre-stop roll-call while persisting final telemetry to sidecar and thread resume", async () => {
    const dir = root(), workerPath = join(dir, "worker.jsonl"), forkPath = join(dir, "fork.jsonl"), sidecar = join(dir, "orphans.json"), threadPath = join(dir, "threads.json");
    write(workerPath, [header("worker")]); write(forkPath, [header("fork")]);
    const makeLive = (agentId: string, sessionPath: string, input: number, cost: number, threadBound = false) => {
      let record!: RpcAgentRecord;
      const client = { abort: async () => { assert.equal(record.client, undefined, "shutdown must clear live client before child teardown"); write(sessionPath, [header(agentId), assistant(`${agentId}-final`, input, cost)]); }, stop: async () => {} } as unknown as RpcClient;
      record = { agentId, client, launchGeneration: 1, sessionPath, systemPromptPath: join(dir, `${agentId}.md`), telemetry: { version: 1, origin: { sessionId: agentId, sessionPath, emptyPrefix: true } }, wsToolNames: [], toolGroup: "full-worker", reportLog: [], spawnRole: threadBound ? "fork" : "worker", threadBound, streaming: true, running: true } as RpcAgentRecord;
      return record;
    };
    const worker = makeLive("worker", workerPath, 31, .7), fork = makeLive("fork", forkPath, 37, .9, true);
    const registry = new Map([[worker.agentId, worker], [fork.agentId, fork]]);
    const threads = createThreadRegistryHandle(); threads.pathRef.current = threadPath;
    threads.threads.set("q1", { threadId: "q1", title: "question", status: "dormant", origin: "fork-raised", createdAt: "2026-09-10T00:00:00.000Z", touchedAt: "2026-09-10T00:00:00.000Z", respondentAgentId: "fork", forkResume: captureForkResume(fork) });
    await persistShutdownAgentSnapshots({ rpcRegistry: registry, stopAll: async () => { await stopAgent(registry, "worker", undefined, { silent: true }); await stopAgent(registry, "fork", undefined, { silent: true }); } }, sidecar, threads);
    const [saved] = parseOrphans(readFileSync(sidecar, "utf8"));
    assert.equal(saved.state, "running", "roll-call is captured before orderly stop clears live state"); assert.equal(saved.telemetry?.contextTokens, 31); assert.equal(saved.telemetry?.estimatedUsd, .7);
    const persistedThread = JSON.parse(readFileSync(threadPath, "utf8")).threads[0];
    assert.equal(persistedThread.forkResume.telemetry.contextTokens, 37); assert.equal(persistedThread.forkResume.telemetry.estimatedUsd, .9);
    assert.equal(worker.client, undefined); assert.equal(fork.client, undefined);
  });

  test("ordinary legacy recovery reconstructs a no-parent session, while a boundaryless fork keeps lifetime cost unknown", () => {
    const dir = root(), ordinary = join(dir, "ordinary.jsonl"), fork = join(dir, "legacy-fork.jsonl");
    write(ordinary, [header("ordinary"), assistant("ordinary-call", 10, .1)]);
    write(fork, [header("fork", "parent"), assistant("inherited", 900, 9), assistant("unknown-boundary", 10, .1)]);
    const ordinaryRecord = rehydrateOrphanRecord({ agentId: "ordinary", sessionPath: ordinary, systemPromptPath: join(dir, "p.md"), wsToolNames: [], toolGroup: "full-worker", spawnRole: "worker", state: "running" } as any);
    assert.equal(ordinaryRecord.telemetry?.contextTokens, 10); assert.equal(ordinaryRecord.telemetry?.estimatedUsd, .1);
    const forkRecord = rehydrateOrphanRecord({ agentId: "fork", sessionPath: fork, systemPromptPath: join(dir, "p.md"), wsToolNames: [], toolGroup: "full-worker", spawnRole: "fork", state: "running" } as any);
    assert.equal(forkRecord.telemetry, undefined); assert.equal(forkRecord.observedContextTokens, undefined);
  });

  test("thread hydration itself reconciles a thread-only fork snapshot", () => {
    const dir = root(), session = join(dir, "child.jsonl"); write(session, [header("child"), assistant("old", 5, .1), assistant("new", 19, .3)]);
    const live = { agentId: "a", sessionPath: session, systemPromptPath: join(dir, "p.md"), telemetry: { version: 1, origin: { sessionId: "child", sessionPath: session, emptyPrefix: true }, contextTokens: 5, estimatedUsd: .1 }, wsToolNames: [], toolGroup: "full-worker", reportLog: [], spawnRole: "worker", running: true } as RpcAgentRecord;
    const [orphan] = parseOrphans(serializeOrphans(captureOrphans(new Map([["a", live]]))));
    const revived = rehydrateOrphanRecord(orphan); assert.equal(revived.telemetry?.contextTokens, 19); assert.equal(revived.telemetry?.estimatedUsd, .4);
    const resume = captureForkResume({ ...live, agentId: "fork", spawnRole: "fork", threadBound: true });
    const threadPath = join(dir, "threads.json"); saveThreadRegistryFile(threadPath, [{ threadId: "q1", title: "q", question: "q", status: "dormant", origin: "fork-raised", createdAt: "2026-09-10T00:00:00.000Z", touchedAt: "2026-09-10T00:00:00.000Z", respondentAgentId: "fork", forkResume: resume }]);
    const handle = createThreadRegistryHandle(); hydrateThreadRegistry(handle, threadPath);
    const threadOnly = handle.threads.get("q1")!;
    assert.equal(threadOnly.forkResume?.telemetry?.contextTokens, 19); assert.equal(threadOnly.forkResume?.telemetry?.estimatedUsd, .4, "hydration updates the row before /answer rehydrates its respondent"); assert.ok(readFileSync(threadPath, "utf8").includes("q1"));
  });
});

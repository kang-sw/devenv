/** Production-boundary telemetry lifecycle fixtures.  These use the real
 * spawner entry points with RpcClient's process transport replaced; no child
 * process, owner history, or model request is involved. */
import assert from "node:assert/strict";
import { afterEach, describe, test } from "node:test";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { RpcClient, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { attachEventListener, refreshAgentTelemetry, registerAgentTools, sendToAgent, stopAgent, type RpcAgentRecord } from "../src/spawner.ts";
import { captureOrphans, parseOrphans, rehydrateOrphanRecord, serializeOrphans } from "../src/agent-sidecar.ts";
import { captureForkResume, createThreadRegistryHandle, hydrateThreadRegistry, rehydrateForkRecord, saveThreadRegistryFile } from "../src/ask.ts";
import { persistShutdownAgentSnapshots } from "../src/index.ts";

const roots = new Set<string>();
afterEach(() => { for (const root of roots) rmSync(root, { recursive: true, force: true }); roots.clear(); });
function root() { const value = mkdtempSync(join(tmpdir(), "ws-pi-telemetry-life-")); roots.add(value); return value; }
const header = (id: string, parentSession?: string) => ({ type: "session", version: 3, id, timestamp: "x", cwd: "/", ...(parentSession ? { parentSession } : {}) });
const assistant = (id: string, input: number, cost: number) => ({ type: "message", id, parentId: null, timestamp: "x", message: { role: "assistant", usage: { input, cost: { total: cost } } } });
function write(path: string, lines: unknown[]) { writeFileSync(path, `${lines.map(JSON.stringify).join("\n")}\n`); }
async function ticks() { await new Promise(resolve => setImmediate(resolve)); await new Promise(resolve => setImmediate(resolve)); }

describe("agent telemetry lifecycle at production boundaries", () => {
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
      assert.equal(record.telemetry?.latestInput, 20); assert.equal(record.telemetry?.estimatedUsd, .2);
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

  test("a newer unknown floor-backed child call clears the prior latest input", () => {
    const dir = root(), session = join(dir, "floor.jsonl");
    const record = { agentId: "floor", sessionPath: session, telemetryInputFloor: { sessionId: "floor", sessionPath: session, prefixEntryId: "parent" }, observedLatestInput: 20, wsToolNames: [], toolGroup: "full-worker", reportLog: [], spawnRole: "fork" } as RpcAgentRecord;
    write(session, [header("floor", "parent-session"), assistant("parent", 900, 9), assistant("child-known", 20, .2), { type: "message", id: "child-unknown", parentId: null, timestamp: "x", message: { role: "assistant", usage: { cost: { total: .3 } } } }]);
    refreshAgentTelemetry(record, { sessionId: "floor", sessionFile: session });
    assert.equal(record.observedLatestInput, undefined, "the prior child input cannot stand in for a later unknown call");
    assert.equal(record.telemetry, undefined, "a legacy floor proves only latest-call observation, never lifetime cost");
  });

  test("spawnAgent captures the real pre-prompt baseline, then prompt sees actual clamped state", async () => {
    const dir = root(), session = join(dir, "child.jsonl"); write(session, [header("child")]);
    const original = Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"].map(key => [key, RpcClient.prototype[key as keyof RpcClient]]));
    const prompts: string[] = []; let resumed = false;
    Object.assign(RpcClient.prototype, {
      start: async () => {}, stop: async () => {}, abort: async () => {}, onEvent: () => () => {}, setThinkingLevel: async () => {},
      getState: async () => ({ sessionId: "child", sessionFile: session, model: { provider: "actual", id: resumed ? "resumed" : "clamped" }, thinkingLevel: resumed ? "medium" : "low" }),
      prompt: async (message: string) => { prompts.push(message); },
    });
    try {
      const tools = new Map<string, any>();
      const pi = { registerTool: (tool: any) => tools.set(tool.name, tool), sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
      const handle = registerAgentTools(pi, { client: { callTool: async () => { throw new Error("no tier lookup"); } }, wsToolNames: [], defaultSessionKeyRef: { current: "lead" } } as never, { cwd: dir });
      const result = await tools.get("ws-agent-spawn").execute("call", { system_prompt_path: join(dir, "p.md"), prompt: "first" }, undefined, undefined, { sessionManager: { getSessionId: () => "lead" }, agentStorageRoot: dir, model: { provider: "requested", id: "large" }, thinkingLevel: "high", modelRegistry: { getAll: () => [], hasConfiguredAuth: () => true } });
      const record = handle.rpcRegistry.get(JSON.parse(result.content[0].text).agent_id)!;
      assert.deepEqual(record.telemetry?.origin, { sessionId: "child", sessionPath: session, emptyPrefix: true });
      assert.equal(record.observedModel, "actual/clamped"); assert.equal(record.observedEffort, "low"); assert.deepEqual(prompts, ["first"]);
      await stopAgent(handle.rpcRegistry, record.agentId, undefined, { silent: true }); resumed = true;
      await sendToAgent(handle.rpcRegistry, { cwd: dir }, record.agentId, "resume");
      assert.deepEqual(prompts, ["first", "resume"], "the actual dormant send path relaunches then prompts");
      assert.equal(record.observedModel, "actual/resumed"); assert.equal(record.observedEffort, "medium");
      await handle.stopAll();
    } finally { Object.assign(RpcClient.prototype, original); }
  });

  test("message_end waits for its getState barrier, coalesces bursts, and ignores an old launch", async () => {
    const dir = root(), session = join(dir, "child.jsonl"); write(session, [header("child")]);
    let listener: ((event: unknown) => void) | undefined, calls = 0, release!: () => void;
    const barrier = new Promise<void>(resolve => { release = resolve; });
    const client = { onEvent: (fn: (event: unknown) => void) => (listener = fn, () => {}), getState: async () => { calls++; await barrier; return { sessionId: "child", sessionFile: session, model: { provider: "p", id: "new" }, thinkingLevel: "high" }; } } as unknown as RpcClient;
    const record = { agentId: "a", client, launchGeneration: 2, sessionPath: session, wsToolNames: [], toolGroup: "full-worker", reportLog: [], streaming: true, running: true } as RpcAgentRecord;
    attachEventListener(undefined, new Map([["a", record]]), record, client);
    listener!({ type: "message_end", message: { role: "assistant" } }); listener!({ type: "message_update" });
    write(session, [header("child"), assistant("call", 17, .4)]); release(); await ticks();
    assert.equal(calls, 2, "a burst during the first read requests one final follow-up"); assert.equal(record.telemetry?.latestInput, 17); assert.equal(record.telemetry?.estimatedUsd, .4);
    const old = record.client; record.client = {} as RpcClient; listener!({ type: "message_end", message: { role: "assistant" } }); await ticks();
    assert.equal(record.client, old === record.client ? old : record.client, "late old-client work cannot revive or overwrite a replacement"); assert.equal(record.telemetry?.latestInput, 17);
  });

  test("stop clears live state before final disk reconciliation", async () => {
    const dir = root(), session = join(dir, "child.jsonl"); write(session, [header("stop")]);
    let record!: RpcAgentRecord;
    const client = { abort: async () => { assert.equal(record.client, undefined); write(session, [header("stop"), assistant("last", 23, .8)]); }, stop: async () => {} } as unknown as RpcClient;
    record = { agentId: "a", client, launchGeneration: 1, sessionPath: session, telemetry: { version: 1, origin: { sessionId: "stop", sessionPath: session, emptyPrefix: true } }, wsToolNames: [], toolGroup: "full-worker", reportLog: [], streaming: true, running: true } as RpcAgentRecord;
    await stopAgent(new Map([["a", record]]), "a", undefined, { silent: true });
    assert.equal(record.client, undefined); assert.equal(record.telemetry?.latestInput, 23); assert.equal(record.telemetry?.estimatedUsd, .8);
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
    assert.equal(saved.state, "running", "roll-call is captured before orderly stop clears live state"); assert.equal(saved.telemetry?.latestInput, 31); assert.equal(saved.telemetry?.estimatedUsd, .7);
    const persistedThread = JSON.parse(readFileSync(threadPath, "utf8")).threads[0];
    assert.equal(persistedThread.forkResume.telemetry.latestInput, 37); assert.equal(persistedThread.forkResume.telemetry.estimatedUsd, .9);
    assert.equal(worker.client, undefined); assert.equal(fork.client, undefined);
  });

  test("ordinary legacy recovery reconstructs a no-parent session, while a boundaryless fork keeps lifetime cost unknown", () => {
    const dir = root(), ordinary = join(dir, "ordinary.jsonl"), fork = join(dir, "legacy-fork.jsonl");
    write(ordinary, [header("ordinary"), assistant("ordinary-call", 10, .1)]);
    write(fork, [header("fork", "parent"), assistant("inherited", 900, 9), assistant("unknown-boundary", 10, .1)]);
    const ordinaryRecord = rehydrateOrphanRecord({ agentId: "ordinary", sessionPath: ordinary, systemPromptPath: join(dir, "p.md"), wsToolNames: [], toolGroup: "full-worker", spawnRole: "worker", state: "running" } as any);
    assert.equal(ordinaryRecord.telemetry?.latestInput, 10); assert.equal(ordinaryRecord.telemetry?.estimatedUsd, .1);
    const forkRecord = rehydrateOrphanRecord({ agentId: "fork", sessionPath: fork, systemPromptPath: join(dir, "p.md"), wsToolNames: [], toolGroup: "full-worker", spawnRole: "fork", state: "running" } as any);
    assert.equal(forkRecord.telemetry, undefined); assert.equal(forkRecord.observedLatestInput, undefined);
  });

  test("thread hydration itself reconciles a thread-only fork snapshot", () => {
    const dir = root(), session = join(dir, "child.jsonl"); write(session, [header("child"), assistant("old", 5, .1), assistant("new", 19, .3)]);
    const live = { agentId: "a", sessionPath: session, systemPromptPath: join(dir, "p.md"), telemetry: { version: 1, origin: { sessionId: "child", sessionPath: session, emptyPrefix: true }, latestInput: 5, estimatedUsd: .1 }, wsToolNames: [], toolGroup: "full-worker", reportLog: [], spawnRole: "worker", running: true } as RpcAgentRecord;
    const [orphan] = parseOrphans(serializeOrphans(captureOrphans(new Map([["a", live]]))));
    const revived = rehydrateOrphanRecord(orphan); assert.equal(revived.telemetry?.latestInput, 19); assert.equal(revived.telemetry?.estimatedUsd, .4);
    const resume = captureForkResume({ ...live, agentId: "fork", spawnRole: "fork", threadBound: true });
    const threadPath = join(dir, "threads.json"); saveThreadRegistryFile(threadPath, [{ threadId: "q1", title: "q", question: "q", status: "dormant", origin: "fork-raised", createdAt: "2026-09-10T00:00:00.000Z", touchedAt: "2026-09-10T00:00:00.000Z", respondentAgentId: "fork", forkResume: resume }]);
    const handle = createThreadRegistryHandle(); hydrateThreadRegistry(handle, threadPath);
    const threadOnly = handle.threads.get("q1")!;
    assert.equal(threadOnly.forkResume?.telemetry?.latestInput, 19); assert.equal(threadOnly.forkResume?.telemetry?.estimatedUsd, .4, "hydration updates the row before /answer rehydrates its respondent"); assert.ok(readFileSync(threadPath, "utf8").includes("q1"));
  });
});

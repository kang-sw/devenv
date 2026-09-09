/** Production-boundary telemetry lifecycle fixtures.  These use the real
 * spawner entry points with RpcClient's process transport replaced; no child
 * process, owner history, or model request is involved. */
import assert from "node:assert/strict";
import { afterEach, describe, test } from "node:test";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { RpcClient, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { attachEventListener, registerAgentTools, sendToAgent, stopAgent, type RpcAgentRecord } from "../src/spawner.ts";
import { captureOrphans, parseOrphans, rehydrateOrphanRecord, serializeOrphans } from "../src/agent-sidecar.ts";
import { captureForkResume, createThreadRegistryHandle, hydrateThreadRegistry, rehydrateForkRecord, saveThreadRegistryFile } from "../src/ask.ts";

const roots = new Set<string>();
afterEach(() => { for (const root of roots) rmSync(root, { recursive: true, force: true }); roots.clear(); });
function root() { const value = mkdtempSync(join(tmpdir(), "ws-pi-telemetry-life-")); roots.add(value); return value; }
const header = (id: string, parentSession?: string) => ({ type: "session", version: 3, id, timestamp: "x", cwd: "/", ...(parentSession ? { parentSession } : {}) });
const assistant = (id: string, input: number, cost: number) => ({ type: "message", id, parentId: null, timestamp: "x", message: { role: "assistant", usage: { input, cost: { total: cost } } } });
function write(path: string, lines: unknown[]) { writeFileSync(path, `${lines.map(JSON.stringify).join("\n")}\n`); }
async function ticks() { await new Promise(resolve => setImmediate(resolve)); await new Promise(resolve => setImmediate(resolve)); }

describe("agent telemetry lifecycle at production boundaries", () => {
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

  test("both recovery formats retain stale snapshots while replaying new durable calls, including a thread-only row", () => {
    const dir = root(), session = join(dir, "child.jsonl"); write(session, [header("child"), assistant("old", 5, .1), assistant("new", 19, .3)]);
    const live = { agentId: "a", sessionPath: session, systemPromptPath: join(dir, "p.md"), telemetry: { version: 1, origin: { sessionId: "child", sessionPath: session, emptyPrefix: true }, latestInput: 5, estimatedUsd: .1 }, wsToolNames: [], toolGroup: "full-worker", reportLog: [], spawnRole: "worker", running: true } as RpcAgentRecord;
    const [orphan] = parseOrphans(serializeOrphans(captureOrphans(new Map([["a", live]]))));
    const revived = rehydrateOrphanRecord(orphan); assert.equal(revived.telemetry?.latestInput, 19); assert.equal(revived.telemetry?.estimatedUsd, .4);
    const resume = captureForkResume({ ...live, agentId: "fork", spawnRole: "fork", threadBound: true });
    const threadPath = join(dir, "threads.json"); saveThreadRegistryFile(threadPath, [{ threadId: "q1", title: "q", question: "q", status: "dormant", origin: "fork-raised", createdAt: "2026-09-10T00:00:00.000Z", touchedAt: "2026-09-10T00:00:00.000Z", respondentAgentId: "fork", forkResume: resume }]);
    const handle = createThreadRegistryHandle(); hydrateThreadRegistry(handle, threadPath);
    const threadOnly = handle.threads.get("q1")!; const fork = rehydrateForkRecord("fork", threadOnly.forkResume!);
    assert.equal(fork.telemetry?.latestInput, 19); assert.equal(fork.telemetry?.estimatedUsd, .4); assert.ok(readFileSync(threadPath, "utf8").includes("q1"));
  });
});

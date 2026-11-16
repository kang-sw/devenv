/**
 * A dormant relaunch re-stamps the dispatching session's current key as a
 * child's ferrule parent (`relaunchDelegation`), so a child spawned
 * without one, one whose stored parent key was pruned, or one outliving a lead
 * re-login bootstraps with lineage instead of a parent-less `ferrule`. Forks
 * retain lead authority while carrying the current launching lead as parent.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { RpcClient, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { createAgentStorageContext } from "../src/agent-storage.ts";
import { DELEGATION_ENV, type DelegationPolicy } from "../src/delegation-policy.ts";
import { registerAgentTools, relaunchDelegation, sendToAgent, spawnAgent, type RpcAgentRecord } from "../src/spawner.ts";
import { closeFakeChildren, connectFakeChild } from "./fixtures/channel-child.ts";
import { closeThreadOnDone, createForkChannel, createThreadRegistryHandle, ensureRespondent, type ThreadRecord } from "../src/ask.ts";
import { until } from "./fixtures/subtree-channels.ts";

const TEST_EXTENSION_ENTRY = "/tmp/loaded ws adapter/index copy.ts";
const leafPolicy: DelegationPolicy = { version: 1, depth: 1, maxDepth: 2, authority: "leaf", tools: ["read"], write: { mode: "none" } };

test("relaunchDelegation re-stamps every policy without a pre-minted key, including forks", () => {
  const worker = { spawnRole: "worker" as const, delegation: leafPolicy };
  assert.deepEqual(relaunchDelegation(worker, "lead-now"), { ...leafPolicy, parentSessionKey: "lead-now" });
  assert.equal(leafPolicy.parentSessionKey, undefined, "the stored policy is not mutated");

  const stale = { spawnRole: "execute-worker" as const, delegation: { ...leafPolicy, parentSessionKey: "pruned-key" } };
  assert.equal(relaunchDelegation(stale, "lead-now")?.parentSessionKey, "lead-now", "a stale stored parent is replaced");

  const preMinted = { spawnRole: "worker" as const, delegation: { ...leafPolicy, sessionKey: "child-own-key" } };
  assert.equal(relaunchDelegation(preMinted, "lead-now"), preMinted.delegation, "a pre-minted key needs no parent");

  const fork = { spawnRole: "fork" as const, delegation: { ...leafPolicy, authority: "lead" as const } };
  assert.deepEqual(relaunchDelegation(fork, "lead-now"), { ...fork.delegation, parentSessionKey: "lead-now" });
  const staleFork = { ...fork, delegation: { ...fork.delegation, parentSessionKey: "stale-lead" } };
  assert.equal(relaunchDelegation(staleFork, "lead-now")?.parentSessionKey, "lead-now");
  const preMintedFork = { ...fork, delegation: { ...fork.delegation, sessionKey: "own-fork" } };
  assert.equal(relaunchDelegation(preMintedFork, "lead-now"), preMintedFork.delegation);

  assert.equal(relaunchDelegation(worker, undefined), leafPolicy, "no current key leaves the stored policy unchanged");
  assert.equal(relaunchDelegation({ spawnRole: "worker" }, "lead-now"), undefined);
});

type Proto = Record<string, unknown>;
function installRpcHarness(start: (this: any) => Promise<void>, extra: Proto = {}): () => void {
  const proto = RpcClient.prototype as unknown as Proto;
  const names = ["start", "stop", "abort", "onEvent", "prompt", "getState", "getSessionStats", "setThinkingLevel"];
  const saved = Object.fromEntries(names.map(name => [name, proto[name]]));
  Object.assign(proto, {
    start, stop: async () => {}, abort: async () => {}, onEvent: () => () => {}, prompt: async () => {}, setThinkingLevel: async () => {},
    getState: async () => ({}), getSessionStats: async () => { throw new Error("no stats"); },
    ...extra,
  });
  return () => { closeFakeChildren(); Object.assign(proto, saved); };
}

test("internal execution-worker resume uses the lead's key read at send time", async () => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "ws-pi-relaunch-parent-")));
  const launched: DelegationPolicy[] = [];
  const restore = installRpcHarness(async function (this: { options: { env: Record<string, string>; args: string[] } }) {
    launched.push(JSON.parse(this.options.env[DELEGATION_ENV]!));
    await connectFakeChild(this.options.env, this.options.args);
  });
  const tools = new Map<string, any>();
  const pi = { registerTool: (tool: any) => tools.set(tool.name, tool), sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
  const bridge = { wsToolNames: [], defaultSessionKeyRef: { current: "lead-at-spawn" }, client: {} };
  const handle = registerAgentTools(pi, bridge as never, { cwd: root, extensionPath: TEST_EXTENSION_ENTRY, storage: createAgentStorageContext("relaunch-owner", root) });
  try {
    for (const [agentId, spawnRole] of [["pre-fix-execute", "execute-worker"]] as const) {
      const dormant: RpcAgentRecord = {
        agentId, spawnRole, sessionPath: join(root, `${agentId}.jsonl`), systemPromptPath: join(root, "prompt.md"),
        wsToolNames: [], toolGroup: spawnRole === "execute-worker" ? "execute-worker" : "full-worker",
        delegation: leafPolicy, streaming: false, running: false, reportLog: [],
      };
      handle.rpcRegistry.set(agentId, dormant);
    }
    bridge.defaultSessionKeyRef.current = "lead-after-relogin";
    for (const agentId of ["pre-fix-execute"]) {
      await sendToAgent(handle.rpcRegistry, { pi, cwd: root, extensionPath: TEST_EXTENSION_ENTRY, leadSessionKey: bridge.defaultSessionKeyRef.current }, agentId, "resume");
    }
    assert.equal(launched.length, 1);
    for (const policy of launched) {
      assert.equal(policy.parentSessionKey, "lead-after-relogin");
      assert.equal(policy.sessionKey, undefined);
      assert.equal(policy.authority, "leaf", "only the parent key is re-stamped");
    }
    assert.equal(handle.rpcRegistry.get("pre-fix-execute")!.delegation?.parentSessionKey, undefined, "the stored record policy is left as spawned");
  } finally {
    await handle.stopAll();
    restore();
    rmSync(root, { recursive: true, force: true });
  }
});

for (const route of ["agent-send", "owner-send", "done"] as const) {
for (const storedParent of [undefined, "stale-parent"]) {
test(`a dormant fork re-stamps ${storedParent ?? "missing"} parent through ${route}`, async () => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "ws-pi-relaunch-fork-")));
  const storage = createAgentStorageContext("relaunch-fork-owner", root);
  const registry = new Map<string, RpcAgentRecord>();
  const launched: DelegationPolicy[] = [];
  const context = { version: 1, kind: "task", effectiveSystemPrompt: "captured", activeTools: [], registeredTools: [] };
  const restore = installRpcHarness(async function (this: any) {
    const policy = JSON.parse(this.options.env[DELEGATION_ENV]);
    assert.equal(this.options.env.WS_PI_PARENT_SESSION_KEY, policy.parentSessionKey);
    launched.push(policy);
    const home = this.options.args[this.options.args.indexOf("--session-dir") + 1];
    this.sessionFile = join(home, "fork-child.jsonl");
    if (!existsSync(this.sessionFile)) writeFileSync(this.sessionFile, "offline fork session\n");
    await connectFakeChild(this.options.env, this.options.args, { fork: { sessionId: "child-id", sessionPath: this.sessionFile, ownSessionKey: "fork-child-key" } });
  }, {
    async getState(this: any) { return { sessionId: "child-id", sessionFile: this.sessionFile, model: { provider: "offline", id: "test" }, thinkingLevel: "off" }; },
  });
  const pi = { sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
  try {
    const bridge = { wsToolNames: [], defaultSessionKeyRef: { current: "lead-at-spawn" }, client: {} };
    const result = await spawnAgent(registry, {
      pi, cwd: root, extensionPath: TEST_EXTENSION_ENTRY, storage,
      wsToolNames: [], client: bridge.client as never,
      parentSessionKey: bridge.defaultSessionKeyRef.current,
      parentPolicy: { version: 1, depth: 0, maxDepth: 2, authority: "lead", tools: ["read"], sessionKey: bridge.defaultSessionKeyRef.current },
      inheritModel: "offline/test", catalog: [], explicitTools: "read", forkFrom: "/tmp/relaunch-parent.jsonl", spawnRole: "fork", forkContext: context as never,
    }, { prompt: "task" });
    const record = registry.get(result.agent_id)!;
    await record.client?.stop();
    record.client = undefined;
    record.delegation = { ...record.delegation!, parentSessionKey: storedParent };
    const resumeCtx = { cwd: root, extensionPath: TEST_EXTENSION_ENTRY, leadSessionKeyRef: bridge.defaultSessionKeyRef };
    const channel = createForkChannel(pi, registry, resumeCtx, result.agent_id);
    bridge.defaultSessionKeyRef.current = "lead-now";
    if (route === "owner-send") await channel.send!("resume");
    else if (route === "done") {
      const thread: ThreadRecord = { threadId: "q1", title: "question", origin: "fork-raised", status: "open", respondentAgentId: result.agent_id, createdAt: "now", touchedAt: "now" };
      closeThreadOnDone(pi, createThreadRegistryHandle(), registry, thread, "", resumeCtx);
      await until(() => launched.length === 2 && record.running, "dormant /done relaunch");
    } else await sendToAgent(registry, { cwd: root, pi, extensionPath: TEST_EXTENSION_ENTRY, leadSessionKey: "lead-now" }, result.agent_id, "resume");
    assert.equal(launched.length, 2);
    assert.equal(launched[0]!.parentSessionKey, "lead-at-spawn");
    assert.equal(launched[1]!.parentSessionKey, "lead-now");
    for (const policy of launched) assert.equal(policy.authority, "lead");
    assert.equal(record.delegation.parentSessionKey, storedParent, "stored policy remains immutable");
  } finally {
    for (const record of registry.values()) record.ownershipObserverStop?.();
    restore();
    rmSync(root, { recursive: true, force: true });
  }
});
}
}

test("the discussion-fork spawn passes the current lead's parent policy", async () => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "ws-pi-discussion-parent-")));
  const registry = new Map<string, RpcAgentRecord>();
  const launched: DelegationPolicy[] = [];
  const restore = installRpcHarness(async function (this: any) {
    launched.push(JSON.parse(this.options.env[DELEGATION_ENV]));
    assert.equal(this.options.env.WS_PI_PARENT_SESSION_KEY, "discussion-lead");
    const home = this.options.args[this.options.args.indexOf("--session-dir") + 1];
    this.sessionFile = join(home, "discussion.jsonl");
    writeFileSync(this.sessionFile, "offline discussion session\n");
    await connectFakeChild(this.options.env, this.options.args, { fork: { sessionId: "discussion-id", sessionPath: this.sessionFile, ownSessionKey: "discussion-key", activeTools: ["read"], registeredTools: [{ name: "read", description: "Read", parameters: { type: "object" } }] } });
  }, {
    async getState(this: any) { return { sessionId: "discussion-id", sessionFile: this.sessionFile }; },
  });
  const pi = { sendMessage() {}, sendUserMessage() {}, getActiveTools: () => ["read"], getAllTools: () => [{ name: "read", description: "Read", parameters: { type: "object" } }], getThinkingLevel: () => "off" } as unknown as ExtensionAPI;
  const bridge = { wsToolNames: [], defaultSessionKeyRef: { current: "discussion-lead" }, client: {} };
  const thread: ThreadRecord = { threadId: "q1", title: "question", origin: "fork-raised", status: "open", createdAt: "now", touchedAt: "now" };
  try {
    await ensureRespondent(pi, {
      agentStorageRoot: root,
      model: { provider: "offline", id: "test" },
      sessionManager: { getSessionId: () => "parent-session", getSessionFile: () => "/tmp/parent-session.jsonl" },
    } as never, bridge as never, registry, createThreadRegistryHandle(), thread, {
      cwd: root, extensionPath: TEST_EXTENSION_ENTRY,
      effectivePromptRef: { current: { effectiveSystemPrompt: "captured", activeTools: [], registeredTools: [] } } as never,
    });
    assert.equal(launched.length, 1);
    assert.equal(launched[0]!.parentSessionKey, "discussion-lead");
    assert.equal(launched[0]!.authority, "lead");
  } finally {
    for (const record of registry.values()) record.ownershipObserverStop?.();
    restore();
    rmSync(root, { recursive: true, force: true });
  }
});

/**
 * A dormant relaunch re-stamps the dispatching session's current key as a
 * non-fork child's ferrule parent (`relaunchDelegation`), so a child spawned
 * without one, one whose stored parent key was pruned, or one outliving a lead
 * re-login bootstraps with lineage instead of a parent-less `ferrule`. Fork
 * records stay lateral leads with no policy parent.
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

const TEST_EXTENSION_ENTRY = "/tmp/loaded ws adapter/index copy.ts";
const leafPolicy: DelegationPolicy = { version: 1, depth: 1, maxDepth: 2, authority: "leaf", tools: ["read"], write: { mode: "none" } };

test("relaunchDelegation re-stamps only a non-fork policy without a pre-minted key", () => {
  const worker = { spawnRole: "worker" as const, delegation: leafPolicy };
  assert.deepEqual(relaunchDelegation(worker, "lead-now"), { ...leafPolicy, parentSessionKey: "lead-now" });
  assert.equal(leafPolicy.parentSessionKey, undefined, "the stored policy is not mutated");

  const stale = { spawnRole: "execute-worker" as const, delegation: { ...leafPolicy, parentSessionKey: "pruned-key" } };
  assert.equal(relaunchDelegation(stale, "lead-now")?.parentSessionKey, "lead-now", "a stale stored parent is replaced");

  const preMinted = { spawnRole: "worker" as const, delegation: { ...leafPolicy, sessionKey: "child-own-key" } };
  assert.equal(relaunchDelegation(preMinted, "lead-now"), preMinted.delegation, "a pre-minted key needs no parent");

  const fork = { spawnRole: "fork" as const, delegation: { ...leafPolicy, authority: "lead" as const } };
  assert.equal(relaunchDelegation(fork, "lead-now"), fork.delegation, "a fork keeps its parent-less policy");

  assert.equal(relaunchDelegation(worker, undefined), leafPolicy, "no current key leaves the stored policy unchanged");
  assert.equal(relaunchDelegation({ spawnRole: "worker" }, "lead-now"), undefined);
});

type Proto = Record<string, unknown>;
function patchRpc(start: (this: any) => Promise<void>, extra: Proto = {}): () => void {
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

test("ws-agent-send relaunches a dormant worker with the lead's key read at send time", async () => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "ws-pi-relaunch-parent-")));
  const launched: DelegationPolicy[] = [];
  const restore = patchRpc(async function (this: { options: { env: Record<string, string>; args: string[] } }) {
    launched.push(JSON.parse(this.options.env[DELEGATION_ENV]!));
    await connectFakeChild(this.options.env, this.options.args);
  });
  const tools = new Map<string, any>();
  const pi = { registerTool: (tool: any) => tools.set(tool.name, tool), sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
  const bridge = { wsToolNames: [], defaultSessionKeyRef: { current: "lead-at-spawn" }, client: {} };
  const handle = registerAgentTools(pi, bridge as never, { cwd: root, extensionPath: TEST_EXTENSION_ENTRY, storage: createAgentStorageContext("relaunch-owner", root) });
  try {
    for (const [agentId, spawnRole] of [["pre-fix-execute", "execute-worker"], ["pre-fix-worker", "worker"]] as const) {
      const dormant: RpcAgentRecord = {
        agentId, spawnRole, sessionPath: join(root, `${agentId}.jsonl`), systemPromptPath: join(root, "prompt.md"),
        wsToolNames: [], toolGroup: spawnRole === "execute-worker" ? "execute-worker" : "full-worker",
        delegation: leafPolicy, streaming: false, running: false, reportLog: [],
      };
      handle.rpcRegistry.set(agentId, dormant);
    }
    bridge.defaultSessionKeyRef.current = "lead-after-relogin";
    for (const agentId of ["pre-fix-execute", "pre-fix-worker"]) {
      await tools.get("ws-agent-send").execute("send", { agent_id: agentId, message: "resume" });
    }
    assert.equal(launched.length, 2);
    for (const policy of launched) {
      assert.equal(policy.parentSessionKey, "lead-after-relogin");
      assert.equal(policy.sessionKey, undefined);
      assert.equal(policy.authority, "leaf", "only the parent key is re-stamped");
    }
    assert.equal(handle.rpcRegistry.get("pre-fix-worker")!.delegation?.parentSessionKey, undefined, "the stored record policy is left as spawned");
  } finally {
    await handle.stopAll();
    restore();
    rmSync(root, { recursive: true, force: true });
  }
});

test("a dormant fork relaunch keeps its policy without a parentSessionKey", async () => {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "ws-pi-relaunch-fork-")));
  const storage = createAgentStorageContext("relaunch-fork-owner", root);
  const registry = new Map<string, RpcAgentRecord>();
  const launched: DelegationPolicy[] = [];
  const context = { version: 1, kind: "task", effectiveSystemPrompt: "captured", activeTools: [], registeredTools: [] };
  const restore = patchRpc(async function (this: any) {
    launched.push(JSON.parse(this.options.env[DELEGATION_ENV]));
    const home = this.options.args[this.options.args.indexOf("--session-dir") + 1];
    this.sessionFile = join(home, "fork-child.jsonl");
    if (!existsSync(this.sessionFile)) writeFileSync(this.sessionFile, "offline fork session\n");
    await connectFakeChild(this.options.env, this.options.args, { fork: { sessionId: "child-id", sessionPath: this.sessionFile, ownSessionKey: "fork-child-key" } });
  }, {
    async getState(this: any) { return { sessionId: "child-id", sessionFile: this.sessionFile, model: { provider: "offline", id: "test" }, thinkingLevel: "off" }; },
  });
  const pi = { sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
  try {
    const result = await spawnAgent(registry, {
      storage, pi, cwd: root, inheritModel: "offline/test", catalog: [], wsToolNames: [], extensionPath: TEST_EXTENSION_ENTRY,
      client: {} as never, forkFrom: "/tmp/relaunch-parent.jsonl", spawnRole: "fork", forkContext: context as never, parentSessionKey: "lead-at-spawn",
    }, { prompt: "task" });
    const record = registry.get(result.agent_id)!;
    await record.client?.stop();
    record.client = undefined;
    await sendToAgent(registry, { cwd: root, pi, extensionPath: TEST_EXTENSION_ENTRY, parentSessionKey: "lead-now" }, result.agent_id, "resume");
    assert.equal(launched.length, 2);
    for (const policy of launched) {
      assert.equal(policy.authority, "lead");
      assert.equal(policy.parentSessionKey, undefined);
    }
  } finally {
    for (const record of registry.values()) record.ownershipObserverStop?.();
    restore();
    rmSync(root, { recursive: true, force: true });
  }
});

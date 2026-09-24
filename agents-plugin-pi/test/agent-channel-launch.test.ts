/**
 * Launch-path outcomes of the control channel's hello and readiness stages,
 * driven through the production `spawnAgent` / `sendToAgent` with an
 * in-process child (`connectFakeChild`) instead of a Pi process. Every case
 * asserts the exact error text a caller sees and the record's resting shape.
 */
import { afterEach, describe, test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import { createAgentStorageContext } from "../src/agent-storage.ts";
import { CHANNEL_PROTOCOL_VERSION, connectChannelEndpoint, readAndDeleteChannelBootstrap, type ChildChannel, type ParentChannel } from "../src/agent-channel.ts";
import { agentCostRefreshRef, heldPushQueue, leadIdleRef, sendToAgent, spawnAgent, stopAgent, type RpcAgentRecord } from "../src/spawner.ts";
import { DESCENDANT_USAGE_MESSAGE, descendantUsageReporterRef } from "../src/agent-usage-rollup.ts";
import { captureForkResume, rehydrateForkRecord, type PersistedForkResume } from "../src/ask.ts";
import { SubtreeUpstream } from "../src/subtree-lifecycle.ts";
import { closeFakeChildren, connectFakeChild, type FakeChildOptions } from "./fixtures/channel-child.ts";
import { until } from "./fixtures/subtree-channels.ts";

const PACKAGE_ROOT = dirname(dirname(fileURLToPath(import.meta.url)));
const EXTENSION_ENTRY = join(PACKAGE_ROOT, "src", "index.ts");
const EXPLORE_GUIDE = join(PACKAGE_ROOT, "explore-guide.md");
const FORK_CONTEXT = { version: 1, kind: "task", effectiveSystemPrompt: "captured", activeTools: [], registeredTools: [] };
const FORK_MISSING = "ws-pi-agent: fork did not publish readiness";
const WEB_MISSING = "web-search-tool-unavailable: Explore web facade readiness was not proved";

type StartHook = (this: { options?: { env?: Record<string, string>; args?: string[] } }) => Promise<void>;

const roots: string[] = [];
afterEach(() => { for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true }); });

/** RpcClient stand-in: `start()` runs the current hook; `getState()` satisfies both the fork and the Explore selection checks. */
function installRpcHarness() {
  const proto = RpcClient.prototype as any;
  const names = ["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"];
  const saved = Object.fromEntries(names.map(name => [name, proto[name]]));
  const state = { hook: fakeChild() as StartHook };
  Object.assign(proto, {
    async start(this: { options?: { env?: Record<string, string>; args?: string[] }; started?: boolean }) { await state.hook.call(this); this.started = true; },
    async stop() {}, async abort() {}, async setThinkingLevel() {},
    // As the real client: a request before `start()` has run throws.
    async prompt(this: { started?: boolean }) { if (!this.started) throw new Error("Client not started"); },
    onEvent() { return () => {}; },
    async getState(this: { options?: { args?: string[] } }) {
      const args = this.options?.args ?? [];
      const dir = args[args.indexOf("--session-dir") + 1];
      return { model: { provider: "pi", id: "small" }, thinkingLevel: "medium", sessionFile: `${dir}/session.jsonl`, sessionId: "fork-child-session-id" };
    },
  });
  return { state, restore() { closeFakeChildren(); Object.assign(proto, saved); } };
}

function fakeChild(opts: FakeChildOptions = {}): StartHook {
  return async function () { await connectFakeChild(this.options?.env, this.options?.args, opts); };
}

function contexts(registry: Map<string, RpcAgentRecord>, launch: Record<string, unknown>) {
  const root = mkdtempSync(join(tmpdir(), "ws-pi-channel-launch-"));
  roots.push(root);
  const pi = { sendMessage() {}, sendUserMessage() {} } as any;
  const base = { storage: createAgentStorageContext("launch-lead", root), pi, inheritModel: "pi/small", catalog: [], wsToolNames: [], extensionPath: EXTENSION_ENTRY, client: {} as any, channel: launch };
  return {
    fork: { spawn: { ...base, cwd: root, forkFrom: "/tmp/launch-parent.jsonl", spawnRole: "fork", forkContext: FORK_CONTEXT as any }, params: { prompt: "task" } },
    explore: { spawn: { ...base, cwd: PACKAGE_ROOT, toolGroup: "read-only-explore", spawnRole: "explore", exploreMode: "search" }, params: { systemPromptPath: EXPLORE_GUIDE, prompt: "question" } },
    resume: { ...base, cwd: root },
    registry,
    async dormant(role: "fork" | "explore"): Promise<RpcAgentRecord> {
      const { agent_id } = await spawnAgent(registry as any, this[role].spawn as any, this[role].params as any);
      await stopAgent(registry as any, agent_id, undefined, { silent: true });
      const record = registry.get(agent_id)!;
      assert.equal(record.client, undefined);
      return record;
    },
  };
}

function assertResting(record: RpcAgentRecord, channel: ParentChannel | undefined): void {
  assert.equal(record.client, undefined, "a failed launch leaves no live client");
  assert.equal(record.channel, undefined, "nor an open channel on the record");
  assert.equal(record.running, false);
  assert.ok(channel, "the launch bound a channel before starting the child");
  assert.equal(channel.closed, true, "the failed launch's channel is closed");
}

/** The channel of the launch in flight, captured from the record while the (patched) child starts, and when that start finished. */
function captureChannel(registry: Map<string, RpcAgentRecord>, hook: StartHook): { hook: StartHook; get(): ParentChannel | undefined; startedAt: number } {
  let channel: ParentChannel | undefined;
  const capture = {
    get: () => channel,
    startedAt: 0,
    hook: async function (this: { options?: { env?: Record<string, string>; args?: string[] } }) {
      channel = [...registry.values()].find(record => record.client === (this as unknown))?.channel;
      await hook.call(this);
      capture.startedAt = Date.now();
    },
  };
  return capture;
}

for (const path of ["spawn", "resume"] as const) {
  describe(`readiness and hello failures on the ${path} path`, () => {
    // A "fast" case runs under a readiness bound far beyond the test's own
    // timeout, so judging the published payload at once is proved by the test
    // finishing at all, not by a wall-clock margin that load can erode.
    const cases: Array<{ title: string; role: "fork" | "explore"; child: FakeChildOptions; message: string; fast?: boolean }> = [
      { title: "missing fork readiness fails with today's error", role: "fork", child: { fork: null }, message: FORK_MISSING },
      { title: "a fork readiness error is rejected at today's check", role: "fork", child: { fork: { error: "fork bootstrap did not issue a distinct current own key" } }, message: "ws-pi-agent: fork readiness rejected (fork bootstrap did not issue a distinct current own key)", fast: true },
      { title: "mismatched fork readiness is rejected at today's check", role: "fork", child: { fork: { sessionId: "another-session" } }, message: "ws-pi-agent: fork readiness rejected (key/session mismatch)", fast: true },
      { title: "missing web readiness fails with today's error", role: "explore", child: { web: null }, message: WEB_MISSING },
      { title: "invalid web readiness fails with today's error", role: "explore", child: { web: { tools: ["web_search"] } }, message: WEB_MISSING, fast: true },
      { title: "a web readiness proof failure published by the child fails at once with today's error", role: "explore", child: { web: { error: "web-search-tool-unavailable: Explore web facade registration mismatch" } }, message: WEB_MISSING, fast: true },
    ];
    for (const { title, role, child, message, fast } of cases) {
      test(title, { timeout: fast ? 30_000 : undefined }, async () => {
        const rpc = installRpcHarness();
        const registry = new Map<string, RpcAgentRecord>();
        const ctx = contexts(registry, { readinessTimeoutMs: fast ? 600_000 : 300 });
        try {
          const record = path === "resume" ? await ctx.dormant(role) : undefined;
          const capture = captureChannel(registry, fakeChild(child));
          rpc.state.hook = capture.hook;
          if (record) await assert.rejects(sendToAgent(registry as any, ctx.resume as any, record.agentId, "again"), { message });
          else await assert.rejects(spawnAgent(registry as any, ctx[role].spawn as any, ctx[role].params as any), { message });
          const elapsed = Date.now() - capture.startedAt;
          assertResting(record ?? [...registry.values()][0], capture.get());
          if (!fast) assert.ok(elapsed >= 300, `an absent payload waits out the readiness bound (${elapsed}ms)`);
        } finally { rpc.restore(); }
      });
    }

    test("a child that cannot reconnect before the readiness bound fails the launch with today's error", async () => {
      const rpc = installRpcHarness();
      const registry = new Map<string, RpcAgentRecord>();
      const ctx = contexts(registry, { readinessTimeoutMs: 600 });
      try {
        const record = path === "resume" ? await ctx.dormant("explore") : undefined;
        // A raw socket is opened before the drop and sends its authenticated
        // hello the instant the slot frees, so the real child's reconnect loop
        // only ever sees `busy`; the readiness it publishes meanwhile is stuck
        // on its side.
        const capture = captureChannel(registry, async function () {
          const boot = readAndDeleteChannelBootstrap({ ...(this.options?.env ?? {}) })!;
          const channel = capture.get()!;
          const child = await connectFakeChild(this.options?.env, this.options?.args, { web: null, reconnect: true, backoffCapMs: 50 });
          const raw = await connectChannelEndpoint(channel.endpoint);
          raw.unref();
          const welcomed = new Promise<string>(resolve => raw.once("data", data => resolve(String(data))));
          channel.onDisconnect(() => raw.write(JSON.stringify({ t: "hello", v: CHANNEL_PROTOCOL_VERSION, cred: boot.credential, gen: boot.generation, reconnect: true, resume: {} }) + "\n"));
          channel.live!.close();
          assert.match(await welcomed, /"t":"welcome"/);
          child!.publishReadiness("web", { tools: ["web_search", "ws_web_fetch"] });
        });
        rpc.state.hook = capture.hook;
        if (record) await assert.rejects(sendToAgent(registry as any, ctx.resume as any, record.agentId, "again"), { message: WEB_MISSING });
        else await assert.rejects(spawnAgent(registry as any, ctx.explore.spawn as any, ctx.explore.params as any), { message: WEB_MISSING });
        const channel = capture.get()!;
        assertResting(record ?? [...registry.values()][0], channel);
        assert.equal(channel.accepted, 2, "the real child's first hello and the squatter");
        assert.ok(channel.rejects.length >= 1 && channel.rejects.every(reason => reason === "busy"), `the child kept retrying into busy: ${channel.rejects.join(",")}`);
      } finally { rpc.restore(); }
    });

    test("a child that never says hello fails the launch at the hello bound", async () => {
      const rpc = installRpcHarness();
      const registry = new Map<string, RpcAgentRecord>();
      const ctx = contexts(registry, { helloTimeoutMs: 200 });
      try {
        const record = path === "resume" ? await ctx.dormant("fork") : undefined;
        const capture = captureChannel(registry, async () => {});
        rpc.state.hook = capture.hook;
        const message = "ws-pi-agent: child channel hello timed out after 200ms";
        if (record) await assert.rejects(sendToAgent(registry as any, ctx.resume as any, record.agentId, "again"), { message });
        else await assert.rejects(spawnAgent(registry as any, ctx.fork.spawn as any, ctx.fork.params as any), { message });
        assertResting(record ?? [...registry.values()][0], capture.get());
      } finally { rpc.restore(); }
    });
  });
}

test("a stop that lands while a dormant resume is still binding its channel wins: the child is never started", async () => {
  const rpc = installRpcHarness();
  const registry = new Map<string, RpcAgentRecord>();
  const ctx = contexts(registry, {});
  try {
    const record = await ctx.dormant("fork");
    let started = 0;
    rpc.state.hook = async () => { started += 1; };
    const resume = sendToAgent(registry as any, ctx.resume as any, record.agentId, "again");
    assert.ok(record.client, "the resume claims the record before its first await");
    await stopAgent(registry as any, record.agentId, undefined, { silent: true });
    await assert.rejects(resume, { message: "ws-pi-agent: launch stopped (ws-agent-stop) before the agent started" });
    assert.equal(started, 0);
    assert.equal(record.client, undefined);
    assert.equal(record.channel, undefined);
  } finally { rpc.restore(); }
});

describe("a stop during a launch is its single terminal", () => {
  /** Holds every lead push in the shared FIFO (a busy lead) so the test reads exactly what the lead would receive. */
  function captureLeadPushes(): { settled(): unknown[]; restore(): void } {
    const saved = leadIdleRef.current;
    leadIdleRef.current = () => false;
    heldPushQueue.length = 0;
    return {
      settled: () => heldPushQueue.flatMap(held => held.kind === "push" && held.family === "ws-agent-settled" ? [held.payload.reason] : []),
      restore() { heldPushQueue.length = 0; leadIdleRef.current = saved; },
    };
  }

  test("a stop during the resume's bind window pushes only the stop's own settle, no spawn-failed", async () => {
    const rpc = installRpcHarness();
    const registry = new Map<string, RpcAgentRecord>();
    const ctx = contexts(registry, {});
    const pushes = captureLeadPushes();
    try {
      const record = await ctx.dormant("fork");
      heldPushQueue.length = 0;
      const resume = sendToAgent(registry as any, ctx.resume as any, record.agentId, "again");
      assert.ok(record.client && record.launching, "the resume claimed the record and is binding");
      await stopAgent(registry as any, record.agentId, ctx.resume.pi);
      await assert.rejects(resume, { message: "ws-pi-agent: launch stopped (ws-agent-stop) before the agent started" });
      assert.deepEqual(pushes.settled(), ["stopped"]);
    } finally { pushes.restore(); rpc.restore(); }
  });

  test("a stop during the spawn's hello wait pushes only the stop's own settle, and the spawn call rejects as stopped", async () => {
    const rpc = installRpcHarness();
    const registry = new Map<string, RpcAgentRecord>();
    const ctx = contexts(registry, { helloTimeoutMs: 600_000 });
    const pushes = captureLeadPushes();
    try {
      let stopped: Promise<unknown> | undefined;
      // The child process "starts" but never says hello; the stop lands while the parent waits for it.
      rpc.state.hook = async function () {
        const record = [...registry.values()].find(candidate => candidate.client === (this as unknown))!;
        // After the start resolves, the parent's hello wait is already armed by the time this runs.
        setImmediate(() => { stopped = stopAgent(registry as any, record.agentId, ctx.resume.pi); });
      };
      await assert.rejects(spawnAgent(registry as any, ctx.fork.spawn as any, ctx.fork.params as any), { message: "ws-pi-agent: launch stopped (ws-agent-stop) before the agent started" });
      await stopped;
      assert.deepEqual(pushes.settled(), ["stopped"]);
      const record = [...registry.values()][0]!;
      assert.equal(record.client, undefined);
      assert.equal(record.channel, undefined);
    } finally { pushes.restore(); rpc.restore(); }
  });

  for (const path of ["spawn", "resume"] as const) {
    test(`a genuine hello failure on the ${path} path still pushes spawn-failed`, async () => {
      const rpc = installRpcHarness();
      const registry = new Map<string, RpcAgentRecord>();
      const ctx = contexts(registry, { helloTimeoutMs: 100 });
      const pushes = captureLeadPushes();
      try {
        const record = path === "resume" ? await ctx.dormant("fork") : undefined;
        heldPushQueue.length = 0;
        rpc.state.hook = async () => {};
        const message = "ws-pi-agent: child channel hello timed out after 100ms";
        if (record) await assert.rejects(sendToAgent(registry as any, ctx.resume as any, record.agentId, "again"), { message });
        else await assert.rejects(spawnAgent(registry as any, ctx.fork.spawn as any, ctx.fork.params as any), { message });
        assert.deepEqual(pushes.settled(), ["spawn-failed"]);
      } finally { pushes.restore(); rpc.restore(); }
    });
  }
});

test("a second send during a dormant resume's bind waits for that launch: one child, both sends delivered", async () => {
  const rpc = installRpcHarness();
  const registry = new Map<string, RpcAgentRecord>();
  const ctx = contexts(registry, {});
  try {
    const record = await ctx.dormant("fork");
    let started = 0;
    const base = fakeChild();
    rpc.state.hook = async function () { started += 1; await base.call(this); };
    const first = sendToAgent(registry as any, ctx.resume as any, record.agentId, "first");
    assert.ok(record.client && record.launching, "the resume claims the record and marks its launch before its first await");
    const second = sendToAgent(registry as any, ctx.resume as any, record.agentId, "second");
    assert.deepEqual(await first, { agent_id: record.agentId });
    assert.deepEqual(await second, { agent_id: record.agentId }, "the second send delivered to the started child instead of failing on an unstarted one");
    assert.equal(started, 1, "exactly one child process for one session");
    assert.equal(record.launchGeneration, 2);
    assert.equal(record.launching, undefined);
    assert.ok(record.client && record.channel?.live, "the single launch is live");
  } finally { rpc.restore(); }
});

test("a send that arrives while a resume is failing waits, then relaunches on the dormant record", async () => {
  const rpc = installRpcHarness();
  const registry = new Map<string, RpcAgentRecord>();
  const ctx = contexts(registry, { helloTimeoutMs: 150 });
  try {
    const record = await ctx.dormant("fork");
    const hooks: StartHook[] = [async () => {}, fakeChild()];
    rpc.state.hook = async function () { await hooks.shift()!.call(this); };
    const first = sendToAgent(registry as any, ctx.resume as any, record.agentId, "first");
    const second = sendToAgent(registry as any, ctx.resume as any, record.agentId, "second");
    await assert.rejects(first, { message: "ws-pi-agent: child channel hello timed out after 150ms" });
    assert.deepEqual(await second, { agent_id: record.agentId }, "the waiting send found the record dormant and relaunched it");
    assert.equal(record.launchGeneration, 3);
    assert.ok(record.client && record.channel?.live);
  } finally { rpc.restore(); }
});

for (const path of ["spawn", "resume"] as const) {
  test(`the ${path} path attaches its event listener before the first prompt: the launch's first agent_start is observed`, async () => {
    const rpc = installRpcHarness();
    const proto = RpcClient.prototype as any;
    const { onEvent, prompt } = proto;
    const promptedWith: number[] = [];
    Object.assign(proto, {
      onEvent(this: { listeners?: Array<(event: unknown) => void> }, fn: (event: unknown) => void) {
        (this.listeners ??= []).push(fn);
        return () => {};
      },
      // As Pi: the turn's agent_start can be the first stdout line after the prompt is sent.
      async prompt(this: { started?: boolean; listeners?: Array<(event: unknown) => void> }) {
        if (!this.started) throw new Error("Client not started");
        promptedWith.push(this.listeners?.length ?? 0);
        for (const fn of this.listeners ?? []) fn({ type: "agent_start" });
      },
    });
    const registry = new Map<string, RpcAgentRecord>();
    const ctx = contexts(registry, {});
    try {
      const record = path === "resume" ? await ctx.dormant("fork") : undefined;
      promptedWith.length = 0;
      if (record) await sendToAgent(registry as any, ctx.resume as any, record.agentId, "again");
      else await spawnAgent(registry as any, ctx.fork.spawn as any, ctx.fork.params as any);
      const launched = record ?? [...registry.values()][0]!;
      assert.equal(promptedWith.length, 1);
      assert.ok(promptedWith[0]! >= 1, "a listener was attached when the first prompt went out");
      assert.equal(launched.streaming, true, "the first turn's start reached the record");
    } finally {
      Object.assign(proto, { onEvent, prompt });
      rpc.restore();
    }
  });
}

describe("descendant-usage reports over the launch's channel", () => {
  const usage = { knownUsd: 1.25, knownContributors: 2, unknownContributors: 0, descendants: 3 };

  /**
   * Sends one report from the child side of `record`'s current launch and
   * returns the spy trail: the footer refresh (`agentCostRefreshRef`) and this
   * hop's own reporter (evaluation point 2), in call order.
   */
  async function report(record: RpcAgentRecord, child: ChildChannel): Promise<string[]> {
    const savedRefresh = agentCostRefreshRef.current, savedReporter = descendantUsageReporterRef.current;
    const calls: string[] = [];
    agentCostRefreshRef.current = () => { calls.push("refresh"); };
    descendantUsageReporterRef.current = { setSource() {}, evaluate() { calls.push("evaluate"); } };
    try {
      await new Promise(resolve => setImmediate(resolve));
      calls.length = 0;
      child.send({ t: DESCENDANT_USAGE_MESSAGE, seq: 1, usage });
      const deadline = Date.now() + 2_000;
      while (!record.descendantUsage && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 10));
      await new Promise(resolve => setImmediate(resolve));
      return [...calls];
    } finally {
      agentCostRefreshRef.current = savedRefresh;
      descendantUsageReporterRef.current = savedReporter;
    }
  }

  test("spawn: a report from the child updates the record, refreshes the footer, and re-evaluates this hop's own report", async () => {
    const rpc = installRpcHarness();
    const registry = new Map<string, RpcAgentRecord>();
    const ctx = contexts(registry, {});
    try {
      let child: ChildChannel | undefined;
      rpc.state.hook = async function () { child = await connectFakeChild(this.options?.env, this.options?.args); };
      const { agent_id } = await spawnAgent(registry as any, ctx.fork.spawn as any, ctx.fork.params as any);
      const record = registry.get(agent_id)!;
      assert.ok(child && record.channel?.live, "precondition: the fake child holds the launch's channel");
      const calls = await report(record, child);
      assert.deepEqual(record.descendantUsage, usage);
      assert.deepEqual(record.descendantUsageOrder, { generation: record.launchGeneration, seq: 1 });
      assert.deepEqual(calls, ["refresh", "evaluate"]);
    } finally { rpc.restore(); }
  });

  test("dormant relaunch: the new launch's channel is wired the same way", async () => {
    const rpc = installRpcHarness();
    const registry = new Map<string, RpcAgentRecord>();
    const ctx = contexts(registry, {});
    try {
      const record = await ctx.dormant("fork");
      let child: ChildChannel | undefined;
      rpc.state.hook = async function () { child = await connectFakeChild(this.options?.env, this.options?.args); };
      await sendToAgent(registry as any, ctx.resume as any, record.agentId, "again");
      assert.equal(record.launchGeneration, 2);
      assert.ok(child && record.channel?.live, "precondition: the fake child holds the relaunch's channel");
      const calls = await report(record, child);
      assert.deepEqual(record.descendantUsage, usage);
      assert.deepEqual(record.descendantUsageOrder, { generation: 2, seq: 1 });
      assert.deepEqual(calls, ["refresh", "evaluate"]);
    } finally { rpc.restore(); }
  });
});

test("an older fork resume that still carries subtreeChannel relaunches with a fresh channel and reports subtree state over it", async () => {
  const rpc = installRpcHarness();
  const registry = new Map<string, RpcAgentRecord>();
  const ctx = contexts(registry, {});
  try {
    const dormant = await ctx.dormant("fork");
    const delegation = { version: 1, depth: 1, maxDepth: 2, authority: "lead", tools: ["ws-agent-spawn"] };
    const older = {
      ...captureForkResume(dormant),
      delegation, waitingOnChildren: true,
      subtreeChannel: { path: join(dirname(dormant.sessionPath), "subtree.json"), nonce: "retired" },
    } as unknown as PersistedForkResume;
    const revived = rehydrateForkRecord(dormant.agentId, older);
    registry.set(revived.agentId, revived);
    let child: ChildChannel | undefined;
    rpc.state.hook = async function () { child = await connectFakeChild(this.options?.env, this.options?.args, { subtree: null }); };

    await sendToAgent(registry as any, ctx.resume as any, revived.agentId, "again");
    assert.ok(child && revived.channel?.live, "the relaunch bound a fresh channel the child holds");
    assert.equal(revived.channel.generation, revived.launchGeneration, "under the relaunch's own generation");
    assert.equal("subtreeChannel" in revived, false);
    assert.equal(revived.waitingOnChildren, true, "no snapshot over the fresh channel yet reads waiting");

    const upstream = new SubtreeUpstream(child);
    upstream.publish({ outstanding: 0, active: 1, deliveries: 0, delegated: true, turnOwed: false, turnsStarted: 0, descendants: [{ id: "grandchild", parentId: null, depth: 0, role: "worker", live: true }] });
    await until(() => revived.subtreeRevision === 1, "the busy snapshot");
    assert.equal(revived.waitingOnChildren, true);
    assert.deepEqual(revived.subtreeDescendants?.map(row => row.id), ["grandchild"]);
    upstream.publish({ outstanding: 0, active: 0, deliveries: 0, delegated: true, turnOwed: false, turnsStarted: 0, descendants: [] });
    await until(() => revived.subtreeRevision === 2, "the quiescent snapshot");
    assert.equal(revived.waitingOnChildren, false, "the fresh channel's state drives the record");
  } finally { rpc.restore(); }
});

test("concurrent spawns never share a generated alias: the guards and the registration are one synchronous step after the bind", async () => {
  const rpc = installRpcHarness();
  const registry = new Map<string, RpcAgentRecord>();
  const ctx = contexts(registry, {});
  try {
    const spawn = { ...ctx.explore.spawn, aliasPrefix: "explore" };
    const [a, b] = await Promise.all([
      spawnAgent(registry as any, spawn as any, ctx.explore.params as any),
      spawnAgent(registry as any, spawn as any, ctx.explore.params as any),
    ]);
    assert.deepEqual([a.alias, b.alias].sort(), ["explore-1", "explore-2"]);
    assert.equal(registry.size, 2);
  } finally { rpc.restore(); }
});

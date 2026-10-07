/**
 * The lead's child-launch path (261007): the per-registry preflight
 * (`ensureChildRuntime`) runs before every fresh spawn and dormant resume,
 * and a child's error-level bootstrap notify is carried into the launch
 * error the parent reports. Driven through the production `spawnAgent` /
 * `sendToAgent` with a patched `RpcClient`, plus one real child process for
 * the end-to-end bootstrap-failure regression.
 */
import { afterEach, describe, test } from "node:test";
import assert from "node:assert/strict";
import type { ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import { createAgentStorageContext } from "../src/agent-storage.ts";
import { registerChildLaunchPreflight, sendToAgent, spawnAgent, stopAgent, type RpcAgentRecord } from "../src/spawner.ts";
import { closeFakeChildren, connectFakeChild } from "./fixtures/channel-child.ts";

const PACKAGE_ROOT = dirname(dirname(fileURLToPath(import.meta.url)));
const EXTENSION_ENTRY = join(PACKAGE_ROOT, "src", "index.ts");
const FAIL_CHILD = join(PACKAGE_ROOT, "test", "fixtures", "bootstrap-fail-child.ts");
const FORK_CONTEXT = { version: 1, kind: "task", effectiveSystemPrompt: "captured", activeTools: [], registeredTools: [] };
const EXITED = "Agent process exited (code=1 signal=null). Stderr: ";

type Self = { options?: { env?: Record<string, string>; args?: string[] }; started?: boolean; listeners?: Array<(event: unknown) => void> };
type Hook = (this: Self) => Promise<void>;

const roots: string[] = [];
afterEach(() => { for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true }); });

function emit(self: Self, event: unknown): void {
  for (const listener of self.listeners ?? []) listener(event);
}

function notifyEvent(message: string, notifyType: string) {
  return { type: "extension_ui_request", id: `n-${message}`, method: "notify", message, notifyType };
}

/** RpcClient stand-in recording each start's env; `onEvent` listeners are kept so a hook can emit child events. */
function installRpcHarness() {
  const proto = RpcClient.prototype as any;
  const names = ["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"];
  const saved = Object.fromEntries(names.map(name => [name, proto[name]]));
  const state = {
    hook: (async function (this: Self) { await connectFakeChild(this.options?.env, this.options?.args); }) as Hook,
    starts: [] as Array<Record<string, string>>,
    prompt: async () => {},
  };
  Object.assign(proto, {
    async start(this: Self) { state.starts.push({ ...(this.options?.env ?? {}) }); await state.hook.call(this); this.started = true; },
    async stop() {}, async abort() {},
    async setThinkingLevel() {},
    async prompt(this: Self) { if (!this.started) throw new Error("Client not started"); await state.prompt(); },
    onEvent(this: Self, listener: (event: unknown) => void) {
      (this.listeners ??= []).push(listener);
      return () => { this.listeners = this.listeners!.filter(l => l !== listener); };
    },
    async getState(this: Self) {
      const args = this.options?.args ?? [];
      const dir = args[args.indexOf("--session-dir") + 1];
      return { model: { provider: "pi", id: "small" }, thinkingLevel: "medium", sessionFile: `${dir}/session.jsonl`, sessionId: "fork-child-session-id" };
    },
  });
  return { state, restore() { closeFakeChildren(); Object.assign(proto, saved); } };
}

function contexts() {
  const root = mkdtempSync(join(tmpdir(), "ws-pi-child-launch-guard-"));
  roots.push(root);
  const pi = { sendMessage() {}, sendUserMessage() {} } as any;
  const base = { storage: createAgentStorageContext("guard-lead", root), pi, inheritModel: "pi/small", catalog: [], wsToolNames: [], extensionPath: EXTENSION_ENTRY, client: {} as any };
  const registry = new Map<string, RpcAgentRecord>();
  const spawn = { ...base, cwd: root, forkFrom: "/tmp/guard-parent.jsonl", spawnRole: "fork", forkContext: FORK_CONTEXT as any };
  return {
    registry,
    spawn: () => spawnAgent(registry as any, spawn as any, { prompt: "task" }),
    resume: (agentId: string) => sendToAgent(registry as any, { ...base, cwd: root } as any, agentId, "again"),
    async dormant(): Promise<RpcAgentRecord> {
      const { agent_id } = await this.spawn();
      await stopAgent(registry as any, agent_id, undefined, { silent: true });
      const record = registry.get(agent_id)!;
      assert.equal(record.client, undefined);
      return record;
    },
  };
}

describe("child-launch preflight (lead re-bootstrap on runtime.json drift)", () => {
  for (const path of ["spawn", "resume"] as const) {
    test(`${path}: the registered preflight runs once, before the child starts, and the child env still blanks the bootstrap overrides`, async () => {
      const rpc = installRpcHarness();
      const ctx = contexts();
      try {
        const record = path === "resume" ? await ctx.dormant() : undefined;
        const startsBefore = rpc.state.starts.length;
        const order: string[] = [];
        registerChildLaunchPreflight(ctx.registry as any, async () => { order.push(`preflight@${rpc.state.starts.length - startsBefore}`); });
        if (record) await ctx.resume(record.agentId);
        else await ctx.spawn();
        assert.deepEqual(order, ["preflight@0"], "awaited once, with no child started yet");
        const env = rpc.state.starts.at(-1)!;
        assert.equal(env.WS_MCP_BOOTSTRAP_BINARY, "");
        assert.equal(env.WS_MCP_BOOTSTRAP_URL, "");
      } finally { rpc.restore(); }
    });

    test(`${path}: a failing preflight fails the launch with its error and starts no child`, async () => {
      const rpc = installRpcHarness();
      const ctx = contexts();
      try {
        const record = path === "resume" ? await ctx.dormant() : undefined;
        const startsBefore = rpc.state.starts.length;
        registerChildLaunchPreflight(ctx.registry as any, async () => { throw new Error("reinstall failed"); });
        if (record) await assert.rejects(ctx.resume(record.agentId), { message: "reinstall failed" });
        else await assert.rejects(ctx.spawn(), { message: "reinstall failed" });
        assert.equal(rpc.state.starts.length, startsBefore, "no child process was started");
        if (record) {
          assert.equal(record.client, undefined);
          assert.equal(record.running, false);
        } else {
          assert.equal(ctx.registry.size, 0, "a fresh spawn fails before any record is registered");
        }

        // Not memoized here either: once the preflight passes, the same record launches.
        registerChildLaunchPreflight(ctx.registry as any, async () => {});
        if (record) await ctx.resume(record.agentId);
        else await ctx.spawn();
        assert.equal(rpc.state.starts.length, startsBefore + 1);
      } finally { rpc.restore(); }
    });
  }

  test("with no preflight registered the launch path is unchanged", async () => {
    const rpc = installRpcHarness();
    const ctx = contexts();
    try {
      registerChildLaunchPreflight(ctx.registry as any, async () => { throw new Error("should not run"); });
      registerChildLaunchPreflight(ctx.registry as any, undefined);
      await ctx.spawn();
      assert.equal(rpc.state.starts.length, 1);
    } finally { rpc.restore(); }
  });
});

describe("child bootstrap failure reaches the parent's launch error", () => {
  for (const path of ["spawn", "resume"] as const) {
    test(`${path}: an exit surfacing at start carries the most recent error-level notify`, async () => {
      const rpc = installRpcHarness();
      const ctx = contexts();
      try {
        const record = path === "resume" ? await ctx.dormant() : undefined;
        rpc.state.hook = async function () {
          emit(this, notifyEvent("older failure", "error"));
          emit(this, notifyEvent("ws-pi-agent: session bootstrap failed (version mismatch)", "error"));
          emit(this, notifyEvent("informational", "info"));
          throw new Error(EXITED);
        };
        const message = `${EXITED}\nChild reported: ws-pi-agent: session bootstrap failed (version mismatch)`;
        if (record) await assert.rejects(ctx.resume(record.agentId), { message });
        else await assert.rejects(ctx.spawn(), { message });
      } finally { rpc.restore(); }
    });

    test(`${path}: an exit surfacing at the first prompt, after hello, is decorated the same way`, async () => {
      const rpc = installRpcHarness();
      const ctx = contexts();
      try {
        const record = path === "resume" ? await ctx.dormant() : undefined;
        rpc.state.hook = async function () {
          await connectFakeChild(this.options?.env, this.options?.args);
          emit(this, notifyEvent("ws-pi-agent: session bootstrap failed (late)", "error"));
        };
        rpc.state.prompt = async () => { throw new Error(EXITED); };
        const message = `${EXITED}\nChild reported: ws-pi-agent: session bootstrap failed (late)`;
        if (record) await assert.rejects(ctx.resume(record.agentId), { message });
        else await assert.rejects(ctx.spawn(), { message });
      } finally { rpc.restore(); }
    });
  }

  test("a launch failure with no error-level notify keeps its original error", async () => {
    const rpc = installRpcHarness();
    const ctx = contexts();
    try {
      rpc.state.hook = async function () { emit(this, notifyEvent("just info", "info")); throw new Error(EXITED); };
      await assert.rejects(ctx.spawn(), { message: EXITED });
    } finally { rpc.restore(); }
  });

  test("regression: a real child whose bootstrap fails exits fail-closed and the parent's launch error carries its notify", { timeout: 60_000 }, async () => {
    const proto = RpcClient.prototype as any;
    const originalStart = proto.start;
    const children: ChildProcess[] = [];
    proto.start = async function (this: { options: { cliPath?: string; env?: Record<string, string> }; process?: ChildProcess }) {
      this.options.cliPath = FAIL_CHILD;
      // Large enough that the pipe write cannot finish before exit without the drain.
      this.options.env = { ...this.options.env, WS_PI_FAKE_BOOTSTRAP_FAIL_CHILD: "1", FAKE_NOTIFY_PAD: String(256 * 1024) };
      try { await originalStart.call(this); } finally { if (this.process) children.push(this.process); }
    };
    const ctx = contexts();
    try {
      const error = await ctx.spawn().then(() => undefined, (err: unknown) => err as Error);
      assert.ok(error, "the launch fails");
      assert.match(error.message, /Agent process exited \(code=1/);
      assert.match(error.message, /\nChild reported: ws-pi-agent: session bootstrap failed — this session has no ws-mcp bridge or custom tools \(simulated ws-mcp bootstrap failure x+\)$/);
      assert.equal(children.length, 1);
      assert.equal(children[0].exitCode, 1, "the child exited fail-closed instead of running toolless");
      const record = [...ctx.registry.values()][0];
      assert.equal(record.client, undefined);
    } finally {
      proto.start = originalStart;
      closeFakeChildren();
    }
  });
});

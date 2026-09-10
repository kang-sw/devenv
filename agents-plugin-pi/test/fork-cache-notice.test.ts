import { existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createAgentStorageContext, readOwnership } from "../src/agent-storage.ts";
import { test } from "node:test";
import assert from "node:assert/strict";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import { attachFirstTaskForkCacheNotice, formatFirstForkCacheNotice } from "../src/fork-cache-notice.ts";
import { spawnAgent, sendToAgent } from "../src/spawner.ts";
import { buildForkSpawnCtx } from "../src/fork.ts";
import { readForkLaunchContext, writePrivateJson } from "../src/fork-context.ts";

const message = (usage: unknown = { input: 189, cacheRead: 73344 }, stopReason = "toolUse") => ({ role: "assistant", content: [], usage, stopReason });
const event = (m = message()) => ({ type: "message_end", message: m });
const context = { version: 1, kind: "task", effectiveSystemPrompt: "captured", activeTools: [], registeredTools: [] };
const TEST_EXTENSION_ENTRY = "/tmp/loaded ws adapter/index copy.ts";
function harness(recordOverrides = {}, ownerOverrides = {}) {
  const notices: string[] = [];
  const listeners: ((e: any) => void)[] = [];
  const client = { onEvent: (fn: (e: any) => void) => { listeners.push(fn); return () => {}; } };
  const record = { agentId: "abcdefgh-more", alias: "cache-task", spawnRole: "fork", forkContext: context, ...recordOverrides } as any;
  const owner = { mode: "tui", hasUI: true, ui: { notify: (text: string) => notices.push(text) }, ...ownerOverrides } as any;
  attachFirstTaskForkCacheNotice(record, client as any, owner);
  return { notices, listeners, record, emit: (e: any) => listeners.forEach(fn => fn(e)) };
}

test("cache notice uses input + cacheRead, not output/cacheWrite/totalTokens", () => {
  assert.equal(formatFirstForkCacheNotice("task", message({ input: 189, cacheRead: 73344, cacheWrite: 9000, output: 900, totalTokens: 999999 })), "fork task · first response cache 99.7% (73,344 / 73,533 tokens)");
  assert.match(formatFirstForkCacheNotice("task", message({ input: 3, cacheRead: 1 })), /25.0% \(1 \/ 4 tokens\)/);
});

test("zero, full, tiny and nearly full ratios do not mislabel partial hits", () => {
  for (const [input, cacheRead, expected] of [[1, 0, "0.0%"], [0, 1, "100.0%"], [100000, 1, "<0.1%"], [1, 100000, ">99.9%"]] as const) {
    assert.ok(formatFirstForkCacheNotice("task", message({ input, cacheRead })).includes(expected));
  }
});

test("missing, invalid, zero-total, overflow, failed and aborted usage is unknown", () => {
  for (const usage of [undefined, null, {}, { input: 0, cacheRead: 0 }, { input: -1, cacheRead: 1 }, { input: 1, cacheRead: -1 }, { input: "1", cacheRead: 1 }, { input: 1, cacheRead: "1" }, { input: NaN, cacheRead: 1 }, { input: 1, cacheRead: NaN }, { input: Infinity, cacheRead: 1 }, { input: 1, cacheRead: Infinity }, { input: Number.MAX_VALUE, cacheRead: Number.MAX_VALUE }]) {
    assert.match(formatFirstForkCacheNotice("task", { ...message(), usage }), /cache unknown$/);
  }
  for (const stopReason of ["error", "aborted", "pending", "", undefined]) {
    assert.match(formatFirstForkCacheNotice("task", { ...message(), stopReason }), /cache unknown$/);
  }
});

test("first NEW assistant only: ignores history containers, deltas, tools and report followups", () => {
  const h = harness(); const before = JSON.stringify(h.record);
  h.emit({ type: "agent_end", messages: [message()] });
  h.emit({ type: "message_update", message: message() });
  h.emit({ type: "message_end", message: { role: "toolResult" } });
  assert.deepEqual(h.notices, []);
  h.emit(event());
  h.emit({ type: "tool_execution_start", toolName: "ws-report-to-lead", args: { kind: "final" } });
  h.emit(event());
  assert.equal(h.notices.length, 1);
  assert.equal(JSON.stringify(h.record), before, "no durable first-response flag or lineage change");
});

test("failed first response consumes the one cosmetic notice", () => {
  const h = harness(); h.emit(event(message(undefined, "error"))); h.emit(event());
  assert.deepEqual(h.notices, ["fork cache-task · first response cache unknown"]);
});

test("discussion, legacy, worker and exploration roles never subscribe", () => {
  for (const overrides of [{ spawnRole: "worker" }, { spawnRole: "execute-worker" }, { spawnRole: "explore" }, { spawnRole: undefined }, { forkContext: { ...context, kind: "discussion" } }, { forkContext: undefined }]) {
    assert.equal(harness(overrides).listeners.length, 0);
  }
});

test("only the owner's TUI: RPC hasUI is insufficient, absent UI stays quiet", () => {
  for (const overrides of [{ mode: "rpc" }, { mode: "print" }, { mode: "json" }, { mode: undefined }, { hasUI: false }, { ui: undefined }, { ui: {} }]) {
    assert.equal(harness({}, overrides).listeners.length, 0);
  }
  attachFirstTaskForkCacheNotice({ spawnRole: "fork", forkContext: context } as any, { onEvent: () => assert.fail("no UI") } as any, undefined);
});

test("throwing UI cannot break later response/report listeners or re-notify", () => {
  let attempts = 0; let processed = 0;
  const h = harness({}, { ui: { notify: () => { attempts++; throw Error("closed UI"); } } });
  h.listeners.push(() => { processed++; });
  assert.doesNotThrow(() => { h.emit(event()); h.emit(event()); });
  assert.equal(attempts, 1); assert.equal(processed, 2);
});

test("label uses title/id fallback and remains one line", () => {
  const h = harness({ alias: undefined, title: "tiny\ntask" }); h.emit(event());
  assert.match(h.notices[0], /^fork tiny task ·/);
  const fallback = harness({ alias: undefined }); fallback.emit(event());
  assert.match(fallback.notices[0], /^fork abcdefgh ·/);
});

test("task spawn context carries only an ephemeral parent UI reference", () => {
  const owner = { mode: "tui", hasUI: true, ui: { notify() {} } } as any;
  const built = buildForkSpawnCtx({} as any, { client: {}, wsToolNames: [], defaultSessionKeyRef: {} } as any, { cwd: "/tmp" }, { forkFrom: "/tmp/source", explicitTools: "", catalog: [], forkCacheNoticeOwner: owner });
  assert.equal(built.forkCacheNoticeOwner, owner);
});

test("production initial spawn attaches before prompt; live/dormant sends never re-arm or wake lead", async (t) => {
  const root = mkdtempSync(join(tmpdir(), "ws-pi-cache-notice-test-"));
  const storage = createAgentStorageContext("notice-owner", root);
  const registry = new Map();
  const diagnostics = t.mock.method(console, "error", () => {});
  const notices: string[] = [];
  const owner = { mode: "tui", hasUI: true, ui: { notify: (s: string) => notices.push(s) } } as any;
  const pi = { sendMessage: () => assert.fail("cosmetic message push"), sendUserMessage: () => assert.fail("cosmetic lead wake") } as any;
  const proto = RpcClient.prototype as any;
  const names = ["start", "stop", "onEvent", "getState", "prompt", "setThinkingLevel"];
  const saved = Object.fromEntries(names.map(n => [n, proto[n]]));
  Object.assign(proto, {
    async start() {
      this.listeners = [];
      const envelope = readForkLaunchContext(this.options.env)!;
      const home = this.options.args[this.options.args.indexOf("--session-dir") + 1];
      this.sessionFile = join(home, "notice-child.jsonl");
      const metadata = readOwnership(home)!;
      if (!existsSync(this.sessionFile)) {
        assert.equal(metadata.liveness.lifecycle, "starting", "pre-start observer must not downgrade a new spawn");
        assert.equal(metadata.sessionSignature, undefined);
      }
      assert.equal(diagnostics.mock.callCount(), 0, "pre-start observer must not report an expected missing file");
      writeFileSync(this.sessionFile, "offline fork session\n");
      writePrivateJson(envelope.readinessPath, { nonce: envelope.nonce, sessionId: "child-id", sessionPath: this.sessionFile, ownSessionKey: "notice-child-key", activeTools: [], registeredTools: [] });
    },
    async stop() {}, async setThinkingLevel() {},
    onEvent(fn: (e: unknown) => void) { this.listeners.push(fn); return () => {}; },
    async getState() { return { sessionId: "child-id", sessionFile: this.sessionFile, model: { provider: "offline", id: "test" }, thinkingLevel: "off" }; },
    async prompt() { for (const fn of this.listeners) fn(event()); },
  });
  try {
    const result = await spawnAgent(registry, { storage, pi, cwd: root, inheritModel: "offline/test", catalog: [], wsToolNames: [], extensionPath: TEST_EXTENSION_ENTRY, client: {} as any, forkFrom: "/tmp/notice-parent.jsonl", spawnRole: "fork", forkContext: context as any, forkCacheNoticeOwner: owner }, { prompt: "task", alias: "cache-task" });
    assert.equal(notices.length, 1, "the very first prompt response is observed");
    await sendToAgent(registry, { cwd: "/tmp", pi, extensionPath: TEST_EXTENSION_ENTRY }, result.agent_id, "followup");
    assert.equal(notices.length, 1);
    const record = registry.get(result.agent_id)!;
    assert.equal(Object.hasOwn(record, "forkCacheNoticeOwner"), false);
    record.client = undefined;
    await sendToAgent(registry, { cwd: "/tmp", pi, extensionPath: TEST_EXTENSION_ENTRY }, result.agent_id, "resume");
    assert.equal(notices.length, 1, "new resume client has no cosmetic listener");
    assert.equal(diagnostics.mock.callCount(), 0);
  } finally {
    for (const record of registry.values()) record.ownershipObserverStop?.();
    Object.assign(proto, saved);
    rmSync(root, { recursive: true, force: true });
  }
});

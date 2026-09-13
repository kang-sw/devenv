import { afterEach, test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import {
  CHILD_MANAGEMENT_TOOLS,
  type DelegationPolicy,
  childPolicy,
  readOnlyWsTools,
} from "../src/delegation-policy.ts";
import {
  beginSubtreeDispatch,
  installSubtreePublisher,
  readSubtreeSnapshot,
  subtreeOutstanding,
  subtreeWaiting,
} from "../src/subtree-lifecycle.ts";
import {
  applyRpcEvent,
  attachEventListener,
  flushHeldPushes,
  hasRunningAgents,
  heldPushQueue,
  leadIdleRef,
  leadWakeStartPendingRef,
  listAgents,
  OWNER_TERMINAL_RETRY_DELAY_MS,
  ownerNotifyRef,
  probeAgentLiveness,
  promptAgent,
  registerPushFlush,
  resolveTools,
  startForkFinish,
  stopAgent,
  type RpcAgentRecord,
  type RpcAgentRegistry,
} from "../src/spawner.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";

const dirs: string[] = [];
function home(): string {
  const dir = mkdtempSync(join(tmpdir(), "ws-subtree-test-"));
  dirs.push(dir);
  return dir;
}
afterEach(() => {
  for (const dir of dirs.splice(0)) rmSync(dir, { recursive: true, force: true });
  heldPushQueue.length = 0;
  leadIdleRef.current = undefined;
  leadWakeStartPendingRef.current = false;
  ownerNotifyRef.current = undefined;
});

const full = resolveTools("full-worker", ["ws__playbook_render", "ws__git_diff", "ws__git_commit", "ws__ferrule"]).split(",");
const root: DelegationPolicy = { version: 1, depth: 0, maxDepth: 2, authority: "lead", tools: ["ws-agent-spawn"] };
const worker = (): DelegationPolicy => childPolicy(root, full, "lead");
function record(id: string, overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return {
    agentId: id,
    sessionPath: join(home(), "session.jsonl"),
    systemPromptPath: "prompt.md",
    wsToolNames: [],
    toolGroup: "full-worker",
    streaming: false,
    running: false,
    reportLog: [],
    ...overrides,
  };
}
function capturePush(sent: unknown[], message: unknown): void {
  const batch = message as { customType?: string; details?: { items?: unknown[] } };
  if (batch.customType === PUSH_BATCH_CUSTOM_TYPE && Array.isArray(batch.details?.items)) sent.push(...batch.details.items);
  else sent.push(message);
}
function pushHarness(sent: unknown[]) {
  let listener: ((event: unknown) => void) | undefined;
  let last = "settled answer";
  let stops = 0;
  const client = {
    onEvent(fn: (event: unknown) => void) { listener = fn; return () => {}; },
    getLastAssistantText: async () => last,
    getState: async () => ({}),
    abort: async () => {},
    prompt: async () => {},
    followUp: async () => {},
    steer: async () => {},
    stop: async () => { stops += 1; },
  } as unknown as RpcClient;
  const pi = { on: () => {}, sendMessage: (message: unknown) => capturePush(sent, message) } as never;
  leadIdleRef.current = () => false;
  registerPushFlush(pi, { delayMs: () => 10 });
  return { client, pi, emit: (event: unknown) => listener?.(event), setLast: (v: string) => { last = v; }, stops: () => stops };
}
const drain = () => new Promise<void>((resolve) => setTimeout(resolve, 0));
const assistantEnd = (text: string) => ({ type: "message_end", message: { role: "assistant", content: [{ type: "text", text }] } });

test("delegation policy preserves execution at the terminal depth without child-management tools", () => {
  const parent = worker();
  const leaf = childPolicy(parent, full, "lead");
  assert.equal(leaf.depth, 2);
  assert.ok(leaf.tools.includes("bash") && leaf.tools.includes("write"));
  assert.ok(CHILD_MANAGEMENT_TOOLS.every((tool) => !leaf.tools.includes(tool)));
  assert.throws(() => childPolicy(leaf, ["read"], "leaf"), /maximum delegation depth/);
  assert.ok(readOnlyWsTools(["ws__git_diff", "ws__git_commit", "ws__tickets_query"]).includes("ws__git_diff"));
});

test("subtree outstanding counts execution and pending delivery, not report obligations or owner waits", () => {
  const executing = record("running", { running: true });
  const delivering = record("delivery", {
    terminalDelivery: { generation: 1, family: "ws-agent-settled", payload: { agent_id: "delivery" }, state: "held" },
  });
  const waiting = record("waiting", { waitingOnChildren: true, threadBound: true, lastWriter: "owner" });
  const registry = new Map([[executing.agentId, executing], [delivering.agentId, delivering], [waiting.agentId, waiting]]);
  assert.equal(subtreeOutstanding(registry), 2);
  assert.equal(hasRunningAgents(registry), true, "only actual execution contributes to running");

  executing.running = false;
  assert.equal(hasRunningAgents(registry), false);
  assert.equal(subtreeOutstanding(registry), 2, "a descendant wait remains an internal terminal-delivery dependency");
  const rows = listAgents(registry);
  assert.equal(rows.find((row) => row.agent_id === "waiting")?.status, "waiting-on-children");
  assert.equal(rows.find((row) => row.agent_id === "delivery")?.status, "pending-delivery");
});

test("dispatch admission is published as outstanding until registration completes", () => {
  const registry: RpcAgentRegistry = new Map();
  const channel = { path: join(home(), "subtree.json"), nonce: "launch" };
  installSubtreePublisher(registry, channel, () => 0);
  const finish = beginSubtreeDispatch(registry);
  assert.equal(subtreeWaiting(readSubtreeSnapshot(channel)), true);
  finish();
  assert.equal(subtreeWaiting(readSubtreeSnapshot(channel)), false);
});

test("ordinary settlement yields exactly one terminal result and clears execution before delivery", async () => {
  const sent: any[] = [];
  const h = pushHarness(sent);
  const child = record("child", { client: h.client, delegation: worker(), running: true, workGeneration: 1 });
  const registry: RpcAgentRegistry = new Map([[child.agentId, child]]);
  attachEventListener(h.pi, registry, child, h.client);

  h.emit(assistantEnd("settled answer"));
  h.emit({ type: "agent_settled" });
  h.emit({ type: "agent_settled" });
  await drain();
  assert.equal(child.running, false);
  assert.equal(hasRunningAgents(registry), false);
  assert.equal(child.terminalDelivery?.state, "held");
  assert.equal(sent.length, 0);

  flushHeldPushes(h.pi, true);
  await drain();
  assert.equal(sent.length, 1);
  assert.equal(sent[0].customType, "ws-agent-settled");
  assert.equal(sent[0].details.last_message, "settled answer");
  assert.equal(child.terminalDelivery?.state, "enqueued");
  assert.equal(h.stops(), 1, "a settled child is parked only after delivery enqueue");

  h.emit({ type: "agent_settled" });
  await drain();
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 1, "duplicate settlement for the same generation is ignored");
});

test("a failed real settlement batch remains held and retries without duplicate delivery", async () => {
  const sent: any[] = [];
  const h = pushHarness(sent);
  let attempts = 0;
  const sendMessage = (message: unknown) => {
    attempts += 1;
    if (attempts === 1) throw new Error("temporary parent queue failure");
    capturePush(sent, message);
  };
  (h.pi as any).sendMessage = sendMessage;
  const child = record("retry-child", { client: h.client, running: true, workGeneration: 1 });
  const registry = new Map([[child.agentId, child]]);
  attachEventListener(h.pi, registry, child, h.client);

  h.emit(assistantEnd("retryable result"));
  h.emit({ type: "agent_settled" });
  await drain();
  assert.equal(child.terminalDelivery?.state, "held");
  assert.equal(flushHeldPushes(h.pi, true), 0, "the rejected batch stays queued");
  assert.equal(child.terminalDelivery?.state, "held");
  assert.equal(h.stops(), 0, "parking waits for successful admission");

  assert.equal(flushHeldPushes(h.pi, true), 1);
  await drain();
  assert.equal(sent.length, 1);
  assert.equal(sent[0].details.last_message, "retryable result");
  assert.equal(child.terminalDelivery?.state, "enqueued");
  assert.equal(h.stops(), 1);
});

test("a settled parent waits for descendants without remaining globally running", async () => {
  const sent: any[] = [];
  const h = pushHarness(sent);
  const channel = { path: join(home(), "subtree.json"), nonce: "nested" };
  const parent = record("parent", { client: h.client, delegation: worker(), running: true, workGeneration: 1, subtreeChannel: channel });
  const grandchild = record("grandchild", { running: true });
  const inner = new Map([[grandchild.agentId, grandchild]]);
  installSubtreePublisher(inner, channel, () => 0);
  const registry = new Map([[parent.agentId, parent]]);
  attachEventListener(h.pi, registry, parent, h.client);

  h.emit(assistantEnd("waiting answer"));
  h.emit({ type: "agent_settled" });
  await drain();
  assert.equal(parent.running, false);
  assert.equal(parent.waitingOnChildren, true);
  assert.equal(hasRunningAgents(registry), false);
  assert.equal(listAgents(registry)[0]?.status, "waiting-on-children");
  assert.equal(sent.length, 0);

  grandchild.running = false;
  installSubtreePublisher(inner, channel, () => 0);
  assert.equal(subtreeWaiting(readSubtreeSnapshot(channel)), false);
  h.emit({ type: "agent_start" });
  h.emit(assistantEnd("settled answer"));
  h.emit({ type: "agent_settled" });
  await drain();
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 1);
  assert.equal(sent[0].details.last_message, "settled answer");
});

test("settled prose is preserved without adapter adequacy parsing for every role", async () => {
  const cases: Array<[RpcAgentRecord["spawnRole"], string | undefined]> = [
    ["explore", "short acknowledgement"],
    ["worker", "Outcome: complete\nVerification: passed"],
    ["fork", "malformed but still terminal prose"],
    ["fork", undefined],
  ];
  for (const [role, text] of cases) {
    heldPushQueue.length = 0;
    const sent: any[] = [];
    const h = pushHarness(sent);
    (h.client as any).getLastAssistantText = async () => text;
    const child = record(`${role}-${sent.length}`, { client: h.client, spawnRole: role, running: true, workGeneration: 1 });
    const registry = new Map([[child.agentId, child]]);
    attachEventListener(h.pi, registry, child, h.client);
    if (text !== undefined) h.emit(assistantEnd(text));
    h.emit({ type: "agent_settled" });
    await drain();
    flushHeldPushes(h.pi, true);
    await drain();
    assert.equal(sent.length, 1);
    assert.equal(sent[0].details.last_message, text);
    assert.equal(hasRunningAgents(registry), false);
  }
});

test("owner-held settlement notifies the owner once and generic Finish produces a fresh lead result", async () => {
  const sent: any[] = [];
  const notices: string[] = [];
  ownerNotifyRef.current = (message) => notices.push(message);
  const ownerHarness = pushHarness(sent);
  const owner = record("owner", { client: ownerHarness.client, spawnRole: "fork", lastWriter: "owner", running: true, workGeneration: 1 });
  const ownerRegistry = new Map([[owner.agentId, owner]]);
  attachEventListener(ownerHarness.pi, ownerRegistry, owner, ownerHarness.client);
  ownerHarness.emit(assistantEnd("settled answer"));
  ownerHarness.emit({ type: "agent_settled" });
  ownerHarness.emit({ type: "agent_settled" });
  await drain();
  assert.equal(notices.length, 1);
  assert.equal(sent.length, 0);
  assert.equal(hasRunningAgents(ownerRegistry), false);
  assert.equal(owner.terminalDelivery?.state, "enqueued");

  ownerNotifyRef.current = undefined;
  const finishSent: any[] = [];
  const finishHarness = pushHarness(finishSent);
  const fork = record("fork", { client: finishHarness.client, spawnRole: "fork", threadBound: true, running: false, workGeneration: 1, launchGeneration: 1 });
  const finishRegistry = new Map([[fork.agentId, fork]]);
  attachEventListener(finishHarness.pi, finishRegistry, fork, finishHarness.client);
  startForkFinish(fork, finishRegistry, finishHarness.pi, { cwd: ".", extensionPath: "index.ts" });
  await drain();
  finishHarness.setLast("fresh Finish result");
  finishHarness.emit({ type: "agent_start" });
  finishHarness.emit(assistantEnd("fresh Finish result"));
  finishHarness.emit({ type: "agent_settled" });
  await drain();
  flushHeldPushes(finishHarness.pi, true);
  await drain();
  assert.equal(finishSent.length, 1);
  assert.equal(finishSent[0].details.last_message, "fresh Finish result");
  assert.equal(notices.length, 1, "Finish does not replay the old owner-held result");
});

test("owner-route terminal delivery retries after a temporary notifier failure", async () => {
  const sent: any[] = [];
  const notices: string[] = [];
  let attempts = 0;
  ownerNotifyRef.current = (message) => {
    attempts += 1;
    if (attempts === 1) throw new Error("overlay transitioning");
    notices.push(message);
  };
  const h = pushHarness(sent);
  const child = record("owner-retry", { client: h.client, lastWriter: "owner", running: true, workGeneration: 1 });
  const registry = new Map([[child.agentId, child]]);
  attachEventListener(h.pi, registry, child, h.client);

  h.emit(assistantEnd("owner result"));
  h.emit({ type: "agent_settled" });
  await drain();
  assert.equal(child.terminalDelivery?.state, undefined);
  assert.equal(notices.length, 0);
  await new Promise((resolve) => setTimeout(resolve, OWNER_TERMINAL_RETRY_DELAY_MS + 20));
  assert.equal(attempts, 2);
  assert.equal(notices.length, 1);
  assert.match(notices[0], /owner result/);
  assert.equal(child.terminalDelivery?.state, "enqueued");
});

test("a successor prompt invalidates a late harvest from the previous generation", async () => {
  const sent: any[] = [];
  let release!: (value: string) => void;
  const h = pushHarness(sent);
  (h.client as any).getLastAssistantText = () => new Promise<string>((resolve) => { release = resolve; });
  (h.client as any).prompt = async () => {};
  const child = record("child", { client: h.client, running: true, workGeneration: 1 });
  const registry = new Map([[child.agentId, child]]);
  attachEventListener(h.pi, registry, child, h.client);

  h.emit(assistantEnd("stale answer"));
  h.emit({ type: "agent_settled" });
  await drain();
  await promptAgent(child, h.client, "successor");
  release("stale answer");
  await drain();
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 0);
  assert.equal(child.running, true);
  assert.equal(child.workGeneration, 2);
});

test("empty or missing current output never falls back to a previous generation", async () => {
  const cases: Array<{ event?: string; expected: string | undefined }> = [
    { event: "", expected: "" },
    { expected: undefined },
  ];
  for (const [index, item] of cases.entries()) {
    heldPushQueue.length = 0;
    const sent: any[] = [];
    const h = pushHarness(sent);
    (h.client as any).getLastAssistantText = async () => "previous successful answer";
    const child = record(`generation-${index}`, {
      client: h.client,
      running: false,
      workGeneration: 1,
      lastText: "previous successful answer",
      lastTextGeneration: 1,
    });
    const registry = new Map([[child.agentId, child]]);
    attachEventListener(h.pi, registry, child, h.client);
    await promptAgent(child, h.client, "new work");
    if (item.event !== undefined) h.emit(assistantEnd(item.event));
    h.emit({ type: "agent_settled" });
    await drain();
    flushHeldPushes(h.pi, true);
    await drain();
    assert.equal(sent[0].details.last_message, item.expected);
    assert.notEqual(sent[0].details.last_message, "previous successful answer");
  }
});

test("exited and ordinary stopped paths each emit their terminal disposition", async () => {
  const exitedSent: any[] = [];
  const exitedHarness = pushHarness(exitedSent);
  const exited = record("exited", { client: exitedHarness.client, running: true, workGeneration: 1 });
  const exitedRegistry = new Map([[exited.agentId, exited]]);
  attachEventListener(exitedHarness.pi, exitedRegistry, exited, exitedHarness.client);
  exitedHarness.emit(assistantEnd("last output before exit"));
  (exitedHarness.client as any).getState = async () => { throw new Error("process exited"); };
  assert.equal(await probeAgentLiveness(exitedHarness.pi, exitedRegistry, exited), false);
  flushHeldPushes(exitedHarness.pi, true);
  assert.equal(exitedSent.length, 1);
  assert.equal(exitedSent[0].details.reason, "exited");
  assert.equal(exitedSent[0].details.last_message, "last output before exit");
  assert.equal(exited.client, undefined);

  heldPushQueue.length = 0;
  const stoppedSent: any[] = [];
  const stoppedHarness = pushHarness(stoppedSent);
  const stopped = record("stopped", { client: stoppedHarness.client, running: true, workGeneration: 1 });
  const stoppedRegistry = new Map([[stopped.agentId, stopped]]);
  await stopAgent(stoppedRegistry, stopped.agentId, stoppedHarness.pi);
  flushHeldPushes(stoppedHarness.pi, true);
  assert.equal(stoppedSent.length, 1);
  assert.equal(stoppedSent[0].details.reason, "stopped");
  assert.equal(stopped.client, undefined);
});

test("explicit stop remains an idempotent disposition and emits no fabricated terminal result", async () => {
  const sent: unknown[] = [];
  const h = pushHarness(sent);
  const child = record("child", { client: h.client, running: true });
  const registry = new Map([[child.agentId, child]]);
  await stopAgent(registry, child.agentId, h.pi, { silent: true });
  await stopAgent(registry, child.agentId, h.pi, { silent: true });
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 0);
  assert.equal(hasRunningAgents(registry), false);
});

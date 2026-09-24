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
import { ChildChannel, ParentChannel, readAndDeleteChannelBootstrap } from "../src/agent-channel.ts";
import {
  beginSubtreeDispatch,
  installSubtreePublisher,
  MAX_SUBTREE_DESCENDANT_DEPTH,
  MAX_SUBTREE_DESCENDANTS,
  publishSubtree,
  subtreeOutstanding,
  subtreeWaiting,
  SubtreeUpstream,
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
  markAgentExited,
  observeChildSubtree,
  OWNER_TERMINAL_RETRY_DELAY_MS,
  ownerNotifyRef,
  probeAgentLiveness,
  promptAgent,
  registerPushFlush,
  resolveTools,
  sendToAgent,
  startForkFinish,
  stopAgent,
  type RpcAgentRecord,
  type RpcAgentRegistry,
} from "../src/spawner.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";
import { fakeParentChannel, fakeUplink, quiescentSnapshot } from "./fixtures/subtree-channels.ts";

const dirs: string[] = [];
function home(): string {
  const dir = mkdtempSync(join(tmpdir(), "ws-subtree-test-"));
  dirs.push(dir);
  return dir;
}
const opened: Array<{ close(): void }> = [];
/** A real parent/child channel pair in this process; the child's upstream publishes over it. */
async function channelPair(generation = 1) {
  const parent = await ParentChannel.bind(generation, { socketDir: home() });
  let upstream: SubtreeUpstream | undefined;
  const child = await ChildChannel.connect(readAndDeleteChannelBootstrap({ ...parent.bootstrapEnv() })!, { reconnect: false, resume: () => upstream?.resume() ?? {} });
  upstream = new SubtreeUpstream(child);
  opened.push(child, parent);
  return { parent, child, upstream };
}
async function until(condition: () => boolean, what: string, timeoutMs = 2_000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (!condition()) {
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await new Promise((resolve) => setTimeout(resolve, 5));
  }
}
afterEach(() => {
  for (const channel of opened.splice(0)) channel.close();
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

test("dispatch admission is outstanding at the parent before the fence opens and until registration completes", async () => {
  const { parent, upstream } = await channelPair();
  const observed = record("observed", { channel: parent });
  const parentRegistry: RpcAgentRegistry = new Map([[observed.agentId, observed]]);
  observeChildSubtree(parentRegistry, observed, parent);
  const registry: RpcAgentRegistry = new Map();
  installSubtreePublisher(registry, upstream, () => 0);
  await until(() => observed.subtreeRevision === 1, "the initial snapshot");
  assert.equal(observed.waitingOnChildren, false);

  const admission = beginSubtreeDispatch(registry);
  assert.ok(admission instanceof Promise);
  const finish = await admission;
  assert.equal(observed.waitingOnChildren, true, "the parent applied the busy revision before the dispatch was admitted");
  assert.equal(observed.subtreeRevision, 2);
  finish();
  await until(() => observed.subtreeRevision === 3, "the idle revision");
  assert.equal(observed.waitingOnChildren, false);
});

test("RPC settlement is unaffected while the upstream channel is down", async () => {
  const sent: any[] = [];
  const h = pushHarness(sent);
  const child = record("upstream-down", { client: h.client, running: true, workGeneration: 1 });
  const registry: RpcAgentRegistry = new Map([[child.agentId, child]]);
  const uplink = fakeUplink();
  uplink.connected = false;
  installSubtreePublisher(registry, new SubtreeUpstream(uplink.channel), () => 0);
  attachEventListener(h.pi, registry, child, h.client);

  h.emit(assistantEnd("settled while the upstream is down"));
  h.emit({ type: "agent_settled" });
  await drain();
  assert.equal(child.running, false);
  assert.equal(child.terminalDelivery?.state, "held", "settlement admission still executes");
  flushHeldPushes(h.pi, true);
  await drain();
  assert.equal(sent[0]?.details.last_message, "settled while the upstream is down");
  assert.equal(uplink.sent.length, 0, "nothing is sent while the upstream is down");
});

test("a relaunch generation restarts the revision order and drops the previous launch's late snapshots", () => {
  const registry: RpcAgentRegistry = new Map();
  const first = fakeParentChannel();
  const child = record("relaunched", { channel: first.parent, launchGeneration: 1 });
  registry.set(child.agentId, child);
  observeChildSubtree(registry, child, first.parent);
  first.deliver(quiescentSnapshot(5));
  assert.equal(child.subtreeRevision, 5);

  const second = fakeParentChannel();
  child.channel = second.parent;
  child.launchGeneration = 2;
  observeChildSubtree(registry, child, second.parent);
  assert.equal(child.waitingOnChildren, true, "a launch whose child has not connected yet reads as waiting");
  assert.equal(child.subtreeRevision, undefined);
  second.deliver(quiescentSnapshot(1, { active: 1 }));
  assert.equal(child.subtreeRevision, 1, "the new generation's first revision is accepted below the previous generation's");
  assert.equal(child.waitingOnChildren, true);

  first.deliver(quiescentSnapshot(9));
  first.drop();
  assert.equal(child.subtreeRevision, 1, "the previous launch's late snapshot cannot write the record");
  assert.equal(child.waitingOnChildren, true);
  second.deliver(quiescentSnapshot(2));
  assert.equal(child.waitingOnChildren, false);
});

test("subtree publication merges three identity levels without changing authoritative counts", () => {
  const child = record("child", {
    client: {} as never,
    running: true,
    spawnRole: "worker",
    subtreeDescendants: [
      { id: "grandchild", parentId: null, depth: 0, role: "explore", live: true },
      { id: "great-grandchild", parentId: "grandchild", depth: 1, role: "execute", live: false },
    ],
  });
  const registry: RpcAgentRegistry = new Map([[child.agentId, child]]);
  installSubtreePublisher(registry, undefined, () => 2);

  const snapshot = publishSubtree(registry)!;
  assert.deepEqual(snapshot.descendants, [
    { id: "child", parentId: null, depth: 0, role: "worker", live: true },
    { id: "grandchild", parentId: "child", depth: 1, role: "explore", live: true },
    { id: "great-grandchild", parentId: "grandchild", depth: 2, role: "execute", live: false },
  ]);
  assert.deepEqual(
    { outstanding: snapshot.outstanding, active: snapshot.active, deliveries: snapshot.deliveries, delegated: snapshot.delegated, revision: snapshot.revision },
    { outstanding: 0, active: 1, deliveries: 2, delegated: true, revision: 0 },
  );
  assert.equal(subtreeWaiting(snapshot), true, "identity rows are not an input to wait/settle");
});

test("subtree identity breadth guard retains exactly the bounded shallow prefix without corrupting counts", () => {
  const descendants = Array.from({ length: MAX_SUBTREE_DESCENDANTS + 20 }, (_, index) => ({
    id: `sibling-${index}`,
    parentId: null,
    depth: 0,
    role: "worker" as const,
    live: true,
  }));
  const child = record("guard-root", { running: true, subtreeDescendants: descendants });
  const registry: RpcAgentRegistry = new Map([[child.agentId, child]]);
  installSubtreePublisher(registry, undefined, () => 0);

  const snapshot = publishSubtree(registry)!;
  assert.equal(snapshot.descendants.length, MAX_SUBTREE_DESCENDANTS);
  assert.equal(snapshot.descendants[0]?.id, "guard-root");
  assert.equal(snapshot.descendants.at(-1)?.id, `sibling-${MAX_SUBTREE_DESCENDANTS - 2}`);
  assert.deepEqual(
    { outstanding: snapshot.outstanding, active: snapshot.active, deliveries: snapshot.deliveries },
    { outstanding: 0, active: 1, deliveries: 0 },
  );
});

test("subtree identity depth guard retains the boundary and drops the next nested row without corrupting counts", () => {
  const descendants = Array.from({ length: MAX_SUBTREE_DESCENDANT_DEPTH + 2 }, (_, index) => ({
    id: `deep-${index}`,
    parentId: index === 0 ? null : `deep-${index - 1}`,
    depth: index,
    role: "worker" as const,
    live: true,
  }));
  const child = record("depth-root", { running: true, subtreeDescendants: descendants });
  const registry: RpcAgentRegistry = new Map([[child.agentId, child]]);
  installSubtreePublisher(registry, undefined, () => 0);

  const snapshot = publishSubtree(registry)!;
  assert.equal(snapshot.descendants.at(-1)?.depth, MAX_SUBTREE_DESCENDANT_DEPTH);
  assert.equal(snapshot.descendants.at(-1)?.id, `deep-${MAX_SUBTREE_DESCENDANT_DEPTH - 1}`);
  assert.equal(snapshot.descendants.some((row) => row.id === `deep-${MAX_SUBTREE_DESCENDANT_DEPTH}`), false);
  assert.deepEqual(
    { outstanding: snapshot.outstanding, active: snapshot.active, deliveries: snapshot.deliveries },
    { outstanding: 0, active: 1, deliveries: 0 },
  );
});

test("losing a direct child clears and republishes cached descendant liveness", () => {
  const uplink = fakeUplink();
  const h = pushHarness([]);
  const child = record("child", {
    client: h.client,
    running: true,
    waitingOnChildren: true,
    subtreeRevision: 4,
    subtreeDescendants: [{ id: "grandchild", parentId: null, depth: 0, role: "worker", live: true }],
  });
  const registry: RpcAgentRegistry = new Map([[child.agentId, child]]);
  installSubtreePublisher(registry, new SubtreeUpstream(uplink.channel), () => 0);

  markAgentExited(h.pi, registry, child, { suppressTerminal: true });

  assert.equal(child.waitingOnChildren, false);
  assert.equal(child.subtreeRevision, undefined);
  assert.deepEqual(child.subtreeDescendants, []);
  assert.deepEqual(uplink.snapshots().at(-1)?.descendants, [
    { id: "child", parentId: null, depth: 0, role: "worker", live: false },
  ]);
});

test("channel snapshots propagate a nested parent edge through two process hops, and losing the middle child clears it", async () => {
  const leaf = await channelPair();
  const middle = await channelPair();
  const middleHarness = pushHarness([]);
  const rootHarness = pushHarness([]);
  const child = record("child", { client: middleHarness.client, channel: leaf.parent });
  const middleRegistry: RpcAgentRegistry = new Map([[child.agentId, child]]);
  installSubtreePublisher(middleRegistry, middle.upstream, () => 0);
  observeChildSubtree(middleRegistry, child, leaf.parent);

  const parent = record("parent", { client: rootHarness.client, channel: middle.parent });
  const rootRegistry: RpcAgentRegistry = new Map([[parent.agentId, parent]]);
  installSubtreePublisher(rootRegistry, undefined, () => 0);
  observeChildSubtree(rootRegistry, parent, middle.parent);

  const grandchild = record("grandchild", { client: {} as never, running: true, spawnRole: "explore" });
  installSubtreePublisher(new Map([[grandchild.agentId, grandchild]]), leaf.upstream, () => 0);
  await until(() => parent.subtreeDescendants?.some((row) => row.id === grandchild.agentId) === true, "the grandchild's identity at the root");

  assert.deepEqual(parent.subtreeDescendants, [
    { id: "child", parentId: null, depth: 0, role: "worker", live: true },
    { id: "grandchild", parentId: "child", depth: 1, role: "explore", live: true },
  ]);
  assert.equal(parent.waitingOnChildren, true, "the running grandchild keeps the root waiting through the middle hop");
  assert.deepEqual(publishSubtree(rootRegistry)?.descendants, [
    { id: "parent", parentId: null, depth: 0, role: "worker", live: true },
    { id: "child", parentId: "parent", depth: 1, role: "worker", live: true },
    { id: "grandchild", parentId: "child", depth: 2, role: "explore", live: true },
  ]);

  markAgentExited(middleHarness.pi, middleRegistry, child, { suppressTerminal: true });
  await until(() => parent.subtreeDescendants?.length === 1, "the cleared identity at the root");
  assert.deepEqual(parent.subtreeDescendants, [
    { id: "child", parentId: null, depth: 0, role: "worker", live: false },
  ]);
  assert.equal(parent.waitingOnChildren, false);
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
  const { parent: channel, upstream } = await channelPair();
  const parent = record("parent", { client: h.client, delegation: worker(), running: true, workGeneration: 1, channel });
  const grandchild = record("grandchild", { running: true });
  const inner = new Map([[grandchild.agentId, grandchild]]);
  const registry = new Map([[parent.agentId, parent]]);
  observeChildSubtree(registry, parent, channel);
  installSubtreePublisher(inner, upstream, () => 0);
  attachEventListener(h.pi, registry, parent, h.client);
  await until(() => parent.subtreeRevision === 1, "the busy snapshot");

  h.emit(assistantEnd("waiting answer"));
  h.emit({ type: "agent_settled" });
  await drain();
  assert.equal(parent.running, false);
  assert.equal(parent.waitingOnChildren, true);
  assert.deepEqual(parent.subtreeDescendants, [
    { id: "grandchild", parentId: null, depth: 0, role: "worker", live: false },
  ], "the same snapshot that drives waiting also carries advisory identity");
  assert.equal(hasRunningAgents(registry), false);
  assert.equal(listAgents(registry)[0]?.status, "waiting-on-children");
  assert.equal(sent.length, 0);

  grandchild.running = false;
  publishSubtree(inner);
  await until(() => parent.waitingOnChildren === false, "the quiescent snapshot");
  assert.equal(subtreeWaiting(publishSubtree(inner)), false);
  h.emit({ type: "agent_start" });
  h.emit(assistantEnd("settled answer"));
  h.emit({ type: "agent_settled" });
  await drain();
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 1);
  assert.equal(sent[0].details.last_message, "settled answer");
});

test("a disconnected child channel holds settlement until a reconnected quiescent snapshot, and exit releases it", async () => {
  const sent: any[] = [];
  const h = pushHarness(sent);
  const channel = fakeParentChannel();
  const parent = record("parent", { client: h.client, delegation: worker(), running: true, workGeneration: 1, channel: channel.parent });
  const registry = new Map([[parent.agentId, parent]]);
  observeChildSubtree(registry, parent, channel.parent);
  attachEventListener(h.pi, registry, parent, h.client);
  channel.deliver(quiescentSnapshot(3));
  assert.equal(parent.waitingOnChildren, false);

  channel.drop();
  assert.equal(parent.waitingOnChildren, true, "a disconnected channel reads as waiting");
  h.emit(assistantEnd("answer while disconnected"));
  h.emit({ type: "agent_settled" });
  await drain();
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 0, "settlement is deferred while the channel is down");
  assert.equal(listAgents(registry)[0]?.status, "waiting-on-children");

  channel.hello({});
  assert.equal(parent.waitingOnChildren, true, "a reconnect that carries no snapshot keeps waiting");
  channel.hello({ subtree: quiescentSnapshot(3) });
  assert.equal(parent.waitingOnChildren, false, "the reconnected quiescent snapshot releases the wait");
  h.emit({ type: "agent_start" });
  h.emit(assistantEnd("settled after reconnect"));
  h.emit({ type: "agent_settled" });
  await drain();
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 1);
  assert.equal(sent[0].details.last_message, "settled after reconnect");

  heldPushQueue.length = 0;
  const exitSent: any[] = [];
  const exitHarness = pushHarness(exitSent);
  const exitChannel = fakeParentChannel();
  const exiting = record("exiting", { client: exitHarness.client, running: true, workGeneration: 1, channel: exitChannel.parent });
  const exitRegistry = new Map([[exiting.agentId, exiting]]);
  observeChildSubtree(exitRegistry, exiting, exitChannel.parent);
  attachEventListener(exitHarness.pi, exitRegistry, exiting, exitHarness.client);
  exitHarness.emit(assistantEnd("last output before exit"));
  exitHarness.emit({ type: "agent_settled" });
  await drain();
  assert.equal(exiting.waitingOnChildren, true, "a child that never connected reads as waiting");
  markAgentExited(exitHarness.pi, exitRegistry, exiting);
  assert.equal(exiting.waitingOnChildren, false, "exit releases the wait");
  assert.equal(exitChannel.closed, true);
  exitChannel.drop();
  assert.equal(exiting.waitingOnChildren, false, "the closed launch's late disconnect cannot re-arm the wait");
  flushHeldPushes(exitHarness.pi, true);
  assert.equal(exitSent.length, 1);
  assert.equal(exitSent[0].details.reason, "exited");
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

test("active steer and follow-up advance generation only at their queued user boundary", async () => {
  for (const interrupt of [true, false]) {
    heldPushQueue.length = 0;
    const sent: any[] = [];
    const h = pushHarness(sent);
    const child = record(interrupt ? "steer" : "follow-up", { client: h.client, streaming: true, running: true, workGeneration: 1 });
    const registry = new Map([[child.agentId, child]]);
    attachEventListener(h.pi, registry, child, h.client);

    await sendToAgent(registry, { cwd: "." }, child.agentId, "successor instruction", interrupt);
    assert.equal(child.workGeneration, 1, "queue admission is not the execution boundary");
    h.emit(assistantEnd("prior turn output"));
    assert.equal(child.lastTextGeneration, 1);
    h.emit({ type: "message_start", message: { role: "user", content: [{ type: "text", text: "successor instruction" }] } });
    assert.equal(child.workGeneration, 2);
    assert.equal(child.lastText, undefined, "the actual queued-user boundary clears prior text");
    h.setLast("successor output");
    h.emit(assistantEnd("successor output"));
    h.emit({ type: "agent_settled" });
    await drain();
    flushHeldPushes(h.pi, true);
    await drain();
    assert.equal(sent[0].details.last_message, "successor output");
  }
});

test("an exit before an accepted queued instruction starts never reports the prior turn", async () => {
  const sent: any[] = [];
  const h = pushHarness(sent);
  const child = record("queued-exit", { client: h.client, streaming: true, running: true, workGeneration: 1 });
  const registry = new Map([[child.agentId, child]]);
  attachEventListener(h.pi, registry, child, h.client);
  await sendToAgent(registry, { cwd: "." }, child.agentId, "queued successor");
  h.emit(assistantEnd("prior turn output"));
  (h.client as any).getState = async () => { throw new Error("process exited before queue drain"); };
  assert.equal(await probeAgentLiveness(h.pi, registry, child), false);
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 1);
  assert.equal(sent[0].details.reason, "exited");
  assert.equal(sent[0].details.last_message, undefined);
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

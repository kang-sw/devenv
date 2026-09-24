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
  MAX_SUBTREE_DESCENDANT_DEPTH,
  MAX_SUBTREE_DESCENDANTS,
  publishSubtree,
  subtreeOutstanding,
  subtreeWaiting,
  SubtreeUpstream,
  type SubtreeSnapshot,
} from "../src/subtree-lifecycle.ts";
import {
  applyRpcEvent,
  attachEventListener,
  clearWakeStart,
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
  ownTurnRef,
  probeAgentLiveness,
  promptAgent,
  pushToLead,
  registerPushFlush,
  resolveTools,
  sendToAgent,
  startForkFinish,
  stopAgent,
  type RpcAgentRecord,
  type RpcAgentRegistry,
} from "../src/spawner.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";
import { fakeParentChannel, fakeUplink, idleOwnTurn, quiescentSnapshot, subtreeChannelPair, until } from "./fixtures/subtree-channels.ts";

const dirs: string[] = [];
function home(): string {
  const dir = mkdtempSync(join(tmpdir(), "ws-subtree-test-"));
  dirs.push(dir);
  return dir;
}
const opened: Array<{ close(): void }> = [];
async function channelPair() {
  const pair = await subtreeChannelPair(1, { socketDir: home() });
  opened.push(pair.child, pair.parent);
  return pair;
}
afterEach(() => {
  for (const channel of opened.splice(0)) channel.close();
  for (const dir of dirs.splice(0)) rmSync(dir, { recursive: true, force: true });
  heldPushQueue.length = 0;
  leadIdleRef.current = undefined;
  clearWakeStart();
  ownTurnRef.owed = false;
  ownTurnRef.started = 0;
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
  installSubtreePublisher(registry, upstream, () => 0, idleOwnTurn);
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
  installSubtreePublisher(registry, new SubtreeUpstream(uplink.channel), () => 0, idleOwnTurn);
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
  installSubtreePublisher(registry, undefined, () => 2, idleOwnTurn);

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
  installSubtreePublisher(registry, undefined, () => 0, idleOwnTurn);

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
  installSubtreePublisher(registry, undefined, () => 0, idleOwnTurn);

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
  installSubtreePublisher(registry, new SubtreeUpstream(uplink.channel), () => 0, idleOwnTurn);

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
  installSubtreePublisher(middleRegistry, middle.upstream, () => 0, idleOwnTurn);
  observeChildSubtree(middleRegistry, child, leaf.parent);

  const parent = record("parent", { client: rootHarness.client, channel: middle.parent });
  const rootRegistry: RpcAgentRegistry = new Map([[parent.agentId, parent]]);
  installSubtreePublisher(rootRegistry, undefined, () => 0, idleOwnTurn);
  observeChildSubtree(rootRegistry, parent, middle.parent);

  const grandchild = record("grandchild", { client: {} as never, running: true, spawnRole: "explore" });
  installSubtreePublisher(new Map([[grandchild.agentId, grandchild]]), leaf.upstream, () => 0, idleOwnTurn);
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
  installSubtreePublisher(inner, upstream, () => 0, idleOwnTurn);
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

  // The grandchild is stopped with no delivery into the child: the real
  // socket's quiescent snapshot, owing no turn, releases the held settle.
  grandchild.running = false;
  publishSubtree(inner);
  await until(() => parent.waitingOnChildren === false, "the quiescent snapshot");
  assert.equal(subtreeWaiting(publishSubtree(inner)), false);
  await drain();
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 1, "the held settle is admitted with no further child turn");
  assert.equal(sent[0].details.last_message, "waiting answer");
});

test("a settle held only by a disconnect is released by the reconnected quiescent snapshot, with no further turn", async () => {
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
  h.setLast("answer while disconnected");
  h.emit(assistantEnd("answer while disconnected"));
  h.emit({ type: "agent_settled" });
  await drain();
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 0, "settlement is deferred while the channel is down");
  assert.equal(listAgents(registry)[0]?.status, "waiting-on-children");

  channel.hello({});
  await drain();
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 0, "a reconnect that carries no snapshot keeps waiting");
  channel.hello({ subtree: quiescentSnapshot(3) });
  assert.equal(parent.waitingOnChildren, false, "the reconnected quiescent snapshot releases the wait");
  await drain();
  flushHeldPushes(h.pi, true);
  await drain();
  assert.equal(sent.length, 1, "the held settle is admitted without another child turn");
  assert.equal(sent[0].details.last_message, "answer while disconnected");
  channel.deliver(quiescentSnapshot(3));
  await drain();
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 1, "a duplicate snapshot does not admit twice");
});

/**
 * A direct child whose settle is held on busy descendants (`outstanding: 1`
 * at revision 2), observed through a fake channel the test drives. The child
 * has started no turn the parent could count yet.
 */
async function heldOnDescendants() {
  const sent: any[] = [];
  const h = pushHarness(sent);
  const channel = fakeParentChannel();
  const parent = record("parent", { client: h.client, delegation: worker(), running: true, workGeneration: 1, channel: channel.parent });
  const registry = new Map([[parent.agentId, parent]]);
  observeChildSubtree(registry, parent, channel.parent);
  attachEventListener(h.pi, registry, parent, h.client);
  channel.deliver(quiescentSnapshot(1));
  channel.deliver(quiescentSnapshot(2, { outstanding: 1 }));
  h.emit(assistantEnd("dispatched a grandchild"));
  h.emit({ type: "agent_settled" });
  await drain();
  assert.equal(parent.waitingOnChildren, true);
  const admitted = async () => { await drain(); flushHeldPushes(h.pi, true); await drain(); return sent.map(push => push.details.last_message); };
  /** The wake turn over stdout: its start, its answer, its settle. */
  const wakeTurn = (answer: string, parts: { start?: boolean; settle?: boolean } = {}) => {
    if (parts.start !== false) h.emit({ type: "agent_start" });
    if (parts.settle !== false) { h.emit(assistantEnd(answer)); h.emit({ type: "agent_settled" }); }
  };
  return { parent, channel, admitted, wakeTurn };
}

test("a clear that wakes nothing: the quiescent snapshot arriving after the child's settle admits the terminal with no further turn", async () => {
  const { channel, admitted } = await heldOnDescendants();
  // The grandchild was stopped with no delivery: nothing is owed and no turn started.
  channel.deliver(quiescentSnapshot(3));
  assert.deepEqual(await admitted(), ["dispatched a grandchild"]);
  channel.deliver(quiescentSnapshot(3));
  channel.deliver(quiescentSnapshot(4, { descendants: [{ id: "grandchild", parentId: null, depth: 0, role: "worker", live: false }] }));
  assert.deepEqual(await admitted(), ["dispatched a grandchild"], "a duplicate or a later quiescent snapshot does not admit twice");
});

test("a clear that wakes the child, socket ahead of stdout: the owed turn keeps the hold until the wake turn's own settle admits once", async () => {
  const { channel, admitted, wakeTurn } = await heldOnDescendants();
  // The grandchild's terminal is handed into the child's session: quiescent,
  // owing the wake turn, before that turn's agent_start reaches stdout.
  channel.deliver(quiescentSnapshot(3, { turnOwed: true }));
  assert.deepEqual(await admitted(), [], "the previous turn's answer is not reported ahead of the wake turn");
  channel.deliver(quiescentSnapshot(4, { turnsStarted: 1 }));
  assert.deepEqual(await admitted(), [], "the wake turn's start counted over the socket is not yet seen on stdout");

  wakeTurn("folded the grandchild result");
  assert.deepEqual(await admitted(), ["folded the grandchild result"]);
  channel.deliver(quiescentSnapshot(4, { turnsStarted: 1 }));
  assert.deepEqual(await admitted(), ["folded the grandchild result"], "exactly once");
});

test("a clear that wakes the child, stdout ahead of socket: the wake turn's own settle admits once", async () => {
  const { parent, channel, admitted, wakeTurn } = await heldOnDescendants();
  wakeTurn("folded the grandchild result", { settle: false });
  assert.equal(parent.running, true, "the wake turn opened a new work generation");
  // The delivery's snapshot (owing the turn) and the turn start's snapshot lag behind.
  channel.deliver(quiescentSnapshot(3, { turnOwed: true }));
  channel.deliver(quiescentSnapshot(4, { turnsStarted: 1 }));
  assert.deepEqual(await admitted(), [], "the held previous generation is never admitted");
  wakeTurn("folded the grandchild result", { start: false });
  assert.deepEqual(await admitted(), ["folded the grandchild result"]);
});

test("a snapshot owing no turn while its turn count runs ahead of stdout admits nothing until stdout catches up", async () => {
  const { parent, channel, admitted, wakeTurn } = await heldOnDescendants();
  channel.deliver(quiescentSnapshot(3, { turnsStarted: 1 }));
  assert.equal(parent.waitingOnChildren, false);
  assert.deepEqual(await admitted(), [], "the child started a turn stdout has not delivered");
  wakeTurn("folded the grandchild result", { settle: false });
  assert.deepEqual(await admitted(), [], "the caught-up start opens the wake turn, it does not release the old hold");
  wakeTurn("folded the grandchild result", { start: false });
  assert.deepEqual(await admitted(), ["folded the grandchild result"]);
});

test("a relaunch restarts the parent's turn-start count with the new child process's own", async () => {
  const sent: any[] = [];
  const admitted = async (h: ReturnType<typeof pushHarness>) => { await drain(); flushHeldPushes(h.pi, true); await drain(); return sent.map(push => push.details.last_message); };
  const first = pushHarness(sent);
  const firstChannel = fakeParentChannel();
  const parent = record("parent", { client: first.client, delegation: worker(), workGeneration: 0, launchGeneration: 1, channel: firstChannel.parent });
  const registry = new Map([[parent.agentId, parent]]);
  observeChildSubtree(registry, parent, firstChannel.parent);
  attachEventListener(first.pi, registry, parent, first.client);
  firstChannel.deliver(quiescentSnapshot(1));
  first.emit({ type: "agent_start" });
  first.emit(assistantEnd("first launch answer"));
  first.emit({ type: "agent_settled" });
  assert.deepEqual(await admitted(first), ["first launch answer"]);

  const second = pushHarness(sent);
  const secondChannel = fakeParentChannel();
  parent.client = second.client;
  parent.channel = secondChannel.parent;
  parent.launchGeneration = 2;
  observeChildSubtree(registry, parent, secondChannel.parent);
  attachEventListener(second.pi, registry, parent, second.client);
  secondChannel.deliver(quiescentSnapshot(1));
  second.emit({ type: "agent_start" });
  secondChannel.deliver(quiescentSnapshot(2, { outstanding: 1, turnsStarted: 1 }));
  second.emit(assistantEnd("dispatched a grandchild"));
  second.emit({ type: "agent_settled" });
  assert.equal(parent.waitingOnChildren, true);
  // The new process's wake turn is its second: counted over the socket
  // before stdout delivers it. A count carried over from the first launch
  // would read as caught up and release the held settle early.
  secondChannel.deliver(quiescentSnapshot(3, { turnsStarted: 2 }));
  assert.deepEqual(await admitted(second), ["first launch answer"]);
  second.emit({ type: "agent_start" });
  second.emit(assistantEnd("folded the grandchild result"));
  second.emit({ type: "agent_settled" });
  assert.deepEqual(await admitted(second), ["first launch answer", "folded the grandchild result"]);
});

test("a disconnect window covering an identity-only revision change: the reconnect snapshot releases the held settle", async () => {
  const sent: any[] = [];
  const h = pushHarness(sent);
  const channel = fakeParentChannel();
  const parent = record("parent", { client: h.client, delegation: worker(), running: true, workGeneration: 1, channel: channel.parent });
  const registry = new Map([[parent.agentId, parent]]);
  observeChildSubtree(registry, parent, channel.parent);
  attachEventListener(h.pi, registry, parent, h.client);
  const live = [{ id: "grandchild", parentId: null, depth: 0, role: "worker" as const, live: true }];
  channel.deliver(quiescentSnapshot(3, { descendants: live }));
  channel.drop();
  h.emit(assistantEnd("answer across the drop"));
  h.emit({ type: "agent_settled" });
  await drain();
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 0, "held while the channel is down");

  // While disconnected the grandchild's liveness flipped: a higher revision, still quiescent.
  channel.hello({ subtree: quiescentSnapshot(5, { descendants: [{ ...live[0]!, live: false }] }) });
  assert.equal(parent.waitingOnChildren, false);
  await drain();
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 1);
  assert.equal(sent[0].details.last_message, "answer across the drop");
});

test("a disconnect window covering a whole wake turn: the wake turn's terminal is admitted once and the earlier generation's never", async () => {
  const { parent, channel, admitted, wakeTurn } = await heldOnDescendants();
  channel.drop();
  wakeTurn("folded the grandchild result");
  assert.equal(parent.waitingOnChildren, true, "the wake turn's own settle is held too: the channel is down");
  assert.deepEqual(await admitted(), []);
  channel.hello({ subtree: quiescentSnapshot(6, { turnsStarted: 1 }) });
  assert.deepEqual(await admitted(), ["folded the grandchild result"]);
  channel.deliver(quiescentSnapshot(6, { turnsStarted: 1 }));
  assert.deepEqual(await admitted(), ["folded the grandchild result"], "exactly once");
});

/**
 * The child process's side of the wake accounting: `registerPushFlush` and
 * copies of the publish-after-flush handlers `src/index.ts` registers right
 * after it and of the `ownTurn` accessor `registerAgentTools` installs (keep
 * both in step with those sites), on a fake Pi that
 * dispatches its lifecycle events in registration order, publishing through
 * a fake uplink. One live grandchild settles into this process.
 */
function childProcess(idle: boolean) {
  const handlers = new Map<string, Array<() => void>>();
  const sent: unknown[] = [];
  const wakes: string[] = [];
  const pi = {
    on(event: string, fn: () => void) { handlers.set(event, [...(handlers.get(event) ?? []), fn]); },
    sendMessage: (message: unknown) => capturePush(sent, message),
    sendUserMessage: (text: string) => { wakes.push(text); },
  } as never;
  let sessionIdle = idle;
  leadIdleRef.current = () => sessionIdle;
  registerPushFlush(pi, { delayMs: () => 60_000 });
  const uplink = fakeUplink();
  const registry: RpcAgentRegistry = new Map();
  installSubtreePublisher(registry, new SubtreeUpstream(uplink.channel), () => heldPushQueue.length, () => ({ ...ownTurnRef }));
  for (const event of ["agent_start", "agent_settled", "tool_execution_end"]) (pi as any).on(event, () => { publishSubtree(registry); });
  const fire = (event: "agent_start" | "agent_end" | "agent_settled") => {
    sessionIdle = event === "agent_settled";
    for (const fn of handlers.get(event) ?? []) fn();
  };
  let grandchildEvents: ((event: unknown) => void) | undefined;
  const grandchildClient = {
    onEvent(fn: (event: unknown) => void) { grandchildEvents = fn; return () => {}; },
    getLastAssistantText: async () => "grandchild answer",
    getState: async () => ({}), abort: async () => {}, prompt: async () => {}, stop: async () => {},
  } as unknown as RpcClient;
  const grandchild = record("grandchild", { client: grandchildClient, delegation: worker(), running: true, workGeneration: 1 });
  registry.set(grandchild.agentId, grandchild);
  attachEventListener(pi, registry, grandchild, grandchildClient);
  publishSubtree(registry);
  const grandchildSettles = async () => {
    grandchildEvents?.(assistantEnd("grandchild answer"));
    grandchildEvents?.({ type: "agent_settled" });
    await drain();
  };
  return { uplink, registry, sent, wakes, fire, grandchild, grandchildEvents: (event: unknown) => grandchildEvents?.(event), grandchildSettles };
}

/** Snapshots that differ from their predecessor only in `turnOwed` (and revision): an owed flip sent on its own. */
function ownedFlipSends(snapshots: readonly SubtreeSnapshot[]): SubtreeSnapshot[] {
  const rest = ({ turnOwed: _owed, revision: _revision, ...other }: SubtreeSnapshot) => JSON.stringify(other);
  return snapshots.filter((snapshot, i) => i > 0 && rest(snapshot) === rest(snapshots[i - 1]!));
}

test("child wake accounting: an idle wake owes its turn in the delivery's own snapshot, and the turn start counts and discharges it in one snapshot", async () => {
  const child = childProcess(true);
  await child.grandchildSettles();
  assert.equal(child.wakes.length, 1, "the idle child reserved a wake for the delivery");
  const owed = child.uplink.snapshots().at(-1)!;
  assert.deepEqual({ deliveries: owed.deliveries, turnOwed: owed.turnOwed, turnsStarted: owed.turnsStarted }, { deliveries: 1, turnOwed: true, turnsStarted: 0 });

  const before = child.uplink.snapshots().length;
  child.fire("agent_start");
  const started = child.uplink.snapshots().slice(before);
  assert.ok(started.length >= 1);
  assert.deepEqual({ turnOwed: started[0]!.turnOwed, turnsStarted: started[0]!.turnsStarted }, { turnOwed: false, turnsStarted: 1 },
    "the flush's own snapshot already carries the start: no snapshot reports the discharge with the old count");
  assert.equal(subtreeWaiting(started.at(-1)), false);
  assert.ok(child.uplink.snapshots().every(s => subtreeWaiting(s) || s.turnOwed || s.turnsStarted >= 1 || s.revision === 1),
    "after the grandchild was dispatched, no quiescent snapshot owed nothing before the wake turn counted");
  assert.deepEqual(ownedFlipSends(child.uplink.snapshots()), [], "owing and discharging the turn ride sends that already happen");
});

test("child wake accounting: a turn-boundary batch owes the continuation turn in the snapshot that first reads quiescent", async () => {
  const child = childProcess(false);
  child.fire("agent_start");
  await child.grandchildSettles();
  assert.equal(child.wakes.length, 0, "a busy child holds the delivery for the turn boundary");
  const held = child.uplink.snapshots().at(-1)!;
  assert.deepEqual({ deliveries: held.deliveries, turnOwed: held.turnOwed, turnsStarted: held.turnsStarted }, { deliveries: 1, turnOwed: false, turnsStarted: 1 });

  const before = child.uplink.snapshots().length;
  child.fire("agent_end");
  const boundary = child.uplink.snapshots().slice(before);
  const firstQuiescent = boundary.find(s => !subtreeWaiting(s));
  assert.ok(firstQuiescent, "the enqueued batch makes the subtree quiescent");
  assert.deepEqual({ turnOwed: firstQuiescent.turnOwed, turnsStarted: firstQuiescent.turnsStarted }, { turnOwed: true, turnsStarted: 1 });

  child.fire("agent_start");
  const continued = child.uplink.snapshots().at(-1)!;
  assert.deepEqual({ turnOwed: continued.turnOwed, turnsStarted: continued.turnsStarted }, { turnOwed: false, turnsStarted: 2 });
  assert.deepEqual(ownedFlipSends(child.uplink.snapshots()), [], "owing and discharging the turn ride sends that already happen");
});

test("child wake accounting: a steer consumed inside the running loop owes no turn", async () => {
  const child = childProcess(false);
  child.fire("agent_start");
  pushToLead({ sendMessage: (message: unknown) => capturePush(child.sent, message) } as never, child.registry, child.grandchild, "ws-agent-question", { question: "which base?" }, "steer");
  assert.equal(child.sent.length, 1, "the steer went straight into the running turn");
  assert.equal(ownTurnRef.owed, false);
  assert.ok(child.uplink.snapshots().every(s => !s.turnOwed));
});

test("child send volume: a turn start sends one snapshot; unchanged state, streamed deltas, and duplicate events send nothing", async () => {
  const child = childProcess(false);
  const count = () => child.uplink.snapshots().length;
  const base = count();
  child.fire("agent_start");
  assert.equal(count(), base + 1, "the count change rides exactly one snapshot");
  publishSubtree(child.registry);
  child.fire("agent_end");
  assert.equal(count(), base + 1, "nothing changed, nothing sent");
  child.grandchildEvents({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "tok" } });
  child.grandchildEvents({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "en" } });
  assert.equal(count(), base + 1, "streamed deltas send nothing");
  await child.grandchildSettles();
  const settled = count();
  child.grandchildEvents({ type: "agent_settled" });
  await drain();
  assert.equal(count(), settled, "a duplicate settle sends nothing");
  child.fire("agent_start");
  const next = child.uplink.snapshots().slice(settled);
  assert.deepEqual(next.map(s => s.turnsStarted), next.map(() => 2), "the count rides the flush's first snapshot, not a send of its own");
  assert.deepEqual(next.slice(1).map(s => s.descendants.map(d => d.live)), next.slice(1).map(() => [false]), "any later send is the delivered grandchild's parking");
});

test("a disconnect with work still outstanding keeps reading waiting and holding after the reconnect", async () => {
  const { parent, channel, admitted } = await heldOnDescendants();
  channel.drop();
  channel.hello({ subtree: quiescentSnapshot(3, { active: 1 }) });
  assert.equal(parent.waitingOnChildren, true);
  assert.deepEqual(await admitted(), []);
  channel.drop();
  channel.hello({});
  assert.deepEqual(await admitted(), [], "a reconnect with no snapshot holds too");
});

test("exit releases a wait left by a channel that dropped during outstanding work, and the closed launch cannot re-arm it", async () => {
  const sent: any[] = [];
  const h = pushHarness(sent);
  const channel = fakeParentChannel();
  const exiting = record("exiting", { client: h.client, running: true, workGeneration: 1, channel: channel.parent });
  const registry = new Map([[exiting.agentId, exiting]]);
  observeChildSubtree(registry, exiting, channel.parent);
  attachEventListener(h.pi, registry, exiting, h.client);
  channel.deliver(quiescentSnapshot(1, { active: 1 }));
  h.emit(assistantEnd("last output before exit"));
  h.emit({ type: "agent_settled" });
  channel.drop();
  await drain();
  assert.equal(exiting.waitingOnChildren, true);
  markAgentExited(h.pi, registry, exiting);
  assert.equal(exiting.waitingOnChildren, false, "exit releases the wait");
  assert.equal(channel.closed, true);
  channel.drop();
  channel.hello({ subtree: quiescentSnapshot(1, { active: 1 }) });
  assert.equal(exiting.waitingOnChildren, false, "the closed launch's late callbacks cannot re-arm the wait");
  flushHeldPushes(h.pi, true);
  assert.equal(sent.length, 1);
  assert.equal(sent[0].details.reason, "exited");
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

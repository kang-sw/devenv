import { afterEach, test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
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
  markAgentExited,
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

const dirs: string[] = [];
function home(): string {
  const dir = mkdtempSync(join(tmpdir(), "ws-subtree-test-"));
  dirs.push(dir);
  return dir;
}
function blockPublisherPath(path: string): void {
  const directory = dirname(path);
  rmSync(directory, { recursive: true, force: true });
  writeFileSync(directory, "publisher directory replaced by a file");
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

test("subtree publication skips equivalent writes but preserves every effective transport transition", () => {
  const registry: RpcAgentRegistry = new Map();
  const channel = { path: join(home(), "publisher", "subtree.json"), nonce: "dedupe-a" };
  let deliveries = 0;
  installSubtreePublisher(registry, channel, () => deliveries);
  let inode = statSync(channel.path, { bigint: true }).ino;
  const expectPhysicalWrite = (change: () => void): void => {
    change();
    const next = statSync(channel.path, { bigint: true }).ino;
    assert.notEqual(next, inode);
    inode = next;
  };

  publishSubtree(registry);
  assert.equal(statSync(channel.path, { bigint: true }).ino, inode, "an identical effective snapshot does not replace the file");

  let finish!: () => void;
  expectPhysicalWrite(() => { finish = beginSubtreeDispatch(registry); });
  assert.equal(readSubtreeSnapshot(channel)?.active, 1, "dispatch admission remains observable");
  expectPhysicalWrite(finish);
  assert.equal(readSubtreeSnapshot(channel)?.active, 0);

  const child = record("dedupe-child");
  expectPhysicalWrite(() => { registry.set(child.agentId, child); publishSubtree(registry); });
  assert.equal(readSubtreeSnapshot(channel)?.delegated, true);
  assert.equal(readSubtreeSnapshot(channel)?.descendants[0]?.id, child.agentId);

  expectPhysicalWrite(() => { child.running = true; publishSubtree(registry); });
  assert.equal(readSubtreeSnapshot(channel)?.active, 1);
  expectPhysicalWrite(() => { child.waitingOnChildren = true; publishSubtree(registry); });
  assert.equal(readSubtreeSnapshot(channel)?.outstanding, 1);
  expectPhysicalWrite(() => { deliveries = 1; publishSubtree(registry); });
  assert.equal(readSubtreeSnapshot(channel)?.deliveries, 1);
  expectPhysicalWrite(() => {
    child.subtreeDescendants = [{ id: "nested", parentId: null, depth: 0, role: "explore", live: true }];
    publishSubtree(registry);
  });
  assert.equal(readSubtreeSnapshot(channel)?.descendants.some((row) => row.id === "nested"), true);
  expectPhysicalWrite(() => { channel.nonce = "dedupe-b"; publishSubtree(registry); });
  assert.equal(readSubtreeSnapshot(channel)?.nonce, "dedupe-b");
  expectPhysicalWrite(() => { publishSubtree(registry, true); });
  assert.equal(readSubtreeSnapshot(channel)?.revision, 1);
});

test("the initial busy publication remains a hard gate before nested launch", () => {
  const registry: RpcAgentRegistry = new Map();
  const channel = { path: join(home(), "publisher", "subtree.json"), nonce: "hard-gate" };
  installSubtreePublisher(registry, channel, () => 0);
  blockPublisherPath(channel.path);
  let launched = false;
  assert.throws(() => {
    const finish = beginSubtreeDispatch(registry);
    launched = true;
    finish();
  });
  assert.equal(launched, false, "grandchild launch code is unreachable after the hard gate fails");
});

test("RPC settlement survives a secondary upstream publication failure", async () => {
  const notices: string[] = [];
  const sent: any[] = [];
  const h = pushHarness(sent);
  const child = record("publication-failure", { client: h.client, running: true, workGeneration: 1 });
  const registry: RpcAgentRegistry = new Map([[child.agentId, child]]);
  const channel = { path: join(home(), "publisher", "subtree.json"), nonce: "soft-event" };
  installSubtreePublisher(registry, channel, () => 0, (detail) => { notices.push(detail); return true; });
  attachEventListener(h.pi, registry, child, h.client);
  blockPublisherPath(channel.path);

  h.emit(assistantEnd("settled through publication failure"));
  h.emit({ type: "agent_settled" });
  await drain();
  assert.equal(child.running, false);
  assert.equal(child.terminalDelivery?.state, "held", "settlement admission still executes");
  flushHeldPushes(h.pi, true);
  await drain();
  assert.equal(sent[0]?.details.last_message, "settled through publication failure");
  assert.deepEqual(notices, ["ws-pi-agent: subtree publication failed"], "equivalent failures are diagnosed once");
});

test("watcher publication failure is contained and watcher cleanup remains callable", async () => {
  const notices: string[] = [];
  const observedChannel = { path: join(home(), "observed", "subtree.json"), nonce: "observed" };
  const nestedRegistry: RpcAgentRegistry = new Map();
  installSubtreePublisher(nestedRegistry, observedChannel, () => 0);

  const h = pushHarness([]);
  const child = record("watched-child", { client: h.client, subtreeChannel: observedChannel, launchGeneration: 1 });
  const registry: RpcAgentRegistry = new Map([[child.agentId, child]]);
  const upstreamChannel = { path: join(home(), "upstream", "subtree.json"), nonce: "upstream" };
  installSubtreePublisher(registry, upstreamChannel, () => 0, (detail) => { notices.push(detail); return true; });
  attachEventListener(h.pi, registry, child, h.client);
  blockPublisherPath(upstreamChannel.path);

  const nested = record("nested-live", { client: {} as never, running: true, spawnRole: "explore" });
  nestedRegistry.set(nested.agentId, nested);
  publishSubtree(nestedRegistry, true);
  const deadline = Date.now() + 2_000;
  while (!child.subtreeDescendants?.some((row) => row.id === nested.agentId) && Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, 10));
  }

  assert.equal(child.waitingOnChildren, true);
  assert.equal(child.subtreeDescendants?.some((row) => row.id === nested.agentId), true);
  assert.doesNotThrow(() => child.unsubscribe?.(), "publication failure cannot prevent watcher cleanup");
  assert.deepEqual(notices, ["ws-pi-agent: subtree publication failed"]);
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
  const channel = { path: join(home(), "root-subtree.json"), nonce: "root" };
  const h = pushHarness([]);
  const child = record("child", {
    client: h.client,
    running: true,
    waitingOnChildren: true,
    subtreeRevision: 4,
    subtreeDescendants: [{ id: "grandchild", parentId: null, depth: 0, role: "worker", live: true }],
  });
  const registry: RpcAgentRegistry = new Map([[child.agentId, child]]);
  installSubtreePublisher(registry, channel, () => 0);

  markAgentExited(h.pi, registry, child, { suppressTerminal: true });

  assert.equal(child.waitingOnChildren, false);
  assert.equal(child.subtreeRevision, undefined);
  assert.deepEqual(child.subtreeDescendants, []);
  assert.deepEqual(readSubtreeSnapshot(channel)?.descendants, [
    { id: "child", parentId: null, depth: 0, role: "worker", live: false },
  ]);
});

test("channel notifications propagate a nested parent edge through two process hops", async () => {
  const leafChannel = { path: join(home(), "leaf-subtree.json"), nonce: "leaf" };
  const middleChannel = { path: join(home(), "middle-subtree.json"), nonce: "middle" };
  const middleHarness = pushHarness([]);
  const rootHarness = pushHarness([]);
  const child = record("child", { client: middleHarness.client, subtreeChannel: leafChannel });
  const middleRegistry: RpcAgentRegistry = new Map([[child.agentId, child]]);
  installSubtreePublisher(middleRegistry, middleChannel, () => 0);
  attachEventListener(middleHarness.pi, middleRegistry, child, middleHarness.client);

  const parent = record("parent", { client: rootHarness.client, subtreeChannel: middleChannel });
  const rootRegistry: RpcAgentRegistry = new Map([[parent.agentId, parent]]);
  installSubtreePublisher(rootRegistry, undefined, () => 0);
  attachEventListener(rootHarness.pi, rootRegistry, parent, rootHarness.client);

  try {
    const grandchild = record("grandchild", { client: {} as never, running: true, spawnRole: "explore" });
    installSubtreePublisher(new Map([[grandchild.agentId, grandchild]]), leafChannel, () => 0);
    const deadline = Date.now() + 2_000;
    while (!parent.subtreeDescendants?.some((row) => row.id === grandchild.agentId) && Date.now() < deadline) {
      await new Promise((resolve) => setTimeout(resolve, 10));
    }

    assert.deepEqual(parent.subtreeDescendants, [
      { id: "child", parentId: null, depth: 0, role: "worker", live: true },
      { id: "grandchild", parentId: "child", depth: 1, role: "explore", live: true },
    ]);
    assert.deepEqual(publishSubtree(rootRegistry)?.descendants, [
      { id: "parent", parentId: null, depth: 0, role: "worker", live: true },
      { id: "child", parentId: "parent", depth: 1, role: "worker", live: true },
      { id: "grandchild", parentId: "child", depth: 2, role: "explore", live: true },
    ]);
  } finally {
    child.unsubscribe?.();
    parent.unsubscribe?.();
  }
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
  assert.deepEqual(parent.subtreeDescendants, [
    { id: "grandchild", parentId: null, depth: 0, role: "worker", live: false },
  ], "the same child snapshot read that drives waiting also retains advisory identity");
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

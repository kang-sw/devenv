/**
 * 261028 Phase 2: unit tests for `agent-view.ts`, the descendant tool-batch
 * relay over the parent-child control channel. The message parse and the hop
 * merge are pure; the child half runs against a fake channel with an injected
 * clock and mock timers; the parent half against a fake `ParentChannel`. One
 * test chains three hops through fake links, and one runs a real socket pair.
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  AGENT_VIEW_MESSAGE,
  AgentViewUpstream,
  agentViewMessage,
  buildAgentViewModel,
  observeAgentView,
  parseAgentViewMessage,
  type AgentViewChildChannel,
  type AgentViewModel,
  type AgentViewParentChannel,
} from "../src/agent-view.ts";
import { AGENT_BATCH_CAP, BATCH_ARGS_PREVIEW_CAP, BATCH_TOOL_NAME_CAP, batchViews, createBatchBuffer, observeBatchEvent, type AgentBatchBuffer, type BatchView } from "../src/agent-batches.ts";
import { MAX_SUBTREE_DESCENDANTS, SUBTREE_TELEMETRY_INTERVAL_MS } from "../src/subtree-lifecycle.ts";
import { ChildChannel, ParentChannel, readAndDeleteChannelBootstrap } from "../src/agent-channel.ts";
import { collectBatchViews } from "../src/agent-widget.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "../src/spawner.ts";

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const finished = (seq: number, overrides: Record<string, unknown> = {}) => ({ seq, toolNames: ["read"], callCount: 1, failed: 0, startedAt: seq * 10, endedAt: seq * 10 + 5, ...overrides });
const runningBatch = (seq: number, overrides: Record<string, unknown> = {}) => ({
  seq, toolNames: ["bash"], callCount: 2, failed: 0, startedAt: seq * 10,
  running: { toolName: "bash", argsPreview: "command: ls", startedAt: seq * 10 + 1, done: 1, othersRunning: 0 },
  ...overrides,
});
/** Wire round trip: what actually crosses the channel. */
const wire = <T>(value: T): T => JSON.parse(JSON.stringify(value));

/** A buffer built through the real event path; each entry is a finished batch or, for "running", an open one (last only). */
function bufferOf(tag: string, kinds: Array<"done" | "running">): AgentBatchBuffer {
  const buffer = createBatchBuffer();
  kinds.forEach((kind, i) => {
    const id = `${tag}-${i}`;
    const at = 1_000 * (i + 1);
    observeBatchEvent(buffer, { type: "message_end", message: { role: "assistant", content: [{ type: "toolCall", id, name: `${tag}${i}`, arguments: { n: i } }] } }, at);
    observeBatchEvent(buffer, { type: "tool_execution_start", toolCallId: id, toolName: `${tag}${i}` }, at + 1);
    if (kind === "done") observeBatchEvent(buffer, { type: "tool_execution_end", toolCallId: id }, at + 2);
  });
  return buffer;
}

function rec(agentId: string, overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return {
    agentId,
    sessionPath: "/tmp/ws-pi-agent-x/session.jsonl",
    systemPromptPath: "/tmp/ws-pi-agent-x/prompt.md",
    wsToolNames: [],
    toolGroup: "full-worker",
    streaming: false,
    running: false,
    reportLog: [],
    client: {} as never,
    ...overrides,
  };
}
const registryOf = (...records: RpcAgentRecord[]): RpcAgentRegistry => new Map(records.map((record) => [record.agentId, record]));
const nested = (id: string, parentId: string | null = null, depth = 0) => ({ id, parentId, depth, role: "worker" as const, live: true });

// ---------------------------------------------------------------------------
// parseAgentViewMessage
// ---------------------------------------------------------------------------

describe("parseAgentViewMessage", () => {
  test("round-trips agentViewMessage(model) over the wire, order included", () => {
    const model: AgentViewModel = new Map([
      ["zeta", batchViews(bufferOf("z", ["done", "running"]))],
      ["alpha", batchViews(bufferOf("a", ["done"]))],
    ]);
    const message = wire(agentViewMessage(model));
    assert.equal(message.t, AGENT_VIEW_MESSAGE);
    assert.equal(AGENT_VIEW_MESSAGE, "agent-view");
    const parsed = parseAgentViewMessage(message)!;
    assert.deepEqual(parsed, model);
    assert.deepEqual([...parsed.keys()], ["zeta", "alpha"], "parent-before-child order survives");
    assert.equal(parseAgentViewMessage(wire(agentViewMessage(new Map())))!.size, 0, "an empty model is a valid message");
  });

  test("undefined only when `agents` is not an array", () => {
    for (const raw of [undefined, null, 5, "x", {}, { agents: "x" }, { agents: {} }, { agents: null }]) {
      assert.equal(parseAgentViewMessage(raw), undefined, JSON.stringify(raw));
    }
    assert.deepEqual(parseAgentViewMessage({ agents: [] }), new Map());
    assert.deepEqual(parseAgentViewMessage({ agents: [null, 3, "x"] }), new Map(), "junk entries are dropped, not fatal");
  });

  test("malformed batches are dropped individually and the valid ones kept in order", () => {
    const bad = [
      finished(0), finished(-1), finished(1.5), finished(2, { seq: "2" }),
      finished(3, { callCount: -1 }), finished(4, { callCount: 1.5 }), finished(5, { startedAt: null }), finished(6, { startedAt: -1 }),
      finished(7, { toolNames: "read" }), finished(8, { toolNames: undefined }),
      finished(9, { endedAt: undefined }), // neither endedAt nor running
      finished(10, { running: runningBatch(10).running }), // both
      finished(11, { endedAt: "later" }),
      runningBatch(12, { running: { toolName: "bash", argsPreview: "", startedAt: 1, done: 0 } }),
      runningBatch(13, { running: { toolName: 7, argsPreview: "", startedAt: 1, done: 0, othersRunning: 0 } }),
      runningBatch(14, { running: { toolName: "bash", argsPreview: 1, startedAt: 1, done: 0, othersRunning: 0 } }),
      runningBatch(15, { running: { toolName: "bash", argsPreview: "", startedAt: 1, done: -1, othersRunning: 0 } }),
      runningBatch(16, { running: { toolName: "bash", argsPreview: "", startedAt: 1, done: 0, othersRunning: 1.5 } }),
      runningBatch(17, { running: { toolName: "bash", argsPreview: "", startedAt: "x", done: 0, othersRunning: 0 } }),
      runningBatch(18, { running: "bash" }),
      null, 7, "batch",
    ];
    const good1 = finished(20);
    const good2 = runningBatch(21);
    const parsed = parseAgentViewMessage({ agents: [{ id: "a", batches: [...bad.slice(0, 5), good1, ...bad.slice(5), good2] }] })!;
    assert.deepEqual(parsed.get("a")!.map((batch) => batch.seq), [20, 21]);
  });

  test("malformed agent entries are dropped, and an agent with no valid batch is omitted", () => {
    const parsed = parseAgentViewMessage({ agents: [
      { id: "", batches: [finished(1)] },
      { id: 7, batches: [finished(1)] },
      { id: "no-batches" },
      { id: "batches-object", batches: {} },
      { id: "all-bad", batches: [finished(0), null] },
      { id: "empty", batches: [] },
      null,
      { id: "ok", batches: [finished(1)] },
    ] })!;
    assert.deepEqual([...parsed.keys()], ["ok"]);
  });

  test("bounds: MAX_SUBTREE_DESCENDANTS agents with the first id winning, the newest AGENT_BATCH_CAP batches, tool names and args preview capped", () => {
    const many = Array.from({ length: MAX_SUBTREE_DESCENDANTS + 10 }, (_, i) => ({ id: `a${i}`, batches: [finished(1)] }));
    const capped = parseAgentViewMessage({ agents: many })!;
    assert.equal(capped.size, MAX_SUBTREE_DESCENDANTS);
    assert.ok(capped.has(`a${MAX_SUBTREE_DESCENDANTS - 1}`) && !capped.has(`a${MAX_SUBTREE_DESCENDANTS}`));

    const dup = parseAgentViewMessage({ agents: [{ id: "x", batches: [finished(1)] }, { id: "x", batches: [finished(2)] }] })!;
    assert.deepEqual(dup.get("x")!.map((batch) => batch.seq), [1], "the first occurrence wins");

    const seqs = Array.from({ length: AGENT_BATCH_CAP + 4 }, (_, i) => finished(i + 1));
    const newest = parseAgentViewMessage({ agents: [{ id: "x", batches: seqs }] })!.get("x")!;
    assert.deepEqual(newest.map((batch) => batch.seq), [5, 6, 7, 8, 9], "the newest five");

    const names = Array.from({ length: BATCH_TOOL_NAME_CAP + 3 }, (_, i) => `tool${i}`);
    const long = parseAgentViewMessage({ agents: [{ id: "x", batches: [
      finished(1, { toolNames: [...names, ""] }),
      runningBatch(2, { running: { ...runningBatch(2).running, argsPreview: "p".repeat(BATCH_ARGS_PREVIEW_CAP + 100), toolName: "t".repeat(200) } }),
    ] }] })!.get("x")!;
    assert.deepEqual(long[0].toolNames, names.slice(0, BATCH_TOOL_NAME_CAP));
    assert.equal([...long[1].running!.argsPreview].length, BATCH_ARGS_PREVIEW_CAP);
    assert.ok(long[1].running!.argsPreview.endsWith("…"));
    assert.ok([...long[1].running!.toolName].length <= 64, "a relayed tool name is bounded too");
  });

  test("261028 redesign: `failed` and a generating view round-trip; malformed ones are dropped", () => {
    const generating = (seq: number, phase: Record<string, unknown>, overrides: Record<string, unknown> = {}) => ({ seq, toolNames: [], callCount: 0, failed: 0, startedAt: seq * 10, generating: phase, ...overrides });
    const good = [
      finished(1, { callCount: 3, failed: 2 }),
      runningBatch(2, { failed: 1 }),
      generating(3, { kind: "thinking", phaseStartedAt: 31 }),
      generating(4, { kind: "replying", phaseStartedAt: 41 }),
      generating(5, { kind: "tool", phaseStartedAt: 51, toolName: "write", chars: 12_345 }),
    ];
    const parsed = parseAgentViewMessage(wire({ agents: [{ id: "a", batches: good }] }))!.get("a")!;
    assert.deepEqual(parsed, good, "the new fields survive the wire unchanged");

    const bad = [
      finished(10, { failed: undefined }), finished(11, { failed: -1 }), finished(12, { failed: 1.5 }), finished(13, { failed: 2 }),
      generating(14, { kind: "thinking", phaseStartedAt: 1 }, { endedAt: 5 }), // two of endedAt / running / generating
      generating(15, { kind: "dreaming", phaseStartedAt: 1 }),
      generating(16, { kind: "thinking" }),
      generating(17, { kind: "tool", phaseStartedAt: 1, chars: 3 }), // no tool name
      generating(18, { kind: "tool", phaseStartedAt: 1, toolName: "write", chars: -3 }),
      generating(19, "thinking"),
    ];
    const kept = parseAgentViewMessage({ agents: [{ id: "a", batches: [...bad, generating(20, { kind: "thinking", phaseStartedAt: 1, toolName: "ignored", chars: 9 })] }] })!.get("a")!;
    assert.deepEqual(kept, [generating(20, { kind: "thinking", phaseStartedAt: 1 })], "a non-tool phase drops stray tool fields");
    const controls = parseAgentViewMessage({ agents: [{ id: "a", batches: [generating(1, { kind: "tool", phaseStartedAt: 1, toolName: "wr\u001bite" + "x".repeat(100), chars: 1 })] }] })!.get("a")!;
    assert.ok(!/[\u0000-\u001f]/.test(controls[0].generating!.toolName!));
    assert.ok([...controls[0].generating!.toolName!].length <= 64);
  });

  test("a relayed epoch survives the wire; a malformed or absent one is dropped without dropping the batch", () => {
    const parsed = parseAgentViewMessage(wire({ t: AGENT_VIEW_MESSAGE, agents: [{ id: "x", batches: [
      finished(1, { epoch: "ab12-_Z" }),
      finished(2, { epoch: "bad:epoch" }),
      finished(3, { epoch: 7 as never }),
      finished(4),
    ] }] }))!.get("x")!;
    assert.deepEqual(parsed.map((view) => [view.seq, view.epoch]), [[1, "ab12-_Z"], [2, undefined], [3, undefined], [4, undefined]]);
    assert.equal("epoch" in parsed[1], false);
  });

  test("control characters in relayed strings never reach the widget", () => {
    const parsed = parseAgentViewMessage({ agents: [{ id: "x", batches: [
      finished(1, { toolNames: ["re\u001b[31mad\u0000"] }),
      runningBatch(2, { running: { ...runningBatch(2).running, argsPreview: "a\nb\u0007c", toolName: "ba\tsh" } }),
    ] }] })!.get("x")!;
    assert.ok(!/[\u0000-\u001f\u007f-\u009f]/.test(parsed[0].toolNames[0]));
    assert.ok(!/[\u0000-\u001f\u007f-\u009f]/.test(parsed[1].running!.argsPreview));
    assert.ok(!/[\u0000-\u001f\u007f-\u009f]/.test(parsed[1].running!.toolName));
  });
});

// ---------------------------------------------------------------------------
// buildAgentViewModel
// ---------------------------------------------------------------------------

describe("buildAgentViewModel", () => {
  test("merges direct children's own batches with each child's forwarded descendant views", () => {
    const child = rec("child", {
      toolBatches: bufferOf("c", ["done", "running"]),
      subtreeDescendants: [nested("grand")],
      descendantViews: new Map([["grand", batchViews(bufferOf("g", ["done"]))]]),
    });
    const model = buildAgentViewModel(registryOf(child));
    assert.deepEqual([...model.keys()], ["child", "grand"]);
    assert.deepEqual(model.get("child"), batchViews(child.toolBatches));
    assert.deepEqual(model.get("grand"), child.descendantViews!.get("grand"));
  });

  test("a local buffer wins over a forwarded entry for the same id", () => {
    const local = rec("shared", { toolBatches: bufferOf("local", ["done"]) });
    const other = rec("other", { descendantViews: new Map([["shared", [wire(finished(99)) as unknown as BatchView]]]) });
    const model = buildAgentViewModel(registryOf(other, local));
    assert.deepEqual(model.get("shared"), batchViews(local.toolBatches));
  });

  test("order follows the subtree snapshot's parent-before-child selection, not the forwarding order", () => {
    const c1 = rec("c1", {
      toolBatches: bufferOf("a", ["done"]),
      subtreeDescendants: [nested("c1-g"), nested("c1-gg", "c1-g", 1)],
      // Forwarded in scrambled order.
      descendantViews: new Map([["c1-gg", batchViews(bufferOf("x", ["done"]))], ["c1-g", batchViews(bufferOf("y", ["done"]))]]),
    });
    const c2 = rec("c2", { toolBatches: bufferOf("b", ["done"]), subtreeDescendants: [nested("c2-g")], descendantViews: new Map([["c2-g", batchViews(bufferOf("z", ["done"]))]]) });
    assert.deepEqual([...buildAgentViewModel(registryOf(c1, c2)).keys()], ["c1", "c1-g", "c1-gg", "c2", "c2-g"]);
  });

  test("the bound: a forwarded entry outside the subtree snapshot's selection is dropped", () => {
    const child = rec("child", {
      toolBatches: bufferOf("c", ["done"]),
      subtreeDescendants: [nested("known")],
      descendantViews: new Map([["known", batchViews(bufferOf("k", ["done"]))], ["ghost", batchViews(bufferOf("g", ["done"]))]]),
    });
    assert.deepEqual([...buildAgentViewModel(registryOf(child)).keys()], ["child", "known"], "an agent absent from subtreeDescendants is not relayed");
  });

  test("the bound past MAX_SUBTREE_DESCENDANTS keeps exactly the agents the snapshot keeps", () => {
    const ids = Array.from({ length: MAX_SUBTREE_DESCENDANTS + 12 }, (_, i) => `d${i}`);
    const child = rec("child", {
      toolBatches: bufferOf("c", ["done"]),
      subtreeDescendants: ids.map((id) => nested(id)),
      descendantViews: new Map(ids.map((id) => [id, batchViews(bufferOf("d", ["done"]))])),
    });
    const model = buildAgentViewModel(registryOf(child));
    assert.equal(model.size, MAX_SUBTREE_DESCENDANTS);
    assert.deepEqual([...model.keys()].slice(0, 2), ["child", "d0"]);
    assert.ok(model.has(`d${MAX_SUBTREE_DESCENDANTS - 2}`));
    assert.ok(!model.has(`d${MAX_SUBTREE_DESCENDANTS - 1}`), "the 129th agent is cut, as in the snapshot");
  });

  test("agents with no batches are omitted", () => {
    const quiet = rec("quiet", { toolBatches: createBatchBuffer(), subtreeDescendants: [nested("silent")] });
    const bare = rec("bare");
    const busy = rec("busy", { toolBatches: bufferOf("b", ["running"]) });
    assert.deepEqual([...buildAgentViewModel(registryOf(quiet, bare, busy)).keys()], ["busy"]);
    assert.equal(buildAgentViewModel(registryOf()).size, 0);
  });
});

// ---------------------------------------------------------------------------
// AgentViewUpstream
// ---------------------------------------------------------------------------

function fakeChild() {
  const reconnectListeners = new Set<() => void>();
  const link = {
    connected: true,
    closed: false,
    sent: [] as Array<{ t: string; agents: Array<{ id: string; batches: BatchView[] }> }>,
    failSend: false,
    send(msg: Record<string, unknown>) {
      if (link.failSend || !link.connected) throw new Error("ws-pi-channel: not connected to the parent");
      link.sent.push(msg as never);
    },
    onReconnect(cb: () => void) { reconnectListeners.add(cb); return () => { reconnectListeners.delete(cb); }; },
    reconnect() { link.connected = true; for (const cb of [...reconnectListeners]) cb(); },
    get channel(): AgentViewChildChannel { return link as unknown as AgentViewChildChannel; },
  };
  return link;
}

/** A mutable model source plus a clock the test advances together with the mock timers. */
function upstreamHarness(t: { mock: { timers: { enable(options: { apis: string[] }): void; tick(ms: number): void } } }, intervalMs = 1_000) {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const link = fakeChild();
  let clock = 5_000;
  const state = { model: new Map([["a", batchViews(bufferOf("a", ["done"]))]]) as AgentViewModel };
  const upstream = new AgentViewUpstream(link.channel, { intervalMs, now: () => clock });
  upstream.setSource(() => state.model);
  const advance = (ms: number) => { clock += ms; t.mock.timers.tick(ms); };
  const change = (tag: string) => { state.model = new Map([["a", batchViews(bufferOf(tag, ["done"]))]]); };
  return { link, upstream, state, advance, change };
}

describe("AgentViewUpstream", () => {
  test("the first schedule publishes immediately as an agent-view message", (t) => {
    const { link, upstream, state } = upstreamHarness(t);
    upstream.schedule();
    assert.equal(link.sent.length, 1);
    assert.deepEqual(link.sent[0], wire(agentViewMessage(state.model)));
    assert.equal(link.sent[0].t, "agent-view");
  });

  test("the default interval is the subtree telemetry interval", (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const link = fakeChild();
    let clock = 0;
    const upstream = new AgentViewUpstream(link.channel, { now: () => clock });
    let model: AgentViewModel = new Map([["a", batchViews(bufferOf("a", ["done"]))]]);
    upstream.setSource(() => model);
    upstream.schedule();
    model = new Map([["a", batchViews(bufferOf("b", ["done"]))]]);
    upstream.schedule();
    clock += SUBTREE_TELEMETRY_INTERVAL_MS - 1;
    t.mock.timers.tick(SUBTREE_TELEMETRY_INTERVAL_MS - 1);
    assert.equal(link.sent.length, 1);
    clock += 1;
    t.mock.timers.tick(1);
    assert.equal(link.sent.length, 2);
  });

  test("a change within the interval publishes once at the interval end, and repeated schedules coalesce", (t) => {
    const { link, upstream, advance, change } = upstreamHarness(t);
    upstream.schedule();
    advance(100);
    change("b");
    upstream.schedule();
    change("c");
    upstream.schedule();
    upstream.schedule();
    assert.equal(link.sent.length, 1, "throttled");
    advance(899);
    assert.equal(link.sent.length, 1, "still inside the interval");
    advance(1);
    assert.equal(link.sent.length, 2, "one publish at the interval end");
    assert.equal(link.sent[1].agents[0].batches[0].toolNames[0], "c0", "carrying the latest model, read at fire time");
    advance(5_000);
    assert.equal(link.sent.length, 2, "nothing further without a new schedule");
  });

  test("a schedule after the interval has elapsed publishes at once", (t) => {
    const { link, upstream, advance, change } = upstreamHarness(t);
    upstream.schedule();
    advance(1_000);
    change("b");
    upstream.schedule();
    assert.equal(link.sent.length, 2);
  });

  test("an unchanged model is not resent", (t) => {
    const { link, upstream, advance } = upstreamHarness(t);
    upstream.schedule();
    advance(1_000);
    upstream.schedule();
    upstream.publish();
    advance(1_000);
    upstream.schedule();
    assert.equal(link.sent.length, 1);
  });

  test("an unchanged rebuild holds off the next build for an interval", (t) => {
    const { link, upstream, state, advance } = upstreamHarness(t);
    let builds = 0;
    upstream.setSource(() => { builds += 1; return state.model; });
    upstream.schedule();
    advance(1_000);
    upstream.schedule();
    assert.equal(builds, 2, "the schedule past the interval rebuilds");
    upstream.schedule();
    upstream.schedule();
    assert.equal(builds, 2, "repeated schedules inside the interval wait for one timer");
    advance(1_000);
    assert.equal(builds, 3, "the timer rebuilds once at the interval end");
    assert.equal(link.sent.length, 1, "an unchanged model is still not resent");
  });

  test("while disconnected nothing is sent and nothing is queued; the reconnect resends the current model once", (t) => {
    const { link, upstream, advance, change } = upstreamHarness(t);
    upstream.schedule();
    link.connected = false;
    advance(1_000);
    change("b");
    upstream.schedule();
    change("c");
    upstream.schedule();
    advance(5_000);
    assert.equal(link.sent.length, 1, "no send and no throw while disconnected");
    link.reconnect();
    assert.equal(link.sent.length, 2, "one resend, not one per missed change");
    assert.equal(link.sent[1].agents[0].batches[0].toolNames[0], "c0");
  });

  test("onReconnect resends the current model even when unchanged", (t) => {
    const { link, upstream } = upstreamHarness(t);
    upstream.schedule();
    link.reconnect();
    assert.equal(link.sent.length, 2);
    assert.deepEqual(link.sent[1], link.sent[0], "the parent dropped it at the disconnect, so it is sent again unchanged");
    link.reconnect();
    assert.equal(link.sent.length, 3);
  });

  test("setSource(undefined) cancels a pending timer and stops publishing; a new source resumes", (t) => {
    const { link, upstream, state, advance, change } = upstreamHarness(t);
    upstream.schedule();
    advance(10);
    change("b");
    upstream.schedule();
    upstream.setSource(undefined);
    advance(5_000);
    assert.equal(link.sent.length, 1, "the pending publish never fires");
    upstream.schedule();
    upstream.publish();
    link.reconnect();
    assert.equal(link.sent.length, 1, "no source: schedule, publish and reconnect all send nothing");
    upstream.setSource(() => state.model);
    upstream.schedule();
    assert.equal(link.sent.length, 2);
  });

  test("a closed channel is never scheduled or published to", (t) => {
    const { link, upstream } = upstreamHarness(t);
    link.closed = true;
    upstream.schedule();
    upstream.publish();
    assert.equal(link.sent.length, 0);
  });

  test("a throwing source never throws out and the next publish recovers", (t) => {
    const { link, upstream, state, advance } = upstreamHarness(t);
    let broken = true;
    upstream.setSource(() => { if (broken) throw new Error("registry mid-mutation"); return state.model; });
    assert.doesNotThrow(() => upstream.schedule());
    assert.doesNotThrow(() => upstream.publish());
    assert.equal(link.sent.length, 0);
    broken = false;
    advance(0);
    upstream.schedule();
    assert.equal(link.sent.length, 1);
  });

  test("a throwing send never throws out, is not recorded as sent, and is retried by the next publish", (t) => {
    const { link, upstream } = upstreamHarness(t);
    link.failSend = true;
    assert.doesNotThrow(() => upstream.schedule());
    assert.doesNotThrow(() => upstream.publish());
    assert.equal(link.sent.length, 0);
    link.failSend = false;
    upstream.publish();
    assert.equal(link.sent.length, 1, "the unchanged-key skip does not swallow a model that was never delivered");
  });
});

// ---------------------------------------------------------------------------
// observeAgentView
// ---------------------------------------------------------------------------

function fakeParent() {
  const messageListeners = new Set<(msg: Record<string, unknown>) => void>();
  const disconnectListeners = new Set<() => void>();
  const channel = {
    onMessage(cb: (msg: Record<string, unknown>) => void) { messageListeners.add(cb); return () => { messageListeners.delete(cb); }; },
    onDisconnect(cb: () => void) { disconnectListeners.add(cb); return () => { disconnectListeners.delete(cb); }; },
    deliver(msg: Record<string, unknown>) { for (const cb of [...messageListeners]) cb(wire(msg)); },
    drop() { for (const cb of [...disconnectListeners]) cb(); },
    listeners: () => messageListeners.size + disconnectListeners.size,
    get parent(): AgentViewParentChannel { return channel as unknown as AgentViewParentChannel; },
  };
  return channel;
}

describe("observeAgentView", () => {
  const views = batchViews(bufferOf("g", ["done", "running"]));
  const model = (): AgentViewModel => new Map([["g", views]]);

  test("stores the parsed model on the record and reports the change", () => {
    const parent = fakeParent();
    const record = rec("child", { channel: parent.parent as never });
    let changes = 0;
    observeAgentView(record, parent.parent as never, () => { changes += 1; });
    parent.deliver(agentViewMessage(model()));
    assert.deepEqual(record.descendantViews, model());
    assert.equal(changes, 1);
    parent.deliver(agentViewMessage(new Map()));
    assert.equal(record.descendantViews!.size, 0, "latest value wins, an empty model included");
    assert.equal(changes, 2);
  });

  test("ignores non-agent-view and malformed messages", () => {
    const parent = fakeParent();
    const record = rec("child", { channel: parent.parent as never });
    let changes = 0;
    observeAgentView(record, parent.parent as never, () => { changes += 1; });
    parent.deliver({ t: "subtree", agents: [] });
    parent.deliver({ t: "ready", kind: "x" });
    parent.deliver({ t: AGENT_VIEW_MESSAGE, agents: "nope" });
    parent.deliver({ t: AGENT_VIEW_MESSAGE });
    assert.equal(record.descendantViews, undefined);
    assert.equal(changes, 0);
    parent.deliver(agentViewMessage(model()));
    parent.deliver({ t: AGENT_VIEW_MESSAGE, agents: 5 });
    assert.deepEqual(record.descendantViews, model(), "a malformed message leaves the last good model");
    assert.equal(changes, 1);
  });

  test("ignores a channel that is no longer the record's current launch", () => {
    const stale = fakeParent();
    const current = fakeParent();
    const record = rec("child", { channel: current.parent as never });
    let changes = 0;
    observeAgentView(record, stale.parent as never, () => { changes += 1; });
    stale.deliver(agentViewMessage(model()));
    assert.equal(record.descendantViews, undefined);
    record.descendantViews = model();
    stale.drop();
    assert.deepEqual(record.descendantViews, model(), "a stale disconnect does not drop the current launch's model");
    assert.equal(changes, 0);
  });

  test("a disconnect drops the model and reports it once", () => {
    const parent = fakeParent();
    const record = rec("child", { channel: parent.parent as never });
    let changes = 0;
    observeAgentView(record, parent.parent as never, () => { changes += 1; });
    parent.deliver(agentViewMessage(model()));
    parent.drop();
    assert.equal(record.descendantViews, undefined);
    assert.equal(changes, 2);
    parent.drop();
    assert.equal(changes, 2, "nothing left to drop, nothing to report");
  });

  test("the returned function detaches both listeners", () => {
    const parent = fakeParent();
    const record = rec("child", { channel: parent.parent as never });
    let changes = 0;
    const off = observeAgentView(record, parent.parent as never, () => { changes += 1; });
    assert.equal(parent.listeners(), 2);
    off();
    assert.equal(parent.listeners(), 0);
    parent.deliver(agentViewMessage(model()));
    assert.equal(record.descendantViews, undefined);
    assert.equal(changes, 0);
  });
});

// ---------------------------------------------------------------------------
// Three-level relay
// ---------------------------------------------------------------------------

/** A fake parent/child link: the child's sends reach the parent's listeners as the wire would deliver them. */
function link() {
  const child = fakeChild();
  const parent = fakeParent();
  const send = child.send;
  child.send = (msg) => { send(msg); parent.deliver(msg); };
  return { child, parent, drop() { child.connected = false; parent.drop(); } };
}

describe("three-level relay", () => {
  test("a grandchild's batches reach the root widget's batch map through two hops, and a mid-level disconnect drops them", () => {
    // Process X (registry: Z) -> link1 -> process M (registry: X) -> link2 -> root (registry: M).
    const z = rec("zzzzzzzz-z", { toolBatches: bufferOf("z", ["done", "running"]) });
    const registryX = registryOf(z);
    const xInM = rec("xxxxxxxx-x", { toolBatches: bufferOf("x", ["done"]), subtreeDescendants: [nested("zzzzzzzz-z")] });
    const registryM = registryOf(xInM);
    const mInRoot = rec("mmmmmmmm-m");
    const registryRoot = registryOf(mInRoot);

    const link2 = link();
    mInRoot.channel = link2.parent.parent as never;
    let rootChanges = 0;
    observeAgentView(mInRoot, link2.parent.parent as never, () => { rootChanges += 1; });
    const upstreamM = new AgentViewUpstream(link2.child.channel, { intervalMs: 0 });
    upstreamM.setSource(() => buildAgentViewModel(registryM));

    const link1 = link();
    xInM.channel = link1.parent.parent as never;
    observeAgentView(xInM, link1.parent.parent as never, () => { upstreamM.schedule(); });
    const upstreamX = new AgentViewUpstream(link1.child.channel, { intervalMs: 0 });
    upstreamX.setSource(() => buildAgentViewModel(registryX));

    upstreamX.schedule();

    // X's process relayed Z (its direct child) to M; M merged X's own batches and forwarded both to the root.
    assert.deepEqual(link1.child.sent.map((msg) => msg.agents.map((agent) => agent.id)), [["zzzzzzzz-z"]]);
    assert.deepEqual(link2.child.sent.map((msg) => msg.agents.map((agent) => agent.id)), [["xxxxxxxx-x", "zzzzzzzz-z"]]);
    const views = collectBatchViews(registryRoot);
    assert.deepEqual(views.get("zzzzzzzz-z"), batchViews(z.toolBatches), "the grandchild's batches, running one included, keyed by its id");
    assert.deepEqual(views.get("xxxxxxxx-x"), batchViews(xInM.toolBatches));
    assert.equal(views.get("zzzzzzzz-z")!.at(-1)!.running?.toolName, "z1");
    assert.ok(rootChanges >= 1);

    // The grandchild's call finishes below: the change flows up again.
    observeBatchEvent(z.toolBatches!, { type: "tool_execution_end", toolCallId: "z-1" }, 9_000);
    upstreamX.schedule();
    assert.equal(collectBatchViews(registryRoot).get("zzzzzzzz-z")!.at(-1)!.running, undefined);
    assert.equal(collectBatchViews(registryRoot).get("zzzzzzzz-z")!.at(-1)!.endedAt, 9_000);

    // X disconnects from M: M drops X's forwarded model and relays the loss.
    link1.drop();
    assert.equal(xInM.descendantViews, undefined);
    const afterX = collectBatchViews(registryRoot);
    assert.ok(!afterX.has("zzzzzzzz-z"), "the grandchild's rows disappear at the root");
    assert.ok(afterX.has("xxxxxxxx-x"), "M's own view of X remains");

    // M disconnects from the root: everything forwarded through it is dropped.
    link2.drop();
    assert.equal(mInRoot.descendantViews, undefined);
    assert.equal(collectBatchViews(registryRoot).size, 0);
  });
});

describe("three-level relay: generating phase and failures (261028 redesign)", () => {
  test("a grandchild's generating phase, streamed character count and failure count reach the root through two hops", () => {
    let now = 50_000;
    const z = rec("zzzzzzzz-z", { toolBatches: createBatchBuffer() });
    const registryX = registryOf(z);
    const xInM = rec("xxxxxxxx-x", { toolBatches: bufferOf("x", ["done"]), subtreeDescendants: [nested("zzzzzzzz-z")] });
    const registryM = registryOf(xInM);
    const mInRoot = rec("mmmmmmmm-m");
    const registryRoot = registryOf(mInRoot);
    const link2 = link();
    mInRoot.channel = link2.parent.parent as never;
    observeAgentView(mInRoot, link2.parent.parent as never, () => {});
    const upstreamM = new AgentViewUpstream(link2.child.channel, { intervalMs: 0, now: () => now });
    upstreamM.setSource(() => buildAgentViewModel(registryM));
    const link1 = link();
    xInM.channel = link1.parent.parent as never;
    observeAgentView(xInM, link1.parent.parent as never, () => { upstreamM.schedule(); });
    const upstreamX = new AgentViewUpstream(link1.child.channel, { intervalMs: 0, now: () => now });
    upstreamX.setSource(() => buildAgentViewModel(registryX));
    const atRoot = () => collectBatchViews(registryRoot).get("zzzzzzzz-z")!;

    // RPC-shaped stream: no `partial`, the tool name on toolcall_start, bare deltas.
    const zb = z.toolBatches!;
    observeBatchEvent(zb, { type: "message_start", message: { role: "assistant", content: [] } }, now);
    upstreamX.schedule();
    assert.deepEqual(atRoot().at(-1)!.generating, { kind: "thinking", phaseStartedAt: 50_000 });
    now = 53_000;
    observeBatchEvent(zb, { type: "message_update", assistantMessageEvent: { type: "toolcall_start", contentIndex: 0, id: "w1", toolName: "write" } }, now);
    for (const delta of ["{\"path\":", "\"a.ts\",", "\"content\":\"xyz\"}"]) observeBatchEvent(zb, { type: "message_update", assistantMessageEvent: { type: "toolcall_delta", contentIndex: 0, delta } }, now);
    upstreamX.schedule();
    assert.deepEqual(atRoot().at(-1)!.generating, { kind: "tool", phaseStartedAt: 53_000, toolName: "write", chars: 31 });

    // The message ends with the call; it executes and fails.
    observeBatchEvent(zb, { type: "message_end", message: { role: "assistant", content: [{ type: "toolCall", id: "w1", name: "write", arguments: { path: "a.ts" } }] } }, now);
    observeBatchEvent(zb, { type: "tool_execution_start", toolCallId: "w1", toolName: "write" }, now);
    observeBatchEvent(zb, { type: "tool_execution_end", toolCallId: "w1", isError: true }, now + 1);
    upstreamX.schedule();
    const last = atRoot().at(-1)!;
    assert.equal(last.generating, undefined);
    assert.equal(last.failed, 1);
    assert.equal(last.endedAt, now + 1);
    assert.deepEqual(atRoot(), batchViews(zb), "the root holds exactly the grandchild's own views");
  });
});

// ---------------------------------------------------------------------------
// Real socket
// ---------------------------------------------------------------------------

describe("real channel", () => {
  test("an agent-view message from a ChildChannel-backed upstream reaches observeAgentView on the ParentChannel side, and a close drops it", async () => {
    const parent = await ParentChannel.bind(1, { force: "tcp" });
    const child = await ChildChannel.connect(readAndDeleteChannelBootstrap({ ...parent.bootstrapEnv() })!, { reconnect: false });
    try {
      const record = rec("child", { channel: parent });
      let notify: () => void = () => {};
      const changed = () => new Promise<void>((resolve) => { notify = resolve; });
      let next = changed();
      observeAgentView(record, parent, () => { notify(); });
      const expected: AgentViewModel = new Map([["grand", batchViews(bufferOf("g", ["done", "running"]))]]);
      const upstream = new AgentViewUpstream(child, { intervalMs: 0 });
      upstream.setSource(() => expected);
      upstream.schedule();
      await next;
      assert.deepEqual(record.descendantViews, expected, "the model survives the real wire");

      next = changed();
      child.close();
      await next;
      assert.equal(record.descendantViews, undefined, "the parent drops the model at the disconnect");
    } finally {
      child.close();
      parent.close();
    }
  });
});

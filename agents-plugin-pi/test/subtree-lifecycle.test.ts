/**
 * Channel-delivered subtree state (260924-feat-pi-agent-channel-subtree-state):
 * the child's revisioned snapshots and busy fence (`SubtreeUpstream`,
 * `beginSubtreeDispatch`), the parent's revision-ordered view
 * (`observeSubtreeChannel`), and the acknowledgment latency over real sockets.
 */
import { after, describe, test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { performance } from "node:perf_hooks";
import { ChildChannel, ParentChannel, readAndDeleteChannelBootstrap } from "../src/agent-channel.ts";
import {
  beginSubtreeDispatch,
  installSubtreePublisher,
  observeSubtreeChannel,
  publishSubtree,
  SUBTREE_ACK_TIMEOUT_MS,
  SubtreeUpstream,
  type SubtreeSnapshot,
  type SubtreeView,
} from "../src/subtree-lifecycle.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "../src/spawner.ts";
import { fakeParentChannel, fakeUplink, quiescentSnapshot } from "./fixtures/subtree-channels.ts";

const roots: string[] = [];
const opened: Array<{ close(): void }> = [];
after(() => {
  for (const channel of opened.splice(0)) channel.close();
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true });
});
const drain = () => new Promise<void>(resolve => setTimeout(resolve, 0));

const snapshot = quiescentSnapshot;

function record(id: string, overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return { agentId: id, sessionPath: `/tmp/${id}.jsonl`, wsToolNames: [], toolGroup: "full-worker", streaming: false, running: false, reportLog: [], ...overrides };
}

async function channelPair(force: "pipe" | "tcp", generation = 1) {
  const socketDir = mkdtempSync(join(tmpdir(), "ws-st-"));
  roots.push(socketDir);
  const parent = await ParentChannel.bind(generation, { force, socketDir });
  let upstream: SubtreeUpstream | undefined;
  const child = await ChildChannel.connect(readAndDeleteChannelBootstrap({ ...parent.bootstrapEnv() })!, { reconnect: false, resume: () => upstream?.resume() ?? {} });
  upstream = new SubtreeUpstream(child);
  opened.push(child, parent);
  return { parent, child, upstream };
}

describe("child side: revisioned snapshots and the busy fence", () => {
  test("publication sends only effective changes, each with a rising revision", () => {
    const link = fakeUplink();
    const registry: RpcAgentRegistry = new Map();
    let deliveries = 0;
    installSubtreePublisher(registry, new SubtreeUpstream(link.channel), () => deliveries);
    assert.deepEqual(link.snapshots().map(s => s.revision), [1], "installation publishes the initial snapshot");

    publishSubtree(registry);
    publishSubtree(registry);
    assert.equal(link.snapshots().length, 1, "an unchanged snapshot is not sent again");

    const child = record("child");
    registry.set(child.agentId, child);
    publishSubtree(registry);
    child.running = true;
    publishSubtree(registry);
    child.waitingOnChildren = true;
    publishSubtree(registry);
    deliveries = 1;
    publishSubtree(registry);
    child.subtreeDescendants = [{ id: "nested", parentId: null, depth: 0, role: "explore", live: true }];
    publishSubtree(registry);
    publishSubtree(registry, true);
    const sent = link.snapshots();
    assert.deepEqual(sent.map(s => s.revision), [1, 2, 3, 4, 5, 6], "every effective transition is one send; a repeat dispatch mark with no change is none");
    assert.deepEqual(sent.at(-1), {
      outstanding: 1, active: 1, deliveries: 1, delegated: true, revision: 6,
      descendants: [
        { id: "child", parentId: null, depth: 0, role: "worker", live: false },
        { id: "nested", parentId: "child", depth: 1, role: "explore", live: true },
      ],
    });
  });

  test("a publisher with no parent channel has no fence and admits synchronously", () => {
    const registry: RpcAgentRegistry = new Map();
    installSubtreePublisher(registry, undefined, () => 0);
    const admission = beginSubtreeDispatch(registry);
    assert.equal(typeof admission, "function");
    assert.equal(publishSubtree(registry)?.active, 1);
    (admission as () => void)();
    assert.equal(publishSubtree(registry)?.active, 0);
  });

  test("no dispatch is admitted before the parent acknowledges the busy revision", async () => {
    const link = fakeUplink();
    const registry: RpcAgentRegistry = new Map();
    installSubtreePublisher(registry, new SubtreeUpstream(link.channel), () => 0);
    link.ack(1);
    let admitted: (() => void) | undefined;
    const admission = beginSubtreeDispatch(registry);
    assert.ok(admission instanceof Promise, "an unacknowledged busy revision is owed");
    void admission.then(finish => { admitted = finish; });
    const busy = link.snapshots().at(-1)!;
    assert.deepEqual({ revision: busy.revision, active: busy.active }, { revision: 2, active: 1 }, "the busy edge is published before the wait");

    await drain();
    assert.equal(admitted, undefined, "no acknowledgment yet");
    link.ack(1);
    await drain();
    assert.equal(admitted, undefined, "an acknowledgment of an earlier revision does not open the fence");
    link.ack(2);
    await drain();
    assert.ok(admitted, "the busy revision's acknowledgment admits the dispatch");
    admitted();
    assert.deepEqual({ revision: link.snapshots().at(-1)!.revision, active: link.snapshots().at(-1)!.active }, { revision: 3, active: 0 });
  });

  test("a dropped acknowledgment, a disconnect during the wait, and a down channel each refuse the dispatch and undo its admission", async () => {
    const dropped = fakeUplink();
    const droppedRegistry: RpcAgentRegistry = new Map();
    installSubtreePublisher(droppedRegistry, new SubtreeUpstream(dropped.channel, { ackTimeoutMs: 30 }), () => 0);
    await assert.rejects(Promise.resolve(beginSubtreeDispatch(droppedRegistry)), /nested dispatch refused: the parent did not acknowledge revision 2 within 30ms/);
    assert.equal(dropped.snapshots().at(-1)!.active, 0, "the refused admission is republished as idle");

    const cut = fakeUplink();
    const cutRegistry: RpcAgentRegistry = new Map();
    installSubtreePublisher(cutRegistry, new SubtreeUpstream(cut.channel), () => 0);
    const waiting = Promise.resolve(beginSubtreeDispatch(cutRegistry));
    cut.disconnect();
    await assert.rejects(waiting, /nested dispatch refused: the channel to the parent disconnected/);
    assert.equal(publishSubtree(cutRegistry)?.active, 0);

    const down = fakeUplink();
    down.connected = false;
    const downRegistry: RpcAgentRegistry = new Map();
    assert.doesNotThrow(() => installSubtreePublisher(downRegistry, new SubtreeUpstream(down.channel), () => 0), "publication while disconnected never throws");
    await assert.rejects(Promise.resolve(beginSubtreeDispatch(downRegistry)), /nested dispatch refused: the channel to the parent is down/);
    assert.equal(publishSubtree(downRegistry)?.active, 0);
    assert.equal(down.sent.length, 0);
  });

  test("the latest snapshot rides the reconnect hello's resume section", () => {
    const link = fakeUplink();
    const upstream = new SubtreeUpstream(link.channel);
    assert.deepEqual(upstream.resume(), {}, "nothing to resume before the first publication");
    const registry: RpcAgentRegistry = new Map();
    installSubtreePublisher(registry, upstream, () => 0);
    link.disconnect();
    registry.set("child", record("child", { running: true }));
    publishSubtree(registry);
    assert.equal(link.snapshots().length, 1, "the disconnected change was not sent");
    assert.deepEqual(upstream.resume(), { subtree: publishSubtree(registry) });
    assert.equal((upstream.resume().subtree as SubtreeSnapshot).revision, 2);
  });
});

describe("parent side: the revision-ordered view", () => {
  test("stale and duplicate snapshots cannot regress the view; every received snapshot is acknowledged with the highest applied revision", () => {
    const channel = fakeParentChannel();
    const views: SubtreeView[] = [];
    observeSubtreeChannel(channel.parent, view => views.push(view));
    assert.deepEqual(views, [{ waiting: true }], "nothing received yet reads as waiting");

    channel.deliver(snapshot(3, { active: 1 }));
    channel.deliver(snapshot(2));
    channel.deliver(snapshot(3, { active: 1 }));
    channel.deliver({ revision: 9, outstanding: -1 });
    assert.deepEqual(views.map(v => v.waiting), [true, true, true], "the lower quiescent revision and the duplicate never apply a quiescent view");
    assert.deepEqual(channel.acks, [3, 3, 3], "a malformed snapshot is neither applied nor acknowledged");

    channel.deliver(snapshot(4));
    assert.equal(views.at(-1)?.waiting, false);
    assert.equal(views.at(-1)?.snapshot?.revision, 4);
    assert.deepEqual(channel.acks, [3, 3, 3, 4]);
  });

  test("a disconnect reads as waiting until a reconnect hello restores the latest snapshot", () => {
    const channel = fakeParentChannel();
    const views: SubtreeView[] = [];
    observeSubtreeChannel(channel.parent, view => views.push(view));
    const identity = [{ id: "grandchild", parentId: null, depth: 0, role: "worker" as const, live: true }];
    channel.deliver(snapshot(2, { descendants: identity }));
    assert.equal(views.at(-1)?.waiting, false);

    channel.drop();
    assert.equal(views.at(-1)?.waiting, true, "a disconnected channel is never permission to settle");
    assert.deepEqual(views.at(-1)?.snapshot?.descendants, identity, "advisory identity survives the disconnect");
    channel.hello({});
    assert.equal(views.at(-1)?.waiting, true, "a reconnect without a snapshot keeps waiting");
    channel.hello({ subtree: snapshot(2, { descendants: identity }) });
    assert.equal(views.at(-1)?.waiting, false, "the resumed snapshot at the last-seen revision restores the view");
    assert.deepEqual(channel.acks, [2, 2]);
  });
});

for (const kind of ["pipe", "tcp"] as const) {
  describe(`busy fence over the real channel [${kind}]`, () => {
    test("the parent has applied the busy snapshot by the time the fence opens; the acknowledgment round trip is recorded", async t => {
      const { parent, upstream } = await channelPair(kind);
      let view: SubtreeView | undefined;
      observeSubtreeChannel(parent, next => { view = next; });
      const registry: RpcAgentRegistry = new Map();
      installSubtreePublisher(registry, upstream, () => 0);

      const samples: number[] = [];
      for (let i = 0; i < 200; i++) {
        const started = performance.now();
        const admission = beginSubtreeDispatch(registry);
        assert.ok(admission instanceof Promise, "every busy edge owes an acknowledgment");
        const finish = await admission;
        samples.push(performance.now() - started);
        assert.equal(view?.waiting, true, "the parent's view is busy before any grandchild could start");
        assert.equal(view?.snapshot?.active, 1);
        finish();
      }
      samples.sort((a, b) => a - b);
      const at = (q: number) => samples[Math.min(samples.length - 1, Math.floor(q * samples.length))];
      const summary = `${kind} ack round trip over ${samples.length} fences: p50 ${at(0.5).toFixed(3)}ms, p95 ${at(0.95).toFixed(3)}ms, max ${samples.at(-1)!.toFixed(3)}ms`;
      t.diagnostic(summary);
      assert.ok(samples.at(-1)! < SUBTREE_ACK_TIMEOUT_MS, summary);
    });
  });
}

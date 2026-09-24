/**
 * Unit tests for approval-protocol.ts (260924-feat-pi-agent-channel-approval-
 * decisions): the message shapes both sides agree on and the child-side wait
 * (`ChildApprovalGate`) over a fake link, so the acknowledgment-send failure
 * that decides "not consumed" and the early-decision keep are deterministic here. The same gate over the
 * real channel, on both backends, is agent-channel.test.ts's contract case.
 */
import { describe, test } from "node:test";
import assert from "node:assert/strict";
import {
  APPROVAL_CONSUMED_MESSAGE,
  APPROVAL_DECISION_MESSAGE,
  ChildApprovalGate,
  approvalConsumedMessage,
  approvalDecisionMessage,
  consumedApprovalsFromResume,
  parseApprovalConsumedMessage,
  parseApprovalDecisionMessage,
  pendingApprovalFromResume,
  type ApprovalChildLink,
} from "../src/approval-protocol.ts";

/** A link whose deliveries, connection ends, and send failures the test controls. */
function fakeLink(): ApprovalChildLink & {
  deliver(msg: Record<string, unknown>): void;
  onDisconnect(cb: () => void): () => void;
  disconnect(): void;
  sent: Record<string, unknown>[];
  failSend: boolean;
  listeners: number;
} {
  const listeners = new Set<(msg: Record<string, unknown>) => void>();
  const disconnects = new Set<() => void>();
  const link = {
    sent: [] as Record<string, unknown>[],
    failSend: false,
    get listeners() { return listeners.size; },
    send(msg: Record<string, unknown>) {
      if (link.failSend) throw new Error("ws-pi-channel: not connected to the parent");
      link.sent.push(msg);
    },
    onMessage(cb: (msg: Record<string, unknown>) => void) {
      listeners.add(cb);
      return () => { listeners.delete(cb); };
    },
    deliver(msg: Record<string, unknown>) { for (const cb of [...listeners]) cb(msg); },
    onDisconnect(cb: () => void) {
      disconnects.add(cb);
      return () => { disconnects.delete(cb); };
    },
    disconnect() { for (const cb of [...disconnects]) cb(); },
  };
  return link;
}

const settled = <T>(promise: Promise<T>): Promise<T | "pending"> => Promise.race([promise, new Promise<"pending">(resolve => setTimeout(() => resolve("pending"), 20))]);

describe("approval message shapes", () => {
  test("a decision message round-trips through the parser, omitting absent reason/command", () => {
    const msg = approvalDecisionMessage("call|1", { decision: "approve" });
    assert.deepEqual(msg, { t: APPROVAL_DECISION_MESSAGE, cmd_id: "call|1", decision: "approve" });
    assert.deepEqual(parseApprovalDecisionMessage({ ...msg, gen: 3 }), { cmdId: "call|1", decision: { decision: "approve" } });
    const deny = approvalDecisionMessage("c", { decision: "deny", reason: "no" });
    assert.deepEqual(parseApprovalDecisionMessage(deny), { cmdId: "c", decision: { decision: "deny", reason: "no" } });
    const instead = approvalDecisionMessage("c", { decision: "run-instead", command: "echo x" });
    assert.deepEqual(parseApprovalDecisionMessage(instead), { cmdId: "c", decision: { decision: "run-instead", command: "echo x" } });
  });

  test("the logical cmd_id is carried verbatim: no filesystem encoding remains", () => {
    const unsafe = "call<>:\"/\\|?*%\u0000\u001f\u007f\u009f";
    assert.equal(parseApprovalDecisionMessage(approvalDecisionMessage(unsafe, { decision: "approve" }))?.cmdId, unsafe);
    assert.equal(parseApprovalConsumedMessage(approvalConsumedMessage(unsafe)), unsafe);
  });

  test("other, malformed, or unknown-decision messages parse to undefined", () => {
    for (const msg of [
      { t: "ready", cmd_id: "c", decision: "approve" },
      { t: APPROVAL_DECISION_MESSAGE, decision: "approve" },
      { t: APPROVAL_DECISION_MESSAGE, cmd_id: "", decision: "approve" },
      { t: APPROVAL_DECISION_MESSAGE, cmd_id: "c", decision: "allow" },
      { t: APPROVAL_DECISION_MESSAGE, cmd_id: "c" },
      { t: APPROVAL_CONSUMED_MESSAGE, cmd_id: "c" },
    ]) assert.equal(parseApprovalDecisionMessage(msg), undefined, JSON.stringify(msg));
    for (const msg of [{ t: APPROVAL_DECISION_MESSAGE, cmd_id: "c" }, { t: APPROVAL_CONSUMED_MESSAGE }, { t: APPROVAL_CONSUMED_MESSAGE, cmd_id: 1 }]) {
      assert.equal(parseApprovalConsumedMessage(msg), undefined, JSON.stringify(msg));
    }
    assert.equal(parseApprovalConsumedMessage({ t: APPROVAL_CONSUMED_MESSAGE, cmd_id: "c", gen: 1 }), "c");
  });

  test("the resume section reports a pending cmd_id or nothing", () => {
    assert.equal(pendingApprovalFromResume({ approval: { pending: "call-1" } }), "call-1");
    assert.equal(pendingApprovalFromResume({ approval: { pending: "" } }), undefined);
    assert.equal(pendingApprovalFromResume({ approval: "call-1" }), undefined);
    assert.equal(pendingApprovalFromResume({ approval: ["call-1"] }), undefined);
    assert.equal(pendingApprovalFromResume({ readiness: {} }), undefined);
    assert.equal(pendingApprovalFromResume(undefined), undefined);
  });

  test("the resume section's consumed list is read as positive evidence; a hello without the array reads as undefined, an empty array as none consumed", () => {
    assert.deepEqual(consumedApprovalsFromResume({ approval: { consumed: ["call-1", "call-2"] } }), ["call-1", "call-2"]);
    assert.deepEqual(consumedApprovalsFromResume({ approval: { pending: "call-3", consumed: [] } }), [], "an empty array is not absence");
    assert.deepEqual(consumedApprovalsFromResume({ approval: { consumed: ["call-1", 7, "", null] } }), ["call-1"], "malformed entries are dropped");
    assert.equal(consumedApprovalsFromResume({ approval: { pending: "call-3" } }), undefined, "an older child's section");
    assert.equal(consumedApprovalsFromResume({ approval: { consumed: "call-1" } }), undefined);
    assert.equal(consumedApprovalsFromResume({}), undefined);
    assert.equal(consumedApprovalsFromResume(undefined), undefined);
  });
});

describe("ChildApprovalGate", () => {
  test("approve, deny, and run-instead are consumed after the acknowledgment is sent, and the wait then reports nothing pending", async () => {
    const gate = new ChildApprovalGate();
    const link = fakeLink();
    for (const decision of [{ decision: "approve" as const }, { decision: "deny" as const, reason: "no" }, { decision: "run-instead" as const, command: "echo y" }]) {
      const wait = gate.waitForDecision(link, "call-1", undefined);
      assert.equal(gate.pending, "call-1");
      assert.deepEqual((gate.resume().approval as { pending?: string }).pending, "call-1");
      link.deliver({ ...approvalDecisionMessage("call-1", decision), gen: 1 });
      assert.deepEqual(await wait, decision);
      assert.deepEqual(link.sent.at(-1), approvalConsumedMessage("call-1"), "the acknowledgment names the consumed cmd_id");
      assert.equal(gate.pending, undefined);
      assert.equal((gate.resume().approval as { pending?: string }).pending, undefined);
    }
    assert.equal(link.listeners, 0, "every settled wait detached its listener");
  });

  test("the acknowledgment is sent before the wait resolves: a send that throws means the decision is not consumed and the cmd_id stays pending", async () => {
    const gate = new ChildApprovalGate();
    const link = fakeLink();
    const wait = gate.waitForDecision(link, "call-2", undefined);
    link.failSend = true;
    link.deliver(approvalDecisionMessage("call-2", { decision: "approve" }));
    assert.equal(await settled(wait), "pending", "an unacknowledged decision never starts the command");
    assert.equal(gate.pending, "call-2");
    assert.deepEqual(gate.resume(), { approval: { pending: "call-2", consumed: [] } }, "the next hello reports the cmd_id as still waiting and not consumed");
    // Only a decision that can be acknowledged (over the new connection) is consumed.
    link.failSend = false;
    link.deliver(approvalDecisionMessage("call-2", { decision: "deny", reason: "later" }));
    assert.deepEqual(await wait, { decision: "deny", reason: "later" });
    assert.deepEqual(link.sent, [approvalConsumedMessage("call-2")]);
  });

  test("a decision for a different cmd_id never satisfies the wait and is not acknowledged", async () => {
    const gate = new ChildApprovalGate();
    const link = fakeLink();
    const wait = gate.waitForDecision(link, "call-3", undefined);
    link.deliver(approvalDecisionMessage("call-OTHER", { decision: "approve" }));
    link.deliver({ t: "approval-decision", cmd_id: "call-3" });
    link.deliver({ t: "ready", kind: "web", payload: {} });
    assert.equal(await settled(wait), "pending");
    assert.deepEqual(link.sent, [], "nothing was consumed, so nothing was acknowledged");
    link.deliver(approvalDecisionMessage("call-3", { decision: "approve" }));
    assert.deepEqual(await wait, { decision: "approve" });
  });

  test("abort resolves \"aborted\" immediately or mid-wait, clears the pending cmd_id, and a later decision is ignored", async () => {
    const gate = new ChildApprovalGate();
    const link = fakeLink();
    const pre = new AbortController();
    pre.abort();
    assert.equal(await gate.waitForDecision(link, "call-4", pre.signal), "aborted");
    assert.equal(gate.pending, undefined);
    assert.equal(link.listeners, 0, "a pre-aborted wait never subscribes");

    const mid = new AbortController();
    const wait = gate.waitForDecision(link, "call-5", mid.signal);
    assert.equal(gate.pending, "call-5");
    mid.abort();
    assert.equal(await wait, "aborted");
    assert.equal(gate.pending, undefined);
    link.deliver(approvalDecisionMessage("call-5", { decision: "approve" }));
    assert.deepEqual(link.sent, [], "a decision after the abort is neither consumed nor acknowledged");
    assert.equal(link.listeners, 0);
  });

  test("attach keeps a decision that arrives before its wait, bounded per cmd_id, and the wait consumes it at once after acknowledging", async () => {
    const gate = new ChildApprovalGate();
    const link = fakeLink();
    const detach = gate.attach(link);
    link.deliver(approvalDecisionMessage("call-8", { decision: "approve" }));
    link.deliver(approvalDecisionMessage("call-8", { decision: "deny", reason: "changed" }));
    link.deliver({ t: "ready", kind: "web", payload: {} });
    assert.deepEqual(link.sent, [], "keeping a decision is not consuming it: no acknowledgment yet");
    assert.equal(gate.pending, undefined);
    assert.deepEqual(await gate.waitForDecision(link, "call-8", undefined), { decision: "deny", reason: "changed" }, "the latest early decision wins");
    assert.deepEqual(link.sent, [approvalConsumedMessage("call-8")]);
    assert.deepEqual(gate.resume(), { approval: { consumed: ["call-8"] } });
    // Consumed once: a second wait on the same cmd_id does not see it again.
    assert.equal(await settled(gate.waitForDecision(link, "call-8", undefined)), "pending");

    // A decision for a cmd_id with an open wait is left to that wait, never kept twice.
    const open = gate.waitForDecision(link, "call-9", undefined);
    link.deliver(approvalDecisionMessage("call-9", { decision: "approve" }));
    assert.deepEqual(await open, { decision: "approve" });
    assert.equal(await settled(gate.waitForDecision(link, "call-9", undefined)), "pending", "not buffered as well as consumed");

    // The bound: the oldest early decision is dropped first.
    for (let i = 0; i <= ChildApprovalGate.EARLY_DECISION_CAP; i++) link.deliver(approvalDecisionMessage(`bulk-${i}`, { decision: "approve" }));
    assert.equal(await settled(gate.waitForDecision(link, "bulk-0", undefined)), "pending", "evicted");
    assert.deepEqual(await gate.waitForDecision(link, `bulk-${ChildApprovalGate.EARLY_DECISION_CAP}`, undefined), { decision: "approve" });
    detach();
    link.deliver(approvalDecisionMessage("call-10", { decision: "approve" }));
    assert.equal(await settled(gate.waitForDecision(link, "call-10", undefined)), "pending", "a detached gate keeps nothing");
  });

  test("an early decision whose acknowledgment fails is dropped, the cmd_id stays pending, and an abort forgets it", async () => {
    const gate = new ChildApprovalGate();
    const link = fakeLink();
    gate.attach(link);
    link.deliver(approvalDecisionMessage("call-11", { decision: "approve" }));
    link.failSend = true;
    const wait = gate.waitForDecision(link, "call-11", undefined);
    assert.equal(await settled(wait), "pending");
    assert.equal(gate.pending, "call-11", "reported by the next hello");
    link.failSend = false;
    link.deliver(approvalDecisionMessage("call-11", { decision: "deny", reason: "fresh" }));
    assert.deepEqual(await wait, { decision: "deny", reason: "fresh" });

    link.deliver(approvalDecisionMessage("call-12", { decision: "approve" }));
    const aborted = new AbortController();
    aborted.abort();
    assert.equal(await gate.waitForDecision(link, "call-12", aborted.signal), "aborted");
    assert.equal(await settled(gate.waitForDecision(link, "call-12", undefined)), "pending", "the kept decision went with the abort");
    assert.deepEqual(link.sent, [approvalConsumedMessage("call-11")]);
  });

  test("two open waits keep their own cmd_id; the resume section reports the latest", async () => {
    const gate = new ChildApprovalGate();
    const link = fakeLink();
    const first = gate.waitForDecision(link, "call-6", undefined);
    const second = gate.waitForDecision(link, "call-7", undefined);
    assert.deepEqual(gate.resume(), { approval: { pending: "call-7", consumed: [] } });
    link.deliver(approvalDecisionMessage("call-7", { decision: "deny", reason: "x" }));
    assert.deepEqual(await second, { decision: "deny", reason: "x" });
    assert.equal(await settled(first), "pending");
    assert.deepEqual(gate.resume(), { approval: { pending: "call-6", consumed: ["call-7"] } });
    link.deliver(approvalDecisionMessage("call-6", { decision: "approve" }));
    assert.deepEqual(await first, { decision: "approve" });
    assert.deepEqual(gate.resume(), { approval: { consumed: ["call-7", "call-6"] } });
  });

  test("a new gate always reports the approval section with a consumed array, even when nothing was consumed", () => {
    assert.deepEqual(new ChildApprovalGate().resume(), { approval: { consumed: [] } });
  });

  test("the consumed list records each acknowledged cmd_id, bounded like the early keep with the oldest dropped first", async () => {
    const gate = new ChildApprovalGate();
    const link = fakeLink();
    for (let i = 0; i <= ChildApprovalGate.EARLY_DECISION_CAP; i++) {
      const wait = gate.waitForDecision(link, `c-${i}`, undefined);
      link.deliver(approvalDecisionMessage(`c-${i}`, { decision: "approve" }));
      await wait;
    }
    const consumed = (gate.resume().approval as { consumed: string[] }).consumed;
    assert.equal(consumed.length, ChildApprovalGate.EARLY_DECISION_CAP);
    assert.equal(consumed[0], "c-1", "c-0 was the oldest and was dropped");
    assert.equal(consumed.at(-1), `c-${ChildApprovalGate.EARLY_DECISION_CAP}`);
  });

  test("a failed acknowledgment send leaves the cmd_id pending and out of the consumed list", async () => {
    const gate = new ChildApprovalGate();
    const link = fakeLink();
    const wait = gate.waitForDecision(link, "call-13", undefined);
    link.failSend = true;
    link.deliver(approvalDecisionMessage("call-13", { decision: "approve" }));
    assert.equal(await settled(wait), "pending");
    assert.deepEqual(gate.resume(), { approval: { pending: "call-13", consumed: [] } });
    link.failSend = false;
    link.deliver(approvalDecisionMessage("call-13", { decision: "approve" }));
    await wait;
    assert.deepEqual(gate.resume(), { approval: { consumed: ["call-13"] } }, "consumed only once the acknowledgment went out");
  });

  test("an early decision is bound to the connection it arrived on: its end discards it, and only a decision over the new connection is consumed", async () => {
    const gate = new ChildApprovalGate();
    const link = fakeLink();
    gate.attach(link);
    link.deliver(approvalDecisionMessage("call-14", { decision: "approve" }));
    link.disconnect();
    assert.deepEqual(gate.resume(), { approval: { consumed: [] } }, "the reconnect hello reports it neither pending nor consumed, so the parent re-asks");
    const wait = gate.waitForDecision(link, "call-14", undefined);
    assert.equal(await settled(wait), "pending", "the old connection's decision is never consumed");
    assert.deepEqual(link.sent, [], "nothing acknowledged");
    link.deliver(approvalDecisionMessage("call-14", { decision: "deny", reason: "fresh" }));
    assert.deepEqual(await wait, { decision: "deny", reason: "fresh" }, "the decision sent over the new connection is the one consumed");
    assert.deepEqual(link.sent, [approvalConsumedMessage("call-14")]);
  });
});

/**
 * Unit tests for approval-protocol.ts (260924-feat-pi-agent-channel-approval-
 * decisions): the message shapes both sides agree on and the child-side wait
 * (`ChildApprovalGate`) over a fake link, so the acknowledgment-send failure
 * that decides "not consumed" is deterministic here. The same gate over the
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
  parseApprovalConsumedMessage,
  parseApprovalDecisionMessage,
  pendingApprovalFromResume,
  type ApprovalChildLink,
} from "../src/approval-protocol.ts";

/** A link whose deliveries and send failures the test controls. */
function fakeLink(): ApprovalChildLink & { deliver(msg: Record<string, unknown>): void; sent: Record<string, unknown>[]; failSend: boolean; listeners: number } {
  const listeners = new Set<(msg: Record<string, unknown>) => void>();
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
});

describe("ChildApprovalGate", () => {
  test("approve, deny, and run-instead are consumed after the acknowledgment is sent, and the wait then reports nothing pending", async () => {
    const gate = new ChildApprovalGate();
    const link = fakeLink();
    for (const decision of [{ decision: "approve" as const }, { decision: "deny" as const, reason: "no" }, { decision: "run-instead" as const, command: "echo y" }]) {
      const wait = gate.waitForDecision(link, "call-1", undefined);
      assert.equal(gate.pending, "call-1");
      assert.deepEqual(gate.resume(), { approval: { pending: "call-1" } });
      link.deliver({ ...approvalDecisionMessage("call-1", decision), gen: 1 });
      assert.deepEqual(await wait, decision);
      assert.deepEqual(link.sent.at(-1), approvalConsumedMessage("call-1"), "the acknowledgment names the consumed cmd_id");
      assert.equal(gate.pending, undefined);
      assert.deepEqual(gate.resume(), {});
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
    assert.deepEqual(gate.resume(), { approval: { pending: "call-2" } }, "the next hello reports the cmd_id as still waiting");
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

  test("two open waits keep their own cmd_id; the resume section reports the latest", async () => {
    const gate = new ChildApprovalGate();
    const link = fakeLink();
    const first = gate.waitForDecision(link, "call-6", undefined);
    const second = gate.waitForDecision(link, "call-7", undefined);
    assert.deepEqual(gate.resume(), { approval: { pending: "call-7" } });
    link.deliver(approvalDecisionMessage("call-7", { decision: "deny", reason: "x" }));
    assert.deepEqual(await second, { decision: "deny", reason: "x" });
    assert.equal(await settled(first), "pending");
    assert.deepEqual(gate.resume(), { approval: { pending: "call-6" } });
    link.deliver(approvalDecisionMessage("call-6", { decision: "approve" }));
    assert.deepEqual(await first, { decision: "approve" });
    assert.deepEqual(gate.resume(), {});
  });
});

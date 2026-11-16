import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  applyRpcEvent,
  attachEventListener,
  agentWidgetRefreshRef,
  observeChildSubtree,
  REPORT_TO_LEAD_TOOL_NAME,
  GATED_EXEC_TOOL_NAME,
  attachApprovalChannel,
  syncOwnershipProtection,
  lastActivityAt,
  type RpcAgentRecord,
  WORKER_LIFECYCLE_GUIDE,
} from "../src/spawner.ts";
// 260906 (registerAgentTools's role-keyed explore registration): a VALUE
// import, not `import type` — the new describe block below monkey-patches
// RpcClient.prototype.{start,onEvent,prompt} for the lead/fork execute() path
// so the real spawnAgent code runs to completion with no real subprocess
// (see that describe block's own doc comment for why this is safe and the
// worker/execute-worker leaf path deliberately never invokes execute() at all).
import { RpcClient } from "@earendil-works/pi-coding-agent";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { allocateAgentHome, createAgentStorageContext, readOwnership } from "../src/agent-storage.ts";
import { approvalConsumedMessage } from "../src/approval-protocol.ts";
import { type ChannelConnection, type ChannelHello } from "../src/agent-channel.ts";
import { installSubtreePublisher, SubtreeUpstream } from "../src/subtree-lifecycle.ts";
import { agentViewUpstreamRef } from "../src/agent-view.ts";
import { fakeParentChannel, fakeUplink, idleOwnTurn } from "./fixtures/subtree-channels.ts";
import { freshRpcRecord, liveRpcRecord } from "./fixtures/spawner-records.ts";

describe("applyRpcEvent", () => {
  test("agent_start flips streaming true and returns an empty outcome (no push)", () => {
    const record = freshRpcRecord({ running: true });
    assert.deepEqual(applyRpcEvent(record, { type: "agent_start" }), {});
    assert.equal(record.streaming, true);
    assert.equal(record.running, true, "agent_start must not disturb the fan-in latch set at prompt time");
  });

  test("agent_settled flips streaming false, clears running, stamps the freeze clock, and reports settled so the caller can decide on the push", (t) => {
    t.mock.method(Date, "now", () => 1_000);
    const record = freshRpcRecord({ streaming: true, running: true });
    assert.deepEqual(applyRpcEvent(record, { type: "agent_settled" }), { settled: true });
    assert.equal(record.streaming, false);
    assert.equal(record.running, false, "the run is over — the child stops counting toward the fan-in whatever the caller pushes");
    assert.equal(record.settledAt, 1_000);
    t.mock.method(Date, "now", () => 2_000);
    applyRpcEvent(record, { type: "agent_settled" });
    assert.equal(record.settledAt, 1_000, "a duplicate settle cannot extend the frozen duration");
  });

  test("only agent output advances the shared activity high-water mark", (t) => {
    t.mock.method(Date, "now", () => 1_000);
    const record = freshRpcRecord({
      lastLeadPromptAt: 900,
      ownerSends: [{ text: "input", at: 950 }],
      reportLog: [{ at: 990 }],
    });
    assert.equal(lastActivityAt(record), 0, "lead and owner inputs plus old report bookkeeping are not output");

    applyRpcEvent(record, { type: "tool_execution_start", toolName: "bash" });
    assert.equal(lastActivityAt(record), 1_000);
    t.mock.method(Date, "now", () => 1_100);
    applyRpcEvent(record, { type: "tool_execution_end", toolName: "bash" });
    assert.equal(lastActivityAt(record), 1_100);
  });

  test("other event types (e.g. message_update) are ignored — no streaming/running mutation, no push", () => {
    const record = freshRpcRecord({ streaming: true, running: true });
    assert.deepEqual(applyRpcEvent(record, { type: "message_update" }), {});
    assert.equal(record.streaming, true);
    assert.equal(record.running, true);
  });

  test("streamed assistant text and thinking bump activity without per-token gutter refresh", (t) => {
    let now = 1_000;
    t.mock.method(Date, "now", () => now);
    let listener: ((event: unknown) => void) | undefined;
    const client = {
      onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
      getState: async () => ({}),
    } as unknown as RpcClient;
    const record = freshRpcRecord({
      client,
      running: true,
      streaming: true,
    });
    const registry = new Map([[record.agentId, record]]);
    const uplink = fakeUplink();
    installSubtreePublisher(registry, new SubtreeUpstream(uplink.channel), () => 0, idleOwnTurn);
    const sendsBefore = uplink.sent.length;
    let refreshes = 0;
    agentWidgetRefreshRef.current = () => { refreshes += 1; };
    t.after(() => { agentWidgetRefreshRef.current = undefined; });
    attachEventListener(undefined, registry, record, client);

    listener?.({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "first" } });
    assert.equal(record.lastOutputAt, 1_000);
    now = 1_100;
    listener?.({ type: "message_update", assistantMessageEvent: { type: "thinking_delta", delta: "considering" } });
    assert.equal(record.lastOutputAt, 1_100);
    assert.equal(refreshes, 0, "streaming writes the high-water field only; the periodic gutter clock renders it");
    assert.equal(uplink.sent.length, sendsBefore, "token deltas send no subtree snapshot upstream");
  });
});

describe("attachEventListener: tool-call argument streaming (261028 batch-row redesign)", () => {
  test("N toolcall_delta events cost no subtree publish and no refresh, yet the character count reaches the widget's buffer and the relay, and the activity clock resets", (t) => {
    let now = 1_000;
    t.mock.method(Date, "now", () => now);
    let listener: ((event: unknown) => void) | undefined;
    let getStateCalls = 0;
    const client = {
      onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
      getState: async () => { getStateCalls += 1; return {}; },
    } as unknown as RpcClient;
    const record = freshRpcRecord({ client, running: true, streaming: true });
    const registry = new Map([[record.agentId, record]]);
    let publishes = 0;
    installSubtreePublisher(registry, new SubtreeUpstream(fakeUplink().channel), () => 0, () => { publishes += 1; return idleOwnTurn(); });
    let refreshes = 0;
    agentWidgetRefreshRef.current = () => { refreshes += 1; };
    let relaySchedules = 0;
    agentViewUpstreamRef.current = { schedule() { relaySchedules += 1; } } as never;
    t.after(() => { agentWidgetRefreshRef.current = undefined; agentViewUpstreamRef.current = undefined; });
    attachEventListener(undefined, registry, record, client);

    // RPC shape: message_start keeps its message; updates carry no message and no partial.
    listener?.({ type: "message_start", message: { role: "assistant", content: [] } });
    listener?.({ type: "message_update", assistantMessageEvent: { type: "toolcall_start", contentIndex: 0, id: "w1", toolName: "write" } });
    assert.equal(record.toolBatches?.generating?.toolName, "write");
    const before = { publishes, refreshes, getStateCalls, relaySchedules };
    const N = 500;
    for (let i = 0; i < N; i++) {
      now = 2_000 + i;
      listener?.({ type: "message_update", assistantMessageEvent: { type: "toolcall_delta", contentIndex: 0, delta: "abcd" } });
    }
    assert.equal(publishes, before.publishes, "no publishSubtree per delta");
    assert.equal(getStateCalls, before.getStateCalls, "no telemetry refresh per delta");
    assert.equal(refreshes, before.refreshes, "no widget refresh per delta; the 1 s expanded tick repaints");
    assert.equal(relaySchedules - before.relaySchedules, N, "each delta only marks the throttled agent-view relay dirty");
    assert.equal(record.toolBatches?.generating?.chars, 4 * N, "the count reaches the buffer the widget and relay read");
    assert.equal(record.lastOutputAt, 2_000 + N - 1, "argument streaming resets the activity clock");
  });
});

describe("attachEventListener: output-rate wiring", () => {
  test("each attach gets a fresh tracker, toolcall blocks count, and the widget refreshes only when the rate changes", (t) => {
    let now = 0;
    t.mock.method(Date, "now", () => now);
    let listener: ((event: unknown) => void) | undefined;
    const client = {
      onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
      getState: async () => ({}),
    } as unknown as RpcClient;
    const record = freshRpcRecord({ client, running: true, streaming: true });
    const registry = new Map([[record.agentId, record]]);
    let refreshes = 0;
    agentWidgetRefreshRef.current = () => { refreshes += 1; };
    t.after(() => { agentWidgetRefreshRef.current = undefined; });
    const toolCallMessage = (output: number) => {
      const message = { role: "assistant", provider: "p", model: "m", stopReason: "toolUse", usage: { output } };
      now = 0; listener?.({ type: "message_start", message });
      listener?.({ type: "message_update", message, assistantMessageEvent: { type: "toolcall_start", contentIndex: 0 } });
      now = 1_000; listener?.({ type: "message_update", message, assistantMessageEvent: { type: "toolcall_end", contentIndex: 0 } });
      const before = refreshes;
      listener?.({ type: "message_end", message });
      return refreshes - before; // synchronous only: the telemetry refresh awaits getState
    };

    attachEventListener(undefined, registry, record, client);
    const first = record.outputRate;
    assert.equal(first?.rate(), undefined);
    assert.equal(toolCallMessage(100), 1, "a toolcall-only message yields a rate and one widget refresh");
    assert.equal(record.outputRate?.rate(), 100);
    assert.equal(toolCallMessage(100), 0, "an unchanged rate does not refresh");
    assert.equal(toolCallMessage(1), 0, "an ineligible message leaves the rate unchanged and does not refresh");
    assert.equal(toolCallMessage(300), 1, "a changed rate refreshes again");

    attachEventListener(undefined, registry, record, client);
    assert.notEqual(record.outputRate, first, "a new attach replaces the tracker");
    assert.equal(record.outputRate?.rate(), undefined, "the new tracker carries no prior launch's window");
  });
});

describe("attachEventListener: tool batch wiring (261028)", () => {
  test("client events feed record.toolBatches, refresh the widget on batch changes only, and a re-attach keeps the buffer but closes an open batch", (t) => {
    let now = 1_000;
    t.mock.method(Date, "now", () => now);
    let listener: ((event: unknown) => void) | undefined;
    const client = {
      onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
      getState: async () => ({}),
    } as unknown as RpcClient;
    const record = freshRpcRecord({ client, running: true, streaming: true });
    const registry = new Map([[record.agentId, record]]);
    let refreshes = 0;
    agentWidgetRefreshRef.current = () => { refreshes += 1; };
    t.after(() => { agentWidgetRefreshRef.current = undefined; });
    attachEventListener(undefined, registry, record, client);
    assert.deepEqual(record.toolBatches?.batches, [], "attach creates an empty buffer");
    const buffer = record.toolBatches!;

    const message = { role: "assistant", provider: "p", model: "m", stopReason: "toolUse", usage: { output: 10 }, content: [
      { type: "toolCall", id: "c1", name: "read", arguments: { path: "a.ts" } },
      { type: "toolCall", id: "c2", name: "grep", arguments: { pattern: "x" } },
    ] };
    listener?.({ type: "message_update", message, assistantMessageEvent: { type: "text_delta", delta: "hi" } });
    assert.deepEqual(buffer.batches, [], "token deltas never touch the buffer");

    now = 2_000;
    const before = refreshes;
    listener?.({ type: "message_end", message });
    assert.equal(buffer.batches.length, 1);
    assert.deepEqual(buffer.batches[0].calls.map((call) => [call.toolName, call.argsPreview]), [["read", "path: a.ts"], ["grep", "pattern: x"]]);
    assert.ok(refreshes > before, "an opened batch refreshes the widget");

    now = 3_000;
    listener?.({ type: "tool_execution_start", toolCallId: "c1", toolName: "read", args: { path: "a.ts" } });
    now = 4_000;
    listener?.({ type: "tool_execution_start", toolCallId: "c2", toolName: "grep", args: { pattern: "x" } });
    assert.deepEqual(buffer.batches[0].calls.map((call) => call.startedAt), [3_000, 4_000]);
    now = 5_000;
    listener?.({ type: "tool_execution_end", toolCallId: "c1", result: { content: [{ type: "text", text: "done" }] } });
    assert.equal(buffer.batches[0].endedAt, undefined, "one call is still in flight");

    const quiet = refreshes;
    listener?.({ type: "message_update", message, assistantMessageEvent: { type: "text_delta", delta: "more" } });
    assert.equal(refreshes, quiet, "a token delta causes no widget refresh");

    // The launch ends mid-batch; a re-attach (next launch) keeps history but never leaves the batch running.
    now = 9_000;
    attachEventListener(undefined, registry, record, client);
    assert.equal(record.toolBatches, buffer, "the buffer survives the re-attach");
    assert.equal(buffer.batches.length, 1);
    assert.equal(buffer.batches[0].endedAt, 9_000);

    now = 10_000;
    listener?.({ type: "tool_execution_start", toolCallId: "c3", toolName: "bash", args: { command: "ls" } });
    now = 11_000;
    listener?.({ type: "tool_execution_end", toolCallId: "c3" });
    assert.equal(buffer.batches.length, 2);
    assert.equal(buffer.batches[1].endedAt, 11_000);
    assert.deepEqual(buffer.batches.map((batch) => batch.seq), [1, 2]);
  });
});

describe("WORKER_LIFECYCLE_GUIDE: no mid-run questions", () => {
  test("limits ws-report-to-lead to progress and directs decide-or-settle instead of asking", () => {
    assert.match(WORKER_LIFECYCLE_GUIDE, /ws-report-to-lead only for intermediate progress, not completion/);
    assert.match(WORKER_LIFECYCLE_GUIDE, /Every shell command uses ws-worker-exec/);
    assert.match(WORKER_LIFECYCLE_GUIDE, /Do not spawn native children/);
    assert.match(WORKER_LIFECYCLE_GUIDE, /settle with a blocker/);
  });
});

describe("applyRpcEvent: ws-report-to-lead is intermediate only", () => {
  test("every report pushes immediately as ws-agent-report; a residual kind (question or final) has no meaning", () => {
    const r = freshRpcRecord();
    assert.equal(applyRpcEvent(r, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "progress" } }).push?.family, "ws-agent-report");
    r.onQuestionReport = () => "must not be consulted";
    assert.deepEqual(
      applyRpcEvent(r, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "question", message: "question" } }),
      { push: { family: "ws-agent-report", payload: { report: "question" }, deliverAs: "followUp" } },
      "kind:question yields a plain report, never ws-agent-question, even with a stale hook on the record",
    );
    assert.ok(r.reportLog.every((entry) => entry.kind === undefined), "no report is logged as a question");
    const legacy = applyRpcEvent(r, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "final", message: "legacy final" } });
    assert.equal(legacy.push?.family, "ws-agent-report");
    assert.equal(r.terminalDelivery, undefined);
  });
});

describe("applyRpcEvent: ws-worker-exec (260904 Phase 1 approval-request capture)", () => {
  test("a tool_execution_start for the gated-exec tool with a string toolCallId + command sets pendingApproval from the event's own toolCallId (the cmd_id)", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, {
      type: "tool_execution_start",
      toolName: GATED_EXEC_TOOL_NAME,
      toolCallId: "call-123",
      args: { command: "rm -rf build", rationale: "clean stale build output" },
    });
    assert.deepEqual(record.pendingApproval, { cmdId: "call-123", command: "rm -rf build", rationale: "clean stale build output", cwd: undefined });
  });

  test("rationale is omitted (undefined) when args.rationale is not a string", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, {
      type: "tool_execution_start",
      toolName: GATED_EXEC_TOOL_NAME,
      toolCallId: "call-456",
      args: { command: "echo hi" },
    });
    assert.deepEqual(record.pendingApproval, { cmdId: "call-456", command: "echo hi", rationale: undefined, cwd: undefined });
  });

  test("a missing toolCallId is ignored — pendingApproval is never set without a usable cmd_id", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, args: { command: "echo hi" } });
    assert.equal(record.pendingApproval, undefined);
  });

  test("a missing or non-string args.command is ignored", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-1", args: {} });
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-2", args: { command: 42 } });
    assert.equal(record.pendingApproval, undefined);
  });

  test("a tool_execution_start for a different toolName (e.g. ws-report-to-lead) never sets pendingApproval", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, toolCallId: "call-1", args: { message: "hi", command: "echo hi" } });
    assert.equal(record.pendingApproval, undefined);
  });

  test("a later gated-exec event overwrites a still-pending approval from an earlier one (the newest command is the one awaiting a decision)", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-1", args: { command: "echo one" } });
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-2", args: { command: "echo two" } });
    assert.deepEqual(record.pendingApproval, { cmdId: "call-2", command: "echo two", rationale: undefined, cwd: undefined });
  });

  test("review fix (relay #1, CORRECTNESS finding #1): a string args.cwd override is captured onto pendingApproval.cwd", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, {
      type: "tool_execution_start",
      toolName: GATED_EXEC_TOOL_NAME,
      toolCallId: "call-1",
      args: { command: "echo hi", cwd: "/repo/subdir" },
    });
    assert.deepEqual(record.pendingApproval, { cmdId: "call-1", command: "echo hi", rationale: undefined, cwd: "/repo/subdir" });
  });
  test("260905: the gated-exec branch returns an empty outcome — the approval PUSH is createApprovalRelay's job, not applyRpcEvent's", () => {
    const record = freshRpcRecord();
    const result = applyRpcEvent(record, {
      type: "tool_execution_start",
      toolName: GATED_EXEC_TOOL_NAME,
      toolCallId: "call-1",
      args: { command: "echo hi" },
    });
    assert.deepEqual(result, {}, "execute-gateway.ts owns the §7 payload and the working-context scrape");
    assert.deepEqual(record.pendingApproval, { cmdId: "call-1", command: "echo hi", rationale: undefined, cwd: undefined });
  });

  test("260905: an approval request does NOT mark the child terminal — it is still outstanding, and the lead is what unblocks it", () => {
    const record = freshRpcRecord({ running: true });
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-1", args: { command: "echo hi" } });
    assert.equal(record.running, true);
  });


  test("cwd is omitted (undefined) when args.cwd is missing or not a string", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-1", args: { command: "echo hi" } });
    assert.equal(record.pendingApproval?.cwd, undefined);
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-2", args: { command: "echo hi", cwd: 42 } });
    assert.equal(record.pendingApproval?.cwd, undefined);
  });
});

describe("attachApprovalChannel (260924: parent side of the channel-delivered decision)", () => {
  type Host = Parameters<typeof attachApprovalChannel>[1] & {
    deliver(msg: Record<string, unknown>): void;
    drop(): void;
    reconnect(hello: Partial<ChannelHello>): void;
    listeners(): number;
  };

  /** In-memory stand-in for the launch's `ParentChannel` subscription surface. */
  function fakeHost(): Host {
    const messages = new Set<(msg: Record<string, unknown>) => void>();
    const disconnects = new Set<() => void>();
    const connections = new Set<(conn: ChannelConnection, hello: ChannelHello) => void>();
    return {
      onMessage: (cb) => { messages.add(cb); return () => { messages.delete(cb); }; },
      onDisconnect: (cb) => { disconnects.add(cb); return () => { disconnects.delete(cb); }; },
      onConnection: (cb) => { connections.add(cb); return () => { connections.delete(cb); }; },
      deliver: (msg) => { for (const cb of [...messages]) cb(msg); },
      drop: () => { for (const cb of [...disconnects]) cb(); },
      reconnect: (hello) => { for (const cb of [...connections]) cb({} as ChannelConnection, { reconnect: true, resume: {}, ...hello }); },
      listeners: () => messages.size + disconnects.size + connections.size,
    };
  }

  function pendingRecord(decision?: "sent" | "discarded"): RpcAgentRecord {
    return liveRpcRecord({ running: true, pendingApproval: { cmdId: "call-1", command: "rm -rf build", rationale: "clean", cwd: "/repo/sub", decision } });
  }

  test("the child's consumption acknowledgment for the pending cmd_id releases the request and refreshes the widget; any other cmd_id or message is ignored", (t) => {
    let refreshes = 0;
    agentWidgetRefreshRef.current = () => { refreshes += 1; };
    t.after(() => { agentWidgetRefreshRef.current = undefined; });
    const host = fakeHost();
    const record = pendingRecord("sent");
    attachApprovalChannel(record, host, () => undefined);

    host.deliver(approvalConsumedMessage("call-OTHER"));
    host.deliver({ t: "ready", kind: "web", payload: {} });
    host.deliver({ t: "approval-consumed" });
    assert.equal(record.pendingApproval?.decision, "sent", "a foreign or malformed acknowledgment consumes nothing");
    assert.equal(refreshes, 0);

    host.deliver({ ...approvalConsumedMessage("call-1"), gen: 4 });
    assert.equal(record.pendingApproval, undefined, "the acknowledged cmd_id is no longer pending");
    assert.equal(refreshes, 1);
  });

  for (const consumption of ["acknowledgment", "reconnect hello"] as const) test(`attachEventListener publishes approval consumption from ${consumption} immediately despite pending telemetry`, t => {
    t.mock.timers.enable({ apis: ["Date", "setTimeout"], now: 10_000 });
    const host = fakeHost(), uplink = fakeUplink(), ancestorChannel = fakeParentChannel();
    const send = uplink.send;
    uplink.send = msg => { send(msg); if (msg.t === "subtree") ancestorChannel.deliver(msg.snapshot); };
    const ancestor = liveRpcRecord({ agentId: "ancestor", channel: ancestorChannel.parent, running: false });
    observeChildSubtree(undefined, ancestor, ancestorChannel.parent);
    let listener!: (event: unknown) => void, stateReads = 0, approvalPushes = 0;
    const client = {
      onEvent: (fn: typeof listener) => (listener = fn, () => {}),
      getState: async () => { stateReads++; return {}; },
    } as unknown as RpcClient;
    const record = liveRpcRecord({ client, channel: host as unknown as RpcAgentRecord["channel"], running: true, runStartedAt: 9_000, observedContextTokens: 100 });
    const registry = new Map([[record.agentId, record]]);
    installSubtreePublisher(registry, new SubtreeUpstream(uplink.channel), () => 0, idleOwnTurn);
    attachEventListener(undefined, registry, record, client, () => { approvalPushes++; });
    t.after(() => record.unsubscribe?.());
    const remote = () => ancestor.subtreeDescendants!.find(row => row.id === record.agentId)!.display!;

    listener({ type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-1", args: { command: "rm -rf build" } });
    record.pendingApproval = { ...record.pendingApproval!, decision: "sent" };
    assert.equal(remote().state, "awaiting-approval", "the RPC request reaches the ancestor through the publisher and observer");
    assert.equal(approvalPushes, 1);
    t.mock.timers.tick(1);
    record.observedContextTokens = 200;
    listener({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "x" } });
    assert.equal(remote().contextTokens, 100, "numeric telemetry is still waiting for its trailing sample");
    assert.equal(remote().lastOutputAt, 10_000);
    const sendsBefore = uplink.snapshots().length;

    // Only the approval channel speaks from here on: no RPC completion or manual publication.
    if (consumption === "acknowledgment") host.deliver(approvalConsumedMessage("call-1"));
    else {
      host.drop();
      assert.equal(record.pendingApproval?.decision, "discarded");
      host.reconnect({ resume: { approval: { consumed: ["call-1"] } } });
    }
    assert.equal(record.pendingApproval, undefined);
    assert.equal(uplink.snapshots().length, sendsBefore + 1, "consumption publishes before the numeric telemetry deadline");
    assert.equal(remote().state, "running", "an idle ancestor immediately replaces awaiting approval with the owner's classified state");
    assert.equal(remote().contextTokens, 200, "the state edge also flushes the latest pending numeric sample");
    assert.equal(remote().lastOutputAt, 10_001);
    assert.equal(stateReads, 0, "no RPC refresh or later output can serve as the publication oracle");
    assert.equal(approvalPushes, 1, "a consumed reconnect decision is not reissued");
    t.mock.timers.tick(1_000);
    assert.equal(uplink.snapshots().length, sendsBefore + 1, "the flushed trailing sample does not publish again");
  });

  test("a disconnect discards a sent-but-unacknowledged decision and leaves an unanswered request untouched", () => {
    const host = fakeHost();
    const sent = pendingRecord("sent");
    const unanswered = pendingRecord(undefined);
    attachApprovalChannel(sent, host, () => undefined);
    attachApprovalChannel(unanswered, host, () => undefined);

    host.drop();

    assert.equal(sent.pendingApproval?.decision, "discarded", "the decision is never re-sent; only the reconnect hello can settle it");
    assert.equal(sent.pendingApproval?.cmdId, "call-1", "the request itself stays pending");
    assert.deepEqual(unanswered.pendingApproval, { cmdId: "call-1", command: "rm -rf build", rationale: "clean", cwd: "/repo/sub", decision: undefined }, "nothing was in flight, nothing to discard");
  });

  test("a reconnect hello still reporting the cmd_id after a discard re-issues the request to the lead as a fresh push", () => {
    const host = fakeHost();
    const record = pendingRecord("sent");
    const asked: RpcAgentRecord[] = [];
    attachApprovalChannel(record, host, () => (r) => asked.push(r));
    host.drop();

    host.reconnect({ resume: { approval: { pending: "call-1" } } });

    assert.deepEqual(record.pendingApproval, { cmdId: "call-1", command: "rm -rf build", rationale: "clean", cwd: "/repo/sub", reissued: true, issue: 1 }, "the discarded decision is forgotten; the request is open again as issue 1");
    assert.deepEqual(asked, [record], "the lead is asked exactly once more");
    host.reconnect({ resume: { approval: { pending: "call-1" } } });
    assert.equal(asked.length, 1, "a re-issued request with no decision in flight is not re-asked again on a later reconnect");
  });

  test("an older child's reconnect hello (no consumed array) reporting no pending cmd_id after a discard keeps today's reading: the request is released", (t) => {
    let refreshes = 0;
    agentWidgetRefreshRef.current = () => { refreshes += 1; };
    t.after(() => { agentWidgetRefreshRef.current = undefined; });
    const host = fakeHost();
    const record = pendingRecord("sent");
    const asked: RpcAgentRecord[] = [];
    attachApprovalChannel(record, host, () => (r) => asked.push(r));
    host.drop();

    host.reconnect({ resume: {} });

    assert.equal(record.pendingApproval, undefined);
    assert.equal(refreshes, 1);
    assert.deepEqual(asked, []);
  });

  test("an older child's reconnect hello (no consumed array) reporting a different cmd_id after a discard also releases the old request", () => {
    const host = fakeHost();
    const record = pendingRecord("sent");
    attachApprovalChannel(record, host, () => undefined);
    host.drop();
    host.reconnect({ resume: { approval: { pending: "call-2" } } });
    assert.equal(record.pendingApproval, undefined, "call-2's own request arrives through the RPC event stream, not from the hello");
  });

  test("a reconnect hello with no decision in flight leaves the request alone: the lead already holds it and the RPC event stream owns it", () => {
    const host = fakeHost();
    const record = pendingRecord(undefined);
    const asked: RpcAgentRecord[] = [];
    attachApprovalChannel(record, host, () => (r) => asked.push(r));
    host.drop();
    host.reconnect({ resume: { approval: { pending: "call-1" } } });
    host.reconnect({ resume: {} });
    assert.equal(record.pendingApproval?.cmdId, "call-1");
    assert.equal(record.pendingApproval?.decision, undefined);
    assert.deepEqual(asked, []);
    // A first hello (not a reconnect) never reconciles either: it belongs to a new launch.
    record.pendingApproval = { cmdId: "call-1", command: "rm -rf build", decision: "discarded" };
    host.reconnect({ reconnect: false, resume: { approval: { pending: "call-1" } } });
    assert.equal(record.pendingApproval?.decision, "discarded");
    assert.deepEqual(asked, []);
  });

  test("a reconnect hello listing the discarded cmd_id as consumed releases the request without re-asking", (t) => {
    let refreshes = 0;
    agentWidgetRefreshRef.current = () => { refreshes += 1; };
    t.after(() => { agentWidgetRefreshRef.current = undefined; });
    const host = fakeHost();
    const record = pendingRecord("sent");
    const asked: RpcAgentRecord[] = [];
    attachApprovalChannel(record, host, () => (r) => asked.push(r));
    host.drop();
    host.reconnect({ resume: { approval: { consumed: ["call-0", "call-1"] } } });
    assert.equal(record.pendingApproval, undefined);
    assert.equal(refreshes, 1);
    assert.deepEqual(asked, []);
  });

  test("a reconnect hello with a consumed array that does not list the discarded cmd_id re-issues it, whether or not it reports the cmd_id pending", () => {
    for (const approval of [
      { consumed: [] },
      { consumed: ["call-OTHER"] },
      { pending: "call-1", consumed: [] },
      { pending: "call-2", consumed: [] },
    ]) {
      const host = fakeHost();
      const record = pendingRecord("sent");
      const asked: RpcAgentRecord[] = [];
      attachApprovalChannel(record, host, () => (r) => asked.push(r));
      host.drop();
      host.reconnect({ resume: { approval } });
      assert.deepEqual(record.pendingApproval, { cmdId: "call-1", command: "rm -rf build", rationale: "clean", cwd: "/repo/sub", reissued: true, issue: 1 }, `not released without consumption evidence: ${JSON.stringify(approval)}`);
      assert.deepEqual(asked, [record], "the lead is asked afresh");
    }
  });

  test("each reissue increments the request's issue number", () => {
    const host = fakeHost();
    const record = pendingRecord("sent");
    attachApprovalChannel(record, host, () => undefined);
    host.drop();
    host.reconnect({ resume: { approval: { consumed: [] } } });
    assert.equal(record.pendingApproval?.issue, 1);
    record.pendingApproval = { ...record.pendingApproval!, decision: "sent" };
    host.drop();
    host.reconnect({ resume: { approval: { pending: "call-1", consumed: [] } } });
    assert.equal(record.pendingApproval?.issue, 2);
    assert.equal(record.pendingApproval?.decision, undefined);
  });

  test("the approval hook resolves per event, so a record armed after the attach is still re-asked; detaching stops every subscription", () => {
    const host = fakeHost();
    const record = pendingRecord("sent");
    const asked: RpcAgentRecord[] = [];
    const detach = attachApprovalChannel(record, host, () => record.onApprovalPending);
    record.onApprovalPending = (r) => asked.push(r);
    host.drop();
    host.reconnect({ resume: { approval: { pending: "call-1" } } });
    assert.deepEqual(asked, [record]);

    assert.equal(host.listeners(), 3);
    detach();
    assert.equal(host.listeners(), 0);
    record.pendingApproval = { cmdId: "call-1", command: "rm -rf build", decision: "sent" };
    host.drop();
    host.deliver(approvalConsumedMessage("call-1"));
    assert.equal(record.pendingApproval?.decision, "sent", "a detached host changes nothing");
  });

  test("ownership protection persists from the request through a sent decision and a discard, and is released only by the acknowledgment", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-approval-protection-test-"));
    try {
      const ownership = allocateAgentHome(createAgentStorageContext("lead-1", root), "protected-a", "execute-worker");
      const record = liveRpcRecord({ agentId: "protected-a", ownership, sessionPath: ownership.sessionPath, running: true, pendingApproval: { cmdId: "call-1", command: "rm -rf build" } });
      const host = fakeHost();
      attachApprovalChannel(record, host, () => undefined);
      const protectedId = () => readOwnership(ownership.home)?.liveness.pendingApprovalCommandId;

      assert.ok(syncOwnershipProtection(record));
      assert.equal(protectedId(), "call-1", "the request protects the record");
      record.pendingApproval = { ...record.pendingApproval!, decision: "sent" };
      assert.ok(syncOwnershipProtection(record));
      assert.equal(protectedId(), "call-1", "sending the decision alone releases nothing");
      host.drop();
      assert.ok(syncOwnershipProtection(record));
      assert.equal(protectedId(), "call-1", "a discarded decision keeps the record protected");
      host.reconnect({ resume: { approval: { pending: "call-1" } } });
      assert.equal(protectedId(), "call-1", "a re-issued request keeps the record protected");
      record.pendingApproval = { ...record.pendingApproval!, decision: "sent" };
      host.deliver(approvalConsumedMessage("call-1"));
      assert.equal(protectedId(), undefined, "the acknowledgment releases the protection");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });
});

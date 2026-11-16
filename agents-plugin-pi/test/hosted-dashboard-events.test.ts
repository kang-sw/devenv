import { test } from "node:test";
import assert from "node:assert/strict";
import { createHostedDashboardEvents, startRpcDashboardEvents, HOSTED_EVENTS_WIDGET_KEY } from "../src/hosted-dashboard-events.ts";
import { FLUSH_INTERVAL_MS } from "../src/dashboard-feed.ts";
import { createReasoningSummaryObserver, SUMMARY_FRAME_BYTES } from "../src/reasoning-summary.ts";
import { hostedSummaryFrame } from "../src/reasoning-summary-transport.ts";
import { dashboardTaskCounts } from "../src/dashboard-feed.ts";
import { agentDashboardRefreshRef, markAgentExited, type RpcAgentRegistry } from "../src/spawner.ts";
import { countBackgroundTasks, killAllBackgroundTasks, startBackgroundTask } from "../src/background-tasks.ts";

function harness(reasoningSummary?: () => ReturnType<ReturnType<typeof createReasoningSummaryObserver>["projection"]>, reasoningSummaryCapable?: boolean) {
  let clock = 0;
  let task: (() => void) | undefined;
  let reads = 0;
  let fail = false;
  const counts = { timed: 0, monitors: 0 };
  const widgets: { key: string; lines: string[] }[] = [];
  const channel = createHostedDashboardEvents({
    reasoningSummary,
    reasoningSummaryCapable,
    now: () => clock,
    schedule: (fn) => { task = fn; return () => { task = undefined; }; },
    taskCounts: () => { reads++; return counts; },
    setWidget: (key, lines) => { if (fail) throw new Error("stale context"); widgets.push({ key, lines }); },
  });
  const flush = () => { clock += FLUSH_INTERVAL_MS; const fn = task; task = undefined; fn?.(); };
  const events = () => widgets.flatMap((widget) => widget.lines.map((line) => JSON.parse(line)));
  return { channel, counts, widgets, flush, events, reads: () => reads, fail: (value: boolean) => { fail = value; } };
}

const message = { role: "custom", customType: "ws-agent-report", content: "full report", details: { agent_id: "child", report: "finding" } };

test("hosted widget carries JSON wake/status event lines only, sampled at flush with no intake", () => {
  const h = harness();
  h.channel.start();
  assert.equal(h.reads(), 0);
  h.flush();
  assert.deepEqual(h.events(), [{ kind: "status", background: 0, monitors: 0 }]);
  h.channel.leadEvent({ type: "message_start", message });
  h.channel.leadEvent({ type: "message_end", message });
  h.channel.leadEvent({ type: "message_start", message });
  h.channel.leadEvent({ type: "agent_start" });
  h.counts.timed = 2;
  h.counts.monitors = 1;
  h.flush();
  assert.deepEqual(h.events().slice(1), [
    { kind: "wake", cause: "subagent_report", source: "child", caption: "finding", text: "full report" },
    { kind: "status", background: 2, monitors: 1 },
  ]);
  assert.ok(h.widgets.every((widget) => widget.key === HOSTED_EVENTS_WIDGET_KEY));
  h.channel.leadEvent({ type: "message_start", message: { role: "user", content: "monitor failed" } });
  h.flush();
  assert.equal(h.events().length, 3, "person input is not a wake; unchanged counts send nothing");
  h.channel.stop();
});

test("spawned workers report timed shells and monitors through their RPC widget", async () => {
  assert.equal(startRpcDashboardEvents(undefined, false, { taskCounts: countBackgroundTasks, setWidget: () => assert.fail("owner uses intake") }), undefined);
  const events: any[] = [];
  const channel = startRpcDashboardEvents("worker", false, {
    taskCounts: countBackgroundTasks,
    setWidget: (_key, lines) => events.push(...lines.map(line => JSON.parse(line))),
    schedule: () => () => {},
  })!;
  try {
    for (const timeoutS of [5, undefined]) {
      const done = new Promise<void>(resolve => startBackgroundTask({
        shell: process.execPath, args: ["-e", "setTimeout(() => {}, 100)"], cwd: process.cwd(),
        command: "test", why: "test", timeoutS, capBytes: 100, onComplete: () => resolve(),
      }));
      assert.deepEqual(events.at(-1), { kind: "status", background: timeoutS ? 1 : 0, monitors: timeoutS ? 0 : 1 });
      await done;
      assert.deepEqual(events.at(-1), { kind: "status", background: 0, monitors: 0 });
    }
  } finally { channel.stop(); killAllBackgroundTasks(); }
});

test("execute jobs stay counted from dispatch through approval waits until settlement/exit", () => {
  const registry = new Map([
    ["launching", { agentId: "launching", toolGroup: "execute-worker", launching: {}, reportLog: [] }],
    ["active", { agentId: "active", toolGroup: "execute-worker", running: true, reportLog: [] }],
    ["streaming", { agentId: "streaming", toolGroup: "execute-worker", streaming: true, reportLog: [] }],
    ["approval", { agentId: "approval", toolGroup: "execute-worker", pendingApproval: { decision: "sent" }, reportLog: [] }],
    ["done", { agentId: "done", toolGroup: "execute-worker", running: false, streaming: false, reportLog: [] }],
    ["other", { agentId: "other", toolGroup: "full-worker", running: true, reportLog: [] }],
  ]) as unknown as RpcAgentRegistry;
  const events: any[] = [];
  const channel = startRpcDashboardEvents(undefined, true, {
    taskCounts: () => dashboardTaskCounts(registry),
    setWidget: (_key, lines) => events.push(...lines.map(line => JSON.parse(line))),
    schedule: () => () => {},
  })!;
  agentDashboardRefreshRef.current = () => channel.tasksChanged();
  try {
    channel.leadEvent({ type: "agent_settled" });
    assert.deepEqual(events.at(-1), { kind: "status", background: 4, monitors: 0 });
    for (const id of ["launching", "active", "streaming", "approval"]) {
      const record = registry.get(id)!;
      record.launching = undefined;
      record.client = {} as never;
      markAgentExited(undefined, registry, record, { suppressTerminal: true });
    }
    assert.deepEqual(events.at(-1), { kind: "status", background: 0, monitors: 0 });
  } finally { agentDashboardRefreshRef.current = undefined; channel.stop(); }
});

test("real background task membership immediately updates hosted counts", async () => {
  const events: unknown[] = [];
  const channel = createHostedDashboardEvents({
    taskCounts: countBackgroundTasks,
    setWidget: (_key, lines) => events.push(...lines.map((line) => JSON.parse(line))),
    schedule: () => () => {},
  });
  channel.start();
  try {
    const done = new Promise<void>((resolve) => {
      startBackgroundTask({
        shell: process.execPath, args: ["-e", "setTimeout(() => {}, 100)"], cwd: process.cwd(),
        command: "test", why: "test", timeoutS: 5, capBytes: 100, onComplete: () => resolve(),
      });
    });
    assert.deepEqual(events, [{ kind: "status", background: 1, monitors: 0 }]);
    await done;
    assert.deepEqual(events, [
      { kind: "status", background: 1, monitors: 0 },
      { kind: "status", background: 0, monitors: 0 },
    ]);
    const monitorDone = new Promise<void>((resolve) => {
      startBackgroundTask({
        shell: process.execPath, args: ["-e", "setTimeout(() => {}, 100)"], cwd: process.cwd(),
        command: "test", why: "test", capBytes: 100, onComplete: () => resolve(),
      });
    });
    assert.deepEqual(events.at(-1), { kind: "status", background: 0, monitors: 1 });
    await monitorDone;
    assert.deepEqual(events.at(-1), { kind: "status", background: 0, monitors: 0 });
  } finally {
    channel.stop();
    killAllBackgroundTasks();
  }
});

test("hosted progress is coalesced and final is synchronous before native settlement, never an end command", () => {
  const observer = createReasoningSummaryObserver("1.5.2", "bb2edb47-671e-4f22-a8dc-616321c24cb7");
  const h = harness(() => observer.projection(), true);
  const ctx = { model: { provider: "openai-codex", api: "openai-codex-responses", id: "test" }, modelRegistry: { getRegisteredProviderConfig() {}, getRegisteredNativeProvider() {} } };
  const send = (event: unknown) => { observer.observe(event, ctx); h.channel.leadEvent(event); };
  let sequence = 0;
  const raw = (data: Record<string, unknown>) => send({ type: "provider_stream_event", provider: "openai-codex", api: "openai-codex-responses", model: "test", data: { sequence_number: sequence++, ...data } });
  h.channel.start(); h.flush();
  send({ type: "agent_start" }); send({ type: "before_provider_request" });
  raw({ type: "response.created", response: { id: "r" } });
  raw({ type: "response.output_item.added", output_index: 0, item: { type: "reasoning", id: "i" } });
  for (let i = 0; i < 100; i++) raw({ type: "response.reasoning_summary_text.delta", item_id: "i", output_index: 0, summary_index: 0, delta: "\u0000\"\\한😀" });
  assert.equal(h.events().filter((e) => e.reasoningSummary).length, 0, "provider observer does no widget IO");
  h.flush();
  assert.equal(h.events().filter((e) => e.reasoningSummary).length, 1);
  send({ type: "turn_end" });
  assert.equal(h.events().filter((e) => e.reasoningSummary).length, 1, "per-request turn end cannot flush final");
  const before = h.widgets.length;
  send({ type: "agent_settled", aborted: true });
  assert.equal(h.widgets.length, before + 2, "capability hello and final emitted before handler returns / native agent_settled");
  assert.deepEqual(JSON.parse(h.widgets[before].lines[0]).capabilities, ["reasoning_summary_v1"]);
  const final = h.events().at(-1)!;
  assert.equal(final.kind, "agent"); assert.equal(final.agentId, undefined);
  assert.deepEqual(Object.keys(final).sort(), ["kind", "reasoningSummary"]);
  assert.equal(final.reasoningSummary.activeRequest, null);
  assert.equal(final.reasoningSummary.requests[0].status, "aborted");
  assert.ok(h.widgets.filter((w) => w.lines.some((l) => JSON.parse(l).reasoningSummary)).every((w) => w.lines.length === 1 && Buffer.byteLength(hostedSummaryFrame(w.lines[0])) <= SUMMARY_FRAME_BYTES));
  h.flush(); assert.equal(h.widgets.length, before + 2, "no pending closed-turn source queue");
  h.channel.stop();
});

test("unsupported host/no summary/redacted thinking retains ordinary hosted wake/count fallback", () => {
  for (const version of ["0.84.4", "unknown", "1.5.2"]) {
    const observer = createReasoningSummaryObserver(version);
    const h = harness(() => observer.projection());
    h.channel.start(); h.flush();
    const event = { type: "message_update", assistantMessageEvent: { type: "thinking_delta", delta: "SECRET" } };
    observer.observe(event); h.channel.leadEvent(event);
    h.channel.leadEvent({ type: "agent_settled" }); h.flush();
    assert.deepEqual(h.events(), [{ kind: "status", background: 0, monitors: 0 }]);
    h.channel.stop(); observer.retire();
  }
});

test("new bundle advertises optional capability before data and renews it at real boundaries", () => {
  for (const capable of [true, false]) {
    const h = harness(undefined, capable);
    const expected = capable ? ["reasoning_summary_v1"] : [];
    h.channel.start(); h.flush();
    assert.deepEqual(h.events()[0], { kind: "status", background: 0, monitors: 0, capabilities: expected });
    h.channel.leadEvent({ type: "agent_start" }); h.flush();
    assert.equal(h.events().length, 2, "unchanged counts still carry the new turn's explicit opt-in");
    h.channel.leadEvent({ type: "agent_settled" });
    assert.equal(h.events().length, 3, "hello is synchronous before native settlement even without summary text");
    assert.deepEqual(h.events().at(-1).capabilities, expected);
    h.channel.stop();
  }
});

test("a summary callback alone never opts a legacy or unsupported bundle into sending fields", () => {
  for (const capable of [undefined, false]) {
    const h = harness(() => { throw new Error("summary read without capability"); }, capable);
    h.channel.start(); h.flush();
    h.channel.leadEvent({ type: "before_provider_request" }); h.flush();
    h.channel.leadEvent({ type: "agent_settled" });
    assert.ok(h.events().every((e) => e.reasoningSummary === undefined));
    h.channel.stop();
  }
});

test("throwing widget retains wakes for the next mark; stop drops pending state", () => {
  const h = harness();
  h.channel.start();
  h.flush();
  h.fail(true);
  h.channel.leadEvent({ type: "message_start", message });
  h.flush();
  assert.equal(h.events().length, 1);
  h.fail(false);
  h.channel.leadEvent({ type: "message_start", message: { role: "user", content: "retry mark" } });
  h.flush();
  assert.equal(h.events().filter((event) => event.kind === "wake").length, 1);
  h.channel.leadEvent({ type: "message_start", message: { ...message } });
  h.channel.stop();
  h.flush();
  assert.equal(h.events().filter((event) => event.kind === "wake").length, 1);
});

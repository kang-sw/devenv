/**
 * Unit tests for hosted-agent-rows.ts: the hosted lead's subagent rows, sent
 * as JSON event lines through one reserved widget. Driven with a fake clock
 * and scheduler and a recording `setWidget`, so every update is explicit.
 */

import { test } from "node:test";
import assert from "node:assert/strict";
import { AGENT_POLL_MS, FLUSH_INTERVAL_MS, type DashboardAgentEntry, type DashboardEvent } from "../src/dashboard-feed.ts";
import { PROGRESS_COALESCE_MS, FULL_RESEND_MS, HOSTED_AGENTS_WIDGET_KEY, createHostedAgentRows, type HostedAgentRowsOptions } from "../src/hosted-agent-rows.ts";

const entry = (agentId: string, over: Partial<DashboardAgentEntry> = {}): DashboardAgentEntry => ({
  agentId,
  label: agentId,
  status: "running",
  caption: "worker",
  batches: [],
  ...over,
});

function harness(overrides: Partial<HostedAgentRowsOptions> = {}) {
  let clock = 10_000;
  let nextId = 0;
  const timers = new Map<number, { at: number; fn: () => void }>();
  const widgets: { key: string; lines: string[] }[] = [];
  const tree = { entries: [] as DashboardAgentEntry[] };
  const rows = createHostedAgentRows({
    setWidget: (key, lines) => { widgets.push({ key, lines }); },
    agents: () => tree.entries,
    now: () => clock,
    schedule: (fn, ms) => {
      const id = nextId++;
      timers.set(id, { at: clock + ms, fn });
      return () => { timers.delete(id); };
    },
    ...overrides,
  });
  function advance(ms: number): void {
    const until = clock + ms;
    for (;;) {
      const due = [...timers].filter(([, timer]) => timer.at <= until).sort((a, b) => a[1].at - b[1].at)[0];
      if (!due) break;
      timers.delete(due[0]);
      clock = Math.max(clock, due[1].at);
      due[1].fn();
    }
    clock = until;
  }
  /** Every event sent so far, decoded from the widget lines. */
  const events = (): DashboardEvent[] => widgets.flatMap((w) => w.lines.map((line) => JSON.parse(line) as DashboardEvent));
  return { rows, widgets, events, tree, advance, timers };
}

test("every update is the reserved widget, its lines child agent and permission events only", () => {
  const h = harness();
  h.tree.entries = [entry("a", { sessionKey: "sk-a", model: "m", effort: "high", contextTokens: 10, outputTps: 3 }), entry("b", { parentId: "a", permission: { tool: "ws-worker-exec", args: { command: "ls" } } })];
  h.rows.start();
  h.advance(0);
  assert.equal(h.widgets.length, 1);
  assert.equal(h.widgets[0]!.key, HOSTED_AGENTS_WIDGET_KEY);
  assert.deepEqual(h.events(), [
    { kind: "agent", agentId: "a", label: "a", sessionKey: "sk-a", status: "running", caption: "worker", batches: [], model: "m", effort: "high", contextTokens: 10, outputTps: 3 },
    { kind: "agent", agentId: "b", parentId: "a", label: "b", status: "running", caption: "worker", batches: [] },
    { kind: "permission", agentId: "b", tool: "ws-worker-exec", args: { command: "ls" } },
  ]);
  // Run every timer the channel keeps (poll, resend) through several cycles, with changes.
  h.tree.entries = [entry("a", { status: "done" })];
  h.rows.agentsChanged();
  h.advance(FULL_RESEND_MS * 2 + 1);
  for (const event of h.events()) {
    assert.ok(event.kind === "agent" || event.kind === "permission", `kind ${event.kind}`);
    assert.equal(typeof event.agentId, "string");
    assert.ok((event.agentId as string).length > 0);
  }
});

test("only changes are sent; a child that left the tree while running ends as done", () => {
  const h = harness();
  h.tree.entries = [entry("a"), entry("b")];
  h.rows.start();
  h.advance(0);
  h.widgets.length = 0;
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.deepEqual(h.widgets, [], "an unchanged tree sends nothing");
  h.tree.entries = [entry("a", { caption: "changed" })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.deepEqual(h.events(), [
    { kind: "agent", agentId: "a", label: "a", status: "running", caption: "changed", batches: [] },
    { kind: "agent", agentId: "b", status: "done" },
  ]);
  h.widgets.length = 0;
  h.tree.entries = [entry("a", { status: "done" })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  h.widgets.length = 0;
  h.tree.entries = [];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.deepEqual(h.widgets, [], "a finished child leaving the tree sends nothing");
});

test("dirty marks coalesce at most once per flush interval; the tree is polled while a child runs", () => {
  const h = harness();
  let reads = 0;
  h.tree.entries = [entry("a")];
  const rows = createHostedAgentRows({ setWidget: () => {}, agents: () => { reads++; return h.tree.entries; }, now: () => 0, schedule: () => () => {} });
  rows.agentsChanged();
  assert.equal(reads, 0, "a stopped channel reads nothing");

  h.rows.start();
  h.advance(0);
  assert.equal(h.widgets.length, 1);
  h.tree.entries = [entry("a", { caption: "c1" })];
  h.rows.agentsChanged();
  h.advance(10);
  h.tree.entries = [entry("a", { caption: "c2" })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS - 20);
  assert.equal(h.widgets.length, 1, "inside the interval nothing is sent");
  h.advance(10);
  assert.equal(h.widgets.length, 2, "one update at the interval's end");
  assert.equal((JSON.parse(h.widgets[1]!.lines[0]!) as DashboardEvent).caption, "c2");

  // No dirty mark: the poll still picks up a change while the child runs.
  h.tree.entries = [entry("a", { caption: "c3" })];
  h.advance(AGENT_POLL_MS);
  assert.equal(h.widgets.length, 3);
  // Once nothing runs, the poll stops.
  h.tree.entries = [entry("a", { status: "done" })];
  h.advance(AGENT_POLL_MS);
  assert.equal(h.widgets.length, 4);
  h.tree.entries = [entry("a", { status: "done", caption: "changed" })];
  h.advance(AGENT_POLL_MS * 5);
  assert.equal(h.widgets.length, 4, "no poll while nothing runs");
});

test("while any child exists the whole tree is sent again without batches; with none, nothing", () => {
  const h = harness();
  const batch = { epoch: "e", seq: 1, toolNames: ["read"], callCount: 1, failed: 0, startedAt: 1, endedAt: 2 };
  h.tree.entries = [entry("a", { status: "done", batches: [batch as never] }), entry("b", { permission: { tool: "ws-worker-exec", args: { command: "x" } } })];
  h.rows.start();
  h.advance(0);
  h.tree.entries = [entry("a", { status: "done", batches: [batch as never] }), entry("b", { status: "done" })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  h.widgets.length = 0;
  h.advance(FULL_RESEND_MS);
  assert.equal(h.widgets.length, 1);
  assert.deepEqual(h.events(), [
    { kind: "agent", agentId: "a", label: "a", status: "done", caption: "worker" },
    { kind: "permission", agentId: "a", clear: true },
    { kind: "agent", agentId: "b", label: "b", status: "done", caption: "worker" },
    { kind: "permission", agentId: "b", clear: true },
  ]);
  h.advance(FULL_RESEND_MS);
  assert.equal(h.widgets.length, 2, "the resend repeats");
  h.tree.entries = [];
  h.advance(FULL_RESEND_MS * 3);
  assert.equal(h.widgets.length, 2, "an empty tree is not resent");
  assert.equal(h.timers.size, 0, "and nothing stays armed");
});

test("a throwing setWidget is swallowed and the next change sends every child; stop sends nothing more", () => {
  let fail = true;
  const sent: string[][] = [];
  const h = harness({ setWidget: (_key, lines) => { if (fail) throw new Error("stale ctx"); sent.push(lines); } });
  h.tree.entries = [entry("a"), entry("b")];
  h.rows.start();
  h.advance(0);
  fail = false;
  h.tree.entries = [entry("a"), entry("b", { contextTokens: 1 })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.deepEqual(sent.flat().map((line) => (JSON.parse(line) as DashboardEvent).agentId), ["a", "b"]);
  h.rows.stop();
  assert.equal(h.timers.size, 0);
  h.rows.agentsChanged();
  h.advance(FULL_RESEND_MS * 2);
  assert.equal(sent.length, 1);
});

const generating = (chars: number, kind: "tool" | "thinking" = "tool") => ({
  epoch: "e", seq: 1, toolNames: [], callCount: 0, failed: 0, startedAt: 1,
  generating: kind === "tool" ? { kind, phaseStartedAt: 1, toolName: "write", chars } : { kind, phaseStartedAt: 1 },
}) as never;
const finished = (seq: number) => ({ epoch: "e", seq, toolNames: ["read"], callCount: 1, failed: 0, startedAt: seq, endedAt: seq + 1 }) as never;

test("streaming character growth alone sends nothing, polled or marked", () => {
  const h = harness();
  h.tree.entries = [entry("a", { batches: [generating(10)] })];
  h.rows.start();
  h.advance(0);
  assert.equal(h.widgets.length, 1);
  for (let chars = 20; chars <= 2000; chars += 20) {
    h.tree.entries = [entry("a", { batches: [generating(chars)] })];
    h.rows.agentsChanged();
    h.advance(FLUSH_INTERVAL_MS);
  }
  h.advance(PROGRESS_COALESCE_MS * 3);
  assert.equal(h.widgets.length, 1, "chars growth is no change, even after the batch window");
  // A later real change carries the current count.
  h.tree.entries = [entry("a", { status: "done", batches: [generating(2000)] })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.equal(h.widgets.length, 2);
  assert.deepEqual((h.events()[1]!.batches as { generating: { chars: number } }[])[0]!.generating.chars, 2000);
});

test("a batch-only change goes out once per coalesce window however many arrive", () => {
  const h = harness();
  h.tree.entries = [entry("a", { batches: [generating(0, "thinking")] }), entry("b")];
  h.rows.start();
  h.advance(0);
  assert.equal(h.widgets.length, 1, "the first send carries batches at once");
  // Batch changes every flush interval across several windows.
  let seq = 1;
  const sendTimes: number[] = [];
  let elapsed = 0;
  while (elapsed < PROGRESS_COALESCE_MS * 3) {
    h.tree.entries = [entry("a", { batches: [finished(seq++)] }), entry("b")];
    h.rows.agentsChanged();
    const before = h.widgets.length;
    h.advance(FLUSH_INTERVAL_MS);
    elapsed += FLUSH_INTERVAL_MS;
    if (h.widgets.length > before) sendTimes.push(elapsed);
  }
  assert.deepEqual(sendTimes, [PROGRESS_COALESCE_MS, PROGRESS_COALESCE_MS * 2, PROGRESS_COALESCE_MS * 3], "one send per window, at its end");
  // Each send is only the changed child's agent event, with the newest batches.
  const last = h.widgets.at(-1)!.lines.map((line) => JSON.parse(line) as DashboardEvent);
  assert.equal(last.length, 1);
  assert.equal(last[0]!.agentId, "a");
  assert.deepEqual(last[0]!.batches, [finished(seq - 1)]);

  // A held change with no further marks still goes out when the window ends.
  h.advance(PROGRESS_COALESCE_MS * 2);
  const count = h.widgets.length;
  h.tree.entries = [entry("a", { status: "done", batches: [finished(100)] }), entry("b", { status: "done" })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.equal(h.widgets.length, count + 1);
  h.tree.entries = [entry("a", { status: "done", batches: [finished(100), finished(101)] }), entry("b", { status: "done" })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.equal(h.widgets.length, count + 1, "held inside the window");
  h.advance(PROGRESS_COALESCE_MS);
  assert.equal(h.widgets.length, count + 2, "sent by the window timer, no poll or mark needed");
  assert.deepEqual(h.events().at(-1)!.batches, [finished(100), finished(101)]);
});

test("status, permission, label, and model changes go out at the next flush inside a batch window", () => {
  const h = harness();
  h.tree.entries = [entry("a", { batches: [finished(1)] })];
  h.rows.start();
  h.advance(0);
  // Open a window with a batch-only send.
  h.advance(PROGRESS_COALESCE_MS);
  h.tree.entries = [entry("a", { batches: [finished(2)] })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.equal(h.widgets.length, 2);
  // A batch change now is held...
  h.tree.entries = [entry("a", { batches: [finished(3)] })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.equal(h.widgets.length, 2);
  // ...but a permission goes out at once, alone.
  h.tree.entries = [entry("a", { batches: [finished(3)], permission: { tool: "ws-worker-exec", args: { command: "ls" } } })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.equal(h.widgets.length, 3);
  assert.deepEqual(h.widgets[2]!.lines.map((line) => JSON.parse(line) as DashboardEvent), [{ kind: "permission", agentId: "a", tool: "ws-worker-exec", args: { command: "ls" } }]);
  // Status, label, and model each go out at once, carrying the current batches.
  for (const over of [{ status: "done" as const }, { status: "done" as const, label: "renamed" }, { status: "done" as const, label: "renamed", model: "m2" }]) {
    const before = h.widgets.length;
    h.tree.entries = [entry("a", { batches: [finished(3)], permission: { tool: "ws-worker-exec", args: { command: "ls" } }, ...over })];
    h.rows.agentsChanged();
    h.advance(FLUSH_INTERVAL_MS);
    assert.equal(h.widgets.length, before + 1, JSON.stringify(over));
    assert.deepEqual(h.events().at(-1)!.batches, [finished(3)]);
  }
});

test("the full resend and a done for a vanished child still go out while batches are held", () => {
  const h = harness();
  h.tree.entries = [entry("a", { batches: [finished(1)] }), entry("b")];
  h.rows.start();
  h.advance(0);
  h.advance(PROGRESS_COALESCE_MS);
  h.tree.entries = [entry("a", { batches: [finished(2)] }), entry("b")];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  // Held batch change for a, and b vanishes while running.
  h.tree.entries = [entry("a", { batches: [finished(3)] })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.deepEqual(h.widgets.at(-1)!.lines.map((line) => JSON.parse(line) as DashboardEvent), [{ kind: "agent", agentId: "b", status: "done" }]);
  h.widgets.length = 0;
  h.tree.entries = [entry("a", { status: "done", batches: [finished(3)] })];
  h.rows.agentsChanged();
  h.advance(FULL_RESEND_MS);
  assert.deepEqual(h.events().at(-1), { kind: "permission", agentId: "a", clear: true });
  assert.ok(h.events().some((event) => event.kind === "agent" && event.agentId === "a" && event.batches === undefined), "the resend still carries no batches");
});

test("contextTokens and outputTps changing every flush interval go out only on the coalesce windows", () => {
  const h = harness();
  h.tree.entries = [entry("a", { model: "m", effort: "high", contextTokens: 0, outputTps: 0 })];
  h.rows.start();
  h.advance(0);
  assert.equal(h.widgets.length, 1);
  const sendTimes: number[] = [];
  let elapsed = 0;
  let tick = 0;
  while (elapsed < PROGRESS_COALESCE_MS * 3) {
    tick++;
    h.tree.entries = [entry("a", { model: "m", effort: "high", contextTokens: tick * 100, outputTps: 40 + (tick % 7) })];
    h.rows.agentsChanged();
    const before = h.widgets.length;
    h.advance(FLUSH_INTERVAL_MS);
    elapsed += FLUSH_INTERVAL_MS;
    if (h.widgets.length > before) sendTimes.push(elapsed);
  }
  assert.deepEqual(sendTimes, [PROGRESS_COALESCE_MS, PROGRESS_COALESCE_MS * 2, PROGRESS_COALESCE_MS * 3], "one send per window, at its end");
  assert.equal(h.events().at(-1)!.contextTokens, tick * 100, "each send carries the newest values");
  // Tokens alone and batches alone share one window: a batch change right after a tokens send is held too.
  h.tree.entries = [entry("a", { model: "m", effort: "high", contextTokens: tick * 100, outputTps: 40 + (tick % 7), batches: [finished(1)] })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.equal(h.widgets.length, 4, "held inside the shared window");
  // An immediate field (effort) goes out at once inside the window, carrying the current tokens, rate, and batches.
  h.tree.entries = [entry("a", { model: "m", effort: "low", contextTokens: 99_999, outputTps: 1, batches: [finished(1)] })];
  h.rows.agentsChanged();
  h.advance(FLUSH_INTERVAL_MS);
  assert.equal(h.widgets.length, 5);
  assert.deepEqual(h.events().at(-1), { kind: "agent", agentId: "a", label: "a", status: "running", caption: "worker", batches: [finished(1)], model: "m", effort: "low", contextTokens: 99_999, outputTps: 1 });
});

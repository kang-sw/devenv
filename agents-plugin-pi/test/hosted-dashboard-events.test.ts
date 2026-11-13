import { test } from "node:test";
import assert from "node:assert/strict";
import { createHostedDashboardEvents, HOSTED_EVENTS_WIDGET_KEY } from "../src/hosted-dashboard-events.ts";
import { FLUSH_INTERVAL_MS } from "../src/dashboard-feed.ts";

function harness() {
  let clock = 0;
  let task: (() => void) | undefined;
  let reads = 0;
  let fail = false;
  const counts = { timed: 0, monitors: 0 };
  const widgets: { key: string; lines: string[] }[] = [];
  const channel = createHostedDashboardEvents({
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

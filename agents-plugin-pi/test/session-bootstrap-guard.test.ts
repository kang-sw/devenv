/** Bootstrap guard and output-drain contracts, without launching Pi or MCP. */
import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { mock, test } from "node:test";
import { bootstrapOrFailLoud, drainStdoutBeforeExit } from "../src/index.ts";
import { registerAgentTools } from "../src/spawner.ts";

function fakeUi() {
  const notify = mock.fn();
  return { ui: { notify }, notify };
}

test("execution child: bootstrap failure notifies loudly and exits instead of falling through toolless", async () => {
  const { ui, notify } = fakeUi();
  const events: string[] = [];
  const exit = mock.fn((code: number) => { events.push(`exit:${code}`); return undefined as never; });
  const result = await bootstrapOrFailLoud(ui, "worker", async () => { throw new Error("bridge initialization failed"); }, exit, async () => { events.push("drained"); });
  assert.equal(result, undefined);
  assert.equal(notify.mock.callCount(), 1);
  assert.equal(notify.mock.calls[0].arguments[1], "error");
  assert.match(String(notify.mock.calls[0].arguments[0]), /bridge initialization failed/);
  assert.deepEqual(events, ["drained", "exit:1"]);
});

test("host lead: bootstrap failure notifies loudly but never exits the interactive owner", async () => {
  const { ui, notify } = fakeUi();
  const exit = mock.fn((_code: number) => undefined as never);
  const result = await bootstrapOrFailLoud(ui, undefined, async () => { throw new Error("bridge unavailable"); }, exit);
  assert.equal(result, undefined);
  assert.equal(notify.mock.callCount(), 1);
  assert.match(String(notify.mock.calls[0].arguments[0]), /bridge unavailable/);
  assert.equal(exit.mock.callCount(), 0);
});

test("healthy bootstrap returns a live execution engine, not retired native tools", async t => {
  const tools: string[] = [];
  const pi = { registerTool: (tool: { name: string }) => tools.push(tool.name), on() {}, sendMessage() {}, sendUserMessage() {} } as never;
  const bridge = { client: {}, wsToolNames: [], defaultSessionKeyRef: {} } as never;
  const { ui, notify } = fakeUi();
  const exit = mock.fn((_code: number) => undefined as never);
  const result = await bootstrapOrFailLoud(ui, undefined, async () => ({ handle: bridge, agentTools: registerAgentTools(pi, bridge, { cwd: "/unused", extensionPath: "/unused/index.ts" }) }), exit);
  t.after(() => result?.agentTools.stopAll());
  assert.ok(result?.agentTools.rpcRegistry instanceof Map);
  for (const name of ["ws-agent-spawn", "ws-agent-send", "ws-agent-list", "ws-agent-stop", "ws-agent-transcript", "explore", "ws-fork"]) assert.equal(tools.includes(name), false);
  assert.equal(notify.mock.callCount(), 0);
  assert.equal(exit.mock.callCount(), 0);
});

class FakeStdout extends EventEmitter {
  writableLength = 0;
}

async function withStderrSpy<T>(fn: () => Promise<T>): Promise<{ result: T; stderrWrites: number }> {
  const original = process.stderr.write;
  let stderrWrites = 0;
  process.stderr.write = (() => { stderrWrites++; return true; }) as typeof process.stderr.write;
  try { return { result: await fn(), stderrWrites }; }
  finally { process.stderr.write = original; }
}

test("spawned child: exit waits for deferred notify output to drain", async () => {
  const stdout = new FakeStdout();
  const events: string[] = [];
  const ui = { notify: () => { queueMicrotask(() => { stdout.writableLength = 4096; events.push("notify-buffered"); }); } };
  const run = withStderrSpy(() => bootstrapOrFailLoud(ui, "worker", async () => { throw new Error("startup failure"); }, code => {
    assert.equal(stdout.writableLength, 0);
    events.push(`exit:${code}`);
    return undefined as never;
  }, () => drainStdoutBeforeExit(stdout, 1000)));
  await new Promise(resolve => setTimeout(resolve, 20));
  assert.deepEqual(events, ["notify-buffered"]);
  stdout.writableLength = 0;
  stdout.emit("drain");
  const { result, stderrWrites } = await run;
  assert.equal(result, undefined);
  assert.equal(stderrWrites, 0);
  assert.deepEqual(events, ["notify-buffered", "exit:1"]);
});

test("stdout drain is bounded when a broken pipe never drains", async () => {
  const stdout = new FakeStdout();
  stdout.writableLength = 1;
  await drainStdoutBeforeExit(stdout, 20);
  assert.equal(stdout.listenerCount("drain"), 0, "the bounded wait leaves no listener behind");
});

test("an already-drained stdout still yields for the queued RPC notification", async () => {
  const stdout = new FakeStdout();
  let yielded = false;
  queueMicrotask(() => { yielded = true; });
  await drainStdoutBeforeExit(stdout);
  assert.equal(yielded, true);
});

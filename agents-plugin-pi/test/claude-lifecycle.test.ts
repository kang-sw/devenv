import assert from "node:assert/strict";
import { EventEmitter, getEventListeners } from "node:events";
import { PassThrough } from "node:stream";
import { test } from "node:test";
import { createClaudeDelegateController } from "../src/claude-delegate.ts";
import { disposeClaudeChild, runClaudeItem } from "../src/claude-sdk.ts";

const item = { preset: "consult", request: "fixture" } as const;
const terminal = (result = "ok") => ({ type: "result", subtype: "success", is_error: false, result, usage: {}, modelUsage: {}, total_cost_usd: 0 });
const tick = () => new Promise<void>(resolve => setImmediate(resolve));
class Child extends EventEmitter {
  stdin = new PassThrough(); stdout = new PassThrough(); stderr = new PassThrough();
  exitCode: number | null = null; signalCode: string | null = null; killed = false;
  kills: string[] = [];
  kill(signal: string) { this.kills.push(signal); return false; }
  exit() { this.exitCode = 0; this.emit("exit", 0, null); }
}

test("R3-C2 throwing SDK close cannot bypass kill, quarantine, or finite queue settlement", { timeout: 2000 }, async () => {
  const children: Child[] = [];
  const controller = createClaudeDelegateController(() => "/tmp", {
    executable: process.execPath, cleanupMs: 24, timeoutMs: 10,
    spawnProcess: () => { const child = new Child(); children.push(child); return child as any; },
    loadSdk: async () => ({ query: ({ options }: any) => {
      options.spawnClaudeCodeProcess({});
      return { close() { throw new Error("private SDK exception"); }, async *[Symbol.asyncIterator]() { await new Promise(() => {}); } };
    } }) as any,
  });
  const results = await controller.execute(Array(5).fill(item));
  assert.equal(children.length, 3);
  assert.deepEqual(results.map(r => r.error?.code), ["cleanup_failed", "cleanup_failed", "cleanup_failed", "cancelled", "cancelled"]);
  for (const child of children) {
    assert.deepEqual(child.kills, ["SIGTERM", "SIGKILL"]);
    assert.equal(child.listenerCount("exit"), 0);
    for (const stream of [child.stdin, child.stdout, child.stderr]) assert.equal(stream.destroyed, true);
  }
  assert.equal((await controller.execute([item]))[0].error?.code, "cancelled");
  assert.equal(children.length, 3);
  await controller.shutdown();
});

test("R3-C2 exited children still dispose streams when SDK close throws", async () => {
  const child = new Child(); child.exit(); let closes = 0;
  assert.equal(await disposeClaudeChild({ close() { closes++; throw new Error("close failed"); } } as any, child as any, 20), true);
  assert.equal(closes, 1);
  for (const stream of [child.stdin, child.stdout, child.stderr]) assert.equal(stream.destroyed, true);
  assert.equal(child.listenerCount("exit"), 0);
});

test("execution detaches cancellation and successful exit listeners", async () => {
  const child = new Child(); const abortController = new AbortController();
  const done = runClaudeItem({ ...item, cwd: "/tmp", abortController }, {
    executable: process.execPath, cleanupMs: 100, spawnProcess: () => child as any,
    loadSdk: async () => ({ query: ({ options }: any) => { options.spawnClaudeCodeProcess({}); return { close() { setImmediate(() => child.exit()); }, async *[Symbol.asyncIterator]() { yield terminal(); } }; } }) as any,
  });
  assert.equal((await done).output, "ok");
  assert.equal(getEventListeners(abortController.signal, "abort").length, 0);
  assert.equal(child.listenerCount("exit"), 0);
});

function deferred<T>() {
  let resolve!: (value: T) => void; let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
function controlledSdk() {
  const starts: { prompt: string; options: any; gate: ReturnType<typeof deferred<void>>; child: Child }[] = [];
  const deps = {
    executable: process.execPath, cleanupMs: 100, timeoutMs: 500,
    spawnProcess: () => new Child() as any,
    loadSdk: async () => ({ query: ({ prompt, options }: any) => {
      const child = options.spawnClaudeCodeProcess({}); const gate = deferred<void>();
      starts.push({ prompt, options, gate, child });
      return { close() { child.exit(); }, async *[Symbol.asyncIterator]() { await gate.promise; yield terminal(prompt); } };
    } }) as any,
  };
  return { starts, deps };
}

test("overlapping invocations preserve FIFO admission, settled siblings, and independent cancellation", { timeout: 2000 }, async () => {
  const { starts, deps } = controlledSdk(); const controller = createClaudeDelegateController(() => "/tmp", deps);
  const cancel = new AbortController();
  const a = controller.execute(["a0", "a1", "a2", "a3", "a4", "a5"].map(request => ({ ...item, request })), cancel.signal);
  const b = controller.execute(["b0", "b1"].map(request => ({ ...item, request })));
  await tick(); assert.deepEqual(starts.map(s => s.prompt), ["a0", "a1", "a2"]);
  starts[0].gate.resolve(); await tick(); await tick();
  assert.deepEqual(starts.map(s => s.prompt), ["a0", "a1", "a2", "a3"]);
  cancel.abort(); const ar = await a; await tick();
  assert.equal(ar[0].output, "a0"); assert.deepEqual(ar.slice(1).map(r => r.error?.code), Array(5).fill("cancelled"));
  assert.deepEqual(starts.slice(4).map(s => s.prompt), ["b0", "b1"]);
  for (const start of starts.slice(4)) { assert.equal(start.options.abortController.signal.aborted, false); start.gate.resolve(); }
  assert.deepEqual((await b).map(r => r.output), ["b0", "b1"]);
  assert.equal(getEventListeners(cancel.signal, "abort").length, 0);
  for (const start of starts) assert.equal(start.child.listenerCount("exit"), 0);
  await controller.shutdown();
});

test("pre-abort and immediate post-admission abort launch no SDK query", async () => {
  const { starts, deps } = controlledSdk(); const controller = createClaudeDelegateController(() => "/tmp", deps);
  const early = new AbortController(); early.abort();
  assert.equal((await controller.execute([item], early.signal))[0].error?.code, "cancelled");
  const immediate = new AbortController(); const pending = controller.execute([item], immediate.signal); immediate.abort();
  assert.equal((await pending)[0].error?.code, "cancelled"); assert.equal(starts.length, 0);
});

test("queued admissions detach their abort listener after normal acquisition and shutdown", async () => {
  const { starts, deps } = controlledSdk(); const controller = createClaudeDelegateController(() => "/tmp", deps);
  const signal = new AbortController();
  const pending = controller.execute(Array(4).fill(item), signal.signal); await tick();
  starts[0].gate.resolve(); await tick(); await tick();
  for (const start of starts) start.gate.resolve(); await pending;
  assert.equal(getEventListeners(signal.signal, "abort").length, 0);
  const queued = controller.execute(Array(5).fill(item), signal.signal); await tick();
  await controller.shutdown();
  assert.deepEqual((await queued).map(r => r.error?.code), Array(5).fill("cancelled"));
  assert.equal(getEventListeners(signal.signal, "abort").length, 0);
});

for (const mode of ["never", "late-resolve", "late-reject", "failure"] as const) {
  test(`lazy SDK ${mode} is bounded and cannot start a query after settlement`, { timeout: 1000 }, async () => {
    let queries = 0; const lazy = deferred<any>();
    const controller = createClaudeDelegateController(() => "/tmp", { executable: process.execPath, timeoutMs: 10, cleanupMs: 10, loadSdk: () => mode === "failure" ? Promise.reject(new Error("SECRET")) : lazy.promise });
    const result = await controller.execute([item]);
    assert.equal(result[0].error?.code, mode === "failure" ? "sdk_error" : "timeout");
    const snapshot = JSON.stringify(result);
    if (mode === "late-resolve") lazy.resolve({ query() { queries++; throw new Error("should not query"); } });
    if (mode === "late-reject") lazy.reject(new Error("SECRET late"));
    await tick(); await tick(); assert.equal(queries, 0); assert.equal(JSON.stringify(result), snapshot);
    await controller.shutdown();
  });
}

for (const mode of ["late-result", "late-rejection", "abort-result"] as const) {
  test(`iterator ${mode} cannot mutate settled output or leak rejection`, { timeout: 1000 }, async () => {
    const gate = deferred<any>(); let spawn: any; let spawns = 0; let closeCount = 0;
    const cancel = new AbortController();
    const controller = createClaudeDelegateController(() => "/tmp", { executable: process.execPath, timeoutMs: 10, cleanupMs: 10,
      spawnProcess: () => { spawns++; return new Child() as any; },
      loadSdk: async () => ({ query: ({ options }: any) => { spawn = options.spawnClaudeCodeProcess; return { close() { closeCount++; }, async *[Symbol.asyncIterator]() { yield await gate.promise; } }; } }) as any,
    });
    const pending = controller.execute([item], cancel.signal); await tick();
    if (mode === "abort-result") { gate.resolve(terminal("too late")); cancel.abort(); }
    const result = await pending; const snapshot = JSON.stringify(result);
    assert.equal(result[0].error?.code, mode === "abort-result" ? "cancelled" : "timeout");
    if (mode === "late-result") gate.resolve(terminal("too late"));
    if (mode === "late-rejection") gate.reject(new Error("late private error"));
    assert.throws(() => spawn({}), { code: "cancelled" });
    await tick(); await tick();
    assert.equal(JSON.stringify(result), snapshot); assert.equal(closeCount, 1); assert.equal(spawns, 0);
  });
}

test("no permit is reused before exit and shutdown awaits owned cleanup", { timeout: 2000 }, async () => {
  const children: Child[] = []; let closeCount = 0;
  const controller = createClaudeDelegateController(() => "/tmp", { executable: process.execPath, cleanupMs: 500,
    spawnProcess: () => { const child = new Child(); children.push(child); return child as any; },
    loadSdk: async () => ({ query: ({ options }: any) => { options.spawnClaudeCodeProcess({}); return { close() { closeCount++; }, async *[Symbol.asyncIterator]() { yield terminal(); } }; } }) as any,
  });
  const pending = controller.execute(Array(4).fill(item)); await tick(); assert.equal(closeCount, 3); assert.equal(children.length, 3);
  let stopped = false; const stopping = controller.shutdown().then(() => { stopped = true; });
  await tick(); assert.equal(stopped, false); assert.equal(children.length, 3);
  children.forEach(child => child.exit()); await stopping;
  assert.equal(stopped, true); assert.equal((await pending)[3].error?.code, "cancelled");
  for (const child of children) { assert.equal(child.listenerCount("exit"), 0); assert.equal(child.stdout.closed, true); }
  await assert.rejects(() => controller.execute([item]), /shutting down/);
});

test("real local child ignores TERM, receives owned KILL, and closes all pipes despite throwing SDK close", { timeout: 3000 }, async () => {
  const abortController = new AbortController(); let child: any; const kills: string[] = [];
  const result = await runClaudeItem({ ...item, cwd: process.cwd(), abortController }, {
    executable: process.execPath, cleanupMs: 200,
    loadSdk: async () => ({ query: ({ options }: any) => {
      child = options.spawnClaudeCodeProcess({ command: process.execPath, args: ["-e", "process.on('SIGTERM',()=>{}); process.stdout.write('ready'); setInterval(()=>{},1000)"], cwd: process.cwd(), env: options.env, signal: new AbortController().signal });
      const kill = child.kill.bind(child); child.kill = (signal: string) => { kills.push(signal); return kill(signal); };
      const ready = new Promise<void>(resolve => child.stdout.once("data", () => resolve()));
      return { close() { throw new Error("private close failure"); }, async *[Symbol.asyncIterator]() { await ready; yield terminal(); } };
    } }) as any,
  });
  assert.equal(result.output, "ok"); assert.deepEqual(kills, ["SIGTERM", "SIGKILL"]);
  assert.equal(child.signalCode, "SIGKILL"); assert.equal(child.listenerCount("exit"), 0); assert.equal(child.listenerCount("error"), 0);
  for (const stream of [child.stdin, child.stdout, child.stderr]) { assert.equal(stream.destroyed, true); assert.equal(stream.closed, true); }
  assert.equal(getEventListeners(abortController.signal, "abort").length, 0);
});

test("real local spawn forwards SDK signal and observes owned signal termination", { timeout: 3000 }, async () => {
  const forwarded = new AbortController(); let child: any;
  await assert.rejects(() => runClaudeItem({ ...item, cwd: process.cwd(), abortController: new AbortController() }, {
    executable: process.execPath, cleanupMs: 100,
    loadSdk: async () => ({ query: ({ options }: any) => {
      child = options.spawnClaudeCodeProcess({ command: process.execPath, args: ["-e", "process.stdout.write('ready'); setInterval(()=>{},1000)"], cwd: process.cwd(), env: options.env, signal: forwarded.signal });
      const ended = new Promise<void>(resolve => child.once("exit", () => resolve()));
      child.stdout.once("data", () => forwarded.abort());
      return { close() {}, async *[Symbol.asyncIterator]() { await ended; throw new Error("fixture process ended"); } };
    } }) as any,
  }), { code: "sdk_error" });
  assert.equal(child.signalCode, "SIGTERM"); assert.equal(child.listenerCount("error"), 0); assert.equal(child.listenerCount("exit"), 0);
  assert.equal(getEventListeners(forwarded.signal, "abort").length, 0);
  for (const stream of [child.stdin, child.stdout, child.stderr]) assert.equal(stream.closed, true);
});

test("real asynchronous ENOENT spawn failure settles without waiting for an iterator result", { timeout: 3000 }, async () => {
  let child: any;
  await assert.rejects(() => runClaudeItem({ ...item, cwd: process.cwd(), abortController: new AbortController() }, {
    executable: process.execPath, cleanupMs: 100,
    loadSdk: async () => ({ query: ({ options }: any) => {
      child = options.spawnClaudeCodeProcess({ command: "/definitely-missing-ws-fixture", args: [], env: options.env, signal: new AbortController().signal });
      return { close() {}, async *[Symbol.asyncIterator]() { await new Promise(() => {}); } };
    } }) as any,
  }), { code: "sdk_error" });
  assert.notEqual(child.exitCode, null); assert.equal(child.listenerCount("error"), 0); assert.equal(child.listenerCount("exit"), 0);
  for (const stream of [child.stdin, child.stdout, child.stderr]) assert.equal(stream.closed, true);
});

test("throwing close after terminal success still quarantines an unkillable child", { timeout: 2000 }, async () => {
  const children: Child[] = [];
  const controller = createClaudeDelegateController(() => "/tmp", { executable: process.execPath, cleanupMs: 30,
    spawnProcess: () => { const child = new Child(); children.push(child); return child as any; },
    loadSdk: async () => ({ query: ({ options }: any) => { options.spawnClaudeCodeProcess({}); return { close() { throw new Error("failure"); }, async *[Symbol.asyncIterator]() { yield terminal(); } }; } }) as any,
  });
  const result = await controller.execute(Array(5).fill(item));
  assert.equal(children.length, 3);
  assert.deepEqual(result.map(r => r.error?.code), ["cleanup_failed", "cleanup_failed", "cleanup_failed", "cancelled", "cancelled"]);
  children.forEach(child => assert.deepEqual(child.kills, ["SIGTERM", "SIGKILL"]));
});

test("execution and cleanup timers are removed after normal completion", async t => {
  const pending = new Set<any>(); const originalSet = globalThis.setTimeout; const originalClear = globalThis.clearTimeout;
  t.mock.method(globalThis, "setTimeout", (callback: (...args: any[]) => void, ms: number, ...args: any[]) => {
    const handle = originalSet(() => { pending.delete(handle); callback(...args); }, ms); pending.add(handle); return handle;
  });
  t.mock.method(globalThis, "clearTimeout", (handle: any) => { pending.delete(handle); return originalClear(handle); });
  const { starts, deps } = controlledSdk(); const controller = createClaudeDelegateController(() => "/tmp", deps);
  const result = controller.execute([item]); await tick(); assert.ok(pending.size > 0);
  starts[0].gate.resolve(); await result; assert.equal(pending.size, 0);
  await controller.shutdown(); assert.equal(pending.size, 0);
});

test("owned stream errors settle the consumer race and detach their handler after disposal", async () => {
  const child = new Child();
  const pending = runClaudeItem({ ...item, cwd: "/tmp", abortController: new AbortController() }, {
    executable: process.execPath, cleanupMs: 100, spawnProcess: () => child as any,
    loadSdk: async () => ({ query: ({ options }: any) => { options.spawnClaudeCodeProcess({}); return { close() { child.exit(); }, async *[Symbol.asyncIterator]() { await new Promise(() => {}); } }; } }) as any,
  });
  await tick(); child.stdout.emit("error", new Error("private stream error"));
  await assert.rejects(() => pending, { code: "sdk_error", message: "Claude request failed." });
  assert.equal(child.stdout.listenerCount("error"), 0); assert.equal(child.stdout.closed, true);
});

test("exit without confirmable stream closure is cleanup failure", async () => {
  const child = new Child(); child.exit();
  // A transport that never completes stream destruction must not report cleanup success.
  child.stdout.destroy = () => child.stdout;
  assert.equal(await disposeClaudeChild({ close() {} } as any, child as any, 20), false);
  assert.equal(child.stdout.listenerCount("close"), 0); assert.equal(child.stdout.listenerCount("error"), 0);
  PassThrough.prototype.destroy.call(child.stdout);
});

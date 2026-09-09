import assert from "node:assert/strict";
import { test } from "node:test";
import { EventEmitter, getEventListeners } from "node:events";
import { PassThrough } from "node:stream";
import { readFileSync } from "node:fs";
import { registerClaudeDelegateSession, addClaudeDelegateIfLead } from "../src/claude-delegate.ts";
import { createToolPreviewTuiRef } from "../src/tool-result-render.ts";
const tick = () => new Promise<void>(resolve => setImmediate(resolve));

test("production session seam registers once, preserves roles/fork capture, forwards signal and awaits replacement/shutdown", { timeout: 3000 }, async () => {
  const oldRole = process.env.WS_PI_SPAWN_ROLE; const oldExplore = process.env.WS_PI_EXPLORE_MODE;
  delete process.env.WS_PI_SPAWN_ROLE; delete process.env.WS_PI_EXPLORE_MODE;
  let definition: any; let registrations = 0; let active = ["read"]; const requests: any[] = [];
  const pi = { registerTool(tool: any) { definition = tool; registrations++; }, getActiveTools() { return [...active]; } };
  const session = registerClaudeDelegateSession(pi as any, createToolPreviewTuiRef(), {
    executable: process.execPath, cleanupMs: 500,
    spawnProcess: () => Object.assign(new EventEmitter(), { stdin: new PassThrough(), stdout: new PassThrough(), stderr: new PassThrough(), exitCode: null, signalCode: null, killed: false, kill() { return false; } }) as any,
    loadSdk: async () => ({ query: ({ options }: any) => {
      const child = options.spawnClaudeCodeProcess({}); const request = { options, child, closes: 0 };
      requests.push(request);
      return { close() { request.closes++; }, async *[Symbol.asyncIterator]() { await new Promise(() => {}); } };
    } }) as any,
  });
  const invoke = (signal = new AbortController().signal) => definition.execute("id", { items: [{ preset: "audit", request: "x" }] }, signal);
  const exit = (request: any) => { request.child.exitCode = 0; request.child.emit("exit", 0, null); };
  try {
    assert.equal(registrations, 1); assert.equal(requests.length, 0);
    await assert.rejects(invoke, /unavailable/);
    await session.start("/first"); assert.equal(requests.length, 0);
    active = addClaudeDelegateIfLead(active, undefined); assert.deepEqual(active, ["read", "ws-claude"]);
    const signal = new AbortController(); const pending = invoke(signal.signal); await tick();
    assert.equal(requests[0].options.cwd, "/first"); signal.abort(); await tick();
    assert.equal(requests[0].options.abortController.signal.aborted, true); assert.equal(requests[0].closes, 1);
    let replaced = false; const replacing = session.start("/second").then(() => { replaced = true; });
    await tick(); assert.equal(replaced, false); await assert.rejects(invoke, /unavailable/);
    exit(requests[0]); await replacing;
    assert.equal(JSON.parse((await pending).content[0].text)[0].error.code, "cancelled");
    assert.equal(getEventListeners(signal.signal, "abort").length, 0);
    const second = invoke(); await tick(); assert.equal(requests[1].options.cwd, "/second");
    let stopped = false; const stopping = session.shutdown().then(() => { stopped = true; });
    await tick(); assert.equal(stopped, false); exit(requests[1]); await stopping; await second;
    for (const role of ["worker", "explore", "fork"]) {
      process.env.WS_PI_SPAWN_ROLE = role; active = ["read"];
      await session.start("/child"); await assert.rejects(invoke, /unavailable/);
      assert.deepEqual(addClaudeDelegateIfLead(active, role), ["read"]);
    }
    process.env.WS_PI_SPAWN_ROLE = "fork"; active = ["read", "ws-claude"];
    await session.start("/fork"); const fork = invoke(); await tick();
    assert.equal(requests.length, 3); assert.equal(requests[2].options.cwd, "/fork");
    assert.deepEqual(active, ["read", "ws-claude"]);
    const stopFork = session.shutdown(); await tick(); exit(requests[2]); await stopFork; await fork;
    assert.equal(registrations, 1);
    for (const request of requests) {
      assert.equal(request.closes, 1); assert.equal(request.child.listenerCount("exit"), 0);
      for (const stream of [request.child.stdin, request.child.stdout, request.child.stderr]) assert.equal(stream.closed, true);
    }
    // Guard the real extension call sites; the exercised seam owns their role/controller logic.
    const index = readFileSync(new URL("../src/index.ts", import.meta.url), "utf8");
    assert.match(index, /const claudeDelegateSession = registerClaudeDelegateSession\(pi, toolPreviewTuiRef\)/);
    assert.match(index, /await claudeDelegateSession.start\(ctx.cwd\)/);
    assert.match(index, /await claudeDelegateSession.shutdown\(\)/);
  } finally {
    await session.shutdown();
    if (oldRole === undefined) delete process.env.WS_PI_SPAWN_ROLE; else process.env.WS_PI_SPAWN_ROLE = oldRole;
    if (oldExplore === undefined) delete process.env.WS_PI_EXPLORE_MODE; else process.env.WS_PI_EXPLORE_MODE = oldExplore;
  }
});

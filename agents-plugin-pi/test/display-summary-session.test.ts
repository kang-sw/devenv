/**
 * Unit tests for display-summary-session.ts: the lead-TUI gate, the event
 * wiring, and that handlers never await a summary request.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { SessionEntry } from "@earendil-works/pi-coding-agent";
import { DISPLAY_SUMMARY_SIDECAR_SUFFIX, type SidecarIO } from "../src/display-summary-sidecar.ts";
import type { AssistantMessage } from "@earendil-works/pi-ai";
import { createDisplaySummaryStore, DISPLAY_SUMMARY_OUTPUT_TOOL } from "../src/display-summary.ts";
import { registerDisplaySummarySession, shouldRunDisplaySummary } from "../src/display-summary-session.ts";

type Handler = (event: any, ctx: any) => unknown;

function fakePi() {
  const handlers = new Map<string, Handler[]>();
  const injected: unknown[] = [];
  return {
    handlers,
    injected,
    sendMessage(message: unknown) { injected.push(message); },
    sendUserMessage(message: unknown) { injected.push(message); },
    on(name: string, handler: Handler) { handlers.set(name, [...(handlers.get(name) ?? []), handler]); },
    getActiveTools: () => ["edit", "write", "ws__tickets_query"],
    emit(name: string, event: unknown = {}, ctx: unknown = {}) {
      return (handlers.get(name) ?? []).map((handler) => handler(event, ctx));
    },
  };
}

const registry = {
  find: (provider: string, id: string) => ({ provider, id, api: "openai-responses", reasoning: false, contextWindow: 100_000, maxTokens: 1000 }),
};

function setup(env: NodeJS.ProcessEnv = {}, completion?: () => Promise<AssistantMessage>, sidecarIO?: SidecarIO, readConfig = async () => ({ model: "acme/mini" })) {
  const pi = fakePi();
  const store = createDisplaySummaryStore();
  store.toolNames.add("ws__tickets_query");
  const wrapped: Array<{ cwd: string; active: readonly string[] }> = [];
  let renderers = 0;
  let calls = 0;
  const session = registerDisplaySummarySession(pi as never, {
    store,
    readConfig,
    sidecarIO,
    registerBuiltinWrappers: (cwd, active) => { wrapped.push({ cwd, active }); },
    registerMessageRenderers: () => { renderers += 1; },
    createCompletion: () => async () => { calls += 1; return completion ? completion() : ({ role: "assistant", content: [{ type: "toolCall", id: "x", name: DISPLAY_SUMMARY_OUTPUT_TOOL, arguments: { items: [{ id: "t1", toolIntention: "i", toolResult: "r" }] } }], stopReason: "toolUse", usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } } as unknown as AssistantMessage); },
    env,
  });
  return { pi, store, session, wrapped, renderers: () => renderers, calls: () => calls };
}

function runTool(pi: ReturnType<typeof fakePi>, id: string) {
  pi.emit("tool_execution_start", { toolCallId: id, toolName: "ws__tickets_query", args: { q: 1 } });
  pi.emit("message_end", { message: { role: "toolResult", toolCallId: id, toolName: "ws__tickets_query", content: [{ type: "text", text: "ok" }], isError: false, timestamp: 0 } });
  pi.emit("tool_execution_end", { toolCallId: id, toolName: "ws__tickets_query", result: {}, isError: false });
}

describe("display summary session", () => {
  test("gate: lead TUI only", () => {
    assert.equal(shouldRunDisplaySummary("tui", undefined), true);
    assert.equal(shouldRunDisplaySummary("rpc", undefined), false);
    assert.equal(shouldRunDisplaySummary("tui", "fork"), false);
    assert.equal(shouldRunDisplaySummary("tui", "worker"), false);
    assert.equal(shouldRunDisplaySummary("tui", "explore"), false);
  });

  for (const role of ["fork", "worker", "explore"]) {
    test(`inactive in a ${role} session: no wrappers, no renderers, no requests`, async () => {
      const h = setup({ WS_PI_SPAWN_ROLE: role });
      h.pi.emit("session_start", {}, { mode: "tui", cwd: "/w", modelRegistry: registry });
      assert.equal(h.session.current(), undefined);
      runTool(h.pi, "c1");
      await Promise.all(h.pi.emit("turn_end"));
      assert.deepEqual(h.wrapped, []);
      assert.equal(h.renderers(), 0);
      assert.equal(h.calls(), 0);
    });
  }

  test("inactive outside the TUI", () => {
    const h = setup();
    h.pi.emit("session_start", {}, { mode: "print", cwd: "/w", modelRegistry: registry });
    assert.equal(h.session.current(), undefined);
    assert.deepEqual(h.wrapped, []);
  });

  test("lead TUI: wrappers and renderers registered once; turn_end flushes without being awaited", async () => {
    let release!: (message: AssistantMessage) => void;
    const h = setup({}, () => new Promise<AssistantMessage>((resolve) => { release = resolve; }));
    h.pi.emit("session_start", {}, { mode: "tui", cwd: "/w", modelRegistry: registry });
    h.pi.emit("session_start", {}, { mode: "tui", cwd: "/w", modelRegistry: registry });
    assert.deepEqual(h.wrapped, [{ cwd: "/w", active: ["edit", "write", "ws__tickets_query"] }]);
    assert.equal(h.renderers(), 1);
    runTool(h.pi, "c1");
    const results = h.pi.emit("turn_end");
    assert.deepEqual(results, [undefined], "the handler returns immediately");
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(h.calls(), 1);
    release({ role: "assistant", content: [{ type: "toolCall", id: "x", name: DISPLAY_SUMMARY_OUTPUT_TOOL, arguments: { items: [{ id: "t1", toolIntention: "i", toolResult: "r" }] } }], stopReason: "toolUse", usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } } as unknown as AssistantMessage);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(h.store.get("c1")?.toolResult, "r");
  });

  for (const [name, completion] of [
    ["a thrown request", () => Promise.reject(new Error("provider down"))],
    ["a provider error", () => Promise.resolve({ role: "assistant", content: [], stopReason: "error", errorMessage: "rate limited", usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } } as unknown as AssistantMessage)],
    ["a provider-reported overflow", () => Promise.resolve({ role: "assistant", content: [], stopReason: "error", errorMessage: "prompt is too long: context length exceeded", usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } } as unknown as AssistantMessage)],
  ] as const) {
    test(`${name} leaves the row raw and never injects a lead turn`, async () => {
      const h = setup({}, completion);
      h.pi.emit("session_start", {}, { mode: "tui", cwd: "/w", modelRegistry: registry });
      runTool(h.pi, "c1");
      assert.deepEqual(h.pi.emit("turn_end"), [undefined]);
      await new Promise((resolve) => setImmediate(resolve));
      await Promise.all(h.pi.emit("agent_end"));
      await new Promise((resolve) => setImmediate(resolve));
      assert.equal(h.calls(), 1);
      assert.equal(h.store.get("c1"), undefined, "the row stays raw");
      assert.deepEqual(h.pi.injected, [], "no message or user turn reaches the lead");
    });
  }

  test("history never requests summaries at initialization, reload or compaction; other rows still do", async () => {
    const h = setup();
    const ctx = { mode: "tui", cwd: "/w", modelRegistry: registry };
    for (const lifecycle of ["session_start", "session_start", "session_compact"]) {
      h.pi.emit(lifecycle, {}, ctx);
      h.store.set("history", { toolIntention: "cached intention", toolResult: "cached result" });
      h.session.enqueueStandalone("history", "ws-lead-compaction-history", "User: original conversation");
      await h.session.current()!.flush();
      await Promise.all(h.pi.emit("turn_end"));
      await Promise.all(h.pi.emit("agent_end"));
      assert.equal(h.calls(), 0, lifecycle);
      assert.equal(h.session.current()!.log.length, 0, "history never enters the summary transcript");
    }
    runTool(h.pi, "other-row");
    await h.session.current()!.flush();
    assert.equal(h.calls(), 1, "ordinary tools still summarize after compaction");
    assert.equal(h.store.get("other-row")?.toolResult, "r");
  });

  test("agent_end flushes; compaction resets the log; shutdown stops the summarizer", async () => {
    const h = setup();
    h.pi.emit("session_start", {}, { mode: "tui", cwd: "/w", modelRegistry: registry });
    runTool(h.pi, "c1");
    await Promise.all(h.pi.emit("agent_end"));
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(h.calls(), 1);
    assert.ok(h.session.current()!.log.length > 0);
    h.pi.emit("session_compact");
    assert.equal(h.session.current()!.log.length, 0);
    h.session.enqueueStandalone("entry-1", "ws-custom-entry", "User: hi");
    await Promise.all(h.pi.emit("turn_end"));
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(h.calls(), 2);
    h.pi.emit("session_shutdown");
    assert.equal(h.session.current(), undefined);
  });
});

function barrier<T = void>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
const acceptedAnswer = (result = "accepted"): AssistantMessage => ({ role: "assistant", content: [{ type: "toolCall", id: "output", name: DISPLAY_SUMMARY_OUTPUT_TOOL, arguments: { items: [{ id: "t1", toolIntention: "inspect", toolResult: result }] } }], stopReason: "toolUse" } as AssistantMessage);

async function savedSession(t: { after(fn: () => Promise<void>): void }) {
  const directory = await fs.mkdtemp(join(tmpdir(), "ws-summary-session-"));
  t.after(() => fs.rm(directory, { recursive: true, force: true }));
  async function conversation(name: string, ids: string[], parentSession?: string) {
    const path = join(directory, name + ".jsonl");
    const entries = ids.map((id): SessionEntry => ({ type: "message", id: `entry-${id}`, parentId: null, timestamp: "now", message: { role: "toolResult", toolCallId: id, toolName: "ws__tickets_query", content: [], isError: false, timestamp: 0 } }));
    await fs.writeFile(path, [JSON.stringify({ type: "session", id: name, ...(parentSession ? { parentSession } : {}) }), ...entries.map((e) => JSON.stringify(e))].join("\n") + "\n");
    return { path, entries, ctx: { mode: "tui", cwd: directory, modelRegistry: registry, sessionManager: { getSessionId: () => name, getSessionFile: () => path, getEntries: () => entries } } };
  }
  return { directory, conversation };
}
async function diskRows(path: string) {
  return (await fs.readFile(path + DISPLAY_SUMMARY_SIDECAR_SUFFIX, "utf8")).trim().split("\n").map((line) => JSON.parse(line));
}

for (const transition of ["shutdown", "replacement", "same-file-reload"] as const) {
  test(`${transition} drains held accepted batches in order, bound to the originating file`, async (t) => {
    const f = await savedSession(t);
    const old = await f.conversation("old", ["a", "b"]);
    const replacement = transition === "same-file-reload" ? old : await f.conversation("new", ["new-row"]);
    const entered = barrier();
    const release = barrier();
    let held = false;
    const io: SidecarIO = { ...fs, open: async (...args: Parameters<typeof fs.open>) => {
      if (args[0] === old.path + DISPLAY_SUMMARY_SIDECAR_SUFFIX && !held) { held = true; entered.resolve(); await release.promise; }
      return fs.open(...args);
    } };
    const h = setup({}, async () => acceptedAnswer(), io);
    await Promise.all(h.pi.emit("session_start", {}, old.ctx));
    runTool(h.pi, "a");
    await h.session.current()!.flush();
    await entered.promise;
    runTool(h.pi, "b");
    await h.session.current()!.flush();
    let settled = false;
    const ending = Promise.all(transition === "shutdown" ? h.pi.emit("session_shutdown") : h.pi.emit("session_start", {}, replacement.ctx)).then(() => { settled = true; });
    assert.equal(settled, false, "accepted disk work is part of lifecycle completion");
    release.resolve();
    await ending;
    assert.deepEqual((await diskRows(old.path)).map((r) => [r.sessionId, r.id]), [["old", "a"], ["old", "b"]]);
    if (transition === "same-file-reload") assert.equal(h.store.get("a")?.toolResult, "accepted", "reload reads after outgoing writes");
    if (transition === "replacement") {
      assert.equal(h.store.get("a"), undefined, "old replay never reaches replacement store");
      runTool(h.pi, "new-row");
      await h.session.current()!.flush();
      await Promise.all(h.pi.emit("session_shutdown"));
      assert.deepEqual((await diskRows(replacement.path)).map((r) => [r.sessionId, r.id]), [["new", "new-row"]]);
    }
  });
}

for (const fail of [false, true]) {
  test(`native shutdown inherits accepted values at cutover with held append${fail ? " failure" : ""}`, async (t) => {
    const f = await savedSession(t);
    const parent = await f.conversation("parent", ["a", "b"]);
    const child = await f.conversation("child", ["a", "child-only"], parent.path);
    const entered = barrier();
    const release = barrier();
    let held = false;
    const io: SidecarIO = { ...fs, open: async (...args: Parameters<typeof fs.open>) => {
      if (args[0] === parent.path + DISPLAY_SUMMARY_SIDECAR_SUFFIX) {
        if (!held) { held = true; entered.resolve(); await release.promise; }
        if (fail) throw new Error("parent append failed");
      }
      return fs.open(...args);
    } };
    const unfinished = barrier<AssistantMessage>();
    let calls = 0;
    const started = barrier();
    const h = setup({}, async () => {
      calls += 1;
      if (calls === 1) return acceptedAnswer();
      started.resolve();
      return unfinished.promise;
    }, io);
    await Promise.all(h.pi.emit("session_start", {}, parent.ctx));
    runTool(h.pi, "a");
    await h.session.current()!.flush();
    await entered.promise;
    runTool(h.pi, "b");
    const old = h.session.current()!;
    const request = old.flush();
    await started.promise;
    const shutdown = Promise.all(h.pi.emit("session_shutdown", { reason: "fork", targetSessionFile: child.path }));
    assert.equal(h.session.current(), undefined);
    release.resolve();
    await shutdown; // Must finish without resolving the provider's second request.
    assert.deepEqual((await diskRows(child.path)).map((r) => [r.sessionId, r.id, r.summary.toolResult]), [["child", "a", "accepted"]]);
    unfinished.resolve(acceptedAnswer("too late"));
    await request;
    assert.equal(h.store.get("b"), undefined);
    const reopened = setup();
    await Promise.all(reopened.pi.emit("session_start", {}, child.ctx));
    assert.equal(reopened.store.get("a")?.toolResult, "accepted");
    await Promise.all(reopened.pi.emit("session_shutdown"));
  });
}

for (const role of ["fork", "worker", "explore"]) {
  test(`${role} leaves durable files and orphan candidates untouched`, async (t) => {
    const f = await savedSession(t);
    const s = await f.conversation("lead", ["a"]);
    const data = JSON.stringify({ version: 1, sessionId: "lead", id: "a", summary: { toolIntention: "i", toolResult: "r" } }) + "\n";
    const orphan = join(f.directory, "orphan.jsonl" + DISPLAY_SUMMARY_SIDECAR_SUFFIX);
    await fs.writeFile(s.path + DISPLAY_SUMMARY_SIDECAR_SUFFIX, data);
    await fs.writeFile(orphan, data);
    const h = setup({ WS_PI_SPAWN_ROLE: role });
    await Promise.all(h.pi.emit("session_start", {}, s.ctx));
    runTool(h.pi, "a");
    await Promise.all(h.pi.emit("session_shutdown", { reason: "fork", targetSessionFile: s.path }));
    assert.equal(h.store.get("a"), undefined);
    assert.equal(h.calls(), 0);
    assert.equal(await fs.readFile(orphan, "utf8"), data);
    assert.equal(await fs.readFile(s.path + DISPLAY_SUMMARY_SIDECAR_SUFFIX, "utf8"), data);
  });
}

test("compaction preserves accepted summaries on disk and in store, while resetting provider log", async (t) => {
  const f = await savedSession(t);
  const s = await f.conversation("lead", ["a"]);
  const h = setup({}, async () => acceptedAnswer());
  await Promise.all(h.pi.emit("session_start", {}, s.ctx));
  runTool(h.pi, "a");
  await h.session.current()!.flush();
  assert.ok(h.session.current()!.log.length);
  h.pi.emit("session_compact");
  assert.equal(h.session.current()!.log.length, 0);
  assert.equal(h.store.get("a")?.toolResult, "accepted");
  await Promise.all(h.pi.emit("session_shutdown"));
  assert.equal((await diskRows(s.path))[0].summary.toolResult, "accepted");
});

for (const transition of ["shutdown", "replacement"] as const) {
  test(`paused configuration cannot start a provider or write after ${transition}`, async (t) => {
    const f = await savedSession(t);
    const s = await f.conversation("lead", ["a"]);
    const entered = barrier();
    const release = barrier<{ model: string }>();
    const h = setup({}, async () => acceptedAnswer(), undefined, async () => { entered.resolve(); return release.promise; });
    await Promise.all(h.pi.emit("session_start", {}, s.ctx));
    runTool(h.pi, "a");
    const pending = h.session.current()!.flush();
    await entered.promise;
    await Promise.all(transition === "shutdown" ? h.pi.emit("session_shutdown") : h.pi.emit("session_start", {}, (await f.conversation("new", [])).ctx));
    release.resolve({ model: "acme/mini" });
    await pending;
    assert.equal(h.calls(), 0);
    assert.equal(h.store.get("a"), undefined);
    await assert.rejects(fs.lstat(s.path + DISPLAY_SUMMARY_SIDECAR_SUFFIX), { code: "ENOENT" });
    await Promise.all(h.pi.emit("session_shutdown"));
  });
}

test("first save assigns a conversation path and backfills values accepted while unsaved", async (t) => {
  const f = await savedSession(t);
  const saved = await f.conversation("lead", ["a", "b"]);
  await fs.unlink(saved.path);
  let path: string | undefined;
  const manager = { ...saved.ctx.sessionManager, getSessionFile: () => path };
  const h = setup({}, async () => acceptedAnswer());
  await Promise.all(h.pi.emit("session_start", {}, { ...saved.ctx, sessionManager: manager }));
  runTool(h.pi, "a");
  await h.session.current()!.flush();
  assert.equal(h.store.get("a")?.toolResult, "accepted");
  await assert.rejects(fs.lstat(saved.path + DISPLAY_SUMMARY_SIDECAR_SUFFIX), { code: "ENOENT" });
  await f.conversation("lead", ["a", "b"]);
  path = saved.path;
  h.pi.emit("message_end", { message: { role: "user", content: "saved", timestamp: 1 } });
  await Promise.all(h.pi.emit("session_shutdown"));
  assert.deepEqual((await diskRows(saved.path)).map((r) => [r.sessionId, r.id]), [["lead", "a"]]);
});

test("late provider response after replacement never repaints or writes either conversation", async (t) => {
  const f = await savedSession(t);
  const old = await f.conversation("old", ["a"]);
  const next = await f.conversation("new", ["b"]);
  const entered = barrier();
  const release = barrier<AssistantMessage>();
  const h = setup({}, async () => { entered.resolve(); return release.promise; });
  await Promise.all(h.pi.emit("session_start", {}, old.ctx));
  runTool(h.pi, "a");
  const pending = h.session.current()!.flush();
  await entered.promise;
  await Promise.all(h.pi.emit("session_start", {}, next.ctx));
  let renders = 0;
  h.store.requestRender = () => { renders += 1; };
  release.resolve(acceptedAnswer("late"));
  await pending;
  await Promise.all(h.pi.emit("session_shutdown"));
  assert.equal(h.store.get("a"), undefined);
  assert.equal(renders, 0);
  for (const s of [old, next]) await assert.rejects(fs.lstat(s.path + DISPLAY_SUMMARY_SIDECAR_SUFFIX), { code: "ENOENT" });
});

test("replacement deactivates a held old replay before it can populate the new store", async (t) => {
  const f = await savedSession(t);
  const old = await f.conversation("old", ["a"]);
  const next = await f.conversation("new", ["b"]);
  const data = JSON.stringify({ version: 1, sessionId: "old", id: "a", summary: { toolIntention: "i", toolResult: "old" } }) + "\n";
  await fs.writeFile(old.path + DISPLAY_SUMMARY_SIDECAR_SUFFIX, data);
  const entered = barrier();
  const release = barrier();
  let held = false;
  const io: SidecarIO = { ...fs, open: async (...args: Parameters<typeof fs.open>) => {
    if (args[0] === old.path + DISPLAY_SUMMARY_SIDECAR_SUFFIX && !held) { held = true; entered.resolve(); await release.promise; }
    return fs.open(...args);
  } };
  const h = setup({}, undefined, io);
  const opening = Promise.all(h.pi.emit("session_start", {}, old.ctx));
  await entered.promise;
  const replacing = Promise.all(h.pi.emit("session_start", {}, next.ctx));
  release.resolve();
  await Promise.all([opening, replacing]);
  assert.equal(h.store.get("a"), undefined);
  assert.equal(await fs.readFile(old.path + DISPLAY_SUMMARY_SIDECAR_SUFFIX, "utf8"), data);
  await Promise.all(h.pi.emit("session_shutdown"));
} );

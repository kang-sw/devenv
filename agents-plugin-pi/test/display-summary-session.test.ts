/**
 * Unit tests for display-summary-session.ts: the lead-TUI gate, the event
 * wiring, and that handlers never await a summary request.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
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

function setup(env: NodeJS.ProcessEnv = {}, completion?: () => Promise<AssistantMessage>) {
  const pi = fakePi();
  const store = createDisplaySummaryStore();
  store.toolNames.add("ws__tickets_query");
  const wrapped: Array<{ cwd: string; active: readonly string[] }> = [];
  let renderers = 0;
  let calls = 0;
  const session = registerDisplaySummarySession(pi as never, {
    store,
    readConfig: async () => ({ model: "acme/mini" }),
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
    h.session.enqueueStandalone("entry-1", "ws-lead-compaction-history", "User: hi");
    await Promise.all(h.pi.emit("turn_end"));
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(h.calls(), 2);
    h.pi.emit("session_shutdown");
    assert.equal(h.session.current(), undefined);
  });
});

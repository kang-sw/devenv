/**
 * Unit tests for display-summary.ts: the lead TUI display summarizer's
 * batching, append-only log, failure fallbacks, effort mapping, and the
 * provider-neutral call path. Every model call is an injected fake; no test
 * makes a live model call.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import type { Api, AssistantMessage, Context, Model, SimpleStreamOptions } from "@earendil-works/pi-ai";
import {
  DISPLAY_SUMMARY_CONFIG_KEYS,
  DISPLAY_SUMMARY_OUTPUT_TOOL,
  DISPLAY_SUMMARY_SYSTEM_PROMPT,
  createDisplaySummarizer,
  createDisplaySummaryStore,
  createModelResolver,
  createProviderCompletion,
  displaySummaryConfigFrom,
  parseSummaryResponse,
  resolveDisplaySummaryReasoning,
  summaryItemsForMessage,
  type DisplaySummaryConfig,
} from "../src/display-summary.ts";
import { SUMMARY_ID_KEY, summaryIdOf, withItemSummaryIds, withSummaryId } from "../src/summary-id.ts";
import { THREAD_SUMMARY_CUSTOM_TYPE } from "../src/ask.ts";
import { LEAD_COMPACT_CUSTOM_TYPE, LEAD_CONTEXT_MILESTONE_CUSTOM_TYPE } from "../src/lead-compaction.ts";
import { PUSH_FAMILIES } from "../src/spawner.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";
import { SUMMARIZED_MESSAGE_TYPES, PUSH_BATCH_TYPE } from "../src/display-summary.ts";

function model(overrides: Partial<Model<Api>> = {}): Model<Api> {
  return {
    id: "mini",
    name: "mini",
    api: "openai-responses",
    provider: "acme",
    baseUrl: "https://acme.invalid",
    reasoning: true,
    input: ["text"],
    cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
    contextWindow: 200_000,
    maxTokens: 8192,
    ...overrides,
  } as Model<Api>;
}

function answer(items: unknown[] | undefined, extra: Partial<AssistantMessage> = {}): AssistantMessage {
  return {
    role: "assistant",
    content: items === undefined ? [{ type: "text", text: "no tool" }] : [{ type: "toolCall", id: "call-1", name: DISPLAY_SUMMARY_OUTPUT_TOOL, arguments: { items } }],
    api: "openai-responses",
    provider: "acme",
    model: "mini",
    usage: { input: 10, output: 5, cacheRead: 0, cacheWrite: 0, totalTokens: 15, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
    stopReason: items === undefined ? "stop" : "toolUse",
    timestamp: Date.now(),
    ...extra,
  } as AssistantMessage;
}

interface Call { model: Model<Api>; context: Context; options: SimpleStreamOptions }

function harness(config: DisplaySummaryConfig = { model: "acme/mini" }, modelOverrides: Partial<Model<Api>> = {}) {
  const store = createDisplaySummaryStore();
  store.toolNames.add("ws__tickets_query");
  store.toolNames.add("edit");
  const calls: Call[] = [];
  const responses: Array<AssistantMessage | Error | Promise<AssistantMessage>> = [];
  let renders = 0;
  store.requestRender = () => { renders += 1; };
  const state = { config };
  const summarizer = createDisplaySummarizer({
    store,
    readConfig: async () => state.config,
    resolveModel: (spec) => spec === "acme/mini" ? model(modelOverrides) : undefined,
    complete: async (m, context, options) => {
      calls.push({ model: m, context: structuredClone({ ...context, tools: context.tools }), options });
      const next = responses.shift() ?? answer([]);
      if (next instanceof Error) throw next;
      return next;
    },
    sessionId: "summary-session",
  });
  return { store, calls, responses, summarizer, state, renders: () => renders };
}

function toolRow(h: ReturnType<typeof harness>, id: string, name = "ws__tickets_query", args: unknown = { query: id }) {
  h.summarizer.observeToolStart(id, name, args);
  h.summarizer.observeMessage({ role: "assistant", content: [{ type: "toolCall", id, name, arguments: args }], timestamp: 0 });
  h.summarizer.observeMessage({ role: "toolResult", toolCallId: id, toolName: name, content: [{ type: "text", text: `result of ${id}` }], isError: false, timestamp: 0 });
  h.summarizer.observeToolEnd(id, name);
}

function requestText(call: Call): string {
  const last = call.context.messages.at(-1)!;
  const content = last.content as Array<{ type: string; text?: string }>;
  return content.map((part) => part.text ?? "").join("");
}

describe("summary store lifetimes", () => {
  test("clear preserves mounted links and generation; retire detaches outgoing rows", () => {
    const store = createDisplaySummaryStore();
    assert.equal(store.enabled, true, "direct renderer tests start enabled");
    let old = 0;
    let incoming = 0;
    store.trackInvalidate("old", () => { old++; });
    store.set("old", { toolIntention: "i", toolResult: "r" });
    const generation = store.generation;
    store.clear();
    assert.equal(store.get("old"), undefined);
    assert.equal(store.generation, generation);
    store.notify(["old"]);
    assert.equal(old, 1);
    store.retire();
    assert.equal(store.generation, generation + 1);
    assert.equal(store.enabled, false);
    store.trackInvalidate("incoming", () => { incoming++; });
    store.clear();
    store.notify(["old", "incoming"]);
    assert.equal(old, 1);
    assert.equal(incoming, 1);
  });

  test("confirmed ownership notifies once and unsubscribe removes its listener", () => {
    const store = createDisplaySummaryStore();
    let calls = 0;
    const unsubscribe = store.onToolRegistered(() => { calls++; });
    store.confirmTool("ws__late");
    store.confirmTool("ws__late");
    assert.equal(calls, 1);
    unsubscribe();
    store.confirmTool("edit");
    assert.equal(calls, 1);
    assert.deepEqual([...store.toolNames], ["ws__late", "edit"]);
  });
});

describe("summary ids", () => {
  test("each stamp is a fresh id, and batch items get their own", () => {
    const a = withSummaryId({ agent_id: "x" });
    const b = withSummaryId({ agent_id: "x" });
    assert.equal(a.agent_id, "x");
    assert.ok(summaryIdOf(a));
    assert.notEqual(summaryIdOf(a), summaryIdOf(b));
    assert.ok(summaryIdOf(withSummaryId(undefined)));
    const items = withItemSummaryIds([{ customType: "ws-mailbox", details: { from: "p" } }, { customType: "ws-agent-report" }]);
    assert.equal(new Set(items.map((item) => summaryIdOf(item.details))).size, 2);
    assert.equal((items[0]!.details as Record<string, unknown>).from, "p");
  });

  test("the summarized message set matches the adapter's message types", () => {
    for (const family of PUSH_FAMILIES) assert.ok(SUMMARIZED_MESSAGE_TYPES.has(family), family);
    for (const type of [LEAD_COMPACT_CUSTOM_TYPE, LEAD_CONTEXT_MILESTONE_CUSTOM_TYPE, THREAD_SUMMARY_CUSTOM_TYPE]) assert.ok(SUMMARIZED_MESSAGE_TYPES.has(type), type);
    assert.equal(PUSH_BATCH_TYPE, PUSH_BATCH_CUSTOM_TYPE);
  });

  test("a batch contributes one row per item except goal control; an unstamped message none", () => {
    const items = withItemSummaryIds([
      { customType: "ws-agent-report", content: "[ws-agent-report] agent a\nreport: ok", display: true, state: "informational" },
      { customType: "ws-mailbox", content: "mail from p:\nhello", display: true, state: "informational" },
      { customType: "ws-thread-summary", content: "thread", display: true, state: "informational" },
      { customType: "ws-goal-control", content: "goal", display: true, state: "informational" },
    ]);
    const rows = summaryItemsForMessage({ customType: PUSH_BATCH_CUSTOM_TYPE, content: "x", details: { version: "1", items } });
    assert.deepEqual(rows.map((row) => row.kind === "message" ? row.label : row.kind), ["ws-agent-report", "ws-mailbox", "ws-thread-summary"]);
    assert.deepEqual(rows.map((row) => row.id), items.slice(0, 3).map((item) => summaryIdOf(item.details)));
    assert.deepEqual(summaryItemsForMessage({ customType: "ws-agent-report", content: "x", details: {} }), []);
    assert.deepEqual(summaryItemsForMessage({ customType: "ws-unrelated", content: "x", details: withSummaryId({}) }), []);
    const single = summaryItemsForMessage({ customType: "ws-lead-compact", content: "[system message from ws-pi-plugin]\nprepare", details: withSummaryId({ trigger: "hard" }) });
    assert.equal(single.length, 1);
    assert.equal(single[0]!.kind === "message" && single[0]!.text, "prepare");
  });
});

describe("configuration and effort", () => {
  test("reads model, effort and the unprefixed workflow.lang", () => {
    assert.deepEqual([...DISPLAY_SUMMARY_CONFIG_KEYS], ["pi.display_summary_model", "pi.display_summary_effort", "workflow.lang"]);
    assert.deepEqual(displaySummaryConfigFrom({ "pi.display_summary_model": " acme/mini ", "pi.display_summary_effort": "low", "workflow.lang": "Korean" }), { model: "acme/mini", effort: "low", lang: "Korean" });
    assert.deepEqual(displaySummaryConfigFrom({ "pi.display_summary_model": "", "workflow.lang": "" }), { model: undefined, effort: undefined, lang: undefined });
  });

  test("off sends no reasoning; others clamp; unknown or absent defaults to medium", () => {
    const m = model();
    assert.equal(resolveDisplaySummaryReasoning(m, "off"), undefined);
    assert.equal(resolveDisplaySummaryReasoning(m, undefined), "medium");
    assert.equal(resolveDisplaySummaryReasoning(m, "bogus"), "medium");
    assert.equal(resolveDisplaySummaryReasoning(m, "low"), "low");
    // xhigh/max need an explicit map entry; the clamp falls back to high.
    assert.equal(resolveDisplaySummaryReasoning(m, "max"), "high");
    // A model without reasoning clamps every level to off: no reasoning sent.
    assert.equal(resolveDisplaySummaryReasoning(model({ reasoning: false }), "high"), undefined);
  });

  test("model spec splits at the first slash", () => {
    const seen: string[] = [];
    const resolve = createModelResolver({ find: (provider, id) => { seen.push(`${provider}|${id}`); return model(); } });
    assert.ok(resolve("openrouter/meta/llama"));
    assert.equal(resolve("noslash"), undefined);
    assert.equal(resolve("trailing/"), undefined);
    assert.deepEqual(seen, ["openrouter|meta/llama"]);
  });
});

describe("provider-neutral call path", () => {
  test("passes model, Context and options unchanged to public streamSimple", async () => {
    const response = answer([]);
    const m = model();
    const context: Context = { systemPrompt: "summary", messages: [], tools: [] };
    const options: SimpleStreamOptions = { reasoning: "low", sessionId: "s", cacheRetention: "short", signal: new AbortController().signal };
    const complete = createProviderCompletion({
      find: () => undefined,
      streamSimple: (actualModel, actualContext, actualOptions) => {
        assert.equal(actualModel, m);
        assert.equal(actualContext, context);
        assert.equal(actualOptions, options);
        return { result: async () => response };
      },
    });
    assert.equal(await complete(m, context, options), response);
  });

  test("summary flush reaches public completion with prompt, output tool and reasoning, never direct provider dispatch", async () => {
    const calls: Call[] = [];
    const registry = {
      find: () => model(),
      getApiKeyAndHeaders: () => { assert.fail("manual auth must not run"); },
      getProvider: () => { assert.fail("direct provider dispatch must not run"); },
      streamSimple: (m: Model<Api>, context: Context, options?: SimpleStreamOptions) => {
        calls.push({ model: m, context, options: options! });
        return { result: async () => answer([{ id: "t1", toolIntention: "i", toolResult: "r" }]) };
      },
    };
    const store = createDisplaySummaryStore();
    store.toolNames.add("read");
    const summarizer = createDisplaySummarizer({
      store, sessionId: "public-summary", readConfig: async () => ({ model: "acme/mini", effort: "low" }),
      resolveModel: createModelResolver(registry), complete: createProviderCompletion(registry),
    });
    summarizer.observeToolStart("row", "read", {});
    summarizer.observeToolEnd("row", "read");
    await summarizer.flush();
    assert.equal(calls.length, 1);
    assert.equal(calls[0]!.context.systemPrompt, DISPLAY_SUMMARY_SYSTEM_PROMPT);
    assert.equal(calls[0]!.context.tools?.length, 1);
    assert.equal(calls[0]!.context.tools![0]!.name, DISPLAY_SUMMARY_OUTPUT_TOOL);
    assert.equal(calls[0]!.context.tools![0]!.parameters.type, "object");
    assert.equal(calls[0]!.context.messages.length, 1);
    assert.match(requestText(calls[0]!), /Rows:/);
    assert.equal(calls[0]!.options.reasoning, "low");
    assert.equal(calls[0]!.options.sessionId, "public-summary");
    assert.equal(calls[0]!.options.cacheRetention, "short");
    assert.equal(calls[0]!.options.signal?.aborted, false);
    assert.ok(store.get("row"));
  });

  test("public completion rejections propagate to the summarizer's failure boundary", async () => {
    const complete = createProviderCompletion({ find: () => undefined, streamSimple: () => ({ result: async () => { throw new Error("not configured"); } }) });
    await assert.rejects(complete(model(), { messages: [] }, {}), /not configured/);
  });
});

describe("summarizer", () => {
  test("feature off when the model key is empty: no request, queue dropped", async () => {
    const h = harness({});
    toolRow(h, "c1");
    await h.summarizer.flush();
    assert.equal(h.calls.length, 0);
    h.state.config = { model: "acme/mini" };
    await h.summarizer.flush();
    assert.equal(h.calls.length, 0, "rows queued while off stay raw");
  });

  test("an unknown model leaves rows raw", async () => {
    const h = harness({ model: "acme/none" });
    toolRow(h, "c1");
    await h.summarizer.flush();
    assert.equal(h.calls.length, 0);
    assert.equal(h.store.get("c1"), undefined);
  });

  test("only summary-aware tools are queued; an empty queue sends nothing", async () => {
    const h = harness();
    toolRow(h, "c1", "read");
    await h.summarizer.flush();
    assert.equal(h.calls.length, 0);
  });

  test("labels resolve to rows; unknown labels are ignored; missing labels stay raw", async () => {
    const h = harness();
    toolRow(h, "c1");
    toolRow(h, "c2", "edit", { path: "a.ts" });
    toolRow(h, "c3");
    h.responses.push(answer([
      { id: "t2", toolIntention: "edit a.ts", toolResult: "applied", optionalContext: "follows the query" },
      { id: "t1", toolIntention: "query tickets", toolResult: "3 found" },
      { id: "t9", toolIntention: "x", toolResult: "y" },
    ]));
    await h.summarizer.flush();
    assert.deepEqual(h.store.get("c1"), { toolIntention: "query tickets", toolResult: "3 found" });
    assert.deepEqual(h.store.get("c2"), { optionalContext: "follows the query", toolIntention: "edit a.ts", toolResult: "applied" });
    assert.equal(h.store.get("c3"), undefined);
    const text = requestText(h.calls[0]!);
    assert.match(text, /- t1: tool call `ws__tickets_query`/);
    assert.match(text, /- t2: tool call `edit` with arguments \{"path":"a.ts"\}/);
    assert.match(text, /\[Tool result\]: result of c1/);
  });

  test("message rows re-render: tracked tool rows invalidate, then one TUI render", async () => {
    const h = harness();
    let invalidated = 0;
    h.store.trackInvalidate("c1", () => { invalidated += 1; });
    toolRow(h, "c1");
    const details = withSummaryId({ agent_id: "a" });
    h.summarizer.observeMessage({ role: "custom", customType: "ws-agent-report", content: "[ws-agent-report] agent a\nreport: ok", display: true, details, timestamp: 0 });
    h.responses.push(answer([{ id: "t1", toolIntention: "i", toolResult: "r" }, { id: "t2", toolIntention: "agent a reported", toolResult: "ok" }]));
    await h.summarizer.flush();
    assert.equal(invalidated, 1);
    assert.equal(h.renders(), 1);
    assert.equal(h.store.get(summaryIdOf(details)!)?.toolResult, "ok");
  });

  test("a response without the output-tool call leaves its rows raw", async () => {
    const h = harness();
    toolRow(h, "c1");
    h.responses.push(answer(undefined));
    await h.summarizer.flush();
    assert.equal(h.store.get("c1"), undefined);
    assert.equal(h.renders(), 0);
  });

  test("one request in flight; rows arriving meanwhile wait for the next flush", async () => {
    const h = harness();
    let release!: (value: AssistantMessage) => void;
    h.responses.push(new Promise<AssistantMessage>((resolve) => { release = resolve; }));
    toolRow(h, "c1");
    const first = h.summarizer.flush();
    await new Promise((resolve) => setImmediate(resolve));
    toolRow(h, "c2");
    await h.summarizer.flush();
    assert.equal(h.calls.length, 1, "no second request while one is in flight");
    release(answer([{ id: "t1", toolIntention: "i1", toolResult: "r1" }]));
    await first;
    assert.equal(h.store.get("c2"), undefined);
    h.responses.push(answer([{ id: "t1", toolIntention: "i2", toolResult: "r2" }]));
    await h.summarizer.flush();
    assert.equal(h.calls.length, 2);
    assert.match(requestText(h.calls[1]!), /- t1: tool call `ws__tickets_query` with arguments \{"query":"c2"\}/);
    assert.equal(h.store.get("c2")?.toolResult, "r2");
  });

  test("append-only log: each request extends the previous one, stable session id and cache retention", async () => {
    const h = harness();
    toolRow(h, "c1");
    h.responses.push(answer([{ id: "t1", toolIntention: "i", toolResult: "r" }]));
    await h.summarizer.flush();
    toolRow(h, "c2");
    await h.summarizer.flush();
    toolRow(h, "c3");
    await h.summarizer.flush();
    assert.equal(h.calls.length, 3);
    for (let i = 1; i < h.calls.length; i += 1) {
      const previous = h.calls[i - 1]!.context.messages;
      const current = h.calls[i]!.context.messages;
      assert.deepEqual(current.slice(0, previous.length), previous);
      // The previous answer and an explicit tool result follow the previous request.
      assert.equal(current[previous.length]!.role, "assistant");
      assert.equal(current[previous.length + 1]!.role, "toolResult");
      assert.equal(h.calls[i]!.context.systemPrompt, h.calls[0]!.context.systemPrompt);
    }
    for (const call of h.calls) {
      assert.equal(call.options.sessionId, "summary-session");
      assert.equal(call.options.cacheRetention, "short");
      assert.notEqual(call.options.cacheRetention, "none");
      assert.equal(call.options.reasoning, "medium");
      assert.deepEqual(call.context.tools?.map((tool) => tool.name), [DISPLAY_SUMMARY_OUTPUT_TOOL]);
    }
  });

  test("effort reaches the request: off sends no reasoning, a level is clamped", async () => {
    const off = harness({ model: "acme/mini", effort: "off" });
    toolRow(off, "c1");
    await off.summarizer.flush();
    assert.equal("reasoning" in off.calls[0]!.options, false);
    const clamped = harness({ model: "acme/mini", effort: "high" }, { reasoning: false });
    toolRow(clamped, "c1");
    await clamped.summarizer.flush();
    assert.equal("reasoning" in clamped.calls[0]!.options, false);
  });

  test("language: workflow.lang names it; empty falls back to the user's language", async () => {
    const korean = harness({ model: "acme/mini", lang: "Korean" });
    toolRow(korean, "c1");
    await korean.summarizer.flush();
    assert.match(requestText(korean.calls[0]!), /Write every field in Korean\./);
    const unset = harness({ model: "acme/mini" });
    toolRow(unset, "c1");
    await unset.summarizer.flush();
    assert.match(requestText(unset.calls[0]!), /Write every field in the language the user writes in\./);
  });

  test("compaction resets the log", async () => {
    const h = harness();
    toolRow(h, "c1");
    await h.summarizer.flush();
    h.summarizer.reset();
    toolRow(h, "c2");
    await h.summarizer.flush();
    assert.equal(h.calls[1]!.context.messages.length, 1);
  });

  test("a failed request leaves rows raw, commits nothing to the log, and carries its conversation", async () => {
    const h = harness();
    toolRow(h, "c1");
    h.responses.push(new Error("provider down"));
    await h.summarizer.flush();
    assert.equal(h.store.get("c1"), undefined);
    assert.equal(h.summarizer.log.length, 0);
    toolRow(h, "c2");
    h.responses.push(answer([], { stopReason: "error", errorMessage: "rate limited" }));
    await h.summarizer.flush();
    assert.equal(h.summarizer.log.length, 0);
    toolRow(h, "c3");
    await h.summarizer.flush();
    const text = requestText(h.calls[2]!);
    assert.match(text, /result of c1/);
    assert.match(text, /result of c2/);
    assert.match(text, /result of c3/);
    assert.equal(h.calls[2]!.context.messages.length, 1);
  });

  test("overflow by estimate stops summaries until compaction", async () => {
    const h = harness({ model: "acme/mini" }, { contextWindow: 1500 });
    toolRow(h, "c1", "ws__tickets_query", { query: "x".repeat(8000) });
    await h.summarizer.flush();
    assert.equal(h.calls.length, 0);
    toolRow(h, "c2");
    await h.summarizer.flush();
    assert.equal(h.calls.length, 0, "stopped");
    h.summarizer.reset();
    toolRow(h, "c3", "ws__tickets_query", {});
    await h.summarizer.flush();
    assert.equal(h.calls.length, 1, "reset lifts the stop");
  });

  test("a provider-reported overflow stops summaries until compaction", async () => {
    const h = harness();
    toolRow(h, "c1");
    h.responses.push(answer([], { stopReason: "error", errorMessage: "prompt is too long: 300000 tokens > 200000 maximum" }));
    await h.summarizer.flush();
    toolRow(h, "c2");
    await h.summarizer.flush();
    assert.equal(h.calls.length, 1, "stopped after overflow");
    h.summarizer.reset();
    toolRow(h, "c3");
    await h.summarizer.flush();
    assert.equal(h.calls.length, 2, "reset lifts the stop");
  });

  test("an overflow from a request that started before compaction does not stop the fresh log", async () => {
    const h = harness();
    let release!: (value: AssistantMessage) => void;
    h.responses.push(new Promise<AssistantMessage>((resolve) => { release = resolve; }));
    toolRow(h, "c1");
    const first = h.summarizer.flush();
    await new Promise((resolve) => setImmediate(resolve));
    h.summarizer.reset();
    release(answer([], { stopReason: "error", errorMessage: "prompt is too long: 300000 tokens > 200000 maximum" }));
    await first;
    toolRow(h, "c2");
    h.responses.push(answer([{ id: "t1", toolIntention: "i2", toolResult: "r2" }]));
    await h.summarizer.flush();
    assert.equal(h.calls.length, 2, "the compaction already lifted the stop");
    assert.equal(h.store.get("c2")?.toolResult, "r2");
  });

  test("excluded history is never queued or included alongside another row", async () => {
    const h = harness();
    h.store.set("history", { toolIntention: "cached history", toolResult: "cached summary" });
    h.summarizer.enqueueStandalone("history", "ws-lead-compaction-history", "EXCLUDED ORIGINAL CONVERSATION");
    await h.summarizer.flush();
    assert.equal(h.calls.length, 0);
    assert.equal(h.summarizer.log.length, 0);
    toolRow(h, "other-row");
    h.responses.push(answer([{ id: "t1", toolIntention: "i", toolResult: "r" }]));
    await h.summarizer.flush();
    assert.equal(h.calls.length, 1);
    assert.doesNotMatch(requestText(h.calls[0]!), /ws-lead-compaction-history|EXCLUDED ORIGINAL CONVERSATION/);
    assert.equal(h.store.get("other-row")?.toolResult, "r");
    assert.equal(h.store.get("history")?.toolResult, "cached summary", "no need to delete old cache entries");
  });

  test("standalone rows list their full content", async () => {
    const h = harness();
    h.summarizer.enqueueStandalone("entry-1", "ws-custom-entry", "User: please fix the build\nAssistant: fixed");
    h.responses.push(answer([{ id: "t1", toolIntention: "earlier conversation", toolResult: "the build was fixed" }]));
    await h.summarizer.flush();
    assert.match(requestText(h.calls[0]!), /- t1: message `ws-custom-entry` \(not in the conversation above; its full content follows\): User: please fix the build\nAssistant: fixed/);
    assert.equal(h.store.get("entry-1")?.toolResult, "the build was fixed");
  });

  test("dispose drops queued rows and stops further requests", async () => {
    const h = harness();
    toolRow(h, "c1");
    h.summarizer.dispose();
    await h.summarizer.flush();
    assert.equal(h.calls.length, 0);
  });
});

function acceptanceBarrier<T = void>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

test("onAccepted synchronously hands off each parsed nonempty batch before repaint", async () => {
  const store = createDisplaySummaryStore();
  store.toolNames.add("read");
  const batches: Array<ReadonlyMap<string, unknown>> = [];
  const order: string[] = [];
  store.requestRender = () => { order.push("render"); assert.equal(batches.length, 1); };
  const responses = [answer([{ id: "t1", toolIntention: "i", toolResult: "r" }, { id: "unknown", toolIntention: "bad", toolResult: "bad" }]), answer([]), answer(undefined)];
  const s = createDisplaySummarizer({ store, sessionId: "provider-cache", readConfig: async () => ({ model: "acme/mini" }), resolveModel: () => model(), complete: async () => responses.shift()!,
    onAccepted(batch) { order.push("accept"); assert.deepEqual(store.get("row-0"), batch.get("row-0")); batches.push(new Map(batch)); } });
  for (let i = 0; i < 3; i += 1) {
    s.observeToolEnd(`row-${i}`, "read");
    await s.flush();
  }
  assert.deepEqual(order, ["accept", "render"]);
  assert.deepEqual([...batches[0]!], [["row-0", { toolIntention: "i", toolResult: "r" }]]);
});

for (const stage of ["configuration", "provider"] as const) {
  for (const lifecycle of ["dispose", "reset"] as const) {
    test(`${lifecycle} while ${stage} is paused preserves acceptance ownership and resets only the log`, async () => {
      const entered = acceptanceBarrier();
      const release = acceptanceBarrier();
      const store = createDisplaySummaryStore();
      store.toolNames.add("read");
      const batches: Array<ReadonlyMap<string, unknown>> = [];
      let calls = 0;
      const s = createDisplaySummarizer({ store, sessionId: "cache", resolveModel: () => model(),
        readConfig: async () => { if (stage === "configuration") { entered.resolve(); await release.promise; } return { model: "acme/mini" }; },
        complete: async () => { calls += 1; if (stage === "provider") { entered.resolve(); await release.promise; } return answer([{ id: "t1", toolIntention: "i", toolResult: "r" }]); },
        onAccepted: (batch) => { batches.push(new Map(batch)); },
      });
      store.set("already", { toolIntention: "old", toolResult: "accepted" });
      s.observeToolEnd("row", "read");
      const pending = s.flush();
      await entered.promise;
      if (lifecycle === "dispose") s.dispose(); else s.reset();
      release.resolve();
      await pending;
      assert.equal(store.get("already")?.toolResult, "accepted");
      if (lifecycle === "dispose") {
        assert.equal(batches.length, 0);
        assert.equal(store.get("row"), undefined);
        assert.equal(calls, stage === "provider" ? 1 : 0);
        assert.equal(s.log.length, 0);
      } else {
        assert.deepEqual([...batches[0]!.keys()], ["row"]);
        assert.equal(store.get("row")?.toolResult, "r");
        if (stage === "provider") assert.equal(s.log.length, 0, "old-generation completion must not restore precompaction log");
        s.observeToolEnd("fresh", "read");
        await s.flush();
        assert.equal(store.get("fresh")?.toolResult, "r");
        assert.equal(s.log.length, stage === "provider" ? 3 : 6, "a request started after reset belongs to the fresh log, even if its config lookup began earlier");
      }
    });
  }
}

describe("parseSummaryResponse", () => {
  test("drops items missing a required field", () => {
    const labels = new Map([["t1", "a"], ["t2", "b"]]);
    const parsed = parseSummaryResponse(answer([{ id: "t1", toolIntention: "x" }, { id: "t2", toolIntention: " y ", toolResult: " z ", optionalContext: " " }]), labels);
    assert.deepEqual([...parsed.entries()], [["b", { toolIntention: "y", toolResult: "z" }]]);
    assert.equal(SUMMARY_ID_KEY, "ws_summary_id");
  });
});

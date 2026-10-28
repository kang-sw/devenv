/**
 * 261028 Phase 3: unit tests for `batch-summary.ts`, the flag-gated, coalescing,
 * once-per-batch summarizer behind the expanded agent widget's finished rows.
 * The model call is an injected fake; the coalescing window is a few real
 * milliseconds, and every wait polls a condition rather than sleeping blind.
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import type { Api, AssistantMessage, Context, Model, SimpleStreamOptions } from "@earendil-works/pi-ai";
import {
  BATCH_SUMMARY_ENABLED_KEY,
  BATCH_SUMMARY_OUTPUT_TOOL,
  BATCH_SUMMARY_SYSTEM_PROMPT,
  batchSummaryConfigFrom,
  batchSummaryItem,
  batchSummaryKey,
  buildBatchSummaryRequest,
  createBatchSummarizer,
  parseBatchSummaryResponse,
  type BatchSummaryConfig,
  type BatchSummaryItem,
} from "../src/batch-summary.ts";
import { DISPLAY_SUMMARY_EFFORT_KEY, DISPLAY_SUMMARY_MODEL_KEY, WORKFLOW_LANG_KEY } from "../src/display-summary.ts";
import { batchView, createBatchBuffer, observeBatchEvent } from "../src/agent-batches.ts";

function model(): Model<Api> {
  return {
    id: "mini", name: "mini", api: "openai-responses", provider: "acme", baseUrl: "https://acme.invalid", reasoning: true,
    input: ["text"], cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }, contextWindow: 200_000, maxTokens: 8192,
  } as Model<Api>;
}

function reply(summaries: unknown, extra: Partial<AssistantMessage> = {}): AssistantMessage {
  return {
    role: "assistant",
    content: summaries === undefined ? [{ type: "text", text: "no tool" }] : [{ type: "toolCall", id: "call-1", name: BATCH_SUMMARY_OUTPUT_TOOL, arguments: { summaries } }],
    api: "openai-responses", provider: "acme", model: "mini",
    usage: { input: 1, output: 1, cacheRead: 0, cacheWrite: 0, totalTokens: 2, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
    stopReason: summaries === undefined ? "stop" : "toolUse",
    timestamp: 0,
    ...extra,
  } as AssistantMessage;
}

const item = (agent: string, seq: number, overrides: Partial<BatchSummaryItem> = {}): BatchSummaryItem => ({
  key: batchSummaryKey(agent, seq), agentName: agent, toolNames: ["read"], callCount: 1, ...overrides,
});

interface Call { model: Model<Api>; context: Context; options: SimpleStreamOptions; text: string }
type Next = AssistantMessage | Error | Promise<AssistantMessage>;

async function until(condition: () => boolean, what: string): Promise<void> {
  const deadline = Date.now() + 2_000;
  while (!condition()) {
    if (Date.now() > deadline) assert.fail(`timed out waiting for ${what}`);
    await new Promise((resolve) => setTimeout(resolve, 2));
  }
}
/** Lets any wrongly scheduled flush run before an absence is asserted. */
const quiet = () => new Promise((resolve) => setTimeout(resolve, 40));

function harness(config: BatchSummaryConfig = { enabled: true, model: "acme/mini" }, opts: { windowMs?: number; timeoutMs?: number; resolvable?: boolean } = {}) {
  const calls: Call[] = [];
  const responses: Next[] = [];
  const state = { config, reads: 0, summaries: 0, resolvable: opts.resolvable ?? true };
  const summarizer = createBatchSummarizer({
    readConfig: async () => { state.reads += 1; return state.config; },
    resolveModel: () => state.resolvable ? model() : undefined,
    complete: async (m, context, options) => {
      const last = context.messages.at(-1)!.content as Array<{ text?: string }>;
      const text = last[0].text ?? "";
      calls.push({ model: m, context, options, text });
      const next = responses.shift();
      if (next instanceof Error) throw next;
      if (next) return next;
      // Default: answer every label in the request.
      return reply([...text.matchAll(/^\[(b\d+)\]/gm)].map((match) => ({ label: match[1], summary: `sum ${match[1]}` })));
    },
    onSummaries: () => { state.summaries += 1; },
    windowMs: opts.windowMs ?? 5,
    ...(opts.timeoutMs !== undefined ? { timeoutMs: opts.timeoutMs } : {}),
  });
  return { summarizer, calls, responses, state };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => { resolve = r; });
  return { promise, resolve };
}

describe("gating", () => {
  test("flag off: zero complete calls even with a model configured and resolvable", async () => {
    const h = harness({ enabled: false, model: "acme/mini" });
    h.summarizer.request([item("a", 1), item("b", 1)]);
    await until(() => h.state.reads >= 1, "the config read");
    await quiet();
    assert.equal(h.calls.length, 0);
    assert.equal(h.summarizer.get(batchSummaryKey("a", 1)), undefined);
    assert.equal(h.state.summaries, 0);
  });

  test("flag on with no model, or an unresolvable model: zero complete calls", async () => {
    const none = harness({ enabled: true });
    none.summarizer.request([item("a", 1)]);
    await until(() => none.state.reads >= 1, "the config read");
    await quiet();
    assert.equal(none.calls.length, 0);

    const unresolvable = harness({ enabled: true, model: "acme/missing" }, { resolvable: false });
    unresolvable.summarizer.request([item("a", 1)]);
    await until(() => unresolvable.state.reads >= 1, "the config read");
    await quiet();
    assert.equal(unresolvable.calls.length, 0);
  });

  test("a batch seen while the flag was off is not summarized after the flag is raised; later batches are", async () => {
    const h = harness({ enabled: false, model: "acme/mini" });
    h.summarizer.request([item("a", 1)]);
    await until(() => h.state.reads >= 1, "the config read");
    await quiet();
    h.state.config = { enabled: true, model: "acme/mini" };
    h.summarizer.request([item("a", 1)]);
    await quiet();
    assert.equal(h.calls.length, 0, "the key stays marked: one config read per batch, not one per repaint");
    h.summarizer.request([item("a", 2)]);
    await until(() => h.calls.length === 1, "the call for the later batch");
    assert.match(h.calls[0].text, /agent "a"/);
    assert.equal(h.summarizer.get(batchSummaryKey("a", 1)), undefined);
    await until(() => h.summarizer.get(batchSummaryKey("a", 2)) !== undefined, "the later summary");
  });
});

describe("once per batch", () => {
  test("a success is stored, reported, and never requested again", async () => {
    const h = harness();
    h.summarizer.request([item("a", 1)]);
    await until(() => h.state.summaries === 1, "the summary");
    assert.equal(h.summarizer.get("a:1"), "sum b1");
    h.summarizer.request([item("a", 1)]);
    await quiet();
    assert.equal(h.calls.length, 1);
    assert.equal(h.state.summaries, 1);
  });

  for (const [name, failure] of [
    ["an error stop reason", () => reply([{ label: "b1", summary: "late" }], { stopReason: "error" })],
    ["an aborted stop reason", () => reply([{ label: "b1", summary: "late" }], { stopReason: "aborted" })],
    ["a response with no output tool call", () => reply(undefined)],
    ["a response whose labels all miss", () => reply([{ label: "zz", summary: "x" }])],
    ["a thrown completion", () => new Error("provider down")],
  ] as const) {
    test(`${name} leaves the fallback and is not retried`, async () => {
      const h = harness();
      h.responses.push(failure());
      h.summarizer.request([item("a", 1)]);
      await until(() => h.calls.length === 1, "the call");
      await quiet();
      assert.equal(h.summarizer.get("a:1"), undefined);
      assert.equal(h.state.summaries, 0);
      h.summarizer.request([item("a", 1)]);
      await quiet();
      assert.equal(h.calls.length, 1, "no second call for the same key");
    });
  }

  test("keys are agentId:seq, so descendant batches are told apart by seq", async () => {
    assert.equal(batchSummaryKey("agent-1", 7), "agent-1:7");
    const h = harness();
    h.summarizer.request([item("agent-1", 7), item("agent-1", 8), item("agent-2", 7)]);
    await until(() => h.state.summaries === 1, "the summary");
    assert.equal(h.calls.length, 1);
    assert.deepEqual(["agent-1:7", "agent-1:8", "agent-2:7"].map((key) => h.summarizer.get(key)), ["sum b1", "sum b2", "sum b3"]);
    h.summarizer.request([item("agent-1", 7), item("agent-1", 9)]);
    await until(() => h.calls.length === 2, "the call for the one new key");
    assert.equal((h.calls[1].text.match(/^\[b\d+\]/gm) ?? []).length, 1, "only agent-1:9 is new");
  });

  test("the epoch keeps a relaunched agent's batch 1 apart from its earlier launch's batch 1", () => {
    assert.equal(batchSummaryKey("agent-1", 1, "e1"), "agent-1:e1:1");
    assert.notEqual(batchSummaryKey("agent-1", 1, "e1"), batchSummaryKey("agent-1", 1, "e2"));
    assert.equal(batchSummaryKey("agent-1", 1), "agent-1:1", "a peer without an epoch keeps the plain key");
  });

  test("duplicate keys inside one request collapse to one entry", async () => {
    const h = harness();
    h.summarizer.request([item("a", 1), item("a", 1)]);
    await until(() => h.calls.length === 1, "the call");
    assert.equal((h.calls[0].text.match(/^\[b\d+\]/gm) ?? []).length, 1);
  });
});

describe("coalescing", () => {
  test("finished batches of several agents requested within one window share one call", async () => {
    const h = harness(undefined, { windowMs: 30 });
    h.summarizer.request([item("a", 1)]);
    h.summarizer.request([item("b", 4)]);
    h.summarizer.request([item("c", 2)]);
    await until(() => h.state.summaries === 1, "the summary");
    assert.equal(h.calls.length, 1);
    assert.match(h.calls[0].text, /\[b1\] agent "a"[\s\S]*\[b2\] agent "b"[\s\S]*\[b3\] agent "c"/);
    assert.equal(h.summarizer.get("b:4"), "sum b2");
  });

  test("while one request is in flight, new items wait and then go out in one follow-up call", async () => {
    const h = harness();
    const gate = deferred<AssistantMessage>();
    h.responses.push(gate.promise);
    h.summarizer.request([item("a", 1)]);
    await until(() => h.calls.length === 1, "the first call");
    h.summarizer.request([item("b", 1)]);
    await quiet();
    h.summarizer.request([item("c", 1)]);
    await quiet();
    assert.equal(h.calls.length, 1, "nothing goes out while the first is in flight");
    gate.resolve(reply([{ label: "b1", summary: "first" }]));
    await until(() => h.calls.length === 2, "the follow-up call");
    assert.match(h.calls[1].text, /\[b1\] agent "b"[\s\S]*\[b2\] agent "c"/, "the waiting items share one request, labelled afresh");
    await until(() => h.summarizer.get("c:1") !== undefined, "the follow-up summaries");
    assert.equal(h.summarizer.get("a:1"), "first");
    assert.equal(h.summarizer.get("b:1"), "sum b1");
    assert.equal(h.summarizer.get("c:1"), "sum b2");
    assert.equal(h.calls.length, 2);
  });
});

describe("call shape", () => {
  test("the request carries the system prompt, one output tool, and the configured reasoning", async () => {
    const h = harness({ enabled: true, model: "acme/mini", effort: "low" });
    h.summarizer.request([item("a", 1)]);
    await until(() => h.calls.length === 1, "the call");
    const call = h.calls[0];
    assert.equal(call.context.systemPrompt, [BATCH_SUMMARY_SYSTEM_PROMPT, "Write every summary in the language the batch content is mostly written in, else English."].join("\n"));
    assert.equal(call.text.split("\n")[0], "Write every summary in the language the batch content is mostly written in, else English.");
    assert.deepEqual(call.context.tools?.map((tool) => tool.name), [BATCH_SUMMARY_OUTPUT_TOOL]);
    assert.equal(call.options.reasoning, "low");
    assert.ok(call.options.signal instanceof AbortSignal);
    assert.equal(call.model.id, "mini");

    const off = harness({ enabled: true, model: "acme/mini", effort: "off" });
    off.summarizer.request([item("a", 1)]);
    await until(() => off.calls.length === 1, "the call");
    assert.equal("reasoning" in off.calls[0].options, false);
  });

  test("system and user prompts follow the effective language on each request", async () => {
    const h = harness(batchSummaryConfigFrom({
      [BATCH_SUMMARY_ENABLED_KEY]: true, [DISPLAY_SUMMARY_MODEL_KEY]: "acme/mini", [WORKFLOW_LANG_KEY]: " Korean ",
    }));
    h.summarizer.request([item("a", 1)]);
    await until(() => h.state.summaries === 1, "the Korean summary");
    assert.equal(h.calls[0].context.systemPrompt, [BATCH_SUMMARY_SYSTEM_PROMPT, "Write every summary in this language: Korean."].join("\n"));
    assert.equal(h.calls[0].text.split("\n")[0], "Write every summary in this language: Korean.");

    h.state.config = { ...h.state.config, lang: "French" };
    h.summarizer.request([item("a", 2)]);
    await until(() => h.state.summaries === 2, "the changed-language summary");
    assert.equal(h.calls[1].context.systemPrompt, [BATCH_SUMMARY_SYSTEM_PROMPT, "Write every summary in this language: French."].join("\n"));
    assert.equal(h.calls[1].text.split("\n")[0], "Write every summary in this language: French.");
    assert.ok(!h.calls[1].context.systemPrompt!.includes("Korean"));
  });

  test("a request that outlives timeoutMs is aborted and leaves the fallback", async () => {
    const calls: AbortSignal[] = [];
    const summarizer = createBatchSummarizer({
      readConfig: async () => ({ enabled: true, model: "acme/mini" }),
      resolveModel: () => model(),
      complete: (_m, _c, options) => new Promise<AssistantMessage>((_resolve, reject) => {
        calls.push(options.signal!);
        options.signal!.addEventListener("abort", () => reject(new Error("aborted")));
      }),
      onSummaries: () => assert.fail("no summary expected"),
      windowMs: 1,
      timeoutMs: 20,
    });
    summarizer.request([item("a", 1)]);
    await until(() => calls.length === 1 && calls[0].aborted, "the timeout abort");
    assert.equal(summarizer.get("a:1"), undefined);
  });

  test("a throwing onSummaries never fails the summarizer", async () => {
    const calls: number[] = [];
    const summarizer = createBatchSummarizer({
      readConfig: async () => ({ enabled: true, model: "acme/mini" }),
      resolveModel: () => model(),
      complete: async () => { calls.push(1); return reply([{ label: "b1", summary: "ok" }]); },
      onSummaries: () => { throw new Error("repaint failed"); },
      windowMs: 1,
    });
    summarizer.request([item("a", 1)]);
    await until(() => summarizer.get("a:1") !== undefined, "the stored summary");
    summarizer.request([item("a", 2)]);
    await until(() => calls.length === 2, "the next call still goes out");
  });
});

describe("item detail", () => {
  const buffer = createBatchBuffer();
  observeBatchEvent(buffer, { type: "message_end", message: { role: "assistant", content: [
    { type: "toolCall", id: "c1", name: "read", arguments: { path: "src/a.ts" } },
    { type: "toolCall", id: "c2", name: "bash", arguments: { command: "npm test" } },
  ] } }, 100);
  observeBatchEvent(buffer, { type: "tool_execution_start", toolCallId: "c1", toolName: "read" }, 101);
  observeBatchEvent(buffer, { type: "tool_execution_start", toolCallId: "c2", toolName: "bash" }, 102);
  observeBatchEvent(buffer, { type: "tool_execution_end", toolCallId: "c1", result: { content: [{ type: "text", text: "file body" }] } }, 110);
  observeBatchEvent(buffer, { type: "tool_execution_end", toolCallId: "c2", isError: true, result: { content: [{ type: "text", text: "exit 1" }] } }, 120);
  const view = batchView(buffer.batches[0]);

  test("a direct child's item carries each call's args and result previews", () => {
    const direct = batchSummaryItem("child", "alpha", view, buffer.batches[0].calls);
    assert.equal(direct.key, "child:1");
    assert.equal(direct.agentName, "alpha");
    assert.deepEqual(direct.toolNames, ["read", "bash"]);
    assert.equal(direct.callCount, 2);
    assert.deepEqual(direct.calls, [
      { toolName: "read", argsPreview: "path: src/a.ts", resultPreview: "file body", isError: false },
      { toolName: "bash", argsPreview: "command: npm test", resultPreview: "exit 1", isError: true },
    ]);
  });

  test("a relayed descendant's item carries tool names only: no calls property, and no call lines in the request", async () => {
    const relayed = batchSummaryItem("grand", "beta", view);
    assert.equal("calls" in relayed, false);
    assert.deepEqual(relayed.toolNames, ["read", "bash"]);
    const text = buildBatchSummaryRequest([{ label: "b1", item: relayed }], undefined);
    assert.match(text, /\[b1\] agent "beta", 2 tool calls: read, bash/);
    assert.ok(!text.includes("  - "), "no per-call lines");
    assert.ok(!text.includes("src/a.ts") && !text.includes("npm test") && !text.includes("file body"));

    const h = harness();
    h.summarizer.request([relayed, batchSummaryItem("child", "alpha", view, buffer.batches[0].calls)]);
    await until(() => h.calls.length === 1, "the call");
    const sent = h.calls[0].text;
    const [grandBlock, childBlock] = sent.split(/\n(?=\[b2\])/);
    assert.ok(!grandBlock.includes("src/a.ts"), "the descendant block holds no args");
    assert.match(childBlock, /- read\(path: src\/a\.ts\) -> file body/);
    assert.match(childBlock, /- bash\(command: npm test\) -> error: exit 1/);
  });
});

describe("buildBatchSummaryRequest", () => {
  test("the language line follows lang, with an English-or-content fallback when absent", () => {
    const rows = [{ label: "b1", item: item("a", 1) }];
    const withLang = buildBatchSummaryRequest(rows, "ko");
    assert.equal(withLang.split("\n")[0], "Write every summary in this language: ko.");
    const without = buildBatchSummaryRequest(rows, undefined);
    assert.equal(without.split("\n")[0], "Write every summary in the language the batch content is mostly written in, else English.");
    assert.ok(!without.includes("this language:"));
  });

  test("one labelled block per batch, singular for one call, and a result-less call has no arrow", () => {
    const text = buildBatchSummaryRequest([
      { label: "b1", item: item("a", 1, { toolNames: ["bash"], callCount: 1, calls: [{ toolName: "bash", argsPreview: "command: ls" }] }) },
      { label: "b2", item: item("b", 2, { toolNames: ["read", "grep"], callCount: 3 }) },
    ], "en");
    assert.equal(text, [
      "Write every summary in this language: en.",
      "",
      '[b1] agent "a", 1 tool call: bash',
      "  - bash(command: ls)",
      '[b2] agent "b", 3 tool calls: read, grep',
    ].join("\n"));
  });
});

describe("parseBatchSummaryResponse", () => {
  const labels = new Map([["b1", "a:1"], ["b2", "b:1"], ["b3", "c:1"]]);
  const parse = (summaries: unknown, extraBlocks: unknown[] = []) =>
    parseBatchSummaryResponse({ ...reply(summaries), content: [...extraBlocks, ...reply(summaries).content] } as AssistantMessage, labels);

  test("labels map to keys", () => {
    assert.deepEqual([...parse([{ label: "b2", summary: "two" }, { label: " b1 ", summary: "one" }])], [["b:1", "two"], ["a:1", "one"]]);
  });

  test("unknown labels, malformed entries, empty summaries and duplicates are skipped", () => {
    const parsed = parse([
      { label: "zz", summary: "unknown" },
      null, "text", 7,
      { label: 1, summary: "bad label" },
      { label: "b1", summary: 5 },
      { label: "b1" },
      { label: "b1", summary: "   " },
      { label: "b1", summary: "first" },
      { label: "b1", summary: "second" },
      { label: "b3", summary: "three" },
    ]);
    assert.deepEqual([...parsed], [["a:1", "first"], ["c:1", "three"]]);
  });

  test("a non-array summaries argument, other tools' calls, and text blocks yield nothing", () => {
    assert.equal(parse("nope").size, 0);
    assert.equal(parse(undefined).size, 0);
    const other = { type: "toolCall", id: "x", name: "something_else", arguments: { summaries: [{ label: "b1", summary: "wrong tool" }] } };
    assert.equal(parseBatchSummaryResponse({ ...reply(undefined), content: [other] } as AssistantMessage, labels).size, 0);
  });

  test("control characters are stripped and whitespace collapsed", () => {
    const parsed = parse([{ label: "b1", summary: "  read\u001b[31m the\n\n files\u0000\tnow\u0085 " }]);
    assert.equal(parsed.get("a:1"), "read [31m the files now");
    assert.ok(!/[\u0000-\u001f\u007f-\u009f]/.test(parsed.get("a:1")!));
  });

  test("the summary is capped at 160 characters with an ellipsis", () => {
    const exact = "x".repeat(160);
    assert.equal(parse([{ label: "b1", summary: exact }]).get("a:1"), exact);
    const long = parse([{ label: "b1", summary: "y".repeat(400) }]).get("a:1")!;
    assert.equal([...long].length, 160);
    assert.equal(long, "y".repeat(159) + "…");
  });
});

describe("batchSummaryConfigFrom", () => {
  test("only a literal true raises the flag", () => {
    for (const value of [false, "true", 1, "yes", null, undefined]) {
      assert.equal(batchSummaryConfigFrom({ [BATCH_SUMMARY_ENABLED_KEY]: value }).enabled, false, String(value));
    }
    assert.equal(batchSummaryConfigFrom({ [BATCH_SUMMARY_ENABLED_KEY]: true }).enabled, true);
    assert.equal(batchSummaryConfigFrom({}).enabled, false);
  });

  test("model, effort and language follow the display summarizer's parsing, and are omitted when blank", () => {
    assert.deepEqual(batchSummaryConfigFrom({
      [BATCH_SUMMARY_ENABLED_KEY]: true, [DISPLAY_SUMMARY_MODEL_KEY]: " acme/mini ", [DISPLAY_SUMMARY_EFFORT_KEY]: "low", [WORKFLOW_LANG_KEY]: "ko",
    }), { enabled: true, model: "acme/mini", effort: "low", lang: "ko" });
    assert.deepEqual(batchSummaryConfigFrom({ [BATCH_SUMMARY_ENABLED_KEY]: true, [DISPLAY_SUMMARY_MODEL_KEY]: "  ", [WORKFLOW_LANG_KEY]: 3 }), { enabled: true });
    assert.equal(BATCH_SUMMARY_ENABLED_KEY, "pi.agent_widget_batch_summary_enabled");
  });
});

describe("dispose", () => {
  test("before the window fires: no call is ever made and later requests are ignored", async () => {
    const h = harness(undefined, { windowMs: 30 });
    h.summarizer.request([item("a", 1)]);
    h.summarizer.dispose();
    await new Promise((resolve) => setTimeout(resolve, 80));
    assert.equal(h.calls.length, 0);
    assert.equal(h.state.reads, 0);
    h.summarizer.request([item("b", 1)]);
    await quiet();
    assert.equal(h.calls.length, 0);
    assert.equal(h.state.reads, 0);
  });

  test("during the config read: the call is never made", async () => {
    const gate = deferred<BatchSummaryConfig>();
    const calls: number[] = [];
    const summarizer = createBatchSummarizer({
      readConfig: () => gate.promise,
      resolveModel: () => model(),
      complete: async () => { calls.push(1); return reply([]); },
      onSummaries: () => {},
      windowMs: 1,
    });
    summarizer.request([item("a", 1)]);
    await quiet();
    summarizer.dispose();
    gate.resolve({ enabled: true, model: "acme/mini" });
    await quiet();
    assert.equal(calls.length, 0);
  });

  test("during a request: the signal aborts, the late answer is dropped, and nothing more flushes", async () => {
    const h = harness();
    const gate = deferred<AssistantMessage>();
    h.responses.push(gate.promise);
    h.summarizer.request([item("a", 1)]);
    await until(() => h.calls.length === 1, "the call");
    h.summarizer.request([item("b", 1)]);
    h.summarizer.dispose();
    assert.equal(h.calls[0].options.signal!.aborted, true);
    gate.resolve(reply([{ label: "b1", summary: "too late" }]));
    await quiet();
    assert.equal(h.summarizer.get("a:1"), undefined);
    assert.equal(h.state.summaries, 0);
    assert.equal(h.calls.length, 1, "the queued item is never flushed");
    h.summarizer.request([item("c", 1)]);
    await quiet();
    assert.equal(h.calls.length, 1);
  });
});

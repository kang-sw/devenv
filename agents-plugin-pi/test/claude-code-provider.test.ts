/**
 * Fake-SDK unit tests for src/claude-code-provider.ts
 * (260908-feat-ws-pi-claude-code-lead-provider, Phase 1). No claude binary:
 * the `ClaudeCodeSdk` seam is replaced by a scripted fake whose `query()`
 * records the SDK options, drains the streaming-input prompt into `inputs`,
 * and lets the test emit assistant/result frames and invoke the reflected
 * MCP tool handlers exactly as the claude process would.
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { z } from "zod";
import type { Api, AssistantMessage, AssistantMessageEvent, Context, Message, Model, SimpleStreamOptions } from "@earendil-works/pi-ai";
import {
  buildReplayPrompt, CLAUDE_CODE_MODELS, CLAUDE_CODE_REMEDY, createClaudeCodeProvider, effortForThinking, jsonSchemaToZodShape, registerClaudeCodeProvider,
  type ClaudeCodeSdk, type SdkMessage, type SdkToolHandler, type SdkToolResult, type SdkUserMessage,
} from "../src/claude-code-provider.ts";

type ToolDef = { name: string; description: string; shape: Record<string, unknown>; handler: SdkToolHandler };

class FakeQuery implements AsyncIterable<SdkMessage> {
  inputs: SdkUserMessage[] = [];
  interrupts = 0;
  closed = false;
  private queue: SdkMessage[] = [];
  private waiters: Array<(r: IteratorResult<SdkMessage>) => void> = [];
  private ended = false;
  private failure: Error | undefined;
  readonly options: Record<string, unknown>;
  constructor(prompt: AsyncIterable<SdkUserMessage>, options: Record<string, unknown>) {
    this.options = options;
    void (async () => { for await (const m of prompt) this.inputs.push(m); })();
  }
  get tools(): ToolDef[] { return ((this.options.mcpServers as { pi: { tools: ToolDef[] } }).pi.tools); }
  tool(name: string): ToolDef { const t = this.tools.find(t => t.name === name); assert.ok(t, `tool ${name} reflected`); return t; }
  invoke(name: string, args: Record<string, unknown> = {}, extra: unknown = {}): Promise<SdkToolResult> { return this.tool(name).handler(args, extra); }
  emit(message: SdkMessage): void {
    const waiter = this.waiters.shift();
    if (waiter) waiter({ value: message, done: false }); else this.queue.push(message);
  }
  assistant(blocks: unknown[], { message, ...extra }: Record<string, unknown> = {}): void {
    this.emit({ type: "assistant", message: { id: "msg_1", model: "claude-test", content: blocks as never, usage: { input_tokens: 2, output_tokens: 5, cache_read_input_tokens: 100, cache_creation_input_tokens: 7 }, ...message as object }, ...extra } as SdkMessage);
  }
  result(extra: Record<string, unknown> = {}): void { this.emit({ type: "result", subtype: "success", is_error: false, ...extra }); }
  end(): void { this.ended = true; for (const w of this.waiters.splice(0)) w({ value: undefined, done: true }); }
  fail(error: Error): void { this.failure = error; this.end(); }
  async interrupt(): Promise<unknown> { this.interrupts++; return undefined; }
  close(): void { this.closed = true; this.end(); }
  async *[Symbol.asyncIterator](): AsyncIterator<SdkMessage> {
    for (;;) {
      if (this.queue.length) { yield this.queue.shift()!; continue; }
      if (this.ended) { if (this.failure) throw this.failure; return; }
      const r = await new Promise<IteratorResult<SdkMessage>>(resolve => this.waiters.push(resolve));
      if (r.done) { if (this.failure) throw this.failure; return; }
      yield r.value;
    }
  }
}

function fakeSdk(overrides: Partial<ClaudeCodeSdk> = {}) {
  const queries: FakeQuery[] = [];
  const sdk: ClaudeCodeSdk = {
    query: ({ prompt, options }) => { const q = new FakeQuery(prompt, options); queries.push(q); return q; },
    createSdkMcpServer: (o) => ({ name: o.name, tools: o.tools, alwaysLoad: o.alwaysLoad }),
    tool: (name, description, shape, handler) => ({ name, description, shape, handler }),
    z,
    ...overrides,
  };
  return { sdk, queries };
}

const model: Model<Api> = { id: "sonnet", name: "sonnet", api: "anthropic-messages", provider: "claude-code", baseUrl: "http://x", reasoning: true, input: ["text"], cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }, contextWindow: 1000, maxTokens: 100 };
const user = (text: string): Message => ({ role: "user", content: text, timestamp: 1 });
const toolResult = (toolCallId: string, toolName: string, text: string, isError = false): Message => ({ role: "toolResult", toolCallId, toolName, content: [{ type: "text", text }], isError, timestamp: 2 });
const tools = [
  { name: "probe", description: "Probe tool", parameters: { type: "object", properties: { word: { type: "string", description: "A word" } }, required: ["word"] } },
  { name: "other", description: "Other tool", parameters: { type: "object", properties: {} } },
] as unknown as Context["tools"];
const baseContext = (messages: Message[], extra: Partial<Context> = {}): Context => ({ systemPrompt: "SYS", messages, tools, ...extra });

async function until(predicate: () => boolean, what = "condition"): Promise<void> {
  for (let i = 0; i < 200; i++) { if (predicate()) return; await new Promise(r => setTimeout(r, 1)); }
  throw new Error(`timed out waiting for ${what}`);
}

function harness(overrides: Partial<ClaudeCodeSdk> = {}) {
  const { sdk, queries } = fakeSdk(overrides);
  const logs: string[] = [];
  const provider = createClaudeCodeProvider({ sdk: async () => sdk, cwd: () => "/tmp/cwd", log: line => logs.push(line) });
  const events: AssistantMessageEvent[][] = [];
  async function call(context: Context, options: Partial<SimpleStreamOptions> = {}) {
    const stream = provider.streamSimple(model, context, options as SimpleStreamOptions);
    const list: AssistantMessageEvent[] = [];
    events.push(list);
    const done = (async () => { for await (const e of stream) list.push(e); return stream.result(); })();
    return { stream, events: list, done };
  }
  return { sdk, queries, logs, provider, call, events, latest: () => queries[queries.length - 1] };
}

describe("claude-code provider: pure helpers", () => {
  test("effort mapping for every Pi thinking level", () => {
    assert.deepEqual(
      [undefined, "off", "minimal", "low", "medium", "high", "xhigh", "max"].map(l => effortForThinking(l as never)),
      ["off", "off", "low", "low", "medium", "high", "xhigh", "max"],
    );
  });

  test("json schema to zod shape keeps nested values as any and marks optional fields", () => {
    const shape = jsonSchemaToZodShape({
      type: "object",
      properties: { s: { type: "string", description: "d" }, n: { type: "integer" }, b: { type: "boolean" }, o: { type: "object", properties: { x: { type: "number" } } }, a: { type: "array" }, e: { type: "string", enum: ["x", "y"] } },
      required: ["s", "o"],
    }, z) as Record<string, z.ZodTypeAny>;
    assert.deepEqual(Object.keys(shape).sort(), ["a", "b", "e", "n", "o", "s"]);
    assert.equal(z.object(shape).safeParse({ s: "ok", o: { x: 1, deep: [1] } }).success, true);
    assert.equal(z.object(shape).safeParse({ o: {} }).success, false, "required string missing");
    assert.equal(z.object(shape).safeParse({ s: "ok", o: {}, e: "z" }).success, false, "enum enforced");
    assert.equal(z.object(shape).safeParse({ s: "ok", o: {}, a: [1, { k: 2 }], n: 3 }).success, true);
    assert.equal(shape.s.description, "d");
    assert.deepEqual(jsonSchemaToZodShape(undefined, z), {});
  });

  test("replay prompt is deterministic and ordered, and a lone user message replays as itself", () => {
    const history: Message[] = [
      user("first"),
      { role: "assistant", content: [{ type: "thinking", thinking: "secret" }, { type: "text", text: "calling" }, { type: "toolCall", id: "t1", name: "probe", arguments: { word: "kiwi" } }], api: "anthropic-messages", provider: "claude-code", model: "sonnet", usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } }, stopReason: "toolUse", timestamp: 3 },
      toolResult("t1", "probe", "echo kiwi"),
      { role: "assistant", content: [{ type: "text", text: "done" }], api: "anthropic-messages", provider: "claude-code", model: "sonnet", usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } }, stopReason: "stop", timestamp: 4 },
      user("second"),
    ];
    const prompt = buildReplayPrompt(history);
    assert.equal(prompt, buildReplayPrompt(JSON.parse(JSON.stringify(history))), "byte-stable for identical histories");
    const order = ["[user]\nfirst", "[assistant]\ncalling\n[tool_call id=\"t1\" name=\"probe\"]\n{\"word\":\"kiwi\"}\n[/tool_call]", "[tool_result id=\"t1\" name=\"probe\" error=\"false\"]\necho kiwi\n[/tool_result]", "[assistant]\ndone", "</conversation_transcript>\n\nsecond"];
    let cursor = -1;
    for (const piece of order) { const at = prompt.indexOf(piece); assert.ok(at > cursor, `${piece} in order`); cursor = at; }
    assert.ok(!prompt.includes("secret"), "thinking is not replayed");
    assert.ok(prompt.endsWith("second"));
    assert.equal(buildReplayPrompt([user("only")]), "only");
    assert.ok(buildReplayPrompt(history.slice(0, 3)).endsWith("Continue from the end of the transcript above."));
  });

  test("model surface: three reasoning text models with zero cost", () => {
    assert.deepEqual(CLAUDE_CODE_MODELS.map(m => m.id), ["opus", "sonnet", "haiku"]);
    for (const m of CLAUDE_CODE_MODELS) { assert.equal(m.reasoning, true); assert.deepEqual(m.input, ["text"]); assert.deepEqual(m.cost, { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }); }
  });
});

describe("claude-code provider: turns", () => {
  test("text-only turn starts one process lazily, maps the text block, sums usage, and calls onPayload before onResponse", async () => {
    const h = harness();
    assert.equal(h.provider.hasProcess(), false);
    const order: string[] = [];
    const turn = await h.call(baseContext([user("hello")]), {
      reasoning: "high",
      onPayload: (payload) => { order.push("payload"); assert.equal(h.queries.length, 0, "payload precedes the SDK call"); assert.equal((payload as { messages: SdkUserMessage[] }).messages[0].message.content, "hello"); return undefined; },
      onResponse: (response) => { order.push(`response:${response.status}`); },
    });
    await until(() => h.queries.length === 1 && h.latest().inputs.length === 1, "query and first input");
    const q = h.latest();
    assert.equal(q.options.systemPrompt, "SYS");
    assert.deepEqual(q.options.tools, []);
    assert.equal(q.options.strictMcpConfig, true);
    assert.deepEqual(q.options.settingSources, []);
    assert.equal(q.options.persistSession, false);
    assert.equal(q.options.permissionMode, "bypassPermissions");
    assert.equal(q.options.allowDangerouslySkipPermissions, true);
    assert.equal(q.options.model, "sonnet");
    assert.equal(q.options.effort, "high");
    assert.equal(q.options.thinking, undefined);
    assert.equal(q.options.cwd, "/tmp/cwd");
    assert.deepEqual(q.tools.map(t => t.name), ["probe", "other"]);
    assert.deepEqual(q.inputs[0], { type: "user", message: { role: "user", content: "hello" }, parent_tool_use_id: null });
    q.assistant([{ type: "text", text: "hi there" }]);
    q.result();
    const final = await turn.done;
    assert.equal(final.stopReason, "stop");
    assert.deepEqual(final.content, [{ type: "text", text: "hi there" }]);
    assert.deepEqual(turn.events.map(e => e.type), ["start", "text_start", "text_delta", "text_end", "done"]);
    assert.equal(final.usage.input, 2); assert.equal(final.usage.output, 5); assert.equal(final.usage.cacheRead, 100); assert.equal(final.usage.cacheWrite, 7); assert.equal(final.usage.totalTokens, 114);
    assert.equal(final.usage.cost.total, 0);
    assert.equal(final.responseModel, "claude-test");
    assert.deepEqual(order, ["payload", "response:200"]);
    assert.equal(h.provider.hasProcess(), true);
  });

  test("onPayload replacement is honored", async () => {
    const h = harness();
    const turn = await h.call(baseContext([user("hello")]), { onPayload: (payload) => ({ ...(payload as object), messages: [{ type: "user", message: { role: "user", content: "replaced" }, parent_tool_use_id: null }] }) });
    await until(() => h.queries.length === 1 && h.latest().inputs.length === 1);
    assert.equal(h.latest().inputs[0].message.content, "replaced");
    h.latest().assistant([{ type: "text", text: "x" }]); h.latest().result();
    await turn.done;
  });

  test("single tool round trip: handler parks, the turn ends toolUse, the next call resolves it by tool_use id in the same process", async () => {
    const h = harness();
    const first = await h.call(baseContext([user("call probe")]));
    await until(() => h.queries.length === 1 && h.latest().inputs.length === 1);
    const q = h.latest();
    q.assistant([{ type: "text", text: "sure" }]);
    q.assistant([{ type: "tool_use", id: "toolu_1", name: "mcp__pi__probe", input: { word: "kiwi" } }]);
    await until(() => first.events.some(e => e.type === "toolcall_end"), "toolcall_end");
    assert.equal(first.events.some(e => e.type === "done"), false, "turn stays open until the handler parks");
    let settled: string | undefined;
    const handler = q.invoke("probe", { word: "kiwi" }).then(r => { settled = r.content[0].text; return r; });
    const final = await first.done;
    assert.equal(final.stopReason, "toolUse");
    assert.deepEqual(final.content, [{ type: "text", text: "sure" }, { type: "toolCall", id: "toolu_1", name: "probe", arguments: { word: "kiwi" } }]);
    assert.equal(settled, undefined, "handler is parked");
    const second = await h.call(baseContext([user("call probe"), final, toolResult("toolu_1", "probe", "probe says kiwi")]));
    const resolved = await handler;
    assert.deepEqual(resolved, { content: [{ type: "text", text: "probe says kiwi" }] });
    assert.equal(h.queries.length, 1, "same process");
    assert.equal(q.inputs.length, 1, "no new user message pushed for a tool result");
    q.assistant([{ type: "text", text: "kiwi it is" }]);
    q.result();
    const done = await second.done;
    assert.equal(done.stopReason, "stop");
    assert.deepEqual(done.content, [{ type: "text", text: "kiwi it is" }]);
    assert.equal(h.logs.some(l => l.startsWith("resync")), false);
  });

  test("error tool results are forwarded as isError", async () => {
    const h = harness();
    const first = await h.call(baseContext([user("go")]));
    await until(() => h.queries.length === 1);
    const q = h.latest();
    q.assistant([{ type: "tool_use", id: "t1", name: "mcp__pi__probe", input: {} }]);
    const handler = q.invoke("probe");
    const final = await first.done;
    await h.call(baseContext([user("go"), final, toolResult("t1", "probe", "boom", true)]));
    assert.deepEqual(await handler, { content: [{ type: "text", text: "boom" }], isError: true });
  });

  test("two parallel tool_use blocks of the same tool become two sequential Pi turns and both resolve with their own result", async () => {
    const h = harness();
    const first = await h.call(baseContext([user("two calls")]));
    await until(() => h.queries.length === 1 && h.latest().inputs.length === 1);
    const q = h.latest();
    q.assistant([{ type: "tool_use", id: "tA", name: "mcp__pi__probe", input: { word: "a" } }]);
    q.assistant([{ type: "tool_use", id: "tB", name: "mcp__pi__probe", input: { word: "b" } }]);
    const handlerA = q.invoke("probe", { word: "a" });
    const handlerB = q.invoke("probe", { word: "b" });
    const one = await first.done;
    assert.equal(one.stopReason, "toolUse");
    assert.deepEqual(one.content, [{ type: "toolCall", id: "tA", name: "probe", arguments: { word: "a" } }]);
    const second = await h.call(baseContext([user("two calls"), one, toolResult("tA", "probe", "ra")]));
    assert.deepEqual(await handlerA, { content: [{ type: "text", text: "ra" }] });
    const two = await second.done;
    assert.equal(two.stopReason, "toolUse");
    assert.deepEqual(two.content, [{ type: "toolCall", id: "tB", name: "probe", arguments: { word: "b" } }]);
    const third = await h.call(baseContext([user("two calls"), one, toolResult("tA", "probe", "ra"), two, toolResult("tB", "probe", "rb")]));
    assert.deepEqual(await handlerB, { content: [{ type: "text", text: "rb" }] });
    q.assistant([{ type: "text", text: "both" }]); q.result();
    assert.equal((await third.done).stopReason, "stop");
    assert.equal(h.queries.length, 1);
  });

  test("a handler invoked before its block arrives still pairs and parks", async () => {
    const h = harness();
    const first = await h.call(baseContext([user("go")]));
    await until(() => h.queries.length === 1);
    const q = h.latest();
    const handler = q.invoke("probe", { word: "x" });
    await new Promise(r => setTimeout(r, 2));
    q.assistant([{ type: "tool_use", id: "tX", name: "mcp__pi__probe", input: { word: "x" } }]);
    const final = await first.done;
    assert.equal(final.stopReason, "toolUse");
    await h.call(baseContext([user("go"), final, toolResult("tX", "probe", "rx")]));
    assert.deepEqual(await handler, { content: [{ type: "text", text: "rx" }] });
  });

  test("a second user turn reuses the process; tool results plus a steer user message keep the turn open until the last result", async () => {
    const h = harness();
    const first = await h.call(baseContext([user("one")]));
    await until(() => h.queries.length === 1 && h.latest().inputs.length === 1);
    const q = h.latest();
    q.assistant([{ type: "text", text: "1" }]); q.result();
    const one = await first.done;
    const second = await h.call(baseContext([user("one"), one, user("two")]));
    await until(() => q.inputs.length === 2, "second user input");
    assert.equal(h.queries.length, 1, "same process");
    assert.equal(q.inputs[1].message.content, "two", "no transcript for a plain continuation");
    q.assistant([{ type: "tool_use", id: "t1", name: "mcp__pi__probe", input: {} }]);
    const handler = q.invoke("probe");
    const two = await second.done;
    assert.equal(two.stopReason, "toolUse");
    const third = await h.call(baseContext([user("one"), one, user("two"), two, toolResult("t1", "probe", "r"), user("steer")]));
    await until(() => q.inputs.length === 3, "steer input");
    assert.deepEqual(await handler, { content: [{ type: "text", text: "r" }] });
    q.assistant([{ type: "text", text: "tail of turn two" }]);
    q.result();
    await new Promise(r => setTimeout(r, 3));
    assert.equal(third.events.some(e => e.type === "done"), false, "first result does not end the turn while the steer turn is in flight");
    q.assistant([{ type: "text", text: "answer to steer" }], { message: { id: "msg_2" } });
    q.result();
    const done = await third.done;
    assert.equal(done.stopReason, "stop");
    assert.deepEqual(done.content.map(c => (c as { text: string }).text), ["tail of turn two", "answer to steer"]);
  });

  test("thinking blocks map to Pi thinking content with signature; unknown blocks are ignored", async () => {
    const h = harness();
    const turn = await h.call(baseContext([user("think")]), { reasoning: "medium" });
    await until(() => h.queries.length === 1);
    const q = h.latest();
    q.assistant([{ type: "thinking", thinking: "hmm", signature: "sig" }, { type: "server_tool_use", id: "x" }, { type: "text", text: "ok" }]);
    q.result();
    const final = await turn.done;
    assert.deepEqual(final.content, [{ type: "thinking", thinking: "hmm", thinkingSignature: "sig" }, { type: "text", text: "ok" }]);
    assert.deepEqual(turn.events.map(e => e.type), ["start", "thinking_start", "thinking_delta", "thinking_end", "text_start", "text_delta", "text_end", "done"]);
    assert.equal((turn.events[4] as { contentIndex: number }).contentIndex, 1);
  });

  test("usage is counted once per SDK message id and summed across distinct messages", async () => {
    const h = harness();
    const turn = await h.call(baseContext([user("u")]));
    await until(() => h.queries.length === 1);
    const q = h.latest();
    q.assistant([{ type: "text", text: "a" }]);
    q.assistant([{ type: "text", text: "b" }]);
    q.assistant([{ type: "text", text: "c" }], { message: { id: "msg_2" } });
    q.result();
    const final = await turn.done;
    assert.equal(final.usage.input, 4); assert.equal(final.usage.cacheRead, 200); assert.equal(final.usage.output, 10); assert.equal(final.usage.cacheWrite, 14);
  });

  test("thinking off disables SDK thinking; each effort change restarts the process with that effort", async () => {
    const h = harness();
    const seen: Array<[unknown, unknown]> = [];
    let history: Message[] = [user("u")];
    for (const level of [undefined, "minimal", "low", "medium", "high", "xhigh", "max"] as const) {
      const turn = await h.call(baseContext(history), { reasoning: level as never });
      await until(() => h.latest()?.inputs.length === 1, `query for ${level}`);
      const q = h.latest();
      seen.push([q.options.effort, q.options.thinking]);
      q.assistant([{ type: "text", text: "r" }]); q.result();
      history = [...history, await turn.done, user(`next ${level}`)];
    }
    assert.deepEqual(seen, [[undefined, { type: "disabled" }], ["low", undefined], ["low", undefined], ["medium", undefined], ["high", undefined], ["xhigh", undefined], ["max", undefined]]);
    assert.equal(h.queries.length, 6, "minimal and low share one effort, so that boundary does not resync");
    assert.equal(h.logs.filter(l => l.startsWith("resync: effort changed")).length, 5);
    assert.equal(h.queries.filter(q => q.closed).length, 5, "every superseded process is closed");
  });
});

describe("claude-code provider: resync", () => {
  async function seeded() {
    const h = harness();
    const first = await h.call(baseContext([user("one")]));
    await until(() => h.queries.length === 1 && h.latest().inputs.length === 1);
    h.latest().assistant([{ type: "text", text: "1" }]); h.latest().result();
    const one = await first.done;
    return { h, one, history: [user("one"), one] as Message[] };
  }
  async function expectRestart(h: ReturnType<typeof harness>, context: Context, reason: RegExp) {
    const old = h.latest();
    const turn = await h.call(context);
    await until(() => h.queries.length === 2 && h.latest().inputs.length === 1, "replacement process");
    assert.equal(old.closed, true, "old process closed");
    const fresh = h.latest();
    assert.equal(fresh.inputs[0].message.content, buildReplayPrompt(context.messages), "prior history replayed as the transcript, in order");
    assert.ok(h.logs.some(l => reason.test(l)), `logged ${reason}: ${h.logs.join(" | ")}`);
    fresh.assistant([{ type: "text", text: "again" }]); fresh.result();
    assert.equal((await turn.done).stopReason, "stop");
    return fresh;
  }

  test("compaction-shaped history restarts and replays", async () => {
    const { h, one } = await seeded();
    await expectRestart(h, baseContext([user("summary of one"), one, user("two")]), /resync: history diverged at message 0/);
  });

  test("history that is not longer restarts", async () => {
    const { h, history } = await seeded();
    await expectRestart(h, baseContext(history), /resync: history is not an extension/);
  });

  test("changed system prompt restarts with the new prompt", async () => {
    const { h, history } = await seeded();
    const fresh = await expectRestart(h, baseContext([...history, user("two")], { systemPrompt: "SYS2" }), /resync: system prompt changed/);
    assert.equal(fresh.options.systemPrompt, "SYS2");
  });

  test("grown tool set restarts with the new tools reflected", async () => {
    const { h, history } = await seeded();
    const grown = [...tools!, { name: "late", description: "late tool", parameters: { type: "object", properties: {} } }] as Context["tools"];
    const fresh = await expectRestart(h, baseContext([...history, user("two")], { tools: grown }), /resync: tool set changed/);
    assert.deepEqual(fresh.tools.map(t => t.name), ["probe", "other", "late"]);
  });

  test("changed model restarts with the new model id", async () => {
    const { h, history } = await seeded();
    const context = baseContext([...history, user("two")]);
    const old = h.latest();
    const turn = (() => { const stream = h.provider.streamSimple({ ...model, id: "opus" }, context, {} as SimpleStreamOptions); return { stream, done: (async () => { for await (const _ of stream) { /* drain */ } return stream.result(); })() }; })();
    await until(() => h.queries.length === 2 && h.latest().inputs.length === 1);
    assert.equal(old.closed, true);
    assert.equal(h.latest().options.model, "opus");
    assert.ok(h.logs.some(l => /resync: model changed \(sonnet -> opus\)/.test(l)));
    h.latest().assistant([{ type: "text", text: "o" }]); h.latest().result();
    assert.equal((await turn.done).stopReason, "stop");
  });

  test("a toolResult without a parked handler restarts and replays the result inside the transcript", async () => {
    const { h, one } = await seeded();
    const context = baseContext([user("one"), one, toolResult("ghost", "probe", "r")]);
    const fresh = await expectRestart(h, context, /resync: no parked handler for probe \(ghost\)/);
    assert.ok(fresh.inputs[0].message.content.includes("[tool_result id=\"ghost\" name=\"probe\" error=\"false\"]\nr"));
  });
});

describe("claude-code provider: lifecycle and failure", () => {
  test("abort interrupts, ends the turn aborted, rejects parked handlers, and discards the interrupted result", async () => {
    const h = harness();
    const controller = new AbortController();
    const first = await h.call(baseContext([user("go")]), { signal: controller.signal });
    await until(() => h.queries.length === 1 && h.latest().inputs.length === 1);
    const q = h.latest();
    const early = q.invoke("other").catch(e => `rejected:${(e as Error).message}`);
    q.assistant([{ type: "tool_use", id: "tS", name: "mcp__pi__probe", input: {} }]);
    await until(() => first.events.some(e => e.type === "toolcall_end"));
    controller.abort();
    const final = await first.done;
    assert.equal(final.stopReason, "aborted");
    assert.equal(first.events.at(-1)?.type, "error");
    assert.equal(q.interrupts, 1);
    assert.equal(q.closed, false, "process is kept");
    assert.equal(await early, "rejected:Pi aborted the turn");
    const late = await q.invoke("probe").catch(e => `rejected:${(e as Error).message}`);
    assert.equal(late, "rejected:Pi aborted the turn", "a handler arriving for the aborted turn's block is rejected");
    q.result({ subtype: "error_during_execution", is_error: true, result: "interrupted" });
    const second = await h.call(baseContext([user("go"), final, user("again")]));
    await until(() => q.inputs.length === 2);
    assert.equal(h.queries.length, 1, "next call continues the kept process");
    q.assistant([{ type: "text", text: "back" }]); q.result();
    const done = await second.done;
    assert.equal(done.stopReason, "stop");
    assert.deepEqual(done.content, [{ type: "text", text: "back" }]);
  });

  test("an already-aborted signal ends the turn immediately", async () => {
    const h = harness();
    const controller = new AbortController();
    controller.abort();
    const turn = await h.call(baseContext([user("go")]), { signal: controller.signal });
    assert.equal((await turn.done).stopReason, "aborted");
  });

  test("session_shutdown closes the process through the registered handler", async () => {
    const { sdk, queries } = fakeSdk();
    const handlers = new Map<string, (...args: unknown[]) => unknown>();
    const registered: Array<{ name: string; config: Record<string, unknown> }> = [];
    const pi = { registerProvider: (name: string, config: Record<string, unknown>) => registered.push({ name, config }), on: (event: string, handler: (...args: unknown[]) => unknown) => handlers.set(event, handler) } as never;
    const handle = registerClaudeCodeProvider(pi, { sdk: async () => sdk, log: () => {} });
    assert.equal(registered[0].name, "claude-code");
    assert.equal(registered[0].config.streamSimple, handle.streamSimple);
    assert.equal(registered[0].config.models, CLAUDE_CODE_MODELS);
    assert.equal(typeof registered[0].config.apiKey, "string");
    assert.equal(registered[0].config.api, "anthropic-messages");
    const stream = handle.streamSimple(model, baseContext([user("x")]));
    const done = (async () => { for await (const _ of stream) { /* drain */ } return stream.result(); })();
    await until(() => queries.length === 1 && queries[0].inputs.length === 1);
    queries[0].assistant([{ type: "text", text: "y" }]); queries[0].result();
    await done;
    assert.equal(handle.hasProcess(), true);
    await handlers.get("session_shutdown")!({}, {});
    assert.equal(queries[0].closed, true);
    assert.equal(handle.hasProcess(), false);
    handle.close();
  });

  test("shutdown mid-turn ends the open turn as an error and rejects parked handlers", async () => {
    const h = harness();
    const turn = await h.call(baseContext([user("x")]));
    await until(() => h.queries.length === 1);
    const parked = h.latest().invoke("probe").catch(e => `rejected:${(e as Error).message}`);
    h.provider.close();
    const final = await turn.done;
    assert.equal(final.stopReason, "error");
    assert.match(final.errorMessage ?? "", /session shutdown/);
    assert.equal(await parked, "rejected:session shutdown");
  });

  test("SDK load failure yields an error turn naming the remedy", async () => {
    const provider = createClaudeCodeProvider({ sdk: async () => { throw new Error("Cannot find module '@anthropic-ai/claude-agent-sdk'"); }, log: () => {} });
    const stream = provider.streamSimple(model, baseContext([user("x")]));
    const events: AssistantMessageEvent[] = [];
    for await (const e of stream) events.push(e);
    const final = await stream.result();
    assert.equal(final.stopReason, "error");
    assert.match(final.errorMessage ?? "", /Cannot find module/);
    assert.ok(final.errorMessage?.includes(CLAUDE_CODE_REMEDY));
    assert.match(final.errorMessage ?? "", /claude auth login/);
    assert.deepEqual(events.map(e => e.type), ["start", "error"]);
    assert.equal(provider.hasProcess(), false);
  });

  test("query() throwing at start yields an error turn and no process", async () => {
    const h = harness({ query: () => { throw new Error("spawn claude ENOENT"); } });
    const turn = await h.call(baseContext([user("x")]));
    const final = await turn.done;
    assert.equal(final.stopReason, "error");
    assert.match(final.errorMessage ?? "", /ENOENT.*claude auth login/s);
    assert.equal(h.provider.hasProcess(), false);
  });

  test("process failure and premature end surface as error turns with the remedy", async () => {
    for (const mode of ["fail", "end"] as const) {
      const h = harness();
      const turn = await h.call(baseContext([user("x")]));
      await until(() => h.queries.length === 1);
      if (mode === "fail") h.latest().fail(new Error("transport closed")); else h.latest().end();
      const final = await turn.done;
      assert.equal(final.stopReason, "error", mode);
      assert.match(final.errorMessage ?? "", mode === "fail" ? /transport closed/ : /claude process ended/);
      assert.ok(final.errorMessage?.includes(CLAUDE_CODE_REMEDY));
      assert.equal(h.provider.hasProcess(), false);
      const next = await h.call(baseContext([user("x"), final, user("y")]));
      await until(() => h.queries.length === 2 && h.latest().inputs.length === 1, "restart after failure");
      assert.ok(h.logs.some(l => /resync: process is not alive/.test(l)));
      h.latest().assistant([{ type: "text", text: "ok" }]); h.latest().result();
      assert.equal((await next.done).stopReason, "stop");
    }
  });

  test("a result with is_error ends the turn as error carrying the SDK text", async () => {
    const h = harness();
    const turn = await h.call(baseContext([user("x")]));
    await until(() => h.queries.length === 1);
    h.latest().assistant([{ type: "text", text: "partial" }]);
    h.latest().result({ subtype: "error_during_execution", is_error: true, result: "Not logged in", errors: ["run claude auth login"] });
    const final = await turn.done;
    assert.equal(final.stopReason, "error");
    assert.equal(final.errorMessage, "Not logged in\nrun claude auth login");
    assert.deepEqual(final.content, [{ type: "text", text: "partial" }]);
    assert.equal(h.provider.hasProcess(), true, "the process stays for the next call's extension check");
  });
});

import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import {
  createReasoningSummaryObserver, eligibleSummaryModel, fitReasoningSummary, summaryPrefix,
  SUMMARY_ATTEMPTS, SUMMARY_FRAME_BYTES, SUMMARY_PARTS, SUMMARY_PROJECTION_BYTES, SUMMARY_TEXT_BYTES,
  type ReasoningSummary,
} from "../src/reasoning-summary.ts";
import { hostedSummaryFrame, hostedSummaryLine, serializeMcpRequest } from "../src/reasoning-summary-transport.ts";

export const producer = "c3e3ae17-5a37-4223-ab94-53d31fd43dba";
const context = (provider = "openai", api = "openai-responses") => ({
  model: { provider, api, id: "gpt-test" },
  modelRegistry: { getRegisteredProviderConfig: () => undefined, getRegisteredNativeProvider: () => undefined },
});
function fixture(provider = "openai", api = "openai-responses", version = "1.5.2") {
  const observer = createReasoningSummaryObserver(version, producer);
  const ctx = context(provider, api);
  let sequence = 0;
  const raw = (data: Record<string, unknown>) => observer.observe({ type: "provider_stream_event", provider, api, model: "gpt-test", data });
  const emit = (data: Record<string, unknown>) => raw({ sequence_number: sequence++, ...data });
  const hook = () => observer.observe({ type: "before_provider_request", get payload() { throw new Error("payload inspected"); } }, ctx);
  const created = (id: string) => { sequence = 0; emit({ type: "response.created", response: { id } }); };
  const item = (id = "item", output = 0) => emit({ type: "response.output_item.added", output_index: output, item: { type: "reasoning", id, encrypted_content: "SECRET" } });
  const delta = (text: string, id = "item", output = 0, part = 0) => emit({ type: "response.reasoning_summary_text.delta", item_id: id, output_index: output, summary_index: part, delta: text });
  const done = (text: string, id = "item", output = 0, part = 0) => emit({ type: "response.reasoning_summary_text.done", item_id: id, output_index: output, summary_index: part, text });
  observer.observe({ type: "agent_start" });
  hook();
  return { observer, ctx, raw, emit, hook, created, item, delta, done, snapshot: () => observer.projection()!, text: () => observer.projection()?.requests.flatMap((r) => r.parts.map((p) => p.text)) };
}

export function sampleSummary(text = "received"): ReasoningSummary {
  return { v: 1, producer, turn: 1, revision: 1, activeRequest: { request: 1, attempt: 1 }, truncated: false, requests: [{ request: 1, attempt: 1, provider: "openai", api: "openai-responses", model: "gpt-test", status: "streaming", truncated: false, parts: [{ output: 0, part: 0, text, observedBytes: Buffer.byteLength(text), retainedBytes: Buffer.byteLength(text), truncated: false, status: "streaming" }] }] };
}

test("exact host and both public override checks are mandatory on every request", () => {
  for (const version of ["0.84.4", "1.5.1", "1.5.3", undefined]) assert.equal(eligibleSummaryModel(version, context()), undefined);
  for (const [provider, api] of [["anthropic", "anthropic-messages"], ["openai", "openai-completions"], ["proxy", "openai-responses"]]) assert.equal(eligibleSummaryModel("1.5.2", context(provider, api)), undefined);
  const ctx = context();
  let config = 0, native = 0;
  ctx.modelRegistry.getRegisteredProviderConfig = () => { config++; return {} as never; };
  ctx.modelRegistry.getRegisteredNativeProvider = () => { native++; return undefined; };
  assert.equal(eligibleSummaryModel("1.5.2", ctx), undefined);
  assert.equal(config, 1); assert.equal(native, 1);
  assert.equal(eligibleSummaryModel("1.5.2", { model: ctx.model, modelRegistry: {} }), undefined);
  ctx.modelRegistry.getRegisteredProviderConfig = () => { throw new Error("unavailable"); };
  assert.equal(eligibleSummaryModel("1.5.2", ctx), undefined);
  const h = fixture();
  h.created("r1"); h.item(); h.delta("allowed");
  h.ctx.modelRegistry.getRegisteredNativeProvider = () => ({} as never);
  h.hook(); h.created("r2"); h.item("i2"); h.delta("not allowed", "i2");
  assert.deepEqual(h.text(), ["allowed"]);
});

test("model identity matches the receiver's existing 80-rune display bound without truncation", () => {
  const ctx = context();
  for (const name of ["x".repeat(80), "😀".repeat(80)]) {
    ctx.model.id = name;
    assert.equal(eligibleSummaryModel("1.5.2", ctx)?.model, name);
  }
  for (const name of ["x".repeat(81), "😀".repeat(81), ""]) {
    ctx.model.id = name;
    assert.equal(eligibleSummaryModel("1.5.2", ctx), undefined);
  }
});

test("1.5.2 Responses SSE and Codex SSE/WS replay only explicit summaries before message_start", () => {
  for (const [provider, api, transport] of [["openai", "openai-responses", "sse"], ["openai-codex", "openai-codex-responses", "sse"], ["openai-codex", "openai-codex-responses", "websocket"]]) {
    const h = fixture(provider, api);
    h.created(`response-${transport}`); h.item();
    h.delta("verified ");
    h.observer.observe({ type: "message_start", message: { role: "assistant", content: [] } });
    h.delta("prefix"); h.done("authoritative");
    assert.deepEqual(h.text(), ["authoritative"]);
    h.emit({ type: "response.completed", response: { id: `response-${transport}`, status: "completed", output: ["SECRET"] } });
    assert.equal(h.snapshot().requests[0].status, "complete");
    assert.equal(h.snapshot().activeRequest, null);
    assert.ok(!JSON.stringify(h.snapshot()).includes("SECRET"));
  }
});

test("read-only whitelist never inspects output arrays, encrypted replay or request/error bodies", () => {
  const h = fixture();
  const forbidden = () => { throw new Error("forbidden field inspected"); };
  h.raw(Object.freeze({ type: "response.created", sequence_number: 0, response: Object.freeze({ id: "r", get output() { return forbidden(); } }) }));
  h.raw(Object.freeze({ type: "response.output_item.added", sequence_number: 1, output_index: 0, item: Object.freeze({ id: "item", type: "reasoning", get encrypted_content() { return forbidden(); }, get summary() { return forbidden(); } }) }));
  h.raw(Object.freeze({ type: "response.reasoning_summary_part.added", sequence_number: 2, item_id: "item", output_index: 0, summary_index: 0, part: Object.freeze({ type: "summary_text", text: "start" }) }));
  h.raw(Object.freeze({ type: "response.reasoning_summary_text.delta", sequence_number: 3, item_id: "item", output_index: 0, summary_index: 0, delta: " received" }));
  h.raw(Object.freeze({ type: "response.failed", sequence_number: 4, response: Object.freeze({ id: "r", status: "failed", get error() { return forbidden(); }, get output() { return forbidden(); } }) }));
  assert.deepEqual(h.text(), ["start received"]);
  assert.equal(h.snapshot().requests[0].status, "failed");
});

test("part binding, indexes, ordering, response sequence dedup and authoritative replacements", () => {
  const h = fixture(); h.created("r");
  h.delta("unbound"); h.item("b", 4); h.item("a", 1);
  h.delta("later", "b", 4, 2); h.delta("earlier", "a", 1, 3);
  h.raw({ type: "response.reasoning_summary_text.delta", sequence_number: 5, item_id: "a", output_index: 1, summary_index: 3, delta: "duplicate" });
  h.emit({ type: "response.reasoning_summary_part.done", item_id: "a", output_index: 1, summary_index: 3, part: { type: "summary_text", text: "exact" }, status: "incomplete" });
  h.emit({ type: "response.reasoning_summary_text.delta", item_id: "a", output_index: 1, summary_index: -1, delta: "invalid" });
  h.emit({ type: "response.reasoning_summary_text.delta", item_id: "a", output_index: 1, summary_index: Number.MAX_SAFE_INTEGER + 1, delta: "unsafe" });
  assert.deepEqual(h.text(), ["exact", "later"]);
  assert.equal(h.snapshot().requests[0].parts[0].status, "incomplete");
  assert.equal(h.snapshot().requests[0].parts[0].observedBytes, 5);
  h.raw({ type: "response.reasoning_summary_text.done", sequence_number: Number.MAX_SAFE_INTEGER + 1, item_id: "a", output_index: 1, summary_index: 3, text: "unsafe" });
  assert.deepEqual(h.text(), ["exact", "later"]);
});

test("hook-once transport retry/missing-continuation fixture preserves request and advances explicit attempt only after failure", () => {
  const h = fixture("openai-codex", "openai-codex-responses");
  h.emit({ type: "error", sequence_number: undefined }); // response-less missing continuation
  assert.equal(h.observer.projection(), undefined);
  h.created("retry"); h.item("i2"); h.delta("failed prefix", "i2");
  h.emit({ type: "response.failed", response: { id: "retry", status: "failed", error: { message: "SECRET" } } });
  h.created("retry"); // repeated response cannot advance
  h.created("recovered"); h.item("i3"); h.delta("new prefix", "i3");
  h.raw({ type: "response.completed", sequence_number: 999, response: { id: "retry", status: "completed" } });
  h.emit({ type: "response.completed", response: { id: "recovered", status: "completed" } });
  h.hook(); h.created("next-request"); h.item("i4"); h.delta("next", "i4");
  assert.deepEqual(h.snapshot().requests.map((r) => [r.request, r.attempt, r.status]), [[1, 2, "failed"], [1, 3, "complete"], [2, 1, "streaming"]]);
  assert.deepEqual(h.text(), ["failed prefix", "new prefix", "next"]);
  assert.ok(!JSON.stringify(h.snapshot()).includes("SECRET"));
});

test("unexplained overlap, retired responses/items and mismatched provider/model cannot contaminate attempts", () => {
  const h = fixture(); h.created("old"); h.item("old-item"); h.delta("old", "old-item");
  h.created("overlap"); h.delta("ambiguous", "old-item");
  assert.deepEqual(h.text(), ["old"]);
  assert.equal(h.snapshot().activeRequest, null);
  h.hook(); h.created("old"); h.delta("stale", "old-item");
  h.created("new"); h.item("old-item"); h.delta("stale", "old-item"); h.item("new-item"); h.delta("new", "new-item");
  h.observer.observe({ type: "provider_stream_event", provider: "openai", api: "openai-responses", model: "other", data: { type: "response.reasoning_summary_text.delta", sequence_number: 99, item_id: "new-item", output_index: 0, summary_index: 0, delta: "wrong" } });
  assert.deepEqual(h.text(), ["old", "new"]);
});

test("generic thinking, raw reasoning, signatures, encrypted/redacted input and subagent events never become text", () => {
  const h = fixture(); h.created("r"); h.item();
  for (const type of ["response.reasoning_text.delta", "response.output_item.done", "response.output_text.delta"]) h.emit({ type, item_id: "item", output_index: 0, summary_index: 0, delta: "SECRET", item: { type: "reasoning", summary: [{ text: "SECRET" }], encrypted_content: "SECRET" } });
  h.observer.observe({ type: "message_update", assistantMessageEvent: { type: "thinking_delta", delta: "SECRET" } });
  h.observer.observe({ type: "message_end", message: { role: "assistant", content: [{ type: "thinking", thinking: "SECRET", thinkingSignature: "SECRET", redacted: true }] } });
  h.observer.observe({ type: "provider_stream_event", agentId: "child", provider: "openai", api: "openai-responses", model: "gpt-test", data: { type: "response.reasoning_summary_text.delta", sequence_number: 99, item_id: "item", output_index: 0, summary_index: 0, delta: "SECRET" } });
  assert.equal(h.observer.projection(), undefined);
  h.delta("public"); assert.deepEqual(h.text(), ["public"]);
  h.observer.retire(); h.delta("stale closure"); assert.equal(h.observer.projection(), undefined);
});

test("actual message abort/error/length facts and settled abort preserve earlier complete attempts", () => {
  for (const [stopReason, status] of [["aborted", "aborted"], ["error", "failed"], ["length", "incomplete"], ["stop", "complete"], ["toolUse", "complete"]]) {
    const h = fixture(); h.created("r1"); h.item(); h.delta("first");
    h.emit({ type: "response.completed", response: { id: "r1", status: "completed" } });
    h.hook(); h.created("r2"); h.item("i2"); h.delta("second", "i2");
    h.observer.observe({ type: "message_end", message: { role: "assistant", responseId: "r2", provider: "openai", api: "openai-responses", model: "gpt-test", stopReason } });
    h.observer.observe({ type: "agent_settled", aborted: true });
    assert.deepEqual(h.snapshot().requests.map((r) => r.status), ["complete", status]);
    assert.deepEqual(h.text(), ["first", "second"]);
  }
  const h = fixture(); h.created("r"); h.item(); h.delta("prefix");
  h.observer.observe({ type: "agent_settled", aborted: true });
  assert.equal(h.snapshot().requests[0].status, "aborted");
});

test("outer turn/revision survive repeated agent_start and transport retries; no fabricated response-less history", () => {
  const h = fixture(); h.created("r"); h.item(); h.delta("one");
  const revision = h.snapshot().revision;
  h.observer.observe({ type: "agent_start" }); h.delta(" two");
  assert.equal(h.snapshot().turn, 1);
  assert.ok(h.snapshot().revision > revision);
  h.observer.observe({ type: "agent_settled" });
  const closedRevision = h.snapshot().revision;
  h.observer.observe({ type: "agent_start" }); h.hook();
  assert.equal(h.observer.projection(), undefined);
  h.created("r-next"); h.item("next-item"); h.delta("next", "next-item");
  assert.equal(h.snapshot().turn, 2);
  assert.equal(h.snapshot().requests[0].request, 1);
  assert.ok(h.snapshot().revision > closedRevision);
});

test("8-KiB aggregate exact UTF-8 prefixes, replacement byte accounting and separate local truncation", () => {
  const h = fixture(); h.created("r"); h.item();
  h.delta("한😀".repeat(2000)); h.delta("must not append after clipped gap");
  const part = h.snapshot().requests[0].parts[0];
  assert.ok(part.retainedBytes <= SUMMARY_TEXT_BYTES);
  assert.equal(part.observedBytes, Buffer.byteLength("한😀".repeat(2000) + "must not append after clipped gap"));
  assert.equal(part.text, summaryPrefix("한😀".repeat(2000), SUMMARY_TEXT_BYTES));
  assert.equal(part.status, "streaming"); assert.equal(part.truncated, true);
  h.done("short");
  assert.equal(h.snapshot().requests[0].parts[0].observedBytes, 5);
  assert.equal(h.snapshot().requests[0].parts[0].truncated, false);
  h.item("i2", 1); h.delta("a".repeat(SUMMARY_TEXT_BYTES), "i2", 1);
  assert.equal(h.snapshot().requests[0].parts.reduce((n, p) => n + p.retainedBytes, 0), SUMMARY_TEXT_BYTES);
});

test("16 aggregate bound attempts, not sixteen requests each with retries; active omission is null", () => {
  for (const retry of [true, false]) {
    const h = fixture();
    for (let i = 0; i < SUMMARY_ATTEMPTS + 4; i++) {
      if (i && !retry) h.hook();
      h.created(`r${i}`); h.item(`i${i}`); h.delta(`${i}`, `i${i}`);
      if (i < SUMMARY_ATTEMPTS - 1) h.emit({ type: "response.failed", response: { id: `r${i}`, status: "failed" } });
      else if (i === SUMMARY_ATTEMPTS - 1) h.emit({ type: "response.failed", response: { id: `r${i}`, status: "failed" } });
    }
    const summary = h.snapshot();
    assert.equal(summary.requests.length, SUMMARY_ATTEMPTS);
    assert.equal(summary.activeRequest, null);
    assert.equal(summary.truncated, true);
    assert.deepEqual(summary.requests.map((r) => [r.request, r.attempt]), Array.from({ length: 16 }, (_, i) => retry ? [1, i + 1] : [i + 1, 1]));
  }
});

test("64 aggregate parts across requests and interleaved outputs", () => {
  const h = fixture();
  for (let r = 0; r < 2; r++) {
    if (r) h.hook();
    h.created(`r${r}`); h.item(`i${r}`);
    for (let p = 0; p < 40; p++) h.delta("x", `i${r}`, 0, p);
  }
  assert.equal(h.snapshot().requests.reduce((n, r) => n + r.parts.length, 0), SUMMARY_PARTS);
  assert.equal(h.snapshot().truncated, true);
});

test("24-KiB projection and actual terminal/hosted JSONL frames include escaping, counters, ID, LF and both encodings", () => {
  for (const text of ["\u0000\n\t\"\\".repeat(1300), "한😀".repeat(1100), "<>&\u2028\u2029".repeat(900), "x".repeat(8192)]) {
    const summary = sampleSummary(text);
    summary.turn = summary.revision = summary.requests[0].request = summary.requests[0].attempt = Number.MAX_SAFE_INTEGER;
    summary.activeRequest = { request: Number.MAX_SAFE_INTEGER, attempt: Number.MAX_SAFE_INTEGER };
    const projection = fitReasoningSummary(summary)!;
    assert.ok(Buffer.byteLength(JSON.stringify(projection)) <= SUMMARY_PROJECTION_BYTES);
    const line = serializeMcpRequest({ jsonrpc: "2.0", id: Number.MAX_SAFE_INTEGER, method: "tools/call", params: { name: "dashboard.intake", arguments: { session_key: "key".repeat(100), adapter: "pi", events: [{ kind: "turn_end", at: Number.MAX_SAFE_INTEGER, reasoningSummary: projection }] } } });
    assert.ok(Buffer.byteLength(line) <= SUMMARY_FRAME_BYTES);
    assert.ok(line.endsWith("\n"));
    const sent = JSON.parse(line).params.arguments.events[0].reasoningSummary;
    for (const r of sent.requests) for (const p of r.parts) { assert.ok(text.startsWith(p.text)); assert.equal(p.retainedBytes, Buffer.byteLength(p.text)); assert.equal(p.observedBytes, Buffer.byteLength(text)); }
    const widget = hostedSummaryLine(projection)!;
    assert.ok(Buffer.byteLength(hostedSummaryFrame(widget)) <= SUMMARY_FRAME_BYTES);
    assert.equal(JSON.parse(hostedSummaryFrame(widget)).widgetLines.length, 1);
    assert.deepEqual(JSON.parse(JSON.parse(hostedSummaryFrame(widget)).widgetLines[0]).reasoningSummary, JSON.parse(widget).reasoningSummary);
  }
  const summary = sampleSummary();
  assert.throws(() => serializeMcpRequest({ jsonrpc: "2.0", id: 10, method: "tools/call", params: { name: "dashboard.intake", arguments: { session_key: "x".repeat(SUMMARY_FRAME_BYTES), events: [{ kind: "turn_end", reasoningSummary: summary }] } } }), /frame budget/);
  assert.throws(() => serializeMcpRequest({ jsonrpc: "2.0", id: 10, method: "tools/call", params: { name: "dashboard.intake", arguments: { events: [{ kind: "reply", text: "x" }, { kind: "agent", reasoningSummary: summary }] } } }), /isolated/);
  const omitted = fitReasoningSummary(summary, (s) => s.requests.length === 0)!;
  assert.equal(omitted.activeRequest, null); assert.equal(omitted.truncated, true);
});

test("source and shipped bundle preserve the explicit-summary gate and capability declaration", () => {
  const source = readFileSync(new URL("../src/index.ts", import.meta.url), "utf8");
  assert.match(source, /import \{ VERSION, type ExtensionAPI/);
  assert.match(source, /sessionRole === undefined \? createReasoningSummaryObserver\(VERSION\)/);
  assert.match(source, /reasoningSummaryObserver\?\.retire\(\)/);
  assert.match(source, /reasoningSummaryCapable: VERSION === "1\.5\.2"/);
  const bundle = readFileSync(new URL("../dist/index.js", import.meta.url), "utf8");
  for (const pattern of [
    /response\.reasoning_summary_part\.added/,
    /response\.reasoning_summary_part\.done/,
    /response\.reasoning_summary_text\.delta/,
    /version !== "1\.5\.2"/,
    /provider === "openai" && api === "openai-responses"/,
    /provider === "openai-codex" && api === "openai-codex-responses"/,
    /getRegisteredProviderConfig/,
    /getRegisteredNativeProvider/,
    /reasoning_summary_v1/,
    /reasoningSummaryCapable: VERSION === "1\.5\.2"/,
  ]) assert.match(bundle, pattern);
  const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  assert.equal(pkg.devDependencies["@earendil-works/pi-coding-agent"], "0.84.4");
});

import { test } from "node:test";
import assert from "node:assert/strict";
import { modelObservation, ownModelObservations, parseModelObservation, retainModelObservation } from "../src/model-observation.ts";
import { createOutputRateTracker } from "../src/output-rate.ts";

const message = (model = "physical", timestamp = 1000) => ({ role: "assistant", model, provider: "p", timestamp, stopReason: "stop", content: [{ type: "text", text: "private <answer>" }], usage: { output: 20 } });

test("only completed response fields establish identity, with original time and recorded effort", () => {
  assert.equal(modelObservation({ model: "settings-only", thinkingLevel: "high" }), undefined);
  assert.equal(modelObservation({ ...message(), timestamp: 0 }), undefined);
  for (const stopReason of ["pending", "error", "aborted"]) assert.equal(modelObservation({ ...message(), stopReason }), undefined);
  assert.deepEqual(modelObservation({ ...message(), responseId: "answer-1", responseModel: "provider-concrete", thinkingLevel: "low" }, 42), {
    id: "answer-1", model: "provider-concrete", source: "pi.assistant", at: 1000, effort: "low", effortSource: "message", outputTps: 42,
  });
  assert.equal(modelObservation(message())?.effort, undefined);
});

test("digest identity is replay-stable under reordered JSON, with no raw response in metadata", () => {
  const a = modelObservation(message())!;
  const b = modelObservation({ content: [{ text: "private <answer>", type: "text" }], usage: { output: 20 }, stopReason: "stop", timestamp: 1000, provider: "p", model: "physical", role: "assistant" })!;
  assert.deepEqual(a, b);
  assert.match(a.id, /^message:1000:[a-f0-9]{64}$/);
  assert.ok(!JSON.stringify(a).includes("private"));
  const retried = { ...a, at: 9999 };
  assert.deepEqual(retainModelObservation([a], retried), [a]);
  assert.notEqual(modelObservation(message("physical-next", 2000))?.id, a.id);
});

test("descendant evidence validation is independent and never promotes RPC/request telemetry", () => {
  assert.equal(parseModelObservation({ model: "request", at: 1 }), undefined);
  assert.equal(parseModelObservation({ id: "a", model: "request", source: "state", at: 1 }), undefined);
  const own = modelObservation(message())!;
  assert.deepEqual(parseModelObservation({ ...own, outputTps: Infinity, effort: "high", effortSource: "state" }), own);
});

test("resume restores response evidence but fork prefixes cannot supply child identity", () => {
  const entries = [1000, 2000, 3000].map((at) => ({ type: "message", message: message(`model-${at}`, at) }));
  assert.equal(ownModelObservations(entries).length, 3);
  assert.deepEqual(ownModelObservations(entries, { parentSession: "/parent", timestamp: new Date(2500).toISOString() }).map((o) => o.model), ["model-3000"]);
  assert.deepEqual(ownModelObservations(entries, { parentSession: "/parent" }), []);
});

test("only a current eligible block-span sample supplies observed t/s, not stale rolling residue", () => {
  const tracker = createOutputRateTracker();
  tracker.observe({ type: "message_start", message: message() }, 0);
  tracker.observe({ type: "message_update", assistantMessageEvent: { type: "text_start", contentIndex: 0 } }, 10);
  tracker.observe({ type: "message_update", assistantMessageEvent: { type: "text_end", contentIndex: 0 } }, 1010);
  tracker.observe({ type: "message_end", message: message() }, 1020);
  assert.equal(tracker.rate(), 20);
  assert.equal(tracker.sampleRate?.(), 20);
  tracker.observe({ type: "message_start", message: message("next") }, 2000);
  tracker.observe({ type: "message_end", message: { ...message("next"), usage: { output: 1 } } }, 2010);
  assert.equal(tracker.rate(), 20); // legacy behavior is deliberately unchanged
  assert.equal(tracker.sampleRate?.(), undefined);
  assert.equal(modelObservation(message("next"), tracker.sampleRate?.())?.outputTps, undefined);
});

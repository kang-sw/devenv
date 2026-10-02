import assert from "node:assert/strict";
import { test } from "node:test";
import { createOutputRateTracker, formatOutputRate, OUTPUT_RATE_WINDOW, type OutputRateTracker } from "../src/output-rate.ts";

type Block = { kind: "text" | "thinking" | "toolcall"; index: number; start: number; end?: number };
interface MessageOptions {
  blocks: Block[];
  output: number;
  reasoning?: number;
  stopReason?: string;
  model?: string;
  provider?: string;
  deltas?: boolean;
}

/** Replays one assistant message: block edges in receipt-time order, then message_end at `endAt`. */
function feed(tracker: OutputRateTracker, options: MessageOptions, endAt = 100_000): void {
  const message = {
    role: "assistant",
    provider: options.provider ?? "p",
    model: options.model ?? "m",
    stopReason: options.stopReason ?? "stop",
    usage: { output: options.output, ...(options.reasoning !== undefined ? { reasoning: options.reasoning } : {}) },
  };
  const events: Array<[number, unknown]> = [];
  for (const block of options.blocks) {
    events.push([block.start, { type: `${block.kind}_start`, contentIndex: block.index }]);
    if (options.deltas !== false) events.push([block.start, { type: `${block.kind}_delta`, contentIndex: block.index, delta: "x" }]);
    if (block.end !== undefined) events.push([block.end, { type: `${block.kind}_end`, contentIndex: block.index }]);
  }
  events.sort((a, b) => a[0] - b[0]);
  tracker.observe({ type: "message_start", message }, 0);
  for (const [at, assistantMessageEvent] of events) tracker.observe({ type: "message_update", message, assistantMessageEvent }, at);
  tracker.observe({ type: "message_end", message }, endAt);
}

test("reasoning breakdown: counts output minus reasoning over text and toolcall spans only", () => {
  const tracker = createOutputRateTracker();
  feed(tracker, {
    output: 600, reasoning: 200,
    blocks: [
      { kind: "thinking", index: 0, start: 0, end: 5_000 },
      { kind: "text", index: 1, start: 6_000, end: 7_000 },
      { kind: "toolcall", index: 2, start: 9_000, end: 10_000 },
    ],
  });
  assert.equal(tracker.rate(), 200, "400 tokens over 2s of visible spans; the gaps and the thinking span never count");
});

test("no reasoning breakdown: counts all output over every block span, thinking included", () => {
  const tracker = createOutputRateTracker();
  feed(tracker, {
    output: 600,
    blocks: [
      { kind: "thinking", index: 0, start: 0, end: 1_000 },
      { kind: "text", index: 1, start: 3_000, end: 5_000 },
    ],
  });
  assert.equal(tracker.rate(), 200);
});

test("a no-delta thinking bracket counts its span only without a reasoning breakdown", () => {
  const withBreakdown = createOutputRateTracker();
  feed(withBreakdown, { output: 300, reasoning: 200, deltas: false, blocks: [{ kind: "thinking", index: 0, start: 0, end: 9_000 }, { kind: "text", index: 1, start: 9_000, end: 10_000 }] });
  assert.equal(withBreakdown.rate(), 100);
  const without = createOutputRateTracker();
  feed(without, { output: 300, deltas: false, blocks: [{ kind: "thinking", index: 0, start: 0, end: 2_000 }, { kind: "text", index: 1, start: 2_000, end: 3_000 }] });
  assert.equal(without.rate(), 100);
});

test("interleaved blocks pair by contentIndex and sum per block", () => {
  const tracker = createOutputRateTracker();
  // text 0 [0,2000] overlaps toolcall 1 [1000,3000]; thinking 2 sits between with reasoning excluded.
  feed(tracker, {
    output: 450, reasoning: 50,
    blocks: [
      { kind: "text", index: 0, start: 0, end: 2_000 },
      { kind: "toolcall", index: 1, start: 1_000, end: 3_000 },
      { kind: "thinking", index: 2, start: 1_500, end: 2_500 },
    ],
  });
  assert.equal(tracker.rate(), 100, "400 tokens over 2s + 2s of block spans");
});

test("error and aborted messages are skipped", () => {
  const tracker = createOutputRateTracker();
  feed(tracker, { output: 100, blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }], stopReason: "error" });
  feed(tracker, { output: 100, blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }], stopReason: "aborted" });
  assert.equal(tracker.rate(), undefined);
  feed(tracker, { output: 100, blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }] });
  assert.equal(tracker.rate(), 100);
});

test("a counted block left open by message_end makes the message ineligible", () => {
  const tracker = createOutputRateTracker();
  feed(tracker, { output: 100, blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }, { kind: "toolcall", index: 1, start: 1_000 }] });
  assert.equal(tracker.rate(), undefined);
  // An open thinking block is uncounted when the breakdown excludes reasoning.
  feed(tracker, { output: 150, reasoning: 50, blocks: [{ kind: "thinking", index: 0, start: 0 }, { kind: "text", index: 1, start: 0, end: 1_000 }] });
  assert.equal(tracker.rate(), 100);
});

test("a model change resets the window; ineligible messages neither enter nor reset it", () => {
  const tracker = createOutputRateTracker();
  feed(tracker, { output: 100, model: "a", blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }] });
  feed(tracker, { output: 300, model: "a", blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }] });
  assert.equal(tracker.rate(), 200);
  feed(tracker, { output: 900, model: "b", stopReason: "error", blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }] });
  assert.equal(tracker.rate(), 200, "an ineligible message from another model does not reset");
  feed(tracker, { output: 50, model: "b", blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }] });
  assert.equal(tracker.rate(), 50);
  feed(tracker, { output: 70, model: "b", provider: "other", blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }] });
  assert.equal(tracker.rate(), 70, "provider is part of the model identity");
});

test("the window keeps the last 8 eligible messages, token-weighted", () => {
  const tracker = createOutputRateTracker();
  feed(tracker, { output: 10_000, blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }] });
  for (let i = 0; i < OUTPUT_RATE_WINDOW - 1; i++) feed(tracker, { output: 100, blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }] });
  assert.equal(tracker.rate(), (10_000 + 700) / 8);
  feed(tracker, { output: 300, blocks: [{ kind: "text", index: 0, start: 0, end: 3_000 }] });
  assert.equal(tracker.rate(), (700 + 300) / 10, "the oldest message left; the mean is token-weighted, not a mean of rates");
});

test("eligibility guards: span under 250 ms and tokens under 8", () => {
  const tracker = createOutputRateTracker();
  feed(tracker, { output: 5_000, blocks: [{ kind: "toolcall", index: 0, start: 0, end: 249 }] });
  feed(tracker, { output: 7, blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }] });
  feed(tracker, { output: 120, reasoning: 113, blocks: [{ kind: "text", index: 0, start: 0, end: 1_000 }] });
  assert.equal(tracker.rate(), undefined);
  feed(tracker, { output: 8, blocks: [{ kind: "text", index: 0, start: 0, end: 250 }] });
  assert.equal(tracker.rate(), 32);
});

test("non-assistant messages and stray events are ignored", () => {
  const tracker = createOutputRateTracker();
  tracker.observe({ type: "message_end", message: { role: "user", usage: { output: 100 } } }, 0);
  tracker.observe({ type: "tool_execution_start" }, 0);
  tracker.observe(undefined, 0);
  assert.equal(tracker.rate(), undefined);
  assert.equal(formatOutputRate(41.5), "42t/s");
  assert.equal(formatOutputRate(41.4), "41t/s");
});

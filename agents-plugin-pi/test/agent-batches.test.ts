/**
 * 261028 Phase 1: unit tests for `agent-batches.ts`, the pure per-agent tool
 * batch model behind the live agent widget's expanded mode. Everything here is
 * driven with plain event objects and explicit clocks; no registry, timer or
 * widget is involved.
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  cleanToolName,
  AGENT_BATCH_CAP,
  BATCH_ARGS_PREVIEW_CAP,
  BATCH_TOOL_NAME_CAP,
  batchView,
  batchViews,
  closeOpenBatch,
  createBatchBuffer,
  flattenArgsPreview,
  observeBatchEvent,
  type AgentBatchBuffer,
} from "../src/agent-batches.ts";

type Call = { id: string; name: string; arguments?: unknown };

/** An assistant `message_end` announcing `calls`. */
function messageEnd(...calls: Call[]) {
  return {
    type: "message_end",
    message: {
      role: "assistant",
      content: [{ type: "text", text: "thinking" }, ...calls.map((call) => ({ type: "toolCall", id: call.id, name: call.name, arguments: call.arguments ?? {} }))],
    },
  };
}
const start = (id: string, toolName: string, args?: unknown) => ({ type: "tool_execution_start", toolCallId: id, toolName, args });
const end = (id: string, extra: { result?: unknown; isError?: boolean } = {}) => ({ type: "tool_execution_end", toolCallId: id, ...extra });

describe("observeBatchEvent: batch boundaries", () => {
  test("an assistant message_end with tool calls opens a batch; one without makes none", () => {
    const buffer = createBatchBuffer();
    assert.equal(observeBatchEvent(buffer, { type: "message_end", message: { role: "assistant", content: [{ type: "text", text: "just words" }] } }, 100), false);
    assert.equal(observeBatchEvent(buffer, { type: "message_end", message: { role: "user", content: [{ type: "toolCall", id: "x", name: "read" }] } }, 100), false);
    assert.equal(observeBatchEvent(buffer, { type: "message_end", message: { role: "assistant", content: "plain string" } }, 100), false);
    assert.equal(observeBatchEvent(buffer, { type: "message_end" }, 100), false);
    assert.deepEqual(buffer.batches, []);
    assert.equal(buffer.nextSeq, 1, "a message without calls never consumes a sequence number");

    assert.equal(observeBatchEvent(buffer, messageEnd({ id: "c1", name: "read", arguments: { path: "a.ts" } }), 100), true);
    assert.equal(buffer.batches.length, 1);
    assert.equal(buffer.batches[0].seq, 1);
    assert.equal(buffer.batches[0].startedAt, 100);
    assert.equal(buffer.batches[0].endedAt, undefined);
    assert.deepEqual(buffer.batches[0].calls, [{ id: "c1", toolName: "read", argsPreview: "path: a.ts" }]);
  });

  test("unrelated events change nothing", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd({ id: "c1", name: "read" }), 1);
    for (const type of ["message_start", "message_update", "agent_start", "turn_start", "compaction_end"]) {
      assert.equal(observeBatchEvent(buffer, { type }, 2), false, type);
    }
    assert.equal(buffer.batches[0].endedAt, undefined);
  });

  test("the batch ends when every call has ended", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd({ id: "c1", name: "read" }, { id: "c2", name: "grep" }), 10);
    observeBatchEvent(buffer, start("c1", "read"), 11);
    observeBatchEvent(buffer, start("c2", "grep"), 12);
    assert.equal(observeBatchEvent(buffer, end("c1"), 20), true);
    assert.equal(buffer.batches[0].endedAt, undefined, "one call still in flight");
    assert.equal(observeBatchEvent(buffer, end("c2"), 30), true);
    assert.equal(buffer.batches[0].endedAt, 30);
    assert.equal(observeBatchEvent(buffer, end("c2"), 40), false, "an end for an already finished call is ignored");
    assert.equal(buffer.batches[0].endedAt, 30);
  });

  for (const type of ["turn_end", "agent_end", "agent_settled"]) {
    test(`${type} closes an open batch and is a no-op without one`, () => {
      const buffer = createBatchBuffer();
      assert.equal(observeBatchEvent(buffer, { type }, 5), false, "nothing open");
      observeBatchEvent(buffer, messageEnd({ id: "c1", name: "bash" }), 10);
      observeBatchEvent(buffer, start("c1", "bash"), 11);
      assert.equal(observeBatchEvent(buffer, { type }, 50), true);
      assert.equal(buffer.batches[0].endedAt, 50);
      assert.equal(batchView(buffer.batches[0]).running, undefined, "an aborted turn never leaves a running view");
      assert.equal(observeBatchEvent(buffer, { type }, 60), false, "already closed");
      assert.equal(buffer.batches[0].endedAt, 50);
    });
  }

  test("a new tool-call message_end closes a still-open batch and opens the next", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd({ id: "c1", name: "read" }), 10);
    observeBatchEvent(buffer, start("c1", "read"), 11);
    observeBatchEvent(buffer, messageEnd({ id: "c2", name: "edit" }), 40);
    assert.equal(buffer.batches.length, 2);
    assert.equal(buffer.batches[0].endedAt, 40);
    assert.equal(buffer.batches[1].seq, 2);
    assert.equal(buffer.batches[1].startedAt, 40);
    assert.equal(buffer.batches[1].endedAt, undefined);
  });

  test("a tool-free message_end does not close the open batch", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd({ id: "c1", name: "read" }), 10);
    observeBatchEvent(buffer, { type: "message_end", message: { role: "assistant", content: [{ type: "text", text: "x" }] } }, 20);
    assert.equal(buffer.batches[0].endedAt, undefined);
  });

  test("an unannounced tool_execution_start joins the open batch, or opens one", () => {
    const buffer = createBatchBuffer();
    assert.equal(observeBatchEvent(buffer, start("u1", "bash", { command: "ls" }), 10), true);
    assert.equal(buffer.batches.length, 1);
    assert.equal(buffer.batches[0].startedAt, 10);
    assert.deepEqual(buffer.batches[0].calls.map((call) => [call.id, call.toolName, call.argsPreview, call.startedAt]), [["u1", "bash", "command: ls", 10]]);
    observeBatchEvent(buffer, start("u2", "read"), 11);
    assert.equal(buffer.batches.length, 1, "joins the open batch");
    assert.equal(buffer.batches[0].calls.length, 2);
    assert.equal(observeBatchEvent(buffer, end("never-started"), 12), false, "an end for an unknown call changes nothing");
  });

  test("a start replaces the announced args preview when the event carries args", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd({ id: "c1", name: "read", arguments: { path: "old" } }), 10);
    observeBatchEvent(buffer, start("c1", "read", { path: "new" }), 11);
    assert.equal(buffer.batches[0].calls[0].argsPreview, "path: new");
    observeBatchEvent(buffer, messageEnd({ id: "c2", name: "read", arguments: { path: "kept" } }), 12);
    observeBatchEvent(buffer, start("c2", "read"), 13);
    assert.equal(buffer.batches[1].calls[0].argsPreview, "path: kept");
  });

  test("the end captures the result preview and error flag", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd({ id: "c1", name: "read" }, { id: "c2", name: "bash" }, { id: "c3", name: "grep" }), 10);
    for (const id of ["c1", "c2", "c3"]) observeBatchEvent(buffer, start(id, "x"), 11);
    observeBatchEvent(buffer, end("c1", { result: { content: [{ type: "text", text: "line one\nline   two" }, { type: "image" }] } }), 12);
    observeBatchEvent(buffer, end("c2", { result: { content: [{ type: "text", text: "boom" }] }, isError: true }), 13);
    observeBatchEvent(buffer, end("c3", { result: { code: 7 } }), 14);
    const [c1, c2, c3] = buffer.batches[0].calls;
    assert.equal(c1.resultPreview, "line one line two");
    assert.equal(c1.isError, false);
    assert.equal(c2.resultPreview, "boom");
    assert.equal(c2.isError, true);
    assert.equal(c3.resultPreview, '{"code":7}');
    assert.equal(c1.endedAt, 12);
  });

  test("closeOpenBatch reports whether anything changed", () => {
    const buffer = createBatchBuffer();
    assert.equal(closeOpenBatch(buffer, 1), false);
    observeBatchEvent(buffer, messageEnd({ id: "c1", name: "read" }), 2);
    assert.equal(closeOpenBatch(buffer, 9), true);
    assert.equal(buffer.batches[0].endedAt, 9);
    assert.equal(closeOpenBatch(buffer, 10), false);
    assert.equal(buffer.batches[0].endedAt, 9, "an already closed batch keeps its end time");
  });
});

describe("batchView: running projection", () => {
  test("before any call starts, the first unfinished call is reported with the batch start time", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd({ id: "c1", name: "read", arguments: { path: "a" } }, { id: "c2", name: "grep" }), 100);
    const view = batchView(buffer.batches[0]);
    assert.deepEqual(view.running, { toolName: "read", argsPreview: "path: a", startedAt: 100, done: 0, othersRunning: 0 });
    assert.equal(view.endedAt, undefined);
    assert.equal(view.callCount, 2);
  });

  test("parallel calls: the OLDEST in-flight call is reported with done and othersRunning", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd({ id: "a", name: "read", arguments: { path: "a" } }, { id: "b", name: "grep", arguments: { q: "b" } }, { id: "c", name: "bash", arguments: { command: "c" } }), 100);
    observeBatchEvent(buffer, start("a", "read"), 110);
    observeBatchEvent(buffer, start("b", "grep"), 120);
    observeBatchEvent(buffer, start("c", "bash"), 130);
    assert.deepEqual(batchView(buffer.batches[0]).running, { toolName: "read", argsPreview: "path: a", startedAt: 110, done: 0, othersRunning: 2 });

    observeBatchEvent(buffer, end("a"), 140);
    assert.deepEqual(batchView(buffer.batches[0]).running, { toolName: "grep", argsPreview: "q: b", startedAt: 120, done: 1, othersRunning: 1 });

    // Out of order: the oldest remaining call finishes last.
    observeBatchEvent(buffer, end("c"), 150);
    assert.deepEqual(batchView(buffer.batches[0]).running, { toolName: "grep", argsPreview: "q: b", startedAt: 120, done: 2, othersRunning: 0 });

    observeBatchEvent(buffer, end("b"), 160);
    const finished = batchView(buffer.batches[0]);
    assert.equal(finished.running, undefined);
    assert.equal(finished.endedAt, 160);
  });

  test("a call announced but not yet started is not counted as running", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd({ id: "a", name: "read" }, { id: "b", name: "grep" }), 100);
    observeBatchEvent(buffer, start("b", "grep"), 120);
    assert.deepEqual(batchView(buffer.batches[0]).running, { toolName: "grep", argsPreview: "", startedAt: 120, done: 0, othersRunning: 0 });
  });

  test("a view is exactly one of running or ended", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd({ id: "a", name: "read" }), 1);
    const running = batchView(buffer.batches[0]);
    assert.ok(running.running && running.endedAt === undefined);
    observeBatchEvent(buffer, { type: "turn_end" }, 2);
    const ended = batchView(buffer.batches[0]);
    assert.ok(!ended.running && ended.endedAt === 2);
  });
});

describe("batch buffer bounds", () => {
  test("the buffer keeps the newest AGENT_BATCH_CAP batches with monotonically increasing seq", () => {
    assert.equal(AGENT_BATCH_CAP, 5);
    const buffer: AgentBatchBuffer = createBatchBuffer();
    for (let i = 1; i <= 8; i++) observeBatchEvent(buffer, messageEnd({ id: `c${i}`, name: "read" }), i * 10);
    assert.equal(buffer.batches.length, AGENT_BATCH_CAP);
    assert.deepEqual(buffer.batches.map((batch) => batch.seq), [4, 5, 6, 7, 8]);
    assert.deepEqual(batchViews(buffer).map((view) => view.seq), [4, 5, 6, 7, 8], "views are oldest first");
    assert.equal(buffer.nextSeq, 9);
    observeBatchEvent(buffer, messageEnd({ id: "c9", name: "read" }), 90);
    assert.deepEqual(buffer.batches.map((batch) => batch.seq), [5, 6, 7, 8, 9]);
    assert.ok(buffer.batches.slice(0, -1).every((batch) => batch.endedAt !== undefined), "only the newest batch can be open");
  });

  test("batchViews of an absent buffer is empty", () => {
    assert.deepEqual(batchViews(undefined), []);
  });

  test("tool names are deduplicated in call order", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd(
      { id: "1", name: "read" }, { id: "2", name: "edit" }, { id: "3", name: "read" }, { id: "4", name: "bash" }, { id: "5", name: "edit" },
    ), 1);
    const view = batchView(buffer.batches[0]);
    assert.deepEqual(view.toolNames, ["read", "edit", "bash"]);
    assert.equal(view.callCount, 5, "callCount counts calls, not distinct names");
  });

  test("tool names cap at BATCH_TOOL_NAME_CAP", () => {
    assert.equal(BATCH_TOOL_NAME_CAP, 5);
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd(...["a", "b", "c", "d", "e", "f", "g"].map((name, i) => ({ id: String(i), name }))), 1);
    const view = batchView(buffer.batches[0]);
    assert.deepEqual(view.toolNames, ["a", "b", "c", "d", "e"]);
    assert.equal(view.callCount, 7);
  });
});

describe("flattenArgsPreview", () => {
  test("objects flatten to one `key: value, ...` line; strings raw, other values JSON", () => {
    assert.equal(flattenArgsPreview({ path: "src/a.ts", limit: 20, force: true, opts: { deep: [1, 2] } }), 'path: src/a.ts, limit: 20, force: true, opts: {"deep":[1,2]}');
  });

  test("scalars, arrays, and absent args", () => {
    assert.equal(flattenArgsPreview(undefined), "");
    assert.equal(flattenArgsPreview(null), "");
    assert.equal(flattenArgsPreview("ls -la"), "ls -la");
    assert.equal(flattenArgsPreview([1, "a"]), '[1,"a"]');
    assert.equal(flattenArgsPreview(42), "42");
  });

  test("whitespace and newlines collapse to single spaces", () => {
    assert.equal(flattenArgsPreview({ command: "echo  a\n\n  b\t\tc\r\nd", note: "  x  " }), "command: echo a b c d, note: x");
  });

  test("control characters are stripped", () => {
    assert.equal(flattenArgsPreview({ text: "a\u0000b\u0007c\u001b[31md\u007fe\u0085f" }), "text: abc[31mdef");
  });

  test("the result is capped at 200 characters with a trailing ellipsis", () => {
    assert.equal(BATCH_ARGS_PREVIEW_CAP, 200);
    const exact = "x".repeat(200);
    assert.equal(flattenArgsPreview(exact), exact, "exactly at the cap is untouched");
    const long = flattenArgsPreview("y".repeat(500));
    assert.equal([...long].length, 200);
    assert.ok(long.endsWith("…"));
    assert.equal(long, "y".repeat(199) + "…");
    const small = flattenArgsPreview("abcdefghij", 5);
    assert.equal(small, "abcd…");
  });

  test("an unserializable value does not throw", () => {
    const cyclic: Record<string, unknown> = {};
    cyclic.self = cyclic;
    assert.doesNotThrow(() => flattenArgsPreview({ cyclic }));
    assert.match(flattenArgsPreview({ cyclic }), /^cyclic: /);
  });
});

test("model-supplied tool names lose control characters before they are stored", () => {
  const buffer = createBatchBuffer();
  observeBatchEvent(buffer, { type: "message_end", message: { role: "assistant", content: [{ type: "toolCall", id: "c1", name: "gr\u001b[31mep", arguments: {} }] } }, 1);
  observeBatchEvent(buffer, { type: "tool_execution_start", toolCallId: "c2", toolName: "\u0007bash", args: {} }, 2);
  const names = buffer.batches.flatMap((batch) => batch.calls.map((call) => call.toolName));
  assert.ok(names.length >= 2);
  for (const name of names) assert.doesNotMatch(name, /[\u0000-\u001f\u007f-\u009f]/);
  assert.equal(cleanToolName("\u001b"), "tool");
});

// ---------------------------------------------------------------------------
// 261028 batch-row redesign: the generating phase and failure counts.
// ---------------------------------------------------------------------------

/** RPC-shaped assistant stream events: no `partial`, no `message` on updates. */
const assistantStart = { type: "message_start", message: { role: "assistant", content: [] } };
const update = (assistantMessageEvent: Record<string, unknown>) => ({ type: "message_update", assistantMessageEvent });
const toolcallStart = (toolName: string, contentIndex = 0) => update({ type: "toolcall_start", contentIndex, id: `id-${contentIndex}`, toolName });
const toolcallDelta = (delta: string, contentIndex = 0) => update({ type: "toolcall_delta", contentIndex, delta });
const generatingOf = (buffer: AgentBatchBuffer) => batchViews(buffer).at(-1)?.generating;

describe("generating phase", () => {
  test("an assistant message_start opens a generating view; user and toolResult message_starts do not", () => {
    const buffer = createBatchBuffer();
    assert.equal(observeBatchEvent(buffer, { type: "message_start", message: { role: "user", content: "hi" } }, 5), false);
    assert.equal(observeBatchEvent(buffer, { type: "message_start", message: { role: "toolResult", content: [] } }, 5), false);
    assert.deepEqual(batchViews(buffer), []);
    assert.equal(observeBatchEvent(buffer, assistantStart, 10), true);
    assert.deepEqual(batchViews(buffer), [{ seq: 1, toolNames: [], callCount: 0, failed: 0, startedAt: 10, generating: { kind: "thinking", phaseStartedAt: 10 } }]);
    assert.deepEqual(buffer.batches, [], "the generating message is held outside the batch list");
  });

  test("before the first content block the row reads `thinking`, and a following thinking block keeps that phase clock", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, assistantStart, 10);
    assert.equal(observeBatchEvent(buffer, update({ type: "thinking_start", contentIndex: 0 }), 14), false, "already thinking: nothing changes");
    assert.deepEqual(generatingOf(buffer), { kind: "thinking", phaseStartedAt: 10 });
  });

  test("phase transitions: thinking -> replying -> tool streaming -> execution -> finished", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, assistantStart, 10);
    observeBatchEvent(buffer, update({ type: "thinking_start", contentIndex: 0 }), 11);
    observeBatchEvent(buffer, update({ type: "thinking_delta", contentIndex: 0, delta: "hmm" }), 12);
    observeBatchEvent(buffer, update({ type: "thinking_end", contentIndex: 0, content: "hmm" }), 13);
    assert.equal(observeBatchEvent(buffer, update({ type: "text_start", contentIndex: 1 }), 20), true);
    assert.deepEqual(generatingOf(buffer), { kind: "replying", phaseStartedAt: 20 }, "elapsed restarts at the new phase");
    assert.equal(observeBatchEvent(buffer, update({ type: "text_delta", contentIndex: 1, delta: "ok" }), 21), false);
    assert.equal(observeBatchEvent(buffer, toolcallStart("write", 2), 30), true);
    assert.deepEqual(generatingOf(buffer), { kind: "tool", phaseStartedAt: 30, toolName: "write", chars: 0 });
    assert.equal(observeBatchEvent(buffer, toolcallDelta("{\"path\":\"a", 2), 31), true);
    observeBatchEvent(buffer, toolcallDelta(".ts\"}", 2), 32);
    assert.deepEqual(generatingOf(buffer), { kind: "tool", phaseStartedAt: 30, toolName: "write", chars: 15 });
    assert.equal(observeBatchEvent(buffer, update({ type: "toolcall_end", contentIndex: 2, toolCall: { type: "toolCall", id: "w", name: "write", arguments: {} } }), 33), false);
    assert.equal(generatingOf(buffer)?.chars, 15, "the count holds until the next block or message_end");

    assert.equal(observeBatchEvent(buffer, messageEnd({ id: "w", name: "write", arguments: { path: "a.ts" } }), 40), true);
    assert.equal(buffer.generating, undefined);
    assert.equal(buffer.batches.length, 1);
    assert.equal(buffer.batches[0].seq, 1, "the batch takes the sequence number the generating view carried");
    const views = batchViews(buffer);
    assert.equal(views.length, 1);
    assert.ok(views[0].running && !views[0].generating, "execution phase");
    observeBatchEvent(buffer, start("w", "write"), 41);
    observeBatchEvent(buffer, end("w"), 50);
    assert.equal(batchViews(buffer)[0].endedAt, 50, "finished");
  });

  test("several streamed tool calls: the row shows the call now streaming, its count covering that call alone", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, assistantStart, 10);
    observeBatchEvent(buffer, toolcallStart("read", 0), 11);
    observeBatchEvent(buffer, toolcallDelta("x".repeat(40), 0), 12);
    observeBatchEvent(buffer, toolcallStart("write", 1), 20);
    observeBatchEvent(buffer, toolcallDelta("y".repeat(7), 1), 21);
    assert.deepEqual(generatingOf(buffer), { kind: "tool", phaseStartedAt: 20, toolName: "write", chars: 7 });
  });

  test("a toolcall_start tool name is cleaned; a missing one reads `tool`; a delta outside tool streaming or without text changes nothing", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, assistantStart, 10);
    assert.equal(observeBatchEvent(buffer, toolcallDelta("early"), 11), false, "no tool call is streaming yet");
    observeBatchEvent(buffer, update({ type: "toolcall_start", contentIndex: 0 }), 12);
    assert.equal(generatingOf(buffer)?.toolName, "tool");
    observeBatchEvent(buffer, toolcallStart("wr\u001bite"), 13);
    assert.equal(generatingOf(buffer)?.toolName, "wr ite");
    assert.equal(observeBatchEvent(buffer, update({ type: "toolcall_delta", contentIndex: 0 }), 14), false);
    assert.equal(observeBatchEvent(buffer, toolcallDelta(""), 14), false);
    assert.equal(observeBatchEvent(buffer, update(null as never), 14), false);
  });

  test("message_update without a generating message changes nothing (it opens only on message_start)", () => {
    const buffer = createBatchBuffer();
    for (const event of [toolcallStart("write"), toolcallDelta("abc"), update({ type: "text_start", contentIndex: 0 })]) {
      assert.equal(observeBatchEvent(buffer, event, 5), false);
    }
    assert.deepEqual(batchViews(buffer), []);
  });

  test("a message ending with no tool calls removes the row and never evicts a retained batch", () => {
    const buffer = createBatchBuffer();
    for (let i = 1; i <= AGENT_BATCH_CAP; i++) observeBatchEvent(buffer, messageEnd({ id: `c${i}`, name: "read" }), i);
    observeBatchEvent(buffer, { type: "turn_end" }, 9);
    observeBatchEvent(buffer, assistantStart, 10);
    observeBatchEvent(buffer, update({ type: "text_start", contentIndex: 0 }), 11);
    assert.equal(batchViews(buffer).length, AGENT_BATCH_CAP + 1, "the generating view rides on top of a full history");
    assert.equal(observeBatchEvent(buffer, { type: "message_end", message: { role: "assistant", content: [{ type: "text", text: "done" }] } }, 20), true);
    assert.equal(buffer.generating, undefined);
    assert.deepEqual(batchViews(buffer).map((view) => view.seq), [1, 2, 3, 4, 5], "no batch was evicted and no sequence number consumed");
    assert.equal(buffer.nextSeq, AGENT_BATCH_CAP + 1);
  });

  for (const type of ["turn_end", "agent_end", "agent_settled"]) {
    test(`${type} drops a generating message`, () => {
      const buffer = createBatchBuffer();
      observeBatchEvent(buffer, assistantStart, 10);
      assert.equal(observeBatchEvent(buffer, { type }, 20), true);
      assert.deepEqual(batchViews(buffer), []);
    });
  }

  test("closeOpenBatch (a re-attach or launch end) drops a generating message and reports the change", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, assistantStart, 10);
    assert.equal(closeOpenBatch(buffer, 20), true);
    assert.equal(buffer.generating, undefined);
    assert.equal(closeOpenBatch(buffer, 30), false);
  });
});

describe("failure count", () => {
  test("failed counts error results, also while the batch still runs", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, messageEnd({ id: "a", name: "edit" }, { id: "b", name: "bash" }, { id: "c", name: "read" }), 10);
    for (const id of ["a", "b", "c"]) observeBatchEvent(buffer, start(id, "x"), 11);
    assert.equal(batchView(buffer.batches[0]).failed, 0);
    observeBatchEvent(buffer, end("a", { isError: true }), 20);
    const running = batchView(buffer.batches[0]);
    assert.equal(running.failed, 1);
    assert.ok(running.running, "a running batch stays running after a failure");
    observeBatchEvent(buffer, end("b"), 21);
    observeBatchEvent(buffer, end("c", { isError: true }), 22);
    assert.equal(batchView(buffer.batches[0]).failed, 2);
    assert.equal(batchView(buffer.batches[0]).endedAt, 22);
  });

  test("calls still unfinished when turn_end / agent_end / agent_settled closes the batch count as failed", () => {
    for (const type of ["turn_end", "agent_end", "agent_settled"]) {
      const buffer = createBatchBuffer();
      observeBatchEvent(buffer, messageEnd({ id: "a", name: "bash" }, { id: "b", name: "read" }, { id: "c", name: "grep" }), 10);
      observeBatchEvent(buffer, start("a", "bash"), 11);
      observeBatchEvent(buffer, start("b", "read"), 11);
      observeBatchEvent(buffer, end("b"), 12);
      assert.equal(batchView(buffer.batches[0]).failed, 0, `${type}: unfinished calls of a running batch are not failures yet`);
      observeBatchEvent(buffer, { type }, 20);
      assert.equal(batchView(buffer.batches[0]).failed, 2, `${type}: the in-flight call and the never-started one`);
    }
  });
});

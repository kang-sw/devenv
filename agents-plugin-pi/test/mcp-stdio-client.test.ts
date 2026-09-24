/**
 * Unit tests for mcp-stdio-client.ts's IO-free seams: JsonRpcLineBuffer
 * (newline framing + decode) and PendingRequestRegistry (request/response id
 * correlation). Both are exercised with synthetic Buffers/messages — no
 * subprocess is spawned.
 *
 * Covers the two live-discovered defects fixed in this pass:
 *   - a multibyte UTF-8 codepoint split across a chunk boundary must decode
 *     correctly (StringDecoder, not per-chunk Buffer#toString()).
 *   - responses arriving out of request order must settle the correct
 *     promise (live-probe-confirmed: ws-mcp does not guarantee response
 *     order matches request order).
 *
 * McpStdioClient's write-error ownership is driven through its
 * `spawnProcess` seam with a fake child, so exit/write-error order is
 * chosen by the test, never raced.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import type { ChildProcessWithoutNullStreams } from "node:child_process";
import {
  JsonRpcLineBuffer,
  McpStdioClient,
  PendingRequestRegistry,
  WRITE_ERROR_FALLBACK_MS,
} from "../src/mcp-stdio-client.ts";

describe("JsonRpcLineBuffer", () => {
  test("parses a single complete line in one chunk", () => {
    const messages: unknown[] = [];
    const buf = new JsonRpcLineBuffer((msg) => messages.push(msg));
    buf.feed(Buffer.from('{"jsonrpc":"2.0","id":1,"result":{}}\n'));
    assert.equal(messages.length, 1);
    assert.deepEqual(messages[0], { jsonrpc: "2.0", id: 1, result: {} });
  });

  test("parses a message split across two chunks", () => {
    const messages: unknown[] = [];
    const buf = new JsonRpcLineBuffer((msg) => messages.push(msg));
    const full = '{"jsonrpc":"2.0","id":1,"result":{"ok":true}}\n';
    const splitAt = 20;
    buf.feed(Buffer.from(full.slice(0, splitAt)));
    assert.equal(messages.length, 0, "must not emit until the newline arrives");
    buf.feed(Buffer.from(full.slice(splitAt)));
    assert.equal(messages.length, 1);
    assert.deepEqual(messages[0], { jsonrpc: "2.0", id: 1, result: { ok: true } });
  });

  test("parses multiple messages delivered in a single chunk", () => {
    const messages: unknown[] = [];
    const buf = new JsonRpcLineBuffer((msg) => messages.push(msg));
    buf.feed(
      Buffer.from(
        '{"jsonrpc":"2.0","id":1,"result":1}\n' +
          '{"jsonrpc":"2.0","id":2,"result":2}\n' +
          '{"jsonrpc":"2.0","id":3,"result":3}\n',
      ),
    );
    assert.equal(messages.length, 3);
    assert.deepEqual(
      messages.map((m) => (m as { id: number }).id),
      [1, 2, 3],
    );
  });

  test("decodes a multibyte UTF-8 codepoint split exactly across a chunk boundary", () => {
    const messages: unknown[] = [];
    const buf = new JsonRpcLineBuffer((msg) => messages.push(msg));
    // em-dash U+2014 is 3 bytes in UTF-8: 0xE2 0x80 0x94. Build a JSON line
    // containing it, then split the raw bytes so the split lands inside the
    // codepoint (after the first byte).
    const payload = JSON.stringify({ jsonrpc: "2.0", id: 1, result: { text: "before—after" } });
    const fullBuf = Buffer.from(`${payload}\n`, "utf8");
    const emDashByteOffset = fullBuf.indexOf(Buffer.from([0xe2, 0x80, 0x94]));
    assert.ok(emDashByteOffset > 0, "test setup: em-dash bytes must be present");
    const splitPoint = emDashByteOffset + 1; // split after the first byte of the 3-byte sequence
    buf.feed(fullBuf.subarray(0, splitPoint));
    buf.feed(fullBuf.subarray(splitPoint));
    assert.equal(messages.length, 1);
    assert.deepEqual(messages[0], { jsonrpc: "2.0", id: 1, result: { text: "before—after" } });
  });

  test("handles a multibyte split across three chunks (arrow + box-drawing)", () => {
    const messages: unknown[] = [];
    const buf = new JsonRpcLineBuffer((msg) => messages.push(msg));
    const text = "step → next ─── done";
    const payload = JSON.stringify({ jsonrpc: "2.0", id: 7, result: { text } });
    const fullBuf = Buffer.from(`${payload}\n`, "utf8");
    // Feed byte-by-byte to exercise every possible split point, including
    // mid-codepoint splits for every multibyte character in `text`.
    for (let i = 0; i < fullBuf.length; i++) {
      buf.feed(fullBuf.subarray(i, i + 1));
    }
    assert.equal(messages.length, 1);
    assert.deepEqual(messages[0], { jsonrpc: "2.0", id: 7, result: { text } });
  });

  test("reports a parse error for an invalid line without throwing", () => {
    const errors: string[] = [];
    const messages: unknown[] = [];
    const buf = new JsonRpcLineBuffer(
      (msg) => messages.push(msg),
      (line) => errors.push(line),
    );
    buf.feed(Buffer.from("not json at all\n"));
    assert.equal(messages.length, 0);
    assert.equal(errors.length, 1);
    assert.equal(errors[0], "not json at all");
  });

  test("ignores blank lines", () => {
    const messages: unknown[] = [];
    const buf = new JsonRpcLineBuffer((msg) => messages.push(msg));
    buf.feed(Buffer.from('\n\n{"jsonrpc":"2.0","id":1,"result":{}}\n\n'));
    assert.equal(messages.length, 1);
  });
});

describe("PendingRequestRegistry", () => {
  test("register() allocates monotonically increasing ids", () => {
    const registry = new PendingRequestRegistry();
    const id1 = registry.register(() => {}, () => {});
    const id2 = registry.register(() => {}, () => {});
    assert.equal(id2, id1 + 1);
  });

  test("settle() resolves the matching promise with the result", () => {
    const registry = new PendingRequestRegistry();
    let resolved: unknown;
    const id = registry.register((v) => (resolved = v), () => {});
    const matched = registry.settle({ jsonrpc: "2.0", id, result: { ok: true } });
    assert.equal(matched, true);
    assert.deepEqual(resolved, { ok: true });
    assert.equal(registry.size, 0);
  });

  test("settle() rejects the matching promise on a JSON-RPC error", () => {
    const registry = new PendingRequestRegistry();
    let rejected: unknown;
    const id = registry.register(() => {}, (e) => (rejected = e));
    registry.settle({ jsonrpc: "2.0", id, error: { code: -1, message: "boom" } });
    assert.ok(rejected instanceof Error);
    assert.match((rejected as Error).message, /boom/);
  });

  test("settle() returns false and does nothing for an unknown id", () => {
    const registry = new PendingRequestRegistry();
    const matched = registry.settle({ jsonrpc: "2.0", id: 999, result: {} });
    assert.equal(matched, false);
  });

  test("out-of-order responses settle the correct promise (live-probe-confirmed risk)", () => {
    const registry = new PendingRequestRegistry();
    const resolved: Record<number, unknown> = {};
    const idA = registry.register((v) => (resolved[idA] = v), () => {});
    const idB = registry.register((v) => (resolved[idB] = v), () => {});
    const idC = registry.register((v) => (resolved[idC] = v), () => {});

    // Responses arrive in reverse order: C, then A, then B.
    registry.settle({ jsonrpc: "2.0", id: idC, result: "result-C" });
    registry.settle({ jsonrpc: "2.0", id: idA, result: "result-A" });
    registry.settle({ jsonrpc: "2.0", id: idB, result: "result-B" });

    assert.equal(resolved[idA], "result-A");
    assert.equal(resolved[idB], "result-B");
    assert.equal(resolved[idC], "result-C");
    assert.equal(registry.size, 0);
  });

  test("cancel() removes a pending entry without settling it", () => {
    const registry = new PendingRequestRegistry();
    let called = false;
    const id = registry.register(
      () => (called = true),
      () => (called = true),
    );
    registry.cancel(id);
    assert.equal(registry.size, 0);
    const matched = registry.settle({ jsonrpc: "2.0", id, result: {} });
    assert.equal(matched, false);
    assert.equal(called, false);
  });

  test("rejectAll() rejects every still-pending call and clears the registry", () => {
    const registry = new PendingRequestRegistry();
    const rejections: unknown[] = [];
    registry.register(() => {}, (e) => rejections.push(e));
    registry.register(() => {}, (e) => rejections.push(e));
    const err = new Error("subprocess died");
    registry.rejectAll(err);
    assert.equal(rejections.length, 2);
    assert.equal(rejections[0], err);
    assert.equal(rejections[1], err);
    assert.equal(registry.size, 0);
  });

  test("reject() rejects one pending id once and reports whether it was pending", () => {
    const registry = new PendingRequestRegistry();
    const rejections: unknown[] = [];
    const id = registry.register(() => {}, (e) => rejections.push(e));
    const err = new Error("write EPIPE");
    assert.equal(registry.reject(id, err), true);
    assert.equal(registry.reject(id, err), false);
    assert.deepEqual(rejections, [err]);
    assert.equal(registry.size, 0);
  });
});

/**
 * Fake child process for McpStdioClient's `spawnProcess` seam: the test
 * holds each stdin write callback and decides when (and with what error) it
 * runs, and emits `exit`/`error` itself, so event order is driven, not raced.
 */
function fakeChild() {
  const writes: Array<(err?: Error | null) => void> = [];
  const proc = new EventEmitter() as EventEmitter & {
    stdin: EventEmitter & { write: (data: string, cb: (err?: Error | null) => void) => boolean; end: () => void };
    stdout: EventEmitter;
    stderr: EventEmitter;
    kill: () => boolean;
  };
  proc.stdin = Object.assign(new EventEmitter(), {
    write: (_data: string, cb: (err?: Error | null) => void) => {
      writes.push(cb);
      return true;
    },
    end: () => {},
  });
  proc.stdout = new EventEmitter();
  proc.stderr = new EventEmitter();
  proc.kill = () => true;
  const client = new McpStdioClient("launcher", [], {
    cwd: "/",
    onStderr: () => {},
    spawnProcess: () => proc as unknown as ChildProcessWithoutNullStreams,
  });
  return { proc, writes, client };
}

function epipe(): Error {
  return Object.assign(new Error("write EPIPE"), { code: "EPIPE" });
}

/** Records how a promise settles without letting a rejection go unhandled. */
function observe(promise: Promise<unknown>) {
  const state: { settled: "pending" | "resolved" | "rejected"; value?: unknown } = { settled: "pending" };
  promise.then(
    (value) => Object.assign(state, { settled: "resolved", value }),
    (value) => Object.assign(state, { settled: "rejected", value }),
  );
  return state;
}

const flush = () => new Promise<void>((resolve) => setImmediate(resolve));

describe("McpStdioClient write-error ownership", () => {
  const EXIT_MESSAGE = "ws-mcp process exited unexpectedly (code=1, signal=null)";

  test("write error first, then exit: rejects with the exit-coded message", async (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const { proc, writes, client } = fakeChild();
    const state = observe(client.initialize({ name: "t", version: "0" }));
    assert.equal(writes.length, 1);
    writes[0](epipe());
    proc.emit("exit", 1, null);
    // Running past the fallback bound cannot re-settle the call with the write error.
    t.mock.timers.tick(WRITE_ERROR_FALLBACK_MS * 2);
    await flush();
    assert.equal(state.settled, "rejected");
    assert.equal((state.value as Error).message, EXIT_MESSAGE);
  });

  test("exit first, then write error: same exit message, the late write error is inert", async (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const { proc, writes, client } = fakeChild();
    const state = observe(client.initialize({ name: "t", version: "0" }));
    proc.emit("exit", 1, null);
    writes[0](epipe());
    t.mock.timers.tick(WRITE_ERROR_FALLBACK_MS * 2);
    await flush();
    assert.equal(state.settled, "rejected");
    assert.equal((state.value as Error).message, EXIT_MESSAGE);
  });

  test("write error with no exit/error rejects with the write error at the bound", async (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const { proc, writes, client } = fakeChild();
    const state = observe(client.initialize({ name: "t", version: "0" }));
    const err = epipe();
    writes[0](err);
    t.mock.timers.tick(WRITE_ERROR_FALLBACK_MS - 1);
    await flush();
    assert.equal(state.settled, "pending", "the exit/error handler still owns the rejection inside the bound");
    t.mock.timers.tick(1);
    await flush();
    assert.equal(state.settled, "rejected");
    assert.equal(state.value, err);
    // A later exit finds nothing pending: no second settle.
    proc.emit("exit", 1, null);
    await flush();
    assert.equal(state.value, err);
  });

  test("write error, then a spawn 'error' event: rejects with the failed-to-start message", async (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const { proc, writes, client } = fakeChild();
    const call = client.initialize({ name: "t", version: "0" });
    writes[0](epipe());
    proc.emit("error", new Error("spawn python3 ENOENT"));
    await assert.rejects(call, { message: "ws-mcp process failed to start: spawn python3 ENOENT" });
    t.mock.timers.tick(WRITE_ERROR_FALLBACK_MS * 2);
  });
});

/** Replay contract for the incremental child-session reader
 * (260926-bug-pi-telemetry-refresh-full-session-reparse): the full
 * `readSessionEntries` is the oracle after every read, a refresh reads only
 * the bytes appended since the previous one, an invalid verdict is cached,
 * retained state holds only entry projections, and the reader lives and dies
 * with its registry record. */
import assert from "node:assert/strict";
import { appendFileSync, mkdtempSync, rmSync, truncateSync, unlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, test } from "node:test";
import { IncrementalSessionReader, nodeSessionFileIo, projectEntry, readSessionEntries, type SessionEntriesRead, type SessionFileIo } from "../src/agent-telemetry.ts";
import { allocateAgentHome, createAgentStorageContext, persistOwnershipTelemetry, removeOwnedAgentHome, readOwnership, updateOwnership } from "../src/agent-storage.ts";
import { descendantUsageValue, registerAgentCostOwner, retentionEvictionCost } from "../src/agent-cost.ts";
import { evictForCapacity, refreshAgentTelemetry, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";

const roots = new Set<string>();
afterEach(() => { for (const root of roots) rmSync(root, { recursive: true, force: true }); roots.clear(); });
function root(): string { const value = mkdtempSync(join(tmpdir(), "ws-pi-telemetry-incr-")); roots.add(value); return value; }
function sessionPath(name = "session.jsonl"): string { return join(root(), name); }

const header = (id = "child", extra: Record<string, unknown> = {}) => ({ type: "session", version: 3, id, timestamp: "t", cwd: "/", ...extra });
const assistant = (id: string, input: number, cost: number, content: unknown = "same") => ({ type: "message", id, parentId: null, timestamp: "t", message: { role: "assistant", content, usage: { input, cost: { total: cost } } } });
const toolResult = (id: string, text: string) => ({ type: "message", id, parentId: null, timestamp: "t", message: { role: "toolResult", toolName: "read", content: [{ type: "text", text }] } });
const line = (value: unknown): string => `${JSON.stringify(value)}\n`;
const lines = (values: unknown[]): string => values.map(line).join("");

type Kind = "entries" | "transient" | "invalid";
const kindOf = (read: SessionEntriesRead): Kind => read === undefined ? "invalid" : "transient" in read ? "transient" : "entries";

/** Reads once through the reader and once through the oracle on the same file state; they must agree. */
function check(reader: IncrementalSessionReader, path: string, label: string, expected?: Kind): SessionEntriesRead {
  const incremental = reader.read(path);
  const oracle = readSessionEntries(path);
  assert.equal(kindOf(incremental), kindOf(oracle), `${label}: classification`);
  if (oracle && !("transient" in oracle)) {
    assert.ok(incremental && !("transient" in incremental));
    assert.equal(incremental.headerId, oracle.headerId, `${label}: headerId`);
    assert.equal(incremental.parentSession, oracle.parentSession, `${label}: parentSession`);
    assert.equal(Object.hasOwn(incremental, "parentSession"), Object.hasOwn(oracle, "parentSession"), `${label}: parentSession presence`);
    assert.deepEqual(incremental.entries, oracle.entries.map(projectEntry), `${label}: projected entries`);
  }
  if (expected) assert.equal(kindOf(incremental), expected, `${label}: expected ${expected}`);
  return incremental;
}

/** Deterministic PRNG so a failing chunking replays exactly. */
function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => { a = (a + 0x6d2b79f5) >>> 0; let t = a; t = Math.imul(t ^ (t >>> 15), t | 1); t ^= t + Math.imul(t ^ (t >>> 7), t | 61); return ((t ^ (t >>> 14)) >>> 0) / 4294967296; };
}

/** A realistic session: multi-byte text, usage variants, identical replays, summaries. */
function realisticSession(count: number, rand: () => number): Buffer {
  const values: unknown[] = [header("child", { parentSession: "/parent.jsonl" })];
  for (let i = 0; i < count; i++) {
    const r = rand();
    if (r < .35) values.push(assistant(`a${i}`, Math.floor(rand() * 5000), Math.round(rand() * 1000) / 1000, `응답 ${i} — café ☕ 🎉 ${"x".repeat(Math.floor(rand() * 400))}`));
    else if (r < .6) values.push(toolResult(`t${i}`, `결과 ${"ÿ€😀".repeat(Math.floor(rand() * 200))}`));
    else if (r < .7) values.push({ type: "compaction", id: `c${i}`, parentId: null, timestamp: "t", summary: "요약", usage: { input: 10, cost: { total: .01 } } });
    else if (r < .8) values.push({ type: "custom", id: `u${i}`, parentId: null, timestamp: "t", data: { note: "ünïcödé" } });
    else if (r < .9 && values.length > 1) values.push(values[1 + Math.floor(rand() * (values.length - 1))]); // identical replay: a duplicate
    else values.push({ type: "message", id: `m${i}`, parentId: null, timestamp: "t", message: { role: "user", content: "질문" } });
  }
  return Buffer.from(lines(values), "utf8");
}

/** Every open and every byte range a reader requests. */
function recordingIo(): { io: SessionFileIo; reads: { start: number; length: number }[]; opens: number } {
  const log = { reads: [] as { start: number; length: number }[], opens: 0, io: undefined as unknown as SessionFileIo };
  log.io = {
    open(path) {
      log.opens++;
      const file = nodeSessionFileIo.open(path);
      if (!file) return undefined;
      return { size: file.size, read(start, length) { log.reads.push({ start, length }); return file.read(start, length); }, close() { file.close(); } };
    },
  };
  return log;
}
const bytesRead = (reads: { length: number }[]): number => reads.reduce((total, r) => total + r.length, 0);

describe("incremental session reader: replay oracle", () => {
  test("seeded pseudo-random chunked appends, torn at arbitrary bytes including inside multi-byte characters", () => {
    for (const seed of [1, 7, 42, 2026]) {
      const rand = mulberry32(seed);
      const full = realisticSession(80, rand);
      const path = sessionPath(`seed-${seed}.jsonl`);
      const reader = new IncrementalSessionReader();
      // Cut points: random strides, plus every position inside the first few multi-byte characters.
      const cuts = new Set<number>();
      for (let at = 0; at < full.length;) { at += 1 + Math.floor(rand() * 400); cuts.add(Math.min(at, full.length)); }
      let multibyte = 0;
      for (let i = 0; i < full.length && multibyte < 6; i++) if (full[i] >= 0xc0) { cuts.add(i + 1); cuts.add(i + 2); multibyte++; }
      for (let i = 0; i < full.length; i++) if (full[i] === 0x0a && rand() < .15) { cuts.add(i); cuts.add(i + 1); } // exactly before and after "\n"
      const ordered = [...cuts].filter(c => c > 0).sort((a, b) => a - b);
      writeFileSync(path, "");
      check(reader, path, `seed ${seed} empty`, "invalid");
      let written = 0, reads = 0;
      for (const cut of ordered) {
        appendFileSync(path, full.subarray(written, cut));
        written = cut;
        check(reader, path, `seed ${seed} at byte ${cut}/${full.length}`);
        reads++;
      }
      assert.ok(reads > 100, `seed ${seed}: enough replay points (${reads})`);
      check(reader, path, `seed ${seed} complete`, "entries");
    }
  });

  test("torn mid-line writes read transient until the line completes", () => {
    const path = sessionPath(), reader = new IncrementalSessionReader();
    const next = line(assistant("b", 20, .2));
    writeFileSync(path, lines([header(), assistant("a", 10, .1)]));
    check(reader, path, "complete", "entries");
    appendFileSync(path, next.slice(0, 5));
    check(reader, path, "torn after 5 chars", "transient");
    appendFileSync(path, next.slice(5, 30));
    check(reader, path, "torn after 30 chars", "transient");
    appendFileSync(path, next.slice(30));
    check(reader, path, "completed", "entries");
    // A torn header.
    const other = sessionPath("other.jsonl"), fresh = new IncrementalSessionReader();
    writeFileSync(other, line(header()).slice(0, 12));
    check(fresh, other, "torn header", "transient");
    appendFileSync(other, line(header()).slice(12));
    check(fresh, other, "header only", "entries");
  });

  test("a tear exactly before a line's newline is a parseable unterminated tail", () => {
    const path = sessionPath(), reader = new IncrementalSessionReader();
    writeFileSync(path, line(header()) + JSON.stringify(assistant("a", 10, .1)));
    const read = check(reader, path, "unterminated tail", "entries");
    assert.ok(read && !("transient" in read)); assert.equal(read.entries.length, 1);
    appendFileSync(path, "\n");
    check(reader, path, "terminated", "entries");
    appendFileSync(path, JSON.stringify(assistant("b", 20, .2)));
    check(reader, path, "second unterminated tail", "entries");
    appendFileSync(path, `\n${line(assistant("c", 30, .3))}`);
    check(reader, path, "three entries", "entries");
    // An unterminated header only.
    const other = sessionPath("h.jsonl"), fresh = new IncrementalSessionReader();
    writeFileSync(other, JSON.stringify(header()));
    check(fresh, other, "unterminated header", "entries");
  });

  test("a torn line repaired by appending a newline reads transient, then invalid once a later line exists", () => {
    const path = sessionPath(), reader = new IncrementalSessionReader();
    writeFileSync(path, lines([header(), assistant("a", 10, .1)]) + line(assistant("b", 20, .2)).slice(0, 17));
    check(reader, path, "torn", "transient");
    appendFileSync(path, "\n");
    check(reader, path, "repaired: malformed terminated last line", "transient");
    check(reader, path, "repaired, unchanged", "transient");
    appendFileSync(path, line(assistant("c", 30, .3)));
    check(reader, path, "later line after the malformed one", "invalid");
    appendFileSync(path, line(assistant("d", 40, .4)));
    check(reader, path, "still invalid", "invalid");
  });

  test("a missing file reads transient and then recovers", () => {
    const path = sessionPath(), reader = new IncrementalSessionReader();
    check(reader, path, "missing", "transient");
    writeFileSync(path, lines([header(), assistant("a", 10, .1)]));
    check(reader, path, "created", "entries");
    appendFileSync(path, line(assistant("b", 20, .2)));
    check(reader, path, "appended", "entries");
    unlinkSync(path);
    check(reader, path, "deleted", "transient");
    writeFileSync(path, lines([header("other"), assistant("x", 1, .01)]));
    check(reader, path, "recreated with another session", "entries");
  });

  test("a truncate-and-rewrite to a smaller size resets", () => {
    const path = sessionPath(), reader = new IncrementalSessionReader();
    writeFileSync(path, lines([header(), assistant("a", 10, .1), assistant("b", 20, .2), assistant("c", 30, .3)]));
    check(reader, path, "long", "entries");
    writeFileSync(path, lines([header(), assistant("z", 5, .05)]));
    check(reader, path, "rewritten smaller, same header", "entries");
    appendFileSync(path, line(assistant("y", 6, .06)));
    check(reader, path, "appended after rewrite", "entries");
    truncateSync(path, 0);
    check(reader, path, "truncated to empty", "invalid");
    writeFileSync(path, lines([header("next"), assistant("n", 1, .01)]));
    check(reader, path, "rewritten from empty", "entries");
    // Truncated exactly to the consumed offset (header plus one line), leaving no unconsumed tail.
    const consumed = Buffer.byteLength(lines([header("next")]));
    appendFileSync(path, line(assistant("o", 2, .02)));
    check(reader, path, "three lines", "entries");
    truncateSync(path, consumed + Buffer.byteLength(line(assistant("n", 1, .01))));
    check(reader, path, "truncated to the consumed boundary", "entries");
    truncateSync(path, consumed);
    check(reader, path, "truncated to the header", "entries");
  });

  test("a same-size or larger rewrite that changes only the header line resets (migration shape)", () => {
    const path = sessionPath(), reader = new IncrementalSessionReader();
    const body = lines([assistant("a", 10, .1), assistant("b", 20, .2)]);
    writeFileSync(path, line({ ...header(), version: 2 }) + body);
    check(reader, path, "version 2", "invalid");
    check(reader, path, "version 2 again", "invalid");
    writeFileSync(path, line(header()) + body); // same size: "2" -> "3"
    check(reader, path, "migrated in place to version 3", "entries");
    writeFileSync(path, line({ ...header(), version: 2 }) + body);
    check(reader, path, "back to version 2, same size", "invalid");
    writeFileSync(path, line(header("child", { parentSession: "/p.jsonl" })) + body);
    check(reader, path, "larger header with parentSession", "entries");
    writeFileSync(path, line(header("child", { parentSession: "/q.jsonl" })) + body);
    check(reader, path, "same-size header, different parentSession", "entries");
    writeFileSync(path, line(header("child", { parentSession: "/q.jsonl", note: "extra" })) + body + line(assistant("c", 1, .01)));
    check(reader, path, "larger header and appended entry", "entries");
  });

  test("a path change resets, and returning to the first path reads it afresh", () => {
    const a = sessionPath("a.jsonl"), b = sessionPath("b.jsonl"), reader = new IncrementalSessionReader();
    writeFileSync(a, lines([header("A"), assistant("a1", 10, .1), assistant("a2", 20, .2)]));
    writeFileSync(b, lines([header("B", { parentSession: a }), assistant("b1", 30, .3)]));
    check(reader, a, "path a", "entries");
    check(reader, b, "path b", "entries");
    appendFileSync(a, line(assistant("a3", 1, .01)));
    check(reader, a, "back to a after an append", "entries");
    check(reader, sessionPath("missing.jsonl"), "a missing third path", "transient");
    check(reader, b, "back to b", "entries");
  });

  test("same-id lines: identical or deep-equal is a duplicate, different content is a contradiction", () => {
    // Identical raw line.
    let path = sessionPath("same.jsonl"), reader = new IncrementalSessionReader();
    writeFileSync(path, lines([header(), assistant("a", 10, .1)]));
    check(reader, path, "one entry", "entries");
    appendFileSync(path, line(assistant("a", 10, .1)));
    check(reader, path, "identical replay as last line", "entries");
    appendFileSync(path, line(assistant("b", 20, .2)));
    const read = check(reader, path, "identical replay consumed", "entries");
    assert.ok(read && !("transient" in read)); assert.deepEqual(read.entries.map(e => e.id), ["a", "b"]);

    // Different raw text, deep-equal value (key order).
    path = sessionPath("order.jsonl"); reader = new IncrementalSessionReader();
    const original = assistant("a", 10, .1);
    const reordered = { message: original.message, timestamp: original.timestamp, parentId: original.parentId, id: original.id, type: original.type };
    assert.notEqual(JSON.stringify(reordered), JSON.stringify(original));
    writeFileSync(path, lines([header(), original]));
    check(reader, path, "original", "entries");
    appendFileSync(path, line(reordered));
    check(reader, path, "reordered duplicate as last line", "entries");
    check(reader, path, "reordered duplicate unchanged", "entries");
    appendFileSync(path, line(assistant("b", 20, .2)));
    check(reader, path, "reordered duplicate consumed", "entries");

    // Different content: conflicting duplicate as last line, then consumed.
    path = sessionPath("conflict.jsonl"); reader = new IncrementalSessionReader();
    writeFileSync(path, lines([header(), assistant("a", 10, .1), assistant("b", 20, .2)]));
    check(reader, path, "before conflict", "entries");
    appendFileSync(path, line(assistant("a", 11, .1)));
    check(reader, path, "conflicting duplicate as last line", "invalid");
    check(reader, path, "conflicting duplicate unchanged", "invalid");
    appendFileSync(path, line(assistant("c", 30, .3)));
    check(reader, path, "conflicting duplicate consumed", "invalid");

    // Conflict in the same batch of newly appended lines as the first occurrence.
    path = sessionPath("batch.jsonl"); reader = new IncrementalSessionReader();
    writeFileSync(path, line(header()));
    check(reader, path, "header only", "entries");
    appendFileSync(path, lines([assistant("x", 1, .01), assistant("x", 2, .01), assistant("y", 3, .03)]));
    check(reader, path, "first occurrence and conflict appended together", "invalid");
  });

  test("absent, null, and present message, message.usage, and usage project exactly", () => {
    const path = sessionPath(), reader = new IncrementalSessionReader();
    const variants: unknown[] = [
      { type: "custom", id: "no-message" },
      { type: "message", id: "null-message", message: null },
      { type: "message", id: "string-message", message: "text" },
      { type: "message", id: "empty-message", message: {} },
      { type: "message", id: "no-usage", message: { role: "assistant", content: "c" } },
      { type: "message", id: "null-usage", message: { role: "assistant", usage: null } },
      { type: "message", id: "undefined-role", message: { usage: { input: 1 } } },
      { type: "compaction", id: "top-usage", usage: { input: 2, cost: { total: .1 } } },
      { type: "compaction", id: "null-top-usage", usage: null },
      { type: "message", id: "both-usages", message: { role: "assistant", usage: { input: 3 } }, usage: { input: 4 } },
      { type: "message", id: "null-message-top-usage", message: null, usage: { input: 5 } },
    ];
    writeFileSync(path, line(header()));
    check(reader, path, "header", "entries");
    for (const [i, v] of variants.entries()) {
      appendFileSync(path, line(v));
      check(reader, path, `variant ${i} as last line`, "entries");
    }
    appendFileSync(path, line({ type: "custom", id: "tail" }));
    const read = check(reader, path, "every variant consumed", "entries");
    assert.ok(read && !("transient" in read));
    const byId = new Map(read.entries.map(e => [e.id, e]));
    assert.equal(Object.hasOwn(byId.get("no-message")!, "message"), false);
    assert.equal(byId.get("null-message")!.message, null);
    assert.equal(Object.hasOwn(byId.get("no-usage")!.message!, "usage"), false);
    assert.equal(byId.get("null-usage")!.message!.usage, null);
    assert.equal(byId.get("null-top-usage")!.usage, null);
  });

  test("a malformed consumed line followed by a malformed last line stays invalid", () => {
    const path = sessionPath(), reader = new IncrementalSessionReader();
    writeFileSync(path, lines([header(), assistant("a", 10, .1)]) + "{not json\n");
    check(reader, path, "malformed last line", "transient");
    appendFileSync(path, "{also not json");
    check(reader, path, "malformed consumed then malformed torn last", "invalid");
    appendFileSync(path, "\n");
    check(reader, path, "malformed consumed then malformed terminated last", "invalid");
    appendFileSync(path, line(assistant("b", 20, .2)));
    check(reader, path, "then a valid line", "invalid");
    // The same shape written at once to a fresh reader.
    const fresh = new IncrementalSessionReader();
    check(fresh, path, "fresh reader", "invalid");
  });

  test("an entry without an id followed by a torn last line reads transient, then invalid", () => {
    const path = sessionPath(), reader = new IncrementalSessionReader();
    writeFileSync(path, lines([header(), assistant("a", 10, .1), { type: "message", message: { role: "user" } }]));
    check(reader, path, "id-less entry as last line", "invalid");
    appendFileSync(path, line(assistant("b", 20, .2)).slice(0, 9));
    check(reader, path, "id-less entry then torn last line", "transient");
    appendFileSync(path, line(assistant("b", 20, .2)).slice(9));
    check(reader, path, "torn line completed", "invalid");
    // Other readable contradictions before a torn tail: a bad header and an empty id.
    const other = sessionPath("bad-header.jsonl"), fresh = new IncrementalSessionReader();
    writeFileSync(other, line({ type: "session", version: 3, id: "" }) + line(assistant("a", 1, .01)) + "{\"torn");
    check(fresh, other, "bad header then torn tail", "transient");
    appendFileSync(other, "\":1}\n");
    check(fresh, other, "bad header then complete line", "invalid");
    const third = sessionPath("empty-id.jsonl"), again = new IncrementalSessionReader();
    writeFileSync(third, lines([header(), { type: "custom", id: "" }, 42, null]) + "[");
    check(again, third, "empty id, number, null, then torn tail", "transient");
  });
});

describe("incremental session reader: cost", () => {
  function largeSession(path: string, count = 200): void {
    const big = "p".repeat(20_000);
    writeFileSync(path, lines([header(), ...Array.from({ length: count }, (_, i) => i % 2 ? assistant(`a${i}`, i, .001) : toolResult(`t${i}`, big)), assistant("last", 1, .001)]));
  }
  const headerLength = Buffer.byteLength(line(header()));

  test("a refresh reads only from the consumed offset, bounded by the appended bytes", () => {
    const path = sessionPath(), log = recordingIo(), reader = new IncrementalSessionReader(log.io);
    largeSession(path);
    check(reader, path, "initial", "entries");
    const sizeBefore = Buffer.byteLength(readFileContent(path));
    assert.ok(sizeBefore > 2_000_000, "the session is large");
    const lastLine = Buffer.byteLength(line(assistant("last", 1, .001)));
    const consumed = sizeBefore - lastLine;
    const appended = line(assistant("new", 2, .002));
    for (let round = 0; round < 3; round++) {
      log.reads.length = 0;
      if (round < 2) appendFileSync(path, appended); // the third round refreshes an unchanged file
      check(reader, path, `refresh ${round}`, "entries");
      for (const r of log.reads) {
        const boundary = r.length === 1;
        const headerRead = r.start === 0 && r.length <= headerLength;
        assert.ok(boundary || headerRead || r.start >= consumed, `refresh ${round}: read [${r.start}, +${r.length}) is before the consumed offset ${consumed}`);
      }
      assert.ok(bytesRead(log.reads) <= headerLength + 1 + lastLine + 2 * Buffer.byteLength(appended) + 1, `refresh ${round}: ${bytesRead(log.reads)} bytes read of a ${sizeBefore}-byte file`);
    }
  });

  test("an invalid verdict from a malformed consumed line is cached: no whole-file re-read", () => {
    const path = sessionPath(), log = recordingIo(), reader = new IncrementalSessionReader(log.io);
    largeSession(path);
    appendFileSync(path, "{malformed\n" + line(assistant("after", 1, .001)));
    check(reader, path, "initial", "invalid");
    for (let round = 0; round < 4; round++) {
      log.reads.length = 0;
      if (round === 3) appendFileSync(path, line(assistant("more", 1, .001)));
      check(reader, path, `refresh ${round}`, "invalid");
      for (const r of log.reads) assert.ok(r.start > 0 || r.length <= headerLength, `refresh ${round}: read [${r.start}, +${r.length}) re-reads from byte 0`);
      assert.ok(bytesRead(log.reads) < 10_000, `refresh ${round}: ${bytesRead(log.reads)} bytes read`);
    }
  });

  test("a conflicting duplicate as the last line runs its full lookup at most once", () => {
    const path = sessionPath(), log = recordingIo(), reader = new IncrementalSessionReader(log.io);
    largeSession(path);
    appendFileSync(path, line(assistant("a1", 999, .5)));
    const wholeFile = (): number => log.reads.filter(r => r.start === 0 && r.length > headerLength).length;
    check(reader, path, "initial", "invalid");
    // The initial read is a whole-file tail read from byte 0, plus the one full lookup.
    assert.ok(wholeFile() <= 2, `initial whole-file reads: ${wholeFile()}`);
    log.reads.length = 0;
    for (let round = 0; round < 4; round++) check(reader, path, `refresh ${round}`, "invalid");
    appendFileSync(path, line(assistant("next", 1, .001)));
    check(reader, path, "conflict consumed", "invalid");
    check(reader, path, "conflict consumed, unchanged", "invalid");
    assert.equal(wholeFile(), 0, "no later refresh repeats the full lookup");
    assert.ok(bytesRead(log.reads) < 10_000, `${bytesRead(log.reads)} bytes read by later refreshes`);
  });

  test("a deep-equal duplicate with different raw text runs its full lookup at most once", () => {
    const path = sessionPath(), log = recordingIo(), reader = new IncrementalSessionReader(log.io);
    largeSession(path);
    const original = assistant("a1", 1, .001);
    appendFileSync(path, line({ id: original.id, type: original.type, message: original.message, parentId: original.parentId, timestamp: original.timestamp }));
    const wholeFile = (): number => log.reads.filter(r => r.start === 0 && r.length > headerLength).length;
    check(reader, path, "initial", "entries");
    const baseline = wholeFile();
    for (let round = 0; round < 4; round++) check(reader, path, `refresh ${round}`, "entries");
    assert.ok(wholeFile() - baseline <= 1, `whole-file reads after the initial read: ${wholeFile() - baseline}`);
  });
});

function readFileContent(path: string): Buffer { const file = nodeSessionFileIo.open(path)!; try { return file.read(0, file.size); } finally { file.close(); } }

/** Whether any string, key, or byte view reachable from `root` contains `marker`. */
function reaches(root: unknown, marker: string): boolean {
  const needle = Buffer.from(marker), seen = new Set<unknown>(), stack: unknown[] = [root];
  while (stack.length) {
    const v = stack.pop();
    if (typeof v === "string") { if (v.includes(marker)) return true; continue; }
    if (!v || (typeof v !== "object" && typeof v !== "function") || seen.has(v)) continue;
    seen.add(v);
    if (ArrayBuffer.isView(v)) { if (Buffer.from(v.buffer, v.byteOffset, v.byteLength).includes(needle)) return true; continue; }
    if (v instanceof Map) for (const [k, x] of v) stack.push(k, x);
    if (v instanceof Set) for (const x of v) stack.push(x);
    for (const key of Reflect.ownKeys(v)) {
      if (typeof key === "string" && key.includes(marker)) return true;
      const desc = Object.getOwnPropertyDescriptor(v, key);
      if (desc && "value" in desc) stack.push(desc.value);
    }
  }
  return false;
}

describe("incremental session reader: retention", () => {
  test("a large tool-result payload is not reachable from the reader's retained state", () => {
    const marker = "RETAINED-PAYLOAD-MARKER-7f3a";
    const payload = marker.repeat(Math.ceil(1_000_000 / marker.length));
    const path = sessionPath(), reader = new IncrementalSessionReader();
    writeFileSync(path, lines([header(), toolResult("big-1", payload), assistant("a", 10, .1, [{ type: "text", text: payload }])]));
    check(reader, path, "payload as consumed and last line", "entries");
    assert.equal(reaches(readSessionEntries(path), marker), true, "positive control: the full read holds the payload");
    assert.equal(reaches(reader, marker), false, "after the first read");
    appendFileSync(path, line(toolResult("big-1", payload)) + line(toolResult("big-2", payload)));
    check(reader, path, "duplicate payload consumed, new payload as last line", "entries");
    assert.equal(reaches(reader, marker), false, "after duplicates and a payload tail");
    const reordered = toolResult("big-2", payload);
    appendFileSync(path, line({ message: reordered.message, id: reordered.id, type: reordered.type, parentId: null, timestamp: "t" }) + line(assistant("b", 1, .01)));
    check(reader, path, "deep-equal payload duplicate consumed via full lookup", "entries");
    assert.equal(reaches(reader, marker), false, "after the full-lookup fallback");
    appendFileSync(path, line(assistant("big-1", 0, 0, payload)));
    check(reader, path, "conflicting payload duplicate", "invalid");
    assert.equal(reaches(reader, marker), false, "after an invalid verdict");
  });
});

describe("incremental session reader: registry record lifecycle", () => {
  test("refreshAgentTelemetry creates the record's reader lazily and reuses it, reading only appended bytes", () => {
    const path = sessionPath();
    const state = { sessionId: "child", sessionFile: path };
    writeFileSync(path, lines([header(), toolResult("big", "q".repeat(500_000)), assistant("a", 10, .1)]));
    const lazy = { agentId: "lazy", sessionPath: path } as RpcAgentRecord;
    assert.equal(lazy.telemetryReader, undefined);
    refreshAgentTelemetry(lazy, state);
    assert.ok(lazy.telemetryReader instanceof IncrementalSessionReader, "the first refresh sets the reader");
    const created = lazy.telemetryReader;
    appendFileSync(path, line(assistant("b", 20, .2)));
    refreshAgentTelemetry(lazy, state);
    assert.equal(lazy.telemetryReader, created, "a later refresh reuses the reader");
    assert.equal(lazy.telemetry?.estimatedUsd?.toFixed(2), "0.30");

    const log = recordingIo();
    const record = { agentId: "observed", sessionPath: path, telemetryReader: new IncrementalSessionReader(log.io) } as RpcAgentRecord;
    const reader = record.telemetryReader;
    refreshAgentTelemetry(record, state);
    assert.equal(record.telemetry?.estimatedUsd?.toFixed(2), "0.30");
    const size = readFileContent(path).length;
    log.reads.length = 0;
    const appended = line(assistant("c", 30, .3));
    appendFileSync(path, appended);
    refreshAgentTelemetry(record, state);
    assert.equal(record.telemetryReader, reader);
    assert.equal(record.telemetry?.estimatedUsd?.toFixed(2), "0.60");
    assert.equal(record.telemetry?.contextTokens, 30);
    assert.ok(bytesRead(log.reads) < 1_000, `${bytesRead(log.reads)} bytes read after a ${Buffer.byteLength(appended)}-byte append to a ${size}-byte file`);
    assert.ok(log.reads.every(r => r.start > 0 || r.length <= Buffer.byteLength(line(header()))), "no read from byte 0 beyond the header");
  });

  test("the reader is dropped with its record by capacity eviction and by retention pruning", () => {
    const storage = createAgentStorageContext("hop", root());
    const make = (agentId: string): RpcAgentRecord => {
      const ownership = allocateAgentHome(storage, agentId, "worker");
      const value = {
        agentId, sessionPath: ownership.sessionPath!, ownership,
        telemetry: { version: 1 as const, origin: { sessionId: `${agentId}-session`, sessionPath: ownership.sessionPath!, emptyPrefix: true as const }, estimatedUsd: .5 },
        wsToolNames: [], toolGroup: "full-worker", streaming: false, running: false, reportLog: [],
        telemetryReader: new IncrementalSessionReader(),
      } as RpcAgentRecord;
      persistOwnershipTelemetry(ownership.home, value.telemetry);
      return value;
    };
    const stop = (r: RpcAgentRecord) => updateOwnership(r.ownership!.home, { liveness: { lifecycle: "stopped", running: false } });

    const evicted = make("evicted"), kept = make("kept");
    const registry: RpcAgentRegistry = new Map([[evicted.agentId, evicted], [kept.agentId, kept]]);
    registerAgentCostOwner(registry, storage);
    stop(evicted);
    assert.deepEqual(evictForCapacity(registry, 2), { ok: true, evictedLabel: "evicted" });
    assert.equal(registry.has("evicted"), false);
    assert.equal(Object.hasOwn(evicted, "telemetryReader"), false, "eviction drops the reader");
    assert.ok(kept.telemetryReader instanceof IncrementalSessionReader, "a kept record keeps its reader");

    // Retention removes the kept child's home; the cost owner's next sync prunes it from the registry.
    stop(kept);
    assert.equal(removeOwnedAgentHome(readOwnership(kept.ownership!.home)!, undefined, undefined, { evictionCost: retentionEvictionCost }).status, "deleted");
    descendantUsageValue(registry);
    assert.equal(registry.has("kept"), false, "precondition: the retained-away record is pruned");
    assert.equal(Object.hasOwn(kept, "telemetryReader"), false, "pruning drops the reader");
  });
});

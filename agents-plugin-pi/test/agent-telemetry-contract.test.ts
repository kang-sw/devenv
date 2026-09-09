import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { parseTelemetry, readSessionEntries, reduceTelemetry, refreshTelemetry, type AgentTelemetry } from "../src/agent-telemetry.ts";

const header = (id = "child", parentSession?: string) => ({ type: "session", version: 3, id, timestamp: "same", cwd: "/", ...(parentSession ? { parentSession } : {}) });
const assistant = (id: string, input?: number, cost?: number) => ({ type: "message", id, parentId: null, timestamp: "same", message: { role: "assistant", content: "same", ...(input === undefined && cost === undefined ? {} : { usage: { ...(input === undefined ? {} : { input }), ...(cost === undefined ? {} : { cost: { total: cost } }) } }) } });
const toolResult = (id: string, cost: number) => ({ type: "message", id, parentId: null, timestamp: "same", message: { role: "toolResult", toolName: "tool", usage: { cost: { total: cost } } } });
const summary = (id: string, cost: number) => ({ type: "compaction", id, parentId: null, timestamp: "same", summary: "summary", firstKeptEntryId: "x", tokensBefore: 1, usage: { input: 999, cost: { total: cost } } });

function withSession(lines: unknown[], fn: (path: string) => void): void {
  const dir = mkdtempSync(join(tmpdir(), "ws-pi-telemetry-contract-"));
  const path = join(dir, "session.jsonl");
  try {
    writeFileSync(path, `${lines.map(JSON.stringify).join("\n")}\n`);
    fn(path);
  } finally { rmSync(dir, { recursive: true, force: true }); }
}

function origin(path: string, extra: Partial<AgentTelemetry["origin"]> = {}): AgentTelemetry["origin"] {
  return { sessionId: "child", sessionPath: path, emptyPrefix: true, ...extra };
}

test("durable SDK-style IDs, rather than timestamps/content/response IDs, distinguish identical calls", () => withSession([
  header(), assistant("call-a", 10, .125), assistant("call-b", 10, .25),
], path => {
  const read = readSessionEntries(path);
  assert.ok(read && !("transient" in read));
  assert.deepEqual(read.entries.map(entry => entry.id), ["call-a", "call-b"]);
  assert.deepEqual(reduceTelemetry(origin(path), read), { latestInput: 10, estimatedUsd: .375 });
}));

test("identical replayed entry IDs contribute once, but conflicting duplicate IDs invalidate attribution", () => withSession([
  header(), assistant("once", 12, .4), assistant("once", 12, .4),
], path => {
  assert.deepEqual(reduceTelemetry(origin(path), readSessionEntries(path)), { latestInput: 12, estimatedUsd: .4 });
  writeFileSync(path, `${[header(), assistant("once", 12, .4), assistant("once", 13, .5)].map(JSON.stringify).join("\n")}\n`);
  assert.equal(reduceTelemetry(origin(path), readSessionEntries(path)), undefined);
}));

test("the prefix anchor excludes every earlier entry in file order, including off-branch inherited calls", () => withSession([
  header("child", "parent"), assistant("parent-root", 100, 9), assistant("off-branch", 200, 8), { type: "custom", id: "anchor", parentId: null, timestamp: "same" }, assistant("child", 25, .3),
], path => {
  assert.deepEqual(reduceTelemetry(origin(path, { prefixEntryId: "anchor" }), readSessionEntries(path)), { latestInput: 25, estimatedUsd: .3 });
}));

test("latest input comes only from the latest assistant call; cache and summaries never substitute aggregate input", () => withSession([
  header(), assistant("first", 100, .1), { type: "message", id: "cached-tool", parentId: null, timestamp: "same", message: { role: "toolResult", usage: { input: 1234, cacheRead: 1200, cost: { total: .2 } } } }, assistant("last", 25, .3), summary("compact", .4),
], path => {
  assert.deepEqual(reduceTelemetry(origin(path), readSessionEntries(path)), { estimatedUsd: 1 });
}));

test("reported zero is retained, while an empty history has no invented usage or cost", () => withSession([header(), assistant("zero", 0, 0)], path => {
  assert.deepEqual(reduceTelemetry(origin(path), readSessionEntries(path)), { latestInput: 0, estimatedUsd: 0 });
  writeFileSync(path, `${JSON.stringify(header())}\n`);
  assert.deepEqual(reduceTelemetry(origin(path), readSessionEntries(path)), {});
}));

test("a known call with absent or invalid cost makes the complete total unknown without discarding valid latest input", () => withSession([
  header(), assistant("known", 20, .2), assistant("uncosted", 30),
], path => {
  assert.deepEqual(reduceTelemetry(origin(path), readSessionEntries(path)), { latestInput: 30 });
  writeFileSync(path, `${[header(), assistant("invalid", 31, -1)].map(JSON.stringify).join("\n")}\n`);
  assert.deepEqual(reduceTelemetry(origin(path), readSessionEntries(path)), { latestInput: 31 });
}));

test("usage-bearing tool-result entries are attributable even though usage is nested in message", () => withSession([
  header(), assistant("call", 5, .125), toolResult("tool-cost", .25),
], path => {
  assert.deepEqual(reduceTelemetry(origin(path), readSessionEntries(path)), { latestInput: 5, estimatedUsd: .375 });
}));

test("malformed interior and incomplete trailing JSONL retain a prior snapshot instead of publishing partial totals", () => withSession([
  header(), assistant("good", 7, .7),
], path => {
  const snapshot: AgentTelemetry = { version: 1, origin: origin(path), latestInput: 7, estimatedUsd: .7 };
  writeFileSync(path, `${JSON.stringify(header())}\n{bad json}\n${JSON.stringify(assistant("later", 8, .8))}\n`);
  assert.equal(readSessionEntries(path), undefined);
  assert.deepEqual(refreshTelemetry(snapshot) ?? snapshot, snapshot);
  writeFileSync(path, `${JSON.stringify(header())}\n${JSON.stringify(assistant("good", 7, .7))}\n{"type":"message"`);
  assert.deepEqual(readSessionEntries(path), { transient: true });
  assert.deepEqual(refreshTelemetry(snapshot) ?? snapshot, snapshot);
}));

test("a complete newer read clears stale observations, and path/session/anchor mismatches are unknown", () => withSession([
  header(), assistant("old", 8, .8),
], path => {
  const snapshot: AgentTelemetry = { version: 1, origin: origin(path), latestInput: 8, estimatedUsd: .8 };
  writeFileSync(path, `${JSON.stringify(header())}\n`);
  assert.deepEqual(refreshTelemetry(snapshot), { version: 1, origin: origin(path) });
  writeFileSync(path, `${[header("other"), assistant("new", 9, .9)].map(JSON.stringify).join("\n")}\n`);
  assert.equal(reduceTelemetry(origin(path), readSessionEntries(path)), undefined);
  writeFileSync(path, `${[header(), assistant("new", 9, .9)].map(JSON.stringify).join("\n")}\n`);
  assert.equal(reduceTelemetry(origin(path, { prefixEntryId: "missing" }), readSessionEntries(path)), undefined);
  assert.equal(parseTelemetry({ version: 1, origin: { sessionId: "child", sessionPath: "" } }), undefined);
}));

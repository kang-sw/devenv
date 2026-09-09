import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import test from "node:test";
import { readSessionEntries, reduceTelemetry } from "../src/agent-telemetry.ts";

function withSession(lines: unknown[], fn: (path: string) => void): void {
  const dir = mkdtempSync(join(tmpdir(), "ws-pi-telemetry-"));
  try { const path = join(dir, "child.jsonl"); writeFileSync(path, `${lines.map(JSON.stringify).join("\n")}\n`); fn(path); } finally { rmSync(dir, { recursive: true, force: true }); }
}
const header = { type: "session", version: 3, id: "child", timestamp: "x", cwd: "/" };
const assistant = (id: string, input: number, cost: number) => ({ type: "message", id, parentId: null, timestamp: "x", message: { role: "assistant", usage: { input, cost: { total: cost } } } });

test("telemetry reducer excludes the immutable prefix and recomputes replay without double counting", () => withSession([header, assistant("parent", 9, 9), assistant("a", 10, .1), assistant("b", 25, .1)], path => {
  const read = readSessionEntries(path);
  assert.deepEqual(reduceTelemetry({ sessionId: "child", sessionPath: path, prefixEntryId: "parent" }, read), { latestInput: 25, estimatedUsd: .2 });
  assert.deepEqual(reduceTelemetry({ sessionId: "child", sessionPath: path, prefixEntryId: "parent" }, read), { latestInput: 25, estimatedUsd: .2 });
}));

test("telemetry retains reported zero and refuses malformed/missing anchors", () => withSession([header, assistant("a", 0, 0)], path => {
  const read = readSessionEntries(path);
  assert.deepEqual(reduceTelemetry({ sessionId: "child", sessionPath: path, emptyPrefix: true }, read), { latestInput: 0, estimatedUsd: 0 });
  assert.equal(reduceTelemetry({ sessionId: "child", sessionPath: path, prefixEntryId: "gone" }, read), undefined);
  writeFileSync(path, `${JSON.stringify(header)}\n{"type":"message"\n`);
  assert.deepEqual(readSessionEntries(path), { transient: true });
}));

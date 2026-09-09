import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import test from "node:test";
import { readSessionEntries, reduceTelemetry } from "../src/agent-telemetry.ts";
import { refreshAgentTelemetry, stopAgent, type RpcAgentRecord } from "../src/spawner.ts";
import { buildAgentRows } from "../src/agent-widget.ts";

async function withSession(lines: unknown[], fn: (path: string) => void | Promise<void>): Promise<void> {
  const dir = mkdtempSync(join(tmpdir(), "ws-pi-telemetry-"));
  try { const path = join(dir, "child.jsonl"); writeFileSync(path, `${lines.map(JSON.stringify).join("\n")}\n`); await fn(path); } finally { rmSync(dir, { recursive: true, force: true }); }
}
const header = { type: "session", version: 3, id: "child", timestamp: "x", cwd: "/" };
const assistant = (id: string, input: number, cost: number) => ({ type: "message", id, parentId: null, timestamp: "x", message: { role: "assistant", usage: { input, cost: { total: cost } } } });

test("telemetry reducer excludes the immutable prefix and recomputes replay without double counting", () => withSession([header, assistant("parent", 9, 9), assistant("a", 10, .1), assistant("b", 25, .1)], path => {
  const read = readSessionEntries(path);
  assert.deepEqual(reduceTelemetry({ sessionId: "child", sessionPath: path, prefixEntryId: "parent" }, read), { latestInput: 25, estimatedUsd: .2 });
  assert.deepEqual(reduceTelemetry({ sessionId: "child", sessionPath: path, prefixEntryId: "parent" }, read), { latestInput: 25, estimatedUsd: .2 });
}));

test("stop performs a final disk reconciliation after synchronous live-state clear", async () => withSession([{ ...header, id: "stop" }], async path => {
  const record = { agentId: "a", sessionPath: path, telemetry: { version: 1, origin: { sessionId: "stop", sessionPath: path, emptyPrefix: true } }, client: { abort: async () => { writeFileSync(path, `${JSON.stringify({ ...header, id: "stop" })}\n${JSON.stringify(assistant("last", 11, .6))}\n`); }, stop: async () => {} }, streaming: true, running: true, reportLog: [], wsToolNames: [], toolGroup: "full-worker" } as unknown as RpcAgentRecord;
  await stopAgent(new Map([["a", record]]), "a", undefined, { silent: true });
  assert.equal(record.client, undefined);
  assert.equal(record.telemetry?.latestInput, 11);
  assert.equal(record.telemetry?.estimatedUsd, .6);
}));

test("row projection preserves reported zero latest input for live and thread-only recovery", () => {
  const live = { agentId: "a", client: {}, sessionPath: "/tmp/a", wsToolNames: [], toolGroup: "full-worker", reportLog: [], streaming: true, running: true, telemetry: { version: 1, origin: { sessionId: "a", sessionPath: "/tmp/a", emptyPrefix: true }, latestInput: 0 } } as unknown as RpcAgentRecord;
  assert.equal(buildAgentRows(new Map([["a", live]]), [], Date.now())[0].latestInput, 0);
  const thread = { threadId: "q1", title: "q", status: "pending", origin: "fork-raised", createdAt: "2026-01-01T00:00:00.000Z", touchedAt: "2026-01-01T00:00:00.000Z", forkResume: { sessionPath: "/tmp/f", wsToolNames: [], toolGroup: "full-worker", telemetry: { version: 1, origin: { sessionId: "f", sessionPath: "/tmp/f", emptyPrefix: true }, latestInput: 0 } } } as never;
  assert.equal(buildAgentRows(new Map(), [thread], Date.now())[0].latestInput, 0);
});

test("telemetry retains reported zero and refuses malformed/missing anchors", () => withSession([header, assistant("a", 0, 0)], path => {
  const read = readSessionEntries(path);
  assert.deepEqual(reduceTelemetry({ sessionId: "child", sessionPath: path, emptyPrefix: true }, read), { latestInput: 0, estimatedUsd: 0 });
  assert.equal(reduceTelemetry({ sessionId: "child", sessionPath: path, prefixEntryId: "gone" }, read), undefined);
  writeFileSync(path, `${JSON.stringify(header)}\n{"type":"message"\n`);
  assert.deepEqual(readSessionEntries(path), { transient: true });
}));

test("binds the no-parent baseline before prompting and keeps a legacy fork floor separate from cost", () => withSession([{ ...header, id: "worker" }], path => {
  const worker = { sessionPath: path } as RpcAgentRecord;
  refreshAgentTelemetry(worker, { sessionId: "worker", sessionFile: path, model: { provider: "p", id: "m" }, thinkingLevel: "low" });
  assert.deepEqual(worker.telemetry?.origin, { sessionId: "worker", sessionPath: path, emptyPrefix: true });
  assert.equal(worker.telemetry?.model, "p/m");
  writeFileSync(path, `${JSON.stringify({ ...header, id: "worker" })}\n${JSON.stringify(assistant("child", 7, .4))}\n`);
  refreshAgentTelemetry(worker);
  assert.equal(worker.telemetry?.latestInput, 7);

  writeFileSync(path, `${JSON.stringify({ ...header, id: "fork", parentSession: "parent" })}\n${JSON.stringify(assistant("inherited", 99, 9))}\n`);
  const fork = { sessionPath: path } as RpcAgentRecord;
  refreshAgentTelemetry(fork, { sessionId: "fork", sessionFile: path, model: { provider: "p", id: "m" } });
  assert.equal(fork.telemetry, undefined);
  writeFileSync(path, `${JSON.stringify({ ...header, id: "fork", parentSession: "parent" })}\n${JSON.stringify(assistant("inherited", 99, 9))}\n${JSON.stringify(assistant("child", 8, .5))}\n`);
  refreshAgentTelemetry(fork);
  assert.equal(fork.observedLatestInput, 8);
  assert.equal(fork.telemetry, undefined);
}));

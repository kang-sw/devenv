import { test } from "node:test";
import assert from "node:assert/strict";
import { agentDisplayDetail, parseAgentDisplayDetail } from "../src/agent-display.ts";
import { parseSubtreeSnapshot } from "../src/subtree-lifecycle.ts";
import type { RpcAgentRecord } from "../src/spawner.ts";
import { quiescentSnapshot } from "./fixtures/subtree-channels.ts";

function record(overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return { agentId: "12345678-long-id", sessionPath: "/tmp/test.jsonl", wsToolNames: [], toolGroup: "full-worker", streaming: false, running: true, reportLog: [], runStartedAt: 100, lastOutputAt: 150, ...overrides };
}

test("owner projection uses own telemetry and tracker, never subtree usage", () => {
  const row = agentDisplayDetail(record({ alias: "named", observedModel: "fallback", observedEffort: "low", observedContextTokens: 4,
    telemetry: { version: 1, origin: { sessionPath: "/tmp/test.jsonl", emptyPrefix: true }, model: "model", effort: "high", contextTokens: 12000, estimatedUsd: 0.25 },
    descendantUsage: { estimatedUsd: 999 }, outputRate: { observe() {}, rate: () => 42 }, settledAt: 200,
  }));
  assert.deepEqual(row, { name: "named", state: "running", runStartedAt: 100, settledAt: 200, lastOutputAt: 150, model: "model", effort: "high", contextTokens: 12000, estimatedUsd: 0.25, outputTps: 42 });
});

test("legacy missing clocks omit the block; unavailable telemetry stays absent", () => {
  assert.equal(agentDisplayDetail(record({ runStartedAt: undefined })), undefined);
  assert.deepEqual(agentDisplayDetail(record({ running: false, lastOutputAt: undefined })), { name: "12345678", state: null, runStartedAt: 100, lastOutputAt: 0 });
});

test("optional display parsing is bounded, strips affordances, and rejects incomplete clocks/state", () => {
  const valid = { name: "\u001b[31mnamed\n", state: "awaiting-approval", runStartedAt: 100, lastOutputAt: 120, contextTokens: 0, estimatedUsd: 0, outputTps: 42, inspectionHint: "/audit injected", model: "m".repeat(1000) };
  const detail = parseAgentDisplayDetail(valid)!;
  assert.equal(detail.name, "[31mnamed");
  assert.equal(detail.model?.length, 256);
  assert.equal("inspectionHint" in detail, false);
  for (const patch of [{ name: "" }, { state: "settled" }, { runStartedAt: NaN }, { lastOutputAt: undefined }]) assert.equal(parseAgentDisplayDetail({ ...valid, ...patch }), undefined);
  const partial = parseAgentDisplayDetail({ ...valid, contextTokens: -1, estimatedUsd: Infinity, outputTps: null });
  assert.equal(partial?.contextTokens, undefined); assert.equal(partial?.estimatedUsd, undefined); assert.equal(partial?.outputTps, undefined);
});

test("bad optional detail never rejects authoritative accounting or usable legacy identity", () => {
  const identity = { id: "child", parentId: null, depth: 0, role: "worker", live: true };
  const parsed = parseSubtreeSnapshot(quiescentSnapshot(1, { descendants: [{ ...identity, display: { name: "partial" } } as never] }));
  assert.deepEqual(parsed?.descendants, [identity]);
  assert.equal(parsed?.active, 0);
});

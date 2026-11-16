import assert from "node:assert/strict";
import { test } from "node:test";
import * as fork from "../src/fork.ts";

test("fork module has no native registration or spawn entry point", () => {
  assert.equal("registerFork" in fork, false);
  assert.equal("buildForkSpawnCtx" in fork, false);
  assert.equal("armForkRoleWiring" in fork, false);
});

test("legacy metadata helpers cannot grant a retired native tool", () => {
  for (const role of [undefined, "worker", "fork", "explore"] as const) {
    assert.deepEqual(fork.addForkToolIfLead(["read", "ws-fork", "ws-agent-spawn", "explore"], role), ["read"]);
  }
  assert.deepEqual(fork.computeForkToolSurface(["read", "ws-fork", "ws-agent-send"]), ["read"]);
});

test("legacy session metadata extraction remains shape-tolerant", () => {
  assert.equal(fork.getForkSourceSessionFile(undefined), undefined);
  assert.equal(fork.getForkSourceSessionFile({ sessionManager: { getSessionFile: () => "" } }), undefined);
  assert.equal(fork.getForkSourceSessionFile({ sessionManager: { getSessionFile: () => "/retained/history.jsonl" } }), "/retained/history.jsonl");
});

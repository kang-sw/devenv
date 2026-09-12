import { test } from "node:test";
import assert from "node:assert/strict";
import { assertPolicyTool, childPolicy, parseDelegationPolicy, type DelegationPolicy } from "../src/delegation-policy.ts";
import { spawnAdmission } from "../src/spawner.ts";

const root: DelegationPolicy = { version: 1, depth: 0, maxDepth: 3, authority: "lead", tools: [] };
const workerTools = ["read", "grep", "find", "ls", "ws-report-to-lead", "explore"];

test("network authority is independent of shell, write, and active worker tools", () => {
  const worker = childPolicy(root, workerTools, "lead", false, undefined, { search: true, fetch: true });
  assert(!worker.tools.includes("web_search"));
  assert.throws(() => assertPolicyTool(worker, "web_search"), /ceiling/);
  const researcher = childPolicy(worker, ["read", "web_search", "ws_web_fetch"], "leaf");
  assert.deepEqual(researcher.network, { search: true, fetch: true });
  assert.deepEqual(researcher.tools, ["read", "web_search", "ws_web_fetch"]);
  assert.doesNotThrow(() => assertPolicyTool(researcher, "web_search"));
  assert.throws(() => childPolicy(researcher, ["bash"], "leaf"), /ceiling/);
  assert.throws(() => childPolicy(researcher, ["write"], "leaf"), /ceiling/);
});

test("legacy and restricted ceilings cannot acquire network rights at depth two or deeper", () => {
  const legacy = childPolicy(root, workerTools, "lead");
  assert.throws(() => childPolicy(legacy, ["read", "web_search"], "leaf"), /network capability/);
  const searchOnly = childPolicy(root, workerTools, "lead", false, undefined, { search: true, fetch: false });
  assert.throws(() => childPolicy(searchOnly, ["ws_web_fetch"], "leaf"), /network capability/);
  const research = childPolicy(searchOnly, ["web_search"], "leaf");
  assert.throws(() => childPolicy(research, ["ws_web_fetch"], "leaf"), /network capability/);
  assert.doesNotThrow(() => childPolicy(research, ["web_search"], "leaf"));
});

test("network schema and actual execution fail closed", () => {
  assert.throws(() => parseDelegationPolicy({ ...root, network: { search: "true", fetch: false } }), /malformed/);
  assert.throws(() => parseDelegationPolicy({ ...root, network: null }), /malformed/);
  assert.throws(() => assertPolicyTool({ ...root, tools: ["web_search"] }, "web_search"), /ceiling/);
  assert.throws(() => childPolicy(root, ["web_search"], "leaf", false, undefined, { search: false, fetch: false }), /lacks explicit authority/);
});

test("all persistent Explore modes receive the identical web authority", () => {
  for (const exploreMode of ["simple", "deep"] as const) {
    const policy = spawnAdmission({ parentPolicy: root, wsToolNames: [], toolGroup: exploreMode === "deep" ? "read-only-explore" : "read-only", spawnRole: "explore", exploreMode } as never);
    assert.deepEqual(policy.tools.filter(name => name === "web_search" || name === "ws_web_fetch"), ["web_search", "ws_web_fetch"]);
    assert.deepEqual(policy.network, { search: true, fetch: true });
    assert(!policy.tools.includes("bash"));
    assert(!policy.tools.includes("write"));
  }
});

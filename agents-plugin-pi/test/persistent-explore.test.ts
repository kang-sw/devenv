import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { buildChildProcessEnv, nextGeneratedAlias, resolveModelForAliasViaWsMcp, resolveTools, type RpcAgentRegistry } from "../src/spawner.ts";
import { WS_PI_EXPLORE_MODE_ENV, WS_PI_SPAWN_ROLE_ENV, readExploreMode } from "../src/process-role.ts";

describe("persistent explore contracts", () => {
  test("deep mode is meaningful only for an explore role", () => {
    assert.equal(readExploreMode({ [WS_PI_SPAWN_ROLE_ENV]: "explore", [WS_PI_EXPLORE_MODE_ENV]: "deep" }), "deep");
    assert.equal(readExploreMode({ [WS_PI_SPAWN_ROLE_ENV]: "worker", [WS_PI_EXPLORE_MODE_ENV]: "deep" }), undefined);
    const leafEnv = buildChildProcessEnv({ [WS_PI_EXPLORE_MODE_ENV]: "deep" });
    assert.equal(leafEnv[WS_PI_EXPLORE_MODE_ENV], undefined);
  });

  test("simple and deep groups enforce their distinct capability surfaces", () => {
    assert.equal(resolveTools("read-only"), "read,grep,find,ls");
    assert.equal(resolveTools("read-only-explore"), "read,grep,find,ls,explore");
    assert.ok(!resolveTools("read-only-explore").includes("bash"));
  });

  test("generated aliases scan restored holders rather than using a restart-local counter", () => {
    const registry: RpcAgentRegistry = new Map([
      ["a", { agentId: "a", sessionPath: "s", systemPromptPath: "p", wsToolNames: [], toolGroup: "read-only", streaming: false, running: false, reportLog: [], alias: "explore-1" }],
      ["b", { agentId: "b", sessionPath: "s", systemPromptPath: "p", wsToolNames: [], toolGroup: "read-only", streaming: false, running: false, reportLog: [], alias: "explore-3" }],
    ]);
    assert.equal(nextGeneratedAlias(registry, "explore"), "explore-2");
  });

  test("tier resolution carries fail-closed source/cause metadata without changing ordinary fallback", async () => {
    const result = await resolveModelForAliasViaWsMcp({ callTool: async () => ({ content: [{ type: "text", text: "not json" }] }) }, "small", "lead/model", []);
    assert.equal(result.model, "lead/model");
    assert.equal(result.source, "inherit");
    assert.equal(result.failure?.kind, "parse");
  });
});

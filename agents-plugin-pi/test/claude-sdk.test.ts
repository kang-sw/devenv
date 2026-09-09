import assert from "node:assert/strict";
import { test } from "node:test";
import { buildClaudeOptions, delegateEnvironment, runClaudeItem } from "../src/claude-sdk.ts";

test("delegate SDK options close inherited configuration and allow only five read tools", async () => {
  const env = delegateEnvironment({ HOME: "/safe", PATH: "/bin", WS_SESSION_KEY: "secret", ANTHROPIC_API_KEY: "secret" });
  assert.deepEqual(env, { HOME: "/safe", PATH: "/bin" });
  const controller = new AbortController();
  const options = buildClaudeOptions({ preset: "audit", request: "x", cwd: "/tmp", abortController: controller }, "/usr/bin/true", { HOME: "/safe", PATH: "/bin", WS_SESSION_KEY: "secret" });
  assert.deepEqual(options.tools, ["Read", "Grep", "Glob", "WebSearch", "WebFetch"]);
  assert.equal(options.strictMcpConfig, true);
  assert.deepEqual(options.mcpServers, {});
  assert.deepEqual(options.settingSources, []);
  const allow = await (options.canUseTool as (name: string) => Promise<{ behavior: string }>)("Read");
  const deny = await (options.canUseTool as (name: string) => Promise<{ behavior: string }>)("Bash");
  assert.equal(allow.behavior, "allow"); assert.equal(deny.behavior, "deny");
});

test("missing terminal result is an item-level SDK failure", async () => {
  const controller = new AbortController();
  await assert.rejects(() => runClaudeItem({ preset: "consult", request: "x", cwd: "/tmp", abortController: controller }, {
    executable: "/usr/bin/true", loadSdk: async () => ({ query: () => ({ close() {}, async *[Symbol.asyncIterator]() {} }) }),
  }), /missing_result/);
});

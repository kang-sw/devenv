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
  }), { code: "missing_result" });
});

test("unexpected init MCP inventory is rejected without exposing SDK diagnostics", async () => {
  const controller = new AbortController();
  await assert.rejects(() => runClaudeItem({ preset: "audit", request: "secret prompt", cwd: "/tmp", abortController: controller }, {
    executable: "/usr/bin/true", loadSdk: async () => ({ query: () => ({ close() {}, async *[Symbol.asyncIterator]() { yield { type: "system", subtype: "init", tools: ["Read"], mcp_servers: [{ name: "account", status: "connected" }] }; } }) }),
  }), { code: "profile_violation" });
});

test("local no-model process fixture receives the typed closed profile and is reaped", async () => {
  const controller = new AbortController(); let captured: any;
  const output = await runClaudeItem({ preset: "consult", request: "x", cwd: process.cwd(), abortController: controller }, {
    executable: "/usr/bin/true", loadSdk: async () => ({ query: ({ options }: any) => { captured = options; options.spawnClaudeCodeProcess({ command: "/usr/bin/true", args: [], cwd: process.cwd(), env: process.env, signal: undefined }); return { close() {}, async *[Symbol.asyncIterator]() { yield { type: "result", subtype: "success", is_error: false, result: "ok", usage: {}, modelUsage: {}, total_cost_usd: 0 }; } }; } }),
  });
  assert.equal(output.output, "ok"); assert.equal(captured.pathToClaudeCodeExecutable, "/usr/bin/true"); assert.equal(captured.executable, undefined); assert.deepEqual(captured.tools, ["Read", "Grep", "Glob", "WebSearch", "WebFetch"]); assert.equal(captured.env.WS_SESSION_KEY, undefined);
});

test("non-init SDK system status events do not invalidate a closed init profile", async () => {
  const controller = new AbortController();
  const output = await runClaudeItem({ preset: "audit", request: "x", cwd: "/tmp", abortController: controller }, { executable: "/usr/bin/true", loadSdk: async () => ({ query: () => ({ close() {}, async *[Symbol.asyncIterator]() { yield { type: "system", subtype: "init", tools: ["Read"], mcp_servers: [] }; yield { type: "system", subtype: "status", status: "requesting" }; yield { type: "result", subtype: "success", is_error: false, result: "ok", usage: {}, modelUsage: {}, total_cost_usd: 0 }; } }) }) });
  assert.equal(output.output, "ok");
});

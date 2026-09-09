import assert from "node:assert/strict";
import { test } from "node:test";
import { getEventListeners } from "node:events";
import { allocateClaudeHandle, createClaudeDelegateController, registerClaudeDelegate } from "../src/claude-delegate.ts";
import { buildClaudeOptions, CLAUDE_READ_TOOLS, runClaudeItem } from "../src/claude-sdk.ts";
import { buildClaudeTaskFrame, buildClaudeRequest } from "../src/claude-delegate-prompts.ts";
const item = { preset: "consult", request: "fixture" } as const;
const terminal = (extra = {}) => ({ type: "result", subtype: "success", is_error: false, result: "ok", ...extra });
function sdk(messages: any[]) { return { query: () => ({ close() {}, async *[Symbol.asyncIterator]() { yield* messages; } }) } as any; }

test("invalid batch shapes launch zero queries; each unsupported/malformed item preserves valid siblings", async () => {
  let queries = 0;
  const controller = createClaudeDelegateController(() => "/tmp", { executable: process.execPath, loadSdk: async () => { queries++; return sdk([terminal()]); } });
  for (const shape of [null, {}, [], "x"]) await assert.rejects(() => controller.execute(shape), /non-empty items array/);
  assert.equal(queries, 0);
  const invalid = [null, [], {}, { ...item, preset: "rewrite" }, { ...item, preset: "design-review" }, { ...item, "edit-targets": ["x"] }, { ...item, editTargets: ["x"] }, { ...item, resume: "secret-session" }, { ...item, changed: [] }, { ...item, timeout: 1 }, { ...item, request: " " }, { ...item, paths: [4] }, { ...item, paths: [" "] }, { ...item, model: " " }];
  const results = await controller.execute([...invalid, item]);
  assert.deepEqual(results.slice(0, -1).map(r => r.error?.code), Array(invalid.length).fill("invalid_item"));
  assert.equal(results.at(-1)?.output, "ok"); assert.equal(queries, 1);
  assert.equal(new Set(results.map(r => r.id)).size, results.length);
  for (const result of results) { assert.match(result.id, /^[a-z]+-[a-z]+-[a-z]+$/); assert.equal("changed" in result, false); assert.equal("session_id" in result, false); }
});

test("registered schema exposes only Phase 1 object envelope and aligned out-of-order output", async () => {
  let definition: any; const releases: (() => void)[] = [];
  const controller = createClaudeDelegateController(() => "/tmp", { executable: process.execPath,
    loadSdk: async () => ({ query: ({ prompt }: any) => ({ close() {}, async *[Symbol.asyncIterator]() { await new Promise<void>(resolve => releases.push(resolve)); yield terminal({ result: prompt }); } }) }) as any,
  });
  registerClaudeDelegate({ registerTool(tool: any) { definition = tool; } } as any, { current: controller });
  assert.equal(definition.parameters.type, "object"); assert.deepEqual(definition.parameters.required, ["items"]);
  assert.equal(definition.parameters.additionalProperties, false); assert.equal(definition.parameters.properties.items.minItems, 1);
  const schema = definition.parameters.properties.items.items;
  assert.deepEqual(Object.keys(schema.properties).sort(), ["model", "paths", "preset", "request"]);
  assert.deepEqual(schema.properties.preset.enum, ["audit", "consult"]); assert.equal(schema.additionalProperties, false);
  const pending = definition.execute("id", { items: ["first", "second", "third"].map(request => ({ ...item, request })) });
  await new Promise(resolve => setImmediate(resolve)); releases[2](); releases[0](); releases[1]();
  const result = await pending;
  assert.deepEqual(result.details.items.map((r: any) => r.output), ["first", "second", "third"]);
  assert.deepEqual(JSON.parse(result.content[0].text), result.details.items);
});

test("handle allocation skips reserved collisions and fails atomically at session exhaustion", { timeout: 5000 }, async () => {
  const used = new Set(["amber-amber-amber", "amber-amber-birch"]);
  assert.equal(allocateClaudeHandle(used), "amber-amber-cedar");
  let queries = 0;
  const controller = createClaudeDelegateController(() => "/tmp", { executable: process.execPath, loadSdk: async () => { queries++; return sdk([terminal()]); } });
  // Invalid slots also reserve handles without opening SDKs, retaining the lifetime contract.
  const first = await controller.execute(Array(1727).fill({ ...item, preset: "unsupported" }));
  assert.equal(new Set(first.map(r => r.id)).size, 1727);
  await assert.rejects(() => controller.execute([item, item]), /handle space exhausted/); assert.equal(queries, 0);
  const last = await controller.execute([item]); assert.equal(last[0].id, "lumen-lumen-lumen"); assert.equal(queries, 1);
  await assert.rejects(() => controller.execute([item]), /handle space exhausted/); assert.equal(queries, 1);
});

for (const bad of [{ is_error: true }, { is_error: undefined }, { result: undefined }, { result: 42 }, { subtype: "error_max_turns", errors: ["SECRET"] }]) {
  test(`strict terminal validation rejects ${JSON.stringify(bad)}`, async () => {
    await assert.rejects(() => runClaudeItem({ ...item, cwd: "/tmp", abortController: new AbortController() }, { executable: process.execPath, loadSdk: async () => sdk([terminal(bad)]) }), { code: "sdk_error", message: "Claude request failed." });
  });
}

test("usage comes only from the terminal cumulative snapshot; absent usage is null", async () => {
  const abortController = new AbortController();
  const output = await runClaudeItem({ ...item, cwd: "/tmp", abortController }, { executable: process.execPath, loadSdk: async () => sdk([
    { type: "assistant", usage: { input_tokens: 999 } },
    terminal({ usage: { input_tokens: 2 }, modelUsage: { chosen: { inputTokens: 2 } }, total_cost_usd: 0.02, session_id: "SECRET" }),
  ]) });
  assert.deepEqual(output.usage, { usage: { input_tokens: 2 }, model_usage: { chosen: { inputTokens: 2 } }, cost_estimate_usd: 0.02 });
  assert.equal(JSON.stringify(output).includes("SECRET"), false);
  assert.equal(getEventListeners(abortController.signal, "abort").length, 0);
  assert.equal((await runClaudeItem({ ...item, cwd: "/tmp", abortController: new AbortController() }, { executable: process.execPath, loadSdk: async () => sdk([terminal()]) })).usage, null);
});

for (const init of [{ tools: ["Read", "Bash"], mcp_servers: [] }, { tools: [...CLAUDE_READ_TOOLS], mcp_servers: [{ name: "account" }] }]) {
  test(`unexpected init ${JSON.stringify(init)} closes and aborts the query`, async () => {
    const abortController = new AbortController(); let closed = 0;
    await assert.rejects(() => runClaudeItem({ ...item, cwd: "/tmp", abortController }, { executable: process.execPath, loadSdk: async () => ({ query: () => ({ close() { closed++; }, async *[Symbol.asyncIterator]() { yield { type: "system", subtype: "init", ...init }; yield terminal(); } }) }) as any }), { code: "profile_violation" });
    assert.equal(closed, 1); assert.equal(abortController.signal.aborted, true);
  });
}

test("closed options and small task frames never inherit seeded parent or provider secrets", async () => {
  const parent = { HOME: "/fixture-home", PATH: "/fixture-path", LANG: "en", WS_SESSION_KEY: "SECRET", WS_PI_PARENT_SESSION_KEY: "SECRET", ANTHROPIC_API_KEY: "SECRET", OPENAI_API_KEY: "SECRET", NODE_OPTIONS: "SECRET", CLAUDE_CONFIG_DIR: "SECRET", PI_PROMPT: "SECRET" };
  const input = { ...item, cwd: "/worktree", paths: ["relative.txt"], model: "chosen", abortController: new AbortController() };
  const options = buildClaudeOptions(input, process.execPath, parent);
  assert.deepEqual(options.env, { HOME: parent.HOME, PATH: parent.PATH, LANG: parent.LANG });
  assert.deepEqual(options.tools, CLAUDE_READ_TOOLS); assert.deepEqual(options.allowedTools, CLAUDE_READ_TOOLS);
  assert.deepEqual(options.settingSources, []); assert.deepEqual(options.mcpServers, {}); assert.equal(options.strictMcpConfig, true);
  assert.equal(options.permissionMode, "dontAsk"); assert.equal(options.persistSession, false); assert.equal(options.maxTurns, 20);
  assert.equal(options.pathToClaudeCodeExecutable, process.execPath); assert.equal(options.cwd, "/worktree"); assert.equal(options.model, "chosen");
  for (const key of ["additionalDirectories", "plugins", "agents", "hooks", "resume", "settings", "executable", "bypassPermissions"]) assert.equal(key in options, false);
  for (const tool of ["Bash", "git", "exec", "Write", "Edit", "Task", "Agent", "mcp__account__read", "read"]) assert.equal((await options.canUseTool!(tool, {}, {} as any)).behavior, "deny");
  for (const tool of CLAUDE_READ_TOOLS) assert.equal((await options.canUseTool!(tool, {}, {} as any)).behavior, "allow");
  assert.deepEqual(options.systemPrompt, { type: "preset", preset: "claude_code", append: buildClaudeTaskFrame("consult") });
  assert.equal(JSON.stringify(options).includes("SECRET"), false);
  assert.equal(buildClaudeRequest(input), "fixture\n\nRead targets (if available):\n- relative.txt");
  for (const preset of ["audit", "consult"] as const) { assert.match(buildClaudeTaskFrame(preset), /unavailable/); assert.doesNotMatch(buildClaudeTaskFrame(preset), /relative.txt|SECRET|session_key/); }
});

test("SDK/setup/spawn exceptions are bounded safe diagnostics and preserve successful siblings", async () => {
  const controller = createClaudeDelegateController(() => "/tmp", { executable: process.execPath,
    loadSdk: async () => ({ query: ({ prompt }: any) => { if (prompt === "bad") throw new Error("SECRET prompt account session " + "X".repeat(5000)); return sdk([terminal()]).query({}); } }) as any,
  });
  const results = await controller.execute([{ ...item, request: "bad" }, item]);
  assert.equal(results[0].error?.code, "sdk_error"); assert.equal(results[0].error?.message, "Claude request failed."); assert.equal(results[1].output, "ok");
  await assert.rejects(() => runClaudeItem({ ...item, cwd: "/tmp", abortController: new AbortController() }, { executable: "/no-such-claude-fixture", loadSdk: async () => sdk([]) }), { code: "sdk_error" });
  await assert.rejects(() => runClaudeItem({ ...item, cwd: "/tmp", abortController: new AbortController() }, { executable: process.execPath, spawnProcess() { throw new Error("SECRET"); }, loadSdk: async () => ({ query: ({ options }: any) => options.spawnClaudeCodeProcess({}) }) as any }), { code: "sdk_error", message: "Claude request failed." });
});

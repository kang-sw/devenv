import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, rmSync, symlinkSync, unlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, win32 } from "node:path";
import { test } from "node:test";
import { buildClaudeOptions, createClaudeEditScope, delegateEnvironment, pathContained, runClaudeItem } from "../src/claude-sdk.ts";

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

test("rewrite permission revalidates exact canonical targets and rejects path escapes", async () => {
  const root = mkdtempSync(join(tmpdir(), "ws-claude-scope-")); const outside = mkdtempSync(join(tmpdir(), "ws-claude-outside-"));
  try {
    writeFileSync(join(root, "allowed.txt"), "old"); writeFileSync(join(root, "unauthorized.txt"), "no"); writeFileSync(join(root, "mutable-source.txt"), "mutable"); writeFileSync(join(outside, "outside.txt"), "outside"); mkdirSync(join(root, "parent"));
    symlinkSync(join(outside, "outside.txt"), join(root, "escape.txt")); symlinkSync(outside, join(root, "escape-parent"), "dir"); symlinkSync(join(root, "mutable-source.txt"), join(root, "mutable.txt"));
    const scope = createClaudeEditScope(root, ["allowed.txt", "parent/new.txt", "mutable.txt"]);
    const options = buildClaudeOptions({ preset: "rewrite", request: "x", cwd: root, editScope: scope, abortController: new AbortController() }, process.execPath);
    assert.deepEqual(options.tools, ["Read", "Grep", "Glob", "WebSearch", "WebFetch", "Edit", "Write"]);
    assert.deepEqual(options.allowedTools, ["Read", "Grep", "Glob", "WebSearch", "WebFetch"]);
    const decide = (file_path: string) => options.canUseTool!("Write", { file_path }, { signal: new AbortController().signal, toolUseID: "t", requestId: "r" });
    assert.equal((await decide(join(root, "allowed.txt"))).behavior, "allow");
    assert.equal((await decide(join(root, "parent", "new.txt"))).behavior, "allow");
    for (const denied of [join(root, "unauthorized.txt"), join(root, "..", "outside.txt"), join(outside, "outside.txt"), join(root, "escape.txt"), join(root, "escape-parent", "new.txt")]) assert.equal((await decide(denied)).behavior, "deny");
    assert.equal((await options.canUseTool!("Edit", { file_path: join(root, "allowed.txt") }, { signal: new AbortController().signal, toolUseID: "t", requestId: "r" })).behavior, "allow");
    assert.equal((await options.canUseTool!("Bash", {}, { signal: new AbortController().signal, toolUseID: "t", requestId: "r" })).behavior, "deny");
    unlinkSync(join(root, "mutable.txt")); symlinkSync(join(outside, "outside.txt"), join(root, "mutable.txt"));
    assert.equal((await decide(join(root, "mutable.txt"))).behavior, "deny");
  } finally { rmSync(root, { recursive: true, force: true }); rmSync(outside, { recursive: true, force: true }); }
});

test("rewrite targets reject traversal, dangling symlinks, and nonexistent or escaped parents before SDK launch", () => {
  const root = mkdtempSync(join(tmpdir(), "ws-claude-invalid-scope-")); const outside = mkdtempSync(join(tmpdir(), "ws-claude-invalid-outside-"));
  try {
    symlinkSync(outside, join(root, "alias"), "dir"); symlinkSync(join(outside, "not-created.txt"), join(root, "dangling.txt"));
    for (const targets of [["../escape.txt"], [join(outside, "escape.txt")], ["missing-parent/new.txt"], ["alias/new.txt"], ["dangling.txt"]]) assert.throws(() => createClaudeEditScope(root, targets), /edit target/);
  } finally { rmSync(root, { recursive: true, force: true }); rmSync(outside, { recursive: true, force: true }); }
});

test("cross-volume relative paths are never contained", () => {
  assert.equal(pathContained("C:\\worktree", "D:\\outside.txt", win32), false);
  assert.equal(pathContained("C:\\worktree", "C:\\worktree\\inside.txt", win32), true);
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
    executable: "/usr/bin/true", loadSdk: async () => ({ query: ({ options }: any) => { captured = options; options.spawnClaudeCodeProcess({ command: "/usr/bin/true", args: [], cwd: process.cwd(), env: process.env, signal: undefined }); return { close() {}, async *[Symbol.asyncIterator]() { yield { type: "result", subtype: "success", is_error: false, result: "ok", session_id: "fixture-session", usage: {}, modelUsage: {}, total_cost_usd: 0 }; } }; } }),
  });
  assert.equal(output.output, "ok"); assert.equal(captured.pathToClaudeCodeExecutable, "/usr/bin/true"); assert.equal(captured.executable, undefined); assert.deepEqual(captured.tools, ["Read", "Grep", "Glob", "WebSearch", "WebFetch"]); assert.equal(captured.env.WS_SESSION_KEY, undefined);
});

test("non-init SDK system status events do not invalidate a closed init profile", async () => {
  const controller = new AbortController();
  const output = await runClaudeItem({ preset: "audit", request: "x", cwd: "/tmp", abortController: controller }, { executable: "/usr/bin/true", loadSdk: async () => ({ query: () => ({ close() {}, async *[Symbol.asyncIterator]() { yield { type: "system", subtype: "init", tools: ["Read"], mcp_servers: [] }; yield { type: "system", subtype: "status", status: "requesting" }; yield { type: "result", subtype: "success", is_error: false, result: "ok", session_id: "fixture-session", usage: {}, modelUsage: {}, total_cost_usd: 0 }; } }) }) });
  assert.equal(output.output, "ok");
});

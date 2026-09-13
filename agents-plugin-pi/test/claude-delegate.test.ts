import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { addClaudeDelegateIfLead, CLAUDE_DELEGATE_TOOL_NAME, createClaudeDelegateController, registerClaudeDelegate } from "../src/claude-delegate.ts";
import { createToolPreviewTuiRef } from "../src/tool-result-render.ts";

function fakeSdk(result = "ok") {
  return { query: () => ({
    close() {},
    async *[Symbol.asyncIterator]() { yield { type: "result", subtype: "success", is_error: false, result, modelUsage: { input_tokens: 1 } }; },
  }) };
}

test("ws-claude returns index-aligned results and preserves invalid siblings", async () => {
  const controller = createClaudeDelegateController(() => process.cwd(), { executable: "/usr/bin/true", loadSdk: async () => fakeSdk() });
  const results = await controller.execute([{ preset: "audit", request: "check this" }, { preset: "design-review", request: "no" }]);
  assert.equal(results.length, 2);
  assert.equal(results[0].status, "success");
  assert.equal(results[0].output, "ok");
  assert.equal(results[1].error?.code, "invalid_item");
  assert.match(results[0].id, /^[a-z]+-[a-z]+-[a-z]+$/);
});

test("ws-claude lead activation neither adds to forks nor duplicates", () => {
  assert.deepEqual(addClaudeDelegateIfLead(["read"], undefined), ["read", CLAUDE_DELEGATE_TOOL_NAME]);
  assert.deepEqual(addClaudeDelegateIfLead(["read"], "fork"), ["read"]);
  assert.deepEqual(addClaudeDelegateIfLead([CLAUDE_DELEGATE_TOOL_NAME], undefined), [CLAUDE_DELEGATE_TOOL_NAME]);
});

test("shared controller admits only three items and rejects edit targets for read-only presets", async () => {
  let active = 0; let maximum = 0; const releases: (() => void)[] = [];
  const controller = createClaudeDelegateController(() => process.cwd(), { executable: "/usr/bin/true", loadSdk: async () => ({ query: () => ({ close() {}, async *[Symbol.asyncIterator]() { active += 1; maximum = Math.max(maximum, active); await new Promise<void>((resolve) => releases.push(resolve)); active -= 1; yield { type: "result", subtype: "success", is_error: false, result: "ok", usage: {}, modelUsage: {}, total_cost_usd: 0 }; } }) }) });
  const promise = controller.execute(Array.from({ length: 5 }, () => ({ preset: "audit", request: "x" })));
  await new Promise((resolve) => setTimeout(resolve, 5)); assert.equal(maximum, 3);
  while (releases.length) releases.shift()!(); await new Promise((resolve) => setTimeout(resolve, 5)); while (releases.length) releases.shift()!();
  assert.equal((await promise).filter((item) => item.status === "success").length, 5);
  const invalid = await controller.execute([{ preset: "audit", request: "x", "edit-targets": ["x"] }]);
  assert.equal(invalid[0]?.error?.code, "invalid_item");
});

test("disjoint rewrites run concurrently, edit exact targets, report changed paths, and remain unstaged", async () => {
  const root = mkdtempSync(join(tmpdir(), "ws-claude-rewrite-"));
  try {
    execFileSync("git", ["init", "-q"], { cwd: root });
    writeFileSync(join(root, "first.txt"), "old-first"); writeFileSync(join(root, "second.txt"), "old-second");
    execFileSync("git", ["add", "."], { cwd: root }); execFileSync("git", ["-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "base"], { cwd: root });
    let active = 0; let maximum = 0; const releases: (() => void)[] = [];
    const controller = createClaudeDelegateController(() => root, { executable: process.execPath, loadSdk: async () => ({ query: ({ prompt, options }: any) => ({
      close() {}, async *[Symbol.asyncIterator]() {
        active++; maximum = Math.max(maximum, active); await new Promise<void>(resolve => releases.push(resolve)); active--;
        const first = prompt.includes("first.txt"); const target = join(root, first ? "first.txt" : "second.txt");
        const permission = await options.canUseTool("Write", { file_path: target }, { signal: new AbortController().signal, toolUseID: "t", requestId: "r" });
        assert.equal(permission.behavior, "allow"); writeFileSync(target, `new-${first ? "first" : "second"}`); yield { type: "result", subtype: "success", is_error: false, result: "rewritten" };
      },
    }) }) as any });
    const pending = controller.execute([
      { preset: "rewrite", request: "rewrite first.txt", "edit-targets": ["first.txt"] },
      { preset: "rewrite", request: "rewrite second.txt", "edit-targets": ["second.txt"] },
    ]);
    await new Promise(resolve => setImmediate(resolve)); assert.equal(maximum, 2); for (const release of releases.splice(0)) release();
    const results = await pending;
    assert.deepEqual(results.map(result => result.changed), [["first.txt"], ["second.txt"]]);
    assert.equal(readFileSync(join(root, "first.txt"), "utf8"), "new-first"); assert.equal(readFileSync(join(root, "second.txt"), "utf8"), "new-second");
    assert.deepEqual(execFileSync("git", ["status", "--short"], { cwd: root, encoding: "utf8" }).trimEnd().split("\n").sort(), [" M first.txt", " M second.txt"]);
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test("an authorized new file is reported changed and remains untracked", async () => {
  const root = mkdtempSync(join(tmpdir(), "ws-claude-new-file-"));
  try {
    execFileSync("git", ["init", "-q"], { cwd: root }); mkdirSync(join(root, "parent")); writeFileSync(join(root, "parent", ".keep"), "");
    execFileSync("git", ["add", "."], { cwd: root }); execFileSync("git", ["-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "base"], { cwd: root });
    const controller = createClaudeDelegateController(() => root, { executable: process.execPath, loadSdk: async () => ({ query: ({ options }: any) => ({ close() {}, async *[Symbol.asyncIterator]() {
      const target = join(root, "parent", "lass.md"); const permission = await options.canUseTool("Write", { file_path: target }, { signal: new AbortController().signal, toolUseID: "t", requestId: "r" });
      assert.equal(permission.behavior, "allow"); writeFileSync(target, "new"); yield { type: "result", subtype: "success", is_error: false, result: "created" };
    } }) }) as any });
    const [result] = await controller.execute([{ preset: "rewrite", request: "create", "edit-targets": ["parent/lass.md"] }]);
    assert.deepEqual(result?.changed, ["parent/lass.md"]); assert.equal(readFileSync(join(root, "parent", "lass.md"), "utf8"), "new");
    assert.equal(execFileSync("git", ["status", "--short"], { cwd: root, encoding: "utf8" }), "?? parent/lass.md\n");
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test("overlapping canonical rewrite targets are rejected while a sibling completes", async () => {
  const root = mkdtempSync(join(tmpdir(), "ws-claude-overlap-")); writeFileSync(join(root, "target.txt"), "old"); symlinkSync(join(root, "target.txt"), join(root, "alias.txt"));
  try {
    let release!: () => void; const gate = new Promise<void>(resolve => { release = resolve; }); let queries = 0;
    const controller = createClaudeDelegateController(() => root, { executable: process.execPath, loadSdk: async () => ({ query: () => ({ close() {}, async *[Symbol.asyncIterator]() { queries++; await gate; yield { type: "result", subtype: "success", is_error: false, result: "ok" }; } }) }) as any });
    const pending = controller.execute([
      { preset: "rewrite", request: "one", "edit-targets": ["target.txt"] },
      { preset: "rewrite", request: "two", "edit-targets": ["alias.txt"] },
    ]);
    await new Promise(resolve => setImmediate(resolve)); release(); const results = await pending;
    assert.equal(queries, 1); assert.equal(results[0]?.status, "success"); assert.equal(results[1]?.error?.code, "edit_target_conflict");
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test("actual registration exposes the object envelope and starts no child before execution", async () => {
  let definition: any; let queries = 0; const ref: { current: any } = { current: undefined };
  registerClaudeDelegate({ registerTool: (tool: any) => { definition = tool; } } as any, ref, createToolPreviewTuiRef());
  assert.equal(definition.name, CLAUDE_DELEGATE_TOOL_NAME); assert.equal(definition.parameters.properties.items.type, "array"); assert.equal(queries, 0);
  ref.current = createClaudeDelegateController(() => process.cwd(), { executable: "/usr/bin/true", loadSdk: async () => ({ query: () => { queries += 1; return { close() {}, async *[Symbol.asyncIterator]() { yield { type: "result", subtype: "success", is_error: false, result: "ok", usage: {}, modelUsage: {}, total_cost_usd: 0 }; } }; } }) });
  const result = await definition.execute("id", { items: [{ preset: "consult", request: "x" }] }, new AbortController().signal);
  assert.equal(queries, 1); assert.deepEqual(JSON.parse(result.content[0].text), result.details.items);
});

test("queued invocation cancellation settles queued work without launching it", async () => {
  let starts = 0; const gate: (() => void)[] = []; const controller = createClaudeDelegateController(() => process.cwd(), { executable: "/usr/bin/true", loadSdk: async () => ({ query: () => ({ close() {}, async *[Symbol.asyncIterator]() { starts += 1; await new Promise<void>((resolve) => gate.push(resolve)); yield { type: "result", subtype: "success", is_error: false, result: "ok", usage: {}, modelUsage: {}, total_cost_usd: 0 }; } }) }) });
  const first = controller.execute(Array.from({ length: 3 }, () => ({ preset: "audit", request: "x" }))); await new Promise((resolve) => setTimeout(resolve, 5));
  const cancellation = new AbortController(); const queued = controller.execute([{ preset: "consult", request: "x" }], cancellation.signal); cancellation.abort();
  assert.equal((await queued)[0]?.error?.code, "cancelled"); assert.equal(starts, 3);
  for (const release of gate.splice(0)) release(); await first;
});

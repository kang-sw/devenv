import assert from "node:assert/strict";
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
  const results = await controller.execute([{ preset: "audit", request: "check this" }, { preset: "rewrite", request: "no" }]);
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

test("shared controller admits only three items and rejects later-phase direct fields", async () => {
  let active = 0; let maximum = 0; const releases: (() => void)[] = [];
  const controller = createClaudeDelegateController(() => process.cwd(), { executable: "/usr/bin/true", loadSdk: async () => ({ query: () => ({ close() {}, async *[Symbol.asyncIterator]() { active += 1; maximum = Math.max(maximum, active); await new Promise<void>((resolve) => releases.push(resolve)); active -= 1; yield { type: "result", subtype: "success", is_error: false, result: "ok", usage: {}, modelUsage: {}, total_cost_usd: 0 }; } }) }) });
  const promise = controller.execute(Array.from({ length: 5 }, () => ({ preset: "audit", request: "x" })));
  await new Promise((resolve) => setTimeout(resolve, 5)); assert.equal(maximum, 3);
  while (releases.length) releases.shift()!(); await new Promise((resolve) => setTimeout(resolve, 5)); while (releases.length) releases.shift()!();
  assert.equal((await promise).filter((item) => item.status === "success").length, 5);
  const invalid = await controller.execute([{ preset: "audit", request: "x", "edit-targets": ["x"] }]);
  assert.equal(invalid[0]?.error?.code, "invalid_item");
});

test("actual registration exposes the object envelope and starts no child before execution", async () => {
  let definition: any; let queries = 0; const ref: { current: any } = { current: undefined };
  registerClaudeDelegate({ registerTool: (tool: any) => { definition = tool; } } as any, ref, createToolPreviewTuiRef());
  assert.equal(definition.name, CLAUDE_DELEGATE_TOOL_NAME); assert.equal(definition.parameters.properties.items.type, "array"); assert.equal(queries, 0);
  ref.current = createClaudeDelegateController(() => process.cwd(), { executable: "/usr/bin/true", loadSdk: async () => ({ query: () => { queries += 1; return { close() {}, async *[Symbol.asyncIterator]() { yield { type: "result", subtype: "success", is_error: false, result: "ok", usage: {}, modelUsage: {}, total_cost_usd: 0 }; } }; } }) });
  const result = await definition.execute("id", { items: [{ preset: "consult", request: "x" }] }, new AbortController().signal);
  assert.equal(queries, 1); assert.deepEqual(JSON.parse(result.content[0].text), result.details.items);
});

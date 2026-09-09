import assert from "node:assert/strict";
import { test } from "node:test";
import { addClaudeDelegateIfLead, CLAUDE_DELEGATE_TOOL_NAME, createClaudeDelegateController } from "../src/claude-delegate.ts";

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

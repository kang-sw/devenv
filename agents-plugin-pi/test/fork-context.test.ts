import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { applyForkAffinity, captureForkContext, captureRegisteredTools, compareForkRegistrations, parseForkContext, readForkLaunchContext, restoreForkContext, writePrivateJson } from "../src/fork-context.ts";

describe("ForkContext", () => {
  const context = captureForkContext({
    kind: "task",
    effectiveSystemPrompt: "lead\r\nprompt  ",
    wsBlock: "block",
    parentSessionKey: "parent",
    parentAffinityId: "affinity",
    activeTools: ["read", "ws-fork"],
    registeredTools: [
      { name: "read", description: "Read", parameters: { type: "object" } },
      { name: "ws-fork", description: "Fork", parameters: { type: "object", properties: { prompt: { type: "string" } } } },
    ],
    modelDescriptor: { provider: "openai-codex", api: "openai-codex-responses", model: "x" },
  });

  test("round-trips exact prompt bytes and rejects malformed present metadata", () => {
    assert.deepEqual(parseForkContext(JSON.stringify(context)), context);
    assert.equal(parseForkContext(undefined), undefined, "only absent metadata is legacy");
    assert.throws(() => parseForkContext('{'), /malformed/);
    assert.throws(() => parseForkContext({ version: 1 }), /malformed/);
  });

  test("captures actual registrations in active order and exposes drift", () => {
    const actual = captureRegisteredTools(context.activeTools, context.registeredTools);
    assert.equal(compareForkRegistrations(context.registeredTools, actual), undefined);
    assert.match(compareForkRegistrations(context.registeredTools, [...actual].reverse()) ?? "", /index 0/);
    assert.match(compareForkRegistrations(context.registeredTools, [{ ...actual[0], description: "changed" }, actual[1]]) ?? "", /index 0/);
  });

  test("rejects an envelope that omits present context rather than taking the legacy path", () => {
    const directory = mkdtempSync(join(tmpdir(), "ws-pi-fork-context-"));
    try {
      const path = join(directory, "launch.json");
      writePrivateJson(path, { nonce: "nonce", readinessPath: join(directory, "ready.json") });
      assert.throws(() => readForkLaunchContext({ WS_PI_FORK_CONTEXT: path }), /malformed launch envelope/);
    } finally {
      rmSync(directory, { recursive: true, force: true });
    }
  });

  test("does not trust a parent-copied durable entry", () => {
    const entry = { type: "custom", customType: "ws-pi-fork-context", data: { sessionId: "parent", context } };
    assert.equal(restoreForkContext([entry], "child"), undefined);
    assert.deepEqual(restoreForkContext([entry], "parent"), context);
  });

  test("rewrites only compatible Codex body affinity", () => {
    const payload = { prompt_cache_key: "child", instructions: "same", tools: [] };
    assert.deepEqual(applyForkAffinity(payload, context, context.modelDescriptor, "child-affinity"), { ...payload, prompt_cache_key: "affinity" });
    assert.equal(applyForkAffinity(payload, context, { ...context.modelDescriptor, api: "openai-completions" }, "child-affinity"), undefined);
    assert.equal(applyForkAffinity({ instructions: "same" }, context, context.modelDescriptor, "child-affinity"), undefined);
  });
});

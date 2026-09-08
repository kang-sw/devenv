/**
 * Offline provider-bound regression coverage for the fork prefix contract.
 *
 * These tests deliberately invoke Pi's shipped serializers, not a copied
 * converter. `onPayload` is the provider seam before transport; it captures
 * the exact request and throws before fetch can run, so every case is
 * zero-network. The reference continuation is independently assembled from
 * the original lead history while the fork side is JSON-restored durable
 * ForkContext/history data. No payload keys, schemas, ordering, annotations,
 * or prompt bytes are normalized before comparison.
 */
import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { buildForkInitialMessage, registerFork } from "../src/fork.ts";
import { buildDiscussionForkInitialMessage, registerAsk } from "../src/ask.ts";
import { applyForkAffinity, captureForkContext, captureRegisteredTools, parseForkContext } from "../src/fork-context.ts";
import { forkSessionKeyRefusal } from "../src/bridge.ts";

type RegisteredTool = { name: string; description: string; parameters: unknown };
type ProviderCase = { name: string; provider: string; api: string; chunk: string };

const LOCAL_SDK = join(process.cwd(), "node_modules/@earendil-works/pi-coding-agent");
const GLOBAL_SDK = "/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent";
const PROVIDERS: ProviderCase[] = [
  { name: "anthropic", provider: "anthropic", api: "anthropic-messages", chunk: "anthropic-messages-" },
  { name: "codex", provider: "openai-codex", api: "openai-codex-responses", chunk: "openai-codex-responses-" },
  { name: "completions", provider: "openrouter", api: "openai-completions", chunk: "openai-completions-" },
];

function codexTestToken(): string {
  return `e30.${Buffer.from(JSON.stringify({ "https://api.openai.com/auth": { chatgpt_account_id: "fork-prefix-test" } })).toString("base64url")}.x`;
}

function serializerPath(root: string, prefix: string): string {
  const chunks = join(root, "dist/bundle/chunks");
  assert.ok(existsSync(chunks), `missing installed Pi SDK chunks at ${chunks}`);
  const file = readdirSync(chunks).find((name) => name.startsWith(prefix) && name.endsWith(".js"));
  assert.ok(file, `missing ${prefix} serializer in ${chunks}`);
  return join(chunks, file);
}

async function loadRenderedAppend(root: string, append: string): Promise<{ systemPrompt: string | undefined; append: string[] }> {
  const modulePath = join(root, "dist/core/resource-loader.js");
  assert.ok(existsSync(modulePath), `missing resource loader at ${modulePath}`);
  const mod = await import(modulePath);
  const loader = new mod.DefaultResourceLoader({
    cwd: process.cwd(), agentDir: process.cwd(), noExtensions: true, noSkills: true,
    noPromptTemplates: true, noThemes: true, noContextFiles: true,
    systemPrompt: "Custom base", appendSystemPrompt: [append],
  });
  await loader.reload();
  return { systemPrompt: loader.getSystemPrompt(), append: loader.getAppendSystemPrompt() };
}

async function capturePayload(root: string, provider: ProviderCase, context: unknown, sessionId: string, matrix?: { oauth: boolean; retention: string; midEffort: boolean }): Promise<unknown> {
  const mod = await import(serializerPath(root, provider.chunk));
  const stream = mod.stream as (model: unknown, context: unknown, options: unknown) => AsyncIterable<unknown>;
  let payload: unknown;
  let fetchCalls = 0;
  const model = {
    provider: provider.provider,
    api: provider.api,
    id: provider.name === "anthropic" ? "claude-prefix-test" : "prefix-test",
    baseUrl: matrix ? "https://api.anthropic.com" : "https://pi-prefix-test.invalid/v1",
    ...(matrix ? { reasoning: true, compat: { forceAdaptiveThinking: true, supportsMidConvoEffort: matrix.midEffort } } : {}),
    input: ["text", "image"],
    maxTokens: 8192,
    contextWindow: 128000,
  };
  const result = stream(model, context, {
    apiKey: provider.name === "codex" ? codexTestToken() : matrix?.oauth ? "sk-ant-oat-offline" : "offline-prefix-test",
    sessionId,
    cacheRetention: matrix?.retention ?? "short",
    ...(matrix ? { thinkingEnabled: true, effort: "medium" } : {}),
    fetch: async () => {
      fetchCalls += 1;
      throw new Error("network transport must not run in fork-prefix integration coverage");
    },
    onPayload: (value: unknown) => {
      payload = structuredClone(value);
      throw new Error("captured before provider transport");
    },
  });
  // The serializer reports the intentional callback stop as an error event.
  for await (const _event of result) { /* drain the real SDK stream */ }
  assert.equal(fetchCalls, 0, `${provider.name}: onPayload must stop before network transport`);
  assert.notEqual(payload, undefined, `${provider.name}: real serializer did not reach onPayload`);
  return payload;
}

/** Production registration declarations, collected through their real registerTool calls. */
function actualForkRegistrations(): RegisteredTool[] {
  const definitions: RegisteredTool[] = [];
  const pi = {
    registerTool(definition: RegisteredTool) { definitions.push(definition); },
    getActiveTools: () => definitions.map((definition) => definition.name),
    getAllTools: () => definitions,
    on() {},
  };
  const registry = new Map();
  registerFork(pi as never, { defaultSessionKeyRef: { current: "lead-key" } } as never, registry as never, { cwd: process.cwd() });
  registerAsk(pi as never, { threads: new Map(), ctxRef: { current: undefined } } as never, registry as never);
  return definitions.map(({ name, description, parameters }) => ({ name, description, parameters }));
}

function continuationHistories(): Array<[string, unknown[]]> {
  const toolCall = { role: "assistant", api: "anthropic-messages", provider: "anthropic", model: "claude-prefix-test", providerThinkingLevel: "low", stopReason: "toolUse", content: [{ type: "toolCall", id: "call_1", name: "ws-fork", arguments: { prompt: "inspect" } }, { type: "toolCall", id: "call_2", name: "ws-fork", arguments: { prompt: "inspect again" } }] };
  return [
    ["string-user", [{ role: "user", content: "Lead context with CRLF\r\nand trailing space " }]],
    ["block-user", [{ role: "user", content: [{ type: "text", text: "Lead context with CRLF\r\nand trailing space " }] }]],
    ["consecutive-users", [{ role: "user", content: "First context." }, { role: "user", content: "Second context." }]],
    ["grouped-tool-results", [toolCall, { role: "toolResult", toolCallId: "call_1", toolName: "ws-fork", content: [{ type: "text", text: "tool result" }] }, { role: "toolResult", toolCallId: "call_2", toolName: "ws-fork", content: [{ type: "text", text: "second result" }] }]],
    ["image-user", [{ role: "user", content: [{ type: "image", mimeType: "image/png", data: "AQ==" }] }]],
    ["empty-text-and-thinking", [{ role: "user", content: [{ type: "text", text: "" }] }, { role: "assistant", api: "anthropic-messages", provider: "anthropic", model: "claude-prefix-test", providerThinkingLevel: "low", stopReason: "stop", content: [{ type: "thinking", thinking: "private reasoning", thinkingSignature: "signature" }] }]],
    ["empty-tool-result", [toolCall, { role: "toolResult", toolCallId: "call_1", toolName: "ws-fork", content: [] }]],
  ];
}

function providerTools(registrations: readonly RegisteredTool[]): unknown[] {
  // This is the actual production registration output above, never a test-authored schema.
  return registrations.map((tool) => ({ name: tool.name, description: tool.description, parameters: tool.parameters }));
}

function context(systemPrompt: string, messages: unknown[], tools: unknown[]): unknown {
  return { systemPrompt, messages, tools };
}

function continuationMessage(kind: "task" | "discussion"): string {
  return kind === "task"
    ? buildForkInitialMessage("Inspect the exact inherited prefix and report the result.")
    : buildDiscussionForkInitialMessage("Owner context", "Which decision best preserves the prefix?");
}

describe("fork prefix actual SDK serializers (offline)", () => {
  const registrations = actualForkRegistrations();
  const activeTools = registrations.map((tool) => tool.name);
  const exactPrompt = "Custom base\r\nExplicit append keeps these bytes.  \r\n<ws>manual-old</ws>\n";
  const captured = captureForkContext({
    kind: "task",
    effectiveSystemPrompt: exactPrompt,
    basePromptOptions: { appendSystemPrompt: "Explicit append keeps these bytes.  \r\n" },
    wsBlock: "<ws>manual-old</ws>",
    parentSessionKey: "parent-key",
    parentPiSessionId: "parent-pi",
    parentAffinityId: "lead-transport-id",
    activeTools,
    registeredTools: captureRegisteredTools(activeTools, registrations),
    modelDescriptor: { provider: "openai-codex", api: "openai-codex-responses", model: "prefix-test", endpoint: "https://pi-prefix-test.invalid/v1" },
  });

  test("uses production task/discussion tool registrations without schema or order substitution", () => {
    assert.deepEqual(captured.registeredTools, registrations);
    assert.deepEqual(captured.activeTools, activeTools);
    assert.ok(captured.activeTools.includes("ws-fork"));
    assert.ok(captured.activeTools.includes("ws-ask"));
    assert.ok(captured.activeTools.includes("ws-resolve"));
  });

  for (const [sdkName, root] of [["local-0.84.4", LOCAL_SDK], ["global-0.85.1", GLOBAL_SDK]] as const) {
    test(`${sdkName}: actual resource loader retains explicit append bytes`, async () => {
      const append = "Explicit append keeps these bytes.  \r\n";
      const loaded = await loadRenderedAppend(root, append);
      assert.equal(loaded.systemPrompt, "Custom base");
      assert.deepEqual(loaded.append, [append]);
    });
  }

  test("parent and stale keys are refused before bridge forwarding", () => {
    const previous = process.env.WS_PI_SPAWN_ROLE;
    process.env.WS_PI_SPAWN_ROLE = "fork";
    try {
      assert.match(forkSessionKeyRefusal("parent-key", { ownKey: "child-key", sentinel: "obsidian-latch", parentLeadKey: "parent-key" }) ?? "", /refuses its parent/);
      assert.match(forkSessionKeyRefusal("old-child-key", { ownKey: "child-key", sentinel: "obsidian-latch", previousOwnKeys: ["old-child-key"] }) ?? "", /stale prior/);
      assert.equal(forkSessionKeyRefusal("child-key", { ownKey: "child-key", sentinel: "obsidian-latch", previousOwnKeys: ["old-child-key"] }), undefined);
    } finally {
      if (previous === undefined) delete process.env.WS_PI_SPAWN_ROLE;
      else process.env.WS_PI_SPAWN_ROLE = previous;
    }
  });

  for (const [sdkName, root] of [["local-0.84.4", LOCAL_SDK], ["global-0.85.1", GLOBAL_SDK]] as const) {
    for (const oauth of [false, true]) for (const retention of ["none", "short", "long"]) for (const midEffort of [false, true]) for (const [shape, history] of continuationHistories()) {
      test(`${sdkName}/Anthropic matrix/${oauth ? "OAuth" : "API-key"}/${retention}/${midEffort}/${shape}: three exact recovery generations`, async () => {
        const referenceHistory = structuredClone(history);
        let recoveredHistory = JSON.parse(JSON.stringify(history));
        const configuration = { oauth, retention, midEffort };
        for (let generation = 0; generation < 3; generation++) {
          const fixture = { role: "user", content: `Independent suffix ${generation} Ω\r\n  ` };
          referenceHistory.push(structuredClone(fixture));
          recoveredHistory = JSON.parse(JSON.stringify([...recoveredHistory, fixture]));
          const expected = await capturePayload(root, PROVIDERS[0], context(exactPrompt, referenceHistory, providerTools(registrations)), "lead-reference", configuration);
          const actual = await capturePayload(root, PROVIDERS[0], context(exactPrompt, recoveredHistory, providerTools(registrations)), "fork-child", configuration);
          assert.deepEqual(actual, expected);
          assert.deepEqual(recoveredHistory, referenceHistory);
          assert.deepEqual((actual as { output_config: unknown }).output_config, { effort: midEffort && sdkName === "global-0.85.1" ? "high" : "medium" });
        }
      });
    }
    for (const provider of PROVIDERS) {
      for (const kind of ["task", "discussion"] as const) {
        for (const [shape, originalHistory] of continuationHistories()) {
        test(`${sdkName}/${provider.name}/${shape}: ${kind}, dormant and repeated JSON recovery exactly match an independent continuation`, async () => {
          const newMessage = continuationMessage(kind);
          // Oracle: independently accumulated lead history plus the same new message.
          const referenceMessages = [...structuredClone(originalHistory), { role: "user", content: newMessage }];
          const reference = await capturePayload(root, provider, context(exactPrompt, referenceMessages, providerTools(registrations)), "lead-transport-id");

          // Fork: durable metadata and inherited Pi entries both survive two JSON recovery generations.
          const firstRecovery = parseForkContext(JSON.stringify(captured));
          const secondRecovery = parseForkContext(JSON.stringify(firstRecovery));
          assert.ok(secondRecovery);
          const restoredMessages = JSON.parse(JSON.stringify(originalHistory));
          restoredMessages.push({ role: "user", content: newMessage });
          // Deliberately changed child files/resources are not part of this input. The captured exact
          // prompt/block is the only permitted fork system prefix.
          const changedChildResources = "CHANGED CHILD GUIDE/SKILLS/APPEND MUST NOT REBUILD PREFIX";
          assert.notEqual(secondRecovery.effectiveSystemPrompt, changedChildResources);
          const childSerialized = await capturePayload(root, provider, context(secondRecovery.effectiveSystemPrompt, restoredMessages, providerTools(secondRecovery.registeredTools)), "fork-transport-id");
          // The only cross-session body difference that production is allowed to change is the
          // guarded Codex affinity key. This invokes the same production hook helper, not a
          // comparator-side deletion or a copied serializer rule.
          const actual = provider.api === "openai-codex-responses"
            ? applyForkAffinity(childSerialized, secondRecovery, secondRecovery.modelDescriptor, "fork-transport-id") ?? childSerialized
            : childSerialized;
          assert.deepEqual(actual, reference, `${sdkName}/${provider.name}/${kind}: raw payload mismatch after durable recovery`);

          // Earlier lead request remains visible separately. Native serializers may legitimately advance
          // cache annotations for the added suffix; the exact continuation comparison above is the oracle.
          const earlier = await capturePayload(root, provider, context(exactPrompt, structuredClone(originalHistory), providerTools(registrations)), "lead-transport-id");
          assert.notDeepEqual(earlier, reference, `${sdkName}/${provider.name}/${shape}/${kind}: added suffix unexpectedly produced an identical raw request`);
        });
        }
      }
    }
  }
});

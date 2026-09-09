import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { SessionManager } from "@earendil-works/pi-coding-agent";
import { Agent } from "../node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-agent-core/dist/index.js";
import { createAssistantMessageEventStream } from "../node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-ai/dist/utils/event-stream.js";
import { dedupeRead, playbookReadKey, wsSkillKey } from "../src/playbook-read-dedupe.ts";

const body = "# Title\n\n## Detail\n\n### Leaf\ntext";
const call = (id: string, name: string, arguments_: Record<string, unknown>) => ({ type: "message", message: { role: "assistant", content: [{ type: "toolCall", id, name, arguments: arguments_ }] } });
const result = (id: string, text: string, isError = false) => ({ type: "message", message: { role: "toolResult", toolCallId: id, content: [{ type: "text", text }], isError } });

test("playbook repeat is full, pointer, then full and ignores session_key/map ordering", () => {
  const key = playbookReadKey({ name: "lead", context: { b: 2, a: 1 }, session_key: "now" });
  assert.equal(key, playbookReadKey({ context: { a: 1, b: 2 }, name: "lead", session_key: "other" }));
  const first = [call("one", "ws__playbook_read", { name: "lead", context: { a: 1, b: 2 } }), result("one", body)];
  const second = dedupeRead(first, "two", "playbook.read", key, body);
  assert.equal(second.deduped, true);
  assert.match(second.text, /`one`/);
  assert.match(second.text, /# Title; ## Detail; ### Leaf/);
  const third = dedupeRead([...first, call("two", "ws__playbook_read", { name: "lead", context: { b: 2, a: 1 } }), result("two", second.text)], "three", "playbook.read", key, body);
  assert.deepEqual(third, { text: body, deduped: false });
  const fourth = dedupeRead([...first, call("two", "ws__playbook_read", { name: "lead", context: { a: 1, b: 2 } }), result("two", second.text), call("three", "ws__playbook_read", { name: "lead", context: { a: 1, b: 2 } }), result("three", body)], "four", "playbook.read", key, body);
  assert.deepEqual(fourth, { text: body, deduped: false });
});

test("only outer session_key is ignored; context.session_key remains semantic substitution data", () => {
  assert.equal(playbookReadKey({ name: "lead", context: { session_key: "context-a" }, session_key: "outer-a" }), playbookReadKey({ name: "lead", context: { session_key: "context-a" }, session_key: "outer-b" }));
  assert.notEqual(playbookReadKey({ name: "lead", context: { session_key: "context-a" } }), playbookReadKey({ name: "lead", context: { session_key: "context-b" } }));
});

test("only an authentic successful visible full result validates a pointer", () => {
  const key = playbookReadKey({ name: "lead" });
  const first = [call("one", "ws__playbook_read", { name: "lead" }), result("one", body)];
  const pointer = dedupeRead(first, "two", "playbook.read", key, body).text;
  const forged = pointer.replace('"originalToolCallId":"one"', '"originalToolCallId":"missing"');
  assert.deepEqual(dedupeRead([...first, call("two", "ws__playbook_read", { name: "lead" }), result("two", forged)], "three", "playbook.read", key, body), { text: body, deduped: false });
  assert.equal(dedupeRead([call("one", "ws__playbook_read", { name: "lead" }), result("one", body, true)], "two", "playbook.read", key, body).deduped, false);
  assert.equal(dedupeRead([call("other", "ws__playbook_read", { name: "other" }), result("other", body)], "two", "playbook.read", key, body).deduped, false);
});

test("malformed, stale, and ambiguous pointer provenance fail closed to the fresh body", () => {
  const key = playbookReadKey({ name: "lead" });
  const first = [call("one", "ws__playbook_read", { name: "lead" }), result("one", body)];
  const valid = dedupeRead(first, "two", "playbook.read", key, body).text;
  for (const pointer of ["text\n<!-- ws-pi-read-dedupe-v1 broken -->", valid.replace('"key":"{\\"name\\":\\"lead\\"}"', '"key":"stale"')]) {
    assert.deepEqual(dedupeRead([...first, call("two", "ws__playbook_read", { name: "lead" }), result("two", pointer)], "three", "playbook.read", key, body), { text: body, deduped: false });
  }
  assert.deepEqual(dedupeRead([...first, result("one", body), call("two", "ws__playbook_read", { name: "lead" }), result("two", valid)], "three", "playbook.read", key, body), { text: body, deduped: false });
  const missingEnvelope = valid.split("\n")[0]!;
  assert.deepEqual(dedupeRead([...first, call("two", "ws__playbook_read", { name: "lead" }), result("two", missingEnvelope)], "three", "playbook.read", key, body), { text: body, deduped: false });
});

test("ws-skill is isolated from playbook reads and includes args in its key", () => {
  const key = wsSkillKey({ name: "implementer", args: "a" });
  assert.notEqual(key, wsSkillKey({ name: "implementer", args: "b" }));
  const entries = [call("one", "ws__playbook_read", { name: "implementer" }), result("one", body), call("two", "ws-skill", { name: "implementer", args: "a" }), result("two", body)];
  assert.equal(dedupeRead(entries, "three", "ws-skill", key, body).deduped, true);
  assert.equal(dedupeRead(entries, "three", "playbook.read", playbookReadKey({ name: "implementer" }), body).deduped, true);
});

test("current call and omitted contexts do not count", () => {
  const key = playbookReadKey({ name: "lead" });
  const current = [call("now", "ws__playbook_read", { name: "lead" }), result("now", body)];
  assert.equal(dedupeRead(current, "now", "playbook.read", key, body).deduped, false);
  assert.equal(dedupeRead([], "next", "playbook.read", key, body).deduped, false);
});

test("installed SessionManager exposes the finalized assistant call during execute-time scanning, while branch and fork windows remain Pi-owned", () => {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-dedupe-sdk-"));
  const manager = SessionManager.create(directory, join(directory, "sessions"));
  const root = manager.appendMessage({ role: "user", content: "start", timestamp: 1 });
  manager.appendMessage({ role: "assistant", api: "openai-completions", provider: "test", model: "offline", providerThinkingLevel: "off", stopReason: "toolUse", timestamp: 2, content: [{ type: "toolCall", id: "one", name: "ws__playbook_read", arguments: { name: "lead" } }] });
  manager.appendMessage({ role: "toolResult", toolCallId: "one", toolName: "ws__playbook_read", content: [{ type: "text", text: body }], isError: false, timestamp: 3 });
  // This mirrors Pi's finalized-assistant-before-execute ordering without a
  // model stream: the real SDK persists call two, then execute scans its real
  // active window and must exclude its own id while retaining call one.
  manager.appendMessage({ role: "assistant", api: "openai-completions", provider: "test", model: "offline", providerThinkingLevel: "off", stopReason: "toolUse", timestamp: 4, content: [{ type: "toolCall", id: "two", name: "ws__playbook_read", arguments: { name: "lead" } }] });
  const active = manager.buildContextEntries();
  assert.ok(JSON.stringify(active).includes('"id":"two"'), "SDK active context contains the in-flight assistant tool call");
  assert.equal(dedupeRead(active, "two", "playbook.read", playbookReadKey({ name: "lead" }), body).deduped, true);
  const fork = SessionManager.forkFrom(manager.getSessionFile()!, join(directory, "fork"), join(directory, "fork-sessions"));
  assert.equal(dedupeRead(fork.buildContextEntries(), "four", "playbook.read", playbookReadKey({ name: "lead" }), body).deduped, true, "the fork retains its inherited active prefix");

  manager.branch(root);
  manager.appendMessage({ role: "user", content: "rewritten", timestamp: 5 });
  manager.appendMessage({ role: "assistant", api: "openai-completions", provider: "test", model: "offline", providerThinkingLevel: "off", stopReason: "toolUse", timestamp: 6, content: [{ type: "toolCall", id: "three", name: "ws__playbook_read", arguments: { name: "lead" } }] });
  assert.equal(dedupeRead(manager.buildContextEntries(), "three", "playbook.read", playbookReadKey({ name: "lead" }), body).deduped, false, "rewound abandoned reads are absent from the SDK active path");

  const compacted = SessionManager.inMemory(directory);
  compacted.appendMessage({ role: "assistant", api: "openai-completions", provider: "test", model: "offline", providerThinkingLevel: "off", stopReason: "toolUse", timestamp: 7, content: [{ type: "toolCall", id: "old", name: "ws__playbook_read", arguments: { name: "lead" } }] });
  compacted.appendMessage({ role: "toolResult", toolCallId: "old", toolName: "ws__playbook_read", content: [{ type: "text", text: body }], isError: false, timestamp: 8 });
  const kept = compacted.appendMessage({ role: "user", content: "post-compact", timestamp: 9 });
  compacted.appendCompaction("summary", kept, 100);
  compacted.appendMessage({ role: "assistant", api: "openai-completions", provider: "test", model: "offline", providerThinkingLevel: "off", stopReason: "toolUse", timestamp: 10, content: [{ type: "toolCall", id: "after-compact", name: "ws__playbook_read", arguments: { name: "lead" } }] });
  assert.equal(dedupeRead(compacted.buildContextEntries(), "after-compact", "playbook.read", playbookReadKey({ name: "lead" }), body).deduped, false, "SDK compaction removes reads before its kept entry");

});

test("installed agent-core invokes a registered tool only after real SessionManager persistence of its assistant call", async () => {
  const manager = SessionManager.inMemory("/tmp/ws-pi-dedupe-agent-core");
  let observed = false;
  const model = { provider: "offline", api: "openai-completions", id: "offline", name: "offline", reasoning: false, input: ["text"], contextWindow: 1024, maxTokens: 128, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } };
  const assistant = { role: "assistant", api: "openai-completions", provider: "offline", model: "offline", providerThinkingLevel: "off", stopReason: "toolUse", timestamp: 2, content: [{ type: "toolCall", id: "live-call", name: "live-read", arguments: {} }] };
  const agent = new Agent({
    initialState: {
      systemPrompt: "offline", model, thinkingLevel: "off",
      tools: [{ name: "live-read", label: "live-read", description: "fixture", parameters: { type: "object", properties: {} }, execute: async (id: string) => {
        observed = id === "live-call" && manager.buildContextEntries().some((entry: any) => entry.message?.role === "assistant" && entry.message.content?.some((part: any) => part.type === "toolCall" && part.id === id));
        return { content: [{ type: "text", text: "ok" }], details: {} };
      } }] as any,
    },
    shouldStopAfterTurn: () => true,
    streamFn: () => {
      const stream = createAssistantMessageEventStream();
      stream.push({ type: "start", partial: assistant } as any);
      stream.push({ type: "done", reason: "toolUse", message: assistant } as any);
      stream.end(assistant as any);
      return stream;
    },
  });
  agent.subscribe((event: any) => {
    if (event.type === "message_end") manager.appendMessage(event.message);
  });
  await agent.prompt("run");
  assert.equal(observed, true, "real agent-core execution observes the assistant tool call in the real SessionManager active context");
});

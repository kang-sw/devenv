import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { SessionManager } from "@earendil-works/pi-coding-agent";
import { Agent } from "../node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-agent-core/dist/index.js";
import { createAssistantMessageEventStream } from "../node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-ai/dist/utils/event-stream.js";
import { dedupePlaybook, dedupeRead, workflowManualKey, workflowManualResultText, wsSkillKey } from "../src/playbook-read-dedupe.ts";

const body = "# Title\n\n## Detail\n\n### Leaf\ntext";
const call = (id: string, name: string, arguments_: Record<string, unknown>) => ({ type: "message", message: { role: "assistant", content: [{ type: "toolCall", id, name, arguments: arguments_ }] } });
const result = (id: string, text: string, isError = false) => ({ type: "message", message: { role: "toolResult", toolCallId: id, content: [{ type: "text", text }], isError } });

test("playbook repeat is full, pointer, then full and ignores session_key/context", () => {
  const first = [call("one", "ws__playbook_read", { name: "lead", context: { a: 1, b: 2 } }), result("one", body)];
  const second = dedupePlaybook(first, "two", "lead", body, "result");
  assert.equal(second.deduped, true);
  const [prose, provenance] = second.text.split("\n");
  assert.equal(prose, "The result is unchanged. Continue using the full result already present 1 tool call ago. Do not read it again (headings: # Title; ## Detail; ### Leaf).");
  assert.doesNotMatch(prose!, /one/, "the original toolCallId belongs only in machine provenance");
  assert.equal(provenance, '<!-- ws-pi-read-dedupe-v1 {"family":"playbook","key":"{\\"name\\":\\"lead\\"}","originalToolCallId":"one"} -->');
  // Bodies compare trimmed; the call's context and session_key never separate them.
  assert.equal(dedupePlaybook([call("one", "ws__playbook_read", { name: "lead", context: { x: 1 }, session_key: "k" }), result("one", `\n${body}\n`)], "two", "lead", body, "result").deduped, true);
  assert.equal(dedupePlaybook(first, "two", "lead", `${body} changed`, "result").deduped, false, "a different body is a fresh read");
  const third = dedupePlaybook([...first, call("two", "ws__playbook_read", { name: "lead" }), result("two", second.text)], "three", "lead", body, "result");
  assert.deepEqual(third, { text: body, deduped: false });
  const fourth = dedupePlaybook([...first, call("two", "ws__playbook_read", { name: "lead" }), result("two", second.text), call("three", "ws__playbook_read", { name: "lead" }), result("three", body)], "four", "lead", body, "result");
  assert.deepEqual(fourth, { text: body, deduped: false });
});

test("a pointer candidate of the unit, forged or stale, never suppresses a read; errors and other names are no copy", () => {
  const first = [call("one", "ws__playbook_read", { name: "lead" }), result("one", body)];
  const pointer = dedupePlaybook(first, "two", "lead", body, "result").text;
  const forged = pointer.replace('"originalToolCallId":"one"', '"originalToolCallId":"missing"');
  const stale = pointer.replace('\\"lead\\"', '\\"stale\\"');
  for (const text of [forged, stale, "text\n<!-- ws-pi-read-dedupe-v1 broken -->", pointer.split("\n")[0]!]) {
    assert.deepEqual(dedupePlaybook([...first, call("two", "ws__playbook_read", { name: "lead" }), result("two", text)], "three", "lead", body, "result"), { text: body, deduped: false }, text);
  }
  assert.equal(dedupePlaybook([call("one", "ws__playbook_read", { name: "lead" }), result("one", body, true)], "two", "lead", body, "result").deduped, false);
  assert.equal(dedupePlaybook([call("other", "ws__playbook_read", { name: "other" }), result("other", body)], "two", "lead", body, "result").deduped, false);
  assert.deepEqual(dedupePlaybook([...first, result("one", body)], "two", "lead", body, "result"), { text: body, deduped: false }, "two visible copies point at neither");
});

test("one playbook unit across ws-skill, typed /skill:, and ws__playbook_read copies", () => {
  const skill = `# Lead\n\n${body}\n\nShim tail.`;
  const typed = (id: string | undefined, text: string) => ({ type: "message", ...(id === undefined ? {} : { id }), message: { role: "user", content: [{ type: "text", text }] } });
  // An inlined ws-skill copy, then a read: Pi's result-form pointer at the ws-skill call.
  const viaSkill = [call("s", "ws-skill", { name: "lead" }), result("s", skill)];
  const read = dedupePlaybook(viaSkill, "r", "lead", body, "result");
  assert.equal(read.deduped, true);
  assert.match(read.text, /^The result is unchanged\. Continue using the full result already present 1 tool call ago\. Do not read it again/);
  assert.match(read.text, /"originalToolCallId":"s"/);
  // A read, then a skill that would inline the same body: the block form, keyed the same.
  const viaRead = [call("r", "ws__playbook_read", { name: "lead" }), result("r", body)];
  const block = dedupePlaybook(viaRead, "s", "lead", body, "block");
  assert.equal(block.deduped, true);
  assert.match(block.text, /^The full `lead` procedure is already present 1 tool call ago; follow it and do not read it again \(headings: # Title; ## Detail; ### Leaf\)\.\n<!-- ws-pi-read-dedupe-v1 \{"family":"playbook","key":"\{\\"name\\":\\"lead\\"\}","originalToolCallId":"r"\} -->$/);
  assert.match(dedupePlaybook(viaRead, undefined, "lead", body, "block").text, /already present earlier in this conversation;/, "a typed expansion has no current call to count from");
  // A typed copy is provenanced by its session entry id; one without an id is never pointed at.
  const viaTyped = [typed("e1", `<skill name="lead" location="/x/SKILL.md">\nReferences are relative to /x.\n\n${skill}\n</skill>`)];
  const fromTyped = dedupePlaybook(viaTyped, "r", "lead", body, "result");
  assert.match(fromTyped.text, /already present in an earlier message of this conversation\. Do not read it again/);
  assert.match(fromTyped.text, /"originalEntryId":"e1"/);
  assert.equal(dedupePlaybook([typed(undefined, skill)], "r", "lead", body, "result").deduped, false);
  // Every pointer kind of the unit counts toward the after-pointer rule.
  const afterBlock = [...viaRead, call("s", "ws-skill", { name: "lead" }), result("s", `# Lead\n\n${block.text}\n\nShim tail.`)];
  assert.deepEqual(dedupePlaybook(afterBlock, "t", "lead", body, "result"), { text: body, deduped: false });
  const afterTyped = [...viaRead, typed("e2", `<skill name="lead" location="/x/SKILL.md">\n\n${dedupePlaybook(viaRead, undefined, "lead", body, "block").text}\n</skill>`)];
  assert.deepEqual(dedupePlaybook(afterTyped, "t", "lead", body, "result"), { text: body, deduped: false });
  // A marker of another unit, or an unparsable one, is no pointer of this unit.
  const otherUnit = dedupePlaybook([call("o", "ws__playbook_read", { name: "other" }), result("o", body)], undefined, "other", body, "block").text;
  assert.equal(dedupePlaybook([...viaRead, typed("e3", otherUnit), typed("e4", "<!-- ws-pi-read-dedupe-v1 {broken -->")], "t", "lead", body, "result").deduped, true);
});

test("ws-skill whole-result dedupe keys by name only and ignores playbook reads", () => {
  const key = wsSkillKey({ name: "implementer", args: "b" });
  assert.equal(key, wsSkillKey({ name: "implementer", args: "a" }));
  assert.equal(key, wsSkillKey({ name: "implementer" }));
  assert.equal(dedupeRead([call("one", "ws__playbook_read", { name: "implementer" }), result("one", body)], "three", "ws-skill", key, body).deduped, false);
  const entries = [call("one", "ws__playbook_read", { name: "implementer" }), result("one", body), call("two", "ws-skill", { name: "implementer", args: "a" }), result("two", body)];
  assert.equal(dedupeRead(entries, "three", "ws-skill", key, body).deduped, true);
});

test("current call and omitted contexts do not count", () => {
  const current = [call("now", "ws__playbook_read", { name: "lead" }), result("now", body)];
  assert.equal(dedupePlaybook(current, "now", "lead", body, "result").deduped, false);
  assert.equal(dedupePlaybook([], "next", "lead", body, "result").deduped, false);
});

test("a typed /skill: copy compacted away yields the full body", () => {
  const manager = SessionManager.inMemory(mkdtempSync(join(tmpdir(), "ws-pi-dedupe-typed-compact-")));
  manager.appendMessage({ role: "user", content: `<skill name="lead" location="/x/SKILL.md">\n\n${body}\n</skill>`, timestamp: 1 });
  assert.equal(dedupePlaybook(manager.buildContextEntries(), undefined, "lead", body, "block").deduped, true, "a visible typed copy carries its SDK entry id");
  const kept = manager.appendMessage({ role: "user", content: "post-compact", timestamp: 2 });
  manager.appendCompaction("summary", kept, 100);
  assert.deepEqual(dedupePlaybook(manager.buildContextEntries(), undefined, "lead", body, "block"), { text: body, deduped: false });
});

test("workflow_manual family: keyed by resolved session key, full text across text items, full then pointer then full", () => {
  const state = "Mapping line.\n\n## Session Key\nk\n\n## Session State\n### Todos\n(no todos)";
  const advisory = "> [!note]\n> advisory";
  const key = workflowManualKey("k");
  const resolve = { workflowManualSessionKey: (args: Record<string, unknown>) => args.session_key ?? "k" };
  const multi = (id: string) => ({ type: "message", message: { role: "toolResult", toolCallId: id, content: [{ type: "text", text: state }, { type: "text", text: advisory }], isError: false } });
  const fresh = workflowManualResultText([{ type: "text", text: state }, { type: "text", text: advisory }])!;
  assert.equal(fresh, `${state}\n${advisory}`);

  const first = [call("one", "ws__workflow_manual", {}), multi("one")];
  const second = dedupeRead(first, "two", "workflow_manual", key, fresh, resolve);
  assert.equal(second.deduped, true, "an omitted prior session_key resolves to the current key");
  const [prose] = second.text.split("\n");
  // workflow_manual is mutable session state: the pointer asks for a refresh
  // after mutation instead of forbidding a re-read.
  assert.equal(prose, "The result is unchanged. Continue using the full result already present 1 tool call ago as your current session state; call workflow_manual again after you change agenda, todos, or notes. (headings: ## Session Key; ## Session State; ### Todos)");
  assert.doesNotMatch(second.text, /Do not read it again/);
  assert.match(second.text, /"family":"workflow_manual"/);
  const far = dedupeRead([...first, call("x", "read", {}), call("two", "ws__workflow_manual", {})], "two", "workflow_manual", key, fresh, resolve);
  assert.match(far.text, /^The result is unchanged\. Continue using the full result already present 2 tool calls ago as your current session state;/);
  // A first-text-only comparison would wrongly match when only the advisory differs.
  assert.equal(dedupeRead(first, "two", "workflow_manual", key, state, resolve).deduped, false);
  // Pointer provenance for the new family validates; the next repeat is full.
  const third = dedupeRead([...first, call("two", "ws__workflow_manual", {}), result("two", second.text)], "three", "workflow_manual", key, fresh, resolve);
  assert.deepEqual(third, { text: fresh, deduped: false });
  // Other session keys, and other families with the same text, do not count.
  assert.equal(dedupeRead([call("one", "ws__workflow_manual", { session_key: "other" }), multi("one")], "two", "workflow_manual", key, fresh, resolve).deduped, false);
  assert.equal(dedupeRead([call("one", "ws__playbook_read", { name: "lead" }), multi("one")], "two", "workflow_manual", key, fresh, resolve).deduped, false);
});

test("workflow_manual family: a compacted-away first result yields the full text", () => {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-dedupe-manual-compact-"));
  const text = "Mapping line.\n\n## Session State\n### Todos\n(no todos)";
  const key = workflowManualKey("k");
  const resolve = { workflowManualSessionKey: () => "k" };
  const manager = SessionManager.inMemory(directory);
  manager.appendMessage({ role: "assistant", api: "openai-completions", provider: "test", model: "offline", providerThinkingLevel: "off", stopReason: "toolUse", timestamp: 1, content: [{ type: "toolCall", id: "old", name: "ws__workflow_manual", arguments: {} }] });
  manager.appendMessage({ role: "toolResult", toolCallId: "old", toolName: "ws__workflow_manual", content: [{ type: "text", text }], isError: false, timestamp: 2 });
  manager.appendMessage({ role: "assistant", api: "openai-completions", provider: "test", model: "offline", providerThinkingLevel: "off", stopReason: "toolUse", timestamp: 3, content: [{ type: "toolCall", id: "visible", name: "ws__workflow_manual", arguments: {} }] });
  assert.equal(dedupeRead(manager.buildContextEntries(), "visible", "workflow_manual", key, text, resolve).deduped, true, "the visible prior result is a dedupe source");
  manager.appendMessage({ role: "toolResult", toolCallId: "visible", toolName: "ws__workflow_manual", content: [{ type: "text", text }], isError: false, timestamp: 4 });
  const kept = manager.appendMessage({ role: "user", content: "post-compact", timestamp: 5 });
  manager.appendCompaction("summary", kept, 100);
  manager.appendMessage({ role: "assistant", api: "openai-completions", provider: "test", model: "offline", providerThinkingLevel: "off", stopReason: "toolUse", timestamp: 6, content: [{ type: "toolCall", id: "after", name: "ws__workflow_manual", arguments: {} }] });
  assert.deepEqual(dedupeRead(manager.buildContextEntries(), "after", "workflow_manual", key, text, resolve), { text, deduped: false });
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
  assert.equal(dedupePlaybook(active, "two", "lead", body, "result").deduped, true);
  const fork = SessionManager.forkFrom(manager.getSessionFile()!, join(directory, "fork"), join(directory, "fork-sessions"));
  assert.equal(dedupePlaybook(fork.buildContextEntries(), "four", "lead", body, "result").deduped, true, "the fork retains its inherited active prefix");

  manager.branch(root);
  manager.appendMessage({ role: "user", content: "rewritten", timestamp: 5 });
  manager.appendMessage({ role: "assistant", api: "openai-completions", provider: "test", model: "offline", providerThinkingLevel: "off", stopReason: "toolUse", timestamp: 6, content: [{ type: "toolCall", id: "three", name: "ws__playbook_read", arguments: { name: "lead" } }] });
  assert.equal(dedupePlaybook(manager.buildContextEntries(), "three", "lead", body, "result").deduped, false, "rewound abandoned reads are absent from the SDK active path");

  const compacted = SessionManager.inMemory(directory);
  compacted.appendMessage({ role: "assistant", api: "openai-completions", provider: "test", model: "offline", providerThinkingLevel: "off", stopReason: "toolUse", timestamp: 7, content: [{ type: "toolCall", id: "old", name: "ws__playbook_read", arguments: { name: "lead" } }] });
  compacted.appendMessage({ role: "toolResult", toolCallId: "old", toolName: "ws__playbook_read", content: [{ type: "text", text: body }], isError: false, timestamp: 8 });
  const kept = compacted.appendMessage({ role: "user", content: "post-compact", timestamp: 9 });
  compacted.appendCompaction("summary", kept, 100);
  compacted.appendMessage({ role: "assistant", api: "openai-completions", provider: "test", model: "offline", providerThinkingLevel: "off", stopReason: "toolUse", timestamp: 10, content: [{ type: "toolCall", id: "after-compact", name: "ws__playbook_read", arguments: { name: "lead" } }] });
  assert.equal(dedupePlaybook(compacted.buildContextEntries(), "after-compact", "lead", body, "result").deduped, false, "SDK compaction removes reads before its kept entry");

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

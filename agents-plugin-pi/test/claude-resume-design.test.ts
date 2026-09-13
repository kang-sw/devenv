import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { createClaudeDelegateController } from "../src/claude-delegate.ts";
import { createClaudeDesignReviewContextProvider } from "../src/claude-design-review.ts";

const terminal = (result: string, sessionId: string) => ({ type: "result", subtype: "success", is_error: false, result, session_id: sessionId, usage: {}, modelUsage: {}, total_cost_usd: 0 });

function textResult(text: string) { return { content: [{ type: "text", text }] }; }

test("resume continues the mapped Claude session under the same opaque handle", async () => {
  const calls: Array<{ prompt: string; options: any }> = [];
  const controller = createClaudeDelegateController(() => "/tmp", {
    executable: process.execPath,
    loadSdk: async () => ({ query: ({ prompt, options }: any) => {
      calls.push({ prompt, options });
      const resumed = options.resume === "private-session-1";
      return { close() {}, async *[Symbol.asyncIterator]() { yield terminal(resumed ? "remembered cobalt" : "stored cobalt", "private-session-1"); } };
    } }) as any,
  });

  const [first] = await controller.execute([{ preset: "consult", request: "Remember cobalt." }]);
  assert.equal(first?.status, "success");
  assert.equal(first?.output, "stored cobalt");
  assert.equal(JSON.stringify(first).includes("private-session-1"), false);
  assert.equal(calls[0]?.options.persistSession, true);
  assert.equal(calls[0]?.options.resume, undefined);

  const [resumed] = await controller.execute([{ resume: first!.id, request: "What fact did I state?" }]);
  assert.equal(resumed?.id, first?.id);
  assert.equal(resumed?.output, "remembered cobalt");
  assert.equal(calls[1]?.options.resume, "private-session-1");
  assert.equal(calls[1]?.prompt, "What fact did I state?");
  assert.equal(JSON.stringify(resumed).includes("private-session-1"), false);

  const beforeUnknown = calls.length;
  const [unknown] = await controller.execute([{ resume: "cedar-dawn-elm", request: "continue" }]);
  assert.equal(unknown?.id, "cedar-dawn-elm");
  assert.equal(unknown?.error?.code, "unknown_resume");
  assert.equal(calls.length, beforeUnknown);
  await controller.shutdown();
});

test("concurrent continuation of one handle is rejected without opening a second session writer", async () => {
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  let calls = 0;
  const controller = createClaudeDelegateController(() => "/tmp", {
    executable: process.execPath,
    loadSdk: async () => ({ query: ({ options }: any) => ({ close() {}, async *[Symbol.asyncIterator]() {
      calls++;
      if (options.resume) await gate;
      yield terminal("ok", "private-session-2");
    } }) }) as any,
  });
  const [first] = await controller.execute([{ preset: "audit", request: "first" }]);
  const continuing = controller.execute([{ resume: first!.id, request: "one" }]);
  await new Promise(resolve => setImmediate(resolve));
  const [conflict] = await controller.execute([{ resume: first!.id, request: "two" }]);
  assert.equal(conflict?.error?.code, "resume_conflict");
  assert.equal(calls, 2);
  release();
  assert.equal((await continuing)[0]?.status, "success");
  await controller.shutdown();
});

test("design-review injects the canonical playbook and complete parent-side read bundle without credentials or child tools", async () => {
  const root = mkdtempSync(join(tmpdir(), "ws-claude-design-"));
  try {
    mkdirSync(join(root, "ai-docs", "tickets", "ready"), { recursive: true });
    mkdirSync(join(root, "ai-docs", "tickets", "todo"), { recursive: true });
    mkdirSync(join(root, "ai-docs", "ref"), { recursive: true });
    const target = "ai-docs/tickets/ready/260101-feat-target.md";
    const peer = "ai-docs/tickets/ready/260102-feat-peer.md";
    const parent = "ai-docs/tickets/todo/260100-epic-parent.md";
    const extra = "ai-docs/ref/context.md";
    writeFileSync(join(root, target), "---\ntitle: target\nparent: 260100-epic-parent\n---\n# target body\n");
    writeFileSync(join(root, peer), "---\ntitle: peer\n---\n# peer body\n");
    writeFileSync(join(root, parent), "---\ntitle: parent\n---\n# parent invariant\n");
    writeFileSync(join(root, extra), "# supplied reference\n");
    const calls: Array<{ name: string; args: Record<string, unknown> }> = [];
    const access = { callTool: async (name: string, args: Record<string, unknown>) => {
      calls.push({ name, args });
      if (name === "playbook.read") return textResult("CANONICAL DESIGN CRITERIA\nverdict: <pass|concern|block>");
      if (name === "tickets.query" && Array.isArray(args.statuses)) return textResult(JSON.stringify([{ path: target }, { path: peer }]));
      if (name === "tickets.query" && args.ticket_stem === "260100-epic-parent") return textResult(JSON.stringify({ path: parent }));
      throw new Error(`unexpected ${name}`);
    } };
    let captured: { prompt: string; options: any } | undefined;
    const controller = createClaudeDelegateController(() => root, {
      executable: process.execPath,
      designReviewContext: createClaudeDesignReviewContextProvider(access),
      loadSdk: async () => ({ query: ({ prompt, options }: any) => {
        captured = { prompt, options };
        return { close() {}, async *[Symbol.asyncIterator]() { yield terminal("verdict: pass", "private-design-session"); } };
      } }) as any,
    });
    const [result] = await controller.execute([{ preset: "design-review", request: `Ticket: ${target}\nRelations:\nnone`, paths: [extra] }]);
    assert.equal(result?.output, "verdict: pass");
    assert.deepEqual(captured?.options.tools, ["Read", "Grep", "Glob", "WebSearch", "WebFetch"]);
    assert.deepEqual(captured?.options.mcpServers, {});
    assert.match(captured?.options.systemPrompt.append, /CANONICAL DESIGN CRITERIA/);
    assert.match(captured?.options.systemPrompt.append, /do not call ws tools or spawn explorers/i);
    assert.match(captured?.prompt ?? "", /# target body/);
    assert.match(captured?.prompt ?? "", /# peer body/);
    assert.match(captured?.prompt ?? "", /# parent invariant/);
    assert.match(captured?.prompt ?? "", /# supplied reference/);
    assert.match(captured?.prompt ?? "", /Relations:\nnone/);
    assert.equal(JSON.stringify(captured).includes("finishing-hazelnut-tuesday"), false);
    assert.equal(calls.every(call => !("session_key" in call.args)), true);
    assert.deepEqual(calls.map(call => call.name), ["playbook.read", "tickets.query", "tickets.query"]);
    await controller.shutdown();
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test("design-review fails per item when its structured request or essential lookup context is missing", async () => {
  let queries = 0;
  const provider = createClaudeDesignReviewContextProvider({ callTool: async (name) => name === "playbook.read" ? textResult("canonical") : { isError: true, content: [{ type: "text", text: "missing" }] } });
  const controller = createClaudeDelegateController(() => "/tmp", {
    executable: process.execPath,
    designReviewContext: provider,
    loadSdk: async () => { queries++; throw new Error("must not launch"); },
  });
  const results = await controller.execute([
    { preset: "design-review", request: "review something" },
    { preset: "design-review", request: "Ticket: missing.md\nRelations:\nnone" },
    { preset: "design-review", request: "Ticket: missing.md\nRelations:\n" },
  ]);
  assert.deepEqual(results.map(result => result.error?.code), ["missing_context", "missing_context", "missing_context"]);
  assert.equal(queries, 0);
  await controller.shutdown();
});

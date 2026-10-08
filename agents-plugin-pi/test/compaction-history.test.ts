import { test, after } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createAgentSession, DefaultResourceLoader, ModelRuntime, SettingsManager, SessionManager, type ExtensionAPI, type ExtensionContext, type EntryRenderer } from "@earendil-works/pi-coding-agent";
import { COMPACTION_HISTORY_TYPE, registerCompactionHistory, selectCompactionHistory } from "../src/compaction-history.ts";
import { NO_KEPT_ENTRY_ID, GOAL_REMINDER_MARKER_PREFIX, collectDialogItems } from "../src/lead-compaction.ts";
import { buildCompactionResumeMessage } from "../src/goal-loop.ts";
import { buildPushWakeLine } from "../src/spawner.ts";
import { stripTerminalSequences, visibleWidth } from "../src/pi-tui.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";
import { createDisplaySummaryStore } from "../src/display-summary.ts";
import { registerDisplaySummarySession } from "../src/display-summary-session.ts";

const dir = mkdtempSync(join(tmpdir(), "ws-history-test-"));
after(() => rmSync(dir, { recursive: true, force: true }));
const user = (sm: SessionManager, text: string) => sm.appendMessage({ role: "user", content: text, timestamp: 1 });
const assistant = (sm: SessionManager, content: unknown) => sm.appendMessage({ role: "assistant", content, timestamp: 2 } as never);

function fixture(sm = SessionManager.create(dir, dir)) {
  const handlers = new Map<string, (event: any, ctx: ExtensionContext) => unknown>();
  let renderer: EntryRenderer<any>;
  let appends = 0;
  const pi = {
    on: (event: string, handler: any) => handlers.set(event, handler),
    registerEntryRenderer: (type: string, render: EntryRenderer<any>) => { assert.equal(type, COMPACTION_HISTORY_TYPE); renderer = render; },
    appendEntry: (type: string, data: unknown) => { appends++; sm.appendCustomEntry(type, data); },
    sendMessage: () => assert.fail("history must not send a model message"),
    sendUserMessage: () => assert.fail("history must not start a turn"),
  } as unknown as ExtensionAPI;
  registerCompactionHistory(pi);
  const ctx = { sessionManager: sm } as unknown as ExtensionContext;
  const success = (id: string) => handlers.get("session_compact")!({ compactionEntry: sm.getEntry(id), reason: "manual", fromExtension: true }, ctx);
  const compact = (summary = "summary only") => {
    const id = sm.appendCompaction(summary, NO_KEPT_ENTRY_ID, 99, { kind: "ws-pi-lead-compaction" }, true);
    success(id);
    return id;
  };
  const blocks = () => sm.getBranch().filter((entry) => entry.type === "custom" && entry.customType === COMPACTION_HISTORY_TYPE) as any[];
  return { sm, handlers, ctx, success, compact, blocks, appends: () => appends, renderer: () => renderer! };
}

test("latest twenty combined messages, in chronological branch order, keep complete text bodies", () => {
  const sm = SessionManager.inMemory();
  for (let i = 0; i < 25; i++) {
    if (i % 2 === 0) user(sm, ` user ${i} \n`);
    else assistant(sm, [
      { type: "text", text: `assistant ${i} first` },
      { type: "thinking", thinking: "THINKING" },
      { type: "toolCall", id: `call-${i}`, name: "read", arguments: { path: "SECRET" } },
      { type: "text", text: `assistant ${i} last` },
    ]);
  }
  const result = selectCompactionHistory(sm.getBranch());
  assert.equal(result.length, 20, "messages, not exchanges or text blocks");
  assert.deepEqual(result.map((m) => m.entryId), sm.getBranch().slice(-20).map((e) => e.id));
  assert.equal(result[0]!.text, "assistant 5 first\nassistant 5 last");
  assert.equal(result.at(-1)!.text, " user 24 \n");
  assert.doesNotMatch(JSON.stringify(result), /THINKING|SECRET/);
});

test("fewer than twenty, empty and excluded message/entry types", () => {
  const sm = SessionManager.inMemory();
  assert.deepEqual(selectCompactionHistory(sm.getBranch()), []);
  user(sm, "  ");
  user(sm, buildPushWakeLine(2));
  user(sm, `Goal yet running <!-- ${GOAL_REMINDER_MARKER_PREFIX}1 -->`);
  user(sm, "Goal armed: injected goal");
  user(sm, buildCompactionResumeMessage("key"));
  // 261007: the same injections as sent today, under the adapter label.
  user(sm, `[system message from ws-pi-plugin]\n${buildPushWakeLine(2)}`);
  user(sm, `[system message from ws-pi-plugin]\nGoal yet running <!-- ${GOAL_REMINDER_MARKER_PREFIX}2 -->`);
  user(sm, "[system message from ws-pi-plugin]\nGoal armed: labeled goal");
  user(sm, `[system message from ws-pi-plugin]\n${buildCompactionResumeMessage("key")}`);
  assistant(sm, [{ type: "thinking", thinking: "think only" }, { type: "toolCall", id: "call", name: "ws-compact", arguments: { current_work: "PROSE" } }]);
  sm.appendMessage({ role: "toolResult", toolCallId: "call", toolName: "ws-compact", content: [{ type: "text", text: "TOOL RESULT" }], isError: false, timestamp: 3 });
  sm.appendCustomMessageEntry("control", "CUSTOM MESSAGE", true);
  sm.appendCustomEntry(COMPACTION_HISTORY_TYPE, { messages: [{ text: "OLD SNAPSHOT" }] });
  sm.appendCompaction("OLD SUMMARY", NO_KEPT_ENTRY_ID, 1);
  sm.branchWithSummary(sm.getLeafId(), "BRANCH SUMMARY", sm.getLeafId());
  user(sm, "original user text");
  assistant(sm, [{ type: "text", text: "a".repeat(6000) }]);
  sm.appendMessage({ role: "user", content: [{ type: "image", data: "IMAGE", mimeType: "image/png" }, { type: "text", text: "image caption" }], timestamp: 4 });
  sm.appendMessage({ role: "user", content: [{ type: "image", data: "IMAGE ONLY", mimeType: "image/png" }], timestamp: 4 });
  assert.deepEqual(selectCompactionHistory(sm.getBranch()).map((m) => m.text), ["original user text", "a".repeat(6000), "image caption"]);
});

test("success persists a plain custom entry, retains raw records and reloads with zero extra context", async () => {
  const f = fixture();
  await f.handlers.get("session_start")!({}, f.ctx);
  const ids = [user(f.sm, "before user"), assistant(f.sm, [{ type: "text", text: "before assistant" }])];
  const id = f.compact();
  assert.equal(f.appends(), 1);
  assert.equal(f.blocks()[0].data.compactionId, id);
  assert.deepEqual(f.blocks()[0].data.messages.map((m: any) => m.entryId), ids);
  assert.deepEqual(f.sm.buildContextEntries().map((e) => e.type), ["compaction", "custom"]);
  assert.deepEqual(f.sm.buildSessionContext().messages.map((m) => m.role), ["compactionSummary"]);
  assert.deepEqual(f.sm.buildSessionContext().messages.map((m: any) => m.summary), ["summary only"]);
  const file = f.sm.getSessionFile()!;
  const raw = readFileSync(file, "utf8");
  assert.ok(ids.every((id) => raw.includes(id)), "original records survive");
  const reloaded = SessionManager.open(file, dir);
  const reloadFixture = fixture(reloaded);
  reloadFixture.success(id);
  assert.equal(reloadFixture.appends(), 0, "duplicate success after reload cannot duplicate the block");
  assert.deepEqual(reloaded.buildSessionContext(), f.sm.buildSessionContext());
  assert.deepEqual(reloadFixture.blocks(), f.blocks());
});

test("history append and disk reload remain raw and make no display-summary requests", async () => {
  const themeModule = await import(new URL("./modes/interactive/theme/theme.js", import.meta.resolve("@earendil-works/pi-coding-agent")).href);
  themeModule.initTheme("dark");
  let sm = SessionManager.create(dir, dir);
  user(sm, "original human body");
  assistant(sm, [{ type: "text", text: "original assistant body" }]);
  let requests = 0;
  const setup = () => {
    const handlers = new Map<string, Array<(event: any, ctx: any) => unknown>>();
    const store = createDisplaySummaryStore();
    let renderer: EntryRenderer<any>;
    const pi = {
      on: (name: string, handler: any) => handlers.set(name, [...(handlers.get(name) ?? []), handler]),
      getActiveTools: () => [],
      appendEntry: (type: string, data: unknown) => sm.appendCustomEntry(type, data),
      registerEntryRenderer: (_type: string, render: EntryRenderer<any>) => { renderer = render; },
    };
    const ctx = { mode: "tui", cwd: dir, sessionManager: sm, modelRegistry: { find: () => ({ provider: "offline", id: "mini", contextWindow: 100_000 }) } };
    const session = registerDisplaySummarySession(pi as never, {
      store, readConfig: async () => ({ model: "offline/mini" }), env: {},
      registerBuiltinWrappers() {}, registerMessageRenderers() {},
      createCompletion: () => async () => { requests++; throw new Error("history must not call the provider"); },
    });
    registerCompactionHistory(pi as never);
    const emit = async (name: string, event: unknown = {}) => {
      for (const handler of handlers.get(name) ?? []) await handler(event, ctx);
    };
    const verify = async () => {
      const entry = sm.buildContextEntries().find((entry) => entry.type === "custom" && entry.customType === COMPACTION_HISTORY_TYPE)!;
      assert.ok(entry, "history block remains in the transcript");
      store.set(entry.id, { toolIntention: "CACHED HISTORY", toolResult: "CACHED RESULT" });
      for (const expanded of [false, true]) {
        const component = renderer!(entry as never, { expanded }, themeModule.theme)!;
        const rendered = component.render(80).map(stripTerminalSequences).join("\n");
        assert.match(rendered, /Previous conversation · display-only/);
        assert.match(rendered, /original human body/);
        assert.match(rendered, /original assistant body/);
        assert.match(rendered, /End previous conversation/);
        assert.doesNotMatch(rendered, /CACHED HISTORY|CACHED RESULT/);
      }
      await emit("turn_end");
      await emit("agent_end");
      await session.current()!.flush();
      assert.equal(requests, 0);
      assert.equal(session.current()!.log.length, 0);
      assert.deepEqual(sm.buildSessionContext().messages.map((message) => message.role), ["compactionSummary"]);
    };
    return { emit, verify };
  };
  let h = setup();
  await h.emit("session_start");
  for (const summary of ["first native summary", "second native summary"]) {
    const id = sm.appendCompaction(summary, NO_KEPT_ENTRY_ID, 99);
    await h.emit("session_compact", { compactionEntry: sm.getEntry(id) });
    await h.verify();
    assert.equal((sm.buildSessionContext().messages[0] as any).summary, summary, "separate native compaction summary stays unchanged");
  }
  await h.emit("session_shutdown");
  sm = SessionManager.open(sm.getSessionFile()!, dir);
  h = setup();
  await h.emit("session_start");
  await h.verify();
  await h.emit("session_shutdown");
});

test("repeat compaction refreshes from original branch messages, never snapshots or summary prose", () => {
  const f = fixture();
  for (let i = 0; i < 23; i++) user(f.sm, `original ${i}`);
  const first = f.compact("UNRELATED SUMMARY PROSE");
  f.success(first);
  assert.equal(f.appends(), 1);
  user(f.sm, "latest human");
  assistant(f.sm, [{ type: "text", text: "latest reply" }]);
  const second = f.compact("NEXT SUMMARY");
  f.success(first);
  f.success(second);
  assert.equal(f.appends(), 2);
  const messages = f.blocks().at(-1).data.messages;
  assert.equal(messages.length, 20);
  assert.equal(messages[0].text, "original 5");
  assert.deepEqual(messages.slice(-2).map((m: any) => m.text), ["latest human", "latest reply"]);
  assert.doesNotMatch(JSON.stringify(messages), /SUMMARY/);
  assert.equal(f.sm.buildContextEntries().filter((e) => e.type === "custom").length, 1, "only the refreshed block renders after retain-none compaction");
  assert.deepEqual(f.sm.buildSessionContext().messages.map((m) => m.role), ["compactionSummary"]);
  const dialog = collectDialogItems(f.sm.getBranch());
  assert.equal(dialog.length, 25, "snapshots add no recursive dialog content");
});

test("active branch isolation and delayed success for an abandoned branch", () => {
  const f = fixture();
  const common = user(f.sm, "common");
  user(f.sm, "abandoned");
  const abandonedCompaction = f.compact();
  f.sm.branch(common);
  user(f.sm, "active");
  f.success(abandonedCompaction);
  assert.equal(f.appends(), 1, "no append for an event outside the active branch");
  f.compact();
  assert.deepEqual(f.blocks()[0].data.messages.map((m: any) => m.text), ["common", "active"]);
});

test("failure/cancellation and empty history append nothing; child native compaction stays unchanged", () => {
  const f = fixture();
  assert.equal(f.handlers.has("session_before_compact"), false, "no pre-success history creation");
  assert.equal(f.handlers.has("session_compact_failed"), false, "failure cannot create a block");
  f.compact();
  assert.equal(f.appends(), 0, "empty history has no block");
  user(f.sm, "human");
  const oldRole = process.env[WS_PI_SPAWN_ROLE_ENV];
  try {
    for (const role of ["worker", "explore", "fork"]) {
      process.env[WS_PI_SPAWN_ROLE_ENV] = role;
      f.compact();
      assert.equal(f.appends(), 0, role);
    }
  } finally {
    if (oldRole === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV]; else process.env[WS_PI_SPAWN_ROLE_ENV] = oldRole;
  }
});

test("installed manual compaction with identical summaries reports an old id but refreshes latest history", async () => {
  const root = mkdtempSync(join(dir, "sdk-"));
  const settingsManager = SettingsManager.inMemory({ compaction: { enabled: false, keepRecentTokens: 1 }, retry: { enabled: false } });
  const modelRuntime = await ModelRuntime.create({ authPath: join(root, "auth.json"), modelsPath: join(root, "models.json"), modelsStorePath: join(root, "store.json"), allowModelNetwork: false });
  await modelRuntime.setRuntimeApiKey("anthropic", "offline-test-key");
  const sm = SessionManager.create(root, root);
  user(sm, "first human");
  assistant(sm, [{ type: "text", text: "first reply" }]);
  const reported: string[] = [];
  const loader = new DefaultResourceLoader({
    cwd: root, agentDir: root, settingsManager,
    noExtensions: true, noSkills: true, noThemes: true, noPromptTemplates: true, noContextFiles: true,
    extensionFactories: [(pi) => {
      registerCompactionHistory(pi);
      pi.on("session_before_compact", (event) => ({ compaction: { summary: "IDENTICAL SUMMARY", firstKeptEntryId: NO_KEPT_ENTRY_ID, tokensBefore: event.preparation.tokensBefore } }));
      pi.on("session_compact", (event) => { reported.push(event.compactionEntry.id); });
    }],
  });
  await loader.reload();
  assert.deepEqual(loader.getExtensions().errors, []);
  const model = { provider: "anthropic", api: "anthropic-messages", id: "offline-model", name: "offline", reasoning: false, input: ["text"], contextWindow: 128000, maxTokens: 8192, baseUrl: "https://offline.invalid/v1", cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } };
  const { session } = await createAgentSession({ cwd: root, agentDir: root, sessionManager: sm, resourceLoader: loader, modelRuntime, model: model as never, thinkingLevel: "off", settingsManager });
  const errors: unknown[] = [];
  const agentStarts: unknown[] = [];
  session.subscribe((event) => { if (event.type === "agent_start") agentStarts.push(event); });
  try {
    await session.bindExtensions({ mode: "rpc", onError: (error) => errors.push(error) });
    await session.compact();
    user(sm, "new human");
    assistant(sm, [{ type: "text", text: "new reply" }]);
    await session.compact();
    const compactions = sm.getBranch().filter((entry) => entry.type === "compaction");
    assert.equal(compactions.length, 2);
    assert.deepEqual(reported, [compactions[0]!.id, compactions[0]!.id], "regression reproduces installed host's first-summary match");
    const block = sm.buildContextEntries().find((entry) => entry.type === "custom" && entry.customType === COMPACTION_HISTORY_TYPE) as any;
    assert.ok(block, "refreshed block survives latest retain-none boundary");
    assert.equal(block.data.compactionId, compactions[1]!.id);
    assert.deepEqual(block.data.messages.map((m: any) => m.text), ["first human", "first reply", "new human", "new reply"]);
    assert.deepEqual(sm.buildSessionContext().messages.map((message) => message.role), ["compactionSummary"]);
    const reloaded = SessionManager.open(sm.getSessionFile()!, root);
    assert.deepEqual(reloaded.buildContextEntries(), JSON.parse(JSON.stringify(sm.buildContextEntries())), "reload preserves serialized entry data (undefined optional fields are not JSON)");
    assert.deepEqual(agentStarts, [], "neither history nor manual compaction starts inference");
    assert.deepEqual(errors, []);
  } finally {
    session.dispose();
  }
});

test("renderer gives user blocks exactly one colored inner and plain outer row at every boundary", async () => {
  const themeModule = await import(new URL("./modes/interactive/theme/theme.js", import.meta.resolve("@earendil-works/pi-coding-agent")).href);
  themeModule.initTheme();
  const bg = "\u001b[48;5;123m";
  const theme = {
    fg: (_color: string, text: string) => text,
    bg: (color: string, text: string) => { assert.equal(color, "userMessageBg"); return `${bg}${text}\u001b[49m`; },
  } as never;
  for (const roles of [["user"], ["user", "user"], ["user", "assistant"], ["assistant", "user"], ["assistant", "user", "assistant"], ["user", "assistant", "user"]]) {
    const f = fixture();
    await f.handlers.get("session_start")!({}, f.ctx);
    for (const role of roles) {
      if (role === "user") user(f.sm, "first user line wraps across narrow columns\n\nlast user line also wraps");
      else assistant(f.sm, [{ type: "text", text: "assistant body" }]);
    }
    f.compact();
    for (const width of [12, 80]) {
      const component = f.renderer()(f.blocks()[0], { expanded: false }, theme)!;
      const lines = component.render(width);
      const plain = lines.map((line) => stripTerminalSequences(line).trim());
      const colored = lines.map((line) => line.includes(bg));
      assert.ok(!plain.includes("User"));
      assert.equal(plain.filter((line) => line === "Assistant").length, roles.filter((role) => role === "assistant").length);
      const starts = colored.flatMap((color, i) => color && !colored[i - 1] ? [i] : []);
      assert.equal(starts.length, roles.filter((role) => role === "user").length);
      for (const start of starts) {
        let end = start;
        while (colored[end + 1]) end++;
        const block = lines.slice(start, end + 1);
        assert.ok(block.every((line) => visibleWidth(line) === width), "full-width background on wrapped/multiline bodies and padding");
        assert.equal(plain[start], "", "colored top row");
        assert.notEqual(plain[start + 1], "", "exactly one colored top row");
        assert.equal(plain[end], "", "colored bottom row");
        assert.notEqual(plain[end - 1], "", "exactly one colored bottom row");
        for (const outside of [start - 1, end + 1]) {
          assert.equal(plain[outside], "", "plain outside row");
          assert.equal(colored[outside], false, "outside row is not user background");
        }
        assert.ok(colored[start - 2] || plain[start - 2] !== "", "no duplicated outside top row");
        assert.ok(colored[end + 2] || plain[end + 2] !== "", "no duplicated outside bottom row");
      }
      component.invalidate();
      assert.deepEqual(component.render(width), lines);
    }
  }
});

test("renderer uses native Markdown, user styling and preservation without navigation controls", async () => {
  const themeModule = await import(new URL("./modes/interactive/theme/theme.js", import.meta.resolve("@earendil-works/pi-coding-agent")).href);
  themeModule.initTheme();
  const f = fixture();
  await f.handlers.get("session_start")!({}, f.ctx);
  const userBody = "# 한글 🦦 heading\n\n**strong** and *emphasis*\n\n7. preserved\n9. numbering\n\n\\*escaped\\*\n\n```text\nuser code\n```\n\u001b]133;A\u0007safe";
  user(f.sm, userBody);
  assistant(f.sm, [{ type: "text", text: "# Reply\n\n**bold reply**\n\n```text\nassistant code\n```\nreply\u001b[31m colored\u001b[0m\u001b]133;B\u0007" }]);
  f.compact();
  const fgCalls: string[] = [];
  const theme = {
    fg: (color: string, text: string) => { fgCalls.push(color); return color === "userMessageText" ? `\u001b[38;5;123m${text}\u001b[39m` : text; },
    bg: (color: string, text: string) => { assert.equal(color, "userMessageBg"); return `\u001b[48;5;123m${text}\u001b[49m`; },
  } as never;
  for (const width of [12, 80]) {
    const component = f.renderer()(f.blocks()[0], { expanded: false }, theme)!;
    const lines = component.render(width);
    assert.ok(lines.every((line) => visibleWidth(line) <= width), "host wrapping respects terminal columns");
    const text = lines.map((line) => stripTerminalSequences(line).trimEnd()).join("\n");
    if (width === 80) {
      assert.match(text, /Previous conversation · display-only\n.*─/);
      assert.match(text, /한글 🦦 heading/);
      assert.match(text, /strong and emphasis/);
      assert.doesNotMatch(text, /\*\*strong\*\*|# Reply/);
      assert.match(text, /7\. preserved/);
      assert.match(text, /9\. numbering/);
      assert.ok(text.includes("\\*escaped\\*"), "native user backslash preservation");
      assert.match(text, /user code/);
      assert.match(text, /assistant code/);
      assert.match(text, /reply colored/);
      assert.match(text, /End previous conversation/);
      assert.doesNotMatch(text, /^User$/m, "user has no label");
      const userStart = lines.findIndex((line) => line.includes("\u001b[48;5;123m"));
      const assistantStart = lines.findIndex((line) => stripTerminalSequences(line).trim() === "Assistant");
      const userLines = lines.slice(userStart, assistantStart - 1);
      assert.equal(stripTerminalSequences(lines[assistantStart - 1]!).trim(), "", "outside bottom spacer");
      assert.ok(!lines[assistantStart - 1]!.includes("\u001b[48;5;123m"));
      assert.ok(userLines.every((line) => line.includes("\u001b[48;5;123m")), "user background covers body and padding");
      assert.ok(userLines.every((line) => visibleWidth(line) === width), "user block fills terminal width");
      assert.ok(lines.slice(assistantStart).every((line) => !line.includes("\u001b[48;5;123m")), "assistant has no user background");
      assert.ok(userLines.some((line) => stripTerminalSequences(line).startsWith(" 한글")), "horizontal padding");
    }
    assert.ok(lines.every((line) => !line.includes("\u001b]133;")), "no native prompt-navigation sequences");
    component.invalidate();
    assert.deepEqual(component.render(width), lines);
  }
  assert.ok(fgCalls.includes("userMessageText"));
  assert.equal(f.blocks()[0].data.messages[0].text, userBody, "storage remains original");
  assert.match(f.blocks()[0].data.messages[1].text, /\u001b\[31m/, "renderer sanitization does not alter storage");
  const transparent = { fg: (_color: string, text: string) => text, bg: (_color: string, text: string) => text } as never;
  const transparentLines = f.renderer()(f.blocks()[0], { expanded: false }, transparent)!.render(80);
  assert.ok(transparentLines.every((line) => !line.includes("\u001b[48;")), "transparent theme never gains a forced background");
  assert.match(transparentLines.map(stripTerminalSequences).join("\n"), /bold reply/);
});

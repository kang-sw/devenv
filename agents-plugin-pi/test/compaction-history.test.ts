import { test, after } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { SessionManager, type ExtensionAPI, type ExtensionContext, type EntryRenderer } from "@earendil-works/pi-coding-agent";
import { COMPACTION_HISTORY_TYPE, registerCompactionHistory, selectCompactionHistory } from "../src/compaction-history.ts";
import { NO_KEPT_ENTRY_ID, GOAL_REMINDER_MARKER_PREFIX, collectDialogItems } from "../src/lead-compaction.ts";
import { buildCompactionResumeMessage } from "../src/goal-loop.ts";
import { buildPushWakeLine } from "../src/spawner.ts";
import { stripTerminalSequences, visibleWidth } from "../src/pi-tui.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";

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

test("renderer labels historical boundary and roles, renders text literally at narrow/wide widths", async () => {
  const f = fixture();
  await f.handlers.get("session_start")!({}, f.ctx);
  user(f.sm, "한글 🦦 *literal markdown*\nsecond line");
  assistant(f.sm, [{ type: "text", text: "reply\u001b[31m colored\u001b[0m" }]);
  f.compact();
  const theme = { fg: (_color: string, text: string) => text } as never;
  for (const width of [12, 80]) {
    const component = f.renderer()(f.blocks()[0], { expanded: false }, theme)!;
    const lines = component.render(width);
    assert.ok(lines.every((line) => visibleWidth(line) <= width), "host wrapping respects terminal columns");
    const text = lines.map((line) => stripTerminalSequences(line).trimEnd()).join("\n");
    if (width === 80) {
      assert.match(text, /Previous conversation · display-only\n.*─/);
      assert.match(text, /User\n한글 🦦 \*literal markdown\*\nsecond line/);
      assert.match(text, /Assistant\nreply colored/);
      assert.match(text, /End previous conversation/);
    }
    component.invalidate();
    assert.deepEqual(component.render(width), lines);
  }
  assert.match(f.blocks()[0].data.messages[1].text, /\u001b\[31m/, "renderer sanitization does not alter storage");
});

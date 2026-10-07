/**
 * Unit tests for lead-compaction.ts (261002): the summary the lead session's
 * `session_before_compact` handler returns. The hook wiring itself is covered
 * in test/goal-loop.test.ts's "lead compaction ownership" suites.
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import type { SessionEntry } from "@earendil-works/pi-coding-agent";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  buildDialogSection,
  buildFileListSections,
  buildContextMilestoneMessage,
  buildFallbackSummaryPrompt,
  buildLeadCompactionSummary,
  buildPreparationMessage,
  collectDialogItems,
  describeFinishedChildren,
  describeInFlightChildren,
  elideMiddle,
  extractLeadProse,
  findActivePlaybook,
  foldToolRuns,
  findActiveTicket,
  humanTextOf,
  LEAD_CONTEXT_MILESTONE_CUSTOM_TYPE,
  LEAD_PROSE_SECTIONS,
  leadProseParameterSchema,
  normalizeFileList,
  readLeadCompactGuide,
  renderDialogItem,
  renderLeadProse,
  selectDialogItems,
  type DialogItem,
} from "../src/lead-compaction.ts";
import { buildPushWakeLine, type RpcAgentRegistry } from "../src/spawner.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";

let seq = 0;
const at = (minute: number): string => new Date(Date.UTC(2026, 9, 2, 10, minute)).toISOString();
const user = (minute: number, content: unknown): SessionEntry => ({ type: "message", id: `u${++seq}`, parentId: null, timestamp: at(minute), message: { role: "user", content, timestamp: 0 } } as never);
const assistantCall = (minute: number, name: string, args: Record<string, unknown>): SessionEntry => ({
  type: "message", id: `a${++seq}`, parentId: null, timestamp: at(minute),
  message: { role: "assistant", content: [{ type: "toolCall", id: `t${seq}`, name, arguments: args }] },
} as never);
const assistant = (minute: number, content: unknown[]): SessionEntry => ({ type: "message", id: `a${++seq}`, parentId: null, timestamp: at(minute), message: { role: "assistant", content } } as never);
const toolResult = (minute: number, toolCallId: string, text: string, isError = false): SessionEntry => ({
  type: "message", id: `r${++seq}`, parentId: null, timestamp: at(minute),
  message: { role: "toolResult", toolCallId, toolName: "x", content: [{ type: "text", text }], isError, timestamp: 0 },
} as never);
const custom = (minute: number, customType: string, content: string): SessionEntry => ({ type: "custom_message", id: `c${++seq}`, parentId: null, timestamp: at(minute), customType, content, display: true } as never);
const compaction = (minute: number, summary: string): SessionEntry => ({ type: "compaction", id: `k${++seq}`, parentId: null, timestamp: at(minute), summary, firstKeptEntryId: "x", tokensBefore: 1 } as never);
const branchSummary = (minute: number, summary: string): SessionEntry => ({ type: "branch_summary", id: `b${++seq}`, parentId: null, timestamp: at(minute), fromId: "x", summary } as never);
const dialog = { dialogBudgetBytes: 40960, sessionFile: "/sessions/lead.jsonl" };
const tool = (name: string, args: Record<string, unknown> = {}): DialogItem => ({ kind: "tool", name, args: JSON.stringify(args), failed: false });
const skillExpansion = (name: string, args?: string): string =>
  `<skill name="${name}" location="/skills/${name}/SKILL.md">\nReferences are relative to /skills/${name}.\n\nSKILL BODY LINE\n</skill>${args ? `\n\n${args}` : ""}`;

function registry(records: Array<Record<string, unknown>>): RpcAgentRegistry {
  return new Map(records.map((record) => [record.agentId as string, { running: false, streaming: false, reportLog: [], ...record }])) as unknown as RpcAgentRegistry;
}

describe("lead prose", () => {
  test("renders every fixed heading in order, verbatim text, (none) for blanks", () => {
    const text = renderLeadProse({ current_work: "  exact\ttext ", next_step: "" });
    const headings = [...text.matchAll(/^### (.+)$/gm)].map((match) => match[1]);
    assert.deepEqual(headings, LEAD_PROSE_SECTIONS.map((section) => section.heading));
    assert.match(text, /### Current work\n {2}exact\ttext \n/);
    assert.match(text, /### Immediate next step\n\(none\)/);
  });

  test("the lever schema requires one string field per heading", () => {
    const schema = leadProseParameterSchema();
    assert.deepEqual(schema.required, LEAD_PROSE_SECTIONS.map((section) => section.key));
    for (const key of schema.required) assert.equal(schema.properties[key]!.type, "string");
    assert.deepEqual(Object.keys(schema.properties), schema.required, "prose-only: the lever composes continue_after_compact on top");
  });

  test("the fallback summary prompt never mentions the lever's continuation flag", () => {
    assert.doesNotMatch(buildFallbackSummaryPrompt("conversation", "previous prose"), /continue_after_compact/);
  });
});

describe("human-typed user text", () => {
  test("adapter traffic is excluded and a /skill: expansion collapses to what the human typed", () => {
    assert.equal(humanTextOf(buildPushWakeLine(3)), undefined, "push wake line");
    assert.equal(humanTextOf("Goal yet running: \"x\".\n\n<!-- ws-pi-goal-reminder:1-1 -->"), undefined, "goal reminder");
    assert.equal(humanTextOf(skillExpansion("lead-run", "run 261002")), "/skill:lead-run run 261002");
    assert.equal(humanTextOf(skillExpansion("lead-discuss")), "/skill:lead-discuss");
    assert.equal(humanTextOf("Goal armed: ship"), "Goal armed: ship", "the /goal announcement carries the human's goal");
    assert.equal(humanTextOf("ordinary request"), "ordinary request");
  });

  test("261007: a labeled message is adapter traffic, except the goal announcement, which collapses to /goal <goal>", () => {
    const label = "[system message from ws-pi-plugin]";
    assert.equal(humanTextOf(`${label}\n${buildPushWakeLine(3)}`), undefined, "labeled push wake line");
    assert.equal(humanTextOf(`${label}\nGoal yet running: "x".\n\n<!-- ws-pi-goal-reminder:1-1 -->`), undefined, "labeled goal reminder");
    assert.equal(humanTextOf(`${label}\nCompaction complete. Invoke \`lead-revive\` (\`ws-skill lead-revive\`) with session key \`k\`, then continue.`), undefined, "labeled resume prompt");
    assert.equal(humanTextOf(`${label}\nany future adapter notice`), undefined, "the label alone recognizes adapter traffic, whatever follows it");
    assert.equal(humanTextOf(label), undefined, "a bare label line");
    assert.equal(humanTextOf(`${label}\nGoal armed: ship the widget`), "/goal ship the widget", "the labeled goal announcement keeps the human's goal");
    assert.equal(humanTextOf(`${label}\nGoal armed: line one\nline two`), "/goal line one\nline two");
    assert.equal(humanTextOf(`please read ${label} literally`), "please read [system message from ws-pi-plugin] literally", "the label counts only as the first line");
    assert.equal(humanTextOf(`${label} inline`), `${label} inline`, "the label is a whole first line, not a prefix");
  });

  test("261007: the dialog keeps human text, drops labeled traffic, and shows the goal as /goal", () => {
    const label = "[system message from ws-pi-plugin]";
    const items = collectDialogItems([
      user(0, "please fix the parser"),
      user(1, `${label}\n${buildPushWakeLine(1)}`),
      user(2, `${label}\nGoal armed: fix the parser`),
      user(3, `${label}\nGoal yet running: "fix the parser".\n\n<!-- ws-pi-goal-reminder:1-1 -->`),
      user(4, buildPushWakeLine(2)),
      user(5, "and the lexer too"),
    ] as never);
    assert.deepEqual(items.map((item) => (item as { text?: string }).text), ["please fix the parser", "/goal fix the parser", "and the lexer too"]);
  });
});

describe("dialog transcript (261003)", () => {
  test("user text, assistant text, and tool lines interleave chronologically", () => {
    const entries = [
      user(0, "please fix the parser"),
      assistant(1, [
        { type: "thinking", thinking: "SECRET THINKING" },
        { type: "text", text: "Looking at it." },
        { type: "toolCall", id: "call-read", name: "Read", arguments: { path: "src/a.ts" } },
        { type: "toolCall", id: "call-bash", name: "Bash", arguments: { command: "npm test" } },
      ]),
      toolResult(2, "call-read", "READ RESULT BODY"),
      toolResult(3, "call-bash", "BASH FAILURE OUTPUT", true),
      assistant(4, [{ type: "text", text: "The test fails; fixing." }, { type: "toolCall", id: "call-edit", name: "Edit", arguments: { path: "src/a.ts" } }, { type: "text", text: "Done." }]),
      user(5, "thanks"),
    ];
    const lines = selectDialogItems(foldToolRuns(collectDialogItems(entries)), 40960).lines;
    assert.deepEqual(lines, [
      `<message role="user" timestamp="${at(0)}">\nplease fix the parser\n</message>`,
      `<message role="assistant" timestamp="${at(1)}">\nLooking at it.\n</message>`,
      '\u2192 Read {"path":"src/a.ts"}',
      '\u2192 Bash {"command":"npm test"} \u2717failed',
      `<message role="assistant" timestamp="${at(4)}">\nThe test fails; fixing.\n</message>`,
      '\u2192 Edit {"path":"src/a.ts"}',
      `<message role="assistant" timestamp="${at(4)}">\nDone.\n</message>`,
      `<message role="user" timestamp="${at(5)}">\nthanks\n</message>`,
    ]);
  });

  test("XML-style boundaries escape attributes but preserve literal markup payloads verbatim", () => {
    const payload = '  <dialog>& "raw" </message>\n</tool-call></branch-summary></dialog>  ';
    const timestamp = 'T<&">';
    const escaped = "T&lt;&amp;&quot;&gt;";
    for (const role of ["user", "assistant"] as const) {
      assert.equal(renderDialogItem({ kind: role, timestamp, text: payload }), `<message role="${role}" timestamp="${escaped}">\n${payload}\n</message>`);
    }
    assert.equal(renderDialogItem({ kind: "branch_summary", timestamp, text: payload }), `<branch-summary timestamp="${escaped}">\n${payload}\n</branch-summary>`);
    assert.equal(renderDialogItem({ kind: "tool", name: timestamp, args: payload, failed: false }), `→ ${timestamp} ${payload}`);
    const section = buildDialogSection([user(0, payload)], 40960, undefined);
    assert.ok(section.endsWith(`<dialog>\n<message role="user" timestamp="${at(0)}">\n${payload}\n</message>\n</dialog>`));
  });

  test("an empty or fully omitted dialog still has its outer boundaries", () => {
    const empty = buildDialogSection([], 40960, undefined);
    assert.match(empty, /All 0 dialog items of this session\./);
    assert.ok(empty.endsWith("\n<dialog>\n</dialog>"));
    const omitted = buildDialogSection([user(0, "not retained")], 0, undefined);
    assert.match(omitted, /The newest 0 of 1 dialog items/);
    assert.ok(omitted.endsWith("\n<dialog>\n</dialog>"));
    assert.ok(!omitted.includes("not retained"));
  });

  test("only adjacent tool calls and folds share a group, split by messages and branch summaries", () => {
    const entries = [
      ...Array.from({ length: 10 }, (_, i) => assistantCall(i, "Read", { i })),
      assistant(10, [{ type: "text", text: "pause" }]),
      assistantCall(11, "Bash", { command: "ls" }),
      branchSummary(12, "branch"),
      assistantCall(13, "Read", { path: '</tool-calls><dialog>&"raw"' }),
      user(14, "stop"),
      assistantCall(15, "Edit", {}),
    ];
    const items = foldToolRuns(collectDialogItems(entries));
    const section = buildDialogSection(entries, 40960, undefined);
    const body = section.slice(section.indexOf("\n<dialog>\n") + 1);
    assert.equal(body, [
      "<dialog>", "<tool-calls>", ...items.slice(0, 9).map(renderDialogItem), "</tool-calls>",
      renderDialogItem(items[9]!), "<tool-calls>", renderDialogItem(items[10]!), "</tool-calls>",
      renderDialogItem(items[11]!), "<tool-calls>", renderDialogItem(items[12]!), "</tool-calls>",
      renderDialogItem(items[13]!), "<tool-calls>", renderDialogItem(items[14]!), "</tool-calls>", "</dialog>",
    ].join("\n"));
    assert.ok(body.includes('→ Read {"path":"</tool-calls><dialog>&\\"raw\\""}'), "literal argument markup stays verbatim");
    assert.ok(!body.includes("<tool-call name="));
    assert.ok(!body.includes("<tool-fold>"));
  });

  test("budget edges charge one wrapper for the selected tool run, including a singleton suffix", () => {
    const entries = [assistantCall(0, "Read", { n: 0 }), assistantCall(1, "Read", { n: 1 })];
    const items = collectDialogItems(entries);
    const wrapperBytes = Buffer.byteLength("<tool-calls>\n</tool-calls>\n");
    const cost = (item: DialogItem): number => Buffer.byteLength(renderDialogItem(item), "utf8") + 1;
    const singletonBudget = wrapperBytes + cost(items[1]!);
    assert.equal(selectDialogItems(items, singletonBudget - 1).lines.length, 0);
    assert.deepEqual(selectDialogItems(items, singletonBudget).lines, [renderDialogItem(items[1]!)]);
    assert.ok(buildDialogSection(entries, singletonBudget, undefined).endsWith(`<dialog>\n<tool-calls>\n${renderDialogItem(items[1]!)}\n</tool-calls>\n</dialog>`));
    const bothBudget = singletonBudget + cost(items[0]!);
    assert.equal(selectDialogItems(items, bothBudget - 1).lines.length, 1);
    assert.equal(selectDialogItems(items, bothBudget).lines.length, 2);
    assert.ok(buildDialogSection(entries, bothBudget, undefined).endsWith(`<dialog>\n<tool-calls>\n${items.map(renderDialogItem).join("\n")}\n</tool-calls>\n</dialog>`));
    const separated = [entries[0]!, branchSummary(2, "split"), entries[1]!];
    const separatedItems = collectDialogItems(separated);
    const separateBudget = separatedItems.reduce((sum, item) => sum + cost(item), 0) + wrapperBytes * 2;
    assert.equal(selectDialogItems(separatedItems, separateBudget - 1).lines.length, 2);
    assert.equal(selectDialogItems(separatedItems, separateBudget).lines.length, 3);
  });

  test("thinking, tool results, adapter traffic, bash executions, and the ws-compact call never cross", () => {
    const entries = [
      user(0, "first human message"),
      custom(1, PUSH_BATCH_CUSTOM_TYPE, "<ws-push-batch>WORKER REPORT</ws-push-batch>"),
      custom(2, "ws-lead-compact", "PREPARATION GUIDE"),
      user(3, buildPushWakeLine(1)),
      user(4, [{ type: "text", text: "with" }, { type: "image", data: "", mimeType: "image/png" }]),
      { type: "message", id: "b", parentId: null, timestamp: at(5), message: { role: "bashExecution", command: "ls", output: "BASH OUTPUT" } } as never,
      assistant(6, [{ type: "thinking", thinking: "SECRET THINKING" }]),
      assistantCall(7, "ws-compact", { current_work: "LEVER PROSE" }),
      toolResult(8, "t1", "TOOL RESULT BODY"),
      user(9, "Goal yet running.\n\n<!-- ws-pi-goal-reminder:1-1 -->"),
    ];
    const items = collectDialogItems(entries);
    assert.deepEqual(items, [
      { kind: "user", timestamp: at(0), text: "first human message" },
      { kind: "user", timestamp: at(4), text: "with\n[image]" },
    ]);
    const section = buildDialogSection(entries, 40960, undefined);
    for (const absent of ["WORKER REPORT", "PREPARATION GUIDE", "BASH OUTPUT", "SECRET THINKING", "LEVER PROSE", "ws-compact", "TOOL RESULT BODY", "ws-pi-goal-reminder"]) {
      assert.ok(!section.includes(absent), absent);
    }
  });

  test("a run of more than 8 tool lines keeps its last 8 and folds the rest, counted highest first", () => {
    const run = [tool("Read"), tool("Bash"), tool("Bash"), tool("Grep"), tool("Read"), tool("Bash"), ...Array.from({ length: 8 }, (_, i) => tool("Edit", { n: i }))];
    const items: DialogItem[] = [{ kind: "user", timestamp: at(0), text: "go" }, ...run, { kind: "assistant", timestamp: at(1), text: "done" }, tool("Read"), tool("Read")];
    const folded = foldToolRuns(items);
    assert.equal(folded.length, 1 + 1 + 8 + 1 + 2);
    assert.deepEqual(folded[1], { kind: "fold", total: 6, counts: [{ name: "Bash", count: 3 }, { name: "Read", count: 2 }, { name: "Grep", count: 1 }] });
    assert.equal(renderDialogItem(folded[1]!), "\u2192 (+6 more: Bash\u00d73, Read\u00d72, Grep\u00d71)");
    assert.deepEqual(folded.slice(2, 10), run.slice(6), "the newest 8 calls stay as lines");
    assert.deepEqual(folded.slice(11), [tool("Read"), tool("Read")], "a short run is untouched");
    assert.deepEqual(foldToolRuns(run.slice(6)), run.slice(6), "exactly 8 lines need no fold");
  });

  test("a long message keeps its first and last 1024 bytes around the skipped-byte marker", () => {
    const exact = "e".repeat(2560);
    assert.equal(elideMiddle(exact, 2560, 1024, "\n"), exact, "2560 bytes is not over the threshold");
    const long = `${"h".repeat(1024)}${"m".repeat(952)}${"t".repeat(1024)}`;
    assert.equal(renderDialogItem({ kind: "user", timestamp: "T", text: long }), `<message role="user" timestamp="T">\n${"h".repeat(1024)}\n[... 952 bytes skipped ...]\n${"t".repeat(1024)}\n</message>`);
  });

  test("elision never splits a multi-byte character", () => {
    // 1000 Hangul syllables = 3000 bytes; 1024 is not a multiple of 3, so each end keeps 341 characters (1023 bytes).
    const korean = "\ud55c".repeat(1000);
    const elided = renderDialogItem({ kind: "assistant", timestamp: "T", text: korean });
    assert.equal(elided, `<message role="assistant" timestamp="T">\n${"\ud55c".repeat(341)}\n[... 954 bytes skipped ...]\n${"\ud55c".repeat(341)}\n</message>`);
    assert.ok(!elided.includes("\ufffd"), "no replacement character from a split code point");
    // 4-byte emoji straddle both 150-byte cut points, so each end stops at its 148 ASCII bytes and every emoji is skipped.
    const args = `${"a".repeat(148)}${"\u{1f9a6}".repeat(40)}${"z".repeat(148)}`;
    const line = renderDialogItem({ kind: "tool", name: "Write", args, failed: false });
    assert.equal(line, `\u2192 Write ${"a".repeat(148)} [... 160 bytes skipped ...] ${"z".repeat(148)}`);
  });

  test("long tool arguments keep their first and last 150 bytes", () => {
    const call = { content: "x".repeat(400) };
    const json = JSON.stringify(call);
    const line = renderDialogItem({ kind: "tool", name: "Write", args: json, failed: true });
    assert.equal(line, `\u2192 Write ${json.slice(0, 150)} [... ${json.length - 300} bytes skipped ...] ${json.slice(-150)} \u2717failed`);
    assert.equal(renderDialogItem(tool("Read", { path: "short" })), '\u2192 Read {"path":"short"}', "short arguments are untouched");
  });

  test("selection is newest-first within the byte budget, drops a too-large item whole, and stops there", () => {
    const items: DialogItem[] = [
      { kind: "user", timestamp: "T", text: "old" },
      { kind: "assistant", timestamp: "T", text: "B".repeat(200) },
      tool("Read", { p: 1 }),
      { kind: "user", timestamp: "T", text: "newest" },
    ];
    const cost = (item: DialogItem): number => Buffer.byteLength(renderDialogItem(item), "utf8") + 1;
    const newestTwo = cost(items[2]!) + cost(items[3]!) + Buffer.byteLength("<tool-calls>\n</tool-calls>\n");
    const tight = selectDialogItems(items, newestTwo + cost(items[0]!));
    assert.deepEqual(tight.lines, [renderDialogItem(items[2]!), renderDialogItem(items[3]!)], "the old item fits the remainder but is not taken past the dropped one");
    assert.equal(tight.omitted, 2);
    assert.equal(tight.total, 4);
    assert.equal(selectDialogItems(items, newestTwo - 1).lines.length, 1, "one byte short drops the second-newest whole");
    const all = selectDialogItems(items, 40960);
    assert.equal(all.omitted, 0);
    assert.deepEqual(all.lines, items.map(renderDialogItem), "kept oldest-first for reading");

    const korean: DialogItem = { kind: "user", timestamp: "T", text: "\ud55c".repeat(10) };
    assert.equal(selectDialogItems([korean], cost(korean)).lines.length, 1);
    assert.equal(selectDialogItems([korean], cost(korean) - 1).lines.length, 0, "the budget counts UTF-8 bytes, not characters");
  });

  test("a branch summary is a labeled, unelided item in chronological order that ends a tool run", () => {
    const long = "S".repeat(5000);
    const entries = [user(0, "before"), branchSummary(1, long), user(2, "after")];
    const items = collectDialogItems(entries);
    assert.deepEqual(items.map((item) => item.kind), ["user", "branch_summary", "user"]);
    assert.equal(renderDialogItem(items[1]!), `<branch-summary timestamp="${at(1)}">\n${long}\n</branch-summary>`, "carried whole, over the 2560-byte elision threshold");
    const calls = Array.from({ length: 6 }, (_, i) => tool("Read", { i }));
    const folded = foldToolRuns([...calls, items[1]!, ...calls]);
    assert.deepEqual(folded, [...calls, items[1]!, ...calls], "6 + 6 calls need no fold: the summary splits the run");
    assert.equal(foldToolRuns([...calls, ...calls]).some((item) => item.kind === "fold"), true);
  });

  test("a whitespace-only or non-string branch summary yields no item or block", () => {
    const entries = [
      user(0, "before"),
      branchSummary(1, " \n\t "),
      branchSummary(2, 42 as never),
      branchSummary(3, undefined as never),
      user(4, "after"),
    ];
    assert.deepEqual(collectDialogItems(entries).map((item) => item.kind), ["user", "user"]);
    assert.doesNotMatch(buildDialogSection(entries, dialog.dialogBudgetBytes, dialog.sessionFile), /<branch-summary/);
  });

  test("a branch summary counts against the budget and is dropped whole, ending selection", () => {
    const items: DialogItem[] = [
      { kind: "user", timestamp: "T", text: "old" },
      { kind: "branch_summary", timestamp: "T", text: "S".repeat(3000) },
      { kind: "user", timestamp: "T", text: "newest" },
    ];
    const cost = (item: DialogItem): number => Buffer.byteLength(renderDialogItem(item), "utf8") + 1;
    const fits = selectDialogItems(items, cost(items[0]!) + cost(items[1]!) + cost(items[2]!));
    assert.equal(fits.lines.length, 3);
    const tight = selectDialogItems(items, cost(items[1]!) - 1 + cost(items[2]!));
    assert.deepEqual(tight.lines, [renderDialogItem(items[2]!)], "the summary does not fit, so the older item is not reached");
  });

  test("the section header states the omitted count and the session file is named for search", () => {
    const entries = [user(0, "a".repeat(100)), user(1, "b"), user(2, "c")];
    const roomy = buildDialogSection(entries, 40960, "/sessions/lead.jsonl");
    assert.equal(roomy.split("\n").slice(0, 3).join("\n"), [
      "## Dialog",
      "All 3 dialog items of this session.",
      "Full tool output and every earlier message remain in the session file `/sessions/lead.jsonl`; search it (for example with grep) when you need them.",
    ].join("\n"));
    const newestTwoBytes = collectDialogItems(entries).slice(1).reduce((sum, item) => sum + Buffer.byteLength(renderDialogItem(item), "utf8") + 1, 0);
    const tight = buildDialogSection(entries, newestTwoBytes, undefined);
    assert.match(tight, /^## Dialog\nThe newest 2 of 3 dialog items \(older ones are carried by the prose below\)\.\nThis session has no session file/);
    assert.ok(!tight.includes("a".repeat(100)));
  });
});

describe("child agents", () => {
  const report = "Done.\nstatus: [ok]\nstop: none\nticket: ai-docs/tickets/ready/261002-feat-pi-lead-ws-owned-compaction.md\ncommits: abc1234..def5678";

  test("in-flight children list role, ticket, and state; owner-thread agents are left out", () => {
    const lines = describeInFlightChildren(registry([
      { agentId: "id-1", alias: "w1", spawnRole: "worker", running: true, prompt: "Ticket: ai-docs/tickets/ready/261001-bug-thing.md" },
      { agentId: "id-2", spawnRole: "explore", streaming: true, waitingOnChildren: true },
      { agentId: "id-3", running: true, threadBound: true },
      { agentId: "id-4", running: false, settledAt: 1 },
    ]));
    assert.deepEqual(lines, [
      "- w1 (id-1) [worker]: ticket ai-docs/tickets/ready/261001-bug-thing.md; running",
      "- id-2 [explore]: ticket none named; waiting on its own children",
    ]);
  });

  test("finished children are listed once: only those settled after the previous compaction, with report fields", () => {
    const reg = registry([
      { agentId: "early", alias: "e", settledAt: Date.parse(at(5)), lastText: "status: [escalate-to-lead]\nstop: c" },
      { agentId: "late", alias: "l", settledAt: Date.parse(at(20)), lastText: report },
      { agentId: "plain", settledAt: Date.parse(at(25)), prompt: "work on 260999-chore-cleanup please" },
    ]);
    assert.deepEqual(describeFinishedChildren(reg, undefined).map((line) => line.split(":")[0]), ["- e (early)", "- l (late)", "- plain"]);
    const afterFirst = describeFinishedChildren(reg, Date.parse(at(10)));
    assert.deepEqual(afterFirst, [
      "- l (late): ticket ai-docs/tickets/ready/261002-feat-pi-lead-ws-owned-compaction.md; status [ok]; commits abc1234..def5678",
      "- plain: ticket 260999-chore-cleanup; status settled (no report block); commits none reported",
    ]);
    assert.match(describeFinishedChildren(reg, undefined)[0]!, /status \[escalate-to-lead\]; stop c;/);
  });
});

describe("active ticket and playbook", () => {
  test("the newest ticket-naming tool call wins, with its phase when given", () => {
    const entries = [
      assistantCall(0, "ws__tickets_acquire", { ticket_stem: "260001-feat-old" }),
      assistantCall(1, "ws__route_resolve_implement", { params: {}, target: { ticket_path: "ai-docs/tickets/ready/261002-feat-new.md" }, phase: 2 }),
      assistantCall(2, "ws__agenda_list", {}),
    ];
    assert.deepEqual(findActiveTicket(entries), { ref: "ai-docs/tickets/ready/261002-feat-new.md", phase: "2", tool: "ws__route_resolve_implement" });
    assert.equal(findActiveTicket([assistantCall(0, "ws__agenda_list", {})]), undefined);
  });

  test("the newest loaded playbook wins; lead-revive is never the active playbook", () => {
    assert.deepEqual(findActivePlaybook([user(0, skillExpansion("lead-discuss")), assistantCall(1, "ws-skill", { name: "lead-run" })]), { name: "lead-run", via: "ws-skill" });
    assert.deepEqual(findActivePlaybook([assistantCall(0, "ws__playbook_read", { name: "ticket-worker" })]), { name: "ticket-worker", via: "ws__playbook_read" });
    assert.deepEqual(findActivePlaybook([assistantCall(0, "ws-skill", { name: "lead-run" }), assistantCall(1, "ws-skill", { name: "lead-revive" })]), { name: "lead-run", via: "ws-skill" });
    assert.deepEqual(findActivePlaybook([user(0, skillExpansion("lead-discuss", "topic"))]), { name: "lead-discuss", via: "/skill" });
    assert.equal(findActivePlaybook([]), undefined);
  });
});

describe("buildLeadCompactionSummary", () => {
  const prose = renderLeadProse({ current_work: "LEAD PROSE MARKER" });

  test("carries the deterministic sections and the prose, closes with lead-revive, and lists no files", () => {
    const entries = [
      user(0, "please keep replies short"),
      assistantCall(1, "ws-skill", { name: "lead-run" }),
      assistantCall(2, "ws__tickets_acquire", { ticket_stem: "261002-feat-pi-lead-ws-owned-compaction" }),
      custom(3, PUSH_BATCH_CUSTOM_TYPE, "WORKER REPORT BODY"),
    ];
    const summary = buildLeadCompactionSummary({
      sessionKey: "engaged-key",
      branchEntries: entries,
      registry: registry([{ agentId: "w", alias: "w1", running: true, spawnRole: "worker" }]),
      prose,
      ...dialog,
    });
    for (const heading of ["## Session", "## Child agents in flight", "## Child agents finished since the previous compaction", "## Dialog", "## Carried forward by the lead", "## Resume"]) {
      assert.ok(summary.includes(`\n${heading}\n`), heading);
    }
    assert.match(summary, /ws session key: `engaged-key` \(preserve verbatim\)/);
    assert.match(summary, /Active ticket: `261002-feat-pi-lead-ws-owned-compaction`/);
    assert.match(summary, /Active playbook: `lead-run`\. Its body is not re-attached: re-read it with `ws-skill lead-run`/);
    assert.match(summary, /- w1 \(w\) \[worker\]/);
    assert.match(summary, /<message role="user" timestamp=".+">\nplease keep replies short\n<\/message>\n<tool-calls>\n\u2192 ws-skill \{"name":"lead-run"\}\n\u2192 ws__tickets_acquire \{"ticket_stem":"261002-feat-pi-lead-ws-owned-compaction"\}\n<\/tool-calls>/);
    assert.match(summary, /search it \(for example with grep\)/);
    assert.match(summary, /`\/sessions\/lead\.jsonl`/);
    assert.match(summary, /LEAD PROSE MARKER/);
    assert.doesNotMatch(summary, /WORKER REPORT BODY/);
    assert.doesNotMatch(summary, /read-files|modified-files/);
    assert.ok(summary.trimEnd().endsWith("it restores agenda, todos, and notes through `workflow_manual`. After `lead-revive`, read the files under Required re-reads (to read first), then resume from this summary, the inlined files, and the immediate next step; open a reference or any other file only when a step needs it, not to rebuild the earlier context."));
    assert.doesNotMatch(summary, /## Required re-reads|## On-demand references/, "each file-list section is omitted when empty");
    assert.match(summary, /invoke `lead-revive` \(`ws-skill lead-revive`\) with session key `engaged-key`/);
    assert.equal(extractLeadProse(summary), prose);
  });

  test("sections are recomputed from the full history across two compactions, not inherited from the previous summary", () => {
    const reg = registry([{ agentId: "a", alias: "first", settledAt: Date.parse(at(5)), lastText: "status: [ok]" }]);
    const before = [user(0, "message before the first compaction")];
    const first = buildLeadCompactionSummary({ sessionKey: "k", branchEntries: before, registry: reg, prose, ...dialog });
    assert.match(first, /- first \(a\)/);

    reg.set("b", { agentId: "b", alias: "second", running: false, streaming: false, reportLog: [], settledAt: Date.parse(at(30)), lastText: "status: [ok]" } as never);
    const after = [...before, compaction(10, first), user(20, "message after the first compaction")];
    const second = buildLeadCompactionSummary({ sessionKey: "k", branchEntries: after, registry: reg, prose: renderLeadProse({ current_work: "NEW PROSE" }), ...dialog });
    assert.doesNotMatch(second, /- first \(a\)/, "a child finished before the previous compaction is not listed again");
    assert.match(second, /- second \(b\)/);
    assert.match(second, /message before the first compaction/, "older human messages come from branch history, not the old summary");
    assert.match(second, /message after the first compaction/);
    assert.equal(second.split("message before the first compaction").length, 2, "the earlier message appears once");
    assert.doesNotMatch(second, /LEAD PROSE MARKER/, "the previous prose is not inherited");
    assert.match(second, /All 2 dialog items of this session\./);
  });

  test("a missing session key is named as unknown rather than left blank", () => {
    const summary = buildLeadCompactionSummary({ sessionKey: undefined, branchEntries: [], registry: undefined, prose, ...dialog });
    assert.match(summary, /ws session key: unknown/);
    assert.match(summary, /Active ticket: none/);
    assert.match(summary, /## Child agents in flight\n\(none\)/);
  });
});

describe("preparation and fallback text", () => {
  // 261006: the advisory head, the guide's opening paragraph, and the
  // milestone line are pinned verbatim from the ticket's agreed prose.
  test("the advisory head is the pinned standing-intent text, then the guide", () => {
    const message = buildPreparationMessage({ kind: "advisory", percent: 51.4, threshold: 50, hardPercent: 80, milestonePercents: [60, 70] }, "GUIDE BODY");
    assert.equal(
      message,
      "[system message from ws-pi-plugin]\nContext usage is 51% of the window (advisory point: 50%). Compaction becomes forced at 80%.\n\nThis is not an instruction to compact right now. If you are in active discussion with the human, awaiting their answer or clarification, or holding working context that would be costly to rebuild (a half-applied change or a diagnosis in progress), carry on for now. A pause after asking the human a question is not a boundary.\n\nFrom here on, look for a good moment to compact before 80%. A good moment is a natural boundary where most of what this context holds is no longer needed for the work ahead, and what is still needed can be restored cheaply after compaction from durable records (tickets, commits, notes, agenda) and the summary. Typical cases: work just landed, you are waiting only on background agents, or the next work is weakly related to the current context. When such a moment comes, run the preparation below. Compacting on your own terms keeps the summary in your hands; at 80% it is forced, mid-work if need be. Reminders follow at 60% and 70%.\n\nIf now is not such a moment, end this turn without replying and keep looking for one as you work.\n\nGUIDE BODY",
    );
  });

  test("the advisory head interpolates non-default thresholds", () => {
    const message = buildPreparationMessage({ kind: "advisory", percent: 41.6, threshold: 40, hardPercent: 75, milestonePercents: [40 + 35 / 3, 40 + 70 / 3] }, "GUIDE BODY");
    assert.match(message, /^\[system message from ws-pi-plugin\]\nContext usage is 42% of the window \(advisory point: 40%\)\. Compaction becomes forced at 75%\.\n/);
    assert.match(message, /look for a good moment to compact before 75%\./);
    assert.match(message, /at 75% it is forced, mid-work if need be\. Reminders follow at 52% and 63%\.\n/);
  });

  // 261007: the two milestone texts are pinned verbatim, strengthening per milestone.
  test("each milestone is its pinned text with the rounded usage and the hard point", () => {
    assert.equal(
      buildContextMilestoneMessage(1, 62.6, 85),
      "[system message from ws-pi-plugin]\nContext window: 63% / 85% (forced compaction point). Past 85%, compaction is forced, mid-work if need be. From here on, a good boundary is worth taking; if this is one, run the preparation from the advisory. Otherwise end this turn without replying.",
    );
    assert.equal(
      buildContextMilestoneMessage(2, 70.2, 80),
      "[system message from ws-pi-plugin]\nContext window: 70% / 80% (forced compaction point). This is the last reading before compaction is forced. Compacting at a boundary you choose keeps the summary in your hands; take this one if it fits, and run the preparation from the advisory. Otherwise end this turn without replying, and take the next good boundary.",
    );
    assert.equal(LEAD_CONTEXT_MILESTONE_CUSTOM_TYPE, "ws-lead-context-milestone");
  });

  test("hard and manual triggers keep their imperative heads and verbatim guide", () => {
    const guide = "GUIDE BODY";
    assert.equal(buildPreparationMessage({ kind: "hard", percent: 80, threshold: 80 }, guide), "[system message from ws-pi-plugin]\nContext usage is 80% of the window, past the hard compaction point (80%). Stop the current work now and prepare for compaction with the guide below before anything else.\n\nGUIDE BODY");
    const manualHead = "[system message from ws-pi-plugin]\nThe user ran /compact; the adapter cancelled Pi's native compaction so you can prepare it. Prepare for compaction now with the guide below.";
    assert.equal(buildPreparationMessage({ kind: "reroute", focus: "  keep the API notes  " }, guide), `${manualHead}\nThe user's /compact focus text, to honor in your prose:\nkeep the API notes\n\nGUIDE BODY`);
    assert.equal(buildPreparationMessage({ kind: "reroute" }, guide), `${manualHead}\n\nGUIDE BODY`);
  });

  test("the packaged guide is read fresh and ends with the lever call; a missing file falls back", () => {
    const guide = readLeadCompactGuide(new URL("../lead-compact-guide.md", import.meta.url).pathname);
    assert.match(guide, /^# Preparing for compaction/);
    assert.ok(
      guide.startsWith("# Preparing for compaction\n\nThe preparation below runs once compaction is decided: at the hard point, for\nthe user's `/compact`, or at a safe boundary you pick after the advisory. After\nthe advisory, compaction is a standing intent rather than an immediate task. A\nsafe boundary needs both no active discussion with the human and no human\nanswer or clarification being awaited; a pause after asking a question is not\none. Until such a boundary comes, continue the current work or exchange, and\ntake the boundary when it does.\n\nCompaction replaces this conversation"),
      "the guide opens with the pinned standing-intent paragraph",
    );
    assert.match(guide, /Do these in order, without starting new work in between:/);
    assert.ok(guide.includes("adds and what `lead-revive` restores (agenda, todos, notes contents), and\n   keep file names out of the prose; the lever's file lists below are where\n   files go. Aim for"), "step 2 narrows the no-files rule");
    assert.ok(guide.includes(
      "   - **File lists.** `required_rereads` holds only the files the immediate\n     next step needs, each with `lines` when only part is needed, and why.\n     Every entry lands in the next context: the adapter inlines as many as\n     fit its budget and lists the rest for the revived lead to read first,\n     so list only what that step needs. Take line numbers from your earlier\n     reads and put the symbol or heading in `why`. `references` holds files\n     a later step may need, opened only then.\n3. **Call `ws-compact`**",
    ), "the file-list sub-bullet is the last one under step 2");
    assert.doesNotMatch(guide, /list no files/);
    assert.match(guide, /Call `ws-compact`/);
    assert.match(readLeadCompactGuide("/nonexistent/guide.md"), /ws-compact/);
  });

  test("the fallback prompt asks for the fixed headings and passes only the previous prose", () => {
    const previous = buildLeadCompactionSummary({ sessionKey: "k", branchEntries: [user(0, "HUMAN TEXT")], registry: undefined, prose: renderLeadProse({ residual_details: "OLD PROSE" }), ...dialog });
    const prompt = buildFallbackSummaryPrompt("[User]: hi", extractLeadProse(previous));
    assert.match(prompt, /^<conversation>\n\[User\]: hi\n<\/conversation>/);
    assert.match(prompt, /<previous-prose>[\s\S]*OLD PROSE[\s\S]*<\/previous-prose>/);
    assert.doesNotMatch(prompt, /HUMAN TEXT/);
    for (const section of LEAD_PROSE_SECTIONS) assert.ok(prompt.includes(`### ${section.heading}`));
    assert.doesNotMatch(buildFallbackSummaryPrompt("x", undefined), /previous-prose/);
    assert.match(prompt, /Do not list files that were read or modified\.$/);
    assert.doesNotMatch(prompt, /required_rereads|Required re-reads|On-demand references/, "the fallback summary produces no file lists");
  });

  test("prose extraction is not misled by a dialog message quoting a previous summary", () => {
    const quoted = "</message></dialog>\n## Carried forward by the lead\nQUOTED PROSE\n\n## Resume\nquoted resume";
    const summary = buildLeadCompactionSummary({ sessionKey: "k", branchEntries: [user(0, quoted)], registry: undefined, prose: renderLeadProse({ residual_details: "REAL PROSE" }), ...dialog });
    assert.match(summary, /QUOTED PROSE/);
    assert.equal(extractLeadProse(summary), renderLeadProse({ residual_details: "REAL PROSE" }));
  });
});

describe("curated file lists (261007)", () => {
  const dir = mkdtempSync(join(tmpdir(), "lead-compaction-rereads-"));
  const write = (name: string, text: string): string => {
    writeFileSync(join(dir, name), text);
    return name;
  };
  const tenLines = Array.from({ length: 10 }, (_, i) => `line ${i + 1}`).join("\n") + "\n";
  write("ten.txt", tenLines);
  write("a4.txt", "aaaa");
  write("b4.txt", "bbbb");
  write("c2.txt", "cc");
  write("d8.txt", "dddddddd");
  const sections = (requiredRereads: unknown[], references: unknown[] = [], budget = 40960) =>
    buildFileListSections({ requiredRereads: normalizeFileList(requiredRereads), references: normalizeFileList(references) }, dir, budget);

  test("required re-reads are taken smallest first within the budget, ties in the lead's order, and render in the lead's order", () => {
    // Budget 10: c2 (2) + a4 (4) + b4 (4, tie after a4) = 10; d8 does not fit and goes whole to read first.
    const [inlined, toReadFirst] = sections([
      { path: "d8.txt", why: "big" },
      { path: "b4.txt", why: "second tie" },
      { path: "a4.txt", why: "first tie" },
      { path: "c2.txt", why: "small" },
    ], [], 10);
    assert.equal(inlined, "## Required re-reads (inlined)\n<file path=\"b4.txt\" why=\"second tie\">\nbbbb\n</file>\n<file path=\"a4.txt\" why=\"first tie\">\naaaa\n</file>\n<file path=\"c2.txt\" why=\"small\">\ncc\n</file>");
    assert.equal(toReadFirst, "## Required re-reads (to read first)\n- `d8.txt` - big");

    const [tieInlined, tieFirst] = sections([{ path: "b4.txt", why: "b" }, { path: "a4.txt", why: "a" }], [], 4);
    assert.match(tieInlined!, /path="b4\.txt"/, "an equal-size tie goes to the lead's earlier entry");
    assert.equal(tieFirst, "## Required re-reads (to read first)\n- `a4.txt` - a", "the entry that does not fit is listed whole, never cut");
  });

  test("a lines range is cut 1-based and inclusive, clamped at the file's end, and the heading shows the range taken", () => {
    const [inlined] = sections([{ path: "ten.txt", lines: "2-3", why: "head" }, { path: "ten.txt", lines: "9-40", why: "tail" }]);
    assert.equal(inlined, "## Required re-reads (inlined)\n<file path=\"ten.txt\" lines=\"2-3\" why=\"head\">\nline 2\nline 3\n</file>\n<file path=\"ten.txt\" lines=\"9-10\" why=\"tail\">\nline 9\nline 10\n</file>");
  });

  test("a whole-file entry inlines the file and drops the lines part", () => {
    const [inlined] = sections([{ path: "ten.txt", why: "all of it" }]);
    assert.equal(inlined, `## Required re-reads (inlined)\n<file path="ten.txt" why="all of it">\n${tenLines}</file>`);
  });

  test("an unparsable range and one starting past the last line are listed to read first as written; a missing path is marked", () => {
    const rendered = sections([
      { path: "ten.txt", lines: "the parser", why: "unparsable" },
      { path: "ten.txt", lines: "11-12", why: "past the end" },
      { path: "ten.txt", lines: "0-2", why: "not 1-based" },
      { path: "missing.ts", why: "gone" },
      { path: "missing.ts", lines: "1-2", why: "gone too" },
    ]);
    assert.deepEqual(rendered, [
      "## Required re-reads (to read first)\n- `ten.txt` lines the parser - unparsable\n- `ten.txt` lines 11-12 - past the end\n- `ten.txt` lines 0-2 - not 1-based\n- `missing.ts` - gone (not found)\n- `missing.ts` lines 1-2 - gone too (not found)",
    ], "no inlined section when nothing is inlined");
  });

  test("a file that is not strict UTF-8 text is listed to read first unmarked, never inlined", () => {
    writeFileSync(join(dir, "nul.bin"), Buffer.from([0x61, 0x00, 0x62]));
    writeFileSync(join(dir, "latin1.txt"), Buffer.from([0x63, 0x61, 0x66, 0xe9]));
    writeFileSync(join(dir, "pixel.png"), Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00]));
    assert.deepEqual(sections([
      { path: "nul.bin", why: "has a NUL" },
      { path: "latin1.txt", lines: "1-1", why: "invalid UTF-8" },
      { path: "pixel.png", why: "an image" },
      { path: "c2.txt", why: "small" },
    ]), [
      "## Required re-reads (inlined)\n<file path=\"c2.txt\" why=\"small\">\ncc\n</file>",
      "## Required re-reads (to read first)\n- `nul.bin` - has a NUL\n- `latin1.txt` lines 1-1 - invalid UTF-8\n- `pixel.png` - an image",
    ]);
  });

  test("a whole file over the budget is listed to read first while a lines range of it still inlines", () => {
    assert.deepEqual(sections([{ path: "ten.txt", why: "all of it" }, { path: "ten.txt", lines: "1-1", why: "first line" }], [], 10), [
      "## Required re-reads (inlined)\n<file path=\"ten.txt\" lines=\"1-1\" why=\"first line\">\nline 1\n</file>",
      "## Required re-reads (to read first)\n- `ten.txt` - all of it",
    ]);
  });

  test("an inlined file sits in a file tag with escaped attributes and a verbatim body", () => {
    write("fenced.md", "before\n```ts\ncode & <b>\n```\nafter");
    const [inlined] = sections([{ path: "fenced.md", why: "has \"fences\" & <tags>" }]);
    assert.equal(inlined, '## Required re-reads (inlined)\n<file path="fenced.md" why="has &quot;fences&quot; &amp; &lt;tags&gt;">\nbefore\n```ts\ncode & <b>\n```\nafter\n</file>');
  });

  test("references are listed in order and never read; each section is omitted when empty", () => {
    assert.deepEqual(sections([], [{ path: "missing.ts", lines: "3-4", why: "later" }, { path: "ten.txt", why: "maybe" }]), [
      "## On-demand references\n- `missing.ts` lines 3-4 - later\n- `ten.txt` - maybe",
    ]);
    assert.deepEqual(sections([], []), []);
    assert.deepEqual(buildFileListSections(undefined, dir, 40960), []);
  });

  test("lever entries without a string path are dropped; a blank lines value means the whole file", () => {
    assert.deepEqual(normalizeFileList([{ path: "a", why: "w" }, { why: "no path" }, "x", null, { path: "b", lines: " ", why: "blank" }, { path: "c", lines: "1-2" }]), [
      { path: "a", why: "w" },
      { path: "b", why: "blank" },
      { path: "c", lines: "1-2", why: "" },
    ]);
    assert.deepEqual(normalizeFileList(undefined), []);
  });

  test("the summary places the sections right before the lead's prose, which extraction keeps clean", () => {
    const prose = renderLeadProse({ current_work: "PROSE" });
    const summary = buildLeadCompactionSummary({
      sessionKey: "k",
      branchEntries: [],
      registry: undefined,
      prose,
      ...dialog,
      fileLists: { requiredRereads: [{ path: "c2.txt", why: "small" }], references: [{ path: "ten.txt", why: "later" }] },
      cwd: dir,
      rereadBudgetBytes: 40960,
    });
    assert.ok(summary.includes("</dialog>\n\n## Required re-reads (inlined)\n<file path=\"c2.txt\" why=\"small\">\ncc\n</file>\n\n## On-demand references\n- `ten.txt` - later\n\n## Carried forward by the lead\n"));
    assert.equal(extractLeadProse(summary), prose);
  });
});

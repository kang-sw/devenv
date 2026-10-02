/**
 * Unit tests for lead-compaction.ts (261002): the summary the lead session's
 * `session_before_compact` handler returns. The hook wiring itself is covered
 * in test/goal-loop.test.ts's "lead compaction ownership" suites.
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import type { SessionEntry } from "@earendil-works/pi-coding-agent";
import {
  buildFallbackSummaryPrompt,
  buildLeadCompactionSummary,
  buildPreparationMessage,
  collectHumanMessages,
  describeFinishedChildren,
  describeInFlightChildren,
  extractLeadProse,
  findActivePlaybook,
  findActiveTicket,
  humanTextOf,
  LEAD_PROSE_SECTIONS,
  leadProseParameterSchema,
  readLeadCompactGuide,
  renderLeadProse,
  selectHumanMessages,
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
const custom = (minute: number, customType: string, content: string): SessionEntry => ({ type: "custom_message", id: `c${++seq}`, parentId: null, timestamp: at(minute), customType, content, display: true } as never);
const compaction = (minute: number, summary: string): SessionEntry => ({ type: "compaction", id: `k${++seq}`, parentId: null, timestamp: at(minute), summary, firstKeptEntryId: "x", tokensBefore: 1 } as never);
const budgets = { totalTokens: 8000, perMessageTokens: 1500 };
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
  });
});

describe("human-typed user messages", () => {
  test("adapter traffic is excluded and a /skill: expansion collapses to what the human typed", () => {
    assert.equal(humanTextOf(buildPushWakeLine(3)), undefined, "push wake line");
    assert.equal(humanTextOf("Goal yet running: \"x\".\n\n<!-- ws-pi-goal-reminder:1-1 -->"), undefined, "goal reminder");
    assert.equal(humanTextOf(skillExpansion("lead-run", "run 261002")), "/skill:lead-run run 261002");
    assert.equal(humanTextOf(skillExpansion("lead-discuss")), "/skill:lead-discuss");
    assert.equal(humanTextOf("Goal armed: ship"), "Goal armed: ship", "the /goal announcement carries the human's goal");
    assert.equal(humanTextOf("ordinary request"), "ordinary request");
  });

  test("only user-role message entries qualify; push batches, mailbox, and preparation messages never do", () => {
    const entries = [
      user(0, "first human message"),
      custom(1, PUSH_BATCH_CUSTOM_TYPE, "<ws-push-batch>WORKER REPORT</ws-push-batch>"),
      custom(2, "ws-lead-compact", "PREPARATION GUIDE"),
      user(3, buildPushWakeLine(1)),
      user(4, [{ type: "text", text: "with" }, { type: "image", data: "", mimeType: "image/png" }]),
      { type: "message", id: "b", parentId: null, timestamp: at(5), message: { role: "bashExecution", command: "ls", output: "BASH OUTPUT" } } as never,
      assistantCall(6, "ws-skill", { name: "lead-run" }),
    ];
    const texts = collectHumanMessages(entries).map((message) => message.text);
    assert.deepEqual(texts, ["first human message", "with\n[image]"]);
  });

  test("each message is capped with a truncation marker and the section budget keeps a contiguous newest run", () => {
    const capChars = 10 * 4;
    const messages = [
      { timestamp: at(0), text: "oldest" },
      { timestamp: at(1), text: "x".repeat(100) },
      { timestamp: at(2), text: "newest" },
    ];
    const tight = selectHumanMessages(messages, { totalTokens: 21, perMessageTokens: 10 });
    assert.equal(tight.kept.length, 2);
    assert.equal(tight.kept[1]!.text, "newest");
    assert.ok(tight.kept[0]!.text.startsWith("x".repeat(capChars)));
    assert.match(tight.kept[0]!.text, /\n\[\.\.\. truncated: 60 more characters\]$/);
    assert.equal(tight.omitted, 1, "the oldest message drops out once the budget is spent");

    const roomy = selectHumanMessages(messages, { totalTokens: 1000, perMessageTokens: 1000 });
    assert.deepEqual(roomy.kept.map((message) => message.text), messages.map((message) => message.text), "kept oldest-first for reading");
    assert.equal(roomy.omitted, 0);
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
      budgets,
    });
    for (const heading of ["## Session", "## Child agents in flight", "## Child agents finished since the previous compaction", "## User messages", "## Carried forward by the lead", "## Resume"]) {
      assert.ok(summary.includes(`\n${heading}\n`), heading);
    }
    assert.match(summary, /ws session key: `engaged-key` \(preserve verbatim\)/);
    assert.match(summary, /Active ticket: `261002-feat-pi-lead-ws-owned-compaction`/);
    assert.match(summary, /Active playbook: `lead-run`\. Its body is not re-attached: re-read it with `ws-skill lead-run`/);
    assert.match(summary, /- w1 \(w\) \[worker\]/);
    assert.match(summary, /please keep replies short/);
    assert.match(summary, /LEAD PROSE MARKER/);
    assert.doesNotMatch(summary, /WORKER REPORT BODY/);
    assert.doesNotMatch(summary, /read-files|modified-files/);
    assert.ok(summary.trimEnd().endsWith("it restores agenda, todos, and notes through `workflow_manual`."));
    assert.match(summary, /invoke `lead-revive` \(`ws-skill lead-revive`\) with session key `engaged-key`/);
    assert.equal(extractLeadProse(summary), prose);
  });

  test("sections are recomputed from the full history across two compactions, not inherited from the previous summary", () => {
    const reg = registry([{ agentId: "a", alias: "first", settledAt: Date.parse(at(5)), lastText: "status: [ok]" }]);
    const before = [user(0, "message before the first compaction")];
    const first = buildLeadCompactionSummary({ sessionKey: "k", branchEntries: before, registry: reg, prose, budgets });
    assert.match(first, /- first \(a\)/);

    reg.set("b", { agentId: "b", alias: "second", running: false, streaming: false, reportLog: [], settledAt: Date.parse(at(30)), lastText: "status: [ok]" } as never);
    const after = [...before, compaction(10, first), user(20, "message after the first compaction")];
    const second = buildLeadCompactionSummary({ sessionKey: "k", branchEntries: after, registry: reg, prose: renderLeadProse({ current_work: "NEW PROSE" }), budgets });
    assert.doesNotMatch(second, /- first \(a\)/, "a child finished before the previous compaction is not listed again");
    assert.match(second, /- second \(b\)/);
    assert.match(second, /message before the first compaction/, "older human messages come from branch history, not the old summary");
    assert.match(second, /message after the first compaction/);
    assert.equal(second.split("message before the first compaction").length, 2, "the earlier message appears once");
    assert.doesNotMatch(second, /LEAD PROSE MARKER/, "the previous prose is not inherited");
    assert.match(second, /All 2 human-typed messages/);
  });

  test("a missing session key is named as unknown rather than left blank", () => {
    const summary = buildLeadCompactionSummary({ sessionKey: undefined, branchEntries: [], registry: undefined, prose, budgets });
    assert.match(summary, /ws session key: unknown/);
    assert.match(summary, /Active ticket: none/);
    assert.match(summary, /## Child agents in flight\n\(none\)/);
  });
});

describe("preparation and fallback text", () => {
  test("each trigger leads the guide body; the reroute carries the /compact focus text", () => {
    const guide = "GUIDE BODY";
    assert.match(buildPreparationMessage({ kind: "advisory", percent: 51.4, threshold: 50 }, guide), /^Context usage is 51% .*advisory point \(50%\)[\s\S]*\n\nGUIDE BODY$/);
    assert.match(buildPreparationMessage({ kind: "hard", percent: 80, threshold: 80 }, guide), /^Context usage is 80% .*hard compaction point \(80%\)\. Stop the current work now[\s\S]*GUIDE BODY$/);
    const reroute = buildPreparationMessage({ kind: "reroute", focus: "  keep the API notes  " }, guide);
    assert.match(reroute, /^The user ran \/compact/);
    assert.match(reroute, /focus text, to honor in your prose:\nkeep the API notes\n\nGUIDE BODY$/);
    assert.doesNotMatch(buildPreparationMessage({ kind: "reroute" }, guide), /focus text/);
  });

  test("the packaged guide is read fresh and ends with the lever call; a missing file falls back", () => {
    const guide = readLeadCompactGuide(new URL("../lead-compact-guide.md", import.meta.url).pathname);
    assert.match(guide, /^# Preparing for compaction/);
    assert.match(guide, /Call `ws-compact`/);
    assert.match(readLeadCompactGuide("/nonexistent/guide.md"), /ws-compact/);
  });

  test("the fallback prompt asks for the fixed headings and passes only the previous prose", () => {
    const previous = buildLeadCompactionSummary({ sessionKey: "k", branchEntries: [user(0, "HUMAN TEXT")], registry: undefined, prose: renderLeadProse({ residual_details: "OLD PROSE" }), budgets });
    const prompt = buildFallbackSummaryPrompt("[User]: hi", extractLeadProse(previous));
    assert.match(prompt, /^<conversation>\n\[User\]: hi\n<\/conversation>/);
    assert.match(prompt, /<previous-prose>[\s\S]*OLD PROSE[\s\S]*<\/previous-prose>/);
    assert.doesNotMatch(prompt, /HUMAN TEXT/);
    for (const section of LEAD_PROSE_SECTIONS) assert.ok(prompt.includes(`### ${section.heading}`));
    assert.doesNotMatch(buildFallbackSummaryPrompt("x", undefined), /previous-prose/);
  });
});

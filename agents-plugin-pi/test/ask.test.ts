/**
 * Unit tests for ask.ts's pure-logic seams (260904 Phase 2, side-thread
 * owner-question surface): the thread-id/title derivation, the persisted
 * registry's path/serialize/parse contract (including its never-throw
 * tolerance), the `N pending` widget arithmetic, `/thread`'s listing and the
 * reopen selection, the §7 compaction check and verbatim excerpt, the
 * role-differentiated `addAskToolsIfLead`, the Entry-B prompt/directive/
 * injection texts (asserting Entry A's structural frame is ABSENT), and the
 * dormant-rehydration record shape.
 *
 * NOT covered here — genuinely live-gate only, mirroring test/fork.test.ts's
 * own pure/IO split: `registerAsk`'s tool `execute()` bodies,
 * `registerThreadCommands`'s handlers, the lazy discussion-fork spawn and
 * the overlay attach (all need a live `pi` session or a real `RpcClient`).
 * Those are the plan's tmux-probe and owner-runbook tiers.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, readFileSync, chmodSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  ASK_TOOL_NAME,
  RESOLVE_TOOL_NAME,
  PENDING_WIDGET_KEY,
  nextThreadId,
  deriveThreadTitle,
  threadRegistryPath,
  serializeThreadRegistry,
  parseThreadRegistry,
  loadThreadRegistryFile,
  saveThreadRegistryFile,
  hydrateThreadRegistry,
  createThreadRegistryHandle,
  refreshPendingWidget,
  countPending,
  buildWidgetLines,
  buildThreadListLines,
  mostRecentReopenable,
  isEntryLive,
  extractEntryText,
  buildVerbatimExcerpt,
  addAskToolsIfLead,
  buildDiscussionForkDirectiveText,
  buildDiscussionForkInitialMessage,
  buildInjectionMessage,
  captureForkResume,
  rehydrateForkRecord,
  getLeafEntryId,
  handleForkRaisedQuestion,
  type ThreadRecord,
} from "../src/ask.ts";
import { FORK_EXCLUDED_TOOL_NAMES } from "../src/fork.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "../src/spawner.ts";

function thread(overrides: Partial<ThreadRecord> = {}): ThreadRecord {
  return {
    threadId: "q1",
    title: "a question",
    status: "pending",
    createdAt: "2026-09-05T10:00:00.000Z",
    touchedAt: "2026-09-05T10:00:00.000Z",
    ...overrides,
  };
}

describe("tool names", () => {
  test("are the literals fork.ts duplicates in its exclusion set (the intentional-duplication pin)", () => {
    assert.equal(ASK_TOOL_NAME, "ws-ask");
    assert.equal(RESOLVE_TOOL_NAME, "ws-resolve");
    assert.ok(FORK_EXCLUDED_TOOL_NAMES.has(ASK_TOOL_NAME), "fork.ts must exclude ws-ask from a fork's own surface");
    assert.ok(FORK_EXCLUDED_TOOL_NAMES.has(RESOLVE_TOOL_NAME), "fork.ts must exclude ws-resolve from a fork's own surface");
  });
});

describe("nextThreadId", () => {
  test("starts at q1 for an empty registry", () => {
    assert.equal(nextThreadId([]), "q1");
  });

  test("walks past the highest existing numeric suffix, never reusing an id", () => {
    assert.equal(nextThreadId(["q1", "q2", "q3"]), "q4");
    assert.equal(nextThreadId(["q3", "q1"]), "q4");
  });

  test("ignores ids that do not match the q<N> shape", () => {
    assert.equal(nextThreadId(["legacy", "q2", ""]), "q3");
  });
});

describe("deriveThreadTitle (fork-raised threads have no author-supplied title)", () => {
  test("takes the first non-empty line", () => {
    assert.equal(deriveThreadTitle("\n\nShould I rebase or merge?\nMore detail below."), "Should I rebase or merge?");
  });

  test("truncates a long first line to the max length with an ellipsis", () => {
    const title = deriveThreadTitle("x".repeat(200), 20);
    assert.equal(title.length, 20);
    assert.ok(title.endsWith("…"));
  });

  test("falls back to a fixed label rather than producing an unlabelled thread", () => {
    assert.equal(deriveThreadTitle("   \n  "), "(untitled question)");
    assert.equal(deriveThreadTitle(""), "(untitled question)");
  });
});

describe("threadRegistryPath / serialize / parse", () => {
  test("the registry file is a sibling of the lead's own session file", () => {
    assert.equal(threadRegistryPath("/tmp/pi/session.jsonl"), "/tmp/pi/session.jsonl.ws-threads.json");
  });

  test("serialize/parse round-trips every field", () => {
    const records = [
      thread({ threadId: "q1", question: "why?", context: "because", entryId: "e7", status: "open", respondentAgentId: "agent-1" }),
      thread({ threadId: "q2", status: "dormant" }),
    ];
    assert.deepEqual(parseThreadRegistry(serializeThreadRegistry(records)), records);
  });

  test("malformed JSON degrades to an empty registry instead of throwing (goal-loop.ts's never-throw contract)", () => {
    assert.deepEqual(parseThreadRegistry("{not json"), []);
    assert.deepEqual(parseThreadRegistry(""), []);
    assert.deepEqual(parseThreadRegistry("null"), []);
    assert.deepEqual(parseThreadRegistry('{"threads": "nope"}'), []);
  });

  test("entries missing a threadId/title/status are dropped without poisoning the rest", () => {
    const raw = JSON.stringify({ threads: [{ threadId: "q1" }, { title: "no id", status: "pending" }, thread({ threadId: "q9" })] });
    const parsed = parseThreadRegistry(raw);
    assert.equal(parsed.length, 1);
    assert.equal(parsed[0].threadId, "q9");
  });

  test("an unknown status value is rejected (status drives every downstream branch)", () => {
    assert.deepEqual(parseThreadRegistry(JSON.stringify({ threads: [{ threadId: "q1", title: "t", status: "weird" }] })), []);
  });
});

describe("loadThreadRegistryFile / saveThreadRegistryFile (never-throw IO)", () => {
  test("a missing file loads as an empty registry", () => {
    assert.deepEqual(loadThreadRegistryFile(join(tmpdir(), "ws-pi-does-not-exist-9d1f", "threads.json")), []);
  });

  test("save then load round-trips through a real file", () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "session.jsonl.ws-threads.json");
    const records = [thread({ threadId: "q1", status: "open" })];
    saveThreadRegistryFile(path, records);
    assert.deepEqual(loadThreadRegistryFile(path), records);
    assert.ok(readFileSync(path, "utf8").endsWith("\n"));
  });

  test("a corrupt file on disk loads as an empty registry, never throwing", () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "threads.json");
    writeFileSync(path, "}}}not json{{{");
    assert.deepEqual(loadThreadRegistryFile(path), []);
  });

  test("an unwritable target degrades to a no-op rather than throwing", () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    chmodSync(dir, 0o500);
    try {
      saveThreadRegistryFile(join(dir, "threads.json"), [thread()]);
    } finally {
      chmodSync(dir, 0o700);
    }
  });

  test("hydrateThreadRegistry fills the in-memory map and records the path", () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "threads.json");
    saveThreadRegistryFile(path, [thread({ threadId: "q1" }), thread({ threadId: "q2", status: "dormant" })]);
    const handle = createThreadRegistryHandle();
    hydrateThreadRegistry(handle, path);
    assert.equal(handle.pathRef.current, path);
    assert.deepEqual([...handle.threads.keys()], ["q1", "q2"]);
  });
});

describe("countPending / buildWidgetLines", () => {
  test("counts pending threads only — an open or dormant thread is not still owed an answer", () => {
    const records = [thread({ threadId: "q1" }), thread({ threadId: "q2", status: "open" }), thread({ threadId: "q3", status: "dormant" }), thread({ threadId: "q4" })];
    assert.equal(countPending(records), 2);
  });

  test("zero pending clears the widget entirely (undefined content)", () => {
    assert.equal(buildWidgetLines(0), undefined);
    assert.equal(buildWidgetLines(-1), undefined);
  });

  test("the widget wording is singular for one and plural beyond", () => {
    assert.match(buildWidgetLines(1)![0], /1 pending question\b/);
    assert.match(buildWidgetLines(3)![0], /3 pending questions\b/);
  });
});

describe("refreshPendingWidget", () => {
  function fakeCtx(mode: string) {
    const calls: Array<{ key: string; content: string[] | undefined; options?: { placement?: string } }> = [];
    return {
      ctx: { mode, ui: { setWidget: (key: string, content: string[] | undefined, options?: { placement?: string }) => calls.push({ key, content, options }) } },
      calls,
    };
  }

  test("paints an aboveEditor widget in TUI mode", () => {
    const handle = createThreadRegistryHandle();
    handle.threads.set("q1", thread());
    const { ctx, calls } = fakeCtx("tui");
    refreshPendingWidget(ctx, handle);
    assert.equal(calls.length, 1);
    assert.equal(calls[0].key, PENDING_WIDGET_KEY);
    assert.equal(calls[0].options?.placement, "aboveEditor");
    assert.match(calls[0].content![0], /1 pending question/);
  });

  test("§8: never paints a widget outside TUI mode, and no-ops with no captured ctx", () => {
    const handle = createThreadRegistryHandle();
    handle.threads.set("q1", thread());
    const { ctx, calls } = fakeCtx("rpc");
    refreshPendingWidget(ctx, handle);
    refreshPendingWidget(undefined, handle);
    assert.deepEqual(calls, []);
  });
});

describe("buildThreadListLines / mostRecentReopenable", () => {
  const records = [
    thread({ threadId: "q1", title: "oldest", touchedAt: "2026-09-05T10:00:00.000Z" }),
    thread({ threadId: "q2", title: "newest", status: "open", respondentAgentId: "abcdef01-2345", touchedAt: "2026-09-05T12:00:00.000Z" }),
    thread({ threadId: "q3", title: "resolved by the lead itself", status: "closed", touchedAt: "2026-09-05T13:00:00.000Z" }),
  ];

  test("lists every non-closed thread, newest touch first, with its status and id", () => {
    const lines = buildThreadListLines(records);
    assert.match(lines[0], /^ws threads \(2\)/);
    assert.match(lines[1], /q2/);
    assert.match(lines[1], /open/);
    assert.match(lines[2], /q1/);
    assert.ok(!lines.join("\n").includes("resolved by the lead itself"), "a lead self-resolved thread is not listed");
  });

  test("renders an explicit empty state rather than a bare header", () => {
    assert.deepEqual(buildThreadListLines([]), ["ws threads: none open or pending."]);
    assert.deepEqual(buildThreadListLines([thread({ status: "closed" })]), ["ws threads: none open or pending."]);
  });

  test("the reopen shortcut targets the most recently touched non-closed thread", () => {
    assert.equal(mostRecentReopenable(records)?.threadId, "q2");
    assert.equal(mostRecentReopenable([]), undefined);
    assert.equal(mostRecentReopenable([thread({ status: "closed" })]), undefined);
  });

  test("a dormant thread is still reopenable (§9 retained, not deleted)", () => {
    const dormant = thread({ threadId: "q7", status: "dormant", touchedAt: "2026-09-05T23:00:00.000Z" });
    assert.equal(mostRecentReopenable([...records, dormant])?.threadId, "q7");
  });
});

describe("isEntryLive / extractEntryText / buildVerbatimExcerpt (§7 compaction anchoring)", () => {
  const branch = [
    { id: "e1", type: "message", message: { role: "user", content: "first" } },
    { id: "e2", type: "message", message: { role: "assistant", content: [{ type: "text", text: "second" }] } },
    { id: "e3", type: "model_change", provider: "p", modelId: "m" },
    { id: "e4", type: "message", message: { role: "user", content: "fourth" } },
  ];

  test("an entry still in the live context is live; one that has fallen behind a compaction boundary is not", () => {
    assert.equal(isEntryLive("e4", [{ id: "e3" }, { id: "e4" }]), true);
    assert.equal(isEntryLive("e1", [{ id: "e3" }, { id: "e4" }]), false);
    assert.equal(isEntryLive("e1", []), false);
  });

  test("extractEntryText handles string content, text parts, summaries, and unknown shapes", () => {
    assert.equal(extractEntryText(branch[0]), "first");
    assert.equal(extractEntryText(branch[1]), "second");
    assert.equal(extractEntryText({ type: "compaction", summary: "compacted" }), "compacted");
    assert.equal(extractEntryText(branch[2]), "");
    assert.equal(extractEntryText(undefined), "");
  });

  test("the excerpt renders a window ENDING at the anchor entry, tagged by role", () => {
    const excerpt = buildVerbatimExcerpt("e2", branch, 2);
    assert.ok(excerpt.includes("[user] first"));
    assert.ok(excerpt.includes("[assistant] second"));
    assert.ok(!excerpt.includes("fourth"), "entries after the anchor are never included");
  });

  test("entries with no renderable text are dropped from the window", () => {
    const excerpt = buildVerbatimExcerpt("e4", branch, 2);
    assert.ok(!excerpt.includes("model_change"));
    assert.ok(excerpt.includes("[user] fourth"));
  });

  test("an anchor absent from the branch yields no excerpt (never a fabricated one)", () => {
    assert.equal(buildVerbatimExcerpt("nope", branch, 3), "");
    assert.equal(buildVerbatimExcerpt("e2", branch, 0), "");
  });
});

describe("addAskToolsIfLead (role-differentiated, never folded into computeLeadActiveTools)", () => {
  test("the true top lead (role undefined) gains both tools", () => {
    assert.deepEqual(addAskToolsIfLead(["bash"], undefined), ["bash", ASK_TOOL_NAME, RESOLVE_TOOL_NAME]);
  });

  test("neither is duplicated when already present", () => {
    const result = addAskToolsIfLead(["bash", ASK_TOOL_NAME], undefined);
    assert.equal(result.filter((name) => name === ASK_TOOL_NAME).length, 1);
  });

  test('a "fork"/"worker"/"explore" role never gains them (a fork\'s only question path is ws-report-to-lead)', () => {
    for (const role of ["fork", "worker", "explore"] as const) {
      const result = addAskToolsIfLead(["bash"], role);
      assert.deepEqual(result, ["bash"], `role ${role} must not gain the owner-question tools`);
    }
  });
});

describe("Entry B texts (deliberately NOT wrapped in Entry A's structural frame)", () => {
  const framedMarkers = ["# Forked session", "--- Message from the lead ---", "--- end of message ---", "reference/background only"];

  test("the initial message carries the question, and the context when given", () => {
    const message = buildDiscussionForkInitialMessage("We are choosing between two anchors.", "Rebase or merge?");
    assert.ok(message.includes("Rebase or merge?"));
    assert.ok(message.includes("We are choosing between two anchors."));
  });

  test("the initial message omits the context section entirely when there is none", () => {
    const message = buildDiscussionForkInitialMessage(undefined, "Rebase or merge?");
    assert.ok(!message.includes("Context:"));
    assert.ok(!/compacted/i.test(message), "no excerpt section without an excerpt");
  });

  test("a post-compaction excerpt is inserted verbatim, labelled as compacted context", () => {
    const message = buildDiscussionForkInitialMessage(undefined, "Rebase or merge?", "[user] the original ask");
    assert.ok(message.includes("[user] the original ask"));
    assert.match(message, /compacted/i);
  });

  test("NONE of Entry A's structural-frame markers appear (the ticket's explicit do-not-wrap rule)", () => {
    const message = buildDiscussionForkInitialMessage("ctx", "q", "excerpt");
    for (const marker of framedMarkers) {
      assert.ok(!message.includes(marker), `Entry B must not carry Entry A's frame marker: ${marker}`);
    }
  });

  test("the directive names no report contract (a discussion thread exits via /done, not a report)", () => {
    const directive = buildDiscussionForkDirectiveText();
    assert.ok(!directive.includes("ws-report-to-lead"));
    assert.ok(!directive.includes('kind:"final"'));
    for (const marker of framedMarkers) {
      assert.ok(!directive.includes(marker));
    }
  });

  test("both texts stay calm — no identity framing, no ALL-CAPS override words", () => {
    for (const text of [buildDiscussionForkDirectiveText(), buildDiscussionForkInitialMessage("ctx", "q")]) {
      assert.ok(!/\byou\s+are\s+a\b/i.test(text), `must not open with identity framing: ${text}`);
      assert.deepEqual(text.match(/\b[A-Z]{4,}\b/g) ?? [], []);
    }
  });
});

describe("buildInjectionMessage (§6 payload: context + original question + summary)", () => {
  test("carries all three parts and marks itself as a discussion outcome, not an owner instruction", () => {
    const message = buildInjectionMessage("the background", "the question", "we picked merge");
    assert.ok(message.includes("the background"));
    assert.ok(message.includes("the question"));
    assert.ok(message.includes("we picked merge"));
    assert.match(message, /not a new instruction from the owner/i);
  });

  test("omits absent context/question sections rather than emitting empty labels", () => {
    const message = buildInjectionMessage(undefined, undefined, "we picked merge");
    assert.ok(!message.includes("Context:"));
    assert.ok(!message.includes("Question:"));
    assert.ok(message.includes("we picked merge"));
  });
});

describe("captureForkResume / rehydrateForkRecord (the persistence-gap resolution)", () => {
  const live = {
    agentId: "agent-1",
    client: { marker: "live" },
    sessionPath: "/tmp/forked/session.jsonl",
    systemPromptPath: "/tmp/prompt.md",
    modelBase: "prov/model",
    modelEffort: "high",
    wsToolNames: ["ws__todo_list"],
    toolGroup: "full-worker",
    explicitTools: "bash,ws-report-to-lead",
    streaming: true,
    idlePending: true,
    waiters: [() => {}],
    pendingReports: [{ message: "hi" }],
    reportsDropped: 2,
  } as unknown as RpcAgentRecord;

  test("capture keeps only JSON-serializable resume fields (never the live client or runtime state)", () => {
    const resume = captureForkResume(live);
    assert.deepEqual(resume, {
      sessionPath: "/tmp/forked/session.jsonl",
      systemPromptPath: "/tmp/prompt.md",
      explicitTools: "bash,ws-report-to-lead",
      wsToolNames: ["ws__todo_list"],
      toolGroup: "full-worker",
      modelBase: "prov/model",
      modelEffort: "high",
    });
    assert.deepEqual(JSON.parse(JSON.stringify(resume)), resume, "must round-trip through JSON");
  });

  test("rehydration reconstructs a spec-conformant dormant record with client undefined", () => {
    const record = rehydrateForkRecord("agent-1", captureForkResume(live));
    assert.equal(record.agentId, "agent-1");
    assert.equal(record.client, undefined, "client === undefined is what makes sendToAgent take its dormant-resume branch");
    assert.equal(record.sessionPath, "/tmp/forked/session.jsonl");
    assert.equal(record.systemPromptPath, "/tmp/prompt.md");
    assert.equal(record.explicitTools, "bash,ws-report-to-lead");
    assert.equal(record.toolGroup, "full-worker");
    assert.equal(record.modelBase, "prov/model");
    assert.equal(record.streaming, false);
    assert.equal(record.idlePending, false);
    assert.deepEqual(record.waiters, []);
    assert.deepEqual(record.pendingReports, []);
    assert.equal(record.reportsDropped, 0);
  });

  test("rehydration copies the tool-name list instead of aliasing the persisted array", () => {
    const resume = captureForkResume(live);
    const record = rehydrateForkRecord("agent-1", resume);
    assert.notEqual(record.wsToolNames, resume.wsToolNames);
    assert.deepEqual(record.wsToolNames, resume.wsToolNames);
  });

  test("a registry emptied by a lead restart accepts the rehydrated record under the same agent_id", () => {
    const registry: RpcAgentRegistry = new Map();
    const record = rehydrateForkRecord("agent-1", captureForkResume(live));
    registry.set("agent-1", record);
    assert.equal(registry.get("agent-1")?.sessionPath, "/tmp/forked/session.jsonl");
  });
});

describe("getLeafEntryId", () => {
  test("reads the tip entry id from a well-formed sessionManager", () => {
    assert.equal(getLeafEntryId({ sessionManager: { getLeafId: () => "entry-9" } }), "entry-9");
  });

  test("returns undefined for a missing/empty/null leaf (a fresh session has no tip yet)", () => {
    assert.equal(getLeafEntryId(undefined), undefined);
    assert.equal(getLeafEntryId({}), undefined);
    assert.equal(getLeafEntryId({ sessionManager: {} }), undefined);
    assert.equal(getLeafEntryId({ sessionManager: { getLeafId: () => null } }), undefined);
    assert.equal(getLeafEntryId({ sessionManager: { getLeafId: () => "" } }), undefined);
  });
});

describe("handleForkRaisedQuestion (Entry A meets Entry B)", () => {
  function setup() {
    const handle = createThreadRegistryHandle();
    const widgets: Array<string[] | undefined> = [];
    handle.ctxRef.current = { mode: "tui", ui: { setWidget: (_key, content) => widgets.push(content) } };
    const registry: RpcAgentRegistry = new Map();
    return { handle, registry, widgets };
  }

  test("registers a pending thread whose respondent is already the live fork, with no entryId", () => {
    const { handle, registry } = setup();
    const record = handleForkRaisedQuestion(handle, registry, "agent-7", "Should I rebase?\nDetail follows.");
    assert.equal(record.threadId, "q1");
    assert.equal(record.title, "Should I rebase?");
    assert.equal(record.question, "Should I rebase?\nDetail follows.");
    assert.equal(record.respondentAgentId, "agent-7");
    assert.equal(record.status, "pending");
    assert.equal(record.entryId, undefined, "the lead never authored an entry for a fork-raised question");
    assert.equal(handle.threads.get("q1"), record);
  });

  test("captures resume fields when the fork is on the shared registry, and tolerates it not being there", () => {
    const { handle, registry } = setup();
    registry.set("agent-7", {
      agentId: "agent-7",
      sessionPath: "/tmp/s.jsonl",
      systemPromptPath: "/tmp/p.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      streaming: false,
      idlePending: false,
      waiters: [],
      pendingReports: [],
      reportsDropped: 0,
    } as unknown as RpcAgentRecord);
    assert.equal(handleForkRaisedQuestion(handle, registry, "agent-7", "q?").forkResume?.sessionPath, "/tmp/s.jsonl");
    assert.equal(handleForkRaisedQuestion(handle, new Map(), "agent-9", "q?").forkResume, undefined);
  });

  test("bumps the pending widget through the re-captured ctx", () => {
    const { handle, registry, widgets } = setup();
    handleForkRaisedQuestion(handle, registry, "agent-7", "first?");
    handleForkRaisedQuestion(handle, registry, "agent-8", "second?");
    assert.match(widgets.at(-1)![0], /2 pending questions/);
  });

  test("a not-yet-captured ctx (the restart race the ticket names) is a guarded no-op, not a crash", () => {
    const handle = createThreadRegistryHandle();
    const record = handleForkRaisedQuestion(handle, new Map(), "agent-7", "q?");
    assert.equal(record.status, "pending");
  });

  test("registration persists to the registry file once a path is known", () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "session.jsonl.ws-threads.json");
    const handle = createThreadRegistryHandle();
    hydrateThreadRegistry(handle, path);
    handleForkRaisedQuestion(handle, new Map(), "agent-7", "persisted?");
    assert.equal(loadThreadRegistryFile(path)[0]?.question, "persisted?");
  });
});

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
 * Also covered (review relay #1, test-Important): `registerAsk`'s two tool
 * `execute()` bodies and `injectDiscussionSummary`, driven against a fake
 * `pi` + duck-typed `toolCtx` — neither spawns anything (§1 "registers only,
 * NO spawn"), so both are offline-testable in the `createApprovalRelay` mold
 * (test/execute-gateway.test.ts).
 *
 * Review relay #1 (C4/C5, I2/I5/I6) adds the `threadBound` lifecycle, which
 * was wired but barely asserted: bound from fork-raised REGISTRATION (before
 * any overlay exists), set by `ensureRespondent` on a first open and on a
 * post-restart reopen (now exported for that), and released by every close
 * path — `/done`'s two functions, `ws-resolve` against a REAL registry, a lead
 * stop, and the headless paths (the fork's own final on a never-opened thread,
 * and the lead answering through `ws-agent-send`).
 *
 * NOT covered here — genuinely live-gate only, mirroring test/fork.test.ts's
 * own pure/IO split: `registerThreadCommands`'s handlers, the lazy
 * discussion-fork spawn and the overlay attach (all need a live `pi` session
 * or a real `RpcClient`). Those are the plan's tmux-probe and owner-runbook
 * tiers.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe, afterEach } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, readFileSync, chmodSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  ASK_TOOL_NAME,
  RESOLVE_TOOL_NAME,
  nextThreadId,
  deriveThreadTitle,
  threadRegistryPath,
  serializeThreadRegistry,
  parseThreadRegistry,
  loadThreadRegistryFile,
  saveThreadRegistryFile,
  hydrateThreadRegistry,
  createThreadRegistryHandle,
  countPending,
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
  ensureRespondent,
  registerAsk,
  injectDiscussionSummary,
  closeThreadOnDone,
  handleRespondentFinalReport,
  normalizeThreadOrigin,
  normalizeTranscript,
  THREAD_TRANSCRIPT_CAP,
  checkContextLength,
  buildForkQuestionLeadNotice,
  MAX_CONTEXT_CHARS,
  type ThreadRecord,
} from "../src/ask.ts";
import type { OverlayHandle } from "../src/overlay-chat.ts";
import { FORK_EXCLUDED_TOOL_NAMES } from "../src/fork.ts";
import {
  agentWidgetRefreshRef,
  applyRpcEvent,
  computeRunningStatusLine,
  flushHeldPushes,
  heldPushQueue,
  leadCompactingRef,
  REPORT_TO_LEAD_TOOL_NAME,
  sendToAgent,
  stopAgent,
  type RpcAgentRecord,
  type RpcAgentRegistry,
} from "../src/spawner.ts";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

// 260905 (live-agent widget ticket): every widget-refresh call site in
// ask.ts now fires through spawner.ts's module-level `agentWidgetRefreshRef`
// instead of a locally-passed ctx/handle — reset it after every test so a
// spy installed by one test can never leak into the next.
//
// 260906 (compaction push-hold ticket, Phase 1): `leadCompactingRef` and
// `heldPushQueue` are the same module state `spawner.ts`'s own hold uses —
// reset both for the same leak-proofing reason.
afterEach(() => {
  agentWidgetRefreshRef.current = undefined;
  leadCompactingRef.current = false;
  heldPushQueue.length = 0;
});

function thread(overrides: Partial<ThreadRecord> = {}): ThreadRecord {
  return {
    threadId: "q1",
    title: "a question",
    status: "pending",
    origin: "fork-raised",
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

  test("a persisted transcript round-trips (dogfood: Esc/reopen and restart must not open an empty view)", () => {
    const record = thread({
      threadId: "q1",
      status: "open",
      transcript: [
        { who: "note", text: "Rebase or merge?" },
        { who: "you", text: "merge" },
        { who: "thread", text: "Merging keeps both histories." },
      ],
    });
    assert.deepEqual(parseThreadRegistry(serializeThreadRegistry([record])), [record]);
  });

  test("an absent transcript stays absent, and a malformed one degrades to only its well-formed entries", () => {
    const [absent] = parseThreadRegistry(JSON.stringify({ threads: [thread({ threadId: "q1" })] }));
    assert.ok(!("transcript" in absent), "no field is invented for a record written before transcripts existed");
    const [notArray] = parseThreadRegistry(JSON.stringify({ threads: [{ ...thread({ threadId: "q2" }), transcript: "nope" }] }));
    assert.ok(!("transcript" in notArray));
    const [mixed] = parseThreadRegistry(
      JSON.stringify({ threads: [{ ...thread({ threadId: "q3" }), transcript: [{ who: "you", text: "ok" }, { who: "alien", text: "x" }, { who: "note" }, null, 7] }] }),
    );
    assert.deepEqual(mixed.transcript, [{ who: "you", text: "ok" }]);
  });

  test("normalizeTranscript caps at the newest THREAD_TRANSCRIPT_CAP entries", () => {
    const many = Array.from({ length: THREAD_TRANSCRIPT_CAP + 25 }, (_, i) => ({ who: "thread" as const, text: `turn ${i}` }));
    const capped = normalizeTranscript(many)!;
    assert.equal(capped.length, THREAD_TRANSCRIPT_CAP);
    assert.equal(capped[0].text, "turn 25", "the oldest entries are the ones dropped");
    assert.equal(capped.at(-1)!.text, `turn ${THREAD_TRANSCRIPT_CAP + 24}`);
    assert.equal(normalizeTranscript(undefined), undefined);
    assert.equal(normalizeTranscript({}), undefined);
  });

  test("C2: origin round-trips, and an unknown/absent one defaults to fork-raised (never stop a task fork by mistake)", () => {
    const leadAsk = thread({ threadId: "q1", origin: "lead-ask" });
    assert.equal(parseThreadRegistry(serializeThreadRegistry([leadAsk]))[0].origin, "lead-ask");

    const raw = JSON.stringify({
      threads: [
        { threadId: "q1", title: "t", status: "pending", createdAt: "x", touchedAt: "x" },
        { threadId: "q2", title: "t", status: "pending", origin: "nonsense", createdAt: "x", touchedAt: "x" },
      ],
    });
    assert.deepEqual(
      parseThreadRegistry(raw).map((r) => r.origin),
      ["fork-raised", "fork-raised"],
    );
  });

  test("C2: normalizeThreadOrigin accepts only the lead-ask literal", () => {
    assert.equal(normalizeThreadOrigin("lead-ask"), "lead-ask");
    assert.equal(normalizeThreadOrigin("fork-raised"), "fork-raised");
    assert.equal(normalizeThreadOrigin(undefined), "fork-raised");
    assert.equal(normalizeThreadOrigin(""), "fork-raised");
    assert.equal(normalizeThreadOrigin(42), "fork-raised");
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

describe("countPending", () => {
  test("counts pending threads only — an open or dormant thread is not still owed an answer", () => {
    const records = [thread({ threadId: "q1" }), thread({ threadId: "q2", status: "open" }), thread({ threadId: "q3", status: "dormant" }), thread({ threadId: "q4" })];
    assert.equal(countPending(records), 2);
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

  test("the directive names both exits: the owner's /done, and the fork's own kind:\"final\" report once a decision is stated", () => {
    const directive = buildDiscussionForkDirectiveText();
    assert.ok(directive.includes("/done"));
    assert.ok(directive.includes("ws-report-to-lead"));
    assert.ok(directive.includes('kind:"final"'));
    assert.match(directive, /decision/i);
    assert.match(directive, /delivered to the lead/);
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
  test("carries all three parts, presented as the owner's own decisions but labeled a thread summary", () => {
    const message = buildInjectionMessage("the background", "the question", "we picked merge");
    assert.ok(message.includes("the background"));
    assert.ok(message.includes("the question"));
    assert.ok(message.includes("we picked merge"));
    // §6: the summary carries owner authority (the owner was present), so it
    // must not be demoted to "not an instruction from the owner"…
    assert.match(message, /owner's decisions/i);
    assert.match(message, /owner's authority/i);
    // …while still being distinguishable from a fresh owner turn.
    assert.match(message, /rather than as a new owner turn/i);
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
    spawnRole: "fork",
    streaming: true,
    running: true,
    threadBound: true,
    terminalThisTurn: true,
    reportLog: [{ kind: "final", at: 1 }],
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
    assert.equal(record.running, false, "a rehydrated record is dormant — it must not count toward the lead's fan-in until prompted");
    assert.deepEqual(record.reportLog, [], "the push model never restores report history; the reports were already delivered");
    assert.equal(record.spawnRole, "fork", "a rehydrated respondent is always a discussion/task fork");
    assert.equal(record.threadBound, undefined, "binding is re-established by ensureRespondent, not carried in the resume snapshot");
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
    const registry: RpcAgentRegistry = new Map();
    return { handle, registry };
  }

  test("registers a pending thread whose respondent is already the live fork, with no entryId", () => {
    const { handle, registry } = setup();
    const record = handleForkRaisedQuestion(handle, registry, "agent-7", "Should I rebase?\nDetail follows.");
    assert.equal(record.threadId, "q1");
    assert.equal(record.title, "Should I rebase?");
    assert.equal(record.question, "Should I rebase?\nDetail follows.");
    assert.equal(record.respondentAgentId, "agent-7");
    assert.equal(record.status, "pending");
    assert.equal(record.origin, "fork-raised", "C2: the origin decides whether /done may stop this respondent");
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
      spawnRole: "fork",
      streaming: false,
      running: false,
      reportLog: [],
    } as unknown as RpcAgentRecord);
    assert.equal(handleForkRaisedQuestion(handle, registry, "agent-7", "q?").forkResume?.sessionPath, "/tmp/s.jsonl");
    assert.equal(handleForkRaisedQuestion(handle, new Map(), "agent-9", "q?").forkResume, undefined);
  });

  test("bumps the merged live-agent widget refresh (260905: no widget of its own left here)", () => {
    const { handle, registry } = setup();
    let calls = 0;
    agentWidgetRefreshRef.current = () => {
      calls += 1;
    };
    handleForkRaisedQuestion(handle, registry, "agent-7", "first?");
    handleForkRaisedQuestion(handle, registry, "agent-8", "second?");
    assert.equal(calls, 2, "each registration fires the merged refresh once");
  });

  test("a not-yet-captured ctx (the restart race the ticket names) is a guarded no-op, not a crash", () => {
    const handle = createThreadRegistryHandle();
    const record = handleForkRaisedQuestion(handle, new Map(), "agent-7", "q?");
    assert.equal(record.status, "pending");
  });

  /** A live task fork on the shared registry, as `ws-fork` left it. */
  function liveFork(): RpcAgentRecord {
    return {
      agentId: "agent-7",
      sessionPath: "/tmp/s.jsonl",
      systemPromptPath: "/tmp/p.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      spawnRole: "fork",
      streaming: false,
      running: true,
      reportLog: [],
      client: {},
    } as unknown as RpcAgentRecord;
  }

  test("C4: the fork is threadBound from REGISTRATION — before the owner opens anything", () => {
    const { handle, registry } = setup();
    const live = liveFork();
    registry.set("agent-7", live);

    handleForkRaisedQuestion(handle, registry, "agent-7", "Should I rebase?");

    assert.equal(live.threadBound, true, "the exchange belongs to the owner from the moment the fork raised it");
    assert.equal(live.overlayAttached, undefined, "no VIEW is attached yet — the two flags have different lifetimes");
    assert.equal(
      computeRunningStatusLine(registry),
      undefined,
      "a question-parked fork is outside the fan-in count entirely, and an empty fan-in produces no line at all",
    );
  });

  test("I2 (headless): the fork's OWN final releases the bind even though no owner ever opened the thread", () => {
    const { handle, registry } = setup();
    // Headless (§8): `index.ts` still registers the thread, returns undefined
    // (so the question is relayed to the lead), and there is no owner surface
    // that could ever run /answer or /done on it.
    const pi = { sendMessage: () => assert.fail("§1: a fork-raised close never injects a summary") } as unknown as ExtensionAPI;
    const live = liveFork();
    registry.set("agent-7", live);
    const thread = handleForkRaisedQuestion(handle, registry, "agent-7", "Should I rebase?", pi);
    assert.equal(live.threadBound, true);
    assert.equal(thread.status, "pending", "the owner never opened it");

    // The fork works it out and files its own completion.
    const outcome = applyRpcEvent(live, {
      type: "tool_execution_start",
      toolName: REPORT_TO_LEAD_TOOL_NAME,
      args: { kind: "final", message: "Outcome: rebased." },
    });

    assert.equal(live.threadBound, false, "without this the fork is outside the fan-in count, and settle-suppressed, forever");
    assert.equal(handle.threads.get(thread.threadId)!.status, "dormant", "the owner has nothing left to answer");
    assert.deepEqual(outcome, {}, "Edition: a final is stashed, not pushed at tool-invocation time");
    assert.equal(live.pendingFinal, "Outcome: rebased.", "a fork-raised final is still the lead's completion signal — it is released when the fork's turn ends");
  });

  test("I2 (headless): the lead answering through ws-agent-send releases the bind at that moment", async () => {
    const { handle, registry } = setup();
    const live = liveFork();
    const prompts: string[] = [];
    (live as { client?: unknown }).client = { prompt: async (m: string) => void prompts.push(m) };
    registry.set("agent-7", live);
    handleForkRaisedQuestion(handle, registry, "agent-7", "Should I rebase?");
    assert.equal(live.threadBound, true);

    await sendToAgent(registry, { cwd: "/repo", leadSend: true }, "agent-7", "yes, rebase");

    assert.equal(live.threadBound, false, "the lead took over the exchange — the fork rejoins the fan-in immediately");
    assert.deepEqual(prompts, ["yes, rebase"]);
  });

  test("I2: the OWNER's own overlay message (no leadSend) leaves the bind exactly as it is", async () => {
    const { handle, registry } = setup();
    const live = liveFork();
    (live as { client?: unknown }).client = { prompt: async () => {} };
    registry.set("agent-7", live);
    handleForkRaisedQuestion(handle, registry, "agent-7", "Should I rebase?");

    await sendToAgent(registry, { cwd: "/repo" }, "agent-7", "what are the options?");

    assert.equal(live.threadBound, true, "ask.ts's overlay channel must never unbind the thread it is driving");
  });

  test("I2: a lead stop is a close path too — stopAgent releases the bind", async () => {
    const { handle, registry } = setup();
    const live = liveFork();
    (live as { client?: unknown }).client = { abort: async () => {}, stop: async () => {} };
    registry.set("agent-7", live);
    handleForkRaisedQuestion(handle, registry, "agent-7", "Should I rebase?");

    await stopAgent(registry, "agent-7", undefined, { silent: true });

    assert.equal(live.threadBound, false, "a stopped agent must not carry a latched bind into a later revival");
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

/**
 * Review relay #1 (test-Important): `registerAsk`'s two tool bodies and
 * `injectDiscussionSummary` need no live session — neither spawns anything
 * (§1: "registers only, NO spawn") — so both are driven here against a fake
 * `pi` + duck-typed `toolCtx`, exactly the `createApprovalRelay` convention
 * in test/execute-gateway.test.ts.
 */
describe("registerAsk (fake pi)", () => {
  interface FakeTool {
    name: string;
    execute(
      id: string,
      params: unknown,
      signal: unknown,
      onUpdate: unknown,
      toolCtx: unknown,
    ): Promise<{ content: Array<{ type: string; text: string }> }>;
  }

  function setup() {
    const tools = new Map<string, FakeTool>();
    const pi = { registerTool: (t: FakeTool) => tools.set(t.name, t) } as unknown as ExtensionAPI;
    const handle = createThreadRegistryHandle();
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "session.jsonl.ws-threads.json");
    hydrateThreadRegistry(handle, path);
    // Review relay #1 (I6): the shared registry is threaded in, as `index.ts`
    // does. Without it `ws-resolve`'s `threadBound` release was only ever
    // exercised in its always-false no-op-guard form.
    const rpcRegistry: RpcAgentRegistry = new Map();
    registerAsk(pi, handle, rpcRegistry);
    return { tools, handle, path, rpcRegistry };
  }

  function uiCtx(mode: string) {
    const notices: Array<{ message: string; type?: string }> = [];
    return {
      ctx: {
        mode,
        ui: {
          notify: (message: string, type?: string) => notices.push({ message, type }),
        },
      },
      notices,
    };
  }

  /** Installs a counting spy on the merged live-agent widget refresh; returns a reader for the current count. */
  function spyOnAgentWidgetRefresh(): () => number {
    let calls = 0;
    agentWidgetRefreshRef.current = () => {
      calls += 1;
    };
    return () => calls;
  }

  async function callAsk(tools: Map<string, FakeTool>, params: unknown, ctx: unknown) {
    const res = await tools.get(ASK_TOOL_NAME)!.execute("call-1", params, undefined, undefined, ctx);
    return JSON.parse(res.content[0].text) as { question_id: string };
  }

  test("registers both tools declaratively (so a fork's exclusion set has them to exclude)", () => {
    const { tools } = setup();
    assert.deepEqual([...tools.keys()].sort(), [ASK_TOOL_NAME, RESOLVE_TOOL_NAME].sort());
  });

  test("tui: returns {question_id}, stores the thread, and fires the merged live-agent widget refresh", async () => {
    const { tools, handle } = setup();
    const ui = uiCtx("tui");
    const calls = spyOnAgentWidgetRefresh();
    const out = await callAsk(tools, { title: "rebase or merge?", question: "Which?", context: "short" }, ui.ctx);

    assert.equal(out.question_id, "q1");
    const record = handle.threads.get("q1")!;
    assert.equal(record.title, "rebase or merge?");
    assert.equal(record.question, "Which?");
    assert.equal(record.context, "short");
    assert.equal(record.status, "pending");
    assert.equal(record.origin, "lead-ask", "C2: ws-ask owns its (lazily spawned) discussion fork, so /done may stop it");
    assert.equal(calls(), 1, "§8: the merged refresh is the TUI branch's own signal");
    assert.deepEqual(ui.notices, [], "the TUI branch must not also fire the headless notify");
  });

  test("headless: notifies instead of refreshing the widget (§8 baseline)", async () => {
    const { tools } = setup();
    const ui = uiCtx("print");
    const calls = spyOnAgentWidgetRefresh();
    await callAsk(tools, { title: "rebase or merge?", question: "Which?" }, ui.ctx);

    assert.equal(calls(), 0, "no widget refresh outside tui");
    assert.equal(ui.notices.length, 1);
    assert.equal(ui.notices[0].type, "info");
    assert.match(ui.notices[0].message, /q1/);
  });

  test("wires entryId from toolCtx's own leaf id (§7 anchor)", async () => {
    const { tools, handle } = setup();
    const ui = uiCtx("tui");
    await callAsk(tools, { title: "t", question: "q" }, { ...ui.ctx, sessionManager: { getLeafId: () => "entry-42" } });
    assert.equal(handle.threads.get("q1")!.entryId, "entry-42");
  });

  test("§7 bound: an over-long context warns but is stored unchanged", async () => {
    const { tools, handle } = setup();
    const ui = uiCtx("tui");
    const long = "x".repeat(MAX_CONTEXT_CHARS + 1);
    await callAsk(tools, { title: "t", question: "q", context: long }, ui.ctx);

    assert.equal(handle.threads.get("q1")!.context, long, "the lead's context is never truncated");
    assert.equal(ui.notices.length, 1);
    assert.equal(ui.notices[0].type, "warning");
    assert.match(ui.notices[0].message, new RegExp(String(long.length)));
  });

  test("a context inside the bound warns about nothing", async () => {
    const { tools } = setup();
    const ui = uiCtx("tui");
    await callAsk(tools, { title: "t", question: "q", context: "x".repeat(MAX_CONTEXT_CHARS) }, ui.ctx);
    assert.deepEqual(ui.notices, []);
  });

  test("persists on register, and again on ws-resolve's close transition", async () => {
    const { tools, handle, path } = setup();
    const ui = uiCtx("tui");
    const calls = spyOnAgentWidgetRefresh();
    await callAsk(tools, { title: "t", question: "q" }, ui.ctx);
    assert.equal(loadThreadRegistryFile(path)[0]?.status, "pending", "restart survival: pending is on disk before any answer");

    const res = await tools.get(RESOLVE_TOOL_NAME)!.execute("call-2", { question_id: "q1" }, undefined, undefined, ui.ctx);
    assert.deepEqual(JSON.parse(res.content[0].text), { question_id: "q1", status: "closed" });
    assert.equal(handle.threads.get("q1")!.status, "closed");
    assert.equal(loadThreadRegistryFile(path)[0]?.status, "closed");
    assert.equal(calls(), 2, "the merged refresh fires again on ws-resolve's close transition");
  });

  test("I6: ws-resolve releases the respondent's threadBound on a REAL registry, not just the no-op guard", async () => {
    const { tools, handle, rpcRegistry } = setup();
    const ui = uiCtx("tui");
    await callAsk(tools, { title: "t", question: "q" }, ui.ctx);
    handle.threads.get("q1")!.respondentAgentId = "agent-7";
    const respondent = {
      agentId: "agent-7",
      sessionPath: "/tmp/s.jsonl",
      systemPromptPath: "/tmp/p.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      streaming: false,
      running: true,
      reportLog: [],
      threadBound: true,
      client: {},
    } as unknown as RpcAgentRecord;
    rpcRegistry.set("agent-7", respondent);

    await tools.get(RESOLVE_TOOL_NAME)!.execute("call-2", { question_id: "q1" }, undefined, undefined, ui.ctx);

    assert.equal(respondent.threadBound, false, "the thread is closed for good — the fork rejoins the lead's fan-in");
    assert.equal(handle.threads.get("q1")!.status, "closed");
  });

  test("I6: ws-resolve on a never-opened thread (no respondent yet) is still just a close", async () => {
    const { tools, handle, rpcRegistry } = setup();
    const ui = uiCtx("tui");
    await callAsk(tools, { title: "t", question: "q" }, ui.ctx);
    await tools.get(RESOLVE_TOOL_NAME)!.execute("call-2", { question_id: "q1" }, undefined, undefined, ui.ctx);
    assert.equal(handle.threads.get("q1")!.status, "closed");
    assert.equal(rpcRegistry.size, 0);
  });

  test("ws-resolve throws on an unknown question_id rather than silently closing nothing", async () => {
    const { tools } = setup();
    await assert.rejects(
      () => tools.get(RESOLVE_TOOL_NAME)!.execute("call-2", { question_id: "nope" }, undefined, undefined, uiCtx("tui").ctx),
      /unknown question_id "nope"/,
    );
  });
});

describe("closeThreadOnDone / injectDiscussionSummary (fake pi)", () => {
  function setup(origin: "lead-ask" | "fork-raised" = "lead-ask") {
    const sent: Array<{ message: unknown; options: unknown }> = [];
    const pi = { sendMessage: (message: unknown, options: unknown) => sent.push({ message, options }) } as unknown as ExtensionAPI;
    const handle = createThreadRegistryHandle();
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "session.jsonl.ws-threads.json");
    hydrateThreadRegistry(handle, path);
    const record = thread({ threadId: "q1", question: "Which anchor?", context: "background", origin });
    handle.threads.set(record.threadId, record);
    return { pi, sent, handle, path, record };
  }

  /** A live respondent on the shared registry, with a stop-observing fake client. */
  function liveRespondent(stops: string[]): RpcAgentRecord {
    return {
      agentId: "agent-7",
      sessionPath: "/tmp/s.jsonl",
      systemPromptPath: "/tmp/p.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      spawnRole: "fork",
      streaming: false,
      running: false,
      reportLog: [],
      overlayAttached: true,
      threadBound: true,
      client: {
        abort: async () => {},
        stop: async () => {
          stops.push("agent-7");
        },
      },
    } as unknown as RpcAgentRecord;
  }

  test("§6: one custom message, delivered via followUp and triggering a turn, carrying the thread id", () => {
    const { pi, sent, handle, record } = setup();
    injectDiscussionSummary(pi, handle, new Map(), record, "we take the second anchor");

    assert.equal(sent.length, 1);
    const msg = sent[0].message as { customType: string; content: string; display: boolean; details: { threadId: string; title: string } };
    assert.equal(msg.customType, "ws-thread-summary");
    assert.equal(msg.display, true);
    assert.equal(msg.details.threadId, "q1");
    assert.equal(msg.details.title, record.title);
    assert.ok(msg.content.includes("we take the second anchor"));
    assert.deepEqual(
      sent[0].options,
      { deliverAs: "followUp", triggerTurn: true },
      "never steer (§6 requires the lead's own turn boundary), and triggerTurn so an idle lead acts on the decision instead of queueing it",
    );
  });

  test("260906 (Phase 1): held while a compaction is in flight, delivered once released — the thread-close side effects run immediately regardless", () => {
    leadCompactingRef.current = true;
    const { pi, sent, handle, path, record } = setup();
    injectDiscussionSummary(pi, handle, new Map(), record, "we take the second anchor");

    assert.deepEqual(sent, [], "the outbound ws-thread-summary message is held while compacting");
    assert.equal(heldPushQueue.length, 1);
    // Per the recommended ordering: the dormant transition and persistence
    // are NOT part of the race being fixed, so they run immediately either way.
    assert.equal(record.status, "dormant");
    assert.equal(loadThreadRegistryFile(path)[0]?.status, "dormant");

    leadCompactingRef.current = false;
    assert.equal(flushHeldPushes(pi), 1);
    assert.equal(sent.length, 1, "delivered once released");
    const msg = sent[0].message as { customType: string; content: string; details: { threadId: string } };
    assert.equal(msg.customType, "ws-thread-summary");
    assert.equal(msg.details.threadId, "q1");
    assert.ok(msg.content.includes("we take the second anchor"));
    assert.deepEqual(sent[0].options, { deliverAs: "followUp", triggerTurn: true }, "the recorded delivery mode survives the hold");
  });

  test("260906 (Phase 1): not held when the lead is idle and no compaction is in flight (baseline behavior unchanged)", () => {
    leadCompactingRef.current = false;
    const { pi, sent, handle, record } = setup();
    injectDiscussionSummary(pi, handle, new Map(), record, "decided");
    assert.equal(sent.length, 1, "sent immediately — the ordinary path");
    assert.deepEqual(heldPushQueue, []);
  });

  test("§9: the thread goes dormant (retained, not deleted) and is persisted", () => {
    const { pi, handle, path, record } = setup();
    injectDiscussionSummary(pi, handle, new Map(), record, "decided");

    assert.equal(record.status, "dormant");
    assert.ok(handle.threads.has("q1"), "dormant means retained and reopenable");
    assert.equal(loadThreadRegistryFile(path)[0]?.status, "dormant");
  });

  test("I5: snapshots the resume fields BEFORE stopping the respondent, and clears the overlay flag", async () => {
    const { pi, handle, record } = setup();
    record.respondentAgentId = "agent-7";
    const stops: string[] = [];
    const live = liveRespondent(stops);
    const registry: RpcAgentRegistry = new Map([["agent-7", live]]);

    injectDiscussionSummary(pi, handle, registry, record, "decided");

    assert.equal(record.forkResume?.sessionPath, "/tmp/s.jsonl", "captured while the record was still live");
    assert.equal(live.overlayAttached, false);
    assert.equal(live.threadBound, false, "I5: the thread itself closed here, so the thread-lifetime bind is released too");
    await new Promise((resolve) => setImmediate(resolve));
    assert.deepEqual(stops, ["agent-7"], "260903 ws-agent-stop semantics: the child process is actually stopped");
    assert.ok(registry.has("agent-7"), "stopAgent retains the entry — the thread stays rehydratable");
  });

  test("a respondent missing from the registry does not throw or block the summary", () => {
    const { pi, sent, handle, record } = setup();
    record.respondentAgentId = "gone";
    injectDiscussionSummary(pi, handle, new Map(), record, "decided");
    assert.equal(sent.length, 1);
    assert.equal(record.status, "dormant");
  });

  test("C2: a lead-ask thread routes through the full §6/§9 close (summary injected, respondent stopped)", async () => {
    const { pi, sent, handle, record } = setup("lead-ask");
    record.respondentAgentId = "agent-7";
    const stops: string[] = [];
    const registry: RpcAgentRegistry = new Map([["agent-7", liveRespondent(stops)]]);

    closeThreadOnDone(pi, handle, registry, record, "we take the second anchor");

    assert.equal(sent.length, 1, "this surface owns the discussion fork, so its summary is the lead's channel");
    assert.equal(record.status, "dormant");
    await new Promise((resolve) => setImmediate(resolve));
    assert.deepEqual(stops, ["agent-7"]);
  });

  test("C2: a fork-raised thread only detaches — no summary injected and the live task fork keeps running", async () => {
    const { pi, sent, handle, path, record } = setup("fork-raised");
    record.respondentAgentId = "agent-7";
    const stops: string[] = [];
    const live = liveRespondent(stops);
    const registry: RpcAgentRegistry = new Map([["agent-7", live]]);

    closeThreadOnDone(pi, handle, registry, record, "");

    assert.deepEqual(sent, [], "§1: the lead is not part of a fork-raised exchange — it learns the outcome from the fork's final report");
    await new Promise((resolve) => setImmediate(resolve));
    assert.deepEqual(stops, [], "stopping a live task fork would destroy the in-flight task the lead is still expecting a pushed final from");
    assert.ok(live.client, "the fork's client is untouched");
    assert.equal(live.overlayAttached, false, "the overlay is detached, so the anti-bleed loop is armed again");
    assert.equal(live.threadBound, false, "review relay #1 (I5): the fork rejoins the lead's fan-in and its settles are audible again");
    assert.equal(record.status, "dormant", "dormant means reopenable while the fork lives");
    assert.ok(handle.threads.has("q1"));
    assert.equal(loadThreadRegistryFile(path)[0]?.status, "dormant", "the detach is persisted like every other transition");
    assert.equal(record.forkResume?.sessionPath, "/tmp/s.jsonl", "the resume snapshot is refreshed on detach");
  });

  test("C2: detaching a fork-raised thread whose respondent is gone is a no-op, not a throw", () => {
    const { pi, sent, handle, record } = setup("fork-raised");
    record.respondentAgentId = "gone";
    closeThreadOnDone(pi, handle, new Map(), record, "");
    assert.deepEqual(sent, []);
    assert.equal(record.status, "dormant");
  });

  describe("handleRespondentFinalReport (the fork ends the thread itself)", () => {
    /** An overlay stub whose `closeWithSummary` does what the real component does: fire `onDone` (= closeThreadOnDone) with the text. */
    function overlayStub(onDone: (summary: string) => void) {
      const calls: { close: number; summaries: string[] } = { close: 0, summaries: [] };
      const handle: OverlayHandle = {
        close: () => {
          calls.close += 1;
        },
        closeWithSummary: (summary) => {
          calls.summaries.push(summary);
          onDone(summary);
        },
      };
      return { handle, calls };
    }

    test("lead-ask, no overlay attached (owner pressed Esc): injects the report as the summary, stops the fork, goes dormant", async () => {
      const { pi, sent, handle, path, record } = setup("lead-ask");
      record.status = "open";
      record.respondentAgentId = "agent-7";
      const stops: string[] = [];
      const live = liveRespondent(stops);
      const registry: RpcAgentRegistry = new Map([["agent-7", live]]);

      handleRespondentFinalReport(pi, handle, registry, record, "Decided: merge, keep both histories.", undefined);

      assert.equal(sent.length, 1, "no summary turn is requested — the report text is the summary");
      const msg = sent[0].message as { content: string };
      assert.ok(msg.content.includes("Decided: merge, keep both histories."));
      assert.equal(record.status, "dormant");
      assert.equal(live.overlayAttached, false);
      assert.equal(loadThreadRegistryFile(path)[0]?.status, "dormant");
      await new Promise((resolve) => setImmediate(resolve));
      assert.deepEqual(stops, ["agent-7"]);
    });

    test("lead-ask with the overlay attached: the overlay is closed with the report text, and that close runs the same /done path", async () => {
      const { pi, sent, handle, record } = setup("lead-ask");
      record.status = "open";
      record.respondentAgentId = "agent-7";
      const stops: string[] = [];
      const registry: RpcAgentRegistry = new Map([["agent-7", liveRespondent(stops)]]);
      const overlay = overlayStub((summary) => closeThreadOnDone(pi, handle, registry, record, summary));

      handleRespondentFinalReport(pi, handle, registry, record, "We go with the second anchor.", overlay.handle);

      assert.deepEqual(overlay.calls.summaries, ["We go with the second anchor."]);
      assert.equal(overlay.calls.close, 0, "closed through closeWithSummary, never the bare close");
      assert.equal(sent.length, 1);
      assert.equal(record.status, "dormant");
      await new Promise((resolve) => setImmediate(resolve));
      assert.deepEqual(stops, ["agent-7"]);
    });

    test("fork-raised with the overlay attached: closes the overlay and detaches only — no injection, no stop", async () => {
      const { pi, sent, handle, record } = setup("fork-raised");
      record.status = "open";
      record.respondentAgentId = "agent-7";
      const stops: string[] = [];
      const live = liveRespondent(stops);
      const registry: RpcAgentRegistry = new Map([["agent-7", live]]);
      const overlay = overlayStub((summary) => closeThreadOnDone(pi, handle, registry, record, summary));

      handleRespondentFinalReport(pi, handle, registry, record, "Task done. Decisions: rebase.", overlay.handle);

      assert.deepEqual(overlay.calls.summaries, [""], "a task fork's final is not a thread summary");
      assert.deepEqual(sent, [], "the lead reads the fork's own final report; nothing is injected");
      assert.equal(record.status, "dormant");
      assert.equal(live.overlayAttached, false);
      await new Promise((resolve) => setImmediate(resolve));
      assert.deepEqual(stops, []);
    });

    test("260905: fork-raised with no overlay attached still detaches the thread and releases the bind", () => {
      const { pi, sent, handle, record } = setup("fork-raised");
      record.status = "open";
      record.respondentAgentId = "agent-7";
      const live = liveRespondent([]);
      const consumed = handleRespondentFinalReport(pi, handle, new Map([["agent-7", live]]), record, "Task done.", undefined);
      assert.equal(consumed, false, "a fork-raised final IS the completion signal — it must still be pushed to the lead");
      assert.deepEqual(sent, [], "nothing is injected; the lead reads the pushed report itself");
      assert.equal(record.status, "dormant", "the thread that the question opened is over");
      assert.equal(live.threadBound, false, "the fork rejoins the lead's fan-in on the very report that ends the thread");
    });

    test("260905 suppression contract: a lead-ask final returns true (consumed), a fork-raised final returns false", () => {
      const leadAsk = setup("lead-ask");
      leadAsk.record.status = "open";
      assert.equal(
        handleRespondentFinalReport(leadAsk.pi, leadAsk.handle, new Map(), leadAsk.record, "Decided.", undefined),
        true,
        "the decision already reaches the lead as the ws-thread-summary message; pushing the raw report too would duplicate it",
      );

      const forkRaised = setup("fork-raised");
      forkRaised.record.status = "open";
      assert.equal(handleRespondentFinalReport(forkRaised.pi, forkRaised.handle, new Map(), forkRaised.record, "Task done.", undefined), false);
    });

    test("260905: a final on a non-open thread returns false — nothing was consumed", () => {
      const { pi, handle, record } = setup("lead-ask");
      record.status = "dormant";
      assert.equal(handleRespondentFinalReport(pi, handle, new Map(), record, "late", undefined), false);
    });

    test("a final report on a thread that is not open (pending, dormant, closed) is ignored — no duplicate injection", () => {
      for (const status of ["pending", "dormant", "closed"] as const) {
        const { pi, sent, handle, record } = setup("lead-ask");
        record.status = status;
        const overlay = overlayStub(() => {});
        handleRespondentFinalReport(pi, handle, new Map(), record, "late", overlay.handle);
        assert.deepEqual(sent, [], status);
        assert.deepEqual(overlay.calls.summaries, [], status);
        assert.equal(record.status, status);
      }
    });
  });
});

describe("checkContextLength / buildForkQuestionLeadNotice", () => {
  test("returns undefined at or below the bound, a warning above it", () => {
    assert.equal(checkContextLength(undefined), undefined);
    assert.equal(checkContextLength(""), undefined);
    assert.equal(checkContextLength("x".repeat(MAX_CONTEXT_CHARS)), undefined);
    const warn = checkContextLength("x".repeat(MAX_CONTEXT_CHARS + 5));
    assert.ok(warn && warn.includes(String(MAX_CONTEXT_CHARS + 5)) && warn.includes(String(MAX_CONTEXT_CHARS)));
  });

  test("I6 notice names the thread and the owner channel, and tells the lead to end its turn (never to poll a deleted wait verb)", () => {
    const notice = buildForkQuestionLeadNotice("agent-7", "q3");
    assert.ok(notice.includes("agent-7"));
    assert.ok(notice.includes("q3"));
    assert.match(notice, /\/answer q3/);
    assert.match(notice, /end your turn/i);
    assert.ok(!/ws-agent-wait/i.test(notice), "ws-agent-wait is deleted — the notice must not send the lead to a tool that no longer exists");
    assert.match(notice, /do not relay/i);
    // C2: the decision comes back on the fork's own final report, not as a
    // thread-summary message — /done never injects one for this origin.
    assert.match(notice, /Decisions:/);
    assert.ok(!/thread-summary/i.test(notice), notice);
  });
});

/**
 * Review relay #1, test partition C5: the ticket names "`threadBound` is set
 * on a thread reopen as well as first open" as a verify item, and
 * `ensureRespondent` is where every open path sets it (`openThread` calls it
 * unconditionally). Only the fresh-discussion-fork branch needs a subprocess;
 * the already-live and rehydrate-from-`forkResume` branches — first open and
 * reopen respectively — are driven directly here.
 */
describe("ensureRespondent (threadBound on open and on reopen)", () => {
  const bridge = { wsToolNames: [], defaultSessionKeyRef: { current: undefined } } as never;
  const sessionCtx = { cwd: "/repo" };

  function askCtx() {
    const notices: Array<{ message: string; type?: string }> = [];
    return {
      ctx: { mode: "tui", ui: { notify: (message: string, type?: string) => notices.push({ message, type }) } } as never,
      notices,
    };
  }

  function openThreadRecord(): ThreadRecord {
    return thread({ threadId: "q1", status: "open", origin: "fork-raised", respondentAgentId: "agent-7", question: "Which anchor?" });
  }

  test("first open of an already-live respondent binds the thread and arms the final-report hook", async () => {
    const handle = createThreadRegistryHandle();
    const record = openThreadRecord();
    handle.threads.set(record.threadId, record);
    const live = {
      agentId: "agent-7",
      sessionPath: "/tmp/s.jsonl",
      systemPromptPath: "/tmp/p.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      streaming: false,
      running: true,
      reportLog: [],
      client: {},
    } as unknown as RpcAgentRecord;
    const registry: RpcAgentRegistry = new Map([["agent-7", live]]);
    const ui = askCtx();

    const agentId = await ensureRespondent({} as never, ui.ctx, bridge, registry, handle, record, sessionCtx);

    assert.equal(agentId, "agent-7");
    assert.equal(live.threadBound, true);
    assert.equal(typeof live.onFinalReport, "function", "the respondent can end its own thread");
  });

  test("REOPEN after a lead restart rehydrates the record from forkResume and binds it again", async () => {
    const handle = createThreadRegistryHandle();
    const record = openThreadRecord();
    record.forkResume = {
      sessionPath: "/tmp/s.jsonl",
      systemPromptPath: "/tmp/p.md",
      wsToolNames: [],
      toolGroup: "full-worker",
    } as never;
    handle.threads.set(record.threadId, record);
    // The in-memory registry is empty — exactly the post-restart state.
    const registry: RpcAgentRegistry = new Map();
    const ui = askCtx();

    const agentId = await ensureRespondent({} as never, ui.ctx, bridge, registry, handle, record, sessionCtx);

    assert.equal(agentId, "agent-7");
    const revived = registry.get("agent-7")!;
    assert.equal(revived.threadBound, true, "a reopen binds just like a first open");
    assert.equal(revived.client, undefined, "still dormant — the relaunch happens on the owner's first message");
    assert.equal(typeof revived.onFinalReport, "function");
  });

  test("a second open of the same live respondent re-binds rather than leaving a stale unbound record", async () => {
    const handle = createThreadRegistryHandle();
    const record = openThreadRecord();
    handle.threads.set(record.threadId, record);
    const live = {
      agentId: "agent-7",
      sessionPath: "/tmp/s.jsonl",
      systemPromptPath: "/tmp/p.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      streaming: false,
      running: true,
      reportLog: [],
      client: {},
      // The state a /done left behind.
      threadBound: false,
    } as unknown as RpcAgentRecord;
    const registry: RpcAgentRegistry = new Map([["agent-7", live]]);

    await ensureRespondent({} as never, askCtx().ctx, bridge, registry, handle, record, sessionCtx);

    assert.equal(live.threadBound, true);
  });

  test("a respondent that can no longer be resumed notifies the owner and returns undefined (no bind)", async () => {
    const handle = createThreadRegistryHandle();
    const record = openThreadRecord();
    handle.threads.set(record.threadId, record);
    const ui = askCtx();

    const agentId = await ensureRespondent({} as never, ui.ctx, bridge, new Map(), handle, record, sessionCtx);

    assert.equal(agentId, undefined);
    assert.equal(ui.notices.length, 1);
    assert.equal(ui.notices[0].type, "error");
  });
});

import { test, describe, afterEach } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, readFileSync, chmodSync, readdirSync, renameSync } from "node:fs";
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
  buildQueuedAnswerInjectionMessage,
  captureAskCommitHash,
  buildAskAnchorLine,
  captureForkResume,
  rehydrateForkRecord,
  getLeafEntryId,
  normalizeThreadOrigin,
  checkContextLength,
  buildForkQuestionLeadNotice,
  MAX_CONTEXT_CHARS,
  resolveChildLiveness,
  buildThreadHeaderHint,
  formatSpawnTime,
  type ThreadRecord,
  type PersistedForkResume,
} from "../src/ask.ts";
import {
  agentWidgetRefreshRef,
  heldPushQueue,
  leadCompactingRef,
  leadWakeStartPendingRef,
  type RpcAgentRecord,
  type RpcAgentRegistry,
} from "../src/spawner.ts";
import { allocateAgentHome, createAgentStorageContext } from "../src/agent-storage.ts";
import { rmSync } from "node:fs";
import { thread } from "./fixtures/ask-threads.ts";

// 260905 (live-agent widget ticket): every widget-refresh call site in
// ask.ts now fires through spawner.ts's module-level `agentWidgetRefreshRef`
// instead of a locally-passed ctx/handle — reset it after every test so a
// spy installed by one test can never leak into the next.
//
// 260906 (compaction push-hold ticket, Phase 1): `leadCompactingRef` and
// `heldPushQueue` are the same module state `spawner.ts`'s own hold uses —
// reset both for the same leak-proofing reason.
//
// 260906 Phase 1 review relay #1 (Important #2): `leadWakeStartPendingRef`
// is the goal-loop's boundary guard, now also read by `injectDiscussionSummary`
// via `composeLeadTurnStartOptions` — reset for the same leak-proofing reason.
afterEach(() => {
  agentWidgetRefreshRef.current = undefined;
  leadCompactingRef.current = false;
  heldPushQueue.length = 0;
  leadWakeStartPendingRef.current = false;
});

describe("tool names", () => {
  test("260911: renamed from ws-ask/ws-resolve; fork role handlers refuse execution instead of hiding schemas", () => {
    assert.equal(ASK_TOOL_NAME, "ws-queue-question");
    assert.equal(RESOLVE_TOOL_NAME, "ws-withdraw-question");
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

  test("legacy thread-local transcripts are discarded on hydration so the child record/session is the only conversation source", () => {
    const original = {
      ...thread({ threadId: "q1", status: "open" }),
      transcript: [{ kind: "user", text: "legacy owner text" }],
    } as ThreadRecord & { transcript: unknown[] };
    const [parsed] = parseThreadRegistry(serializeThreadRegistry([original]));
    assert.ok(!("transcript" in parsed));
    assert.equal(parsed.threadId, "q1");
  });

  describe("resolveChildLiveness", () => {
    test("running wins; an owner last-writer makes an idle child await the owner", () => {
      assert.equal(resolveChildLiveness(true, true), "running");
      assert.equal(resolveChildLiveness(false, true), "idle-awaiting-owner");
      assert.equal(resolveChildLiveness(false, false), "settled");
    });
  });

  test("buildThreadHeaderHint keeps only compact metadata and controls; the title/question is absent", () => {
    const hint = buildThreadHeaderHint(thread({ title: "Which color do you prefer?" }));
    assert.equal(
      hint,
      "ws thread q1 · opened 2026-09-05 10:00 UTC\nEsc: close view (thread stays open) · /done: end thread",
    );
    assert.ok(!hint.includes("Which color"));
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

  test("a crash during the registry write leaves the previous registry intact and no temp file", () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "session.jsonl.ws-threads.json");
    const previous = [thread({ threadId: "q1", status: "open" })];
    saveThreadRegistryFile(path, previous);
    let renames = 0;
    saveThreadRegistryFile(path, [...previous, thread({ threadId: "q2" })], {
      rename: source => {
        renames += 1;
        assert.match(source, /\.tmp$/, "the replacement is staged in a temp file");
        assert.deepEqual(loadThreadRegistryFile(path), previous, "the target is untouched until the rename");
        throw Object.assign(new Error("simulated crash"), { code: "EIO" });
      },
    });
    assert.equal(renames, 1);
    assert.deepEqual(loadThreadRegistryFile(path), previous);
    assert.deepEqual(readdirSync(dir), ["session.jsonl.ws-threads.json"]);
  });

  test("the registry replacement retries a Windows sharing violation", () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "threads.json");
    const next = [thread({ threadId: "q2", status: "dormant" })];
    saveThreadRegistryFile(path, [thread({ threadId: "q1" })]);
    const delays: number[] = [];
    let renames = 0;
    saveThreadRegistryFile(path, next, {
      platform: "win32",
      sleep: milliseconds => delays.push(milliseconds),
      rename: (source, destination) => {
        if (++renames === 1) throw Object.assign(new Error("EBUSY"), { code: "EBUSY" });
        renameSync(source, destination);
      },
    });
    assert.equal(renames, 2);
    assert.deepEqual(delays, [10]);
    assert.deepEqual(loadThreadRegistryFile(path), next);
    assert.deepEqual(readdirSync(dir), ["threads.json"]);
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

  // 260911: no fork-less lead-ask answer VIEW can survive a lead-process
  // restart (there is no live overlay to reattach to), so `hydrateThreadRegistry`
  // normalizes a persisted "open" lead-ask thread on load. These drive that
  // normalization through a REAL saveThreadRegistryFile -> hydrateThreadRegistry
  // round-trip (not a record seeded directly into the in-memory map), since
  // that persisted-restart path is the actual guarantee being made.
  test("a persisted lead-ask/open thread with no deferred withdrawal reverts to pending on restart", () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "threads.json");
    saveThreadRegistryFile(path, [thread({ threadId: "q1", origin: "lead-ask", status: "open" })]);
    const handle = createThreadRegistryHandle();
    hydrateThreadRegistry(handle, path);
    const record = handle.threads.get("q1")!;
    assert.equal(record.status, "pending", "still answerable via a fresh /answer");
    assert.equal(record.withdrawnPending, false);
  });

  test("a persisted lead-ask/open thread with a deferred withdrawal finalizes to closed on restart", () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "threads.json");
    saveThreadRegistryFile(path, [thread({ threadId: "q1", origin: "lead-ask", status: "open", withdrawnPending: true })]);
    const handle = createThreadRegistryHandle();
    hydrateThreadRegistry(handle, path);
    const record = handle.threads.get("q1")!;
    assert.equal(record.status, "closed", "no view survives restart to protect from being yanked, so the deferred removal now finalizes");
    assert.equal(record.withdrawnPending, false);
  });

  test("a persisted lead-ask/pending (not open) thread has its status untouched by the restart normalization, though withdrawnPending is still reset (it applies to any lead-ask record, not only \"open\" ones)", () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "threads.json");
    saveThreadRegistryFile(path, [thread({ threadId: "q1", origin: "lead-ask", status: "pending" })]);
    const handle = createThreadRegistryHandle();
    hydrateThreadRegistry(handle, path);
    assert.equal(handle.threads.get("q1")!.status, "pending");
    assert.equal(handle.threads.get("q1")!.withdrawnPending, false);
  });

  test("a persisted fork-raised/open thread is never touched by the lead-ask restart normalization, and gains no stray withdrawnPending field", () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "threads.json");
    saveThreadRegistryFile(path, [thread({ threadId: "q1", origin: "fork-raised", status: "open", respondentAgentId: "agent-7" })]);
    const handle = createThreadRegistryHandle();
    hydrateThreadRegistry(handle, path);
    const record = handle.threads.get("q1")!;
    assert.equal(record.status, "open", "fork-raised is out of 260911's scope — its persisted view-survival contract is unchanged");
    assert.equal(record.withdrawnPending, undefined, "withdrawnPending is a lead-ask-only field; a fork-raised record must not pick up a stray false");
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

describe("addAskToolsIfLead (role-differentiated, never folded into computeLeadActiveTools; 260911 lifts the tool-surface hide from ac998f77/a8cf1183/5f366eff)", () => {
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

  test("the directive names /done and ordinary settled output as the two close paths", () => {
    const directive = buildDiscussionForkDirectiveText();
    assert.ok(directive.includes("/done"));
    assert.ok(!directive.includes("ws-report-to-lead"));
    assert.ok(!directive.includes('kind:"final"'));
    assert.match(directive, /ordinary settled answer/i);
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

describe("buildQueuedAnswerInjectionMessage (260911 D1/D3 fork-less return-path payload)", () => {
  test("carries the answer, context, question and anchor, presented as the owner's own answer", () => {
    const message = buildQueuedAnswerInjectionMessage("the background", "the question", "we picked merge", "Asked at: commit abc123, entry e7");
    assert.ok(message.includes("the background"));
    assert.ok(message.includes("the question"));
    assert.ok(message.includes("we picked merge"));
    assert.ok(message.includes("Asked at: commit abc123, entry e7"));
    assert.match(message, /owner's authority/i);
    assert.match(message, /rather than a new owner turn/i);
  });

  test("omits absent context/question/anchor/excerpt sections rather than emitting empty labels", () => {
    const message = buildQueuedAnswerInjectionMessage(undefined, undefined, "we picked merge");
    assert.ok(!message.includes("Context:"));
    assert.ok(!message.includes("Question:"));
    assert.ok(!message.includes("Asked at:"));
    assert.ok(!message.includes("compacted"));
    assert.ok(message.includes("we picked merge"));
  });

  test("includes the verbatim excerpt, framed as no-longer-live context, when given", () => {
    const message = buildQueuedAnswerInjectionMessage(undefined, "the question", "we picked merge", undefined, "old dialogue verbatim");
    assert.match(message, /no longer in your live context/i);
    assert.ok(message.includes("old dialogue verbatim"));
  });
});

describe("captureAskCommitHash / buildAskAnchorLine (260911 D3 return-path anchor)", () => {
  test("captureAskCommitHash returns the short HEAD hash for a real git checkout", () => {
    const hash = captureAskCommitHash(process.cwd());
    assert.match(hash ?? "", /^[0-9a-f]{4,}$/);
  });

  test("captureAskCommitHash never throws — a non-git cwd degrades to undefined", () => {
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-nogit-"));
    assert.equal(captureAskCommitHash(dir), undefined);
  });

  test("buildAskAnchorLine pairs both halves when present", () => {
    assert.equal(buildAskAnchorLine("abc123", "e7"), "Asked at: commit abc123, entry e7");
  });

  test("buildAskAnchorLine degrades to whichever half is present", () => {
    assert.equal(buildAskAnchorLine("abc123", undefined), "Asked at: commit abc123");
    assert.equal(buildAskAnchorLine(undefined, "e7"), "Asked at: entry e7");
  });

  test("buildAskAnchorLine is undefined when neither half is present", () => {
    assert.equal(buildAskAnchorLine(undefined, undefined), undefined);
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
        reportLog: [{ at: 1 }],
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

  test("last-writer ownership and owner-send attribution survive the thread resume copy", () => {
    const original = { ...live, lastWriter: "owner" as const, ownerSends: [{ text: "owner turn", at: 42 }] };
    const revived = rehydrateForkRecord("agent-owner", captureForkResume(original));
    assert.equal(revived.lastWriter, "owner");
    assert.deepEqual(revived.ownerSends, [{ text: "owner turn", at: 42 }]);
  });

  test("the durable waitingOnChildren mirror round-trips, and an older resume's retired subtreeChannel is ignored", () => {
    const delegation = { version: 1, depth: 1, maxDepth: 2, authority: "lead", tools: ["ws-agent-spawn"] };
    const waiting = { ...live, delegation, waitingOnChildren: true } as unknown as RpcAgentRecord;
    const resume = captureForkResume(waiting);
    assert.equal(resume.waitingOnChildren, true);
    assert.equal("subtreeChannel" in resume, false, "no subtree channel is captured");
    assert.equal(rehydrateForkRecord("agent-1", resume).waitingOnChildren, true, "the waiting state survives a lead restart");

    const older = { ...resume, subtreeChannel: { path: "/tmp/old-home/subtree.json", nonce: "retired" } } as PersistedForkResume;
    const revived = rehydrateForkRecord("agent-1", older);
    assert.equal("subtreeChannel" in revived, false, "the retired field never reaches the record");
    assert.equal(revived.waitingOnChildren, true);
    assert.deepEqual(revived.delegation, rehydrateForkRecord("agent-1", resume).delegation, "the delegation policy parses as it would without the retired field");
    assert.equal(revived.client, undefined, "the record stays dormant; the relaunch binds a fresh control channel");
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

  test("owner-bound fork resume restores valid ownership and falls back safely on a mismatched descriptor", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-ask-ownership-test-"));
    try {
      const ownership = allocateAgentHome(createAgentStorageContext("lead-1", root), "agent-1", "fork");
      writeFileSync(ownership.sessionPath!, "fork session\n");
      const owned = { ...live, sessionPath: ownership.sessionPath, ownership } as unknown as RpcAgentRecord;
      const resume = captureForkResume(owned);
      assert.deepEqual(resume.ownership, ownership);
      assert.deepEqual(rehydrateForkRecord("agent-1", resume).ownership, ownership);

      const bad = { ...resume, ownership: { ...ownership, ownerSessionId: "different" } };
      const fallback = rehydrateForkRecord("agent-1", bad);
      assert.equal(fallback.ownership, undefined);
      assert.equal(fallback.sessionPath, ownership.sessionPath, "invalid ownership cannot erase a usable recorded resume path");
    } finally { rmSync(root, { recursive: true, force: true }); }
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
    // The outcome comes back through the fork's later ordinary settlement,
    // not as a thread-summary message from /done.
    assert.match(notice, /ordinary settled answer/i);
    assert.ok(!/thread-summary/i.test(notice), notice);
  });
});

/** `formatSpawnTime`: moved verbatim from the old, now-deleted per-thread overlay module. */
describe("formatSpawnTime", () => {
  test("formats a valid ISO timestamp as a UTC-labeled minute-resolution string", () => {
    assert.equal(formatSpawnTime("2026-09-05T10:07:33.000Z"), "2026-09-05 10:07 UTC");
  });

  test("returns undefined for a missing or unparseable timestamp", () => {
    assert.equal(formatSpawnTime(undefined), undefined);
    assert.equal(formatSpawnTime("not a date"), undefined);
  });
});

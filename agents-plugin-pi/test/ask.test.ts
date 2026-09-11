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
  buildQueuedAnswerInjectionMessage,
  captureAskCommitHash,
  buildAskAnchorLine,
  withdrawQueuedQuestion,
  deliverQueuedAnswer,
  resolveLeadAskEscapeAction,
  runLeadAskEscapeAction,
  type LeadAskEscapeAction,
  type WithdrawOutcome,
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
  resolveChildLiveness,
  buildInitialConversationItems,
  buildThreadHeaderHint,
  formatSpawnTime,
  buildDoneSummaryPrompt,
  EMPTY_SUMMARY_TEXT,
  summarizeThenClose,
  buildOverlayHandle,
  resolveDoneAction,
  runDoneAction,
  collectLeadAskQueue,
  countQueueAnswered,
  buildQueueCoverageLine,
  buildQueueSubmitConfirmMessage,
  resolveLeadAskQueueEntryAction,
  LeadAskQueueComponent,
  type ThreadRecord,
  type OverlayHandle,
  type DoneAction,
  type FocusableEditorLike,
  type LeadAskQueueOptions,
} from "../src/ask.ts";
import { ConversationViewComponent, type ConversationItem, type ConversationChannel, type ConversationViewTui } from "../src/conversation-view.ts";
import { FORK_EXCLUDED_TOOL_NAMES } from "../src/fork.ts";
import {
  agentWidgetRefreshRef,
  applyRpcEvent,
  computeRunningStatusLine,
  flushHeldPushes,
  heldPushQueue,
  leadCompactingRef,
  leadWakeStartPendingRef,
  REPORT_TO_LEAD_TOOL_NAME,
  sendToAgent,
  stopAgent,
  type RpcAgentRecord,
  type RpcAgentRegistry,
  leadIdleRef, registerPushFlush, clearWakeStart,
} from "../src/spawner.ts";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { allocateAgentHome, createAgentStorageContext } from "../src/agent-storage.ts";
import { rmSync } from "node:fs";

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
  test("260911: renamed from ws-ask/ws-resolve; fork role handlers refuse execution instead of hiding schemas", () => {
    assert.equal(ASK_TOOL_NAME, "ws-queue-question");
    assert.equal(RESOLVE_TOOL_NAME, "ws-withdraw-question");
    assert.equal(FORK_EXCLUDED_TOOL_NAMES.size, 0);
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
        { kind: "note", text: "Rebase or merge?" },
        { kind: "user", text: "merge" },
        { kind: "assistant", text: "Merging keeps both histories." },
        { kind: "tool-call", id: "t1", name: "ws-note", args: { text: "x" } },
        { kind: "tool-result", id: "t1", name: "ws-note", content: "ok", isError: false },
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
      JSON.stringify({
        threads: [{ ...thread({ threadId: "q3" }), transcript: [{ kind: "user", text: "ok" }, { kind: "alien", text: "x" }, { kind: "note" }, null, 7] }],
      }),
    );
    assert.deepEqual(mixed.transcript, [{ kind: "user", text: "ok" }]);
  });

  test("review relay #2 I5: a malformed native tool-call/tool-result is dropped, never poisoning a well-formed neighbor", () => {
    const [record] = parseThreadRegistry(
      JSON.stringify({
        threads: [
          {
            ...thread({ threadId: "q4" }),
            transcript: [
              { kind: "tool-call", name: "ws-read", args: {} }, // missing id
              { kind: "tool-call", id: "c1", name: "ws-read", args: { path: "a.txt" } }, // well-formed
              { kind: "tool-result", id: "c1", name: "ws-read", content: { not: "a string" } }, // non-string content
              { kind: "tool-result", id: "c1", name: "ws-read", content: "ok" }, // well-formed
              { kind: "tool-result", id: "c2" }, // missing name/content
            ],
          },
        ],
      }),
    );
    assert.deepEqual(record.transcript, [
      { kind: "tool-call", id: "c1", name: "ws-read", args: { path: "a.txt" } },
      { kind: "tool-result", id: "c1", name: "ws-read", content: "ok" },
    ]);
  });

  test("legacy {who,text}[] entries hydrate to their ConversationItem.kind equivalents (records written before Phase 2)", () => {
    const [record] = parseThreadRegistry(
      JSON.stringify({
        threads: [
          {
            ...thread({ threadId: "q1" }),
            transcript: [
              { who: "note", text: "Rebase or merge?" },
              { who: "you", text: "merge" },
              { who: "thread", text: "Merging keeps both histories." },
              { who: "alien", text: "dropped" },
            ],
          },
        ],
      }),
    );
    assert.deepEqual(record.transcript, [
      { kind: "note", text: "Rebase or merge?" },
      { kind: "user", text: "merge" },
      { kind: "assistant", text: "Merging keeps both histories." },
    ]);
  });

  test("normalizeTranscript caps at the newest THREAD_TRANSCRIPT_CAP entries", () => {
    const many = Array.from({ length: THREAD_TRANSCRIPT_CAP + 25 }, (_, i) => ({ who: "thread" as const, text: `turn ${i}` }));
    const capped = normalizeTranscript(many)!;
    assert.equal(capped.length, THREAD_TRANSCRIPT_CAP);
    assert.equal((capped[0] as { text: string }).text, "turn 25", "the oldest entries are the ones dropped");
    assert.equal((capped.at(-1) as { text: string }).text, `turn ${THREAD_TRANSCRIPT_CAP + 24}`);
    assert.equal(normalizeTranscript(undefined), undefined);
    assert.equal(normalizeTranscript({}), undefined);
  });

  describe("resolveChildLiveness", () => {
    test("streaming -> running, not streaming -> settled", () => {
      assert.equal(resolveChildLiveness(true), "running");
      assert.equal(resolveChildLiveness(false), "settled");
    });
  });

  describe("buildInitialConversationItems", () => {
    test("the original question is the first assistant dialogue turn even when it equals the metadata title", () => {
      assert.deepEqual(buildInitialConversationItems({ transcript: [], question: "Rebase or merge?", title: "Rebase or merge?" }), [
        { kind: "assistant", text: "**Question:** Rebase or merge?" },
      ]);
    });

    test("an empty or absent transcript trims and seeds the question once", () => {
      assert.deepEqual(buildInitialConversationItems({ transcript: undefined, question: "  Rebase or merge?  " }), [
        { kind: "assistant", text: "**Question:** Rebase or merge?" },
      ]);
    });

    test("repairs an existing history that omitted the question without discarding later turns", () => {
      const items: ConversationItem[] = [
        { kind: "user", text: "What about blue?" },
        { kind: "assistant", text: "Blue is also available." },
      ];
      assert.deepEqual(buildInitialConversationItems({ transcript: items, question: "Which color do you prefer?" }), [
        { kind: "assistant", text: "**Question:** Which color do you prefer?" },
        ...items,
      ]);
    });

    test("upgrades both legacy leading note forms and does not duplicate the question on later reopen", () => {
      const later: ConversationItem[] = [{ kind: "user", text: "Blue." }];
      for (const noteText of ["Which color?", "Question: Which color?"]) {
        const upgraded = buildInitialConversationItems({
          transcript: [{ kind: "note", text: noteText }, ...later],
          question: "Which color?",
        });
        assert.deepEqual(upgraded, [{ kind: "assistant", text: "**Question:** Which color?" }, ...later]);
        assert.deepEqual(buildInitialConversationItems({ transcript: upgraded, question: "Which color?" }), upgraded);
      }
    });

    test("an absent question leaves genuine history unchanged", () => {
      const items: ConversationItem[] = [{ kind: "note", text: "already open" }];
      assert.equal(buildInitialConversationItems({ transcript: items, question: undefined }), items);
      assert.deepEqual(buildInitialConversationItems({ transcript: undefined, question: undefined }), []);
      assert.deepEqual(buildInitialConversationItems({ transcript: [], question: undefined }), []);
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
    assert.deepEqual(JSON.parse(res.content[0].text), { question_id: "q1", status: "closed", outcome: "removed" });
    assert.equal(handle.threads.get("q1")!.status, "closed");
    assert.equal(loadThreadRegistryFile(path)[0]?.status, "closed");
    assert.equal(calls(), 2, "the merged refresh fires again on ws-resolve's close transition");
  });

  test("I6: ws-resolve releases the respondent's threadBound on a REAL registry, not just the no-op guard (fork-raised — 260911's withdrawal concurrency contract is lead-ask-only; fork-raised keeps this unconditional-close/release behavior verbatim)", async () => {
    const { tools, handle, rpcRegistry } = setup();
    const ui = uiCtx("tui");
    await callAsk(tools, { title: "t", question: "q" }, ui.ctx);
    handle.threads.get("q1")!.origin = "fork-raised";
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

describe("withdrawQueuedQuestion (260911 ws-withdraw-question's concurrency contract)", () => {
  function handleWith(record: ThreadRecord) {
    const handle = createThreadRegistryHandle();
    handle.threads.set(record.threadId, record);
    return handle;
  }

  test("a pending (unopened) lead-ask thread is removed immediately", () => {
    const record = thread({ origin: "lead-ask", status: "pending" });
    const outcome = withdrawQueuedQuestion(handleWith(record), undefined, record);
    assert.equal(outcome, "removed");
    assert.equal(record.status, "closed");
  });

  test("an open lead-ask thread (the owner has it open) defers the close rather than yanking the in-progress view", () => {
    const record = thread({ origin: "lead-ask", status: "open" });
    const outcome = withdrawQueuedQuestion(handleWith(record), undefined, record);
    assert.equal(outcome, "deferred");
    assert.equal(record.status, "open", "status stays open — the owner's view is not torn down");
    assert.equal(record.withdrawnPending, true);
  });

  test("a dormant (already-answered) lead-ask thread is a no-op", () => {
    const record = thread({ origin: "lead-ask", status: "dormant" });
    const outcome = withdrawQueuedQuestion(handleWith(record), undefined, record);
    assert.equal(outcome, "no-op");
    assert.equal(record.status, "dormant");
    assert.equal(record.withdrawnPending, undefined);
  });

  test("an already-closed (already-withdrawn) lead-ask thread is a no-op", () => {
    const record = thread({ origin: "lead-ask", status: "closed" });
    const outcome = withdrawQueuedQuestion(handleWith(record), undefined, record);
    assert.equal(outcome, "no-op");
    assert.equal(record.status, "closed");
  });

  test("a fork-raised thread closes unconditionally and releases the respondent's threadBound (unchanged pre-260911 ws-resolve behavior; the concurrency contract above is lead-ask-only)", () => {
    const record = thread({ origin: "fork-raised", status: "open", respondentAgentId: "agent-7" });
    const respondent = { agentId: "agent-7", threadBound: true } as unknown as RpcAgentRecord;
    const rpcRegistry: RpcAgentRegistry = new Map([["agent-7", respondent]]);
    const outcome = withdrawQueuedQuestion(handleWith(record), rpcRegistry, record);
    assert.equal(outcome, "removed");
    assert.equal(record.status, "closed");
    assert.equal(respondent.threadBound, false);
  });

  test("a fork-raised thread with no respondent yet still closes without touching the registry", () => {
    const record = thread({ origin: "fork-raised", status: "pending" });
    const rpcRegistry: RpcAgentRegistry = new Map();
    const outcome = withdrawQueuedQuestion(handleWith(record), rpcRegistry, record);
    assert.equal(outcome, "removed");
    assert.equal(rpcRegistry.size, 0);
  });
});

describe("closeThreadOnDone / injectDiscussionSummary (fake pi)", () => {
  function setup(origin: "lead-ask" | "fork-raised" = "lead-ask") {
    const sent: Array<{ message: unknown; options: unknown }> = [];
    leadIdleRef.current = () => true;
    const handlers = new Map<string, () => void>();
    const pi = {
      on: (event: string, fn: () => void) => handlers.set(event, fn),
      sendUserMessage: () => handlers.get("agent_start")?.(),
      sendMessage: (message: unknown, options: unknown) => sent.push({ message, options }),
    } as unknown as ExtensionAPI;
    registerPushFlush(pi, { delayMs: () => 10 });
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

  test("§6: one custom message, admitted as followUp then released as steering, carrying the thread id", () => {
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
      { deliverAs: "steer", triggerTurn: true },
      "followUp remains the busy-time admission rule; confirmed start steers the summary before the first response",
    );
  });

  test("Phase 2: a summary arriving during reminder preflight is held until start", () => {
    leadWakeStartPendingRef.current = true;
    const { pi, sent, handle, record } = setup();
    injectDiscussionSummary(pi, handle, new Map(), record, "we take the second anchor");

    assert.equal(sent.length, 0, "pending wake holds the summary");
    assert.equal(record.status, "dormant");
    clearWakeStart();
    flushHeldPushes(pi, true);
    assert.deepEqual(sent[0].options, { deliverAs: "steer", triggerTurn: true });
  });

  test("Phase 2: pending start prevents release of a held raw summary", () => {
    leadWakeStartPendingRef.current = false;
    leadCompactingRef.current = true;
    const { pi, sent, handle, record } = setup();
    injectDiscussionSummary(pi, handle, new Map(), record, "we take the second anchor");

    assert.deepEqual(sent, [], "held — compacting");
    assert.equal(heldPushQueue.length, 1);

    // The flag flips to true only AFTER the send was held — a closure that
    // captured `triggerTurn` at hold time would still send `true` here.
    leadWakeStartPendingRef.current = true;
    leadCompactingRef.current = false;
    assert.equal(flushHeldPushes(pi), 0);
    assert.equal(sent.length, 0, "still held for pending start");
    clearWakeStart();
    assert.equal(flushHeldPushes(pi, true), 1);
    assert.deepEqual(sent[0].options, { deliverAs: "steer", triggerTurn: true });
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
    assert.equal(flushHeldPushes(pi), 0, "release requests wake, whose synchronous fake start drains");
    assert.equal(sent.length, 1, "delivered once started");
    const msg = sent[0].message as { customType: string; content: string; details: { threadId: string } };
    assert.equal(msg.customType, "ws-thread-summary");
    assert.equal(msg.details.threadId, "q1");
    assert.ok(msg.content.includes("we take the second anchor"));
    assert.deepEqual(sent[0].options, { deliverAs: "steer", triggerTurn: true }, "confirmed start overrides the recorded admission mode");
  });

  test("Phase 2: an idle summary is delivered after the fake user wake confirms start", () => {
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

describe("deliverQueuedAnswer (260911 D1: the fork-less lead-ask send path — no respondent to stop or summarize)", () => {
  function setup() {
    const sent: Array<{ message: unknown; options: unknown }> = [];
    leadIdleRef.current = () => true;
    const handlers = new Map<string, () => void>();
    const pi = {
      on: (event: string, fn: () => void) => handlers.set(event, fn),
      sendUserMessage: () => handlers.get("agent_start")?.(),
      sendMessage: (message: unknown, options: unknown) => sent.push({ message, options }),
    } as unknown as ExtensionAPI;
    registerPushFlush(pi, { delayMs: () => 10 });
    const handle = createThreadRegistryHandle();
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "session.jsonl.ws-threads.json");
    hydrateThreadRegistry(handle, path);
    const record = thread({ threadId: "q1", question: "Which anchor?", context: "background", origin: "lead-ask" });
    handle.threads.set(record.threadId, record);
    return { pi, sent, handle, path, record };
  }

  test("delivers one custom message carrying the answer, admitted as followUp then released as steering", () => {
    const { pi, sent, handle, record } = setup();
    deliverQueuedAnswer(pi, handle, record, "we take the second anchor");

    assert.equal(sent.length, 1);
    const msg = sent[0].message as { customType: string; content: string; display: boolean; details: { threadId: string; title: string } };
    assert.equal(msg.customType, "ws-thread-summary");
    assert.equal(msg.display, true);
    assert.equal(msg.details.threadId, "q1");
    assert.equal(msg.details.title, record.title);
    assert.ok(msg.content.includes("we take the second anchor"));
    assert.deepEqual(sent[0].options, { deliverAs: "steer", triggerTurn: true });
  });

  test("the thread goes dormant (retained, not deleted), is persisted, and any deferred withdrawal is cleared", () => {
    const { pi, handle, path, record } = setup();
    record.withdrawnPending = true;
    deliverQueuedAnswer(pi, handle, record, "decided");

    assert.equal(record.status, "dormant");
    assert.equal(record.withdrawnPending, false, "the answer was delivered — a deferred withdrawal must not also fire");
    assert.ok(handle.threads.has("q1"), "dormant means retained and reopenable");
    assert.equal(loadThreadRegistryFile(path)[0]?.status, "dormant");
  });

  test("carries the D3 anchor (ask-time commit hash + entry_id) when the record has one", () => {
    const { pi, sent, handle, record } = setup();
    record.askCommitHash = "abc123";
    record.entryId = "entry-9";
    deliverQueuedAnswer(pi, handle, record, "decided");
    const msg = sent[0].message as { content: string };
    assert.ok(msg.content.includes("Asked at: commit abc123, entry entry-9"));
  });

  test("omits the anchor line entirely when neither half is present", () => {
    const { pi, sent, handle, record } = setup();
    deliverQueuedAnswer(pi, handle, record, "decided");
    const msg = sent[0].message as { content: string };
    assert.ok(!msg.content.includes("Asked at:"));
  });

  test("no live entries/branch available (headless, no sessionManager) still delivers — the excerpt is simply omitted", () => {
    const { pi, sent, handle, record } = setup();
    record.entryId = "entry-9";
    deliverQueuedAnswer(pi, handle, record, "decided", undefined);
    assert.equal(sent.length, 1);
    const msg = sent[0].message as { content: string };
    assert.ok(!msg.content.includes("no longer in your live context"));
  });

  test("attaches a verbatim excerpt when the anchored entry has fallen off the live branch (post-compaction)", () => {
    const { pi, sent, handle, record } = setup();
    record.entryId = "entry-9";
    const sessionManager = {
      buildContextEntries: () => [{ id: "entry-10" }],
      getBranch: (id: string) => (id === "entry-9" ? [{ id: "entry-9", role: "user", content: [{ type: "text", text: "old question turn" }] } as never] : []),
    };
    deliverQueuedAnswer(pi, handle, record, "decided", sessionManager);
    assert.equal(sent.length, 1);
    const msg = sent[0].message as { content: string };
    assert.match(msg.content, /no longer in your live context/i);
  });

  test("omits the excerpt when the anchored entry is still on the live branch", () => {
    const { pi, sent, handle, record } = setup();
    record.entryId = "entry-9";
    const sessionManager = {
      buildContextEntries: () => [{ id: "entry-9" }],
      getBranch: () => [],
    };
    deliverQueuedAnswer(pi, handle, record, "decided", sessionManager);
    const msg = sent[0].message as { content: string };
    assert.ok(!msg.content.includes("no longer in your live context"));
  });

  test("a session-tree read failure degrades to no excerpt rather than blocking delivery", () => {
    const { pi, sent, handle, record } = setup();
    record.entryId = "entry-9";
    const sessionManager = {
      buildContextEntries: () => { throw new Error("boom"); },
      getBranch: () => [],
    };
    deliverQueuedAnswer(pi, handle, record, "decided", sessionManager as never);
    assert.equal(sent.length, 1, "the answer still arrives");
    assert.equal(handle.threads.get("q1")!.status, "dormant");
  });
});

describe("resolveLeadAskEscapeAction / runLeadAskEscapeAction (260911 D-model-side-withdrawal: the queue modal's exit decision, extracted per review relay #1/#2)", () => {
  test("resolveLeadAskEscapeAction: withdrawn + non-empty draft -> deliver (a withdrawal never discards typed prose)", () => {
    assert.equal(resolveLeadAskEscapeAction(true, "we take the second anchor"), "deliver");
  });

  test("resolveLeadAskEscapeAction: withdrawn + empty draft -> finalize-withdrawal", () => {
    assert.equal(resolveLeadAskEscapeAction(true, ""), "finalize-withdrawal");
  });

  test("resolveLeadAskEscapeAction: not withdrawn + non-empty draft -> revert-pending (Phase 1 does not carry an in-progress, non-withdrawn draft forward)", () => {
    assert.equal(resolveLeadAskEscapeAction(false, "half-typed answer"), "revert-pending");
  });

  test("resolveLeadAskEscapeAction: not withdrawn + empty draft -> revert-pending", () => {
    assert.equal(resolveLeadAskEscapeAction(false, ""), "revert-pending");
  });

  function setup() {
    const sent: Array<{ message: unknown; options: unknown }> = [];
    leadIdleRef.current = () => true;
    const handlers = new Map<string, () => void>();
    const pi = {
      on: (event: string, fn: () => void) => handlers.set(event, fn),
      sendUserMessage: () => handlers.get("agent_start")?.(),
      sendMessage: (message: unknown, options: unknown) => sent.push({ message, options }),
    } as unknown as ExtensionAPI;
    registerPushFlush(pi, { delayMs: () => 10 });
    const handle = createThreadRegistryHandle();
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "session.jsonl.ws-threads.json");
    hydrateThreadRegistry(handle, path);
    const record = thread({ threadId: "q1", question: "Which anchor?", origin: "lead-ask", status: "open" });
    handle.threads.set(record.threadId, record);
    return { pi, sent, handle, record };
  }

  test("runLeadAskEscapeAction(\"deliver\"): routes through deliverQueuedAnswer exactly as a normal send — dormant, message sent, withdrawnPending cleared", () => {
    const { pi, sent, handle, record } = setup();
    record.withdrawnPending = true;
    runLeadAskEscapeAction("deliver", pi, handle, record, "we take the second anchor", undefined);
    assert.equal(sent.length, 1);
    assert.ok((sent[0].message as { content: string }).content.includes("we take the second anchor"));
    assert.equal(record.status, "dormant");
    assert.equal(record.withdrawnPending, false);
  });

  test("runLeadAskEscapeAction(\"finalize-withdrawal\"): closes with nothing delivered", () => {
    const { pi, sent, handle, record } = setup();
    record.withdrawnPending = true;
    runLeadAskEscapeAction("finalize-withdrawal", pi, handle, record, "", undefined);
    assert.equal(sent.length, 0, "nothing to deliver");
    assert.equal(record.status, "closed");
    assert.equal(record.withdrawnPending, false);
  });

  test("runLeadAskEscapeAction(\"revert-pending\"): closes with nothing delivered, stays answerable via a later /answer", () => {
    const { pi, sent, handle, record } = setup();
    runLeadAskEscapeAction("revert-pending", pi, handle, record, "half-typed answer", undefined);
    assert.equal(sent.length, 0, "the discarded draft is never delivered");
    assert.equal(record.status, "pending");
  });

  test("runLeadAskEscapeAction(\"revert-pending\"): persists a non-empty draft (260911 Phase 2 D3 — per-question drafts persist)", () => {
    const { pi, handle, record } = setup();
    runLeadAskEscapeAction("revert-pending", pi, handle, record, "half-typed answer", undefined);
    assert.equal(record.draftAnswer, "half-typed answer");
  });

  test("runLeadAskEscapeAction(\"revert-pending\"): an empty draft clears any earlier persisted one", () => {
    const { pi, handle, record } = setup();
    record.draftAnswer = "stale earlier draft";
    runLeadAskEscapeAction("revert-pending", pi, handle, record, "", undefined);
    assert.equal(record.draftAnswer, undefined);
  });

  test("runLeadAskEscapeAction(\"deliver\"): clears any persisted draft once the answer lands", () => {
    const { pi, handle, record } = setup();
    record.withdrawnPending = true;
    record.draftAnswer = "an earlier draft";
    runLeadAskEscapeAction("deliver", pi, handle, record, "final answer", undefined);
    assert.equal(record.draftAnswer, undefined);
  });

  test("runLeadAskEscapeAction(\"finalize-withdrawal\"): clears any persisted draft too", () => {
    const { pi, handle, record } = setup();
    record.withdrawnPending = true;
    record.draftAnswer = "an earlier draft";
    runLeadAskEscapeAction("finalize-withdrawal", pi, handle, record, "", undefined);
    assert.equal(record.draftAnswer, undefined);
  });
});

describe("resolveLeadAskQueueEntryAction (260911 Phase 2 D3: the queue modal's per-question close decision)", () => {
  test("mode=\"submit\", not withdrawn, non-empty draft -> deliver", () => {
    assert.equal(resolveLeadAskQueueEntryAction("submit", false, "an answer"), "deliver");
  });

  test("mode=\"submit\", not withdrawn, whitespace-only draft -> falls through to revert-pending (blank stays pending)", () => {
    assert.equal(resolveLeadAskQueueEntryAction("submit", false, "   "), "revert-pending");
  });

  test("mode=\"submit\", not withdrawn, empty draft -> revert-pending", () => {
    assert.equal(resolveLeadAskQueueEntryAction("submit", false, ""), "revert-pending");
  });

  test("mode=\"submit\", withdrawn pending, non-empty draft -> deliver via the withdrawal branch too (either path agrees)", () => {
    assert.equal(resolveLeadAskQueueEntryAction("submit", true, "an answer"), "deliver");
  });

  test("mode=\"submit\", withdrawn pending, empty draft -> finalize-withdrawal, never delivered as a false submit", () => {
    assert.equal(resolveLeadAskQueueEntryAction("submit", true, ""), "finalize-withdrawal");
  });

  test("mode=\"preserve\" (Esc-declined submit) -> always defers to resolveLeadAskEscapeAction, non-empty draft included", () => {
    assert.equal(resolveLeadAskQueueEntryAction("preserve", false, "an answer"), "revert-pending");
    assert.equal(resolveLeadAskQueueEntryAction("preserve", true, "an answer"), "deliver");
    assert.equal(resolveLeadAskQueueEntryAction("preserve", true, ""), "finalize-withdrawal");
  });
});

describe("collectLeadAskQueue (260911 Phase 2 D3: batch queue order)", () => {
  test("keeps only lead-ask origin, pending/open status, oldest-asked first", () => {
    const records = [
      thread({ threadId: "q3", origin: "lead-ask", status: "pending", createdAt: "2026-09-05T12:00:00.000Z" }),
      thread({ threadId: "q1", origin: "lead-ask", status: "open", createdAt: "2026-09-05T10:00:00.000Z" }),
      thread({ threadId: "q2", origin: "lead-ask", status: "pending", createdAt: "2026-09-05T11:00:00.000Z" }),
      thread({ threadId: "f1", origin: "fork-raised", status: "pending", createdAt: "2026-09-05T09:00:00.000Z" }),
      thread({ threadId: "d1", origin: "lead-ask", status: "dormant", createdAt: "2026-09-05T08:00:00.000Z" }),
      thread({ threadId: "c1", origin: "lead-ask", status: "closed", createdAt: "2026-09-05T07:00:00.000Z" }),
    ];
    const queue = collectLeadAskQueue(records);
    assert.deepEqual(queue.map((t) => t.threadId), ["q1", "q2", "q3"]);
  });

  test("ties on createdAt break by threadId", () => {
    const records = [
      thread({ threadId: "q2", origin: "lead-ask", status: "pending", createdAt: "2026-09-05T10:00:00.000Z" }),
      thread({ threadId: "q1", origin: "lead-ask", status: "pending", createdAt: "2026-09-05T10:00:00.000Z" }),
    ];
    assert.deepEqual(collectLeadAskQueue(records).map((t) => t.threadId), ["q1", "q2"]);
  });

  test("empty input -> empty queue", () => {
    assert.deepEqual(collectLeadAskQueue([]), []);
  });
});

describe("countQueueAnswered / buildQueueCoverageLine / buildQueueSubmitConfirmMessage (260911 Phase 2 D3)", () => {
  test("countQueueAnswered counts only non-empty-after-trim drafts", () => {
    const drafts = new Map([
      ["q1", "an answer"],
      ["q2", ""],
      ["q3", "   "],
      ["q4", "  another  "],
    ]);
    assert.equal(countQueueAnswered(drafts), 2);
  });

  test("countQueueAnswered on an empty map is zero", () => {
    assert.equal(countQueueAnswered(new Map()), 0);
  });

  test("buildQueueCoverageLine renders 1-based question index with the live answered count", () => {
    assert.equal(buildQueueCoverageLine(0, 3, 1), "Q1/3 · 1 answered");
    assert.equal(buildQueueCoverageLine(2, 3, 3), "Q3/3 · 3 answered");
  });

  test("buildQueueSubmitConfirmMessage pluralizes and reports the pending remainder", () => {
    assert.equal(buildQueueSubmitConfirmMessage(1, 3), "Submit 1 answered question? 2 left pending.");
    assert.equal(buildQueueSubmitConfirmMessage(3, 3), "Submit 3 answered questions? 0 left pending.");
    assert.equal(buildQueueSubmitConfirmMessage(0, 2), "Submit 0 answered questions? 2 left pending.");
  });
});

describe("LeadAskQueueComponent (260911 Phase 2 D3: sequential prose-modal tier)", () => {
  /** Mirrors the real host `Editor`'s clear-then-callback `onSubmit` contract (see `editor.js`'s `submitValue`) — the component must restore the text itself. */
  class FakeQueueEditor implements FocusableEditorLike {
    text = "";
    focused = false;
    onSubmit: ((text: string) => void) | undefined;
    render(width: number): string[] {
      return [`[e:${this.text}]`.slice(0, Math.max(1, width))];
    }
    invalidate(): void {}
    handleInput(data: string): void {
      if (data === "\r" || data === "\n") {
        // Matches the real Editor.submitValue(): clears its own buffer, THEN
        // fires onSubmit with the pre-clear text, trimmed (review-round-1
        // test Minor fix — this fake previously didn't trim, understating
        // its fidelity to the contract it claims to mirror).
        const pending = this.text.trim();
        this.text = "";
        this.onSubmit?.(pending);
        return;
      }
      this.text += data;
    }
    getText(): string {
      return this.text;
    }
    setText(text: string): void {
      this.text = text;
    }
  }

  function fakeTui(): ConversationViewTui & { renderCount: number } {
    const state = { requestRender: () => {}, renderCount: 0 };
    state.requestRender = () => {
      state.renderCount += 1;
    };
    return state as ConversationViewTui & { renderCount: number };
  }

  /** Minimal fake covering only the keyIds the confirm screens actually probe. */
  function fakeMatchesKey(data: string, keyId: string): boolean {
    if (keyId === "enter") return data === "\r" || data === "\n";
    if (keyId === "left") return data === "\x1b[D";
    if (keyId === "right") return data === "\x1b[C";
    if (keyId === "up") return data === "\x1b[A";
    if (keyId === "down") return data === "\x1b[B";
    return false;
  }

  /** No-op word-wrap — tests use widths and strings that never need to split. */
  function fakeWrapText(text: string, _width: number): string[] {
    return text.length === 0 ? [""] : text.split("\n");
  }

  function buildQueue(threads: ThreadRecord[], overrides: Partial<LeadAskQueueOptions> = {}) {
    const editors: FakeQueueEditor[] = [];
    const tui = fakeTui();
    const closes: Array<{ mode: "submit" | "preserve"; drafts: Map<string, string> }> = [];
    const component = new LeadAskQueueComponent(tui, {
      threads,
      initialFocusIndex: 0,
      editorFactory: () => {
        const editor = new FakeQueueEditor();
        editors.push(editor);
        return editor;
      },
      matchesKey: fakeMatchesKey,
      wrapText: fakeWrapText,
      border: false,
      onClose: (mode, drafts) => closes.push({ mode, drafts }),
      ...overrides,
    });
    return { component, editors, tui, closes };
  }

  function threeThreads(): ThreadRecord[] {
    return [
      thread({ threadId: "q1", question: "First?", origin: "lead-ask", status: "open", createdAt: "2026-09-05T10:00:00.000Z" }),
      thread({ threadId: "q2", question: "Second?", origin: "lead-ask", status: "open", createdAt: "2026-09-05T11:00:00.000Z" }),
      thread({ threadId: "q3", question: "Third?", origin: "lead-ask", status: "open", createdAt: "2026-09-05T12:00:00.000Z" }),
    ];
  }

  test("Enter on a non-last question commits the draft and advances focus, without closing", () => {
    const { component, editors, closes } = buildQueue(threeThreads());
    editors[0].handleInput("first answer");
    editors[0].handleInput("\r");
    assert.equal(component.getFocusedIndex(), 1);
    assert.equal(component.getDraft(0), "first answer", "Enter commits — the text is restored after the real Editor's clear");
    assert.equal(closes.length, 0);
  });

  test("the coverage line reports the focused 1-based index, total, and live answered count", () => {
    const { component, editors } = buildQueue(threeThreads());
    editors[0].handleInput("first answer");
    const lines = component.render(80);
    assert.ok(lines.some((l) => l.includes(buildQueueCoverageLine(0, 3, 1))), lines.join("\n"));
    editors[0].handleInput("\r");
    const linesAfter = component.render(80);
    assert.ok(linesAfter.some((l) => l.includes(buildQueueCoverageLine(1, 3, 1))), linesAfter.join("\n"));
  });

  test("Enter on the LAST question raises the final confirm instead of advancing, cursor defaulting to No", () => {
    const { component, editors, closes } = buildQueue(threeThreads(), { initialFocusIndex: 2 });
    editors[2].handleInput("last answer");
    editors[2].handleInput("\r");
    assert.equal(closes.length, 0, "the confirm intercepts — it does not close on its own");
    const lines = component.render(80);
    assert.ok(lines.some((l) => l.includes("[No]")), lines.join("\n"));
    assert.ok(lines.some((l) => l.includes(" Yes ")), lines.join("\n"));
  });

  test("final confirm: Enter on the default No cancels back to editing the last question", () => {
    const { component, editors, closes } = buildQueue(threeThreads(), { initialFocusIndex: 2 });
    editors[2].handleInput("last answer");
    editors[2].handleInput("\r"); // raises the confirm
    component.handleInput("\r"); // Enter on default No
    assert.equal(closes.length, 0);
    const lines = component.render(80);
    assert.ok(lines.some((l) => l.includes("Third?")), "back to editing the last question, not the confirm screen");
    assert.equal(component.getDraft(2), "last answer", "declining never discards the typed answer");
  });

  test("final confirm: moving to Yes then Enter submits with every drafted answer, including blanks", () => {
    const { component, editors, closes } = buildQueue(threeThreads(), { initialFocusIndex: 2 });
    editors[0].handleInput("first answer");
    editors[2].handleInput("last answer");
    editors[2].handleInput("\r"); // raises the confirm on the last question
    component.handleInput("\x1b[C"); // -> right/Yes
    component.handleInput("\r"); // confirm
    assert.equal(closes.length, 1);
    assert.equal(closes[0].mode, "submit");
    assert.equal(closes[0].drafts.get("q1"), "first answer");
    assert.equal(closes[0].drafts.get("q2"), "");
    assert.equal(closes[0].drafts.get("q3"), "last answer");
  });

  test("Esc with nothing answered anywhere closes immediately, preserving, with no confirm screen", () => {
    const { component, closes } = buildQueue(threeThreads());
    component.handleInput("\x1b");
    assert.equal(closes.length, 1);
    assert.equal(closes[0].mode, "preserve");
  });

  test("Esc with something answered raises the Esc-partial-submit confirm instead of closing", () => {
    const { component, editors, closes } = buildQueue(threeThreads());
    editors[0].handleInput("first answer");
    component.handleInput("\x1b");
    assert.equal(closes.length, 0);
    const lines = component.render(80);
    assert.ok(lines.some((l) => l.includes(buildQueueSubmitConfirmMessage(1, 3))), lines.join("\n"));
    assert.ok(lines.some((l) => l.includes("[No]")), "defaults to the safe No");
  });

  test("Esc confirm: Enter on default No exits WITHOUT submitting, but still preserves the typed draft", () => {
    const { component, editors, closes } = buildQueue(threeThreads());
    editors[0].handleInput("first answer");
    component.handleInput("\x1b"); // raises the esc confirm
    component.handleInput("\r"); // Enter on default No
    assert.equal(closes.length, 1);
    assert.equal(closes[0].mode, "preserve");
    assert.equal(closes[0].drafts.get("q1"), "first answer", "Esc never discards typed prose even when declining to submit");
  });

  test("Esc confirm: moving to Yes then Enter submits only the answered questions", () => {
    const { component, editors, closes } = buildQueue(threeThreads());
    editors[0].handleInput("first answer");
    component.handleInput("\x1b");
    component.handleInput("\x1b[B"); // down -> Yes
    component.handleInput("\r");
    assert.equal(closes.length, 1);
    assert.equal(closes[0].mode, "submit");
    assert.equal(closes[0].drafts.get("q1"), "first answer");
  });

  test("Esc inside a confirm screen always takes the No branch, regardless of the cursor's current side", () => {
    const { component, editors, closes } = buildQueue(threeThreads());
    editors[0].handleInput("first answer");
    component.handleInput("\x1b"); // esc confirm raised, default No
    component.handleInput("\x1b[C"); // move to Yes
    component.handleInput("\x1b"); // Esc inside the confirm -> No branch (exit, no submit)
    assert.equal(closes.length, 1);
    assert.equal(closes[0].mode, "preserve");
  });

  test("Tab/Shift+Tab wrap last->first and first->last, and never submit or raise a confirm", () => {
    const { component, closes } = buildQueue(threeThreads(), { initialFocusIndex: 2 });
    component.handleInput("\t");
    assert.equal(component.getFocusedIndex(), 0, "wraps from the last question to the first");
    component.handleInput("\x1b[Z"); // Shift+Tab
    assert.equal(component.getFocusedIndex(), 2, "wraps back from the first to the last");
    assert.equal(closes.length, 0);
  });

  test("Ctrl+C is swallowed, not forwarded to the focused editor", () => {
    const { component, editors } = buildQueue(threeThreads());
    component.handleInput("\x03");
    assert.equal(editors[0].getText(), "");
  });

  test("a withdrawn-pending question's banner renders without touching its preloaded draft", () => {
    const threads = threeThreads();
    threads[0].withdrawnPending = true;
    threads[0].draftAnswer = "typed before the withdrawal";
    const { component } = buildQueue(threads);
    const lines = component.render(80);
    assert.ok(lines.some((l) => l.toLowerCase().includes("withdrew")), lines.join("\n"));
    assert.equal(component.getDraft(0), "typed before the withdrawal");
  });

  test("per-question drafts persist across construction — a thread's draftAnswer seeds its editor", () => {
    const threads = threeThreads();
    threads[1].draftAnswer = "resumed draft text";
    const { component } = buildQueue(threads);
    assert.equal(component.getDraft(1), "resumed draft text");
  });

  test("initialFocusIndex positions the focused question (e.g. from /answer <id> or the reopen shortcut)", () => {
    const { component } = buildQueue(threeThreads(), { initialFocusIndex: 1 });
    assert.equal(component.getFocusedIndex(), 1);
    const lines = component.render(80);
    assert.ok(lines.some((l) => l.includes("Second?")));
  });

  test("a stale submit from a no-longer-focused editor is ignored (defensive — input is only ever forwarded to the focused editor)", () => {
    const { component, editors, closes } = buildQueue(threeThreads());
    component.handleInput("\t"); // focus moves to index 1, without touching editor 0
    editors[0].handleInput("late answer");
    editors[0].handleInput("\r"); // fires editor 0's onSubmit directly, out of band
    assert.equal(component.getFocusedIndex(), 1, "the stale submit never advances focus");
    assert.equal(closes.length, 0);
  });

  test("a stale submit still restores its own editor's text before the focus guard returns (no silent data loss)", () => {
    const { component, editors } = buildQueue(threeThreads());
    component.handleInput("\t"); // focus moves to index 1
    editors[0].handleInput("late answer");
    editors[0].handleInput("\r");
    assert.equal(editors[0].getText(), "late answer", "the real Editor already cleared its buffer before firing onSubmit — the callback must restore it even when the submit is stale");
  });

  test("an ordinary keystroke through the component's own handleInput reaches the currently focused editor (the default-dispatch path), not a fixed one", () => {
    const { component, editors } = buildQueue(threeThreads(), { initialFocusIndex: 1 });
    component.handleInput("h");
    component.handleInput("i");
    assert.equal(editors[1].getText(), "hi");
    assert.equal(editors[0].getText(), "");
    assert.equal(editors[2].getText(), "");
  });

  test("review-round-1 correctness Critical regression: a whitespace-only draft on a withdrawn-pending question never delivers as an answer", () => {
    const threads = [
      thread({ threadId: "q1", question: "Only one?", origin: "lead-ask", status: "open", withdrawnPending: true, createdAt: "2026-09-05T10:00:00.000Z" }),
    ];
    const { component, editors, closes } = buildQueue(threads);
    editors[0].handleInput(" "); // whitespace only, never committed via Enter
    component.handleInput("\x1b"); // Esc — nothing "answered" once trimmed, so this closes immediately with no confirm
    assert.equal(closes.length, 1);
    assert.equal(closes[0].mode, "preserve");
    assert.equal(closes[0].drafts.get("q1"), "", "the draft handed to the host must be trimmed — a lone space must never read as a real answer");
  });

  test("a withdrawn-pending question's typed content still reaches onClose's drafts map on submit (a withdrawal never discards typed prose)", () => {
    const threads = threeThreads();
    threads[2].withdrawnPending = true;
    const { component, editors, closes } = buildQueue(threads, { initialFocusIndex: 2 });
    editors[2].handleInput("answered despite the withdrawal");
    editors[2].handleInput("\r"); // raises the final confirm (last question)
    component.handleInput("\x1b[C"); // -> Yes
    component.handleInput("\r");
    assert.equal(closes.length, 1);
    assert.equal(closes[0].mode, "submit");
    assert.equal(closes[0].drafts.get("q3"), "answered despite the withdrawal");
  });

  test("initialFocusIndex is clamped into range for an out-of-bounds value (negative or overflowing)", () => {
    const low = buildQueue(threeThreads(), { initialFocusIndex: -5 });
    assert.equal(low.component.getFocusedIndex(), 0);
    const high = buildQueue(threeThreads(), { initialFocusIndex: 99 });
    assert.equal(high.component.getFocusedIndex(), 2);
  });

  test("a single-question queue: Enter raises the final confirm directly (there is no 'next' to advance to)", () => {
    const threads = [
      thread({ threadId: "q1", question: "Only one?", origin: "lead-ask", status: "open", createdAt: "2026-09-05T10:00:00.000Z" }),
    ];
    const { component, editors, closes } = buildQueue(threads);
    editors[0].handleInput("the only answer");
    editors[0].handleInput("\r");
    assert.equal(closes.length, 0, "the confirm intercepts first, even with only one question in the batch");
    const lines = component.render(80);
    assert.ok(lines.some((l) => l.includes("Q1/1")), lines.join("\n"));
    component.handleInput("\x1b[C");
    component.handleInput("\r");
    assert.equal(closes.length, 1);
    assert.equal(closes[0].drafts.get("q1"), "the only answer");
  });

  test("review-round-1 correctness Important fix: a very long question never pushes the answer Editor or the Esc hint off a short viewport", () => {
    const threads = threeThreads();
    threads[0].question = Array.from({ length: 200 }, (_, i) => `line ${i}`).join("\n");
    const tui: ConversationViewTui & { renderCount: number } = { requestRender: () => {}, renderCount: 0, terminal: { rows: 20 } };
    const editors: FakeQueueEditor[] = [];
    const component = new LeadAskQueueComponent(tui, {
      threads,
      initialFocusIndex: 0,
      editorFactory: () => {
        const editor = new FakeQueueEditor();
        editors.push(editor);
        return editor;
      },
      matchesKey: fakeMatchesKey,
      wrapText: fakeWrapText,
      border: true,
      onClose: () => {},
    });
    const lines = component.render(80);
    assert.ok(lines.length <= 20, `rendered ${lines.length} lines against a 20-row viewport`);
    assert.ok(lines.some((l) => l.includes("Esc: exit")), "the exit hint (the modal's only way out — Ctrl+C is swallowed) must survive even behind a very long question");
    assert.ok(lines.some((l) => l.includes("[e:")), "the answer editor itself must still render");
    assert.ok(lines.some((l) => l.includes("truncated")), "the dropped question tail is flagged, not silently vanished");
  });

  test("a short question well within the viewport is never truncated", () => {
    const tui: ConversationViewTui & { renderCount: number } = { requestRender: () => {}, renderCount: 0, terminal: { rows: 40 } };
    const editors: FakeQueueEditor[] = [];
    const component = new LeadAskQueueComponent(tui, {
      threads: threeThreads(),
      initialFocusIndex: 0,
      editorFactory: () => {
        const editor = new FakeQueueEditor();
        editors.push(editor);
        return editor;
      },
      matchesKey: fakeMatchesKey,
      wrapText: fakeWrapText,
      border: true,
      onClose: () => {},
    });
    const lines = component.render(80);
    assert.ok(lines.some((l) => l.includes("First?")));
    assert.ok(!lines.some((l) => l.includes("truncated")));
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

/**
 * Migrated from the old, now-deleted per-thread overlay module's test file's
 * "`/done` (the single fixed round-trip)" describe block (260908 Phase 2,
 * plan step 4): the state machine itself moved from that module's own
 * component's internal channel-event/liveness handling into `ask.ts`'s
 * `summarizeThenClose`, so these now drive that function directly against a
 * fake `ConversationViewComponent` (only `appendItem` is read), a fake
 * `ConversationChannel`, and a fake `OverlayHandle` — the same fake-`pi`-free
 * style `handleRespondentFinalReport`'s `overlayStub` already uses above.
 */
describe("summarizeThenClose (/done's single fixed round-trip)", () => {
  function fakeComponent() {
    const items: unknown[] = [];
    return { items, appendItem: (item: unknown) => items.push(item) };
  }

  function fakeChannel() {
    const listeners = new Set<(evt: unknown) => void>();
    const sent: string[] = [];
    let unsubscribed = 0;
    const channel: ConversationChannel = {
      onEvent(listener) {
        listeners.add(listener);
        return () => {
          listeners.delete(listener);
          unsubscribed += 1;
        };
      },
      liveness: () => "running",
      async send(text: string) {
        sent.push(text);
      },
    };
    return {
      channel,
      sent,
      get unsubscribed() {
        return unsubscribed;
      },
      delta: (text: string) => {
        for (const l of [...listeners]) l({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: text } });
      },
      settle: () => {
        for (const l of [...listeners]) l({ type: "agent_settled" });
      },
    };
  }

  function fakeOverlay() {
    const summaries: string[] = [];
    const alreadyRenderedFlags: (boolean | undefined)[] = [];
    const overlay: OverlayHandle = {
      close: () => {},
      closeWithSummary: (summary: string, alreadyRendered?: boolean) => {
        summaries.push(summary);
        alreadyRenderedFlags.push(alreadyRendered);
      },
    };
    return { overlay, summaries, alreadyRenderedFlags };
  }

  test("appends the ending note, sends exactly the fixed summary request, then takes the fork's next settled turn as the summary", async () => {
    const component = fakeComponent();
    const ch = fakeChannel();
    const ov = fakeOverlay();

    const unsubscribe = summarizeThenClose(component as never, ch.channel, ov.overlay);
    assert.equal(typeof unsubscribe, "function", "the fresh listener's own unsubscribe is returned so an owner Esc can tear it down early");

    assert.deepEqual(component.items, [{ kind: "note", text: "ending the thread — asking for a summary…" }]);
    assert.deepEqual(ch.sent, [buildDoneSummaryPrompt()]);
    assert.equal(ov.summaries.length, 0, "no summary before the fork settles");

    ch.delta("We agreed to merge and keep both histories.");
    ch.settle();
    assert.deepEqual(ov.summaries, ["We agreed to merge and keep both histories."]);
    assert.equal(ch.unsubscribed, 1, "the fresh event subscription is released once it fires");
    assert.deepEqual(
      ov.alreadyRenderedFlags,
      [true],
      "review relay #2 C1: the settled text was already appended by the component's own internal listener on this same agent_settled — closeWithSummary must be told not to append it again",
    );
  });

  test("a settled turn producing no text still closes the thread, with an explicit placeholder", () => {
    const ch = fakeChannel();
    const ov = fakeOverlay();
    summarizeThenClose(fakeComponent() as never, ch.channel, ov.overlay);
    ch.settle();
    assert.deepEqual(ov.summaries, [EMPTY_SUMMARY_TEXT]);
  });

  test("fires the summary at most once even if a further settle arrives", () => {
    const ch = fakeChannel();
    const ov = fakeOverlay();
    summarizeThenClose(fakeComponent() as never, ch.channel, ov.overlay);
    ch.delta("decided");
    ch.settle();
    ch.delta("more");
    ch.settle();
    assert.deepEqual(ov.summaries, ["decided"]);
  });

  test("M11: a half-streamed turn from before /done was submitted does not leak into the summary", () => {
    const ch = fakeChannel();
    const ov = fakeOverlay();
    // Streamed BEFORE summarizeThenClose is ever called — no listener exists
    // yet to see it, so there is nothing to leak by construction.
    ch.delta("partial answer that never settled");
    summarizeThenClose(fakeComponent() as never, ch.channel, ov.overlay);
    ch.delta("the actual summary");
    ch.settle();
    assert.deepEqual(ov.summaries, ["the actual summary"]);
  });

  test("the summary request text asks for a summary now, with no questions back", () => {
    const prompt = buildDoneSummaryPrompt();
    assert.match(prompt, /summary/i);
    assert.match(prompt, /owner ended the discussion/i);
  });
});

/**
 * Migrated from the old, now-deleted per-thread overlay module's test file's
 * "closeWithSummary (the fork ended the thread itself)" describe block: that
 * behavior is now
 * `buildOverlayHandle`'s own `closeWithSummary`, which appends the summary to
 * the live view before routing through the real `closeThreadOnDone` — driven
 * here against the same fake-`pi` `setup()` shape the
 * `closeThreadOnDone`/`injectDiscussionSummary` describe block above uses.
 */
describe("buildOverlayHandle (wraps a live component + the ctx.ui.custom done callback as an OverlayHandle)", () => {
  function setup(origin: "lead-ask" | "fork-raised" = "lead-ask") {
    const sent: Array<{ message: unknown; options: unknown }> = [];
    leadIdleRef.current = () => true;
    const handlers = new Map<string, () => void>();
    const pi = {
      on: (event: string, fn: () => void) => handlers.set(event, fn),
      sendUserMessage: () => handlers.get("agent_start")?.(),
      sendMessage: (message: unknown, options: unknown) => sent.push({ message, options }),
    } as unknown as ExtensionAPI;
    registerPushFlush(pi, { delayMs: () => 10 });
    const handle = createThreadRegistryHandle();
    const record = thread({ threadId: "q1", question: "Which anchor?", context: "background", origin, status: "open" });
    handle.threads.set(record.threadId, record);
    return { pi, sent, handle, record };
  }

  function fakeComponent() {
    const items: unknown[] = [];
    return { items, appendItem: (item: unknown) => items.push(item) };
  }

  test("close(): closes the view only — no summary is injected, the thread is untouched", () => {
    const { pi, sent, handle, record } = setup();
    const component = fakeComponent();
    let doneCalls = 0;
    const overlay = buildOverlayHandle(pi, handle, new Map(), record, component as never, () => (doneCalls += 1));

    overlay.close();

    assert.equal(doneCalls, 1);
    assert.deepEqual(component.items, []);
    assert.deepEqual(sent, []);
    assert.equal(record.status, "open");
  });

  test("closeWithSummary(summary): appends the summary as the child's own turn, then routes through closeThreadOnDone", () => {
    const { pi, sent, handle, record } = setup("lead-ask");
    const component = fakeComponent();
    let doneCalls = 0;
    const overlay = buildOverlayHandle(pi, handle, new Map(), record, component as never, () => (doneCalls += 1));

    overlay.closeWithSummary("We go with the second anchor.");

    assert.deepEqual(component.items, [{ kind: "assistant", text: "We go with the second anchor." }]);
    assert.equal(sent.length, 1, "a lead-ask thread's summary is injected to the lead");
    assert.equal(record.status, "dormant");
    assert.equal(doneCalls, 1);
  });

  test("closeWithSummary(\"\") (fork-raised detach): appends nothing, still detaches and still fires done", () => {
    const { pi, sent, handle, record } = setup("fork-raised");
    const component = fakeComponent();
    let doneCalls = 0;
    const overlay = buildOverlayHandle(pi, handle, new Map(), record, component as never, () => (doneCalls += 1));

    overlay.closeWithSummary("");

    assert.deepEqual(component.items, [], "an empty summary is never appended as a turn");
    assert.deepEqual(sent, [], "fork-raised: no summary is injected to the lead");
    assert.equal(record.status, "dormant");
    assert.equal(doneCalls, 1);
  });

  test("review relay #2 I1b/I4: close() then a later closeWithSummary() — the second call is a full no-op (finished guard)", () => {
    const { pi, sent, handle, record } = setup("lead-ask");
    const component = fakeComponent();
    let doneCalls = 0;
    let finishCalls = 0;
    const overlay = buildOverlayHandle(pi, handle, new Map(), record, component as never, () => (doneCalls += 1), () => (finishCalls += 1));

    overlay.close();
    overlay.closeWithSummary("late summary, after the owner already left");

    assert.equal(doneCalls, 1, "the second call must not fire done again");
    assert.equal(finishCalls, 1, "onFinish runs exactly once, for whichever call wins the race");
    assert.deepEqual(component.items, [], "a late summary must never be appended once the overlay is finished");
    assert.deepEqual(sent, [], "no injection from the losing call");
    assert.equal(record.status, "open", "close() won the race — the thread stays open");
  });

  test("review relay #2 I1b/I4: closeWithSummary() then a later close() — the second call is a full no-op", () => {
    const { pi, sent, handle, record } = setup("lead-ask");
    const component = fakeComponent();
    let doneCalls = 0;
    const overlay = buildOverlayHandle(pi, handle, new Map(), record, component as never, () => (doneCalls += 1));

    overlay.closeWithSummary("Decided: merge.");
    overlay.close();

    assert.equal(doneCalls, 1, "the second call must not fire done again");
    assert.deepEqual(component.items, [{ kind: "assistant", text: "Decided: merge." }], "not appended again, and not retracted by the losing close()");
    assert.equal(sent.length, 1, "exactly one injection, from the winning call");
    assert.equal(record.status, "dormant", "closeWithSummary() won the race — close() afterward must not undo it");
  });
});

describe("resolveDoneAction (review relay #2 I3: the summarizeOnDone branch, extracted for direct unit testing)", () => {
  test("summarizeOnDone === true resolves to 'summarize'; false resolves to 'close-empty'", () => {
    const summarize: DoneAction = resolveDoneAction(true);
    assert.equal(summarize, "summarize");
    const closeEmpty: DoneAction = resolveDoneAction(false);
    assert.equal(closeEmpty, "close-empty");
  });
});

/**
 * 260909 F1: `openThread`'s inline `onDone` dispatch — the seam that regressed
 * live (a `lead-ask` `/done` closed the overlay but injected no summary) — is
 * now the exported `runDoneAction`, so the whole (origin -> route -> lead
 * injection) chain is unit-lockable through the REAL `ConversationViewComponent`
 * + `buildOverlayHandle` wiring `openThread` uses, not a live TUI. The two
 * routes are asserted together so the scope guard (fork-raised injects
 * nothing) can never drift back onto the lead-ask route or vice versa.
 */
describe("runDoneAction (openThread's /done dispatch — F1 regression guard + scope guard)", () => {
  function sharedChannel(): { channel: ConversationChannel; fire: (evt: unknown) => void; sent: string[] } {
    const listeners = new Set<(evt: unknown) => void>();
    const sent: string[] = [];
    const channel: ConversationChannel = {
      onEvent: (l) => {
        listeners.add(l);
        return () => listeners.delete(l);
      },
      liveness: () => "running",
      send: async (text) => {
        sent.push(text);
      },
    };
    return { channel, fire: (evt) => { for (const l of [...listeners]) l(evt); }, sent };
  }

  function setup(origin: "lead-ask" | "fork-raised") {
    const sent: Array<{ message: unknown; options: unknown }> = [];
    leadIdleRef.current = () => true;
    const handlers = new Map<string, () => void>();
    const pi = {
      on: (event: string, fn: () => void) => handlers.set(event, fn),
      sendUserMessage: () => handlers.get("agent_start")?.(),
      sendMessage: (message: unknown, options: unknown) => sent.push({ message, options }),
    } as unknown as ExtensionAPI;
    registerPushFlush(pi, { delayMs: () => 10 });
    const handle = createThreadRegistryHandle();
    const record = thread({ threadId: "q1", question: "Which anchor?", context: "background", origin, status: "open" });
    handle.threads.set(record.threadId, record);
    return { pi, sent, handle, record };
  }

  test("F1: a lead-ask /done drives the fork's summary settle into a ws-thread-summary injection to the lead", () => {
    const { pi, sent, handle, record } = setup("lead-ask");
    const ch = sharedChannel();
    const component = new ConversationViewComponent({ requestRender: () => {} }, { channel: ch.channel });
    const overlay = buildOverlayHandle(pi, handle, new Map(), record, component, () => {});

    // Exactly openThread's own onDone: resolveDoneAction(thread.origin === "lead-ask").
    const pending = runDoneAction(resolveDoneAction(record.origin === "lead-ask"), component, ch.channel, overlay);
    assert.equal(typeof pending, "function", "the summarize route returns the pending listener's unsubscribe");
    assert.deepEqual(ch.sent, [buildDoneSummaryPrompt()], "the discussion fork is asked for a summary turn");
    assert.equal(sent.length, 0, "nothing is injected until the fork settles that summary");

    ch.fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "We take the second anchor." } });
    ch.fire({ type: "agent_settled" });

    assert.equal(sent.length, 1, "F1: the lead-ask /done route injects the summary into the lead (the regression)");
    const msg = sent[0].message as { customType: string; content: string };
    assert.equal(msg.customType, "ws-thread-summary");
    assert.ok(msg.content.includes("We take the second anchor."));
    assert.equal(record.status, "dormant");
  });

  test("scope guard: a fork-raised /done detaches with NO injection and no pending summarize listener", () => {
    const { pi, sent, handle, record } = setup("fork-raised");
    const ch = sharedChannel();
    const component = new ConversationViewComponent({ requestRender: () => {} }, { channel: ch.channel });
    const overlay = buildOverlayHandle(pi, handle, new Map(), record, component, () => {});

    const pending = runDoneAction(resolveDoneAction(record.origin === "lead-ask"), component, ch.channel, overlay);

    assert.equal(pending, undefined, "close-empty has no pending summarize listener");
    assert.deepEqual(ch.sent, [], "a fork-raised thread's fork is never asked to summarize");
    assert.deepEqual(sent, [], "the scope guard: a fork-raised /done injects nothing into the lead");
    assert.equal(record.status, "dormant", "the thread detaches (dormant, reopenable) — the task fork keeps running");
  });

  /**
   * 260909 F1, live-path reproduction. The two tests above feed `runDoneAction`
   * a record whose `origin` was set by hand. These two instead build the record
   * through its ACTUAL construction site and then round-trip it through
   * `serializeThreadRegistry` -> `parseThreadRegistry` — exactly the on-disk
   * `<sessionFile>.ws-threads.json` persist + `/reload` re-hydration the owner's
   * live thread went through — before reading `thread.origin` at `/done` time.
   * This closes the gap the owner's live evidence exposed: a `/done` on the real
   * "테스트 질문" thread closed with no summary because that thread had
   * `origin: "fork-raised"` on disk (a fork raised it via
   * `ws-report-to-lead(kind:"question")`, NOT the lead via `ws-ask`), so
   * close-empty was correct. These lock which construction site yields which
   * origin, and that the origin the `/done` predicate reads survives a reload
   * unchanged in BOTH directions.
   */
  function reload(record: ReturnType<typeof thread>): ReturnType<typeof thread> {
    const [reloaded] = parseThreadRegistry(serializeThreadRegistry([record]));
    return reloaded as ReturnType<typeof thread>;
  }

  test("F1 live path: a lead-ask thread built by the real ws-ask tool still routes /done to summarize+inject after a persist/reload", async () => {
    // Construct via the REAL ws-ask tool execute — the only lead-ask origin site.
    const tools = new Map<string, { execute: (...a: unknown[]) => Promise<{ content: Array<{ text: string }> }> }>();
    const askPi = { registerTool: (t: { name: string }) => tools.set(t.name, t as never) } as unknown as ExtensionAPI;
    const askHandle = createThreadRegistryHandle();
    registerAsk(askPi, askHandle, new Map());
    await tools.get(ASK_TOOL_NAME)!.execute("call-1", { title: "Which anchor?", question: "Which anchor?" }, undefined, undefined, { mode: "tui", ui: { notify: () => {} } });
    const created = askHandle.threads.get("q1")!;
    assert.equal(created.origin, "lead-ask", "sanity: ws-ask is the lead-ask origin site");

    // The /reload round-trip openThread's record actually survives, then opened.
    const record = reload(created);
    record.status = "open";
    assert.equal(record.origin, "lead-ask", "origin survives the persist/reload the live thread goes through");

    const { pi, sent, handle } = setup("lead-ask");
    handle.threads.set(record.threadId, record);
    const ch = sharedChannel();
    const component = new ConversationViewComponent({ requestRender: () => {} }, { channel: ch.channel });
    const overlay = buildOverlayHandle(pi, handle, new Map(), record, component, () => {});

    runDoneAction(resolveDoneAction(record.origin === "lead-ask"), component, ch.channel, overlay);
    assert.deepEqual(ch.sent, [buildDoneSummaryPrompt()], "the reloaded lead-ask thread is still asked for a summary turn");
    ch.fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "Second anchor." } });
    ch.fire({ type: "agent_settled" });
    assert.equal(sent.length, 1, "the reloaded lead-ask /done still injects the summary into the lead");
    assert.equal((sent[0].message as { customType: string }).customType, "ws-thread-summary");
  });

  test("F1 live path: the owner's real symptom — a fork-raised thread (ws-report-to-lead question) closes with NO summary after a reload, which is correct", () => {
    // Construct via the REAL fork-raised site — a fork's ws-report-to-lead(kind:"question").
    const frHandle = createThreadRegistryHandle();
    const created = handleForkRaisedQuestion(frHandle, new Map(), "agent-7", "테스트 질문입니다. 모달이 정상적으로 보이나요?");
    assert.equal(created.origin, "fork-raised", "sanity: a fork-raised question is the fork-raised origin site");
    assert.equal(created.entryId, undefined, "a fork-raised thread has no lead entry — the on-disk field profile the owner's q1 matched");

    const record = reload(created);
    record.status = "open";
    assert.equal(record.origin, "fork-raised", "origin survives the reload — the /done predicate reads fork-raised, exactly the live thread's shape");

    const { pi, sent, handle } = setup("fork-raised");
    handle.threads.set(record.threadId, record);
    const ch = sharedChannel();
    const component = new ConversationViewComponent({ requestRender: () => {} }, { channel: ch.channel });
    const overlay = buildOverlayHandle(pi, handle, new Map(), record, component, () => {});

    const pending = runDoneAction(resolveDoneAction(record.origin === "lead-ask"), component, ch.channel, overlay);
    assert.equal(pending, undefined, "close-empty: no summarize listener — matches the owner's 'overlay just closed' report");
    assert.deepEqual(ch.sent, [], "no summary turn is ever requested (the owner saw none) — the scope guard, not a bug");
    assert.deepEqual(sent, [], "nothing is injected into the lead for a fork-raised /done");
  });
});

/**
 * Review relay #2 — Critical + Important, exercised together against the
 * REAL `ConversationViewComponent` sharing ONE channel with `summarizeThenClose`
 * (exactly `openThread`'s own wiring), rather than the fakes the describe
 * blocks above use: those fakes each isolate one function, which is exactly
 * why the original bug — the component's own internal listener and
 * `summarizeThenClose`'s fresh listener both reacting to the SAME
 * `agent_settled` — was invisible to them. `pendingUnsubscribe`/`onFinish`
 * below reproduce `openThread`'s own local wiring so these tests exercise the
 * real interaction, not a re-description of it.
 */
describe("summarizeThenClose + buildOverlayHandle + ConversationViewComponent wiring (the shared-channel races)", () => {
  function sharedChannel(): { channel: ConversationChannel; fire: (evt: unknown) => void; sent: string[] } {
    const listeners = new Set<(evt: unknown) => void>();
    const sent: string[] = [];
    const channel: ConversationChannel = {
      onEvent: (l) => {
        listeners.add(l);
        return () => listeners.delete(l);
      },
      liveness: () => "running",
      send: async (text) => {
        sent.push(text);
      },
    };
    return { channel, fire: (evt) => { for (const l of [...listeners]) l(evt); }, sent };
  }

  function setup(origin: "lead-ask" | "fork-raised" = "lead-ask") {
    const sent: Array<{ message: unknown; options: unknown }> = [];
    leadIdleRef.current = () => true;
    const handlers = new Map<string, () => void>();
    const pi = {
      on: (event: string, fn: () => void) => handlers.set(event, fn),
      sendUserMessage: () => handlers.get("agent_start")?.(),
      sendMessage: (message: unknown, options: unknown) => sent.push({ message, options }),
    } as unknown as ExtensionAPI;
    registerPushFlush(pi, { delayMs: () => 10 });
    const handle = createThreadRegistryHandle();
    const record = thread({ threadId: "q1", question: "Which anchor?", context: "background", origin, status: "open" });
    handle.threads.set(record.threadId, record);
    return { pi, sent, handle, record };
  }

  test("C1: the fork's summary turn appears exactly once in the view and is injected exactly once — no double-append regression", () => {
    const { pi, sent, handle, record } = setup("lead-ask");
    const ch = sharedChannel();
    const component = new ConversationViewComponent({ requestRender: () => {} }, { channel: ch.channel });
    let doneCalls = 0;
    const overlay = buildOverlayHandle(pi, handle, new Map(), record, component, () => (doneCalls += 1));

    summarizeThenClose(component, ch.channel, overlay);
    assert.deepEqual(ch.sent, [buildDoneSummaryPrompt()]);

    ch.fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "We agreed to merge and keep both histories." } });
    ch.fire({ type: "agent_settled" });

    const assistantItems = component.getItems().filter((item) => item.kind === "assistant");
    assert.deepEqual(
      assistantItems,
      [{ kind: "assistant", text: "We agreed to merge and keep both histories." }],
      "the summary turn must appear exactly once, not twice",
    );
    assert.equal(sent.length, 1, "exactly one ws-thread-summary injection");
    assert.equal(record.status, "dormant");
    assert.equal(doneCalls, 1);
  });

  test("I1a: an owner Esc during the pending summary wait tears down the listener — the thread stays open and a later settle is inert", () => {
    const { pi, sent, handle, record } = setup("lead-ask");
    const ch = sharedChannel();
    const component = new ConversationViewComponent({ requestRender: () => {} }, { channel: ch.channel });
    let doneCalls = 0;
    let pendingUnsubscribe: (() => void) | undefined;
    const overlay = buildOverlayHandle(pi, handle, new Map(), record, component, () => (doneCalls += 1), () => {
      pendingUnsubscribe?.();
      pendingUnsubscribe = undefined;
    });
    // openThread's onDone: summarizeOnDone === true -> summarizeThenClose,
    // whose returned unsubscribe is stored exactly like openThread's own
    // `pendingSummarizeUnsubscribe`.
    pendingUnsubscribe = summarizeThenClose(component, ch.channel, overlay);

    // openThread's onEscape routes Esc through overlay.close(), not the raw `done`.
    overlay.close();
    assert.equal(doneCalls, 1);
    assert.equal(record.status, "open", "Esc closes the VIEW only — the thread itself is untouched");
    assert.deepEqual(sent, []);

    // The fork answers the summary request anyway, after the owner left.
    ch.fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "late answer" } });
    ch.fire({ type: "agent_settled" });

    assert.deepEqual(sent, [], "the late settle must inject nothing — its listener was torn down by onFinish");
    assert.equal(record.status, "open", "still open — a later settle must not silently dormant the thread behind the owner's back");
    assert.equal(doneCalls, 1, "done must not fire a second time");
  });

  test("I1b/I4: the fork's own final report (an external closeWithSummary) racing a pending /done wins — no duplicate injection, and the later settle is inert", () => {
    const { pi, sent, handle, record } = setup("lead-ask");
    const ch = sharedChannel();
    const component = new ConversationViewComponent({ requestRender: () => {} }, { channel: ch.channel });
    let doneCalls = 0;
    let pendingUnsubscribe: (() => void) | undefined;
    const overlay = buildOverlayHandle(pi, handle, new Map(), record, component, () => (doneCalls += 1), () => {
      pendingUnsubscribe?.();
      pendingUnsubscribe = undefined;
    });
    pendingUnsubscribe = summarizeThenClose(component, ch.channel, overlay);
    assert.equal(ch.sent.length, 1);

    // handleRespondentFinalReport's path: the fork's kind:"final" report
    // arrives out-of-band (never a channel event) and wins the race.
    overlay.closeWithSummary("We go with the second anchor.");
    assert.equal(sent.length, 1);
    assert.equal(record.status, "dormant");
    assert.equal(doneCalls, 1);

    // The summary turn the owner was waiting on settles anyway, after the
    // race is already decided.
    ch.fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "late" } });
    ch.fire({ type: "agent_settled" });

    assert.equal(sent.length, 1, "no duplicate ws-thread-summary injection");
    assert.equal(doneCalls, 1, "done must not fire a second time");
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

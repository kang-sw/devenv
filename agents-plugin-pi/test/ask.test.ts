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
 * The `/answer` command's target-selection/refusal branches are covered with
 * a fake command registry. The genuinely live-only remainder is the lazy
 * discussion-fork spawn and overlay attachment, which need a live Pi session
 * or a real `RpcClient`.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe, afterEach } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  ASK_TOOL_NAME,
  RESOLVE_TOOL_NAME,
  loadThreadRegistryFile,
  hydrateThreadRegistry,
  createThreadRegistryHandle,
  withdrawQueuedQuestion,
  deliverQueuedAnswer,
  deliverQueuedAnswers,
  handleForkRaisedQuestion,
  registerAsk,
  injectDiscussionSummary,
  closeThreadOnDone,
  MAX_CONTEXT_CHARS,
  type ThreadRecord,
} from "../src/ask.ts";
import {
  agentWidgetRefreshRef,
  computeRunningStatusLine,
  flushHeldPushes,
  heldPushQueue,
  leadCompactingRef,
  leadWakeStartPendingRef,
  sendToAgent,
  stopAgent,
  type RpcAgentRecord,
  type RpcAgentRegistry,
  leadIdleRef,
  registerPushFlush,
  clearWakeStart,
} from "../src/spawner.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { capturePush, thread } from "./fixtures/ask-threads.ts";

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
    assert.equal(
      computeRunningStatusLine(registry),
      undefined,
      "a question-parked fork is outside the fan-in count entirely, and an empty fan-in produces no line at all",
    );
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
    /** Each message exactly as sent, before batch items are unpacked into `sent`. */
    const raw: unknown[] = [];
    leadIdleRef.current = () => true;
    const handlers = new Map<string, () => void>();
    const pi = {
      on: (event: string, fn: () => void) => handlers.set(event, fn),
      sendUserMessage: () => handlers.get("agent_start")?.(),
      sendMessage: (message: unknown, options: unknown) => {
        raw.push(message);
        capturePush(sent, message, options);
      },
    } as unknown as ExtensionAPI;
    registerPushFlush(pi, { delayMs: () => 10 });
    const handle = createThreadRegistryHandle();
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    const path = join(dir, "session.jsonl.ws-threads.json");
    hydrateThreadRegistry(handle, path);
    const record = thread({ threadId: "q1", question: "Which anchor?", context: "background", origin });
    handle.threads.set(record.threadId, record);
    return { pi, sent, raw, handle, path, record };
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
      threadBound: true,
      client: {
        prompt: async () => {},
        abort: async () => {},
        stop: async () => {
          stops.push("agent-7");
        },
      },
    } as unknown as RpcAgentRecord;
  }

  test("§6: one custom message, admitted as followUp then released as steering, carrying the thread id", () => {
    const { pi, sent, raw, handle, record } = setup();
    injectDiscussionSummary(pi, handle, new Map(), record, "we take the second anchor");

    assert.equal(sent.length, 1);
    const msg = sent[0].message as { customType: string; content: string; display: boolean; details: { threadId: string; title: string } };
    assert.equal(msg.customType, "ws-thread-summary");
    const envelope = raw[0] as { content: string };
    assert.match(envelope.content, /^\[system message from ws-pi-plugin\]\n<ws-push-batch /, "the model reads the summary under the adapter label");
    assert.ok(!msg.content.includes("[system message from ws-pi-plugin]"), "the batch carries the label, not its inner summary");
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

  test("I5: snapshots the resume fields BEFORE stopping the respondent and releases the thread bind", async () => {
    const { pi, handle, record } = setup();
    record.respondentAgentId = "agent-7";
    const stops: string[] = [];
    const live = liveRespondent(stops);
    const registry: RpcAgentRegistry = new Map([["agent-7", live]]);

    injectDiscussionSummary(pi, handle, registry, record, "decided");

    assert.equal(record.forkResume?.sessionPath, "/tmp/s.jsonl", "captured while the record was still live");
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

  test("/done on an already-idle fork-raised thread closes the view and issues one closeout without injecting a summary", async () => {
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
    assert.equal(live.threadBound, true, "the temporary bind protects the finish operation until its closeout settles");
    assert.ok(live.forkFinish, "an already-idle fork is evaluated immediately rather than stranded awaiting another event");
    assert.equal(record.status, "dormant", "the ordinary thread snapshot is persisted while finish continues in memory");
    assert.ok(handle.threads.has("q1"));
    assert.equal(loadThreadRegistryFile(path)[0]?.status, "dormant", "the detach is persisted like every other transition");
    assert.equal(record.forkResume?.sessionPath, "/tmp/s.jsonl", "the resume snapshot is refreshed on detach");
  });

  test("a repeated /done after finish released a dormant fork thread is a no-op", () => {
    const { pi, sent, handle, record } = setup("fork-raised");
    record.status = "dormant";
    record.respondentAgentId = "agent-7";
    const live = liveRespondent([]);
    live.threadBound = false;
    closeThreadOnDone(pi, handle, new Map([[live.agentId, live]]), record, "");
    assert.equal(live.forkFinish, undefined);
    assert.deepEqual(sent, []);
  });

  test("C2: detaching a fork-raised thread whose respondent is gone is a no-op, not a throw", () => {
    const { pi, sent, handle, record } = setup("fork-raised");
    record.respondentAgentId = "gone";
    closeThreadOnDone(pi, handle, new Map(), record, "");
    assert.deepEqual(sent, []);
    assert.equal(record.status, "dormant");
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
      sendMessage: (message: unknown, options: unknown) => capturePush(sent, message, options),
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

  test("one modal batch becomes one injected follow-up with every answer and provenance in queue order", () => {
    const rawSent: Array<{ message: unknown; options: unknown }> = [];
    const handlers = new Map<string, () => void>();
    leadIdleRef.current = () => false; // the lead is mid-turn; no eager wake can split the submission
    const pi = {
      on: (event: string, fn: () => void) => handlers.set(event, fn),
      sendUserMessage: () => assert.fail("a busy lead is not woken before its turn boundary"),
      sendMessage: (message: unknown, options: unknown) => rawSent.push({ message, options }),
    } as unknown as ExtensionAPI;
    registerPushFlush(pi, { delayMs: () => 10 });
    const handle = createThreadRegistryHandle();
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    hydrateThreadRegistry(handle, join(dir, "session.jsonl.ws-threads.json"));
    const q1 = thread({ threadId: "q1", origin: "lead-ask", status: "open", question: "First?", context: "first context", askCommitHash: "aaa111", entryId: "entry-1" });
    const q2 = thread({ threadId: "q2", origin: "lead-ask", status: "open", question: "Second?", context: "second context", askCommitHash: "bbb222", entryId: "entry-2" });
    const q3 = thread({ threadId: "q3", origin: "lead-ask", status: "pending", question: "Blank stays pending?" });
    handle.threads.set(q1.threadId, q1);
    handle.threads.set(q2.threadId, q2);
    handle.threads.set(q3.threadId, q3);

    deliverQueuedAnswers(pi, handle, [
      { thread: q1, answer: "alpha" },
      { thread: q2, answer: "beta" },
      { thread: q3, answer: "   " },
    ]);

    assert.equal(heldPushQueue.length, 1, "the modal submission admits one raw follow-up, not one per answer");
    handlers.get("agent_end")?.();
    assert.equal(rawSent.length, 1, "one lead turn boundary receives one batch envelope");
    const envelope = rawSent[0].message as { customType: string; content: string; details: { items: Array<{ content: string; details: { threadIds: string[] } }> } };
    assert.equal(envelope.customType, PUSH_BATCH_CUSTOM_TYPE);
    assert.match(envelope.content, /^\[system message from ws-pi-plugin\]\n<ws-push-batch /);
    assert.equal(envelope.details.items.length, 1, "the envelope contains one aggregated queued-answer item");
    const item = envelope.details.items[0];
    assert.deepEqual(item.details.threadIds, ["q1", "q2"]);
    assert.ok(item.content.indexOf("Question: First?") < item.content.indexOf("Question: Second?"), "payload order follows the deterministic queue snapshot");
    assert.match(item.content, /first context/);
    assert.match(item.content, /alpha/);
    assert.match(item.content, /Asked at: commit aaa111, entry entry-1/);
    assert.match(item.content, /second context/);
    assert.match(item.content, /beta/);
    assert.match(item.content, /Asked at: commit bbb222, entry entry-2/);
    assert.equal(q1.status, "dormant");
    assert.equal(q2.status, "dormant");
    assert.equal(q3.status, "pending", "a blank entry is excluded from delivery and remains pending");
  });
});

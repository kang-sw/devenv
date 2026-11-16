import { test, describe, afterEach } from "node:test";
import assert from "node:assert/strict";
import {
  ASK_TOOL_NAME,
  serializeThreadRegistry,
  parseThreadRegistry,
  createThreadRegistryHandle,
  handleForkRaisedQuestion,
  ensureRespondent,
  registerAsk,
  buildDoneSummaryPrompt,
  EMPTY_SUMMARY_TEXT,
  summarizeThenClose,
  buildOverlayHandle,
  resolveDoneAction,
  runDoneAction,
  type ThreadRecord,
  type OverlayHandle,
  type DoneAction,
} from "../src/ask.ts";
import { ConversationViewComponent, type ConversationChannel } from "../src/conversation-view.ts";
import {
  agentWidgetRefreshRef,
  heldPushQueue,
  leadCompactingRef,
  leadWakeStartPendingRef,
  type RpcAgentRecord,
  type RpcAgentRegistry,
  leadIdleRef,
  registerPushFlush,
} from "../src/spawner.ts";
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

  test("first open of an already-live respondent binds the thread", async () => {
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
 * `ConversationChannel`, and a fake `OverlayHandle`.
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
      sendMessage: (message: unknown, options: unknown) => capturePush(sent, message, options),
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
      sendMessage: (message: unknown, options: unknown) => capturePush(sent, message, options),
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
      sendMessage: (message: unknown, options: unknown) => capturePush(sent, message, options),
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

    // An external close-with-summary arrives out-of-band and wins the race.
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

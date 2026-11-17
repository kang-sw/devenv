import { test, describe, afterEach } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  hydrateThreadRegistry,
  createThreadRegistryHandle,
  resolveLeadAskEscapeAction,
  runLeadAskEscapeAction,
  registerThreadCommands,
  collectLeadAskQueue,
  countQueueAnswered,
  buildQueueCoverageLine,
  buildQueueSubmitConfirmMessage,
  resolveLeadAskQueueEntryAction,
  buildLeadAskQueueOnClose,
  LeadAskQueueComponent,
  type ThreadRecord,
  type FocusableEditorLike,
  type LeadAskQueueOptions,
} from "../src/ask.ts";
import { type ConversationViewTui } from "../src/conversation-view.ts";
import {
  agentWidgetRefreshRef,
  heldPushQueue,
  leadCompactingRef,
  leadWakeStartPendingRef,
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
      sendMessage: (message: unknown, options: unknown) => capturePush(sent, message, options),
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

describe("registerThreadCommands /answer target selection", () => {
  function setup() {
    const commands = new Map<string, { description?: string; handler: (args: string, ctx: unknown) => Promise<void> }>();
    const pi = {
      registerCommand: (name: string, command: { description?: string; handler: (args: string, ctx: unknown) => Promise<void> }) => commands.set(name, command),
      registerShortcut: () => {},
    } as unknown as ExtensionAPI;
    const handle = createThreadRegistryHandle();
    const answered = thread({ threadId: "q6", origin: "lead-ask", status: "dormant" });
    const withdrawn = thread({ threadId: "q7", origin: "lead-ask", status: "closed" });
    const pending = thread({ threadId: "q8", origin: "lead-ask", status: "pending", createdAt: "2026-09-05T12:00:00.000Z" });
    const oldest = thread({ threadId: "q5", origin: "lead-ask", status: "pending", createdAt: "2026-09-05T09:00:00.000Z" });
    for (const record of [answered, withdrawn, pending, oldest]) handle.threads.set(record.threadId, record);
    const opened: string[] = [];
    registerThreadCommands(pi, {} as never, new Map(), handle, {} as never, async (record) => { opened.push(record.threadId); });
    const notices: Array<{ message: string; level: string }> = [];
    const ctx = { mode: "tui", ui: { notify: (message: string, level: string) => notices.push({ message, level }) } };
    return { commands, handle, pending, notices, opened, ctx };
  }

  test("explicit answered or withdrawn ids report their exact terminal state and never fall through to another modal", async () => {
    const { commands, pending, notices, ctx } = setup();
    await commands.get("answer")!.handler("q6", ctx);
    await commands.get("answer")!.handler("q7", ctx);
    assert.deepEqual(notices.map((notice) => notice.message), [
      "ws: question q6 was already answered.",
      "ws: question q7 was withdrawn.",
    ]);
    assert.equal(pending.status, "pending", "a terminal explicit target never opens the rest of the queue");
  });

  test("an unknown explicit id reports an error and returns without opening the queue", async () => {
    const { commands, pending, notices, ctx } = setup();
    await commands.get("answer")!.handler("q404", ctx);
    assert.deepEqual(notices, [{ message: 'ws: no thread "q404" — /thread lists them.', level: "error" }]);
    assert.equal(pending.status, "pending");
  });

  test("bare /answer opens the oldest answerable item, while an explicit answerable id opens exactly that item", async () => {
    const { commands, opened, ctx } = setup();
    assert.match(commands.get("answer")!.description ?? "", /oldest queued/i);
    await commands.get("answer")!.handler("", ctx);
    await commands.get("answer")!.handler("q8", ctx);
    assert.deepEqual(opened, ["q5", "q8"]);
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
    if (keyId === "pageUp") return data === "\x1b[5~";
    if (keyId === "pageDown") return data === "\x1b[6~";
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
    component.handleInput("\x1b[D"); // left from the right-hand No -> Yes
    component.handleInput("\r"); // confirm
    assert.equal(closes.length, 1);
    assert.equal(closes[0].mode, "submit");
    assert.equal(closes[0].drafts.get("q1"), "first answer");
    assert.equal(closes[0].drafts.get("q2"), "");
    assert.equal(closes[0].drafts.get("q3"), "last answer");
  });

  test("the production modal-close callback batches multiple component answers into one follow-up and leaves blanks pending", () => {
    const rawSent: Array<{ message: unknown; options: unknown }> = [];
    const handlers = new Map<string, () => void>();
    leadIdleRef.current = () => false;
    const pi = {
      on: (event: string, fn: () => void) => handlers.set(event, fn),
      sendUserMessage: () => assert.fail("a busy lead is not woken before its boundary"),
      sendMessage: (message: unknown, options: unknown) => rawSent.push({ message, options }),
    } as unknown as ExtensionAPI;
    registerPushFlush(pi, { delayMs: () => 10 });
    const handle = createThreadRegistryHandle();
    const dir = mkdtempSync(join(tmpdir(), "ws-pi-ask-test-"));
    hydrateThreadRegistry(handle, join(dir, "session.jsonl.ws-threads.json"));
    const threads = threeThreads();
    for (const record of threads) handle.threads.set(record.threadId, record);
    const { component } = buildQueue(threads, {
      onClose: buildLeadAskQueueOnClose(pi, handle, threads),
    });

    component.handleInput("alpha");
    component.handleInput("\r"); // commit q1, advance q2
    component.handleInput("beta");
    component.handleInput("\r"); // commit q2, advance q3
    component.handleInput("\r"); // blank q3 raises final confirmation
    component.handleInput("\x1b[D"); // Yes
    component.handleInput("\r");

    assert.equal(heldPushQueue.length, 1, "the actual modal close callback admits one combined follow-up");
    handlers.get("agent_end")?.();
    assert.equal(rawSent.length, 1);
    const envelope = rawSent[0].message as { details: { items: Array<{ details: { threadIds: string[] } }> } };
    assert.deepEqual(envelope.details.items[0].details.threadIds, ["q1", "q2"]);
    assert.equal(threads[0].status, "dormant");
    assert.equal(threads[1].status, "dormant");
    assert.equal(threads[2].status, "pending");
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
    component.handleInput("\x1b[D"); // left from the right-hand No -> Yes
    component.handleInput("\r");
    assert.equal(closes.length, 1);
    assert.equal(closes[0].mode, "submit");
    assert.equal(closes[0].drafts.get("q1"), "first answer");
  });

  test("Esc inside a confirm screen always takes the No branch, regardless of the cursor's current side", () => {
    const { component, editors, closes } = buildQueue(threeThreads());
    editors[0].handleInput("first answer");
    component.handleInput("\x1b"); // esc confirm raised, default No
    component.handleInput("\x1b[D"); // move left to Yes
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

  test("Korean multiline drafts remain exact and isolated while switching questions", () => {
    const { component, editors } = buildQueue(threeThreads());
    editors[0].setText("첫째 줄\n둘째 줄");
    component.handleInput("\t");
    editors[1].setText("다른 답변");
    component.handleInput("\x1b[Z");
    assert.equal(component.getDraft(0), "첫째 줄\n둘째 줄");
    assert.equal(component.getDraft(1), "다른 답변");
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
    component.handleInput("\x1b[D"); // left -> Yes
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
    component.handleInput("\x1b[D");
    component.handleInput("\r");
    assert.equal(closes.length, 1);
    assert.equal(closes[0].drafts.get("q1"), "the only answer");
  });

  test("a very long question scrolls from first through final line while the answer editor and footer stay fixed", () => {
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
      overflowText: (text) => `[overflow:${text}]`,
      onClose: () => {},
    });

    let lines = component.render(80);
    assert.ok(lines.length <= 20, `rendered ${lines.length} lines against a 20-row viewport`);
    assert.ok(lines.some((l) => l.includes("line 0")), "the question starts at its first line");
    assert.ok(lines.some((l) => l.includes("[overflow:")), "the inaccessible boundary cue is semantically distinct");
    assert.ok(!lines.some((l) => l.includes("truncated")), "scrolling replaces destructive truncation");

    for (let i = 0; i < 100; i += 1) component.handleInput("\x1b[6~");
    lines = component.render(80);
    assert.ok(lines.some((l) => l.includes("line 199")), "repeated PageDown reaches the complete original question");
    assert.ok(lines.some((l) => l.includes("Esc: exit")), "the fixed footer survives at the bottom of the question");
    assert.ok(lines.some((l) => l.includes("[e:")), "the fixed answer editor survives at the bottom of the question");

    for (let i = 0; i < 100; i += 1) component.handleInput("\x1b[5~");
    lines = component.render(80);
    assert.ok(lines.some((l) => l.includes("line 0")), "repeated PageUp returns to the first line");
    assert.equal(editors[0].getText(), "", "question scrolling is not forwarded into the answer editor");
  });

  test("a one-row question viewport reserves cue width so every character remains reachable", () => {
    const threads = [thread({
      threadId: "q1",
      question: `${"A".repeat(72)}TAIL\nsecond row`,
      origin: "lead-ask",
      status: "open",
    })];
    const wrapByWidth = (text: string, width: number): string[] => {
      if (text.length === 0) return [""];
      const chunks: string[] = [];
      for (let i = 0; i < text.length; i += Math.max(1, width)) chunks.push(text.slice(i, i + Math.max(1, width)));
      return chunks;
    };
    const tui: ConversationViewTui = { requestRender: () => {}, terminal: { rows: 14 } };
    const component = new LeadAskQueueComponent(tui, {
      threads,
      initialFocusIndex: 0,
      editorFactory: () => new FakeQueueEditor(),
      matchesKey: fakeMatchesKey,
      wrapText: wrapByWidth,
      border: true,
      onClose: () => {},
    });
    let lines = component.render(80);
    assert.ok(lines.some((line) => line.includes("↕")), "the only row still exposes the overflow cue");
    component.handleInput("\x1b[6~");
    lines = component.render(80);
    assert.ok(lines.some((line) => line.includes("IL")), "the characters displaced by the cue are reachable on a later page");
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

  test("confirmation navigation follows bounded horizontal positions without wrapping", () => {
    const { component, editors } = buildQueue(threeThreads(), { initialFocusIndex: 2 });
    editors[2].handleInput("answer");
    editors[2].handleInput("\r");

    component.handleInput("\x1b[C"); // already on the right-hand No: edge no-op
    let lines = component.render(80);
    assert.ok(lines.some((l) => l.includes("[No]")), "Right at the right edge stays on No");

    component.handleInput("\x1b[D"); // left -> Yes
    component.handleInput("\x1b[D"); // already at left edge: no wrap
    lines = component.render(80);
    assert.ok(lines.some((l) => l.includes("[Yes]")), "Left moves to and stays on the visually left Yes");

    component.handleInput("\x1b[C"); // right -> No
    lines = component.render(80);
    assert.ok(lines.some((l) => l.includes("[No]")), "Right moves to the visually right No");
  });

  test("semantic painters distinguish queue hierarchy while keeping shortcut help dimmable", () => {
    const { component } = buildQueue(threeThreads(), {
      headerText: (text) => `[header:${text}]`,
      separatorText: (text) => `[separator:${text}]`,
      helpText: (text) => `[help:${text}]`,
    });
    const lines = component.render(80);
    assert.ok(lines.some((line) => line.includes("[header:Q1/3")), lines.join("\n"));
    assert.ok(lines.some((line) => line.includes("[separator:")), lines.join("\n"));
    assert.ok(lines.some((line) => line.includes("[help:Enter: answer")), lines.join("\n"));
  });
});

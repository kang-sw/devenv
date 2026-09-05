/**
 * Unit tests for overlay-chat.ts (260904 Phase 2, side-thread owner-question
 * surface) — the ticket's own named UNIT tier: instantiate the overlay
 * `Component` and drive `render(width)`/`handleInput(data)` in `node --test`
 * with no TTY, no subprocess and no real `RpcClient`, against a duck-typed
 * fake `ForkChannel` (the same convention test/fork.test.ts already uses for
 * a fake `toolCtx`/client).
 *
 * NOT covered here — the integration/human tiers the plan names: the overlay
 * actually rendering inside a live `ctx.ui.custom` overlay, visual polish,
 * IME/CJK candidate placement, and a real owner<->fork exchange producing a
 * genuine summary.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  OverlayChatComponent,
  DONE_COMMAND,
  EMPTY_SUMMARY_TEXT,
  MAX_TRANSCRIPT_LINES,
  buildDoneSummaryPrompt,
  visibleWidth,
  wrapLine,
  formatSpawnTime,
  PASTE_START,
  PASTE_END,
  type ForkChannel,
} from "../src/overlay-chat.ts";
import { resolveOwnerSendInterrupt } from "../src/ask.ts";

function harness(options: { question?: string; streaming?: boolean; createdAt?: string } = {}) {
  const sent: string[] = [];
  const listeners = new Set<(evt: unknown) => void>();
  let unsubscribed = 0;
  let renders = 0;
  const summaries: string[] = [];
  let doneCalls = 0;
  let streaming = options.streaming ?? false;

  const channel: ForkChannel = {
    onEvent(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
        unsubscribed += 1;
      };
    },
    isStreaming: () => streaming,
    async send(text) {
      sent.push(text);
    },
  };

  const component = new OverlayChatComponent(
    { requestRender: () => (renders += 1) },
    {
      title: "Rebase or merge?",
      threadId: "q3",
      question: options.question,
      createdAt: options.createdAt,
      channel,
      onDone: (summary) => summaries.push(summary),
    },
    () => {
      doneCalls += 1;
    },
  );

  function type(text: string): void {
    for (const char of text) component.handleInput(char);
  }

  return {
    component,
    sent,
    summaries,
    type,
    enter: () => component.handleInput("\r"),
    emit: (evt: unknown) => {
      for (const listener of [...listeners]) listener(evt);
    },
    delta: (text: string) => {
      for (const listener of [...listeners]) listener({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: text } });
    },
    settle: () => {
      for (const listener of [...listeners]) listener({ type: "agent_settled" });
    },
    setStreaming: (value: boolean) => {
      streaming = value;
    },
    get doneCalls() {
      return doneCalls;
    },
    get unsubscribed() {
      return unsubscribed;
    },
    get renders() {
      return renders;
    },
  };
}

/** Flushes the microtask queue so a `void channel.send(...)` settles before assertions. */
const flush = () => new Promise((resolve) => setImmediate(resolve));

describe("visibleWidth / wrapLine", () => {
  test("counts CJK as two columns and ASCII as one", () => {
    assert.equal(visibleWidth("abc"), 3);
    assert.equal(visibleWidth("한글"), 4);
    assert.equal(visibleWidth(""), 0);
  });

  test("never emits a wrapped line wider than the requested width", () => {
    for (const width of [1, 3, 8, 40]) {
      for (const line of wrapLine("the quick brown fox jumps over the lazy dog", width)) {
        assert.ok(visibleWidth(line) <= width, `line ${JSON.stringify(line)} exceeds width ${width}`);
      }
    }
  });

  test("wide characters never straddle the boundary", () => {
    for (const line of wrapLine("한글한글한글", 3)) {
      assert.ok(visibleWidth(line) <= 3, `line ${JSON.stringify(line)} exceeds width 3`);
    }
  });

  test("an empty line stays one empty line, and a non-positive width degrades safely", () => {
    assert.deepEqual(wrapLine("", 10), [""]);
    assert.deepEqual(wrapLine("abc", 0), [""]);
  });
});

describe("formatSpawnTime", () => {
  test("renders an ISO timestamp as a labeled UTC minute, and degrades to undefined", () => {
    assert.equal(formatSpawnTime("2026-09-05T11:52:30.000Z"), "2026-09-05 11:52 UTC");
    assert.equal(formatSpawnTime(undefined), undefined);
    assert.equal(formatSpawnTime(""), undefined);
    assert.equal(formatSpawnTime("nonsense"), undefined);
  });
});

describe("render(width)", () => {
  test("every rendered line fits the requested width, at several widths", () => {
    const h = harness({ question: "Should we rebase onto develop, or merge it in and keep both histories intact?" });
    h.delta("A long streamed answer that will certainly need wrapping at narrow widths. ".repeat(3));
    // No invalidate() between widths: review relay #1 I2 — the render cache is
    // keyed on width, so a resize with no other state change must still
    // re-wrap rather than replay the previous width's lines.
    for (const width of [20, 40, 80, 120, 20]) {
      for (const line of h.component.render(width)) {
        assert.ok(visibleWidth(line) <= width, `line ${JSON.stringify(line)} exceeds width ${width}`);
      }
    }
  });

  test("I2: the cache is reused only for the width it was built for", () => {
    const h = harness({ question: "A question long enough that its wrapping visibly differs between widths." });
    const wide = h.component.render(120);
    const narrow = h.component.render(20);
    assert.notDeepEqual(narrow, wide, "a narrower render must not replay the wide cache");
    assert.deepEqual(h.component.render(120), wide, "and re-rendering at the original width is stable");
  });

  test("I4: the header carries the thread's registration time when known, and omits it otherwise", () => {
    const withTime = harness({ createdAt: "2026-09-05T11:52:30.000Z" }).component.render(80).join("\n");
    assert.ok(withTime.includes("2026-09-05 11:52 UTC"), withTime);

    const withoutTime = harness().component.render(80).join("\n");
    assert.ok(!/opened /.test(withoutTime), "no timestamp line without a createdAt");

    const badTime = harness({ createdAt: "not-a-date" }).component.render(80).join("\n");
    assert.ok(!/opened /.test(badTime), "an unparseable timestamp must not break the header");
  });

  test("the header names the thread id and title (never let the two lead-voiced agents be confused)", () => {
    const h = harness();
    const text = h.component.render(80).join("\n");
    assert.ok(text.includes("q3"));
    assert.ok(text.includes("Rebase or merge?"));
  });

  test("the footer names /done and says Esc leaves the thread running", () => {
    const h = harness();
    const text = h.component.render(80).join("\n");
    assert.ok(text.includes(DONE_COMMAND));
    assert.match(text, /keeps running/);
  });

  test("the registered question is seeded into the transcript", () => {
    const h = harness({ question: "Rebase or merge?" });
    assert.ok(h.component.render(80).join("\n").includes("Rebase or merge?"));
  });

  test("streamed text_delta events land in the transcript and are echoed as they arrive", () => {
    const h = harness();
    h.delta("Merging ");
    h.delta("keeps history.");
    assert.ok(h.component.render(80).join("\n").includes("Merging keeps history."));
    assert.ok(h.renders >= 2, "each delta requests a render");
  });

  test("the input box echoes what the owner types, and backspace removes the last character", () => {
    const h = harness();
    h.type("merge");
    assert.ok(h.component.render(80).join("\n").includes("> merge"));
    h.component.handleInput("\x7f");
    assert.ok(h.component.render(80).join("\n").includes("> merg"));
  });

  test("the transcript is tail-truncated rather than growing without bound", () => {
    const h = harness();
    for (let i = 0; i < 60; i++) {
      h.delta(`line ${i}\n`);
      h.settle();
    }
    const rendered = h.component.render(80);
    assert.ok(rendered.length <= MAX_TRANSCRIPT_LINES + 8, `transcript grew unbounded: ${rendered.length} lines`);
    assert.ok(rendered.join("\n").includes("line 59"), "the newest turn must stay visible");
  });

  test("non-printable input is ignored rather than pasted in as control characters", () => {
    const h = harness();
    h.component.handleInput("\x1b[A");
    h.component.handleInput("\x00");
    h.type("ok");
    assert.ok(h.component.render(80).join("\n").includes("> ok"));
  });
});

describe("owner input routing", () => {
  test("a submitted line is delivered to the fork and echoed as the owner's turn", async () => {
    const h = harness();
    h.type("let's merge");
    h.enter();
    await flush();
    assert.deepEqual(h.sent, ["let's merge"]);
    assert.ok(h.component.render(80).join("\n").includes("you: let's merge"));
  });

  test("the input box is cleared after submitting", () => {
    const h = harness();
    h.type("hello");
    h.enter();
    assert.ok(!h.component.render(80).join("\n").includes("> hello"));
  });

  test("an empty submission sends nothing", () => {
    const h = harness();
    h.type("   ");
    h.enter();
    assert.deepEqual(h.sent, []);
  });

  test("a delivery failure becomes a transcript note, not an unhandled rejection", async () => {
    const h = harness();
    // Re-point the channel at a rejecting send by driving the component's own
    // failure path through a rejected promise.
    const failing = new OverlayChatComponent(
      { requestRender: () => {} },
      {
        title: "t",
        threadId: "q1",
        channel: {
          onEvent: () => () => {},
          isStreaming: () => false,
          send: async () => {
            throw new Error("child is gone");
          },
        },
        onDone: () => {},
      },
      () => {},
    );
    for (const char of "hi") failing.handleInput(char);
    failing.handleInput("\r");
    await flush();
    assert.ok(failing.render(80).join("\n").includes("child is gone"));
    assert.equal(h.sent.length, 0);
  });

  test("I3: a bracketed paste lands in the input box instead of being swallowed as an escape sequence", async () => {
    const h = harness();
    h.component.handleInput(`${PASTE_START}pasted decision text${PASTE_END}`);
    assert.ok(h.component.render(80).join("\n").includes("> pasted decision text"));
    h.enter();
    await flush();
    assert.deepEqual(h.sent, ["pasted decision text"]);
  });

  test("I3: a paste split across chunks is reassembled, and newlines inside it never submit", async () => {
    const h = harness();
    h.component.handleInput(`${PASTE_START}first line\n`);
    h.component.handleInput("second line");
    assert.deepEqual(h.sent, [], "a newline inside a paste is not an Enter");
    h.component.handleInput(` tail${PASTE_END}`);
    h.enter();
    await flush();
    assert.deepEqual(h.sent, ["first line second line tail"]);
  });

  test("I3: paste handling does not reopen the escape-sequence leak", () => {
    const h = harness();
    h.component.handleInput("\x1b[A");
    h.component.handleInput("\x1b[6~");
    assert.ok(!h.component.render(80).join("\n").includes("> ["), "arrow/function keys must still be dropped whole");
  });

  test("I3: text pasted at the cursor respects an existing cursor position", async () => {
    const h = harness();
    h.type("ab");
    h.component.handleInput("\x1b[D");
    h.component.handleInput(`${PASTE_START}XY${PASTE_END}`);
    h.enter();
    await flush();
    assert.deepEqual(h.sent, ["aXYb"]);
  });

  test("§5's prompt()-vs-steer() discriminator: a streaming fork is interrupted, an idle one is prompted", () => {
    assert.equal(resolveOwnerSendInterrupt(true), true, "streaming -> steer");
    assert.equal(resolveOwnerSendInterrupt(false), false, "idle/dormant -> prompt");
  });
});

describe("/done (the single fixed round-trip)", () => {
  test("sends exactly the fixed summary request, then takes the fork's next settled turn as the summary", async () => {
    const h = harness();
    h.type(DONE_COMMAND);
    h.enter();
    await flush();
    assert.deepEqual(h.sent, [buildDoneSummaryPrompt()]);
    assert.equal(h.summaries.length, 0, "no summary before the fork settles");

    h.delta("We agreed to merge and keep both histories.");
    h.settle();
    assert.deepEqual(h.summaries, ["We agreed to merge and keep both histories."]);
    assert.equal(h.doneCalls, 1, "the overlay exits itself once the summary arrives");
    assert.equal(h.unsubscribed, 1, "the event subscription is released on close");
  });

  test("/done is matched on the trimmed line only — it is never sent as a chat message", async () => {
    const h = harness();
    h.type(`  ${DONE_COMMAND}  `);
    h.enter();
    await flush();
    assert.deepEqual(h.sent, [buildDoneSummaryPrompt()]);
  });

  test("a settled turn producing no text still closes the thread, with an explicit placeholder", () => {
    const h = harness();
    h.type(DONE_COMMAND);
    h.enter();
    h.settle();
    assert.deepEqual(h.summaries, [EMPTY_SUMMARY_TEXT]);
  });

  test("ordinary turns before /done never trigger onDone", () => {
    const h = harness();
    h.delta("just chatting");
    h.settle();
    h.delta("still chatting");
    h.settle();
    assert.deepEqual(h.summaries, []);
    assert.equal(h.doneCalls, 0);
  });

  test("onDone fires at most once even if further settles arrive", () => {
    const h = harness();
    h.type(DONE_COMMAND);
    h.enter();
    h.delta("decided");
    h.settle();
    h.delta("more");
    h.settle();
    assert.deepEqual(h.summaries, ["decided"]);
    assert.equal(h.doneCalls, 1);
  });

  test("M11: a half-streamed pre-/done turn does not leak into the summary", () => {
    const h = harness();
    h.delta("partial answer that never settled");
    h.type(DONE_COMMAND);
    h.enter();
    h.delta("the actual summary");
    h.settle();
    assert.deepEqual(h.summaries, ["the actual summary"]);
  });

  test("the summary request text asks for a summary now, with no questions back", () => {
    const prompt = buildDoneSummaryPrompt();
    assert.match(prompt, /summary/i);
    assert.match(prompt, /owner ended the discussion/i);
  });
});

describe("closing without /done (§5: no effect on the fork)", () => {
  test("Escape closes the view, releases the subscription, and never reports a summary", () => {
    const h = harness();
    h.component.handleInput("\x1b");
    assert.equal(h.doneCalls, 1, "the overlay closes");
    assert.deepEqual(h.summaries, [], "no summary is injected — the thread stays open and reopenable");
    assert.equal(h.unsubscribed, 1);
  });

  test("close() from outside (a second /answer swapping overlays) behaves the same way", () => {
    const h = harness();
    h.component.close();
    assert.equal(h.doneCalls, 1);
    assert.deepEqual(h.summaries, []);
  });

  test("dispose() releases the fork event subscription", () => {
    const h = harness();
    h.component.dispose();
    assert.equal(h.unsubscribed, 1);
  });
});

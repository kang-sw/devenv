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
  isEscapeKey,
  BOX_MIN_WIDTH,
  PASTE_START,
  PASTE_END,
  OWNER_LINE_BG,
  stripAnsi,
  loadMarkdownRenderer,
  openOverlayChat,
  type ForkChannel,
  type OverlayTheme,
  type OverlayHandle,
  type TranscriptEntry,
} from "../src/overlay-chat.ts";
import { resolveOwnerSendInterrupt } from "../src/ask.ts";

function harness(
  options: {
    question?: string;
    streaming?: boolean;
    createdAt?: string;
    theme?: OverlayTheme;
    initialEntries?: TranscriptEntry[];
    renderMarkdown?: (text: string, width: number) => string[];
  } = {},
) {
  const sent: string[] = [];
  const listeners = new Set<(evt: unknown) => void>();
  let unsubscribed = 0;
  let renders = 0;
  const summaries: string[] = [];
  const transcripts: TranscriptEntry[][] = [];
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
      initialEntries: options.initialEntries,
      onTranscriptChange: (entries) => transcripts.push(entries),
      renderMarkdown: options.renderMarkdown,
    },
    () => {
      doneCalls += 1;
    },
    options.theme,
  );

  function type(text: string): void {
    for (const char of text) component.handleInput(char);
  }

  return {
    component,
    sent,
    summaries,
    transcripts,
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

  test("the view is a rounded box: with a theme, every line is exactly the requested width and the corners are present", () => {
    const fgCalls: string[] = [];
    // Identity stub: records the color requests but returns the text
    // unchanged, so the local `visibleWidth` still measures the padded lines.
    const theme: OverlayTheme = {
      fg: (color, text) => {
        fgCalls.push(color);
        return text;
      },
    };
    const h = harness({ question: "Should we rebase onto develop, or merge it in and keep both histories intact?", createdAt: "2026-09-05T11:52:30.000Z", theme });
    h.delta("A long streamed answer that will certainly need wrapping at narrow widths. ".repeat(3));
    h.type("한글 input");
    for (const width of [20, 40, 80, 120]) {
      const lines = h.component.render(width);
      for (const line of lines) {
        assert.equal(visibleWidth(line), width, `line ${JSON.stringify(line)} is not exactly width ${width}`);
      }
      assert.ok(lines[0].startsWith("╭") && lines[0].endsWith("╮"), `top border at width ${width}: ${lines[0]}`);
      assert.ok(lines.at(-1)!.startsWith("╰") && lines.at(-1)!.endsWith("╯"), `bottom border at width ${width}: ${lines.at(-1)}`);
      for (const line of lines.slice(1, -1)) {
        assert.ok(line.startsWith("│ ") && line.endsWith(" │"), `interior row at width ${width}: ${JSON.stringify(line)}`);
      }
    }
    assert.ok(fgCalls.includes("border"), "the border is colored with the theme's border color");
    assert.ok(fgCalls.includes("dim"), "the footer is colored with the theme's dim color");
  });

  test("without a theme the box is drawn unstyled, and tiny widths degrade to bare content that still fits", () => {
    const h = harness({ question: "Rebase or merge?" });
    const lines = h.component.render(40);
    assert.ok(lines[0].startsWith("╭") && lines.at(-1)!.startsWith("╰"));
    for (const line of lines) assert.equal(visibleWidth(line), 40);

    for (const width of [1, 2, BOX_MIN_WIDTH - 1]) {
      for (const line of h.component.render(width)) {
        assert.ok(visibleWidth(line) <= width, `line ${JSON.stringify(line)} exceeds width ${width}`);
        assert.ok(!line.includes("│"), "no box below the minimum box width");
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

  test("C2: summarizeOnDone:false closes immediately — nothing is sent to the fork and no settle is awaited", async () => {
    const summaries: string[] = [];
    let doneCalls = 0;
    const sent: string[] = [];
    const component = new OverlayChatComponent(
      { requestRender: () => {} },
      {
        title: "t",
        threadId: "q1",
        summarizeOnDone: false,
        channel: {
          onEvent: () => () => {},
          isStreaming: () => false,
          async send(text) {
            sent.push(text);
          },
        },
        onDone: (summary) => summaries.push(summary),
      },
      () => {
        doneCalls += 1;
      },
    );

    for (const char of DONE_COMMAND) component.handleInput(char);
    component.handleInput("\r");
    await flush();

    assert.deepEqual(sent, [], "a live task fork must not be asked to summarize mid-task");
    assert.deepEqual(summaries, [""], "onDone fires immediately, with no summary to carry");
    assert.equal(doneCalls, 1, "the overlay closes on the spot");
  });

  test("the summary request text asks for a summary now, with no questions back", () => {
    const prompt = buildDoneSummaryPrompt();
    assert.match(prompt, /summary/i);
    assert.match(prompt, /owner ended the discussion/i);
  });
});

describe("isEscapeKey", () => {
  test("accepts bare ESC, the kitty CSI-u forms with no modifier, and the modifyOtherKeys form", () => {
    for (const data of ["\x1b", "\x1b[27u", "\x1b[27;1u", "\x1b[27;1:1u", "\x1b[27;1:3u", "\x1b[27::27;1u", "\x1b[27;1;27~"]) {
      assert.equal(isEscapeKey(data), true, JSON.stringify(data));
    }
  });

  test("rejects modified Escapes, other keys, and text", () => {
    for (const data of ["\x1b[27;2u", "\x1b[27;5u", "\x1b[27;2:1u", "\x1b[27;2;27~", "\x1b[97u", "\x1b[A", "\x1b[200~", "", "a", "\x1b\x1b"]) {
      assert.equal(isEscapeKey(data), false, JSON.stringify(data));
    }
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

  test("Escape under the kitty keyboard protocol / modifyOtherKeys closes the view too (dogfood: Pi 0.84.4 sends CSI-u)", () => {
    for (const data of ["\x1b[27u", "\x1b[27;1u", "\x1b[27;1:1u", "\x1b[27;1;27~"]) {
      const h = harness();
      h.component.handleInput(data);
      assert.equal(h.doneCalls, 1, `${JSON.stringify(data)} closes the overlay exactly once`);
      assert.deepEqual(h.summaries, []);
      assert.equal(h.unsubscribed, 1);
    }
  });

  test("a modified Escape (Shift+Esc) is dropped whole: the view stays open and nothing leaks into the input", () => {
    const h = harness();
    h.component.handleInput("\x1b[27;2u");
    assert.equal(h.doneCalls, 0, "the overlay stays open");
    h.type("ok");
    assert.ok(h.component.render(80).join("\n").includes("> ok"), "the input box holds only what was typed");
    assert.ok(!h.component.render(80).join("\n").includes("27;2u"), "the sequence's tail is not pasted in");
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

describe("transcript persistence (dogfood: Esc then /answer must not open an empty view)", () => {
  test("every append is reported with the full transcript, in order; the streaming tail never is", () => {
    const h = harness({ question: "Rebase or merge?" });
    assert.deepEqual(h.transcripts, [[{ who: "note", text: "Rebase or merge?" }]], "the seeded question is an append too");
    h.delta("partial");
    assert.equal(h.transcripts.length, 1, "a text_delta is not a transcript change");
    h.settle();
    h.type("merge");
    h.enter();
    assert.deepEqual(h.transcripts.at(-1), [
      { who: "note", text: "Rebase or merge?" },
      { who: "thread", text: "partial" },
      { who: "you", text: "merge" },
    ]);
  });

  test("a restored transcript is rendered and the question note is NOT seeded a second time", () => {
    const restored: TranscriptEntry[] = [
      { who: "note", text: "Rebase or merge?" },
      { who: "you", text: "merge" },
      { who: "thread", text: "Merging keeps both histories." },
    ];
    const h = harness({ question: "Rebase or merge?", initialEntries: restored });
    const text = h.component.render(80).join("\n");
    assert.ok(text.includes("you: merge"));
    assert.ok(text.includes("Merging keeps both histories."));
    assert.equal(text.split("Rebase or merge?").length - 1, 2, "once in the header title, once as the restored note — not a third time");
    assert.deepEqual(h.transcripts, [], "restoring is not an append");
  });

  test("an empty initial transcript behaves like none: the question is seeded", () => {
    const h = harness({ question: "Rebase or merge?", initialEntries: [] });
    assert.deepEqual(h.transcripts, [[{ who: "note", text: "Rebase or merge?" }]]);
  });

  test("the reported array is a copy — later appends do not mutate what was handed out", () => {
    const h = harness({ question: "q" });
    const first = h.transcripts[0];
    h.type("a");
    h.enter();
    assert.equal(first.length, 1);
  });
});

describe("owner lines (dogfood: visually distinct from thread text)", () => {
  /** Identity theme that marks bg-wrapped spans so the test can see the block boundaries; widths stay measurable. */
  function markingTheme() {
    const bgCalls: Array<{ color: string; text: string }> = [];
    const theme: OverlayTheme = {
      fg: (_color, text) => text,
      bg: (color, text) => {
        bgCalls.push({ color, text });
        return `«${text}»`;
      },
    };
    return { theme, bgCalls };
  }

  test("with a theme that can paint a background, an owner turn is a solid block: padded to the interior with one space of inner padding, no `you:` prefix", () => {
    const { theme, bgCalls } = markingTheme();
    const h = harness({ theme });
    h.type("let's merge");
    h.enter();
    h.delta("Merging keeps history.");
    h.settle();
    const lines = h.component.render(40);
    const inner = 40 - 4;
    const ownerLines = lines.filter((line) => line.includes("«"));
    assert.equal(ownerLines.length, 1);
    assert.ok(!ownerLines[0].includes("you:"), "the prefix is replaced by the background");
    assert.equal(bgCalls.length, 1);
    assert.equal(bgCalls[0].color, OWNER_LINE_BG);
    assert.equal(bgCalls[0].text, ` let's merge${" ".repeat(inner - 2 - "let's merge".length)} `, "the whole interior is painted, text inset by one space");
    assert.ok(ownerLines[0].startsWith("│ «") && ownerLines[0].endsWith("» │"));
    const threadLine = lines.find((line) => line.includes("Merging keeps history."))!;
    assert.ok(!threadLine.includes("«"), "thread text stays unpadded and unpainted");
    assert.ok(threadLine.startsWith("│ Merging"), "thread text is not inset");
  });

  test("a long owner turn wraps inside the block, every wrapped row painted to the full interior", () => {
    const { theme, bgCalls } = markingTheme();
    const h = harness({ theme });
    h.type("a decision long enough to wrap onto several rows of a narrow overlay box");
    h.enter();
    h.component.render(30);
    assert.ok(bgCalls.length >= 2);
    for (const call of bgCalls) {
      assert.equal(call.text.length, 30 - 4, `every painted row spans the interior: ${JSON.stringify(call.text)}`);
      assert.ok(call.text.startsWith(" ") && call.text.endsWith(" "));
    }
  });

  test("with a theme that has only fg (or no theme), the `you:` prefix fallback is kept", () => {
    const fgOnly = harness({ theme: { fg: (_c, t) => t } });
    fgOnly.type("merge");
    fgOnly.enter();
    assert.ok(fgOnly.component.render(80).join("\n").includes("you: merge"));
    const bare = harness();
    bare.type("merge");
    bare.enter();
    assert.ok(bare.component.render(80).join("\n").includes("you: merge"));
  });

  test("styled lines still measure and pad correctly: visibleWidth ignores ANSI sequences", () => {
    assert.equal(visibleWidth("\x1b[44m padded \x1b[0m"), 8);
    assert.equal(visibleWidth("\x1b]8;;http://x\x07link\x1b]8;;\x07"), 4);
    assert.equal(stripAnsi("\x1b[1mbold\x1b[22m"), "bold");
    const theme: OverlayTheme = { fg: (_c, t) => `\x1b[2m${t}\x1b[22m`, bg: (_c, t) => `\x1b[44m${t}\x1b[49m` };
    const h = harness({ theme });
    h.type("merge");
    h.enter();
    for (const line of h.component.render(50)) assert.equal(visibleWidth(line), 50, JSON.stringify(line));
  });
});

describe("markdown rendering for thread text", () => {
  test("a supplied renderMarkdown is used for thread entries and the streaming tail only — never for owner lines or notes", () => {
    const calls: Array<{ text: string; width: number }> = [];
    const renderMarkdown = (text: string, width: number): string[] => {
      calls.push({ text, width });
      return [`MD<${text}>`];
    };
    const h = harness({ question: "Rebase or merge?", renderMarkdown });
    h.type("merge");
    h.enter();
    h.delta("**Merging** keeps history.");
    const streaming = h.component.render(80).join("\n");
    assert.ok(streaming.includes("MD<**Merging** keeps history.>"), "the streaming tail goes through the renderer");
    h.settle();
    const settled = h.component.render(80).join("\n");
    assert.ok(settled.includes("MD<**Merging** keeps history.>"));
    assert.ok(settled.includes("· Rebase or merge?"), "notes keep their plain rendering");
    assert.ok(settled.includes("you: merge"), "owner lines keep their plain rendering");
    assert.deepEqual(
      calls.map((c) => c.text),
      ["**Merging** keeps history.", "**Merging** keeps history."],
    );
    assert.ok(calls.every((c) => c.width === 80 - 4), "rendered to the box interior");
  });

  test("without a renderer, thread text falls back to plain wrapping", () => {
    const h = harness();
    h.delta("**Merging** keeps history.");
    h.settle();
    assert.ok(h.component.render(80).join("\n").includes("**Merging** keeps history."));
  });

  test("a renderer line wider than the interior is re-wrapped from its plain text, so the box stays aligned", () => {
    const wide = "x".repeat(100);
    const h = harness({ theme: { fg: (_c, t) => t }, renderMarkdown: () => [`\x1b[1m${wide}\x1b[22m`, "short"] });
    h.delta("anything");
    h.settle();
    const lines = h.component.render(40);
    for (const line of lines) assert.equal(visibleWidth(line), 40, JSON.stringify(line));
    assert.ok(lines.join("\n").includes("short"));
    assert.ok(!lines.join("\n").includes("\x1b[1m"), "the over-wide styled line was replaced by its plain re-wrap");
  });

  test("a throwing or non-array renderer degrades to plain wrapping rather than breaking the view", () => {
    const throwing = harness({
      renderMarkdown: () => {
        throw new Error("renderer broke");
      },
    });
    throwing.delta("still visible");
    throwing.settle();
    assert.ok(throwing.component.render(80).join("\n").includes("still visible"));

    const bogus = harness({ renderMarkdown: () => "nope" as unknown as string[] });
    bogus.delta("also visible");
    bogus.settle();
    assert.ok(bogus.component.render(80).join("\n").includes("also visible"));
  });

  test("loadMarkdownRenderer resolves to undefined under node --test (pi-tui is not resolvable here) instead of throwing", async () => {
    assert.equal(await loadMarkdownRenderer(), undefined);
  });

  test("openOverlayChat falls back to plain thread text and hands back an OverlayHandle", async () => {
    let handle: OverlayHandle | undefined;
    let built: OverlayChatComponent | undefined;
    const listeners = new Set<(evt: unknown) => void>();
    const ctx = {
      ui: {
        custom<T>(factory: (tui: { requestRender(): void }, theme: undefined, kb: unknown, done: (r: T) => void) => OverlayChatComponent) {
          return new Promise<T>((resolve) => {
            built = factory({ requestRender: () => {} }, undefined, undefined, resolve);
          });
        },
      },
    };
    const summaries: string[] = [];
    const opened = openOverlayChat(ctx, {
      title: "t",
      threadId: "q1",
      channel: {
        onEvent: (l) => {
          listeners.add(l);
          return () => listeners.delete(l);
        },
        isStreaming: () => false,
        send: async () => {},
      },
      onDone: (summary) => summaries.push(summary),
      onOpened: (h) => {
        handle = h;
      },
    });
    await flush();
    assert.ok(handle && built);
    for (const l of listeners) l({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "**plain**" } });
    assert.ok(built!.render(60).join("\n").includes("**plain**"), "no renderer under node --test: markdown source shows as-is");
    handle!.closeWithSummary("decided");
    await opened;
    assert.deepEqual(summaries, ["decided"]);
  });
});

describe("closeWithSummary (the fork ended the thread itself)", () => {
  test("fires onDone with the supplied text, appends it to the transcript, closes the view, and never sends the /done prompt", async () => {
    const h = harness({ question: "q" });
    h.delta("half-streamed turn");
    h.component.closeWithSummary("Decided: merge.");
    await flush();
    assert.deepEqual(h.sent, [], "no summary turn is requested");
    assert.deepEqual(h.summaries, ["Decided: merge."]);
    assert.equal(h.doneCalls, 1);
    assert.equal(h.unsubscribed, 1);
    assert.deepEqual(h.transcripts.at(-1)!.at(-1), { who: "thread", text: "Decided: merge." }, "the decision is persisted for a later reopen");
  });

  test("an empty summary (fork-raised detach) appends nothing and still fires onDone with the empty string", () => {
    const h = harness({ question: "q" });
    h.component.closeWithSummary("");
    assert.deepEqual(h.summaries, [""]);
    assert.equal(h.transcripts.length, 1, "only the seeded question");
    assert.equal(h.doneCalls, 1);
  });

  test("is a no-op once the view has finished, and a later settle cannot fire onDone again", () => {
    const h = harness();
    h.type(DONE_COMMAND);
    h.enter();
    // The fork answered the summary request with its own final report instead
    // of a text turn: the external close wins, the pending /done is dropped.
    h.component.closeWithSummary("via report");
    h.delta("late text");
    h.settle();
    h.component.closeWithSummary("again");
    assert.deepEqual(h.summaries, ["via report"]);
    assert.equal(h.doneCalls, 1);
  });
});

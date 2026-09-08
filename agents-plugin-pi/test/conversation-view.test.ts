/**
 * Unit tests for `conversation-view.ts`: the Phase 1 shared
 * `ConversationViewComponent` (`260908-feat-ws-pi-conversation-view-component`).
 *
 * Every test drives `render(width)`/`handleInput(data)` directly against a
 * fake `ConversationChannel` and a fake `tui` (`requestRender` only, per the
 * plan) — no live TTY, no real Pi host. `Editor` is the one `pi-tui`
 * primitive faked here (it needs the full `TUI` surface the fake `tui` does
 * not implement); `ScrollView`/`Markdown`/`Text` use the real `pi-tui`
 * classes through the component's default `primitives`.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  ConversationViewComponent,
  DONE_COMMAND,
  isEscapeKey,
  type ChildLiveness,
  type ConversationChannel,
  type ConversationItem,
  type EditorLike,
} from "../src/conversation-view.ts";
import { visibleWidth } from "../src/text-width.ts";

const WIDTHS = [40, 80, 120];

/** Fake `tui`: `requestRender` only, per the plan's offline test tier. */
function fakeTui(): { requestRender: () => void; renderCount: number } {
  const state = { requestRender: () => {}, renderCount: 0 };
  state.requestRender = () => {
    state.renderCount += 1;
  };
  return state;
}

/** Fake `ConversationChannel`: a mutable `liveness()`, a recorded `send()`, and a manually-fired event stream. */
function fakeChannel(initialLiveness: ChildLiveness = "settled"): {
  channel: ConversationChannel;
  fire: (evt: unknown) => void;
  sent: string[];
  setLiveness: (next: ChildLiveness) => void;
} {
  let liveness = initialLiveness;
  const sent: string[] = [];
  let listener: ((evt: unknown) => void) | undefined;
  const channel: ConversationChannel = {
    onEvent: (l) => {
      listener = l;
      return () => {
        listener = undefined;
      };
    },
    liveness: () => liveness,
    send: async (text) => {
      sent.push(text);
    },
  };
  return {
    channel,
    fire: (evt) => listener?.(evt),
    sent,
    setLiveness: (next) => {
      liveness = next;
    },
  };
}

/** Fake `Editor`: records typed input and fires `onSubmit` on Enter, exactly like the real `pi-tui` `Editor` does for this component's purposes. */
class FakeEditor implements EditorLike {
  text = "";
  onSubmit: ((text: string) => void) | undefined;
  received: string[] = [];

  render(width: number): string[] {
    return [`[editor:${this.text}]`.padEnd(width).slice(0, width)];
  }

  invalidate(): void {}

  handleInput(data: string): void {
    this.received.push(data);
    if (data === "\r" || data === "\n") {
      this.onSubmit?.(this.text);
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

function assertWidthBounded(lines: string[], width: number, label: string): void {
  for (const line of lines) {
    assert.ok(visibleWidth(line) <= width, `${label}: line exceeds ${width} columns: ${JSON.stringify(line)}`);
  }
}

const ALL_ITEM_KINDS: ConversationItem[] = [
  { kind: "owner", text: "hello" },
  { kind: "lead-message", text: "hi back" },
  { kind: "note", text: "a system note" },
  { kind: "tool-call", name: "ws-read", args: { path: "a.txt" } },
  { kind: "tool-result", name: "ws-read", text: JSON.stringify({ ok: true, lines: 3 }) },
];

describe("ConversationViewComponent — item kinds and width invariant", () => {
  test("every item kind renders and stays width-bounded at 40/80/120 columns", () => {
    for (const width of WIDTHS) {
      const { channel } = fakeChannel();
      const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
      const lines = view.render(width);
      assertWidthBounded(lines, width, `width ${width}`);
      // Every item contributed at least one line.
      assert.ok(lines.length >= ALL_ITEM_KINDS.length);
    }
  });

  test("stays width-bounded with tool items expanded, at 40/80/120 columns", () => {
    for (const width of WIDTHS) {
      const { channel } = fakeChannel();
      const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
      // Expand every expandable item (tool-call, tool-result).
      view.handleInput("\x0f");
      const lines = view.render(width);
      assertWidthBounded(lines, width, `width ${width} (expanded)`);
    }
  });
});

describe("ConversationViewComponent — header hint", () => {
  test("renders the key-hint line exactly once regardless of transcript size", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
    const lines = view.render(80);
    const hintOccurrences = lines.filter((l) => l.includes("Ctrl+O")).length;
    assert.equal(hintOccurrences, 1);
  });
});

describe("ConversationViewComponent — lead-message label", () => {
  test("a lead-message item renders a distinct 'lead:' label line", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: [{ kind: "lead-message", text: "decided: ship it" }] });
    const lines = view.render(80);
    assert.ok(lines.some((l) => l.trim() === "lead:"));
    assert.ok(lines.some((l) => l.includes("decided: ship it")));
  });

  test("a streaming (not-yet-settled) turn also renders under the 'lead:' label", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "partial thought" } });
    const lines = view.render(80);
    assert.ok(lines.some((l) => l.trim() === "lead:"));
    assert.ok(lines.some((l) => l.includes("partial thought")));
  });
});

describe("ConversationViewComponent — liveness banner", () => {
  test("'working…' appears only while liveness() is 'running'", () => {
    const { channel, setLiveness } = fakeChannel("running");
    const view = new ConversationViewComponent(fakeTui(), { channel });
    assert.ok(view.render(80).some((l) => l.includes("working…")));

    setLiveness("settled");
    assert.ok(!view.render(80).some((l) => l.includes("working…")));
  });

  test("'idle-awaiting-owner' renders a distinctly more prominent banner than 'settled'", () => {
    const { channel, setLiveness } = fakeChannel("idle-awaiting-owner");
    const view = new ConversationViewComponent(fakeTui(), { channel });
    const idleLines = view.render(80);
    assert.ok(idleLines.some((l) => l.includes("AWAITING OWNER")));
    assert.ok(!idleLines.some((l) => l.includes("working…")));

    setLiveness("settled");
    const settledLines = view.render(80);
    assert.ok(!settledLines.some((l) => l.includes("AWAITING OWNER")));
    assert.ok(!settledLines.some((l) => l.includes("working…")));
  });

  test("liveness() is read fresh every render, never cached", () => {
    const { channel, setLiveness } = fakeChannel("settled");
    const view = new ConversationViewComponent(fakeTui(), { channel });
    view.render(80); // prime any accidental cache
    setLiveness("running");
    assert.ok(view.render(80).some((l) => l.includes("working…")));
    setLiveness("idle-awaiting-owner");
    assert.ok(view.render(80).some((l) => l.includes("AWAITING OWNER")));
  });
});

describe("ConversationViewComponent — collapse/expand and key precedence", () => {
  test("Tab cycles focus only among expandable items, wrapping around", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
    // ALL_ITEM_KINDS: [owner, lead-message, note, tool-call(3), tool-result(4)] — only 3 and 4 are expandable.
    view.handleInput("\t");
    assert.equal(view.getFocusedIndex(), 3);
    view.handleInput("\t");
    assert.equal(view.getFocusedIndex(), 4);
    view.handleInput("\t");
    assert.equal(view.getFocusedIndex(), 3, "wraps back to the first expandable item");
  });

  test("Shift+Tab cycles focus backwards", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
    view.handleInput("\x1b[Z");
    assert.equal(view.getFocusedIndex(), 4, "wraps to the last expandable item going backwards from nothing focused");
    view.handleInput("\x1b[Z");
    assert.equal(view.getFocusedIndex(), 3);
  });

  test("Space toggles only the focused item", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
    view.handleInput("\t"); // focus index 3
    assert.equal(view.isItemExpanded(3), false);
    view.handleInput(" ");
    assert.equal(view.isItemExpanded(3), true);
    assert.equal(view.isItemExpanded(4), false);
    view.handleInput(" ");
    assert.equal(view.isItemExpanded(3), false);
  });

  test("Ctrl+O toggles expand/collapse for every expandable item at once", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
    view.handleInput("\x0f");
    assert.equal(view.isItemExpanded(3), true);
    assert.equal(view.isItemExpanded(4), true);
    view.handleInput("\x0f");
    assert.equal(view.isItemExpanded(3), false);
    assert.equal(view.isItemExpanded(4), false);
  });

  test("expanding a tool-call item reveals its full args as body lines", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: [{ kind: "tool-call", name: "ws-read", args: { path: "a.txt", limit: 10 } }],
    });
    const collapsed = view.render(80);
    view.handleInput("\t");
    view.handleInput(" ");
    const expanded = view.render(80);
    assert.ok(expanded.length > collapsed.length);
    assert.ok(expanded.some((l) => l.includes("path")));
  });

  test("a non-object tool-call (no expandable args) never enters focus cycling", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: [{ kind: "tool-call", name: "ws-noop", args: undefined }] });
    view.handleInput("\t");
    assert.equal(view.getFocusedIndex(), undefined);
  });

  test("precedence: in interactive mode, Ctrl+O/Esc/Ctrl+C are intercepted before reaching the Editor", () => {
    const { channel } = fakeChannel();
    const editor = new FakeEditor();
    let escaped = false;
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: ALL_ITEM_KINDS,
      primitives: { Editor: class {
        constructor() {
          return editor as never;
        }
      } as never },
      onEscape: () => {
        escaped = true;
      },
    });
    view.setMode("interactive");
    view.handleInput("\x0f"); // Ctrl+O: expand-all, not typed
    view.handleInput("\x03"); // Ctrl+C: swallowed, not typed
    view.handleInput("\x1b"); // Esc: onEscape, not typed
    assert.deepEqual(editor.received, []);
    assert.equal(escaped, true);
    assert.equal(view.isItemExpanded(3), true);
  });

  test("precedence: in view mode, Space toggles the focused item rather than doing nothing", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
    view.handleInput("\t");
    view.handleInput(" ");
    assert.equal(view.isItemExpanded(3), true);
  });
});

describe("ConversationViewComponent — view/interactive mode differences", () => {
  test("the Editor only renders once the view is 'interactive'", () => {
    const { channel } = fakeChannel();
    const editor = new FakeEditor();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      primitives: { Editor: class {
        constructor() {
          return editor as never;
        }
      } as never },
    });
    assert.ok(!view.render(80).some((l) => l.includes("[editor:")));
    view.setMode("interactive");
    assert.ok(view.render(80).some((l) => l.includes("[editor:")));
  });

  test("Enter in 'view' mode calls onEnter (there is no Editor to submit)", () => {
    const { channel } = fakeChannel();
    let entered = 0;
    const view = new ConversationViewComponent(fakeTui(), { channel, onEnter: () => {
      entered += 1;
    } });
    view.handleInput("\r");
    assert.equal(entered, 1);
  });

  test("Enter in 'interactive' mode is forwarded to the Editor instead of calling onEnter", () => {
    const { channel } = fakeChannel();
    const editor = new FakeEditor();
    let entered = 0;
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      primitives: { Editor: class {
        constructor() {
          return editor as never;
        }
      } as never },
      onEnter: () => {
        entered += 1;
      },
    });
    view.setMode("interactive");
    editor.setText("hello");
    view.handleInput("\r");
    assert.equal(entered, 0);
    assert.deepEqual(view.getItems(), [{ kind: "owner", text: "hello" }]);
  });
});

describe("ConversationViewComponent — /done, Esc, and submit routing", () => {
  function buildInteractive(): { view: ConversationViewComponent; editor: FakeEditor; channel: ReturnType<typeof fakeChannel>; done: number; escape: number } {
    const fake = fakeChannel();
    const editor = new FakeEditor();
    let done = 0;
    let escape = 0;
    const view = new ConversationViewComponent(fakeTui(), {
      channel: fake.channel,
      primitives: { Editor: class {
        constructor() {
          return editor as never;
        }
      } as never },
      onDone: () => {
        done += 1;
      },
      onEscape: () => {
        escape += 1;
      },
    });
    view.setMode("interactive");
    return { view, editor, channel: fake, get done() { return done; }, get escape() { return escape; } } as never;
  }

  test("typing '/done' and submitting calls onDone and sends nothing to the channel", () => {
    const { view, editor, channel } = buildInteractive();
    for (const ch of DONE_COMMAND) view.handleInput(ch);
    view.handleInput("\r");
    assert.equal(editor.getText(), "");
    assert.deepEqual(channel.sent, []);
  });

  test("submitting ordinary text appends an owner item and sends it to the channel", () => {
    const { view, editor, channel } = buildInteractive();
    for (const ch of "ping") view.handleInput(ch);
    view.handleInput("\r");
    assert.deepEqual(view.getItems(), [{ kind: "owner", text: "ping" }]);
    assert.deepEqual(channel.sent, ["ping"]);
  });

  test("submitting empty/whitespace-only text does nothing", () => {
    const { view, channel } = buildInteractive();
    view.handleInput(" ");
    view.handleInput("\r");
    assert.deepEqual(view.getItems(), []);
    assert.deepEqual(channel.sent, []);
  });

  test("Esc calls onEscape in both view and interactive mode", () => {
    const { channel } = fakeChannel();
    let escapeCount = 0;
    const view = new ConversationViewComponent(fakeTui(), { channel, onEscape: () => {
      escapeCount += 1;
    } });
    view.handleInput("\x1b");
    assert.equal(escapeCount, 1);
    view.setMode("interactive");
    view.handleInput("\x1b");
    assert.equal(escapeCount, 2);
  });

  test("isEscapeKey recognizes the kitty CSI-u and modifyOtherKeys encodings, not just bare ESC", () => {
    assert.equal(isEscapeKey("\x1b"), true);
    assert.equal(isEscapeKey("\x1b[27u"), true);
    assert.equal(isEscapeKey("\x1b[27;1u"), true);
    assert.equal(isEscapeKey("\x1b[27;1;27~"), true);
    assert.equal(isEscapeKey("\x1b[27;2u"), false, "a modified Escape (Shift+Esc) is not a plain Escape");
    assert.equal(isEscapeKey("a"), false);
  });
});

describe("ConversationViewComponent — Ctrl+C is swallowed in both modes", () => {
  test("view mode: no callback fires, no item is added, no exception", () => {
    const { channel } = fakeChannel();
    let calls = 0;
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      onEnter: () => {
        calls += 1;
      },
      onEscape: () => {
        calls += 1;
      },
    });
    assert.doesNotThrow(() => view.handleInput("\x03"));
    assert.equal(calls, 0);
    assert.deepEqual(view.getItems(), []);
  });

  test("interactive mode: the Editor never receives it", () => {
    const { channel } = fakeChannel();
    const editor = new FakeEditor();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      primitives: { Editor: class {
        constructor() {
          return editor as never;
        }
      } as never },
    });
    view.setMode("interactive");
    view.handleInput("\x03");
    assert.deepEqual(editor.received, []);
  });
});

describe("ConversationViewComponent — setMode raise-only and state preservation", () => {
  test("setMode('interactive') then setMode('view') stays interactive (raise-only)", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    view.setMode("interactive");
    view.setMode("view");
    assert.equal(view.getMode(), "interactive");
  });

  test("a repeated setMode('interactive') call preserves the existing Editor's typed text and the transcript", () => {
    const { channel } = fakeChannel();
    const editor = new FakeEditor();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      primitives: { Editor: class {
        constructor() {
          return editor as never;
        }
      } as never },
      initialItems: [{ kind: "note", text: "seeded" }],
    });
    view.setMode("interactive");
    editor.setText("draft in progress");
    view.setMode("interactive"); // no-op: already interactive
    assert.equal(editor.getText(), "draft in progress", "the same Editor instance/state survives a redundant setMode call");
    assert.deepEqual(view.getItems(), [{ kind: "note", text: "seeded" }]);
  });
});

describe("ConversationViewComponent — channel events", () => {
  test("agent_start triggers a render even with no accompanying text delta", () => {
    const tui = fakeTui();
    const { channel, fire } = fakeChannel();
    new ConversationViewComponent(tui, { channel });
    const before = tui.renderCount;
    fire({ type: "agent_start" });
    assert.ok(tui.renderCount > before);
  });

  test("text_delta accumulates, and agent_settled commits it as a lead-message item", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "part one " } });
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "part two" } });
    fire({ type: "agent_settled" });
    assert.deepEqual(view.getItems(), [{ kind: "lead-message", text: "part one part two" }]);
  });

  test("agent_settled with no accumulated text commits nothing", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "agent_settled" });
    assert.deepEqual(view.getItems(), []);
  });
});

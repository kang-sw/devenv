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
import { completedTextPreview, logicalPreview, yamlContainerDisplay, yamlInputPreview } from "../src/tool-result-render.ts";
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

/**
 * Fake `Editor`: records typed input, fires `onSubmit` on a real Enter, and
 * models the real `pi-tui` `Editor`'s bracketed-paste handling (`\x1b[200~
 * ... \x1b[201~`) closely enough to prove a paste's embedded newlines never
 * trigger a submit — only a standalone Enter does. See
 * `node_modules/.../pi-tui/dist/components/editor.js`'s own `pasteBuffer`/
 * `isInPaste` handling, which this mirrors at the shape level.
 */
class FakeEditor implements EditorLike {
  text = "";
  onSubmit: ((text: string) => void) | undefined;
  received: string[] = [];
  private isInPaste = false;
  private pasteBuffer = "";

  render(width: number): string[] {
    return [`[editor:${this.text}]`.padEnd(width).slice(0, width)];
  }

  invalidate(): void {}

  handleInput(data: string): void {
    this.received.push(data);
    let rest = data;
    if (!this.isInPaste && rest.includes("\x1b[200~")) {
      this.isInPaste = true;
      this.pasteBuffer = "";
      rest = rest.replace("\x1b[200~", "");
    }
    if (this.isInPaste) {
      this.pasteBuffer += rest;
      const endIndex = this.pasteBuffer.indexOf("\x1b[201~");
      if (endIndex === -1) return; // still buffering the paste
      const pasted = this.pasteBuffer.slice(0, endIndex);
      this.text += pasted; // embedded newlines land in the text verbatim, never as a submit
      const remaining = this.pasteBuffer.slice(endIndex + "\x1b[201~".length);
      this.isInPaste = false;
      this.pasteBuffer = "";
      if (remaining.length > 0) this.handleInput(remaining);
      return;
    }
    if (rest === "\r" || rest === "\n") {
      this.onSubmit?.(this.text);
      return;
    }
    this.text += rest;
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

/** id=c1 for the call, matching the id=c1 result so a future correlation pass (Phase 2+) has something to key on. */
const ALL_ITEM_KINDS: ConversationItem[] = [
  { kind: "user", text: "hello" },
  { kind: "assistant", text: "hi back" },
  { kind: "lead-message", text: "the lead sent this directly to the child" },
  { kind: "note", text: "a system note" },
  { kind: "tool-call", id: "c1", name: "ws-read", args: { path: "a.txt" } },
  { kind: "tool-result", id: "c1", name: "ws-read", content: JSON.stringify({ ok: true, lines: 3 }) },
];
// Expandable indices in ALL_ITEM_KINDS: 4 (tool-call) and 5 (tool-result).
// Tab/Shift+Tab traverse newest-first, i.e. 5 before 4.

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

describe("ConversationViewComponent — headerHint option", () => {
  test("a consumer-supplied headerHint replaces the default hint and renders exactly once at 40/80/120 columns", () => {
    const supplied = "Esc: close view (thread stays open) · /done: end thread";
    for (const width of WIDTHS) {
      const { channel } = fakeChannel();
      const view = new ConversationViewComponent(fakeTui(), { channel, headerHint: supplied });
      const lines = view.render(width);
      const occurrences = lines.filter((l) => l.includes("close view")).length;
      assert.equal(occurrences, 1, `width ${width}`);
      assert.ok(!lines.some((l) => l.includes("Ctrl+O: expand all")), `width ${width}: the internal default hint must not also appear`);
    }
  });

  test("omitting headerHint keeps the built-in mode-dependent default", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    assert.ok(view.render(80).some((l) => l.includes("Ctrl+O")));
  });
});

describe("ConversationViewComponent — lead-message vs assistant labeling", () => {
  test("a lead-message item (the LEAD messaging the child) renders under a distinct 'lead:' label", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: [{ kind: "lead-message", text: "decided: ship it" }] });
    const lines = view.render(80);
    assert.ok(lines.some((l) => l.trim() === "lead:"));
    assert.ok(lines.some((l) => l.includes("decided: ship it")));
  });

  test("a settled child turn commits and renders as 'assistant' — Markdown, never the 'lead:' label", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "hello from the child" } });
    fire({ type: "agent_settled" });
    assert.deepEqual(view.getItems(), [{ kind: "assistant", text: "hello from the child" }]);
    const lines = view.render(80);
    assert.ok(lines.some((l) => l.includes("hello from the child")));
    assert.ok(!lines.some((l) => l.trim() === "lead:"), "the child's own text must never carry the lead label");
  });

  test("a streaming (not-yet-settled) turn also renders with no 'lead:' label", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "partial thought" } });
    const lines = view.render(80);
    assert.ok(lines.some((l) => l.includes("partial thought")));
    assert.ok(!lines.some((l) => l.trim() === "lead:"));
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

  test("'working…' is replaced by the first streamed delta, even while liveness() stays 'running'", () => {
    const { channel, fire } = fakeChannel("running");
    const view = new ConversationViewComponent(fakeTui(), { channel });
    assert.ok(view.render(80).some((l) => l.includes("working…")));
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "first bit" } });
    const lines = view.render(80);
    assert.ok(!lines.some((l) => l.includes("working…")), "the marker must be replaced by the first delta, not linger alongside it");
    assert.ok(lines.some((l) => l.includes("first bit")));
  });

  test("'idle-awaiting-owner' renders a distinctly more prominent banner than 'settled', at both header and foot", () => {
    const { channel, setLiveness } = fakeChannel("idle-awaiting-owner");
    const view = new ConversationViewComponent(fakeTui(), { channel });
    const idleLines = view.render(80);
    const occurrences = idleLines.filter((l) => l.includes("AWAITING OWNER")).length;
    assert.equal(occurrences, 2, "renders once in the header and once at the transcript foot");
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

describe("ConversationViewComponent — view-mode collapse/expand and focus (newest first)", () => {
  test("Tab cycles focus among expandable items newest-first, wrapping around", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
    view.handleInput("\t");
    assert.equal(view.getFocusedIndex(), 5, "first Tab selects the NEWEST expandable item");
    view.handleInput("\t");
    assert.equal(view.getFocusedIndex(), 4);
    view.handleInput("\t");
    assert.equal(view.getFocusedIndex(), 5, "wraps back to the newest expandable item");
  });

  test("Shift+Tab cycles focus backwards through the same newest-first order", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
    view.handleInput("\x1b[Z");
    assert.equal(view.getFocusedIndex(), 4);
    view.handleInput("\x1b[Z");
    assert.equal(view.getFocusedIndex(), 5);
  });

  test("Space toggles only the focused item", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
    view.handleInput("\t"); // focus index 5 (newest)
    assert.equal(view.isItemExpanded(5), false);
    view.handleInput(" ");
    assert.equal(view.isItemExpanded(5), true);
    assert.equal(view.isItemExpanded(4), false);
    view.handleInput(" ");
    assert.equal(view.isItemExpanded(5), false);
  });

  test("Ctrl+O toggles expand/collapse for every expandable item at once", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
    view.handleInput("\x0f");
    assert.equal(view.isItemExpanded(4), true);
    assert.equal(view.isItemExpanded(5), true);
    view.handleInput("\x0f");
    assert.equal(view.isItemExpanded(4), false);
    assert.equal(view.isItemExpanded(5), false);
  });

  test("expanding a tool-call item reveals its full args as body lines", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: [{ kind: "tool-call", id: "c1", name: "ws-read", args: { path: "a.txt", limit: 10 } }],
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
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: [{ kind: "tool-call", id: "c1", name: "ws-noop", args: undefined }],
    });
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
      primitives: {
        Editor: class {
          constructor() {
            return editor as never;
          }
        } as never,
      },
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
    assert.equal(view.isItemExpanded(4), true);
    assert.equal(view.isItemExpanded(5), true);
  });

  test("precedence: in view mode, Space toggles the focused item rather than doing nothing", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
    view.handleInput("\t");
    view.handleInput(" ");
    assert.equal(view.isItemExpanded(5), true);
  });
});

describe("ConversationViewComponent — interactive-mode key precedence (editor-empty / selection-active)", () => {
  function buildFocusable(): { view: ConversationViewComponent; editor: FakeEditor } {
    const { channel } = fakeChannel();
    const editor = new FakeEditor();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: ALL_ITEM_KINDS,
      primitives: {
        Editor: class {
          constructor() {
            return editor as never;
          }
        } as never,
      },
    });
    view.setMode("interactive");
    return { view, editor };
  }

  test("Tab/Shift+Tab move the selection only while the Editor is empty", () => {
    const { view, editor } = buildFocusable();
    view.handleInput("\t");
    assert.equal(view.getFocusedIndex(), 5, "editor is empty: Tab navigates");
    assert.deepEqual(editor.received, [], "Tab must not reach the editor while it navigates");

    editor.setText("draft");
    view.handleInput("\t");
    assert.equal(view.getFocusedIndex(), undefined, "typing clears the stale selection instead of navigating further");
    assert.ok(editor.received.includes("\t"), "once the editor is non-empty, Tab is forwarded instead of navigating");
  });

  test("Shift+Tab likewise only navigates while the Editor is empty", () => {
    const { view, editor } = buildFocusable();
    editor.setText("draft");
    view.handleInput("\x1b[Z");
    assert.equal(view.getFocusedIndex(), undefined);
    assert.ok(editor.received.includes("\x1b[Z"));
  });

  test("Space toggles the selection only while one is active; otherwise it is ordinary typed input", () => {
    const { view, editor } = buildFocusable();
    // No selection yet: Space is ordinary typed input, forwarded to the editor.
    view.handleInput(" ");
    assert.equal(editor.getText(), " ");
    assert.equal(view.isItemExpanded(5), false);

    editor.setText("");
    view.handleInput("\t"); // editor is empty again: select the newest expandable item (5)
    assert.equal(view.getFocusedIndex(), 5);
    view.handleInput(" "); // a selection IS active: Space toggles, does not reach the editor
    assert.equal(view.isItemExpanded(5), true);
    assert.equal(editor.getText(), "", "Space must not reach the editor while a selection is active");
  });

  test("any other typed character clears an active selection and reaches the Editor", () => {
    const { view, editor } = buildFocusable();
    view.handleInput("\t");
    assert.equal(view.getFocusedIndex(), 5);
    view.handleInput("x");
    assert.equal(view.getFocusedIndex(), undefined, "typing clears the selection");
    assert.equal(editor.getText(), "x", "and the character still reaches the editor");
  });

  test("a selection carried over from 'view' mode via setMode('interactive') can still be toggled while the Editor is empty", () => {
    const { channel } = fakeChannel();
    const editor = new FakeEditor();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: ALL_ITEM_KINDS,
      primitives: {
        Editor: class {
          constructor() {
            return editor as never;
          }
        } as never,
      },
    });
    view.handleInput("\t"); // select while still in "view" mode
    assert.equal(view.getFocusedIndex(), 5);
    view.setMode("interactive");
    // The Editor starts empty, so the carried-over selection is still live and actionable.
    view.handleInput(" ");
    assert.equal(view.isItemExpanded(5), true);
  });
});

describe("ConversationViewComponent — view/interactive mode differences", () => {
  test("the Editor only renders once the view is 'interactive'", () => {
    const { channel } = fakeChannel();
    const editor = new FakeEditor();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      primitives: {
        Editor: class {
          constructor() {
            return editor as never;
          }
        } as never,
      },
    });
    assert.ok(!view.render(80).some((l) => l.includes("[editor:")));
    view.setMode("interactive");
    assert.ok(view.render(80).some((l) => l.includes("[editor:")));
  });

  test("Enter in 'view' mode calls onEnter (there is no Editor to submit)", () => {
    const { channel } = fakeChannel();
    let entered = 0;
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      onEnter: () => {
        entered += 1;
      },
    });
    view.handleInput("\r");
    assert.equal(entered, 1);
  });

  test("Enter in 'interactive' mode is forwarded to the Editor instead of calling onEnter", () => {
    const { channel } = fakeChannel();
    const editor = new FakeEditor();
    let entered = 0;
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      primitives: {
        Editor: class {
          constructor() {
            return editor as never;
          }
        } as never,
      },
      onEnter: () => {
        entered += 1;
      },
    });
    view.setMode("interactive");
    editor.setText("hello");
    view.handleInput("\r");
    assert.equal(entered, 0);
    assert.deepEqual(view.getItems(), [{ kind: "user", text: "hello" }]);
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
      primitives: {
        Editor: class {
          constructor() {
            return editor as never;
          }
        } as never,
      },
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

  test("submitting ordinary text appends a user item and sends it to the channel", () => {
    const { view, editor, channel } = buildInteractive();
    for (const ch of "ping") view.handleInput(ch);
    view.handleInput("\r");
    assert.deepEqual(view.getItems(), [{ kind: "user", text: "ping" }]);
    assert.deepEqual(channel.sent, ["ping"]);
  });

  test("submitting empty/whitespace-only text does nothing", () => {
    const { view, channel } = buildInteractive();
    view.handleInput(" ");
    view.handleInput("\r");
    assert.deepEqual(view.getItems(), []);
    assert.deepEqual(channel.sent, []);
  });

  test("a bracketed paste with embedded newlines is one send, not many — only a real Enter submits", () => {
    const { view, editor, channel } = buildInteractive();
    view.handleInput("\x1b[200~line one\nline two\x1b[201~");
    assert.equal(editor.getText(), "line one\nline two", "the paste's embedded newline lands in the text, not as a submit");
    assert.deepEqual(channel.sent, [], "no premature send from the newline inside the paste");
    view.handleInput("\r");
    assert.deepEqual(channel.sent, ["line one\nline two"], "exactly one send, triggered by the real Enter");
  });

  test("Esc calls onEscape in both view and interactive mode", () => {
    const { channel } = fakeChannel();
    let escapeCount = 0;
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      onEscape: () => {
        escapeCount += 1;
      },
    });
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
      primitives: {
        Editor: class {
          constructor() {
            return editor as never;
          }
        } as never,
      },
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
      primitives: {
        Editor: class {
          constructor() {
            return editor as never;
          }
        } as never,
      },
      initialItems: [{ kind: "note", text: "seeded" }],
    });
    view.setMode("interactive");
    editor.setText("draft in progress");
    view.setMode("interactive"); // no-op: already interactive
    assert.equal(editor.getText(), "draft in progress", "the same Editor instance/state survives a redundant setMode call");
    assert.deepEqual(view.getItems(), [{ kind: "note", text: "seeded" }]);
  });
});

describe("ConversationViewComponent — optional ConversationChannel.send", () => {
  test("a channel with no send() still works in 'view' mode (send is optional per the ticket)", () => {
    const listeners = new Set<(evt: unknown) => void>();
    const channel: ConversationChannel = {
      onEvent: (l) => {
        listeners.add(l);
        return () => listeners.delete(l);
      },
      liveness: () => "settled",
      // No `send` — a "view"-only consumer (e.g. the audit window) need not implement it.
    };
    const view = new ConversationViewComponent(fakeTui(), { channel });
    assert.doesNotThrow(() => view.render(80));
    assert.doesNotThrow(() => view.handleInput("\t"));
  });
});

describe("ConversationViewComponent — tool preview reuse (pinned to tool-result-render.ts)", () => {
  test("a tool-call's collapsed head and expanded body come from yamlInputPreview/logicalPreview, not ad hoc formatting", () => {
    const args = { path: "a.txt", limit: 10 };
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: [{ kind: "tool-call", id: "c1", name: "ws-read", args }],
    });
    const full = yamlInputPreview(args);
    const expectedHeadFragment = logicalPreview(full, 1);
    const collapsed = view.render(120);
    assert.ok(collapsed.some((l) => l.includes(expectedHeadFragment)), `no rendered line has the yamlInputPreview/logicalPreview text: ${collapsed.join("\n")}`);

    view.handleInput("\t");
    view.handleInput(" ");
    const expanded = view.render(120);
    for (const bodyLine of full.split("\n")) {
      assert.ok(expanded.some((l) => l.includes(bodyLine)), `expanded body is missing a yamlInputPreview line: ${bodyLine}`);
    }
  });

  test("a tool-result's collapsed head and expanded body come from completedTextPreview/logicalPreview/yamlContainerDisplay", () => {
    const content = JSON.stringify({ ok: true, lines: 3 });
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: [{ kind: "tool-result", id: "r1", name: "ws-read", content }],
    });
    const preview = completedTextPreview(content);
    const expectedHeadFragment = logicalPreview(preview.text, 1);
    const collapsed = view.render(120);
    assert.ok(collapsed.some((l) => l.includes(expectedHeadFragment)), `no rendered line has the completedTextPreview/logicalPreview text: ${collapsed.join("\n")}`);

    const yaml = yamlContainerDisplay(content);
    assert.ok(yaml !== undefined, "test fixture must be expandable for this pin to be meaningful");
    view.handleInput("\t");
    view.handleInput(" ");
    const expanded = view.render(120);
    for (const bodyLine of (yaml as string).split("\n")) {
      assert.ok(expanded.some((l) => l.includes(bodyLine)), `expanded body is missing a yamlContainerDisplay line: ${bodyLine}`);
    }
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

  test("text_delta accumulates, and agent_settled commits it as an 'assistant' item", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "part one " } });
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "part two" } });
    fire({ type: "agent_settled" });
    assert.deepEqual(view.getItems(), [{ kind: "assistant", text: "part one part two" }]);
  });

  test("agent_settled with no accumulated text commits nothing", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "agent_settled" });
    assert.deepEqual(view.getItems(), []);
  });

  test("tool_execution_start appends a tool-call item with the wire event's id/name/args", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "tool_execution_start", toolCallId: "c1", toolName: "ws-read", args: { path: "a.txt" } });
    assert.deepEqual(view.getItems(), [{ kind: "tool-call", id: "c1", name: "ws-read", args: { path: "a.txt" } }]);
  });

  test("tool_execution_end appends a tool-result item, deriving content text from a single-text-part result", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "tool_execution_end", toolCallId: "c1", toolName: "ws-read", result: { content: [{ type: "text", text: "line 1\nline 2" }] } });
    assert.deepEqual(view.getItems(), [{ kind: "tool-result", id: "c1", name: "ws-read", content: "line 1\nline 2", isError: undefined }]);
  });

  test("tool_execution_end carries isError through, and falls back to JSON.stringify for a non-single-text result", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "tool_execution_end", toolCallId: "c2", toolName: "ws-write", result: { ok: false }, isError: true });
    assert.deepEqual(view.getItems(), [{ kind: "tool-result", id: "c2", name: "ws-write", content: JSON.stringify({ content: undefined, ok: false }), isError: true }]);
  });

  test("a malformed tool_execution_start/end (missing toolCallId or toolName) is ignored rather than appending a broken item", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "tool_execution_start", toolCallId: "c1" });
    fire({ type: "tool_execution_start", toolName: "ws-read" });
    fire({ type: "tool_execution_end", toolCallId: "c1" });
    assert.deepEqual(view.getItems(), []);
  });
});

describe("ConversationViewComponent — userLineBg option", () => {
  // Wrapped in real ANSI (SGR) codes rather than visible bracket characters:
  // `applyBackgroundToLine` pads the line to the FULL render width before
  // handing it to the bg painter, so a painter that adds visible characters
  // pushes the line's visible width past that render width and trips the
  // module's own defensive final `truncateToWidth` pass (real `pi-tui`
  // behavior, not specific to this component) — exactly what an ANSI-only
  // paint (invisible to `visibleWidth`) is meant to avoid in real usage.
  const ANSI_OPEN = "\x1b[45m";
  const ANSI_CLOSE = "\x1b[0m";
  const paint = (text: string) => `${ANSI_OPEN}${text}${ANSI_CLOSE}`;

  test("a supplied bg painter wraps the full-width 'you: ' line and one blank row above/below it, at 40/80/120 columns", () => {
    for (const width of WIDTHS) {
      const { channel } = fakeChannel();
      const view = new ConversationViewComponent(fakeTui(), {
        channel,
        initialItems: [{ kind: "user", text: "hello" }],
        userLineBg: paint,
      });
      const lines = view.render(width);
      assert.ok(
        lines.some((l) => l.startsWith(ANSI_OPEN) && l.endsWith(ANSI_CLOSE) && l.includes("you: hello")),
        `width ${width}: no line shows the bg wrapper: ${JSON.stringify(lines)}`,
      );
      const userIndex = lines.findIndex((l) => l.includes("you: hello"));
      assert.ok(userIndex > 0 && userIndex < lines.length - 1, `width ${width}: user content must have padding on both sides`);
      for (const paddingLine of [lines[userIndex - 1], lines[userIndex + 1]]) {
        assert.ok(paddingLine.startsWith(ANSI_OPEN) && paddingLine.endsWith(ANSI_CLOSE), `width ${width}: padding row is outside the user background`);
        assert.equal(visibleWidth(paddingLine), width, `width ${width}: painted padding row must span the full content width`);
        assert.equal(paddingLine.replace(ANSI_OPEN, "").replace(ANSI_CLOSE, "").trim(), "", `width ${width}: user padding row must be blank`);
      }
    }
  });

  test("a long user turn wraps across multiple lines, every wrapped line individually painted", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: [{ kind: "user", text: "word ".repeat(30).trim() }],
      userLineBg: paint,
    });
    const lines = view.render(40).filter((l) => l.startsWith(ANSI_OPEN) && l.endsWith(ANSI_CLOSE));
    assert.ok(lines.length > 1, "the long turn must wrap onto more than one painted line");
  });

  test("omitting userLineBg leaves 'you:' lines unpainted (no wrapper text)", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: [{ kind: "user", text: "hello" }] });
    const lines = view.render(80);
    assert.ok(lines.some((l) => l.includes("you: hello")));
    assert.ok(!lines.some((l) => l.includes("<<") || l.includes("[you")));
  });

  test("a painted user line stays width-bounded (the bg fn only recolors, ANSI does not count toward width)", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: [{ kind: "user", text: "hello" }],
      userLineBg: (text) => `\x1b[7m${text}\x1b[0m`,
    });
    for (const width of WIDTHS) {
      assertWidthBounded(view.render(width), width, `width ${width}`);
    }
  });
});

describe("ConversationViewComponent — semantic muted text painters", () => {
  const MUTED_OPEN = "\x1b[2m";
  const ANSI_CLOSE = "\x1b[0m";
  const muted = (text: string) => `${MUTED_OPEN}${text}${ANSI_CLOSE}`;

  test("tool-call/tool-result heads and expanded bodies use the supplied muted foreground while staying width-bounded", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      headerHint: "HDR",
      initialItems: [
        { kind: "tool-call", id: "c1", name: "ws-read", args: { path: "a.txt" } },
        { kind: "tool-result", id: "c1", name: "ws-read", content: '{"ok":true}' },
      ],
      toolTextFg: muted,
    });
    view.handleInput("\x0f"); // expand both structured tool rows
    const lines = view.render(40);
    assertWidthBounded(lines, 40, "muted tool rows");
    const toolLines = lines.filter((line) => /ws-read|path:|ok:/.test(line));
    assert.ok(toolLines.length >= 4, `expected muted heads and bodies: ${JSON.stringify(lines)}`);
    for (const line of toolLines) assert.ok(line.includes(MUTED_OPEN) && line.includes(ANSI_CLOSE), `tool text is not muted: ${JSON.stringify(line)}`);
  });

  test("the working marker uses its supplied dim foreground painter", () => {
    const { channel } = fakeChannel("running");
    const view = new ConversationViewComponent(fakeTui(), { channel, workingTextFg: muted });
    const marker = view.render(80).find((line) => line.includes("working…"));
    assert.ok(marker?.includes(MUTED_OPEN) && marker.includes(ANSI_CLOSE), `working marker is not dimmed: ${JSON.stringify(marker)}`);
  });
});

describe("ConversationViewComponent — markdownTheme option", () => {
  test("a supplied theme's bold function is applied to an 'assistant' item's markdown", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: [{ kind: "assistant", text: "**bold word**" }],
      markdownTheme: {
        heading: (t) => t, link: (t) => t, linkUrl: (t) => t, code: (t) => t, codeBlock: (t) => t, codeBlockBorder: (t) => t,
        quote: (t) => t, quoteBorder: (t) => t, hr: (t) => t, listBullet: (t) => t,
        bold: (t) => `<B>${t}</B>`, italic: (t) => t, strikethrough: (t) => t, underline: (t) => t,
      },
    });
    assert.ok(view.render(80).some((l) => l.includes("<B>bold word</B>")));
  });

  test("a supplied theme also styles a 'lead-message' item's markdown, not just 'assistant'", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: [{ kind: "lead-message", text: "*emphasis*" }],
      markdownTheme: {
        heading: (t) => t, link: (t) => t, linkUrl: (t) => t, code: (t) => t, codeBlock: (t) => t, codeBlockBorder: (t) => t,
        quote: (t) => t, quoteBorder: (t) => t, hr: (t) => t, listBullet: (t) => t,
        bold: (t) => t, italic: (t) => `<I>${t}</I>`, strikethrough: (t) => t, underline: (t) => t,
      },
    });
    assert.ok(view.render(80).some((l) => l.includes("<I>emphasis</I>")));
  });

  test("omitting markdownTheme falls back to the identity theme — markdown formatting characters are stripped but no styling is added", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: [{ kind: "assistant", text: "**bold word**" }] });
    const lines = view.render(80);
    assert.ok(lines.some((l) => l.includes("bold word")));
    assert.ok(!lines.some((l) => l.includes("**")), "markdown syntax markers are consumed by the renderer even under the identity theme");
  });
});

describe("ConversationViewComponent — onItemsChange option", () => {
  test("fires with a full copy of the transcript after every appendItem, never for the streaming tail", () => {
    const { channel, fire } = fakeChannel();
    const snapshots: ConversationItem[][] = [];
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: [{ kind: "note", text: "seed" }],
      onItemsChange: (items) => snapshots.push([...items]),
    });
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "partial" } });
    assert.equal(snapshots.length, 0, "a streaming delta alone never appends an item, so no callback yet");

    fire({ type: "agent_settled" });
    assert.equal(snapshots.length, 1, "the settle commits one assistant item");
    assert.deepEqual(snapshots[0], [{ kind: "note", text: "seed" }, { kind: "assistant", text: "partial" }]);

    fire({ type: "tool_execution_start", toolCallId: "c1", toolName: "ws-read", args: {} });
    assert.equal(snapshots.length, 2);
    assert.deepEqual(snapshots[1].at(-1), { kind: "tool-call", id: "c1", name: "ws-read", args: {} });
  });

  test("a snapshot is a copy, not a live reference — later appends do not mutate an already-delivered snapshot", () => {
    const { channel, fire } = fakeChannel();
    let captured: ConversationItem[] | undefined;
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      onItemsChange: (items) => {
        captured = items as ConversationItem[];
      },
    });
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "first" } });
    fire({ type: "agent_settled" });
    const firstSnapshotLength = captured!.length;
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "second" } });
    fire({ type: "agent_settled" });
    assert.equal(captured!.length, firstSnapshotLength + 1, "the new snapshot grew");
    assert.equal(view.getItems().length, firstSnapshotLength + 1);
  });

  test("omitting onItemsChange is safe — appends still work with no callback to invoke", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "x" } });
    fire({ type: "agent_settled" });
    assert.deepEqual(view.getItems(), [{ kind: "assistant", text: "x" }]);
  });
});

describe("ConversationViewComponent — overlay border chrome (V1/V2)", () => {
  test("border:true wraps the whole view in a box border with a one-column horizontal margin, at 40/80/120", () => {
    for (const width of WIDTHS) {
      const { channel } = fakeChannel();
      const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS, border: true });
      const lines = view.render(width);
      assertWidthBounded(lines, width, `width ${width} (bordered)`);
      const top = lines[0];
      const bottom = lines[lines.length - 1];
      // V1: a border is present, top and bottom, spanning the full width.
      assert.ok(top.startsWith("┌") && top.endsWith("┐"), `V1: no top border at width ${width}: ${JSON.stringify(top)}`);
      assert.ok(bottom.startsWith("└") && bottom.endsWith("┘"), `V1: no bottom border at width ${width}`);
      assert.equal(visibleWidth(top), width, "the top border spans the full width");
      assert.equal(visibleWidth(bottom), width, "the bottom border spans the full width");
      // V2: every interior line carries the side borders AND a one-column
      // horizontal margin inside them (`│ … │`).
      for (const line of lines.slice(1, -1)) {
        assert.ok(line.startsWith("│ ") && line.endsWith(" │"), `V2: interior line lacks the bordered ~1-char margin at width ${width}: ${JSON.stringify(line)}`);
      }
      // Density polish: one blank interior row just below the top border and
      // just above the bottom border, so content never touches the box.
      assert.equal(lines[1].slice(1, -1).trim(), "", `no blank padding row below the top border at width ${width}: ${JSON.stringify(lines[1])}`);
      assert.equal(lines[lines.length - 2].slice(1, -1).trim(), "", `no blank padding row above the bottom border at width ${width}`);
    }
  });

  test("border defaults off — compact embeds render with no box-drawing chrome", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS });
    assert.ok(!view.render(80).some((l) => /[┌┐└┘│]/.test(l)), "no border chrome unless border:true");
  });

  test("border:true remains width-safe below the four-column frame minimum", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: ALL_ITEM_KINDS, border: true });
    for (const width of [1, 2, 3, 4]) {
      const lines = view.render(width);
      assertWidthBounded(lines, width, `width ${width} (narrow bordered)`);
    }
  });
});

describe("ConversationViewComponent — model-turn gutter (V3)", () => {
  const GUTTER = "▌ ";

  test("every rendered line of an assistant (model) turn carries the gutter, and stays width-bounded", () => {
    for (const width of WIDTHS) {
      const { channel } = fakeChannel();
      // headerHint "" collapses the header to a single blank line, so after
      // filtering blanks the only non-empty lines are the model turn's.
      const view = new ConversationViewComponent(fakeTui(), {
        channel,
        headerHint: "",
        initialItems: [{ kind: "assistant", text: "alpha beta gamma delta epsilon zeta eta theta ".repeat(4).trim() }],
      });
      const rendered = view.render(width);
      assertWidthBounded(rendered, width, `width ${width}`);
      const contentLines = rendered.filter((l) => l.trim().length > 0);
      assert.ok(contentLines.length > 1, `the long model turn must wrap onto multiple lines at width ${width}`);
      for (const line of contentLines) {
        assert.ok(line.startsWith(GUTTER), `V3: a model-turn line is missing the gutter at width ${width}: ${JSON.stringify(line)}`);
      }
    }
  });

  test("the gutter sets the model turn apart from lead-message (keeps its 'lead:' label), user, and note rows", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      initialItems: [
        { kind: "assistant", text: "model says hi" },
        { kind: "lead-message", text: "lead says hi" },
        { kind: "user", text: "owner says hi" },
        { kind: "note", text: "a note" },
      ],
    });
    const lines = view.render(80);
    assert.ok(lines.some((l) => l.startsWith(GUTTER) && l.includes("model says hi")), "the model turn carries the gutter");
    assert.ok(lines.some((l) => l.trim() === "lead:"), "the lead message keeps its own label");
    assert.ok(!lines.some((l) => l.startsWith(GUTTER) && l.includes("lead says hi")), "the lead message is not given the model gutter");
    assert.ok(!lines.some((l) => l.startsWith(GUTTER) && l.includes("owner says hi")), "a user turn is not given the model gutter");
    assert.ok(!lines.some((l) => l.startsWith(GUTTER) && l.includes("a note")), "a note is not given the model gutter");
  });

  test("a streaming (not-yet-settled) model tail also carries the gutter", () => {
    const { channel, fire } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel });
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "partial thought" } });
    assert.ok(view.render(80).some((l) => l.startsWith(GUTTER) && l.includes("partial thought")));
  });
});

describe("ConversationViewComponent — density / vertical rhythm (260909 polish)", () => {
  test("a note renders with no leading '·' bullet (noise removed)", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: [{ kind: "note", text: "adapter note" }] });
    const lines = view.render(80);
    assert.ok(lines.some((l) => l.includes("adapter note")));
    assert.ok(!lines.some((l) => l.trimStart().startsWith("· ")), "the noise bullet must be gone");
  });

  test("the header block is separated from the conversation body by a blank line", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      headerHint: "HEADER-ONLY-LINE",
      initialItems: [{ kind: "assistant", text: "body line" }],
    });
    const lines = view.render(80);
    const hIdx = lines.findIndex((l) => l.includes("HEADER-ONLY-LINE"));
    const bIdx = lines.findIndex((l) => l.includes("body line"));
    assert.ok(hIdx >= 0 && bIdx > hIdx, "header and body both render, body after header");
    assert.ok(lines.slice(hIdx + 1, bIdx).some((l) => l.trim() === ""), "a blank line separates the header block from the body");
  });

  test("conversation turns are separated by a blank line (question vs. answer)", () => {
    const { channel } = fakeChannel();
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      headerHint: "HDR",
      initialItems: [{ kind: "assistant", text: "**Question:** pick one" }, { kind: "user", text: "answer here" }],
    });
    const lines = view.render(80);
    const qIdx = lines.findIndex((l) => l.includes("Question: pick one"));
    const aIdx = lines.findIndex((l) => l.includes("answer here"));
    assert.ok(qIdx >= 0 && aIdx > qIdx, "question and answer both render, answer after question");
    assert.ok(lines[qIdx].startsWith("▌ "), "the question uses the assistant dialogue gutter");
    assert.ok(lines.slice(qIdx + 1, aIdx).some((l) => l.trim() === ""), "a blank line separates the two turns");
  });
});

describe("ConversationViewComponent — bounded transcript scrolling", () => {
  const bindings = {
    matches(data: string, id: string): boolean {
      return ({ "tui.altScreen.pageUp": "pgup", "tui.altScreen.pageDown": "pgdown", "tui.altScreen.top": "home", "tui.altScreen.bottom": "end" } as Record<string, string>)[id] === data;
    },
  };
  const items = Array.from({ length: 12 }, (_, index) => ({ kind: "note" as const, text: `line-${index}` }));

  test("opens at the tail, retains manual scroll on append and mode promotion, and resumes follow at end", () => {
    const { channel } = fakeChannel();
    const editor = new FakeEditor();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: items, viewportHeight: () => 6, keybindings: bindings, primitives: { Editor: class { constructor() { return editor as never; } } as never } });
    assert.ok(view.render(80).some((line) => line.includes("line-11")), "initial render follows the tail");
    view.handleInput("pgup");
    const backedUp = view.getScrollTop();
    view.appendItem({ kind: "note", text: "late" });
    view.render(80);
    assert.equal(view.getScrollTop(), backedUp, "manual scroll does not jump on append");
    view.handleInput("end");
    view.render(80);
    assert.equal(view.isFollowingTail(), true);
    assert.ok(view.render(80).some((line) => line.includes("late")));
    view.handleInput("pgup");
    const promotedFrom = view.getScrollTop();
    view.setMode("interactive");
    view.render(80);
    assert.equal(view.getScrollTop(), promotedFrom, "mode promotion retains manual scroll position");
  });

  test("view mode supports line/page/start/end plus wheel while interactive mode leaves editor arrows/home/end alone", () => {
    const { channel } = fakeChannel();
    const editor = new FakeEditor();
    const view = new ConversationViewComponent(fakeTui(), { channel, initialItems: items, viewportHeight: () => 6, keybindings: bindings, primitives: { Editor: class { constructor() { return editor as never; } } as never } });
    view.render(80);
    view.handleInput("home");
    assert.equal(view.getScrollTop(), 0);
    view.handleInput("\x1b[B");
    assert.equal(view.getScrollTop(), 1);
    view.handleInput("\x1b[<65;1;1M");
    assert.equal(view.getScrollTop(), 2);
    assert.deepEqual(view.handleMouse({ type: "wheel", deltaY: -1 }), { handled: true, render: true });
    assert.equal(view.getScrollTop(), 1);
    view.setMode("interactive");
    view.handleInput("\x1b[A");
    view.handleInput("home");
    assert.deepEqual(editor.received, ["\x1b[A", "home"], "interactive editor keeps normal navigation keys");
    view.handleInput("pgdown");
    assert.ok(view.getScrollTop() > 1, "interactive PageDown scrolls the transcript");
  });
});

describe("ConversationViewComponent — working marker location (F2)", () => {
  test("'working…' renders at the END of the agent dialogue (the streaming slot at the foot), never in the header", () => {
    const { channel } = fakeChannel("running");
    const view = new ConversationViewComponent(fakeTui(), {
      channel,
      headerHint: "HEADER-HINT",
      initialItems: [{ kind: "assistant", text: "the model dialogue" }],
    });
    const lines = view.render(80);
    const hintIdx = lines.findIndex((l) => l.includes("HEADER-HINT"));
    const dialogueIdx = lines.findIndex((l) => l.includes("the model dialogue"));
    const workingIdx = lines.findIndex((l) => l.includes("working…"));
    assert.ok(hintIdx >= 0 && dialogueIdx >= 0 && workingIdx >= 0, "hint, dialogue, and marker all render");
    assert.ok(workingIdx > dialogueIdx, "the marker sits at the END of the agent dialogue, after the last dialogue line — not the header");
    assert.equal(workingIdx, lines.length - 1, "the marker is the final line: the streaming slot at the transcript foot");
  });

  test("the marker is shown while running and BEFORE a turn's tool output/text — then the first delta replaces it", () => {
    const { channel, fire } = fakeChannel("running");
    const view = new ConversationViewComponent(fakeTui(), { channel });
    // Running, nothing streamed yet (the agent_start moment): the marker is
    // present at the foot before any tool output or text of the turn.
    assert.ok(view.render(80).some((l) => l.includes("working…")), "the marker is shown as soon as the child is working");
    // A tool executes; its output appends. The marker stays pinned to the foot,
    // never jumping to the header.
    fire({ type: "tool_execution_start", toolCallId: "c1", toolName: "ws-read", args: { path: "a.txt" } });
    const withTool = view.render(80);
    const workingIdx = withTool.findIndex((l) => l.includes("working…"));
    const toolIdx = withTool.findIndex((l) => l.includes("ws-read"));
    assert.ok(toolIdx >= 0 && workingIdx >= 0);
    assert.equal(workingIdx, withTool.length - 1, "the marker stays at the transcript foot alongside tool output, not the header");
    // The first streamed delta of the turn replaces the marker with the tail.
    fire({ type: "message_update", assistantMessageEvent: { type: "text_delta", delta: "answering now" } });
    const streaming = view.render(80);
    assert.ok(!streaming.some((l) => l.includes("working…")), "the first delta replaces the marker");
    assert.ok(streaming.some((l) => l.includes("answering now")));
  });
});

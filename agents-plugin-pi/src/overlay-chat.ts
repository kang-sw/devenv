/**
 * 260904 Phase 2 (`260904-feat-ws-pi-side-thread-fork-question-surface`):
 * the owner-facing overlay chat (§5) — a `pi-tui` `Component` shown through
 * `ctx.ui.custom(factory, {overlay:true})` that streams one discussion
 * fork's output and routes the owner's typed lines back into it.
 *
 * Shape decisions worth keeping:
 *   - The component is a THIN, DISPOSABLE VIEW over persistent state held
 *     elsewhere (the thread record + the live fork on the shared
 *     `rpcRegistry`), never the other way around — the same split the
 *     bundled `doom-overlay` example uses for its persistent engine. Closing
 *     the overlay disposes only the view; the fork keeps running and the
 *     thread is reopenable, which is exactly §5's requirement.
 *   - `/done` is intercepted HERE, in the overlay's own input handling, not
 *     registered as a `pi.registerCommand`: an active `ctx.ui.custom` overlay
 *     holds keyboard focus, and Pi's slash-command dispatch runs on the main
 *     app editor the overlay took focus from.
 *   - The fork is reached through the injected `ForkChannel`, never through a
 *     bare `RpcClient`. A dormant thread has NO live client at open time (it
 *     only gets one once `sendToAgent` relaunches the child on the first
 *     message), and `sendToAgent` already owns the prompt/steer/resume branch
 *     table §5 describes. The channel also makes this whole file drivable
 *     from a plain fake object in `node --test` with no subprocess — the
 *     ticket's own unit tier.
 *   - No `@earendil-works/pi-tui` import: that package is not resolvable from
 *     this one (it is nested inside `pi-coding-agent`'s own `node_modules`),
 *     so the `Component` contract is satisfied structurally and the input
 *     box, wrapping and width measurement are small local implementations.
 *     Rendering stays plain, unstyled text, which is also why width
 *     assertions in the tests are exact rather than ANSI-tolerant.
 *
 * Never auto-popped: only `/answer` (or the reopen shortcut) in `ask.ts`
 * constructs one of these. No child-side `ctx.ui.*` dialog is involved
 * anywhere in this path.
 */

/** The literal the owner types to end a discussion thread (§5). */
export const DONE_COMMAND = "/done";

/** Transcript lines kept on screen; older ones scroll off (tail truncation — the ticket's own documented `ScrollView` fallback). */
export const MAX_TRANSCRIPT_LINES = 24;

/** Minimal `pi-tui` `TUI` surface this component needs. */
export interface OverlayTui {
  requestRender(): void;
}

/**
 * The overlay's only route to its fork. `ask.ts` implements it over the
 * shared `rpcRegistry` + `sendToAgent`; tests implement it as a plain object.
 */
export interface ForkChannel {
  /** Subscribe to the fork's RPC event stream. Returns an unsubscribe function. */
  onEvent(listener: (evt: unknown) => void): () => void;
  /** `true` while the fork is mid-run — §5's prompt()-vs-steer() discriminator. */
  isStreaming(): boolean;
  /** Deliver one owner message to the fork (resuming it first when dormant). */
  send(text: string): Promise<void>;
}

export interface OverlayChatOptions {
  title: string;
  threadId: string;
  question?: string;
  channel: ForkChannel;
  /** Called once, with the fork's own summary, when the owner ends the thread with `/done`. */
  onDone: (summary: string) => void;
}

/**
 * The fixed message sent to the fork when the owner types `/done`. One
 * round-trip only: the fork's next settled turn IS the summary — there is no
 * separate hand-shake protocol.
 */
export function buildDoneSummaryPrompt(): string {
  return "The owner ended the discussion. Write a concise summary of what was decided now — a few sentences, no preamble, no questions back.";
}

/** Fallback when the fork settles the `/done` turn without producing any text. */
export const EMPTY_SUMMARY_TEXT = "(the discussion ended without a summary from the thread)";

/**
 * Display width of a string: code points, counting the common East-Asian
 * wide ranges as two columns. A local approximation of `pi-tui`'s own
 * `visibleWidth` (that package is not resolvable from here — see the file
 * header); exact per-emulator behavior for CJK/IME input is a human-runbook
 * verification item, not something this adapter can assert offline.
 */
export function visibleWidth(text: string): number {
  let width = 0;
  for (const char of text) {
    const code = char.codePointAt(0) ?? 0;
    width += isWideCodePoint(code) ? 2 : 1;
  }
  return width;
}

function isWideCodePoint(code: number): boolean {
  return (
    (code >= 0x1100 && code <= 0x115f) || // Hangul Jamo
    (code >= 0x2e80 && code <= 0xa4cf) || // CJK radicals .. Yi
    (code >= 0xac00 && code <= 0xd7a3) || // Hangul syllables
    (code >= 0xf900 && code <= 0xfaff) || // CJK compatibility ideographs
    (code >= 0xfe30 && code <= 0xfe6f) || // CJK compatibility forms
    (code >= 0xff00 && code <= 0xff60) || // Fullwidth forms
    (code >= 0xffe0 && code <= 0xffe6) ||
    (code >= 0x1f300 && code <= 0x1f64f) || // Emoji
    (code >= 0x20000 && code <= 0x3fffd)
  );
}

/**
 * Hard-wraps one logical line to `width` display columns, never emitting a
 * line wider than `width`. Breaks mid-word: a chat transcript's own line
 * lengths are not the owner's to control, and a horizontal overflow inside
 * an overlay is worse than an ugly break.
 */
export function wrapLine(text: string, width: number): string[] {
  if (width <= 0) return [""];
  if (text.length === 0) return [""];
  const lines: string[] = [];
  let current = "";
  let currentWidth = 0;
  for (const char of text) {
    const charWidth = visibleWidth(char);
    if (currentWidth + charWidth > width) {
      lines.push(current);
      current = "";
      currentWidth = 0;
    }
    current += char;
    currentWidth += charWidth;
  }
  lines.push(current);
  return lines;
}

interface TranscriptEntry {
  who: "you" | "thread" | "note";
  text: string;
}

/**
 * The overlay's `Component` (structural: `render(width)` /
 * `handleInput(data)` / `invalidate()`, plus `dispose()` for the event
 * unsubscribe Pi calls on close).
 */
export class OverlayChatComponent {
  private readonly tui: OverlayTui;
  private readonly options: OverlayChatOptions;
  private readonly done: (result: undefined) => void;
  private readonly unsubscribe: () => void;

  private entries: TranscriptEntry[] = [];
  /** Text of the fork's in-flight turn, accumulated from `text_delta`s. */
  private streaming = "";
  /** Set once `/done` has been sent; the next settled turn is taken as the summary. */
  private donePending = false;
  private finished = false;
  private input = "";
  private cursor = 0;
  private cachedLines: string[] | undefined;

  constructor(tui: OverlayTui, options: OverlayChatOptions, done: (result: undefined) => void) {
    this.tui = tui;
    this.options = options;
    this.done = done;
    if (options.question) {
      this.entries.push({ who: "note", text: options.question });
    }
    this.unsubscribe = options.channel.onEvent((evt) => this.handleEvent(evt));
  }

  /**
   * Transcript accumulation over the same `message_update`/`text_delta` wire
   * shape `fork.ts`'s own anti-bleed loop already consumes; `agent_settled`
   * closes the fork's turn (and, once `/done` has been sent, IS the summary).
   */
  private handleEvent(evt: unknown): void {
    const e = evt as { type?: string; assistantMessageEvent?: { type?: string; delta?: string } };
    if (e.type === "message_update" && e.assistantMessageEvent?.type === "text_delta" && typeof e.assistantMessageEvent.delta === "string") {
      this.streaming += e.assistantMessageEvent.delta;
      this.refresh();
      return;
    }
    if (e.type !== "agent_settled") return;

    const settled = this.streaming.trim();
    this.streaming = "";
    if (settled.length > 0) {
      this.entries.push({ who: "thread", text: settled });
    }
    if (this.donePending) {
      this.donePending = false;
      this.finish(settled.length > 0 ? settled : EMPTY_SUMMARY_TEXT);
      return;
    }
    this.refresh();
  }

  private finish(summary: string): void {
    if (this.finished) return;
    this.finished = true;
    this.options.onDone(summary);
    this.close();
  }

  private refresh(): void {
    this.cachedLines = undefined;
    this.tui.requestRender();
  }

  /** Closes the overlay view only — the fork and its thread are untouched. */
  close(): void {
    this.dispose();
    this.done(undefined);
  }

  dispose(): void {
    this.unsubscribe();
  }

  invalidate(): void {
    this.cachedLines = undefined;
  }

  private submit(): void {
    const text = this.input.trim();
    this.input = "";
    this.cursor = 0;
    if (text.length === 0) {
      this.refresh();
      return;
    }

    if (text === DONE_COMMAND) {
      this.donePending = true;
      this.entries.push({ who: "note", text: "ending the thread — asking for a summary…" });
      this.deliver(buildDoneSummaryPrompt());
      this.refresh();
      return;
    }

    this.entries.push({ who: "you", text });
    this.deliver(text);
    this.refresh();
  }

  private deliver(text: string): void {
    // Fire-and-report: a delivery failure becomes a transcript note rather
    // than an unhandled rejection inside a keystroke handler.
    void this.options.channel.send(text).catch((err: unknown) => {
      this.entries.push({ who: "note", text: `delivery failed: ${err instanceof Error ? err.message : String(err)}` });
      this.donePending = false;
      this.refresh();
    });
  }

  handleInput(data: string): void {
    if (data === "\r" || data === "\n") {
      this.submit();
      return;
    }
    if (data === "\x7f" || data === "\b") {
      if (this.cursor > 0) {
        this.input = this.input.slice(0, this.cursor - 1) + this.input.slice(this.cursor);
        this.cursor -= 1;
        this.refresh();
      }
      return;
    }
    if (data === "\x1b") {
      // Bare escape: close the view. §5 — this has no effect on the fork.
      this.close();
      return;
    }
    if (data === "\x1b[D") {
      if (this.cursor > 0) {
        this.cursor -= 1;
        this.refresh();
      }
      return;
    }
    if (data === "\x1b[C") {
      if (this.cursor < this.input.length) {
        this.cursor += 1;
        this.refresh();
      }
      return;
    }
    if (data === "\x15") {
      this.input = "";
      this.cursor = 0;
      this.refresh();
      return;
    }
    if (data.startsWith("\x1b")) {
      // Any other escape sequence (arrow up/down, function keys, mouse
      // reports): ignored WHOLE. Filtering it character-by-character below
      // would strip the leading ESC and paste the sequence's printable tail
      // ("[A") straight into the input box.
      return;
    }
    // Printable text only — every other control/escape sequence is ignored
    // rather than pasted into the input box as mojibake.
    const printable = [...data].filter((char) => {
      const code = char.codePointAt(0) ?? 0;
      return code >= 0x20 && code !== 0x7f;
    });
    if (printable.length === 0) return;
    const inserted = printable.join("");
    this.input = this.input.slice(0, this.cursor) + inserted + this.input.slice(this.cursor);
    this.cursor += inserted.length;
    this.refresh();
  }

  render(width: number): string[] {
    if (this.cachedLines) return this.cachedLines;
    const w = Math.max(1, width);
    const lines: string[] = [];

    // §5: the header names the thread explicitly, so the two lead-voiced
    // agents (the lead itself and this discussion fork) can never be
    // confused for one another on screen.
    lines.push(...wrapLine(`── ws thread ${this.options.threadId} · ${this.options.title}`, w));
    lines.push("");

    for (const line of this.transcriptLines(w)) lines.push(line);

    lines.push("");
    lines.push(...wrapLine(`> ${this.input}`, w));
    lines.push(...wrapLine(`${DONE_COMMAND} to end the thread · Esc closes this view (the thread keeps running)`, w));

    this.cachedLines = lines;
    return lines;
  }

  private transcriptLines(width: number): string[] {
    const rendered: string[] = [];
    const all = [...this.entries];
    if (this.streaming.trim().length > 0) {
      all.push({ who: "thread", text: this.streaming.trim() });
    }
    for (const entry of all) {
      const prefix = entry.who === "you" ? "you: " : entry.who === "thread" ? "" : "· ";
      for (const paragraph of `${prefix}${entry.text}`.split("\n")) {
        rendered.push(...wrapLine(paragraph, width));
      }
      rendered.push("");
    }
    // Tail truncation: the overlay is height-bounded by `overlayOptions`, so
    // the newest turns are the ones that must stay visible.
    return rendered.length > MAX_TRANSCRIPT_LINES ? rendered.slice(rendered.length - MAX_TRANSCRIPT_LINES) : rendered;
  }
}

/** Minimal `ctx.ui.custom` surface — kept duck-typed for the same reason the component avoids `pi-tui` imports. */
export interface OverlayCustomCtx {
  ui: {
    custom<T>(
      factory: (tui: OverlayTui, theme: unknown, keybindings: unknown, done: (result: T) => void) => OverlayChatComponent,
      options?: { overlay?: boolean; overlayOptions?: unknown },
    ): Promise<T>;
  };
}

/**
 * Shows one thread's overlay chat and resolves when it closes (by `/done`,
 * by Escape, or because another `/answer` closed it). `onOpened` hands the
 * caller a close function so `ask.ts` can enforce §5's one-overlay-at-a-time
 * rule without reaching into the component itself.
 */
export async function openOverlayChat(
  ctx: OverlayCustomCtx,
  options: OverlayChatOptions & { onOpened?: (close: () => void) => void },
): Promise<void> {
  await ctx.ui.custom<undefined>(
    (tui, _theme, _keybindings, done) => {
      const component = new OverlayChatComponent(tui, options, done);
      options.onOpened?.(() => component.close());
      return component;
    },
    {
      overlay: true,
      overlayOptions: { width: "80%", maxHeight: "80%", anchor: "center" },
    },
  );
}

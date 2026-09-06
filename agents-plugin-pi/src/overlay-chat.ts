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
 *   - No static `@earendil-works/pi-tui` import: that package is not
 *     resolvable from this one under `node --test` (it is nested inside
 *     `pi-coding-agent`'s own `node_modules`), so the `Component` contract is
 *     satisfied structurally and the input box, wrapping, width measurement
 *     and the Escape-key matcher are small local implementations. Pi's own
 *     Markdown renderer IS reachable when Pi loads this extension (jiti), so
 *     `openOverlayChat` tries a guarded dynamic `import()` of `pi-tui` +
 *     `pi-coding-agent` and, when both resolve, hands the component a
 *     `renderMarkdown` for thread text; otherwise thread text wraps as plain
 *     text (the path the tests exercise). Theme colors are applied AFTER
 *     padding, and `visibleWidth` ignores ANSI sequences, so a styled line
 *     still measures and pads correctly.
 *   - The transcript is NOT owned by the component (dogfood 2026-09-05: Esc
 *     then `/answer` opened an empty view). It is seeded from
 *     `initialEntries` and every append is reported through
 *     `onTranscriptChange`, so `ask.ts` can persist it on the thread record
 *     and restore it across reopen and restart.
 *   - Owner lines render as a padded block under the theme's
 *     `userMessageBg` (the way Pi paints its own user messages) when the
 *     theme offers `bg`; the no-theme fallback keeps the `you: ` prefix.
 *   - Escape is matched by `isEscapeKey`, not by `data === "\x1b"`: Pi runs
 *     the terminal with the kitty keyboard protocol / modifyOtherKeys
 *     enabled, so a real Escape press arrives as `\x1b[27u`, `\x1b[27;1u`
 *     (optionally with `:<event>` suffixes) or `\x1b[27;1;27~` — none of
 *     which the bare-ESC comparison ever saw (dogfood 2026-09-05, Pi 0.84.4).
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
  /** ISO timestamp the thread was registered at, rendered in the header (review relay #1 I4). */
  createdAt?: string;
  /**
   * Whether `/done` asks the fork for a closing summary first (review relay
   * #2 C2). True for a discussion fork this surface spawned; FALSE for a live
   * task fork that merely raised a question mid-task — asking that one to
   * summarize would derail the task the lead is waiting on, so `/done` just
   * closes the view. Defaults to true.
   */
  summarizeOnDone?: boolean;
  channel: ForkChannel;
  /**
   * Called once when the thread ends — the owner's `/done` (with the fork's
   * own summary, or an empty string when `summarizeOnDone` is false and no
   * summary was ever requested), or an external `closeWithSummary(text)`
   * (with that text).
   */
  onDone: (summary: string) => void;
  /**
   * Transcript to restore on (re)open. When non-empty, the registered
   * `question` is NOT seeded again — it is already the first note in there.
   */
  initialEntries?: readonly TranscriptEntry[];
  /** Reported with the full transcript after every append (never for the streaming tail). */
  onTranscriptChange?: (entries: TranscriptEntry[]) => void;
  /**
   * Host Markdown renderer for `thread` entries and the streaming tail:
   * returns already-styled lines for `width` columns. Absent under
   * `node --test` (see the file header), in which case thread text wraps as
   * plain text via `wrapLine`.
   */
  renderMarkdown?: (text: string, width: number) => string[];
}

/** One transcript line-group: an owner turn, a settled thread turn, or an adapter note. Plain data — persisted on the thread record. */
export interface TranscriptEntry {
  who: "you" | "thread" | "note";
  text: string;
}

/** Terminal bracketed-paste markers (`\x1b[?2004h` mode), which Pi enables on the real TTY. */
export const PASTE_START = "\x1b[200~";
export const PASTE_END = "\x1b[201~";

/**
 * Kitty keyboard-protocol CSI-u key report: `ESC [ <codepoint>[:<shifted>[:<base>]] [; <modifier>[:<event>]] u`.
 * Same shape `pi-tui`'s `parseKittySequence` accepts (that package is not
 * importable from here — see the file header); only the codepoint and
 * modifier groups are consulted.
 */
const KITTY_CSI_U = /^\x1b\[(\d+)(?::(\d*))?(?::(\d+))?(?:;(\d+))?(?::(\d+))?u$/;

/** modifyOtherKeys (xterm) report of an unmodified Escape press. */
const MODIFY_OTHER_KEYS_ESCAPE = "\x1b[27;1;27~";

/**
 * Whether one input chunk is an UNMODIFIED Escape press, in any of the
 * encodings a Pi-driven terminal produces: bare `\x1b`, the kitty CSI-u form
 * with codepoint 27 and either no modifier field or modifier `1` (= none),
 * any `:<event>` suffix allowed, and the modifyOtherKeys form. A modified
 * Escape (`\x1b[27;2u` = Shift+Esc, ...) is deliberately NOT an Escape here:
 * it is somebody else's chord, and the catch-all in `handleInput` drops it
 * whole.
 */
export function isEscapeKey(data: string): boolean {
  if (data === "\x1b" || data === MODIFY_OTHER_KEYS_ESCAPE) return true;
  const match = KITTY_CSI_U.exec(data);
  if (!match || match[1] !== "27") return false;
  const modifier = match[4];
  return modifier === undefined || modifier === "1";
}

/**
 * Optional duck-typed slice of Pi's theme (`ctx.ui.custom`'s second factory
 * argument): `fg(color, text)` wraps `text` in the ANSI foreground color
 * named `color` (`"border"`, `"dim"`), `bg(color, text)` in the background
 * named `color` (`"userMessageBg"` — Pi's own user-message ground). Absent in
 * tests, or supplied as identity stubs.
 */
export interface OverlayTheme {
  fg?: (color: string, text: string) => string;
  bg?: (color: string, text: string) => string;
}

/** The theme background Pi paints its own user messages with; owner lines reuse it so they read as the owner's. */
export const OWNER_LINE_BG = "userMessageBg";

/**
 * Review relay #1 I4: owner-facing rendering of a thread's registration time.
 * Deliberately UTC-and-labeled rather than locale-formatted so the header is
 * identical in a test run, a CI container and the owner's terminal.
 * `undefined` for a missing or unparseable timestamp — an old registry entry
 * must not break the header.
 */
export function formatSpawnTime(iso: string | undefined): string | undefined {
  if (!iso) return undefined;
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return undefined;
  return `${at.toISOString().slice(0, 16).replace("T", " ")} UTC`;
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

/** Narrowest width that still fits a box (two border glyphs, two padding spaces, one content column). */
export const BOX_MIN_WIDTH = 5;

/** CSI (`ESC [ ... final`) and OSC (`ESC ] ... BEL|ST`) sequences — what theme `fg`/`bg` and the host Markdown renderer emit. */
const ANSI_SEQUENCE = /\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g;

/** Removes ANSI CSI/OSC sequences, leaving the printable text. */
export function stripAnsi(text: string): string {
  return text.replace(ANSI_SEQUENCE, "");
}

/**
 * Display width of a string: code points, counting the common East-Asian
 * wide ranges as two columns and ANSI escape sequences as zero. A local
 * approximation of `pi-tui`'s own `visibleWidth` (that package is not
 * statically resolvable from here — see the file header); exact
 * per-emulator behavior for CJK/IME input is a human-runbook verification
 * item, not something this adapter can assert offline.
 */
export function visibleWidth(text: string): number {
  let width = 0;
  for (const char of stripAnsi(text)) {
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

/**
 * One row of the transcript area, before box padding: `text` is measured
 * with `visibleWidth`; `ownerBlock` rows are padded to the full interior and
 * wrapped in the owner background AFTER padding, so the block is solid.
 */
interface TranscriptRow {
  text: string;
  ownerBlock?: boolean;
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
  private readonly theme: OverlayTheme | undefined;
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
  /** Width `cachedLines` was produced for — the cache is only valid for that width (review relay #1 I2). */
  private cachedWidth: number | undefined;
  /**
   * `channel.isStreaming()` value `cachedLines` was produced for (review
   * relay #1 Critical). The flag can flip with NO event this component ever
   * observes — the dormant-relaunch channel attaches its listener only after
   * `sendToAgent` resolves, so an early `agent_start` on the freshly-created
   * client never reaches `handleEvent`, and `clearLiveState` can drop the
   * flag on a failed liveness probe with no event at all. Keying the cache on
   * this value too means ANY repaint (not only ones a known event triggered)
   * re-derives the marker from the current flag, so a stale cached frame can
   * never outlive the state it was built from.
   */
  private cachedStreaming: boolean | undefined;
  /** Inside a bracketed paste whose start marker arrived in an earlier chunk (review relay #1 I3). */
  private pasting = false;

  constructor(tui: OverlayTui, options: OverlayChatOptions, done: (result: undefined) => void, theme?: OverlayTheme) {
    this.tui = tui;
    this.options = options;
    this.done = done;
    this.theme = theme;
    if (options.initialEntries && options.initialEntries.length > 0) {
      // Restored transcript (reopen after Esc, or after a lead restart): the
      // question note is already its first entry, so it is not seeded twice.
      this.entries = options.initialEntries.map((entry) => ({ who: entry.who, text: entry.text }));
    } else if (options.question) {
      this.append({ who: "note", text: options.question });
    }
    this.unsubscribe = options.channel.onEvent((evt) => this.handleEvent(evt));
  }

  /** The only transcript write path: every append is reported to `onTranscriptChange` so the owner of the thread record can persist it. */
  private append(entry: TranscriptEntry): void {
    this.entries.push(entry);
    this.options.onTranscriptChange?.([...this.entries]);
  }

  /**
   * External finish (the thread's own `kind:"final"` report, routed here by
   * `ask.ts`): closes the view and reports `summary` through `onDone` WITHOUT
   * sending `buildDoneSummaryPrompt()` — the fork already said what was
   * decided. Any half-streamed turn is discarded. A no-op once finished.
   */
  closeWithSummary(summary: string): void {
    if (this.finished) return;
    this.streaming = "";
    this.donePending = false;
    if (summary.trim().length > 0) this.append({ who: "thread", text: summary.trim() });
    this.finish(summary);
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
    // Review relay #1 Critical: this component's own render cache also keys
    // on `channel.isStreaming()` now (see `render()`), so this refresh is
    // belt-and-suspenders rather than the sole trigger — but it is still the
    // one that makes the working marker appear PROMPTLY (well before any
    // delta) for a channel whose listener was already attached when the
    // fork's run started (the live, already-open-overlay case): without it,
    // nothing requests a repaint until the next unrelated event, and the
    // marker would only appear once something else happened to redraw.
    if (e.type === "agent_start") {
      this.refresh();
      return;
    }
    if (e.type !== "agent_settled") return;

    const settled = this.streaming.trim();
    this.streaming = "";
    if (settled.length > 0) {
      this.append({ who: "thread", text: settled });
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
    this.cachedWidth = undefined;
    this.cachedStreaming = undefined;
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
    this.cachedWidth = undefined;
    this.cachedStreaming = undefined;
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
      if (this.options.summarizeOnDone === false) {
        // Review relay #2 C2: nothing is sent to the fork and nothing is
        // waited for — this thread's respondent is a live task fork that
        // keeps working; `/done` only closes the owner's view of it.
        this.finish("");
        return;
      }
      this.donePending = true;
      // Any half-streamed turn belongs to the exchange the owner just ended;
      // leaving it in the buffer would make it the leading text of the
      // summary turn (review relay #1 M11).
      this.streaming = "";
      this.append({ who: "note", text: "ending the thread — asking for a summary…" });
      this.deliver(buildDoneSummaryPrompt());
      this.refresh();
      return;
    }

    this.append({ who: "you", text });
    this.deliver(text);
    this.refresh();
  }

  private deliver(text: string): void {
    // Fire-and-report: a delivery failure becomes a transcript note rather
    // than an unhandled rejection inside a keystroke handler. The success
    // refresh (review relay #1 Critical) covers the dormant-relaunch path:
    // `createForkChannel`'s own listener sync (`ask.ts`) attaches to a
    // freshly-spawned client only AFTER `sendToAgent` resolves, so an
    // `agent_start` the new client already emitted during that call never
    // reaches this component's `handleEvent` — but `isStreaming()` reads the
    // registry flag directly, independent of that listener, so it is already
    // correct by the time `send()` resolves; this refresh is what gets a
    // `render()` call to look at it, rather than replaying whatever was
    // cached before the send.
    void this.options.channel.send(text).then(
      () => this.refresh(),
      (err: unknown) => {
        this.append({ who: "note", text: `delivery failed: ${err instanceof Error ? err.message : String(err)}` });
        this.donePending = false;
        this.refresh();
      },
    );
  }

  /**
   * Review relay #1 I3: a terminal in bracketed-paste mode wraps pasted text
   * in `\x1b[200~`/`\x1b[201~`, so the whole paste starts with ESC and the
   * catch-all escape filter below would drop it silently. Markers are
   * stripped and the payload inserted as text; `pasting` carries the state
   * across chunks, since a large paste arrives split across several reads.
   * Newlines inside a paste become spaces rather than submits — a pasted
   * paragraph must not fire off half-messages.
   */
  private handlePaste(data: string): void {
    let text = data;
    if (!this.pasting) {
      text = text.slice(PASTE_START.length);
      this.pasting = true;
    }
    const end = text.indexOf(PASTE_END);
    if (end >= 0) {
      text = text.slice(0, end);
      this.pasting = false;
    }
    this.insertText(text.replace(/\r\n|\r|\n/g, " "));
  }

  /** Inserts the printable characters of `text` at the cursor; ignores the rest. */
  private insertText(text: string): void {
    const printable = [...text].filter((char) => {
      const code = char.codePointAt(0) ?? 0;
      return code >= 0x20 && code !== 0x7f;
    });
    if (printable.length === 0) return;
    const inserted = printable.join("");
    this.input = this.input.slice(0, this.cursor) + inserted + this.input.slice(this.cursor);
    this.cursor += inserted.length;
    this.refresh();
  }

  handleInput(data: string): void {
    if (this.pasting || data.startsWith(PASTE_START)) {
      this.handlePaste(data);
      return;
    }
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
    if (isEscapeKey(data)) {
      // Escape (bare, kitty CSI-u, or modifyOtherKeys): close the view. §5 —
      // this has no effect on the fork. Must run before the `\x1b` catch-all
      // below, which would otherwise swallow the CSI-u encodings.
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
    this.insertText(data);
  }

  /**
   * Draws the view as a rounded box (`╭─╮ │ │ ╰─╯`, the shape Pi's own
   * `overlay-test` example uses) so the overlay is visibly distinct from the
   * transcript it floats over. Every emitted line is EXACTLY `width` columns:
   * the interior is `width - 4` (one space of padding inside each border
   * glyph), content is wrapped to that interior and space-padded with the
   * local `visibleWidth`, and theme colors are applied only after padding.
   * Below `BOX_MIN_WIDTH` there is no room for a box, so the content renders
   * bare — still never wider than `width`.
   */
  render(width: number): string[] {
    const w = Math.max(1, width);
    const streamingNow = this.options.channel.isStreaming();
    // The cache is keyed on width (a resize between renders must re-wrap
    // rather than replay lines built for the old width — review relay #1 I2)
    // AND on `channel.isStreaming()` (review relay #1 Critical): the working
    // marker is read fresh from the flag every render, not only when a known
    // event refreshed the cache, so a repaint that this component did not
    // itself request (or one whose triggering event this component drops)
    // still reflects the CURRENT flag rather than replaying a frame built
    // when it last differed.
    if (this.cachedLines && this.cachedWidth === w && this.cachedStreaming === streamingNow) return this.cachedLines;

    const boxed = w >= BOX_MIN_WIDTH;
    const inner = boxed ? w - 4 : w;
    const fg = (color: string, text: string): string => this.theme?.fg?.(color, text) ?? text;
    const bg = (color: string, text: string): string => this.theme?.bg?.(color, text) ?? text;
    const pad = (text: string): string => text + " ".repeat(Math.max(0, inner - visibleWidth(text)));
    const row = (text: string, color?: string, ownerBlock?: boolean): string => {
      const padded = pad(text);
      const content = ownerBlock ? bg(OWNER_LINE_BG, padded) : color ? fg(color, padded) : padded;
      return boxed ? `${fg("border", "│")} ${content} ${fg("border", "│")}` : content;
    };

    const lines: string[] = [];
    if (boxed) lines.push(fg("border", `╭${"─".repeat(inner + 2)}╮`));

    // §5: the header names the thread explicitly, so the two lead-voiced
    // agents (the lead itself and this discussion fork) can never be
    // confused for one another on screen. The registration time (I4) sits
    // with it, so a reopened dormant thread is placeable in time.
    for (const line of wrapLine(`ws thread ${this.options.threadId} · ${this.options.title}`, inner)) lines.push(row(line));
    const opened = formatSpawnTime(this.options.createdAt);
    if (opened) for (const line of wrapLine(`  opened ${opened}`, inner)) lines.push(row(line));
    // Moved here from the footer (260905): the overlay is height-bounded and
    // caps at 24 transcript rows, so on a short terminal the footer was the
    // first line pushed out of view — the header is always on screen. Still
    // deliberately generic about what `/done` does beyond closing: the two
    // thread origins close differently (review relay #2 C2), and this line is
    // not the place to explain the difference.
    for (const line of wrapLine(`Esc: close view (thread stays open) · ${DONE_COMMAND}: end thread`, inner)) {
      lines.push(row(line, "dim"));
    }
    lines.push(row(""));

    for (const line of this.transcriptRows(inner)) lines.push(row(line.text, undefined, line.ownerBlock));

    lines.push(row(""));
    for (const line of wrapLine(`> ${this.input}`, inner)) lines.push(row(line));

    if (boxed) lines.push(fg("border", `╰${"─".repeat(inner + 2)}╯`));

    this.cachedLines = lines;
    this.cachedWidth = w;
    this.cachedStreaming = streamingNow;
    return lines;
  }

  /**
   * Transcript rows for an interior of `width` columns. Owner turns become a
   * solid block (one space of inner padding, so the text is wrapped to
   * `width - 2`) when the theme can paint a background, and a `you: `-prefixed
   * paragraph otherwise; thread turns and the streaming tail go through the
   * host Markdown renderer when one was supplied; notes keep their `· ` mark.
   */
  private transcriptRows(width: number): TranscriptRow[] {
    const rendered: TranscriptRow[] = [];
    const all = [...this.entries];
    if (this.streaming.trim().length > 0) {
      all.push({ who: "thread", text: this.streaming.trim() });
    }
    const ownerBlocks = typeof this.theme?.bg === "function";
    for (const entry of all) {
      if (entry.who === "you" && ownerBlocks) {
        const textWidth = Math.max(1, width - 2);
        for (const paragraph of entry.text.split("\n")) {
          for (const line of wrapLine(paragraph, textWidth)) {
            rendered.push({ text: ` ${line}${" ".repeat(Math.max(0, textWidth - visibleWidth(line)))} `, ownerBlock: true });
          }
        }
      } else if (entry.who === "thread") {
        for (const line of this.renderThreadText(entry.text, width)) rendered.push({ text: line });
      } else {
        const prefix = entry.who === "you" ? "you: " : "· ";
        for (const paragraph of `${prefix}${entry.text}`.split("\n")) {
          for (const line of wrapLine(paragraph, width)) rendered.push({ text: line });
        }
      }
      rendered.push({ text: "" });
    }
    // 260905: the working marker lives ONLY in this render-time slot — it is
    // never pushed to `this.entries`/`all` above, so it can never reach
    // `append()`/`onTranscriptChange` and therefore never the persisted
    // transcript. Read fresh from `channel.isStreaming()` (the registry's
    // streaming flag, not `agent_start`/`agent_settled` events this component
    // receives) so it is already correct on the very first `render()` call
    // for a channel that was already mid-turn when the overlay attached
    // (fork-raised mid-turn, dormant-relaunch first message) — two paths that
    // never deliver a start event here. Pushed as a plain row, bypassing
    // `renderThreadText`/Markdown: "working…" is not thread content.
    if (this.streaming.trim().length === 0 && this.options.channel.isStreaming()) {
      rendered.push({ text: "working…" });
    }
    // Tail truncation: the overlay is height-bounded by `overlayOptions`, so
    // the newest turns are the ones that must stay visible.
    return rendered.length > MAX_TRANSCRIPT_LINES ? rendered.slice(rendered.length - MAX_TRANSCRIPT_LINES) : rendered;
  }

  /**
   * Thread text through `renderMarkdown` when present, else `wrapLine`. Every
   * line the host renderer returns is re-checked against the local
   * `visibleWidth` and, when too wide (or when the renderer throws), its plain
   * text is re-wrapped — the box must stay aligned regardless of what the
   * renderer does with a width it disagrees about.
   */
  private renderThreadText(text: string, width: number): string[] {
    const plain = (source: string): string[] => source.split("\n").flatMap((paragraph) => wrapLine(paragraph, width));
    const renderMarkdown = this.options.renderMarkdown;
    if (!renderMarkdown) return plain(text);
    let lines: string[];
    try {
      lines = renderMarkdown(text, width);
    } catch {
      return plain(text);
    }
    if (!Array.isArray(lines)) return plain(text);
    return lines.flatMap((line) => (typeof line === "string" && visibleWidth(line) <= width ? [line] : plain(stripAnsi(String(line)))));
  }
}

/** Minimal `ctx.ui.custom` surface — kept duck-typed for the same reason the component avoids `pi-tui` imports. */
export interface OverlayCustomCtx {
  ui: {
    custom<T>(
      factory: (tui: OverlayTui, theme: OverlayTheme | undefined, keybindings: unknown, done: (result: T) => void) => OverlayChatComponent,
      options?: { overlay?: boolean; overlayOptions?: unknown },
    ): Promise<T>;
  };
}

/** What `openOverlayChat` hands back through `onOpened`: the two external ways to end the view. */
export interface OverlayHandle {
  /** Close the view only (the thread is untouched). */
  close(): void;
  /** End the view with a supplied summary — see `OverlayChatComponent.closeWithSummary`. */
  closeWithSummary(summary: string): void;
}

/** Structural slice of `pi-tui`'s `Markdown` component and `pi-coding-agent`'s theme export, as reached through the dynamic imports below. */
interface MarkdownHostModules {
  tui: { Markdown: new (text: string, paddingX: number, paddingY: number, theme: unknown) => { render(width: number): string[] } };
  pi: { getMarkdownTheme(): unknown };
}

/**
 * Pi's own Markdown renderer, when the host can supply it. Both packages are
 * reached through guarded dynamic `import()`s: neither is resolvable from
 * this package under `node --test` (`pi-tui` is nested inside
 * `pi-coding-agent`'s own `node_modules`; `pi-coding-agent`'s runtime exports
 * are only reachable once Pi loads this extension via jiti), so a failed
 * import resolves to `undefined` and thread text falls back to plain
 * wrapping. `pi-coding-agent` is only attempted after `pi-tui` resolved —
 * the theme is useless without the component.
 */
export async function loadMarkdownRenderer(): Promise<((text: string, width: number) => string[]) | undefined> {
  let tui: MarkdownHostModules["tui"] | undefined;
  try {
    tui = (await import("@earendil-works/pi-tui")) as unknown as MarkdownHostModules["tui"];
  } catch {
    return undefined;
  }
  let pi: MarkdownHostModules["pi"] | undefined;
  try {
    pi = (await import("@earendil-works/pi-coding-agent")) as unknown as MarkdownHostModules["pi"];
  } catch {
    return undefined;
  }
  if (typeof tui?.Markdown !== "function" || typeof pi?.getMarkdownTheme !== "function") return undefined;
  const Markdown = tui.Markdown;
  const theme = pi.getMarkdownTheme();
  return (text, width) => new Markdown(text, 0, 0, theme).render(width);
}

/**
 * Shows one thread's overlay chat and resolves when it closes (by `/done`,
 * by Escape, by `closeWithSummary`, or because another `/answer` closed it).
 * `onOpened` hands the caller an `OverlayHandle` so `ask.ts` can enforce §5's
 * one-overlay-at-a-time rule — and route a fork's own final report into the
 * view — without reaching into the component itself.
 */
export async function openOverlayChat(
  ctx: OverlayCustomCtx,
  options: OverlayChatOptions & { onOpened?: (handle: OverlayHandle) => void },
): Promise<void> {
  const renderMarkdown = options.renderMarkdown ?? (await loadMarkdownRenderer());
  await ctx.ui.custom<undefined>(
    (tui, theme, _keybindings, done) => {
      const component = new OverlayChatComponent(tui, { ...options, renderMarkdown }, done, theme);
      options.onOpened?.({ close: () => component.close(), closeWithSummary: (summary) => component.closeWithSummary(summary) });
      return component;
    },
    {
      overlay: true,
      overlayOptions: { width: "80%", maxHeight: "80%", anchor: "center" },
    },
  );
}

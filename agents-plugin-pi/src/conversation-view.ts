/**
 * `260908-feat-ws-pi-conversation-view-component` Phase 1: the shared
 * `ConversationViewComponent` — a `pi-tui` `Component` (`render(width)` /
 * `handleInput(data)`) that draws one child's transcript (owner turns, lead
 * messages, tool calls/results, and adapter notes) and, once promoted to
 * `"interactive"` mode, routes typed input back to it through a
 * `ConversationChannel`.
 *
 * Phase 2 (`ask.ts` rebinding onto this component, the old per-thread overlay
 * module's deletion, legacy transcript hydration into `ConversationItem[]`)
 * has since landed; the audit window, the owner-steering ownership rule, and
 * the Esc modal remain child B's. This file defines the shared component and
 * its structural contract, tested offline against a fake `ConversationChannel`
 * and a fake `tui`.
 *
 * Shape decisions:
 *   - `ConversationItem` has six kinds, per the ticket's Decisions message
 *     model: `"user"` (an owner turn), `"assistant"` (the CHILD's own
 *     finalized text — rendered as Markdown, no label), `"lead-message"` (a
 *     message the LEAD sent the child — carries a `lead:` label precisely so
 *     it is never mistaken for the child's own text), `"note"` (an adapter
 *     note), `"tool-call"` and `"tool-result"` (both carry an `id: string` so
 *     a later phase can correlate a call with its result; `tool-result`'s
 *     payload field is `content`, matching the ticket's field name). The
 *     child's streamed `text_delta`/`agent_settled` turn is committed and
 *     rendered as `"assistant"`, never `"lead-message"` — the old per-thread
 *     overlay component committed the identical stream as the child's own
 *     turn (`who: "thread"`), and this component preserves that direction.
 *   - `ConversationChannel` is the old overlay component's `ForkChannel` with
 *     `isStreaming(): boolean` widened to `liveness(): ChildLiveness` — a
 *     3-state read (`"running" | "idle-awaiting-owner" | "settled"`) taken
 *     FRESH on every `render()`, never cached across a call, so a state flip
 *     with no accompanying event still shows up on the very next repaint
 *     (the same class of gap the old overlay component's own `cachedStreaming`
 *     field worked around). Producing `"idle-awaiting-owner"` from a real registry
 *     is child B's ownership rule (`260908` sibling ticket) — this phase only
 *     defines the type and renders its three states structurally against a
 *     fake channel. `send` is OPTIONAL on the interface (`"interactive" mode
 *     only`) so a `"view"`-mode-only consumer (e.g. child B's audit window)
 *     is not forced to implement it.
 *   - `mode` is `"view" | "interactive"`, and `setMode` is RAISE-ONLY: once
 *     `"interactive"`, a later `setMode("view")` call is a no-op. A view a
 *     host already promoted to accept typed input must never be silently
 *     demoted back to read-only under it.
 *   - Tool call/result heads and expanded bodies reuse
 *     `tool-result-render.ts`'s already-pure preview helpers
 *     (`yamlInputPreview`, `completedTextPreview`/`logicalPreview`,
 *     `yamlContainerDisplay`) — this component supplies only the
 *     collapse/expand chrome around them, never its own preview logic. A
 *     `"tool-call"` item is expandable only when `yamlInputPreview` produces
 *     a non-empty body (object-shaped args); a `"tool-result"` item is
 *     expandable only when `yamlContainerDisplay` recognizes its content as a
 *     JSON container. A non-expandable item never enters focus cycling.
 *   - `isEscapeKey` (kitty-protocol-safe Esc detection) is COPIED from the
 *     old per-thread overlay component rather than imported: that module was
 *     always slated for Phase 2 deletion, and importing from a
 *     soon-to-be-deleted file would just have moved the coupling problem to
 *     Phase 2 instead of avoiding it up front.
 *   - Key-handling precedence: `\x03` (Ctrl+C) is swallowed first in both
 *     modes; `isEscapeKey` next, invoking `onEscape`, in both modes; then
 *     `Ctrl+O` (`\x0f`) toggles collapse/expand for every expandable item at
 *     once, in both modes — these three are non-printable chords that can
 *     never be legitimate typed content. In `"view"` mode (no `Editor`
 *     capturing input) the remaining keys are component navigation:
 *     Tab/Shift+Tab move the item selection (newest expandable item first),
 *     Space toggles the selected item, Enter calls `onEnter` (there is no
 *     input box to submit). In `"interactive"` mode the ticket's precedence
 *     table applies: Tab/Shift+Tab move the selection only while the
 *     `Editor` is EMPTY; Space toggles the selection only while a selection
 *     is ACTIVE; any other typed character (including Tab/Shift+Tab/Space in
 *     the states above) first clears the selection, then is forwarded to
 *     `this.editor.handleInput`. The `Editor`'s own `onSubmit` intercepts a
 *     trimmed `/done` (calls `onDone`) and otherwise appends a `"user"` item
 *     and `channel.send`s it.
 *   - `Editor` is the one `pi-tui` primitive that needs the FULL `TUI`
 *     surface (`showOverlay`, `setFocus`, 20+ members) — the fake `tui` the
 *     test tier drives (`requestRender` only) does not implement it, while
 *     `ScrollView`/`Markdown`/`Text` construct and render fine against the
 *     real classes with no `tui` reference at all. `primitives` is therefore
 *     an injectable option (default: `./pi-tui.ts`'s static classes) so tests
 *     inject a minimal fake for `Editor` only, and so live wiring (Phase 2 /
 *     child B) can later feed host-resolved classes through the very same
 *     seam without this component's own code changing — see `pi-tui.ts`'s
 *     Addendum doc comment for why the default classes and the live host's
 *     classes are not always the same physical objects.
 *   - Width invariant: every rendered line's `visibleWidth(line) <= width`.
 *     `pi-tui`'s own `Text`/`Markdown` already pad/wrap to exactly `width`
 *     (confirmed against their compiled output), but `render()` still runs a
 *     defensive final `truncateToWidth` pass over every line — the same
 *     belt-and-suspenders discipline the old overlay component's
 *     `renderThreadText` used for a host-supplied renderer it did not fully
 *     trust.
 */

import {
  Editor as RealEditor,
  Markdown as RealMarkdown,
  ScrollView as RealScrollView,
  Text as RealText,
  truncateToWidth,
  type Component,
  type EditorTheme,
  type MarkdownTheme,
  type SelectListTheme,
  type TuiMouseEvent,
  type TuiMouseEventResult,
} from "./pi-tui.ts";
import { completedTextPreview, logicalPreview, yamlContainerDisplay, yamlInputPreview } from "./tool-result-render.ts";
import { visibleWidth } from "./text-width.ts";

/** The literal the owner types to end an interactive session (mirrors the old overlay component's own `DONE_COMMAND`). */
export const DONE_COMMAND = "/done";

/**
 * Kitty keyboard-protocol CSI-u key report, and the modifyOtherKeys escape
 * report — copied from the old overlay component's well-tested `isEscapeKey` (see
 * this file's header for why it is copied rather than imported).
 */
const KITTY_CSI_U = /^\x1b\[(\d+)(?::(\d*))?(?::(\d+))?(?:;(\d+))?(?::(\d+))?u$/;
const MODIFY_OTHER_KEYS_ESCAPE = "\x1b[27;1;27~";

/** Whether one input chunk is an unmodified Escape press — see the old overlay component's `isEscapeKey` for the full rationale. */
export function isEscapeKey(data: string): boolean {
  if (data === "\x1b" || data === MODIFY_OTHER_KEYS_ESCAPE) return true;
  const match = KITTY_CSI_U.exec(data);
  if (!match || match[1] !== "27") return false;
  const modifier = match[4];
  return modifier === undefined || modifier === "1";
}

/** Ctrl+O — toggles collapse/expand for every expandable item at once. Not a printable chord, so it is always intercepted ahead of mode-specific routing. */
const CTRL_O = "\x0f";
/** Shift+Tab (CSI `Z`), as reported by every terminal Pi runs under. */
const SHIFT_TAB = "\x1b[Z";

/**
 * 260909 F2 (regression of 260905): the activity marker lives in the STREAMING
 * SLOT at the foot of the transcript — "where the tail would appear" — not in
 * the header. It is drawn only while the child is running and no streamed tail
 * has arrived yet; the first text delta replaces it.
 */
const WORKING_MARKER = "working…";

/** 260909 F2: the loud idle banner, rendered at BOTH the header and the transcript foot so it is hard to miss. */
const AWAITING_OWNER_BANNER = "── AWAITING OWNER ──";

/**
 * 260909 V3: a left gutter that visually sets a model (`"assistant"`) turn —
 * and its streaming tail — apart from owner/lead/note/tool rows, WITHOUT
 * reintroducing a misleading label (the header rationale keeps `"assistant"`
 * label-free precisely so it is not mistaken for a lead message). A thin block
 * gutter reads as "the model spoke here" on its own.
 */
const ASSISTANT_GUTTER = "▌ ";

/**
 * 260909 V1/V2: columns the overlay border chrome consumes on each rendered
 * line — one border glyph plus one column of horizontal margin on each side
 * (`│ … │`). Inner content is rendered at `width - BORDER_OVERHEAD` and then
 * wrapped, so the finished line's visible width is exactly `width`.
 */
const BORDER_OVERHEAD = 4;

/**
 * 260909 V1/V2 (+ density polish): wraps already-rendered, `innerWidth`-clamped
 * inner lines in a single-line box border with a one-column horizontal margin,
 * plus one blank interior row just below the top border and just above the
 * bottom border so content never touches the box. Pure and exported so
 * `render()`'s chrome is unit-lockable on its own. Every returned line's
 * visible width is exactly `width`.
 */
export function wrapInBorder(innerLines: readonly string[], width: number, innerWidth: number): string[] {
  // The four chrome columns plus one content column need five columns. At
  // narrower widths, retain the content (clamped by callers) rather than
  // emitting over-wide Unicode.
  if (width <= BORDER_OVERHEAD) return innerLines.map((line) => truncateToWidth(line, Math.max(1, width)));
  const horizontal = "─".repeat(Math.max(0, width - 2));
  const top = `┌${horizontal}┐`;
  const bottom = `└${horizontal}┘`;
  // A leading and trailing blank row give the content vertical breathing room
  // inside the border.
  const body = ["", ...innerLines, ""].map((line) => {
    const pad = " ".repeat(Math.max(0, innerWidth - visibleWidth(line)));
    return `│ ${line}${pad} │`;
  });
  return [top, ...body, bottom];
}

/** `mode` values `ConversationViewComponent` can be in. See `setMode`'s raise-only contract. */
export type ConversationViewMode = "view" | "interactive";

/** One line-group in the transcript. See this file's header for the kind-by-kind rationale. */
export type ConversationItem =
  | { kind: "user"; text: string }
  | { kind: "assistant"; text: string }
  | { kind: "lead-message"; text: string }
  | { kind: "note"; text: string }
  | { kind: "tool-call"; id: string; name: string; args: unknown }
  | { kind: "tool-result"; id: string; name: string; content: string; isError?: boolean };

/**
 * A child's liveness, read fresh on every `render()` — never cached. Producing
 * `"idle-awaiting-owner"` from a real registry is child B's ownership rule;
 * this phase only defines the type and renders its three states.
 */
export type ChildLiveness = "running" | "idle-awaiting-owner" | "settled";

/**
 * The component's only route to its child. The old overlay component's `ForkChannel`
 * widened to a 3-state `liveness()` in place of `isStreaming()` — everything
 * else (`onEvent`, `send`) is unchanged, so the same wire events
 * (`message_update`/`text_delta`, `agent_start`, `agent_settled`) drive both.
 */
export interface ConversationChannel {
  /** Subscribe to the child's RPC event stream. Returns an unsubscribe function. */
  onEvent(listener: (evt: unknown) => void): () => void;
  /** Read fresh on every render — see the type doc above. */
  liveness(): ChildLiveness;
  /** Deliver one owner message to the child. Optional: `"interactive"` mode only — a `"view"`-only consumer need not implement it. */
  send?(text: string): Promise<void>;
}

/** Minimal `pi-tui` `TUI` surface this component needs directly. The real `TUI` (needed by the real `Editor`) is a structural superset — see the `primitives` doc below. */
export interface ConversationViewTui {
  requestRender(): void;
  /** The live host TUI exposes terminal rows; test fakes may omit it. */
  terminal?: { rows?: number };
}

/** The shared overlays use `maxHeight: "80%"`; derive that live cap from the host terminal on every render. */
export function conversationOverlayHeight(tui: ConversationViewTui): number {
  const rows = tui.terminal?.rows;
  return typeof rows === "number" && rows > 0 ? Math.max(1, Math.floor(rows * 0.8)) : Number.POSITIVE_INFINITY;
}

interface ScrollViewLike extends Component {
  readonly scrollTop: number;
  readonly isFollowingEnd: boolean;
  readonly viewportHeight: number;
  updateLayout(contentHeight: number, viewportHeight: number, requestRender: () => void): void;
  scrollBy(lines: number): number;
  scrollToStart(): void;
  scrollToEnd(): void;
}

/** The `pi-tui` primitive surface `Editor` needs beyond `Component` — `handleInput` narrowed from `Component`'s optional to required, since this component always forwards keys to it while interactive. */
export interface EditorLike extends Component {
  onSubmit?: (text: string) => void;
  getText(): string;
  setText(text: string): void;
  handleInput(data: string): void;
}

/**
 * Injectable primitive constructors — see this file's header for why `Editor`
 * is the one primitive tests must fake. Each defaults to `./pi-tui.ts`'s
 * static classes.
 */
export interface ConversationViewPrimitives {
  ScrollView: new (component: Component, options?: { follow?: "none" | "end"; overscroll?: "chain" | "contain" }) => ScrollViewLike;
  Markdown: new (text: string, paddingX: number, paddingY: number, theme: MarkdownTheme) => Component;
  Text: new (text?: string, paddingX?: number, paddingY?: number, customBgFn?: (text: string) => string) => Component;
  Editor: new (tui: unknown, theme: EditorTheme) => EditorLike;
}

const IDENTITY_MARKDOWN_THEME: MarkdownTheme = {
  heading: (t) => t,
  link: (t) => t,
  linkUrl: (t) => t,
  code: (t) => t,
  codeBlock: (t) => t,
  codeBlockBorder: (t) => t,
  quote: (t) => t,
  quoteBorder: (t) => t,
  hr: (t) => t,
  listBullet: (t) => t,
  bold: (t) => t,
  italic: (t) => t,
  strikethrough: (t) => t,
  underline: (t) => t,
};

const IDENTITY_SELECT_LIST_THEME: SelectListTheme = {
  selectedPrefix: (t) => t,
  selectedText: (t) => t,
  description: (t) => t,
  scrollInfo: (t) => t,
  noMatch: (t) => t,
};

const IDENTITY_EDITOR_THEME: EditorTheme = {
  borderColor: (t) => t,
  selectList: IDENTITY_SELECT_LIST_THEME,
};

const DEFAULT_PRIMITIVES: ConversationViewPrimitives = {
  ScrollView: RealScrollView as unknown as ConversationViewPrimitives["ScrollView"],
  Markdown: RealMarkdown as unknown as ConversationViewPrimitives["Markdown"],
  Text: RealText as unknown as ConversationViewPrimitives["Text"],
  Editor: RealEditor as unknown as ConversationViewPrimitives["Editor"],
};

export interface ConversationViewOptions {
  channel: ConversationChannel;
  /** Overrides for one or more primitive constructors. Unset entries default to `./pi-tui.ts`'s static classes. */
  primitives?: Partial<ConversationViewPrimitives>;
  /** Transcript to seed at construction (Phase 2's hydration path). */
  initialItems?: readonly ConversationItem[];
  /**
   * The single hint line rendered in the header. Consumer-supplied because
   * different hosts need different text for the same key contract — e.g. the
   * `/answer` binding passes `Esc: close view (thread stays open) · /done: end
   * thread`, the audit window passes its own. Defaults to a generic,
   * mode-dependent hint describing this component's own key contract when
   * omitted.
   */
  headerHint?: string;
  /** Enter in `"view"` mode (no `Editor` to consume it) — e.g. a host promoting the view to `"interactive"`. */
  onEnter?: () => void;
  /** Esc, in either mode. */
  onEscape?: () => void;
  /** `/done`, typed and submitted while `"interactive"`. */
  onDone?: () => void;
  /** Host Markdown theme for `"assistant"`/`"lead-message"` items. Defaults to the identity theme (Phase 1's plain-text baseline) when omitted. */
  markdownTheme?: MarkdownTheme;
  /** Background painter for `"user"` items — e.g. `(text) => theme.bg("userMessageBg", text)`. Left unpainted when omitted. */
  userLineBg?: (text: string) => string;
  /** Muted foreground painter for tool-call/tool-result heads and expanded bodies. Left unpainted when omitted. */
  toolTextFg?: (text: string) => string;
  /** Dim foreground painter for the transient `working…` marker. Left unpainted when omitted. */
  workingTextFg?: (text: string) => string;
  /** Fired with a full copy of the transcript after every append — never for the streaming tail. Lets a host persist the transcript as it grows. */
  onItemsChange?: (items: readonly ConversationItem[]) => void;
  /** Live overlay height, supplied by each consumer from host TUI geometry. Unset keeps non-overlay/unit consumers unbounded. */
  viewportHeight?: () => number;
  /** Host keybinding matcher injected by the custom-overlay factory. */
  keybindings?: { matches(data: string, id: string): boolean };
  /**
   * 260909 V1/V2: draw a single-line box border (with a one-column horizontal
   * margin) around the whole view so it separates from the lead's background.
   * Opt-in — `/answer` and `/audit` overlays set it; compact embeds leave it
   * off (the default).
   */
  border?: boolean;
}

/**
 * Copied from `tool-result-render.ts`'s own private `isSingleTextContent` —
 * this component must not import a private helper from a file it does not
 * otherwise couple to (the same convention already used for `isEscapeKey`).
 */
function isSingleTextContent(content: unknown): content is Array<{ type: string; text?: string }> {
  return Array.isArray(content) && content.length === 1 && content[0]?.type === "text";
}

/**
 * `tool_execution_end`'s `result` -> the `"tool-result"` item's `content`
 * text: the single-text-content precedent, else a best-effort
 * stringification. Exported (260908 audit-window ticket) so `audit.ts`'s
 * session-file parser converts a persisted `toolResult` message's `content`
 * array (identical `{ content?: unknown }` shape) through the exact same
 * rule the live event path already uses, rather than a second copy.
 */
export function toolResultContentText(result: unknown): string {
  const content = (result as { content?: unknown } | null)?.content;
  if (isSingleTextContent(content)) return content[0]?.text ?? "";
  try {
    return JSON.stringify(result) ?? String(result);
  } catch {
    return String(result);
  }
}

function isToolCallExpandable(item: Extract<ConversationItem, { kind: "tool-call" }>): boolean {
  return yamlInputPreview(item.args).length > 0;
}

function isToolResultExpandable(item: Extract<ConversationItem, { kind: "tool-result" }>): boolean {
  return yamlContainerDisplay(item.content) !== undefined;
}

function toolCallHead(item: Extract<ConversationItem, { kind: "tool-call" }>): string {
  const full = yamlInputPreview(item.args);
  const first = full.length > 0 ? logicalPreview(full, 1) : "";
  return first ? `▸ ${item.name} ${first}` : `▸ ${item.name}`;
}

function toolCallBody(item: Extract<ConversationItem, { kind: "tool-call" }>): string[] {
  const full = yamlInputPreview(item.args);
  return full.length > 0 ? full.split("\n") : [];
}

function toolResultHead(item: Extract<ConversationItem, { kind: "tool-result" }>): string {
  const preview = completedTextPreview(item.content);
  const first = logicalPreview(preview.text, 1);
  const marker = item.isError ? "✗" : "✓";
  return `${marker} ${item.name} ${first}`.trimEnd();
}

function toolResultBody(item: Extract<ConversationItem, { kind: "tool-result" }>): string[] {
  const yaml = yamlContainerDisplay(item.content);
  return yaml !== undefined ? yaml.split("\n") : [];
}

/** A trivial `Component` wrapper around a live `render()` closure — the `ScrollView`'s one child. */
class ClosureComponent implements Component {
  private readonly getLines: (width: number) => string[];

  constructor(getLines: (width: number) => string[]) {
    this.getLines = getLines;
  }

  render(width: number): string[] {
    return this.getLines(width);
  }

  invalidate(): void {
    // Nothing cached at this level — every render re-derives from live state.
  }
}

export class ConversationViewComponent implements Component {
  private readonly tui: ConversationViewTui;
  private readonly options: ConversationViewOptions;
  private readonly primitives: ConversationViewPrimitives;
  private readonly unsubscribe: () => void;
  private readonly scrollView: ScrollViewLike;

  private mode: ConversationViewMode = "view";
  private items: ConversationItem[] = [];
  private streaming = "";
  private expanded = new Set<number>();
  private focusIndex: number | undefined;
  private editor: EditorLike | undefined;

  constructor(tui: ConversationViewTui, options: ConversationViewOptions) {
    this.tui = tui;
    this.options = options;
    this.primitives = { ...DEFAULT_PRIMITIVES, ...options.primitives };
    this.items = options.initialItems ? [...options.initialItems] : [];
    this.scrollView = new this.primitives.ScrollView(new ClosureComponent((width) => this.renderItems(width)), { follow: "end", overscroll: "contain" });
    this.unsubscribe = options.channel.onEvent((evt) => this.handleEvent(evt));
  }

  // ---- host-visible state -------------------------------------------------

  getMode(): ConversationViewMode {
    return this.mode;
  }

  getItems(): readonly ConversationItem[] {
    return this.items;
  }

  isItemExpanded(index: number): boolean {
    return this.expanded.has(index);
  }

  getFocusedIndex(): number | undefined {
    return this.focusIndex;
  }

  getScrollTop(): number {
    return this.scrollView.scrollTop;
  }

  isFollowingTail(): boolean {
    return this.scrollView.isFollowingEnd;
  }

  /** Appends one item to the transcript and requests a repaint. */
  appendItem(item: ConversationItem): void {
    this.items.push(item);
    this.options.onItemsChange?.([...this.items]);
    this.tui.requestRender();
  }

  /**
   * Promotes/keeps the view in `mode`. RAISE-ONLY: once `"interactive"`, a
   * later call with `"view"` is a no-op — see this file's header.
   */
  setMode(mode: ConversationViewMode): void {
    if (this.mode === "interactive") return;
    if (mode === this.mode) return;
    this.mode = mode;
    if (mode === "interactive" && !this.editor) {
      this.editor = new this.primitives.Editor(this.tui, IDENTITY_EDITOR_THEME);
      this.editor.onSubmit = (text) => this.handleSubmit(text);
    }
    this.tui.requestRender();
  }

  dispose(): void {
    this.unsubscribe();
  }

  invalidate(): void {
    this.scrollView.invalidate();
    this.editor?.invalidate();
  }

  // ---- channel events -------------------------------------------------

  private handleEvent(evt: unknown): void {
    const e = evt as {
      type?: string;
      assistantMessageEvent?: { type?: string; delta?: string };
      toolCallId?: string;
      toolName?: string;
      args?: unknown;
      result?: unknown;
      isError?: boolean;
    };
    if (e.type === "message_update" && e.assistantMessageEvent?.type === "text_delta" && typeof e.assistantMessageEvent.delta === "string") {
      this.streaming += e.assistantMessageEvent.delta;
      this.tui.requestRender();
      return;
    }
    if (e.type === "agent_start") {
      this.tui.requestRender();
      return;
    }
    if (e.type === "tool_execution_start" && typeof e.toolCallId === "string" && typeof e.toolName === "string") {
      this.appendItem({ kind: "tool-call", id: e.toolCallId, name: e.toolName, args: e.args });
      return;
    }
    if (e.type === "tool_execution_end" && typeof e.toolCallId === "string" && typeof e.toolName === "string") {
      this.appendItem({ kind: "tool-result", id: e.toolCallId, name: e.toolName, content: toolResultContentText(e.result), isError: e.isError });
      return;
    }
    if (e.type !== "agent_settled") return;
    const settled = this.streaming.trim();
    this.streaming = "";
    // The CHILD's own finalized turn — always "assistant", never "lead-message"
    // (that kind is reserved for a message the LEAD sends the child; see this
    // file's header and the old overlay component's identical `who: "thread"` direction).
    if (settled.length > 0) this.appendItem({ kind: "assistant", text: settled });
    else this.tui.requestRender();
  }

  // ---- input ------------------------------------------------------------

  handleInput(data: string): void {
    if (data === "\x03") return; // Ctrl+C swallowed in both modes.
    if (isEscapeKey(data)) {
      this.options.onEscape?.();
      return;
    }
    if (data === CTRL_O) {
      this.toggleExpandAll();
      return;
    }
    if (this.handleTranscriptNavigation(data)) return;
    if (this.mode === "interactive" && this.editor) {
      const editor = this.editor;
      // Ticket key-precedence table for "interactive" mode: Tab/Shift+Tab
      // move the selection only while the Editor is empty; Space toggles the
      // selection only while one is active; anything else first clears the
      // selection (a stale selection carried over from "view" mode must not
      // silently linger once the owner starts typing), then goes to the
      // Editor.
      const editorEmpty = editor.getText().length === 0;
      if ((data === "\t" || data === SHIFT_TAB) && editorEmpty) {
        this.focusNext(data === "\t" ? 1 : -1);
        return;
      }
      if (data === " " && this.focusIndex !== undefined) {
        this.toggleFocused();
        return;
      }
      if (this.focusIndex !== undefined) {
        this.focusIndex = undefined;
        this.tui.requestRender();
      }
      editor.handleInput(data);
      return;
    }
    // "view" mode navigation — no Editor is capturing input.
    if (data === "\t") {
      this.focusNext(1);
      return;
    }
    if (data === SHIFT_TAB) {
      this.focusNext(-1);
      return;
    }
    if (data === " ") {
      this.toggleFocused();
      return;
    }
    if (data === "\r" || data === "\n") {
      this.options.onEnter?.();
      return;
    }
    // Every other key is ignored in "view" mode — there is nothing to type into.
  }

  private handleSubmit(text: string): void {
    const trimmed = text.trim();
    this.editor?.setText("");
    if (trimmed.length === 0) {
      this.tui.requestRender();
      return;
    }
    if (trimmed === DONE_COMMAND) {
      this.options.onDone?.();
      return;
    }
    this.appendItem({ kind: "user", text: trimmed });
    void this.options.channel.send?.(trimmed);
  }

  private expandableIndices(): number[] {
    const result: number[] = [];
    for (let i = 0; i < this.items.length; i += 1) {
      if (this.isExpandable(i)) result.push(i);
    }
    return result;
  }

  private isExpandable(index: number): boolean {
    const item = this.items[index];
    if (item.kind === "tool-call") return isToolCallExpandable(item);
    if (item.kind === "tool-result") return isToolResultExpandable(item);
    return false;
  }

  private focusNext(direction: 1 | -1): void {
    // Newest expandable item first: traverse in descending (highest-index-first) order.
    const order = this.expandableIndices().reverse();
    if (order.length === 0) return;
    const currentPos = this.focusIndex === undefined ? -1 : order.indexOf(this.focusIndex);
    const basePos = currentPos === -1 ? (direction === 1 ? -1 : 0) : currentPos;
    const nextPos = (basePos + direction + order.length) % order.length;
    this.focusIndex = order[nextPos];
    this.tui.requestRender();
  }

  private toggleFocused(): void {
    if (this.focusIndex === undefined) return;
    this.toggle(this.focusIndex);
  }

  private toggle(index: number): void {
    if (this.expanded.has(index)) this.expanded.delete(index);
    else this.expanded.add(index);
    this.tui.requestRender();
  }

  private toggleExpandAll(): void {
    const indices = this.expandableIndices();
    const allExpanded = indices.length > 0 && indices.every((i) => this.expanded.has(i));
    this.expanded = new Set(allExpanded ? [] : indices);
    this.tui.requestRender();
  }

  // ---- rendering ----------------------------------------------------------

  render(width: number): string[] {
    const w = Math.max(1, width);
    const border = this.options.border === true;
    const innerW = border ? Math.max(1, w - BORDER_OVERHEAD) : w;
    const inner = this.renderInner(innerW).map((line) => (visibleWidth(line) > innerW ? truncateToWidth(line, innerW) : line));
    return border ? wrapInBorder(inner, w, innerW) : inner;
  }

  private renderInner(w: number): string[] {
    const liveness = this.options.channel.liveness();
    const idleBanner = liveness === "idle-awaiting-owner" ? this.textLines(AWAITING_OWNER_BANNER, w) : [];
    const header = [...this.textLines(this.hintText(), w), ...idleBanner];
    const body = this.renderItems(w);
    const editor = this.mode === "interactive" && this.editor ? this.editor.render(w) : [];
    // The scroll slice owns only the transcript. Header, idle footer, editor,
    // and border breathing rows remain fixed chrome outside the viewport.
    const fixedRows = header.length + idleBanner.length + (body.length > 0 ? 1 : 0) + (idleBanner.length > 0 ? 1 : 0) + (editor.length > 0 ? editor.length + 1 : 0);
    const frameRows = this.options.border === true ? 4 : 0;
    const cap = this.options.viewportHeight?.() ?? Number.POSITIVE_INFINITY;
    const viewport = Number.isFinite(cap) ? Math.max(1, Math.floor(cap) - frameRows - fixedRows) : body.length;
    this.scrollView.updateLayout(body.length, viewport, () => this.tui.requestRender());
    const transcript = body.slice(this.scrollView.scrollTop, this.scrollView.scrollTop + viewport);
    const lines = [...header];
    if (transcript.length > 0) lines.push("", ...transcript);
    if (idleBanner.length > 0) lines.push("", ...idleBanner);
    if (editor.length > 0) lines.push("", ...editor);
    return lines;
  }

  private handleTranscriptNavigation(data: string): boolean {
    const matches = (id: string) => this.options.keybindings?.matches(data, id) === true;
    const page = Math.max(1, this.scrollView.viewportHeight - 4);
    const wheel = /^\x1b\[<(64|65);\d+;\d+[Mm]$/.exec(data);
    if (wheel) {
      this.scrollView.scrollBy(wheel[1] === "64" ? -1 : 1);
      return true;
    }
    if (matches("tui.altScreen.pageUp")) { this.scrollView.scrollBy(-page); return true; }
    if (matches("tui.altScreen.pageDown")) { this.scrollView.scrollBy(page); return true; }
    if (this.mode === "interactive") return false;
    if (matches("tui.altScreen.halfPageUp")) { this.scrollView.scrollBy(-Math.max(1, Math.floor(page / 2))); return true; }
    if (matches("tui.altScreen.halfPageDown")) { this.scrollView.scrollBy(Math.max(1, Math.floor(page / 2))); return true; }
    if (matches("tui.altScreen.lineUp") || data === "\x1b[A") { this.scrollView.scrollBy(-1); return true; }
    if (matches("tui.altScreen.lineDown") || data === "\x1b[B") { this.scrollView.scrollBy(1); return true; }
    if (matches("tui.altScreen.top") || data === "\x1b[H") { this.scrollView.scrollToStart(); return true; }
    if (matches("tui.altScreen.bottom") || data === "\x1b[F") { this.scrollView.scrollToEnd(); return true; }
    return false;
  }

  handleMouse(event: TuiMouseEvent): TuiMouseEventResult | undefined {
    if (event.type !== "wheel" || !event.wheelDelta) return undefined;
    this.scrollView.scrollBy(event.wheelDelta < 0 ? -1 : 1);
    return { handled: true, render: true };
  }

  private hintText(): string {
    if (this.options.headerHint !== undefined) return this.options.headerHint;
    return this.mode === "interactive"
      ? `Tab/Shift+Tab: focus item · Space: expand/collapse · Ctrl+O: expand all · Esc: close · ${DONE_COMMAND}: end`
      : `Tab/Shift+Tab: focus item · Space: expand/collapse · Ctrl+O: expand all · Esc: close · Enter: interact`;
  }

  private renderItems(width: number): string[] {
    // Each turn is its own block; blocks are separated by one blank line for
    // vertical rhythm (question vs. answer, and between turns).
    const blocks: string[][] = [];
    for (let i = 0; i < this.items.length; i += 1) {
      blocks.push(this.renderItem(this.items[i], i, width));
    }
    const tail = this.streaming.trim();
    if (tail.length > 0) {
      // Partial "assistant" text: same gutter treatment as the settled kind,
      // no label — the streamed tail is itself the "child is working" signal.
      blocks.push(this.assistantLines(tail, width));
    } else if (this.options.channel.liveness() === "running") {
      // F2 (260905): the activity marker sits at the END of the dialogue — the
      // streaming slot where the tail would appear — so it is not missed, and
      // is shown while the child works and before any streamed text/tool
      // output of the turn arrives. Replaced by the first delta (the branch
      // above), so it never lingers alongside a non-empty tail.
      blocks.push(this.textLines(WORKING_MARKER, width, undefined, this.options.workingTextFg));
    }
    const lines: string[] = [];
    for (const block of blocks) {
      if (block.length === 0) continue;
      if (lines.length > 0) lines.push("");
      lines.push(...block);
    }
    return lines;
  }

  private renderItem(item: ConversationItem, index: number, width: number): string[] {
    const focusMarker = this.focusIndex === index ? "> " : "";
    switch (item.kind) {
      case "user":
        // Keep one blank row above and below the owner turn inside the same
        // full-width background as its content (`Text` paddingY=1).
        return new this.primitives.Text(`you: ${item.text}`, 0, 1, this.options.userLineBg).render(width);
      case "note":
        // Density polish: no leading `·` bullet — it read as noise and the
        // note's own phrasing already sets it apart from owner/model turns.
        return this.textLines(item.text, width);
      case "assistant":
        // The child's own finalized text — Markdown, no "lead:" label (see this
        // file's header). V3: carries the model gutter so it reads apart from
        // owner/lead/note/tool rows without a misleading label.
        return this.assistantLines(item.text, width);
      case "lead-message": {
        const lines = this.textLines("lead:", width);
        return [...lines, ...this.markdownLines(item.text, width)];
      }
      case "tool-call": {
        const lines = this.textLines(`${focusMarker}${toolCallHead(item)}`, width, undefined, this.options.toolTextFg);
        if (this.expanded.has(index)) {
          for (const bodyLine of toolCallBody(item)) lines.push(...this.textLines(`  ${bodyLine}`, width, undefined, this.options.toolTextFg));
        }
        return lines;
      }
      case "tool-result": {
        const lines = this.textLines(`${focusMarker}${toolResultHead(item)}`, width, undefined, this.options.toolTextFg);
        if (this.expanded.has(index)) {
          for (const bodyLine of toolResultBody(item)) lines.push(...this.textLines(`  ${bodyLine}`, width, undefined, this.options.toolTextFg));
        }
        return lines;
      }
    }
  }

  private textLines(text: string, width: number, bg?: (text: string) => string, fg?: (text: string) => string): string[] {
    return new this.primitives.Text(fg?.(text) ?? text, 0, 0, bg).render(width);
  }

  private markdownLines(text: string, width: number): string[] {
    return new this.primitives.Markdown(text, 0, 0, this.options.markdownTheme ?? IDENTITY_MARKDOWN_THEME).render(width);
  }

  /**
   * V3: a model turn (`"assistant"`, and its streaming tail) rendered as
   * Markdown behind the `ASSISTANT_GUTTER`. Markdown is rendered narrower by
   * exactly the gutter's width and then prefixed, so each line's visible width
   * still lands at `width`.
   */
  private assistantLines(text: string, width: number): string[] {
    const gutterWidth = visibleWidth(ASSISTANT_GUTTER);
    const body = this.markdownLines(text, Math.max(1, width - gutterWidth));
    return body.map((line) => `${ASSISTANT_GUTTER}${line}`);
  }
}

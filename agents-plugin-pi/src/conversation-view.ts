/**
 * `260908-feat-ws-pi-conversation-view-component` Phase 1: the shared
 * `ConversationViewComponent` — a `pi-tui` `Component` (`render(width)` /
 * `handleInput(data)`) that draws one child's transcript (owner turns, lead
 * messages, tool calls/results, and adapter notes) and, once promoted to
 * `"interactive"` mode, routes typed input back to it through a
 * `ConversationChannel`.
 *
 * Out of THIS phase (see the plan's Out of Scope section): `ask.ts` rebinding
 * onto this component, `overlay-chat.ts` deletion, `TranscriptEntry[]` ->
 * `ConversationItem[]` hydration, the audit window, the owner-steering
 * ownership rule, and the Esc modal — all Phase 2 / child B. This file only
 * defines the shared component and its structural contract, tested offline
 * against a fake `ConversationChannel` and a fake `tui`.
 *
 * Shape decisions:
 *   - `ConversationChannel` is `overlay-chat.ts`'s `ForkChannel` with
 *     `isStreaming(): boolean` widened to `liveness(): ChildLiveness` — a
 *     3-state read (`"running" | "idle-awaiting-owner" | "settled"`) taken
 *     FRESH on every `render()`, never cached across a call, so a state flip
 *     with no accompanying event still shows up on the very next repaint
 *     (the same class of gap `overlay-chat.ts`'s own `cachedStreaming` field
 *     works around). Producing `"idle-awaiting-owner"` from a real registry
 *     is child B's ownership rule (`260908` sibling ticket) — this phase only
 *     defines the type and renders its three states structurally against a
 *     fake channel.
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
 *     expandable only when `yamlContainerDisplay` recognizes its text as a
 *     JSON container. A non-expandable item never enters focus cycling.
 *   - `isEscapeKey` (kitty-protocol-safe Esc detection) is COPIED from
 *     `overlay-chat.ts` rather than imported: `overlay-chat.ts` stays running
 *     untouched until Phase 2 deletes it, and importing from a
 *     soon-to-be-deleted file would just move the coupling problem to Phase
 *     2 instead of avoiding it now.
 *   - Key-handling precedence (both modes): `\x03` (Ctrl+C) is swallowed
 *     first; `isEscapeKey` next, invoking `onEscape`; then `Ctrl+O` (`\x0f`)
 *     toggles collapse/expand for every expandable item at once — these three
 *     are non-printable chords that can never be legitimate typed content, so
 *     they are intercepted before mode-specific routing regardless of mode.
 *     In `"view"` mode (no `Editor` capturing input) the remaining keys are
 *     component navigation: Tab/Shift+Tab cycle focus among expandable items,
 *     Space toggles the focused item, Enter calls `onEnter` (there is no
 *     input box to submit). In `"interactive"` mode every other key is
 *     forwarded to the `Editor`, whose own `onSubmit` intercepts a trimmed
 *     `/done` (calls `onDone`) and otherwise appends an `"owner"` item and
 *     `channel.send`s it.
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
 *     belt-and-suspenders discipline `overlay-chat.ts`'s `renderThreadText`
 *     already uses for a host-supplied renderer it does not fully trust.
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
} from "./pi-tui.ts";
import { completedTextPreview, logicalPreview, yamlContainerDisplay, yamlInputPreview } from "./tool-result-render.ts";
import { visibleWidth } from "./text-width.ts";

/** The literal the owner types to end an interactive session (mirrors `overlay-chat.ts`'s `DONE_COMMAND`). */
export const DONE_COMMAND = "/done";

/**
 * Kitty keyboard-protocol CSI-u key report, and the modifyOtherKeys escape
 * report — copied from `overlay-chat.ts`'s well-tested `isEscapeKey` (see
 * this file's header for why it is copied rather than imported).
 */
const KITTY_CSI_U = /^\x1b\[(\d+)(?::(\d*))?(?::(\d+))?(?:;(\d+))?(?::(\d+))?u$/;
const MODIFY_OTHER_KEYS_ESCAPE = "\x1b[27;1;27~";

/** Whether one input chunk is an unmodified Escape press — see `overlay-chat.ts`'s `isEscapeKey` for the full rationale. */
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

/** `mode` values `ConversationViewComponent` can be in. See `setMode`'s raise-only contract. */
export type ConversationViewMode = "view" | "interactive";

/** One line-group in the transcript. */
export type ConversationItem =
  | { kind: "owner"; text: string }
  | { kind: "lead-message"; text: string }
  | { kind: "note"; text: string }
  | { kind: "tool-call"; name: string; args: unknown }
  | { kind: "tool-result"; name: string; text: string; isError?: boolean };

/**
 * A child's liveness, read fresh on every `render()` — never cached. Producing
 * `"idle-awaiting-owner"` from a real registry is child B's ownership rule;
 * this phase only defines the type and renders its three states.
 */
export type ChildLiveness = "running" | "idle-awaiting-owner" | "settled";

/**
 * The component's only route to its child. `overlay-chat.ts`'s `ForkChannel`
 * widened to a 3-state `liveness()` in place of `isStreaming()` — everything
 * else (`onEvent`, `send`) is unchanged, so the same wire events
 * (`message_update`/`text_delta`, `agent_start`, `agent_settled`) drive both.
 */
export interface ConversationChannel {
  /** Subscribe to the child's RPC event stream. Returns an unsubscribe function. */
  onEvent(listener: (evt: unknown) => void): () => void;
  /** Read fresh on every render — see the type doc above. */
  liveness(): ChildLiveness;
  /** Deliver one owner message to the child. */
  send(text: string): Promise<void>;
}

/** Minimal `pi-tui` `TUI` surface this component needs directly. The real `TUI` (needed by the real `Editor`) is a structural superset — see the `primitives` doc below. */
export interface ConversationViewTui {
  requestRender(): void;
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
  ScrollView: new (component: Component) => Component;
  Markdown: new (text: string, paddingX: number, paddingY: number, theme: MarkdownTheme) => Component;
  Text: new (text?: string, paddingX?: number, paddingY?: number) => Component;
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
  /** Enter in `"view"` mode (no `Editor` to consume it) — e.g. a host promoting the view to `"interactive"`. */
  onEnter?: () => void;
  /** Esc, in either mode. */
  onEscape?: () => void;
  /** `/done`, typed and submitted while `"interactive"`. */
  onDone?: () => void;
}

function isToolCallExpandable(item: Extract<ConversationItem, { kind: "tool-call" }>): boolean {
  return yamlInputPreview(item.args).length > 0;
}

function isToolResultExpandable(item: Extract<ConversationItem, { kind: "tool-result" }>): boolean {
  return yamlContainerDisplay(item.text) !== undefined;
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
  const preview = completedTextPreview(item.text);
  const first = logicalPreview(preview.text, 1);
  const marker = item.isError ? "✗" : "✓";
  return `${marker} ${item.name} ${first}`.trimEnd();
}

function toolResultBody(item: Extract<ConversationItem, { kind: "tool-result" }>): string[] {
  const yaml = yamlContainerDisplay(item.text);
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
  private readonly scrollView: Component;

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
    this.scrollView = new this.primitives.ScrollView(new ClosureComponent((width) => this.renderItems(width)));
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

  /** Appends one item to the transcript and requests a repaint. */
  appendItem(item: ConversationItem): void {
    this.items.push(item);
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
    const e = evt as { type?: string; assistantMessageEvent?: { type?: string; delta?: string } };
    if (e.type === "message_update" && e.assistantMessageEvent?.type === "text_delta" && typeof e.assistantMessageEvent.delta === "string") {
      this.streaming += e.assistantMessageEvent.delta;
      this.tui.requestRender();
      return;
    }
    if (e.type === "agent_start") {
      this.tui.requestRender();
      return;
    }
    if (e.type !== "agent_settled") return;
    const settled = this.streaming.trim();
    this.streaming = "";
    if (settled.length > 0) this.appendItem({ kind: "lead-message", text: settled });
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
    if (this.mode === "interactive" && this.editor) {
      this.editor.handleInput(data);
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
    this.appendItem({ kind: "owner", text: trimmed });
    void this.options.channel.send(trimmed);
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
    const indices = this.expandableIndices();
    if (indices.length === 0) return;
    const currentPos = this.focusIndex === undefined ? -1 : indices.indexOf(this.focusIndex);
    const basePos = currentPos === -1 ? (direction === 1 ? -1 : 0) : currentPos;
    const nextPos = (basePos + direction + indices.length) % indices.length;
    this.focusIndex = indices[nextPos];
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
    const lines: string[] = [];
    lines.push(...this.textLines(this.hintText(), w));
    const banner = this.livenessBanner();
    if (banner) lines.push(...this.textLines(banner, w));
    lines.push(...this.scrollView.render(w));
    if (this.mode === "interactive" && this.editor) lines.push(...this.editor.render(w));
    return lines.map((line) => (visibleWidth(line) > w ? truncateToWidth(line, w) : line));
  }

  private hintText(): string {
    return this.mode === "interactive"
      ? `Tab/Shift+Tab: focus item · Space: expand/collapse · Ctrl+O: expand all · Esc: close · ${DONE_COMMAND}: end`
      : `Tab/Shift+Tab: focus item · Space: expand/collapse · Ctrl+O: expand all · Esc: close · Enter: interact`;
  }

  private livenessBanner(): string | undefined {
    const liveness = this.options.channel.liveness();
    if (liveness === "running") return "working…";
    if (liveness === "idle-awaiting-owner") return "── AWAITING OWNER ──";
    return undefined;
  }

  private renderItems(width: number): string[] {
    const lines: string[] = [];
    for (let i = 0; i < this.items.length; i += 1) {
      lines.push(...this.renderItem(this.items[i], i, width));
    }
    if (this.streaming.trim().length > 0) {
      lines.push(...this.textLines("lead:", width));
      lines.push(...this.markdownLines(this.streaming.trim(), width));
    }
    return lines;
  }

  private renderItem(item: ConversationItem, index: number, width: number): string[] {
    const focusMarker = this.focusIndex === index ? "> " : "";
    switch (item.kind) {
      case "owner":
        return this.textLines(`you: ${item.text}`, width);
      case "note":
        return this.textLines(`· ${item.text}`, width);
      case "lead-message": {
        const lines = this.textLines("lead:", width);
        return [...lines, ...this.markdownLines(item.text, width)];
      }
      case "tool-call": {
        const lines = this.textLines(`${focusMarker}${toolCallHead(item)}`, width);
        if (this.expanded.has(index)) {
          for (const bodyLine of toolCallBody(item)) lines.push(...this.textLines(`  ${bodyLine}`, width));
        }
        return lines;
      }
      case "tool-result": {
        const lines = this.textLines(`${focusMarker}${toolResultHead(item)}`, width);
        if (this.expanded.has(index)) {
          for (const bodyLine of toolResultBody(item)) lines.push(...this.textLines(`  ${bodyLine}`, width));
        }
        return lines;
      }
    }
  }

  private textLines(text: string, width: number): string[] {
    return new this.primitives.Text(text, 0, 0).render(width);
  }

  private markdownLines(text: string, width: number): string[] {
    return new this.primitives.Markdown(text, 0, 0, IDENTITY_MARKDOWN_THEME).render(width);
  }
}

/**
 * Lead TUI display-summary rendering: the summary components a collapsed row
 * shows once `display-summary.ts`'s summarizer has filled the store, and the
 * seams that put them in front of the raw renderers.
 *
 * Display contract: a summarized row is a one-line header (the bold tool
 * name, or the message kind, then the summary's `subtitle` in the accent color
 * Pi uses for tool-call paths), a blank line, and the fields.
 * `optionalContext` stays dim and indented; `toolIntention` is normal text
 * indented four spaces, then a blank line separates the unindented muted
 * `toolResult`, which opens with a dim inline `[N.N KB]` token: the context
 * size the row occupies (`formatContextSize`), measured here from the row's
 * own args/result/content at render time, never by the summarizer.
 * Collapsed rows show the raw rendering until a summary exists; expanded rows
 * (Pi's Ctrl+O) are always raw.
 *
 * Re-render paths differ by row kind:
 * - Tool rows re-run their renderers on `ToolRenderContext.invalidate()`, so
 *   the wrapped renderers record that handle in the store and decide
 *   summary-vs-raw on each call.
 * - Message and entry renderers run only on a rebuild (expand toggle, theme
 *   change); `requestRender()` repaints without one. Their component is a
 *   switch that reads the store at render time.
 *
 * pi-tui classes come in as injected modules (the host's copy at runtime, a
 * duck-typed stand-in under test); see `pi-tui.ts` for why a static runtime
 * import would be the wrong copy. This module must not import
 * `tool-result-render.ts` at runtime: that module imports this one.
 *
 * Summary text is model output that may echo terminal sequences from a tool
 * result (OSC 52 clipboard writes, cursor moves), and the sidecar replays it on
 * every reload; `summaryText` strips it at this render boundary as raw rows do.
 * The static pure-string `stripTerminalSequences`, like `truncateToWidth`
 * above, carries no class identity, so the dual-package hazard does not apply.
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { getMarkdownTheme } from "@earendil-works/pi-coding-agent";
import type { DisplaySummary, DisplaySummaryStore } from "./display-summary.ts";
import { UseNativeResultFallback } from "./native-fallback.ts";
import { loadHostPiTui, stripTerminalSequences, truncateToWidth } from "./pi-tui.ts";
import { summaryIdOf } from "./summary-id.ts";
import { sanitizePreviewText } from "./text-width.ts";

// ---------------------------------------------------------------------------
// Host surface
// ---------------------------------------------------------------------------

export interface SummaryComponent {
  render(width: number): string[];
  invalidate(): void;
}

/** The pi-tui slice a tool-row summary needs. */
export interface SummaryTextModules {
  Text: new (text?: string, paddingX?: number, paddingY?: number) => SummaryComponent;
  /** Self-framed native tools own their backgrounds; the host supplies Box. */
  Box?: new (paddingX?: number, paddingY?: number, bgFn?: (text: string) => string) => SummaryComponent & { addChild(child: SummaryComponent): void };
}

/** The pi-tui slice a message summary card needs. */
export interface SummaryCardModules extends SummaryTextModules {
  Box: new (paddingX?: number, paddingY?: number, bgFn?: (text: string) => string) => SummaryComponent & { addChild(child: SummaryComponent): void };
}

/** The pi-tui slice the adapter message renderers need (Pi's default custom-message look). */
export interface AdapterMessageTuiModules extends SummaryCardModules {
  Spacer: new (lines?: number) => SummaryComponent;
  Markdown: new (
    text: string,
    paddingX: number,
    paddingY: number,
    theme: unknown,
    defaultTextStyle?: { color?: (text: string) => string },
  ) => SummaryComponent;
}

/** Duck-typed slice of Pi's `Theme`. */
export interface SummaryTheme {
  fg?(color: string, text: string): string;
  bg?(color: string, text: string): string;
  bold?(text: string): string;
}

function themeOf(theme: unknown): SummaryTheme | undefined {
  return typeof theme === "object" && theme !== null ? theme as SummaryTheme : undefined;
}

export function paintFg(theme: unknown, color: string, text: string): string {
  try {
    return themeOf(theme)?.fg?.(color, text) ?? text;
  } catch {
    return text;
  }
}

function paintBg(theme: unknown, color: string, text: string): string {
  try {
    return themeOf(theme)?.bg?.(color, text) ?? text;
  } catch {
    return text;
  }
}

export function paintBold(theme: unknown, text: string): string {
  try {
    return themeOf(theme)?.bold?.(text) ?? text;
  } catch {
    return text;
  }
}

// ---------------------------------------------------------------------------
// Summary components
// ---------------------------------------------------------------------------

/** Preserved indent of optional context under a summary header. */
export const SUMMARY_FIELD_INDENT = 2;

const summaryComponentBrand = Symbol("ws-display-summary-component");
const rawComponents = new WeakMap<object, unknown>();
const rawRenderers = new WeakMap<Function, Function | undefined>();

function validSummary(value: DisplaySummary | undefined): value is DisplaySummary {
  return !!value && typeof value.toolIntention === "string" && !!value.toolIntention.trim() &&
    typeof value.toolResult === "string" && !!value.toolResult.trim() &&
    (value.optionalContext === undefined || typeof value.optionalContext === "string");
}

function component(value: unknown): value is SummaryComponent {
  return !!value && typeof (value as SummaryComponent).render === "function" && typeof (value as SummaryComponent).invalidate === "function";
}

/** Last-resort raw text when a delayed summary fault cannot reach a raw slot. */
function rawText(value: unknown): SummaryComponent {
  let text = "";
  try { text = typeof value === "string" ? value : JSON.stringify(value) ?? ""; } catch { /* malformed raw metadata */ }
  return { render: (width) => text.split("\n").map((line) => truncateToWidth(line, Math.max(0, width))), invalidate() {} };
}

/** Host catches renderer construction, not a returned component's later faults. */
function guardedSummary(build: () => SummaryComponent, raw: () => unknown, fallback: SummaryComponent, previousRaw?: unknown, onFailure: () => void = () => {}): SummaryComponent {
  let summary: SummaryComponent | undefined;
  let failed = false;
  const fail = () => { failed = true; try { onFailure(); } catch { /* Stale row metadata. */ } };
  const guard = {
    [summaryComponentBrand]: true,
    render(width: number): string[] {
      if (!failed) {
        try {
          summary ??= build();
          if (!component(summary)) throw new Error("Invalid summary component");
          const lines = summary.render(width);
          if (!Array.isArray(lines) || lines.some((line) => typeof line !== "string")) throw new Error("Invalid summary lines");
          return lines;
        } catch { fail(); }
      }
      try {
        let original = rawComponents.get(guard);
        if (!component(original)) { original = raw(); rawComponents.set(guard, original); }
        if (component(original)) return original.render(width);
      } catch { /* Raw fallback may itself be unavailable; never return an invalid component. */ }
      return fallback.render(width);
    },
    invalidate() {
      try { summary?.invalidate(); } catch { fail(); }
      try { const original = rawComponents.get(guard); if (component(original)) original.invalidate(); } catch { /* torn-down raw component */ }
    },
  };
  if (component(previousRaw)) rawComponents.set(guard, previousRaw);
  return guard;
}

/** True for a component this module built in place of a raw rendering. */
export function isSummaryComponent(value: unknown): boolean {
  return typeof value === "object" && value !== null && (value as Record<symbol, unknown>)[summaryComponentBrand] === true;
}

function stack(children: readonly SummaryComponent[]): SummaryComponent {
  return {
    [summaryComponentBrand]: true,
    render(width: number): string[] {
      return children.flatMap((child) => child.render(width));
    },
    invalidate(): void {
      for (const child of children) child.invalidate();
    },
  } as SummaryComponent;
}

/** A real blank row: Text with an empty string renders no lines. */
function summarySpacer(): SummaryComponent {
  return { render: () => [""], invalidate: () => {} };
}

// ---------------------------------------------------------------------------
// Context size
// ---------------------------------------------------------------------------

/**
 * UTF-8 bytes of message or tool-result content as the agent received it:
 * text blocks by their text; image blocks by their base64 `data` string (the
 * encoded payload the provider is sent, not the decoded image); any other
 * block by its JSON serialization. A string content is measured directly.
 */
export function contentBytes(content: unknown): number {
  if (typeof content === "string") return Buffer.byteLength(content, "utf8");
  if (!Array.isArray(content)) return 0;
  let bytes = 0;
  for (const block of content) {
    const part = block as { type?: unknown; text?: unknown; data?: unknown } | undefined;
    if (part?.type === "text" && typeof part.text === "string") bytes += Buffer.byteLength(part.text, "utf8");
    else if (part?.type === "image" && typeof part.data === "string") bytes += Buffer.byteLength(part.data, "utf8");
    else {
      try { bytes += Buffer.byteLength(JSON.stringify(block) ?? "", "utf8"); } catch { /* unserializable block counts as nothing */ }
    }
  }
  return bytes;
}

/** A tool row's context: the call (tool name plus JSON arguments) and the result content. */
export function toolRowBytes(toolName: string, args: unknown, result: unknown): number {
  let argsJson = "";
  try { argsJson = JSON.stringify(args) ?? ""; } catch { /* unserializable args count as nothing */ }
  const content = typeof result === "object" && result !== null ? (result as { content?: unknown }).content : result;
  return Buffer.byteLength(toolName, "utf8") + Buffer.byteLength(argsJson, "utf8") + contentBytes(content);
}

/** `[N.N KB]`: bytes / 1024 at one decimal, floored at `[0.1 KB]` so a row never reads as empty. */
export function formatContextSize(bytes: number): string {
  const kb = Math.max(0.1, (Number.isFinite(bytes) ? bytes : 0) / 1024);
  return `[${kb.toFixed(1)} KB]`;
}

/** A summary field made terminal-safe: escape sequences stripped, remaining controls neutralized. */
function summaryText(text: string): string {
  return sanitizePreviewText(stripTerminalSequences(text));
}

/**
 * Shared field layout relative to the header; padX is the row's frame margin.
 * `bytes`, when given, prefixes the result text with the dim context-size token.
 */
export function buildSummaryFields(tui: SummaryTextModules, summary: DisplaySummary, theme: unknown, padX = 0, bytes?: number): SummaryComponent[] {
  const fields: SummaryComponent[] = [summarySpacer()];
  const context = summary.optionalContext ? summaryText(summary.optionalContext) : "";
  if (context) fields.push(new tui.Text(paintFg(theme, "dim", context), padX + SUMMARY_FIELD_INDENT, 0));
  fields.push(new tui.Text(paintFg(theme, "text", summaryText(summary.toolIntention)), padX + 4, 0));
  fields.push(summarySpacer());
  const result = paintFg(theme, "muted", summaryText(summary.toolResult));
  const size = bytes === undefined ? undefined : paintFg(theme, "dim", formatContextSize(bytes));
  // The size token's position within the result text is this one join.
  fields.push(new tui.Text(size ? `${size} ${result}` : result, padX, 0));
  return fields;
}

/**
 * `subtitle` with the raw tool name (or message kind) `name` removed: the
 * header already shows it, yet the model sometimes repeats it ("write —
 * /tmp/x.txt") or, for a call without arguments, answers with it alone.
 * An exact match (case-insensitive) yields nothing; a leading copy is
 * stripped when a separator follows it: whitespace, `:`, or a dash with
 * whitespace on both sides (`write - x`, `write — x`). Compares against the
 * raw name, never the painted header.
 */
export function dedupeSubtitle(subtitle: string, name: string | undefined): string {
  const trimmed = subtitle.trim();
  const bare = name?.trim().toLowerCase();
  if (!bare) return trimmed;
  if (trimmed.toLowerCase() === bare) return "";
  if (!trimmed.toLowerCase().startsWith(bare)) return trimmed;
  const rest = trimmed.slice(bare.length);
  // A longer identifier that merely starts with the name ("write_file",
  // "write-scopes.ts") is not a repeat, so an unspaced hyphen is no separator.
  // A flag-like dash after the space ("find -name x") reads as a command line,
  // not as the name repeated before a separator, so the subtitle stays whole.
  if (!/^(?:\s|:)/.test(rest) || /^[\s:]*[—–-]\S/.test(rest)) return trimmed;
  // Leading whitespace/colons, then any spaced dashes, each followed by more separators.
  return rest.replace(/^[\s:]*(?:[—–-](?!\S)[\s:]*)*/, "").trim();
}

/**
 * `header` (already painted) followed by the summary's subtitle on the same
 * line. `name` is the raw tool name or message kind the header shows; the
 * subtitle is deduped against it and cut to its first line so the header stays
 * one line. A summary without a subtitle (none given, or nothing left after
 * the dedupe) keeps the bare header.
 */
export function summaryHeaderLine(header: string, summary: DisplaySummary, theme: unknown, name?: string): string {
  const firstLine = summaryText(summary.subtitle ?? "").trim().split("\n", 1)[0] ?? "";
  const subtitle = dedupeSubtitle(firstLine, name);
  return subtitle ? `${header} ${paintFg(theme, "accent", subtitle)}` : header;
}

/** A painted header line (plus the subtitle) over the fields, with no frame of its own. */
export function buildSummaryBlock(tui: SummaryTextModules, header: string, summary: DisplaySummary, theme: unknown, name?: string): SummaryComponent {
  return stack([new tui.Text(summaryHeaderLine(header, summary, theme, name), 0, 0), ...buildSummaryFields(tui, summary, theme)]);
}

/** A message summary on the custom-message card: the shared `customMessageBg` box, header, fields (with the size token when `bytes` is given). */
export function buildSummaryCard(tui: SummaryCardModules, header: string, summary: DisplaySummary, theme: unknown, name?: string, bytes?: number): SummaryComponent {
  const box = new tui.Box(1, 1, (text) => paintBg(theme, "customMessageBg", text));
  box.addChild(new tui.Text(summaryHeaderLine(header, summary, theme, name), 0, 0));
  for (const field of buildSummaryFields(tui, summary, theme, 0, bytes)) box.addChild(field);
  return box;
}

/**
 * A message/entry row that shows `buildSummary(summary)` while the store has
 * a summary for `id`, and `raw` otherwise, decided at every render: message
 * renderers are not re-run when a summary arrives, only repainted.
 */
export function createSummarySwitch(
  store: DisplaySummaryStore,
  id: string,
  raw: SummaryComponent,
  buildSummary: (summary: DisplaySummary) => SummaryComponent,
): SummaryComponent {
  const generation = store.generation;
  let cached: { summary: DisplaySummary; component: SummaryComponent } | undefined;
  return {
    render(width: number): string[] {
      try {
        const summary = store.enabled && store.generation === generation ? store.get(id) : undefined;
        if (validSummary(summary)) {
          if (cached?.summary !== summary) cached = { summary, component: guardedSummary(() => buildSummary(summary), () => raw, raw) };
          return cached.component.render(width);
        }
      } catch { /* Summary lookup/readiness is cosmetic. */ }
      return raw.render(width);
    },
    invalidate(): void {
      try { raw.invalidate(); } catch { /* A stale message card. */ }
      cached?.component.invalidate();
    },
  };
}

/**
 * `raw` itself when there is nothing to switch on (no store, no row id, or an
 * expanded row, which is always raw); otherwise the render-time switch.
 */
export function summarizedRow(
  raw: SummaryComponent,
  store: DisplaySummaryStore | undefined,
  id: string | undefined,
  expanded: boolean | undefined,
  buildSummary: (summary: DisplaySummary) => SummaryComponent,
): SummaryComponent {
  if (!store || !id || expanded) return raw;
  return createSummarySwitch(store, id, raw, buildSummary);
}

const KIND_LABELS: Record<string, string> = {
  "ws-agent-orphaned": "Orphaned agents",
  "ws-goal-control": "Goal control",
  "ws-lead-compact": "Compaction prep",
  "ws-lead-compaction-history": "Previous conversation",
  "ws-lead-context-milestone": "Context milestone",
  "ws-mailbox": "Mailbox",
  "ws-thread-summary": "Thread summary",
};

/** A short readable kind for a summary header of a custom type with no human head of its own. */
export function summaryKindLabel(customType: string): string {
  return KIND_LABELS[customType] ?? customType.replace(/^ws-/, "").replace(/-/g, " ");
}

// ---------------------------------------------------------------------------
// Tool rows
// ---------------------------------------------------------------------------

/** Pi's renderer slots, typed loosely: wrapped definitions come from several typed sources. */
export type ToolCallRenderer = (args: unknown, theme: unknown, context: unknown) => unknown;
export type ToolResultRenderer = (result: unknown, options: unknown, theme: unknown, context: unknown) => unknown;

interface SummaryRenderContext {
  toolCallId?: unknown;
  /** The call's arguments, shared across the call and result slots. */
  args?: unknown;
  invalidate?: unknown;
  lastComponent?: unknown;
  expanded?: unknown;
  isPartial?: unknown;
  isError?: unknown;
  state?: unknown;
}

export type DisplaySummaryStoreSource = DisplaySummaryStore | (() => DisplaySummaryStore | undefined) | undefined;
export type SummaryTuiSource = SummaryTextModules | { current: SummaryTextModules | undefined } | undefined;

function resolveStore(source: DisplaySummaryStoreSource): DisplaySummaryStore | undefined {
  return typeof source === "function" ? source() : source;
}

function resolveTui(source: SummaryTuiSource): SummaryTextModules | undefined {
  return source && "current" in source ? source.current : source;
}

function asContext(context: unknown): SummaryRenderContext | undefined {
  return typeof context === "object" && context !== null ? context as SummaryRenderContext : undefined;
}

/**
 * Wraps a tool's `renderCall`/`renderResult` so a collapsed row with a
 * summary shows it (the call slot draws the header, the result slot the
 * fields), and everything else reaches the inner renderer unchanged. Every
 * call records the row's `invalidate` handle so the summarizer can re-render
 * the row when its summary lands. An undefined inner slot throws
 * `UseNativeResultFallback`, Pi's per-slot fallback, as an absent slot would.
 *
 * Inner renderers may reuse `context.lastComponent` blindly (Pi's built-ins
 * call `setText` on it), so a summary component is never handed back to them.
 * `padX` indents the summary for tools that draw their own frame
 * (`renderShell: "self"`); `selfFramed` retains their themed status background
 * because Pi does not supply an outer Box for those tools.
 */
export function wrapToolRenderersWithSummary(
  toolName: string,
  renderCall: ToolCallRenderer | undefined,
  renderResult: ToolResultRenderer | undefined,
  store: DisplaySummaryStoreSource,
  tui: SummaryTuiSource,
  options: { padX?: number; selfFramed?: boolean; eligible?: () => boolean } = {},
): { renderCall: ToolCallRenderer; renderResult: ToolResultRenderer } {
  const padX = options.padX ?? 0;
  const generations = new WeakMap<object, number>();
  const failedSummaries = new WeakMap<object, DisplaySummary>();
  const identityOf = (context: SummaryRenderContext): object => context.state && typeof context.state === "object" ? context.state : context;

  function summaryFor(context: SummaryRenderContext | undefined, expanded: unknown): { summary: DisplaySummary; tui: SummaryTextModules } | undefined {
    const current = resolveStore(store);
    if (!current || !context || typeof context.toolCallId !== "string" || !context.toolCallId.trim()) return undefined;
    const identity = identityOf(context);
    if (!generations.has(identity)) generations.set(identity, current.generation);
    if (generations.get(identity) !== current.generation) return undefined;
    if (typeof context.invalidate === "function") current.trackInvalidate(context.toolCallId, context.invalidate as () => void);
    if (!current.enabled || expanded || (options.eligible && !options.eligible())) return undefined;
    const summary = current.get(context.toolCallId);
    if (summary && failedSummaries.get(identity) === summary) return undefined;
    const modules = summary ? resolveTui(tui) : undefined;
    return validSummary(summary) &&
      typeof modules?.Text === "function" && (!options.selfFramed || typeof modules.Box === "function") ? { summary, tui: modules } : undefined;
  }

  function summaryFrame(children: SummaryComponent[], modules: SummaryTextModules, theme: unknown, context: SummaryRenderContext | undefined): SummaryComponent {
    const component = stack(children);
    // Pi's default shell already paints its Box. A self shell is a bare
    // Container, so replacing the native renderer must retain its background.
    if (!options.selfFramed || !modules.Box) return component;
    const color = context?.isPartial ? "toolPendingBg" : context?.isError ? "toolErrorBg" : "toolSuccessBg";
    const box = new modules.Box(0, 0, (line) => paintBg(theme, color, line));
    box.addChild(component);
    return stack([box]);
  }

  function rawContext(context: unknown): unknown {
    const ctx = asContext(context);
    return ctx && isSummaryComponent(ctx.lastComponent) ? { ...ctx, lastComponent: rawComponents.get(ctx.lastComponent as object) } : context;
  }

  const wrapped = {
    renderCall(args: unknown, theme: unknown, context: unknown) {
      const ctx = asContext(context);
      const raw = () => { if (!renderCall) throw new UseNativeResultFallback(); return renderCall(args, theme, rawContext(context)); };
      try {
        const hit = summaryFor(ctx, ctx?.expanded);
        if (hit) return guardedSummary(() => summaryFrame([new hit.tui.Text(summaryHeaderLine(paintFg(theme, "toolTitle", paintBold(theme, toolName)), hit.summary, theme, toolName), padX, 0)], hit.tui, theme, ctx), raw, rawText(`${toolName}\n${JSON.stringify(args)}`), (rawContext(context) as SummaryRenderContext)?.lastComponent, () => { if (ctx) failedSummaries.set(identityOf(ctx), hit.summary); });
      } catch { /* Our readiness/construction path must fall through to raw. */ }
      return raw();
    },
    renderResult(result: unknown, renderOptions: unknown, theme: unknown, context: unknown) {
      const ctx = asContext(context);
      const raw = () => { if (!renderResult) throw new UseNativeResultFallback(); return renderResult(result, renderOptions, theme, rawContext(context)); };
      try {
        const hit = summaryFor(ctx, (renderOptions as { expanded?: unknown } | undefined)?.expanded);
        if (hit) return guardedSummary(() => summaryFrame(buildSummaryFields(hit.tui, hit.summary, theme, padX, toolRowBytes(toolName, ctx?.args, result)), hit.tui, theme, ctx), raw, rawText((result as { content?: unknown })?.content ?? result), (rawContext(context) as SummaryRenderContext)?.lastComponent, () => { if (ctx) failedSummaries.set(identityOf(ctx), hit.summary); });
      } catch { /* Our readiness/construction path must fall through to raw. */ }
      return raw();
    },
  };
  rawRenderers.set(wrapped.renderCall, renderCall);
  rawRenderers.set(wrapped.renderResult, renderResult);
  return wrapped;
}

/** Native eligibility is confirmed from the actual active loadout at session_start. */
export const SUMMARIZED_BUILTIN_TOOL_NAMES = ["edit", "write", "grep", "find", "ls", "powershell"] as const;

export function confirmSummarizedBuiltinTools(store: DisplaySummaryStore, activeToolNames: Iterable<string>): void {
  const active = new Set(activeToolNames);
  for (const name of SUMMARIZED_BUILTIN_TOOL_NAMES) {
    store.toolNames.delete(name);
    if (active.has(name)) store.confirmTool(name);
  }
}

/** Compatibility slice: the installed 1.0.4 API is newer than our dev declarations. */
export interface SummaryToolRenderers {
  renderShell?: "default" | "self";
  renderCall?: ToolCallRenderer;
  renderResult?: ToolResultRenderer;
}
export type SummaryToolResolver = (name: string, next: () => SummaryToolRenderers | undefined) => SummaryToolRenderers | undefined;

/** Presentation only, registered at factory time, before Pi constructs retained rows. */
export function registerDisplaySummaryToolResolver(
  pi: { registerToolRenderer?: (resolver: SummaryToolResolver) => void },
  store: DisplaySummaryStore,
  tui: SummaryTuiSource,
): boolean {
  if (typeof pi.registerToolRenderer !== "function") return false;
  pi.registerToolRenderer((name, next) => {
    let initial: SummaryToolRenderers | undefined;
    try { initial = next(); } catch { return undefined; }
    try {
    // A registration-time wrapper already has exactly this presentation layer.
    if (initial?.renderCall && rawRenderers.has(initial.renderCall) && initial.renderResult && rawRenderers.has(initial.renderResult)) return initial;
    const shell = initial?.renderShell;
    const resolveRaw = () => next(); // Late tools resolve through the same public chain, not a name registry.
    const call: ToolCallRenderer = (args, theme, context) => {
      const renderer = resolveRaw()?.renderCall;
      const raw = renderer && rawRenderers.has(renderer) ? rawRenderers.get(renderer) as ToolCallRenderer | undefined : renderer;
      if (!raw) throw new UseNativeResultFallback();
      return raw(args, theme, context);
    };
    const result: ToolResultRenderer = (value, options, theme, context) => {
      const renderer = resolveRaw()?.renderResult;
      const raw = renderer && rawRenderers.has(renderer) ? rawRenderers.get(renderer) as ToolResultRenderer | undefined : renderer;
      if (!raw) throw new UseNativeResultFallback();
      return raw(value, options, theme, context);
    };
    // Track even unknown incoming rows; eligibility gates values, not repaint linkage.
    const wrapped = wrapToolRenderersWithSummary(name, call, result, store, tui, {
      selfFramed: shell === "self", padX: shell === "self" ? 1 : 0,
      eligible: () => store.toolNames.has(name),
    });
    return { renderShell: shell, ...wrapped };
    } catch { return undefined; } // Malformed presentation metadata cannot escape our resolver.
  });
  return true;
}

// ---------------------------------------------------------------------------
// Adapter custom messages without a renderer of their own
// ---------------------------------------------------------------------------

/** Adapter custom messages that had Pi's default look and gain a summary-aware renderer. */
export const ADAPTER_SUMMARIZED_MESSAGE_TYPES = ["ws-lead-compact", "ws-lead-context-milestone", "ws-thread-summary"] as const;

function contentText(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content
    .filter((part) => (part as { type?: unknown })?.type === "text")
    .map((part) => (part as { text?: string }).text)
    .join("\n");
}

/**
 * Pi's default custom-message rendering (`CustomMessageComponent`'s
 * fallback): the `customMessageBg` box, the bold `[customType]` label, a
 * spacer and the content as Markdown in `customMessageText`. Pi's own default
 * uses the session's settings-aware Markdown theme, which extensions cannot
 * reach; `getMarkdownTheme()` is its settings-free base.
 */
export function buildDefaultCustomMessageComponent(
  tui: AdapterMessageTuiModules,
  message: { customType?: unknown; content?: unknown },
  theme: unknown,
  markdownTheme: unknown = getMarkdownTheme(),
): SummaryComponent {
  const customType = typeof message.customType === "string" ? message.customType : "";
  const box = new tui.Box(1, 1, (text) => paintBg(theme, "customMessageBg", text));
  box.addChild(new tui.Text(paintFg(theme, "customMessageLabel", `\x1b[1m[${customType}]\x1b[22m`), 0, 0));
  box.addChild(new tui.Spacer(1));
  box.addChild(new tui.Markdown(contentText(message.content), 0, 0, markdownTheme, {
    color: (text) => paintFg(theme, "customMessageText", text),
  }));
  return box;
}

/** One adapter message row: the default look, switching to its summary card when collapsed and summarized. */
export function buildAdapterMessageComponent(
  tui: AdapterMessageTuiModules,
  message: { customType?: unknown; content?: unknown; details?: unknown },
  theme: unknown,
  expanded: boolean | undefined,
  summaries: DisplaySummaryStore | undefined,
): SummaryComponent {
  const raw = buildDefaultCustomMessageComponent(tui, message, theme);
  const customType = typeof message.customType === "string" ? message.customType : "";
  const kind = summaryKindLabel(customType);
  const header = paintFg(theme, "customMessageLabel", paintBold(theme, kind));
  return summarizedRow(raw, summaries, summaryIdOf(message.details), expanded, (summary) => buildSummaryCard(tui, header, summary, theme, kind, contentBytes(message.content)));
}

/**
 * Registers the summary-aware renderer for `ADAPTER_SUMMARIZED_MESSAGE_TYPES`.
 * Lead TUI only: elsewhere these messages keep Pi's own default renderer.
 */
export async function registerAdapterMessageRenderers(
  pi: Pick<ExtensionAPI, "registerMessageRenderer">,
  summaries: DisplaySummaryStore | undefined,
  tuiModules?: AdapterMessageTuiModules,
): Promise<boolean> {
  let tui: AdapterMessageTuiModules;
  try {
    tui = tuiModules ?? ((await loadHostPiTui({ fallback: false })) as unknown as AdapterMessageTuiModules);
    if (![tui?.Text, tui?.Box, tui?.Spacer, tui?.Markdown].every((value) => typeof value === "function")) return false;
  } catch { return false; }
  for (const customType of ADAPTER_SUMMARIZED_MESSAGE_TYPES) {
    pi.registerMessageRenderer(customType, (message, options, theme) => {
      try {
        return buildAdapterMessageComponent(tui, message as { customType?: unknown; content?: unknown; details?: unknown }, theme, (options as { expanded?: boolean } | undefined)?.expanded, summaries) as never;
      } catch { return undefined; } // Pi's default raw message, never an invalid component.
    });
  }
  return true;
}

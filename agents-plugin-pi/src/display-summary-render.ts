/**
 * Lead TUI display-summary rendering: the summary components a collapsed row
 * shows once `display-summary.ts`'s summarizer has filled the store, and the
 * seams that put them in front of the raw renderers.
 *
 * Display contract: a summarized row is a bold header (the tool name, or the
 * message kind), a blank line, and the fields. `optionalContext` stays dim
 * and indented; `toolIntention` is normal text indented four spaces, then a
 * blank line separates the unindented muted `toolResult`. Collapsed rows show
 * the raw rendering until a summary exists; expanded rows (Pi's Ctrl+O) are
 * always raw.
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
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import {
  createEditToolDefinition,
  createFindToolDefinition,
  createGrepToolDefinition,
  createLsToolDefinition,
  createPowerShellToolDefinition,
  createWriteToolDefinition,
  getMarkdownTheme,
} from "@earendil-works/pi-coding-agent";
import type { DisplaySummary, DisplaySummaryStore } from "./display-summary.ts";
import { UseNativeResultFallback } from "./native-fallback.ts";
import { loadHostPiTui } from "./pi-tui.ts";
import { summaryIdOf } from "./summary-id.ts";

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

/** Shared field layout relative to the header; padX is the row's frame margin. */
export function buildSummaryFields(tui: SummaryTextModules, summary: DisplaySummary, theme: unknown, padX = 0): SummaryComponent[] {
  const fields: SummaryComponent[] = [summarySpacer()];
  if (summary.optionalContext) fields.push(new tui.Text(paintFg(theme, "dim", summary.optionalContext), padX + SUMMARY_FIELD_INDENT, 0));
  fields.push(new tui.Text(paintFg(theme, "text", summary.toolIntention), padX + 4, 0));
  fields.push(summarySpacer());
  fields.push(new tui.Text(paintFg(theme, "muted", summary.toolResult), padX, 0));
  return fields;
}

/** A painted header line over the fields, with no frame of its own. */
export function buildSummaryBlock(tui: SummaryTextModules, header: string, summary: DisplaySummary, theme: unknown): SummaryComponent {
  return stack([new tui.Text(header, 0, 0), ...buildSummaryFields(tui, summary, theme)]);
}

/** A message summary on the custom-message card: the shared `customMessageBg` box, header, fields. */
export function buildSummaryCard(tui: SummaryCardModules, header: string, summary: DisplaySummary, theme: unknown): SummaryComponent {
  const box = new tui.Box(1, 1, (text) => paintBg(theme, "customMessageBg", text));
  box.addChild(new tui.Text(header, 0, 0));
  for (const field of buildSummaryFields(tui, summary, theme)) box.addChild(field);
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
  let cached: { summary: DisplaySummary; component: SummaryComponent } | undefined;
  return {
    render(width: number): string[] {
      const summary = store.get(id);
      if (!summary) return raw.render(width);
      if (cached?.summary !== summary) cached = { summary, component: buildSummary(summary) };
      return cached.component.render(width);
    },
    invalidate(): void {
      raw.invalidate();
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
  invalidate?: unknown;
  lastComponent?: unknown;
  expanded?: unknown;
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
 * (`renderShell: "self"`).
 */
export function wrapToolRenderersWithSummary(
  toolName: string,
  renderCall: ToolCallRenderer | undefined,
  renderResult: ToolResultRenderer | undefined,
  store: DisplaySummaryStoreSource,
  tui: SummaryTuiSource,
  options: { padX?: number } = {},
): { renderCall: ToolCallRenderer; renderResult: ToolResultRenderer } {
  const padX = options.padX ?? 0;

  function summaryFor(context: SummaryRenderContext | undefined, expanded: unknown): { summary: DisplaySummary; tui: SummaryTextModules } | undefined {
    const current = resolveStore(store);
    if (!current || !context || typeof context.toolCallId !== "string") return undefined;
    if (typeof context.invalidate === "function") current.trackInvalidate(context.toolCallId, context.invalidate as () => void);
    if (expanded) return undefined;
    const summary = current.get(context.toolCallId);
    const modules = summary ? resolveTui(tui) : undefined;
    return summary && modules ? { summary, tui: modules } : undefined;
  }

  function rawContext(context: unknown): unknown {
    const ctx = asContext(context);
    return ctx && isSummaryComponent(ctx.lastComponent) ? { ...ctx, lastComponent: undefined } : context;
  }

  return {
    renderCall(args, theme, context) {
      const ctx = asContext(context);
      const hit = summaryFor(ctx, ctx?.expanded);
      if (hit) return stack([new hit.tui.Text(paintFg(theme, "toolTitle", paintBold(theme, toolName)), padX, 0)]);
      if (!renderCall) throw new UseNativeResultFallback();
      return renderCall(args, theme, rawContext(context));
    },
    renderResult(result, renderOptions, theme, context) {
      const ctx = asContext(context);
      const hit = summaryFor(ctx, (renderOptions as { expanded?: unknown } | undefined)?.expanded);
      if (hit) return stack(buildSummaryFields(hit.tui, hit.summary, theme, padX));
      if (!renderResult) throw new UseNativeResultFallback();
      return renderResult(result, renderOptions, theme, rawContext(context));
    },
  };
}

type ToolDefinition = Parameters<ExtensionAPI["registerTool"]>[0];

/** Pi built-ins whose lead rows are summarized when the session activates them. */
const SUMMARIZED_BUILTIN_FACTORIES: Record<string, (cwd: string) => unknown> = {
  edit: createEditToolDefinition,
  write: createWriteToolDefinition,
  grep: createGrepToolDefinition,
  find: createFindToolDefinition,
  ls: createLsToolDefinition,
  powershell: createPowerShellToolDefinition,
};

export const SUMMARIZED_BUILTIN_TOOL_NAMES: readonly string[] = Object.keys(SUMMARIZED_BUILTIN_FACTORIES);

/**
 * Registers a same-name wrapper for each active summarized built-in: the
 * native definition spread as-is (name, description, parameters and execute
 * unchanged, which fork registration comparison relies on) with only its
 * renderers wrapped. Lead TUI only, at `session_start`; returns the wrapped
 * names, which are also added to `store.toolNames`.
 */
export function registerSummarizedBuiltinTools(
  pi: Pick<ExtensionAPI, "registerTool">,
  cwd: string,
  store: DisplaySummaryStore,
  activeToolNames: Iterable<string>,
  tuiModules?: SummaryTextModules,
): string[] {
  const active = new Set(activeToolNames);
  const tuiRef: { current: SummaryTextModules | undefined } = { current: tuiModules };
  if (!tuiModules) {
    // Summaries arrive after a model round trip; until the host copy loads,
    // rows simply stay raw.
    void loadHostPiTui().then((modules) => { tuiRef.current ??= modules as unknown as SummaryTextModules; }, () => {});
  }
  const registered: string[] = [];
  for (const [name, create] of Object.entries(SUMMARIZED_BUILTIN_FACTORIES)) {
    if (!active.has(name)) continue;
    const native = create(cwd) as ToolDefinition & { renderCall?: ToolCallRenderer; renderResult?: ToolResultRenderer; renderShell?: string };
    const wrapped = wrapToolRenderersWithSummary(name, native.renderCall, native.renderResult, store, tuiRef, {
      padX: native.renderShell === "self" ? 1 : 0,
    });
    pi.registerTool({ ...native, renderCall: wrapped.renderCall, renderResult: wrapped.renderResult } as ToolDefinition);
    store.toolNames.add(name);
    registered.push(name);
  }
  return registered;
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
  const header = paintFg(theme, "customMessageLabel", paintBold(theme, summaryKindLabel(customType)));
  return summarizedRow(raw, summaries, summaryIdOf(message.details), expanded, (summary) => buildSummaryCard(tui, header, summary, theme));
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
  const tui = tuiModules ?? ((await loadHostPiTui()) as unknown as AdapterMessageTuiModules);
  for (const customType of ADAPTER_SUMMARIZED_MESSAGE_TYPES) {
    pi.registerMessageRenderer(customType, (message, options, theme) =>
      buildAdapterMessageComponent(tui, message as { customType?: unknown; content?: unknown; details?: unknown }, theme, (options as { expanded?: boolean } | undefined)?.expanded, summaries) as never,
    );
  }
  return true;
}

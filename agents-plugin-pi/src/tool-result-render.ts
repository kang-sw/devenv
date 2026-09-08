import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { stringify as stringifyYaml } from "yaml";

/** The tiny host surface required for YAML previews. */
export interface ToolResultTuiModules {
  Text: new (text?: string, paddingX?: number, paddingY?: number) => NativeText;
  Box: new (paddingX?: number, paddingY?: number, bgFn?: (text: string) => string) => NativeBox;
  stripTerminalSequences(text: string): string;
  truncateToWidth(text: string, width: number, ellipsis?: string): string;
  /** Test-only optional probes for cached preparation/display work. */
  onPreviewLayout?: () => void;
  onPreviewJoin?: () => void;
  onPreviewStyle?: () => void;
}

export interface NativeText {
  setText(text: string): void;
  render(width: number): string[];
  invalidate(): void;
}

export interface NativePreviewComponent {
  render(width: number): string[];
  invalidate(): void;
}

/** A late-filled TUI reference lets native tools register before MCP startup. */
export interface ToolPreviewTuiRef {
  current: ToolResultTuiModules | undefined;
}

export function createToolPreviewTuiRef(): ToolPreviewTuiRef {
  return { current: undefined };
}

export interface NativeBox extends NativePreviewComponent {
  addChild(component: NativePreviewComponent): void;
  setBgFn(bgFn?: (text: string) => string): void;
}

interface ToolPreviewTheme {
  bold(text: string): string;
  fg(color: "text" | "toolTitle" | "toolOutput", text: string): string;
}

export type YamlSerializer = (value: object) => string;

interface InputCache {
  args: unknown;
  text: string;
}

interface ResultCache {
  content: unknown;
  text: string;
}

interface PreviewState {
  input?: InputCache;
  result?: ResultCache;
  /** 260906 Phase 2: last-seen resolved-model line, sticky across calls that report none (see `ToolPreviewOverrides.resolvedLine`). */
  resolvedLine?: string;
}

interface PreviewFormat {
  expanded: boolean;
  trimOuterWhitespace: boolean;
  markerIndent?: number;
  markerStyle?: (text: string) => string;
}

interface BoundedText extends NativePreviewComponent {
  text: NativeText;
  source: string | undefined;
  sanitized: string | undefined;
  style: ((text: string) => string) | undefined;
  format: PreviewFormat | undefined;
  display: string | undefined;
  displayTheme: unknown;
  plain: string | undefined;
  theme: unknown;
  layoutKey: string | undefined;
  plainLayout: string[] | undefined;
  marker: string | undefined;
  cachedWidth: number | undefined;
  cachedNativeLines: string[] | undefined;
  cachedLines: string[] | undefined;
}

interface CallPreviewComponent extends NativePreviewComponent {
  title: BoundedText;
  input: BoundedText;
  inputBox: NativeBox;
}

interface ResultPreviewComponent extends NativePreviewComponent {
  output: BoundedText;
  outputBox: NativeBox;
}

/**
 * 260906 Phase 2: the resolved-model-line variant of `ResultPreviewComponent`
 * — a distinct shape (`resolvedLine` field) so `isResolvedResultComponent`
 * never collides with the plain `isResultPreviewComponent` check used on the
 * unmodified default path. `hasBody` toggles per render: a partial/error
 * call shows only `resolvedLine`, a completed single-text result shows both.
 */
interface ResolvedResultComponent extends NativePreviewComponent {
  resolvedLine: BoundedText;
  output: BoundedText;
  outputBox: NativeBox;
  hasBody: boolean;
}

const previewStateKey = Symbol("ws-yaml-physical-preview");
const PREVIEW_ROWS = 10;
const INPUT_START_INDENT = 4;
const CONTINUATION_INDENT = 3;

function fittedIndent(width: number, preferred: number, remainder: string): number {
  if (!remainder) return Math.min(preferred, width);
  const firstCodePoint = String.fromCodePoint(remainder.codePointAt(0)!);
  return Math.max(0, Math.min(preferred, width - approximateCodePointWidth(firstCodePoint)));
}

function truncatedMarker(width: number, preferredIndent = 0): string {
  if (width <= 0) return "";
  const indent = Math.max(0, Math.min(preferredIndent, width - 1));
  return `${" ".repeat(indent)}${".".repeat(Math.min(3, width - indent))}`;
}

interface PhysicalPreviewLayout {
  rows: string[];
  marker: string | undefined;
}

function physicalPreviewLayout(
  text: string,
  width: number,
  { expanded, trimOuterWhitespace, markerIndent }: PreviewFormat,
): PhysicalPreviewLayout {
  const source = trimOuterWhitespace ? text.trim() : text;
  const boundedWidth = Math.max(0, Math.floor(width));
  const rows: string[] = [];
  const limit = expanded ? Number.POSITIVE_INFINITY : PREVIEW_ROWS;
  const appendMarker = (): PhysicalPreviewLayout => {
    const marker = truncatedMarker(boundedWidth, markerIndent);
    return marker ? { rows: [...rows, marker], marker } : { rows, marker: undefined };
  };

  let lineStart = 0;
  while (true) {
    const lineEnd = source.indexOf("\n", lineStart);
    const logicalLine = lineEnd === -1 ? source.slice(lineStart) : source.slice(lineStart, lineEnd);
    let firstRow = true;
    let remainder = logicalLine;
    do {
      if (rows.length === limit) return appendMarker();
      const preferredIndent = firstRow ? INPUT_START_INDENT : CONTINUATION_INDENT;
      const indent = fittedIndent(boundedWidth, preferredIndent, remainder);
      const contentWidth = boundedWidth - indent;

      let consumed = 0;
      let usedWidth = 0;
      for (const codePoint of remainder) {
        const codePointWidth = approximateCodePointWidth(codePoint);
        if (usedWidth + codePointWidth > contentWidth) break;
        usedWidth += codePointWidth;
        consumed += codePoint.length;
      }
      // At one terminal column a two-column code point cannot fit even with
      // zero indent. Expanded output still consumes it so later logical lines
      // are never silently lost; native fitting decides its final appearance.
      if (remainder && consumed === 0) consumed = String.fromCodePoint(remainder.codePointAt(0)!).length;
      rows.push(`${" ".repeat(indent)}${remainder.slice(0, consumed)}`);
      remainder = remainder.slice(consumed);
      firstRow = false;
    } while (remainder);

    if (lineEnd === -1) return { rows, marker: undefined };
    lineStart = lineEnd + 1;
  }
}

/**
 * Wraps logical text into presentation rows. It walks only as far as the
 * collapsed budget needs, avoiding per-redraw grapheme segmentation.
 */
export function physicalPreview(
  text: string,
  width: number,
  format: PreviewFormat,
): string[] {
  return physicalPreviewLayout(text, width, format).rows;
}

/**
 * Pi catches renderer errors and uses its standard text/image fallback for
 * that slot. This marker intentionally keeps unsupported output on that path.
 */
export class UseNativeResultFallback extends Error {}

function isObjectLike(value: unknown): value is object {
  return typeof value === "object" && value !== null;
}

function isInputObject(value: unknown): value is Record<string, unknown> {
  return isObjectLike(value) && !Array.isArray(value);
}

function stateFor(context: { state: unknown }): PreviewState {
  const state = context.state as Record<PropertyKey, unknown>;
  const current = state[previewStateKey];
  if (isObjectLike(current)) return current as PreviewState;
  const next: PreviewState = {};
  state[previewStateKey] = next;
  return next;
}

/** Legacy logical-line helper retained for callers outside the renderer. */
export function logicalPreview(text: string, limit = PREVIEW_ROWS): string {
  return text.replace(/\r\n?/g, "\n").split("\n").slice(0, limit).join("\n");
}

/** YAML only JSON containers; scalar JSON and non-JSON prose stay RAW. */
export function yamlContainerDisplay(text: string, serialize: YamlSerializer = stringifyYaml): string | undefined {
  try {
    const value: unknown = JSON.parse(text);
    if (!isObjectLike(value)) return undefined;
    return serialize(value);
  } catch {
    return undefined;
  }
}

export interface CompletedTextPreview {
  kind: "yaml" | "raw";
  text: string;
}

/** Every completed single text block previews: containers as YAML, all else RAW. */
export function completedTextPreview(text: string, serialize: YamlSerializer = stringifyYaml): CompletedTextPreview {
  const yaml = yamlContainerDisplay(text, serialize);
  return yaml === undefined ? { kind: "raw", text } : { kind: "yaml", text: yaml };
}

/** Object-shaped call arguments are rendered as YAML; physical row capping happens at layout time. */
export function yamlInputPreview(args: unknown, serialize: YamlSerializer = stringifyYaml): string {
  if (!isInputObject(args)) return "";
  try {
    return serialize(args);
  } catch {
    return "";
  }
}

function isSingleTextContent(content: unknown): content is Array<{ type: string; text?: string }> {
  return Array.isArray(content) && content.length === 1 && content[0]?.type === "text";
}

/** Printable ASCII costs one column; all other code points conservatively cost two. */
export function approximateCodePointWidth(codePoint: string): number {
  const value = codePoint.codePointAt(0) ?? 0;
  return value >= 0x20 && value <= 0x7e ? 1 : 2;
}

/** Normalize unsafe controls before width accounting; tabs become stable four-column spaces. */
export function sanitizePreviewText(text: string): string {
  return text
    .replace(/\r\n?/g, "\n")
    .replace(/\t/g, "    ")
    .replace(/[\u0000-\u0008\u000b-\u001f\u007f]/g, "?");
}

function createBoundedText(tui: ToolResultTuiModules): BoundedText {
  const component: BoundedText = {
    text: new tui.Text("", 0, 0),
    source: undefined,
    sanitized: undefined,
    style: undefined,
    format: undefined,
    display: undefined,
    displayTheme: undefined,
    plain: undefined,
    theme: undefined,
    layoutKey: undefined,
    plainLayout: undefined,
    marker: undefined,
    cachedWidth: undefined,
    cachedNativeLines: undefined,
    cachedLines: undefined,
    render(width: number): string[] {
      const boundedWidth = Math.max(0, Math.floor(width));
      if (boundedWidth === 0) return [];
      const layoutKey = component.format
        ? `${boundedWidth}:${component.format.expanded ? "expanded" : "collapsed"}:${component.format.trimOuterWhitespace ? "trim" : "raw"}:${component.format.markerIndent ?? 0}`
        : undefined;
      if (layoutKey !== undefined && (component.layoutKey !== layoutKey || component.plainLayout === undefined)) {
        tui.onPreviewLayout?.();
        const layout = physicalPreviewLayout(component.sanitized ?? "", boundedWidth, component.format!);
        component.plainLayout = layout.rows;
        component.plain = undefined;
        component.display = undefined;
        component.displayTheme = undefined;
        component.marker = layout.marker;
        component.layoutKey = layoutKey;
      }
      const plainRows = component.format ? component.plainLayout ?? [] : undefined;
      if (component.plain === undefined) {
        tui.onPreviewJoin?.();
        component.plain = plainRows ? plainRows.join("\n") : component.sanitized ?? "";
      }
      const plain = component.plain;
      if (component.display === undefined || component.displayTheme !== component.theme) {
        tui.onPreviewStyle?.();
        const markerPrefix = component.marker ? `\n${component.marker}` : "";
        const body = component.marker ? plain.slice(0, -markerPrefix.length) : plain;
        component.display = component.marker && component.format?.markerStyle
          ? `${component.style?.(body) ?? body}\n${component.format.markerStyle(component.marker)}`
          : component.style?.(plain) ?? plain;
        component.displayTheme = component.theme;
        component.text.setText(component.display);
        component.cachedWidth = undefined;
        component.cachedNativeLines = undefined;
        component.cachedLines = undefined;
      }
      const nativeLines = component.text.render(boundedWidth);
      if (component.cachedWidth === boundedWidth && component.cachedNativeLines === nativeLines && component.cachedLines) {
        return component.cachedLines;
      }
      component.cachedWidth = boundedWidth;
      component.cachedNativeLines = nativeLines;
      component.cachedLines = nativeLines.map((line) => tui.truncateToWidth(line, boundedWidth, ""));
      return component.cachedLines;
    },
    invalidate(): void {
      component.text.invalidate();
      component.display = undefined;
      component.displayTheme = undefined;
      component.cachedWidth = undefined;
      component.cachedNativeLines = undefined;
      component.cachedLines = undefined;
    },
  };
  return component;
}

function updateText(
  tui: ToolResultTuiModules,
  component: BoundedText,
  source: string,
  style: (text: string) => string,
  format?: PreviewFormat,
  theme?: unknown,
): void {
  if (component.source !== source) {
    component.source = source;
    component.sanitized = sanitizePreviewText(tui.stripTerminalSequences(source));
    component.display = undefined;
    component.displayTheme = undefined;
    component.plain = undefined;
    component.layoutKey = undefined;
    component.plainLayout = undefined;
    component.marker = undefined;
    component.cachedWidth = undefined;
    component.cachedNativeLines = undefined;
    component.cachedLines = undefined;
  }
  component.style = style;
  component.theme = theme;
  component.format = format;
}

function isCallPreviewComponent(component: unknown): component is CallPreviewComponent {
  return isObjectLike(component) && "title" in component && "inputBox" in component;
}

function isResultPreviewComponent(component: unknown): component is ResultPreviewComponent {
  return isObjectLike(component) && "output" in component && "outputBox" in component;
}

function isResolvedResultComponent(component: unknown): component is ResolvedResultComponent {
  return isObjectLike(component) && "resolvedLine" in component && "outputBox" in component;
}

/** The call owns both separators so every result path follows the same input boundary. */
function createInputPreview(preview: NativePreviewComponent): NativePreviewComponent {
  return {
    render(width: number): string[] {
      if (width <= 0) return [];
      return ["", ...preview.render(width), ""];
    },
    invalidate(): void {
      preview.invalidate();
    },
  };
}

function createCallPreviewComponent(tui: ToolResultTuiModules): CallPreviewComponent {
  const title = createBoundedText(tui);
  const input = createBoundedText(tui);
  const inputBox = new tui.Box(0, 0);
  inputBox.addChild(createInputPreview(input));
  return {
    title,
    input,
    inputBox,
    render(width: number): string[] {
      return [...title.render(width), ...inputBox.render(width)];
    },
    invalidate(): void {
      title.invalidate();
      inputBox.invalidate();
    },
  };
}

function createResultPreviewComponent(tui: ToolResultTuiModules): ResultPreviewComponent {
  const output = createBoundedText(tui);
  const outputBox = new tui.Box(0, 0);
  outputBox.addChild(output);
  return {
    output,
    outputBox,
    render(width: number): string[] {
      return outputBox.render(width);
    },
    invalidate(): void {
      outputBox.invalidate();
    },
  };
}

/**
 * 260906 Phase 2: `resolvedLine` renders alone when `hasBody` is `false`
 * (partial/error results), or with a blank separator plus the existing
 * YAML/RAW body box beneath it once a completed single-text result arrives
 * — same input/output separator convention `createInputPreview` uses.
 */
function createResolvedResultComponent(tui: ToolResultTuiModules): ResolvedResultComponent {
  const resolvedLine = createBoundedText(tui);
  const output = createBoundedText(tui);
  const outputBox = new tui.Box(0, 0);
  outputBox.addChild(output);
  const component: ResolvedResultComponent = {
    resolvedLine,
    output,
    outputBox,
    hasBody: false,
    render(width: number): string[] {
      const lineRows = resolvedLine.render(width);
      return component.hasBody ? [...lineRows, "", ...outputBox.render(width)] : lineRows;
    },
    invalidate(): void {
      resolvedLine.invalidate();
      outputBox.invalidate();
    },
  };
  return component;
}

export interface PreviewRenderContext {
  state: unknown;
  lastComponent: unknown;
  argsComplete: boolean;
  isPartial: boolean;
  isError?: boolean;
}

/**
 * 260906 Phase 2 (YAML/TUI dispatch-row rendering): optional per-tool
 * overrides to `createToolPreviewRenderers`'s two hooks. Both fields
 * default to `undefined`, in which case behavior is byte-identical to the
 * pre-Phase-2 generic YAML-dump renderer (every other `registerWsTool`
 * caller, e.g. `ws-approve`/read/exec, stays on that unmodified default
 * path).
 */
export interface ToolPreviewOverrides {
  /** Replaces `yamlInputPreview(args, serialize)` as the call-preview text. */
  buildCallPreview?: (args: unknown, context: PreviewRenderContext) => string;
  /**
   * Computes the resolved-model line for a result. Called on every
   * `renderResult` invocation (partial, success, error alike); a `undefined`
   * return leaves the last cached line in place (the cross-call cache path —
   * a later error/partial call with no `result.details` still shows the
   * last-seen line). When this returns/has ever returned a defined line,
   * `renderResult` no longer throws `UseNativeResultFallback` purely because
   * of `isPartial`/`isError`.
   */
  resolvedLine?: (result: { details?: unknown }, context: PreviewRenderContext) => string | undefined;
}

/** Creates the two Pi renderer hooks once the guarded host import succeeds. */
export function createToolPreviewRenderers(
  tui: ToolResultTuiModules,
  toolName: string,
  serialize: YamlSerializer = stringifyYaml,
  overrides?: ToolPreviewOverrides,
): {
  renderCall(args: unknown, theme: unknown, context: PreviewRenderContext): NativePreviewComponent;
  renderResult(
    result: { content?: unknown; details?: unknown },
    options: { expanded: boolean; isPartial: boolean },
    theme: unknown,
    context: PreviewRenderContext,
  ): NativePreviewComponent;
} {
  return {
    renderCall(args, theme, context) {
      const state = stateFor(context);
      // Streaming argument objects can be mutated in place. Re-prepare while
      // incomplete; after completion their stable object identity is enough.
      const preview = context.argsComplete && state.input?.args === args
        ? state.input.text
        : overrides?.buildCallPreview
          ? overrides.buildCallPreview(args, context)
          : yamlInputPreview(args, serialize);
      if (context.argsComplete) state.input = { args, text: preview };
      else state.input = undefined;

      const component = isCallPreviewComponent(context.lastComponent)
        ? context.lastComponent
        : createCallPreviewComponent(tui);
      const previewTheme = theme as ToolPreviewTheme;
      updateText(tui, component.title, toolName, (text) => previewTheme.fg("toolTitle", previewTheme.bold(text)), undefined, previewTheme);
      updateText(tui, component.input, preview, (text) => previewTheme.fg("text", text), {
        expanded: false,
        trimOuterWhitespace: true,
        markerIndent: INPUT_START_INDENT,
        markerStyle: (marker) => previewTheme.fg("toolOutput", marker),
      }, previewTheme);
      return component;
    },

    renderResult(result, options, theme, context) {
      const previewTheme = theme as ToolPreviewTheme;
      const state = stateFor(context);

      if (!overrides?.resolvedLine) {
        // Byte-identical to the pre-Phase-2 behavior: errors, partials, and
        // non-text/mixed content retain Pi's native fallback. Completed
        // single text blocks always preview, preserving RAW prose/scalars
        // byte-for-byte before display-only sanitization.
        if (options.isPartial || context.isPartial || context.isError || !isSingleTextContent(result.content)) {
          throw new UseNativeResultFallback();
        }
        const raw = result.content[0]?.text ?? "";
        const rendered = state.result?.content === result.content
          ? state.result.text
          : completedTextPreview(raw, serialize).text;
        state.result = { content: result.content, text: rendered };

        const component = isResultPreviewComponent(context.lastComponent)
          ? context.lastComponent
          : createResultPreviewComponent(tui);
        updateText(tui, component.output, rendered, (output) => previewTheme.fg("toolOutput", output), {
          expanded: options.expanded,
          trimOuterWhitespace: false,
        }, previewTheme);
        return component;
      }

      // 260906 Phase 2: a resolved line is available (now or from a prior
      // call on this same context) — a mandatory display, so partial/error
      // results still render it instead of falling back to Pi's native
      // display.
      const computed = overrides.resolvedLine(result, context);
      if (computed !== undefined) state.resolvedLine = computed;
      const resolvedLineText = state.resolvedLine;

      const bodyAvailable = !options.isPartial && !context.isPartial && !context.isError && isSingleTextContent(result.content);
      if (resolvedLineText === undefined && !bodyAvailable) {
        throw new UseNativeResultFallback();
      }

      const component = isResolvedResultComponent(context.lastComponent)
        ? context.lastComponent
        : createResolvedResultComponent(tui);
      updateText(tui, component.resolvedLine, resolvedLineText ?? "", (text) => previewTheme.fg("toolOutput", text), undefined, previewTheme);

      if (bodyAvailable) {
        const raw = (result.content as Array<{ type: string; text?: string }>)[0]?.text ?? "";
        const rendered = state.result?.content === result.content
          ? state.result.text
          : completedTextPreview(raw, serialize).text;
        state.result = { content: result.content, text: rendered };
        component.hasBody = true;
        updateText(tui, component.output, rendered, (output) => previewTheme.fg("toolOutput", output), {
          expanded: options.expanded,
          trimOuterWhitespace: false,
        }, previewTheme);
      } else {
        component.hasBody = false;
      }
      return component;
    },
  };
}

type ToolDefinition = Parameters<ExtensionAPI["registerTool"]>[0];

/**
 * Register a ws-owned tool through the one presentation seam. Existing custom
 * renderers are deliberately left untouched; unavailable helpers throw into
 * Pi's documented per-slot native fallback.
 */
export function registerWsTool(
  pi: Pick<ExtensionAPI, "registerTool">,
  definition: ToolDefinition,
  tuiRef: ToolPreviewTuiRef,
): void {
  const existing = definition as ToolDefinition & { renderCall?: unknown; renderResult?: unknown };
  if (existing.renderCall || existing.renderResult) {
    pi.registerTool(definition);
    return;
  }

  let cachedTui: ToolResultTuiModules | undefined;
  let cachedRenderers: ReturnType<typeof createToolPreviewRenderers> | undefined;
  const renderers = () => {
    const tui = tuiRef.current;
    if (!tui) throw new UseNativeResultFallback();
    if (cachedTui !== tui || !cachedRenderers) {
      cachedTui = tui;
      cachedRenderers = createToolPreviewRenderers(tui, definition.name);
    }
    return cachedRenderers;
  };
  pi.registerTool({
    ...definition,
    renderCall: (...args: Parameters<ReturnType<typeof createToolPreviewRenderers>["renderCall"]>) => renderers().renderCall(...args),
    renderResult: (...args: Parameters<ReturnType<typeof createToolPreviewRenderers>["renderResult"]>) => renderers().renderResult(...args),
  } as ToolDefinition);
}

/** Guarded because Pi resolves its nested TUI package only while loading us. */
export async function loadToolResultTuiModules(): Promise<ToolResultTuiModules | undefined> {
  try {
    const tui = await import("@earendil-works/pi-tui") as unknown as ToolResultTuiModules;
    return typeof tui?.Text === "function" &&
      typeof tui?.Box === "function" &&
      typeof tui?.stripTerminalSequences === "function" &&
      typeof tui?.truncateToWidth === "function"
      ? tui
      : undefined;
  } catch {
    return undefined;
  }
}

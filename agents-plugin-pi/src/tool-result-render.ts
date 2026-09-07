import { stringify as stringifyYaml } from "yaml";

/** The tiny host surface required for YAML previews. */
export interface ToolResultTuiModules {
  Text: new (text?: string, paddingX?: number, paddingY?: number) => NativeText;
  Box: new (paddingX?: number, paddingY?: number, bgFn?: (text: string) => string) => NativeBox;
  stripTerminalSequences(text: string): string;
  truncateToWidth(text: string, width: number, ellipsis?: string): string;
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

export interface NativeBox extends NativePreviewComponent {
  addChild(component: NativePreviewComponent): void;
  setBgFn(bgFn?: (text: string) => string): void;
}

interface ToolPreviewTheme {
  bold(text: string): string;
  fg(color: "text" | "toolTitle" | "toolOutput", text: string): string;
  bg(color: "toolPendingBg" | "toolSuccessBg", text: string): string;
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
}

interface PreviewFormat {
  expanded: boolean;
  trimOuterWhitespace: boolean;
}

interface BoundedText extends NativePreviewComponent {
  text: NativeText;
  source: string | undefined;
  sanitized: string | undefined;
  style: ((text: string) => string) | undefined;
  format: PreviewFormat | undefined;
  display: string | undefined;
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

const previewStateKey = Symbol("ws-yaml-physical-preview");
const PREVIEW_ROWS = 10;
const INPUT_START_INDENT = 4;
const CONTINUATION_INDENT = 3;

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

/** YAML only JSON containers; scalar JSON and non-JSON prose stay native. */
export function yamlContainerDisplay(text: string, serialize: YamlSerializer = stringifyYaml): string | undefined {
  try {
    const value: unknown = JSON.parse(text);
    if (!isObjectLike(value)) return undefined;
    return serialize(value);
  } catch {
    return undefined;
  }
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

/**
 * Wraps logical text into presentation rows. It walks only as far as the
 * collapsed budget needs, avoiding per-redraw grapheme segmentation.
 */
export function physicalPreview(
  text: string,
  width: number,
  { expanded, trimOuterWhitespace }: PreviewFormat,
): string[] {
  const source = trimOuterWhitespace ? text.trim() : text;
  const boundedWidth = Math.max(0, Math.floor(width));
  const rows: string[] = [];
  const limit = expanded ? Number.POSITIVE_INFINITY : PREVIEW_ROWS;
  const appendMarker = (): string[] => [...rows, "..."];

  let lineStart = 0;
  while (true) {
    const lineEnd = source.indexOf("\n", lineStart);
    const logicalLine = lineEnd === -1 ? source.slice(lineStart) : source.slice(lineStart, lineEnd);
    let firstRow = true;
    let remainder = logicalLine;
    do {
      if (rows.length === limit) return appendMarker();
      const indent = firstRow ? INPUT_START_INDENT : CONTINUATION_INDENT;
      const contentWidth = boundedWidth - indent;
      if (contentWidth <= 0) {
        rows.push("");
        const hasHiddenContent = remainder.length > 0 || lineEnd !== -1;
        return !expanded && hasHiddenContent ? appendMarker() : rows;
      }

      let consumed = 0;
      let usedWidth = 0;
      for (const codePoint of remainder) {
        const codePointWidth = approximateCodePointWidth(codePoint);
        if (usedWidth + codePointWidth > contentWidth) break;
        usedWidth += codePointWidth;
        consumed += codePoint.length;
      }
      // A non-empty line always consumes at least one code point while a
      // positive width is available: non-ASCII costs two, so width one is a
      // deliberate conservative early-wrap/truncation case.
      if (remainder && consumed === 0) {
        rows.push(" ".repeat(indent));
        return expanded ? rows : appendMarker();
      }
      rows.push(`${" ".repeat(indent)}${remainder.slice(0, consumed)}`);
      remainder = remainder.slice(consumed);
      firstRow = false;
    } while (remainder);

    if (lineEnd === -1) return rows;
    lineStart = lineEnd + 1;
  }
}

function createBoundedText(tui: ToolResultTuiModules): BoundedText {
  const component: BoundedText = {
    text: new tui.Text("", 0, 0),
    source: undefined,
    sanitized: undefined,
    style: undefined,
    format: undefined,
    display: undefined,
    cachedWidth: undefined,
    cachedNativeLines: undefined,
    cachedLines: undefined,
    render(width: number): string[] {
      const boundedWidth = Math.max(0, Math.floor(width));
      const plain = component.format
        ? physicalPreview(component.sanitized ?? "", boundedWidth, component.format).join("\n")
        : component.sanitized ?? "";
      const display = component.style?.(plain) ?? plain;
      if (component.display !== display) {
        component.text.setText(display);
        component.display = display;
        component.cachedWidth = undefined;
        component.cachedNativeLines = undefined;
        component.cachedLines = undefined;
      }
      if (boundedWidth === 0) return [];
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
): void {
  if (component.source !== source) {
    component.source = source;
    component.sanitized = sanitizePreviewText(tui.stripTerminalSequences(source));
    component.display = undefined;
    component.cachedWidth = undefined;
    component.cachedNativeLines = undefined;
    component.cachedLines = undefined;
  }
  component.style = style;
  component.format = format;
}

function isCallPreviewComponent(component: unknown): component is CallPreviewComponent {
  return isObjectLike(component) && "title" in component && "inputBox" in component;
}

function isResultPreviewComponent(component: unknown): component is ResultPreviewComponent {
  return isObjectLike(component) && "output" in component && "outputBox" in component;
}

/** The parent shell supplies its own padding; these rows supply only separators. */
function createSeparatedPreview(preview: NativePreviewComponent): NativePreviewComponent {
  return {
    render(width: number): string[] {
      if (width <= 0) return [];
      return ["", ...preview.render(width)];
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
  inputBox.addChild(createSeparatedPreview(input));
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
  outputBox.addChild(createSeparatedPreview(output));
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

export interface PreviewRenderContext {
  state: unknown;
  lastComponent: unknown;
  argsComplete: boolean;
  isPartial: boolean;
  isError?: boolean;
}

/** Creates the two Pi renderer hooks once the guarded host import succeeds. */
export function createToolPreviewRenderers(
  tui: ToolResultTuiModules,
  toolName: string,
  serialize: YamlSerializer = stringifyYaml,
): {
  renderCall(args: unknown, theme: unknown, context: PreviewRenderContext): NativePreviewComponent;
  renderResult(
    result: { content?: unknown },
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
        : yamlInputPreview(args, serialize);
      if (context.argsComplete) state.input = { args, text: preview };
      else state.input = undefined;

      const component = isCallPreviewComponent(context.lastComponent)
        ? context.lastComponent
        : createCallPreviewComponent(tui);
      const previewTheme = theme as ToolPreviewTheme;
      updateText(tui, component.title, toolName, (text) => previewTheme.fg("toolTitle", previewTheme.bold(text)));
      updateText(tui, component.input, preview, (text) => previewTheme.fg("text", text), {
        expanded: false,
        trimOuterWhitespace: true,
      });
      component.inputBox.setBgFn((text) => previewTheme.bg("toolPendingBg", text));
      return component;
    },

    renderResult(result, options, theme, context) {
      if (options.isPartial || context.isPartial || context.isError || !isSingleTextContent(result.content)) {
        throw new UseNativeResultFallback();
      }
      const raw = result.content[0]?.text ?? "";
      const state = stateFor(context);
      const rendered = state.result?.content === result.content
        ? state.result.text
        : yamlContainerDisplay(raw, serialize);
      // Errors, prose, scalar JSON, later text blocks, and image/mixed output
      // stay on Pi's existing text/image fallback path.
      if (rendered === undefined) throw new UseNativeResultFallback();
      state.result = { content: result.content, text: rendered };

      const component = isResultPreviewComponent(context.lastComponent)
        ? context.lastComponent
        : createResultPreviewComponent(tui);
      const previewTheme = theme as ToolPreviewTheme;
      updateText(tui, component.output, rendered, (output) => previewTheme.fg("toolOutput", output), {
        expanded: options.expanded,
        trimOuterWhitespace: false,
      });
      component.outputBox.setBgFn((output) => previewTheme.bg("toolSuccessBg", output));
      return component;
    },
  };
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

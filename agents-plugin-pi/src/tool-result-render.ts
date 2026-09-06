import { stringify as stringifyYaml } from "yaml";

/** The tiny host surface required for YAML previews. */
export interface ToolResultTuiModules {
  Text: new (text?: string, paddingX?: number, paddingY?: number) => NativeText;
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

export type YamlSerializer = (value: object) => string;

interface InputCache {
  args: unknown;
  text: string;
}

interface ResultCache {
  content: unknown;
  expanded: boolean;
  text: string;
}

interface PreviewState {
  input?: InputCache;
  result?: ResultCache;
}

interface BoundedText extends NativePreviewComponent {
  text: NativeText;
  source: string | undefined;
  cachedWidth: number | undefined;
  cachedNativeLines: string[] | undefined;
  cachedLines: string[] | undefined;
}

const previewStateKey = Symbol("ws-yaml-logical-preview");
const components = new WeakMap<object, BoundedText>();

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

/** Select logical lines before Pi's native Text component lays them out. */
export function logicalPreview(text: string, limit = 10): string {
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

/** Object-shaped call arguments are rendered as a safe ten-logical-line YAML preview. */
export function yamlInputPreview(args: unknown, serialize: YamlSerializer = stringifyYaml): string {
  if (!isInputObject(args)) return "";
  try {
    return logicalPreview(serialize(args));
  } catch {
    return "";
  }
}

function isSingleTextContent(content: unknown): content is Array<{ type: string; text?: string }> {
  return Array.isArray(content) && content.length === 1 && content[0]?.type === "text";
}

function createBoundedText(tui: ToolResultTuiModules): BoundedText {
  const component: BoundedText = {
    text: new tui.Text("", 0, 0),
    source: undefined,
    cachedWidth: undefined,
    cachedNativeLines: undefined,
    cachedLines: undefined,
    render(width: number): string[] {
      const nativeLines = component.text.render(width);
      if (component.cachedWidth === width && component.cachedNativeLines === nativeLines && component.cachedLines) {
        return component.cachedLines;
      }
      // Native Text owns layout. This only clips an indivisible grapheme that
      // is wider than a narrow terminal row; it is not a Unicode engine.
      const boundedWidth = Math.max(0, Math.floor(width));
      component.cachedWidth = width;
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

function updateText(tui: ToolResultTuiModules, lastComponent: unknown, source: string): BoundedText {
  const component = isObjectLike(lastComponent) ? components.get(lastComponent) : undefined;
  const next = component ?? createBoundedText(tui);
  if (next.source !== source) {
    next.text.setText(tui.stripTerminalSequences(source));
    next.source = source;
    next.cachedWidth = undefined;
    next.cachedNativeLines = undefined;
    next.cachedLines = undefined;
  }
  components.set(next, next);
  return next;
}

function callDisplayText(toolName: string, preview: string): string {
  return preview ? `${toolName}\n${preview}` : toolName;
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
    renderCall(args, _theme, context) {
      const state = stateFor(context);
      // Streaming argument objects can be mutated in place. Re-prepare while
      // incomplete; after completion their stable object identity is enough.
      const preview = context.argsComplete && state.input?.args === args
        ? state.input.text
        : yamlInputPreview(args, serialize);
      if (context.argsComplete) state.input = { args, text: preview };
      else state.input = undefined;
      return updateText(tui, context.lastComponent, callDisplayText(toolName, preview));
    },

    renderResult(result, options, _theme, context) {
      if (options.isPartial || context.isPartial || context.isError || !isSingleTextContent(result.content)) {
        throw new UseNativeResultFallback();
      }
      const raw = result.content[0]?.text ?? "";
      const state = stateFor(context);
      const rendered = state.result?.content === result.content && state.result.expanded === options.expanded
        ? state.result.text
        : yamlContainerDisplay(raw, serialize);
      // Errors, prose, scalar JSON, later text blocks, and image/mixed output
      // stay on Pi's existing text/image fallback path.
      if (rendered === undefined) throw new UseNativeResultFallback();
      const text = options.expanded ? rendered.replace(/\r\n?/g, "\n") : logicalPreview(rendered);
      state.result = { content: result.content, expanded: options.expanded, text };
      return updateText(tui, context.lastComponent, text);
    },
  };
}

/** Guarded because Pi resolves its nested TUI package only while loading us. */
export async function loadToolResultTuiModules(): Promise<ToolResultTuiModules | undefined> {
  try {
    const tui = await import("@earendil-works/pi-tui") as unknown as ToolResultTuiModules;
    return typeof tui?.Text === "function" &&
      typeof tui?.stripTerminalSequences === "function" &&
      typeof tui?.truncateToWidth === "function"
      ? tui
      : undefined;
  } catch {
    return undefined;
  }
}

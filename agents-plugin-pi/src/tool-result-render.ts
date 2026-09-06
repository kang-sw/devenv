import { stringify as stringifyYaml } from "yaml";

/** The tiny host surface required for YAML previews. */
export interface ToolResultTuiModules {
  Text: new (text?: string, paddingX?: number, paddingY?: number) => NativeText;
  stripTerminalSequences(text: string): string;
}

export interface NativeText {
  setText(text: string): void;
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

const previewStateKey = Symbol("ws-yaml-logical-preview");
const componentText = new WeakMap<object, string>();

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

function updateText(
  tui: ToolResultTuiModules,
  lastComponent: unknown,
  source: string,
): NativeText {
  const component = isObjectLike(lastComponent) && componentText.has(lastComponent)
    ? lastComponent as NativeText
    : new tui.Text("", 0, 0);
  if (componentText.get(component) !== source) {
    component.setText(tui.stripTerminalSequences(source));
    componentText.set(component, source);
  }
  return component;
}

export interface PreviewRenderContext {
  state: unknown;
  lastComponent: unknown;
  argsComplete: boolean;
  isPartial: boolean;
  isError?: boolean;
}

/** Creates the two Pi renderer hooks once the guarded host import succeeds. */
export function createToolPreviewRenderers(tui: ToolResultTuiModules, serialize: YamlSerializer = stringifyYaml): {
  renderCall(args: unknown, theme: unknown, context: PreviewRenderContext): NativeText;
  renderResult(
    result: { content?: unknown },
    options: { expanded: boolean; isPartial: boolean },
    theme: unknown,
    context: PreviewRenderContext,
  ): NativeText;
} {
  return {
    renderCall(args, _theme, context) {
      const state = stateFor(context);
      // Streaming argument objects can be mutated in place. Re-prepare while
      // incomplete; after completion their stable object identity is enough.
      const text = context.argsComplete && state.input?.args === args
        ? state.input.text
        : yamlInputPreview(args, serialize);
      if (context.argsComplete) state.input = { args, text };
      else state.input = undefined;
      return updateText(tui, context.lastComponent, text);
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
    return typeof tui?.Text === "function" && typeof tui?.stripTerminalSequences === "function" ? tui : undefined;
  } catch {
    return undefined;
  }
}

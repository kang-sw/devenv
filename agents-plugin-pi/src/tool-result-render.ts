import { keyHint } from "@earendil-works/pi-coding-agent";
import { stringify as stringifyYaml } from "yaml";

/** Generic Pi/MCP content shape; this module intentionally has no bridge dependency. */
export interface ToolResultContentItem {
  type: string;
  text?: string;
  data?: string;
  mimeType?: string;
  [key: string]: unknown;
}

export type YamlSerializer = (value: object) => string;

/**
 * Converts JSON containers to YAML for display only. Scalars are intentionally
 * left alone, and either parser or serializer failure preserves the raw text.
 */
export function yamlDisplayText(text: string, serialize: YamlSerializer = stringifyYaml): string {
  try {
    const value: unknown = JSON.parse(text);
    if (!Array.isArray(value) && (typeof value !== "object" || value === null)) return text;
    return serialize(value);
  } catch {
    return text;
  }
}

/**
 * Drops terminal control sequences before rendering untrusted tool output and
 * expands tabs to Pi's three-cell display convention. Execute content remains
 * untouched; this is a display-only sanitization equivalent to Pi's fallback.
 */
export function sanitizeToolDisplayText(text: string): string {
  let output = "";
  for (let index = 0; index < text.length; index++) {
    const char = text[index];
    if (char === "\x1b") {
      const kind = text[index + 1];
      if (kind === "[") {
        index += 2;
        while (index < text.length && !/[\x40-\x7e]/.test(text[index] ?? "")) index += 1;
      } else if (kind === "]" || kind === "P" || kind === "_" || kind === "^") {
        index += 2;
        while (index < text.length) {
          if (text[index] === "\x07") break;
          if (text[index] === "\x1b" && text[index + 1] === "\\") {
            index += 1;
            break;
          }
          index += 1;
        }
      } else {
        index += 1;
      }
      continue;
    }
    const code = char.codePointAt(0) ?? 0;
    if (code < 0x20 && char !== "\t" && char !== "\n" && char !== "\r") continue;
    output += char;
  }
  return output.replace(/\t/g, "   ");
}

/** Pi-compatible textual fallback when its own image component is disabled. */
function imageFallback(item: ToolResultContentItem): string {
  return `[Image: [${item.mimeType ?? "image/unknown"}]]`;
}

export interface RenderResultTextOptions {
  isError: boolean;
  showImages?: boolean;
  serialize?: YamlSerializer;
}

/**
 * Builds result text without mutating the original content array. Only its
 * first text block can become YAML; errors and all later text stay raw.
 */
export function renderResultText(content: readonly ToolResultContentItem[], options: RenderResultTextOptions): string {
  let firstText = true;
  const textBlocks: string[] = [];
  for (const item of content) {
    if (item.type !== "text") continue;
    const raw = item.text ?? "";
    textBlocks.push(sanitizeToolDisplayText(firstText && !options.isError ? yamlDisplayText(raw, options.serialize) : raw));
    firstText = false;
  }
  if (options.showImages === false) {
    textBlocks.push(...content.filter((item) => item.type === "image").map(imageFallback));
  }
  return textBlocks.join("\n");
}

const graphemeSegmenter = new Intl.Segmenter(undefined, { granularity: "grapheme" });
const zeroWidthGrapheme = /^(?:\p{Control}|\p{Mark}|\p{Default_Ignorable_Code_Point})+$/u;
const leadingNonPrinting = /^[\p{Control}\p{Mark}\p{Default_Ignorable_Code_Point}]+/u;
const rgiEmoji = /^\p{RGI_Emoji}$/v;

function graphemeDisplayWidth(grapheme: string): number {
  if (zeroWidthGrapheme.test(grapheme)) return 0;
  // This includes keycap, flag, skin-tone, and ZWJ sequences as one terminal glyph.
  if (rgiEmoji.test(grapheme)) return 2;
  const base = grapheme.replace(leadingNonPrinting, "");
  const code = base.codePointAt(0);
  if (code === undefined) return 0;
  if (code >= 0x1f1e6 && code <= 0x1f1ff) return 2;
  return (
    (code >= 0x1100 && code <= 0x115f) ||
    (code >= 0x2329 && code <= 0x232a) ||
    (code >= 0x2e80 && code <= 0xa4cf) ||
    (code >= 0xac00 && code <= 0xd7a3) ||
    (code >= 0xf900 && code <= 0xfaff) ||
    (code >= 0xfe10 && code <= 0xfe6f) ||
    (code >= 0xff01 && code <= 0xff60) ||
    (code >= 0xffe0 && code <= 0xffe6) ||
    (code >= 0x20000 && code <= 0x3fffd)
  ) ? 2 : 1;
}

/** Terminal-cell width after applying the same safety normalization as rendering. */
export function visibleDisplayWidth(text: string): number {
  let width = 0;
  for (const { segment } of graphemeSegmenter.segment(sanitizeToolDisplayText(text))) width += graphemeDisplayWidth(segment);
  return width;
}

function wrapDisplayLine(text: string, width: number): string[] {
  const maxWidth = Math.max(1, Math.floor(width));
  if (text.length === 0) return [""];
  const rows: string[] = [];
  let row = "";
  let rowWidth = 0;
  for (const { segment } of graphemeSegmenter.segment(text)) {
    const segmentWidth = graphemeDisplayWidth(segment);
    if (rowWidth > 0 && rowWidth + segmentWidth > maxWidth) {
      rows.push(row);
      row = "";
      rowWidth = 0;
    }
    // A one-column viewport has no faithful wide-glyph representation; use a
    // one-cell placeholder rather than violate Pi's component width contract.
    if (rowWidth === 0 && segmentWidth > maxWidth) {
      rows.push("?");
      continue;
    }
    row += segment;
    rowWidth += segmentWidth;
  }
  if (row.length > 0) rows.push(row);
  return rows.length > 0 ? rows : [""];
}

export interface RenderResultRowsOptions extends RenderResultTextOptions {
  expanded: boolean;
  width: number;
  expandHint?: string;
}

/**
 * Wraps display text and collapses after ten visual body rows. The marker's
 * count is based on wrapped rows, not logical source lines.
 */
export function renderResultRows(content: readonly ToolResultContentItem[], options: RenderResultRowsOptions): string[] {
  const text = renderResultText(content, options);
  if (!text) return [];
  const logicalLines = text.replace(/\r\n?/g, "\n").split("\n");
  if (logicalLines.at(-1) === "") logicalLines.pop();
  const bodyRows = logicalLines.flatMap((line) => wrapDisplayLine(line, options.width));
  if (options.expanded || bodyRows.length <= 10) return bodyRows;
  const remaining = bodyRows.length - 10;
  const hint = options.expandHint ?? keyHint("app.tools.expand", "to expand");
  return [...bodyRows.slice(0, 10), ...wrapDisplayLine(`… ${remaining} more rows (${hint})`, options.width)];
}

/** Minimal structural component; callers supply Pi's theme without pi-tui import. */
export function createToolResultComponent(
  content: readonly ToolResultContentItem[],
  options: { expanded: boolean },
  theme: { fg(color: string, text: string): string },
  context: { isError: boolean; showImages: boolean },
): { render(width: number): string[]; invalidate(): void } {
  return {
    render: (width) => renderResultRows(content, {
      isError: context.isError,
      showImages: context.showImages,
      expanded: options.expanded,
      width,
    }).map((row) => theme.fg("toolOutput", row)),
    invalidate: () => {},
  };
}

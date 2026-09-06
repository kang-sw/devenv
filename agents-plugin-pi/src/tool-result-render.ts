import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";
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
export type DisplayWidth = (text: string) => number;

/**
 * Use Pi's own width model, never a second Unicode table. The guarded dynamic
 * import follows the other adapter renderers; the second resolution supports
 * npm's nested coding-agent dependency layout without hardcoded package paths.
 * If neither is available, YAML rendering remains enabled with a logical-line
 * preview cap followed by ASCII escaping and wrapping. The owner accepts more
 * than ten visual rows and escaped Unicode in this supported fallback; no
 * guessed Unicode metric or default raw renderer is substituted.
 */
export async function loadToolResultWidth(
  load: (specifier: string) => Promise<{ visibleWidth?: DisplayWidth }> = (specifier) => import(specifier),
  nestedSpecifier: () => string = () => pathToFileURL(
    createRequire(import.meta.resolve("@earendil-works/pi-coding-agent")).resolve("@earendil-works/pi-tui"),
  ).href,
): Promise<DisplayWidth | undefined> {
  for (const specifier of [() => "@earendil-works/pi-tui", nestedSpecifier]) {
    try {
      const tui = await load(specifier());
      if (typeof tui.visibleWidth === "function") return tui.visibleWidth;
    } catch {
      // Optional UI capability: do not prevent MCP execution in headless hosts.
    }
  }
  return undefined;
}

function wrapDisplayLine(text: string, width: number, measure: DisplayWidth): string[] {
  const maxWidth = Math.floor(width);
  if (!(maxWidth > 0)) return [];
  if (text.length === 0) return [""];
  const rows: string[] = [];
  let row = "";
  for (const { segment } of graphemeSegmenter.segment(text)) {
    // Measure the actual candidate row, not a sum of independently measured
    // pieces: segmentation and terminal spacing can depend on adjacency.
    if (row && measure(row + segment) > maxWidth) {
      rows.push(row);
      row = "";
    }
    // An indivisible grapheme can occupy more than two cells. If it cannot
    // fit even on an empty row, retain the existing display-only placeholder.
    if (measure(segment) > maxWidth) {
      rows.push("?");
      continue;
    }
    row += segment;
  }
  if (row.length > 0) rows.push(row);
  return rows.length > 0 ? rows : [""];
}

/**
 * Last-resort display-only representation: printable ASCII is one cell per
 * character. Escape UTF-16 code units (including surrogate pairs) rather than
 * approximate Unicode width. Sanitize controls before escaping and wrapping.
 */
function wrapAsciiDisplayLine(text: string, width: number): string[] {
  const maxWidth = Math.floor(width);
  if (!(maxWidth > 0)) return [];
  const ascii = sanitizeToolDisplayText(text).replace(/[^\x20-\x7e]/g, (char) =>
    `\\u${char.charCodeAt(0).toString(16).padStart(4, "0")}`,
  );
  if (!ascii) return [""];
  const rows: string[] = [];
  for (let start = 0; start < ascii.length; start += maxWidth) rows.push(ascii.slice(start, start + maxWidth));
  return rows;
}

export interface RenderResultRowsOptions extends RenderResultTextOptions {
  expanded: boolean;
  width: number;
  measure?: DisplayWidth;
  expandHint?: string;
}

/**
 * With Pi's metric, cap after ten visual rows. Without it, select ten logical
 * lines BEFORE ASCII escaping/wrapping; the marker counts omitted logical
 * lines. Both modes keep each emitted row safe, but the fallback can show more
 * than ten visual body rows. Expanded mode applies the same transform uncapped.
 */
export function renderResultRows(content: readonly ToolResultContentItem[], options: RenderResultRowsOptions): string[] {
  if (!(Math.floor(options.width) > 0)) return [];
  const text = renderResultText(content, options);
  if (!text) return [];
  const logicalLines = text.replace(/\r\n?/g, "\n").split("\n");
  if (logicalLines.at(-1) === "") logicalLines.pop();
  const measure = options.measure;
  const bodyRows = measure ? logicalLines.flatMap((line) => wrapDisplayLine(line, options.width, measure)) : logicalLines;
  const selected = options.expanded ? bodyRows : bodyRows.slice(0, 10);
  if (!options.expanded && bodyRows.length > 10) {
    // keyHint may include theme ANSI; wrap plain text, then color complete rows.
    const hint = sanitizeToolDisplayText(options.expandHint ?? keyHint("app.tools.expand", "to expand"));
    const marker = `… ${bodyRows.length - 10} more ${measure ? "rows" : "lines"} (${hint})`;
    selected.push(...(measure ? wrapDisplayLine(marker, options.width, measure) : [marker]));
  }
  return measure ? selected : selected.flatMap((line) => wrapAsciiDisplayLine(line, options.width));
}

/** Minimal structural component; callers supply Pi's theme without pi-tui import. */
export function createToolResultComponent(
  content: readonly ToolResultContentItem[],
  options: { expanded: boolean },
  theme: { fg(color: string, text: string): string },
  context: { isError: boolean; showImages: boolean },
  measure?: DisplayWidth,
): { render(width: number): string[]; invalidate(): void } {
  return {
    // Without a host metric, keep the final output plain printable ASCII too;
    // theme callbacks must not reintroduce unmeasured text or control sequences.
    render: (width) => renderResultRows(content, {
      isError: context.isError,
      showImages: context.showImages,
      expanded: options.expanded,
      width,
      measure,
    }).map((row) => measure ? theme.fg("toolOutput", row) : row),
    invalidate: () => {},
  };
}

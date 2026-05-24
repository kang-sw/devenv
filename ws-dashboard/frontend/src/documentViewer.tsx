import { createElement, useMemo, useState, type ReactNode } from "react";
import { unified } from "unified";
import remarkParse from "remark-parse";
import remarkGfm from "remark-gfm";
import { toString } from "mdast-util-to-string";

type MarkdownNode = {
  type: string;
  value?: string;
  checked?: boolean | null;
  lang?: string | null;
  url?: string;
  alt?: string;
  identifier?: string;
  label?: string;
  children?: MarkdownNode[];
  position?: {
    start?: { line?: number; column?: number; offset?: number };
    end?: { line?: number; column?: number; offset?: number };
  };
  align?: Array<"left" | "right" | "center" | null>;
};

export type DocumentBlockKind =
  | "paragraph"
  | "heading"
  | "listItem"
  | "callout"
  | "code"
  | "table"
  | "taskItem"
  | string;

export type DocumentBlock = {
  blockId: string;
  ordinal: number;
  kind: DocumentBlockKind;
  markdown: string;
  plainText: string;
  lineStart?: number;
  lineEnd?: number;
  pathref?: string;
  translatable: boolean;
};

export type DocumentTranslationOverlay = {
  contentHash: string;
  blocks: Record<
    string,
    { translatedMarkdown: string; status: "ok" | "pending" | "failed" }
  >;
};

type RenderBlock = DocumentBlock & {
  node: MarkdownNode;
  calloutKind?: string;
};

export type MarkdownDocumentModel = {
  contentHash: string;
  blocks: DocumentBlock[];
  renderBlocks: RenderBlock[];
  footnotes: Record<string, string>;
};

const parser = unified().use(remarkParse).use(remarkGfm);

export function isMarkdownDocumentSource(source: {
  extension: string | null;
  languageHint: string | null;
  path?: string;
}) {
  const extension = source.extension?.toLowerCase().replace(/^\./, "") ?? "";
  const languageHint = source.languageHint?.toLowerCase() ?? "";
  const path = source.path?.toLowerCase() ?? "";
  return (
    languageHint === "markdown" ||
    extension === "md" ||
    extension === "markdown" ||
    path.endsWith(".md") ||
    path.endsWith(".markdown")
  );
}

export function localDocumentContentHash(markdown: string) {
  let hash = 0x811c9dc5;
  for (let index = 0; index < markdown.length; index += 1) {
    hash ^= markdown.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return `fnv1a32:${hash.toString(16).padStart(8, "0")}`;
}

export function buildOverlayKey(contentHash: string, blockId: string) {
  return `${contentHash}:${blockId}`;
}

export function deriveMarkdownDocumentModel(
  markdown: string,
  options: { path?: string; contentHash?: string } = {},
): MarkdownDocumentModel {
  const tree = parser.parse(markdown) as MarkdownNode;
  const contentHash = options.contentHash ?? localDocumentContentHash(markdown);
  const lines = markdown.split(/\r?\n/);
  const renderBlocks: RenderBlock[] = [];
  const pushBlock = (node: MarkdownNode, kind: DocumentBlockKind, calloutKind?: string) => {
    const lineStart = node.position?.start?.line;
    const lineEnd = node.position?.end?.line;
    const plainText = normalizedPlainText(node, kind, calloutKind);
    const ordinal = renderBlocks.length;
    const blockMarkdown = markdownForLines(lines, lineStart, lineEnd) || plainText;
    const blockId = stableBlockId({ ordinal, kind, lineStart, lineEnd, plainText });
    renderBlocks.push({
      blockId,
      ordinal,
      kind,
      markdown: blockMarkdown,
      plainText,
      lineStart,
      lineEnd,
      pathref: options.path && lineStart ? pathrefForLineRange(options.path, lineStart, lineEnd) : undefined,
      translatable: kind !== "code" && plainText.trim().length > 0,
      node,
      calloutKind,
    });
  };

  for (const child of tree.children ?? []) {
    if (child.type === "list") {
      for (const item of child.children ?? []) {
        pushBlock(item, item.checked === true || item.checked === false ? "taskItem" : "listItem");
      }
      continue;
    }
    if (child.type === "blockquote") {
      const calloutKind = calloutKindForNode(child);
      if (calloutKind) {
        pushBlock(child, "callout", calloutKind);
        continue;
      }
      pushBlock(child, "paragraph");
      continue;
    }
    if (child.type === "heading") {
      pushBlock(child, "heading");
    } else if (child.type === "paragraph") {
      pushBlock(child, "paragraph");
    } else if (child.type === "code") {
      pushBlock(child, "code");
    } else if (child.type === "table") {
      pushBlock(child, "table");
    } else if (child.type === "thematicBreak" || child.type === "html") {
      continue;
    } else {
      pushBlock(child, child.type);
    }
  }

  return {
    contentHash,
    blocks: renderBlocks.map(({ node: _node, calloutKind: _calloutKind, ...block }) => block),
    renderBlocks,
    footnotes: footnotesForTree(tree),
  };
}

function markdownForLines(lines: string[], lineStart?: number, lineEnd?: number) {
  if (!lineStart || !lineEnd) {
    return "";
  }
  return lines.slice(lineStart - 1, lineEnd).join("\n");
}

function normalizedPlainText(node: MarkdownNode, kind: string, calloutKind?: string) {
  let text = toString(node as never).replace(/\s+/g, " ").trim();
  if (kind === "callout" && calloutKind) {
    text = text.replace(/^\[![^\]]+\]\s*/i, "").trim();
  }
  return text;
}

function stableBlockId(input: {
  ordinal: number;
  kind: string;
  lineStart?: number;
  lineEnd?: number;
  plainText: string;
}) {
  const material = [
    input.ordinal,
    input.kind,
    input.lineStart ?? "",
    input.lineEnd ?? "",
    input.plainText.toLowerCase().replace(/\s+/g, " ").trim(),
  ].join("|");
  return `${input.kind}-${input.ordinal + 1}-${localDocumentContentHash(material).slice("fnv1a32:".length)}`;
}

function pathrefForLineRange(path: string, lineStart: number, lineEnd = lineStart) {
  const safePath = workRootRelativePathForPathref(path);
  if (!safePath) {
    return undefined;
  }
  if (lineEnd <= lineStart) {
    return `@${safePath}#L${lineStart}`;
  }
  return `@${safePath}#L${lineStart}-L${lineEnd}`;
}

export function workRootRelativePathForPathref(path: string) {
  const candidate = path.replace(/^@+/, "");
  if (
    !candidate ||
    candidate.startsWith("/") ||
    candidate.startsWith("~") ||
    candidate.includes("\\") ||
    candidate.split("/").includes("..") ||
    /^[a-z]:/i.test(candidate)
  ) {
    return undefined;
  }
  return candidate;
}

function calloutKindForNode(node: MarkdownNode) {
  const firstParagraph = node.children?.find((child) => child.type === "paragraph");
  const firstText = firstParagraph?.children?.find((child) => child.type === "text");
  const match = /^\[!([a-z][a-z0-9_-]*)\]/i.exec(firstText?.value ?? "");
  return match?.[1]?.toLowerCase();
}

export function translationForBlock(
  overlay: DocumentTranslationOverlay | undefined,
  contentHash: string,
  blockId: string,
) {
  if (!overlay || overlay.contentHash !== contentHash) {
    return undefined;
  }
  return overlay.blocks[blockId] ?? overlay.blocks[buildOverlayKey(contentHash, blockId)];
}

export function DocumentViewer({
  markdown,
  path,
  overlay,
}: {
  markdown: string;
  path: string;
  overlay?: DocumentTranslationOverlay;
}) {
  const model = useMemo(() => deriveMarkdownDocumentModel(markdown, { path }), [markdown, path]);
  const [selectedBlockIds, setSelectedBlockIds] = useState<Set<string>>(() => new Set());
  const selectedBlocks = model.blocks.filter((block) => selectedBlockIds.has(block.blockId));

  const copyText = (text: string) => {
    void navigator.clipboard?.writeText(text);
  };

  return (
    <div className="document-viewer" data-content-hash={model.contentHash}>
      <div className="document-viewer-mode ws-toolbar" aria-label="Document mode">
        <div className="document-viewer-segmented" role="group" aria-label="Document mode">
          <button className="document-viewer-segment is-active" type="button" aria-pressed="true">
            view
          </button>
          <button className="document-viewer-segment" type="button" disabled title="Edit mode is deferred">
            edit
          </button>
        </div>
        <span className="ws-chip">Markdown</span>
      </div>
      {selectedBlocks.length > 0 ? (
        <div className="document-viewer-action-strip" data-selected-block-count={selectedBlocks.length}>
          <span>{selectedBlocks.length} block{selectedBlocks.length === 1 ? "" : "s"} selected</span>
          <button type="button" onClick={() => copyText(selectedBlocks.map((block) => block.plainText).join("\n\n"))}>
            Copy text
          </button>
          <button
            type="button"
            disabled={!selectedBlocks.some((block) => translationForBlock(overlay, model.contentHash, block.blockId)?.status === "ok")}
            onClick={() => copyText(selectedBlocks.map((block) => translationForBlock(overlay, model.contentHash, block.blockId)?.translatedMarkdown ?? block.plainText).join("\n\n"))}
          >
            Copy translation
          </button>
          <button type="button" onClick={() => copyText(selectedBlocks.map((block) => block.pathref).filter(Boolean).join("\n"))}>
            Copy pathref
          </button>
        </div>
      ) : null}
      <div className="document-viewer-scroll ws-doc-surface">
        {model.renderBlocks.map((block) => {
          const selected = selectedBlockIds.has(block.blockId);
          const translation = translationForBlock(overlay, model.contentHash, block.blockId);
          return (
            <section
              key={block.blockId}
              className={`document-block document-block-${block.kind}${selected ? " is-selected" : ""}`}
              data-document-block-id={block.blockId}
              data-document-block-kind={block.kind}
              data-pathref={block.pathref}
              onClick={() => {
                setSelectedBlockIds((current) => {
                  const next = new Set(current);
                  if (next.has(block.blockId)) {
                    next.delete(block.blockId);
                  } else {
                    next.add(block.blockId);
                  }
                  return next;
                });
              }}
            >
              {translation?.status === "ok" ? (
                <div className="document-block-translation" title={block.plainText}>
                  {translation.translatedMarkdown}
                </div>
              ) : (
                renderBlockNode(block, model.footnotes)
              )}
            </section>
          );
        })}
      </div>
    </div>
  );
}

function renderBlockNode(block: RenderBlock, footnotes: Record<string, string>): ReactNode {
  if (block.kind === "callout") {
    return (
      <aside className={`document-callout document-callout-${block.calloutKind ?? "note"}`}>
        <div className="document-callout-label">{block.calloutKind ?? "note"}</div>
        {renderChildrenWithoutCalloutMarker(block.node, footnotes)}
      </aside>
    );
  }
  return renderNode(block.node, `block-${block.blockId}`, footnotes);
}

function renderChildrenWithoutCalloutMarker(node: MarkdownNode, footnotes: Record<string, string>) {
  let removed = false;
  return node.children?.map((child, index) => {
    if (!removed && child.type === "paragraph") {
      removed = true;
      const cloned: MarkdownNode = {
        ...child,
        children: child.children?.map((grandchild, grandIndex) => {
          if (grandIndex === 0 && grandchild.type === "text") {
            return { ...grandchild, value: (grandchild.value ?? "").replace(/^\[![^\]]+\]\s*/i, "") };
          }
          return grandchild;
        }),
      };
      return renderNode(cloned, `callout-${index}`, footnotes);
    }
    return renderNode(child, `callout-${index}`, footnotes);
  });
}

function renderNode(node: MarkdownNode, key: string, footnotes: Record<string, string> = {}): ReactNode {
  switch (node.type) {
    case "heading": {
      const depth = Math.min(6, Math.max(1, (node as MarkdownNode & { depth?: number }).depth ?? 2));
      return createElement(`h${depth}`, { key }, renderChildren(node, key, footnotes));
    }
    case "paragraph":
      return <p key={key}>{renderChildren(node, key, footnotes)}</p>;
    case "text":
      return node.value ?? "";
    case "emphasis":
      return <em key={key}>{renderChildren(node, key, footnotes)}</em>;
    case "strong":
      return <strong key={key}>{renderChildren(node, key, footnotes)}</strong>;
    case "delete":
      return <del key={key}>{renderChildren(node, key, footnotes)}</del>;
    case "inlineCode":
      return <code key={key}>{node.value}</code>;
    case "code":
      return <pre key={key} className="ws-code-block"><code>{node.value}</code></pre>;
    case "break":
      return <br key={key} />;
    case "link": {
      const href = safeMarkdownLinkUrl(node.url);
      if (!href) {
        return <span key={key} className="document-link-inert">{renderChildren(node, key, footnotes)}</span>;
      }
      return <a key={key} href={href} target="_blank" rel="noreferrer">{renderChildren(node, key, footnotes)}</a>;
    }
    case "listItem":
      return <div key={key} className="document-list-item">{typeof node.checked === "boolean" ? <input type="checkbox" checked={node.checked} disabled readOnly /> : null}{renderChildren(node, key, footnotes)}</div>;
    case "blockquote":
      return <blockquote key={key}>{renderChildren(node, key, footnotes)}</blockquote>;
    case "table":
      return renderTable(node, key, footnotes);
    case "tableRow":
      return <tr key={key}>{renderChildren(node, key, footnotes)}</tr>;
    case "tableCell":
      return <td key={key}>{renderChildren(node, key, footnotes)}</td>;
    case "html":
      return null;
    case "footnoteReference": {
      const label = node.label ?? node.identifier ?? "footnote";
      const detail = footnotes[node.identifier ?? label] ?? label;
      return <sup key={key} className="document-footnote-ref" title={detail}>[{label}]</sup>;
    }
    case "footnoteDefinition":
      return <aside key={key} className="document-footnote" title={toString(node as never)}>{renderChildren(node, key, footnotes)}</aside>;
    default:
      return <div key={key}>{renderChildren(node, key, footnotes)}</div>;
  }
}

function renderTable(node: MarkdownNode, key: string, footnotes: Record<string, string>) {
  const rows = node.children ?? [];
  const [head, ...body] = rows;
  return (
    <table key={key}>
      {head ? <thead>{renderTableRow(head, `${key}-head`, true, footnotes)}</thead> : null}
      <tbody>{body.map((row, index) => renderTableRow(row, `${key}-body-${index}`, false, footnotes))}</tbody>
    </table>
  );
}

function renderTableRow(node: MarkdownNode, key: string, header: boolean, footnotes: Record<string, string>) {
  const Cell = header ? "th" : "td";
  return (
    <tr key={key}>
      {(node.children ?? []).map((cell, index) => (
        <Cell key={`${key}-cell-${index}`}>{renderChildren(cell, `${key}-cell-${index}`, footnotes)}</Cell>
      ))}
    </tr>
  );
}

function renderChildren(node: MarkdownNode, key: string, footnotes: Record<string, string> = {}) {
  return node.children?.map((child, index) => renderNode(child, `${key}-${index}`, footnotes));
}

export function safeMarkdownLinkUrl(url: string | undefined) {
  if (!url) {
    return undefined;
  }
  try {
    const parsed = new URL(url);
    return parsed.protocol === "http:" || parsed.protocol === "https:" || parsed.protocol === "mailto:"
      ? parsed.toString()
      : undefined;
  } catch {
    return undefined;
  }
}

function footnotesForTree(tree: MarkdownNode) {
  const footnotes: Record<string, string> = {};
  for (const child of tree.children ?? []) {
    if (child.type === "footnoteDefinition" && child.identifier) {
      footnotes[child.identifier] = toString(child as never).replace(/\s+/g, " ").trim();
    }
  }
  return footnotes;
}

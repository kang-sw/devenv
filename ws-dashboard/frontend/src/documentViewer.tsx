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
  ordered?: boolean | null;
  start?: number | null;
  spread?: boolean | null;
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

export type RenderBlock = DocumentBlock & {
  node: MarkdownNode;
  calloutKind?: string;
  listContext?: ListRenderContext;
};

type ListRenderContext = {
  ordered: boolean;
  start?: number;
  spread?: boolean;
  groupKey: string;
};

export type RenderUnit =
  | { type: "block"; block: RenderBlock }
  | { type: "list"; context: ListRenderContext; blocks: RenderBlock[] };

export type MarkdownDocumentModel = {
  contentHash: string;
  blocks: DocumentBlock[];
  renderBlocks: RenderBlock[];
  footnotes: Record<string, string>;
};


export type TranslationProviderStatus = {
  id: string;
  kind: string;
  label: string;
  configured: boolean;
  reachable: boolean;
  models: Array<{ id: string; label?: string | null }>;
  defaultModel?: string | null;
  error?: string | null;
};

export type DocumentTranslationApiResponse = {
  sourceContentHash: string;
  targetLocale: string;
  status: "completed" | "partial" | "failed" | string;
  cache: { hit: boolean; providerId: string; providerKind: string; model: string };
  blocks: Array<{
    blockId: string;
    translatedMarkdown?: string | null;
    translatedPlainText?: string | null;
    status: "ok" | "omitted" | "failed" | string;
    note?: string | null;
  }>;
  unmatched?: Array<{ ordinal: number; text: string; reason: string }>;
};

export type DocumentTranslationRequestPayload = ReturnType<typeof buildDocumentTranslationRequestPayload>;

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
  const pushBlock = (
    node: MarkdownNode,
    kind: DocumentBlockKind,
    calloutKind?: string,
    listContext?: ListRenderContext,
  ) => {
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
      listContext,
    });
  };

  for (const child of tree.children ?? []) {
    if (child.type === "list") {
      const listContext: ListRenderContext = {
        ordered: child.ordered === true,
        start: typeof child.start === "number" ? child.start : undefined,
        spread: child.spread === true,
        groupKey: [
          child.position?.start?.line ?? "",
          child.position?.end?.line ?? "",
          child.ordered === true ? "ol" : "ul",
          typeof child.start === "number" ? child.start : "",
        ].join(":"),
      };
      for (const item of child.children ?? []) {
        pushBlock(
          item,
          item.checked === true || item.checked === false ? "taskItem" : "listItem",
          undefined,
          listContext,
        );
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
    blocks: renderBlocks.map(({
      node: _node,
      calloutKind: _calloutKind,
      listContext: _listContext,
      ...block
    }) => block),
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

export function documentBlockVisibleText(
  block: DocumentBlock,
  overlay: DocumentTranslationOverlay | undefined,
  contentHash: string,
) {
  const translation = translationForBlock(overlay, contentHash, block.blockId);
  return translation?.status === "ok" ? translation.translatedMarkdown : block.plainText;
}

export function documentBlocksVisibleText(
  blocks: readonly DocumentBlock[],
  overlay: DocumentTranslationOverlay | undefined,
  contentHash: string,
) {
  return blocks.map((block) => documentBlockVisibleText(block, overlay, contentHash)).join("\n\n");
}

export function canCopyTranslatedBlocks(
  blocks: readonly DocumentBlock[],
  overlay: DocumentTranslationOverlay | undefined,
  contentHash: string,
) {
  return blocks.some((block) => translationForBlock(overlay, contentHash, block.blockId)?.status === "ok");
}

export function documentBlocksTranslatedText(
  blocks: readonly DocumentBlock[],
  overlay: DocumentTranslationOverlay | undefined,
  contentHash: string,
) {
  return blocks
    .map((block) => translationForBlock(overlay, contentHash, block.blockId)?.translatedMarkdown ?? block.plainText)
    .join("\n\n");
}

export function groupedMarkdownRenderUnits(renderBlocks: readonly RenderBlock[]): RenderUnit[] {
  const units: RenderUnit[] = [];
  for (const block of renderBlocks) {
    if (block.listContext) {
      const previous = units[units.length - 1];
      if (previous?.type === "list" && previous.context.groupKey === block.listContext.groupKey) {
        previous.blocks.push(block);
      } else {
        units.push({ type: "list", context: block.listContext, blocks: [block] });
      }
      continue;
    }
    units.push({ type: "block", block });
  }
  return units;
}

export function nextRailSelectedBlockIds(options: {
  current: Iterable<string>;
  blockId: string;
  blockOrdinal: number;
  lastSelectedOrdinal?: number;
  shiftKey?: boolean;
  blocks: readonly Pick<DocumentBlock, "blockId" | "ordinal">[];
}) {
  const next = new Set(options.current);
  if (options.shiftKey && typeof options.lastSelectedOrdinal === "number") {
    const start = Math.min(options.lastSelectedOrdinal, options.blockOrdinal);
    const end = Math.max(options.lastSelectedOrdinal, options.blockOrdinal);
    for (const block of options.blocks) {
      if (block.ordinal >= start && block.ordinal <= end) {
        next.add(block.blockId);
      }
    }
    return next;
  }
  if (next.has(options.blockId)) {
    next.delete(options.blockId);
  } else {
    next.add(options.blockId);
  }
  return next;
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
  const renderUnits = useMemo(() => groupedMarkdownRenderUnits(model.renderBlocks), [model.renderBlocks]);
  const [selectedBlockIds, setSelectedBlockIds] = useState<Set<string>>(() => new Set());
  const [lastSelectedOrdinal, setLastSelectedOrdinal] = useState<number | undefined>();
  const selectedBlocks = model.blocks.filter((block) => selectedBlockIds.has(block.blockId));

  const copyText = (text: string) => {
    void navigator.clipboard?.writeText(text);
  };
  const blocksForRailAction = (block: DocumentBlock) =>
    selectedBlockIds.has(block.blockId) && selectedBlocks.length > 0 ? selectedBlocks : [block];
  const toggleBlockFromRail = (block: DocumentBlock, event: { shiftKey?: boolean }) => {
    setSelectedBlockIds((current) => nextRailSelectedBlockIds({
      current,
      blockId: block.blockId,
      blockOrdinal: block.ordinal,
      lastSelectedOrdinal,
      shiftKey: event.shiftKey,
      blocks: model.blocks,
    }));
    setLastSelectedOrdinal(block.ordinal);
  };
  const renderRail = (block: RenderBlock, selected: boolean) => {
    const actionBlocks = blocksForRailAction(block);
    return (
      <div className="document-block-rail" aria-label={`Block actions for ${block.kind}`}>
        <button
          type="button"
          className="document-block-rail-select"
          aria-label={selected ? "Deselect block" : "Select block"}
          aria-pressed={selected}
          onClick={(event) => toggleBlockFromRail(block, event)}
        >
          <span aria-hidden="true">{selected ? "✓" : ""}</span>
        </button>
        <div className="document-block-rail-actions" aria-label="Copy block actions">
          <button
            type="button"
            title="Copy visible text"
            aria-label="Copy visible text"
            onClick={() => copyText(documentBlocksVisibleText(actionBlocks, overlay, model.contentHash))}
          >
            V
          </button>
          <button
            type="button"
            title="Copy translated text"
            aria-label="Copy translated text"
            disabled={!canCopyTranslatedBlocks(actionBlocks, overlay, model.contentHash)}
            onClick={() => copyText(documentBlocksTranslatedText(actionBlocks, overlay, model.contentHash))}
          >
            T
          </button>
          <button
            type="button"
            title="Copy pathref"
            aria-label="Copy pathref"
            disabled={!actionBlocks.some((actionBlock) => actionBlock.pathref)}
            onClick={() => copyText(actionBlocks.map((actionBlock) => actionBlock.pathref).filter(Boolean).join("\n"))}
          >
            @
          </button>
        </div>
      </div>
    );
  };
  const renderBlockContent = (block: RenderBlock) => {
    const translation = translationForBlock(overlay, model.contentHash, block.blockId);
    return translation?.status === "ok" ? (
      <div className="document-block-translation" title={block.plainText}>
        {renderTranslatedMarkdown(translation.translatedMarkdown)}
      </div>
    ) : (
      renderBlockNode(block, model.footnotes)
    );
  };
  const renderBlock = (block: RenderBlock) => {
    const selected = selectedBlockIds.has(block.blockId);
    return (
      <section
        key={block.blockId}
        className={`document-block document-block-${block.kind}${selected ? " is-selected" : ""}`}
        data-document-block-id={block.blockId}
        data-document-block-kind={block.kind}
        data-pathref={block.pathref}
      >
        {renderRail(block, selected)}
        <div className="document-block-body">{renderBlockContent(block)}</div>
      </section>
    );
  };
  const renderListItem = (block: RenderBlock) => {
    const selected = selectedBlockIds.has(block.blockId);
    const translation = translationForBlock(overlay, model.contentHash, block.blockId);
    return (
      <li
        key={block.blockId}
        className={`document-block document-block-${block.kind}${selected ? " is-selected" : ""}`}
        data-document-block-id={block.blockId}
        data-document-block-kind={block.kind}
        data-pathref={block.pathref}
      >
        {renderRail(block, selected)}
        <div className="document-block-body document-list-item-body">
          {translation?.status === "ok" ? (
            <div className="document-block-translation" title={block.plainText}>
              {renderTranslatedMarkdown(translation.translatedMarkdown)}
            </div>
          ) : (
            renderListItemContents(block.node, `block-${block.blockId}`, model.footnotes)
          )}
        </div>
      </li>
    );
  };

  return (
    <div className="document-viewer" data-content-hash={model.contentHash}>
      <div className="document-viewer-scroll ws-doc-surface">
        {renderUnits.map((unit) => {
          if (unit.type === "list") {
            const ListTag = unit.context.ordered ? "ol" : "ul";
            return (
              <ListTag
                key={unit.context.groupKey}
                className={`document-list document-list-${unit.context.ordered ? "ordered" : "unordered"}${unit.context.spread ? " is-spread" : ""}`}
                start={unit.context.ordered && unit.context.start && unit.context.start !== 1 ? unit.context.start : undefined}
              >
                {unit.blocks.map(renderListItem)}
              </ListTag>
            );
          }
          return renderBlock(unit.block);
        })}
      </div>
    </div>
  );
}

function renderTranslatedMarkdown(markdown: string): ReactNode {
  const tree = parser.parse(markdown) as MarkdownNode;
  const footnotes = footnotesForTree(tree);
  return tree.children?.map((child, index) => renderNode(child, `translated-${index}`, footnotes));
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

function renderListItemContents(node: MarkdownNode, key: string, footnotes: Record<string, string>) {
  const children = (node.children ?? []).map((child, index) => renderNode(child, `${key}-${index}`, footnotes));
  if (typeof node.checked !== "boolean") {
    return children;
  }
  return [
    <input key={`${key}-checkbox`} type="checkbox" checked={node.checked} disabled readOnly />,
    ...children,
  ];
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
    case "list": {
      const ListTag = node.ordered === true ? "ol" : "ul";
      return (
        <ListTag
          key={key}
          className={`document-list document-list-${node.ordered === true ? "ordered" : "unordered"}${node.spread ? " is-spread" : ""}`}
          start={node.ordered === true && node.start && node.start !== 1 ? node.start : undefined}
        >
          {renderChildren(node, key, footnotes)}
        </ListTag>
      );
    }
    case "listItem":
      return (
        <li key={key} className={typeof node.checked === "boolean" ? "document-nested-task-item" : undefined}>
          {typeof node.checked === "boolean" ? <input type="checkbox" checked={node.checked} disabled readOnly /> : null}
          {renderChildren(node, key, footnotes)}
        </li>
      );
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


export function buildDocumentTranslationRequestPayload(options: {
  markdown: string;
  workRootId: string;
  path: string;
  title?: string;
  targetLocale?: string;
  providerId?: string;
  model?: string;
}) {
  const model = deriveMarkdownDocumentModel(options.markdown, { path: options.path });
  return {
    source: {
      kind: "workRootFile",
      workRootId: options.workRootId,
      path: options.path,
      contentHash: model.contentHash,
      format: "markdown",
      title: options.title,
    },
    provider: options.providerId ? { id: options.providerId, model: options.model } : undefined,
    locale: { source: null, target: options.targetLocale ?? "ko" },
    blocks: model.blocks,
    cachePolicy: "preferCached",
  };
}

export function overlayFromTranslationResponse(
  response: DocumentTranslationApiResponse,
): DocumentTranslationOverlay {
  return {
    contentHash: response.sourceContentHash,
    blocks: Object.fromEntries(
      response.blocks.map((block) => [
        buildOverlayKey(response.sourceContentHash, block.blockId),
        {
          translatedMarkdown: block.translatedMarkdown ?? "",
          status: block.status === "ok" ? "ok" : block.status === "failed" ? "failed" : "pending",
        },
      ]),
    ),
  };
}

export async function fetchTranslationProviders(): Promise<{ providers: TranslationProviderStatus[] }> {
  const response = await fetch("/api/dashboard/document-translation/providers", {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await response.text());
  }
  return (await response.json()) as { providers: TranslationProviderStatus[] };
}

export async function requestDocumentTranslation(
  payload: DocumentTranslationRequestPayload,
): Promise<DocumentTranslationApiResponse> {
  const response = await fetch("/api/dashboard/document-translation/translate", {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    const body = await response.json().catch(() => null) as { error?: string } | null;
    throw new Error(body?.error ?? "translation request failed");
  }
  return (await response.json()) as DocumentTranslationApiResponse;
}

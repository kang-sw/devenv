import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import {
  buildDocumentTranslationRequestPayload,
  buildOverlayKey,
  deriveMarkdownDocumentModel,
  documentBlocksTranslatedText,
  documentBlocksVisibleText,
  DocumentViewer,
  groupedMarkdownRenderUnits,
  nextRailSelectedBlockIds,
  canCopyTranslatedBlocks,
  isMarkdownDocumentSource,
  localDocumentContentHash,
  overlayFromTranslationResponse,
  safeMarkdownLinkUrl,
  translationForBlock,
  workRootRelativePathForPathref,
  type DocumentTranslationOverlay,
} from "./documentViewer.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

function assert(condition: unknown, label: string) {
  if (!condition) {
    throw new Error(label);
  }
}

function assertDeepEqual(actual: unknown, expected: unknown, label: string) {
  const actualJson = JSON.stringify(actual);
  const expectedJson = JSON.stringify(expected);
  if (actualJson !== expectedJson) {
    throw new Error(`${label}: expected ${expectedJson}, got ${actualJson}`);
  }
}

const markdown = [
  "# Title",
  "",
  "First prose line",
  "soft continuation line[^one]",
  "",
  "- first item",
  "- [x] done item",
  "",
  "> [!note] Heads up",
  "> Callout body",
  "",
  "| Name | Value |",
  "| --- | --- |",
  "| Alpha | 1 |",
  "",
  "<script>window.evil = true</script>",
  "",
  "```ts",
  "const host = '/Users/example/private';",
  "```",
  "",
  "[^one]: Footnote detail",
].join("\n");

const model = deriveMarkdownDocumentModel(markdown, { path: "docs/readme.md" });

assertEqual(model.blocks[0]?.kind, "heading", "heading block is classified");
assertEqual(model.blocks[1]?.kind, "paragraph", "paragraph block is classified");
assertEqual(
  model.blocks[1]?.plainText,
  "First prose line soft continuation line",
  "soft line breaks in prose stay in one paragraph block",
);
assertEqual(model.blocks[2]?.kind, "listItem", "ordinary list item is its own block");
assertEqual(model.blocks[3]?.kind, "taskItem", "GFM task item is classified separately");
assertEqual(model.blocks[4]?.kind, "callout", "Obsidian callout block is classified");
assertEqual(
  model.blocks[4]?.plainText,
  "Heads up Callout body",
  "callout marker is omitted from plain visible text",
);
assertEqual(model.blocks[5]?.kind, "table", "GFM table is classified");
assertEqual(model.blocks[6]?.kind, "code", "fenced code block is classified");
assertEqual(model.blocks[6]?.translatable, false, "code block is non-translatable");
assertEqual(
  model.footnotes.one,
  "Footnote detail",
  "GFM footnote definitions are available for hover affordances",
);
assertDeepEqual(
  model.blocks.map((block) => block.lineStart),
  [1, 3, 6, 7, 9, 12, 18, 22],
  "block line starts come from markdown AST positions",
);
assertEqual(
  model.blocks[1]?.pathref,
  "@docs/readme.md#L3-L4",
  "pathref uses workRoot-relative line ranges",
);
assertEqual(model.blocks[2]?.pathref, "@docs/readme.md#L6", "single-line pathref is compact");
assert(
  model.blocks.every((block) => !block.pathref?.includes("/Users/")),
  "pathrefs do not include raw host paths from content",
);
assert(
  model.blocks.every((block) => !block.plainText.includes("window.evil")),
  "raw HTML nodes are ignored instead of trusted as visible text",
);

const sameModel = deriveMarkdownDocumentModel(markdown, { path: "docs/readme.md" });
assertDeepEqual(
  sameModel.blocks.map((block) => block.blockId),
  model.blocks.map((block) => block.blockId),
  "block ids are stable for unchanged content",
);
const changedModel = deriveMarkdownDocumentModel(markdown.replace("Alpha", "Beta"), {
  path: "docs/readme.md",
});
assert(
  changedModel.blocks.some((block, index) => block.blockId !== model.blocks[index]?.blockId),
  "block ids include normalized content material inside a content namespace",
);

const contentHash = localDocumentContentHash(markdown);
const overlay: DocumentTranslationOverlay = {
  contentHash,
  blocks: {
    [buildOverlayKey(contentHash, model.blocks[1].blockId)]: {
      translatedMarkdown: "번역된 문단",
      status: "ok",
    },
  },
};
assertEqual(
  translationForBlock(overlay, contentHash, model.blocks[1].blockId)?.translatedMarkdown,
  "번역된 문단",
  "translation overlay matches by content hash plus block id key",
);
assertEqual(
  translationForBlock(overlay, "fnv1a32:00000000", model.blocks[1].blockId),
  undefined,
  "translation overlay ignores stale content hashes",
);

assertEqual(
  isMarkdownDocumentSource({ extension: "md", languageHint: null }),
  true,
  "md extension selects markdown viewer",
);
assertEqual(
  isMarkdownDocumentSource({ extension: "html", languageHint: "html", path: "index.html" }),
  false,
  "html hint does not select markdown viewer",
);

assertEqual(
  workRootRelativePathForPathref("/Users/example/project/docs/readme.md"),
  undefined,
  "absolute host paths are rejected before pathref generation",
);
assertEqual(
  workRootRelativePathForPathref("C:/Users/example/project/docs/readme.md"),
  undefined,
  "Windows absolute paths are rejected before pathref generation",
);
assertEqual(
  workRootRelativePathForPathref("docs/readme.md"),
  "docs/readme.md",
  "workRoot-relative pathrefs remain available",
);
assertEqual(
  deriveMarkdownDocumentModel("# Title\n", { path: "/Users/example/private.md" }).blocks[0]?.pathref,
  undefined,
  "absolute caller paths do not become copied pathrefs",
);

assertEqual(
  safeMarkdownLinkUrl("https://example.com/path"),
  "https://example.com/path",
  "https markdown links remain active",
);
assertEqual(
  safeMarkdownLinkUrl("javascript:alert(1)"),
  undefined,
  "active javascript markdown links are rendered inert",
);
assertEqual(
  safeMarkdownLinkUrl("./relative.md"),
  undefined,
  "relative markdown links are inert until dashboard-safe navigation exists",
);

const requestPayload = buildDocumentTranslationRequestPayload({
  markdown: "# Translate me\n\nHello world\n",
  workRootId: "root-local-abc",
  path: "docs/translate.md",
  title: "translate.md",
  targetLocale: "ko",
});
assertEqual(requestPayload.source.kind, "workRootFile", "translation payload source is a workRoot file");
assertEqual(requestPayload.source.workRootId, "root-local-abc", "translation payload uses opaque workRoot id");
assertEqual(requestPayload.source.path, "docs/translate.md", "translation payload uses workRoot-relative path");
assertEqual(requestPayload.locale.target, "ko", "translation payload includes explicit target locale");
assert(
  requestPayload.blocks.length >= 2 && requestPayload.blocks.every((block) => block.blockId),
  "translation payload includes whole document block context",
);
assert(!JSON.stringify(requestPayload).includes("/Users/"), "translation payload does not include host paths");

const responseOverlay = overlayFromTranslationResponse({
  sourceContentHash: requestPayload.source.contentHash,
  targetLocale: "ko",
  status: "completed",
  cache: { hit: false, providerId: "test", providerKind: "llmOpenAICompatible", model: "fake" },
  blocks: [
    {
      blockId: requestPayload.blocks[0].blockId,
      translatedMarkdown: "번역",
      translatedPlainText: "번역",
      status: "ok",
    },
  ],
});
assertEqual(
  translationForBlock(responseOverlay, requestPayload.source.contentHash, requestPayload.blocks[0].blockId)
    ?.translatedMarkdown,
  "번역",
  "daemon translation response maps into content-hash overlay blocks",
);

const selectedBlocksForCopy = requestPayload.blocks.slice(0, 1);
assertEqual(
  canCopyTranslatedBlocks(selectedBlocksForCopy, undefined, requestPayload.source.contentHash),
  false,
  "translated copy is unavailable without an ok overlay",
);
assertEqual(
  documentBlocksVisibleText(selectedBlocksForCopy, undefined, requestPayload.source.contentHash),
  selectedBlocksForCopy[0].plainText,
  "visible copy uses original text without overlay",
);
assertEqual(
  canCopyTranslatedBlocks(selectedBlocksForCopy, responseOverlay, requestPayload.source.contentHash),
  true,
  "translated copy is enabled with an ok overlay",
);
assertEqual(
  documentBlocksVisibleText(selectedBlocksForCopy, responseOverlay, requestPayload.source.contentHash),
  "번역",
  "visible copy uses translated text when overlay is active",
);
assertEqual(
  documentBlocksTranslatedText(selectedBlocksForCopy, responseOverlay, requestPayload.source.contentHash),
  "번역",
  "translated copy uses overlay text",
);

const listPolishMarkdown = [
  "Intro with `inline code` token.",
  "",
  "- Alpha",
  "  - Nested alpha",
  "- [ ] Todo item",
  "",
  "5. Fifth",
  "6. Sixth",
  "",
  "<aside>ignored html</aside>",
].join("\n");
const listPolishModel = deriveMarkdownDocumentModel(listPolishMarkdown, { path: "docs/list.md" });
const listUnits = groupedMarkdownRenderUnits(listPolishModel.renderBlocks);
assertEqual(listUnits[1]?.type, "list", "adjacent unordered list blocks share one visual list unit");
if (listUnits[1]?.type === "list") {
  assertEqual(listUnits[1].context.ordered, false, "unordered list unit keeps unordered context");
  assertEqual(listUnits[1].blocks.length, 2, "unordered visual list preserves per-item block identities");
  assertDeepEqual(
    listUnits[1].blocks.map((block) => block.kind),
    ["listItem", "taskItem"],
    "ordinary and task list items remain independently addressable blocks",
  );
}
assertEqual(listUnits[2]?.type, "list", "ordered list blocks share one visual list unit");
if (listUnits[2]?.type === "list") {
  assertEqual(listUnits[2].context.ordered, true, "ordered list unit keeps ordered context");
  assertEqual(listUnits[2].context.start, 5, "ordered list unit preserves non-default start number");
  assertEqual(listUnits[2].blocks.length, 2, "ordered list preserves each item block identity");
}

const listHtml = renderToStaticMarkup(createElement(DocumentViewer, {
  markdown: listPolishMarkdown,
  path: "docs/list.md",
}));
assert(listHtml.includes("<code>inline code</code>"), "inline code renders as semantic code span");
assert(listHtml.includes("<ul") && listHtml.includes("<li"), "unordered list renders semantic ul/li markup");
assert(listHtml.includes("Nested alpha"), "nested list content renders inside semantic list item");
assert(listHtml.includes("type=\"checkbox\""), "task list renders a disabled checkbox");
assert(listHtml.includes("<ol") && listHtml.includes("start=\"5\""), "ordered list renders semantic ol with non-default start");
assert(!listHtml.includes("ignored html"), "raw HTML remains inert in rendered markup");
assert(listHtml.includes("document-block-rail-select"), "selection control is exposed through the rail");
assert(!listHtml.includes("document-block-rail-actions"), "rail no longer renders per-block copy actions");
assert(!listHtml.includes(">V<") && !listHtml.includes(">T<") && !listHtml.includes(">@<"), "rail no longer renders V/T/@ glyph labels");
assert(!listHtml.includes("✓"), "rail selected-state glyph text is absent from static markup");
assert(!listHtml.includes("Copy visible text"), "copy actions are absent until the selected-block toolbar is shown");
assert(!listHtml.includes("document-viewer-action-strip"), "block actions no longer live in the legacy body-click selection strip");

const visibleCopyMarkdown = [
  "# Copy Format",
  "",
  "Paragraph line",
  "continuation",
  "",
  "- bullet one",
  "  - nested bullet",
  "- [x] done task",
  "",
  "3. third",
  "4. fourth",
  "",
  "```ts",
  "const value = 1;",
  "  console.log(value);",
  "```",
].join("\n");
const visibleCopyModel = deriveMarkdownDocumentModel(visibleCopyMarkdown, { path: "docs/copy.md" });
assertDeepEqual(
  visibleCopyModel.blocks.map((block) => block.kind),
  ["heading", "paragraph", "listItem", "taskItem", "listItem", "listItem", "code"],
  "visible-copy fixture has heading, paragraph, unordered/task list, ordered list, and code blocks",
);
assertEqual(
  documentBlocksVisibleText(visibleCopyModel.blocks, undefined, visibleCopyModel.contentHash),
  [
    "Copy Format",
    "",
    "Paragraph line continuation",
    "",
    "- bullet one",
    "  - nested bullet",
    "- [x] done task",
    "",
    "3. third",
    "4. fourth",
    "",
    "```ts",
    "const value = 1;",
    "  console.log(value);",
    "```",
  ].join("\n"),
  "visible copy preserves list markers, compact adjacent list items, ordered numbering, and fenced code whitespace",
);
assertEqual(
  documentBlocksVisibleText(visibleCopyModel.blocks.slice(2, 4), undefined, visibleCopyModel.contentHash),
  ["- bullet one", "  - nested bullet", "- [x] done task"].join("\n"),
  "adjacent unordered and task list items copy compactly with one newline boundary",
);
assertEqual(
  documentBlocksVisibleText(visibleCopyModel.blocks.slice(4, 6), undefined, visibleCopyModel.contentHash),
  ["3. third", "4. fourth"].join("\n"),
  "adjacent ordered list items copy compactly with source numbering",
);
const visibleCopyOverlay: DocumentTranslationOverlay = {
  contentHash: visibleCopyModel.contentHash,
  blocks: {
    [buildOverlayKey(visibleCopyModel.contentHash, visibleCopyModel.blocks[2].blockId)]: {
      translatedMarkdown: "번역된 항목",
      status: "ok",
    },
  },
};
assertEqual(
  documentBlocksVisibleText(visibleCopyModel.blocks.slice(2, 4), visibleCopyOverlay, visibleCopyModel.contentHash),
  ["번역된 항목", "- [x] done task"].join("\n"),
  "visible copy uses translated markdown when present without inventing list markers",
);

const railBlocks = listPolishModel.blocks.slice(0, 4);
const selectedFirst = nextRailSelectedBlockIds({
  current: [],
  blockId: railBlocks[1].blockId,
  blockOrdinal: railBlocks[1].ordinal,
  blocks: listPolishModel.blocks,
});
assert(selectedFirst.has(railBlocks[1].blockId), "rail click selects the targeted block");
const rangeSelected = nextRailSelectedBlockIds({
  current: selectedFirst,
  blockId: railBlocks[3].blockId,
  blockOrdinal: railBlocks[3].ordinal,
  lastSelectedOrdinal: railBlocks[1].ordinal,
  shiftKey: true,
  blocks: listPolishModel.blocks,
});
assert(
  railBlocks.slice(1, 4).every((block) => rangeSelected.has(block.blockId)),
  "shift-clicking the rail selects the ordinal block range",
);
const toggledOff = nextRailSelectedBlockIds({
  current: selectedFirst,
  blockId: railBlocks[1].blockId,
  blockOrdinal: railBlocks[1].ordinal,
  blocks: listPolishModel.blocks,
});
assert(!toggledOff.has(railBlocks[1].blockId), "rail click toggles an already-selected block off");

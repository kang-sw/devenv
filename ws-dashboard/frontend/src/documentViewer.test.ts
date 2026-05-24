import {
  buildDocumentTranslationRequestPayload,
  buildOverlayKey,
  deriveMarkdownDocumentModel,
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

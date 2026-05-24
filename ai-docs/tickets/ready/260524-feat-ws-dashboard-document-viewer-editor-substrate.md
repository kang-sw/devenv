---
title: Add dashboard document viewer and editor mode substrate
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260514-research-ws-web-dashboard-direction: document viewer, translation, and mention substrate direction
  260523-feat-ws-dashboard-readonly-file-pane-restore: existing read-only pane descriptor replay should evolve into document pane restore
  260523-research-ws-dashboard-persistable-ui-state-map: future persisted UI state must treat document descriptors and overlays as logical state
spec:
  - 260516-ws-web-dashboard-readonly-file-api
  - 260516-ws-web-dashboard-readonly-text-pane
  - 260516-ws-web-dashboard-file-open-placement-policy
  - 260522-ws-dashboard-activity-console-transcript-expansion
  - 260524-ws-dashboard-document-viewer-mode
  - 260524-ws-dashboard-document-translation-overlay
  - 260524-ws-dashboard-document-edit-save-fanout
plans:
  phase-1: 2026-05/24-260524-feat-ws-dashboard-document-viewer-editor-substrate
  phase-2: 2026-05/24-260524-feat-ws-dashboard-document-viewer-editor-substrate-phase-2
related-mental-model:
  - ws-web-dashboard
---

# Add dashboard document viewer and editor mode substrate

## Background

The current read-only file pane renders all previewable text as preformatted raw
text under a workbench surface kind named `editor`. Markdown, Activity
transcripts, future agent-specific document surfaces, translations, mentions,
and rich document formats need a reusable document viewer substrate instead of
one-off renderers.

The accepted direction separates document presentation from mutation:
format-aware read-only viewing belongs to a document viewer mode, while file
editing belongs to a raw-text edit mode. Editing should not be forced into the
first markdown viewer implementation. A single document pane should own one
file attachment and expose an in-pane `view | edit` segmented control, such as
eye and pen icons, rather than opening separate tabs solely to switch between
read-only and edit presentations.

## Decisions

- Treat a document pane as one attachment for a workRoot-relative source. The
  pane owns a panel-local `mode: "view" | "edit"` state.
- View mode is format-aware and read-only. Markdown rendering, paragraph block
  actions, translation overlays, mention/pathref copying, future HTML, draw.io,
  Excalidraw, image, and other provider-specific renderers belong to view mode
  or provider-specific viewer surfaces.
- Edit mode is raw text editing, initially suitable for CodeMirror 6 or an
  equivalent browser-native text editor. Markdown files in edit mode are raw
  markdown text, not a rich markdown editor.
- Keep backend read and write surfaces separate. Reads return content,
  metadata, renderer hints, edit capability, and a content hash. Writes use
  optimistic concurrency through a base content hash and return the new content
  hash.
- Markdown rendering should use a real parser/render pipeline rather than a
  hand-rolled parser. Prefer the `unified`/`remark`/`rehype` ecosystem so the
  renderer and document block model can share one markdown AST. Direct raw HTML
  rendering is not part of this ticket; safe HTML support remains an
  implementation gap for a later ticket.
- Markdown view mode should follow common Obsidian rendering conventions where
  they fit a first implementation: GFM tables and task lists should render as
  polished document blocks, and `> [!note]`-style callouts should render as
  callouts. Footnotes/footer content may be surfaced through hover/tooltip
  affordances, but a full footer section is deferred.
- The reusable document model should expose `DocumentBlock`/translation-unit
  data rather than only rendered React nodes. Soft line breaks inside ordinary
  text stay one block; each list item is a block; headings, paragraphs,
  callout body blocks, and similar markdown units retain line ranges when
  available. Fenced code and other non-prose blocks may be omitted from
  translation or treated as non-translatable blocks.
- Block identity is stable within a `contentHash` namespace, not across edits.
  A block id should be derived from ordinal, kind, line range, and normalized
  content hash material so overlays can match deterministically without
  pretending to be an edit-diff algorithm.
- Path references copy as `@<workRoot-relative-path>#L<line-range>`, using
  `#L<line>` for a single line and `#L<start>-L<end>` for ranges. Absolute host
  paths must not appear in copied pathrefs.
- Translation is a whole-document viewer feature over immutable content hashes,
  not mutation of source files. The frontend builds document blocks and sends
  the whole block set to the daemon so LLM providers can preserve translation
  consistency with full context.
- Translation providers should be represented as a union. The first working
  provider is an LLM OpenAI-compatible provider, with local Ollama at
  `http://localhost:11434/v1` as the dogfood target. The shape should leave room
  for future non-LLM translation APIs without forcing them into the LLM prompt
  contract.
- The daemon owns translation provider configuration, model discovery,
  prompting, bounded model-output parsing, and SHA256/content-hash-based
  caching. The frontend does not cache translations as source of truth.
- LLM prompts must preserve a block-id roundtrip invariant: the request sends
  `blockId + content` pairs and the model is expected to return
  `blockId + translatedContent` pairs. Missing, duplicate, unknown, or
  unparsable block ids become bounded block-level failure states; raw model
  output is not forwarded to the frontend as-is.
- Markdown view mode includes a pane-local translation toggle next to the
  view/edit control. When enabled, opening or focusing the pane requests whole-
  document translation asynchronously. Completed blocks render translated
  markdown as an overlay; hovering a translated block temporarily shows the
  original block text.
- Document blocks are selectable as block ranges, not arbitrary rich-text
  selections in the first pass. Selected blocks show a compact action strip with
  current-visible copy, translated copy when available or pending, and pathref
  copy actions.
- Document freshness should use a per-workRoot document event stream rather
  than one stream per panel. File watching is a freshness optimization; focus
  or visibility re-read plus content-hash checks remain the correctness
  fallback.
- Same-file multi-pane scenarios must route saved-content updates by document
  source identity, not pane identity. A save in one pane should fan out to other
  clean panes for the same `workRootId + path`; dirty edit panes must not be
  overwritten silently.

## API Sketch

Document reads should expose source identity, content hash, renderer hints, and
capability:

```ts
type DocumentReadResponse = {
  documentId: string;
  source: {
    kind: "workRootFile";
    workRootId: string;
    path: string;
  };
  contentHash: string;
  mediaType: string;
  languageHint: string | null;
  rendererKind:
    | "rawText"
    | "markdown"
    | "html"
    | "excalidraw"
    | "drawio"
    | "image"
    | "unsupported";
  access: {
    readable: boolean;
    editable: boolean;
    reason?: string;
  };
  content: string;
  sizeBytes: number;
};
```

Document writes should preserve optimistic concurrency:

```ts
type DocumentWriteRequest = {
  source: {
    kind: "workRootFile";
    workRootId: string;
    path: string;
  };
  baseContentHash: string;
  content: string;
};

type DocumentWriteResponse = {
  contentHash: string;
  sizeBytes: number;
  savedAtMs: number;
};
```

Translation providers, status, and requests should be shaped so the daemon owns
configuration, model discovery, and cache behavior:

```ts
type TranslationProviderConfig =
  | {
      kind: "llmOpenAICompatible";
      id: string;
      label: string;
      baseUrl: string;
      apiKey?: string;
      defaultModel?: string;
      timeoutMs?: number;
    }
  | {
      kind: "genericTranslationApi";
      id: string;
      label: string;
      endpoint: string;
      timeoutMs?: number;
    };

type TranslationProviderStatus = {
  providers: Array<{
    id: string;
    kind: TranslationProviderConfig["kind"];
    label: string;
    configured: boolean;
    reachable: boolean;
    models?: Array<{ id: string; label?: string }>;
    defaultModel?: string;
    error?: string;
  }>;
};

type DocumentBlock = {
  blockId: string;
  ordinal: number;
  kind:
    | "paragraph"
    | "heading"
    | "listItem"
    | "callout"
    | "code"
    | "table"
    | "taskItem"
    | string;
  markdown: string;
  plainText: string;
  lineStart?: number;
  lineEnd?: number;
  pathref?: string;
  translatable: boolean;
};

type DocumentTranslationRequest = {
  source: {
    kind: "workRootFile" | "activityTranscript" | "inlineMarkdown";
    workRootId?: string;
    path?: string;
    contentHash: string;
    format: "markdown" | "text";
    title?: string;
  };
  provider: {
    id: string;
    model?: string;
  };
  locale: {
    source?: string | null;
    target: string;
  };
  blocks: DocumentBlock[];
  requestedBlockIds?: string[];
  cachePolicy?: "preferCached" | "refresh";
};

type DocumentTranslationResponse = {
  sourceContentHash: string;
  targetLocale: string;
  status: "completed" | "partial" | "failed";
  cache: {
    hit: boolean;
    providerId: string;
    providerKind: string;
    model: string;
    providerConfigVersion: string;
    blockModelVersion: string;
    promptVersion: string;
  };
  blocks: Array<{
    blockId: string;
    translatedMarkdown: string;
    translatedPlainText?: string;
    status: "ok" | "omitted" | "failed";
    note?: string;
  }>;
  unmatched?: Array<{
    ordinal: number;
    text: string;
    reason: string;
  }>;
};
```

Document freshness events should be scoped by workRoot and keyed by document
source:

```ts
type DocumentEvent =
  | {
      type: "document.contentChanged";
      source: {
        workRootId: string;
        path: string;
      };
      contentHash: string;
      changedAtMs: number;
      originPaneId?: string;
    }
  | {
      type: "document.watchInvalidated";
      source: {
        workRootId: string;
        path?: string;
      };
      reason: "watcherError" | "tooManyFiles" | "workRootUnavailable";
    };
```

## Phases

### Phase 1: Markdown viewer and block interaction

Introduce a reusable markdown document viewer module for read-only rendering
from workRoot-relative file content. Use a real markdown AST pipeline, derive
`DocumentBlock` entries with stable-in-content block ids, line ranges, pathrefs,
translatability flags, and renderer metadata, then render markdown files through
that viewer instead of the current preformatted text path.

The first renderer should support polished GFM table/task-list rendering,
Obsidian-style callouts for `> [!note]` and adjacent callout kinds, and hover
footnote/footer affordances without implementing a full footer section. It
should keep raw HTML disabled or safely ignored, with an explicit implementation
gap for future sanitized/sandboxed HTML handling.

Markdown view mode should provide block-level selection and a floating action
strip for current-visible copy, translated copy placeholder state, and
workRoot-relative pathref copy. The viewer API must accept translation overlay
data keyed by `contentHash + blockId`, but this phase may use fixture or local
overlay data rather than a real daemon translation provider.

The first viewer should preserve preview/pinned pane identity, workbench
placement policy, restored read-only pane descriptors, owner-authenticated file
reads, and scroll containment inside the pane. Activity Console markdown may
remain plain text unless reusing the viewer does not force transcript-specific
UI decisions.

Deferred scope: daemon translation provider calls, provider configuration,
daemon translation cache, raw-text editing, saving files, CodeMirror
integration, raw HTML rendering, HTML/diagram/image renderers, Activity Console
renderer migration when it would widen the slice, and arbitrary native text
selection semantics beyond block-range selection.

Verification should include pure markdown block/pathref/selection/overlay model
tests and browser-level evidence that markdown renders in the daemon-served
file pane with GFM/task/callout behavior, block actions, preview/pin behavior,
and scroll containment intact.

### Result (b8fb8cb) - 2026-05-24

Implemented a reusable Markdown document viewer for read-only file panes while
preserving existing read-only pane identity, preview/pinned behavior,
workbench placement, descriptor restore compatibility, and scroll containment.
Markdown files now render through an AST-based viewer with GFM tables/task
items, Obsidian-style callouts, footnote hover data, block derivation, local
content hashing, block selection, copy actions, workRoot-relative pathrefs, and
a declarative translation-overlay shape.

Review cycle follow-up `4173dd6` tightened Markdown link handling and pathref
generation: unsafe link schemes render inert, relative Markdown links remain
inactive until a dashboard-safe navigation model exists, and copyable pathrefs
reject absolute, Windows-drive, backslash, home-relative, empty, or traversal
inputs before browser exposure.

Verification passed:

- `npm run test:document-viewer`
- `npm run test:work-root-files`
- `npm run build`
- `npm run test:browser`

Deferred scope remains Phase 2 daemon translation providers/cache/prompting
and Phase 3 raw edit/save/document events.

### Phase 2: Translation provider MVP and overlay UX

Add daemon-owned document translation support for markdown view mode. The
frontend sends whole-document `DocumentBlock` context to the daemon when a pane-
local translation toggle is enabled; translation is always requested at document
scope for consistency even when UI actions operate on selected blocks.

Implement the provider union and make `llmOpenAICompatible` the first supported
provider, dogfooding local Ollama through the OpenAI-compatible
`http://localhost:11434/v1` shape. The daemon should expose provider status and
model discovery by probing `/v1/models`; no default model is required when the
provider cannot report one. Future generic translation APIs should remain type
room unless their adapter contract is explicitly implemented.

The daemon owns provider configuration, model selection, prompt construction,
prompt-versioning, bounded model-output parsing, block-id validation, and
SHA256/content-hash-based translation caching. Cache keys must include source
content hash, target locale, provider id/kind, model, provider config version,
block model version, and prompt version. Raw model output must not be sent to
the browser; JSON parse failures, missing block ids, duplicate block ids,
unknown block ids, or omitted blocks produce bounded failure states.

The markdown toolbar should show a translation icon toggle next to the
panel-local view/edit control in markdown view mode. When enabled, a pane opens
or focuses by requesting translation asynchronously. As block results arrive,
the overlay replaces each block with translated markdown. Hovering a translated
block temporarily renders the original block as a local source peek. Selected
blocks expose current-visible copy, translated copy when available, and pathref
copy actions.

Deferred scope: provider configuration UI, non-LLM translation API adapters,
streaming partial LLM tokens, terminology management, user-editable translation
memory, cross-document translation consistency, and Activity Console translation
unless it reuses the same viewer without widening the slice.

Verification should cover provider status/model probing, cache-key behavior,
block-id roundtrip validation, bounded failure handling, whole-document request
shape, translation toggle behavior, original-on-hover source peek, selected
block copy actions, and browser-level evidence against a daemon-served markdown
pane. When local Ollama is available, dogfood evidence should record the
provider/model used without depending on private prompt or raw model output.

### Result (a4cdbff) - 2026-05-24

Implemented daemon-owned document translation support for Markdown panes. The
daemon now exposes OpenAI-compatible provider status/model probing and
whole-document translation routes, uses environment-backed provider
configuration, validates block-id roundtrips, bounds parse and provider
failures, avoids raw provider output in browser responses, and caches
translations with source, provider, model, locale, prompt, and block-model
dimensions. Frontend Markdown panes now expose a command-routed translation
toggle, request whole-document translation for the current content hash, render
translated block overlays, preserve original-on-hover behavior, and keep
selected current/translated/pathref copy actions.

Review cycle follow-ups `9ac425e` and `51d128a` tightened cache-key dimensions,
unknown provider block-id handling, raw-output leak tests, selected-copy
coverage, and dashboard command-dispatch routing for the visible translation
toggle.

Verification passed:

- `cargo test -p ws-dashboard-daemon`
- `npm run test:commands`
- `npm run test:document-viewer`
- `npm run test:work-root-files`
- `npm run build`
- `npm run test:browser`

Browser evidence covered daemon-served Markdown-pane translation toggle behavior
with configured-unavailable state. Deterministic successful overlay behavior is
covered by backend fake-provider tests and frontend overlay/copy helper tests.

Deferred scope remains Phase 3 raw edit/save/document events, provider
configuration UI, non-LLM provider implementation, streaming token UI, Activity
Console translation, and durable cache persistence.

### Phase 3: Raw text edit mode, save fan-out, and document events

Add raw text edit mode as an in-pane view/edit segmented control on document
panes. Edit mode should use CodeMirror 6 or an equivalent browser-native text
editor foundation, preserve a draft, expose dirty/save/revert/error state, and
save through a backend write route that requires the caller's base content
hash. A content-hash mismatch returns a conflict state instead of overwriting
another writer.

After a successful save, document updates should be keyed by
`workRootId + path`, then update all clean panes for the same source. Dirty
edit panes for that source must remain dirty and show that a newer saved
version exists rather than silently replacing the draft. Translation overlays,
block ids, and pathref line metadata become stale when the source content hash
changes.

Add a per-workRoot document event stream for freshness. The stream should carry
source-keyed document events such as `document.contentChanged` and bounded
watch invalidation. The daemon may use cross-platform file watching for files
that are currently open or recently read, but watcher delivery is only a
freshness optimization: pane focus/visibility re-read plus content-hash checks
remain the correctness fallback, and panel close should remove frontend
interest without requiring a stream per panel.

Deferred scope: collaborative editing, merge/rebase UI, overwrite workflows,
rename/delete/move/create operations, provider-specific rich editors for
Excalidraw/draw.io/HTML, broad workspace filesystem watching, and full
dashboard persistence of translation preferences.

Verification should cover read/write route guards, optimistic-concurrency
conflict handling, panel-local mode switching, dirty draft preservation,
same-document clean pane refresh, dirty pane stale marking, per-workRoot event
delivery or invalidation behavior, focus re-read fallback, and browser-level
evidence that view/edit mode switching does not create duplicate tabs or break
workbench placement.

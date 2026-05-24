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
- Translation is a viewer feature over immutable content hashes, not mutation
  of source files. The browser can scaffold the block model and overlay state
  before a daemon or LLM-backed translation provider exists.
- Translation requests should send the whole document or section as contextual
  blocks while allowing the caller to request only selected block ids. The
  provider should translate with full context and return block-id-addressed
  translated results so the UI can match overlays deterministically.
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

Translation requests should carry all contextual blocks but allow partial
selection:

```ts
type DocumentTranslationRequest = {
  source: {
    kind: "workRootFile" | "activityTranscript" | "inlineMarkdown";
    workRootId?: string;
    path?: string;
    contentHash: string;
    format: "markdown" | "text";
    title?: string;
  };
  locale: {
    source?: string | null;
    target: string;
  };
  blocks: Array<{
    blockId: string;
    ordinal: number;
    kind: "paragraph" | "heading" | "listItem" | "code" | "quote" | string;
    markdown: string;
    plainText: string;
    lineStart?: number;
    lineEnd?: number;
  }>;
  requestedBlockIds?: string[];
};

type DocumentTranslationResult = {
  sourceContentHash: string;
  targetLocale: string;
  status: "completed" | "partial" | "failed";
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

Saved-content events should be keyed by document source:

```ts
type DocumentContentEvent = {
  type: "document.contentChanged";
  source: {
    workRootId: string;
    path: string;
  };
  contentHash: string;
  savedAtMs: number;
  originPaneId?: string;
};
```

## Phases

### Phase 1: Add reusable markdown document viewer substrate

Introduce a reusable document viewer module for read-only markdown rendering
from workRoot-relative file content. It should derive stable block identities,
line ranges when available, a source content hash, renderer kind, and
workRoot-relative pathrefs. Markdown view mode should support paragraph-level
actions for copying pathrefs and should reserve translation overlay state keyed
by `contentHash + blockId`, but it does not need to call a real translation
provider.

The first viewer should replace the plain preformatted markdown rendering path
for read-only file panes while preserving preview/pinned pane identity,
workbench placement policy, restore descriptor behavior, owner-authenticated
file reads, and scroll containment inside the pane. Activity Console markdown
may remain plain text unless this phase can reuse the viewer without increasing
scope.

Deferred scope: saving files, editable CodeMirror integration, real translation
provider calls, HTML/diagram/image renderers, Activity Console renderer
migration when it would require transcript-specific UI decisions.

Verification should include pure block/pathref/translation-overlay model tests
and browser-level evidence that markdown renders in the daemon-served file pane
without breaking preview/pin behavior or scroll containment.

### Phase 2: Add panel-local raw text edit mode and save fan-out contract

Add raw text edit mode as an in-pane view/edit segmented control on document
panes. Edit mode should use a browser-native text editor foundation, preserve a
draft, expose dirty/save/revert/error state, and save through a backend write
route that requires the caller's base content hash. A content-hash mismatch
returns a conflict state instead of overwriting another writer.

After a successful save, the frontend document store should emit or apply a
`document.contentChanged` event keyed by `workRootId + path`, then update all
clean panes for the same document source. Dirty edit panes for that source must
remain dirty and show that a newer saved version exists rather than silently
replacing the draft. Translation overlays and block pathref line metadata should
be marked stale when the source content hash changes.

Deferred scope: collaborative editing, merge/rebase UI, overwrite workflows,
rename/delete/move/create operations, provider-specific rich editors for
Excalidraw/draw.io/HTML, and cross-browser daemon event streaming. The initial
save fan-out can be browser-local as long as its event shape can later be
promoted to daemon SSE or WebSocket delivery.

Verification should cover read/write route guards, optimistic-concurrency
conflict handling, panel-local mode switching, dirty draft preservation,
same-document clean pane refresh, dirty pane stale marking, and browser-level
evidence that view/edit mode switching does not create duplicate tabs or break
workbench placement.

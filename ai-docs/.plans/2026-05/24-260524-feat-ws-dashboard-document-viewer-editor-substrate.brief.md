# Brief: 260524-feat-ws-dashboard-document-viewer-editor-substrate

## Intent

Replace the current markdown-as-plain-preformatted preview path with a reusable
read-only Markdown document viewer substrate for Phase 1 of the dashboard
document/editor track. The first slice should make Markdown files render as
structured documents, derive selectable document blocks with stable-in-content
ids and workRoot-relative pathrefs, and keep all existing read-only file pane
identity, preview/pinned, placement, restore, and scroll-containment behavior.

## Scope Boundary

Implement only Phase 1: Markdown viewer and block interaction.

In scope:

- reusable frontend document viewer module for Markdown view mode;
- Markdown AST-based block derivation and rendering;
- polished GFM table/task-list rendering;
- Obsidian-style callouts such as `> [!note]`;
- bounded footnote or footer hover affordance when supported by the chosen
  Markdown pipeline;
- block-level selection and action strip for visible-text copy, translated-copy
  placeholder state, and pathref copy;
- translation overlay data API shape keyed by `contentHash + blockId`, with
  local/fixture overlay only if needed for rendering proof;
- existing read-only file pane behavior preserved for preview/pinned identity,
  duplicate-open focus, file-open placement, restored descriptors, owner-auth
  file reads, and pane scroll containment.

Deferred:

- daemon translation provider routes, provider configuration, model discovery,
  LLM prompting, cache persistence, and real translation requests;
- raw text edit mode, write routes, optimistic save, dirty/stale state, and
  same-document save fan-out;
- rich markdown editing, CodeMirror, HTML rendering, iframe/sandbox support,
  Excalidraw, draw.io, images, and Activity Console adoption unless reuse is
  trivial and does not force transcript-specific UI decisions.

## Caller-Visible Contract

Opening a previewable Markdown file from the WorkRoot file explorer renders a
document viewer instead of raw preformatted text. Non-Markdown previewable text
files continue to render through the existing read-only text presentation.

The Markdown viewer:

- stays read-only and remains inside the existing workbench pane;
- exposes one pane-local `view | edit` segmented control, with edit disabled or
  unavailable in this phase;
- derives document blocks with stable ids within the current content, ordinal,
  kind, markdown/plain text, line range where available, translatable flag, and
  pathref;
- treats ordinary soft line breaks in prose as one block and list items as
  separate blocks;
- renders GFM tables and task lists as document UI, renders callouts for
  `> [!note]` and adjacent callout kinds, and avoids executing or directly
  trusting raw HTML;
- lets the owner select one or more blocks and copy visible text or pathrefs;
- shows translated-copy affordance as pending/unavailable unless local overlay
  data is present.

Copied pathrefs use `@<workRoot-relative-path>#L<line>` or
`@<workRoot-relative-path>#L<start>-L<end>` and never include absolute host
paths.

## Contract Instructions

Primary frontend files/modules:

- Add a dedicated reusable document module under `ws-dashboard/frontend/src/`
  or a narrow subdirectory such as `documentViewer/`.
- Reuse `ws-dashboard/frontend/src/workRootFiles.ts` for read-only pane source
  identity, preview/pinned keying, restore descriptors, and text-file fetch
  types. Extend these types only as needed to identify Markdown renderer hints
  from existing `extension`/`languageHint` data.
- Update `ws-dashboard/frontend/src/App.tsx` to render Markdown panes through
  the new viewer while preserving the existing `ReadOnlyTextPane` path for
  non-Markdown text and existing pane lifecycle logic.
- Update `ws-dashboard/frontend/src/styles.css` and `DESIGN.md` only as needed
  for reusable document-viewer classes consistent with the existing dark visual
  vocabulary.
- Keep workbench registration, placement, pane ids, logical keys, and restore
  storage compatible with the current read-only file pane model. Do not rename
  existing pane ids or logical keys in this phase.

Markdown parsing/rendering:

- Use a real Markdown pipeline from the unified/remark/rehype ecosystem or a
  similarly standard AST-based parser. Do not hand-roll the Markdown parser.
- Add dependencies to `ws-dashboard/frontend/package.json` and lockfile through
  normal npm install/update if needed.
- Raw HTML must be disabled, escaped, or ignored. Do not add unsafe
  `dangerouslySetInnerHTML` rendering for raw HTML.
- The block derivation API should be exported and independently testable.

Public data/API shape:

- Define a `DocumentBlock`-equivalent frontend type with at least:
  `blockId`, `ordinal`, `kind`, `markdown`, `plainText`, optional `lineStart`,
  optional `lineEnd`, optional `pathref`, and `translatable`.
- Define translation overlay input data keyed by `contentHash + blockId`, even
  though real daemon translation is deferred.
- If the existing backend read response lacks a content hash, derive a
  frontend-local content hash for the viewer namespace only. Do not add daemon
  write or translation routes in Phase 1.

Forbidden temporary wiring:

- Do not make the browser use absolute host paths as document identity or
  copied pathrefs.
- Do not bypass `fileExplorer.openFile`, read-only file fetch, or workbench
  placement policy.
- Do not make Markdown panes editable or mutate file contents.
- Do not route Activity Console transcripts into the new viewer unless it is
  zero-risk reuse after the Markdown file path is working.

## Integration Test Instructions

Add or extend frontend route/helper tests:

- Add tests for block derivation in a new file such as
  `ws-dashboard/frontend/src/documentViewer.test.ts`.
- Extend `ws-dashboard/frontend/src/workRootFiles.test.ts` only if read-only
  pane type or renderer-hint logic changes.
- Assertions must cover paragraph soft-line grouping, list-item block
  boundaries, heading/callout/table/task block classification where supported,
  stable-in-content block ids, pathref generation, raw host-path absence, and
  translation overlay matching by content hash plus block id.

Update browser acceptance:

- Extend `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` so the
  daemon-served production frontend opens a real Markdown fixture and proves
  structured Markdown rendering, block selection/action affordance visibility,
  pathref copy behavior when feasible in Playwright, and existing read-only
  preview/pinned behavior still works.

Verification commands:

- `npm run test:work-root-files`
- any new frontend helper test script added for the document viewer
- `npm run build`
- `npm run test:browser` unless blocked by the existing environment; if blocked,
  record exact blocker and manual/partial evidence.

## Implementation Strategy Decisions

- Prefer an AST-first Markdown module that produces both render data and
  `DocumentBlock[]` from one parse pass.
- Keep the existing read-only pane state model as the owner of pane lifecycle.
  Treat the document viewer as pane content, not a new workbench surface kind.
- Derive block identity within a content namespace; do not attempt cross-edit
  diff identity.
- Keep Phase 1 overlay support local and declarative. Real provider APIs belong
  to Phase 2.

## Rejected Alternatives

- Do not implement Markdown parsing by ad hoc string splitting.
- Do not merge raw edit mode into the Markdown renderer.
- Do not add a second workbench tab only to switch view/edit mode.
- Do not expose or copy absolute host paths.
- Do not make daemon translation calls in this phase.

## Approach

- Introduce a pure Markdown document model helper with tests.
- Add a React viewer component that renders parsed Markdown blocks and receives
  optional overlay data.
- Wire Markdown read-only panes in `App.tsx` to use the viewer based on file
  extension/language hint.
- Preserve the raw preformatted text path for non-Markdown files.
- Add document-viewer styling and focused browser acceptance coverage.

## Constraints

- Browser-visible controls must keep stable command identities where they are
  command actions. Local copy buttons may be component-local only when they do
  not mutate dashboard model state.
- The read-only file fetch route remains owner-authenticated and workRootId +
  relative-path addressed.
- Pane content must remain scroll-contained inside the workbench pane.
- UI changes must be verified against the daemon-served production frontend or
  documented as blocked with exact evidence.

## Out of scope

- Translation provider daemon implementation.
- Raw text editing and saving.
- Document event streams.
- HTML execution/rendering.
- Activity Console transcript Markdown replacement.
- WorkRoot Git management tickets.

## Details

Suggested `DocumentBlock` shape:

```ts
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
```

Suggested overlay shape:

```ts
type DocumentTranslationOverlay = {
  contentHash: string;
  blocks: Record<string, { translatedMarkdown: string; status: "ok" | "pending" | "failed" }>;
};
```

## Verification Contract

The implementation is acceptable when:

- helper tests prove block derivation and pathref behavior;
- existing work-root file pane helper tests still pass;
- production frontend build passes;
- browser acceptance or documented equivalent evidence proves Markdown files
  render through the structured viewer without breaking preview/pinned and
  scroll-containment behavior;
- no raw HTML execution, host-path pathrefs, edit/save behavior, or daemon
  translation routes were added.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md`:
  `260516-ws-web-dashboard-readonly-file-api`,
  `260516-ws-web-dashboard-readonly-text-pane`,
  `260516-ws-web-dashboard-file-open-placement-policy`,
  `260516-ws-web-dashboard-workroot-workbench-substrate`,
  `260516-ws-web-dashboard-browser-ui-acceptance-gate`,
  `260524-ws-dashboard-document-viewer-mode`,
  `260524-ws-dashboard-document-translation-overlay`
- [Must] `ai-docs/mental-model/ws-web-dashboard.md`
- [Must] `ai-docs/tickets/ready/260524-feat-ws-dashboard-document-viewer-editor-substrate.md`

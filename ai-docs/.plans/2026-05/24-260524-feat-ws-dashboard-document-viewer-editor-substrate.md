# Survey: 260524-feat-ws-dashboard-document-viewer-editor-substrate

## Reusable Components
- `ws-dashboard/frontend/src/workRootFiles.ts#L21-L50` — `WorkRootTextFileView` / `ReadOnlyFilePane`: existing read-only file content and pane state already carry `languageHint`/`extension` for Markdown renderer selection without changing backend identity.
- `ws-dashboard/frontend/src/workRootFiles.ts#L94-L124` — `workRootFileReadEndpoint` / `fetchWorkRootTextFile`: owner-authenticated relative-path read helper used by file panes; Markdown panes should consume its result rather than adding a new route in Phase 1.
- `ws-dashboard/frontend/src/workRootFiles.ts#L126-L195` — read-only pane mode/key/id/content helpers: preview vs pinned identity, workRoot-scoped preview replacement, file-path-addressed pinned tabs, and read result application live here.
- `ws-dashboard/frontend/src/workRootFiles.ts#L207-L228` — `readOnlyFilePaneRestoreSnapshot`: restore descriptors intentionally persist only workRoot id, relative path, mode, and title, preserving descriptor compatibility for document-viewer rendering.
- `ws-dashboard/frontend/src/workbench/policy.ts#L169-L220` — `decideSurfaceOpenWithDynamicGroups`: current placement policy focuses duplicate logical keys and opens read-only files in group 2; App already uses this for file opens.
- `ws-dashboard/frontend/src/styles.css#L188-L282` — shared `ws-toolbar`, `ws-control-button`, `ws-chip`, `ws-state-surface`, `ws-doc-surface`, and `ws-code-block` classes: reusable dark vocabulary for pane-local document chrome, actions, states, and code blocks.
- `ws-dashboard/frontend/src/styles.css#L1660-L1738` — `.readonly-text-*` styles: current read-only pane header/body/state styling and scrollable content baseline for the Markdown viewer shell.
- `ws-dashboard/frontend/src/App.tsx#L3321-L3335` — `hashText`: existing private lightweight content hashing pattern used for pane content revisions; relevant if Phase 1 needs a frontend-local viewer namespace hash and no daemon hash is added.

## Existing Patterns
- Read-only file open lifecycle: see `ws-dashboard/frontend/src/App.tsx#L419-L551` — creates loading pane, preserves pinned/preview replacement, applies placement, focuses the pane, fetches via `fetchWorkRootTextFile`, and rejects stale responses through `sameReadOnlyOpenRequest`.
- File explorer command dispatch: see `ws-dashboard/frontend/src/App.tsx#L1599-L1614` and `ws-dashboard/frontend/src/App.tsx#L1706-L1728` — rows dispatch `fileExplorer.openFile` before invoking pane open behavior; visible file-open controls already have stable command identity.
- Workbench pane body injection: see `ws-dashboard/frontend/src/App.tsx#L4216-L4245` — read-only files become `kind: "editor"` workbench panes with existing meta, content revision, and body component; this is the narrow render switch point for Markdown vs raw text.
- Current raw text presentation: see `ws-dashboard/frontend/src/App.tsx#L4248-L4285` — `ReadOnlyTextPane` owns the pane-local header, badges, loading/error states, and raw `<pre><code>` body.
- Route/helper test style: see `ws-dashboard/frontend/src/workRootFiles.test.ts#L1-L49` and `ws-dashboard/frontend/src/workRootFiles.test.ts#L312-L359` — frontend helper tests use plain TypeScript assertions and mocked `globalThis.fetch`, compiled through route-test tsconfig.
- Browser acceptance read-only flow: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1478-L1590` — existing gate proves preview replacement, immediate close, pinned transition, group placement, category presentation, and pane header/body visibility against daemon-served production UI.
- Browser fixture creation: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L50-L80` — the gate writes deterministic temp workRoot files before opening the UI; Markdown acceptance can add a real `.md` fixture here.
- Scroll-containment acceptance: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1592-L1641` — existing long read-only file test is the browser-level pattern for proving pane-local scroll rather than top-level document scroll.

## Relevant Interfaces
- `ws-dashboard/frontend/src/workRootFiles.ts#L5-L31` — `WorkRootFileEntryView`, `WorkRootFileListView`, `WorkRootTextFileView`: frontend shape for list entries, preview eligibility, and read-only text responses.
- `ws-dashboard/frontend/src/workRootFiles.ts#L33-L64` — `ReadOnlyFilePaneMode`, `ReadOnlyFilePane`, restore descriptor: public frontend state shape that App persists and rehydrates.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L23-L33` — `DockviewWorkbenchPane`: body/meta/contentRevision interface used by App to render pane content into Dockview.
- `ws-dashboard/frontend/src/workbench/surfaceRegistry.ts#L1-L19` — `SurfaceKind` includes `editor` and `viewer`; current read-only file panes intentionally use `editor` as a document-provider surface.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L377-L428` — `read_text_file`: backend read response includes content, size, extension, and language hint, but no content hash.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L431-L447` — `language_hint_for_extension`: `.md` maps to `markdown`; `.html` also has a hint but Phase 1 must not render raw HTML as trusted document content.
- `ws-dashboard/frontend/src/workRootActivity.ts#L54-L75` and `ws-dashboard/frontend/src/ActivityConsole.tsx#L640-L676` — Activity transcript blocks can be `renderKind: "markdown"`, but current UI still reduces details to escaped `<pre>` text.
- `ws-dashboard/frontend/tsconfig.route-tests.json#L1-L31` — route/helper test compilation is explicit-file include based; new document viewer tests/modules must be added to this config or a separate script.
- `ws-dashboard/frontend/package.json#L22-L29` — frontend dependencies currently include React, Dockview, Xterm, Lucide, and React Aria only; no Markdown AST/rendering packages are present yet.

## Constraints
- `ai-docs/spec/ws-web-dashboard/index.md#L784-L813` — read-only file API and pane contract require opaque `workRootId` plus workRoot-relative paths, duplicate-open focus/preview semantics, no save/dirty/edit behavior, and pane-local scrolling.
- `ai-docs/spec/ws-web-dashboard/index.md#L815-L837` — planned document viewer requires real Markdown AST pipeline, GFM/task-list/callout support, raw HTML disabled or ignored, stable block model, line ranges, translatability, and relative pathrefs.
- `ai-docs/spec/ws-web-dashboard/index.md#L839-L860` — translation overlay is keyed by immutable content hash plus `blockId`; daemon provider routes/cache are planned later, so Phase 1 should keep overlay declarative/local only.
- `ai-docs/spec/ws-web-dashboard/index.md#L878-L885` and `ws-dashboard/frontend/src/App.tsx#L461-L476` — file-open placement is dashboard-owned and currently uses workbench policy, not direct group insertion.
- `ai-docs/mental-model/ws-web-dashboard.md#L13-L18` — visible frontend changes need browser-level evidence against daemon-served production UI, not only TypeScript tests/build.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L449-L466` — backend file routes reject absolute paths, `..`, backslashes, and Windows drive-like paths before resolving relative paths.
- `ws-dashboard/frontend/DESIGN.md#L187-L197` — read-only and future document panes should use document surfaces, toolbar rules, chips, and existing pane chrome vocabulary rather than redefining pane chrome.

## Risk Signals
- `ws-dashboard/frontend/tsconfig.route-tests.json#L1-L31` — Possible test risk: adding `src/documentViewer.test.ts` alone will not run under existing helper-test compilation unless this explicit include list or a new script is updated.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L50-L80` and `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1478-L1590` — Possible acceptance risk: current browser fixture/file-open flow only uses `.txt` read-only files; Markdown proof needs an added real `.md` fixture and assertions without weakening existing preview/pinned checks.
- `ws-dashboard/frontend/src/App.tsx#L4216-L4245` — Possible contract risk: the workbench pane remains `kind: "editor"` and logical keys remain `editor...`; renaming to a new surface kind for Markdown would break the brief's compatibility constraint unless lead/planner explicitly approves.
- `ws-dashboard/frontend/src/workRootActivity.ts#L54-L75` and `ws-dashboard/frontend/src/ActivityConsole.tsx#L640-L676` — Possible shortcut risk: Activity transcripts advertise `renderKind: "markdown"` but are transcript-specific and rendered as code/detail blocks today; routing them into the new viewer could expand scope beyond Markdown file panes.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L431-L447` — Possible renderer risk: backend hints `.html` as `html`; Markdown rendering should key narrowly on Markdown hints/extensions and must not treat HTML hints as trusted renderable content in this phase.

## Opinion
- The codebase already centralizes read-only pane lifecycle enough that the survey does not need research escalation; the main implementation surface is a new pure document module plus a narrow body switch in `readOnlyWorkbenchPane`/`ReadOnlyTextPane`.
- The likely brittle spots are line-range extraction from the selected Markdown AST pipeline and keeping helper/browser tests wired into the project’s explicit test scripts.

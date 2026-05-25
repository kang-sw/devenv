# Survey: 260524-feat-ws-dashboard-document-viewer-editor-substrate-phase-2

## Reusable Components
- `ws-dashboard/frontend/src/documentViewer.tsx#L34-L52` — `DocumentBlock` / `DocumentTranslationOverlay`: Phase 1 exported block and local overlay types already match the Phase 2 frontend/daemon contract shape closely enough to extend.
- `ws-dashboard/frontend/src/documentViewer.tsx#L85-L103` — `localDocumentContentHash` / `deriveMarkdownDocumentModel`: current viewer derives a content hash and complete block set from Markdown; translation requests can reuse this model rather than reparsing in `App.tsx`.
- `ws-dashboard/frontend/src/documentViewer.tsx#L232-L241` — `translationForBlock`: existing overlay lookup accepts either raw `blockId` or `contentHash:blockId`, useful for preserving compatibility while daemon responses settle on one returned map shape.
- `ws-dashboard/frontend/src/documentViewer.tsx#L243-L327` — `DocumentViewer`: current viewer already owns selected blocks, copy-current/copy-translation/copy-pathref actions, and overlay rendering; Phase 2 can extend props/state instead of adding a parallel Markdown surface.
- `ws-dashboard/frontend/src/App.tsx#L4217-L4250` — `readOnlyWorkbenchPane`: narrow switch point that routes Markdown panes to `ReadOnlyMarkdownPane` while preserving `editor` surface kind, pane ids, meta, and content revision.
- `ws-dashboard/frontend/src/App.tsx#L4253-L4285` — `ReadOnlyMarkdownPane`: pane-local header/loading/error shell for Markdown; this is the owner for per-pane translation toggle/request/error state in the brief.
- `ws-dashboard/frontend/src/styles.css#L2720-L2866` — document viewer CSS: segmented view/edit control, action strip, scroll region, selected block states, callouts, tables, and translation block styling already exist in the dark dashboard vocabulary.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L238-L304` — workRoot file route pattern: handlers resolve opaque `workRootId`, validate relative paths, return bounded JSON errors, and run inside the protected router.
- `ws-dashboard/crates/daemon/src/terminal.rs#L335-L430` — JSON POST/GET route pattern: Axum `State`, `Path`, `Json` extraction plus bounded typed response/error handling for authenticated daemon APIs.
- `ws-dashboard/crates/daemon/tests/routes.rs#L196-L229` — `pair_and_cookie`: existing route-test helper for owner-authenticated requests; translation route auth/status/translate tests can reuse this path.

## Existing Patterns
- Frontend helper test script: see `ws-dashboard/frontend/package.json#L19-L21` and `ws-dashboard/frontend/tsconfig.route-tests.json#L28-L29` — `npm run test:document-viewer` already compiles and runs `documentViewer.test.ts`; translation helper tests can extend that file/script.
- Phase 1 overlay tests: see `ws-dashboard/frontend/src/documentViewer.test.ts#L116-L135` — current tests assert content-hash overlay matching and stale-hash rejection.
- Phase 1 block/pathref/copy browser gate: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L71-L88` and `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1613-L1658` — deterministic `gate-document.md` fixture and daemon-served Markdown-pane assertions already exist.
- Protected router registration: see `ws-dashboard/crates/daemon/src/router.rs#L44-L128` — all dashboard API routes are nested into the owner-auth protected router before static/app fallback routes.
- App state construction sites: see `ws-dashboard/crates/daemon/src/router.rs#L33-L42`, `ws-dashboard/crates/daemon/src/server.rs#L72-L80`, and `ws-dashboard/crates/daemon/tests/routes.rs#L126-L157` — any translation service/cache field must be initialized in server and test helpers.
- Backend path/privacy tests: see `ws-dashboard/crates/daemon/tests/routes.rs#L4617-L4737` — read-only file route tests verify owner auth and no traversal/path leaks; translation route tests should follow this bounded-response style.
- Test HTTP server seam: see `ws-dashboard/crates/daemon/tests/routes.rs#L4606-L4615` and `ws-dashboard/crates/daemon/tests/routes.rs#L5208-L5228` — route tests already bind an in-process Axum server for WebSocket work; a fake OpenAI-compatible server can use the same helper or a focused equivalent.

## Relevant Interfaces
- `ws-dashboard/frontend/src/documentViewer.tsx#L59-L64` — `MarkdownDocumentModel`: carries `contentHash`, public `blocks`, render blocks, and footnotes; request construction needs the first two fields.
- `ws-dashboard/frontend/src/documentViewer.tsx#L256-L288` — copy actions: current visible text, translated text, and pathref copy actions are local to `DocumentViewer`; translated-copy availability is already tied to `translationForBlock(...).status === "ok"`.
- `ws-dashboard/frontend/src/documentViewer.tsx#L314-L320` — overlay rendering: a successful overlay currently replaces block rendering with plain translated text and uses `title={block.plainText}` as hover-original affordance.
- `ws-dashboard/frontend/src/commands.ts#L1-L26` — `DashboardCommandId`: no document/translation command ids exist today; a visible translation toggle may need command-model extension if treated as a dashboard command action.
- `ws-dashboard/crates/daemon/src/config.rs#L8-L21` and `ws-dashboard/crates/daemon/src/cli.rs#L20-L40` — `ServeConfig` / `ServeArgs`: daemon config currently contains bind/static/auth only; provider config is absent.
- `ws-dashboard/Cargo.toml#L15-L31` and `ws-dashboard/crates/daemon/Cargo.toml#L11-L29` — workspace and daemon dependencies lack outbound HTTP and SHA256 crates today.
- `ws-dashboard/crates/daemon/src/lib.rs#L1-L15` — daemon module export list; a new `document_translation` module must be added for tests and router use.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L224-L236` — backend read response shape: it has content and metadata but no daemon SHA256/content hash, so Phase 2 translation source hash currently comes from the frontend document model.

## Constraints
- `ai-docs/spec/ws-web-dashboard/index.md#L845-L866` — translation overlay requires a pane-local toggle, whole-document request keyed by immutable content hash, daemon-owned provider/config/prompt/cache behavior, and block-id roundtrip validation without raw model output in the browser.
- `ai-docs/spec/ws-web-dashboard/index.md#L815-L843` — viewer mode remains read-only, AST-backed, raw HTML disabled/ignored, and block actions/pathrefs remain workRoot-relative.
- `ai-docs/spec/ws-web-dashboard/index.md#L784-L813` — file access remains opaque `workRootId` plus relative path and must not introduce save/dirty/edit behavior.
- `ai-docs/spec/ws-web-dashboard/index.md#L78-L106` — public dashboard APIs expose resource identity through opaque ids, not host paths, Git roots, or daemon-private locations.
- `ai-docs/mental-model/ws-web-dashboard.md#L13-L18` — visible frontend changes require browser-level evidence against daemon-served production UI; helper tests/build are insufficient alone.
- `ai-docs/mental-model/ws-web-dashboard.md#L39-L44` — read-only file panes and `documentViewer.tsx` own the document-viewer surface; file opens must keep workbench placement policy and the browser gate remains the acceptance surface.
- `ws-dashboard/frontend/src/documentViewer.tsx#L400-L401` and `ws-dashboard/frontend/src/documentViewer.test.ts#L97-L100` — raw HTML nodes are ignored in the viewer and covered by tests; translated Markdown rendering must preserve that safety property.

## Risk Signals
- `ws-dashboard/frontend/src/documentViewer.tsx#L314-L317` — Possible rendering risk: current translated overlays render `translatedMarkdown` as plain text, not through the Markdown renderer; Phase 2 may need to decide whether translated Markdown is parsed safely or treated as visible text despite the field name.
- `ws-dashboard/frontend/src/documentViewer.tsx#L252-L254` — Possible request-state risk: `DocumentViewer` derives its model internally and does not expose `contentHash`/`blocks` to `ReadOnlyMarkdownPane`; daemon request construction may require a callback/export refactor to avoid duplicate parsing or stale block sets.
- `ws-dashboard/frontend/src/commands.ts#L1-L26` — Possible command-contract risk: the brief says visible controls need stable command identities where they are command actions, but the command union has no document translation ids yet; local-only toggle behavior should be inspected against this rule.
- `ws-dashboard/crates/daemon/src/router.rs#L33-L42`, `ws-dashboard/crates/daemon/src/server.rs#L72-L80`, and `ws-dashboard/crates/daemon/tests/routes.rs#L126-L157` — Possible wiring risk: adding cloneable translation service/cache to `AppState` will touch multiple constructors; missing one breaks tests or server startup.
- `ws-dashboard/Cargo.toml#L15-L31` and `ws-dashboard/crates/daemon/Cargo.toml#L11-L29` — Possible dependency risk: outbound HTTP and SHA256 support are not present; adding a client crate may require async/runtime feature choices and test fake-server compatibility.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1613-L1658` — Possible acceptance risk: browser gate currently proves Markdown rendering and pathref copy but has no translation provider route stub/harness configuration; success overlay evidence may need a deterministic test provider or documented unavailable-provider path.
- `ws-dashboard/crates/daemon/src/config.rs#L33-L44` and `ws-dashboard/crates/daemon/src/cli.rs#L20-L40` — Possible config risk: config is CLI-derived only today; environment-backed provider defaults must avoid exposing API keys in args, logs, test fixtures, or browser-visible status.

## Opinion
- The survey is sufficient for implementation; no separate research escalation is needed if the implementer keeps provider support narrowly OpenAI-compatible and testable with a fake local server.
- The main uncertainty is not where to integrate, but how to shape the backend provider seam/cache so route tests can prove cache hits avoid provider calls without depending on live Ollama.

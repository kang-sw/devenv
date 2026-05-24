# Brief: 260524-feat-ws-dashboard-document-viewer-editor-substrate Phase 2

## Intent

Add the daemon-backed translation provider MVP and Markdown viewer overlay UX
for the dashboard document viewer. The owner should be able to enable
translation in a Markdown pane, have the frontend send the whole document block
set to the daemon, and see translated block overlays returned by a configured
OpenAI-compatible LLM provider, with bounded failures and cache reuse.

## Scope Boundary

Implement only Phase 2: Translation provider MVP and overlay UX.

In scope:

- daemon-owned translation provider status/model discovery;
- `llmOpenAICompatible` provider implementation compatible with local Ollama at
  `http://localhost:11434/v1`;
- provider config from daemon configuration or environment, with no browser
  provider configuration UI;
- whole-document translation request/response routes under authenticated
  dashboard APIs;
- SHA256/content-hash-based daemon cache keyed by source content hash, target
  locale, provider id/kind, model, provider config version, block model
  version, and prompt version;
- bounded prompt construction, JSON parsing, and block-id validation;
- Markdown toolbar translation toggle, asynchronous request state, overlay
  rendering, hover-original peek, translated copy action, and pathref/current
  copy behavior against selected blocks.

Deferred:

- generic/non-LLM translation provider implementation beyond the union/type
  room;
- provider configuration UI, provider secrets UI, terminology management,
  translation memory editing, streaming partial tokens, and cross-document
  consistency controls;
- Activity Console translation unless it reuses existing viewer code without
  changing transcript UX;
- raw text edit/save and document event streams.

## Caller-Visible Contract

Authenticated owners can query translation provider status and request a
translation for a document source by sending full document block context. The
browser never receives raw model output. Responses are either completed,
partial, or failed and include per-block `ok`, `omitted`, or `failed` states.

Markdown panes show a translation toggle in view mode. When enabled for a
loaded Markdown document, the pane requests whole-document translation for the
current content hash and selected target locale. Successful translated blocks
replace the visible block rendering. Hovering a translated block temporarily
shows the original block. Selected block actions can copy the currently visible
text, translated text when available, and workRoot-relative pathrefs.

Provider status reports configured/reachable/model state without exposing API
keys, prompts, raw provider responses, private host paths, or daemon cache
paths. If no provider is configured or reachable, the UI remains usable and
shows bounded status/error text.

## Contract Instructions

Backend files/modules:

- Add a focused translation module under
  `ws-dashboard/crates/daemon/src/`, for example `document_translation.rs`.
- Register routes in `ws-dashboard/crates/daemon/src/router.rs` behind the
  existing owner-auth protected router.
- Extend `ws-dashboard/crates/daemon/src/config.rs` and `cli.rs` only as needed
  for daemon-side provider configuration. Prefer environment-backed defaults
  over adding broad CLI/provider UI. Accept a local Ollama-compatible base URL
  when configured; do not require a default model if `/v1/models` cannot
  provide one.
- Extend `AppState` with a cloneable translation service/cache if the module
  needs state.
- Add dependency support for outbound HTTP and SHA256 hashing through
  `ws-dashboard/Cargo.toml` workspace dependencies and
  `ws-dashboard/crates/daemon/Cargo.toml` only as needed.

Frontend files/modules:

- Extend `ws-dashboard/frontend/src/documentViewer.tsx` and/or a helper module
  to send `DocumentBlock[]` and overlay state to daemon routes.
- Keep Phase 1 block model and rendering API compatible.
- Wire `ReadOnlyMarkdownPane` in `ws-dashboard/frontend/src/App.tsx` so each
  loaded Markdown pane owns translation toggle/request/error state.
- Add CSS only through existing dark dashboard document/toolbar/control
  vocabulary.

Routes and suggested shapes:

- `GET /api/dashboard/document-translation/providers`
  returns provider status and model discovery.
- `POST /api/dashboard/document-translation/translate`
  accepts a whole-document translation request with source identity, provider,
  locale, and `DocumentBlock[]`.

The implementation may choose equivalent route names if they stay under
`/api/dashboard/` and are documented by tests.

Required request/response semantics:

- Source kinds may include only `workRootFile` for the implemented route; keep
  type room for `activityTranscript` and `inlineMarkdown` without implementing
  those sources.
- The frontend sends content hash, workRoot id/path, format, title, target
  locale, provider id/model when selected, and all blocks.
- The daemon validates block ids are non-empty and unique before provider
  calls.
- LLM prompt requests JSON object/array output containing
  `blockId + translatedContent` pairs.
- Response parsing rejects duplicate, unknown, missing, or unparseable block
  ids into bounded per-block failures or partial status.
- Cache hits must not call the provider.

Forbidden temporary wiring:

- Do not put provider configuration, cache ownership, model probing, or prompt
  construction in the frontend.
- Do not send raw model output to the browser.
- Do not store API keys or raw prompts in tracked fixtures, logs, command
  payloads, or browser-visible state.
- Do not mutate source document content.
- Do not implement raw edit/save in this phase.

## Integration Test Instructions

Backend tests:

- Extend `ws-dashboard/crates/daemon/tests/routes.rs` or add focused module
  tests for provider status, request auth, cache key behavior, model probing
  parsing, block-id validation, bounded parse failures, and no raw output leak.
- Use a deterministic fake/mock OpenAI-compatible provider server or injectable
  provider client; tests must not require live Ollama or external network.

Frontend tests:

- Extend `ws-dashboard/frontend/src/documentViewer.test.ts` or add a
  translation helper test for overlay matching, visible/original copy behavior,
  translated copy state, and request payload construction.

Browser acceptance:

- Extend `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` to exercise a
  daemon-served Markdown pane with the translation toggle. Prefer a deterministic
  test provider or route stub exposed only to the harness; if unavailable,
  browser evidence may verify configured-unavailable behavior plus helper/route
  tests for successful overlay behavior.
- If local Ollama is available during dogfood, record provider/model used in
  output evidence without depending on private prompt/raw output.

Verification commands:

- `cargo test -p ws-dashboard-daemon`
- `npm run test:document-viewer`
- `npm run test:work-root-files`
- `npm run build`
- `npm run test:browser` unless blocked with exact blocker.

## Implementation Strategy Decisions

- Treat translation as a daemon-owned operation over immutable content hashes.
- Keep the cache in-process for the MVP unless existing durable state can be
  reused safely without mixing provider secrets into dashboard state. The
  cache key must still include the full version/provider/model dimensions so it
  can later move to persistence.
- Use OpenAI-compatible `/v1/models` and `/v1/chat/completions` request shapes
  for the first provider.
- Default target locale may be a simple fixed UI value such as Korean for this
  dogfood slice if no preference UI exists; keep the request type explicit so a
  later UI can change it.
- Keep relative/unsafe Markdown link handling from Phase 1 unchanged.

## Rejected Alternatives

- Do not translate block-by-block independently from the frontend.
- Do not let the frontend cache translations as source of truth.
- Do not add a generic provider UI in this phase.
- Do not use plain `git`/shell or external command execution to call Ollama.
- Do not make translation a file mutation.

## Approach

- Add backend types, provider status route, translate route, cache, prompt
  builder, OpenAI-compatible client, and fake-provider tests.
- Add frontend API wrapper and per-pane translation state.
- Extend `DocumentViewer` props to support translated overlay rendering,
  hover-original peek, and translated copy state.
- Add browser acceptance for the toggle and overlay path where the harness can
  deterministically provide translations.
- Update package/Cargo manifests and tests.

## Constraints

- All routes must be owner-authenticated through the protected router.
- Route identity uses opaque `workRootId` plus workRoot-relative path only.
- Browser-visible diagnostics must be bounded and must not include private
  host/cache paths, API keys, prompts, or raw provider output.
- Cache correctness must account for source content hash and provider/model
  versions, not only block text.

## Out of scope

- Raw edit/save and document events.
- Provider management UI.
- Non-LLM provider implementation.
- Streaming token UI.
- Activity Console translation.
- Durable translation cache persistence unless it is trivial and safe.

## Details

Provider config union should keep at least:

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
```

Prompt version and block model version should be stable constants in the
daemon module.

## Verification Contract

The phase is acceptable when:

- unauthenticated route access is rejected by existing auth path tests or new
  route tests;
- provider status/model probing can be tested without live external services;
- successful translation returns matched block ids and cache metadata;
- malformed/duplicate/missing/unknown block ids are bounded failures;
- frontend toggle can request translation and display overlay/error states;
- hover-original and selected translated copy behavior are covered;
- browser evidence is daemon-served or the exact blocker is recorded.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md`:
  `260524-ws-dashboard-document-viewer-mode`,
  `260524-ws-dashboard-document-translation-overlay`,
  `260516-ws-web-dashboard-readonly-file-api`,
  `260516-ws-web-dashboard-readonly-text-pane`,
  `260516-ws-web-dashboard-resource-view-model-contract`,
  `260516-ws-web-dashboard-workroot-workbench-substrate`,
  `260516-ws-web-dashboard-file-open-placement-policy`,
  `260516-ws-web-dashboard-browser-ui-acceptance-gate`
- [Must] `ai-docs/mental-model/ws-web-dashboard.md`
- [Must] `ai-docs/tickets/ready/260524-feat-ws-dashboard-document-viewer-editor-substrate.md`

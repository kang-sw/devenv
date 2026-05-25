# Implementation Plan: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 4

## Scope

Implement only Phase 4 remote files, documents, and document events. Preserve deferred scope: document translation provider forwarding, Activity/Git/workspace mutation/terminal operation coverage, terminal WebSocket forwarding, credential persistence, deployment automation, and public endpoint hardening.

## Concrete File Map

- `ws-dashboard/crates/daemon/src/router.rs#L95-L112` — Existing protected server-scoped aliases for root-picker/open/activation show where Phase 4 should register only explicit file/document aliases, not a wildcard proxy.
- `ws-dashboard/crates/daemon/src/router.rs#L205-L218` — Legacy local file list/read/write and document-event SSE routes are already protected and are the upstream/local compatibility paths for Phase 4.
- `ws-dashboard/crates/daemon/src/servers.rs#L478-L524` — `ServerScopedForwardOperation` currently allowlists one-shot root-picker/open/activation mappings; add file list/read/write mappings here only for ordinary HTTP/JSON operations.
- `ws-dashboard/crates/daemon/src/servers.rs#L539-L628` — Server-local aliases dispatch in-process by parsing route params/body; Phase 4 needs equivalent local aliases for file list/read/write and a separate local SSE alias for document events.
- `ws-dashboard/crates/daemon/src/servers.rs#L630-L790` — Linked one-shot forwarding resolves linked servers, bearer-authenticates upstream, preserves status/content-type/body, and returns bounded refusals; reuse for list/read/write but not for SSE.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L317-L383` — `list_work_root_files` and `read_work_root_file` validate registered online/available roots and relative paths before reading; server-local aliases should call these semantics instead of reimplementing filesystem access.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L385-L439` — `write_work_root_file` serializes writes by `workRootId + path`, performs optimistic hash conflict checks, and publishes `document.contentChanged` only after successful writes.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L441-L466` — `document_events` is a per-process workRoot-scoped SSE stream; linked forwarding must stream upstream `event: document` frames instead of buffering the body.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L65-L78` — `DocumentWriteLocks` keys only by bare workRoot/path today; do not make gateway writes share local locks with remote writes unless the key includes server identity or the lock remains upstream-owned.
- `ws-dashboard/frontend/src/resourceModel.ts#L20-L73` — `LOCAL_DASHBOARD_SERVER_ID`, `localCompatibleDashboardApiRoute`, and `serverScopedIdentity` are the canonical frontend route/key helpers.
- `ws-dashboard/frontend/src/workRootFiles.ts#L182-L207` — File list/read helpers already accept optional `serverId` and build `/api/dashboard/servers/{serverId}/work-roots/...` for linked servers.
- `ws-dashboard/frontend/src/workRootFiles.ts#L249-L300` — Document-event and file-write endpoint helpers already accept optional `serverId`; keep tests aligned with these canonical helpers.
- `ws-dashboard/frontend/src/workRootFiles.ts#L348-L404` — Read-only preview and pinned pane ids/logical keys are already server-scoped; preserve `editor-preview/<server>/<root>` vs `editor/<server>/<root>/<path>` identity.
- `ws-dashboard/frontend/src/App.tsx#L638-L760` — `openReadOnlyFile` derives `serverId` from `workRoot.resourcePath.serverId` and passes it to pane identity plus file reads.
- `ws-dashboard/frontend/src/App.tsx#L901-L927` — Save fan-out applies only to panes whose `serverId`, `workRootId`, and `path` match the saved source.
- `ws-dashboard/frontend/src/App.tsx#L3301-L3349` — `refreshOpenDocument` already uses a server-scoped source key and passes `serverId` into re-reads; keep stale-response guards intact.
- `ws-dashboard/frontend/src/App.tsx#L3837-L3880` — Document SSE subscription uses `workRootDocumentEventsEndpoint(rootId, selectedWorkRootServerId)` and refreshes with that server id; verify linked subscriptions exercise this path.
- `ws-dashboard/frontend/src/App.tsx#L6370-L6460` — Workbench panes are filtered by owning `serverId + workRootId`; remote same-id documents should not render under the wrong selected WorkRoot.
- `ws-dashboard/frontend/src/App.tsx#L6464-L6584` — Document edit/save/revert uses command payloads and `writeWorkRootTextFile(..., pane.serverId)`; conflict/stale behavior is pane-local.
- `ws-dashboard/frontend/src/documentViewer.tsx#L108-L140` — Translation overlay/hash helpers are frontend-local; preserve deferred translation provider forwarding and avoid collapsing remote/local cache identity if touched.
- `ws-dashboard/crates/daemon/tests/routes.rs#L2271-L2655` — Existing server-scoped route tests cover protected local aliases, bounded refusals, bearer forwarding, and resource rewriting patterns to copy for file/document routes.
- `ws-dashboard/crates/daemon/tests/routes.rs#L8440-L8675` — Existing file read tests cover auth, success, traversal, unknown/offline/unavailable, and read error behavior for legacy routes.
- `ws-dashboard/crates/daemon/tests/routes.rs#L9940-L10263` — Existing file write tests cover read/write cycle, conflicts, traversal, size/type errors, and unavailable roots for legacy routes.
- `ws-dashboard/crates/daemon/tests/routes.rs#L10266-L10392` — Existing document-event tests cover auth, unknown-root rejection, and local save invalidation SSE delivery.
- `ws-dashboard/frontend/src/workRootFiles.test.ts#L65-L115` — Helper tests already assert local and server-scoped file/read/write/document-event route construction.
- `ws-dashboard/frontend/src/workRootFiles.test.ts#L117-L203` — Parser and pane identity tests already cover document events and same bare file ids on different servers.
- `ai-docs/mental-model/ws-web-dashboard.md#L123-L131` — Coupling notes: server-scoped forwarding requires explicit mappings/tests; text panes couple route helpers, command controls, document events, and workbench placement.
- `ai-docs/mental-model/ws-web-dashboard.md#L153-L158` — Modification rules for extending server-scoped backend forwarding and changing file/document surfaces.
- `ai-docs/ref/ws-dashboard-playwright.local.md#L7-L29` — Browser gate and evidence redaction expectations for visible dashboard UI changes.

## Sequencing

1. **Register explicit backend aliases.** In `ws-dashboard/crates/daemon/src/router.rs#L95-L112`, add protected routes for `GET /api/dashboard/servers/{server_id}/work-roots/{work_root_id}/files`, `GET /files/read`, `POST /files/write`, and `GET /documents/events`; keep all other server-scoped file/document/Activity/Git/terminal paths as protected-router misses.
2. **Extend one-shot operation mappings for files.** In `ws-dashboard/crates/daemon/src/servers.rs#L478-L524`, add allowlisted `ServerScopedForwardOperation` constructors for file list/read/write. Preserve query strings with the existing `legacy_path_with_query` style for list/read and preserve `Content-Type`/body for write through `request_remote_dashboard_operation`.
3. **Add server-local file aliases.** Near `ws-dashboard/crates/daemon/src/servers.rs#L539-L628`, add handlers that dispatch `server-local` to the legacy `list_work_root_files`, `read_work_root_file`, and `write_work_root_file` semantics. Prefer parsing route/query/body and calling existing handlers or extracted shared helpers; do not duplicate path traversal or file IO rules.
4. **Add linked file forwarding.** For non-local server ids, route list/read/write through `resolve_server_scoped_forwarding` and the one-shot forwarding helper. Preserve upstream JSON/status errors exactly enough for existing frontend `apiErrorDetail` handling, and do not rewrite file response bodies unless a test proves the daemon must add `serverId` locally.
5. **Implement a dedicated document SSE gateway.** Do not use the one-shot body-buffering helper for `documents/events`. Add a route-scoped handler that owner-auth gates at the local gateway, resolves the linked server, sends an upstream bearer-authenticated `GET /api/dashboard/work-roots/{work_root_id}/documents/events`, validates an OK/event-stream response, and returns a streaming `Body`/SSE-compatible response that forwards upstream bytes until disconnect.
6. **Keep SSE bounded and route-specific.** The SSE gateway should forward only document events for the requested server/workRoot. It should return bounded JSON refusals for unknown/auth-required/tunnel-required/unreachable linked servers and must not introduce Activity SSE, terminal WebSocket, terminal HTTP lifecycle, or generic stream proxy behavior.
7. **Audit frontend route call sites.** Confirm every file list/read/write and document-event call uses `workRoot.resourcePath.serverId`, pane `serverId`, or selected workRoot server id. Key areas are `openReadOnlyFile` in `ws-dashboard/frontend/src/App.tsx#L638-L760`, `refreshOpenDocument` at `#L3301-L3349`, document SSE at `#L3837-L3880`, and save at `#L6538-L6584`.
8. **Harden frontend identity tests before UI tweaks.** Extend `ws-dashboard/frontend/src/workRootFiles.test.ts#L65-L203` if needed to prove route helpers, source keys, pane ids, restore descriptors, and same-source fan-out remain server-scoped for same bare `workRootId + path` on different servers.
9. **Preserve document edit semantics.** Keep `documentDraftContentChangeDecision` and save conflict handling in `ws-dashboard/frontend/src/workRootFiles.ts#L71-L101` and `ReadOnlyDocumentPane` in `ws-dashboard/frontend/src/App.tsx#L6464-L6584`; remote save conflicts should surface through the same optimistic hash error path.
10. **Do not move translation provider ownership.** If translation-related state is touched in `ws-dashboard/frontend/src/documentViewer.tsx` or `ws-dashboard/crates/daemon/src/document_translation.rs`, keep provider calls local-gateway-owned and ensure any cache/source key remains scoped by at least server/workRoot/path/content hash; do not forward provider routes.
11. **Run focused verification, then full smoke.** Execute the focused commands from the brief first, fix failures within Phase 4 scope only, then run the final full cargo/build/frontend smoke commands.
12. **Add browser evidence only for visible behavior.** If visible remote file/document behavior changes or can be practically tested, add or run a targeted Playwright case using the daemon-served frontend. If no linked-server fixture is practical, record that automated local-gateway route/unit coverage was used and do not claim real remote Windows dogfood.

## Backend / SSE Strategy

- Reuse `resolve_server_scoped_forwarding` in `ws-dashboard/crates/daemon/src/servers.rs#L658-L700` so file/document routes share the same linked-server status/refusal boundary as Phase 2.
- Keep list/read/write as ordinary allowlisted HTTP operations through `request_remote_dashboard_operation` in `ws-dashboard/crates/daemon/src/servers.rs#L747-L790`; this preserves bearer auth, content type, body forwarding, and bounded unreachable handling.
- Add a separate `request_remote_document_events`/similar helper for SSE that uses `reqwest::Response::bytes_stream()` or equivalent streaming body, forwards `content-type: text/event-stream` where practical, and closes when the browser disconnects.
- Do not rewrite upstream document event payloads in the gateway unless tests reveal the frontend cannot scope by subscription. The frontend subscription already supplies the selected server id when it calls `refreshOpenDocument` in `ws-dashboard/frontend/src/App.tsx#L3866-L3871`.
- Keep local `DocumentWriteLocks` server-local unless refactoring makes server-scoped lock keys necessary. Remote writes should be serialized by the upstream daemon's `DocumentWriteLocks` in `ws-dashboard/crates/daemon/src/work_root_files.rs#L65-L78`.

## Frontend Identity Work

- File routes: `fetchWorkRootFiles`, `fetchWorkRootTextFile`, and `writeWorkRootTextFile` in `ws-dashboard/frontend/src/workRootFiles.ts#L182-L323` already accept `serverId`; verify all App call sites pass a non-local server id for remote WorkRoots.
- Pane/source keys: keep `readOnlyFilePaneSourceKey`, `readOnlyFilePaneLogicalKey`, and `readOnlyFilePaneId` in `ws-dashboard/frontend/src/workRootFiles.ts#L103-L109` and `#L348-L374` as the only local identity builders for document sources and panes.
- Event subscriptions: keep the EventSource URL scoped with `workRootDocumentEventsEndpoint(rootId, selectedWorkRootServerId)` at `ws-dashboard/frontend/src/App.tsx#L3848-L3851`; remote/local same bare root ids must not share one EventSource or refresh sequence.
- Save fan-out: preserve `applyDocumentSaved` matching on `serverId + workRootId + path` at `ws-dashboard/frontend/src/App.tsx#L901-L927`; add tests if same-source preview/pinned panes across two servers can regress.
- Draft/conflict state: preserve pane-local state in `ReadOnlyDocumentPane` at `ws-dashboard/frontend/src/App.tsx#L6494-L6584`; stale remote event refresh should mark dirty drafts stale without overwriting local text.
- Restore descriptors: existing file-pane restore stores `serverId` in `ws-dashboard/frontend/src/workRootFiles.ts#L436-L520`; keep local-only old records loading as `server-local`.

## Tests

### Backend route tests

- Extend `ws-dashboard/crates/daemon/tests/routes.rs#L2271-L2477` with server-local file list/read/write/document-event aliases and assert equivalence or compatible behavior against legacy local routes.
- Extend `ws-dashboard/crates/daemon/tests/routes.rs#L2480-L2556` so unknown/auth-required/tunnel-required linked servers return bounded refusals for the new file/document-event aliases, and non-Phase-4 routes such as terminal WebSocket still return not found.
- Extend `ws-dashboard/crates/daemon/tests/routes.rs#L2559-L2655` or add adjacent tests with a spawned remote app: remote file list/read/write should reach upstream legacy routes with bearer auth and preserve upstream statuses/bodies.
- Add a linked document SSE forwarding test near `ws-dashboard/crates/daemon/tests/routes.rs#L10266-L10392`: subscribe through `/api/dashboard/servers/{serverId}/work-roots/{workRootId}/documents/events`, perform an upstream or gateway write, and assert an `event: document` frame with the expected `document.contentChanged` payload arrives.
- Keep existing legacy file/document coverage in `ws-dashboard/crates/daemon/tests/routes.rs#L8440-L8675`, `#L9940-L10263`, and `#L10266-L10392` passing unchanged.

### Frontend tests

- Keep or extend `ws-dashboard/frontend/src/workRootFiles.test.ts#L65-L115` for local-compatible and remote endpoint construction for list/read/write/document events.
- Extend `ws-dashboard/frontend/src/workRootFiles.test.ts#L117-L203` for same bare `workRootId + path` across servers: source keys, pane ids, logical keys, parser/application helpers, restore snapshots, and source errors must stay isolated.
- Add focused tests around stale/conflict/fan-out helpers in `ws-dashboard/frontend/src/workRootFiles.test.ts` if same-source updates need more coverage than current pane identity assertions.
- Keep command payload coverage in `ws-dashboard/frontend/src/commands.test.ts` aligned with document/file command server identity, but do not add endpoint hints or host paths to command payloads.
- If App-level route mocking is practical, test that a remote WorkRoot file open, document save, and document-event refresh call only local gateway server-scoped URLs.

### Commands

Focused commands from the brief:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml server_scoped
cargo test --manifest-path ws-dashboard/Cargo.toml forwarding
cargo test --manifest-path ws-dashboard/Cargo.toml work_root_files
cargo test --manifest-path ws-dashboard/Cargo.toml document
npm --prefix ws-dashboard/frontend run test:work-root-files
npm --prefix ws-dashboard/frontend run test:open-work-root
npm --prefix ws-dashboard/frontend run test:commands
npm --prefix ws-dashboard/frontend run build
```

Final verification:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml
npm --prefix ws-dashboard/frontend run build
npm --prefix ws-dashboard/frontend run test:work-root-files
npm --prefix ws-dashboard/frontend run test:open-work-root
npm --prefix ws-dashboard/frontend run test:commands
```

## Browser Verification

- `ai-docs/spec/ws-web-dashboard/index.md#L903-L907` and `ai-docs/ref/ws-dashboard-playwright.local.md#L7-L29` require browser-level evidence for visible dashboard UI work; helper, route, and build tests alone do not close visible behavior if the App changes are user-visible.
- Prefer a targeted daemon-served Playwright path in `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`: pair, open or use a linked-server WorkRoot fixture, browse the remote file tree, open markdown/text/code documents, edit/save/revert, and assert preview/pinned panes plus conflict/stale messaging remain scoped to the linked server.
- If real linked-server browser evidence is not practical, use mocked local-gateway server-scoped routes in the browser test and explicitly record that it is not native remote/Windows dogfood.
- Do not put private endpoints, hostnames, paths, tokens, screenshots with sensitive content, or raw tunnel URLs in tracked notes; use ignored Playwright artifacts for sensitive evidence.

## Risks

- `ws-dashboard/crates/daemon/src/servers.rs#L747-L790` — Possible stream risk: the existing forwarding helper buffers the full response body, so using it for document SSE would hang or defeat streaming.
- `ws-dashboard/crates/daemon/src/router.rs#L95-L112` and `#L205-L218` — Possible scope risk: adding a broad `/api/dashboard/servers/{server_id}/*` proxy would violate the Phase 2 allowlist boundary and accidentally include Activity/Git/terminal routes.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L65-L78` — Possible identity risk: write locks are bare `workRootId + path`; if gateway-local locking is introduced for remote writes, same-id local/remote files could serialize or collide unless keyed by server id.
- `ws-dashboard/frontend/src/App.tsx#L3837-L3880` — Possible event identity risk: document event payloads currently carry bare workRoot/path and rely on the subscription's server context; incorrect dependency arrays or selected-server fallback could refresh the wrong same-id pane.
- `ws-dashboard/frontend/src/App.tsx#L901-L927` — Possible fan-out risk: updating only the saving pane or ignoring `serverId` would break preview/pinned same-source fan-out or cross-server isolation.
- `ws-dashboard/frontend/src/App.tsx#L6464-L6584` — Possible draft risk: remote event refreshes must preserve dirty drafts as stale and not overwrite local unsaved text.
- `ws-dashboard/frontend/src/documentViewer.tsx#L93-L108` — Possible deferred-scope risk: translation request/cache identity can collapse remote/local documents if touched without adding server-scoped source identity, but provider forwarding remains out of scope.
- `ai-docs/mental-model/ws-web-dashboard.md#L223-L224` — Possible privacy risk: forwarding or browser harness diagnostics must not log private endpoints, paths, pairing URLs, cookies, or tokens.

## Lead Notes

- No lead decision is required if implementation stays within explicit Phase 4 file list/read/write and document-event SSE gateway behavior.
- Escalate before implementation if the implementer believes document translation provider forwarding, Activity/Git/workspace/terminal route coverage, terminal WebSocket forwarding, credential persistence, deployment hardening, or public endpoint behavior must change to satisfy this phase.

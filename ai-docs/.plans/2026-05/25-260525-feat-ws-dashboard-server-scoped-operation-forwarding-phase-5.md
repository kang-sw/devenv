# Implementation Plan: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 5

## Scope

Implement only Phase 5 remote Activity, Git, workspace, and WorkRoot mutations through explicit server-scoped local gateway routes. Preserve deferred scope: agent control actions, terminal HTTP lifecycle, terminal WebSocket forwarding, document translation forwarding, credential persistence, deployment automation, and public endpoint hardening.

## Concrete File Map

- `ai-docs/spec/ws-web-dashboard/index.md#L212-L283` — Contract requires server id in route/UI identity, local-gateway-only browser calls, explicit backend aliases, linked-server resource rewriting, and route-specific streaming instead of generic proxying.
- `ai-docs/mental-model/ws-web-dashboard.md#L64-L70` — Domain rules for server-scoped identity, explicit gateway aliases, resource response rewriting, activation/availability separation, and Git worktree-add command/path privacy.
- `ai-docs/mental-model/ws-web-dashboard.md#L153-L157` — Modification rules for extending server-scoped forwarding, Git worktree creation, and Activity projection/stream/pane behavior.
- `ws-dashboard/crates/daemon/src/router.rs#L96-L131` — Current protected server-scoped aliases cover root-picker/open/activation/files/documents; Phase 5 adds only Activity, workspace, Git, and Git worktree-add aliases here.
- `ws-dashboard/crates/daemon/src/router.rs#L153-L197` — Legacy local routes for workspace delete, git-worktree-add, activation, and Git toolbar operations are the compatibility paths to alias/forward.
- `ws-dashboard/crates/daemon/src/router.rs#L239-L248` — Legacy Activity snapshot, transcript, and event SSE routes are protected local compatibility routes.
- `ws-dashboard/crates/daemon/src/servers.rs#L483-L568` — `ServerScopedForwardOperation` is the allowlist and already carries method, legacy path, and resource-rewrite flag.
- `ws-dashboard/crates/daemon/src/servers.rs#L640-L735` — Existing server-local aliases show the pattern: dispatch `server-local` in-process, forward linked ordinary operations, and use dedicated document SSE forwarding.
- `ws-dashboard/crates/daemon/src/servers.rs#L737-L798` — Linked forwarding resolution and document-event SSE forwarding are reusable for ordinary HTTP/JSON and Activity SSE, respectively.
- `ws-dashboard/crates/daemon/src/servers.rs#L800-L842` — `resolve_server_scoped_forwarding` centralizes unknown/auth/tunnel/refusal handling; reuse it for every Phase 5 linked route.
- `ws-dashboard/crates/daemon/src/servers.rs#L851-L885` — `ForwardedDashboardResponse::into_response_for` rewrites successful resource-bearing responses when `rewrite_resources` is true.
- `ws-dashboard/crates/daemon/src/servers.rs#L904-L997` — Remote request helpers preserve bearer auth, content type, status, body, opened-workRoot header, and stream content-type validation.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L279-L307` — WorkRoot activation mutates registry state, persists before returning resources, and rolls back on persistence failure.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L310-L356` — Workspace removal unregisters all workspace workRoots, persists rollback-safe state, removes local terminals, and returns resources.
- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L134-L204` — Git status/branches/switch routes run local Git work off async workers and return bounded errors/status refresh payloads.
- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L206-L326` — Git create/fetch/push/pull mutations return refreshed status or bounded errors with optional status bodies.
- `ws-dashboard/crates/daemon/src/git_toolbar.rs#L328-L356` — Git route access derives from live resources, activation, availability, Git kind, and opened-root resolution.
- `ws-dashboard/crates/daemon/src/git_worktree.rs#L155-L285` — Git worktree-add options/preview/submit resolve workspace Git context, validate, run `git worktree add`, persist, and return `{ resources, createdWorkRootId }`.
- `ws-dashboard/crates/daemon/src/git_worktree.rs#L328-L410` — Preview computes target path labels and blockers; owner-visible remote paths may appear in responses but must not become command identity.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L187-L263` — Activity snapshot/transcript/events are read-only, resolve online/available workRoots, and stream named `event: activity` SSE frames.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L281-L360` — Activity SSE emits fallback invalidation, item upserts, heartbeats, and cursor-scoped events; linked gateway must stream bytes, not buffer.
- `ws-dashboard/crates/daemon/tests/routes.rs#L2317-L2655` — Existing server-scoped local-alias route tests for Phase 2/4 are the place to add Phase 5 local alias coverage.
- `ws-dashboard/crates/daemon/tests/routes.rs#L2777-L3232` — Existing linked-server refusal/forwarding tests cover bounded refusals, bearer forwarding, resource rewriting, and SSE forwarding patterns.
- `ws-dashboard/crates/daemon/tests/routes.rs#L3882-L4189` — Existing Git worktree-add route tests cover auth, preview/submit success, blockers, non-Git, and unknown-workspace behavior.
- `ws-dashboard/crates/daemon/tests/routes.rs#L4831-L5072` — Existing WorkRoot Activity route/SSE tests cover auth, unknown roots, event frames, reconnect cursors, and stream scoping.
- `ws-dashboard/frontend/src/resourceModel.ts#L20-L84` — Canonical server id, route, identity, and activation endpoint helpers; workspace endpoint already accepts `serverId`.
- `ws-dashboard/frontend/src/resourceRefresh.ts#L14-L29` — Canonical resource fetch route uses `/api/dashboard/servers/{serverId}/resources` for linked selected servers.
- `ws-dashboard/frontend/src/workRootActivity.ts#L152-L179` — Activity stream key and event endpoint already include `serverId`.
- `ws-dashboard/frontend/src/workRootActivity.ts#L307-L321` — Activity stream stale-response guard compares server id, workRoot id, and request id.
- `ws-dashboard/frontend/src/workRootActivity.ts#L335-L415` — Activity snapshot/transcript helpers already accept `serverId`, but call sites must pass it for remote WorkRoots.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L57-L61` and `#L167-L292` — Transcript request identity is currently workRoot/activity/request only; Phase 5 must account for server id or prevent cross-server transcript application at the caller boundary.
- `ws-dashboard/frontend/src/gitToolbar.ts#L64-L161` — Git toolbar route helpers already accept optional `serverId` for status, branches, switch, create, fetch, push, and pull.
- `ws-dashboard/frontend/src/gitWorktreeAdd.ts#L52-L60` and `#L121-L170` — Git worktree-add helpers already produce local-compatible server-scoped workspace routes.
- `ws-dashboard/frontend/src/commands.ts#L46-L110` — Command payloads support a top-level optional `serverId`, but some payload variants remain typed without server-specific required fields.
- `ws-dashboard/frontend/src/commands.ts#L343-L367` — Workspace removal and activation command builders already include `serverId` in emitted payloads.
- `ws-dashboard/frontend/src/App.tsx#L300-L330` — Activation/removal request functions use server-compatible endpoint helpers and return resource views.
- `ws-dashboard/frontend/src/App.tsx#L784-L825` — Command dispatcher applies activation resources externally and starts workspace removal flow with payload server id.
- `ws-dashboard/frontend/src/App.tsx#L4796-L4883` — Git toolbar derives `serverId` from `root.resourcePath.serverId` and uses it for refresh/status branch calls, but state keys only store `workRootId`.
- `ws-dashboard/frontend/src/App.tsx#L4930-L5060` — Git mutations dispatch command builders and call server-scoped helper functions with `serverId`.
- `ws-dashboard/frontend/src/App.tsx#L6380-L6389` — Workbench file panes are filtered by both root id and server id; mirror this identity discipline for Activity panes and any Phase 5 state.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1064-L1110` and `#L2201-L2221` — Existing browser Activity pane coverage can be extended for visible remote Activity identity where practical.

## Sequencing

1. **Add explicit protected backend routes.** In `ws-dashboard/crates/daemon/src/router.rs#L96-L131`, register only the Phase 5 server-scoped aliases: workspace delete; git-worktree-add options/preview/submit; WorkRoot Activity snapshot/transcript/events; Git status/branches/switch/create/fetch/push/pull. Do not add terminal, agent-control, translation, or wildcard routes.
2. **Extend the allowlist.** Add `ServerScopedForwardOperation` constructors in `ws-dashboard/crates/daemon/src/servers.rs#L483-L568` for each Phase 5 ordinary HTTP/JSON operation. Mark workspace delete, activation, and git-worktree-add submit as resource-rewriting responses; mark Git status/branches/mutations, Git worktree options/preview, Activity snapshot, and transcript as non-resource responses.
3. **Implement server-local aliases first.** Near `ws-dashboard/crates/daemon/src/servers.rs#L640-L735`, dispatch `server-local` to existing legacy handlers in `root_picker.rs`, `git_toolbar.rs`, `git_worktree.rs`, and `work_root_activity.rs`, preserving JSON content-type behavior where legacy Axum extractors require JSON.
4. **Forward linked ordinary operations.** Route non-local ordinary operations through `forward_server_scoped_operation` in `ws-dashboard/crates/daemon/src/servers.rs#L737-L763`, preserving query strings with `legacy_path_with_query` for Activity/Git worktree options/transcript/event cursor paths and preserving request bodies/content types for POSTs.
5. **Add dedicated Activity SSE forwarding.** Use the document-event SSE shape in `ws-dashboard/crates/daemon/src/servers.rs#L765-L798` and `#L904-L952` for Activity events, but target `/api/dashboard/work-roots/{work_root_id}/activity/events` and validate upstream `text/event-stream`. Do not route Activity SSE through `request_remote_dashboard_operation` because it buffers bodies.
6. **Rewrite resource-bearing mutation responses.** Ensure linked activation, workspace removal, and git-worktree submit success bodies are parsed as resource views or response wrappers and rewritten to the selected linked server id/label. If `AddGitWorktreeResponse` wraps `resources`, add route-specific rewriting rather than assuming the existing `DashboardResourcesView` parser will match.
7. **Preserve refusal boundaries.** Reuse `resolve_server_scoped_forwarding` for unknown/auth-required/tunnel-required/unreachable linked servers, and add tests that deferred terminal, agent-control, translation, terminal WebSocket, and unregistered document-translation server-scoped routes still 404 or remain absent.
8. **Thread frontend server identity.** Audit `App.tsx`, `ActivityConsole.tsx`, `workRootActivity.ts`, `gitToolbar.ts`, `gitWorktreeAdd.ts`, `commands.ts`, and `resourceRefresh.ts` so selected WorkRoot Activity, transcript loaders, Git state, add-worktree modal state, activation/removal, and command payloads use `root.resourcePath.serverId` or selected server id and never direct linked endpoint URLs.
9. **Harden same-id frontend state.** Key Activity stream state, transcript request/scroll memory, Git toolbar state, Git worktree modal request state, and mutation resource refresh by `serverId + workspaceId/workRootId/activityId` wherever same bare ids can collide.
10. **Keep command payloads path-private.** Git worktree custom target paths remain only in authenticated request bodies from `gitWorktreeAdd.ts#L137-L170`; command builders in `commands.ts` should carry server/workspace/workRoot ids, not target paths, endpoints, or host paths.
11. **Add focused backend tests.** Extend the existing server-scoped route blocks in `routes.rs` to cover auth, server-local alias parity, bounded linked-server refusals, bearer forwarding, upstream status/content-type/body preservation, resource response rewriting, wrapped git-worktree submit rewriting, Activity SSE content-type rejection, and Activity named-event streaming.
12. **Add focused frontend tests.** Extend `workRootActivity`, `git`, `commands`, and `open-work-root` tests to assert remote helper URLs, command `serverId` payloads, stale-response guards, same bare id local/remote isolation, and resource-refresh application for remote mutations.
13. **Browser verify visible behavior.** Add or run daemon-served Playwright coverage for a linked-server WorkRoot showing remote Activity, Git toolbar operations or state, activation/removal, and add-worktree flow when practical. If a real linked remote fixture is not practical, use mocked local-gateway server-scoped routes and record the limit.
14. **Run focused verification, then full smoke.** Run the brief's focused cargo/frontend commands first, then full `cargo test --manifest-path ws-dashboard/Cargo.toml`, affected frontend tests, and `npm --prefix ws-dashboard/frontend run build`.

## Backend / SSE Strategy

- Ordinary forwarding should stay allowlisted and route-specific through `ServerScopedForwardOperation`; no `/api/dashboard/servers/{server_id}/*` proxy.
- Activity SSE needs a dedicated helper parallel to `request_remote_document_events` so upstream bytes remain streaming and the response preserves `event: activity` frames.
- Consider generalizing `RemoteDocumentEventsResponse` to a route-neutral remote SSE response only if it stays private to the explicit document/activity helpers; do not expose a generic stream proxy API.
- Forwarded resource rewrite must cover both plain `DashboardResourcesView` and route-specific wrappers such as `AddGitWorktreeResponse { resources, createdWorkRootId }`.
- Workspace removal on a linked server must not remove local daemon terminals for the same bare workRoot ids; `server-local` keeps the legacy in-process terminal cleanup, while linked cleanup is upstream-owned.

## Frontend Identity Work

- Activity endpoints and keys in `workRootActivity.ts#L152-L179` and `#L335-L415` already accept `serverId`; ensure all App/ActivityConsole loaders supply it.
- Extend transcript request identity in `ActivityConsole.tsx#L57-L61` or wrap loaders so stale responses compare server id as well as workRoot/activity/request.
- Git route helpers in `gitToolbar.ts#L64-L161` are server-compatible; state in `App.tsx#L4807-L4814` should include server id to prevent same bare WorkRoot collisions.
- Git worktree-add helpers in `gitWorktreeAdd.ts#L52-L170` are server-compatible; modal state should remember the selected workspace's server id and apply rewritten submit resources before selecting `createdWorkRootId`.
- Activation/removal command builders in `commands.ts#L343-L367` already include `serverId`; keep command observer payloads free of host paths, endpoints, and Git target paths.

## Tests

### Backend route tests

- Add protected-route/auth assertions for every new server-scoped alias near `ws-dashboard/crates/daemon/tests/routes.rs#L2317-L2655`.
- Add server-local parity tests for Activity snapshot/transcript/events, workspace removal, Git toolbar routes, and Git worktree-add routes beside the existing server-local alias tests.
- Extend linked refusal tests near `ws-dashboard/crates/daemon/tests/routes.rs#L2777-L2901` for unknown, auth-required, tunnel-required, and unreachable servers for each new route family.
- Extend linked forwarding tests near `ws-dashboard/crates/daemon/tests/routes.rs#L2901-L3232` for bearer auth, upstream body/status/content-type preservation, resource rewriting, and deferred route misses.
- Add Activity SSE forwarding tests using the existing Activity stream coverage around `ws-dashboard/crates/daemon/tests/routes.rs#L4831-L5072`: subscribe through the server-scoped route, assert `event: activity` frames, reject invalid upstream SSE content types, and preserve bounded upstream errors.
- Add Git worktree wrapper rewrite tests near `ws-dashboard/crates/daemon/tests/routes.rs#L3882-L4189` for linked submit response `{ resources, createdWorkRootId }`.

### Frontend tests

- `npm --prefix ws-dashboard/frontend run test:work-root-activity`: assert Activity endpoints, stream keys, stream stale guards, transcript loads, and same-id isolation include server id.
- `npm --prefix ws-dashboard/frontend run test:git`: assert every Git helper uses `/api/dashboard/servers/{serverId}/...` for linked servers and local compatibility routes for `server-local`.
- `npm --prefix ws-dashboard/frontend run test:commands`: assert activation, workspace removal, Git, Activity, and git-worktree command payloads include server id where relevant and omit remote paths/endpoints.
- `npm --prefix ws-dashboard/frontend run test:open-work-root`: preserve remote resource refresh/open reconciliation behavior while adding Phase 5 mutation resource application checks if the helper coverage lives there.
- Add App-level mocked route coverage where practical for remote Activity pane load, Activity SSE refresh, Git toolbar refresh/mutation, workspace removal, and git-worktree submit using only local gateway URLs.

### Commands

Focused commands from the brief:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml server_scoped
cargo test --manifest-path ws-dashboard/Cargo.toml forwarding
cargo test --manifest-path ws-dashboard/Cargo.toml linked_server
cargo test --manifest-path ws-dashboard/Cargo.toml work_root_activity
cargo test --manifest-path ws-dashboard/Cargo.toml git
cargo test --manifest-path ws-dashboard/Cargo.toml git_worktree
cargo test --manifest-path ws-dashboard/Cargo.toml root_picker
cargo test --manifest-path ws-dashboard/Cargo.toml resources
npm --prefix ws-dashboard/frontend run test:work-root-activity
npm --prefix ws-dashboard/frontend run test:git
npm --prefix ws-dashboard/frontend run test:commands
npm --prefix ws-dashboard/frontend run test:open-work-root
npm --prefix ws-dashboard/frontend run build
```

Final verification:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml
npm --prefix ws-dashboard/frontend run test:work-root-activity
npm --prefix ws-dashboard/frontend run test:git
npm --prefix ws-dashboard/frontend run test:commands
npm --prefix ws-dashboard/frontend run test:open-work-root
npm --prefix ws-dashboard/frontend run build
```

## Browser Verification

- `ai-docs/mental-model/ws-web-dashboard.md#L14-L16` and `ai-docs/spec/ws-web-dashboard/index.md#L897-L904` require browser-level evidence for visible dashboard UI changes; pure Rust/TypeScript tests do not close Activity/Git/workspace UI behavior.
- Extend `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1064-L1110` or add a focused linked-server browser case that uses local gateway `/api/dashboard/servers/{serverId}/...` route mocks to prove remote Activity pane open/stream/transcript behavior stays separate from a local same-id WorkRoot.
- Add visible Git/activation/add-worktree coverage if practical: selected remote WorkRoot shows Git toolbar state from server-scoped URLs, mutation feedback updates only that server's resources, and custom target path labels remain owner-visible feedback rather than command identity.
- If using a real remote daemon fixture, keep evidence private/ignored and do not commit endpoints, hostnames, tokens, raw remote paths, screenshots with sensitive content, or tunnel URLs.

## Risks

- `ws-dashboard/crates/daemon/src/servers.rs#L851-L885` — Resource rewrite risk: existing rewrite only parses a bare `DashboardResourcesView`; Git worktree submit wraps resources in `AddGitWorktreeResponse`.
- `ws-dashboard/crates/daemon/src/servers.rs#L954-L997` — Stream risk: ordinary forwarding buffers the response body and must not be used for Activity SSE.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L310-L356` — Side-effect risk: linked workspace removal must be upstream-owned; local terminal cleanup for same bare ids must not run in the gateway for remote workRoots.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L57-L61` — Identity risk: transcript request keys omit server id today, so same bare workRoot/activity ids can accept stale remote/local responses unless guarded.
- `ws-dashboard/frontend/src/App.tsx#L4807-L4814` — Identity risk: Git toolbar state stores only workRoot id; same bare ids across servers can reuse status/branch state.
- `ai-docs/mental-model/ws-web-dashboard.md#L214-L215` — Privacy/UX risk: Git worktree target paths can be owner-visible in preview/errors but must not enter command payloads, URLs, or shared evidence.
- `ai-docs/mental-model/ws-web-dashboard.md#L170-L171` — Scope risk: adding a wildcard server-scoped proxy or including terminal/translation/agent controls would violate the phase boundary.

## Lead Notes

- No lead decision is required if implementation stays within explicit Phase 5 Activity, Git, workspace, and WorkRoot mutation aliases.
- Escalate before implementation if forwarding agent control actions, terminal HTTP/WebSocket routes, document translation, credentials/deployment/public hardening, or a generic proxy appears necessary.

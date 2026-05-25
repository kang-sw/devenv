# Brief: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 4

## Intent

Make file listing, file read, file write, and document content-change events
work for remote WorkRoots through the local gateway server-scoped route model.
Remote file explorer and document panes should behave like local panes while
preserving server identity in routes, pane identity, event subscriptions, and
write/conflict state.

## Scope Boundary

Selected scope: Phase 4, "Remote files, documents, and document events."

In scope:

- Server-scoped backend forwarding for file listing, file read, file write, and
  document-event SSE routes.
- Frontend file/document helpers and call sites use server-scoped routes for
  remote WorkRoots.
- File explorer, read-only/code/markdown views, edit/save/revert, stale/conflict
  handling, same-source fan-out, and document content-change fan-out work for
  remote WorkRoots.
- Document pane identity, document event subscriptions, and write conflict keys
  include `serverId` where bare ids or paths can collide.
- Tests for backend route auth/forwarding/SSE behavior, frontend route helpers,
  pane/source identity, stale/conflict behavior, and visible browser behavior as
  practical.

Deferred:

- Document translation provider forwarding. Translation may remain local
  gateway-owned, but source/cache identity must not collapse remote and local
  documents.
- Activity, Git, workspace mutation, terminal HTTP lifecycle, and terminal
  WebSocket forwarding.
- Credential persistence, deployment automation, and public endpoint hardening.

## Caller-Visible Contract

After a remote WorkRoot is open under a linked server, the user can browse its
files, open text/markdown/code documents, edit and save files, revert local
drafts, observe stale/conflict state, and receive document change fan-out through
the local gateway. The browser continues to call only local gateway routes.

Local and remote documents with the same bare `workRootId` and relative path
must not share pane state, event subscriptions, draft/conflict state, or save
fan-out identity.

## Contract Instructions

Reuse Phase 1 server-scoped frontend helpers and identity utilities. Reuse
Phase 2 linked-server resolver/allowlisted forwarding patterns. Keep new
backend forwarding explicit and route-scoped; do not add wildcard proxying.

SSE forwarding for document events is in scope for this phase, but Activity SSE
and terminal WebSocket forwarding are not. Keep stream handling owner-auth gated
at the local gateway and bearer-authenticated upstream for linked servers.

Do not expose linked daemon endpoints or private host paths in command payloads,
logs, or shared verification notes.

## Integration Test Instructions

Focused commands:

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

Final verification should include:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml
npm --prefix ws-dashboard/frontend run build
npm --prefix ws-dashboard/frontend run test:work-root-files
npm --prefix ws-dashboard/frontend run test:open-work-root
npm --prefix ws-dashboard/frontend run test:commands
```

Run a targeted Playwright/browser case for linked-server file/document behavior
if visible App behavior changes or new browser evidence is practical. Do not
claim real remote Windows dogfood without actually running it.

## Implementation Strategy Decisions

- Add explicit backend mappings for file list/read/write and document-event SSE
  only.
- Preserve existing local routes and `server-local` compatibility.
- Route remote file/document frontend calls through the local gateway.
- Keep source keys and pane keys server-scoped; same bare ids on two servers
  must remain independent.
- Preserve optimistic hash/conflict semantics and same-source fan-out behavior.

## Rejected Alternatives

- Generic daemon proxy for file/document paths: rejected because Phase 2
  established an allowlisted forwarding boundary.
- Folding Activity/Git/terminal operations into this phase: rejected because
  later phases own those surfaces and their verification.
- Moving document translation provider ownership to linked servers now:
  rejected as deferred by the ticket.

## Approach

- Extend backend server-scoped allowlist and router for file list/read/write and
  document-event SSE.
- Thread server identity through frontend file/document helper calls and event
  subscriptions.
- Add or extend tests around route construction, pane/source keys,
  stale/conflict/fan-out behavior, backend forwarding, and document SSE.
- Run focused backend/frontend tests, then full cargo and frontend build smoke.

## Constraints

- All AI-authored docs and comments must be English.
- Do not broaden stream forwarding beyond document-event SSE.
- Do not record private endpoints, hostnames, paths, tokens, or screenshots in
  shared verification notes.

## Survey References

### Must

- `ai-docs/spec/ws-web-dashboard/index.md`
- `ai-docs/mental-model/ws-web-dashboard.md`
- `ws-dashboard/crates/daemon/src/router.rs`
- `ws-dashboard/crates/daemon/src/servers.rs`
- `ws-dashboard/crates/daemon/src/work_root_files.rs`
- `ws-dashboard/crates/core/src/events.rs`
- `ws-dashboard/frontend/src/resourceModel.ts`
- `ws-dashboard/frontend/src/workRootFiles.ts`
- `ws-dashboard/frontend/src/App.tsx`
- `ws-dashboard/frontend/src/documentViewer.tsx`
- `ws-dashboard/frontend/src/workbench/`
- `ws-dashboard/frontend/e2e/`

### Maybe

- `ai-docs/tickets/todo/260525-feat-ws-dashboard-document-polishing-backlog.md`
- `ai-docs/tickets/todo/260514-epic-ws-web-dashboard-mvp.md`
- `ai-docs/tickets/idea/260514-research-ws-web-dashboard-direction.md`
- `ai-docs/ref/ws-dashboard-playwright.local.md`
- `ws-dashboard/crates/daemon/src/document_translation.rs`

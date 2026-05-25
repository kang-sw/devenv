# Brief: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 5

## Intent

Make Activity, Git, workspace, and WorkRoot mutation operations transparent for
linked-server WorkRoots through explicit server-scoped local gateway routes.
This phase completes the non-terminal, non-document operation set that remote
WorkRoots need after root picker and file/document forwarding.

## Scope Boundary

Selected scope: Phase 5, "Remote Activity, Git, workspace, and WorkRoot
mutations."

In scope:

- WorkRoot Activity snapshots, transcript reads, and Activity event SSE through
  server-scoped routes.
- WorkRoot activation changes and workspace removal through server-scoped
  routes.
- Git toolbar operations through server-scoped routes.
- Git worktree-add options, preview, and submit through server-scoped routes.
- Activity stream keys and transcript lookups include `serverId`.
- Mutation responses containing resources are rewritten to the linked-server id.
- Tests for backend forwarding/refusals/SSE, frontend route helpers and command
  payloads, identity isolation, and visible browser behavior where practical.

Deferred:

- Agent control actions such as interrupt, cancel, erase, retry, terminate.
- Terminal HTTP lifecycle and terminal WebSocket forwarding.
- Document translation forwarding, credential persistence, deployment
  automation, and public endpoint hardening.

## Caller-Visible Contract

For a linked-server WorkRoot, Activity panels, transcript reads, Activity live
events, activation, workspace removal, Git toolbar actions, and Git worktree-add
flows operate through the local gateway and apply to the linked server. Same
bare ids on different servers do not share Activity stream state, transcript
state, Git state, workspace mutation state, or resource refresh identity.

Remote Git path previews and errors are remote-host paths. They may be shown as
owner-visible operation feedback, but they must not become command identity or
private shared evidence.

## Contract Instructions

Reuse explicit server-scoped route mappings and the Phase 2/4 forwarding
patterns. Add only the routes needed for Activity, Git, workspace, and WorkRoot
mutations; do not introduce wildcard proxying.

Activity event SSE is in scope. Document SSE already exists; terminal WebSocket
and terminal HTTP lifecycle remain out of scope.

Keep browser calls on the local gateway. Do not call linked daemon endpoints
directly from the frontend.

## Integration Test Instructions

Focused commands:

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

Final verification should include full cargo and frontend build plus the
affected frontend tests above.

## Implementation Strategy Decisions

- Keep forwarding allowlisted and operation-specific.
- Use dedicated SSE forwarding for Activity events.
- Preserve local compatibility for `server-local`.
- Rewrite resource-bearing remote mutation responses to the selected linked
  server id.
- Keep Activity and Git state keyed by server id wherever same bare ids can
  collide.

## Rejected Alternatives

- Generic route proxy: rejected by earlier phase boundaries.
- Agent control forwarding: rejected as explicitly deferred.
- Terminal forwarding: rejected as Phase 6/7 scope.

## Approach

- Extend backend server-scoped route registration and forwarding mappings for
  Activity, activation, workspace removal, Git toolbar, and Git worktree-add.
- Add Activity SSE forwarding using the document-event SSE pattern.
- Audit frontend Activity/Git/workspace call sites and command payloads for
  `serverId`.
- Add backend and frontend tests for route targeting, refUSALS, resource
  rewriting, stream isolation, and same-id local/remote separation.
- Run focused tests first, then full cargo and frontend build smoke.

## Constraints

- All AI-authored docs/comments must be English.
- Do not broaden terminal, agent control, translation, deployment, or public
  endpoint behavior.
- Do not record private endpoints, hostnames, paths, tokens, or sensitive
  screenshots in shared verification notes.

## Survey References

### Must

- `ai-docs/spec/ws-web-dashboard/index.md`
- `ai-docs/mental-model/ws-web-dashboard.md`
- `ws-dashboard/crates/daemon/src/servers.rs`
- `ws-dashboard/crates/daemon/src/work_root_activity.rs`
- `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs`
- `ws-dashboard/crates/core/src/activity.rs`
- `ws-dashboard/crates/daemon/src/root_picker.rs`
- `ws-dashboard/crates/daemon/src/git_worktree.rs`
- `ws-dashboard/crates/daemon/src/resources.rs`
- `ws-dashboard/crates/daemon/src/persistent_state.rs`
- `ws-dashboard/crates/daemon/src/discovery.rs`
- `ws-dashboard/frontend/src/workRootActivity.ts`
- `ws-dashboard/frontend/src/ActivityConsole.tsx`
- `ws-dashboard/frontend/src/App.tsx`
- `ws-dashboard/frontend/src/gitWorktreeAdd.ts`
- `ws-dashboard/frontend/src/commands.ts`
- `ws-dashboard/frontend/src/resourceRefresh.ts`

### Maybe

- `ai-docs/tickets/todo/260525-feat-ws-dashboard-workroot-polishing-backlog.md`
- `ai-docs/tickets/todo/260514-epic-ws-web-dashboard-mvp.md`
- `ai-docs/ref/ws-dashboard-playwright.local.md`
- `ai-docs/tickets/idea/260523-feat-ws-dashboard-main-session-activity-source.md`

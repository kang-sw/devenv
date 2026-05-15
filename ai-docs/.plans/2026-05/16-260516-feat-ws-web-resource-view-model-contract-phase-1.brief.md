# Brief: 260516-feat-ws-web-resource-view-model-contract Phase 1

## Intent

Complete Phase 1 of the dashboard resource view-model contract by making the
core dashboard resource vocabulary use workRoot names and executable public
contract tests. This phase prepares later API, frontend, discovery, and stream
work without adding Phase 2 daemon routes or mock providers.

## Approach

- Treat skeleton commit `8e249ef` as the public contract baseline.
- Keep implementation scoped to `ws-dashboard/crates/core` and workspace
  dependency metadata needed by core tests.
- Ensure public exports use `WorkRootId`, `WorkRootStatus`, `WorkRootKind`, and
  `ResourcePath.work_root_id`.
- Ensure serde-facing field/value names use dashboard API vocabulary:
  `workRootId`, `plainDirectory`, `gitPrimaryRoot`, and `gitLinkedWorktree`.
- Run core and workspace tests after any changes.

## Constraints

- Do not add Phase 2 dashboard HTTP routes, mock providers, golden fixtures, or
  daemon API handlers.
- Do not expose `WorktreeId`, `WorktreeState`, or `worktree_id` as public
  dashboard core API.
- Preserve the daemon foundation auth and bind-mode behavior; existing daemon
  tests must continue to pass.
- Keep the dashboard daemon separate from ws MCP session authority.

## Out of scope

- Authenticated view-model API routes.
- Mock fixture provider and golden fixture files.
- Local workspace discovery and root picker behavior.
- Event streams, PTY, named-agent, editor, viewer, translation, or bookmark
  behavior.

## Details

Existing skeleton contracts:

- `ws-dashboard/crates/core/src/ids.rs` exports `WorkRootId`.
- `ws-dashboard/crates/core/src/resources.rs` defines `WorkRootStatus`,
  `WorkRootKind`, and `ResourcePath.work_root_id`.
- Core serde contract tests assert `ResourcePath` serializes as `workRootId`
  and `WorkRootKind` serializes the three dashboard contract values.

Acceptance checks:

- `cargo test -p ws-dashboard-core`
- `cargo test --workspace`

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-resource-view-model-contract` and
  `260516-ws-web-dashboard-mock-view-model-fixtures`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard auth boundary,
  host-control separation, and daemon-owned view-model expectations.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-resource-view-model-contract.md`
  - Phase 1 scope and success criteria.
- [Must] `ai-docs/tickets/todo/260515-epic-ws-web-dashboard-first-visible-substrate.md`
  - milestone ordering and workRoot hierarchy decisions.
- [Must] `ai-docs/tickets/idea/260514-research-ws-web-dashboard-direction.md`
  - resource model rationale and deferred discovery/UI boundaries.

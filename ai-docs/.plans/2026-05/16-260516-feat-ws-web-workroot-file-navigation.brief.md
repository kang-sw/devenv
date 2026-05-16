# Brief: 260516-feat-ws-web-workroot-file-navigation

## Intent

Implement Phase 1 only: an owner-authenticated daemon API that lists directory
entries below an already opened workRoot so the later browser file explorer can
navigate project files without using raw host paths as route identity.

## Scope Boundary

Selected slice: `Phase 1: Authenticated WorkRoot Listing API`.

Defer Phase 2 entirely. Do not render frontend file explorer UI, wire browser
open actions, or add read-only file pane behavior.

## Caller-Visible Contract

The daemon exposes a protected HTTP listing route for a selected workRoot. The
caller addresses the workRoot by opaque `workRootId`; host absolute paths must
remain daemon-private and must not become browser route identity.

The listing response exposes enough data for a compact file tree:

- directory entry names;
- entry kind for files and directories;
- a workRoot-relative location token or path suitable for later follow-up
  listing/open calls;
- basic status for unreadable or inaccessible entries;
- cheap preview/read eligibility for regular files where practical.

Requests stay rooted below the selected workRoot. Traversal attempts, unknown
workRoots, non-directory targets, missing paths, unreadable paths, and other
filesystem errors return bounded error or unavailable states without filesystem
mutation.

## Implementation Strategy Decisions

- Reuse the existing protected-router pattern; every new route must sit behind
  owner auth.
- Reuse the root picker/open-workRoot flow as the point where daemon-private
  workRoot path state is established.
- Add only the minimum daemon-owned registry needed for opened workRoots so the
  listing route can resolve `workRootId` to a private root path.
- Keep response vocabulary dashboard-local and camelCase, matching existing API
  style.
- Keep listing read-only. No create/delete/rename/move/copy/chmod operations.

## Rejected Alternatives

- Do not accept raw absolute host paths in the listing route.
- Do not make the browser route or resource identity depend on filesystem
  paths.
- Do not switch `/api/dashboard/resources` from its current mock-backed fixture
  behavior in this slice.
- Do not implement read-only file content APIs in this slice.

## Approach

- Add a small daemon module or state component for opened workRoot path
  registration and lookup.
- Update `AppState` and server/test setup as needed to carry that state.
- Update the existing open-workRoot handler so successful opens register the
  returned `workRootId` with its daemon-private path.
- Add a protected listing handler under a workRoot-scoped dashboard API path.
- Add route tests for auth rejection, successful listing after opening a
  workRoot, traversal rejection, and unknown-workRoot handling.

## Constraints

- Preserve owner-auth and Host/Origin behavior.
- Preserve `workRoot` vocabulary; do not introduce worktree identity names into
  public API.
- Keep path traversal rejection before filesystem reads outside the selected
  workRoot.
- Keep all new behavior deterministic enough for route tests.
- Do not add broad file-manager operations.

## Out of scope

- Frontend left-nav explorer UI.
- Read-only file content API and workbench text pane.
- File mutation, save, dirty state, rename, delete, move, copy, chmod, or
  recursive operations.
- Terminal sessions or agent/harness-specific behavior.

## Details

Prefer a route shaped around opaque workRoot identity, for example:

```text
GET /api/dashboard/work-roots/{workRootId}/files?path=<relative-path>
```

The exact query key may differ if existing Axum patterns make another name
cleaner, but it must carry a workRoot-relative value rather than an absolute
host path. The root directory listing should work when the query is absent or
empty.

The response should be JSON and include a stable root/workRoot identity plus
entry array. Entry fields should be sufficient for Phase 2 to render and refresh
a tree without reinterpreting host paths.

## Verification Contract

- Run the daemon route tests that cover the new API.
- Run formatting/checks needed for touched Rust code.
- Confirm unauthenticated requests reject before handler behavior.
- Confirm traversal attempts fail without listing outside the workRoot.
- Confirm opening a workRoot then listing by returned `workRootId` succeeds.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - dashboard spec anchors for daemon foundation, resource view-model, local workRoot discovery, root picker behavior, and planned workRoot file listing.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard auth, route, workRoot identity, and path-leak invariants.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-workroot-file-navigation.md` - selected Phase 1 contract.
- [Must] `ai-docs/tickets/todo/260516-epic-ws-web-dashboard-workroot-io-substrate.md` - milestone boundaries; file navigation stays workRoot-centered.
- [Maybe] `ai-docs/tickets/todo/260516-feat-ws-web-readonly-text-pane.md` - later consumer of preview/read eligibility.
- [Maybe] `ai-docs/tickets/todo/260516-feat-ws-web-workroot-io-workbench-integration.md` - later integration and dogfood expectations.

# Brief: 260516-feat-ws-web-readonly-text-pane

## Intent

Implement the first read-only file preview flow for opened workRoots: an
authenticated daemon file-read API, a workbench text pane surface, and file-open
placement from the left-nav file explorer.

## Scope Boundary

Selected slice: all phases of `260516-feat-ws-web-readonly-text-pane`.

Implement:

- Phase 1: Authenticated Read-Only File API.
- Phase 2: Workbench Text Pane Surface.
- Phase 3: File-Open Placement Policy.

Do not implement write-back editing, save, dirty state, formatting,
language-server behavior, terminal sessions, detached restore UX, or the later
cross-surface workbench integration ticket.

## Caller-Visible Contract

Owners can click/open a previewable file from the selected-workRoot file
explorer and see its content in a read-only workbench pane. File identity stays
workRoot-scoped: the browser uses opaque `workRootId` plus workRoot-relative
path data, never raw absolute host paths as route or surface identity.

The daemon read API rejects traversal, missing files, directories, unreadable
paths, unsupported binary content, and oversized files with bounded unavailable
states. Successful responses include text content and metadata needed for the
frontend to show title, relative path, size/read-only status, and a cheap
language/extension hint when available.

Opening the same file focuses the existing logical pane instead of duplicating
it by default. New file panes prefer the second or later split group when
available so durable terminal or future agent work is not displaced.

## Implementation Strategy Decisions

- Reuse the opened workRoot registry and path validation boundary introduced by
  the listing API.
- Keep read-only file API behavior in daemon-owned routes behind owner auth.
- Add a frontend file-read helper rather than embedding fetch logic directly in
  components.
- Register/use a workbench surface kind or existing editor-like surface policy
  only as needed to represent read-only file panes cleanly.
- Keep the pane body real and inspectable, not a debug card.
- Use existing workbench placement helpers/policies where possible.

## Rejected Alternatives

- Do not expose host absolute paths in routes, pane ids, or visible metadata.
- Do not implement write/save/dirty-state UI.
- Do not add file-manager operations.
- Do not replace terminal/agent-focused panes when support split placement is
  available.
- Do not implement persistence/restore beyond current browser arrangement state.

## Approach

- Add daemon read-only file API types/handler/tests, likely near the
  workRoot-file listing module.
- Extend route tests for auth rejection, success, traversal, binary/unsupported,
  oversized, directory, missing, and unknown-workRoot cases.
- Add frontend fetch/type helpers and tests for endpoint construction and
  bounded errors.
- Wire file explorer open actions to request/read file content and open/focus a
  workbench pane.
- Add/update workbench model tests for logical-key dedupe and placement
  preference when needed.
- Style pane body with existing dark semantic tokens.

## Constraints

- Keep all behavior under owner-auth protected routes.
- Keep path handling workRoot-relative and traversal-safe.
- Use deterministic tests; avoid adding broad browser test machinery unless
  already trivial.
- Preserve existing workbench drag/drop/tab behavior.
- Keep UI text within compact panes and narrow layouts.

## Out of scope

- Editing, saving, formatting, dirty-state, conflict handling.
- Terminal sessions.
- Workbench restore across refresh.
- Full IDE language features.

## Details

Prefer a route shape consistent with the listing API, for example:

```text
GET /api/dashboard/work-roots/{workRootId}/files/read?path=<relative-path>
```

The exact route can differ if local routing patterns make another shape cleaner,
but it must use `workRootId` plus a relative path.

The frontend logical surface key should be derived from `workRootId` and
relative path so duplicate opens focus the existing pane.

## Verification Contract

- Run daemon route tests and daemon crate tests when daemon API changes.
- Run frontend workRoot-files/workbench tests and frontend build.
- Run workspace `cargo check` if Rust daemon code changes.
- Use delegated correctness/fit/test review focused on auth/path safety,
  placement behavior, and coverage.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - daemon auth, protected frontend, workbench substrate, file listing/explorer, and planned read-only pane contracts.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - route auth/path safety, file explorer, and workbench placement invariants.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-readonly-text-pane.md` - selected implementation scope.
- [Must] `ai-docs/tickets/todo/260516-epic-ws-web-dashboard-workroot-io-substrate.md` - milestone decisions and exclusions.
- [Maybe] `ai-docs/tickets/todo/260516-feat-ws-web-workroot-io-workbench-integration.md` - later restore/dogfood expectations.

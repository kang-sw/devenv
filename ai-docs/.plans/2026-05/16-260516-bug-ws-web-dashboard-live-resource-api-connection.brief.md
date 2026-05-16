# Brief: 260516-bug-ws-web-dashboard-live-resource-api-connection

## Intent

Connect the ws dashboard's primary resource flow to real opened workRoots. The
current substrate has live open-workRoot, file, text-pane, and terminal
capabilities, but the default dashboard resource load still uses mock resource
fixtures. This implementation must make normal daemon resource loads and the
browser-visible resource tree reflect opened workRoot state, then record
acceptance evidence for the real product flow.

## Scope Boundary

Implement all unfinished phases for
`260516-bug-ws-web-dashboard-live-resource-api-connection`:

- Phase 1: `GET /api/dashboard/resources` uses live opened workRoot state after
  a workRoot is opened.
- Phase 2: the frontend resource model refreshes or reconciles after opening a
  workRoot so the visible resource tree selects the real opened workRoot.
- Phase 3: record daemon-served dogfood evidence starting from the default
  dashboard resource load and proving the real workRoot, file explorer,
  read-only file pane, and terminal flow are not mock-backed.

Out of scope:

- Replacing completed file listing, read-only file pane, terminal lifecycle, or
  workbench placement behavior.
- Broad root-picker redesign, generic file manager operations, write-back file
  editing, agent presets, named-agent controls, detached terminal restore UX, or
  a new resource hierarchy.
- Treating mock fixture data as production state. Keep fixtures only for tests
  or explicit development/fixture paths.

## Caller-Visible Contract

Normal authenticated daemon resource loads use live opened workRoot state as the
primary resource authority. Once an owner opens a workRoot, subsequent
`/api/dashboard/resources` responses include that opened workRoot and do not
indistinguishably return the static mock fixture workspace.

The no-opened-workRoot fallback must be explicit and honest. Prefer an empty
live resource view or a clearly opt-in fixture/development mode; do not leave
production default behavior silently mock-backed.

After a workRoot is opened from the browser flow, the visible navigation/resource
tree refreshes from the canonical dashboard resources endpoint or reconciles
with an immediately returned live resource view. The canonical endpoint remains
the source for refresh/re-entry behavior.

The default product-flow dogfood evidence must start from the dashboard's normal
resource load, open or use a real workRoot, and then verify that resource tree,
file navigation, read-only text pane, and terminal session behavior all operate
against that real workRoot rather than the mock fixture.

## References

[Must] `ai-docs/spec/ws-web-dashboard/index.md`

- `260516-ws-web-dashboard-live-resource-authority`
- `260516-ws-web-dashboard-open-workroot-resource-refresh`
- `260516-ws-web-dashboard-live-resource-dogfood-verification`
- `260516-ws-web-dashboard-resource-view-model-contract`
- `260516-ws-web-dashboard-mock-view-model-fixtures`
- `260516-ws-web-dashboard-local-workroot-discovery-provider`
- `260516-ws-web-dashboard-inspectable-navigation-shell`

[Must] `ai-docs/mental-model/ws-web-dashboard.md`

- Read this before source edits. It names the existing dashboard resource
  provider seam and common mistakes around mock/live switching.

[Must] `ai-docs/tickets/todo/260516-epic-ws-web-dashboard-workroot-io-substrate.md`

- Parent epic reopened specifically because this acceptance gap remained. Keep
  the completed sibling substrates intact.

[Maybe] Existing completed siblings and dogfood artifacts:

- `260516-feat-ws-web-local-workspace-discovery`
- `260516-feat-ws-web-workroot-file-navigation`
- `260516-feat-ws-web-workroot-io-workbench-integration`
- Prior dogfood artifact under `ai-docs/.plans/2026-05/`

## Implementation Notes

Use the existing resource view-model shape and provider vocabulary. Do not
introduce host paths as identity. Preserve opaque `serverId`, `workspaceId`,
`workRootId`, and `instanceId` usage.

Route tests should prove this sequence:

1. Authenticated caller opens a real temporary workRoot.
2. Caller then requests `/api/dashboard/resources`.
3. The returned tree contains the opened workRoot and not only the static mock
   fixture workspace.

Frontend tests should prove the resource tree changes to the live opened
workRoot after the open/refresh flow and does not leave the mock workspace as
the selected active tree.

Dogfood evidence should be recorded in a new artifact near the previous
workRoot IO dogfood notes. If interactive screenshot tooling is unavailable,
record that limitation separately, but still include HTTP or browser-equivalent
evidence for the primary resources endpoint.

## Verification

Run the narrow backend and frontend tests that cover changed resource and
workbench behavior, then run any existing package-level checks normally used for
the dashboard slices touched here.

At minimum, report:

- Rust formatting/check/test commands that cover daemon resource routes.
- Frontend test command for resource tree/open-workRoot behavior.
- Dogfood command sequence or browser-equivalent evidence path.

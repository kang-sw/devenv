---
title: ws web dashboard live resource API connection
parent: 260516-epic-ws-web-dashboard-workroot-io-substrate
related:
  260516-epic-ws-web-dashboard-workroot-io-substrate: containing reopened milestone
  260516-feat-ws-web-local-workspace-discovery: opened workRoot discovery prerequisite
  260516-feat-ws-web-workroot-file-navigation: live file navigation consumer
  260516-feat-ws-web-workroot-io-workbench-integration: prior integration pass that missed this product-flow acceptance
spec:
  - 260516-ws-web-dashboard-live-resource-authority
  - 260516-ws-web-dashboard-open-workroot-resource-refresh
  - 260516-ws-web-dashboard-live-resource-dogfood-verification
related-mental-model:
  - ws-web-dashboard
plans:
  phase-1: 2026-05/16-260516-bug-ws-web-dashboard-live-resource-api-connection
  phase-2: 2026-05/16-260516-bug-ws-web-dashboard-live-resource-api-connection
  phase-3: 2026-05/16-260516-bug-ws-web-dashboard-live-resource-api-connection
completed: 2026-05-16
---

# ws web dashboard live resource API connection

## Background

The workRoot IO substrate added real opened-workRoot APIs, file listing,
read-only file panes, terminal sessions, and workbench integration, but the
dashboard's primary resource source still opens on mock data. The acceptance gap
is that `GET /api/dashboard/resources` remains mock-backed while the frontend
uses that endpoint as its initial resource tree. Opening a workRoot can return a
live view for that request, but it does not make subsequent dashboard resource
loads reflect the opened workRoot as the source of truth.

This ticket exists to connect the already-built substrate to the primary product
flow. It should not discard the completed file, pane, or terminal work. It
should make the first browser-visible resource model represent real daemon state
after a workRoot is opened, with mock fixtures limited to explicit development
or fixture-only paths.

## Decisions

- Treat this as a completion bug in the current workRoot IO epic, not as a new
  feature direction.
- Preserve the daemon-owned resource hierarchy from the existing dashboard
  contracts: server, workspace, workRoot, and instance identifiers stay opaque.
- Keep mock fixtures available only where tests or explicit development modes
  require them; they must not be the default production resource authority.
- Verification must include evidence that the browser-visible resource tree is
  backed by the opened workRoot state, not only direct calls to child file or
  terminal APIs.

## Phases

### Phase 1: Connect resources endpoint to opened workRoots

Make `GET /api/dashboard/resources` derive its dashboard resource view from
daemon-owned opened workRoot state once a workRoot has been opened. The route
should stop unconditionally returning the mock provider in normal daemon
operation. Define the no-opened-workRoot fallback explicitly: either an empty
live view with an honest empty state or a clearly development-only fixture mode,
but not an indistinguishable mock workspace.

Success means a route-level test can open a real temporary workRoot, call the
resources endpoint afterward, and observe that the returned resource tree
contains the opened workRoot rather than the static mock fixture.

### Result (80bc4e42) - 2026-05-16

The daemon resources route now derives the public resource view from registered
opened workRoots instead of the static mock fixture. With no opened workRoot it
returns an honest empty live server view, and after opening a workRoot the
canonical endpoint includes that live workRoot. The open-workRoot route returns
the aggregated live view so immediate responses and later canonical refreshes
match. Review fixes moved blocking discovery work off the async route path and
kept the mock provider covered as a fixture-only test surface.

### Phase 2: Refresh browser resource state after opening a workRoot

Ensure the frontend's resource model uses the live resources endpoint after
opening or selecting a workRoot. If an open-workRoot response returns enough
resource data to update immediately, reconcile it with the canonical endpoint;
otherwise refetch the resources endpoint after the open succeeds. The left
navigation and workbench selection should not continue presenting the mock
workspace as the active workspace after a live workRoot exists.

Success means frontend tests cover the open-or-refresh path and prove the
resource tree swaps to the live workRoot model without losing existing pane or
terminal behavior.

### Result (80bc4e42) - 2026-05-16

The frontend gained a minimal path-input open-workRoot control, an
`openWorkRoot` helper, and a pure `resourceModel` module that owns entity
flattening and selection reconciliation. After an open succeeds, the browser
reconciles with the aggregated response, selects the just-opened workRoot, and
reloads the canonical resources endpoint. Shared API error handling and
resource-model tests cover the selection and error branches without adding a
broad root-picker redesign.

### Phase 3: Record acceptance dogfood for the default product flow

Run daemon-served verification against the production frontend path and record
the exact evidence in a dogfood artifact. The evidence must start from the
default dashboard resource load, open or use a real workRoot, and then confirm
that the browser-visible resource tree, file navigation, read-only file pane,
and terminal session all operate against that real workRoot.

If interactive screenshot tooling is unavailable, record that limitation
separately, but still include HTTP or browser-equivalent evidence that the
primary resources endpoint no longer returns the mock fixture after the real
workRoot is opened.

### Result (80bc4e42) - 2026-05-16

Daemon-served dogfood evidence was recorded in
`ai-docs/.plans/2026-05/16-260516-bug-ws-web-dashboard-live-resource-api-connection.dogfood.md`.
The evidence starts from the default resources endpoint, confirms an empty live
view before open, opens `/Users/kang-sw/devenv`, confirms the canonical endpoint
matches the live open response with mock `workspace-devenv` absent, and
exercises file listing, read-only file read, terminal create/input/output/list,
and close against the real opened workRoot. Interactive screenshot tooling
remains recorded as unavailable.

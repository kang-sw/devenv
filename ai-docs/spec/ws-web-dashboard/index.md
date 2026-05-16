---
title: ws Web Dashboard
summary: Personal ws-aware web dashboard daemon, browser UI, and host-control behavior.
---

# ws Web Dashboard

The ws web dashboard provides a personal browser-accessible control plane for a
host machine. It serves dashboard UI, gates host-control actions behind owner
authentication, and consumes ws runtime state through daemon-owned view models.

## Daemon Foundation {#260515-ws-web-daemon-foundation}

The dashboard daemon starts through the `ws-dashboard serve` command as a
Rust/Axum HTTP server with explicit serving configuration, structured startup
logging, graceful shutdown, and a minimal health surface. The default bind
target is `127.0.0.1`. The daemon does not treat loopback access as
authorization.

On startup, the daemon creates an in-memory high-entropy one-time pairing token
with an explicit expiry policy and exposes the corresponding pairing URL to the
local owner through startup output. The pairing route is the only
unauthenticated browser entrypoint. A successful pairing exchange consumes the
token, installs an HTTP-only owner session cookie with `SameSite=Lax`, and
redirects browser callers to a token-free stable app URL. Missing, invalid,
reused, or expired pairing tokens fail without installing a session cookie and
without redirecting into an authenticated-looking app route.
{#260516-ws-web-dashboard-token-free-pairing-landing}

Authenticated owner sessions have broad host-control authority for dashboard
features, but the daemon remains separate from ws MCP stdio session authority.
The daemon must not make itself the canonical ws MCP root, harness, model
backend, or named-agent session owner.

HTTP routes other than pairing reject unauthenticated requests before handler
execution, including the health route, the placeholder UI route, and fallback
paths. Browser-facing authentication uses a normal HTTP-only session cookie.
The daemon also accepts a narrow bearer authentication path for CLI and smoke
callers against protected HTTP routes; bearer auth supplements browser cookie
navigation and does not replace it.

Browser entrypoints reject clearly invalid Host and Origin values while
preserving ordinary loopback development usage. Future WebSocket upgrade
requests enter the owner-auth gate before any upgrade acceptance; the foundation
does not yet expose WebSocket endpoint behavior.

Serving configuration supports explicit `local`, `tunnel`, and `public` bind
modes. Local mode remains the default and binds to loopback unless the caller
changes the host. Tunnel mode preserves loopback-oriented serving intent for
external tunnel frontends. Non-loopback hosts such as `0.0.0.0` are rejected
unless the caller explicitly selects public mode. Public mode can accept a
non-loopback host only while owner authentication is enabled; bind-mode
acceptance does not relax browser cookie auth, bearer auth, Host/Origin checks,
or WebSocket pre-upgrade auth.

The initial UI route serves a minimal placeholder surface behind owner
authentication. Health output is the exact minimal body `ok\n`; host paths,
cache paths, Git roots, pairing tokens, session values, diagnostics, and wsstate
internals are not URL identity and are not exposed by the health surface.

## Core Resource Vocabulary {#260516-ws-web-dashboard-core-resource-vocabulary}

The dashboard core crate exposes opaque ids and resource path vocabulary for
the first visible hierarchy without exposing host paths as identity. Core
resource paths carry `serverId`, `workspaceId`, `workRootId`, and optional
`instanceId` fields when serialized for dashboard consumers.

The physical directory target is named `workRoot` in the public dashboard core
vocabulary. WorkRoot metadata distinguishes `plainDirectory`, `gitPrimaryRoot`,
and `gitLinkedWorktree`; status values describe whether the remembered root is
online, offline, moved, or inaccessible. Main/sub instance role, instance kind,
and interaction mode values serialize with the same dashboard camelCase naming
contract.

## Resource View-Model Contract {#260516-ws-web-dashboard-resource-view-model-contract}

The dashboard exposes authenticated HTTP view-model APIs for the first visible
resource hierarchy:

```text
server -> workspace -> workRoot -> mainInstance -> subInstance
```

Callers address API resources through opaque ids rather than host paths, Git
roots, wsstate paths, workRoot keys, or runtime session identifiers. The daemon
owns those private identifiers and exposes only authenticated view-model fields
that the browser needs to render navigation, selection, status,
stale/error/loading state, and available actions.

`workspace` means a daemon-discovered project group, not a user-created
category. `workRoot` means the physical directory used as an open, spawn, and
run target. WorkRoots report additive kind metadata for `plainDirectory`,
`gitPrimaryRoot`, or `gitLinkedWorktree`, plus status such as online, offline,
moved, or inaccessible. Primary roots and linked Git worktrees share the same
core workRoot API shape while preserving enough metadata for the UI to
distinguish their repository role and lifecycle affordances.

The API shape preserves the full hierarchy even when the browser later renders
singleton `workspace -> workRoot -> mainInstance` chains as compact rows.
Authenticated callers may observe compactability hints, but compaction is a
presentation policy and not URL identity.

## Mock View-Model Fixtures {#260516-ws-web-dashboard-mock-view-model-fixtures}

The dashboard daemon provides deterministic fixture-backed resource data that
uses the same view-model API contract as live providers. Frontend and contract
tests can render the first visible shell without live wsstate, PTY, named-agent,
harness, or filesystem discovery dependencies.

Fixtures cover singleton chains, multi-root workspaces, plain directories, Git
primary roots, linked Git worktrees, offline or inaccessible workRoots, main
instances, sub instances, stale/error/loading states, and visible action hints.
Protected API route tests verify that fixture-backed dashboard data remains
behind the owner-auth boundary.

## Protected Frontend Shell {#260516-ws-web-dashboard-protected-frontend-shell}

The dashboard daemon serves the first React/TypeScript/Vite browser shell
behind the same owner-auth boundary as other protected dashboard routes. Static
asset serving does not add another unauthenticated top-level route beside
`/pair`; unauthenticated browser requests for the dashboard shell are rejected
before assets or fallback UI are served.

The frontend package provides documented local development and production build
entrypoints that later dashboard slices can reuse. The first shell remains
narrow: it does not implement PTY, editor, document viewer, live workspace
discovery, event streams, named-agent controls, or root picker behavior.

Browser navigation reserves explicit server-scoped routes such as
`/servers/:serverId/...` for dashboard resources, while daemon-owned opaque ids
remain the source of truth and server identity is not hidden inside workspace,
workRoot, or instance ids. Refreshing `/servers` or server-scoped app paths
serves the protected frontend shell through the same owner-auth static route
boundary. {#260516-ws-web-dashboard-server-scoped-browser-routes}

## Inspectable Navigation Shell {#260516-ws-web-dashboard-inspectable-navigation-shell}

The first browser shell renders the resource view-model contract from the
daemon API. It shows server, workspace, workRoot, main-instance, and
sub-instance state; loading, empty, stale, and error states; compact singleton
rows; and a reserved right-side viewer region without implementing the deferred
viewer feature.

Mouse-triggered navigation actions route through command ids so later keyboard
bindings can call the same commands. The shell reserves `^b` to
mean ctrl plus lowercase `b`; full custom keybinding UI remains out of scope.

## WorkRoot Workbench Substrate {#260516-ws-web-dashboard-workroot-workbench-substrate}

The dashboard frontend presents a `left nav | workRoot workbench` shell. The
left navigation selects server, workspace, and concrete workRoot locations,
while each opened workRoot owns a constrained workbench area backed by a
dashboard-owned adapter over the selected layout library.

The workbench uses sibling split groups with compact editor-like tab strips and
dominant pane bodies. Pinned and opened concepts remain dashboard model
concepts: durable surfaces such as agent and persistent terminal views appear as
compact workbench tabs or chips, while transient or support surfaces such as
editor, viewer, diff, diagnostics, logs/events, task view, and inspector
surfaces appear as ordinary workbench tabs. The compact header preserves that
pinned/opened structure without returning to large explanatory rows. Main
instances are durable workRoot-local surfaces. Sub instances are view-only
projections attached to a main instance through badges, popovers, cards, or
drawers rather than independent top-level navigation rows.

Layout attachment identity stays separate from daemon resource identity. Layout
state records arrangement only; daemon APIs and `/servers/:serverId/...`
browser routes keep authoritative server, workspace, workRoot, and instance
identity. Panel close detaches the frontend view by default, while explicit
terminate commands own daemon-backed lifecycle shutdown. PTY/TUI logical
columns do not continuously follow visual drag resizing.

Surface opening follows dashboard-owned placement policy: already-open logical
surface keys focus their existing attachment, opened/support surfaces prefer the
second or later split group, and durable agent or persistent terminal surfaces
prefer the focused group before falling back to the first group.

Visible tabs select the active pane and support frontend-only movement such as
reordering within a split group and moving to another split group. Tab movement
changes browser arrangement state only: floating/popout groups stay disabled,
daemon-backed lifecycle stays separate, and PTY/TUI logical dimensions do not
continuously follow visual drag resizing.

## Dark-First Visual System {#260516-ws-web-dashboard-dark-visual-system}

The dashboard frontend provides a dark-first visual baseline for the protected
browser shell. Callers see a dashboard-specific `DESIGN.md` guide under the
frontend package, semantic theme tokens instead of scattered literal colors,
and a shell reskin that preserves existing resource, loading, stale, error, and
command behavior while presenting a consistent dark operational interface.

The visual system uses `ai-docs/ref/design.md` as a Carbon-inspired density,
geometry, hairline, and component reference rather than as a default light
palette. Desktop and narrow viewport screenshot checks make the resulting
shell inspectable before larger workbench surfaces depend on it.

## Local WorkRoot Discovery Provider {#260516-ws-web-dashboard-local-workroot-discovery-provider}

The dashboard daemon provides a live local discovery provider that maps opened
physical directories into the resource view-model contract. The provider
classifies workRoots as `plainDirectory`, `gitPrimaryRoot`, or
`gitLinkedWorktree`, reports online, offline, moved, and inaccessible states,
and preserves stable daemon-owned identity when a workRoot's discovered kind
changes.

Discovery refreshes through explicit owner actions that invoke the provider,
including opening a workRoot through the root picker backend. Broad filesystem
watching remains out of scope for the first visible substrate.

## Root Picker Empty Directory Creation {#260516-ws-web-dashboard-root-picker-empty-directory-creation}

The dashboard exposes backend support for a cross-platform root picker that
lists filesystem locations as workRoot candidates without turning the browser
into a general file manager. Authenticated owners can open existing plain
directories or Git-backed directories into the dashboard model.

The picker includes only a narrow `Create empty folder` operation for creating
a new workRoot candidate. Generic delete, rename, move, copy, and recursive
folder deletion operations remain unavailable.

## WorkRoot File Listing API {#260516-ws-web-dashboard-workroot-file-listing-api}

The dashboard exposes an authenticated API for listing directories below a
selected workRoot. Responses identify children by daemon-owned workRoot-relative
location data rather than raw host paths, distinguish file and directory
entries, expose basic readability or preview eligibility when cheap, and report
unreadable or inaccessible locations without mutating the filesystem.

Listing requests remain rooted below the selected workRoot. Traversal attempts,
missing paths, files requested as directories, and inaccessible locations return
bounded unavailable or error states without exposing host paths as browser route
identity.

## WorkRoot File Explorer {#260516-ws-web-dashboard-workroot-file-explorer}

The dashboard browser shell renders a selected-workRoot file explorer in the
lower portion of the left navigation area. The explorer supports directory
expansion, explicit refresh, loading, empty, and error states while keeping
server, workspace, and workRoot identity visible above it.

The first explorer surface is navigation-only. It does not offer delete,
rename, move, copy, chmod, recursive folder deletion, or broad file-manager
operations. Readable file open actions may hand off to read-only text pane
behavior when that later surface exists; until then, the explorer does not imply
write-back editing.

## Read-Only File API {#260516-ws-web-dashboard-readonly-file-api}

The dashboard exposes an authenticated API for reading previewable text files
below an opened workRoot. Callers address files by opaque `workRootId` and
workRoot-relative location data from the file listing API. The route rejects
traversal, missing files, directories, unreadable paths, unsupported binary
content, and oversized files with bounded unavailable states.

Successful responses include read-only text content and enough metadata for the
browser to render a stable viewer title, language or extension hint when cheap,
size information, and read-only status without exposing absolute host paths.

## Read-Only Text Pane {#260516-ws-web-dashboard-readonly-text-pane}

The dashboard workbench can open a read-only text pane for a previewable file
under the selected workRoot. The pane renders file content as an inspectable
viewer/editor body and clearly indicates read-only status. Opening the same
file focuses the existing logical pane instead of duplicating it by default.

The text pane does not provide save, dirty-state, formatting, rename, delete,
move, copy, conflict handling, or language-server behavior.

## File Open Placement Policy {#260516-ws-web-dashboard-file-open-placement-policy}

File-open commands from the workRoot file explorer use workbench placement
policy that prefers the second or later split group when available, so active
terminal or future agent work is not displaced. Placement remains browser
arrangement state; file content authorization and preview availability remain
daemon-owned.

## Terminal Registry And PTY Spawn {#260516-ws-web-dashboard-terminal-registry-pty-spawn}

The dashboard daemon owns shell terminal sessions scoped to opened workRoots.
Authenticated owners can create and list live terminal sessions by opaque
terminal ids. Spawns run in the selected workRoot directory and terminal ids are
not process ids or host paths.

Live terminal sessions persist across browser refresh because the daemon owns
their lifecycle. Browser arrangement state controls where sessions are shown,
not whether the daemon session exists.

## Terminal I/O Transport {#260516-ws-web-dashboard-terminal-io-transport}

The dashboard exposes authenticated terminal output, input, and resize
transport for daemon-owned PTY sessions. Unauthenticated callers are rejected
before stream or upgrade acceptance. Resize forwarding remains bounded and does
not continuously rewrite logical terminal dimensions during visual split drag.

## Terminal Pane {#260516-ws-web-dashboard-terminal-pane}

The dashboard workbench renders daemon-owned terminal sessions in terminal panes
for the selected workRoot. Creating a terminal opens or focuses a terminal pane,
and refresh can reconstruct visible terminal panes from daemon live session
state plus browser arrangement where available.

The terminal pane is a shell terminal substrate only; it does not hardcode
Codex, Claude, or other agent presets.

## Terminal Close Terminates Session {#260516-ws-web-dashboard-terminal-close-termination}

Closing a terminal panel explicitly terminates its daemon-owned terminal
session. The first terminal substrate keeps hidden detached restore UX absent;
future confirmation or foreground-process checks may be added without changing
the basic close-as-terminate contract.

## WorkRoot IO Restore Model {#260516-ws-web-dashboard-workroot-io-restore-model}

The dashboard combines daemon-owned live terminal state, read-only file pane
state, and browser workbench arrangement into one restore model for selected
workRoots. Daemon state is authoritative for live terminal existence, while
browser arrangement remains presentation state. File panes restore only when
the file remains previewable; otherwise the pane shows an honest unavailable
state.

## WorkRoot IO Command And Placement Polish {#260516-ws-web-dashboard-workroot-io-command-placement-polish}

WorkRoot IO commands use consistent command ids and placement behavior across
file open, create terminal, focus existing surface, close terminal, and refresh.
Logical targets that are already open focus existing surfaces rather than
duplicating panes.

## WorkRoot IO Dogfood Verification {#260516-ws-web-dashboard-workroot-io-dogfood-verification}

The dashboard verifies the workRoot IO workflow through the daemon-served
frontend: open/select a workRoot, browse files, open a read-only text pane,
create and use a terminal, refresh without losing the terminal, close the
terminal, and inspect desktop and narrow layouts. Verification records exact
tooling blockers when a check cannot run.

## Instance Event Envelope Fixtures {#260516-ws-web-dashboard-instance-event-envelope-fixtures}

The dashboard defines a shared event envelope for instance-scoped streams.
Events reference opaque server, workspace, workRoot, and instance ids from the
resource view-model contract, carry ordered cursor and sequence data,
timestamps, event categories, payload values, and explicit error or end markers.

Deterministic transcript fixtures cover ordinary output, status transitions,
errors, reconnect/backfill, and empty streams so later PTY, named-agent, exec,
diagnostic, viewer, and translation features can reuse one stream shape.

## Authenticated Instance Event Stream Scaffold {#260516-ws-web-dashboard-authenticated-instance-event-stream-scaffold}

The dashboard exposes an authenticated stream route scaffold that serves
fixture-backed instance events before live PTY, named-agent, exec, diagnostic,
viewer, or translation sources exist. Unauthenticated callers are rejected
before stream acceptance or WebSocket upgrade behavior.

Authenticated callers can request events after a cursor and receive
deterministic fixture events without making the dashboard daemon the ws MCP or
named-agent session authority.

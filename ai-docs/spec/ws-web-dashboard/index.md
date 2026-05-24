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
logging, bounded graceful shutdown, and a minimal health surface. The default
bind target is `127.0.0.1`. The daemon does not treat loopback access as
authorization. After the outer server process receives its shutdown signal,
long-lived browser connections such as idle sockets, SSE streams, or WebSockets
may receive a short drain window, but they must not keep the local development
server alive indefinitely.

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
a singleton `workspace -> workRoot` resource tree as one compact workRoot row.
Authenticated callers may observe compactability hints, but compaction is a
presentation policy and not URL identity; the compact row selects the concrete
workRoot id and does not require a main instance.

## Durable WorkRoot Registry And Activation {#260523-dashboard-workroot-registry-activation}

The dashboard exposes known workspace and workRoot membership from a
daemon-local durable registry instead of treating only currently opened
workRoots as the visible resource set. A known workRoot remains visible until a
future explicit forget/remove policy removes it, even when it is currently
missing, inaccessible, moved, or inactive.

> [!note] Planned 🚧 {#260524-dashboard-workspace-root-prune-policy}
> The registry will distinguish owner-managed workspaces from automatically
> detected workRoots. A workspace has a root workRoot anchor, such as an
> owner-added directory or Git root, and may contain discovered child workRoots
> such as linked Git worktrees. If the root workRoot becomes unavailable, the
> workspace remains visible in a disabled or recovery-needed state while any
> child workRoot is still active, where active means activation permits
> targeting and availability is currently usable. This gives callers room to
> reconnect the root or derive a new workspace from a dangling child. Automatic
> pruning removes a workspace only when it has no active workRoots. Explicit
> forget/remove UI remains a separate owner-driven cleanup policy.

WorkRoot view-models separate live availability from user-controlled
activation. `availability` describes the daemon's current filesystem/Git
assessment of whether the workRoot can be used now, with initial public values
for available, missing, moved, inaccessible, and unknown states. `activation`
describes whether the dashboard is currently allowed to target that workRoot
for file, Activity, and terminal APIs, with `online` and `offline` values.
A reachable workRoot with `activation: offline` remains a visible row and is
not the same state as a missing or inaccessible workRoot.

Existing opened-workRoot persistence migrates into the registry as known
membership with `activation: online`, preserving current restart behavior.
Newly discovered sibling workRoots may enter the same registry with
`activation: offline` while remaining visible in the resource tree.

Resource views, activation actions, and protected route gates derive activation
from the same registered workRoot id regardless of whether the visible row is
reached as an explicitly opened root or as an automatically discovered linked
workRoot. If one physical workRoot is visible through both paths, callers see
one activation state and the same online/offline behavior across navigation,
Activity, file, and terminal surfaces.
{#260524-dashboard-workroot-registry-wide-activation-lookup}

Authenticated route behavior distinguishes registry membership and current
operability. Unknown workRoot ids return not-found responses. Known workRoots
with offline activation return a bounded offline response. Online workRoots
whose availability has degraded return a bounded unavailable response without
exposing host paths. Terminal HTTP routes and already-open terminal WebSockets
re-check the owning workRoot's activation and availability before accepting
input, resize, close, or output/backfill access. Online/offline transitions are
dashboard commands with logical targets so mouse controls and later keybindings
share the same command path.

Explicit resource refresh recomputes availability from filesystem/Git without
changing activation. While the dashboard is open, bounded polling refreshes
known workRoot availability through the same canonical resource endpoint so
external filesystem or Git worktree changes can become visible. Polling is not
the sole correctness mechanism: explicit refresh remains deterministic, polling
does not become browser-side resource authority, overlapping refresh requests
are suppressed, stale poll results do not overwrite newer open or activation
resource views, and refresh failures keep the last known resource tree visible.
Filesystem watchers, if added later, act only as refresh hints.

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

Normal daemon operation uses live opened workRoot state as the primary resource
authority for authenticated dashboard resource loads. Fixture-backed resources
remain available for deterministic tests and explicit fixture contexts, but the
production resources endpoint does not silently return the mock workspace when
no workRoot has been opened. {#260516-ws-web-dashboard-live-resource-authority}

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
daemon API. It shows server, workspace, and workRoot location state; loading,
empty, stale, and error states; compact singleton rows; and a reserved
right-side viewer region without implementing the deferred viewer feature. Each
workspace with one workRoot renders as one compact left-nav row selected by the
concrete workRoot id, without depending on main/sub instance presence.
When the workspace and workRoot labels are identical, the compact row displays
that label once; distinct labels remain visible as a workspace/workRoot pair.
Workspaces with multiple workRoots continue to show separate workspace and
workRoot rows. Main/sub instances remain workbench surfaces or projections
rather than default recursive left-nav rows.

User-visible dashboard controls expose stable command ids so later keyboard
bindings can target the same behaviors. Representative visible controls route
mouse or click behavior through a shared dashboard command dispatch path, with
a command observer preserving recent-command evidence and programmatic dispatch
using the same command ids as click handlers. Command payloads use logical
dashboard targets such as opaque resource ids, pane ids, logical surface keys,
activity ids, or terminal ids; host paths, cache paths, stream paths, pids, and
backend session paths are not command identity. Terminal raw byte input remains
the narrow exception because shell input fidelity must not be forked through
dashboard commands. The shell reserves `^b` to mean ctrl plus lowercase `b`;
full custom keybinding UI remains out of scope.
{#260523-ws-dashboard-single-workroot-nav-collapse}

## WorkRoot Workbench Substrate {#260516-ws-web-dashboard-workroot-workbench-substrate}

The dashboard frontend presents a `left nav | workRoot workbench` shell. The
left navigation selects server, workspace, and concrete workRoot locations,
while each opened workRoot owns a constrained Dockview-backed workbench area
behind a dashboard-owned adapter.

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

Workbench panes do not add a second generic title/status chrome below the
pinned/opened tab rows. The tab rows provide visible surface identity and
selection, while pane-local content or controls provide any useful
surface-specific status.

Layout attachment identity stays separate from daemon resource identity. Layout
state records arrangement only; daemon APIs and `/servers/:serverId/...`
browser routes keep authoritative server, workspace, workRoot, and instance
identity. Panel close follows dashboard surface policy: reversible browser
views detach immediately, while live terminal or agent tab closes require
explicit confirmation before invoking their daemon-backed lifecycle behavior.
PTY/TUI logical columns do not continuously follow visual drag resizing.

Surface opening follows dashboard-owned placement policy: already-open logical
surface keys focus their existing attachment, opened/support surfaces prefer the
second or later split group, and durable agent or persistent terminal surfaces
prefer the focused group before falling back to the first group.

Visible tabs select the active pane and support frontend-only movement such as
reordering within a split group and moving to another split group. Tab movement
changes browser arrangement state only: floating/popout groups stay disabled,
daemon-backed lifecycle stays separate, and PTY/TUI logical dimensions do not
continuously follow visual drag resizing.

Dockview owns the visible workbench group, tab, split-sizing, and pane
attachment layout. Dashboard-owned policy still owns surface identity,
duplicate-open focus, placement, close behavior, restore sanitization, and the
choice to flatten pinned/opened row concepts into Dockview-compatible tab
metadata when a two-row custom tab shell would compete with Dockview ownership.
Synchronization back into Dockview must be group-local: an inactive split
group's selected tab is not treated as inactive merely because another split has
global focus, and pane parameter updates must be keyed by stable content
revisions instead of React node identity so unrelated refreshes do not remount
scrolling pane bodies.

Dockview-created split drops become durable dashboard workbench groups instead
of snapping back to a fixed `primary`/`support` pair. Each opened workRoot owns
its own dynamic group, pane-order, and active-pane state. An opened workRoot
starts from two dynamic groups: terminals prefer group 1, editor/read-only file
panes prefer group 2, editor/file opens create group 2 when only group 1
exists, and groups 3+ remain user-created groups without automatic placement
unless the user explicitly targets them through later policy.

Workbench tabs provide polished lifecycle affordances while keeping Dockview as
the visible tab owner. Pinned/opened hierarchy is visible through
Dockview-compatible tab metadata and pinned-left badge or chip presentation.
Tabs expose hover-only close buttons. Live terminal or agent closes use a
cursor-near `Yes`/`No` confirmation popover; reversible views such as read-only
editor previews, diagnostics, and resource views close immediately and use the
same deterministic focus handoff as ordinary tab close. Opened workRoots do
not show mock or default panes when no live or user-opened surface exists.

### WorkRoot Activity Projection {#260517-ws-dashboard-workroot-activity-projection}

The dashboard exposes a workRoot-owned runtime activity projection for opened
workRoots. Authenticated callers request it through
`GET /api/dashboard/work-roots/{workRootId}/activity`. The projection summarizes
read-only named-agent activity
from daemon-owned wsstate and wsagent state without making browser callers read
cache files or host paths directly.

The projection reports bounded status for named agents, including identity,
backend or model metadata when available, current-call state, last-call timing,
and unavailable or diagnostic states for stale or malformed records. It does not
provide agent control actions such as start, interrupt, cancel, erase, or retry.
Running command activity remains absent until the async exec job model exists.

### WorkRoot Activity Top-Bar Badge {#260517-ws-dashboard-workroot-activity-topbar-badge}

Opened workRoot top bars show a compact activity badge in the existing badge
row. The badge summarizes named-agent activity counts for the selected workRoot
and opens or focuses the detailed WorkRoot Activity pane.

Adding activity summary does not add a new top-bar row or increase the top-bar
height. Under constrained widths the badge compacts, truncates, or hides
secondary text rather than wrapping the toolbar and reducing workbench body
space. Switching workRoots must not briefly render the previous workRoot's
activity state.

### WorkRoot Activity Pane {#260517-ws-dashboard-workroot-activity-pane}

The WorkRoot Activity pane is a reversible workbench surface showing the
selected workRoot's detailed runtime activity projection. Closing it detaches
the browser view immediately without confirmation and without changing daemon
agent state.

Opening the activity detail from the top-bar badge focuses an existing activity
pane for the selected workRoot or creates one through the workbench support
split placement policy. New Activity panes prefer the second/support split when
available or creatable, while duplicate opens focus the existing Activity pane
in whatever split currently owns it. Activity pane close remains reversible and
has no daemon-side effect.

The pane displays named-agent projection rows and an explicit empty Running
Commands section. Real running-command rows remain absent until the async exec
job model exists.

While the Activity pane is open, the dashboard refreshes recently updated
named-agent rows and merges them into the existing projection so newly
registered or called agents appear without a browser reload. The full projection
remains available for the initial selected-workRoot fetch.

## Activity Console Read Model {#260521-ws-dashboard-activity-console-read-model}

The dashboard exposes a workRoot-scoped Activity Console read model that
combines a live/latest Activity Feed snapshot with selected activity transcript
backfill. The existing workRoot Activity endpoint returns selectable Activity
Items for the opened workRoot while preserving a compatibility named-agent
projection for the current Activity pane. A per-item transcript endpoint
returns normalized Transcript Blocks for the selected item. Named agents are the
first supported source, but the public shape stays source-neutral so main-agent
sessions, exec jobs, diagnostics, and later readable activity can fit the same
console contract.

Activity Feed snapshots report enough item state for compact ribbon rendering
without requiring a transcript fetch: stable activity id, kind, label, status,
live/attention flags, timing fields, source display metadata, transcript
availability, bounded diagnostics, selected item hint, feed cursor, and update
mode. Ordering favors active, live, attention, blocked, failed, and recently
updated activity before using alphabetical order as a tie-breaker.

Transcript backfill returns bounded normalized blocks rather than backend-native
cache records, raw session JSON, stdout/stderr paths, or file contents. Each
block carries a cursor, timestamp when available, a render kind such as user,
assistant, tool call/result, status, error, or output, and degraded-state
markers. Cursor, block-count, and byte-count bounds keep transcript reads
finite and make unknown activity ids, unavailable sources, empty transcripts,
and malformed records explicit response states instead of whole-feed failures.
Transcript reads default to the latest bounded tail window for the selected
activity. Older transcript history pages backward from the current earliest
loaded cursor so the UI can prepend older blocks when the user scrolls upward,
without forcing an initial read from the beginning of a long transcript.

Browser callers continue to address the model by opaque `workRootId` and
activity id. Responses must not expose host paths, cache paths, backend session
ids, process ids, stdout/stderr paths, stream paths, or backend-native
transcript paths. The read model remains read-only and does not add agent start,
interrupt, cancel, erase, retry, or exec-job control actions.

## Activity Console UI Shell {#260521-ws-dashboard-activity-console-ui-shell}

The WorkRoot Activity pane renders a reusable read-only Activity Console
instead of a vertical named-agent card dump. The console combines a horizontal
Activity Ribbon for live/latest items with a selected Transcript Block viewer
below it, using the Activity Console read model as its route-backed source.

Ribbon items use a compact three-line shape: small source discriminator text, a
primary name/title line, and small status/recency text. Source discriminator
text identifies the activity channel such as `agent.codex`, `agent.claude`, or
`cmd.exec` rather than repeating the primary title. The status row includes the
current activity status plus relative update time when known; completed activity
may also show bounded elapsed duration when space allows. The text area stays
compact, truncates instead of wrapping, and the ribbon scrolls horizontally at
constrained desktop widths. Live, active, and attention-worthy items use
semantic active styling, and a small short-lived green breathing indicator may
mark newly updated or locally dirty items until the user selects or otherwise
acknowledges them. The Activity Console body does not render a separate summary
chip row above the ribbon; the ribbon is the primary item selector.

The browser may keep a local acknowledgement watermark per workRoot/activity
item. On initial feed load it compares that local watermark with daemon item
timestamps or cursors to mark newly updated items dirty. Selecting or explicitly
acknowledging an item clears only browser-local dirty state; the daemon does not
gain read-receipt authority.

Selecting a ribbon item renders normalized transcript blocks. Agent activity
renders as action-unit blocks where dialogue and assistant output are expanded
by default, while tool calls, MCP activity, and command runs default to one-line
summaries with inline detail expansion. Compact summaries prefer bounded
semantic content from normalized safe fields and first-line text over generic
category titles, so tool calls can show the tool name and argument-size hint,
tool results can show outcome/status/byte hints, and degraded records can show
their omission reason without exposing raw native payloads. Exec activity
renders as terminal-style output. Transcript views follow the tail by default
for newly selected or live updated activity. When the user scrolls away from the
tail, the browser preserves that scroll position across feed refreshes,
transcript refreshes, selected-transcript invalidations, and workbench split
rerenders until the user returns to the tail. Initial selected transcript loads
start from the latest tail window, not the oldest block. Older transcript
history is loaded when the user scrolls near the top and is prepended while
preserving the user's visible position. Explicit refresh or load-more controls
remain available for fallback and error states rather than being the primary
navigation path.

> [!note] Implementation Gap · 2026-05-23
> Missing behavior: expanded dialogue and assistant transcript text is rendered
> as plain preformatted text rather than markdown. The dashboard should define a
> shared markdown rendering component first, then apply that component
> consistently across Activity Console messages and other project surfaces that
> render trusted normalized markdown.

Visible Activity Console controls expose stable command ids and route their
clicked behavior through the dashboard command dispatch path so later keyboard
bindings can invoke the same behavior. The shell remains read-only, does not add
agent control buttons, and does not consume live SSE/watch streams until the
live UX child implements that behavior.

## Activity Console Watch Stream {#260521-ws-dashboard-activity-console-watch-stream}

The dashboard exposes a workRoot-scoped read-only Activity Console event
stream for feed and transcript invalidations:

```text
GET /api/dashboard/work-roots/{workRootId}/activity/events?after={cursor}
```

The stream is owner-authenticated before any transport is accepted. It uses SSE
because Activity Console updates are read-only; a different transport requires
a recorded bidirectional need. Subscriptions are scoped to the requested opened
workRoot instead of every remembered or opened root. The current backend stream
announces `pollFallback` mode and uses bounded polling to produce event updates;
a later native watcher can switch to `watch` mode without changing the public
payload vocabulary.

Stream events carry source-neutral Activity Feed semantics rather than
filesystem or backend-native payloads. Expected event categories include item
upsert/removal, transcript invalidation with transcript cursor metadata,
snapshot invalidation for overflow or watch resets, mode changes between watch
and polling fallback, and heartbeats. Event cursors let reconnecting callers ask
for events after the last observed cursor, but the stream may intentionally
force a snapshot refetch when events were missed or coalesced.

The fallback stream normalizes observed agent changes, missing directories,
agent erasure, and recreated agent directories without leaking cache paths, raw
file paths, backend-native transcript records, process ids, session ids,
stdout/stderr paths, or file contents. The stream remains read-only and does
not make the frontend responsible for consuming live updates; frontend merge
and stale-root behavior belongs to the live UX feature.

## Activity Console Live UX {#260521-ws-dashboard-activity-console-live-ux}

The Activity Console frontend subscribes to the workRoot activity event
stream while the console is visible or otherwise actively used. Stream handling
merges source-neutral events into the route-backed Activity Console state
without making browser state authoritative over daemon activity.

`itemUpserted` and `itemRemoved` events update the current feed while preserving
selection when the selected item still exists. `snapshotInvalidated` causes a
bounded read-model refetch instead of browser-side reconstruction of missed
events. `transcriptUpdated` refreshes or backfills transcript state only when
the affected activity is the currently selected item. `modeChanged` transitions
the frontend between stream-driven updates and bounded fallback polling; always
on full-list polling is not the normal live mode.

The frontend ignores events for stale workRoots after the user switches
roots or closes the console, tears down subscriptions when the console is no
longer visible, and keep the static UI shell usable when the stream is
unavailable. Streamed or polled updates newer than the browser-local
acknowledgement watermark may turn on the ribbon dirty cue; selecting or
acknowledging an item clears only local dirty state and sends no daemon read
receipt.

Live UX adoption remains read-only. It does not expose raw SSE payloads, backend
paths, cache paths, source ids, or control actions in browser UI state.

## Activity Console Transcript Expansion {#260522-ws-dashboard-activity-console-transcript-expansion}

The Activity Console transcript backend supports additional daemon-owned
transcript source adapters behind the existing `ActivityTranscript` and
`TranscriptBlock` contracts. Browser callers continue to request selected
activity transcripts by opaque workRoot and activity ids; they never receive
backend session paths, cache paths, host paths, pids, session ids,
stdout/stderr paths, stream paths, native transcript paths, or backend-native
record formats.

Native backend transcript parsing starts only from fixture-backed formats whose
shape can be verified without invoking a live backend. Codex native session
JSONL is the first supported native source. Claude and Gemini native transcript
handling remain deferred unless their formats are similarly documented or
fixture-backed. Missing, unreadable, malformed, or unsupported native transcript
records degrade individual blocks or source status where possible instead of
failing the whole selected activity transcript, and the existing `output.md`
fallback remains available.

Source adapters normalize dialogue, assistant output, tool calls, tool results,
status/error entries, prompt/user messages, interruptions, handoff/status
records, MCP/tool activity, patch/apply outcomes, and command/output-like
records into bounded `TranscriptBlock` values when their fixture-backed native
shape is known. Raw backend JSON or markdown may be adapter input but is not the
browser contract. Low-value native telemetry may be skipped instead of rendered
as transcript noise. Remaining unsupported records degrade into bounded
structural summaries with omission reasons, never raw JSON, private record
strings, payload snippets, paths, session ids, or tool output. Exec transcript
source integration remains blocked until the async exec output reader model
exists.

The backend continues to use feed-level `transcriptUpdated` invalidations plus
bounded selected backfill for live transcript updates. It does not expose a
selected-activity block-level transcript event stream until block append/update
behavior has a clear UX win.

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

The frontend visual guide also defines a dashboard-local building-block
vocabulary for frames, panels, panes, toolbars, rows, chips, badges, state
surfaces, document surfaces, and code blocks. Current high-impact surfaces
consume that vocabulary while preserving their existing commands and data:
left navigation, open-workRoot chrome, workbench toolbar and Dockview tabs,
Activity Console ribbon/transcript blocks, read-only text panes, and common
empty/loading/error surfaces.

> [!note] Planned 🚧 {#260524-dashboard-icon-first-chrome}
> The dashboard chrome will present left navigation, file explorer, and
> workRoot topbar controls with conventional icon-first affordances, accessible
> names, and reduced visible metadata clutter. High-signal status remains
> visible, secondary diagnostics move to low-weight surfaces or overflow menus,
> and all visible or overflowed actions continue to route through the existing
> dashboard command model.

## Browser UI Acceptance Gate {#260516-ws-web-dashboard-browser-ui-acceptance-gate}

Dashboard frontend changes that affect visible browser behavior provide a
browser-level acceptance gate against the daemon-served production frontend
after owner pairing. The gate exercises the workRoot UI as a user sees it:
opening a real workRoot, browsing files, creating terminals, switching terminal
tabs, sending terminal input, observing terminal output, and checking pane
layout at recorded viewport sizes.

The frontend package exposes this gate through `npm run test:browser`. The gate
builds the production frontend, serves it through the dashboard daemon, pairs
as owner through the startup pairing URL, and records textual evidence plus
regenerable screenshot artifacts outside tracked source.

The gate includes viewport containment checks for long file explorer content:
expanding a large tree must not make the top-level document scroll or push the
dashboard footer out of view, and overflow must stay inside the explorer region.

The browser gate proves the live terminal path uses an owner-authenticated
WebSocket connection instead of periodic output polling while connected. It
covers owner pairing, WebSocket connection, input fidelity, ANSI/control
rendering, resize behavior, close-as-terminate, reconnect or reload
reconstruction, and timing evidence showing local keystroke echo is no longer
bounded by the former polling interval.
{#260516-ws-web-dashboard-terminal-websocket-browser-gate}

For workbench layout changes, the browser gate also proves that the visible
workbench is Dockview-backed rather than a parallel custom tab/split shell. The
assertion checks for the dashboard's stable Dockview owner marker and Dockview
DOM beneath it, and it rejects the retired `.workbench-splits > .workbench-group`
layout as the visible workbench authority.

Workbench split browser evidence verifies that a Dockview split-drop preview
corresponds to durable dashboard behavior: dragging a tab into a new split
target creates or maps a dashboard group, the pane remains there after React
synchronization, ordinary file/terminal interactions still work in the
resulting layout, and opening a second workRoot does not leak the first
workRoot's user-created groups or active panes. Split-scroll evidence also
keeps a scrolled pane away from the top across refresh-driven synchronization.

Workbench tab polish evidence is browser-level Playwright evidence against the
daemon-served frontend. It covers hover-only close affordances, terminal and
agent close confirmation popover cancel/confirm paths, immediate close for
reversible panes, pinned/opened badge or chip presentation, preview-to-pinned
file behavior, and default spawned-daemon agent close coverage. The
implementation workflow also runs a post-implementation frontend-design
verification and autonomous tweak pass before ordinary implementation review,
then reruns the relevant browser evidence.

Read-only text pane scroll containment and terminal input fidelity evidence are
browser-level Playwright evidence against the daemon-served frontend. The gate
covers long read-only file scrolling without top-level document scroll,
shell-visible `ctrl-u` and `ctrl-w` line-editing behavior, WebSocket input
frames for those controls, committed Hangul text input reaching the shell, and
a synthetic IME composition guard proving that composition-in-progress fallback
keystrokes are not forwarded as raw terminal input.

Pure TypeScript helper tests, Vite builds, route tests, curl evidence, and
fixture-only dogfood do not by themselves close UI-facing dashboard work. When
automated browser tooling cannot run, the verification artifact records exact
manual browser steps, viewports, screenshot or trace paths when generated, and
pass/fail observations.

## Deterministic Terminal Endpoint Harness {#260516-ws-web-dashboard-terminal-deterministic-endpoint-harness}

The daemon-served browser acceptance harness supports deterministic daemon
endpoints for terminal portability verification. Spawned mode can run the
dashboard daemon on an explicit host, bind mode, port, daemon binary path, and
static asset directory through `WS_DASHBOARD_DAEMON_HOST`,
`WS_DASHBOARD_DAEMON_BIND_MODE`, `WS_DASHBOARD_DAEMON_PORT`,
`WS_DASHBOARD_DAEMON_BIN`, and `WS_DASHBOARD_STATIC_DIR`. External mode can
attach the browser gate to an already-running base or pairing URL through
`WS_DASHBOARD_DAEMON_BASE_URL` or `WS_DASHBOARD_DAEMON_PAIRING_URL`. When the
daemon runs on a different host from Playwright, the gate can use
`WS_DASHBOARD_TEST_WORKROOT` to open a fixture path that exists on the daemon
host instead of creating a local Playwright-host temporary directory. Browser
checks that need a second opened workRoot can use
`WS_DASHBOARD_TEST_SECOND_WORKROOT`; when attaching to an external daemon
without that second reachable path, only the second-root isolation substep is
skipped while the rest of the browser gate still runs.

The same browser acceptance flow can target a native Windows daemon running on
remote loopback behind SSH local forwarding. The harness waits for an owner
pairing URL and a reachable HTTP readiness signal before starting browser
assertions. Failures identify the failing layer, such as daemon startup,
forwarding or endpoint reachability, pairing, readiness, or browser
assertions, while redacting private endpoints, hostnames, paths, and pairing
tokens from diagnostics.

## Terminal Cross-Platform Evidence {#260516-ws-web-dashboard-terminal-cross-platform-evidence}

Terminal portability runs record durable evidence for each supported
environment exercised during implementation. The evidence identifies the OS,
shell profile, daemon endpoint mode, forwarding path when used, readiness
signal, browser gate result, terminal commands or fixtures used, and any
explicit OS-scoped limitations. Machine-readable evidence stays in ignored
browser-test artifacts, and tracked dogfood summaries omit private endpoint,
user, host, path, pairing-token, and screenshot details.

Native Windows evidence may use a machine-local SSH host recorded outside
tracked source. If native Windows evidence cannot run, the evidence states the
exact blocker and records the result as an explicit gap instead of treating a
POSIX local gate as native-Windows coverage.

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

The browser shell opens workRoots through an owner-authenticated root picker
modal instead of an always-visible path input. The left navigation exposes an
`Open...` entrypoint; the modal is an explorer-style folder selection dialog
with local Back/Forward history, Up and Refresh actions, an address field,
platform-aware built-in places derived from daemon-owned data, a details-style
current-folder row list, and footer controls for opening the selected or typed
path. Directory rows support keyboard-friendly selection and row action, and
closing the modal restores focus to the opener. Opening a selected or exact
directory continues through the `workRoot.open` command path, open-workRoot API,
daemon-opened workRoot id reconciliation, and canonical resource refresh flow.
The modal may expose only the existing single-segment `Create empty folder`
action, not broad file-manager operations. Host paths remain authenticated
picker/open request data rather than loggable command payload fields.
{#260524-ws-dashboard-root-picker-modal}
{#260524-ws-dashboard-react-aria-root-picker-pilot}

Authenticated owners can pin and unpin root picker directories in the picker
sidebar. Pinned directories are stored in daemon-local dashboard persistence,
are visually distinguished from built-in places, and remain selection or
navigation affordances only. Unavailable pinned directories stay visible in a
degraded state so the owner can remove them without the picker exposing private
diagnostics or silently creating, opening, deleting, renaming, or moving
filesystem resources. Pin and unpin controls keep host paths as authenticated
request data rather than loggable command payload fields.
{#260524-ws-dashboard-root-picker-pins}

After an authenticated owner opens a workRoot, the browser-visible resource
tree refreshes from the canonical dashboard resources endpoint and selects the
real opened workRoot instead of continuing to present mock workspace state.
Open-workRoot responses may update the view immediately, and successful
responses include an `x-ws-dashboard-opened-work-root-id` header identifying
the daemon-owned id for the requested root. The resources endpoint remains the
canonical source for subsequent refreshes.
{#260516-ws-web-dashboard-open-workroot-resource-refresh}

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

The explorer presents conventional tree/list affordances that visibly
distinguish files from directories, make expansion and refresh controls
recognizable, keep the selected workRoot identity visible, and avoid hidden or
nonstandard interactions while staying read-only. Long expanded trees scroll
inside the explorer region instead of growing the whole browser document.
{#260516-ws-web-dashboard-file-explorer-conventional-affordance}

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
File explorer single-click opens or replaces one read-only preview tab for the
selected workRoot. Double-click pins that file as a stable opened tab that
later preview opens do not replace. Reopening an already pinned file focuses
that pinned tab.

The text pane does not provide save, dirty-state, formatting, rename, delete,
move, copy, conflict handling, or language-server behavior.

Long read-only file content scrolls inside the text pane without moving the
top-level browser document, displacing dashboard chrome, or requiring a future
editor replacement to prove containment.
{#260517-ws-dashboard-readonly-text-scroll-containment}

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

The dashboard exposes authenticated terminal output, input, status, and resize
transport for daemon-owned PTY sessions. Unauthenticated callers are rejected
before stream or upgrade acceptance. Resize forwarding remains bounded and does
not continuously rewrite logical terminal dimensions during visual split drag.

Live browser terminal I/O uses an owner-authenticated WebSocket as the primary
transport for daemon-owned PTY sessions. The WebSocket attaches to existing
opaque terminal ids after owner auth, carries ordered PTY output, status, and
exit data to the browser, and carries raw input plus bounded resize requests
back to the daemon. If the owning workRoot goes offline or becomes unavailable,
the WebSocket stops accepting client input and stops sending buffered or live
PTY output. HTTP output transport remains available for initial replay, reload
reconstruction, deterministic tests, or fallback, but the normal connected
xterm path does not depend on periodic output polling.
{#260516-ws-web-dashboard-terminal-websocket-transport}

## Terminal Pane {#260516-ws-web-dashboard-terminal-pane}

The dashboard workbench renders daemon-owned terminal sessions in terminal panes
for the selected workRoot. Creating a terminal opens or focuses a terminal pane,
and refresh can reconstruct visible terminal panes from daemon live session
state plus browser arrangement where available.

The terminal pane is a shell terminal substrate only; it does not hardcode
Codex, Claude, or other agent presets.

Terminal tab labels behave as selectable workbench tabs for every visible
terminal session. Opening a real workRoot shows an explicit empty workbench
state or a live daemon terminal surface, never a mock or placeholder terminal.
Selecting a terminal focuses only that session, and terminal input and output
do not cross between sessions.
{#260516-ws-web-dashboard-terminal-tab-selection-and-empty-initial-state}

## Browser Terminal Emulator Behavior {#260516-ws-web-dashboard-browser-terminal-emulator-behavior}

The browser terminal pane behaves as a real terminal emulator surface for a
daemon-owned PTY. PTY output is delivered into the terminal emulator so ANSI
color and control sequences render as terminal behavior rather than raw text.
Keyboard input originates from the focused emulator surface and reaches the
corresponding daemon terminal session.

The terminal fills the available workbench pane and fits or resizes from
measured container dimensions while staying within the daemon PTY size
contract. Resize forwarding remains bounded; visual split dragging does not
continuously rewrite logical PTY dimensions.

Terminal rendering prefers a Powerline/Nerd Font-capable monospace stack when
available, with ordinary monospace fallbacks. HTTP polling is suppressed while a
terminal WebSocket is connecting or connected; fallback polling avoids idle
terminal state churn, discards stale in-flight poll results after socket attach
or cursor advancement, and uses bounded per-terminal in-flight requests.

The browser terminal emulator preserves byte-stream input behavior for ordinary
shell editing and interactive control keys. Acceptance includes Backspace,
left/right cursor movement, command history navigation, Ctrl-C, Ctrl-D or EOF
where safe, Ctrl-L or clear-screen behavior, paste, and ordinary prompt editing
inside a real shell.
{#260516-ws-web-dashboard-terminal-websocket-input-fidelity}

Focused terminal panes preserve native terminal input fidelity for committed
Hangul text, IME fallback guarding, and shell line editing. Committed Hangul
text reaches the shell through the live terminal path,
composition-in-progress keystrokes are not forwarded as raw bytes by fallback
browser handlers, and shell editing controls such as `ctrl-u` and `ctrl-w`
produce their native shell-visible effects through the live terminal path.
Focused terminal panes keep browser focus on the xterm input target across
ordinary input, Enter, shell output, and committed text input unless the owner
interacts outside the terminal surface.
{#260517-ws-dashboard-terminal-ime-and-line-editing-fidelity}

## Terminal Shell Selection Portability {#260516-ws-web-dashboard-terminal-shell-selection-portability}

Dashboard terminal spawning has an explicit, testable shell-selection contract
across supported platforms. Unix-like platforms use `$SHELL` or the `/bin/sh`
fallback. Native Windows prefers `pwsh.exe`, then `powershell.exe`, then
`%COMSPEC%`, and finally the `cmd.exe` fallback. The selection contract is
testable independently from the compile-time host platform so Unix and Windows
fallback behavior can be verified on any developer machine.

Shell spawn failures stay bounded to recoverable diagnostics. Authenticated
terminal creation may report that terminal spawning failed, but private
workRoot host paths are not exposed to unauthenticated callers.

## Platform-Aware Terminal Command Helpers {#260516-ws-web-dashboard-terminal-platform-command-helpers}

Terminal tests and browser acceptance gates express portable terminal intent
through platform-aware helpers rather than shared POSIX command strings.
Acceptance behaviors such as echo, line editing, paste, clear screen,
interrupt, resize, ANSI/control rendering, scroll output, and terminal
isolation map to shell-appropriate commands for Unix shells, `cmd.exe`, and
PowerShell where practical.

External daemon browser gates require an explicit remote command profile
through `WS_DASHBOARD_TERMINAL_SHELL_PROFILE` or target platform hint through
`WS_DASHBOARD_TERMINAL_PLATFORM`, so a locally running Playwright process does
not silently use POSIX commands against a remote native-Windows shell. Any
behavior that cannot be made equivalent on a supported platform carries an
explicit OS or shell limitation and is not presented as native-Windows evidence.

The terminal surface keeps scrolled output and alternate-screen/fullscreen TUI
content within the visible terminal box: the active bottom row must remain
fully visible, and fitted xterm rows are trimmed when the rendered screen would
otherwise exceed the available surface.

## Terminal Close Terminates Session {#260516-ws-web-dashboard-terminal-close-termination}

Closing a terminal panel explicitly terminates its daemon-owned terminal
session after inline `Yes`/`No` confirmation near the close action. Cancel
leaves the terminal open and focus coherent; confirm preserves the
close-as-terminate behavior. Hidden detached restore UX remains absent.

## WorkRoot IO Restore Model {#260516-ws-web-dashboard-workroot-io-restore-model}

The dashboard combines daemon-owned live terminal state, read-only file pane
state, and browser workbench arrangement into one restore model for selected
workRoots. Daemon state is authoritative for live terminal existence, while
browser arrangement remains presentation state. File panes restore only when
the file remains previewable; otherwise the pane shows an honest unavailable
state. The daemon persists the owner's opened workRoot paths in local dashboard
state and seeds the live resource view from that remembered list on startup.
Remembered roots re-run normal discovery instead of bypassing moved, offline,
inaccessible, primary-root, or linked-worktree classification. Auth sessions,
live terminal process survival, Activity acknowledgement state, and exact
browser workbench arrangement remain outside the restore model.

Browser-visible terminal tab descriptors can restore after daemon restart as
newly created daemon terminal sessions attached to the remembered workRoot. The
restore descriptor carries title plus a workRoot-relative cwd hint, but it does
not treat old daemon terminal ids or PTY processes as resumable state.
{#260523-ws-dashboard-terminal-tab-restore}

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

WorkRoot IO acceptance verification starts from the default dashboard resource
load, opens or selects a real workRoot, and proves the browser-visible resource
tree, file navigation, read-only text pane, and terminal session all operate
against that real workRoot rather than mock fixtures.
{#260516-ws-web-dashboard-live-resource-dogfood-verification}

WorkRoot IO dogfood includes browser-level evidence from the daemon-served
production frontend after owner pairing. The artifact records the daemon
command, browser automation or manual browser steps, viewport sizes, terminal
commands used to verify color/control handling, generated screenshot or trace
paths when present, and explicit pass/fail checks for the known UI failures.
{#260516-ws-web-dashboard-browser-workroot-io-dogfood-evidence}

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

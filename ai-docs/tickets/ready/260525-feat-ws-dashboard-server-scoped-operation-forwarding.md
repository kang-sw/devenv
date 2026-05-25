---
title: ws dashboard server-scoped operation forwarding
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260525-feat-ws-dashboard-multi-server-gateway: introduced linked-server registry, link-auth, selected-server resource forwarding, and server-first navigation
  260525-feat-ws-dashboard-endpoint-linked-server-add: exposed endpoint-first linked server add flow and revealed non-resource API locality during Windows dogfood
  260514-research-ws-web-dashboard-direction: longer-range remote hardening and server federation direction
spec:
  - 260525-ws-dashboard-server-scoped-operation-forwarding
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard server-scoped operation forwarding

## Background

Endpoint-linked servers can now be added and selected, and
`GET /api/dashboard/servers/{serverId}/resources` forwards remote resource
snapshots through the local dashboard gateway. Dogfood against a remote Windows
daemon showed that this is not enough for a transparent multi-server dashboard:
most follow-up operations still call local-only routes such as
`/api/dashboard/work-roots/{workRootId}/files`,
`/api/dashboard/root-picker`, `/api/dashboard/work-roots/open`, Git toolbar
routes, Activity routes, and terminal routes.

The product model should be: the browser talks only to the local gateway, while
the selected `serverId` determines whether the local gateway handles the
operation locally or forwards it to a linked daemon with the memory-only bearer
token. A user should be able to attach a remote Windows daemon, click that
server's folder/open-root affordance, browse remote paths, open a remote
workRoot, browse and edit remote files, view Activity, use Git controls, and
spawn terminal panes without routes silently falling back to the local host.

## Decisions

- Make `serverId` part of every operation whose target is a server, workspace,
  workRoot, instance, file, Activity item, terminal, or host filesystem path.
- Keep existing non-server-scoped routes as local compatibility aliases for
  `server-local`; new frontend calls should prefer canonical server-scoped
  routes.
- Route all browser-to-remote traffic through the local gateway. The browser
  must not call the linked endpoint directly.
- For linked servers, forward requests with the linked server's memory-only
  bearer token and preserve upstream status/error shape as much as practical.
- Rewrite returned `DashboardResourcesView` and nested `ResourcePath.serverId`
  values to the linked server id, matching the existing selected-server
  resources forwarding behavior.
- Do not solve credential persistence, automatic deployment, public exposure
  hardening, or cross-server federation beyond one local gateway forwarding to
  remembered linked servers.

## API Inventory

Already server-aware:

- `GET /api/dashboard/servers`
- `GET /api/dashboard/servers/{serverId}/resources`
- `POST /api/dashboard/servers/{serverId}/link-auth`
- `POST /api/dashboard/servers/{serverId}/tunnel/reconnect`
- `POST /api/dashboard/servers/link`

Must become server-aware for transparent linked-server operation:

- Root picker and open WorkRoot:
  `/api/dashboard/root-picker`,
  `/api/dashboard/root-picker/directories`,
  `/api/dashboard/root-picker/pins`,
  `/api/dashboard/work-roots/open`
- WorkRoot resource mutations:
  `/api/dashboard/work-roots/{workRootId}/activation`,
  `/api/dashboard/workspaces/{workspaceId}`
- Git worktree add:
  `/api/dashboard/workspaces/{workspaceId}/git-worktree-add/options`,
  `/api/dashboard/workspaces/{workspaceId}/git-worktree-add/preview`,
  `/api/dashboard/workspaces/{workspaceId}/git-worktree-add`
- Git toolbar:
  `/api/dashboard/work-roots/{workRootId}/git/status`,
  `/api/dashboard/work-roots/{workRootId}/git/branches`,
  `/api/dashboard/work-roots/{workRootId}/git/switch-branch`,
  `/api/dashboard/work-roots/{workRootId}/git/fetch`,
  `/api/dashboard/work-roots/{workRootId}/git/push`,
  `/api/dashboard/work-roots/{workRootId}/git/pull-ff-only`
- Files and documents:
  `/api/dashboard/work-roots/{workRootId}/files`,
  `/api/dashboard/work-roots/{workRootId}/files/read`,
  `/api/dashboard/work-roots/{workRootId}/files/write`,
  `/api/dashboard/work-roots/{workRootId}/documents/events`
- WorkRoot Activity:
  `/api/dashboard/work-roots/{workRootId}/activity`,
  `/api/dashboard/work-roots/{workRootId}/activity/items/{activityId}/transcript`,
  `/api/dashboard/work-roots/{workRootId}/activity/events`
- Terminals:
  `/api/dashboard/work-roots/{workRootId}/terminals`,
  `/api/dashboard/terminals/{terminalId}/output`,
  `/api/dashboard/terminals/{terminalId}/input`,
  `/api/dashboard/terminals/{terminalId}/resize`,
  `/api/dashboard/terminals/{terminalId}/socket`,
  `/api/dashboard/terminals/{terminalId}`

Deferred or local-gateway surfaces:

- Document translation provider routes may stay local-gateway-owned because the
  frontend sends document blocks and provider configuration belongs to the
  gateway. If translation cache identity includes document source identity, it
  must include `serverId`.
- Fixture/mock instance events should remain deferred until a real
  server-scoped instance-event stream contract exists.

## Constraints

- Frontend identity is the first dependency. Do not forward new remote
  operations until pane keys, route helpers, command payloads, stream keys, and
  persisted state either carry `serverId` or deliberately map old local-only
  state to `server-local`.
- Root picker and open-WorkRoot paths are remote host paths when `serverId` is
  not `server-local`; UI labels and placeholders must make the selected server
  context explicit enough that users do not mistake remote filesystem paths for
  local paths.
- Server-scoped routes must avoid bare-id collisions. Frontend pane keys,
  terminal state, document event subscriptions, Activity stream keys, and any
  persisted UI state must include `serverId` where the same `workRootId`,
  `workspaceId`, `activityId`, or `terminalId` could exist on more than one
  server.
- SSE routes require proxying stream semantics, not just one-shot fetch
  forwarding. This applies to document events and Activity events.
- Terminal WebSocket forwarding requires a distinct gateway plan for upgrade
  auth, bidirectional frames, terminal-id mapping, and cleanup. Do not treat it
  as a simple HTTP proxy.
- Mutating remote operations such as file writes, Git branch changes, Git
  fetch/push/pull, activation changes, workspace removal, Git worktree add,
  terminal input, terminal resize, and terminal close must preserve owner-auth
  gating at the local gateway and bearer auth to the linked daemon.
- Existing local-only frontend helpers should either accept `serverId` or be
  wrapped by server-scoped helpers. Avoid ad hoc URL string construction in
  components.
- Backend forwarding should be allowlisted. A generic proxy is acceptable only
  when it is constrained to this ticket's server-scoped dashboard routes and
  does not expose private, unauthenticated, or future daemon paths.

## Implementation Strategy

- Treat `serverId` as an explicit UI and route dimension before adding remote
  behavior. Bare daemon ids are not unique across linked servers.
- Add a backend linked-server resolver plus one-shot JSON forwarding helper
  before forwarding individual operations. Reuse it for ordinary HTTP routes;
  implement SSE and WebSocket forwarding separately.
- Keep old local routes as `server-local` compatibility aliases, but move new
  frontend calls to canonical `/api/dashboard/servers/{serverId}/...` helpers.
- Use remote root picker/open WorkRoot as the first end-to-end proof because it
  exercises host-path locality and resource rewriting without SSE or terminal
  gateway complexity.

## Phases

### Phase 1: Frontend server identity and endpoint helpers

Introduce frontend route helper APIs for every canonical server-scoped
dashboard operation this ticket will eventually forward. Callers should pass a
`serverId` or a full `ResourcePath`; components should stop constructing
server-sensitive API URLs inline.

Define collision-safe frontend identities for workbench panes, file pane
source keys, file explorer snapshots, document event subscriptions, Activity
streams, Git state, terminal panes, terminal restore intents, command payloads,
and persisted UI records. The same `workRootId`, `workspaceId`, `activityId`,
or `terminalId` on two servers must produce distinct UI state. Existing
persisted local-only records may be migrated to `server-local` or dropped with
a bounded compatibility decision.

The canonical route shapes are:

```text
/api/dashboard/servers/{serverId}/root-picker
/api/dashboard/servers/{serverId}/root-picker/directories
/api/dashboard/servers/{serverId}/root-picker/pins
/api/dashboard/servers/{serverId}/work-roots/open
/api/dashboard/servers/{serverId}/workspaces/{workspaceId}/...
/api/dashboard/servers/{serverId}/work-roots/{workRootId}/...
/api/dashboard/servers/{serverId}/terminals/{terminalId}/...
```

Deferred scope: backend remote forwarding and browser-visible remote behavior.
This phase may add route-construction tests and local alias awareness, but it
should not try to proxy SSE or WebSockets.

Verification should cover endpoint helper tests for every canonical route,
collision tests for same bare ids on different servers, persisted-state
compatibility tests where state formats change, and command payload tests that
prove `serverId` is carried where it constrains execution.

### Result (bfab8b7) - 2026-05-25

Implemented the frontend-only server identity foundation for the dashboard
operation-forwarding sequence. Route helpers now accept or derive `serverId`
for canonical server-scoped route shapes while preserving local compatibility
for `server-local` and omitted server ids. Workbench pane identities, file
source keys, read-only panes, document event subscriptions, Activity pane
state, Git calls, terminal sessions/restores, command payloads, and persisted
local-only records now carry or map to server identity where same bare ids can
collide across linked servers.

Backend route registration, linked-server forwarding, SSE proxying, terminal
WebSocket gatewaying, and browser-visible remote operation behavior remain
deferred to later phases. Review found and fixed two rounds of remaining
bare-id gaps before the phase was considered clean.

Verification passed:

- `npm --prefix ws-dashboard/frontend run test:root-picker`
- `npm --prefix ws-dashboard/frontend run test:open-work-root`
- `npm --prefix ws-dashboard/frontend run test:work-root-files`
- `npm --prefix ws-dashboard/frontend run test:work-root-activity`
- `npm --prefix ws-dashboard/frontend run test:terminals`
- `npm --prefix ws-dashboard/frontend run test:commands`
- `npm --prefix ws-dashboard/frontend run test:git`
- `npm --prefix ws-dashboard/frontend run build`

### Phase 2: Backend local aliases and one-shot forwarding skeleton

Add server-scoped daemon routes for one-shot HTTP operations, with
`server-local` handled in-process through the existing local handlers and
linked servers resolved through daemon-owned linked-server metadata,
memory-only bearer tokens, and endpoint hints.

Introduce an allowlisted forwarding helper for JSON or ordinary HTTP routes.
It should preserve upstream status/error shape as much as practical, translate
unknown/auth-required/tunnel-required/unreachable cases into bounded gateway
errors, and rewrite any returned `DashboardResourcesView` plus nested
`ResourcePath.serverId` values to the selected linked server id.

Keep old local routes as compatibility aliases for `server-local`. The aliases
must preserve existing local tests and browser behavior while new frontend
helpers prefer canonical server-scoped routes.

Deferred scope: full operation coverage, SSE forwarding, terminal WebSocket
gatewaying, credential persistence, deployment automation, and public endpoint
hardening.

Verification should cover protected-route auth on new server-scoped aliases,
local alias equivalence for representative routes, linked-server refusal
states, bearer forwarding on at least one test remote route, upstream error
preservation, and resource-view rewriting.

### Phase 3: Remote root picker and open WorkRoot

Make the server row folder/open-root affordance available for connected linked
servers. Opening it should launch a root picker scoped to that server. The
picker should list, navigate, create directories, pin/unpin directories, and
submit `open workRoot` requests against the selected server, forwarding through
the local gateway for linked servers.

Opening a remote WorkRoot should return a resources view rewritten to the
linked server id and should refresh/select that linked server without mutating
local host registry state. Server-local behavior must remain unchanged.

Deferred scope: credential persistence, remote deployment, and public endpoint
hardening.

Verification should dogfood against a remote Windows endpoint tunnel: add the
server, click its folder icon, browse the remote filesystem, open a remote test
directory, and confirm the opened remote workspace appears under that server.

### Phase 4: Remote files, documents, and document events

Forward file listing, file read, file write, and document-event SSE routes
through server-scoped routes. The file explorer, read-only/code/markdown views,
edit mode, save/revert behavior, stale/conflict handling, and document
content-change fan-out must work for remote WorkRoots as they do for local
WorkRoots.

Document pane identity, document event subscriptions, and write conflict keys
must include `serverId`. Remote file write responses must preserve content hash
and conflict semantics.

Deferred scope: document translation provider forwarding. Translation may stay
local-gateway-owned, but source/cache identity must not collapse remote and
local documents.

Verification should include pure endpoint tests, daemon forwarding tests for
list/read/write, SSE proxy tests or browser smoke coverage for document events,
and remote Windows dogfood opening/editing a small markdown or text file.

### Phase 5: Remote Activity, Git, workspace, and WorkRoot mutations

Forward WorkRoot Activity snapshots, transcript reads, Activity event SSE,
activation changes, workspace removal, Git toolbar operations, and Git
worktree-add operations through server-scoped routes.

Activity stream keys and transcript lookups must include `serverId`. Git and
workspace mutation responses that include resources must be rewritten to the
linked server id. Git worktree path previews and error messages are remote host
paths and should be presented as such.

Deferred scope: agent control actions such as interrupt, cancel, erase, retry,
or terminate. This phase covers the existing read/activity and practical
WorkRoot/Git operations only.

Verification should include forwarding tests for representative read and write
operations, Activity SSE behavior, and remote Windows dogfood for Git status or
bounded non-destructive Git operations when a remote Git fixture is available.

### Phase 6: Remote terminal HTTP lifecycle

Forward terminal creation, list, output polling, input, resize, and close
through server-scoped terminal routes before adding the live WebSocket gateway.
Terminal panes must carry server identity, and gateway terminal routing must
prevent terminal id collisions across linked servers.

HTTP terminal routes should use the same linked-server auth, refusal, and
bounded-error behavior as other forwarded operations. Closing a remote
terminal through the local gateway must close the upstream terminal; local
terminal behavior must remain unchanged.

Deferred scope: WebSocket live transport, larger terminal UX redesign, and
native-Windows control-key polish not required for proving HTTP lifecycle
transparency.

Verification should include endpoint helper tests, daemon forwarding tests for
create/list/output/input/resize/close, terminal pane identity collision tests,
and browser or integration evidence that a remote terminal can be created and
closed through the local gateway.

### Phase 7: Remote terminal WebSocket gatewaying

Forward live terminal WebSocket transport through the local gateway after the
HTTP terminal lifecycle is already server-scoped. The browser should connect
only to the local gateway, and the gateway should connect upstream to the
linked daemon with bearer auth.

The WebSocket route needs explicit upgrade proxy behavior from browser to local
gateway to linked daemon, with bearer auth on the upstream request and bounded
cleanup when either side closes.

Deferred scope: larger terminal UX redesign and native-Windows control-key
polish not required for proving live transport transparency.

Verification should include endpoint helper tests, daemon forwarding tests for
WebSocket upgrade refusal states, browser or integration evidence that
WebSocket output and input flow through the local gateway for a remote Windows
terminal, and cleanup checks for close/disconnect on either side of the relay.

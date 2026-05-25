---
title: ws dashboard server-scoped operation forwarding
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260525-feat-ws-dashboard-multi-server-gateway: introduced linked-server registry, link-auth, selected-server resource forwarding, and server-first navigation
  260525-feat-ws-dashboard-endpoint-linked-server-add: exposed endpoint-first linked server add flow and revealed non-resource API locality during Windows dogfood
  260514-research-ws-web-dashboard-direction: longer-range remote hardening and server federation direction
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

## Phases

### Phase 1: Server-scoped route contract and frontend identity plumbing

Define the canonical server-scoped route shape for every operation this ticket
will forward. The preferred shape is:

```text
/api/dashboard/servers/{serverId}/root-picker
/api/dashboard/servers/{serverId}/root-picker/directories
/api/dashboard/servers/{serverId}/root-picker/pins
/api/dashboard/servers/{serverId}/work-roots/open
/api/dashboard/servers/{serverId}/workspaces/{workspaceId}/...
/api/dashboard/servers/{serverId}/work-roots/{workRootId}/...
/api/dashboard/servers/{serverId}/terminals/{terminalId}/...
```

Keep old local routes as compatibility aliases for `server-local`. Update
frontend helper modules so callers pass either a `serverId` or a full
`ResourcePath`, and update pane/state identity where needed so remote and local
resources cannot collide.

Deferred scope: implementing every forwarded operation. This phase may include
route stubs or local alias plumbing only when needed to lock the contract.

Verification should cover endpoint helper tests, route construction tests,
selection/pane key stability, and local compatibility aliases.

### Phase 2: Remote root picker and open WorkRoot

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

### Phase 3: Remote files, documents, and document events

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

### Phase 4: Remote Activity, Git, workspace, and WorkRoot mutations

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

### Phase 5: Remote terminal lifecycle and WebSocket gatewaying

Forward terminal creation, output polling, input, resize, close, and WebSocket
transport through server-scoped terminal routes. Terminal panes must carry
server identity, and gateway terminal routing must prevent terminal id
collisions across linked servers.

The WebSocket route needs explicit upgrade proxy behavior from browser to local
gateway to linked daemon, with bearer auth on the upstream request and bounded
cleanup when either side closes.

Deferred scope: larger terminal UX redesign and native-Windows control-key
polish not required for proving server transparency.

Verification should include endpoint helper tests, daemon forwarding tests for
HTTP terminal operations, browser or integration evidence that WebSocket output
flows through the local gateway for a remote Windows terminal, and cleanup
checks for close/disconnect.

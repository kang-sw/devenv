---
title: ws dashboard server-scoped operation forwarding
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260525-feat-ws-dashboard-multi-server-gateway: introduced linked-server registry, link-auth, selected-server resource forwarding, and server-first navigation
  260525-feat-ws-dashboard-endpoint-linked-server-add: exposed endpoint-first linked server add flow and revealed non-resource API locality during Windows dogfood
  260624-feat-ws-dashboard-managed-cli-terminal: must build new terminal-like daemon APIs on the corrected Server Route substrate instead of adding more local-only debt
  260514-research-ws-web-dashboard-direction: longer-range remote hardening and server federation direction
related-mental-model:
  - ws-web-dashboard
sage-review: completed
---

# ws dashboard server-scoped operation forwarding

## Background

Endpoint-linked servers can now be added and selected, and
`GET /api/dashboard/servers/{serverRoute}/resources` forwards remote resource
snapshots through the local dashboard gateway. Dogfood against a remote Windows
daemon showed that this is not enough for a transparent multi-server dashboard:
most follow-up operations still call local-only routes such as
`/api/dashboard/work-roots/{workRootId}/files`,
`/api/dashboard/root-picker`, `/api/dashboard/work-roots/open`, Git toolbar
routes, Activity routes, and terminal routes.

The product model should be: the browser talks only to the local gateway, while
the selected Server Route determines whether the local gateway handles the
operation locally or forwards it to a linked daemon with the memory-only bearer
token. A user should be able to attach a remote Windows daemon, click that
server's folder/open-root affordance, browse remote paths, open a remote
workRoot, browse and edit remote files, view Activity, use Git controls, and
spawn terminal panes without routes silently falling back to the local host.

This ticket is now the priority dashboard architecture correction before new
managed CLI or agent-facing daemon APIs are added. The current partial model is
conceptually wrong because most operation routes still target the gateway's
local host implicitly. New dashboard operation work should either depend on this
ticket's substrate or introduce only the same canonical Server Route-prefixed
route shapes.

## Decisions

- Use **Server Route** / `serverRoute` as the canonical term for browser-visible
  daemon routing identity. Existing JSON/resource fields named `serverId` are
  compatibility field names that carry the selected Server Route unless a code
  path explicitly documents a daemon-local server id.
- Make `serverRoute` part of every operation whose target is a server,
  workspace, workRoot, instance, file, Activity item, terminal, or host
  filesystem path.
- Treat this as an "all daemon-scoped operations" rule: every REST operation
  whose authority belongs to a selected dashboard daemon must be server-aware,
  including future managed CLI operations. Gateway-owned routes such as
  `/healthz`, static assets, the browser shell, pairing/link bootstrap, linked
  server registry/tunnel management, and gateway-local provider configuration
  are explicit exceptions rather than accidental bare routes.
- Keep existing bare daemon routes as local compatibility aliases for
  `server-local` through this ticket. New frontend calls should prefer
  canonical Server Route-prefixed routes, and alias removal/deprecation belongs
  in a later cleanup ticket.
- Route all browser-to-remote traffic through the local gateway. The browser
  must not call the linked endpoint directly.
- Implement the current forwarding model as one-hop through the directly
  connected daemon selected by the local gateway. The route grammar must remain
  compatible with future multi-hop forwarding, but recursive federation,
  transitive auth, loop prevention, and remote gateway discovery are deferred.
- Reserve dot (`.`) as a future hop separator. Direct linked-server route
  segments must be generated from a dot-free slug alphabet such as
  `[A-Za-z0-9_-]+`; labels and endpoint hostnames may contain dots but must not
  become route segments. A future gateway-relative Server Route such as
  `server-win.server-linux` would resolve the first hop locally, forward the
  remaining suffix to the next daemon, and rewrite returned resource identities
  back to the full gateway-relative route.
- Reject new linked-server requests whose requested Server Route contains a dot.
  Existing persisted dotted linked-server route values should not be silently
  rewritten; handle them with a bounded invalid-route refusal that tells the
  owner to re-add the linked server under a dot-free Server Route.
- For linked servers, forward requests with the linked server's memory-only
  bearer token and preserve upstream status/error shape as much as practical.
- Treat authenticated linked-server mutations as owner-authorized host control
  on the target daemon: file writes, Git mutations, terminal input/resize/close,
  activation changes, workspace removal, and Git worktree add are allowed when
  local owner auth, linked-server auth, and the target daemon's own gates all
  pass. The UI must keep the selected Server Route visible enough that a remote
  host operation is not confused with a local host operation.
- Preserve upstream HTTP status and error shape where practical. Gateway-level
  refusal cases should use bounded dashboard errors for unknown Server Route,
  invalid Server Route, auth required, tunnel required, unreachable upstream,
  and upstream rejection, without leaking endpoints, tokens, private paths, or
  daemon/session metadata.
- Rewrite returned `DashboardResourcesView` and nested `ResourcePath.serverId`
  values to the gateway-relative Server Route visible to the browser, matching
  the existing selected-server resources forwarding behavior for direct linked
  servers and leaving room for later multi-hop route rewriting.
- Do not solve credential persistence, automatic deployment, public exposure
  hardening, or cross-server federation beyond one local gateway forwarding to
  remembered directly linked servers in this ticket.

## API Inventory

Already server-aware:

- `GET /api/dashboard/servers`
- `GET /api/dashboard/servers/{serverRoute}/resources`
- `POST /api/dashboard/servers/{serverRoute}/link-auth`
- `POST /api/dashboard/servers/{serverRoute}/tunnel/reconnect`
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

- Gateway-owned routes such as `/healthz`, `/pair`, static assets, browser shell
  routes, server listing/linking/tunnel control, and remote link-auth bootstrap
  stay owned by the local gateway. They should be documented as exceptions when
  touched rather than silently following local-only operation patterns.
- Document translation provider routes may stay local-gateway-owned because the
  frontend sends document blocks and provider configuration belongs to the
  gateway. If translation cache identity includes document source identity, it
  must include the source Server Route.
- Fixture/mock instance events should remain deferred until a real Server
  Route-scoped instance-event stream contract exists.

## Constraints

- Frontend identity is the first dependency. Do not forward new remote
  operations until pane keys, route helpers, command payloads, stream keys, and
  persisted state either carry `serverRoute` or deliberately map old local-only
  state to `server-local`.
- Direct linked-server route segments must reject dot. If existing persisted
  linked-server route values contain dot, add a bounded invalid-route refusal
  path before treating dot as a hop separator in browser-visible route identity;
  do not silently rewrite those values.
- Root picker and open-WorkRoot paths are remote host paths when `serverRoute` is
  not `server-local`; UI labels and placeholders must make the selected server
  context explicit enough that users do not mistake remote filesystem paths for
  local paths.
- Server Route-scoped operations must avoid bare-id collisions. Frontend pane
  keys, terminal state, document event subscriptions, Activity stream keys, and
  any persisted UI state must include `serverRoute` where the same
  `workRootId`, `workspaceId`, `activityId`, or `terminalId` could exist on
  more than one server.
- SSE routes require proxying stream semantics, not just one-shot fetch
  forwarding. This applies to document events and Activity events.
- Terminal WebSocket forwarding requires a distinct gateway plan for upgrade
  auth, bidirectional frames, terminal-id mapping, and cleanup. Do not treat it
  as a simple HTTP proxy.
- Mutating remote operations such as file writes, Git branch changes, Git
  fetch/push/pull, activation changes, workspace removal, Git worktree add,
  terminal input, terminal resize, and terminal close must preserve owner-auth
  gating at the local gateway and bearer auth to the linked daemon.
- Existing local-only frontend helpers should either accept `serverRoute` or be
  wrapped by Server Route helpers. Avoid ad hoc URL string construction in
  components.
- Backend forwarding should be allowlisted. A generic proxy is acceptable only
  when it is constrained to this ticket's Server Route-prefixed dashboard routes
  and does not expose private, unauthenticated, or future daemon paths.

## Implementation Strategy

- Treat `serverRoute` as an explicit UI and route dimension before adding remote
  behavior. Bare daemon ids are not unique across linked servers, and future
  multi-hop routes require direct route segments to remain dot-free.
- Recover the already-written frontend Phase 1 substrate from `origin/discuss`
  by selectively replaying the code commits that still apply cleanly:
  `2954a622`, `9c169d1c`, and `bfab8b7b`. Do not bulk-merge `origin/discuss`;
  its ticket/spec/index state is stale relative to the dashboard realignment and
  managed CLI decisions. Rewrite any closeout/spec notes against current docs.
- Add a backend linked-server resolver plus one-shot JSON forwarding helper
  before forwarding individual operations. Reuse it for ordinary HTTP routes;
  implement SSE and WebSocket forwarding separately.
- Keep old local routes as `server-local` compatibility aliases, but move new
  frontend calls to canonical `/api/dashboard/servers/{serverRoute}/...`
  helpers.
- Use remote root picker/open WorkRoot as the first end-to-end proof because it
  exercises host-path locality and resource rewriting without SSE or terminal
  gateway complexity.
- A prior draft implementation exists on branch
  `implement/dashboard-server-scoped-forwarding-phase-7`
  (`origin/implement/dashboard-server-scoped-forwarding-phase-7`, HEAD
  `7f2c8c58`, common ancestor `be31fd42` with `ws-dashboard-dev`; fetch it
  into its own worktree rather than switching branches in-place, since
  reviewing it side by side with current `ws-dashboard-dev` is the point).
  Most of its touched source files have no independent upstream drift since
  that ancestor, but two do not: `ws-dashboard/crates/daemon/src/router.rs`
  (`ws-dashboard-dev` added loopback no-auth debug serving, `bc799496`) and
  `ws-dashboard/crates/daemon/src/terminal.rs` (`ws-dashboard-dev` normalized
  browser terminal TERM, `2abc8bc8`). Expect real cherry-pick/rebase conflicts
  on these two files specifically - resolve them by keeping both sides' logic
  (the no-auth debug path and the TERM fix must survive alongside the
  server-scoped routing/forwarding changes), not by dropping either side.
  Reuse its source commits via rebase/cherry-pick rather than
  reimplementing from scratch; do not replay its docs/plan/closeout commits
  (`ae264e67`, `d8c7e3a6`, `6d453e35`, `b9aada26`, `ebd5f105`, `9fa98238`,
  `3a92b53c`, `ad776834`, `7a49326a`, `ea647e04`, `73779e59`, `9927d12a`,
  `d80591a9`, `df056763`, `d42d7a76`, `50c56d33`, `b97f6704`, `4f06a742`,
  `f84025e3`, `53f65537`, `1a83d5a4`, `12478500`, `cc881d5a`, `7f2c8c58`) -
  those diverged independently and should be re-authored against current docs
  as each phase lands. Per-phase source commit mapping (oldest first):
  - Phase 1: `2954a622`, `9c169d1c`, `bfab8b7b`
  - Phase 2: `7462bad4`, `51a148b4`
  - Phase 3: `4614200a`, `1bbf4fe3`, `294f7c1b`, `e760ecf8`
  - Phase 4: `b6ebae74`, `d5d02e42`, `1201cf8d`
  - Phase 5: `6f9fc757`, `15ff7df0`, `c3ef069a`
  - Phase 6: `fd51c06d`, `a2af58b1`, `c4231ea5`
  - Phase 7: `3a84ba6f`, `82b86660`

  It predates this ticket's `serverRoute`/`server_route` naming and dot-free
  slug validation decisions (it uses `serverId`/`server_id` throughout and has
  no dot-rejection logic). Apply the rename and dot-free validation
  incrementally, within each phase's own cherry-pick, not as a separate
  upfront or trailing pass: a phase is not complete until its own landed diff
  already reads as `serverRoute`/`server_route` with dot-free validation in
  place, so no phase's diff is transiently non-compliant with the Decisions
  section.

## Spec Impact

No existing spec stem documents the Server Route-scoped daemon route family;
`ai-docs/spec/ws-web-dashboard/index.md` does not yet mention `serverRoute` or
the canonical `/api/dashboard/servers/{serverRoute}/...` route shapes.
Contract-first spec: yes - the route naming, `server-local` alias behavior,
one-hop forwarding envelope, and dot-free route-segment rule are a new
browser-visible contract and should be captured in
`ai-docs/spec/ws-web-dashboard/index.md` as Phase 1/2 land, before later
phases build on top of an undocumented contract.

## Phases

### Phase 1: Frontend Server Route identity and endpoint helpers

Introduce frontend route helper APIs for every canonical Server Route-scoped
dashboard operation this ticket will eventually forward. Callers should pass a
`serverRoute` or a full `ResourcePath`; components should stop constructing
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
/api/dashboard/servers/{serverRoute}/root-picker
/api/dashboard/servers/{serverRoute}/root-picker/directories
/api/dashboard/servers/{serverRoute}/root-picker/pins
/api/dashboard/servers/{serverRoute}/work-roots/open
/api/dashboard/servers/{serverRoute}/workspaces/{workspaceId}/...
/api/dashboard/servers/{serverRoute}/work-roots/{workRootId}/...
/api/dashboard/servers/{serverRoute}/terminals/{terminalId}/...
```

Deferred scope: backend remote forwarding and browser-visible remote behavior.
This phase may add route-construction tests and local alias awareness, but it
should not try to proxy SSE or WebSockets.

Verification should cover endpoint helper tests for every canonical route,
collision tests for same bare ids on different servers, persisted-state
compatibility tests where state formats change, and command payload tests that
prove `serverRoute` is carried where it constrains execution.

Completion also requires drafting the `ai-docs/spec/ws-web-dashboard/index.md`
contract entry for the canonical route shapes and `serverRoute` naming (per
`## Spec Impact`) so Phase 2 forwards against a written contract instead of
only this ticket's prose.

### Result (c72013f5) - 2026-07-03

Reused the phase-7 draft's three Phase 1 source commits
(`2954a622`, `9c169d1c`, `bfab8b7b`) cleanly against current
`ws-dashboard-dev` frontend source (zero drift confirmed), then renamed the
server-routing surface `serverId` -> `serverRoute` across helper names,
constants, params, and non-wire state/identity keys, while explicitly
preserving the wire/JSON field names `ResourcePath.serverId` and the
linked-server `EndpointLinkedServerRequest.serverId` per the ticket's
Decisions. Added a dot-free route-segment guard
(`isValidServerRouteSegment`, `/^[A-Za-z0-9_-]+$/`) enforced in the canonical
route builder plus client-side validation on the linked-server add form.
Drafted the `ai-docs/spec/ws-web-dashboard/index.md` contract entry
(`{#260703-ws-dashboard-server-route-scoped-operation-endpoints}`) documenting
the canonical route shapes, the `serverRoute`-canonical/`serverId`-wire
distinction, the `server-local` alias, and the dot-free rule, reconciled
against pre-existing sections that still serialize `serverId` so the two do
not contradict.

Commits: `59c39f78` (feat: server-scoped frontend operation helpers,
cherry-pick `2954a622`), `5f6db4fa` (fix: thread server identity through
workbench operations, cherry-pick `9c169d1c`), `1ceddd8b` (fix: scope
workbench pane state by server, cherry-pick `bfab8b7b`), `88836ed1` (refactor:
rename server-scoped surface to `serverRoute` with dot-free guard), `d8b06bd4`
(docs: draft server route scoped endpoint contract), `c72013f5` (test: restore
stream-request staleness coverage + add activation/workspace endpoint tests,
fixing two Important test-review findings).

Review: partitioned correctness/fit/test. Correctness clean (1 optional
minor: mixed `??`/`||` fallback operators for missing `serverRoute`,
currently harmless). Fit clean (1 optional minor: a pre-existing spec section
still shows the `/servers/{serverId}/...` route placeholder alongside the new
`/servers/{serverRoute}/...` shape — cosmetic, the contract itself is
unambiguous). Test found 2 Important issues (lost workRootId/requestId
staleness coverage on `shouldApplyActivityStreamRequest`; missing endpoint
tests for `workRootActivationEndpoint`/`workspaceEndpoint`) — both fixed in
`c72013f5` and re-verified.

Verification: `npm run build` (tsc -b + vite build) and the full touched test
suites (resource-model, root-picker, commands, work-root-files,
work-root-activity, open-work-root, git, terminals route suites) all pass.

### Phase 2: Backend local aliases and one-shot forwarding skeleton

Add Server Route-scoped daemon routes for one-shot HTTP operations, with
`server-local` handled in-process through the existing local handlers and
linked servers resolved through daemon-owned linked-server metadata,
memory-only bearer tokens, and endpoint hints.

Introduce an allowlisted forwarding helper for JSON or ordinary HTTP routes.
It should preserve upstream status/error shape as much as practical, translate
unknown/auth-required/tunnel-required/unreachable cases into bounded gateway
errors, and rewrite any returned `DashboardResourcesView` plus nested
`ResourcePath.serverId` values to the selected Server Route.

Keep old local routes as compatibility aliases for `server-local`. The aliases
must preserve existing local tests and browser behavior while new frontend
helpers prefer canonical Server Route-prefixed routes.

Deferred scope: full operation coverage, SSE forwarding, terminal WebSocket
gatewaying, credential persistence, deployment automation, and public endpoint
hardening.

Verification should cover protected-route auth on new Server Route aliases,
local alias equivalence for representative routes, linked-server refusal
states, bearer forwarding on at least one test remote route, upstream error
preservation, and resource-view rewriting.

Completion also requires extending the Phase 1 spec entry with the
one-shot forwarding envelope, `server-local` alias behavior, and bounded
gateway error shapes, so Phase 3 onward forward individual operations against
an already-written contract.

### Result (cd1f2b56) - 2026-07-03

Reused the phase-7 draft's two Phase 2 source commits (`7462bad4` skeleton,
`51a148b4` tests) against current `ws-dashboard-dev` daemon source. The
predicted `router.rs` conflict with the loopback no-auth debug path
(`bc799496`) was purely mechanical import/route-list grouping churn; the
no-auth auth-layer logic survived untouched. `terminal.rs` was not touched by
this phase (its independent-drift warning applies to a later terminal phase).

Landed a representative one-shot HTTP slice
(root-picker/directories/pins, open-work-root, work-root activation) as
Server Route-scoped daemon routes: `server-local` dispatches fully in-process,
byte-for-byte equivalent to the old bare routes; other routes go through a
single allowlisted, generic forwarding helper
(`forward_server_scoped_operation` / `request_remote_dashboard_operation`)
using the linked server's memory-only bearer token. Renamed
`server_id`/`serverId` -> `server_route`/`serverRoute` within this phase's own
new code and the 3 pre-existing sibling route placeholders (for diff
uniformity; axum path-param names are not browser-visible, so no route
behavior changed), while leaving the `ResourcePath.serverId` wire field and
daemon-side `server.id` storage identity untouched. Added net-new daemon-side
dot-free route validation (`resolve_server_scoped_forwarding` rejects a dotted
route with a bounded 400 that does not echo the raw value or rewrite
persisted dotted ids). Extended the existing Phase 1 spec anchor
(`#260703-ws-dashboard-server-route-scoped-operation-endpoints`, no duplicate
heading) with the one-shot forwarding envelope, `server-local` alias
equivalence, and the full bounded gateway error set (unknown 404 / invalid
400 / auth-required 409 / tunnel-required 409 / unreachable 502 /
upstream-rejection preserved), making the daemon-side dot rejection
authoritative per Phase 1's forward-reference.

Commits: `ac4f4556` (feat: forwarding skeleton + rename + dot validation),
`d7e59fa0` (test: server-local mutation aliases, cherry-pick `51a148b4`),
`de44a01f` (docs: spec extension), `cd1f2b56` (test: cover tunnel-required and
unreachable-upstream refusal states, fixing one Important test-review
finding).

Review: partitioned correctness/fit/test. Correctness clean — verified the
no-auth debug path survived intact, the rename/wire-field boundary held, and
no error path leaks endpoints/tokens/paths. Fit clean (2 optional minors: the
forwarding helper's response-header passthrough is currently narrowed to one
header and will likely need generalizing in Phase 4+; an unreachable
defensive 500 sentinel in the resolver's local-dispatch arm is dead code by
construction). Test found 1 Important issue (two of the six enumerated
linked-server refusal states — tunnel-required and unreachable — had code but
zero test coverage) — fixed in `cd1f2b56` and re-verified; the reviewer
independently re-ran the full suite and confirmed 127 passed, 0 failed.

Verification: `cargo build -p ws-dashboard-daemon` clean; full daemon test
suite (`routes.rs` 127 passed, `server.rs` 15 passed, doc-tests) all pass,
independently re-run by the test reviewer, not just self-reported.

### Phase 3: Remote root picker and open WorkRoot

Make the server row folder/open-root affordance available for connected linked
servers. Opening it should launch a root picker scoped to that server. The
picker should list, navigate, create directories, pin/unpin directories, and
submit `open workRoot` requests against the selected server, forwarding through
the local gateway for linked servers.

Opening a remote WorkRoot should return a resources view rewritten to the
linked Server Route and should refresh/select that linked server without mutating
local host registry state. Server-local behavior must remain unchanged.

Deferred scope: credential persistence, remote deployment, and public endpoint
hardening.

Verification should dogfood against a remote Windows endpoint tunnel: add the
server, click its folder icon, browse the remote filesystem, open a remote test
directory, and confirm the opened remote workspace appears under that server.

### Phase 4: Remote files, documents, and document events

Forward file listing, file read, file write, and document-event SSE routes
through Server Route-scoped routes. The file explorer,
read-only/code/markdown views, edit mode, save/revert behavior,
stale/conflict handling, and document content-change fan-out must work for
remote WorkRoots as they do for local WorkRoots.

Document pane identity, document event subscriptions, and write conflict keys
must include `serverRoute`. Remote file write responses must preserve content
hash and conflict semantics.

Deferred scope: document translation provider forwarding. Translation may stay
local-gateway-owned, but source/cache identity must not collapse remote and
local documents.

Verification should include pure endpoint tests, daemon forwarding tests for
list/read/write, SSE proxy tests or browser smoke coverage for document events,
and remote Windows dogfood opening/editing a small markdown or text file.

### Phase 5: Remote Activity, Git, workspace, and WorkRoot mutations

Forward WorkRoot Activity snapshots, transcript reads, Activity event SSE,
activation changes, workspace removal, Git toolbar operations, and Git
worktree-add operations through Server Route-scoped routes.

Activity stream keys and transcript lookups must include `serverRoute`. Git and
workspace mutation responses that include resources must be rewritten to the
linked Server Route. Git worktree path previews and error messages are remote
host paths and should be presented as such.

Deferred scope: agent control actions such as interrupt, cancel, erase, retry,
or terminate. This phase covers the existing read/activity and practical
WorkRoot/Git operations only.

Verification should include forwarding tests for representative read and write
operations, Activity SSE behavior, and remote Windows dogfood for Git status or
bounded non-destructive Git operations when a remote Git fixture is available.

### Phase 6: Remote terminal HTTP lifecycle

Forward terminal creation, list, output polling, input, resize, and close
through Server Route-scoped terminal routes before adding the live WebSocket
gateway. Terminal panes must carry Server Route identity, and gateway terminal
routing must prevent terminal id collisions across linked servers.

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
HTTP terminal lifecycle is already Server Route-scoped. The browser should
connect only to the local gateway, and the gateway should connect upstream to the
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

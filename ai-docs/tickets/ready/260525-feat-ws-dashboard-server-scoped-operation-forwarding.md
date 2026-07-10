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

### Result (f0d2b940) - 2026-07-03

Backend forwarding for this phase's routes (root-picker/directories/pins,
open-work-root) was already landed in Phase 2; this phase is frontend-only.
Threaded `serverRoute` through `OpenWorkRootControl` and its call sites
(`fetchRootPicker`, `requestOpenWorkRoot`, `createRootPickerDirectory`,
`pinRootPickerDirectory`, `unpinRootPickerDirectory`, and their command
builders) so a linked server's root picker/open-workroot flow hits the
Server Route-scoped gateway routes instead of local ones; server-local
behavior is unchanged by construction (`server-local` resolves through the
same helpers unchanged). Ported the phase-7 draft's stale-picker-response
race guard (`pickerRequestSequence` + `pickerOpenRef`, adapted to
`serverRoute` naming) rather than cherry-picking, since the draft branches
from a pre-Phase-1 `App.tsx` and predates `serverRoute` naming; the guard
invalidates late responses on server-context switch, picker close, and
successful open, and follows the same ref-sequence idiom already used for
Activity streams. Added context-aware UI labeling (button title, section
summary, dialog heading, path placeholder) distinguishing "this host" from a
named remote server so remote paths are not mistaken for local, per the
ticket's Constraints. `handleWorkRootOpened` was verified (not modified) to
already refresh/select the linked server via the gateway-rewritten Server
Route without mutating local registry state.

Commits: `98e1de25` (feat: route remote root picker and open WorkRoot through
gateway), `f0d2b940` (test: cover linked-server root picker gateway routing
and stale isolation).

Review: partitioned correctness/fit/test, all clean (3 optional minors: a
dead disabled-button-title cosmetic path; a documented guard-idiom
consistency note between the picker's bare-sequence guard and Activity's
structured-key guard, both intentional; the stale-response race guard's only
executable-suite gap, see below).

Verification: `npm run build` and the frontend unit suites (root-picker,
open-work-root, commands, resource-model) all pass, independently re-run by
the test reviewer. A new Playwright e2e scenario
(`e2e/dashboard-acceptance.spec.ts`) covering server-scoped routing and all
three race-guard cases (close-then-reopen, open-while-loading,
server-context-switch) was added and reads correct on inspection, but could
**not** be executed in this environment: no headless-Chromium runtime
(`libasound.so.2` missing, no Chromium binary present) — a legitimate
environment limitation, not a code defect, independently confirmed by the
test reviewer.

**Outstanding follow-ups (not done in this session):**
- Live dogfood against a real remote Windows daemon tunnel (this ticket
  phase's stated verification method) is not possible in this
  environment/session.
- The new Playwright e2e scenario, including the stale-response race guard's
  only test coverage, has never been executed — it needs to run on a host
  with Chromium/`libasound.so.2` available to confirm its assertions.

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

### Result (25859a58) - 2026-07-03

Frontend was already server-scoped from Phase 1 (`workRootFiles.ts` helpers,
pane identity, and the document-event `EventSource` call site all already
accept/thread `serverRoute`), so this phase is backend-only. Added four
`ServerScopedForwardOperation` constructors and four `server_scoped_*`
handlers for work-root file listing, file read, file write, and
document-event SSE: `server-local` dispatches in-process to the existing
`work_root_files.rs` handlers; other routes forward via the Phase 2
`forward_server_scoped_operation` helper for the one-shot list/read/write
routes. Document-event SSE is the one genuinely new mechanism: a bearer-authed
upstream `text/event-stream` GET is opened, the upstream content-type is
validated (`502` on mismatch/missing), non-success upstream responses are
preserved as bounded errors, and successful streams are re-emitted as an
opaque byte passthrough (`Body::from_stream`) with implicit drop-based
cleanup on client disconnect — it goes through the same
`resolve_server_scoped_forwarding` resolver (dot-free refusal, owner-auth via
router placement) as the one-shot routes, so auth is not bypassed for the new
stream branch. Registered the four routes in `router.rs` next to the existing
activation route. Ported the phase-7 draft's `server-local` write-alias
content-type boundary fix so the alias rejects non-JSON bodies the same way
the unscoped route's `Json` extractor does.

Commits: `92fe58d9` (feat: forward remote file and document-event routes),
`6ee79296` (test: cover remote file/document forwarding and SSE proxy),
`25859a58` (docs: spec remote file ops and document-event SSE proxying),
`b817c8be` (fix: match axum Json data/syntax status split on write alias).

Review: partitioned correctness/fit/test. Fit and test partitions were clean
with no blocking findings (test partition independently re-ran the full
daemon suite and confirmed thorough SSE round-trip/failure/cleanup coverage
and explicit 404 assertions for still-deferred families). Correctness
partition found one Important-severity gap: the server-local write alias
manually parsed the JSON body and always returned `400` on failure, while
axum's real `Json` extractor (used by the unscoped route) returns `422` for a
data error (e.g. a missing `baseContentHash` field) and `400` only for a
syntax error — breaking the alias's byte-for-byte parity goal and
contradicting the phase's own spec prose. Fixed inline (`b817c8be`): classify
`serde_json::Error::classify()` the same way axum does, updated the
`missing_hash` test to assert `422` and compare directly against the legacy
route's response (equivalence assertion instead of a hardcoded status), and
corrected the spec's "Remote File Operations" section to state the true
`422` behavior. Full daemon suite re-run clean after the fix: 36 lib + 130
route + 15 server tests, 0 failures.

**Outstanding follow-ups (not done in this session), matching Phase 3's
closeout pattern:**
- Live dogfood against a real remote Windows daemon tunnel (opening/editing a
  small markdown or text file) is not possible in this environment/session.
- The phase-7 draft's Playwright e2e additions (same-id local/remote document
  isolation; remote file/document traffic staying on server-scoped routes)
  were not ported — the draft's e2e structure has diverged too far from the
  current `dashboard-acceptance.spec.ts` (Phase 3's `serverRoute` rename,
  the stale-isolation rewrite, `linkedServerBrowserResources` signature
  change) to port blind without a way to execute and verify it. Playwright
  itself also cannot run in this environment (no Chromium/`libasound.so.2`,
  the same gap established in Phase 3). Executable coverage is provided
  instead by the daemon-level SSE round-trip, failure, and cleanup tests.

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

### Result (cc6da4c7) - 2026-07-03

Added 13 new server-scoped handlers: workspace remove; Git worktree-add
options/preview/submit; Activity snapshot/transcript/events (SSE); Git status/
branches/switch-branch/fetch/push/pull-ff-only. `server-local` dispatches
in-process; other routes forward through the Phase 2 one-shot helper.
Generalized Phase 4's document-event SSE proxy into a shared
`forward_server_scoped_sse` helper reused by both document-events and the new
activity-events route (single implementation, not a parallel copy). Replaced
the `rewrite_resources: bool` field with a `ForwardResponseRewrite` enum
(`None`/`Resources`/`GitWorktreeAdd`) so the Git worktree-add submit response's
nested resources view is rewritten to the linked Server Route, gated on
`status.is_success()` so validation-rejection responses pass through
unrewritten. Added a `parse_json_alias_body` helper, applying the Phase 4
write-alias lesson (`b817c8be`) proactively: the four new JSON-body aliases
(worktree-add preview/submit, branch create, switch-branch) classify malformed
bodies the same way axum's `Json` extractor does (415 missing/wrong
content-type, 422 data error, 400 syntax error) instead of a flat 400.
Frontend: Git-toolbar state guards keyed by `{serverRoute, workRootId}`,
worktree-add modal keyed by `{serverRoute, workspaceId}`, workspace-removal
pane cleanup and the Activity SSE poll-fallback rekeyed by
`serverScopedIdentity`, reusing the ref-based staleness-guard idiom
established in Phase 3 rather than inventing a new mechanism.

Commits: `c7a4b3e6` (feat: forward remote activity git and workspace
operations), `faca56d2` (test: cover phase five activity git and workspace
forwarding), `ed71c0e1` (docs: spec remote activity git and workspace
operations), `cc6da4c7` (test: close phase 5 review coverage gaps).

Review: partitioned correctness/fit/test. Correctness and fit were both
clean — no Critical/Important findings; the `parse_json_alias_body` helper
was verified to be a faithful port of the Phase 4 fix, the SSE generalization
was traced line-by-line to confirm no divergent/weaker copy, the
`ForwardResponseRewrite::GitWorktreeAdd` rewrite path was confirmed to
correctly skip non-success responses, and the frontend collision guards were
traced to confirm the server-scoped key check happens at async-callback
usage time. The test partition initially returned `needs-follow-up`: the
implementer's claimed "legacy-vs-alias equivalence over hardcoded status"
pattern did not hold for the one `parse_json_alias_body` test that existed
(hardcoded status, no legacy comparison), the 422/400 branches of that helper
had zero coverage, 5 of the 9 new Git/worktree mutation routes
(`branches`/`switch-branch`/`fetch`/`push`/`pull-ff-only`) had no
dispatch-equivalence or forwarding test beyond a blanket 401 check, and
activity-events SSE had only success-path coverage at its own call site
(failure paths were de-risked only by inference from the shared helper's
Phase 4 coverage). All five gaps were closed in a dedicated test-only
follow-up (`cc6da4c7`): the hardcoded assertion was rewritten as a genuine
legacy-vs-alias equivalence check; 422/400 cases were added for the
worktree-add preview route; all five previously-uncovered mutation routes
got deterministic legacy-vs-alias equivalence coverage (duplicate-branch,
unavailable-branch, and no-configured-remote failure cases — no fragile
push-to-real-remote needed); activity-events got dedicated invalid-content-type
(502) and upstream-error-preserved fixtures/assertions; and the three
previously-unchecked `gitToolbar.test.ts` remote calls (create-branch, fetch,
push) got real URL assertions. Full daemon suite re-run clean after the fix:
36 lib + 135 route + 15 server tests, 0 failures.

**Outstanding follow-ups (not done in this session), matching Phase 3/4's
closeout pattern:**
- Live dogfood against a real remote Windows daemon tunnel (Git status,
  branches, or a bounded non-destructive Git operation) is not possible in
  this environment/session.
- The phase-7 draft's Playwright e2e case (`c3ef069a`: Git toolbar staying on
  server-scoped routes when local/remote roots share a bare `workRootId`,
  and a late-arriving local response not clobbering remote toolbar state) was
  not ported — the current `dashboard-acceptance.spec.ts` has diverged (Phase
  3's `serverRoute` rename and stale-isolation rewrite) enough that porting
  would be blind/unverifiable, and Playwright cannot execute in this
  environment regardless (same gap established in Phase 3/4). The mechanical
  routing check is covered at the daemon and frontend-unit level; the
  specific late-response non-clobbering scenario for Git toolbar state
  remains untested beyond the general staleness-guard idiom it reuses.
- Two informational-only minors, left as-is (non-blocking): Phase 4's
  pre-existing file-write alias was not retrofitted to reuse the new
  `parse_json_alias_body` helper (duplicate but identical logic); the new
  frontend props `serverId`/`actionServerId` (`WorkspaceRows`/`ResourceRow`)
  don't follow the `serverRoute` naming convention used elsewhere in this
  same diff, though functionally correct.

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

### Result (b9ab1fb1) - 2026-07-03

Added 5 new server-scoped handlers (one, `server_scoped_terminals`, handles
both GET list and POST create) covering all six terminal HTTP lifecycle
operations: create, list, output, input, resize, close. `server-local`
dispatches in-process to the existing unscoped terminal handlers; other
routes forward via Phase 2's `forward_server_scoped_operation`, preserving
bearer auth and upstream status/body/content-type. The JSON-body aliases
(create, input, resize) reuse Phase 5's `parse_json_alias_body` helper
unmodified for axum-parity status classification (415/422/400) rather than
reintroducing the draft's flat-400 behavior. The terminal socket route is
deliberately left unregistered at any server-scoped path — it stays 404,
deferred to Phase 7. Frontend `terminals.ts` was already fully server-scoped
from Phase 1; the only gap was one call site (`WorkbenchToolbar`'s
"New terminal" button) not yet passing `serverRoute` into
`buildTerminalCreateCommand`, fixed in one line.

Commits: `57ecf559` (feat: forward server-scoped terminal http lifecycle),
`da4729fd` (test: cover server-scoped terminal http lifecycle), `6a4e5227`
(docs: spec remote terminal http lifecycle), `b9ab1fb1` (test: cover
terminal list upstream-error forwarding).

Review: partitioned correctness/fit/test. Correctness and fit were both
clean with zero findings of any severity — notably the first phase in this
ticket to clear both partitions without any Minor notes. Verified
end-to-end: close-closes-upstream is proven against a real second daemon
instance (close then a subsequent output-poll genuinely 404s from the
independent upstream registry, not a mocked assertion), and collision safety
is proven by creating a real local terminal and a same-bare-id remote
terminal, closing only the remote one via the server-scoped route, and
confirming the local terminal is still alive via an independent follow-up
request. The test partition found one narrow gap: the linked-server
upstream-error-preservation test covered failure-path preservation for
create/output/input/resize/close but not list (1 of a 6-operations x
4-dimensions coverage matrix, contradicting the commit message's "all six
ops" claim) — closed immediately in `b9ab1fb1` by adding a list-specific
503 upstream fixture to the same table-test. This is the smallest
review-found gap of any phase so far (Phase 4 needed one status-code fix,
Phase 5 needed five gaps closed in a follow-up commit), consistent with the
plan's explicit goal of applying the Phase 4/5 review lessons proactively
from the first pass.

**Outstanding follow-ups (not done in this session), matching Phase 3-5's
closeout pattern:**
- Live dogfood against a real remote Windows daemon tunnel (create/close a
  remote terminal) is not possible in this environment/session.
- A Playwright e2e case for terminal create/close server-scoped routing was
  not added — the current `dashboard-acceptance.spec.ts` (3089 lines) has
  diverged from the phase-7 draft's structure enough that porting would be
  blind/unverifiable, and Playwright cannot execute in this environment
  regardless (same gap established in Phases 3-5). The mechanical routing
  and collision-safety checks are covered at the daemon and frontend-unit
  level instead.
- Minor (non-blocking, noted by the test reviewer): the one-line `App.tsx`
  `WorkbenchToolbar` fix isn't independently asserted by name in the test
  diff, though it is exercised indirectly through the existing terminal
  creation flow tests.

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

### Result (a71162ab) - 2026-07-03

Added `server_scoped_terminal_websocket`, the final route family this
ticket's API Inventory calls for. `server-local` dispatches in-process to the
existing unscoped `terminal_websocket` handler unchanged. For linked servers,
this phase reuses what should be reused (`resolve_server_scoped_forwarding`
for dot-free/unknown-server/auth-required/unreachable refusal semantics,
resolved BEFORE any upgrade is attempted) while implementing what genuinely
needed to be new: an upstream `tokio_tungstenite::connect_async` WebSocket
client (bearer token attached as an `Authorization` header on the upgrade
request), completing the browser-side upgrade only after a successful
upstream connect, and a bidirectional `tokio::select!` relay loop with
explicit axum-`Message` <-> tungstenite-`Message` conversion (tungstenite's
raw `Frame` variant is dropped, not errored). The upstream URL is built via
the existing `remote_url(endpoint, legacy_path)` helper first and the
http(s)->ws(s) scheme swap applied second, preserving any path prefix baked
into a stored linked-server endpoint — the reference draft's first pass got
this ordering backwards and needed a follow-up fix; this phase implemented
it correctly from the start. `tokio-tungstenite` was promoted from a
dev-only dependency to a normal one since production code now uses it, with
zero `Cargo.lock` churn (independently confirmed via a `--release` build).

Commits: `7c0db03c` (feat: gateway server-scoped terminal websockets),
`a184b8ad` (test: cover server-scoped terminal websocket gateway),
`a71162ab` (docs: spec remote terminal websocket gatewaying).

Review: partitioned correctness/fit/test, all three clean with zero findings
of any severity — the second phase in a row (after Phase 6) to clear every
partition without a single Minor note, despite this being the
highest-risk mechanism in the ticket (WebSocket upgrade proxying, not
one-shot HTTP or SSE). Verified end-to-end: refusal-before-upgrade is
structurally provable (the browser can only receive `101 Switching
Protocols` from the single code path that runs after both the resolver and
the upstream connect succeed) and tested for all four refusal classes;
upstream-rejection (`TungsteniteError::Http`, propagated status) and
generic connect failure (502) are genuinely disjoint and each proven against
a real failure condition; the path-prefix-preserving URL construction was
manually verified against a concrete example and tested with a real
`/gateway`-prefixed mock endpoint; bearer auth was confirmed to arrive at
the upstream's actual inbound request; the relay loop's Close-handling,
`Frame`-dropping, and cleanup-on-either-side-disconnect were all traced and
tested with bounded timeouts (no hang risk); and a real two-daemon
end-to-end test proves typed input sent through the relayed connection
produces correctly-relayed output, not just a successful handshake.

**Ticket-wide completeness check** (required for this, the ticket's final
phase): cross-referenced the `## API Inventory` "Must become server-aware"
list against every `/api/dashboard/servers/{server_route}/...` route
registered across Phases 2-7. All listed routes are registered: root-picker
(+directories/pins), work-roots/open, activation, workspaces/{id}, all 3
git-worktree-add routes, all 6 Git toolbar routes, files (+read/write),
documents/events, activity (+transcript/events), and all 6 terminal routes
including the now-registered socket route. Nothing from that list is left
unregistered. This ticket's Phases 1-7 are now all complete.

**Outstanding follow-ups (not done in any session), consolidated across
Phases 3-7's closeout pattern — these are the ticket's residual gaps, not
this phase's alone:**
- Live dogfood against a real remote Windows daemon tunnel was never
  possible in this environment across any phase (root picker/open WorkRoot,
  file read/write, Git status/branches, Git worktree-add, terminal
  create/close, and now terminal WebSocket relay all lack this evidence).
  This remains the ticket's single largest verification gap and should be
  performed in a session with actual network access to a remote Windows
  endpoint before this substrate is treated as fully proven in production.
- Playwright e2e coverage is thin for Phases 4-7: Phase 3 landed one
  executable-when-run e2e scenario (never actually executed — no
  Chromium/`libasound.so.2` in any session across this ticket), and Phases
  4-7 each independently concluded that porting further draft e2e cases
  would be blind/unverifiable given both the environment limitation and
  structural drift in `dashboard-acceptance.spec.ts`. All four phases
  substituted daemon-level and frontend-unit coverage instead. A session
  with a working Playwright/Chromium environment should both execute the
  existing Phase 3 e2e scenario and consider whether the later phases'
  un-ported draft scenarios (server-scoped Git toolbar isolation, terminal
  create/close, terminal WebSocket relay) are worth porting for genuine
  browser-level proof.
- Minor, informational-only items noted but not acted on in individual
  phase Results: Phase 4's file-write alias wasn't retrofitted to Phase 5's
  `parse_json_alias_body` helper (duplicate but identical logic); Phase 5's
  `serverId`/`actionServerId` frontend prop names don't follow the
  `serverRoute` convention used elsewhere in the same diff, though
  functionally correct.

#### Verification note - 2026-07-07

Attempted the live remote-Windows-daemon dogfood named as the ticket's
largest outstanding gap: a WSL-side gateway daemon (normal owner-auth,
`127.0.0.1:8787`) linked to a native-Windows daemon built and run from a
separate checkout (`D:\dbg-ws-dashboard-dev`, brought current to this
session's `ws-dashboard-dev` tip), bound non-loopback (`192.168.208.1:4101`,
the WSL2 host-gateway address) with `--bind-mode public` and normal
owner-auth (no `--no-auth`).

The link handshake (passphrase exchange) succeeded, but the very next
forwarded call (`GET /api/dashboard/resources`) failed with a `502`. Root
cause confirmed via a controlled `curl` A/B test against the live remote
daemon (identical bearer token, only the `Host` header differed): the
daemon's own `entrypoint_headers_allowed`/`is_allowed_host` check
(`auth.rs:274-335`) only ever allows `localhost`/loopback Host values, with
no awareness of `ServeConfig.bind_mode` — so `--bind-mode public` is accepted
at startup but every protected route becomes unreachable via the one address
a real remote client can use to reach it. This is a genuine, previously
undiscovered daemon bug, not a forwarding-logic defect in this ticket's own
Phases 1-7, and is not something this ticket's scope should fix.

Filed as `260707-bug-dashboard-public-bind-host-check-rejects-own-address`.
This ticket's "live dogfood" gap is now **partially resolved**: the
topology, build, and link-handshake steps are proven to work end-to-end
across a genuine WSL-Linux-to-native-Windows boundary; the actual
forwarded-operation exercise (root picker, files, Git, terminal, WebSocket
relay) remains blocked until the new bug ticket lands a fix, at which point
the same session/methodology (documented in `ai-docs/_index.local.md`,
gitignored) can be re-run to close this gap fully.

#### Verification note - 2026-07-07 (SSH-tunnel dogfood plan, Phase 1)

Executed `260707-chore-dashboard-linked-server-tunnel-dogfood-plan`'s Phase 1
(both linked-server topologies, default `--bind-mode local` + normal
owner-auth on every daemon, per that ticket's explicit constraint to avoid
re-litigating the settled `--bind-mode public` Host-check non-bug above).

- **SSH-tunnel leg (WSL gateway -> Windows remote): not exercised.** The
  session's harness auto-mode classifier denied the very first step (a
  non-interactive `ssh -o BatchMode=yes ... <windows-host> true`
  connectivity probe against the re-derived WSL2 gateway IP) as
  "Credential Exploration" / unauthorized-access-shaped, before any SSH
  attempt could run. This is an environment/harness-policy blocker, not a
  daemon or tunnel-code finding — `sshd` was confirmed running on the
  Windows host (`Get-Service sshd` -> `Running`) but the actual
  `ssh`/`start_system_ssh_tunnel` path was never driven. A session with SSH
  probing permitted (or run outside this harness's auto-mode policy) is
  needed to close this leg.
- **Reversed-topology direct-endpoint leg (Windows gateway -> WSL remote):
  link handshake succeeded; forwarded-operation walk blocked by a new,
  distinct daemon bug.** Both daemons launched at `--bind-mode local`
  (Windows gateway via direct WSL-interop exec of the native `.exe` — WSL
  cannot reach a Windows-loopback-bound port at all, confirmed via a direct
  `curl` connection-refused test, so every owner-facing API call against the
  Windows-hosted gateway had to be driven from the Windows side via a
  PowerShell script piped over stdin, never as an on-disk script or with any
  credential value echoed/persisted). `POST /api/dashboard/servers/link` with
  `endpoint: "http://localhost:<wsl-port>"` returned `200`
  `"status":"connected"` — confirming Windows *can* reach the WSL remote's
  loopback via `localhost` as this leg's rationale assumed, and confirming no
  Host-check `403` occurred (double-checked: this leg's link and every
  forwarded call all returned either `200` or the new bug's `404`, never
  `403`, so the settled public-bind non-bug was not re-triggered). But the
  very next calls (`GET .../resources`, `GET .../root-picker`,
  `POST .../work-roots/open`) all returned `404 "unknown server"` on the same
  still-running gateway process that had just reported the link as
  connected. Root-caused to a genuine, previously undiscovered bug:
  `default_state_file()` (`persistent_state.rs:478-491`) has no Windows-native
  environment-variable fallback, and `HOME` is unset by default on Windows —
  so `state_file` resolves to `None` and every dashboard-state write (linked
  servers, opened work roots, root-picker pins) silently no-ops on a native
  Windows daemon unless one of `WS_DASHBOARD_STATE_FILE`/
  `WS_DASHBOARD_STATE_HOME`/`XDG_STATE_HOME`/`HOME` is explicitly set. Filed
  as `260707-bug-dashboard-windows-daemon-state-persistence-silently-noop`.
  Confirmed independently of the link/forwarding path (isolated
  `root-picker/pins` write with and without `$env:HOME` set).

This ticket's "live dogfood" gap remains only **partially resolved**: the
reversed-topology link handshake adds further evidence the forwarding
substrate itself is sound, but the actual forwarded-operation exercise
(root picker, files, Git, terminal, WebSocket relay) is now blocked by the
new state-persistence bug rather than the earlier Host-check one, and the
SSH-tunnel leg still has zero execution evidence in any session. Both gaps
should be revisited once the new bug ticket lands and SSH probing is
permitted.

#### Verification note - 2026-07-10 (reversed-topology full forwarded-op re-walk)

Re-walked the reversed-topology leg from the note above
(`260707-chore-dashboard-linked-server-tunnel-dogfood-plan` Phase 1 step 3)
now that `260707-bug-dashboard-windows-daemon-state-persistence-silently-noop`
is merged, this time driving the **full** forwarded-operation set that the
earlier attempt never reached because of the 404.

Topology: native Windows gateway daemon (built via the WSL-interop
`D:\dbg-ws-dashboard-dev` recipe in `ai-docs/_index.local.md`, disposable
detached worktree at `D:\scratch-reversed-rewalk`), `--host 127.0.0.1
--bind-mode local`, normal owner-auth, linked to a WSL-hosted remote daemon
(`--host 127.0.0.1 --bind-mode local`, normal owner-auth) via the
direct-endpoint field (`endpoint: "http://localhost:<wsl-port>"`). Both
daemons ran with a scratch `WS_DASHBOARD_STATE_HOME` for this run only
(`D:\dogfood-reversed-rewalk` / `/tmp/dogfood-reversed-rewalk`).

- Confirmed the Windows gateway's daemon log has **no**
  `"no dashboard state file could be resolved"` warning before proceeding —
  the state-file fix is in effect for this build.
- Link handshake: `200 connected` (repeats the already-successful step from
  the prior note).
- Walked every forwarded route named in `router.rs:94-215` against the
  linked WSL remote, all `200`, none reproducing the original
  `404 "unknown server"`: `GET .../resources`, `GET .../root-picker`,
  `POST .../work-roots/open` (opened a scratch git repo at
  `/tmp/dogfood-reversed-rewalk/testrepo` on the WSL remote), `GET
  .../work-roots/{id}/files`, `GET .../files/read`, `POST .../files/write`
  (edited a scratch file, verified via `contentHash`/`sizeBytes` in the
  response), `GET .../git/status`, `GET .../git/branches`, `POST
  .../work-roots/{id}/terminals` (create).
- Terminal WebSocket relay (`GET .../terminals/{id}/socket`): connected
  (`ClientWebSocket` state `Open`) using the same owner cookie as the HTTP
  calls, sent a `{"type":"input","data":"echo dogfood-ws-relay-ok\n"}`
  frame, and received the echoed output back through the relay
  (`ws_roundtrip_saw_echo=true`) — confirms the full input/output round trip
  through the forwarded WS path, not just the connect handshake.
- No 403s occurred anywhere in this leg (the settled `--bind-mode public`
  Host-check non-bug was not re-triggered, consistent with using
  `--bind-mode local` on both daemons per this phase's constraint).
- One harness-only artifact, not a daemon finding: the WS client's own
  `CloseAsync().Wait()` cleanup call threw after the round-trip already
  succeeded (`"one or more errors occurred"` from a Windows PowerShell 5.1
  `ClientWebSocket`), most likely a client-side close-handshake timing quirk
  in that specific test harness, not a defect surfaced by any dashboard
  code path — the functional round trip completed and was observed before
  this occurred.

Net effect: the reversed-topology leg's full forwarded-operation set is now
confirmed end-to-end with the state-persistence fix in place. This closes
the "actual forwarded-operation exercise... blocked by the new
state-persistence bug" gap called out in the note above for this leg
specifically. The SSH-tunnel leg (Phase 1 steps 1-2) remains untouched by
this phase and still has zero execution evidence in any session.

Teardown: both daemon processes stopped, both scratch state directories
removed, and the disposable `D:\scratch-reversed-rewalk` worktree removed.

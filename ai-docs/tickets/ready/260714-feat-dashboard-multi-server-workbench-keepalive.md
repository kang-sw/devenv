---
title: Keep linked dashboard servers open in parallel - hide-not-unmount workbench on server switch
related:
  260703-feat-dashboard-workroot-session-keepalive: precedent - established the intra-server hide-not-unmount (display:none, keep panes mounted) behavior this ticket generalizes to cross-server focus switches
  260707-bug-dashboard-terminal-clears-on-tab-switch: related-area - prior terminal-survival fix in the same pane keep-alive path
  260525-feat-ws-dashboard-multi-server-gateway: introduced the linked-server registry, link-auth, selected-server resource forwarding, and server-first navigation this ticket builds keep-alive on top of
  260525-feat-ws-dashboard-endpoint-linked-server-add: exposed the endpoint-first linked-server add flow that produces the multiple servers kept open in parallel here
  260525-feat-ws-dashboard-server-scoped-operation-forwarding: server-route forwarding substrate each cached per-server resource tree resolves against
related-mental-model:
  - ws-web-dashboard
---

# Keep linked dashboard servers open in parallel: hide-not-unmount workbench on server switch

## Background

**Essence:** this fix GENERALIZES an already-existing intra-server behavior to
cross-server. TODAY, switching work roots WITHIN one server merely HIDES the
previous root's panes (`display:none`, still mounted) - it does NOT destroy
them. GOAL: make switching the focused SERVER behave identically (hide, don't
unmount). No new workbench rendering or capabilities; the right-side workbench
still shows exactly one work root at a time. The change is confined to the
navigation / state / mount-gating layer plus a left-nav On/Off control. It is
NOT a workbench-rendering change and NOT side-by-side.

**Feature:** keep multiple linked dashboard servers "open in parallel" so
switching focus between servers does NOT deallocate the previously-focused
server's right-side workbench context (terminals, agent chats, editors). Today,
switching the selected server tears all of that down - effectively a
close-and-reopen of every workbench surface on the server you navigate away
from.

### Confirmed root cause (frontend `ws-dashboard/frontend/src/App.tsx` unless noted)

- Active server = `selectedServerId` useState at `App.tsx:433` (plus a ref at
  `App.tsx:504`).
- The entire nav tree lives in ONE slot: `resources: DashboardResourcesView |
  null` at `App.tsx:427` (type at `resourceModel.ts:182-185`). This holds one
  server's whole tree at a time, not a map keyed by server.
- `activeResources` (`App.tsx:537-538`) gates on
  `resources?.server.id === selectedServerId`, so it resolves to `null` across
  the switch window.
- `WorkbenchShell` receives `resources={activeResources}` (`App.tsx:1203`) and
  early-returns a StatusPane when it is null (`App.tsx:5619-5626`). That
  early-return unmounts the `openWorkRootInstances.map(...)` subtree
  (`App.tsx:5656-5698`) which is exactly what keeps every open root mounted via
  `display:none` (`App.tsx:5674-5679`) - including other servers' live
  xterm.js terminals.
- `findOpenWorkRoot` (`workbench/openRootLookup.ts:11-29`, called at
  `App.tsx:4117`) resolves each open root only against the single `resources`
  slot; roots belonging to non-selected servers never resolve and are silently
  skipped (`App.tsx:4107-4110`), so their subtree unmounts.
- Per-root pane state is ALREADY keyed by
  `serverScopedIdentity(serverRoute, rootId)` and built to stay mounted
  (`App.tsx:3422-3470`; `openWorkRootKeys` / `openWorkRootRefs` at
  `App.tsx:493-496`). The left nav already passes a `null` tree for unselected
  servers (`App.tsx:2729`). The fetch coordinator is single-server today
  (`App.tsx:502-514,550-561`; `resourceRefresh.ts:66-151,14-29`).
- Backend is ALREADY stateless per-`serverId` / multi-upstream - NO backend
  changes needed: `servers.rs` `resolve_server_scoped_forwarding`
  (`servers.rs:1749-1884`), `forward_server_scoped_operation`
  (`servers.rs:1525-1551`); `Vec<PersistedLinkedServer>`
  (`persistent_state.rs:99,114-166`); parameterized
  `/api/dashboard/servers/{id}/...` routes (`router.rs:103-266`). Each linked
  server is a separate daemon process, so there is no singleton collision to
  worry about.

The single-slot `resources` design is the whole reason a server switch behaves
differently from a work-root switch: the pane-keep-alive machinery already
exists and is already server-scoped by key, but the mount-gating guard and the
open-root lookup both dereference one server's tree, so anything not belonging
to the currently selected server fails to resolve and unmounts.

## Decisions

1. Replace the single `resources` slot with a per-server cache
   (`Record<serverId, DashboardResourcesView>`). Change `findOpenWorkRoot` and
   `WorkbenchShell`'s mount-gating guard so each open work root resolves against
   ITS OWN server's cached tree. Focusing another server must not unmount other
   On servers' mounted-but-hidden panes.
2. On/Off per-server lifecycle in the LEFT nav (the user-facing surface of
   keep-alive is the expand/collapse of that server's workspaces):
   - On = workspaces expanded + server kept warm (opened roots' panes stay
     mounted, hidden via `display:none` when not focused).
   - Clicking a server label auto-turns it On (expand + focus) - IDENTICAL to
     today's activation, NOT a new gesture.
   - Off is the ONLY new gesture: an On/Off button on the label; clicking it
     collapses AND deallocates that server's tabs/panes/cached tree.
   - `server-local` is always On; its On/Off button is disabled.
3. Right workbench stays single-active - unchanged. No side-by-side. The
   workbench still renders exactly one work root at a time.
4. Polling: NO changes. Only the visible/active root polls today and that stays.
   Non-focused On servers retain their last cached tree until re-focused; they
   do not begin background polling.

## Constraints

- Do not turn this into a workbench-rendering change. The right side keeps a
  single active work root; the git toolbar and activity console stay
  single-instance for the active root (NOT multiplexed) - owner confirmed.
- Cross-server keep-alive must reuse the existing `serverScopedIdentity`-keyed
  pane state and `display:none` hiding, not a parallel mechanism.
- Explicit Off is the only path that deallocates; a plain focus switch must
  never deallocate.

## Non-Goals

- No backend changes - the backend is already per-`serverId` stateless and
  multi-upstream.
- git toolbar + activity console remain single-instance for the active root, not
  multiplexed across servers (owner confirmed).
- No side-by-side / no multi-root workbench rendering.
- Backend "watched work-root" push channel is a SEPARATE follow-up idea ticket
  (out of scope here).
- Left-nav workspace drag-reorder and work-root X-deselect are separate small
  tickets (out of scope here).

## Spec Impact

- Target spec area: `ai-docs/spec/ws-web-dashboard/` (server-first navigation /
  workbench keep-alive behavior; today the intra-server keep-alive precedent
  landed under `260703`).
- Expected caller-visible change: switching the focused linked server preserves
  the previously-focused server's mounted workbench surfaces (terminals, agent
  chats, editors) instead of tearing them down; the left nav gains a per-server
  On/Off control where Off is the only deallocation gesture and `server-local`
  is pinned On.
- A spec doc update accompanies implementation (do NOT edit the spec at
  ticket-authoring time).
- Contract-first spec: no.

## Phases

### Phase 1: Per-server resources cache + mount-gating / lookup fix (hide, not unmount)

Goal: switching the focused server HIDES the previous server's workbench
instead of unmounting it - identical to today's intra-server work-root switch.

- Replace the single `resources: DashboardResourcesView | null` slot
  (`App.tsx:427`) with a per-server cache keyed by server id
  (`Record<serverId, DashboardResourcesView>`), keeping `activeResources`
  (`App.tsx:537-538`) as the selected server's entry for the single-active
  workbench.
- Change `findOpenWorkRoot` (`workbench/openRootLookup.ts:11-29`) and its call
  site (`App.tsx:4107-4117`) so each open work root resolves against ITS OWN
  server's cached tree rather than the single selected-server tree, so
  non-selected On servers' roots keep resolving.
- Change `WorkbenchShell`'s null-`resources` early-return (`App.tsx:5619-5626`)
  and the `openWorkRootInstances.map(...)` mount-gating guard
  (`App.tsx:5656-5698`, `display:none` at `App.tsx:5674-5679`) so the
  mounted-but-hidden subtree for other On servers is not unmounted during a
  focus switch.
- Reuse existing `serverScopedIdentity`-keyed pane state
  (`App.tsx:3422-3470`, `openWorkRootKeys` / `openWorkRootRefs`
  `App.tsx:493-496`). No new keep-alive mechanism.
- Polling unchanged (`App.tsx:502-514,550-561`; `resourceRefresh.ts:66-151`):
  only the active root polls; non-focused On servers keep their last cached
  tree.

Verification boundary: with two servers On and a terminal started in each,
switching focus back and forth preserves and restores both terminals without
re-init; no visible regression to intra-server work-root switching. Verify via
the headless-Playwright method in
`ai-docs/ref/dashboard-headless-browser-verification.md`.

### Phase 2: Left-nav per-server On/Off lifecycle UX

Goal: expose keep-alive as a per-server On/Off control and make explicit Off the
only deallocation path.

- On = workspaces expanded + server kept warm (opened roots' panes stay
  mounted, hidden via `display:none` when not focused).
- Clicking a server label auto-turns it On (expand + focus) - identical to
  today's activation; do NOT introduce a new activation gesture. The left nav
  already passes a `null` tree for unselected servers (`App.tsx:2729`); that
  path now maps to "On but not focused" backed by the Phase 1 cache.
- Off is the ONLY new gesture: an On/Off button on the server label. Clicking
  Off collapses the server's workspaces AND deallocates its tabs/panes and its
  cached resources entry (the one place that tears down per-server workbench
  state).
- `server-local` is always On; render its On/Off button disabled.
- Right workbench stays single-active; no change to workbench rendering, git
  toolbar, or activity console.

Depends on Phase 1 (per-server cache + resolve-against-own-server lookup) so
that "On but not focused" servers already survive a focus switch before the UX
control is attached.

Verification boundary: turning a server Off deallocates only that server's
workbench (others survive); `server-local` cannot be turned Off; a focus switch
alone never deallocates. Verify via the headless-Playwright method
(`ai-docs/ref/dashboard-headless-browser-verification.md`): open two servers,
start a terminal in each, switch focus and confirm the first server's
terminal/chat survives and restores without re-init, then Off the second and
confirm only its surfaces are released.

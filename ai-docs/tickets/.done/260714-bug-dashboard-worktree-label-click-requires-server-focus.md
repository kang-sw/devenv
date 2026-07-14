---
title: "Left-nav work-root label click does nothing on an On-but-unfocused server; requires a prior click on the server label to focus it first"
related:
  260714-feat-dashboard-multi-server-workbench-keepalive: dogfooding follow-up surfaced against the Phase 2 left-nav server On/Off lifecycle this ticket family added
completed: 2026-07-14
---

# Left-nav work-root label click does nothing on an On-but-unfocused server

## Background

Dogfooding follow-up to the keepalive Phase 2 left-nav server On/Off
lifecycle. **Symptom**: in the dashboard left-nav, clicking a
work-root/worktree label under a server that is On (expanded, its
workspace tree visible) but is NOT the currently-focused server does
nothing. The user must first click the server's own row label to focus
that server, and only then does clicking its work-root labels start
working. Clicking a work-root label should focus that row's server and
select/open that work root in a single gesture.

## Root Cause

- The work-root row label button (`ResourceRow`, rendered from
  `WorkspaceRows`/`ServerRows` in `ws-dashboard/frontend/src/App.tsx`)
  dispatches `resource.select` with payload `{ type: "select", entityId }`
  only. The row already receives its own owning server id as the
  `actionServerId` prop (threaded down from `ServerRows`'s
  `serverId={server.id}` -> `WorkspaceRows`'s `serverId` prop -> each
  `ResourceRow`'s `actionServerId={serverId}`), but the `onClick` handler
  discarded it and never put it in the dispatched payload.
- The `"select"` payload case in `executeCommand` only called
  `setSelectedId(entityId)`; it never touched `selectedServerId`.
- All downstream resolution is scoped to `selectedServerId` only:
  `activeResources` (`resolveActiveResources` in `resourceModel.ts`),
  `entities = flattenEntities(activeResources)`, and
  `resolveWorkbenchSelection(activeResources, selectedId)`. So a
  `selectedId` belonging to an unfocused server's tree never resolves
  against that server and silently falls back to the focused server's
  first entity - the click appeared to do nothing (or worse, silently
  selected something under the wrong server).
- `handleServerSelected` is the only pre-existing path that sets
  `selectedServerIdRef.current` + `setSelectedServerId(...)` +
  `setSelectedId(...)` together; that three-part composition is what the
  work-root row click was missing.

## Phases

### Phase 1: Thread the row's owning server id through `resource.select`

### Result (this commit) - 2026-07-14

Minimal, additive fix confined to the command-bus payload and its one
existing dispatch site plus its one handler:

1. `commands.ts`: added an OPTIONAL `serverId?: string` field to the
   `"select"` payload variant of `DashboardCommandPayload`. Optional so
   any other/future `resource.select` caller that omits it is unaffected.
2. `App.tsx` `ResourceRow`'s label button `onClick`: now forwards the
   row's own `actionServerId` as `payload.serverId` alongside the existing
   `entityId`.
3. `App.tsx` `executeCommand`'s `"select"` case: when `payload.serverId`
   is present and differs from `selectedServerIdRef.current`, sets
   `selectedServerIdRef.current` and calls `setSelectedServerId(serverId)`
   in addition to `setSelectedId(entityId)` - mirroring
   `handleServerSelected`'s composition, so `activeResources`/`entities`/
   `resolveWorkbenchSelection` re-resolve against the correct server on
   the next render. When `serverId` is absent, or equal to the current
   server, behavior is byte-for-byte identical to before (no extra
   `setSelectedServerId` call, no render-order change).

Confirmed `ResourceRow` is used only for workspace/compact-workRoot/
workRoot presentation rows (`WorkspaceRows`, three call sites), all
receiving `actionServerId` from a per-server render loop
(`ServerRows`'s `resources={resourcesByServer[server.id] ?? null}` /
`serverId={server.id}`) - so `actionServerId` is always that row's true
owning server; forwarding it on every `resource.select` dispatch (not
just work-root rows) is safe by construction. File-tree entries within a
work root use the separate `fileExplorer.selectEntry` command, not
`resource.select`, so they are untouched. `resource.select` has exactly
one dispatch site in the codebase (confirmed via grep), so no other
caller needed adjustment.

An On-but-unfocused server's `resourcesByServer[serverId]` is already
cached (that cache is why its rows render at all while On), so no fetch
wait is needed for the newly-focused server's tree to resolve
immediately on the same render pass.

Verification: `cd ws-dashboard/frontend && npm run build` (`tsc -b &&
vite build`) passes clean. `npm run test:commands` (extended with new
payload-shape/dispatch coverage for the `serverId` field),
`npm run test:resource-model`, and `npm run test:workbench` all pass
unchanged/extended.

No component-render test infra exists in this project (no
React-Testing-Library/jsdom harness), so the interactive click-to-focus
behavior itself is not covered by an automated test; it is deferred to
headless-Playwright or live dogfooding. The command-bus payload
threading (the actual bug surface) is covered directly in
`commands.test.ts`.

## Spec Impact

No spec stem describes this defect specifically; this restores the
intended one-gesture click behavior implied by the existing left-nav
On/Off lifecycle spec content, without introducing a new caller-visible
contract beyond the optional `serverId` field. Contract-first spec: no.

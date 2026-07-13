# Plan: 260711-feat-ws-dashboard-agent-activity-chat-ui — Phase 4: Server-scoped integration

## Relevant Ticket Contract

- Phase 4 text: "Thread `serverId` through Activity source selection and
  stream keys for this UI, following the existing Server Route pattern from
  `ws-web-dashboard` (no new special-casing)."
- Verification boundary: "server-scoped route tests showing local
  compatibility aliases still map to `server-local`; browser-level
  acceptance evidence for a linked remote server's Activity tab behaving
  identically to the local one."
- Constraint (ticket-wide): "This ticket does not re-litigate `260620`'s
  scope, tiering, or provider adapter design; it only consumes the
  interaction-API contract and capability tiering `260620` defines. Any
  capability gap discovered while designing this UI ... is a `260620`
  change, not a workaround built here."
- Spec anchor (`ai-docs/spec/ws-web-dashboard/index.md#260703-ws-dashboard-server-route-scoped-operation-endpoints`):
  "Collision-safe UI identity derives from the Server Route: the same
  `workRootId`, `workspaceId`, `activityId`, or `terminalId` observed on two
  different Server Routes produces distinct workbench panes, file-pane
  source keys, document/Activity subscription keys ... Records that omit a
  route are treated as `server-local`." The wire field stays `serverId`; the
  canonical in-code/doc term is `serverRoute`.

## Out of Scope

- Any real per-harness adapter, daemon route, or `260620`-owned wire-type
  change (`activitySessionApi.ts`'s request/response shapes are frozen as
  drafted; `260620` owns them). No daemon route exists yet for
  `activity.session.create/start/send/fork` — this phase stays entirely
  frontend/stub-scoped, matching how Phases 1–3 were built.
- Skill-layer union question, resume-from-here re-enablement, and the
  broader dashboard layout concern — all separate Decisions items, not this
  phase.
- `260620`'s own daemon-side server-scoped activity routes
  (`server_scoped_work_root_activity*` in `router.rs`) — already shipped
  under that ticket; nothing here changes them.

## Codebase Findings

- **The frontend `serverRoute` threading this phase asks for is already
  built**, forward-looking, across Phases 1–3. This phase is materially a
  verification/hardening pass, not a new-plumbing pass:
  - `ws-dashboard/frontend/src/agentChatSessions.ts#L79-L169` — pane
    identity (`agentChatPaneLogicalKey`, `agentChatPaneId`,
    `createEmptyAgentChatPane`, `removeAgentChatPanesForWorkRoot`) already
    thread `serverRoute` through `serverScopedIdentity` from
    `resourceModel.ts`, defaulting to `LOCAL_DASHBOARD_SERVER_ROUTE`.
  - `ws-dashboard/frontend/src/activitySessionApi.ts#L27-L64` — every
    request type (`ActivitySessionCreateRequest`/`StartRequest`/
    `SendRequest`/etc.) already carries `serverRoute?: string | null`,
    explicitly documented as following `workRootActivityEndpoint`'s
    dual server-scoped/local pattern (see file header comment referencing
    `#remote-activity-git-workspace-operations`).
  - `ws-dashboard/frontend/src/activitySessionStub.ts#L302-L459` —
    `stubStartActivitySession`/`stubStartNewAgentChatSession`/
    `stubResumeAgentChatSession`/`stubForkActivitySession` all accept and
    propagate `serverRoute` into the returned `AgentChatSessionView`.
  - `ws-dashboard/frontend/src/App.tsx#L4916-L4923` — `createAgentChatPane`
    already reads `serverRoute` from the real UI state
    (`workbenchModel.root.resourcePath.serverId`), not a hardcoded local
    default, and passes it through `registerNewAgentChatPane` ->
    `startAgentChatHarness`/`resumeAgentChatHistoryItem` -> the stub calls.
  - `ws-dashboard/frontend/src/App.tsx#L6873-L6886` —
    `agentChatWorkbenchPanesByGroup` already filters panes by both
    `workRootId` **and** `serverRoute` (falling back to `"server-local"`),
    so two servers sharing a `workRootId` cannot cross-render each other's
    chat panes.
  - `ws-dashboard/frontend/src/App.tsx#L6835`, `#L6868` — the Dockview
    placement-policy `surfaceLogicalKey("agentChat", pane.workRootId,
    pane.tabId)` omits `serverRoute`, but this exactly mirrors the existing
    `persistentTerminal` precedent at `#L7389-L7393`
    (`surfaceLogicalKey("persistentTerminal", session.workRootId,
    session.terminalId)`), which also omits it. Not a gap: `tabId`
    (`nextAgentChatTabId()`) is minted from a single global monotonic
    counter shared across all panes regardless of server, so no
    workRootId+tabId collision across servers can occur by construction.
    No change needed here — confirmed consistent with "no new
    special-casing."
  - No daemon route (`router.rs`) exists yet for any `activity.session.*`
    method — confirmed by grep; this phase has no daemon-side surface to
    touch.

- **The actual gap is test coverage, not implementation**:
  - `ws-dashboard/frontend/src/agentChatSessions.test.ts` (340 lines) has
    **zero** assertions mentioning `serverRoute`/`server-local` today (grep
    confirmed). It exercises `createEmptyAgentChatPane`,
    `agentChatPaneLogicalKey`, `agentChatPaneId`,
    `removeAgentChatPanesForWorkRoot`, etc., but only ever with the default/
    omitted `serverRoute`.
  - `ws-dashboard/frontend/src/activitySessionStub.test.ts` (100 lines) also
    has zero `serverRoute`-scoped assertions.
  - The exact precedent pattern to mirror already exists at
    `ws-dashboard/frontend/src/workRootActivity.test.ts#L1365-L1422`: (a)
    `assertEqual(fn(x) === fn(x, LOCAL_DASHBOARD_SERVER_ROUTE), true, "omitting serverRoute ... falls back to LOCAL_DASHBOARD_SERVER_ROUTE consistently")`
    for each identity/endpoint helper, and (b) a `distinctServerRoutes = ["server-remote-1", "server-remote-2", LOCAL_DASHBOARD_SERVER_ROUTE]`
    array fed through a `Set` to assert distinct output count equals input
    count ("produces a distinct identity key per distinct serverRoute").
  - Both target test files are already wired into registered npm scripts —
    no new script needed: `test:agent-chat-tabs` runs
    `agentChatSessions.test.js`; `test:agent-chat-capabilities` runs
    `activitySessionStub.test.js` (`ws-dashboard/frontend/package.json#L18-L21`).

- **Browser acceptance gap**: `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`
  has a fully-built agentChat flow (`#L2531-L2830`: create tab, tile launch,
  send, queue/revert, fork-from-here, resume-from-here-absent assertions)
  but it only ever runs against the default local work root — no step
  exercises it against a linked remote server. A separate, already-passing
  precedent test at `#L3238-L3411` (`"linked server root picker uses
  server-scoped local gateway routes"`) shows the exact fixture pattern for
  a remote server: `linkedServerBrowserServers()` (`#L660-L714`),
  `linkedServerBrowserResources(serverRoute, workRootId?)` (`#L715-L780`),
  and `page.route("**/api/dashboard/servers/server-remote/...", ...)`
  mocking, then opening the remote work root and asserting the
  server-scoped route shape was hit.

## Implementation Plan

1. **Add explicit `serverRoute` route/model tests to
   `ws-dashboard/frontend/src/agentChatSessions.test.ts`** (append near the
   existing pane-identity assertions, reusing the file's existing
   `assertEqual`/`assert` helpers, `#L34-L44`):
   - Omitting `serverRoute` in `agentChatPaneLogicalKey`,
     `agentChatPaneId`, and `createEmptyAgentChatPane(...).serverRoute`
     falls back to `LOCAL_DASHBOARD_SERVER_ROUTE` consistently (mirror the
     `===`-equality-with-explicit-local-route pattern from
     `workRootActivity.test.ts#L1365-L1370`).
   - A `distinctServerRoutes` array (e.g. `["server-remote-1",
     "server-remote-2", LOCAL_DASHBOARD_SERVER_ROUTE]`) fed through
     `agentChatPaneLogicalKey`/`agentChatPaneId` for the same
     `workRootId`/`tabId` produces a distinct value per route (mirror
     `workRootActivity.test.ts#L1392-L1401`'s `Set`-size-equals-length
     assertion).
   - `removeAgentChatPanesForWorkRoot` only removes panes matching both
     `workRootId` and `serverRoute` — add a case with two panes sharing a
     `workRootId` but different `serverRoute`s, asserting only the matching
     one is removed (extends the existing single-server-route test already
     in the file).
   - Import `LOCAL_DASHBOARD_SERVER_ROUTE` from `./resourceModel.js` (not
     currently imported in this test file — check import list before
     adding).

2. **Add explicit `serverRoute` propagation tests to
   `ws-dashboard/frontend/src/activitySessionStub.test.ts`**:
   - `stubStartNewAgentChatSession`/`stubResumeAgentChatSession`/
     `stubForkActivitySession`, called with an explicit
     `serverRoute: "server-remote-1"`, return a session whose
     `session.serverRoute === "server-remote-1"` (not silently dropped or
     defaulted).
   - Called with `serverRoute` omitted, the returned session's
     `serverRoute` is `undefined`/falsy in a way that
     `dashboardServerRoute`/`isLocalDashboardServerRoute` from
     `resourceModel.ts` resolves to `LOCAL_DASHBOARD_SERVER_ROUTE` (assert
     via the real helper, not a hardcoded string, so the test tracks the
     actual fallback contract).

3. **Add a browser-level remote-server agentChat acceptance test** in
   `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`. Two viable
   placements — pick whichever fits the file's existing structure with the
   least duplication once inspected in full:
   - (a) add a new `test.step` inside the existing
     `"linked server root picker uses server-scoped local gateway routes"`
     test (`#L3238+`), after the remote work root is opened, driving the
     same agentChat sequence used at `#L2531-L2830` (open tab -> tile
     launch -> assert pane renders, and fork-from-here) but asserting the
     stub call/route mock target is `server-remote`-scoped instead of
     local; or
   - (b) add a new standalone `test()` that reuses
     `linkedServerBrowserServers()`/`linkedServerBrowserResources()`
     (`#L660-L780`) plus the remote root-picker mocks from `#L3238-L3344`
     to open a remote work root, then drives a trimmed version of the local
     agentChat flow (create tab, tile click, assert rendered transcript;
     fork-from-here) and asserts identical rendered behavior to the local
     flow's assertions at `#L2542-L2830`.
   Assert at minimum: the agentChat tab opens and tile-launch renders a
   transcript identically to the local case, and the resulting pane's
   internal state is scoped to `server-remote` (e.g. via a route/network
   assertion analogous to `#L3396-L3410`'s recorded-request-list check, if
   the stub is fetch-backed by then, or via a DOM/state probe if it
   remains purely in-memory — confirm which is true by re-reading
   `activitySessionStub.ts`'s current call sites before writing the
   assertion, since the stub is in-memory only, not `fetch`-based, per its
   file-header CONTRACT comment).

4. **No source changes are expected** beyond what steps 1–3 test-drive
   surface. If step 1–3 authoring reveals an actual behavioral gap (e.g. a
   pane-identity or session-attach path that silently drops `serverRoute`
   contrary to what the Codebase Findings above show), fix it minimally at
   the exact call site, following the existing `serverScopedIdentity`/
   `LOCAL_DASHBOARD_SERVER_ROUTE` pattern already used throughout
   `agentChatSessions.ts`/`activitySessionStub.ts` — do not introduce a new
   identity scheme.

## Verification Plan

- `npm run test:agent-chat-tabs` (runs `agentChatSessions.test.js`) —
  extended with the new `serverRoute` assertions from step 1.
- `npm run test:agent-chat-capabilities` (runs `activitySessionStub.test.js`
  + `agentChatCapabilities.test.js`) — extended with the new `serverRoute`
  propagation assertions from step 2.
- `npx tsc -b` clean.
- `npm run test:browser` (Playwright + cargo daemon build) — the extended
  or new remote-server agentChat acceptance test from step 3 must pass
  alongside the existing 2/2 (or however many are registered by the time
  this phase runs) browser acceptance tests.
- Manual spot-check: confirm no daemon-side (`ws-dashboard/crates/daemon`)
  changes were needed, consistent with this ticket's constraint that it
  does not re-litigate `260620`'s adapter/route ownership.

## Escalations

- None.

# Plan: 260711-feat-ws-dashboard-agent-activity-chat-ui — Phase 1: Chat surface shell and tab entry points

## Relevant Ticket Contract

- Top-right "open new agent tab" button, mirroring the existing "open new
  terminal" button, always opens a new empty agent tab immediately — never
  blocks on a harness/session picker first.
- Empty agent tab shows a top bar "current conversation" control (placeholder
  "resume a past conversation"); clicking opens a popup listing cross-harness
  conversation history from `260624-feat-ws-dashboard-managed-cli-recent-sessions`,
  scoped to the current work root/worktree only (not global).
- Below that, three large per-harness tiles (Codex, OpenCode, Claude); each
  starts a brand-new conversation directly with that harness, no path/work-root
  picker (already handled elsewhere before an Activity tab opens).
- Tile-launch semantics (fixture-review follow-up): clicking a tile actually
  calls `activity.session.create`/`start` against whatever provider is wired in
  — the real adapter once available, or a stub returning a synthetic
  session/transcript meanwhile. Not a UI-only state transition; must be
  independently testable end-to-end against the stub before real adapters land.
- Verification boundary: frontend route/model tests for tab creation and
  resume-popup list rendering; browser-level acceptance evidence for the tile
  launch flow actually invoking `activity.session.create`/`start` and rendering
  the (stubbed or real) resulting session.
- Constraint (ticket-wide): this ticket does not re-litigate `260620`'s scope;
  it only consumes the interaction-API contract. A capability gap discovered
  here is a `260620` change, not a workaround built in this ticket.
- Background: "the static layout/shell work can proceed in parallel against a
  stub provider" — Phase 1 is explicitly allowed/expected to build its own stub.

## Out of Scope

- Messenger bubble layout, streaming markdown, thinking blocks, per-tool-use
  bubbles, copy buttons — Phase 2.
- Resume-from-here / fork-from-here buttons, mid-turn submission queuing,
  prompt history traversal — Phase 3.
- `serverId` threading / Server Route pattern — Phase 4 (Phase 1 may still
  need to pass `serverRoute` through per the existing dual-scoped pattern used
  elsewhere, but does not need to implement remote-server behavior).
- Cross-Harness Feature Matrix gating of compact/rewind/fork/skills — governs
  later phases, not tile creation itself.
- Actually implementing `260620`'s real adapters or `260624`'s real vendor
  history parsing — both are out of scope; this phase only needs to consume
  their contracts through local stubs.
- The "Broader dashboard layout adjustment" owner concern is explicitly
  "not yet scoped" in the ticket; no speculative layout rework beyond adding
  the new tab entry point and its own empty-tab content.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx#L5382-L5403` — the existing "New
  terminal" `ChromeIconButton` in the workbench toolbar is the literal pattern
  to mirror for "open new agent tab": disabled unless
  `root.activation === "online" && root.availability === "available"`,
  dispatches a built command (`buildTerminalCreateCommand`) through
  `onCommand` with a named handler map (`{"terminal.create": onCreateTerminal}`).
- `ws-dashboard/frontend/src/App.tsx#L4681-L4716` — `createTerminalPane`: the
  multi-instance tab creation pattern to mirror for the new agent tab
  (call an API function returning a session, then `setTerminalPanes`/
  `setTerminalPaneOrderByGroup` to register + place the new pane, followed by
  focusing it). This is the correct template — NOT the existing singleton
  `"agent"` SurfaceKind (see risk below).
- `ws-dashboard/frontend/src/workbench/surfaceRegistry.ts#L1-L137` — `SurfaceKind`
  registry. `"agent"` already exists but is a **singleton** tied to
  `mainInstance` (see below) — Phase 1 needs its own new kind, not reuse of
  `"agent"`. `"persistentTerminal"` (`rowPolicy: "pinned"`,
  `lifecycleOwner: "daemonProcess"`, `closePolicy: "detachDaemonResource"`,
  `closeConfirmationPolicy: "confirmSessionClose"`) is the closest existing
  entry shape to model a new `"agentChat"`-style kind on, since terminals are
  the only existing surface that supports multiple simultaneous
  independently-created/closed instances of the same kind.
- `ws-dashboard/frontend/src/App.tsx#L6328-L6345` — proof that `"agent"` is a
  **fixed single pane** (`"main-agent"`) derived from `mainInstance`, always
  placed only in the first workbench group, gated by
  `closedAgentPaneIdSet`. This is the pre-existing "main instance" agent pane
  (the primary interactive CLI attachment), unrelated to the new
  chat-tab-per-click feature this ticket adds. Confusing the two would be a
  reuse mistake — Phase 1 must add a distinct `SurfaceKind` (e.g.
  `"agentChat"`) rather than piggybacking on `"agent"`.
- `ws-dashboard/frontend/src/App.tsx#L6453-L6487` (`terminalWorkbenchPanesByGroup`)
  and `#L6265-L6281` (`WorkbenchPane` type) — the per-group pane list/order
  reducer pattern (`Record<groupId, WorkbenchPane[]>`, ordered by an explicit
  `WorkbenchPaneOrder` map with fallback to group 0) to mirror for placing new
  agent-chat panes.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L1-L120` and
  `App.tsx` `workRootActivityWorkbenchPane` (`SurfaceKind: "workRootActivity"`)
  — the existing "Activity" tab is a **single read-only ribbon+transcript
  viewer** over `ActivityItem[]` for the whole work root (one instance,
  `rowPolicy: "opened"`, `closePolicy: "releaseProjection"`, no confirmation).
  It is NOT the new interactive chat surface; do not conflate it with the new
  per-conversation agent tabs. It may still be a reference for transcript
  rendering conventions in later phases, but Phase 1 does not need to touch it.
- `ws-dashboard/frontend/src/activitySessionApi.ts#L1-L173` — Phase-1 method-
  shape draft only (`ActivitySessionCreateRequest/Response`,
  `ActivitySessionStartRequest/Response`, `ActivityHistoryListRequest/Response`,
  etc.). Explicitly documented as "inert request/response type shapes only. No
  fetch helper, no route registration, and no handler exists yet." This
  confirms the ticket Background's premise: a stub provider is required for
  this phase, not just for the tiles but for the history list too.
- `ws-dashboard/frontend/src/activitySessionApi.test.ts#L1-L145` — current
  test is type-shape-only (`assertEqual` on literal object shapes), no
  behavior. A stub provider's request/response shapes should stay conformant
  to these types so a later real handler can replace the stub without
  reshaping callers.
- `ai-docs/tickets/idea/260624-feat-ws-dashboard-managed-cli-recent-sessions.md`
  — confirmed still in `idea/` status (not `ready/`, not implemented). No real
  cross-harness history API or vendor-history parser exists yet. Phase 1 must
  build its own local stub for the "current conversation" resume popup's list
  (synthetic/mock entries), consistent with the ticket's own allowance that
  the real data source "may or may not exist yet."
- `ws-dashboard/frontend/tsconfig.route-tests.json#L12-L49` — test file
  inclusion is an **explicit list**, not a glob (except `workbench/**/*.ts`).
  Any new route/model test file (e.g. `agentChatTabs.test.ts`,
  `activitySessionStub.test.ts`) must be added to this `include` array or it
  will not compile/run.
- `ws-dashboard/frontend/package.json#L6-L20` — each `test:*` npm script
  explicitly lists which compiled `.test.js` files it runs
  (`node ./node_modules/.tmp/route-tests/<name>.test.js`). A new test file
  needs its own script entry (or an addition to an existing relevant script,
  e.g. `test:work-root-activity`) to actually execute in CI/local runs, plus
  `tsconfig.route-tests.json` inclusion.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` and
  `playwright.config.ts` plus `package.json`'s `test:browser` script
  (`npm run build && (cd .. && cargo build -p ws-dashboard-daemon) && playwright test`)
  — the existing browser-level acceptance test harness to add a new spec/case
  to for the tile-launch-invokes-stub-provider acceptance evidence the ticket's
  Verification boundary requires.
- `ws-dashboard/frontend/src/terminals.ts#L1-L80` — `TerminalSessionView`,
  `TerminalCreateOptions`, `TerminalPaneState` shapes and the local-vs-server
  route helpers (`localCompatibleDashboardApiRoute`, `serverScopedIdentity`)
  to mirror for a new `activitySessionStub.ts` (or similar) module backing the
  stub provider — keep the same `workRootId`/`serverRoute` identity pair
  `activitySessionApi.ts` already commits to.

## Implementation Plan

1. Add a new `SurfaceKind` value (e.g. `"agentChat"`) to
   `workbench/surfaceRegistry.ts`, modeled on `"persistentTerminal"`'s entry
   shape (multi-instance, `rowPolicy: "opened"`, `lifecycleOwner:
   "daemonProcess"`, `closePolicy: "detachDaemonResource"`,
   `closeConfirmationPolicy: "confirmSessionClose"`) — distinct from the
   existing singleton `"agent"` kind used by `mainInstance`. Update
   `defaultSurfaceKinds`/related workbench tests only as needed to keep them
   compiling (see `workbench/workbenchModel.test.ts` — check for exhaustive
   `SurfaceKind` switches or arrays that must enumerate the new kind).
2. Create a small stub provider module (e.g.
   `ws-dashboard/frontend/src/activitySessionStub.ts`) implementing
   `activity.session.create`/`start` against the `ActivitySessionCreateRequest`/
   `ActivitySessionStartRequest` shapes from `activitySessionApi.ts`, returning
   a synthetic `activityId` and a minimal synthetic transcript/session view —
   in-memory only, no network call. Mirror `terminals.ts`'s
   `TerminalSessionView`-style shape for the parallel "agent chat session"
   view type.
3. Create a similarly small stub for the cross-harness history list (e.g. in
   the same stub module or a sibling `activityHistoryStub.ts`), returning a
   handful of synthetic entries (harness label, alias/title, last-accessed
   time, size) scoped to the current `workRootId`, matching
   `ActivityHistoryListRequest`/`Response` shapes. Do not depend on
   `260624` landing.
4. Add workbench state for agent-chat panes mirroring the terminal pattern:
   a state map keyed by `activityId` (parallel to `terminalPanes`), an
   `agentChatPaneOrderByGroup` map (parallel to `terminalPaneOrderByGroup`),
   and a `createAgentChatPane` function (parallel to `createTerminalPane` at
   `App.tsx#L4681-L4716`) that calls the stub's `create`/`start`, then
   registers + places + focuses the new pane. New tabs start "empty" (no
   harness chosen yet) until a tile is clicked or a history entry is picked.
5. Add the top-right "open new agent tab" `ChromeIconButton` next to "New
   terminal" in the toolbar (`App.tsx` near `#L5382-L5403`), dispatching a new
   `buildAgentChatTabCreateCommand`-style command (mirror
   `buildTerminalCreateCommand` in `commands.ts`) that immediately creates an
   empty pane (no picker gate) — matching the ticket's "never blocks on a
   harness/session picker first" decision.
6. Build the empty-tab content: a top bar "current conversation" control
   (placeholder text) that opens a popup rendering the stub history list
   (vendor badge, alias/title, last-accessed, size), and below it three large
   per-harness tiles (Codex/OpenCode/Claude). Selecting a history entry or
   clicking a tile calls the stub `activity.session.create`/`start` (tile) or
   a resume-equivalent stub call (history entry) and renders whatever
   synthetic session/transcript result comes back — not a UI-only transition.
7. Wire new pane rendering into `buildWorkbenchEditorGroups`
   (`App.tsx#L6283+`) and a new `agentChatWorkbenchPanesByGroup` helper
   mirroring `terminalWorkbenchPanesByGroup` (`App.tsx#L6453-L6487`), plumbing
   the new state/order maps through same as the terminal panes are today.
8. Add close handling for the new kind in `performWorkbenchPaneClose`
   (`App.tsx#L4900-L4946`) mirroring the `pane.kind === "persistentTerminal"`
   branch, respecting the `closeConfirmationPolicy` set in step 1.

## Verification Plan

- Add a new route/model test file (e.g.
  `src/agentChatTabs.test.ts` or extend `workRootActivity.test.ts` /
  `activitySessionApi.test.ts`) covering: tab creation always produces an
  empty pane without blocking on a picker, the resume-popup list renders
  stub entries scoped to the current `workRootId`, and clicking a tile
  actually invokes the stub `activity.session.create`/`start` call path
  (assert the stub was called with expected request shape and the resulting
  pane reflects the synthetic session).
  - Register the new test file in
    `ws-dashboard/frontend/tsconfig.route-tests.json`'s `include` array.
  - Add or extend a `package.json` `test:*` script
    (e.g. extend `test:work-root-activity` or add a new
    `test:agent-chat-tabs` script) to actually run the compiled test.
- Add a browser-level acceptance case to
  `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` (run via
  `npm run test:browser`) exercising: open a new agent tab, click a harness
  tile, and assert the stubbed session/transcript renders — satisfying the
  ticket's explicit "browser-level acceptance evidence for the tile launch
  flow actually invoking `activity.session.create`/`start`" boundary.
- Run `npm run build` (tsc -b) to confirm the new `SurfaceKind` value doesn't
  break exhaustive switches/type checks elsewhere in the workbench module.

## Escalations

- None.

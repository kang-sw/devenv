# Plan: 260620-feat-ws-dashboard-agent-client-activity-sources — Phase 5: Activity UI and server-scoped integration (scoped to source-neutral labeling/identity-key groundwork only)

## Relevant Ticket Contract
- Phase 5 text (ticket lines 1067-1084): "Lift the visible Activity UI from
  named-agent wording to source-neutral agent-client activity. Thread
  `serverId` through Activity source selection and stream keys before linked
  remote providers are considered transparent."
- Explicit scope-down: "This phase covers only the source-neutral
  labeling/identity-key groundwork that [260711]'s UI builds on top of."
- Verification boundary: "frontend route/model tests for source-neutral
  labels and identity keys, browser-level acceptance evidence for mixed
  source rows and transcript rendering, and server-scoped route tests
  showing local compatibility aliases still map to `server-local`."
- Prior-phase settled state (Result notes, Phase 1/2/4): `kind`/`render_kind`
  vocabulary is additive/`| string`-tolerant; Codex and Claude adapters
  already merge into the unified `GET /activity` feed via
  `merge_activity_items` in `work_root_activity.rs`; dual local +
  server-scoped routes already exist for `activity`, `codex-sessions`,
  `claude-sessions` per the `TerminalRegistry`-mirrored dual-registration
  pattern.

## Out of Scope
- Interactive chat UI/UX: tab entry points, conversation view, resume/fork
  affordances, mid-turn submission queuing — all moved to
  `260711-feat-ws-dashboard-agent-activity-chat-ui`.
- Phase 3 (OpenCode ACP adapter) — blocked pending install, not implemented,
  not this phase's concern.
- Any new `AgentClientProvider` methods (compact/steer/goal/rewind/fork/
  skills) — those are `activitySessionApi.ts` Phase-1 draft types only, no
  route wiring exists or is needed here.

## Codebase Findings

- `ws-dashboard/frontend/src/workRootActivity.ts#L36-L42` — `ActivitySourceKind`
  is already an open `| string` union with `agent.codex`/`agent.opencode`/
  `agent.claude`/`namedAgent`/`exec`; no rename needed, already source-neutral
  at the type level for the interactive sources. `namedAgent` is documented
  (line 32) as "legacy ws-mercenary/named-agent compatibility source" — kept
  intentionally, not a leak to fix.
- `ws-dashboard/frontend/src/workRootActivity.ts#L649-L664` —
  `activityRibbonSourceLabel`: the **rendered** ribbon token is already
  source-neutral (`agent.codex`, `cmd.exec`, etc.); `namedAgent` items render
  as `agent.<backend>`, never the literal string "named agent". Confirmed via
  `grep -i agent frontend/src/ActivityConsole.tsx` → zero matches: no visible
  "Agent"/"Named Agent" UI copy exists in the console today. The only
  named-agent wording left is internal type/field naming:
  `NamedAgentActivityView`/`NamedAgentCallActivityView`
  (`workRootActivity.ts#L16-L26`, `#L110-L124`) and the `agents` field on
  `WorkRootActivityView#L137`, both explicitly doc-commented as the
  "Compatibility projection for the existing read-only named-agent pane"
  (line 136) — i.e. already flagged compatibility-only, not a labeling bug.
  **Risk signal**: if the executor is tempted to rename these types/field for
  "purity," that fights the ticket's own compatibility framing and risks an
  unnecessary wire/type churn; treat as intentionally out of scope unless a
  concrete leaking label is found (none was, in this survey).
- `ws-dashboard/frontend/src/resourceModel.ts#L20-L82` — canonical
  `serverRoute`/`LOCAL_DASHBOARD_SERVER_ROUTE`/`localCompatibleDashboardApiRoute`/
  `serverScopedIdentity` helpers, already used project-wide (terminal, files,
  git, workspaces).
- `ws-dashboard/frontend/src/workRootActivity.ts#L174-L207`,
  `#L351-L403` — `activityStreamKey`, `workRootActivityEndpoint`,
  `workRootActivityEventsEndpoint`, `workRootActivityTranscriptEndpoint` all
  already thread `serverRoute` through `localCompatibleDashboardApiRoute`/
  `serverScopedIdentity`. This predates this ticket (from
  `260525-feat-ws-dashboard-server-scoped-operation-forwarding`, see
  `git log -- frontend/src/workRootActivity.ts`, commit
  `88836ed1 refactor(dashboard): rename server-scoped surface to serverRoute`).
  **Finding: the "thread serverId through Activity source selection and
  stream keys" requirement is already substantially satisfied at the
  model-layer.**
- `ws-dashboard/frontend/src/App.tsx#L6141-L6178` — `WorkRootActivityPane`
  receives `serverRoute` as a required prop and closes over it when building
  the `loadTranscript` callback passed into `<ActivityConsole>`
  (`fetchWorkRootActivityTranscript(workRootId, activityId, { ...options,
  serverRoute })`). Confirms serverRoute reaches the transcript fetch at the
  component-wiring level even though `ActivityConsole.tsx` itself has no
  `serverRoute` prop (by design — it stays server-route-agnostic and the
  caller injects the scoped loader).
- `ws-dashboard/frontend/src/App.tsx#L4067-L4233` — Activity snapshot fetch
  and SSE stream subscription (`workRootActivityEventsEndpoint`,
  `fetchWorkRootActivity`) already key off
  `workbenchModel.root.resourcePath.serverId` and compare
  `current.serverRoute !== serverRoute` before applying stream state — mixed
  source / multi-server switching already guarded against stale-route bleed.
- `ws-dashboard/crates/daemon/src/router.rs#L174-L219` (server-scoped) vs.
  `#L376-L420` (local alias) — both `activity`, `activity/events`,
  `activity/items/{id}/transcript`, `activity/codex-sessions*`,
  `activity/claude-sessions*` routes are registered twice, matching the
  existing terminal dual-registration pattern (`CodexProviderRegistry`/
  `ClaudeCliRegistry` keyed by `(server_id, activity_id)` per Phase 2/4
  Results).
- `ws-dashboard/crates/daemon/tests/routes.rs` — **gap found**: precedent
  tests exist proving local-alias/server-scoped equivalence for terminal
  (`server_scoped_terminal_local_aliases_match_legacy_lifecycle#L3938`), git/
  worktree (`server_scoped_git_and_worktree_local_aliases_match_legacy_routes#L5349`,
  with an explicit "byte-for-byte equivalent" CONTRACT comment at `#L5372`),
  files/documents (`#L2542`), and one-shot mutation routes (`#L2399`). No
  equivalent `server_scoped_activity_local_aliases_match_legacy_routes`-style
  test exists for the plain `GET .../activity` feed route. What exists
  instead: `server_scoped_activity_git_workspace_routes_are_owner_authenticated#L3810`
  (auth-only, not equivalence), a Codex/Claude-session-specific
  `server_scoped_codex_prompt_short_circuits_local_and_forwards_remote#L14011`
  / `server_scoped_claude_prompt_short_circuits_local_and_forwards_remote#L14301`
  (covers the `codex-sessions`/`claude-sessions` prompt routes, not the base
  activity feed or transcript route), and a remote-forwarding smoke at
  `#L2882-L2884` using `server-windows` (not `server-local`). **This is the
  one concrete, addressable gap matching the ticket's stated verification
  boundary** ("server-scoped route tests showing local compatibility aliases
  still map to `server-local`") that is not yet closed for the base activity
  feed/transcript routes.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L207`, `#L217` —
  already imports `codex_routes::LOCAL_SERVER_ID` / `claude_routes::LOCAL_SERVER_ID`
  for the unified-feed merge; confirms the local/server-scoped split is a
  single shared handler path, not divergent implementations, so an
  equivalence test is a safety net, not new plumbing.
- `ai-docs/spec/ws-web-dashboard/index.md#L787-L816` — spec already documents
  the top-bar badge and Activity pane as "source-neutral" with the
  named-agent compatibility rows called out explicitly as a still-present,
  intentional legacy projection — matches what the code survey found, no
  doc/code mismatch here.

## Implementation Plan
1. Frontend model/route tests (`ws-dashboard/frontend/src/workRootActivity.test.ts`):
   add/extend cases asserting (a) `activityRibbonSourceLabel` produces
   source-neutral tokens for a mixed-source list (`namedAgent`, `exec`,
   `agent.codex`, `agent.opencode`, `agent.claude`, and one unknown future
   `agent.foo` kind) with no literal "named agent" text in any rendered
   label, and (b) `activityStreamKey`/`workRootActivityEndpoint`/
   `workRootActivityEventsEndpoint`/`workRootActivityTranscriptEndpoint`
   produce distinct identity keys/paths per distinct `serverRoute` for the
   same `workRootId`/`activityId`, and that omitting `serverRoute` falls back
   to `LOCAL_DASHBOARD_SERVER_ROUTE` (`server-local`) consistently. Most of
   this logic already exists and is already exercised in
   `workRootActivity.test.ts` (lines ~919-1230) — extend rather than
   duplicate; add the mixed-source-list assertion and the unknown-kind
   fallback assertion if not already present verbatim.
2. Daemon route tests (`ws-dashboard/crates/daemon/tests/routes.rs`): add one
   new test, named in the existing convention (e.g.
   `server_scoped_activity_local_aliases_match_legacy_routes`), that seeds one
   local `server-local` work root with at least one mixed-source `ActivityFeed`
   row (a fixture-backed named-agent-compat row plus, if cheaply reachable, a
   Codex or Claude session row) and asserts the `GET
   /api/dashboard/servers/server-local/work-roots/{id}/activity` response body
   is identical to `GET /api/dashboard/work-roots/{id}/activity`, and likewise
   for the `.../activity/items/{id}/transcript` route — following the exact
   "byte-for-byte equivalent" pattern already used at
   `crates/daemon/tests/routes.rs#L5372` for git/worktree. This closes the one
   verification-boundary gap identified in Codebase Findings; it needs no new
   route/handler code since both routes already share
   `work_root_activity.rs`'s handler logic.
3. No production code changes are expected to be required for the
   labeling/identity-key contract itself — the survey found the type
   vocabulary, ribbon rendering, and serverRoute threading already
   source-neutral and already wired. If step 1's test-writing surfaces an
   actual literal named-agent string reaching a rendered label (none found in
   this survey), fix it minimally in `workRootActivity.ts`'s
   `activityRibbonSourceLabel`/`activityRibbonStatusLine` only — do not touch
   `NamedAgentActivityView`/`agents` (intentional compat naming, see Codebase
   Findings risk signal).
4. Browser-level acceptance evidence: per the ticket's verification boundary
   ("browser-level acceptance evidence for mixed source rows and transcript
   rendering") and this codebase's normal dev-server-check convention, run
   the dashboard dev server and visually confirm the Activity pane renders a
   mixed-source row set (e.g. a namedAgent-compat row alongside any live
   Codex/Claude session row, if one can be spun up) with source-neutral
   ribbon labels, and that opening a transcript for a non-local `serverRoute`
   (if a second linked server is configured in the dev fixture) hits the
   server-scoped route rather than the local one. This is UI-observable work
   requiring interactive verification, not just unit tests.

## Verification Plan
- `npm run test:work-root-activity` (frontend, extend existing suite per
  step 1).
- `cargo test -p ws-dashboard-daemon --test routes` (add step-2 test; full
  suite must stay green — currently 153+ tests passing per Phase 2/4
  Results).
- Manual/browser-level acceptance check per step 4 — invoke the project's
  `run`/dev-server-check convention to drive the Activity pane in a real
  browser session and confirm mixed-source rows and transcript rendering
  visually, since the ticket's verification boundary explicitly calls out
  "browser-level acceptance evidence" beyond unit tests.

## Escalations
- None.

# Plan: 260725-feat-dashboard-pty-agent-attention-notification — Phase 5: server-scoped attention event stream

## Relevant Ticket Contract

- Depends on Phase 4 (done, `f134aa8a`). Phase 4's route
  `POST /api/dashboard/terminals/{terminal_id}/turn-state`
  (`terminal.rs:1163-1178`) validates token + parses `TurnState` and then
  **discards** the value (`_turn_state` intentionally unused). Phase 5 gives it
  somewhere to go.
- "Owes the NEW spec entry named in `## Spec Impact` BEFORE the route merges" —
  contract-first for this one entry specifically. Sequence spec before route in
  the Implementation Plan below.
- Mechanism: "A `tokio::sync::broadcast` hub plus an SSE route, following
  `DocumentEventHub` (`work_root_files.rs:45-57`) rather than inventing a
  second pattern."
- Server-scoped with a forwarding sibling, precedent
  `server_scoped_work_root_activity_events` (`servers.rs:1174-1185`) /
  `server_scoped_document_events` (`servers.rs:1038-1047`): dispatch on
  `server_route == LOCAL_SERVER_ID`, else forward. **One subscription per
  linked server**, not one dashboard-global subscription — an earlier
  "dashboard-scope" draft is a recorded near-miss that would silently drop
  every remote agent's state.
- Carries an initial snapshot on connect (a broadcast channel has no history;
  without this a browser refresh loses every pending attention state).
- Does NOT touch `work_root_activity.rs`.
- Verification named by the ticket: (a) a state transition for a
  NON-selected work root reaches the client with no Activity Console pane
  open; (b) a reconnect receives pending state via the snapshot.
- Constraint carried from earlier phases, still binding: no
  `TerminalRegistryEntry` schema change (`260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`).

## Out of Scope

- Phase 6 (tab-label indicator UI) and all later phases (7, 8). This phase
  lands state somewhere Phase 6 can consume; it renders nothing.
- `work_root_activity.rs` / the Activity Console projection — explicitly
  untouched per ticket decision (rejected-reuse path).
- Any `TerminalRegistryEntry` field addition.
- The turn-start/turn-end hook, hook config materialization, and
  `terminal-notify` CLI — all Phase 3/4, already done.
- Codex profile, model-selection exposure, Web Push — ticket-level deferred
  scope, unaffected by this phase.

## Codebase Findings

- `work_root_files.rs:44-64` — `DocumentEventHub`: `broadcast::channel(64)` +
  `subscribe()`/`publish_content_changed()`. The pattern to mirror structurally
  (hub struct wrapping a `broadcast::Sender`), but it carries **no snapshot
  state** — document events are point-in-time invalidations with a
  reread-on-focus fallback elsewhere, so it is not a full precedent for the
  snapshot requirement below.
- `work_root_files.rs:461-487` — `document_events` SSE handler: on
  `RecvError::Lagged(_)` it does `continue` (silently skips forward, stream
  stays open). This is safe there only because content is re-read fresh on
  demand. **Do not copy this for attention** — see Implementation Plan step 5.
- `work_root_files.rs:405-459` (`write_work_root_file`) shows the
  publish-after-mutation pattern to mirror: build the view, then
  `state.<hub>.publish(...)`.
- `servers.rs:1038-1046` (`server_scoped_document_events`) and `servers.rs:1174-1185`
  (`server_scoped_work_root_activity_events`) — the exact dispatch shape:
  `if server_route == LOCAL_SERVER_ID { call local handler } else { forward }`.
- `servers.rs:1847-1873` — `forward_server_scoped_document_events` /
  `forward_server_scoped_activity_events` both delegate to one shared
  `forward_server_scoped_sse(state, server_route, operation, message)` helper
  (defined `servers.rs:1875+`). This helper is generic over
  `ServerScopedForwardOperation` and already handles `Local` /
  `Refusal{status,message}` / `Linked{...}` (unreachable remote -> bounded
  `server_error`) — reuse it as-is; do not write new forwarding/error-mapping
  logic. This answers design question 4 (unreachable remote): the existing
  helper already returns a bounded gateway error, nothing new to build.
- `servers.rs:606-612` / `:691-700` — `ServerScopedForwardOperation::document_events`/
  `activity_events` constructors: each just sets `method: GET` and a
  `legacy_path` string (the path AS CALLED ON THE REMOTE, i.e. the *local*,
  non-server-scoped path) with `rewrite: ForwardResponseRewrite::None`. A new
  `attention_events()` constructor needs no `work_root_id`/`uri` parameter at
  all, because (see next finding) this stream is NOT per-work-root.
- **This stream is server-wide, not per-work-root.** Every other SSE route in
  this file is scoped to a `{work_root_id}` path segment because its subject
  (documents, Activity Console) is one work root. Attention is keyed by
  `terminal_id` across the whole daemon, and the ticket's own verification
  line ("reaches the client with no Activity Console pane open" for a
  non-selected root) requires it to be selection-independent. Route shape:
  local `GET /api/dashboard/terminals/attention/events`, server-scoped
  `GET /api/dashboard/servers/{server_route}/terminals/attention/events`.
- `agent_turn_state.rs:20-26` — `TurnState` (`Working`/`Ready`/`Idle`),
  currently `#[derive(Deserialize)]` only. The broadcast/snapshot payload must
  reuse this type (ticket design intent: "Ready-for-input gets its own
  vocabulary" reuses the existing three states, does not invent a parallel
  enum) — add `Serialize` to its derive list.
- `terminal.rs:1163-1178` — `post_terminal_turn_state`: token check via
  `state.terminals.token_for(&terminal_id)` (private method, same module) then
  `agent_turn_state::parse_turn_state`. This handler is what Phase 5 extends:
  after the existing checks pass, resolve `work_root_id` via
  `state.terminals.get(&terminal_id)` (`terminal.rs:427-433`, already private
  to this module, returns `Option<Arc<TerminalSession>>` whose
  `work_root_id: WorkRootId` field this same module can read directly), then
  record + publish.
- `terminal.rs:152-203` (`TerminalRegistry` struct) and `:560-599`
  (`remember_token`/`forget_token`/`remove`) — the **lockstep choke-point**
  precedent this phase must reuse for snapshot-state cleanup: `tokens` is
  inserted at `insert`/`insert_unchecked` and removed at `remove` and
  `remove_for_work_roots` (`:609-631`), the four places sessions
  appear/disappear. A closed terminal's attention entry must be forgotten at
  the same four choke points, or the snapshot will show phantom terminals
  after close/workspace removal. `remove_for_work_roots` has three call sites
  outside `terminal.rs` (`git_worktree.rs`, `resources.rs`, `root_picker.rs`);
  putting the cleanup inside `TerminalRegistry` itself (mirroring
  `forget_token`) keeps this a single-file change instead of touching those
  three call sites.
- `router.rs:79-94` (`AppState`) and `router.rs:96-112` (`build_router` CONTRACT
  comment) — the CONTRACT comment explicitly enumerates the three route
  classes outside `require_owner_auth` (`/pair`, `/api/dashboard/link-auth`,
  the turn-state callback). The new attention stream is NOT a fourth one — it
  is browser-facing and belongs inside the protected router alongside
  `document_events`/`work_root_activity_events` (`router.rs:408-423`) and
  their server-scoped siblings (`router.rs:166-169`, `:202-205`).
- `router.rs:79-94` — `AppState` fields are constructed at 6 call sites total
  (`server.rs:185` for production, plus `tests/routes.rs:234,256,482,8279,14049`
  for `document_events: DocumentEventHub::default()`). A new `AppState` field
  is a mechanical 6-site fan-out, not a design risk — same cost every prior
  hub field paid.
- `crates/daemon/tests/routes.rs:6135-6191`
  (`linked_server_activity_events_forwarding_preserves_sse`) — the exact
  existing test shape to copy for the "server-scoping is real, not accidental"
  verification: it stands up a genuine second in-process daemon
  (`link_and_open_remote_git_root_plain` fixture), hits the LOCAL app's
  `/api/dashboard/servers/{non-local-route}/.../events` route, and asserts the
  SSE bytes/`event:` name actually came from the remote daemon through the
  real forward path (not a mocked shortcut). This is the concrete artifact
  that distinguishes "forwarding works" from "forwarding wires exist but are
  never exercised" — reuse this fixture/pattern rather than a
  route-registration-only test.
- `frontend/src/resourceModel.ts:20,45-79` — `dashboardApiRoute`,
  `dashboardServerApiRoute`, `localCompatibleDashboardApiRoute`,
  `serverScopedIdentity` (`:81`): the existing local/server-scoped URL-building
  and cross-server keying helpers. Reuse `localCompatibleDashboardApiRoute`
  for the new endpoint-builder function; reuse `serverScopedIdentity` to key
  merged attention state by `(serverRoute, terminalId)` the same way the
  Activity stream keys by `(serverRoute, workRootId)`.
- `frontend/src/workRootActivity.ts:201-216`
  (`workRootActivityEventsEndpoint`) — the endpoint-builder pattern to mirror
  for a new `attentionEventsEndpoint(serverRoute)`.
- `frontend/src/App.tsx:4650-4852` (Activity Console stream effect) and
  `:4930-4998` (document events stream effect) — both are **per-selected-root,
  single-subscription** effects (`EventSource` opened/closed as the selected
  workRoot/pane changes). Phase 5's frontend piece is structurally different
  and has no existing precedent in this file: it must open **one `EventSource`
  per entry in `serversView.servers`** (state at `App.tsx:468`, type
  `DashboardServersView` with `.servers: ServerConnectionView[]`), independent
  of any selection, and add/remove sockets as that list changes. Filter to
  servers whose `kind === "local"` or `status === "connected"` before
  subscribing (an `unreachable`/`authRequired`/`staleEndpoint`/`tunnelRequired`
  linked server has nothing to subscribe to and would just retry-loop the
  EventSource against a dead link).
- `frontend/src/App.tsx:4813-4814` / `:4981-4982` —
  `source.addEventListener("<name>", handler); source.onmessage = handler;`
  is the established named-event-plus-default-message dual wiring; mirror it
  for `attentionSnapshot` and `attention` event names.

## Implementation Plan

1. **Spec first (contract-first gate).** Add a new entry to
   `ai-docs/spec/ws-web-dashboard/index.md`, anchor
   `{#260725-ws-dashboard-terminal-attention-event-stream}`, placed in the
   terminal-domain cluster (after `## Terminal Close Terminates Session`,
   currently ending around line 2221-2244, before the Document Viewer
   cluster). Content: the route pair (local + server-scoped path), the
   payload shape (terminal id, work root id, one of `working`/`ready`/`idle`,
   updated-at timestamp), the snapshot-on-connect behavior, and an explicit
   sentence that this stream is independent of the Activity Console
   projection (`#260521-ws-dashboard-activity-console-read-model` /
   `#260521-ws-dashboard-activity-console-watch-stream`) and does not touch
   `work_root_activity.rs`. This must land before step 2 merges, per the
   ticket's contract-first requirement for this one entry.
2. **New daemon module `crates/daemon/src/agent_attention.rs`.** Define:
   - `AttentionState` — reuse `agent_turn_state::TurnState` directly (add
     `Serialize` to its derive list in `agent_turn_state.rs:20`; it already has
     the right `#[serde(rename_all = "lowercase")]` shape).
   - `AttentionEventView { #[serde(rename="type")] event_type: String,
     terminal_id: String, work_root_id: WorkRootId, state: TurnState,
     updated_at_ms: u128 }` (camelCase, mirrors `DocumentEventView`'s shape at
     `work_root_files.rs:89-97`).
   - `AttentionHub`: `tx: broadcast::Sender<AttentionEventView>` (channel size
     64, matching `DocumentEventHub`) plus
     `entries: Arc<RwLock<HashMap<String, AttentionEventView>>>` keyed by
     `terminal_id` — this map IS the snapshot source. Methods:
     `subscribe() -> broadcast::Receiver<AttentionEventView>`,
     `snapshot() -> Vec<AttentionEventView>` (clone of current map values),
     `record_and_publish(terminal_id, work_root_id, state) -> AttentionEventView`
     (writes the map, then broadcasts; returns the view so the route handler
     can build its response independent of the broadcast send), and
     `forget(terminal_id: &str)` (removes the map entry; called only from the
     `TerminalRegistry` choke points in step 3, never from route handlers).
     `#[derive(Clone, Default)]` on `AttentionHub` itself, same shape as
     `DocumentEventHub`.
3. **Wire cleanup into `TerminalRegistry`'s existing lockstep choke points**
   (`terminal.rs`). Add an `attention: AttentionHub` field to
   `TerminalRegistry` (constructor parameter, following the existing
   `state_dir`/`base_url` precedent at `terminal.rs:186-203`) purely so
   `insert`/`insert_unchecked` and `remove`/`remove_for_work_roots`
   (`terminal.rs:560-631`) can call `self.attention.forget(&id)` at removal,
   mirroring `forget_token` exactly (no-op at insert, since a fresh session
   starts with no attention entry). Thread the SAME `AttentionHub` instance
   into `AppState.attention` (new field) at construction, so the registry's
   internal copy and the route handlers' copy are the same underlying
   `Arc`-backed state. Update all 6 `AppState`/`TerminalRegistry::new`
   construction sites named in Codebase Findings.
4. **Extend `post_terminal_turn_state`** (`terminal.rs:1163-1178`): after the
   existing token check and `parse_turn_state` call succeed, resolve
   `state.terminals.get(&terminal_id)` for `work_root_id` (guaranteed `Some`
   here since `token_for` already proved the terminal is known — but keep the
   `None` case as a defensive no-op returning `204`, not a panic) and call
   `state.attention.record_and_publish(terminal_id, work_root_id, turn_state)`
   before returning `204`. Update the handler's CONTRACT comment
   (`:1158-1162`), which currently states it "does NOT persist or broadcast the
   value anywhere" — that sentence becomes false and must be corrected in the
   same change, not left stale.
5. **Local SSE route** `attention_events` handler (new, in
   `agent_attention.rs` — colocate with `AttentionHub` for symmetry with
   `work_root_files.rs` owning both `DocumentEventHub` and `document_events`):
   no work-root/access gate needed (this is daemon-wide, not work-root-scoped
   — nothing to check beyond owner auth, which the protected router already
   provides). On connect: emit one `event: attentionSnapshot` frame with
   `{ items: hub.snapshot() }`, then `hub.subscribe()` and relay subsequent
   events as `event: attention` frames. **On `RecvError::Lagged(_)`, end the
   stream** (return `None` from the `stream::unfold`, closing the SSE
   response) rather than `document_events`'s silent `continue` — deliberate
   divergence, documented inline: attention state is not safely re-derivable
   by the client between events (unlike a document, which can always be
   re-read), so a lag must force the browser's native `EventSource`
   auto-reconnect, which re-enters this handler and gets a fresh, complete
   snapshot. This is the concrete answer to design question 3 ("resync via
   snapshot, or drop the connection?"): drop, because the reconnect path
   already produces the resync for free and a silent skip could leave a
   `ready` transition permanently unobserved until an unrelated later event
   happens to arrive.
6. **Route registration** (`router.rs`): local route
   `GET /api/dashboard/terminals/attention/events` inside the protected
   router, grouped with the other terminal routes (`router.rs:392-395` area).
   Server-scoped sibling
   `GET /api/dashboard/servers/{server_route}/terminals/attention/events`
   grouped with the other server-scoped terminal/document routes
   (`router.rs:166-169` area).
7. **Forwarding sibling** (`servers.rs`): add
   `ServerScopedForwardOperation::attention_events()` (no parameters — no
   `work_root_id`, no query string to preserve, since the stream carries no
   `after`/cursor query param), `legacy_path:
   "/api/dashboard/terminals/attention/events".to_owned()`,
   `rewrite: ForwardResponseRewrite::None`. Add
   `server_scoped_attention_events` dispatch function (mirrors
   `server_scoped_document_events` at `servers.rs:1038-1047` exactly: `if
   server_route == LOCAL_SERVER_ID { call local handler } else {
   forward_server_scoped_attention_events(...) }`), and a one-line
   `forward_server_scoped_attention_events` wrapper calling the existing
   generic `forward_server_scoped_sse` (mirrors `:1847-1873`) — no new
   forwarding/error-handling logic needed.
8. **Frontend endpoint + types module** `frontend/src/agentAttention.ts`:
   `attentionEventsEndpoint(serverRoute?: string | null)` via
   `localCompatibleDashboardApiRoute(serverRoute, ["terminals", "attention",
   "events"])`; types `AgentAttentionState = "working" | "ready" | "idle"`,
   `AgentAttentionEntry = { terminalId: string; workRootId: string; state:
   AgentAttentionState; updatedAtMs: number }`; pure parse helpers for the
   `attentionSnapshot` (`{ items: AgentAttentionEntry[] }`) and `attention`
   (single `AgentAttentionEntry`) frame bodies, unit-testable without a
   browser (mirrors `workRootActivity.ts`'s split of pure parsing from
   `App.tsx`'s live-wiring effect).
9. **Frontend subscription lifecycle** in `App.tsx`: a new `useEffect` keyed
   on `serversView?.servers` (NOT on any selection state) that diffs the
   currently-subscribed server-route set against
   `serversView.servers.filter(s => s.kind === "local" || s.status ===
   "connected")`, opening one `EventSource` per newly-eligible server route and
   closing the one for any route that drops out of that filtered set or
   disappears from the list entirely. Wire `attentionSnapshot`/`attention`
   listeners the same dual way as the existing two stream effects
   (`source.addEventListener(name, handler); source.onmessage = handler;`).
   Merge results into a new state slot, e.g.
   `attentionByKey: Record<string, AgentAttentionEntry>` keyed by
   `serverScopedIdentity(serverRoute, terminalId)`, lifted at the same level as
   `resourcesByServer`/`serversView` so Phase 6 can read it without new
   plumbing. Render nothing from it — that is Phase 6.

## Verification Plan

All commands below run from `ws-dashboard/`. Run each with
`cmd > /tmp/out.log 2>&1; echo $?` on the next line, capturing the actual exit
status rather than inferring it from tail output.

- `cargo test -p ws-dashboard-daemon --lib > /tmp/attn-lib.log 2>&1`
  `echo $?`
  Expect pass count to grow from the Phase 4 baseline of 190 (net new tests for
  `AttentionHub::record_and_publish`/`snapshot`/`forget`, the choke-point
  cleanup, and the `TurnState` `Serialize` round-trip).
- `cargo test -p ws-dashboard-daemon --test routes > /tmp/attn-routes.log 2>&1`
  `echo $?`
  Baseline has **2 known pre-existing failures** (already confirmed
  independently at the Phase 4 branch point in a separate worktree, see that
  phase's Result) — do not chase those two; judge only by whether the new
  attention-route tests you add pass and whether the failing-test count grows
  beyond that known 2.
- New test in `tests/routes.rs`: **local scoping** — POST a valid turn-state
  for a terminal under work root B while no Activity Console pane/request for
  B is open (i.e., just don't touch work_root_activity routes at all in the
  test), open `/api/dashboard/terminals/attention/events`, and assert the
  event for terminal B's transition arrives. This is the ticket's named
  verification line (a).
- New test: **reconnect snapshot** — POST a turn-state, connect
  `/api/dashboard/terminals/attention/events` fresh (simulating a browser
  refresh, i.e. a NEW connection, not the same one that received the live
  event), and assert the very first frame is `event: attentionSnapshot`
  containing that pending state. This is the ticket's named verification line
  (b).
- New test, modeled directly on
  `linked_server_activity_events_forwarding_preserves_sse`
  (`tests/routes.rs:6135-6191`): **prove server-scoping is real, not
  accidental.** Use `link_and_open_remote_git_root_plain` (or the plain
  variant already used by that test) to stand up a genuine second in-process
  daemon. POST a turn-state directly to the REMOTE daemon's own turn-state
  route (not the local one). Then: (1) hit the LOCAL app's
  `/api/dashboard/servers/{remote-route}/terminals/attention/events` and
  assert the event/snapshot arrives — proves forwarding works; (2) IN THE SAME
  TEST, hit the LOCAL app's own `/api/dashboard/terminals/attention/events`
  (the LOCAL, non-forwarded stream) and assert it does NOT contain that
  remote terminal's entry — proves the two hubs are genuinely separate
  per-daemon state, not one shared/global hub that happens to also get
  forwarded. Local-only green does not prove this property; only this
  negative half of the assertion does, per the design note that a
  "dashboard-scope" bug would still pass every local-only test.
- `npm run build > /tmp/attn-build.log 2>&1` `echo $?` (from
  `ws-dashboard/frontend/`) — must be clean; catches the `agentAttention.ts`
  module and the new `App.tsx` effect under the `Bundler` tsconfig program.
- All pure-TS `npm run test:*` suites (from `ws-dashboard/frontend/`) — must
  stay green (21 suites at Phase 2's count) plus a new suite/spec file for
  `agentAttention.ts`'s pure parse/endpoint helpers.
- Browser acceptance: **not required for visible-UI reasons** — Phase 5 adds
  no rendered surface (Phase 6 owns the tab-label indicator), so the
  mental-model rule requiring browser-level verification for
  *visible*-UI-changing work does not trigger here. Still worth one narrow
  network-level Playwright assertion if convenient: that a request to
  `/api/dashboard/terminals/attention/events` (or its server-scoped form) is
  observed opening once `serversView` is populated — but this is a nice-to-have,
  not a gate, and must not become a dependency on a real vendor agent CLI (the
  same rule Phase 2/6 state explicitly). If added, run it alongside the full
  suite: `npx playwright test dashboard-acceptance.spec.ts` — judge by FAILURE
  SITE; the suite has a known unrelated pre-existing failure at ~line 3779
  (`todo/260725-bug-dashboard-fitnow-short-viewport-shrink`) and passing
  everything else is the bar, not a hard zero-failure count.

## Escalations

- None.

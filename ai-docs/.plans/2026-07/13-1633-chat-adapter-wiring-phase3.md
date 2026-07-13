# Plan: 260713-feat-ws-dashboard-agent-chat-real-adapter-wiring — Phase 3: Missing capability control variants (scoped by harness tiering)

## Relevant Ticket Contract

- Add whatever `CodexControlRequest` needs beyond today's `Compact`/`Steer`/
  `Skills` to support fork-from-here against a real Codex session — the only
  real-fork wiring in MVP scope (Goal section). Touch point:
  `codex_routes.rs` enum (~L88-93) and match arm (~L177-191).
- Do NOT add a live Claude fork control variant (Hack-tier, out of scope).
- Cross-check every candidate against
  `ai-docs/mental-model/ws-dashboard-agent-harness.md`'s tiering; only wire
  what the tiering confirms the harness genuinely supports. Follow the
  `260711` Phase 3 scaffolded-disabled precedent for anything that doesn't
  clear the bar (e.g. rewind stays out — no harness qualifies for
  point-based resume).
- Verification bar: a Rust unit/route test on the new match arm(s)
  confirming dispatch; manual confirmation that fork-from-here against a
  real Codex session produces a new session with the correct transcript cut
  point (defer manual check to Phase 4 if no live daemon available).
- Ticket-wide constraint: no phase may land "designed but not connected" —
  Phase 1's own docstrings in `activitySessionClient.ts` and a comment in
  `App.tsx` explicitly defer two pieces of frontend follow-through to this
  phase (see Codebase Findings) — those are in scope here, not scope creep,
  because leaving them unfixed reproduces exactly the failure mode this
  ticket exists to correct.
- `260713-feat-ws-dashboard-activity-session-fork-cursor` Phase 1 progress
  note: the daemon-side fork handler is this phase's scope; that idea
  ticket stays open until this phase lands and is end-to-end verifiable.

## Out of Scope

- Claude fork-from-here (Hack-tier; needs its own ticket with experimental
  UI labeling/owner sign-off per the mental model's Extension Points).
- Rewind/resume-from-here for any harness (already scaffolded-disabled per
  `260711` Phase 3; not re-litigated here).
- `goal`/`rewind` `CodexControlRequest` variants — `capabilities()` reports
  `goal: true` today but no adapter method or route backs it yet; the
  ticket's "at minimum" framing scopes this phase to fork only. Leave
  `goal` unwired (do not invent a variant nobody asked for this phase).
- OpenCode (Phase 3 of `260620`, blocked on install, unrelated ticket).
- Phase 2's polling/streaming mechanics (already landed).
- SSE/websocket transport (explicit ticket non-goal, unchanged).
- Full E2E automated test and the mandatory manual browser walkthrough —
  that is Phase 4's stated scope; this phase's bar is the unit/route test
  plus a best-effort manual check if a live daemon happens to be available.

## Codebase Findings

- `ws-dashboard/crates/daemon/src/codex_routes.rs#L88-94` — `CodexControlRequest`
  enum, `#[serde(rename_all = "camelCase", tag = "action")]`. Add
  `Fork { cut_cursor: Option<String> }` (serializes as
  `{"action":"fork","cutCursor":...}`), matching the wire shape the
  frontend already sends (`activitySessionClient.ts:314`:
  `{ action: "fork", cutCursor: request.cutCursor ?? null }`).
- `ws-dashboard/crates/daemon/src/codex_routes.rs#L170-205` — the exhaustive
  match arm in `codex_session_control`; add a `Fork` arm calling a new
  `provider.fork(activity_id, cut_cursor)` mirroring the `Compact`/`Steer`
  arms' `.map(|result| CodexControlResponse{...})` shape. Put the new
  `activityId`/`cutCursor` in `CodexControlResponse.data` (already
  `Option<Value>`, same field `Skills` uses).
- `ws-dashboard/crates/daemon/src/codex_app_server.rs#L750-761` —
  `capabilities()` already reports `fork: true` (and `goal: true`, unused).
  No code changes needed here; confirms the frontend-visible capability
  gate is already open for Codex fork.
- **Fixture-verified `thread/fork` schema** (this session, `codex app-server
  generate-json-schema --out <dir> --experimental` against the installed
  `codex-cli 0.144.1`, same version the mental model's existing fixture
  pass used): `ThreadForkParams` requires `threadId`; accepts optional
  `lastTurnId` ("last turn id to fork through, inclusive... turns after
  `last_turn_id` are omitted... cannot be in progress") — this is the real,
  turn-granular cut-point primitive, not the coarser `thread/rollback`
  `numTurns`-from-end shape. `ThreadForkResponse` returns `{ thread: Thread
  { id, turns: Turn[], ... } }` where `turns`/`items` carry the full
  forked-thread history inline in the RPC response (no separate fetch
  needed) — `Thread.turns` doc: "Only populated on `thread/resume`,
  `thread/rollback`, `thread/fork`, and `thread/read`". This resolves the
  ticket's only real open contract question (exact fork params/response
  shape) with a live-fixture answer, not inference.
- **Cursor/turn-id granularity mismatch (non-obvious constraint)**:
  `crates/core/src/codex_projection.rs#L146-165`
  (`transcript_blocks`/`cursor: index.to_string()`) — the frontend's
  `cutCursor` is an ordinal **item** index; `thread/fork`'s cut param is a
  **turn** id. The projector currently discards turn ids entirely by
  design (`CONTRACT` comment at `codex_projection.rs#L11-17`: "provider
  ids... never copies them into any output `TranscriptBlock`" — this is
  about the outward-facing block only, so an internal-only turn-id map does
  not violate it). `codex_app_server.rs#L387-395` (reader loop) already
  extracts `turn.id` off the raw `turn/started` notification for the
  unrelated `active_turn_id`/steer-guard purpose — same shape can be
  reused to stamp each transcript block with the turn it belongs to.
  Implementation needs: extend `CodexProjector` with an internal
  `order_turn_ids: Vec<Option<String>>` (or equivalent per-order-index map)
  populated from `turn/started`'s `params.turn.id` at ingestion time, plus
  a lookup method (e.g. `turn_id_for_cursor(&self, cursor: &str) ->
  Option<String>`) that the route/provider layer uses to resolve
  `cutCursor` -> `lastTurnId` before calling `thread/fork`. `cutCursor:
  None` means fork the whole thread (`lastTurnId: None`).
- `ws-dashboard/crates/daemon/src/codex_app_server.rs#L1025-1095`
  (`create_session`) — the pattern to mirror for `fork()`: `check_plugin_gate`
  first (a fork spawns a new child process, same precondition as any new
  spawn per the mental model's hard-spawn-precondition rule), then
  `self.resolver.resolve_cwd(&session.work_root_id)`, then
  `self.spawn_connection(&cwd)` (a **new** connection/child process — do
  not reuse the source session's connection). Rationale: `thread/fork`'s
  schema doc says it "load[s] the thread from disk by thread_id," i.e. it
  does not require the source connection to be alive; spawning a dedicated
  new connection per forked thread keeps the existing 1-connection-per-
  session invariant (`CodexSession` has exactly one `connection`) and
  avoids adding notification demultiplexing (today one connection's
  notification pump feeds exactly one projector, unfiltered — sharing a
  connection across two threads would require routing every notification by
  `threadId`, a materially bigger change not needed here).
- `ws-dashboard/crates/daemon/src/codex_app_server.rs#L911-931`
  (`project_skills_list`) — existing precedent for projecting a raw
  JSON-RPC **response** (not the live notification stream) into a
  browser-safe shape. Model a new `project_fork_turns(&raw) ->
  Vec<TranscriptBlock>`-style pure function the same way, to seed the new
  session's initial transcript from `ThreadForkResponse.thread.turns`
  (`Turn.items: ThreadItem[]`). Without this, the new `CodexSession`'s
  projector starts empty and the browser would show zero pre-fork history
  even though the provider-side thread has it — that would fail the
  ticket's own manual verification bar ("produces a new session with the
  correct transcript cut point"). This can share render-kind/text-bounding
  helpers with `codex_projection.rs`'s existing per-item-type formatting
  (`CODEX_RENDER_KIND_*` constants, `MAX_BLOCK_TEXT` bounding) but consumes
  the `Turn`/`ThreadItem` JSON shape (already-assembled items), not the
  single-item `item/started`/`item/completed` notification shape — a
  distinct, pure, independently-unit-testable function (no live process
  needed for this half).
- `ws-dashboard/crates/daemon/src/codex_app_server.rs#L1057-1067,1094`
  (`create_session`'s registry insert) — new forked `CodexSession` needs a
  fresh `activity_id` (`new_activity_id()`), the new `thread_id` from
  `ThreadForkResponse.thread.id`, the same `server_id`/`work_root_id` as
  the source session, and `self.registry.insert(session)?` (same
  `MAX_CODEX_SESSIONS` cap enforcement as any new session).
- `ws-dashboard/crates/daemon/tests/routes.rs#L13962-14127`
  (`spawn_codex_reply_peer` + `codex_session_control_skills_projects_without_raw_json`)
  — the test pattern to mirror for the new unit/route test: seed a session
  via `insert_session_for_tests` backed by an in-memory duplex mock
  connection that replies to one RPC call with a fixed JSON result, POST to
  `/control`, assert `applied`/`data` shape and that no raw provider
  id/path leaks. **Testability gap**: `create_codex_session` (which also
  calls the real `spawn_connection`/`check_plugin_gate`) has **no** existing
  unit/route test — spawning a real child process is untested today,
  deferred to manual/E2E per this ticket's own Phase 4 framing. The new
  `fork()` provider method has the same untestable half (the new
  connection spawn + live `thread/fork` round-trip). Scope the new unit
  test to what's actually testable without a seam change: (a) the
  `CodexControlRequest::Fork` JSON deserialization/dispatch shape (mirrors
  the existing Skills route test, using `insert_session_for_tests` for the
  *source* session only, and a lightweight check that the route rejects a
  missing/unknown source session the same way other arms do), and (b) the
  new `project_fork_turns`-style pure projection function against a fixture
  `ThreadForkResponse`-shaped JSON (fully unit-testable, no process spawn).
  The full spawn-a-real-process-and-fork path stays a manual/Phase-4-level
  check, consistent with `create_session`'s own precedent, not a gap
  introduced by this phase.
- `ws-dashboard/frontend/src/activitySessionClient.ts#L294-322`
  (`forkActivitySession`) — **already POSTs the correct wire shape** (
  `{action:"fork", cutCursor}`) against `/control`, but its own docstring
  says "Phase 3 owns defining what a real Fork response actually returns
  (e.g. a new `activityId`)" and today it discards the response body and
  echoes back the **request's own** `activityId`/`cutCursor` unconditionally.
  This must be updated to read the daemon's actual `data.activityId` (and
  `data.cutCursor`) out of the real `CodexControlResponse`, once the daemon
  returns one from the new `Fork` arm above — otherwise the ticket's own
  "not designed-but-disconnected" bar is violated even after the Rust side
  lands.
- `ws-dashboard/frontend/src/App.tsx#L5146-5178` (`forkAgentChatFromBubble`)
  — the Codex real-fork branch **deliberately throws** after the POST
  succeeds today (`"Codex fork is wired to the real /control endpoint, but
  the backend Fork action lands in Phase 3 - not yet usable end-to-end"`),
  by design, pending this phase. Once the daemon returns a real new
  `activityId`, this branch needs to call `applyAgentChatSession(newPane.
  logicalKey, ...)` with the forked session (mirroring the stub branch
  immediately below it, `L5168-5177`) instead of always throwing — same
  `registerNewAgentChatPane`/`applyAgentChatSession` pattern already used
  by the stub path.
- `ai-docs/tickets/idea/260713-feat-ws-dashboard-activity-session-fork-cursor.md`
  — stays open in `idea/` until this phase lands and is end-to-end
  verifiable; not this plan's job to move, but the executor should be aware
  landing this phase is what that ticket is waiting on.

## Implementation Plan

1. `crates/core/src/codex_projection.rs` — extend `CodexProjector` with
   internal turn-id tracking: capture `params.turn.id` on `turn/started`
   (currently discarded) into a `current_turn_id: Option<String>` field;
   record it per order-index as items are appended in `ingest_item`
   (parallel `Vec`/`BTreeMap` keyed the same way `order`/`blocks` are);
   expose a `turn_id_for_cursor(&self, cursor: &str) -> Option<String>`
   lookup. Do not add turn id to `TranscriptBlock` (keep the existing
   privacy contract for the outward struct).
2. `crates/core/src/codex_projection.rs` — add a pure
   `project_fork_turns(thread: &Value) -> Vec<TranscriptBlock>` (or
   similarly-named function near `project_skills_list`'s style, but
   colocated in `codex_projection.rs` since it produces `TranscriptBlock`s)
   that maps `ThreadForkResponse.thread.turns[].items[]` (`ThreadItem`
   union: `userMessage`/`agentMessage`/`reasoning`/`commandExecution`/
   `fileChange`/etc.) into the same `TranscriptBlock` shape/render-kinds
   `ingest_item` already produces for live events, reusing
   `CODEX_RENDER_KIND_*` constants and text-bounding helpers where the
   shapes line up. Unsupported/unknown item types degrade to a bounded
   status block, same discipline as `ingest_item`.
3. `crates/daemon/src/codex_app_server.rs` — add
   `CodexAppServerProvider::fork(&self, activity_id: &str, cut_cursor:
   Option<&str>) -> Result<(String, Option<String>), AgentClientProviderError>`
   (returns `(new_activity_id, echoed_cut_cursor)`): resolve source session
   via `self.session(activity_id)?`; resolve `lastTurnId` via
   `session.projector.lock().await.turn_id_for_cursor(cursor)` when
   `cut_cursor` is `Some`; `check_plugin_gate(&self.config.codex_bin).await?`;
   `let cwd = self.resolver.resolve_cwd(&session.work_root_id)?`; spawn a
   **new** connection via `self.spawn_connection(&cwd).await?` (mirrors
   `create_session`, does not reuse `session.connection`); issue
   `thread/fork` with `{"threadId": session.thread_id, "lastTurnId":
   resolved_turn_id}`; extract `thread.id` as the new thread id and
   `thread.turns` for seeding; build a new `CodexProjector` seeded via
   `project_fork_turns(&thread_value)`; construct and `self.registry.insert(...)`
   a new `CodexSession` (new `activity_id`, new `thread_id`, same
   `server_id`/`work_root_id`, the new connection, the seeded projector);
   spawn the notification pump for the new connection/projector the same
   way `create_session` does; return `(new_activity_id, cut_cursor)`.
4. `crates/daemon/src/codex_routes.rs#L88-94` — add
   `Fork { cut_cursor: Option<String> }` to `CodexControlRequest` (with
   `#[serde(rename_all = "camelCase", ...)]` already on the enum, this
   serializes/deserializes as `cutCursor`).
5. `crates/daemon/src/codex_routes.rs#L170-205` — add the `Fork` match arm:
   call `provider.fork(&activity_id, cut_cursor.as_deref()).await`, map
   `Ok((new_activity_id, echoed_cursor))` to
   `CodexControlResponse { applied: true, data: Some(json!({ "activityId": new_activity_id, "cutCursor": echoed_cursor })) }`,
   `Err` through the existing `provider_error_response`.
6. `frontend/src/activitySessionClient.ts#L306-322` (`forkActivitySession`)
   — replace the "echo the request back" placeholder: parse
   `readJson<RealControlResponse>(...)`, read `response.data.activityId`/
   `response.data.cutCursor`, and return those (falling back to the
   request's own values only if the daemon predictably omits them, which
   post-Phase-3 it should not).
7. `frontend/src/App.tsx#L5156-5178` (`forkAgentChatFromBubble`, Codex
   branch) — replace the unconditional `throw` after `realForkActivitySession`
   resolves with applying the returned session onto `newPane` via
   `applyAgentChatSession`, mirroring the stub branch immediately below.
   This likely needs a small adapter since `realForkActivitySession`
   currently returns only `{activityId, cutCursor}` (no transcript) — check
   whether `applyAgentChatSession` requires a full session object or can
   accept a bare activity id plus a subsequent transcript fetch/poll
   kick-off (the existing `beginRealStreamingTurn`/polling path already
   fetches transcript by activity id); wire whichever is the smaller diff
   consistent with the existing pane-registration flow.

## Verification Plan

- `cargo test -p ws-dashboard-core codex_projection` (or the crate's actual
  test invocation) — new unit tests for `turn_id_for_cursor` (order-index ->
  turn id mapping across multiple turns) and `project_fork_turns` (fixture
  `ThreadForkResponse.thread.turns` JSON -> expected `TranscriptBlock`s,
  including an unsupported-item-type degrade case).
- `cargo test -p ws-dashboard-daemon` — new route/unit test in
  `crates/daemon/tests/routes.rs` mirroring
  `codex_session_control_skills_projects_without_raw_json`: seed a source
  session via `insert_session_for_tests`, POST `{"action":"fork","cutCursor":...}`
  to `/control`, and assert on the dispatch/response shape reachable
  without a real process spawn (deserialization correctness, error mapping
  for an unknown source `activityId`). Do not attempt to assert the full
  spawn-a-new-connection path in this test — that half has no existing test
  seam anywhere in this file (same gap `create_session` already has) and is
  out of this phase's testable surface.
- Frontend: `npm run build` (tsc -b + vite build) and the existing
  `test:agent-chat-client` / `test:agent-chat-tabs` suites, extended with a
  case asserting `forkActivitySession` now surfaces the daemon's
  `data.activityId` rather than echoing the request, and that
  `forkAgentChatFromBubble`'s Codex branch applies the new pane instead of
  throwing.
- Manual (defer to Phase 4 / a human pass if no live daemon is available in
  this environment): open a real Codex session in the chat UI, use
  "fork from here" on a mid-transcript bubble, confirm a new pane opens
  showing exactly the transcript up to and including that bubble's turn
  (turn-granularity caveat: `lastTurnId` cuts at turn boundaries, so a
  cut bubble that is not the last item of its turn will still bring the
  rest of that turn along — this is the real harness's actual granularity,
  not a bug to work around, and is worth surfacing in UI copy if it reads
  as surprising).

## Escalations

- None. Confidence: high — the exact `thread/fork` wire shape was
  fixture-verified this session (not inferred) against the same installed
  `codex-cli 0.144.1` the mental model's prior fixture pass used, and every
  new piece of work mirrors an existing in-repo pattern (`create_session`
  for spawn-and-register, `project_skills_list` for response projection,
  `codex_session_control_skills_projects_without_raw_json` for the route
  test). The one genuine design choice made here (new connection per fork,
  not reusing/demuxing the source connection) is justified above by both
  the schema's own "load from disk by thread_id" language and by avoiding
  a much larger notification-routing change; flagged in Codebase Findings
  for the executor's awareness rather than left implicit.

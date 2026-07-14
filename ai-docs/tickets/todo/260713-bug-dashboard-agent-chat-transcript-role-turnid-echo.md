---
title: "Agent chat replies never render due to a streaming-poll race, and fork/resumed transcripts misclassify/split the user's own message"
sage-review-design: required
related:
  260711-feat-ws-dashboard-agent-activity-chat-ui: introduced the frontend TranscriptBlock role/turnId contract this ticket now fills in
  260713-feat-ws-dashboard-agent-chat-real-adapter-wiring: wired real Codex/Claude adapters without completing this contract
---

# Agent chat replies never render due to a streaming-poll race, and fork/resumed transcripts misclassify/split the user's own message

## Background

Dogfooding with a real no-auth local server + real Codex/Claude sessions
surfaced two independent defects. An earlier design-review cycle blocked a
prior version of this ticket for premising Issue 1 on "no optimistic echo
exists" — that premise was wrong (an optimistic echo does exist for the
live-send path). Both issues below were re-diagnosed and confirmed against
current code, using a headless-Playwright + no-auth-daemon procedure now
recorded at `ai-docs/ref/dashboard-headless-browser-verification.md` (two
independent methods converged on the same conclusion for Issue 1: a static
call-graph trace, and a live dynamic reproduction driving the real
`activitySessionClient.ts` functions directly against the running daemon).

### Issue 1: real-time chat replies never render — a client-side poll race (confirmed primary cause of "no response")

Sending "hello" in a live Codex or Claude tab shows the user's own
optimistic bubble (this already works — see Issue 2's note on why), but the
real agent's reply never appears, even though the daemon does complete the
turn and store the reply within seconds. This reads as "no response" and is
what "fork" appeared to "fix" (fork opens a fresh replay-seeded session that
polls independently and isn't affected by the stuck poll loop below).

Root cause, confirmed both by tracing `beginRealStreamingTurn`
(`ws-dashboard/frontend/src/activitySessionClient.ts:436-492`) against
`codex_app_server.rs`'s `send_prompt`/turn-notification handling, and by
driving the real, unmodified `activitySessionClient.ts` functions directly
against the live no-auth daemon:

- `beginRealStreamingTurn` fires one **immediate** poll at call time (before
  the first `realStreamingPollIntervalMs` interval), then schedules the
  regular interval. That immediate poll's stop condition is
  `if (!transcript.live) { stop... onComplete() }` — unconditionally, even
  when zero new blocks were seen.
- `sendAgentChatPrompt` only awaits the `turn/start` JSON-RPC
  acknowledgment. The daemon's `live` flag (`projector.is_turn_active()`,
  `codex_app_server.rs:847,887`) only flips to `true` once the projector
  ingests a separate, asynchronously-delivered `turn/started`
  *notification* (`codex_projection.rs:110,167,265`, pumped by
  `spawn_projector_pump` on its own notification stream, decoupled from the
  RPC response `send_prompt` awaits).
- In practice the RPC ack resolves before that notification lands. Measured
  live: `POST /prompt` acknowledged, then the immediate poll's
  `GET /transcript` lands 1-9ms later and returns `live: false`, `0` new
  blocks — the turn hasn't been marked active yet, but this looks
  identical to "turn already finished with nothing new." The poll loop
  calls `onComplete()` and permanently clears the interval. The real turn
  actually completes ~10s later with real reply blocks, but nothing is
  polling for them anymore.
- Confirmed via direct daemon `curl` polling in parallel: the same session's
  transcript endpoint does show the real assistant reply text within ~10-20s
  of the prompt — the daemon-side data is fine; the frontend poll loop gave
  up before it existed.

### Issue 2: fork/resumed transcripts misclassify the user's own message and split multi-block replies (missing `role`/`turnId`)

The live-send path already has a working optimistic echo:
`App.tsx`'s `beginSimulatedTurn` -> `sendAgentChatMessage` ->
`appendUserTranscriptBlock` (`agentChatSessions.ts:181`) appends a real,
right-aligned, `role: "user"` block to the active tab's own session state
before POSTing `/prompt` — confirmed rendering correctly in a live headless
browser test (right-aligned, blue-highlighted, appears immediately on
send). This optimistic block is **client-local to the active tab's own
session state only**.

It does **not** cover: reopening/resuming a session, or fork's replay of
stored history (`CodexProjector::seeded`, `260713-feat-...-adapter-wiring`
Phase 3) — both fetch the transcript fresh from the daemon with no
client-local echo. In those paths, `TranscriptBlock`
(`crates/core/src/activity.rs:153`) has never carried a `role` or `turnId`
field, even though the frontend's bubble-grouping logic
(`agentChatBubbles.tsx`, `workRootActivity.ts`) was written expecting them
as "additive, optional fields" back in `260711` Phase 2. This is exactly
what was observed when using "fork": the replayed "hello" rendered
left-aligned (as if agent-authored) because `chatBlockRole()`'s heuristic
fallback defaults an unlabeled block to `"agent"`; and the real reply's
multiple text blocks rendered as separate, disconnected bubbles because
`groupTranscriptIntoBubbles`'s `canMerge` requires a matching `turnId`
that was never sent (`codex_projection.rs` tracks `turn_id`/
`order_turn_ids` purely internally for the fork-cursor feature, and
Claude's projector has no turn-id tracking exposed either).

**Resolved design decision** (this is what blocked the prior version of
this ticket): do **not** remove Codex's `suppress_local_prompt`
(`codex_app_server.rs:1072,1111`; suppression state in
`codex_projection.rs:151,292-315`). It only suppresses the app-server's
echoed `userMessage` for the *live send* path, which the optimistic block
already renders correctly — removing it would double-render that path
(the optimistic block and the daemon's echo have different cursors, so
`mergeStreamingTranscriptBlocks`'s cursor-keyed dedup would show both).
Fork/resume never calls `send_prompt` and its seeded projector has no
suppression state (confirmed in existing code), so leaving suppression
untouched has no effect on Issue 2's fork/resume scenario at all. This
phase is scoped to adding `role`/`turn_id` metadata only — no suppression
change, no source-of-truth ambiguity between Codex and Claude.

## Constraints

- Do not weaken or remove the existing "browser-privacy" cursor contract
  established in `260713-feat-...-adapter-wiring` Phase 3 (provider turn
  ids/cursors stay internal-only where that contract applies) — exposing
  a `turnId` on `TranscriptBlock` for bubble-grouping is a distinct,
  already-anticipated "additive, optional" field per `260711`'s own
  CONTRACT comment, not a reversal of that privacy decision.
- `TranscriptBlock` is a shared wire type serialized to the browser;
  adding fields is additive/backward-compatible (existing optional-field
  handling on the frontend already tolerates their absence), but must
  stay consistent across both the Codex and Claude projection paths.
- Do not remove or weaken Codex's `suppress_local_prompt` — see the
  resolved design decision above; note this explicitly in a code comment
  at the suppression call sites so a future implementer doesn't
  "complete" this ticket by deleting it under the mistaken assumption
  that Issue 1 needed it removed.
- Issue 1's fix must not assume any particular turn completes within a
  fixed short duration — real Codex/Claude turns can take many seconds;
  the fix must distinguish "turn not yet marked active" from "turn
  genuinely produced nothing new," not rely on a timeout guess.

## Phases

### Phase 1: Fix the streaming-poll race so real-time replies actually render

- `activitySessionClient.ts`'s `beginRealStreamingTurn`: stop treating the
  very first (immediate) poll's `live: false` + zero-new-blocks result as
  turn completion. The daemon's `live` flag is not guaranteed to reflect
  the just-started turn yet on that first poll (see Background). Only
  honor a `live: false` stop condition once, either (a) the loop has seen
  `live: true` at least once for this turn, or (b) at least one
  interval-scheduled (non-immediate) poll has run. Keep the immediate
  poll's `onUpdate` behavior unchanged (it still exists to reduce initial
  latency for a turn that legitimately completes instantly); only change
  the completion/stop decision.
- Add unit test coverage in `activitySessionClient.test.ts` (existing
  fake-timer poll environment) covering: an immediate poll that returns
  `live: false` + 0 new blocks must NOT stop the loop; a subsequent poll
  that returns real blocks and `live: false` must stop the loop and
  deliver those blocks via `onUpdate`.
- **Verification**: exercise against a real running no-auth daemon per
  `ai-docs/ref/dashboard-headless-browser-verification.md` — send a real
  prompt to a live Codex or Claude tab and confirm the reply renders in
  the browser without needing fork, waiting for the harness's actual
  (multi-second) completion time rather than checking only immediately
  after send.

### Phase 2: Populate `role`/`turn_id` on `TranscriptBlock` for fork/resume rendering correctness

- Add `role: Option<String>` and `turn_id: Option<String>` to
  `TranscriptBlock` (`crates/core/src/activity.rs`).
- `codex_projection.rs`: set `role` on every constructed block (`"user"`
  for `userMessage`/`hookPrompt`, `"agent"` for `agentMessage`, `"tool"`
  for tool/mcp items, leave `thinking` blocks' role unset since
  `renderKind` already disambiguates them on the frontend); set
  `turn_id` from the already-tracked `current_turn_id` for every block.
  Do not touch `suppress_local_prompt` or its plumbing (see Constraints).
- `claude_projection.rs`: set `role`/`turn_id` analogously on whatever
  turn/message correlation data this projector already has available;
  if Claude's stream-json protocol exposes no per-turn id, a daemon-side
  synthesized counter per turn boundary is acceptable — investigate the
  exact available signal during implementation (a survey plan should
  resolve this `unknown` before editing).
- Frontend: no `chatBlockRole()`/`groupTranscriptIntoBubbles` logic change
  should be needed — they already read `role`/`turnId` per the existing
  `260711` contract; this phase only makes the backend populate what the
  frontend already expects. Verify this assumption during implementation
  rather than taking it on faith.
- Add/extend unit test coverage in `codex_projection.rs` and
  `claude_projection.rs` for the new fields, plus a route-level or
  frontend test proving a real multi-block turn now merges into one
  bubble and a fork-replayed user block now renders right-aligned.
- **Verification**: use fork's replay path (not a live send, since the
  live path's own user block already comes from the client-local
  optimistic echo) — confirm the replayed user message renders
  right-aligned and a multi-block real reply merges into one bubble.

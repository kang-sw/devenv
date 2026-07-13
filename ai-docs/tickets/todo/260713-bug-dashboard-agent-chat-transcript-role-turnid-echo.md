---
title: "Agent chat transcript never carries the user's own message, and misclassifies/splits real Codex+Claude replies"
sage-review-design: required
related:
  260711-feat-ws-dashboard-agent-activity-chat-ui: introduced the frontend TranscriptBlock role/turnId contract this ticket now fills in
  260713-feat-ws-dashboard-agent-chat-real-adapter-wiring: wired real Codex/Claude adapters without completing this contract
---

# Agent chat transcript never carries the user's own message, and misclassifies/splits real Codex+Claude replies

## Background

Dogfooding with a real no-auth local server + real Codex/Claude sessions
surfaced three related defects, all rooted in the same gap: the daemon's
`TranscriptBlock` (`crates/core/src/activity.rs:153`) has never carried a
`role` or `turnId` field, even though the frontend's bubble-grouping logic
(`agentChatBubbles.tsx`, `workRootActivity.ts`) was written expecting them
as "additive, optional fields" back in `260711` Phase 2. Real adapter
wiring (`260713-feat-ws-dashboard-agent-chat-real-adapter-wiring`) never
filled that gap in.

### Issue 1: the user's own submitted message never appears anywhere

Sending "hello" in a live Codex or Claude tab shows no acknowledgment at
all in the pane — reads as "no response," even though the real agent
does reply within a few seconds. Root cause, confirmed by directly
querying the running no-auth daemon's transcript endpoint and
cross-checking against the real Codex CLI's own rollout JSONL
(`~/.codex/sessions/...`):

- **Codex**: `CodexAppServerProvider::send_prompt` calls
  `projector.suppress_local_prompt(text)` before sending the turn
  (`codex_app_server.rs:1104-1123`; suppression logic in
  `codex_projection.rs:151,302-314`). This assumes the app-server's
  echoed `userMessage` item is redundant because the browser already
  shows its own optimistic echo of what it just sent.
- **That assumption is false.** `App.tsx`'s `beginSimulatedTurn` does not
  create any local optimistic bubble for real Codex/Claude harnesses —
  it only POSTs `/prompt` and polls the resulting transcript. Confirmed
  directly: `curl`ing a live session's transcript endpoint after sending
  "hello" shows zero `User`-titled blocks, only the assistant's
  thinking/tool/reply blocks.
- **Claude**: no suppression exists because none is needed — per
  `claude_projection.rs`'s own CONTRACT comment, Claude's
  `stream-json` protocol never echoes the client's own prompt as an
  output event at all. Same end-user-visible result: the user's message
  never becomes a block.
- The message only reappears after using "fork," because fork creates a
  brand-new session/projector (`CodexProjector::seeded`, from
  `260713-feat-...-adapter-wiring` Phase 3) that replays the full stored
  thread from scratch with no suppression state — this is what actually
  makes the "hello" (and the assistant's real replies) visible for the
  first time, misleadingly making it look like fork "fixed" the
  non-response.

### Issue 2: the user's own message renders on the wrong (agent) side

Once a `User`-titled block does get projected (e.g. via fork's replay
path), it still renders left-aligned as if it were an agent message.
`agentChatBubbles.tsx`'s `chatBlockRole()` checks `block.role === "user"`
first, but the daemon has never sent a `role` field on any
`TranscriptBlock` for either harness — every backend-sourced block
defaults through the heuristic fallback to `"agent"` unless it matches a
tool/mcp/thinking pattern.

### Issue 3: a single real turn renders as multiple disconnected response bubbles

A real Codex/Claude turn frequently emits more than one text block for
one logical reply (e.g. an initial orientation message plus a follow-up
"what would you like to work on?" message, confirmed in real rollout
logs from this session's dogfooding). `groupTranscriptIntoBubbles`'s
`canMerge` only merges same-turn blocks when `turnId` matches on both
sides, but `turnId` is also never sent — `codex_projection.rs` tracks
`turn_id`/`order_turn_ids` purely internally (added in `260713-feat-...
-adapter-wiring` Phase 3 for the fork-cursor feature, deliberately kept
off the outward `TranscriptBlock` for the browser-privacy/cursor
contract) and Claude's projector has no turn-id tracking exposed either.
Net effect: multi-block turns render as separate, seemingly unrelated
response bubbles instead of one coherent reply.

## Constraints

- Do not weaken or remove the existing "browser-privacy" cursor contract
  established in `260713-feat-...-adapter-wiring` Phase 3 (provider turn
  ids/cursors stay internal-only where that contract applies) — exposing
  a `turnId` on `TranscriptBlock` for bubble-grouping is a distinct,
  already-anticipated "additive, optional" field per `260711`'s own
  CONTRACT comment, not a reversal of that privacy decision. Confirm the
  two don't conflict before implementing.
- `TranscriptBlock` is a shared wire type serialized to the browser;
  adding fields is additive/backward-compatible (existing optional-field
  handling on the frontend already tolerates their absence), but must
  stay consistent across both the Codex and Claude projection paths.
- Removing Codex's `suppress_local_prompt` call must not reintroduce a
  double-rendered user message if a future change ever does add a local
  optimistic echo bubble — note the coupling explicitly in code comments
  at both ends (send path and any future optimistic-echo code) so the
  next person doesn't reintroduce Issue 1 by fixing only one side.

## Phases

### Phase 1: Populate `role`/`turnId` on `TranscriptBlock` and stop suppressing the user's own Codex prompt

Single combined phase (deliberately not split further — the three issues
share the same `TranscriptBlock` projection code paths and are cheapest
to fix and verify together):

- Add `role: Option<String>` and `turn_id: Option<String>` to
  `TranscriptBlock` (`crates/core/src/activity.rs`).
- `codex_projection.rs`: set `role` on every constructed block
  (`"user"` for `userMessage`/`hookPrompt`, `"agent"` for
  `agentMessage`, `"tool"` for tool/mcp items, leave `thinking` blocks'
  role unset since `renderKind` already disambiguates them on the
  frontend); set `turn_id` from the already-tracked `current_turn_id`
  for every block. Remove (or make a no-op) the `suppress_local_prompt`
  call in `CodexAppServerProvider::send_prompt`, and delete the
  now-dead suppression plumbing (`suppressed_prompts`,
  `suppressed_item_ids`, `take_suppressed_prompt`) if nothing else uses
  it — confirm via a full-repo grep before deleting.
- `claude_projection.rs`: set `role`/`turn_id` analogously on whatever
  turn/message correlation data this projector already has available
  (check what per-message/turn id Claude's stream-json protocol actually
  exposes; if none exists, `turn_id` may need a daemon-side synthesized
  counter per turn boundary — investigate during implementation, this
  is exactly the kind of `unknown` fact a survey plan should resolve
  before editing).
- Frontend: no `chatBlockRole()`/`groupTranscriptIntoBubbles` logic
  change should be needed — they already read `role`/`turnId` per the
  existing `260711` contract; this phase is about the backend finally
  populating what the frontend already expects. Verify this assumption
  during implementation rather than taking it on faith.
- Add/extend unit test coverage in `codex_projection.rs` and
  `claude_projection.rs` for the new fields, plus a route-level or
  frontend test proving a real multi-block turn now merges into one
  bubble and a user block now renders right-aligned.

**Verification**: exercise against the real running no-auth daemon (or
equivalent fixture) the same way this bug was diagnosed — send a message,
confirm a `User`-role block appears in the polled transcript without
needing fork, confirm it renders right-aligned, and confirm a multi-block
real reply merges into one bubble. A live-browser visual confirmation is
preferred if available; if not, the daemon-transcript-API-level
verification (curl/HTTP against a real running session) used to diagnose
this bug is an acceptable substitute, since it directly exercises the
same wire shape the frontend consumes.

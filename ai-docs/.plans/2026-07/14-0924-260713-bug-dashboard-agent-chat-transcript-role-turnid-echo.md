# Plan: 260713-bug-dashboard-agent-chat-transcript-role-turnid-echo — whole target

## Relevant Ticket Contract

- Phase 1: `beginRealStreamingTurn` must not honor a `live: false` stop
  decision on the very first (immediate) poll unless the loop has already
  seen `live: true` at least once, OR at least one interval-scheduled
  (non-immediate) poll has already run. `onUpdate` behavior on the immediate
  poll is unchanged — only the stop/complete decision changes.
- Phase 1 must add `activitySessionClient.test.ts` coverage: an immediate
  poll returning `live:false` + 0 new blocks must NOT stop the loop; a
  subsequent poll returning real blocks + `live:false` must stop the loop
  and deliver blocks via `onUpdate`.
- Phase 1 verification: exercise against a real no-auth daemon per
  `ai-docs/ref/dashboard-headless-browser-verification.md`, waiting for the
  harness's actual multi-second completion, not just checking immediately
  after send.
- Phase 2: add `role: Option<String>` and `turn_id: Option<String>` to
  `TranscriptBlock` (`crates/core/src/activity.rs`).
- Phase 2 Codex: `codex_projection.rs` sets `role` on every constructed
  block (`"user"` for `userMessage`/`hookPrompt`, `"agent"` for
  `agentMessage`, `"tool"` for tool/mcp items, `thinking` blocks' role left
  unset) and `turn_id` from `current_turn_id` for every block. Must not
  touch `suppress_local_prompt` or its plumbing (settled decision — add a
  code comment at the suppression call sites per Constraints so a future
  implementer doesn't "complete" this ticket by deleting it).
- Phase 2 Claude: `claude_projection.rs` sets `role`/`turn_id` analogously
  using whatever turn/message correlation signal is actually available; if
  no per-turn id exists in the protocol, a daemon-side synthesized
  per-turn-boundary counter is acceptable.
- Phase 2 frontend: no `chatBlockRole()`/`groupTranscriptIntoBubbles` change
  expected — confirmed true during survey (see Codebase Findings).
- Phase 2 tests: unit coverage in both projectors for the new fields, plus a
  route-level or frontend test proving a multi-block turn merges into one
  bubble and a fork-replayed user block renders right-aligned.
- Phase 2 verification: use fork's replay path (not a live send, since live
  send's user block already comes from the client-local optimistic echo) —
  confirm replayed user message renders right-aligned and a multi-block real
  reply merges into one bubble.
- Constraint: `TranscriptBlock` is a shared wire type; new fields must be
  additive/backward-compatible and consistent across both Codex and Claude
  projection paths.
- Constraint: Issue 1's fix must not assume any turn completes within a
  fixed duration; must distinguish "not yet marked active" from "genuinely
  produced nothing new."

## Out of Scope

- Removing/weakening Codex's `suppress_local_prompt` — explicitly resolved
  and settled; not revisited.
- `work_root_activity.rs`'s `parse_codex_session_transcript` /
  `codex_session_record` construction sites (~20 `TranscriptBlock { .. }`
  literals, lines ~1537-2138) — this is a separate legacy raw-JSONL
  session-history-file parser, distinct from the app-server
  `codex_projection.rs`/`claude_projection.rs` projectors the ticket names.
  The ticket's Phase 2 bullets name only `codex_projection.rs` and
  `claude_projection.rs`; this file is not in the selected authority's
  scope. **Risk signal for lead awareness**: if this legacy path feeds the
  same frontend bubble-grouping UI (history browsing), it will still lack
  `role`/`turnId` after this ticket ships — flagged, not expanded into.
- Claude fork/session-replay-from-history: survey confirms no equivalent to
  Codex's `project_fork_turns`/`CodexProjector::seeded` exists for Claude
  (`claude_cli.rs`'s `resume_session` just re-attaches to a live in-process
  session via `ensure_live`, it does not reseed a projector from persisted
  history). Phase 2's Claude `turn_id` work is therefore about multi-block
  same-turn merge, not fork replay — the ticket's Verification Plan for
  Phase 2 ("fork's replay path") is Codex-specific; Claude has no fork
  replay to verify against.
- Any change to `agent_client_provider.rs`'s or `activity.rs`'s own test
  fixtures beyond the mechanical field additions needed to keep them
  compiling (see Codebase Findings on construction-site fan-out).

## Codebase Findings

- `ws-dashboard/frontend/src/activitySessionClient.ts#L436-L492` —
  `beginRealStreamingTurn`, current exact shape confirmed. `poll()` closure
  at L462-484; immediate call `poll()` at L488; `env.setInterval(poll, ...)`
  at L489. The stop condition is `if (!transcript.live) { stopped = true;
  clearScheduledPoll(); onComplete?.(); }` (L471-475) with no distinction
  between the immediate call and interval-scheduled calls — matches the
  ticket's root-cause description exactly.
- `ws-dashboard/frontend/src/activitySessionClient.test.ts#L522-547` — an
  **existing passing test** ("a poll that immediately observes live:false
  stops after just the initial poll") asserts the immediate poll alone
  (`nextResponses = [pollFinalFixture]`, one queued response) stops the
  loop and fires `onComplete` after exactly one call. Under the ticket's
  literal fix (never honor `live:false` on the immediate poll unless
  `live:true` was already seen), this exact scenario would **no longer
  stop on the immediate poll** — the test's `await immediateComplete.promise`
  would hang forever since `onComplete` never fires without a second,
  interval-driven poll. This test must be rewritten as part of the Phase 1
  fix, not left as-is: change the fixture queue to two responses (e.g.
  `[pollPartialFixture-or-similar, pollFinalFixture]`), drive one manual
  `tick()`, and re-assert `calls.length === 2`. This is a required
  implementation step, not an optional test-coverage nicety.
- `ws-dashboard/frontend/src/activitySessionClient.test.ts#L416-437` —
  `makeManualPollEnv()` fake-timer environment: `setInterval` captures the
  listener and returns an incrementing handle, `tick()` invokes it
  synchronously, `clearInterval` just records into `clearedHandles`. New
  Phase 1 tests should reuse this exact helper (already the established
  pattern for all poll-loop tests in this file, L474-674).
- `ws-dashboard/frontend/src/App.tsx#L7259-7279` — the only other call site
  of `beginRealStreamingTurn`. It only consumes `onUpdate`/`onComplete`
  callbacks and does not depend on stop-timing internals; the Phase 1 fix
  is behavior-compatible with this caller (confirmed no other caller
  exists via repo-wide `beginRealStreamingTurn` grep).
- `ws-dashboard/crates/core/src/activity.rs#L151-168` — `TranscriptBlock`
  struct definition, exact current field list: `cursor`, `timestamp`,
  `render_kind`, `title`, `text`, `data`, `degraded`. No `role`/`turn_id`
  yet, confirming the ticket's premise.
- **Construction-site fan-out (mechanical, not a strategy question, but
  sizable)**: adding two new fields to `TranscriptBlock` breaks every
  existing Rust struct-literal construction until updated (no `Default`
  impl exists, and none should be added just to paper over this). Full
  count of `TranscriptBlock { ... }` literals found in the crate:
  - In-scope (ticket-named, get real `role`/`turn_id` values):
    `crates/core/src/codex_projection.rs#L198` (`transcript_blocks()`,
    needs both new fields wired to `BlockState`+`order_turn_ids`) and
    `#L654` (`project_fork_turns`, fork-replay path — the ticket's own
    module doc at L107-114 says provider turn ids are "never copied into
    `TranscriptBlock`" for `order_turn_ids`'s *internal* correlation use,
    but the new wire `turn_id` field is a distinct, ticket-approved
    additive field per Constraints, not a reversal); `claude_projection.rs
    #L169` (`transcript_blocks()`).
  - Out-of-scope but must still compile (`role: None, turn_id: None`
    mechanical fill, no behavior change):
    `crates/core/src/activity.rs#L309,#L448` (test fixtures),
    `crates/core/src/agent_client_provider.rs#L317` (test fixture),
    `crates/daemon/src/work_root_activity.rs` — approximately 20 literals
    across `codex_session_record`/`unsupported_codex_session_block`/two
    helper functions at `#L1957`,`#L2017`, and one `.map(...)` at `#L2138`
    (`parse_codex_session_transcript`'s legacy JSONL parser, see Out of
    Scope).
- `crates/core/src/codex_projection.rs#L85-90` — `BlockState` (the
  projector's internal per-item state) has no `role` field today; Phase 2
  needs to add one (or derive role purely from `item_type` at
  `upsert_block`/`ingest_item` call sites, which already know the item type
  — either approach works, deriving at construction time avoids widening
  `BlockState` at all since `role` is a pure function of which `upsert_block`
  call site handles the item, e.g. `"user"` only for the
  `userMessage`/`hookPrompt` branch at L302-315).
- `crates/core/src/codex_projection.rs#L96-115,265-274,441-457` —
  `current_turn_id`/`order_turn_ids` tracking, confirmed exact shape:
  `current_turn_id` is set from `turn/started`'s `params.turn.id` (L268-272)
  and `order_turn_ids` is a parallel `Vec<Option<String>>` pushed once per
  new item in `ensure_block` (L441-457, push at L446 using
  `self.current_turn_id.clone()`). `transcript_blocks()` (L192-209) already
  iterates `self.order` by index — the new `turn_id` field just reads
  `self.order_turn_ids[index].clone()` in the same loop; no new state
  needed for Codex's `turn_id`, only for `role`.
- `crates/core/src/codex_projection.rs#L129-146` — `CodexProjector::seeded`
  (used by fork replay) builds `BlockState` from already-projected
  `TranscriptBlock`s and discards their `turn_id`/provider-id info by
  design (`order_turn_ids.push(None)` at L134) — fork-of-a-fork turn-id
  resolution is explicitly out of this phase's scope per the existing
  module comment (L127-128), consistent with the ticket. `role`, however,
  **should** carry through `seeded()` (a forked user/agent/tool block
  should keep the role it had when originally projected) — `seeded` must
  copy `block.role` (not discard it) into the new `BlockState` it builds,
  the same way it already keeps `render_kind`/`title`/`text`/`degraded`.
- `crates/core/src/claude_projection.rs#L85-146` — `ClaudeProjector`,
  confirmed no per-turn id tracking exists at all today (only a plain
  `active_turn: bool`). `ingest_assistant` (L218-227) already computes
  `let turn_started = !self.active_turn;` before setting `active_turn =
  true` — this is the exact, already-present turn-boundary transition
  point to hook a synthesized counter into (e.g. a new
  `current_turn_seq: u64` incremented and formatted as `turn_id` only when
  `turn_started` is true). `ingest_result` (L299-345) is the matching
  turn-end point (sets `active_turn = false`); no new turn_id action is
  needed there since the counter should persist across the whole turn's
  block creations, only advancing at the *next* turn's start.
- `crates/core/src/claude_projection.rs#L28-31` — module CONTRACT
  explicitly documents that Claude's stream-json protocol never echoes the
  client's own sent prompt, i.e. **Claude's live projector never produces a
  `role: "user"` block at all** in the current protocol usage (no
  `suppress_local_prompt`-equivalent exists or is needed here). Phase 2's
  Claude role vocabulary is therefore only `"agent"` (text/thinking-unset)
  and `"tool"` (tool_use/tool_result), never `"user"` — this is a real
  protocol-shape difference from Codex's role vocabulary, not an
  implementation gap; the plan/tests should not assert a Claude `"user"`
  block ever appears.
- `crates/core/src/claude_projection.rs#L95-104,415-433` — Claude's
  `BlockState` also has no `role` field; same "derive at call site" option
  applies (`push_block`'s callers already know whether they're producing
  text/thinking vs. `upsert_tool_block`'s tool role).
- `crates/daemon/src/claude_cli.rs#L1025-1031` — `resume_session` only
  calls `ensure_live` on an existing in-process `ClaudeSession`; it never
  constructs a fresh `ClaudeProjector` from persisted history the way
  Codex's fork path does. Confirms the Out-of-Scope note above: Claude has
  no fork/resume-replay path exercising role/turn_id the way Codex's
  `project_fork_turns` does.
- `ws-dashboard/frontend/src/workRootActivity.ts#L79-106` — frontend
  `TranscriptBlock` type already declares `role?: "user" | "agent" | "tool"
  | string; turnId?: string | null;` (camelCase, matching
  `#[serde(rename_all = "camelCase")]` on the Rust struct) under an
  existing CONTRACT comment dated to `260711` Phase 2 — confirms these are
  genuinely pre-anticipated additive fields, not a new frontend contract to
  design.
- `ws-dashboard/frontend/src/agentChatBubbles.tsx#L51,92,101,120-128` —
  `chatBlockRole()` and `groupTranscriptIntoBubbles`'s `canMerge` already
  read `block.role`/`block.turnId` today (heuristic fallback to `"agent"`
  when `role` is absent, and `turnId` gates same-bubble merging) — confirms
  the ticket's "no frontend logic change needed" assumption is accurate;
  survey found no reason to doubt it.

## Implementation Plan

1. `ws-dashboard/frontend/src/activitySessionClient.ts` — in
   `beginRealStreamingTurn` (L436-492), change `poll` to accept an
   `isImmediate: boolean` parameter (or equivalent closure flag) and track a
   `sawLiveTrue` boolean, set `true` whenever a poll's `transcript.live` is
   `true`. Change the stop condition at L471 from unconditional
   `if (!transcript.live)` to `if (!transcript.live && (sawLiveTrue ||
   !isImmediate))`. Call `poll(true)` at L488 and
   `env.setInterval(() => poll(false), realStreamingPollIntervalMs)` at
   L489. Keep `onUpdate` delivery (L468-470) unchanged and unconditional.
2. `ws-dashboard/frontend/src/activitySessionClient.test.ts` —
   a. Rewrite the existing test at L522-547 (see Codebase Findings): feed a
      two-response queue, require one manual `tick()` before `onComplete`
      fires, and assert `calls.length === 2`.
   b. Add a new test: immediate poll returns `live:false` + 0 new blocks
      (reuse/extend `pollEmptyFixture`-style live:false variant) — assert
      the loop does NOT stop (`onComplete` not called, interval not
      cleared) after the immediate poll; then a manual `tick()` returning
      real blocks + `live:false` stops the loop and delivers blocks via
      `onUpdate`.
3. `ws-dashboard/crates/core/src/activity.rs` (L153-168) — add
   `pub role: Option<String>` and `pub turn_id: Option<String>` to
   `TranscriptBlock` (after `degraded` or wherever fits `camelCase` rename
   conventions already in place). Update the two test-fixture literals at
   L309 and L448 with `role: None, turn_id: None`.
4. `ws-dashboard/crates/core/src/agent_client_provider.rs#L317` — add
   `role: None, turn_id: None` to the test fixture literal.
5. `ws-dashboard/crates/daemon/src/work_root_activity.rs` — add
   `role: None, turn_id: None` to every `TranscriptBlock { .. }` literal in
   `parse_codex_session_transcript`/`codex_session_record`/
   `unsupported_codex_session_block`/the two helper functions
   (`#L1957`,`#L2017`)/the `.map(...)` at `#L2138` (mechanical, no behavior
   change; this path stays out of scope for actual role/turn_id semantics
   per Out of Scope).
6. `ws-dashboard/crates/core/src/codex_projection.rs`:
   a. Add a `role: Option<&'static str>` (or `Option<String>`) field to
      `BlockState` (L85-90), or derive role purely at each `upsert_block`
      call site — choose whichever keeps `upsert_block`'s signature
      cleanest; recommend adding a `role` parameter to `upsert_block`
      (L419-439) since every call site already knows its role
      (`"user"` for `userMessage`/`hookPrompt` at L302-315, `"agent"` for
      `agentMessage` at L316-318, `"tool"` for `commandExecution`/
      `mcpToolCall`/`dynamicToolCall`/`collabAgentToolCall` at L324-331,
      leave role `None` for `reasoning`/`fileChange`/`plan`/
      `contextCompaction`/unsupported per ticket instruction ("leave
      `thinking` blocks' role unset")).
   b. In `transcript_blocks()` (L192-209), add
      `role: block.role.map(str::to_owned)` (or clone) and
      `turn_id: self.order_turn_ids.get(index)?.clone()` — reuse the
      already-existing `order_turn_ids` parallel array, no new tracking
      state required for `turn_id`.
   c. In `CodexProjector::seeded` (L129-146), copy `block.role` from the
      incoming already-projected `TranscriptBlock` into the new
      `BlockState` (do not discard it, unlike `order_turn_ids.push(None)`
      at L134 which is correct to keep discarding per the existing
      fork-of-a-fork-turn-id-out-of-scope comment).
   d. Add a code comment at the `suppress_local_prompt` call site(s)
      (`codex_app_server.rs:1072,1111`; state at `codex_projection.rs:151,
      292-315`) per the ticket's explicit Constraints instruction, noting
      this is a settled decision and must not be removed as part of "completing"
      this ticket.
7. `ws-dashboard/crates/core/src/claude_projection.rs`:
   a. Add `current_turn_seq: u64` (or similar) to `ClaudeProjector`
      (L109-126) and a `current_turn_id: Option<String>` set/incremented in
      `ingest_assistant` (L218-227) exactly when `turn_started` is `true`
      (before the existing `self.active_turn = true` line), formatted as a
      synthesized id (e.g. `format!("claude-turn-{}", self.current_turn_seq)`).
   b. Add a `role` field to `BlockState` (L95-104) or thread a `role`
      parameter through `push_block` (L387-410) and
      `upsert_tool_block`/`complete_tool_block` (L415-451) analogous to
      Codex: `"agent"` for `text` content blocks (L248-251), role
      unset/`None` for `thinking` (L252-256), `"tool"` for `tool_use`/
      `tool_result` (L258-265, L273-297). Never assign `"user"` (Claude's
      protocol never echoes the client's own prompt — see Codebase
      Findings).
   c. In `transcript_blocks()` (L156-180), add `role`/`turn_id` fields
      analogous to Codex's, reading from `block.role` and the
      per-block-creation-time `current_turn_id` (store `turn_id` in
      `BlockState` at creation time, same pattern as `order_turn_ids` for
      Codex, since Claude has no separate parallel-array precedent to
      reuse).
8. Add/extend unit tests in `codex_projection.rs` and `claude_projection.rs`
   (both files already have inline `#[cfg(test)]` modules, e.g.
   `codex_projection.rs#L814-844`) covering: role assigned per item type,
   `turn_id` grouping multiple blocks from the same turn, and (Codex only)
   `seeded()` preserving `role` across a fork replay.
9. Add a route-level or frontend test proving: (a) a fork-replayed Codex
   user block now carries `role: "user"` end-to-end through the transcript
   route (right-aligned per `chatBlockRole()`), and (b) a multi-block real
   reply (same `turn_id`) merges into one bubble via
   `groupTranscriptIntoBubbles`'s `canMerge`. Locate the existing
   route/transcript-shape test file used for prior phases of this same
   ticket family (`activitySessionClient.test.ts` or a daemon-side
   route test) rather than introducing a new test file.

## Verification Plan

- Automated: run the frontend test suite covering
  `activitySessionClient.test.ts` and the Rust test suites for
  `codex_projection.rs`/`claude_projection.rs` (`cargo test -p
  ws-dashboard-core`).
- Phase 1 manual/live verification: follow
  `ai-docs/ref/dashboard-headless-browser-verification.md` end-to-end —
  start the no-auth daemon, drive a real Codex or Claude tab via the
  documented Playwright selectors, send a real prompt, and confirm the
  reply renders in the browser without needing fork, waiting the harness's
  actual multi-second completion time (not just an immediate check after
  send) per that doc's "Known pitfall" section on wall-clock-ordering.
- Phase 2 manual/live verification: same daemon/browser setup, but exercise
  fork's replay path specifically (per the ticket's own Verification Plan)
  — confirm a replayed user message renders right-aligned and a real
  multi-block reply merges into a single bubble. Note: this verification
  step is Codex-only per the Out of Scope finding (Claude has no fork
  replay path); Claude's `turn_id` correctness should instead be verified
  via a live multi-block Claude turn merging into one bubble, not via fork.

## Escalations

- None.

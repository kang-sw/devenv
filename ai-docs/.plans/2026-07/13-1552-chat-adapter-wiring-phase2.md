# Plan: 260713-feat-ws-dashboard-agent-chat-real-adapter-wiring — Phase 2: Streaming delivery via polling (MVP), not SSE

## Relevant Ticket Contract

- Implement a frontend polling loop against the existing polling-only
  `transcript` GET endpoint, mirroring the interval pattern in
  `gitToolbar.ts:244`; polling is an accepted transport in this codebase
  (`workRootActivity.ts:154`'s `"pollFallback"` mode), not a hack.
- Replace the stub's synthetic `onComplete` ticker with a real
  diff-against-last-seen-length poll, and adapt
  `mergeStreamingTranscriptBlocks`'s delta-merge assumptions to fit
  full-refetch-diffed transcripts rather than true incremental deltas.
- Do **not** build a new SSE/websocket endpoint (`auth.rs:172-180`'s
  `authenticate_websocket_upgrade` seam stays reserved future work).
- Verification: a unit test on the adapted merge/diff logic covering
  unmatched-append-ordering and multi-poll stability, mirroring
  `agentChatStreamMerge.test.ts`'s existing style; manual check that a real
  send against a stub-free session renders incrementally.

## Out of Scope

- Phase 3 (capability control variants / real Codex fork route) and Phase 4
  (E2E/manual walkthrough) — not touched here.
- SSE/websocket transport — explicitly deferred future work.
- OpenCode wiring — stays on `activitySessionStub.ts` (unmodified).
- Any change to `sendAgentChatPrompt`'s single-POST-per-turn contract
  established in Phase 1 (`Result` section: prompt sends once, from
  `sendAgentChatMessage`, never from the streaming handle).

## Codebase Findings

- `ws-dashboard/frontend/src/activitySessionClient.ts#L358-378` —
  `beginRealStreamingTurn` currently does one `fetchRealTranscript` call,
  then immediately fires `onComplete` in `.finally()`. This is the one-shot
  code Phase 1 deliberately left for Phase 2 to replace with a loop; its
  `RealStreamingHandle.stop()` is a documented no-op today.
- `ws-dashboard/crates/daemon/src/codex_app_server.rs#L881-895` and
  `ws-dashboard/crates/daemon/src/claude_cli.rs#L882-896` — both
  `codex_activity_transcript`/`claude_activity_transcript` set
  `live: projector.is_turn_active()` on the returned `ActivityTranscript`.
  This is the authoritative turn-completion signal: poll until
  `transcript.live === false`, then stop and fire `onComplete`. No other
  status field on the transcript response means "turn finished."
- `ws-dashboard/frontend/src/workRootActivity.ts#L109-118` — `ActivityTranscript`
  shape confirms `live: boolean` and `blocks: TranscriptBlock[]` (each block
  has a stable `cursor`); no incremental/delta endpoint exists, only
  full-refetch.
- `ws-dashboard/frontend/src/agentChatStreamMerge.ts#L21-30` —
  `mergeStreamingTranscriptBlocks` already keys by `cursor` and
  replaces-wholesale on match, so passing progressively-growing blocks
  across polls already merges correctly; the ticket's "adapt delta-merge
  assumptions" requirement is about *what subset* of the full refetch gets
  handed to `onUpdate` each poll (a last-seen-length diff), not about this
  function's own replace/append logic, which does not need to change.
- `ws-dashboard/frontend/src/App.tsx#L7224-7241` — call site: `onUpdate`
  merges the array it's given into `streamingBlocks` keyed by cursor;
  `onTurnComplete` (passed as Phase 2's `onComplete`) clears `turnInFlight`
  and dequeues the FIFO queue. This call site does not need to change size
  of the payload it can accept — it already merges by cursor regardless of
  whether it receives the full block list or a tail slice.
- `ws-dashboard/frontend/src/gitToolbar.ts#L48-61,233-251` and
  `gitToolbar.test.ts#L200-230` — the injectable
  `GitRefreshSchedulerEnvironment` (`setInterval`/`clearInterval` passed in,
  not called globally) is the concrete pattern to mirror both for the
  ticket's "mirror `gitToolbar.ts:244`" instruction *and* for testability.
- **Risk signal (test infra)**: `ws-dashboard/frontend/package.json` scripts
  `test:agent-chat-client` / `test:agent-chat-stream-merge` run compiled
  plain-Node scripts (`tsc -p tsconfig.route-tests.json && node ...`), not
  vitest/jest — there is no fake-timer harness anywhere in this test suite.
  A `window.setInterval`-only implementation (like `terminalOutputPollIntervalMs`
  at `App.tsx:406,4696`) would force real-time waits in the new unit test or
  be untestable without a live daemon. Mirroring `gitToolbar`'s injectable
  timer functions (not just its *interval concept*) avoids this: tests can
  supply a manual-tick fake exactly like `gitToolbar.test.ts:218-225` does.
- `ws-dashboard/frontend/src/activitySessionClient.test.ts#L322-338` (plus
  `transcriptFixture` around `#L75-80`, which already includes a `live`
  field) — existing one-shot `beginRealStreamingTurn` test to rewrite for
  polling; the fixture's pre-existing `live: true` field means adding a
  second `live: false` fixture for the "poll completes" case is a small,
  additive change to the existing mock-fetch/`nextResponses` queue idiom
  already used in this file, not a new test infra.

## Implementation Plan

1. `ws-dashboard/frontend/src/agentChatStreamMerge.ts`: add a new exported
   pure helper (e.g. `blocksSincePolledLength(blocks, lastSeenLength)`)
   that, given the full block array from a poll and the previously-seen
   block count, returns blocks from `max(0, lastSeenLength - 1)` onward —
   re-including the last previously-seen block (it may have grown/mutated
   text since it was the in-progress tail) plus any newly appended blocks.
   Keep it pure/side-effect-free, colocated with `mergeStreamingTranscriptBlocks`
   per this file's existing "pure and directly unit-testable" contract.
2. `ws-dashboard/frontend/src/agentChatStreamMerge.test.ts`: add cases for
   the new helper — multi-poll stability (repeated calls with a stable
   `lastSeenLength` against an unchanged tail block yield the same diff,
   i.e. idempotent), and unmatched-append-ordering (a poll that appends
   more than one new block at once still diffs and then merges correctly
   through `mergeStreamingTranscriptBlocks`), mirroring this file's existing
   assertion style (see current cases around `#L39-136`).
3. `ws-dashboard/frontend/src/activitySessionClient.ts#L339-378`: rewrite
   `beginRealStreamingTurn`:
   - Add an optional environment param (default `{ setInterval: (fn, ms) =>
     globalThis.setInterval(fn, ms), clearInterval: (h) =>
     globalThis.clearInterval(h) }`) mirroring `gitToolbar.ts`'s
     `GitRefreshSchedulerEnvironment` injection, so tests can supply a
     manual-tick fake instead of waiting on real intervals.
   - Track `lastSeenLength` in closure state, starting at 0.
   - `poll()`: call `fetchRealTranscript`; on success, compute
     `blocksSincePolledLength(transcript.blocks, lastSeenLength)`, call
     `onUpdate` with that slice (skip the call if the slice is empty),
     update `lastSeenLength = transcript.blocks.length`; if
     `!transcript.live`, clear the interval and call `onComplete()`.
   - On fetch error: call `onError?.(error)`, clear the interval, call
     `onComplete()` (preserve existing finally-always-fires semantics from
     Phase 1's one-shot version).
   - Fire one immediate `poll()` at call time (don't wait a full interval
     for the first update), then schedule the interval for subsequent
     polls.
   - Choose and document an interval constant (e.g. `realStreamingPollIntervalMs`,
     a module-level `const`); no existing constant to reuse for this exact
     purpose (`terminalOutputPollIntervalMs` is for a different feature).
   - `stop()` becomes real: clears the interval handle.
4. `ws-dashboard/frontend/src/activitySessionClient.test.ts`: rewrite the
   `beginRealStreamingTurn` test block (`#L322-338` onward) to drive the new
   injectable environment — queue two `transcriptFixture` variants in
   `nextResponses` (first `live: true` with partial blocks, second
   `live: false` with the full/final blocks), manually invoke the fake
   `setInterval`'s captured tick function to advance polls, and assert:
   `onUpdate` is called once per poll with only the new/changed tail slice
   (not the full array both times), `onComplete` fires exactly once after
   the `live: false` poll, and `stop()` (called externally, e.g. simulating
   unmount) clears the interval and stops further polling. Add/keep the
   existing `onError` coverage, adapted to the new poll-loop shape (error on
   first poll still calls `onError` then `onComplete`).
5. No changes needed at the `App.tsx` call site (`#L7224-7241`) — it already
   merges whatever block slice it receives by cursor; confirm this by
   re-running existing chat-tab/bubble tests, not by editing this file.

## Verification Plan

- `npm run test:agent-chat-stream-merge` (new helper's unit coverage).
- `npm run test:agent-chat-client` (rewritten `beginRealStreamingTurn` poll
  test).
- `npm run build` (tsc -b + vite build) to catch any type drift from the
  `beginRealStreamingTurn` signature change (new optional env param).
- Re-run `test:agent-chat-tabs`, `test:agent-chat-bubbles` to confirm no
  regression at the unchanged `App.tsx` call site.
- Manual: with a real Codex or Claude daemon session (no stub), send a
  message and confirm the transcript bubble grows incrementally across
  polls rather than only updating once at full completion.

## Escalations

- None.

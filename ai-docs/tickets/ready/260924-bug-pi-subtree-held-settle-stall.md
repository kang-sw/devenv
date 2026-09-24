---
title: Pi parent holds a settled child's terminal after its subtree wait clears
related:
  260924-feat-pi-agent-channel-subtree-state: origin; its Result records these residuals and the current release rule
  260913-bug-ws-pi-settled-agent-falsely-remains-running: constrains; lifecycle-only fences around settlement stay intact
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c909082454839a3b
sage-review-completeness-reviewed: c909082454839a3b
---

# Pi parent holds a settled child's terminal after its subtree wait clears

## Background

`260924-feat-pi-agent-channel-subtree-state` moved subtree state onto the parent-child control channel. When a direct child settles while its subtree view reads waiting, the parent holds that settle. Before admitting the terminal it waits for one of two things: the wake turn's own settle, or `record.releaseSettlementHold` (`agents-plugin-pi/src/spawner.ts`, `attachEventListener`).

`observeChildSubtree` (`spawner.ts`) releases the hold only in two cases:

- the wait clears at the same quiescent revision it held before;
- the wait clears on the launch's first snapshot.

It never releases on a busy-to-quiescent clear, on the assumption that the delivery which ended the wait wakes the child. That rule guesses from revisions whether a wake turn is coming, and the guess is wrong in these cases:

- **A clear that wakes nothing.** One example is a grandchild stopped with no delivery. If its quiescent snapshot arrives after `agent_settled` (socket and stdout race), the settle stays held.
- **A disconnect window that covers an identity-only revision change.** The reconnect snapshot is quiescent at a higher revision, so the equality test fails.
- **A disconnect window that covers a whole wake turn.** The wake turn's own settle was held while the view read waiting, and the reconnect snapshot again fails the equality test.

In each case the parent's lead never receives the child's `ws-agent-settled` until another child turn or process exit. The failure is fail-closed, so no wrong answer is reported, but to the lead it looks like a hang.

The origin ticket also left one verification item uncovered. An older `PersistedForkResume` record that still carries `subtreeChannel` is tested for loading but not for relaunching.

## Decisions

- **Wake accounting comes from the child.** The parent stops inferring from revisions whether a wake turn is coming; the child reports it. The subtree snapshot gains two child-authoritative facts:
  - **owed turn:** whether a turn is owed to deliveries the child has already enqueued and that have not yet started a turn;
  - **turn count:** a count of turns the child has started in this launch, rising monotonically within one launch generation.
  - The worker chooses field names and how the child derives both facts from its own events. The facts must be exact: a clear that enqueues a waking delivery reports a turn owed in the same snapshot, or in an earlier one.
- **Release rule.** A held settle is released when all three hold:
  - the view is not waiting;
  - the latest snapshot reports no owed turn;
  - the parent has observed, through the RPC event stream (`agent_start`), at least as many child turns as that snapshot reports.
  - The revision-equality and first-snapshot rules are retired. `releaseSettlementHold`'s existing guards stay: current launch, held generation equal to the work generation, not running, not streaming, not waiting.
- **Why this is complete.**
  - When the socket runs ahead of stdout (a wake owed or already started), the hold waits until stdout catches up. The wake turn's own settle then admits.
  - A clear that wakes nothing, and a reconnect after an identity-only change or after a whole wake turn, all satisfy the rule at once.
  - A missing snapshot still reads waiting, so the view stays fail-closed.
- **Send volume.**
  - The turn count changes once per child turn, which adds at most one snapshot send per turn boundary.
  - Streamed deltas and duplicate events still produce no sends.
  - The busy-before-dispatch fence, revision ordering, generation reset, hello resume, and bounded `descendants[]` from the origin ticket are unchanged.
- **Implementation constraints** (from review):
  - **Count ordering.** The child increments its turn count before `registerPushFlush`'s `agent_start` handler publishes the quiescent snapshot (`agents-plugin-pi/src/index.ts`, `spawner.ts`). A snapshot that carries the old count with no owed turn would be released early. The increment rides the snapshot that the flush already sends, so one turn start sends at most one snapshot.
  - **Deriving "owed".** An owed turn is derived from the child's own state, for example a pending wake reservation (`leadWakeStartPendingRef`) or a boundary batch submitted but not yet started, such as the `agent_end` followUp flush that `agent.continue()` picks up.
    - Pi's `hasPendingMessages` does not track custom messages, so it is not a source.
    - Steers consumed inside a running loop, and owner-routed pushes, never set "owed".
    - Flipping "owed" rides an existing snapshot send and never adds its own. The known flips already do: a wake reservation publishes from `admitPush`, and the `agent_end` flush publishes through `afterEnqueue`. If a flip is found that happens after the last sent snapshot was quiescent with nothing owed, exactness wins and that flip is sent. The send-volume rule never outranks the exactness rule.
  - **Counter scope.** Both counters are scoped to one launch. The parent's observed `agent_start` count lives in the per-client `attachEventListener` closure and resets on relaunch, as the child's per-process count does. A socket reconnect within a launch keeps both.
  - **Strict parse.** The new fields are required. A snapshot missing them is rejected, stays unacknowledged, and the view keeps reading waiting (fail closed, as the origin ticket does). The worker records whether it also bumps `CHANNEL_PROTOCOL_VERSION`. Mixed versions arise only when the plugin is updated on disk while a parent runs.
- **Rejected alternative.** Keying the release on "an `agent_start` was seen since the last busy snapshot", which the origin Result proposed. It leaves a clear that wakes nothing stalled, because no `agent_start` ever follows.
- **Rejected alternative.** A timeout release. It would report the previous generation's answer ahead of a slow wake turn.

## Prior Decisions

- 990a9ca9 (2026-09-24, commit): "The release is scoped rather than blanket: when a grandchild's terminal is enqueued into the child, the child's snapshot turns quiescent before the wake turn starts, so a blanket replay would report the previous generation's answer early" — bearing: constrains
- 260924-feat-pi-agent-channel-subtree-state (2026-09-24, Result (990a9ca)): "Scoped release of a held settlement ... released through `record.releaseSettlementHold` only when the wait clears at the same quiescent revision held before, or on the launch's first snapshot." — bearing: supports
- 990a9ca9 (2026-09-24, commit): "Resending on onReconnect is safe because the parent treats an equal revision as a duplicate (applied idempotently and acknowledged)." — bearing: constrains
- 47b274ef (2026-09-24, commit): "The initial view is waiting until the first snapshot or resume hello arrives: a not-yet-connected child is never permission to settle (fail closed)." — bearing: constrains
- 47b274ef (2026-09-24, commit): "Observation is registered at channel bind time ... because ParentChannel drops feature messages with no listener; late callbacks from a replaced launch are gated on record.channel identity" — bearing: constrains
- 260913-bug-ws-pi-settled-agent-falsely-remains-running (2026-09-13, Decisions): "Preserve lifecycle-only fences around settlement: work generation, descendant waiting and subtree revision, direct-parent routing, terminal queue admission and retry, and `lastWriter` / `threadBound` ownership." — bearing: constrains
- 260912-feat-ws-pi-recursive-worker-subtree-lifecycle (2026-09-12, Decisions): "Local Pi settle is not semantic completion. Pi's raw `agent_settled` cannot be cancelled. Track separate own-turn, expected-descendant-report, and aggregate subtree states." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts observeChildSubtree and attachEventListener; agents-plugin-pi/src/subtree-lifecycle.ts SubtreeSnapshot, parseSubtreeSnapshot, SubtreeUpstream, publishSubtree carry the snapshot shape; test files below |
| scope.surface | cross-module | the child-to-parent subtree snapshot protocol in subtree-lifecycle.ts is consumed by spawner.ts observeChildSubtree agents-plugin-pi/src/spawner.ts#L2549-L2574 |
| scope.new_public_symbol | unknown | the ticket leaves field names and derivation to the worker; whether new helpers are exported is not settled |
| scope.new_type_contract | yes | exported SubtreeSnapshot in agents-plugin-pi/src/subtree-lifecycle.ts gains owed-turn and turn-count fields validated by parseSubtreeSnapshot |
| scope.test_surface | existing | agents-plugin-pi/test/recursive-worker.test.ts, test/subtree-lifecycle.test.ts, test/spawner.test.ts, test/ask.test.ts which currently covers only loading of a legacy subtreeChannel resume |
| complexity.reuse_points | confirmed | observeSubtreeChannel revision ordering and ack, SubtreeUpstream.publish dedup, and releaseSettlementHold guards agents-plugin-pi/src/spawner.ts#L2724-L2729 were read |
| complexity.side_effect_risk | high | replaces the terminal-admission release rule on the settlement path, where a wrong release reports a previous generation's answer or a duplicate ws-agent-settled |
| risk.correctness | high | release depends on ordering between the channel socket and RPC stdout agent_start across reconnects and launch generations |
| risk.fit | moderate | must leave the busy-before-dispatch fence, revision ordering, generation reset, hello resume, and send volume from the origin ticket unchanged |
| risk.test | high | the verification list needs deterministic socket-ahead, stdout-ahead, and disconnect-window orderings plus a legacy relaunch path not covered today |
| risk.security_or_contract | moderate | changes the parent-child snapshot message shape; a snapshot parseSubtreeSnapshot rejects is dropped unacknowledged and the view keeps its prior state, initially waiting agents-plugin-pi/src/subtree-lifecycle.ts#L189-L213 |

## Phases

### Phase 1: Child-authoritative wake accounting for held-settle release

Add the two facts to the child's subtree snapshot, parse them on the parent, and replace the release rule in `observeChildSubtree`. Add relaunch coverage for an older `PersistedForkResume` record.

Verification:

- **Clear that wakes nothing:** the grandchild is stopped with no delivery, and the quiescent snapshot arrives after the child's `agent_settled`. The terminal is admitted with no further child turn.
- **Clear that wakes the child, socket ahead of stdout:** the quiescent snapshot, with its turn owed, arrives before the wake turn's `agent_start`. The previous generation's terminal is not admitted early, and the wake turn's own settle admits exactly once.
- **Clear that wakes the child, stdout ahead of socket:** same outcome as the socket-ahead case.
- **Snapshot reporting no owed turn while the turn count runs ahead:** the child's `agent_start` has not yet reached the parent. Nothing is admitted until stdout catches up.
- **Disconnect window covering an identity-only revision change:** after the reconnect, the held settle is released.
- **Disconnect window covering a whole wake turn:** after the reconnect, the wake turn's terminal is admitted exactly once and the earlier generation's is not reported.
- **Disconnect with work still outstanding:** the parent still reads waiting and holds. The origin ticket's fail-closed behaviour is preserved.
- **Send volume:** unchanged state and streamed deltas send nothing. A child turn start sends at most one snapshot, and flipping the owed turn adds no send of its own.
- **Strict parse:** a snapshot missing the new fields is rejected and the view keeps reading waiting.
- **Legacy relaunch:** an older `PersistedForkResume` record carrying `subtreeChannel` loads, relaunches with a fresh channel, and reports subtree state over it.
- **Full suite:** `npm test` in `agents-plugin-pi/` passes.

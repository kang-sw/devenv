---
title: Fix the pi live-gutter two-clock semantics (activity high-water mark and run-time freeze)
related:
  260921-bug-pi-gutter-stale-pending-approval-deadlock: sibling gutter fix, disjoint scope (non-goal cross-reference)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: cf084bb561053a50
sage-review-completeness-reviewed: cf084bb561053a50
completed: 2026-09-21
---

# Fix the pi live-gutter two-clock semantics (activity high-water mark and run-time freeze)

## Background

Each row in the pi live-agent gutter (`agents-plugin-pi/src/agent-widget.ts`)
prints two time values that are meant to be distinct — a "total run time" and a
"most recent activity time". Rendered together in `formatRow` as
`name · role · state · <elapsedMs> (<lastActivityMs>)` (`agent-widget.ts`),
backed by `AgentRow.elapsedMs` and `AgentRow.lastActivityMs`. Both are currently
wrong relative to intended semantics.

**Most recent activity (`lastActivityMs`).** Computed as `now -
lastActivityAt(record)`, where `lastActivityAt` (`spawner.ts`) is
`max(record.lastLeadPromptAt, last reportLog entry, last ownerSends entry)` —
only three event kinds feed it. `applyRpcEvent` (`spawner.ts`) sees every RPC
event but mutates on only a handful of cases; ordinary tool calls/results,
streamed text/thinking chunks, and approval requests fall through with no
mutation. So a session busy with tool calls between reports shows a large, stale
"last activity" even though it is plainly alive.

**Total run time (`elapsedMs`).** Computed as `now - record.runStartedAt`
(non-thread branch). `runStartedAt` is stamped in `promptAgent` (`spawner.ts`)
on *every* prompt dispatch — spawn **and** every nudge/re-prompt — and is never
reset on settle. A doc comment (the 260905 live-agent-widget ticket) states this
nudge-reset is deliberate: "the nudge really did start a fresh turn on the wire".
This ticket reverses that choice.

## Decisions

### 1a — "most recent activity" = agent-output high-water mark
- Redefine `lastActivityMs` to track the timestamp of the **latest agent-output
  event of any kind**: any sign the LLM is producing output (a tool call, or
  streamed assistant text / "thinking"). This is the "is it alive" signal.
- **Exclude inputs and non-output triggers.** Do NOT bump on lead prompt/nudge
  (`lastLeadPromptAt`) or owner sends (`ownerSends`) — those are input *to* the
  agent, not agent output. Do NOT bump on an agent's own self-scheduled timer
  firing or other mechanical non-output triggers. Confirmed with the user:
  input events are excluded; the clock is a pure "agent is producing" mark.
- **Single elegant path / audit consistency.** Make this one stored high-water
  field that both the gutter and the `/audit` picker (`audit.ts`, which reuses
  the same field) read, rather than forking a second field. The user explicitly
  wants audit kept consistent with the gutter.
- Rejected alternative — a separate new `lastEventAt` field leaving
  `lastActivityAt`'s 3-source `max()` in place: rejected because it desynchronizes
  the gutter from the audit picker; the user wants a single path.

### 1b — "total run time" = run duration, frozen at settle (option B)
- Anchor set at spawn; **not** reset on nudge/re-prompt (reverses the 260905
  nudge-reset). The value accumulates across nudges *within a single run*.
- **Freeze at settle:** while a settled row still lingers in the gutter
  (waiting-on-children / pending-delivery / awaiting-approval), it displays the
  run's final total duration, not a value that keeps growing. This requires
  storing a settle timestamp — the `agent_settled` branch (`spawner.ts`)
  currently records no time. Effective formula becomes roughly
  `elapsedMs = (settledAt ?? now) - runStartedAt`.
- **Re-arm only on a new run:** a re-prompt *after* a settle starts a fresh run —
  reset the anchor to now and clear the frozen settle timestamp. A nudge to an
  already-running (not-yet-settled) agent does not reset. So `promptAgent`'s
  currently-unconditional `runStartedAt` reset must become boundary-aware
  (reset only when the previous state was settled).
- "누적" here means accumulation *across nudges within a run*, not summation
  across separate runs — each new run counts from zero. Confirmed with the user
  that option B (frozen run duration) is preferred over option A (time-since-settle).

## Constraints

- **Streaming throttle — do not trip the per-token mine.** Agent output arrives
  as a stream of many `text_delta`/token chunks. Bumping the stored
  `lastActivityAt` field on each is a cheap assignment and is fine, but it must
  **not** trigger a gutter re-render (`triggerAgentWidgetRefresh` or equivalent)
  per chunk. Keep the per-chunk path O(1) with no render fan-out; let the
  existing periodic render cadence pick up the new value, or coalesce/throttle
  any refresh. The value need not be per-token precise.
- **Blast radius: `/audit`.** Both clocks' backing fields are also consumed by
  the audit picker (`audit.ts`). By design (1a) the activity change is meant to
  propagate there; make sure the run-time (1b) change does not break the audit
  picker's use of the run anchor.
- **Thread-bound branch.** The `isAwaitingOwnerWithThread` branch computes
  `elapsedMs` from `boundThread.touchedAt`, a different anchor. This ticket does
  not change that branch's semantics; leave it as-is unless the freeze logic
  demonstrably must reach it (call it out if so rather than silently changing it).
- pi harness surface (`agents-plugin-pi/`). Confirmed: none of the AGENTS.md
  Implementation Conventions rows match `agents-plugin-pi/` paths (they target
  `agents-plugin/`, `agents-plugin-wsflow/`, and `agents-plugin-tool/`), so no
  path-scoped manual is required for this change.

## Non-goals

- The `ws-execute` awaiting-approval deadlock (stale `pendingApproval` never
  cleared on process death) is a separate bug ticket
  (`260921-bug-pi-gutter-stale-pending-approval-deadlock`). Not addressed here.

## Prior Decisions

- 260905-feat-ws-pi-live-agent-widget (2026-09-05, commit 07d24cdc): "spawner.ts: runStartedAt stamped unconditionally in promptAgent; agentWidgetRefreshRef ... fires on spawn, spawn failure, exit, stop, agent_start, settle and auto-park" — bearing: constrains
- 260913-bug-ws-pi-settled-agent-falsely-remains-running (2026-09-13, commit 6bafc76f): "Latch terminal admission per work generation so repeated settlement and stale harvests cannot duplicate or overwrite a successor turn." — bearing: constrains
- 260908-feat-ws-pi-subagent-audit-window-and-owner-steering (2026-09-13, commit 859a70af): "Use one lastWriter field as the runtime ownership source to avoid drift between ownership metadata and lifecycle decisions." — bearing: constrains
- 260921-bug-pi-gutter-stale-pending-approval-deadlock (2026-09-21, ticket Decisions): "Clear record.pendingApproval = undefined in clearLiveState. It is the single chokepoint that both markAgentExited ... and stopAgent ... route through" — bearing: constrains
- 260916-feat-pi-agent-gutter-active-time-placement (2026-09-16, ticket Result): "Relocated the existing active-duration value beside total elapsed time as 1m (25s), preserving syntaxNumber styling and all time-accounting semantics." — bearing: supports
- 260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets (2026-09-14, ticket Decisions): "Live panel only. These changes apply to the live agent panel only, not the /audit picker. Picker treatment rides the split nested-gutter ticket." — bearing: supports
- 260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter (2026-09-20, commit ab8b02f6): "Render one dim gutter lane per depth and omit clocks and telemetry for propagated rows because the Phase-1 transport does not carry those facts" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-widget.ts, agents-plugin-pi/src/spawner.ts |
| scope.surface | cross-module | exported RpcAgentRecord fields and lastActivityAt (spawner.ts#L2630-L2634) consumed by agent-widget.ts and audit.ts |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | yes | new RpcAgentRecord fields for an output high-water mark and a settle timestamp |
| scope.test_surface | existing | agents-plugin-pi/test/spawner.test.ts, agents-plugin-pi/test/agent-widget.test.ts |
| complexity.reuse_points | confirmed | classifyRegistryRowState/buildAgentTree/lastActivityAt extraction pattern (agent-widget.ts, spawner.ts) |
| complexity.side_effect_risk | moderate | touches registry fields read by the gutter, /audit, and ownership sync across every RPC event |
| risk.correctness | moderate | boundary-aware reset (settled-vs-running) and the output-event taxonomy must not misclassify a case |
| risk.fit | low | reuses the 260908 single-stored-field/shared-consumer pattern already established for lastActivityAt |
| risk.test | moderate | existing pure-function suites cover the seam but the freeze/re-arm and output-taxonomy cases are unwritten |
| risk.security_or_contract | low | display-only gutter/audit surface, no external API or security boundary |

## Phases

### Phase 1: Activity clock = agent-output high-water mark

Goal: `lastActivityMs` reflects the latest agent-output event (tool call or
streamed text/thinking), excluding input events (lead prompt, owner send) and
self-scheduled/non-output triggers; the `/audit` picker reads the same clock.

Work:
- Wire the high-water bump at **both** observation sites — the output signal is
  split across two layers, so `applyRpcEvent` alone is not enough:
  - `tool_execution_start` / `tool_execution_end` reach `applyRpcEvent`
    (`spawner.ts`); bump there.
  - streamed assistant text / "thinking" is observed one layer up in
    `attachEventListener` (`spawner.ts`, e.g. `message_end` setting
    `record.lastText`, `message_update`), NOT in `applyRpcEvent`; bump there too,
    or the pure-streaming case (long text/thinking with no tool calls) silently
    reproduces the exact stale-activity symptom this ticket fixes.
  Classify each event kind as agent-output (bump) vs non-output (skip) and record
  the classification in the code near the mutation site so a future reader can
  re-derive it. Note: the excluded input events (lead prompt, owner send,
  self-scheduled goal-loop nudges) are not RPC events at all — they route through
  `promptAgent` and are the three `lastActivityAt` sources being removed, so an
  output-only high-water field naturally excludes them.
- Replace the 3-source `lastActivityAt` `max()` with the single stored high-water
  field, updated on qualifying output events, read by both the gutter and
  `audit.ts`.
- Honor the streaming-throttle constraint above: no per-chunk render fan-out.

Verification:
- A record that has only emitted ordinary tool calls / streamed output since its
  last report shows a fresh (small) `lastActivityMs`.
- A record that received only a lead prompt / owner send (no agent output yet)
  does NOT get a bumped activity mark from those inputs.
- `/audit` picker reflects the same activity value.
- Streaming a long output does not cause a re-render per token.
- Pi package test suite passes.

### Result (08ac495) - 2026-09-21

- Added the shared `lastOutputAt` high-water mark, updated by tool execution
  and assistant text/thinking output only; gutter, audit, and capacity ordering
  now read that one field, excluding lead/owner input and report history.
- Stream deltas update only that field and defer subtree observation,
  publication, and telemetry refresh, preventing local and parent-watcher
  gutter fan-out per token.
- Verification: `npm test -- test/agent-widget.test.ts test/audit.test.ts`
  passed (102 tests); focused spawner clock/eviction tests passed (7 tests).
  The full `npm test` run had 1,603 passing tests but 14 unrelated failures
  caused by the missing `pi-web-access` search extension; no affected test
  failed. Independent correctness, fit, and test review completed; both
  round-one streaming findings were corrected within the two-round cap.

### Phase 2: Run-time clock frozen at settle, re-armed per run

Goal: `elapsedMs` accumulates across nudges within a run, freezes at settle to
show the run's total duration while the row lingers, and re-arms to zero only
when a new run starts after a settle.

Work:
- Store a settle timestamp in the `agent_settled` handling (`spawner.ts`) and use
  it to freeze `elapsedMs` (`(settledAt ?? now) - runStartedAt`).
- Make `promptAgent`'s `runStartedAt` reset boundary-aware: reset (and clear the
  settle timestamp) only when starting a fresh run after a settle; a nudge to a
  still-running agent does not reset. This reverses the 260905 nudge-reset.

Verification:
- Nudging a running agent does not reset its run-time display.
- A settled-but-lingering row shows a stable (frozen) run duration.
- A re-prompt after settle restarts the run-time from zero.
- The `/audit` picker's run-time display is not regressed by the anchor/freeze
  change (explicit check for the `## Constraints` audit blast-radius item).
- Pi package test suite passes.

Depends on Phase 1 only for shared-file coordination (both edit
`agent-widget.ts` and `spawner.ts`), not behaviorally; order is convenience.

### Result (08ac495) - 2026-09-21

- Added `settledAt` and froze non-thread run duration at the first settle;
  duplicate settle events cannot extend it. Re-prompting a resting record
  starts a new anchor, while active nudges retain the existing run anchor.
- Preserved the `isAwaitingOwnerWithThread` duration branch unchanged and
  applied the frozen duration consistently in the audit picker.
- Verification: focused spawner run-boundary tests and the complete widget/
  audit suites passed; see Phase 1 for the full-suite environment limitation
  and review evidence.


## Resolution (2026-09-21)

Implemented the output-only activity high-water mark and frozen per-run duration semantics, with gutter/audit coverage and two-round independent review. The full Pi suite remains environment-blocked by the missing pi-web-access search extension; affected suites pass.

---
title: Retire stale pendingApproval so an exited/stopped gated agent stops deadlocking the pi live gutter
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a25efecf8203c66d
sage-review-completeness-reviewed: a25efecf8203c66d
---

# Retire stale pendingApproval so an exited/stopped gated agent stops deadlocking the pi live gutter

## Background

In the pi live-agent gutter (`agents-plugin-pi/src/agent-widget.ts`), a
`ws-execute` / gated agent whose process dies or is stopped **while awaiting
approval** stays rendered forever in the `awaiting-approval` state, and is also
permanently pinned against registry capacity eviction. It is a deadlock: no
timeout or sweep ever retires it.

The gutter admits no explicit "retire" mutation — a record simply stops
rendering once `classifyRegistryRowState` returns `undefined` for it. But an
un-cleared `pendingApproval` keeps that classifier pinned to `awaiting-approval`
regardless of process liveness, so the record is rendered indefinitely and is
never garbage-collected.

## Root cause

Verify each claim against the current tree before implementing; line numbers are
approximate anchors, not edit targets.

- `pendingApproval` is cleared in exactly one place in the whole codebase:
  `applyRpcEvent`'s `agent_settled` RPC-event branch (`spawner.ts`, the
  `record.pendingApproval = undefined` line). Confirm with
  `grep -rn "pendingApproval = undefined" agents-plugin-pi/src/`.
- The process-death / stop paths never clear it: `clearLiveState`,
  `markAgentExited`, and `stopAgent` (all in `spawner.ts`) leave `pendingApproval`
  set. `stopAgent` explicitly clears the analogous `threadBound` flag but not
  `pendingApproval`.
- `classifyRegistryRowState` (`agent-widget.ts`) returns `"awaiting-approval"`
  whenever `record.pendingApproval !== undefined`, checked ahead of and
  independent of `client` / `running` / `streaming`. A dead-but-still-pending
  record is therefore indistinguishable from a live approval wait and renders
  forever.
- `evictForCapacity` (`spawner.ts`) skips any record whose `pendingApproval` is
  truthy, so the stuck record is never evicted from the registry either.
- Not `ws-execute`-specific: gated exec (`ws-worker-exec` /
  `GATED_EXEC_TOOL_NAME` in `execute-gateway.ts`) blocks on a filesystem
  decision-file poll and holds `pendingApproval` the longest (the human-decision
  window), so it is the most common victim. `ws-approve` (`execute-gateway.ts`)
  sets `decisionWritten: true` on the pending object but keeps it truthy; it
  relies entirely on the worker resuming to `agent_settled` to clear it — which
  never happens if the process died first.

## Decisions

- **Clear `record.pendingApproval = undefined` in `clearLiveState`.** It is the
  single chokepoint that both `markAgentExited` (exit / liveness-probe path) and
  `stopAgent` (explicit stop path) route through, so one edit covers both death
  paths. Rejected alternative — adding the clear separately in `markAgentExited`
  and `stopAgent`: duplicates the statement and risks a future third exit path
  missing it.
- **Do not add a timeout / expiry sweep for pending approvals.** The deadlock is
  fully resolved by reconciling `pendingApproval` against process death at the
  existing chokepoint; a periodic expiry sweep is a larger, separate concern.
  Out of scope here (may be a separate `idea/` if ever wanted).

## Constraints

- Safety of the fix rests on this invariant: a **live** approval wait keeps the
  process alive with `client` / `running` set and does **not** route through
  `clearLiveState`, so the legitimate live-wait case is unaffected. The existing
  test in `agents-plugin-pi/test/agent-widget.test.ts` ("a pendingApproval
  record is included and ranked awaiting-approval even without a live client")
  documents the legit live case and must still pass.
- Accepted caveat: a transient liveness-probe false-negative could drop a
  genuinely-pending approval when it wrongly marks a live agent as exited. This
  is the same pre-existing risk already accepted for `running` / `streaming` /
  `waitingOnChildren`, which `clearLiveState` already clears — no new risk class
  is introduced.
- pi harness surface (`agents-plugin-pi/`). Confirmed: none of the AGENTS.md
  Implementation Conventions rows match `agents-plugin-pi/` paths (they target
  `agents-plugin/`, `agents-plugin-wsflow/`, and `agents-plugin-tool/`), so no
  path-scoped manual is required for this change.

## Non-goals

- The two gutter-timer changes under separate discussion (the last-activity
  clock made an any-output high-water mark, and the total-run-time freeze on
  settle) are a different ticket. Keep them out of this one.
- No timeout / expiry sweep (see Decisions).

## Prior Decisions

- 260905-bug-ws-pi-approval-relay-deadlocks-under-agent-wait (2026-09-05, commit): "applyRpcEvent sets record.pendingApproval without the settleWaiters call its idlePending/report siblings make" — bearing: constrains
- 260908-feat-ws-pi-agent-session-disk-retention (2026-09-08, Decisions): "Cap eviction removes the evicted child's owned disk material as well as its registry entry. Remove no-session scratch homes after their last consumer ends, including pending approval consumers." — bearing: constrains
- 260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter (2026-09-20, commit): "Clearing a direct child's live state now also removes and republishes its cached descendants so finished nested agents cannot remain visibly live." — bearing: constrains
- 260905-feat-ws-pi-push-only-child-reports (2026-09-05, commit): "Approval alone is pushed as steer; everything else is followUp. An approval is the one signal where the child cannot progress at all until the lead answers." — bearing: supports
- 260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets (2026-09-14, Decisions): "Prefix each subagent row with a bullet keyed to its real AgentRowState, produced by classifyRegistryRowState. Those six values are the only reachable states" — bearing: constrains
- 260908-feat-ws-pi-attention-alert-when-agents-wait-on-owner (2026-09-08, Decisions): "Approval cue: toggle the existing awaiting approval label, with no fabricated /answer target." — bearing: supports
- 260920-feat-pi-agent-spawn-cwd-override (2026-09-21, ticket Result): "The optional cwd_override is validated before allocation and persisted on the agent record so dormant resume retains the selected directory." — bearing: supports
- 260905-feat-ws-pi-live-agent-widget (2026-09-05, Decisions): "One row per child; a thread collapses onto its respondent. Every open thread's respondent is also an RPC record, so buildAgentRows dedupes by agent id." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/spawner.ts (clearLiveState#L1789-L1800), agents-plugin-pi/test/spawner.test.ts (evictForCapacity describe#L2317), agents-plugin-pi/test/agent-widget.test.ts (existing live-pendingApproval test#L127) |
| scope.surface | internal | clearLiveState is unexported (spawner.ts#L1789); classifyRegistryRowState is exported (agent-widget.ts#L179) but its signature and logic are unchanged |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-pi/test/spawner.test.ts and agents-plugin-pi/test/agent-widget.test.ts already exist and already cover clearLiveState/evictForCapacity/classifyRegistryRowState |
| complexity.reuse_points | confirmed | clearLiveState (spawner.ts#L1789-L1800) is the existing chokepoint already clearing running/streaming/waitingOnChildren, confirmed as the single route markAgentExited#L1843 and stopAgent#L3402 both take |
| complexity.side_effect_risk | low | one added assignment in an existing shared chokepoint; the ticket's own Constraints section documents the sole accepted caveat (liveness-probe false-negative) as the same pre-existing risk class already accepted for running/streaming/waitingOnChildren, not a new one |
| risk.correctness | low | one-line addition to a function already covered by spawner.test.ts and agent-widget.test.ts; the live-pendingApproval invariant test (agent-widget.test.ts#L127) plus a new death-path test bound the change |
| risk.fit | low | matches the Decisions section's stated single-chokepoint rationale and the function's existing pattern of clearing running/streaming/waitingOnChildren |
| risk.test | low | spawner.test.ts and agent-widget.test.ts already exercise clearLiveState, evictForCapacity, and classifyRegistryRowState, so the new test slots into established suites |
| risk.security_or_contract | low | internal registry bookkeeping only; no exported contract, security boundary, or persisted schema is touched |

## Phases

### Phase 1: Clear pendingApproval on agent death/stop

Goal: a record whose process exits or is stopped while `pendingApproval` is set
no longer classifies as `awaiting-approval` and becomes eligible for capacity
eviction.

Work:
- Add `record.pendingApproval = undefined` in `clearLiveState` (`spawner.ts`),
  alongside the existing `running` / `streaming` / `waitingOnChildren` clears.
- Confirm `markAgentExited` and `stopAgent` both reach it (they route through
  `clearLiveState`); no separate edits needed there.

Verification:
- Add a test: an exited/stopped record that had `pendingApproval` set is not
  classified `awaiting-approval` by `classifyRegistryRowState` afterward and is
  eligible for `evictForCapacity`.
- The existing live-`pendingApproval` widget test still passes (live wait is
  unaffected).
- Run the pi package test suite.

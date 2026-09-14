---
title: "Approval-relay steer deadlocks when the spawning lead is blocked in ws-agent-wait"
related:
  - 260904-feat-ws-pi-execute-approval-gateway
  - 260904-feat-ws-pi-side-thread-fork-question-surface
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d1c4be09dac53060
sage-review-completeness-reviewed: d1c4be09dac53060
completed: 2026-09-05
---

# Bug: approval-relay steer deadlocks under a blocking ws-agent-wait

## Symptom (live, 2026-09-05)

Observed in an interactive `pi` session (pi 0.84.4, openai-codex provider,
user-scope installed adapter) while running the fork+execute smoke test. The
lead called `ws-fork` on a task that made the fork run a shell command (the task
asked for "the current UTC time", which the fork carried out with a bash call),
then the lead called `ws-agent-wait` to harvest the fork's report. The fork hit
its approval gate; the approval-request `steer` arrived at the lead but showed
as a **queued `steering:` message** that was never processed — a deadlock. It
only unwinds when `ws-agent-wait` eventually times out.

## Mechanism

The child→lead approval relay (`execute-gateway.ts` `createApprovalRelay`)
delivers the approval request by `pi.sendUserMessage(text, {deliverAs:"steer"})`
into the lead's own session, and the child's gated tool blocks polling for the
decision file (`waitForDecisionFile`) until the lead calls `ws-approve`. Per the
code's own note, `steer`/`followUp` are **turn-boundary-only**: a steered
message is delivered only when the lead's current turn/tool-call completes.

`ws-agent-wait` is a blocking lead tool call that does not return until the
awaited agent reports, settles, or times out. So when an agent being harvested
via `ws-agent-wait` needs approval:

1. lead is blocked inside `ws-agent-wait` (no turn boundary reached);
2. the child is blocked in `waitForDecisionFile` waiting for `ws-approve`;
3. the approval steer that would prompt `ws-approve` is queued behind the lead's
   unfinished `ws-agent-wait` turn.

Circular wait — broken only by the `ws-agent-wait` timeout.

## Why this is a real footgun, not just a bad test

`ws-execute` and `ws-fork` both return `{agent_id}` immediately (non-blocking),
and the documented harvest pattern is "spawn then `ws-agent-wait`". A model
following that natural pattern will call `ws-agent-wait` right after spawning —
exactly the blocking window. Any approval the spawned agent needs during that
window deadlocks. It bites `ws-execute` (the whole point of which is
approval-gated commands) and any `ws-fork` whose task touches the shell.

The `260905-pi-side-thread-fork-task-thread` spec section claims
approval-routing-to-the-spawning-parent "falls out free" from per-process
registration. That is structurally true but has this liveness hole; the claim
needs a caveat once this is understood/fixed.

## Fix (settled): approval-pending wake for ws-agent-wait

Chosen among three candidates (recorded so the rejected ones are not
re-litigated):

- **Chosen — `reason:"approval-pending"` wake.** Make a blocking `ws-agent-wait`
  return control to the lead the moment an awaited agent enters
  `pendingApproval`, so the lead reaches a turn boundary, can act on the queued
  approval steer / call `ws-approve`, then re-wait to harvest. Cheapest and most
  local: it reuses the existing waiter-wake machinery and is additive (a new
  return reason), breaking no existing `reason:"idle"|"report"` caller.
- Rejected — out-of-band approval UI (route child approvals to an overlay
  instead of a turn-boundary steer). Larger surface, overlaps the Phase-2
  owner-question overlay work, and does not fix the underlying "blocking wait
  swallows the turn boundary" hazard for other turn-boundary signals.
- Rejected — refuse to enter a blocking wait when a to-be-awaited agent already
  has a pending approval. Only covers the approval-already-pending-at-call case,
  not the common case where the approval arises *during* the wait.

Root cause is precise and local: `applyRpcEvent` (spawner.ts) sets
`record.pendingApproval` from the gated-exec event but, unlike the
`idlePending` (agent_settled) and report branches, never calls `settleWaiters`
— so a lead already blocked in `ws-agent-wait` is not woken, and the
turn-boundary-only approval steer queues behind the unfinished wait turn.

## Phases

### Phase 1: approval-pending wake

- In `applyRpcEvent`, after setting `record.pendingApproval`, call
  `settleWaiters(record)` so an already-blocked `ws-agent-wait` wakes.
- Add a `reason:"approval-pending"` return path to `waitForAgents`/
  `harvestWinner`, mirroring the existing `idlePending` fast-path
  (`firstIdlePendingAgentId`) and report fast-path — a new
  `firstPendingApprovalAgentId` check for a wait entered while `pendingApproval`
  is already set returns immediately. State the fast-path priority explicitly:
  the approval-pending check slots alongside the existing `alreadyIdle`/
  `alreadyReported` checks (an agent cannot be simultaneously `idlePending` and
  mid-gated-tool-call, so ordering is low-risk, but make it explicit for the
  implementer). The result surfaces the pending approval (agent_id, cmd_id,
  command/rationale) so the lead can call `ws-approve` with the matching cmd_id,
  then re-wait to harvest the eventual report.
- Update the `ws-agent-wait` registered tool description (spawner.ts ~line 1523,
  currently documents `reason (idle|report)` verbatim to the model) to include
  the new `approval-pending` reason and the approve-then-re-wait contract — not
  just the type/logic.
- Ensure the wake is edge-consumed like `idlePending` so it does not re-fire
  after `ws-approve` clears `pendingApproval`; a normal report/idle harvest
  follows unchanged.
- Update `pi-lead-guide.md` (and any execute/fork guide text) so the documented
  harvest pattern is: on `approval-pending`, approve then re-wait — never keep
  blocking.
- Offline unit tests: the `settleWaiters`-on-pendingApproval wake, the
  fast-path return, the edge-consume (no re-fire after clear), and the result
  shape carrying the cmd_id.
- Live verification (env now available, openai-codex subscription): the exact
  reproduction from this ticket — a `ws-fork` whose task runs a shell command,
  harvested via `ws-agent-wait`, and a `ws-execute` immediately followed by
  `ws-agent-wait` — must no longer deadlock; the lead is handed back, approves,
  and harvests.

### Result (10cc4c01) - 2026-09-05

Implemented as planned. `applyRpcEvent`'s `pendingApproval` branch now calls
`settleWaiters`; `waitForAgents`/`harvestWinner` gained a
`firstPendingApprovalAgentId` fast-path returning `reason:"approval-pending"`
with `pending_approval:{cmd_id,command,rationale}`, checked before idle/report.
Not edge-consumed (cleared by `ws-approve`), so a pre-approve re-wait re-reports
rather than loops; buffered reports still drain alongside. Tool description,
module header, `pi-lead-guide.md`, and the spec approval-gateway section updated.
348/348 offline tests pass (+8: settleWaiters-on-approval wake and its
no-op-when-nothing-set counterpart, `firstPendingApprovalAgentId`, the wait
fast-path, the not-edge-consumed re-wait, the mid-wait gated-exec race, and the
priority-over-report drain).

Live-verified (pi 0.84.4, openai-codex subscription): a `ws-execute` of an
unpredictable command (`od -An -N8 -tx1 /dev/urandom`, so the worker must
actually run it through the gate) followed immediately by `ws-agent-wait`
returned `reason:"approval-pending"`; the lead approved with the real `cmd_id`
and re-waited to harvest the true random bytes — no deadlock. A `ws-fork` running
the same command harvested cleanly (`reason:"report"`) as well.

**Residual (needs interactive re-check):** this fix is scoped to the adapter's
`GATED_EXEC` (`ws-worker-exec`) approval path — the one that sets
`record.pendingApproval`. The user's original interactive symptom (a fork's
`bash` producing a queued `steering:` approval) may instead be Pi's **native**
tool-approval surfacing, which the adapter does not route through
`record.pendingApproval` at all and this wake therefore does not cover. The
`--print` repros here auto-handle native approvals, so they cannot reproduce that
path; confirm on a live interactive TUI whether a fork's native-`bash` approval
still deadlocks, and if so file a follow-up for a native-approval wake.

## Spec Impact

`pi-adapter-runtime.md`: the approval-routing text (the execute-gateway section
and the side-thread-fork section's approval note) currently presents
approval-routing-to-the-spawning-parent as sufficient. Amend it to document the
blocking-wait deadlock hazard and the `reason:"approval-pending"` wake as the
resolution, and state the harvest contract (on `approval-pending`, approve then
re-wait). Spec text lands with the Phase 1 implementation per the
document-only-implemented-behavior rule.

## GOLDEN RULE

Fix must land in `agents-plugin-pi/` only. `agents-plugin-tool/` (ws-mcp Go) and
`agents-plugin/skills/` canonical text stay untouched; dependency stays
one-directional (adapter → ws-mcp).

---
title: "Pi adapter: push every child report into the lead session and retire `ws-agent-wait`"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260802-research-ws-pi-native-framework: research anchor — the spawn/continue/wait MVP vocabulary this ticket revises for Pi
  260903-feat-ws-pi-subagent-rpc-ux: introduced the persistent RPC children, `pendingReports` and the pull-style `ws-agent-wait` this ticket replaces
  260904-feat-ws-pi-side-thread-fork-question-surface: the fork-raised question notice currently tells the lead to keep polling `ws-agent-wait`; first surface to move to push
  260905-bug-ws-pi-approval-relay-deadlocks-under-agent-wait: the deadlock class that disappears once the lead no longer blocks on wait
  260905-feat-ws-pi-live-agent-widget: sibling — the always-visible running-agent list the lead and owner read instead of polling
related-mental-model:
  - plugin-runtime
spec:
  - pi-adapter-runtime
sage-review-design: recommended
---

# Pi adapter: push every child report into the lead session and retire `ws-agent-wait`

## Background

The Pi adapter's delegation surface (`260903`) is pull-based: a child's
`ws-report-to-lead` reports land in the lead-process `pendingReports` buffer
and the lead harvests them by blocking in `ws-agent-wait`, which races
`agent_settled` events and a timeout. That mirrors the ws doctrine written for
hosts without a push path (Claude, Codex). Pi has one: the adapter runs inside
the lead process, already observes every child event in `applyRpcEvent`, and
`pi.sendMessage(..., { deliverAs: "followUp", triggerTurn: true })` delivers a
custom message to the lead and starts a turn when the lead is idle — the exact
mechanism the owner-question surface already uses for its `ws-thread-summary`
injection and the execute gateway uses for approval relay.

Owner dogfood (2026-09-05) showed the cost of the mismatch: a lead parked on a
fork that had raised an owner question (`260904`'s notice says "keep waiting on
this agent (`ws-agent-wait`)") looped `ws-agent-wait` + timeout for as long as
the owner was away, burning turns to learn nothing. The `260905` approval-relay
deadlock was the same tension from the other side: a lead blocked in `wait`
cannot receive a pushed approval request, so a wake path had to be bolted on.

Structurally, a blocking wait is not needed on Pi. The lead spawns and ends its
turn; each child report arrives as a message and wakes it. This ticket makes
that the only model.

## Decisions

- **Every child report is pushed; nothing is harvested.** `kind:"final"`,
  `kind:"question"` (headless relay case), untagged progress reports, the fork
  anti-bleed advisories (idle-without-final, fail-loud transcript tail,
  `expects_commit` non-completion) and approval requests all reach the lead as
  custom messages (`customType` per family, e.g. `ws-agent-report`,
  `ws-agent-approval`) delivered `followUp` with `triggerTurn: true`, in
  arrival order. The `pendingReports` buffer and its drop-oldest cap go away
  with the harvester.
- **Fan-in stays with the model, not the adapter.** Owner decision
  (2026-09-05): each report is pushed individually and carries a status line
  (`N of M spawned agents still running: <ids>`), so a lead waiting on several
  children can tell "not yet — end the turn again" from "all in — synthesize".
  Rejected: the adapter coalescing a fan-out's reports until the last one
  settles — it would also hold back questions and approval requests, which
  must not wait.
- **`ws-agent-wait` is removed, not deprecated.** A tool that blocks the lead
  is the hazard; leaving it available keeps the timeout loop reachable. The
  approval-pending wake, the `idlePending` edge-consume flag and the waiter
  bookkeeping in `spawner.ts` are deleted with it. `ws-agent-list` remains the
  status query (running / idle / dormant, last report time) and
  `ws-agent-send` / `ws-agent-stop` / `ws-agent-transcript` are unchanged.
- **Goal loop yields to live children.** Owner decision (2026-09-05): while
  any spawned child (worker, execute-worker, explore leaf, fork) of this lead
  is live, an `agent_settled` on the lead does **not** re-inject the goal
  reminder and does not count toward the runaway streak; the pushed report
  that wakes the lead is what continues the goal. Rejected: putting a
  "waiting on N agents" line into the reminder and still re-firing — it still
  spends a turn to say "still waiting".
- **Guide text maps the doctrine, Go stays untouched.** Shared playbooks keep
  saying "wait for the reviewer"; `pi-lead-guide.md` maps that verb to "end
  your turn — the report arrives as a message", and the `260904` fork-raised
  notice drops its `ws-agent-wait` instruction. ws-mcp Go source and
  `agents-plugin/skills/` are not modified (golden rule; the spawner is
  adapter-owned).

## Constraints

- Headless lead (`--mode rpc`): `pi.sendMessage` is host-level, so push works
  there too; the `260904` §8 relay baseline for fork-raised questions becomes
  a pushed `kind:"question"` message rather than a `ws-agent-wait` return.
- A pushed message must not be lost across `/reload` or a lead session switch:
  verify Pi's followUp queue survives, or re-push on `session_start` from the
  child records that still hold an undelivered report.
- Child processes (`isChildProcess`) never push into their own session; a
  depth-1 worker's explore leaf reports to the worker through the same
  mechanism in the worker's process.

## Spec Impact

`pi-adapter-runtime`: the delegation-spawner anchor's tool list and the
"Child→lead report channel" anchor change from harvest to push (report message
families, ordering, the status line); "Turn completion is gated on RPC idle"
keeps its RPC semantics but loses the `ws-agent-wait` wording; the
lead-execute approval gateway anchor's `pending_approval` return path is
replaced by the pushed approval message; the goal-loop anchor gains the
live-children yield rule; the side-thread anchors drop the "keep waiting"
notice text.

## Phases

### Phase 1: Push channel + `ws-agent-wait` removal

In `agents-plugin-pi/src/spawner.ts`, replace the report-enqueue path with a
push: `applyRpcEvent`'s report branch, the fork loop's advisories
(`fork.ts`), and the execute gateway's approval requests
(`execute-gateway.ts`) each call `pi.sendMessage` with their family's
`customType`, `display: true`, `details` carrying `agent_id`, `kind`, and the
running-agent status line, `{ deliverAs: "followUp", triggerTurn: true }`.
Delete `ws-agent-wait`, `pendingReports`, `idlePending`, `settleWaiters`, the
approval-pending wake and their tests; keep `ws-agent-list`, extending it with
last-report time. Register `pi.registerEntryRenderer` for the report families
so a pushed report is readable in the transcript. Rewrite `pi-lead-guide.md`
(wait row → "end your turn; reports arrive as messages", approval row, fork
row) and the tool descriptions that say "harvest with ws-agent-wait".
Verify offline with `npm test` (push call shape per family, ordering, status
line arithmetic, no waiter code paths left) and live: spawn three workers,
end the turn, confirm three separate report turns each carrying the correct
`N still running`; a worker approval request wakes an idle lead; a fork-raised
question in TUI still routes to the owner overlay and its final report wakes
the lead without any wait call.

### Phase 2: Goal loop yields to live children

Depends on Phase 1. In `goal-loop.ts`, the armed `agent_settled` handler
consults the RPC registry: when any non-dormant child of this lead is live it
neither re-injects the reminder nor advances the runaway streak (record a
"yielding to N live agents" status via `ctx.ui.setStatus`). Cover with tests
for: live child → no re-fire, no streak change; all children settled/dormant →
normal re-fire; a pushed report arriving while yielding starts the turn that
continues the goal. Live check: `/goal` a task that spawns a worker and
confirm the lead does not re-fire until the worker's report lands.

## Non-goals

- Changing the shared ws doctrine or ws-mcp's own agent primitives for Claude
  or Codex.
- Coalescing or summarizing reports on the adapter side.
- The always-visible running-agent list (`260905-feat-ws-pi-live-agent-widget`).

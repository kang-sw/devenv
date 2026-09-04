---
title: "Approval-relay steer deadlocks when the spawning lead is blocked in ws-agent-wait"
related:
  - 260904-feat-ws-pi-execute-approval-gateway
  - 260904-feat-ws-pi-side-thread-fork-question-surface
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

## Direction to explore (not settled)

- Make the approval surface reachable without a lead turn boundary — e.g. route
  a spawned child's approval to a UI overlay / out-of-band prompt rather than a
  turn-boundary `steer`, so the human can answer while the lead is mid-wait.
- Or make `ws-agent-wait` yield/return when a harvested agent enters
  `pendingApproval`, handing control back to the lead so it can `ws-approve`,
  then re-wait. (A `reason:"approval-pending"` wake, mirroring the existing
  `reason:"report"` wake.)
- Or detect at spawn/wait time that a to-be-awaited agent has a pending approval
  and refuse to enter a blocking wait until it is cleared.

The second option looks cheapest and most local (it reuses the existing waiter
wake machinery) but needs design in `lead-discuss` before implementation.

## GOLDEN RULE

Fix must land in `agents-plugin-pi/` only. `agents-plugin-tool/` (ws-mcp Go) and
`agents-plugin/skills/` canonical text stay untouched; dependency stays
one-directional (adapter → ws-mcp).

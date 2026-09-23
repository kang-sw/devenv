---
title: Pi execute-worker stays running after accepted command approval
blocked-by: 260924-feat-pi-agent-channel-approval-decisions
related:
  260924-feat-pi-agent-channel-approval-decisions: prerequisite; this bug is repaired on top of channel-delivered decisions, while its incident defects (raw cmd_id vs ownership validation, code-version/path mismatch) stay diagnosed here
  260923-research-pi-parent-child-loopback-control-channel: settled channel-first sequencing
  260905-bug-ws-pi-approval-relay-deadlocks-under-agent-wait: prior resolved approval-relay deadlock under blocking wait
  260921-bug-pi-gutter-stale-pending-approval-deadlock: prior resolved stale approval state on agent death
  260914-bug-pi-execute-worker-resume-session-home-unavailable: separate execute-worker session-home failure after settlement
---

# Pi execute-worker stays running after accepted command approval

## Background

On 2026-09-23 a `ws-execute` worker was asked to delete one already-merged local implementation branch and verify Git status. Its read-only preflight (`git log --oneline --graph -50` plus branch/status checks) raised a `ws-agent-approval` request. `ws-approve` returned `{ "ok": true }`, but the worker remained `running` for over two hours without a further approval, report, or settlement. `ws-agent-send` with `interrupt: true` asking for progress returned the agent ID but produced no report. `ws-agent-transcript` returned `history unavailable — owned session home is busy, gone, or unreadable; retry the reference` both before and after `ws-agent-stop`. The lead stopped the worker; `develop` was clean, the merge had already completed, and branch deletion was not confirmed.

This is not yet diagnosed as an approval-relay deadlock: the request was approved and the failure could be in execution, event delivery, or session ownership. The older approval-relay and stale-approval tickets are closed, and the active session-home idea concerns resuming a *settled* worker rather than this live hang.

## Open Questions

- Did the accepted decision reach the waiting `ws-worker-exec` call, and was the preflight command ever started or completed?
- Why did the worker remain running without further reports or approvals, and why was its transcript unavailable even after stopping it?
- What bounded timeout or recovery behavior should a caller observe when approval succeeds but execution does not settle?

## Phases

### Phase 1: Diagnose the approved execute-worker hang

Reproduce and trace the approval decision, command execution, worker lifecycle, and session-home visibility. Settle the recovery behavior and regression coverage before promoting this idea for implementation.

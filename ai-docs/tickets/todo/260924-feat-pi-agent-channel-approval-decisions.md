---
title: Move Pi execute-approval decisions onto the parent-child control channel
blocked-by: 260924-feat-pi-agent-channel-transport
related:
  260923-research-pi-parent-child-loopback-control-channel: source research; its Outcome Ledger is this ticket's authority
  260924-feat-pi-agent-channel-transport: prerequisite transport
  260923-bug-pi-execute-approval-accepted-worker-hangs: blocked by this ticket; keeps its incident defects
---

# Move Pi execute-approval decisions onto the parent-child control channel

## Background

Today `ws-approve` in the parent writes `<home>/approvals/<encoded cmd_id>.decision.json` with a plain `writeFileSync`. The child's `ws-worker-exec` polls for that file every 200 ms (`waitForDecisionFile`, `agents-plugin-pi/src/execute-gateway.ts`), locating the directory through `WS_PI_APPROVAL_DIR`. Any parseable file at that path is consumed without authentication, so a child's own shell can approve its pending command. The file path also forced a Windows-safe filename encoding for `cmd_id`. The approval *request* already travels child-to-parent over the Pi RPC `tool_execution_start` event, and that stays unchanged.

## Decisions

- The parent sends each decision to the child as a channel message bound to its `cmd_id`. The decision file, `WS_PI_APPROVAL_DIR`, and the 200-ms poll are retired.
- The approval request stays on the Pi RPC event path.
- Approval *waiting* state need not survive a disconnected channel; after reconnect or restart, a fresh approval request is acceptable. A command whose start status is uncertain is never replayed or re-executed automatically.
- Defining how an already-started command is distinguished from an unconsumed decision across a disconnect (a "started" marker or equivalent) is part of this ticket. If the chosen mechanism would contradict the decisions above, the worker stops and escalates.
- The incident defects of `260923-bug-pi-execute-approval-accepted-worker-hangs` stay with that ticket: the raw `pendingApprovalCommandId` against `SAFE_COMPONENT` ownership validation, and the parent/child code-version and path mismatch.

## Phases

### Phase 1: Channel-delivered approval decisions

Replace the decision file with a channel message. Include whatever acknowledgment the protocol layer needs so that the parent knows the decision was consumed.

Verification:

- Approve and deny both reach the waiting `ws-worker-exec` over the channel, and no decision file or approval directory is created.
- A disconnect before consumption leads to a fresh approval request, not a lost or silently applied decision.
- A child crash or disconnect after the command has started never re-executes that command.
- Decisions for one `cmd_id` cannot satisfy a different pending command.
- The contract suite from `260924-feat-pi-agent-channel-transport` covers the approval message types on both backends.

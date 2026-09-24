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
- The child is the authority on consumption, and there is no durable "started" marker.
  - The child sends a consumption acknowledgment for a `cmd_id` before it starts that command.
  - After a reconnect, the child's hello reports which `cmd_id` it is still waiting on, if any. A decision whose `cmd_id` is not reported as pending counts as consumed.
  - If the child process dies, its pending tool call dies with it, so the command cannot be re-executed.
- Re-delivery is conservative. On disconnect, the parent discards every decision it sent that has not been acknowledged. It never re-sends that decision automatically, even when the child still reports the `cmd_id` as pending after reconnecting. Instead, the still-pending `cmd_id` goes back to the user as a fresh approval request, and only a decision sent over the new connection can be consumed.
- Ownership protection keeps its lifetime. Today `pendingApproval.decisionWritten` protects the record until the decision is consumed (5e4502e1). Its replacement protects the record until the consumption acknowledgment arrives or the child exits.
- If a mechanism above would contradict the other decisions, the worker stops and escalates.
- The incident defects of `260923-bug-pi-execute-approval-accepted-worker-hangs` stay with that ticket: the raw `pendingApprovalCommandId` against `SAFE_COMPONENT` ownership validation, and the parent/child code-version and path mismatch.

## Prior Decisions

- 260923-research-pi-parent-child-loopback-control-channel (2026-09-23, Confirmed Decisions): "Approval *waiting* state need not survive a disconnected child channel; a fresh approval request is acceptable. This does not authorize replay or automatic re-execution of a command whose start status is uncertain." — bearing: supports
- 260923-research-pi-parent-child-loopback-control-channel (2026-09-23, Confirmed Decisions): "Sequencing. The channel infrastructure lands first. ... Fixes that the channel absorbs wait for it: the non-atomic approval and web-readiness writes, approval filename encoding, ..." — bearing: supports
- 260924-feat-pi-agent-channel-transport (2026-09-24, Decisions): "Disconnect is judged by process lifecycle, not by the backend ... Acknowledgment and deduplication belong to the protocol layer, not the backend." — bearing: constrains
- 4455ec78 (2026-09-05, commit): "The parent/child decision hand-off is two independent file-polling primitives (approvalDecisionPath + waitForDecisionFile), not an RPC call: Pi's RPC surface has no channel that can resolve an in-flight tool call" — bearing: supports
- 4455ec78 (2026-09-05, commit): "computeLeadActiveTools and validatePendingApproval are the two security-relevant pure functions ... the latter is the cmd_id race-binding" — bearing: constrains
- 4455ec78 (2026-09-05, commit): "deny/run-instead/aborted outcomes inside ws-worker-exec are returned as ordinary tool-result content, not thrown: they are meaningful information the spawned model must read and act on" — bearing: constrains
- 5e4502e1 (2026-09-09, commit): "[fixed] C4 approval protection lifetime through decision consumption." — bearing: constrains
- 260921-bug-pi-approval-file-cmd-id-windows-illegal-char (2026-09-22, commit 731decce): "The implementation preserves logical cmd_ids while applying one shared injective filesystem encoding on both writer and reader paths." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/execute-gateway.ts decision writer and waitForDecisionFile reader; agents-plugin-pi/src/spawner.ts sets WS_PI_APPROVAL_DIR at L2191 and records pendingApproval at L2367; channel protocol module from 260924-feat-pi-agent-channel-transport not yet landed |
| scope.surface | cross-module | parent ws-approve and child ws-worker-exec exchange a new channel message across the transport module, execute-gateway.ts, and spawner.ts |
| scope.new_public_symbol | yes | channel approval decision and acknowledgment message types; names not yet chosen |
| scope.new_type_contract | yes | approval decision message bound to cmd_id plus consumption acknowledgment and a started marker or equivalent |
| scope.test_surface | existing | agents-plugin-pi/test/execute-gateway.test.ts and agents-plugin-pi/test/spawner.test.ts cover the decision file today; transport contract suite pending 260924-feat-pi-agent-channel-transport |
| complexity.reuse_points | unconfirmed | channel send and onMessage contract from 260924-feat-pi-agent-channel-transport is unlanded and could not be read |
| complexity.side_effect_risk | high | the gate decides whether a shell command runs, and a disconnect mishandled could re-execute or silently apply a command |
| risk.correctness | high | started versus unconsumed distinction across disconnect and crash is undesigned and must never replay a command |
| risk.fit | moderate | builds on an unlanded transport API and must retire WS_PI_APPROVAL_DIR plus pendingApproval decisionWritten protection coherently |
| risk.test | high | crash-after-start and disconnect-before-consumption need process-level scenarios on both pipe and TCP backends |
| risk.security_or_contract | high | the change owns the execute-approval authorization path and the cmd_id binding that stops cross-command approval |

## Phases

### Phase 1: Channel-delivered approval decisions

Replace the decision file with a channel message. Include whatever acknowledgment the protocol layer needs so that the parent knows the decision was consumed.

Verification:

- Approve and deny both reach the waiting `ws-worker-exec` over the channel, and no decision file or approval directory is created.
- A disconnect before consumption leads to a fresh approval request, not a lost or silently applied decision. The parent never re-sends the earlier decision automatically after reconnect.
- A child crash or disconnect after the command has started never re-executes that command.
- Decisions for one `cmd_id` cannot satisfy a different pending command.
- The contract suite from `260924-feat-pi-agent-channel-transport` covers the approval message types on both backends.

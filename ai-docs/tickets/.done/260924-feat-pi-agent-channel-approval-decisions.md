---
title: Move Pi execute-approval decisions onto the parent-child control channel
blocked-by: 260924-feat-pi-agent-channel-transport
related:
  260923-research-pi-parent-child-loopback-control-channel: source research; its Outcome Ledger is this ticket's authority
  260924-feat-pi-agent-channel-transport: prerequisite transport
  260923-bug-pi-execute-approval-accepted-worker-hangs: blocked by this ticket; keeps its incident defects
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3d0a60a4c4b011dc
sage-review-completeness-reviewed: 3d0a60a4c4b011dc
completed: 2026-09-24
---

# Move Pi execute-approval decisions onto the parent-child control channel

## Background

Today `ws-approve` in the parent writes `<home>/approvals/<encoded cmd_id>.decision.json` with a plain `writeFileSync`. The child's `ws-worker-exec` polls for that file every 200 ms (`waitForDecisionFile`, `agents-plugin-pi/src/execute-gateway.ts`), locating the directory through `WS_PI_APPROVAL_DIR`. Any parseable file at that path is consumed without authentication, so a child's own shell can approve its pending command. The file path also forced a Windows-safe filename encoding for `cmd_id`. The approval *request* already travels child-to-parent over the Pi RPC `tool_execution_start` event, and that stays unchanged.

## Decisions

- The parent sends each decision to the child as a channel message bound to its `cmd_id`. The decision file, `WS_PI_APPROVAL_DIR`, and the 200-ms poll are retired.
- The approval request stays on the Pi RPC event path.
- Approval *waiting* state need not survive a disconnected channel; after reconnect or restart, a fresh approval request is acceptable. A command whose start status is uncertain is never replayed or re-executed automatically.
- The child is the authority on consumption, and there is no durable "started" marker.
  - This ticket defines an approval-level consumption acknowledgment message. It is not a generic transport ack. The child sends it for a `cmd_id` before it starts that command.
  - If sending the acknowledgment fails because the connection has already ended, the child does not start the command. It keeps the `cmd_id` pending, and the next hello reports it.
  - The pending `cmd_id` travels in the resume section of the transport's hello.
  - After a reconnect, the child's hello reports which `cmd_id` it is still waiting on, if any. A decision whose `cmd_id` is not reported as pending counts as consumed.
  - If the child process dies, its pending tool call dies with it, so the command cannot be re-executed.
- Re-delivery is conservative. On disconnect, the parent discards every decision it sent that has not been acknowledged. It never re-sends that decision automatically, even when the child still reports the `cmd_id` as pending after reconnecting. Instead, the still-pending `cmd_id` goes back to the user as a fresh approval request, and only a decision sent over the new connection can be consumed.
- When `ws-approve` finds no live connection, it discards the decision immediately and returns an error saying the decision was not delivered. It never reports success for an undelivered decision.
- The decision file's helpers go with it: `approvalDecisionPath` and the Windows-safe `cmd_id` filename encoding have no other consumer (`agents-plugin-pi/src/execute-gateway.ts`).
- A process spawned by the child, such as its shell, has no way to deliver a decision. The decision file is gone, the transport's bootstrap values (endpoint, credential, and generation, as `260924-feat-pi-agent-channel-transport` defines them) are deleted from the environment, and the channel is the child's own client socket.
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
- 260921-bug-pi-approval-file-cmd-id-windows-illegal-char (2026-09-22, commit 731decce): "The implementation preserves logical cmd_ids while applying one shared injective filesystem encoding on both writer and reader paths." — bearing: superseded (the encoding is removed with the decision file; logical `cmd_id` preservation still holds)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/execute-gateway.ts decision writer and waitForDecisionFile reader; agents-plugin-pi/src/spawner.ts sets WS_PI_APPROVAL_DIR at L2191 and records pendingApproval at L2367; channel protocol module from 260924-feat-pi-agent-channel-transport not yet landed |
| scope.surface | cross-module | parent ws-approve and child ws-worker-exec exchange a new channel message across the transport module, execute-gateway.ts, and spawner.ts |
| scope.new_public_symbol | yes | channel approval decision and acknowledgment message types; names not yet chosen |
| scope.new_type_contract | yes | approval decision message bound to cmd_id, consumption acknowledgment, and the pending cmd_id reported in the reconnect hello; no durable started marker |
| scope.test_surface | existing | agents-plugin-pi/test/execute-gateway.test.ts and agents-plugin-pi/test/spawner.test.ts cover the decision file today; transport contract suite pending 260924-feat-pi-agent-channel-transport |
| complexity.reuse_points | unconfirmed | channel send and onMessage contract from 260924-feat-pi-agent-channel-transport is unlanded and could not be read |
| complexity.side_effect_risk | high | the gate decides whether a shell command runs, and a disconnect mishandled could re-execute or silently apply a command |
| risk.correctness | high | child-authoritative consumption, discard-on-disconnect of unacknowledged decisions, and re-asking the user must together never replay or silently apply a command |
| risk.fit | moderate | builds on an unlanded transport API and must retire WS_PI_APPROVAL_DIR plus pendingApproval decisionWritten protection coherently |
| risk.test | high | crash-after-start and disconnect-before-consumption need process-level scenarios on both pipe and TCP backends |
| risk.security_or_contract | high | the change owns the execute-approval authorization path and the cmd_id binding that stops cross-command approval |

## Phases

### Phase 1: Channel-delivered approval decisions

Replace the decision file with a channel message and add the consumption acknowledgment message. Report the pending `cmd_id` in the hello's resume section.

Verification:

- Approve and deny both reach the waiting `ws-worker-exec` over the channel, and no decision file or approval directory is created.
- A disconnect before consumption leads to a fresh approval request, not a lost or silently applied decision. After reconnect, the child's hello reports the pending `cmd_id`, and the parent never re-sends the earlier decision automatically.
- A decision sent just before a disconnect and arriving after the child reconnected is never consumed; only a decision sent over the new connection is.
- `ws-approve` with no live connection returns a not-delivered error.
- A failed acknowledgment send means the command does not start.
- On both backends, a decision from anywhere other than the parent's connection is never consumed. Test this from a shell command the child runs.
- Ownership protection holds from the approval request until the consumption acknowledgment arrives or the child exits. Sending the decision alone does not release it.
- A child crash or disconnect after the command has started never re-executes that command.
- Decisions for one `cmd_id` cannot satisfy a different pending command.
- The contract suite from `260924-feat-pi-agent-channel-transport` covers the approval message types on both backends.

### Result (306197ba) - 2026-09-24

Landed on `impl/develop/thumb-tiger-bolt` as `0e3cb0e3`, `e1094fb0`, `f1bc0dcd`, `306197ba`.

What changed:

- New `agents-plugin-pi/src/approval-protocol.ts`: `approval-decision` (parent to child, bound to `cmd_id`), `approval-consumed` (child to parent), the `approval` hello resume key carrying the pending `cmd_id`, and `ChildApprovalGate`, the child-side authority on consumption. The gate sends the consumption acknowledgment before it lets the command start; an acknowledgment send that throws leaves the `cmd_id` pending and the command unstarted. A decision that arrives before the wait opens is kept per `cmd_id` (bounded, latest wins) so a fast lead is not lost.
- `execute-gateway.ts`: `ws-approve` sends the decision over the parent's live channel and marks the request `sent`; with no live connection, or when the send throws, it marks the request `discarded` and returns a not-delivered error. `ws-worker-exec` awaits the gate instead of polling a file. `approvalDecisionPath`, the Windows-safe filename encoding, `waitForDecisionFile`, `WS_PI_APPROVAL_DIR`, and the 200 ms poll are deleted; the child receives no approval directory in its environment.
- `spawner.ts`: `attachApprovalChannel` reconciles the parent's pending request against the channel. An acknowledgment releases the request and its ownership protection. A disconnect turns a `sent` decision into `discarded`; the parent never re-sends. On the reconnect hello, a `discarded` request whose `cmd_id` is still reported pending is re-issued to the user as a fresh approval request (with a note that the earlier decision was discarded); one that is not reported counts as consumed and is released. `heldActionState` treats `sent`/`discarded` as superseded, so a stale ws-approve cannot fire while the decision is in flight.
- `index.ts` wires the gate's resume section into every child hello and attaches the gate to the channel.

Verification:

- `npm test` in `agents-plugin-pi/`: 1725 tests, 1723 pass, 0 fail, 2 skipped (pre-existing platform skips), exit 0.
- `test/agent-channel.integration.test.ts`: 9 pass, including the new `[pipe]` and `[tcp]` forgery probes, which run a real execute-worker whose shell command confirms no `WS_PI_APPROVAL*`/`WS_PI_CHANNEL_*` variables reach it, then tries raw frames (rejected `malformed`), a hello with the credential (rejected `busy`), a hello without it (rejected `auth`), and the legacy decision file; the parent records one accepted connection, no acknowledgment, and the pending request untouched.
- Registered-tool harness in `test/execute-gateway.test.ts` runs a real `ParentChannel`/`ChildChannel` pair with reconnect: approve, deny, and run-instead with no decision directory; drop before a decision (discarded, re-issued once on reconnect, nothing re-sent, one execution); failed acknowledgment send (nothing runs, parent re-asks, only the fresh decision runs, once); lost acknowledgment (one execution, parent releases, later ws-approve rejected); duplicate and foreign `cmd_id` never satisfy the pending one.
- Contract suite (`test/agent-channel.test.ts`) carries the approval case per backend (pipe and TCP): consumption only after the acknowledgment is sent over this connection, and the reconnect hello reports what is still waiting.
- Windows (Node 24.15.0, native): the contract suite including the approval case on named pipes and TCP, `approval-protocol.test.ts`, and `execute-gateway.test.ts` pass. `spawner.test.ts` was not verified there (it hangs on that host before reaching the new cases; pre-existing).
- Independent review, partitioned (correctness/fit/test), two rounds: fit clean; correctness round 2 clean; test round 2 leaves one Important (I1) that is resolved by the structural note below, plus minors listed as gaps.

Decisions taken during implementation:

- An undelivered ws-approve marks the request `discarded` rather than leaving it open, so that the reconnect path re-issues it; the ticket's "discards immediately" for the no-connection case therefore has the same shape as the disconnect case.
- Reconciliation lives in `spawner.ts` (the parent record owner) so `execute-gateway.ts` keeps importing from `spawner.ts` only, never the reverse; the protocol module imports from neither.
- The `PendingApproval` alias stays as a type export for callers that named it before.

Gaps and structural notes:

- Verification item 3 ("a decision sent just before a disconnect and arriving after the child reconnected") is structurally unreachable rather than tested: the child reconnects only from the old connection's `end` event, a socket's data always precedes its own `end`, and the parent sends only over the connection it holds; so no old-connection frame can be delivered after the new connection is up. `ChildChannel` filters inbound frames by generation, not by connection identity, which is why this property rests on stream ordering. The tested neighbour is a decision delivered before the drop whose acknowledgment failed: it is never consumed after reconnect, only the new connection's decision runs.
- Item 6's child-side half is vacuous in the process-level probe: a real pending `ws-worker-exec` in a live worker needs a model turn that tests do not make, so the probe verifies channel rejection and the parent-side state, and the child-side gate's "only this connection" rule is covered in-process by the contract case.
- Not covered by tests: `ws-approve`'s own ownership touch (the harness record carries no `ownership`); `heldActionState`'s superseded treatment of `sent`/`discarded`; `channel.send` throwing on a still-live connection (same `notDelivered` path as no-connection, which is tested); a process-level crash after start (the in-process harness shows the parent never re-sends, and a dead child cannot re-execute).
- Observation from correctness review: an early-kept decision that survives a drop before the wait opens is released by the parent on the reconnect hello (nothing reported pending) slightly before the child actually consumes it. This follows the ticket's "not reported means consumed" rule; noted for a follow-up if it matters.
- Follow-up candidate (not in scope): the child does not authenticate the parent on a reconnect welcome, and on Linux a same-user process can read the credential from `/proc/<pid>/environ`; the forgery probe shows the shell cannot deliver a decision with the credential it can find, because the parent rejects a second hello as `busy` while the child holds the connection.
